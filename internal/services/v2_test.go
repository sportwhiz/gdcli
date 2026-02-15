package services

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	apperr "github.com/sportwhiz/gdcli/internal/errors"
	"github.com/sportwhiz/gdcli/internal/godaddy"
)

type fakeV2Client struct {
	fakeClient
	v2DetailErr       error
	v2NSErr           error
	v2RenewErr        error
	v2Detail          map[string]any
	lastRenewV2       godaddy.RenewV2Request
	requireCustomerID string
	v1RenewErr        error
}

func (f *fakeV2Client) ResolveCustomerID(ctx context.Context, shopperID string) (string, error) {
	return "cust-123", nil
}

func (f *fakeV2Client) DomainDetailV2(ctx context.Context, customerID, domain string, includes []string) (map[string]any, error) {
	if f.v2DetailErr != nil {
		return nil, f.v2DetailErr
	}
	if f.requireCustomerID != "" && customerID != f.requireCustomerID {
		return nil, errors.New("customer mismatch")
	}
	if f.v2Detail != nil {
		return f.v2Detail, nil
	}
	return map[string]any{"domain": domain, "source": "v2", "nameServers": []any{"ns1.afternic.com", "ns2.afternic.com"}}, nil
}

func (f *fakeV2Client) DomainDetailV1(ctx context.Context, domain string) (map[string]any, error) {
	return map[string]any{"domain": domain, "source": "v1"}, nil
}

func (f *fakeV2Client) RenewV2(ctx context.Context, customerID, domain string, req godaddy.RenewV2Request, idempotencyKey string) (godaddy.RenewResult, error) {
	f.lastRenewV2 = req
	if f.requireCustomerID != "" && customerID != f.requireCustomerID {
		return godaddy.RenewResult{}, errors.New("customer mismatch")
	}
	if f.v2RenewErr != nil {
		return godaddy.RenewResult{}, f.v2RenewErr
	}
	return godaddy.RenewResult{Domain: domain, Price: 12.99, Currency: "USD"}, nil
}

func (f *fakeV2Client) Renew(ctx context.Context, domain string, years int, idempotencyKey string) (godaddy.RenewResult, error) {
	if f.v1RenewErr != nil {
		return godaddy.RenewResult{}, f.v1RenewErr
	}
	return f.fakeClient.Renew(ctx, domain, years, idempotencyKey)
}

func (f *fakeV2Client) SetNameserversV2(ctx context.Context, customerID, domain string, nameservers []string) error {
	return f.v2NSErr
}

func (f *fakeV2Client) V2Get(ctx context.Context, path string, query url.Values, out any) error {
	return nil
}

func (f *fakeV2Client) V2Post(ctx context.Context, path string, body any, out any, idempotencyKey string) error {
	return nil
}

func (f *fakeV2Client) V2Put(ctx context.Context, path string, body any, out any) error {
	return nil
}

func (f *fakeV2Client) V2Patch(ctx context.Context, path string, body any, out any) error {
	return nil
}

func TestResolveAndStoreCustomerID(t *testing.T) {
	rt := makeRuntime(t)
	svc := New(rt, &fakeV2Client{})

	got, err := svc.ResolveAndStoreCustomerID(context.Background(), "123456789")
	if err != nil {
		t.Fatalf("resolve customer id: %v", err)
	}
	if got != "cust-123" || rt.Cfg.CustomerID != "cust-123" || rt.Cfg.ShopperID != "123456789" {
		t.Fatalf("unexpected identity state: got=%q cfg=%+v", got, rt.Cfg)
	}
}

func TestDomainDetailFallsBackToV1(t *testing.T) {
	rt := makeRuntime(t)
	rt.Cfg.CustomerID = "cust-123"
	svc := New(rt, &fakeV2Client{v2DetailErr: errors.New("v2 failed")})

	out, err := svc.DomainDetail(context.Background(), "example.com", nil)
	if err != nil {
		t.Fatalf("domain detail: %v", err)
	}
	if out["_api_version"] != "v1" {
		t.Fatalf("expected v1 fallback, got %v", out["_api_version"])
	}
}

func TestSetNameserversSmartFallsBackToV1(t *testing.T) {
	rt := makeRuntime(t)
	rt.Cfg.CustomerID = "cust-123"
	svc := New(rt, &fakeV2Client{v2NSErr: errors.New("v2 ns failed")})

	apiVersion, err := svc.SetNameserversSmart(context.Background(), "example.com", []string{"ns1.afternic.com", "ns2.afternic.com"})
	if err != nil {
		t.Fatalf("set nameservers smart: %v", err)
	}
	if apiVersion != "v1" {
		t.Fatalf("expected v1 fallback, got %q", apiVersion)
	}
}

