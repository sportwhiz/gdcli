package services

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sportwhiz/gdcli/internal/app"
	"github.com/sportwhiz/gdcli/internal/config"
	"github.com/sportwhiz/gdcli/internal/godaddy"
	"github.com/sportwhiz/gdcli/internal/store"
)

type fakeClient struct{}

func (f *fakeClient) Suggest(ctx context.Context, query string, tlds []string, limit int) ([]godaddy.Suggestion, error) {
	return []godaddy.Suggestion{{Domain: "example.com", Score: 0.9}}, nil
}
func (f *fakeClient) Available(ctx context.Context, domain string) (godaddy.Availability, error) {
	if domain == "taken.com" {
		return godaddy.Availability{Domain: domain, Available: false, Price: 0, Currency: "USD"}, nil
	}
	return godaddy.Availability{Domain: domain, Available: true, Price: 12.99, Currency: "USD"}, nil
}
func (f *fakeClient) AvailableBulk(ctx context.Context, domains []string) ([]godaddy.Availability, error) {
	out := make([]godaddy.Availability, 0, len(domains))
	for _, d := range domains {
		out = append(out, godaddy.Availability{Domain: d, Available: true, Price: 12.99, Currency: "USD"})
	}
	return out, nil
}
func (f *fakeClient) Purchase(ctx context.Context, domain string, years int, idempotencyKey string) (godaddy.PurchaseResult, error) {
	return godaddy.PurchaseResult{Domain: domain, Price: 12.99 * float64(years), Currency: "USD", OrderID: "order-1"}, nil
}
func (f *fakeClient) Renew(ctx context.Context, domain string, years int, idempotencyKey string) (godaddy.RenewResult, error) {
	return godaddy.RenewResult{Domain: domain, Price: 12.99, Currency: "USD", OrderID: "renew-1"}, nil
}
func (f *fakeClient) ListDomains(ctx context.Context) ([]godaddy.PortfolioDomain, error) {
	return []godaddy.PortfolioDomain{{Domain: "alpha.com", Expires: time.Now().AddDate(0, 0, 10).Format("2006-01-02")}}, nil
}
func (f *fakeClient) ListOrders(ctx context.Context, limit, offset int) (godaddy.OrdersPage, error) {
	return godaddy.OrdersPage{
		Orders: []godaddy.Order{
			{
				OrderID:   "o-1",
				CreatedAt: "2026-01-01T00:00:00Z",
				Currency:  "USD",
				Items:     []godaddy.OrderItem{{Label: ".COM Domain Name Registration"}},
				Pricing:   godaddy.OrderPricing{Total: 10.69, TotalRaw: 10690000, TotalUnit: "micros"},
			},
		},
		Pagination: godaddy.Pagination{Total: 1, Limit: limit, Offset: offset},
	}, nil
}
func (f *fakeClient) ListSubscriptions(ctx context.Context, limit, offset int) (godaddy.SubscriptionsPage, error) {
	return godaddy.SubscriptionsPage{
		Subscriptions: []godaddy.Subscription{
			{
				SubscriptionID: "s-1",
				Status:         "ACTIVE",
				Label:          "EXAMPLE.COM",
				CreatedAt:      "2026-01-01T00:00:00Z",
				ExpiresAt:      "2027-01-01T00:00:00Z",
				Renewable:      true,
				RenewAuto:      true,
				Product:        godaddy.SubscriptionProduct{Namespace: "domain", ProductGroupKey: "domains"},
				Billing:        godaddy.SubscriptionBilling{Status: "CURRENT", RenewAt: "2027-01-01T00:00:00Z"},
			},
		},
		Pagination: godaddy.Pagination{Total: 1, Limit: limit, Offset: offset},
	}, nil
}
func (f *fakeClient) GetNameservers(ctx context.Context, domain string) ([]string, error) {
	return []string{"ns1.afternic.com", "ns2.afternic.com"}, nil
}
func (f *fakeClient) GetRecords(ctx context.Context, domain string) ([]godaddy.DNSRecord, error) {
	return []godaddy.DNSRecord{{Type: "A", Name: "@", Data: "1.2.3.4"}, {Type: "TXT", Name: "@", Data: "verify=ok"}}, nil
}
func (f *fakeClient) SetNameservers(ctx context.Context, domain string, nameservers []string) error {
	return nil
}
func (f *fakeClient) SetRecords(ctx context.Context, domain string, records []godaddy.DNSRecord) error {
	return nil
}

type flakyPurchaseClient struct {
	fakeClient
	purchaseCalls int
}

func (f *flakyPurchaseClient) Purchase(ctx context.Context, domain string, years int, idempotencyKey string) (godaddy.PurchaseResult, error) {
	f.purchaseCalls++
	if f.purchaseCalls <= 3 {
		return godaddy.PurchaseResult{}, io.ErrUnexpectedEOF
	}
	return godaddy.PurchaseResult{Domain: domain, Price: 12.99 * float64(years), Currency: "USD", OrderID: "order-2"}, nil
}

type eurRenewClient struct {
	fakeClient
}

