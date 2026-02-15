package godaddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	apperr "github.com/sportwhiz/gdcli/internal/errors"
)

type Client interface {
	Suggest(ctx context.Context, query string, tlds []string, limit int) ([]Suggestion, error)
	Available(ctx context.Context, domain string) (Availability, error)
	AvailableBulk(ctx context.Context, domains []string) ([]Availability, error)
	Purchase(ctx context.Context, domain string, years int, idempotencyKey string) (PurchaseResult, error)
	Renew(ctx context.Context, domain string, years int, idempotencyKey string) (RenewResult, error)
	ListDomains(ctx context.Context) ([]PortfolioDomain, error)
	GetNameservers(ctx context.Context, domain string) ([]string, error)
	GetRecords(ctx context.Context, domain string) ([]DNSRecord, error)
	SetNameservers(ctx context.Context, domain string, nameservers []string) error
	SetRecords(ctx context.Context, domain string, records []DNSRecord) error
}

type HTTPClient struct {
	BaseURL    string
	APIKey     string
	APISecret  string
	HTTPClient *http.Client
}

type Suggestion struct {
	Domain string  `json:"domain"`
	Score  float64 `json:"score"`
}

type Availability struct {
	Domain    string  `json:"domain"`
	Available bool    `json:"available"`
	Price     float64 `json:"price,omitempty"`
	Currency  string  `json:"currency,omitempty"`
}

type PurchaseResult struct {
	Domain        string  `json:"domain"`
	Price         float64 `json:"price"`
	Currency      string  `json:"currency"`
	OrderID       string  `json:"order_id,omitempty"`
	AlreadyBought bool    `json:"already_bought,omitempty"`
}

type RenewResult struct {
	Domain   string  `json:"domain"`
	Price    float64 `json:"price"`
	Currency string  `json:"currency"`
	OrderID  string  `json:"order_id,omitempty"`
}

type PortfolioDomain struct {
	Domain  string `json:"domain"`
	Expires string `json:"expires"`
}

type DNSRecord struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Data string `json:"data"`
	TTL  int    `json:"ttl,omitempty"`
}

func NewHTTPClient(baseURL, key, secret string) *HTTPClient {
	return &HTTPClient{
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		APIKey:     key,
		APISecret:  secret,
		HTTPClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *HTTPClient) Suggest(ctx context.Context, query string, tlds []string, limit int) ([]Suggestion, error) {
	q := url.Values{}
	q.Set("query", query)
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if len(tlds) > 0 {
		q.Set("tlds", strings.Join(tlds, ","))
	}
	var out []Suggestion
	if err := c.do(ctx, http.MethodGet, "/v1/domains/suggest?"+q.Encode(), nil, &out, ""); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *HTTPClient) Available(ctx context.Context, domain string) (Availability, error) {
	q := url.Values{}
	q.Set("domain", domain)
	q.Set("checkType", "FAST")
	var out Availability
	if err := c.do(ctx, http.MethodGet, "/v1/domains/available?"+q.Encode(), nil, &out, ""); err != nil {
		return Availability{}, err
	}
	return out, nil
}

func (c *HTTPClient) AvailableBulk(ctx context.Context, domains []string) ([]Availability, error) {
	body := map[string]any{"domains": domains, "checkType": "FAST"}
	var out []Availability
	if err := c.do(ctx, http.MethodPost, "/v1/domains/available", body, &out, ""); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *HTTPClient) Purchase(ctx context.Context, domain string, years int, idempotencyKey string) (PurchaseResult, error) {
	body := map[string]any{"domain": domain, "period": years}
	var out PurchaseResult
	if err := c.do(ctx, http.MethodPost, "/v1/domains/purchase", body, &out, idempotencyKey); err != nil {
		return PurchaseResult{}, err
	}
	return out, nil
}

func (c *HTTPClient) Renew(ctx context.Context, domain string, years int, idempotencyKey string) (RenewResult, error) {
	body := map[string]any{"period": years}
	var out RenewResult
	if err := c.do(ctx, http.MethodPost, "/v1/domains/"+url.PathEscape(domain)+"/renew", body, &out, idempotencyKey); err != nil {
		return RenewResult{}, err
	}
	return out, nil
}

func (c *HTTPClient) ListDomains(ctx context.Context) ([]PortfolioDomain, error) {
	var out []PortfolioDomain
	if err := c.do(ctx, http.MethodGet, "/v1/domains", nil, &out, ""); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *HTTPClient) GetNameservers(ctx context.Context, domain string) ([]string, error) {
	var out struct {
		NameServers []string `json:"nameServers"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/domains/"+url.PathEscape(domain), nil, &out, ""); err != nil {
		return nil, err
	}
	return out.NameServers, nil
}

func (c *HTTPClient) GetRecords(ctx context.Context, domain string) ([]DNSRecord, error) {
	var out []DNSRecord
	if err := c.do(ctx, http.MethodGet, "/v1/domains/"+url.PathEscape(domain)+"/records", nil, &out, ""); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *HTTPClient) SetNameservers(ctx context.Context, domain string, nameservers []string) error {
	body := map[string]any{"nameServers": nameservers}
	return c.do(ctx, http.MethodPatch, "/v1/domains/"+url.PathEscape(domain), body, nil, "")
}

func (c *HTTPClient) SetRecords(ctx context.Context, domain string, records []DNSRecord) error {
	return c.do(ctx, http.MethodPut, "/v1/domains/"+url.PathEscape(domain)+"/records", records, nil, "")
}

func (c *HTTPClient) do(ctx context.Context, method, path string, body any, out any, idempotencyKey string) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "sso-key "+c.APIKey+":"+c.APISecret)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("X-Idempotency-Key", idempotencyKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return &apperr.AppError{Code: apperr.CodeProvider, Message: "provider request failed", Retryable: true, Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil {
			return nil
		}
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil && err != io.EOF {
			return &apperr.AppError{Code: apperr.CodeProvider, Message: "failed decoding provider response", Cause: err}
		}
		return nil
	}

	var raw map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&raw)
	if resp.StatusCode == 429 {
		return &apperr.AppError{Code: apperr.CodeRateLimited, Message: "provider rate limited", Retryable: true, Details: raw}
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return &apperr.AppError{Code: apperr.CodeAuth, Message: "provider authentication failed", Details: raw}
	}
	return &apperr.AppError{Code: apperr.CodeProvider, Message: "provider returned non-success status", Details: map[string]any{"status": resp.StatusCode, "provider": raw}}
}
