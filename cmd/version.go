package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/sportwhiz/gdcli/internal/app"
)

// Version metadata is populated at build time via ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func runVersion(rt *app.Runtime, args []string) error {
	check := hasBoolFlag(args, "check")
	result := map[string]any{
		"version":    Version,
		"commit":     Commit,
		"build_date": BuildDate,
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
	}
	if check {
		result["update_check"] = checkForUpdate(rt.Ctx, Version)
	}
	return emitSuccess(rt, "version", result)
}

func runSelfUpdate(rt *app.Runtime, _ []string) error {
	check := checkForUpdate(rt.Ctx, Version)
	result := map[string]any{
		"current_version": Version,
		"update_check":    check,
		"upgrade_commands": []string{
			"brew update && brew upgrade gdcli",
			"go install github.com/sportwhiz/gdcli/cmd/gdcli@latest",
		},
		"verify_command": "gdcli version --check --json",
	}
	return emitSuccess(rt, "self-update", result)
}

type updateCheck struct {
	OK              bool   `json:"ok"`
	CurrentVersion  string `json:"current_version,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available,omitempty"`
	Comparable      bool   `json:"comparable,omitempty"`
	ReleaseURL      string `json:"release_url,omitempty"`
	Error           string `json:"error,omitempty"`
}

func checkForUpdate(ctx context.Context, current string) map[string]any {
	out := updateCheck{
		OK:             false,
		CurrentVersion: normalizeVersion(current),
	}

	latest, url, err := fetchLatestRelease(ctx)
	if err != nil {
		out.Error = err.Error()
		return map[string]any{
			"ok":      out.OK,
			"error":   out.Error,
			"current": out.CurrentVersion,
		}
	}

	out.OK = true
	out.LatestVersion = latest
	out.ReleaseURL = url
	if newer := isVersionNewer(out.CurrentVersion, out.LatestVersion); newer != nil {
		out.Comparable = true
		out.UpdateAvailable = *newer
	}

	m := map[string]any{
		"ok":             out.OK,
		"current":        out.CurrentVersion,
		"latest":         out.LatestVersion,
		"release_url":    out.ReleaseURL,
		"update_checked": time.Now().UTC().Format(time.RFC3339),
	}
	if out.Comparable {
		m["update_available"] = out.UpdateAvailable
	} else {
		m["update_available"] = nil
	}
	return m
}

func fetchLatestRelease(ctx context.Context) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/sportwhiz/gdcli/releases/latest", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gdcli/"+normalizeVersion(Version))

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", &httpStatusError{StatusCode: resp.StatusCode}
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}
	return normalizeVersion(payload.TagName), payload.HTMLURL, nil
}

type httpStatusError struct {
	StatusCode int
}

func (e *httpStatusError) Error() string {
	return "update check failed with status " + strconv.Itoa(e.StatusCode)
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

// isVersionNewer returns nil when versions are not comparable.
func isVersionNewer(current, latest string) *bool {
	c, okC := parseSemver(current)
	l, okL := parseSemver(latest)
	if !okC || !okL {
		return nil
	}
	if l.major != c.major {
		b := l.major > c.major
		return &b
	}
	if l.minor != c.minor {
		b := l.minor > c.minor
		return &b
	}
	if l.patch != c.patch {
		b := l.patch > c.patch
		return &b
	}

	// Stable beats pre-release when core versions match.
	if c.pre == "" && l.pre != "" {
		f := false
		return &f
	}
	if c.pre != "" && l.pre == "" {
		t := true
		return &t
	}
	if c.pre == l.pre {
		f := false
		return &f
	}
	b := l.pre > c.pre
	return &b
}

type semver struct {
	major int
	minor int
	patch int
	pre   string
}

func parseSemver(v string) (semver, bool) {
	v = normalizeVersion(v)
	if v == "" || v == "dev" {
		return semver{}, false
	}
	main := v
	pre := ""
	if idx := strings.Index(v, "-"); idx >= 0 {
		main = v[:idx]
		pre = v[idx+1:]
	}
	parts := strings.Split(main, ".")
	if len(parts) < 3 {
		return semver{}, false
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return semver{}, false
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil {
		return semver{}, false
	}
	pat, err := strconv.Atoi(parts[2])
	if err != nil {
		return semver{}, false
	}
	return semver{major: maj, minor: min, patch: pat, pre: pre}, true
}
