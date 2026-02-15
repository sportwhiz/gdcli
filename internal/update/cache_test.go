package update

import (
	"testing"
	"time"
)

func TestShouldCheck(t *testing.T) {
	now := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	if !ShouldCheck(now, time.Time{}, 24*time.Hour) {
		t.Fatalf("expected zero last-checked to require check")
	}
	if ShouldCheck(now, now.Add(-1*time.Hour), 24*time.Hour) {
		t.Fatalf("expected fresh cache to skip check")
	}
	if !ShouldCheck(now, now.Add(-25*time.Hour), 24*time.Hour) {
		t.Fatalf("expected stale cache to require check")
	}
}

func TestIsDisabledByEnv(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"yes", true},
		{"y", false},
		{"0", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Setenv("GDCLI_DISABLE_UPDATE_CHECK", tc.v)
		if got := IsDisabledByEnv(); got != tc.want {
			t.Fatalf("env %q expected %v got %v", tc.v, tc.want, got)
		}
	}
}
