package safety

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

func TestTokenPruneRemovesExpired(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	now := time.Now().UTC()

	if _, err := IssueToken("expired.com", 10, "USD", "op-expired", now.Add(-2*TokenTTL)); err != nil {
		t.Fatalf("issue expired token: %v", err)
	}
	fresh, err := IssueToken("fresh.com", 11, "USD", "op-fresh", now)
	if err != nil {
		t.Fatalf("issue fresh token: %v", err)
	}

	ts, err := store.LoadTokens()
	if err != nil {
		t.Fatalf("load tokens: %v", err)
	}
	if len(ts.Tokens) != 1 {
		t.Fatalf("expected one token after prune, got %d", len(ts.Tokens))
	}
	if ts.Tokens[0].TokenID != fresh.TokenID {
		t.Fatalf("expected fresh token to remain")
	}
}

func TestValidateAndUseTokenSingleSuccessUnderConcurrency(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	now := time.Now().UTC()

	tok, err := IssueToken("example.com", 12.99, "USD", "op-concurrent", now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	var successCount int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := ValidateAndUseToken(tok.TokenID, "example.com", now.Add(time.Minute)); err == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if successCount != 1 {
		t.Fatalf("expected exactly one successful token use, got %d", successCount)
	}
}
