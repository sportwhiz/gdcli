package update

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const latestReleaseURL = "https://api.github.com/repos/sportwhiz/gdcli/releases/latest"

type Result struct {
	OK              bool
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable *bool
	ReleaseURL      string
	CheckedAt       time.Time
	Error           string
}

var latestReleaseFetcher = fetchLatestReleaseHTTP

func CheckWithTimeout(ctx context.Context, current string, timeout time.Duration) Result {
	now := time.Now().UTC()
	res := Result{
		OK:             false,
		CurrentVersion: NormalizeVersion(current),
		CheckedAt:      now,
	}

	checkCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		checkCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	latest, releaseURL, err := latestReleaseFetcher(checkCtx, res.CurrentVersion)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	res.OK = true
	res.LatestVersion = latest
	res.ReleaseURL = releaseURL
	res.UpdateAvailable = IsVersionNewer(res.CurrentVersion, res.LatestVersion)
	return res
}

func fetchLatestReleaseHTTP(ctx context.Context, currentVersion string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gdcli/"+currentVersion)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", &HTTPStatusError{StatusCode: resp.StatusCode}
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}
	return NormalizeVersion(payload.TagName), payload.HTMLURL, nil
}

type HTTPStatusError struct {
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return "update check failed with status " + strconv.Itoa(e.StatusCode)
}

func NormalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

func IsVersionNewer(current, latest string) *bool {
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
	v = NormalizeVersion(v)
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