func TestPortfolioWithNameservers(t *testing.T) {
	rt := makeRuntime(t)
	rt.Cfg.CustomerID = "cust-123"
	svc := New(rt, &fakeV2Client{})

	rows, err := svc.PortfolioWithNameservers(context.Background(), 0, "", "", 2)
	if err != nil {
		t.Fatalf("portfolio with nameservers: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if !rows[0].Success || len(rows[0].NameServers) != 2 || rows[0].APIVersion != "v2" {
		t.Fatalf("unexpected row: %+v", rows[0])
	}
}

func TestRenewV2BuildsConsentRequest(t *testing.T) {
	rt := makeRuntime(t)
	rt.Cfg.CustomerID = "cust-123"
	fc := &fakeV2Client{
		v2Detail: map[string]any{
			"domain":    "example.com",
			"expiresAt": "2026-05-27T15:01:38.000Z",
			"renewal": map[string]any{
				"price":    float64(10990000),
				"currency": "USD",
			},
		},
	}
	svc := New(rt, fc)

	out, err := svc.Renew(context.Background(), "example.com", 1, false, true)
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	if out["api_version"] != "v2" {
		t.Fatalf("expected v2 renew path, got %v", out["api_version"])
	}
	if fc.lastRenewV2.Expires == "" || fc.lastRenewV2.Consent.Price != 10990000 {
		t.Fatalf("unexpected renew v2 request: %+v", fc.lastRenewV2)
	}
	if fc.lastRenewV2.Consent.AgreedBy == "" || fc.lastRenewV2.Consent.AgreedAt == "" {
		t.Fatalf("missing renew consent metadata: %+v", fc.lastRenewV2.Consent)
	}
}

func TestRenewFallsBackToV1WhenV2PayloadUnavailable(t *testing.T) {
	rt := makeRuntime(t)
	rt.Cfg.CustomerID = "cust-123"
	svc := New(rt, &fakeV2Client{
		v2Detail: map[string]any{
			"domain":    "example.com",
			"expiresAt": "2026-05-27T15:01:38.000Z",
			"renewal": map[string]any{
				"currency": "USD",
			},
		},
	})

	out, err := svc.Renew(context.Background(), "example.com", 1, false, true)
	if err != nil {
		t.Fatalf("renew fallback: %v", err)
	}
	if out["api_version"] != "v1" {
		t.Fatalf("expected v1 fallback, got %v", out["api_version"])
	}
}

func TestRenewV2FallsBackToShopperIDCustomerCandidate(t *testing.T) {
	rt := makeRuntime(t)
	rt.Cfg.CustomerID = "cust-uuid"
	rt.Cfg.ShopperID = "660323812"
	fc := &fakeV2Client{
		requireCustomerID: "660323812",
		v2Detail: map[string]any{
			"domain":    "example.com",
			"expiresAt": "2026-05-27T15:01:38.000Z",
			"renewal": map[string]any{
				"price":    float64(10990000),
				"currency": "USD",
			},
		},
	}
	svc := New(rt, fc)

	out, err := svc.Renew(context.Background(), "example.com", 1, false, true)
	if err != nil {
		t.Fatalf("renew via shopper-id fallback: %v", err)
	}
	if out["api_version"] != "v2" {
		t.Fatalf("expected v2 renew path, got %v", out["api_version"])
	}
}

func TestRenewReturnsLatestV1PaymentErrorAndGuidance(t *testing.T) {
	rt := makeRuntime(t)
	rt.Cfg.CustomerID = "cust-123"
	rt.Cfg.ShopperID = "660323812"
	svc := New(rt, &fakeV2Client{
		v2RenewErr: errors.New("v2 not implemented"),
		v2Detail: map[string]any{
			"domain":    "example.com",
			"expiresAt": "2026-05-27T15:01:38.000Z",
			"renewal": map[string]any{
				"price":    float64(10990000),
				"currency": "USD",
			},
		},
		v1RenewErr: &apperr.AppError{
			Code:    apperr.CodeProvider,
			Message: "provider returned non-success status",
			Details: map[string]any{
				"status": 402,
				"provider": map[string]any{
					"code":    "INVALID_PAYMENT_INFO",
					"message": "Unable to authorize credit based on specified payment information",
				},
			},
		},
	})

	_, err := svc.Renew(context.Background(), "example.com", 1, false, true)
	if err == nil {
		t.Fatalf("expected renew error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "good as gold") {
		t.Fatalf("expected good as gold guidance, got: %v", err)
	}
}
