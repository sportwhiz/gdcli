package cmd

import (
	"context"
	"time"

	"github.com/sportwhiz/gdcli/internal/app"
	"github.com/sportwhiz/gdcli/internal/output"
	upd "github.com/sportwhiz/gdcli/internal/update"
)

const (
	startupUpdateCheckInterval = 24 * time.Hour
	startupUpdateCheckTimeout  = 300 * time.Millisecond
)

var (
	loadUpdateCache = upd.LoadCache
	saveUpdateCache = upd.SaveCache
	checkUpdate     = upd.CheckWithTimeout
	timeNow         = func() time.Time { return time.Now().UTC() }
)

func maybeStartUpdateNotifier(rt *app.Runtime, rootCommand string) {
	if !shouldRunStartupUpdateCheck(rt, rootCommand) {
		return
	}
	if handled := maybeEmitCachedUpdateNotice(rt); handled {
		return
	}
	go runStartupUpdateNotifier(rt)
}

func shouldRunStartupUpdateCheck(rt *app.Runtime, rootCommand string) bool {
	if rt == nil || rt.Quiet {
		return false
	}
	if rootCommand == "version" || rootCommand == "self-update" {
		return false
	}
	if upd.IsDisabledByEnv() {
		return false
	}
	return true
}

func runStartupUpdateNotifier(rt *app.Runtime) {
	current := upd.NormalizeVersion(Version)
	now := timeNow()

	cache, err := loadUpdateCache()
	if err == nil && cache != nil && cache.CurrentVersion == current && !upd.ShouldCheck(now, cache.LastCheckedAt, startupUpdateCheckInterval) {
		if cache.UpdateAvailable != nil && *cache.UpdateAvailable {
			emitUpdateNotice(rt, current, cache.LatestVersion, cache.ReleaseURL)
		}
		return
	}

	res := checkUpdate(context.Background(), Version, startupUpdateCheckTimeout)
	updateCache := &upd.Cache{
		LastCheckedAt:   now,
		CurrentVersion:  current,
		LatestVersion:   res.LatestVersion,
		UpdateAvailable: res.UpdateAvailable,
		ReleaseURL:      res.ReleaseURL,
		LastError:       res.Error,
	}
	_ = saveUpdateCache(updateCache)
	if res.UpdateAvailable != nil && *res.UpdateAvailable {
		emitUpdateNotice(rt, current, res.LatestVersion, res.ReleaseURL)
	}
}

func maybeEmitCachedUpdateNotice(rt *app.Runtime) bool {
	current := upd.NormalizeVersion(Version)
	now := timeNow()
	cache, err := loadUpdateCache()
	if err != nil || cache == nil {
		return false
	}
	if cache.CurrentVersion != current {
		return false
	}
	if upd.ShouldCheck(now, cache.LastCheckedAt, startupUpdateCheckInterval) {
		return false
	}
	if cache.UpdateAvailable != nil && *cache.UpdateAvailable {
		emitUpdateNotice(rt, current, cache.LatestVersion, cache.ReleaseURL)
	}
	return true
}

func emitUpdateNotice(rt *app.Runtime, current, latest, releaseURL string) {
	output.LogErr(rt.ErrOut, "update available: gdcli %s -> %s (run: gdcli self-update --json)", current, latest)
	if releaseURL != "" {
		output.LogErr(rt.ErrOut, "release: %s", releaseURL)
	}
}

func updateCheckMap(res upd.Result) map[string]any {
	if !res.OK {
		return map[string]any{
			"ok":      false,
			"error":   res.Error,
			"current": res.CurrentVersion,
		}
	}
	m := map[string]any{
		"ok":             true,
		"current":        res.CurrentVersion,
		"latest":         res.LatestVersion,
		"release_url":    res.ReleaseURL,
		"update_checked": res.CheckedAt.UTC().Format(time.RFC3339),
	}
	if res.UpdateAvailable != nil {
		m["update_available"] = *res.UpdateAvailable
	} else {
		m["update_available"] = nil
	}
	return m
}
