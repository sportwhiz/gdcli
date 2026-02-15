package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sportwhiz/gdcli/internal/app"
	"github.com/sportwhiz/gdcli/internal/budget"
	apperr "github.com/sportwhiz/gdcli/internal/errors"
	"github.com/sportwhiz/gdcli/internal/godaddy"
	"github.com/sportwhiz/gdcli/internal/idempotency"
	"github.com/sportwhiz/gdcli/internal/output"
	"github.com/sportwhiz/gdcli/internal/rate"
	"github.com/sportwhiz/gdcli/internal/safety"
	"github.com/sportwhiz/gdcli/internal/store"
)

type Service struct {
	RT     *app.Runtime
	Client godaddy.Client
}

type renewAsShopperClient interface {
	RenewAsShopper(ctx context.Context, shopperID, domain string, years int, idempotencyKey string) (godaddy.RenewResult, error)
}

type v2RouterClient interface {
	ResolveCustomerID(ctx context.Context, shopperID string) (string, error)
	DomainDetailV2(ctx context.Context, customerID, domain string, includes []string) (map[string]any, error)
	DomainDetailV1(ctx context.Context, domain string) (map[string]any, error)
	RenewV2(ctx context.Context, customerID, domain string, req godaddy.RenewV2Request, idempotencyKey string) (godaddy.RenewResult, error)
	SetNameserversV2(ctx context.Context, customerID, domain string, nameservers []string) error
	V2Get(ctx context.Context, path string, query url.Values, out any) error
	V2Post(ctx context.Context, path string, body any, out any, idempotencyKey string) error
	V2Put(ctx context.Context, path string, body any, out any) error
	V2Patch(ctx context.Context, path string, body any, out any) error
}

func canUseV2(customerID string) bool {
	return strings.TrimSpace(customerID) != ""
}

func doV2ThenV1[T any](useV2 bool, runV2 func() (T, error), runV1 func() (T, error)) (T, bool, error) {
	var zero T
	if !useV2 {
		v1, err := runV1()
		return v1, false, err
	}
	v2, err := runV2()
	if err == nil {
		return v2, true, nil
	}
	v1, v1Err := runV1()
	if v1Err == nil {
		return v1, false, nil
	}
	return zero, false, v1Err
}

func isInvalidPaymentInfo(err error) bool {
	var ae *apperr.AppError
	if !apperr.As(err, &ae) || ae == nil || ae.Code != apperr.CodeProvider || ae.Details == nil {
		return false
	}
	providerRaw, ok := ae.Details["provider"]
	if !ok {
		return false
	}
	provider, ok := providerRaw.(map[string]any)
	if !ok {
		return false
	}
	code, _ := provider["code"].(string)
	return strings.EqualFold(strings.TrimSpace(code), "INVALID_PAYMENT_INFO")
}

func enrichRenewError(err error) error {
	if !isInvalidPaymentInfo(err) {
		return err
	}
	var ae *apperr.AppError
	_ = apperr.As(err, &ae)
	details := map[string]any{}
	if ae != nil && ae.Details != nil {
		for k, v := range ae.Details {
			details[k] = v
		}
	}
	details["remediation"] = "Fund your GoDaddy Good As Gold balance or update your default payment profile, then retry renewal."
	return &apperr.AppError{
		Code:      apperr.CodeProvider,
		Message:   "renewal failed: invalid payment info. Fund Good As Gold or update payment profile in GoDaddy.",
		Details:   details,
		Retryable: false,
		Cause:     err,
	}
}

func renewPriceMicros(v any) (int64, error) {
	switch n := v.(type) {
	case int:
		return int64(n), nil
	case int64:
		return n, nil
	case float64:
		if math.Mod(n, 1.0) != 0 {
			return 0, fmt.Errorf("renewal price must be integer micros")
		}
		return int64(n), nil
	case float32:
		f := float64(n)
		if math.Mod(f, 1.0) != 0 {
			return 0, fmt.Errorf("renewal price must be integer micros")
		}
		return int64(f), nil
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i, nil
		}
		return 0, fmt.Errorf("renewal price is not an integer")
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("renewal price is not an integer")
		}
		return i, nil
	default:
		return 0, fmt.Errorf("unsupported renewal price type %T", v)
	}
}

