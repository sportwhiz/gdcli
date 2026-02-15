package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDefaultListenAddr(t *testing.T) {
	t.Setenv("MOCK_GODADDY_LISTEN", "")
	if got := defaultListenAddr(); got != "127.0.0.1:8787" {
		t.Fatalf("expected localhost default, got %q", got)
	}

	t.Setenv("MOCK_GODADDY_LISTEN", "0.0.0.0:8787")
	if got := defaultListenAddr(); got != "0.0.0.0:8787" {
		t.Fatalf("expected env override, got %q", got)
	}
}

func TestDecodeJSONBodyEnforcesMaxBytes(t *testing.T) {
	body := `{"domains":["` + strings.Repeat("a", int(maxRequestBodyBytes)) + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/domains/available", strings.NewReader(body))
	rr := httptest.NewRecorder()
	var payload struct {
		Domains []string `json:"domains"`
	}
	err := decodeJSONBody(rr, req, &payload)
	if err == nil {
		t.Fatalf("expected max-bytes error")
	}
	var maxErr *http.MaxBytesError
	if !errors.As(err, &maxErr) {
		t.Fatalf("expected MaxBytesError, got %T", err)
	}
}
