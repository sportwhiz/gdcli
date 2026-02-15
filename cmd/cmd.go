package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/sportwhiz/gdcli/internal/app"
	"github.com/sportwhiz/gdcli/internal/config"
	apperr "github.com/sportwhiz/gdcli/internal/errors"
	"github.com/sportwhiz/gdcli/internal/godaddy"
	"github.com/sportwhiz/gdcli/internal/output"
	"github.com/sportwhiz/gdcli/internal/safety"
	"github.com/sportwhiz/gdcli/internal/services"
)

type globalFlags struct {
	json   bool
	ndjson bool
	quiet  bool
}

func Execute() {
	err := run(os.Args[1:])
	if err == nil {
		return
	}
	code := apperr.ExitCode(err)
	os.Exit(code)
}

func run(args []string) error {
	g, rest, err := parseGlobalFlags(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return usageError("missing command")
	}
	rt, err := app.NewRuntime(context.Background(), os.Stdout, os.Stderr, g.json || !g.ndjson, g.ndjson, g.quiet, requestID())
	if err != nil {
		return err
	}

	switch rest[0] {
	case "init":
		return runInit(rt, rest[1:])
	case "domains":
		return runDomains(rt, rest[1:])
	case "dns":
		return runDNS(rt, rest[1:])
	case "settings":
		return runSettings(rt, rest[1:])
	case "--help", "help", "-h":
		return emitSuccess(rt, "help", map[string]any{"commands": []string{"init", "domains", "dns", "settings"}})
	default:
		err := usageError("unknown command: " + rest[0])
		emitError(rt, "gdcli", err)
		return err
	}
}

func parseGlobalFlags(args []string) (globalFlags, []string, error) {
	var g globalFlags
	rest := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "--json":
			g.json = true
		case "--ndjson":
			g.ndjson = true
		case "--quiet":
			g.quiet = true
		default:
			rest = append(rest, a)
		}
	}
	return g, rest, nil
}

func runInit(rt *app.Runtime, args []string) error {
	if len(args) > 0 && isHelpToken(args[0]) {
		return emitSuccess(rt, "init help", map[string]any{
			"usage": "gdcli init [--api-environment prod|ote] [--max-price N] [--max-daily-spend N] [--max-domains-per-day N] [--enable-auto-purchase --ack \"I UNDERSTAND PURCHASES ARE FINAL\"] [--store-keychain --api-key KEY --api-secret SECRET] [--verify]",
		})
	}

	flags := parseKVFlags(args)
	changed := map[string]any{}

	if env := strings.TrimSpace(flags["api-environment"]); env != "" {
		if env != "prod" && env != "ote" {
			err := &apperr.AppError{Code: apperr.CodeValidation, Message: "api-environment must be prod or ote"}
			emitError(rt, "init", err)
			return err
		}
		rt.Cfg.APIEnvironment = env
		changed["api_environment"] = env
	}
	if v := strings.TrimSpace(flags["max-price"]); v != "" {
		n := parseFloatDefault(v, -1)
		if n <= 0 {
			err := &apperr.AppError{Code: apperr.CodeValidation, Message: "max-price must be > 0"}
			emitError(rt, "init", err)
			return err
		}
		rt.Cfg.MaxPricePerDomain = n
		changed["max_price_per_domain"] = n
	}
	if v := strings.TrimSpace(flags["max-daily-spend"]); v != "" {
		n := parseFloatDefault(v, -1)
		if n <= 0 {
			err := &apperr.AppError{Code: apperr.CodeValidation, Message: "max-daily-spend must be > 0"}
			emitError(rt, "init", err)
			return err
		}
		rt.Cfg.MaxDailySpend = n
		changed["max_daily_spend"] = n
	}
	if v := strings.TrimSpace(flags["max-domains-per-day"]); v != "" {
		n := parseIntDefault(v, -1)
		if n <= 0 {
			err := &apperr.AppError{Code: apperr.CodeValidation, Message: "max-domains-per-day must be > 0"}
			emitError(rt, "init", err)
			return err
		}
		rt.Cfg.MaxDomainsPerDay = n
		changed["max_domains_per_day"] = n
	}

	if hasBoolFlag(args, "enable-auto-purchase") {
		ack := strings.TrimSpace(flags["ack"])
		hash, err := safety.EnableAutoPurchase(ack)
		if err != nil {
			emitError(rt, "init", err)
			return err
		}
		rt.Cfg.AutoPurchaseEnabled = true
		rt.Cfg.AcknowledgmentHash = hash
		changed["auto_purchase_enabled"] = true
	}

	if len(changed) > 0 {
		if err := config.Save(rt.Cfg); err != nil {
			ae := &apperr.AppError{Code: apperr.CodeInternal, Message: "failed saving config", Cause: err}
			emitError(rt, "init", ae)
			return ae
		}
	}

	keychainStored := false
	if hasBoolFlag(args, "store-keychain") {
		apiKey := strings.TrimSpace(flags["api-key"])
		apiSecret := strings.TrimSpace(flags["api-secret"])
		if apiKey == "" || apiSecret == "" {
			err := &apperr.AppError{Code: apperr.CodeValidation, Message: "--store-keychain requires --api-key and --api-secret"}
			emitError(rt, "init", err)
			return err
		}
		if err := app.StoreCredentialsInKeychain(apiKey, apiSecret); err != nil {
			emitError(rt, "init", err)
			return err
		}
		keychainStored = true
	}

	verified := false
	verifyResult := map[string]any{"ok": false}
	if hasBoolFlag(args, "verify") {
		svc, err := newService(rt)
		if err != nil {
			emitError(rt, "init", err)
			return err
		}
		avail, err := svc.Availability(rt.Ctx, "example.com")
		if err != nil {
			emitError(rt, "init", err)
			return err
		}
		verified = true
		verifyResult = map[string]any{"ok": true, "sample_domain": avail.Domain}
	}

	configPath, _ := config.Path()
	res := map[string]any{
		"configured":        len(changed) > 0,
		"changed":           changed,
		"config_path":       configPath,
		"keychain_stored":   keychainStored,
		"verified":          verified,
		"verification_info": verifyResult,
		"next_steps": []string{
			"set GODADDY_API_KEY and GODADDY_API_SECRET (or use --store-keychain on macOS)",
			"run: gdcli settings show --json",
			"run: gdcli domains avail example.com --json",
		},
	}
	return emitSuccess(rt, "init", res)
}