func (s *Service) buildRenewV2Request(ctx context.Context, v2c v2RouterClient, customerID, domain string, years int) (godaddy.RenewV2Request, error) {
	detail, err := v2c.DomainDetailV2(ctx, customerID, domain, nil)
	if err != nil {
		return godaddy.RenewV2Request{}, err
	}
	expires, _ := detail["expiresAt"].(string)
	if strings.TrimSpace(expires) == "" {
		return godaddy.RenewV2Request{}, &apperr.AppError{
			Code:    apperr.CodeProvider,
			Message: "v2 renew requires domain expiration from domain detail response",
		}
	}
	renewal, ok := detail["renewal"].(map[string]any)
	if !ok {
		return godaddy.RenewV2Request{}, &apperr.AppError{
			Code:    apperr.CodeProvider,
			Message: "v2 renew requires renewal pricing from domain detail response",
		}
	}
	priceMicros, err := renewPriceMicros(renewal["price"])
	if err != nil || priceMicros <= 0 {
		return godaddy.RenewV2Request{}, &apperr.AppError{
			Code:    apperr.CodeProvider,
			Message: "v2 renew requires valid renewal price in micro-units from domain detail response",
			Cause:   err,
		}
	}
	currency, _ := renewal["currency"].(string)
	if strings.TrimSpace(currency) == "" {
		currency = "USD"
	}
	agreedBy := strings.TrimSpace(os.Getenv("GDCLI_AGREED_BY_IP"))
	if agreedBy == "" {
		agreedBy = "127.0.0.1"
	}
	return godaddy.RenewV2Request{
		Expires: expires,
		Period:  years,
		Consent: godaddy.RenewV2Consent{
			Price:    priceMicros,
			Currency: strings.ToUpper(currency),
			AgreedBy: agreedBy,
			AgreedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}, nil
}

func (s *Service) renewV2CustomerCandidates() []string {
	out := make([]string, 0, 2)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		for _, cur := range out {
			if cur == v {
				return
			}
		}
		out = append(out, v)
	}
	// Keep stored customer_id as primary, then try shopper_id for endpoints that require it.
	add(s.RT.Cfg.CustomerID)
	add(s.RT.Cfg.ShopperID)
	return out
}

func (s *Service) v2Client() (v2RouterClient, bool) {
	c, ok := s.Client.(v2RouterClient)
	return c, ok
}

type BulkAvailabilityItem struct {
	Index    int                  `json:"index"`
	Input    string               `json:"input"`
	Success  bool                 `json:"success"`
	Result   godaddy.Availability `json:"result,omitempty"`
	Error    string               `json:"error,omitempty"`
	Duration int64                `json:"duration_ms"`
}

type PortfolioDetailItem struct {
	Index       int      `json:"index"`
	Domain      string   `json:"domain"`
	Expires     string   `json:"expires,omitempty"`
	NameServers []string `json:"nameServers,omitempty"`
	APIVersion  string   `json:"api_version,omitempty"`
	Success     bool     `json:"success"`
	Error       string   `json:"error,omitempty"`
}

func New(rt *app.Runtime, client godaddy.Client) *Service {
	return &Service{RT: rt, Client: client}
}

func (s *Service) appendOperationWithWarning(op store.Operation) {
	if err := store.AppendOperation(op); err != nil {
		output.LogErr(s.RT.ErrOut, "warning: failed writing operation log for operation_id=%s: %v", op.OperationID, err)
	}
}

