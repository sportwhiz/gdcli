package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	apperr "github.com/sportwhiz/gdcli/internal/errors"
	"github.com/sportwhiz/gdcli/internal/godaddy"
	"golang.org/x/net/publicsuffix"
)

const (
	envConsentAgreedBy = "GDCLI_CONSENT_AGREED_BY"
	fallbackAgreedBy   = "gdcli"
)

// tldFromDomain returns the ICANN/private public suffix for the domain
// (e.g. "com" for "example.com", "co.uk" for "example.co.uk"). Empty on failure.
func tldFromDomain(domain string) string {
	d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if d == "" || !strings.Contains(d, ".") {
		return ""
	}
	suffix, _ := publicsuffix.PublicSuffix(d)
	return suffix
}

// consentAgreedBy picks an identifier for the "who agreed" audit field.
// Order: GDCLI_CONSENT_AGREED_BY env var, detected local outbound IP, literal "gdcli".
// The second return is true when the literal fallback is used, so the caller can warn.
func consentAgreedBy(ctx context.Context) (string, bool) {
	if v := strings.TrimSpace(os.Getenv(envConsentAgreedBy)); v != "" {
		return v, false
	}
	if ip := detectOutboundIP(ctx); ip != "" {
		return ip, false
	}
	return fallbackAgreedBy, true
}

// detectOutboundIP returns the local IP used for outbound traffic. Uses a
// short-timeout UDP "dial" (connectionless — no packets are sent) so the
// kernel resolves the egress interface without network I/O.
func detectOutboundIP(ctx context.Context) string {
	dctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(dctx, "udp", "1.1.1.1:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}

// buildPurchaseConsent fetches the required agreement keys for the domain's
// TLD (with the given privacy setting) and assembles a PurchaseConsent ready
// to include in a purchase request.
func (s *Service) buildPurchaseConsent(ctx context.Context, domain string, privacy bool) (godaddy.PurchaseConsent, error) {
	tld := tldFromDomain(domain)
	if tld == "" {
		return godaddy.PurchaseConsent{}, &apperr.AppError{Code: apperr.CodeValidation, Message: "domain has no public suffix", Details: map[string]any{"domain": domain}}
	}
	agreements, err := s.Client.Agreements(ctx, []string{tld}, privacy)
	if err != nil {
		// Preserve existing classification if present; otherwise wrap.
		var ae *apperr.AppError
		if errors.As(err, &ae) {
			return godaddy.PurchaseConsent{}, err
		}
		return godaddy.PurchaseConsent{}, fmt.Errorf("fetch consent agreements for %s: %w", tld, err)
	}
	keys := make([]string, 0, len(agreements))
	for _, a := range agreements {
		if a.AgreementKey != "" {
			keys = append(keys, a.AgreementKey)
		}
	}
	if len(keys) == 0 {
		return godaddy.PurchaseConsent{}, &apperr.AppError{Code: apperr.CodeInternal, Message: "registrar returned no consent agreements", Details: map[string]any{"tld": tld, "privacy": privacy}}
	}
	agreedBy, fellBack := consentAgreedBy(ctx)
	if fellBack {
		fmt.Fprintf(os.Stderr, "warning: consent.agreedBy falling back to %q; set %s to record the real registrant identifier (typically an IP address)\n", fallbackAgreedBy, envConsentAgreedBy)
	}
	return godaddy.PurchaseConsent{
		AgreementKeys: keys,
		AgreedBy:      agreedBy,
		AgreedAt:      time.Now().UTC().Format(time.RFC3339),
	}, nil
}
