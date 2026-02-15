package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/sportwhiz/gdcli/internal/app"
	apperr "github.com/sportwhiz/gdcli/internal/errors"
)

func TestRunAccountOrdersListJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/orders" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"orders":[{"orderId":"3938269704","createdAt":"2025-11-05T12:37:45.000Z","currency":"USD","items":[{"label":".COM Domain Name Registration - 1 Year (recurring)"}],"pricing":{"total":10690000}}],"pagination":{"first":"f","last":"l","next":"n","total":9}}`))
	}))
	defer srv.Close()

	rt, out := testRuntime(t, srv.URL, true, false)
	if err := runAccount(rt, []string{"orders", "list", "--limit", "5", "--offset", "0"}); err != nil {
		t.Fatalf("runAccount: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env["command"] != "account orders list" {
		t.Fatalf("unexpected command: %v", env["command"])
	}
	result, ok := env["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result")
	}
	orders, ok := result["orders"].([]any)
	if !ok || len(orders) != 1 {
		t.Fatalf("expected one order")
	}
}

func TestRunAccountSubscriptionsNDJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/subscriptions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"subscriptions":[{"subscriptionId":"757644825:2","status":"ACTIVE","label":"EXAMPLE.COM","createdAt":"2025-11-05T12:37:46.560Z","expiresAt":"2026-11-05T14:37:57.000Z","renewable":true,"renewAuto":true,"product":{"productGroupKey":"domains","namespace":"domain"},"billing":{"status":"CURRENT","renewAt":"2026-11-06T14:37:57.000Z"}}],"pagination":{"first":"f","last":"l","next":"n","total":22}}`))
	}))
	defer srv.Close()

	rt, out := testRuntime(t, srv.URL, false, true)
	if err := runAccount(rt, []string{"subscriptions", "list", "--limit", "5", "--offset", "0"}); err != nil {
		t.Fatalf("runAccount: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode ndjson record: %v", err)
	}
	if env["command"] != "account subscriptions list" {
		t.Fatalf("unexpected command: %v", env["command"])
	}
}

func TestRunAccountValidationLimit(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	rt, _ := testRuntime(t, srv.URL, true, false)
	err := runAccount(rt, []string{"orders", "list", "--limit", "0"})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	var ae *apperr.AppError
	if !apperr.As(err, &ae) || ae.Code != apperr.CodeValidation {
		t.Fatalf("expected validation app error, got %v", err)
	}
}

func TestRunAccountIdentitySetAndShow(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	rt, out := testRuntime(t, srv.URL, true, false)
	if err := runAccount(rt, []string{"identity", "set", "--shopper-id", "123456789", "--customer-id", "cust-123"}); err != nil {
		t.Fatalf("account identity set: %v", err)
	}
	out.Reset()
	if err := runAccount(rt, []string{"identity", "show"}); err != nil {
		t.Fatalf("account identity show: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	result, ok := env["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result")
	}
	if result["shopper_id"] != "123456789" || result["customer_id"] != "cust-123" {
		t.Fatalf("unexpected identity values: %+v", result)
	}
}

func testRuntime(t *testing.T, baseURL string, jsonMode, ndjsonMode bool) (*app.Runtime, *bytes.Buffer) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GODADDY_API_KEY", "k")
	t.Setenv("GODADDY_API_SECRET", "s")
	t.Setenv("GDCLI_BASE_URL", baseURL)

	out := &bytes.Buffer{}

	rt, err := app.NewRuntime(context.Background(), out, os.Stderr, jsonMode, ndjsonMode, true, "req-test")
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	return rt, out
}
