package budget

import (
	"time"

	"github.com/sportwhiz/gdcli/internal/config"
	apperr "github.com/sportwhiz/gdcli/internal/errors"
	"github.com/sportwhiz/gdcli/internal/store"
)

func CheckPrice(cfg *config.Config, price float64, currency string) error {
	if currency != "USD" {
		return &apperr.AppError{Code: apperr.CodeValidation, Message: "only USD prices are supported in v1", Details: map[string]any{"currency": currency}}
	}
	if price > cfg.MaxPricePerDomain {
		return &apperr.AppError{Code: apperr.CodeBudget, Message: "price exceeds max_price_per_domain", Details: map[string]any{"price": price, "max_price_per_domain": cfg.MaxPricePerDomain}}
	}
	return nil
}

func CheckDailyCaps(cfg *config.Config, now time.Time, candidatePrice float64) error {
	ops, err := store.ReadOperations()
	if err != nil {
		return err
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	totalSpend := 0.0
	totalDomains := 0
	for _, op := range ops {
		if op.CreatedAt.Before(dayStart) || !op.CreatedAt.Before(dayEnd) {
			continue
		}
		if op.Status != "succeeded" {
			continue
		}
		if op.Type != "purchase" && op.Type != "renew" {
			continue
		}
		totalSpend += op.Amount
		totalDomains++
	}

	if totalSpend+candidatePrice > cfg.MaxDailySpend {
		return &apperr.AppError{Code: apperr.CodeBudget, Message: "daily spend cap exceeded", Details: map[string]any{"attempted_total": totalSpend + candidatePrice, "max_daily_spend": cfg.MaxDailySpend}}
	}
	if totalDomains+1 > cfg.MaxDomainsPerDay {
		return &apperr.AppError{Code: apperr.CodeBudget, Message: "daily domain count cap exceeded", Details: map[string]any{"attempted_total": totalDomains + 1, "max_domains_per_day": cfg.MaxDomainsPerDay}}
	}
	return nil
}