func runDomains(rt *app.Runtime, args []string) error {
	if len(args) == 0 || isHelpToken(args[0]) {
		return emitSuccess(rt, "domains help", map[string]any{
			"subcommands": []string{"suggest", "avail", "avail-bulk", "purchase", "renew", "renew-bulk", "list"},
		})
	}
	if len(args) == 0 {
		err := usageError("missing domains subcommand")
		emitError(rt, "domains", err)
		return err
	}
	svc, err := newService(rt)
	if err != nil {
		emitError(rt, "domains", err)
		return err
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "suggest":
		if len(rest) == 0 {
			err := usageError("domains suggest <query>")
			emitError(rt, "domains suggest", err)
			return err
		}
		query := rest[0]
		flags := parseKVFlags(rest[1:])
		tlds := splitCSV(flags["tlds"])
		limit := parseIntDefault(flags["limit"], 20)
		res, err := svc.Suggest(rt.Ctx, query, tlds, limit)
		if err != nil {
			emitError(rt, "domains suggest", err)
			return err
		}
		return emitSuccess(rt, "domains suggest", res)
	case "avail":
		if len(rest) == 0 {
			err := usageError("domains avail <domain>")
			emitError(rt, "domains avail", err)
			return err
		}
		res, err := svc.Availability(rt.Ctx, rest[0])
		if err != nil {
			emitError(rt, "domains avail", err)
			return err
		}
		return emitSuccess(rt, "domains avail", res)
	case "avail-bulk":
		if len(rest) == 0 {
			err := usageError("domains avail-bulk <file>")
			emitError(rt, "domains avail-bulk", err)
			return err
		}
		domains, err := services.LoadDomainFile(rest[0])
		if err != nil {
			ae := &apperr.AppError{Code: apperr.CodeValidation, Message: "failed reading domain list", Cause: err}
			emitError(rt, "domains avail-bulk", ae)
			return ae
		}
		flags := parseKVFlags(rest[1:])
		concurrency := parseIntDefault(flags["concurrency"], 10)
		res, err := svc.AvailabilityBulkConcurrent(rt.Ctx, domains, concurrency)
		recs := make([]any, 0, len(res))
		for _, r := range res {
			row := map[string]any{
				"index":       r.Index,
				"input":       r.Input,
				"success":     r.Success,
				"duration_ms": r.Duration,
			}
			if r.Success {
				row["result"] = r.Result
			} else {
				row["error"] = r.Error
			}
			recs = append(recs, row)
		}
		if rt.NDJSON {
			if emitErr := emitSuccess(rt, "domains avail-bulk", recs); emitErr != nil {
				return emitErr
			}
		} else {
			if emitErr := emitSuccess(rt, "domains avail-bulk", map[string]any{"results": recs}); emitErr != nil {
				return emitErr
			}
		}
		if err != nil {
			return err
		}
		return nil
	case "purchase":
		if len(rest) == 0 {
			err := usageError("domains purchase <domain>")
			emitError(rt, "domains purchase", err)
			return err
		}
		app.MaybeWarnProdFinancial(rt, "domains purchase")
		domain := rest[0]
		flags := parseKVFlags(rest[1:])
		years := parseIntDefault(flags["years"], 1)
		confirm := flags["confirm"]
		auto := hasBoolFlag(rest[1:], "auto")
		if auto {
			res, err := svc.PurchaseAuto(rt.Ctx, domain, years)
			if err != nil {
				emitError(rt, "domains purchase", err)
				return err
			}
			return emitSuccess(rt, "domains purchase", res)
		}
		if confirm != "" {
			res, err := svc.PurchaseConfirm(rt.Ctx, domain, confirm, years)
			if err != nil {
				emitError(rt, "domains purchase", err)
				return err
			}
			return emitSuccess(rt, "domains purchase", res)
		}
		res, err := svc.PurchaseDryRun(rt.Ctx, domain, years)
		if err != nil {
			emitError(rt, "domains purchase", err)
			return err
		}
		return emitSuccess(rt, "domains purchase", res)
	case "renew":
		if len(rest) == 0 {
			err := usageError("domains renew <domain> --years <n>")
			emitError(rt, "domains renew", err)
			return err
		}
		app.MaybeWarnProdFinancial(rt, "domains renew")
		domain := rest[0]
		flags := parseKVFlags(rest[1:])
		years := parseIntDefault(flags["years"], 1)
		dryRun := hasBoolFlag(rest[1:], "dry-run")
		autoApprove := hasBoolFlag(rest[1:], "auto-approve")
		res, err := svc.Renew(rt.Ctx, domain, years, dryRun, autoApprove)
		if err != nil {
			emitError(rt, "domains renew", err)
			return err
		}
		return emitSuccess(rt, "domains renew", res)
	case "renew-bulk":
		if len(rest) == 0 {
			err := usageError("domains renew-bulk <file>")
			emitError(rt, "domains renew-bulk", err)
			return err
		}
		app.MaybeWarnProdFinancial(rt, "domains renew-bulk")
		domains, err := services.LoadDomainFile(rest[0])
		if err != nil {
			ae := &apperr.AppError{Code: apperr.CodeValidation, Message: "failed reading domain list", Cause: err}
			emitError(rt, "domains renew-bulk", ae)
			return ae
		}
		flags := parseKVFlags(rest[1:])
		years := parseIntDefault(flags["years"], 1)
		dryRun := hasBoolFlag(rest[1:], "dry-run")
		autoApprove := hasBoolFlag(rest[1:], "auto-approve")
		results := make([]any, 0, len(domains))
		failed := 0
		for i, d := range domains {
			res, err := svc.Renew(rt.Ctx, d, years, dryRun, autoApprove)
			if err != nil {
				failed++
				results = append(results, map[string]any{"index": i, "input": d, "success": false, "error": err.Error(), "duration_ms": 0})
				continue
			}
			results = append(results, map[string]any{"index": i, "input": d, "success": true, "result": res, "duration_ms": 0})
		}
		if err := emitSuccess(rt, "domains renew-bulk", results); err != nil {
			return err
		}
		if failed > 0 {
			return &apperr.AppError{Code: apperr.CodePartial, Message: fmt.Sprintf("%d renewals failed", failed), Details: map[string]any{"failed": failed, "total": len(domains)}}
		}
		return nil
	case "list":
		flags := parseKVFlags(rest)
		expiring := parseIntDefault(flags["expiring-in"], 0)
		tld := flags["tld"]
		contains := flags["contains"]
		res, err := svc.ListPortfolio(rt.Ctx, expiring, tld, contains)
		if err != nil {
			emitError(rt, "domains list", err)
			return err
		}
		return emitSuccess(rt, "domains list", map[string]any{"domains": res})
	default:
		err := usageError("unknown domains subcommand: " + sub)
		emitError(rt, "domains", err)
		return err
	}
}

