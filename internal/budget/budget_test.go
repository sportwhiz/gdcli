package budget

import (
	"testing"
	"time"

	"github.com/sportwhiz/gdcli/internal/config"
	"github.com/sportwhiz/gdcli/internal/store"
)

func TestCheckDailyCaps(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := config.Default()
	cfg.MaxDailySpend = 100
	cfg.MaxDomainsPerDay = 2

	now := time.Now()
	_ = store.AppendOperation(store.Operation{OperationID: "1", Type: "purchase", Domain: "a.com", Amount: 40, Currency: "USD", CreatedAt: now, Status: "succeeded"})
	_ = store.AppendOperation(store.Operation{OperationID: "2", Type: "renew", Domain: "b.com", Amount: 40, Currency: "USD", CreatedAt: now, Status: "succeeded"})

	if err := CheckDailyCaps(cfg, now, 10); err == nil {
		t.Fatalf("expected domains/day cap to fail")
	}
}

func TestCheckPrice(t *testing.T) {
	cfg := config.Default()
	cfg.MaxPricePerDomain = 20
	if err := CheckPrice(cfg, 25, "USD"); err == nil {
		t.Fatalf("expected max price failure")
	}
	if err := CheckPrice(cfg, 10, "EUR"); err == nil {
		t.Fatalf("expected currency validation failure")
	}
}
