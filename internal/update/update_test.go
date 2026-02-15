package update

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNormalizeAndCompareVersion(t *testing.T) {
	if got := NormalizeVersion("v1.2.3"); got != "1.2.3" {
		t.Fatalf("normalize expected 1.2.3, got %s", got)
	}
	if got := IsVersionNewer("1.0.0", "1.0.1"); got == nil || !*got {
		t.Fatalf("expected latest to be newer")
	}
	if got := IsVersionNewer("dev", "1.0.0"); got != nil {
		t.Fatalf("expected incomparable versions to return nil")
	}
}

func TestCheckWithTimeoutSuccess(t *testing.T) {
	orig := latestReleaseFetcher
	t.Cleanup(func() { latestReleaseFetcher = orig })
	latestReleaseFetcher = func(ctx context.Context, currentVersion string) (string, string, error) {
		return "1.2.4", "https://example.com/release", nil
	}

	res := CheckWithTimeout(context.Background(), "v1.2.3", 50*time.Millisecond)
	if !res.OK {
		t.Fatalf("expected success, got error=%q", res.Error)
	}
	if res.CurrentVersion != "1.2.3" || res.LatestVersion != "1.2.4" {
		t.Fatalf("unexpected versions: %+v", res)
	}
	if res.UpdateAvailable == nil || !*res.UpdateAvailable {
		t.Fatalf("expected update_available=true")
	}
}

func TestCheckWithTimeoutRespectsDeadline(t *testing.T) {
	orig := latestReleaseFetcher
	t.Cleanup(func() { latestReleaseFetcher = orig })
	latestReleaseFetcher = func(ctx context.Context, currentVersion string) (string, string, error) {
		select {
		case <-time.After(200 * time.Millisecond):
			return "", "", errors.New("should have timed out")
		case <-ctx.Done():
			return "", "", ctx.Err()
		}
	}

	start := time.Now()
	res := CheckWithTimeout(context.Background(), "v1.2.3", 25*time.Millisecond)
	elapsed := time.Since(start)
	if res.OK {
		t.Fatalf("expected timeout failure")
	}
	if res.Error == "" {
		t.Fatalf("expected timeout error")
	}
	if elapsed > 150*time.Millisecond {
		t.Fatalf("timeout path took too long: %v", elapsed)
	}
}
