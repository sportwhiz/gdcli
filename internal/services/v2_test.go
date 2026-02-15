package services

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/sportwhiz/gdcli/internal/godaddy"
)

type fakeV2Client struct {
	fakeClient
	v2DetailErr error
	v2NSErr     error
}

func (f *fakeV2Client) ResolveCustomerID(ctx context.Context, shopperID string) (string, error) {
	return "cust-123", nil
}

func (f *fakeV2Client) DomainDetailV2(ctx context.Context, customerID, domain string, includes []string) (map[string]any, error) {
	if f.v2DetailErr != nil {
		return nil, f.v2DetailErr
	}
	return map[string]any{"domain": domain, "source": "v2", "nameServers": []any{"ns1.afternic.com", "ns2.afternic.com"}}, nil
}

func (f *fakeV2Client) DomainDetailV1(ctx context.Context, domain string) (map[string]any, error) {
	return map[string]any{"domain": domain, "source": "v1"}, nil
}

func (f *fakeV2Client) RenewV2(ctx context.Context, customerID, domain string, years int, idempotencyKey string) (godaddy.RenewResult, error) {
	return godaddy.RenewResult{Domain: domain, Price: 12.99, Currency: "USD"}, nil
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

	got, err := svc.ResolveAndStoreCustomerID(context.Background(), "660323812")
	if err != nil {
		t.Fatalf("resolve customer id: %v", err)
	}
	if got != "cust-123" || rt.Cfg.CustomerID != "cust-123" || rt.Cfg.ShopperID != "660323812" {
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