func (s *Service) reserveOperation(opType, domain string, amount float64, currency, operationID string, now time.Time) (bool, error) {
	alreadySucceeded := false
	err := store.LoadAndSaveOperations(func(ops *[]store.Operation) error {
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		dayEnd := dayStart.Add(24 * time.Hour)

		totalSpend := 0.0
		totalDomains := 0
		for _, op := range *ops {
			if op.OperationID == operationID {
				switch op.Status {
				case "succeeded":
					alreadySucceeded = true
					return nil
				case "pending":
					return &apperr.AppError{
						Code:    apperr.CodeRateLimited,
						Message: "operation already in progress",
						Details: map[string]any{"operation_id": operationID},
					}
				}
			}
			if op.CreatedAt.Before(dayStart) || !op.CreatedAt.Before(dayEnd) {
				continue
			}
			if op.Type != "purchase" && op.Type != "renew" {
				continue
			}
			if op.Status != "succeeded" && op.Status != "pending" {
				continue
			}
			totalSpend += op.Amount
			totalDomains++
		}

		if totalSpend+amount > s.RT.Cfg.MaxDailySpend {
			return &apperr.AppError{
				Code:    apperr.CodeBudget,
				Message: "daily spend cap exceeded",
				Details: map[string]any{"attempted_total": totalSpend + amount, "max_daily_spend": s.RT.Cfg.MaxDailySpend},
			}
		}
		if totalDomains+1 > s.RT.Cfg.MaxDomainsPerDay {
			return &apperr.AppError{
				Code:    apperr.CodeBudget,
				Message: "daily domain count cap exceeded",
				Details: map[string]any{"attempted_total": totalDomains + 1, "max_domains_per_day": s.RT.Cfg.MaxDomainsPerDay},
			}
		}

		*ops = append(*ops, store.Operation{
			OperationID: operationID,
			Type:        opType,
			Domain:      domain,
			Amount:      amount,
			Currency:    currency,
			CreatedAt:   now,
			Status:      "pending",
		})
		return nil
	})
	if err != nil {
		return false, err
	}
	return alreadySucceeded, nil
}

