package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sportwhiz/gdcli/internal/app"
	upd "github.com/sportwhiz/gdcli/internal/update"
)

func TestShouldRunStartupUpdateCheck(t *testing.T) {
	rt := testNotifierRuntime(t, false)
	if !shouldRunStartupUpdateCheck(rt, "domains") {
		t.Fatalf("expected notifier to run by default")
	}
	if shouldRunStartupUpdateCheck(rt, "version") {
		t.Fatalf("version command should skip notifier")
	}
	if shouldRunStartupUpdateCheck(rt, "self-update") {
		t.Fatalf("self-update command should skip notifier")
	}

	quietRT := testNotifierRuntime(t, true)
	if shouldRunStartupUpdateCheck(quietRT, "domains") {
		t.Fatalf("quiet mode should skip notifier")
	}

	t.Setenv("GDCLI_DISABLE_UPDATE_CHECK", "1")
	if shouldRunStartupUpdateCheck(rt, "domains") {
		t.Fatalf("env opt-out should skip notifier")
	}
}

func TestRunStartupUpdateNotifierUsesCacheAndWritesStderrOnly(t *testing.T) {
	rt := testNotifierRuntime(t, false)

	origLoad, origSave, origCheck, origNow := loadUpdateCache, saveUpdateCache, checkUpdate, timeNow
	t.Cleanup(func() {
		loadUpdateCache, saveUpdateCache, checkUpdate, timeNow = origLoad, origSave, origCheck, origNow
	})

	truth := true
	loadUpdateCache = func() (*upd.Cache, error) {
		return &upd.Cache{
			LastCheckedAt:   time.Now().UTC(),
			CurrentVersion:  upd.NormalizeVersion(Version),
			LatestVersion:   "9.9.9",
			UpdateAvailable: &truth,
			ReleaseURL:      "https://example.com/release",
		}, nil
	}
	saveUpdateCache = func(c *upd.Cache) error { return nil }
	checkUpdate = func(ctx context.Context, current string, timeout time.Duration) upd.Result {
		t.Fatalf("network check should not run when cache is fresh")
		return upd.Result{}
	}
	timeNow = func() time.Time { return time.Now().UTC() }

	runStartupUpdateNotifier(rt)

	if got := rt.Out.Out.(*bytes.Buffer).String(); got != "" {
		t.Fatalf("stdout should remain untouched, got %q", got)
	}
	errOut := rt.ErrOut.(*bytes.Buffer).String()
	if !strings.Contains(errOut, "update available") {
		t.Fatalf("expected update notice in stderr, got %q", errOut)
	}
	if !strings.Contains(errOut, "https://example.com/release") {
		t.Fatalf("expected release url in stderr, got %q", errOut)
	}
}

func TestRunStartupUpdateNotifierRefreshesStaleCache(t *testing.T) {
	rt := testNotifierRuntime(t, false)

	origLoad, origSave, origCheck, origNow := loadUpdateCache, saveUpdateCache, checkUpdate, timeNow
	t.Cleanup(func() {
		loadUpdateCache, saveUpdateCache, checkUpdate, timeNow = origLoad, origSave, origCheck, origNow
	})

	loadUpdateCache = func() (*upd.Cache, error) {
		return &upd.Cache{
			LastCheckedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			CurrentVersion: upd.NormalizeVersion(Version),
		}, nil
	}
	saved := &upd.Cache{}
	saveUpdateCache = func(c *upd.Cache) error {
		*saved = *c
		return nil
	}
	checkUpdate = func(ctx context.Context, current string, timeout time.Duration) upd.Result {
		if timeout != startupUpdateCheckTimeout {
			t.Fatalf("unexpected timeout: %v", timeout)
		}
		available := true
		return upd.Result{
			OK:              true,
			CurrentVersion:  upd.NormalizeVersion(current),
			LatestVersion:   "9.9.9",
			UpdateAvailable: &available,
			ReleaseURL:      "https://example.com/release",
			CheckedAt:       time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC),
		}
	}
	timeNow = func() time.Time { return time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC) }

	runStartupUpdateNotifier(rt)
	if saved.LatestVersion != "9.9.9" {
		t.Fatalf("expected cache save with latest version, got %+v", saved)
	}
}

func testNotifierRuntime(t *testing.T, quiet bool) *app.Runtime {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt, err := app.NewRuntime(context.Background(), stdout, stderr, true, false, quiet, "req-test")
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	return rt
}

func TestUpdateNotifierDoesNotCorruptJSONOutput(t *testing.T) {
	rt := testNotifierRuntime(t, false)
	origLoad, origSave, origCheck, origNow := loadUpdateCache, saveUpdateCache, checkUpdate, timeNow
	t.Cleanup(func() {
		loadUpdateCache, saveUpdateCache, checkUpdate, timeNow = origLoad, origSave, origCheck, origNow
	})
	truth := true
	loadUpdateCache = func() (*upd.Cache, error) {
		return &upd.Cache{
			LastCheckedAt:   time.Now().UTC(),
			CurrentVersion:  upd.NormalizeVersion(Version),
			LatestVersion:   "9.9.9",
			UpdateAvailable: &truth,
		}, nil
	}
	saveUpdateCache = func(c *upd.Cache) error { return nil }
	checkUpdate = func(ctx context.Context, current string, timeout time.Duration) upd.Result { return upd.Result{} }
	timeNow = func() time.Time { return time.Now().UTC() }

	emitErr := emitSuccess(rt, "help", map[string]any{"commands": []string{"init"}})
	if emitErr != nil {
		t.Fatalf("emit success: %v", emitErr)
	}
	runStartupUpdateNotifier(rt)
	stdout := rt.Out.Out.(*bytes.Buffer).String()
	if !strings.Contains(stdout, "\"command\":\"help\"") {
		t.Fatalf("expected valid json envelope in stdout, got %q", stdout)
	}
}
