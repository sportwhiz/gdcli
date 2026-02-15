package safety

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sportwhiz/gdcli/internal/store"
)

func TestTokenLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	now := time.Now().UTC()
	tok, err := IssueToken("example.com", 12.99, "USD", "op-key", now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if tok.TokenID == "" {
		t.Fatalf("expected token id")
	}

	used, err := ValidateAndUseToken(tok.TokenID, "example.com", now.Add(1*time.Minute))
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if !used.Used {
		t.Fatalf("expected token to be marked used")
	}

	if _, err := ValidateAndUseToken(tok.TokenID, "example.com", now.Add(2*time.Minute)); err == nil {
		t.Fatalf("expected second usage to fail")
	}

	if _, err := os.Stat(filepath.Join(home, ".gdcli", store.TokensFile)); err != nil {
		t.Fatalf("expected tokens file: %v", err)
	}
}

func TestEnableAutoPurchasePhrase(t *testing.T) {
	if _, err := EnableAutoPurchase("bad"); err == nil {
		t.Fatalf("expected bad phrase to fail")
	}
	if _, err := EnableAutoPurchase(AckPhrase); err != nil {
		t.Fatalf("expected correct phrase to pass: %v", err)
	}
}
