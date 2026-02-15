package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sportwhiz/gdcli/internal/app"
	"github.com/sportwhiz/gdcli/internal/budget"
	apperr "github.com/sportwhiz/gdcli/internal/errors"
	"github.com/sportwhiz/gdcli/internal/godaddy"
	"github.com/sportwhiz/gdcli/internal/idempotency"
	"github.com/sportwhiz/gdcli/internal/rate"
	"github.com/sportwhiz/gdcli/internal/safety"
	"github.com/sportwhiz/gdcli/internal/store"
)

type Service struct {
	RT     *app.Runtime
	Client godaddy.Client
}

type BulkAvailabilityItem struct {
	Index    int                  `json:"index"`
	Input    string               `json:"input"`
	Success  bool                 `json:"success"`
	Result   godaddy.Availability `json:"result,omitempty"`
	Error    string               `json:"error,omitempty"`
	Duration int64                `json:"duration_ms"`
}

func New(rt *app.Runtime, client godaddy.Client) *Service {
	return &Service{RT: rt, Client: client}
}

func (s *Service) Suggest(ctx context.Context, query string, tlds []string, limit int) (map[string]any, error) {
	var out []godaddy.Suggestion
	err := rate.Retry(ctx, 3, func() (bool, error) {
		if err := s.RT.Limiter.Wait(ctx); err != nil {
			return false, err
		}
		r, err := s.Client.Suggest(ctx, query, tlds, limit)
		out = r
		if err == nil {
			return false, nil
		}
		var ae *apperr.AppError
		if apperr.As(err, &ae) {
			return ae.Retryable || ae.Code == apperr.CodeRateLimited, err
		}
		return true, err
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"query": query, "suggestions": out}, nil
}

func (s *Service) Availability(ctx context.Context, domain string) (godaddy.Availability, error) {
	var out godaddy.Availability
	err := rate.Retry(ctx, 3, func() (bool, error) {
		if err := s.RT.Limiter.Wait(ctx); err != nil {
			return false, err
		}
		r, err := s.Client.Available(ctx, domain)
		out = r
		if err == nil {
			return false, nil
		}
		var ae *apperr.AppError
		if apperr.As(err, &ae) {
			return ae.Retryable || ae.Code == apperr.CodeRateLimited, err
		}
		return true, err
	})
	return out, err
}

func (s *Service) AvailabilityBulk(ctx context.Context, domains []string) ([]godaddy.Availability, error) {
	var out []godaddy.Availability
	err := rate.Retry(ctx, 3, func() (bool, error) {
		if err := s.RT.Limiter.Wait(ctx); err != nil {
			return false, err
		}
		r, err := s.Client.AvailableBulk(ctx, domains)
		out = r
		if err == nil {
			return false, nil
		}
		var ae *apperr.AppError
		if apperr.As(err, &ae) {
			return ae.Retryable || ae.Code == apperr.CodeRateLimited, err
		}
		return true, err
	})
	return out, err
}

func (s *Service) AvailabilityBulkConcurrent(ctx context.Context, domains []string, concurrency int) ([]BulkAvailabilityItem, error) {
	if concurrency < 1 {
		concurrency = 1
	}
	type job struct {
		idx    int
		domain string
	}
	type result struct {
		item BulkAvailabilityItem
		err  error
	}
	jobs := make(chan job)
	results := make(chan result, len(domains))
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			start := time.Now()
			r, err := s.Availability(ctx, j.domain)
			item := BulkAvailabilityItem{
				Index:    j.idx,
				Input:    j.domain,
				Success:  err == nil,
				Duration: time.Since(start).Milliseconds(),
			}
			if err != nil {
				item.Error = err.Error()
				results <- result{item: item, err: err}
				continue
			}
			item.Result = r
			results <- result{item: item}
		}
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go worker()
	}
	for i, d := range domains {
		jobs <- job{idx: i, domain: d}
	}
	close(jobs)
	wg.Wait()
	close(results)

	out := make([]BulkAvailabilityItem, len(domains))
	failures := 0
	for r := range results {
		out[r.item.Index] = r.item
		if r.err != nil {
			failures++
		}
	}
	if failures > 0 {
		return out, &apperr.AppError{
			Code:    apperr.CodePartial,
			Message: fmt.Sprintf("%d availability checks failed", failures),
			Details: map[string]any{"failed": failures, "total": len(domains)},
		}
	}
	return out, nil
}

