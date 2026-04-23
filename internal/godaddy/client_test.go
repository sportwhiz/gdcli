package godaddy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperr "github.com/sportwhiz/gdcli/internal/errors"
)

func TestNormalizeProviderPriceMicros(t *testing.T) {
	price, raw, unit := normalizeProviderPrice(float64(9_990_000))
	if price != 9.99 {
		t.Fatalf("expected 9.99, got %v", price)
	}
	if raw != 9_990_000 {
		t.Fatalf("expected raw 9990000, got %v", raw)
	}
	if unit != "micros" {
		t.Fatalf("expected micros unit, got %q", unit)
	}
}

func TestNormalizeProviderPriceUSD(t *testing.T) {
	price, raw, unit := normalizeProviderPrice(float64(12.99))
	if price != 12.99 {
		t.Fatalf("expected 12.99, got %v", price)
	}
	if raw != 12.99 {
		t.Fatalf("expected raw 12.99, got %v", raw)
	}
	if unit != "usd" {
		t.Fatalf("expected usd unit, got %q", unit)
	}
}

func TestNormalizeAvailabilityIncludesPriceMetadata(t *testing.T) {
	in := availabilityAPI{
		Domain:     "example.org",
		Available:  true,
		Definitive: true,
		Price:      float64(9_990_000),
		Currency:   "USD",
	}
	out := normalizeAvailability(in)
	if out.Price != 9.99 {
		t.Fatalf("expected normalized price 9.99, got %v", out.Price)
	}
	if out.PriceRaw != 9_990_000 {
		t.Fatalf("expected raw price 9990000, got %v", out.PriceRaw)
	}
	if out.PriceUnit != "micros" {
		t.Fatalf("expected price unit micros, got %q", out.PriceUnit)
	}
	if !out.Definitive {
		t.Fatalf("expected definitive true")
	}
}

func TestListOrdersNormalizesPricingAndPagination(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"orders":[{"orderId":"3938269704","createdAt":"2025-11-05T12:37:45.000Z","currency":"USD","items":[{"label":".COM Domain Name Registration - 1 Year (recurring)"}],"pricing":{"total":10690000}}],"pagination":{"first":"f","last":"l","next":"n","total":9}}`))
	}))
	defer srv.Close()

	c, err := NewHTTPClient(srv.URL, "k", "s")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	out, err := c.ListOrders(context.Background(), 5, 10)
	if err != nil {
		t.Fatalf("list orders: %v", err)
	}
	if gotQuery != "limit=5&offset=10" && gotQuery != "offset=10&limit=5" {
		t.Fatalf("expected limit/offset query, got %q", gotQuery)
	}
	if len(out.Orders) != 1 {
		t.Fatalf("expected one order")
	}
	if out.Orders[0].Pricing.Total != 10.69 {
		t.Fatalf("expected normalized 10.69, got %v", out.Orders[0].Pricing.Total)
	}
	if out.Orders[0].Pricing.TotalRaw != 10690000 {
		t.Fatalf("expected raw 10690000, got %v", out.Orders[0].Pricing.TotalRaw)
	}
	if out.Orders[0].Pricing.TotalUnit != "micros" {
		t.Fatalf("expected micros, got %q", out.Orders[0].Pricing.TotalUnit)
	}
	if out.Pagination.Total != 9 || out.Pagination.Limit != 5 || out.Pagination.Offset != 10 {
		t.Fatalf("unexpected pagination: %+v", out.Pagination)
	}
}

func TestListSubscriptionsMapsFieldsAndPagination(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"subscriptions":[{"subscriptionId":"757644825:2","status":"ACTIVE","label":"EXAMPLE.COM","createdAt":"2025-11-05T12:37:46.560Z","expiresAt":"2026-11-05T14:37:57.000Z","renewable":true,"renewAuto":true,"product":{"productGroupKey":"domains","namespace":"domain"},"billing":{"status":"CURRENT","renewAt":"2026-11-06T14:37:57.000Z"}}],"pagination":{"first":"f","last":"l","next":"n","total":22}}`))
	}))
	defer srv.Close()

	c, err := NewHTTPClient(srv.URL, "k", "s")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	out, err := c.ListSubscriptions(context.Background(), 7, 14)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if gotQuery != "limit=7&offset=14" && gotQuery != "offset=14&limit=7" {
		t.Fatalf("expected limit/offset query, got %q", gotQuery)
	}
	if len(out.Subscriptions) != 1 {
		t.Fatalf("expected one subscription")
	}
	if out.Subscriptions[0].SubscriptionID != "757644825:2" {
		t.Fatalf("unexpected subscription id %q", out.Subscriptions[0].SubscriptionID)
	}
	if out.Subscriptions[0].Product.Namespace != "domain" || out.Subscriptions[0].Product.ProductGroupKey != "domains" {
		t.Fatalf("unexpected product mapping: %+v", out.Subscriptions[0].Product)
	}
	if out.Pagination.Total != 22 || out.Pagination.Limit != 7 || out.Pagination.Offset != 14 {
		t.Fatalf("unexpected pagination: %+v", out.Pagination)
	}
}