func (f *eurRenewClient) Renew(ctx context.Context, domain string, years int, idempotencyKey string) (godaddy.RenewResult, error) {
	return godaddy.RenewResult{Domain: domain, Price: 12.99, Currency: "EUR", OrderID: "renew-eur"}, nil
}

func makeRuntime(t *testing.T) *app.Runtime {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	rt, err := app.NewRuntime(context.Background(), os.Stdout, os.Stderr, true, false, true, "req-test")
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	return rt
}

func TestPurchaseDryRunAndConfirm(t *testing.T) {
	rt := makeRuntime(t)
	svc := New(rt, &fakeClient{})

	dry, err := svc.PurchaseDryRun(context.Background(), "example.com", 1)
	if err != nil {
		t.Fatalf("purchase dry run: %v", err)
	}
	tok, _ := dry["confirmation_token"].(string)
	if tok == "" {
		t.Fatalf("expected confirmation token")
	}

	res, err := svc.PurchaseConfirm(context.Background(), "example.com", tok, 1)
	if err != nil {
		t.Fatalf("purchase confirm: %v", err)
	}
	if res.OrderID == "" {
		t.Fatalf("expected order id")
	}
}

func TestAvailabilityBulkConcurrent(t *testing.T) {
	rt := makeRuntime(t)
	svc := New(rt, &fakeClient{})
	out, err := svc.AvailabilityBulkConcurrent(context.Background(), []string{"one.com", "two.com", "three.com"}, 2)
	if err != nil {
		t.Fatalf("availability bulk: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 results")
	}
	if !out[0].Success || !out[1].Success || !out[2].Success {
		t.Fatalf("expected all successes")
	}
}

func TestOrdersList(t *testing.T) {
	rt := makeRuntime(t)
	svc := New(rt, &fakeClient{})
	out, err := svc.OrdersList(context.Background(), 5, 0)
	if err != nil {
		t.Fatalf("orders list: %v", err)
	}
	orders, ok := out["orders"].([]godaddy.Order)
	if !ok || len(orders) != 1 {
		t.Fatalf("expected one order")
	}
	if orders[0].Pricing.Total != 10.69 {
		t.Fatalf("expected normalized total 10.69, got %v", orders[0].Pricing.Total)
	}
}

func TestSubscriptionsList(t *testing.T) {
	rt := makeRuntime(t)
	svc := New(rt, &fakeClient{})
	out, err := svc.SubscriptionsList(context.Background(), 5, 0)
	if err != nil {
		t.Fatalf("subscriptions list: %v", err)
	}
	subs, ok := out["subscriptions"].([]godaddy.Subscription)
	if !ok || len(subs) != 1 {
		t.Fatalf("expected one subscription")
	}
	if subs[0].SubscriptionID != "s-1" {
		t.Fatalf("unexpected subscription id %q", subs[0].SubscriptionID)
	}
}

func TestAppendOperationWarningOnFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var errBuf bytes.Buffer
	rt, err := app.NewRuntime(context.Background(), io.Discard, &errBuf, true, false, true, "req-test")
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}

	cfgDir, err := config.HomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	if err := os.RemoveAll(cfgDir); err != nil {
		t.Fatalf("remove cfg dir: %v", err)
	}
	if err := os.WriteFile(cfgDir, []byte("not-a-dir"), 0o600); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}

	svc := New(rt, &fakeClient{})
	svc.appendOperationWithWarning(store.Operation{
		OperationID: "op-fail",
		Type:        "purchase",
		Domain:      "example.com",
		Amount:      12.99,
		Currency:    "USD",
		CreatedAt:   time.Now(),
		Status:      "succeeded",
	})

	got := errBuf.String()
	if !strings.Contains(got, "warning: failed writing operation log for operation_id=op-fail") {
		t.Fatalf("expected warning in stderr, got %q", got)
	}
}

func TestPurchaseConfirmTokenReusableAfterTransientFailure(t *testing.T) {
	rt := makeRuntime(t)
	svc := New(rt, &flakyPurchaseClient{})

	dry, err := svc.PurchaseDryRun(context.Background(), "example.com", 1)
	if err != nil {
		t.Fatalf("purchase dry run: %v", err)
	}
	tok, _ := dry["confirmation_token"].(string)
	if tok == "" {
		t.Fatalf("expected confirmation token")
	}

	if _, err := svc.PurchaseConfirm(context.Background(), "example.com", tok, 1); err == nil {
		t.Fatalf("expected first confirm to fail")
	}

	res, err := svc.PurchaseConfirm(context.Background(), "example.com", tok, 1)
	if err != nil {
		t.Fatalf("expected retry with same token to succeed: %v", err)
	}
	if res.OrderID == "" {
		t.Fatalf("expected order id on retry")
	}
}

func TestRenewRejectsNonUSDProviderPrice(t *testing.T) {
	rt := makeRuntime(t)
	svc := New(rt, &eurRenewClient{})

	_, err := svc.Renew(context.Background(), "example.com", 1, false, true)
	if err == nil {
		t.Fatalf("expected non-USD renew to fail budget policy")
	}
}