func (s *Service) PurchaseDryRun(ctx context.Context, domain string, years int) (map[string]any, error) {
	avail, err := s.Availability(ctx, domain)
	if err != nil {
		return nil, err
	}
	if !avail.Available {
		return nil, &apperr.AppError{Code: apperr.CodeValidation, Message: "domain is not available", Details: map[string]any{"domain": domain}}
	}
	if err := budget.CheckPrice(s.RT.Cfg, avail.Price, avail.Currency); err != nil {
		return nil, err
	}
	if err := budget.CheckDailyCaps(s.RT.Cfg, time.Now(), avail.Price); err != nil {
		return nil, err
	}
	opKey := idempotency.OperationKey("purchase", domain, avail.Price, time.Now())
	token, err := safety.IssueToken(domain, avail.Price, avail.Currency, opKey, time.Now())
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"domain":                domain,
		"years":                 years,
		"price":                 avail.Price,
		"currency":              avail.Currency,
		"requires_confirmation": true,
		"confirmation_token":    token.TokenID,
		"token_expires_at":      token.ExpiresAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *Service) PurchaseConfirm(ctx context.Context, domain, token string, years int) (godaddy.PurchaseResult, error) {
	tok, err := safety.ValidateAndUseToken(token, domain, time.Now())
	if err != nil {
		return godaddy.PurchaseResult{}, err
	}
	if err := budget.CheckPrice(s.RT.Cfg, tok.QuotedPrice, tok.Currency); err != nil {
		return godaddy.PurchaseResult{}, err
	}
	if err := budget.CheckDailyCaps(s.RT.Cfg, time.Now(), tok.QuotedPrice); err != nil {
		return godaddy.PurchaseResult{}, err
	}

	already, err := idempotency.AlreadySucceeded(tok.OperationKey)
	if err != nil {
		return godaddy.PurchaseResult{}, err
	}
	if already {
		return godaddy.PurchaseResult{Domain: domain, Price: tok.QuotedPrice, Currency: tok.Currency, AlreadyBought: true}, nil
	}

	var result godaddy.PurchaseResult
	err = rate.Retry(ctx, 3, func() (bool, error) {
		if err := s.RT.Limiter.Wait(ctx); err != nil {
			return false, err
		}
		r, err := s.Client.Purchase(ctx, domain, years, tok.OperationKey)
		result = r
		if err == nil {
			return false, nil
		}
		var ae *apperr.AppError
		if apperr.As(err, &ae) {
			return ae.Retryable || ae.Code == apperr.CodeRateLimited, err
		}
		return true, err
	})
	if err != nil {
		return godaddy.PurchaseResult{}, err
	}

	if result.Price == 0 {
		result.Price = tok.QuotedPrice
	}
	if result.Currency == "" {
		result.Currency = tok.Currency
	}
	_ = store.AppendOperation(store.Operation{
		OperationID: tok.OperationKey,
		Type:        "purchase",
		Domain:      domain,
		Amount:      result.Price,
		Currency:    result.Currency,
		CreatedAt:   time.Now(),
		Status:      "succeeded",
	})
	return result, nil
}