func TestResponseLimitFor(t *testing.T) {
	if got := responseLimitFor(http.MethodPost, "/v1/domains/available"); got != bulkResponseLimitBytes {
		t.Fatalf("expected bulk cap for available bulk, got %d", got)
	}
	if got := responseLimitFor(http.MethodGet, "/v1/orders?limit=5&offset=0"); got != bulkResponseLimitBytes {
		t.Fatalf("expected bulk cap for orders, got %d", got)
	}
	if got := responseLimitFor(http.MethodGet, "/v1/domains/available?domain=example.com"); got != smallResponseLimitBytes {
		t.Fatalf("expected small cap for single availability, got %d", got)
	}
}

func TestDoRejectsOversizedSingleResponse(t *testing.T) {
	large := strings.Repeat("A", 3<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"domain":"example.com","available":true,"price":12.99,"currency":"%s"}`, large)))
	}))
	defer srv.Close()

	c, err := NewHTTPClient(srv.URL, "k", "s")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := c.Available(context.Background(), "example.com"); err == nil {
		t.Fatalf("expected oversized response error")
	}
}

func TestDoAllowsLargeBulkResponseUnderBulkCap(t *testing.T) {
	large := strings.Repeat("B", 3<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/domains/available" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(fmt.Sprintf(`[{"domain":"bulk.com","available":true,"price":12.99,"currency":"%s"}]`, large)))
	}))
	defer srv.Close()

	c, err := NewHTTPClient(srv.URL, "k", "s")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	out, err := c.AvailableBulk(context.Background(), []string{"bulk.com"})
	if err != nil {
		t.Fatalf("expected bulk response to pass under bulk cap: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one item")
	}
}

func TestPurchaseIncludesConsentBlock(t *testing.T) {
	var purchaseBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/domains/agreements":
			if got := r.URL.Query().Get("tlds"); got != "com" {
				t.Fatalf("expected tlds=com, got %q", got)
			}
			_, _ = w.Write([]byte(`[{"agreementKey":"DNRA","title":"Domain Name Registration Agreement"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/domains/purchase":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &purchaseBody)
			_, _ = w.Write([]byte(`{"domain":"example.com","price":12.99,"currency":"USD","order_id":"o1"}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	t.Setenv("GDCLI_CONSENT_AGREED_BY", "203.0.113.9")
	c, err := NewHTTPClient(srv.URL, "k", "s")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := c.Purchase(context.Background(), "example.com", 1, "op-1"); err != nil {
		t.Fatalf("purchase: %v", err)
	}
	consent, ok := purchaseBody["consent"].(map[string]any)
	if !ok {
		t.Fatalf("expected consent block in body, got %#v", purchaseBody)
	}
	keys, _ := consent["agreementKeys"].([]any)
	if len(keys) != 1 || keys[0] != "DNRA" {
		t.Fatalf("expected agreementKeys [DNRA], got %v", keys)
	}
	if consent["agreedBy"] != "203.0.113.9" {
		t.Fatalf("expected agreedBy override, got %v", consent["agreedBy"])
	}
	if consent["agreedAt"] == "" || consent["agreedAt"] == nil {
		t.Fatalf("expected agreedAt to be set")
	}
	if purchaseBody["domain"] != "example.com" || purchaseBody["period"].(float64) != 1 {
		t.Fatalf("unexpected purchase body: %#v", purchaseBody)
	}
}

func TestPurchaseFailsWhenNoAgreementsReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/domains/agreements" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		t.Fatalf("should not reach purchase endpoint without agreements; got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	c, err := NewHTTPClient(srv.URL, "k", "s")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := c.Purchase(context.Background(), "example.com", 1, "op-1"); err == nil {
		t.Fatalf("expected error when registrar returns no agreements")
	}
}

func TestTldFromDomain(t *testing.T) {
	cases := map[string]string{
		"example.com":     "com",
		"Example.COM":     "com",
		"sub.example.net": "net",
		"example":         "",
		"":                "",
	}
	for in, want := range cases {
		if got := tldFromDomain(in); got != want {
			t.Errorf("tldFromDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDoHandlesOversizedErrorBody(t *testing.T) {
	large := strings.Repeat("C", 2<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"message":"%s"}`, large)))
	}))
	defer srv.Close()

	c, err := NewHTTPClient(srv.URL, "k", "s")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = c.Available(context.Background(), "example.com")
	if err == nil {
		t.Fatalf("expected error")
	}
	var ae *apperr.AppError
	if !apperr.As(err, &ae) {
		t.Fatalf("expected app error, got %T", err)
	}
	if ae.Code != apperr.CodeRateLimited {
		t.Fatalf("expected rate-limited code, got %s", ae.Code)
	}
}