func runDNS(rt *app.Runtime, args []string) error {
	if len(args) == 0 || isHelpToken(args[0]) {
		return emitSuccess(rt, "dns help", map[string]any{
			"subcommands": []string{"audit", "apply"},
		})
	}
	if len(args) == 0 {
		err := usageError("missing dns subcommand")
		emitError(rt, "dns", err)
		return err
	}
	svc, err := newService(rt)
	if err != nil {
		emitError(rt, "dns", err)
		return err
	}
	sub := args[0]
	rest := args[1:]
	flags := parseKVFlags(rest)
	switch sub {
	case "audit":
		file := flags["domains"]
		if file == "" {
			err := usageError("dns audit --domains <file>")
			emitError(rt, "dns audit", err)
			return err
		}
		domains, err := services.LoadDomainFile(file)
		if err != nil {
			ae := &apperr.AppError{Code: apperr.CodeValidation, Message: "failed reading domain list", Cause: err}
			emitError(rt, "dns audit", ae)
			return ae
		}
		res, err := svc.DNSAudit(rt.Ctx, domains)
		if err != nil {
			emitError(rt, "dns audit", err)
			return err
		}
		return emitSuccess(rt, "dns audit", res)
	case "apply":
		file := flags["domains"]
		tmpl := flags["template"]
		dryRun := hasBoolFlag(rest, "dry-run")
		if file == "" || tmpl == "" {
			err := usageError("dns apply --template <t> --domains <file>")
			emitError(rt, "dns apply", err)
			return err
		}
		domains, err := services.LoadDomainFile(file)
		if err != nil {
			ae := &apperr.AppError{Code: apperr.CodeValidation, Message: "failed reading domain list", Cause: err}
			emitError(rt, "dns apply", ae)
			return ae
		}
		res, err := svc.DNSApplyTemplate(rt.Ctx, tmpl, domains, dryRun)
		if err != nil {
			emitError(rt, "dns apply", err)
			return err
		}
		return emitSuccess(rt, "dns apply", res)
	default:
		err := usageError("unknown dns subcommand: " + sub)
		emitError(rt, "dns", err)
		return err
	}
}

