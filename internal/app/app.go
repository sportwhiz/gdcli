package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/sportwhiz/gdcli/internal/config"
	apperr "github.com/sportwhiz/gdcli/internal/errors"
	"github.com/sportwhiz/gdcli/internal/output"
	"github.com/sportwhiz/gdcli/internal/rate"
)

type Credentials struct {
	apiKey    string
	apiSecret string
}

func (c Credentials) APIKey() string    { return c.apiKey }
func (c Credentials) APISecret() string { return c.apiSecret }

type Runtime struct {
	Ctx       context.Context
	Cfg       *config.Config
	Out       *output.Writer
	ErrOut    io.Writer
	Limiter   *rate.Limiter
	JSON      bool
	NDJSON    bool
	Quiet     bool
	RequestID string
}

func NewRuntime(ctx context.Context, stdOut, stdErr io.Writer, jsonMode, ndjsonMode, quiet bool, requestID string) (*Runtime, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed loading config", err)
	}
	return &Runtime{
		Ctx:       ctx,
		Cfg:       cfg,
		Out:       output.NewWriter(stdOut),
		ErrOut:    stdErr,
		Limiter:   rate.NewLimiter(55),
		JSON:      jsonMode,
		NDJSON:    ndjsonMode,
		Quiet:     quiet,
		RequestID: requestID,
	}, nil
}

func LoadCredentials() (Credentials, error) {
	key := strings.TrimSpace(os.Getenv("GODADDY_API_KEY"))
	secret := strings.TrimSpace(os.Getenv("GODADDY_API_SECRET"))
	if key != "" && secret != "" {
		return Credentials{apiKey: key, apiSecret: secret}, nil
	}

	if runtime.GOOS == "darwin" {
		k := keychainRead("godaddy_api_key")
		s := keychainRead("godaddy_api_secret")
		if k != "" && s != "" {
			return Credentials{apiKey: k, apiSecret: s}, nil
		}
	}

	return Credentials{}, &apperr.AppError{
		Code:    apperr.CodeAuth,
		Message: "missing GoDaddy credentials; set GODADDY_API_KEY and GODADDY_API_SECRET or store in OS keychain",
		Details: map[string]any{"env_vars": []string{"GODADDY_API_KEY", "GODADDY_API_SECRET"}},
	}
}

func keychainRead(account string) string {
	if account != "godaddy_api_key" && account != "godaddy_api_secret" {
		return ""
	}
	// #nosec G204 -- exec.Command is called with a fixed binary/flags and a strict account allowlist.
	out, err := exec.Command("security", "find-generic-password", "-s", "gdcli", "-a", account, "-w").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func StoreCredentialsInKeychain(key, secret string) error {
	if runtime.GOOS != "darwin" {
		return &apperr.AppError{Code: apperr.CodeValidation, Message: "keychain storage is only supported on macOS"}
	}
	if strings.TrimSpace(key) == "" || strings.TrimSpace(secret) == "" {
		return &apperr.AppError{Code: apperr.CodeValidation, Message: "api key and secret are required"}
	}
	// #nosec G204 -- exec.Command is called with a fixed binary/flags; key is passed as an argument without shell interpolation.
	if out, err := exec.Command("security", "add-generic-password", "-U", "-s", "gdcli", "-a", "godaddy_api_key", "-w", key).CombinedOutput(); err != nil {
		return &apperr.AppError{Code: apperr.CodeInternal, Message: "failed storing keychain api key", Details: map[string]any{"stderr": strings.TrimSpace(string(out))}, Cause: err}
	}
	// #nosec G204 -- exec.Command is called with a fixed binary/flags; secret is passed as an argument without shell interpolation.
	if out, err := exec.Command("security", "add-generic-password", "-U", "-s", "gdcli", "-a", "godaddy_api_secret", "-w", secret).CombinedOutput(); err != nil {
		return &apperr.AppError{Code: apperr.CodeInternal, Message: "failed storing keychain api secret", Details: map[string]any{"stderr": strings.TrimSpace(string(out))}, Cause: err}
	}
	return nil
}

func BaseURL(env string) string {
	if override := strings.TrimSpace(os.Getenv("GDCLI_BASE_URL")); override != "" {
		return strings.TrimSuffix(override, "/")
	}
	if strings.EqualFold(env, "ote") {
		return "https://api.ote-godaddy.com"
	}
	return "https://api.godaddy.com"
}

func MaybeWarnProdFinancial(rt *Runtime, command string) {
	if rt.Quiet {
		return
	}
	if rt.Cfg.APIEnvironment == "prod" && (strings.Contains(command, "purchase") || strings.Contains(command, "renew")) {
		fmt.Fprintf(rt.ErrOut, "warning: running financial action against production API environment\n")
	}
}
