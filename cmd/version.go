package cmd

import (
	"context"
	"runtime"
	"time"

	"github.com/sportwhiz/gdcli/internal/app"
	upd "github.com/sportwhiz/gdcli/internal/update"
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
		result["update_check"] = checkForUpdate(rt.Ctx, Version, 8*time.Second)
	}
	return emitSuccess(rt, "version", result)
}

func runSelfUpdate(rt *app.Runtime, _ []string) error {
	check := checkForUpdate(rt.Ctx, Version, 8*time.Second)
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

func checkForUpdate(ctx context.Context, current string, timeout time.Duration) map[string]any {
	res := upd.CheckWithTimeout(ctx, current, timeout)
	return updateCheckMap(res)
}