func runSettings(rt *app.Runtime, args []string) error {
	if len(args) == 0 || isHelpToken(args[0]) {
		return emitSuccess(rt, "settings help", map[string]any{
			"subcommands": []string{"auto-purchase enable", "auto-purchase disable", "caps set", "show"},
		})
	}
	if len(args) == 0 {
		err := usageError("missing settings subcommand")
		emitError(rt, "settings", err)
		return err
	}
	switch args[0] {
	case "auto-purchase":
		if len(args) < 2 {
			err := usageError("settings auto-purchase <enable|disable>")
			emitError(rt, "settings auto-purchase", err)
			return err
		}
		action := args[1]
		flags := parseKVFlags(args[2:])
		switch action {
		case "enable":
			ack := flags["ack"]
			hash, err := safety.EnableAutoPurchase(ack)
			if err != nil {
				emitError(rt, "settings auto-purchase enable", err)
				return err
			}
			rt.Cfg.AutoPurchaseEnabled = true
			rt.Cfg.AcknowledgmentHash = hash
			if err := config.Save(rt.Cfg); err != nil {
				ae := &apperr.AppError{Code: apperr.CodeInternal, Message: "failed saving config", Cause: err}
				emitError(rt, "settings auto-purchase enable", ae)
				return ae
			}
			return emitSuccess(rt, "settings auto-purchase enable", map[string]any{"auto_purchase_enabled": true})
		case "disable":
			rt.Cfg.AutoPurchaseEnabled = false
			if err := config.Save(rt.Cfg); err != nil {
				ae := &apperr.AppError{Code: apperr.CodeInternal, Message: "failed saving config", Cause: err}
				emitError(rt, "settings auto-purchase disable", ae)
				return ae
			}
			return emitSuccess(rt, "settings auto-purchase disable", map[string]any{"auto_purchase_enabled": false})
		default:
			err := usageError("settings auto-purchase <enable|disable>")
			emitError(rt, "settings auto-purchase", err)
			return err
		}
	case "caps":
		if len(args) < 2 || args[1] != "set" {
			err := usageError("settings caps set --max-price <usd> --max-daily-spend <usd> --max-domains-per-day <n>")
			emitError(rt, "settings caps", err)
			return err
		}
		flags := parseKVFlags(args[2:])
		maxPrice := parseFloatDefault(flags["max-price"], -1)
		maxDaily := parseFloatDefault(flags["max-daily-spend"], -1)
		maxDomains := parseIntDefault(flags["max-domains-per-day"], -1)
		if maxPrice <= 0 || maxDaily <= 0 || maxDomains <= 0 {
			err := &apperr.AppError{Code: apperr.CodeValidation, Message: "cap values must be positive"}
			emitError(rt, "settings caps set", err)
			return err
		}
		rt.Cfg.MaxPricePerDomain = maxPrice
		rt.Cfg.MaxDailySpend = maxDaily
		rt.Cfg.MaxDomainsPerDay = maxDomains
		if err := config.Save(rt.Cfg); err != nil {
			ae := &apperr.AppError{Code: apperr.CodeInternal, Message: "failed saving config", Cause: err}
			emitError(rt, "settings caps set", ae)
			return ae
		}
		return emitSuccess(rt, "settings caps set", map[string]any{"max_price_per_domain": maxPrice, "max_daily_spend": maxDaily, "max_domains_per_day": maxDomains})
	case "show":
		redacted := map[string]any{
			"api_environment":             rt.Cfg.APIEnvironment,
			"auto_purchase_enabled":       rt.Cfg.AutoPurchaseEnabled,
			"acknowledgment_hash_present": rt.Cfg.AcknowledgmentHash != "",
			"max_price_per_domain":        rt.Cfg.MaxPricePerDomain,
			"max_daily_spend":             rt.Cfg.MaxDailySpend,
			"max_domains_per_day":         rt.Cfg.MaxDomainsPerDay,
			"default_years":               rt.Cfg.DefaultYears,
			"default_dns_template":        rt.Cfg.DefaultDNSTemplate,
			"output_default":              rt.Cfg.OutputDefault,
		}
		return emitSuccess(rt, "settings show", redacted)
	default:
		err := usageError("unknown settings subcommand: " + args[0])
		emitError(rt, "settings", err)
		return err
	}
}