func (s *Service) finalizeOperation(operationID string, amount float64, currency, status string) error {
	now := time.Now()
	var policyErr error
	err := store.LoadAndSaveOperations(func(ops *[]store.Operation) error {
		index := -1
		for i := range *ops {
			if (*ops)[i].OperationID == operationID {
				index = i
			}
		}
		if index < 0 {
			*ops = append(*ops, store.Operation{
				OperationID: operationID,
				Type:        "unknown",
				Amount:      amount,
				Currency:    currency,
				CreatedAt:   now,
				Status:      status,
			})
			return nil
		}

		op := (*ops)[index]
		if status == "succeeded" {
			dayStart := time.Date(op.CreatedAt.Year(), op.CreatedAt.Month(), op.CreatedAt.Day(), 0, 0, 0, 0, op.CreatedAt.Location())
			dayEnd := dayStart.Add(24 * time.Hour)
			totalSpend := 0.0
			totalDomains := 0
			for i, existing := range *ops {
				if i == index {
					continue
				}
				if existing.CreatedAt.Before(dayStart) || !existing.CreatedAt.Before(dayEnd) {
					continue
				}
				if existing.Type != "purchase" && existing.Type != "renew" {
					continue
				}
				if existing.Status != "succeeded" && existing.Status != "pending" {
					continue
				}
				totalSpend += existing.Amount
				totalDomains++
			}
			if totalSpend+amount > s.RT.Cfg.MaxDailySpend {
				policyErr = &apperr.AppError{
					Code:    apperr.CodeBudget,
					Message: "daily spend cap exceeded by finalized provider amount",
					Details: map[string]any{"attempted_total": totalSpend + amount, "max_daily_spend": s.RT.Cfg.MaxDailySpend},
				}
				status = "failed"
			}
			if totalDomains+1 > s.RT.Cfg.MaxDomainsPerDay {
				policyErr = &apperr.AppError{
					Code:    apperr.CodeBudget,
					Message: "daily domain count cap exceeded by finalized provider amount",
					Details: map[string]any{"attempted_total": totalDomains + 1, "max_domains_per_day": s.RT.Cfg.MaxDomainsPerDay},
				}
				status = "failed"
			}
		}

		op.Amount = amount
		if strings.TrimSpace(currency) != "" {
			op.Currency = currency
		}
		op.Status = status
		(*ops)[index] = op
		return nil
	})
	if err != nil {
		return err
	}
	return policyErr
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
		return nil, enrichRenewError(err)
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

func (s *Service) IdentityShow() map[string]any {
	return map[string]any{
		"shopper_id":               s.RT.Cfg.ShopperID,
		"customer_id":              s.RT.Cfg.CustomerID,
		"customer_id_resolved_at":  s.RT.Cfg.CustomerIDResolved,
		"customer_id_source":       s.RT.Cfg.CustomerIDSource,
		"v2_customer_scoped_ready": canUseV2(s.RT.Cfg.CustomerID),
	}
}

func (s *Service) ResolveAndStoreCustomerID(ctx context.Context, shopperID string) (string, error) {
	v2c, ok := s.v2Client()
	if !ok {
		return "", &apperr.AppError{Code: apperr.CodeInternal, Message: "client does not support v2 identity resolution"}
	}
	customerID, err := v2c.ResolveCustomerID(ctx, shopperID)
	if err != nil {
		return "", err
	}
	s.RT.Cfg.ShopperID = shopperID
	s.RT.Cfg.CustomerID = customerID
	s.RT.Cfg.CustomerIDResolved = time.Now().UTC().Format(time.RFC3339)
	s.RT.Cfg.CustomerIDSource = "shopper_lookup"
	return customerID, nil
}

func (s *Service) DomainDetail(ctx context.Context, domain string, includes []string) (map[string]any, error) {
	v2c, ok := s.v2Client()
	if !ok {
		return nil, &apperr.AppError{Code: apperr.CodeInternal, Message: "client does not support domain detail"}
	}
	out, usedV2, err := doV2ThenV1(
		canUseV2(s.RT.Cfg.CustomerID),
		func() (map[string]any, error) { return v2c.DomainDetailV2(ctx, s.RT.Cfg.CustomerID, domain, includes) },
		func() (map[string]any, error) { return v2c.DomainDetailV1(ctx, domain) },
	)
	if err != nil {
		return nil, err
	}
	out["_api_version"] = map[bool]string{true: "v2", false: "v1"}[usedV2]
	return out, nil
}

func (s *Service) SetNameserversSmart(ctx context.Context, domain string, nameservers []string) (string, error) {
	if v2c, ok := s.v2Client(); ok && canUseV2(s.RT.Cfg.CustomerID) {
		_, usedV2, err := doV2ThenV1(
			true,
			func() (struct{}, error) {
				return struct{}{}, v2c.SetNameserversV2(ctx, s.RT.Cfg.CustomerID, domain, nameservers)
			},
			func() (struct{}, error) {
				return struct{}{}, s.Client.SetNameservers(ctx, domain, nameservers)
			},
		)
		if err != nil {
			return "", err
		}
		if usedV2 {
			return "v2", nil
		}
		return "v1", nil
	}
	if err := s.Client.SetNameservers(ctx, domain, nameservers); err != nil {
		return "", err
	}
	return "v1", nil
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
	tok, err := safety.ValidateToken(token, domain, time.Now())
	if err != nil {
		return godaddy.PurchaseResult{}, err
	}
	if err := budget.CheckPrice(s.RT.Cfg, tok.QuotedPrice, tok.Currency); err != nil {
		return godaddy.PurchaseResult{}, err
	}
	already, err := s.reserveOperation("purchase", domain, tok.QuotedPrice, tok.Currency, tok.OperationKey, time.Now())
	if err != nil {
		return godaddy.PurchaseResult{}, err
	}
	if already {
		_ = safety.MarkTokenUsed(token, domain, time.Now())
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
		_ = s.finalizeOperation(tok.OperationKey, tok.QuotedPrice, tok.Currency, "failed")
		return godaddy.PurchaseResult{}, err
	}

	if result.Price == 0 {
		result.Price = tok.QuotedPrice
	}
	if result.Currency == "" {
		result.Currency = tok.Currency
	}
	if err := budget.CheckPrice(s.RT.Cfg, result.Price, result.Currency); err != nil {
		_ = s.finalizeOperation(tok.OperationKey, result.Price, result.Currency, "failed")
		return godaddy.PurchaseResult{}, err
	}
	if err := s.finalizeOperation(tok.OperationKey, result.Price, result.Currency, "succeeded"); err != nil {
		return godaddy.PurchaseResult{}, err
	}
	_ = safety.MarkTokenUsed(token, domain, time.Now())
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
	opKey := idempotency.OperationKey("purchase", domain, avail.Price, time.Now())
	already, err := s.reserveOperation("purchase", domain, avail.Price, avail.Currency, opKey, time.Now())
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
		_ = s.finalizeOperation(opKey, avail.Price, avail.Currency, "failed")
		return godaddy.PurchaseResult{}, err
	}
	if result.Price == 0 {
		result.Price = avail.Price
	}
	if result.Currency == "" {
		result.Currency = avail.Currency
	}
	if err := budget.CheckPrice(s.RT.Cfg, result.Price, result.Currency); err != nil {
		_ = s.finalizeOperation(opKey, result.Price, result.Currency, "failed")
		return godaddy.PurchaseResult{}, err
	}
	if err := s.finalizeOperation(opKey, result.Price, result.Currency, "succeeded"); err != nil {
		return godaddy.PurchaseResult{}, err
	}
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
	if dryRun {
		return map[string]any{"domain": domain, "years": years, "dry_run": true, "price": priceEstimate, "currency": currency}, nil
	}
	opKey := idempotency.OperationKey("renew", domain, priceEstimate, time.Now())
	already, err := s.reserveOperation("renew", domain, priceEstimate, currency, opKey, time.Now())
	if err != nil {
		return nil, err
	}
	if already {
		return map[string]any{"domain": domain, "already_renewed": true, "price": priceEstimate, "currency": currency}, nil
	}
	var rr godaddy.RenewResult
	usedV2 := false
	err = rate.Retry(ctx, 3, func() (bool, error) {
		if err := s.RT.Limiter.Wait(ctx); err != nil {
			return false, err
		}
		useV2 := canUseV2(s.RT.Cfg.CustomerID) || strings.TrimSpace(s.RT.Cfg.ShopperID) != ""
		var r godaddy.RenewResult
		if v2c, ok := s.v2Client(); ok && useV2 {
			out, used, callErr := doV2ThenV1(
				true,
				func() (godaddy.RenewResult, error) {
					var lastErr error
					for _, customerID := range s.renewV2CustomerCandidates() {
						req, reqErr := s.buildRenewV2Request(ctx, v2c, customerID, domain, years)
						if reqErr != nil {
							lastErr = reqErr
							continue
						}
						renewRes, renewErr := v2c.RenewV2(ctx, customerID, domain, req, opKey)
						if renewErr == nil {
							return renewRes, nil
						}
						lastErr = renewErr
					}
					if lastErr != nil {
						return godaddy.RenewResult{}, lastErr
					}
					return godaddy.RenewResult{}, &apperr.AppError{Code: apperr.CodeValidation, Message: "v2 renew requires customer_id or shopper_id"}
				},
				func() (godaddy.RenewResult, error) {
					if rc, ok := s.Client.(renewAsShopperClient); ok {
						shopper := strings.TrimSpace(s.RT.Cfg.ShopperID)
						if shopper != "" {
							return rc.RenewAsShopper(ctx, shopper, domain, years, opKey)
						}
					}
					return s.Client.Renew(ctx, domain, years, opKey)
				},
			)
			usedV2 = used
			r, err = out, callErr
		} else {
			if rc, ok := s.Client.(renewAsShopperClient); ok {
				shopper := strings.TrimSpace(s.RT.Cfg.ShopperID)
				if shopper != "" {
					r, err = rc.RenewAsShopper(ctx, shopper, domain, years, opKey)
				} else {
					r, err = s.Client.Renew(ctx, domain, years, opKey)
				}
			} else {
				r, err = s.Client.Renew(ctx, domain, years, opKey)
			}
			usedV2 = false
		}
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
		_ = s.finalizeOperation(opKey, priceEstimate, currency, "failed")
		return nil, enrichRenewError(err)
	}
	if rr.Price == 0 {
		rr.Price = priceEstimate
	}
	if rr.Currency == "" {
		rr.Currency = currency
	}
	if err := budget.CheckPrice(s.RT.Cfg, rr.Price, rr.Currency); err != nil {
		_ = s.finalizeOperation(opKey, rr.Price, rr.Currency, "failed")
		return nil, err
	}
	if err := s.finalizeOperation(opKey, rr.Price, rr.Currency, "succeeded"); err != nil {
		return nil, err
	}
	apiVersion := "v1"
	if usedV2 {
		apiVersion = "v2"
	}
	return map[string]any{"domain": domain, "years": years, "dry_run": false, "price": rr.Price, "currency": rr.Currency, "order_id": rr.OrderID, "api_version": apiVersion}, nil
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

func (s *Service) PortfolioWithNameservers(ctx context.Context, expiringIn int, tld, contains string, concurrency int) ([]PortfolioDetailItem, error) {
	domains, err := s.ListPortfolio(ctx, expiringIn, tld, contains)
	if err != nil {
		return nil, err
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 20 {
		concurrency = 20
	}

	type job struct {
		index int
		item  godaddy.PortfolioDomain
	}
	type result struct {
		item PortfolioDetailItem
		err  error
	}

	jobs := make(chan job)
	results := make(chan result, len(domains))
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			out := PortfolioDetailItem{
				Index:   j.index,
				Domain:  j.item.Domain,
				Expires: j.item.Expires,
				Success: true,
			}
			detail, err := s.DomainDetail(ctx, j.item.Domain, nil)
			if err != nil {
				out.Success = false
				out.Error = err.Error()
				results <- result{item: out, err: err}
				continue
			}
			if ns, ok := detail["nameServers"].([]any); ok {
				for _, n := range ns {
					if s, ok := n.(string); ok && strings.TrimSpace(s) != "" {
						out.NameServers = append(out.NameServers, s)
					}
				}
			}
			if v, ok := detail["_api_version"].(string); ok {
				out.APIVersion = v
			}
			results <- result{item: out}
		}
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go worker()
	}
	for i, d := range domains {
		jobs <- job{index: i, item: d}
	}
	close(jobs)
	wg.Wait()
	close(results)

	out := make([]PortfolioDetailItem, len(domains))
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
			Message: fmt.Sprintf("%d domain detail lookups failed", failures),
			Details: map[string]any{"failed": failures, "total": len(domains)},
		}
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

func (s *Service) requireV2() (v2RouterClient, string, error) {
	v2c, ok := s.v2Client()
	if !ok {
		return nil, "", &apperr.AppError{Code: apperr.CodeInternal, Message: "client does not support v2 operations"}
	}
	if !canUseV2(s.RT.Cfg.CustomerID) {
		return nil, "", &apperr.AppError{Code: apperr.CodeValidation, Message: "customer_id is not configured; run account identity set/resolve first"}
	}
	return v2c, s.RT.Cfg.CustomerID, nil
}

func (s *Service) V2Get(ctx context.Context, path string, q url.Values) (map[string]any, error) {
	v2c, _, err := s.requireV2()
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := v2c.V2Get(ctx, path, q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) V2Apply(ctx context.Context, method, path string, body any, idempotencyKey string) (map[string]any, error) {
	v2c, _, err := s.requireV2()
	if err != nil {
		return nil, err
	}
	var out map[string]any
	switch strings.ToUpper(method) {
	case "POST":
		err = v2c.V2Post(ctx, path, body, &out, idempotencyKey)
	case "PUT":
		err = v2c.V2Put(ctx, path, body, &out)
	case "PATCH":
		err = v2c.V2Patch(ctx, path, body, &out)
	default:
		return nil, &apperr.AppError{Code: apperr.CodeValidation, Message: "unsupported method", Details: map[string]any{"method": method}}
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) V2PathCustomer(pathTemplate string) (string, error) {
	_, customerID, err := s.requireV2()
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(pathTemplate, "{customerId}", url.PathEscape(customerID)), nil
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
			setNS := func() error {
				if v2c, ok := s.v2Client(); ok && canUseV2(s.RT.Cfg.CustomerID) {
					_, _, err := doV2ThenV1(
						true,
						func() (struct{}, error) {
							return struct{}{}, v2c.SetNameserversV2(ctx, s.RT.Cfg.CustomerID, d, ns)
						},
						func() (struct{}, error) {
							return struct{}{}, s.Client.SetNameservers(ctx, d, ns)
						},
					)
					return err
				}
				return s.Client.SetNameservers(ctx, d, ns)
			}
			if err := setNS(); err != nil {
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
					setCustomNS := func() error {
						if v2c, ok := s.v2Client(); ok && canUseV2(s.RT.Cfg.CustomerID) {
							_, _, err := doV2ThenV1(
								true,
								func() (struct{}, error) {
									return struct{}{}, v2c.SetNameserversV2(ctx, s.RT.Cfg.CustomerID, d, custom.NameServers)
								},
								func() (struct{}, error) {
									return struct{}{}, s.Client.SetNameservers(ctx, d, custom.NameServers)
								},
							)
							return err
						}
						return s.Client.SetNameservers(ctx, d, custom.NameServers)
					}
					if err := setCustomNS(); err != nil {
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