func (s *Service) PurchaseAuto(ctx context.Context, domain string, years int) (godaddy.PurchaseResult, error) {
	if err := safety.RequireAutoEnabled(s.RT.Cfg.AutoPurchaseEnabled, s.RT.Cfg.AcknowledgmentHash); err != nil {
		return godaddy.PurchaseResult{}, err
	}
	avail, err := s.Availability(ctx, domain)
	if err != nil {
		return godaddy.PurchaseResult{}, err
	}
	if !avail.Available {
		return godaddy.PurchaseResult{}, &apperr.AppError{Code: apperr.CodeValidation, Message: "domain is not available", Details: map[string]any{"domain": domain}}
	}
	if err := budget.CheckPrice(s.RT.Cfg, avail.Price, avail.Currency); err != nil {
		return godaddy.PurchaseResult{}, err
	}
	if err := budget.CheckDailyCaps(s.RT.Cfg, time.Now(), avail.Price); err != nil {
		return godaddy.PurchaseResult{}, err
	}
	opKey := idempotency.OperationKey("purchase", domain, avail.Price, time.Now())
	already, err := idempotency.AlreadySucceeded(opKey)
	if err != nil {
		return godaddy.PurchaseResult{}, err
	}
	if already {
		return godaddy.PurchaseResult{Domain: domain, Price: avail.Price, Currency: avail.Currency, AlreadyBought: true}, nil
	}
	var result godaddy.PurchaseResult
	err = rate.Retry(ctx, 3, func() (bool, error) {
		if err := s.RT.Limiter.Wait(ctx); err != nil {
			return false, err
		}
		r, err := s.Client.Purchase(ctx, domain, years, opKey)
		result = r
		if err == nil {
			return false, nil
		}
		var ae *apperr.AppError
		if apperr.As(err, &ae) {
			return ae.Retryable || ae.Code == apperr.CodeRateLimited, err
		}
		return true, err
	})
	if err != nil {
		return godaddy.PurchaseResult{}, err
	}
	if result.Price == 0 {
		result.Price = avail.Price
	}
	if result.Currency == "" {
		result.Currency = avail.Currency
	}
	_ = store.AppendOperation(store.Operation{
		OperationID: opKey,
		Type:        "purchase",
		Domain:      domain,
		Amount:      result.Price,
		Currency:    result.Currency,
		CreatedAt:   time.Now(),
		Status:      "succeeded",
	})
	return result, nil
}

func (s *Service) Renew(ctx context.Context, domain string, years int, dryRun bool, autoApprove bool) (map[string]any, error) {
	if !dryRun && !autoApprove {
		dryRun = true
	}
	priceEstimate := 12.99
	currency := "USD"
	if err := budget.CheckPrice(s.RT.Cfg, priceEstimate, currency); err != nil {
		return nil, err
	}
	if err := budget.CheckDailyCaps(s.RT.Cfg, time.Now(), priceEstimate); err != nil {
		return nil, err
	}
	if dryRun {
		return map[string]any{"domain": domain, "years": years, "dry_run": true, "price": priceEstimate, "currency": currency}, nil
	}
	opKey := idempotency.OperationKey("renew", domain, priceEstimate, time.Now())
	already, err := idempotency.AlreadySucceeded(opKey)
	if err != nil {
		return nil, err
	}
	if already {
		return map[string]any{"domain": domain, "already_renewed": true, "price": priceEstimate, "currency": currency}, nil
	}
	var rr godaddy.RenewResult
	err = rate.Retry(ctx, 3, func() (bool, error) {
		if err := s.RT.Limiter.Wait(ctx); err != nil {
			return false, err
		}
		r, err := s.Client.Renew(ctx, domain, years, opKey)
		rr = r
		if err == nil {
			return false, nil
		}
		var ae *apperr.AppError
		if apperr.As(err, &ae) {
			return ae.Retryable || ae.Code == apperr.CodeRateLimited, err
		}
		return true, err
	})
	if err != nil {
		return nil, err
	}
	if rr.Price == 0 {
		rr.Price = priceEstimate
	}
	if rr.Currency == "" {
		rr.Currency = currency
	}
	_ = store.AppendOperation(store.Operation{OperationID: opKey, Type: "renew", Domain: domain, Amount: rr.Price, Currency: rr.Currency, CreatedAt: time.Now(), Status: "succeeded"})
	return map[string]any{"domain": domain, "years": years, "dry_run": false, "price": rr.Price, "currency": rr.Currency, "order_id": rr.OrderID}, nil
}