func parseKVFlags(args []string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if !strings.HasPrefix(tok, "--") {
			continue
		}
		key := strings.TrimPrefix(tok, "--")
		if strings.Contains(key, "=") {
			parts := strings.SplitN(key, "=", 2)
			out[parts[0]] = parts[1]
			continue
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			out[key] = args[i+1]
			i++
			continue
		}
		out[key] = "true"
	}
	return out
}

func hasBoolFlag(args []string, name string) bool {
	needleA := "--" + name
	needleB := "--" + name + "=true"
	for _, t := range args {
		if t == needleA || t == needleB {
			return true
		}
	}
	return false
}

func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseIntDefault(v string, d int) int {
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return n
}

func parseFloatDefault(v string, d float64) float64 {
	if v == "" {
		return d
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return d
	}
	return n
}

func usageError(msg string) error {
	return &apperr.AppError{Code: apperr.CodeValidation, Message: msg}
}

func isHelpToken(v string) bool {
	return v == "--help" || v == "-h" || v == "help"
}

func newService(rt *app.Runtime) (*services.Service, error) {
	creds, err := app.LoadCredentials()
	if err != nil {
		return nil, err
	}
	client, err := godaddy.NewHTTPClient(app.BaseURL(rt.Cfg.APIEnvironment), creds.APIKey(), creds.APISecret())
	if err != nil {
		return nil, err
	}
	return services.New(rt, client), nil
}

func requestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func emitSuccess(rt *app.Runtime, command string, result any) error {
	if rt.NDJSON {
		records, ok := result.([]any)
		if !ok {
			if rs, ok2 := result.([]map[string]any); ok2 {
				records = make([]any, 0, len(rs))
				for _, v := range rs {
					records = append(records, v)
				}
			}
		}
		if records == nil {
			records = []any{result}
		}
		return rt.Out.EmitNDJSON(command, rt.RequestID, records)
	}
	return rt.Out.EmitJSON(command, rt.RequestID, result, nil)
}

func emitError(rt *app.Runtime, command string, err error) {
	var ae *apperr.AppError
	if !apperr.As(err, &ae) {
		ae = &apperr.AppError{Code: apperr.CodeInternal, Message: err.Error()}
	}
	_ = rt.Out.EmitJSON(command, rt.RequestID, nil, ae)
	if !rt.Quiet {
		output.LogErr(rt.ErrOut, "error: %s", err)
	}
}
