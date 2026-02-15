package godaddy

import "testing"

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