func (s *Service) ListPortfolio(ctx context.Context, expiringIn int, tld, contains string) ([]godaddy.PortfolioDomain, error) {
	var all []godaddy.PortfolioDomain
	err := rate.Retry(ctx, 3, func() (bool, error) {
		if err := s.RT.Limiter.Wait(ctx); err != nil {
			return false, err
		}
		r, err := s.Client.ListDomains(ctx)
		all = r
		if err == nil {
			return false, nil
		}
		var ae *apperr.AppError
		if apperr.As(err, &ae) {
			return ae.Retryable || ae.Code == apperr.CodeRateLimited, err
		}
		return true, err
	})
	if err != nil {
		return nil, err
	}
	out := make([]godaddy.PortfolioDomain, 0, len(all))
	now := time.Now()
	for _, d := range all {
		if tld != "" && !strings.HasSuffix(strings.ToLower(d.Domain), "."+strings.ToLower(tld)) {
			continue
		}
		if contains != "" && !strings.Contains(strings.ToLower(d.Domain), strings.ToLower(contains)) {
			continue
		}
		if expiringIn > 0 {
			exp, err := time.Parse("2006-01-02", d.Expires)
			if err == nil {
				if exp.After(now.Add(time.Duration(expiringIn) * 24 * time.Hour)) {
					continue
				}
			}
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *Service) OrdersList(ctx context.Context, limit, offset int) (map[string]any, error) {
	var out godaddy.OrdersPage
	err := rate.Retry(ctx, 3, func() (bool, error) {
		if err := s.RT.Limiter.Wait(ctx); err != nil {
			return false, err
		}
		r, err := s.Client.ListOrders(ctx, limit, offset)
		out = r
		if err == nil {
			return false, nil
		}
		var ae *apperr.AppError
		if apperr.As(err, &ae) {
			return ae.Retryable || ae.Code == apperr.CodeRateLimited, err
		}
		return true, err
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"orders":     out.Orders,
		"pagination": out.Pagination,
	}, nil
}

func (s *Service) SubscriptionsList(ctx context.Context, limit, offset int) (map[string]any, error) {
	var out godaddy.SubscriptionsPage
	err := rate.Retry(ctx, 3, func() (bool, error) {
		if err := s.RT.Limiter.Wait(ctx); err != nil {
			return false, err
		}
		r, err := s.Client.ListSubscriptions(ctx, limit, offset)
		out = r
		if err == nil {
			return false, nil
		}
		var ae *apperr.AppError
		if apperr.As(err, &ae) {
			return ae.Retryable || ae.Code == apperr.CodeRateLimited, err
		}
		return true, err
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"subscriptions": out.Subscriptions,
		"pagination":    out.Pagination,
	}, nil
}

func (s *Service) DNSAudit(ctx context.Context, domains []string) ([]map[string]any, error) {
	results := make([]map[string]any, 0, len(domains))
	for _, d := range domains {
		ns, err := s.Client.GetNameservers(ctx, d)
		if err != nil {
			results = append(results, map[string]any{"domain": d, "issues": []string{"nameserver_fetch_failed"}, "error": err.Error()})
			continue
		}
		recs, err := s.Client.GetRecords(ctx, d)
		if err != nil {
			results = append(results, map[string]any{"domain": d, "issues": []string{"records_fetch_failed"}, "error": err.Error()})
			continue
		}
		issues := make([]string, 0)
		afternic := len(ns) >= 2 && strings.EqualFold(ns[0], "ns1.afternic.com") && strings.EqualFold(ns[1], "ns2.afternic.com")
		if !afternic {
			issues = append(issues, "nameservers_not_afternic")
		}
		hasTXT := false
		hasA := false
		for _, r := range recs {
			if strings.EqualFold(r.Type, "TXT") {
				hasTXT = true
			}
			if strings.EqualFold(r.Type, "A") {
				hasA = true
			}
		}
		if !hasTXT {
			issues = append(issues, "missing_txt_verification")
		}
		if !hasA {
			issues = append(issues, "missing_a_record")
		}
		results = append(results, map[string]any{"domain": d, "afternic_pointed": afternic, "issues": issues})
	}
	return results, nil
}

func (s *Service) DNSApplyTemplate(ctx context.Context, tmpl string, domains []string, dryRun bool) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(domains))
	ns := []string{"ns1.afternic.com", "ns2.afternic.com"}
	var custom *dnsTemplateFile
	if strings.HasSuffix(strings.ToLower(tmpl), ".json") {
		c, err := loadCustomTemplate(tmpl)
		if err != nil {
			return nil, err
		}
		custom = c
	}
	for _, d := range domains {
		if dryRun {
			out = append(out, map[string]any{"domain": d, "template": tmpl, "dry_run": true, "changes": []string{"set_nameservers"}})
			continue
		}
		switch tmpl {
		case "afternic", "afternic-nameservers":
			if err := s.Client.SetNameservers(ctx, d, ns); err != nil {
				out = append(out, map[string]any{"domain": d, "applied": false, "error": err.Error()})
				continue
			}
		case "parking":
			recs := []godaddy.DNSRecord{{Type: "A", Name: "@", Data: "52.71.57.184", TTL: 600}}
			if err := s.Client.SetRecords(ctx, d, recs); err != nil {
				out = append(out, map[string]any{"domain": d, "applied": false, "error": err.Error()})
				continue
			}
		default:
			if custom != nil {
				if len(custom.NameServers) > 0 {
					if err := s.Client.SetNameservers(ctx, d, custom.NameServers); err != nil {
						out = append(out, map[string]any{"domain": d, "applied": false, "error": err.Error()})
						continue
					}
				}
				if len(custom.Records) > 0 {
					if err := s.Client.SetRecords(ctx, d, custom.Records); err != nil {
						out = append(out, map[string]any{"domain": d, "applied": false, "error": err.Error()})
						continue
					}
				}
			} else {
				return nil, &apperr.AppError{Code: apperr.CodeValidation, Message: "unsupported template", Details: map[string]any{"template": tmpl}}
			}
		}
		out = append(out, map[string]any{"domain": d, "template": tmpl, "applied": true})
	}
	return out, nil
}

type dnsTemplateFile struct {
	NameServers []string            `json:"nameservers"`
	Records     []godaddy.DNSRecord `json:"records"`
}

func loadCustomTemplate(path string) (*dnsTemplateFile, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	abs = filepath.Clean(abs)
	// #nosec G304 -- custom template path is intentionally user-provided local file input.
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, &apperr.AppError{Code: apperr.CodeValidation, Message: "custom template file not found", Details: map[string]any{"template": abs}}
	}
	var tmpl dnsTemplateFile
	if err := json.Unmarshal(b, &tmpl); err != nil {
		return nil, &apperr.AppError{Code: apperr.CodeValidation, Message: "invalid custom template JSON", Cause: err}
	}
	if len(tmpl.NameServers) == 0 && len(tmpl.Records) == 0 {
		return nil, &apperr.AppError{Code: apperr.CodeValidation, Message: "custom template must include nameservers or records"}
	}
	return &tmpl, nil
}

func LoadDomainFile(path string) ([]string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	abs = filepath.Clean(abs)
	// #nosec G304 -- domain list path is intentionally user-provided local file input.
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no domains found in %s", abs)
	}
	return out, nil
}
