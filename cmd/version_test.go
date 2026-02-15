package cmd

import "testing"

func TestNormalizeVersion(t *testing.T) {
	if got := normalizeVersion("v1.2.3"); got != "1.2.3" {
		t.Fatalf("expected 1.2.3, got %s", got)
	}
}

func TestIsVersionNewer(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    *bool
	}{
		{current: "1.0.0", latest: "1.0.1", want: boolPtr(true)},
		{current: "1.2.0", latest: "1.1.9", want: boolPtr(false)},
		{current: "1.2.3", latest: "1.2.3", want: boolPtr(false)},
		{current: "1.2.3-rc1", latest: "1.2.3", want: boolPtr(true)},
		{current: "dev", latest: "1.2.3", want: nil},
	}
	for _, tt := range tests {
		got := isVersionNewer(tt.current, tt.latest)
		if tt.want == nil && got != nil {
			t.Fatalf("%s -> %s expected nil, got %v", tt.current, tt.latest, *got)
		}
		if tt.want != nil {
			if got == nil {
				t.Fatalf("%s -> %s expected %v, got nil", tt.current, tt.latest, *tt.want)
			}
			if *got != *tt.want {
				t.Fatalf("%s -> %s expected %v, got %v", tt.current, tt.latest, *tt.want, *got)
			}
		}
	}
}

func boolPtr(v bool) *bool {
	return &v
}
