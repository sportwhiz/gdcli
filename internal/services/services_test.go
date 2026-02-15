package services

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sportwhiz/gdcli/internal/app"
	"github.com/sportwhiz/gdcli/internal/godaddy"
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
