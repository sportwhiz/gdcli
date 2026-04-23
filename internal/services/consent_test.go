package services

import (
	"context"
	"testing"
)

func TestTldFromDomain(t *testing.T) {
	cases := map[string]string{
		"example.com":           "com",
		"Example.COM":           "com",
		"sub.example.net":       "net",
		"apitestdomain4.com":    "com",
		"ddcprotest.co.uk":      "co.uk",
		"alpha.beta.co.uk":      "co.uk",
		"site.com.au":           "com.au",
		"example":               "",
		"":                      "",
		"trailing.dot.example.": "example",
	}
	for in, want := range cases {
		if got := tldFromDomain(in); got != want {
			t.Errorf("tldFromDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConsentAgreedByPrefersEnvVar(t *testing.T) {
	t.Setenv(envConsentAgreedBy, "198.51.100.4")
	got, fellBack := consentAgreedBy(context.Background())
	if got != "198.51.100.4" {
		t.Fatalf("got %q, want env override", got)
	}
	if fellBack {
		t.Fatalf("fellBack should be false when env is set")
	}
}

func TestConsentAgreedByTrimsWhitespace(t *testing.T) {
	t.Setenv(envConsentAgreedBy, "   10.0.0.1   ")
	got, _ := consentAgreedBy(context.Background())
	if got != "10.0.0.1" {
		t.Fatalf("got %q, want trimmed value", got)
	}
}

func TestConsentAgreedByFallbackReturnsNonEmpty(t *testing.T) {
	t.Setenv(envConsentAgreedBy, "")
	got, _ := consentAgreedBy(context.Background())
	if got == "" {
		t.Fatalf("consentAgreedBy must never return empty string")
	}
	// Either a detected IP (most environments) or the literal "gdcli" fallback —
	// both are valid, we just require something non-empty that the API will accept.
}
