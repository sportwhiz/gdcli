package godaddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
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
	ListOrders(ctx context.Context, limit, offset int) (OrdersPage, error)
	ListSubscriptions(ctx context.Context, limit, offset int) (SubscriptionsPage, error)
	GetNameservers(ctx context.Context, domain string) ([]string, error)
	GetRecords(ctx context.Context, domain string) ([]DNSRecord, error)
	SetNameservers(ctx context.Context, domain string, nameservers []string) error
	SetRecords(ctx context.Context, domain string, records []DNSRecord) error
}

type HTTPClient struct {
	baseURL    string
	apiKey     string
	apiSecret  string
	httpClient *http.Client
}

const (
	smallResponseLimitBytes = int64(2 << 20)
	bulkResponseLimitBytes  = int64(50 << 20)
	errorResponseLimitBytes = int64(1 << 20)
)

type V2DomainAction struct {
	ActionID   string `json:"actionId,omitempty"`
	Type       string `json:"type,omitempty"`
	Status     string `json:"status,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
}

type V2NotificationsOptIn struct {
	NotificationTypes []string `json:"notificationTypes,omitempty"`
}

type Suggestion struct {
	Domain string  `json:"domain"`
	Score  float64 `json:"score"`
}

type Availability struct {
	Domain     string  `json:"domain"`
	Available  bool    `json:"available"`
	Definitive bool    `json:"definitive,omitempty"`
	Price      float64 `json:"price,omitempty"`
	Currency   string  `json:"currency,omitempty"`
	PriceRaw   float64 `json:"price_raw,omitempty"`
	PriceUnit  string  `json:"price_unit,omitempty"`
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

type RenewV2Consent struct {
	Price                  int64  `json:"price"`
	Currency               string `json:"currency"`
	AgreedBy               string `json:"agreedBy"`
	AgreedAt               string `json:"agreedAt"`
	RegistryPremiumPricing bool   `json:"registryPremiumPricing,omitempty"`
}

type RenewV2Request struct {
	Expires string         `json:"expires"`
	Consent RenewV2Consent `json:"consent"`
	Period  int            `json:"period,omitempty"`
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

type Pagination struct {
	First  string `json:"first,omitempty"`
	Last   string `json:"last,omitempty"`
	Next   string `json:"next,omitempty"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type OrderItem struct {
	Label string `json:"label"`
}

type OrderPricing struct {
	Total     float64 `json:"total"`
	TotalRaw  float64 `json:"total_raw,omitempty"`
	TotalUnit string  `json:"total_unit,omitempty"`
}

type Order struct {
	OrderID   string       `json:"order_id"`
	CreatedAt string       `json:"created_at,omitempty"`
	Currency  string       `json:"currency,omitempty"`
	Items     []OrderItem  `json:"items,omitempty"`
	Pricing   OrderPricing `json:"pricing"`
}

type OrdersPage struct {
	Orders     []Order    `json:"orders"`
	Pagination Pagination `json:"pagination"`
}

type SubscriptionProduct struct {
	Namespace       string `json:"namespace,omitempty"`
	ProductGroupKey string `json:"product_group_key,omitempty"`
}

type SubscriptionBilling struct {
	Status  string `json:"status,omitempty"`
	RenewAt string `json:"renew_at,omitempty"`
}

type Subscription struct {
	SubscriptionID string              `json:"subscription_id"`
	Status         string              `json:"status,omitempty"`
	Label          string              `json:"label,omitempty"`
	CreatedAt      string              `json:"created_at,omitempty"`
	ExpiresAt      string              `json:"expires_at,omitempty"`
	Renewable      bool                `json:"renewable"`
	RenewAuto      bool                `json:"renew_auto"`
	Product        SubscriptionProduct `json:"product"`
	Billing        SubscriptionBilling `json:"billing"`
}

type SubscriptionsPage struct {
	Subscriptions []Subscription `json:"subscriptions"`
	Pagination    Pagination     `json:"pagination"`
}

func NewHTTPClient(baseURL, key, secret string) (*HTTPClient, error) {
	if err := validateBaseURL(baseURL); err != nil {
		return nil, err
	}
	return &HTTPClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     key,
		apiSecret:  secret,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}, nil
}

func validateBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return &apperr.AppError{Code: apperr.CodeValidation, Message: "invalid base URL"}
	}
	host := strings.ToLower(u.Hostname())
	allowedHosts := map[string]bool{
		"api.godaddy.com":     true,
		"api.ote-godaddy.com": true,
		"localhost":           true,
		"127.0.0.1":           true,
		"::1":                 true,
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
		return &apperr.AppError{Code: apperr.CodeValidation, Message: "base URL must target GoDaddy APIs or loopback"}
	}
	if !allowedHosts[host] {
		return &apperr.AppError{Code: apperr.CodeValidation, Message: "base URL host is not allowed"}
	}
	if host == "api.godaddy.com" || host == "api.ote-godaddy.com" {
		if !strings.EqualFold(u.Scheme, "https") {
			return &apperr.AppError{Code: apperr.CodeValidation, Message: "GoDaddy API base URL must use https"}
		}
	}
	return nil
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
	// FULL provides a definitive answer for single lookups and avoids FAST-mode ambiguity.
	q.Set("checkType", "FULL")
	var raw availabilityAPI
	if err := c.do(ctx, http.MethodGet, "/v1/domains/available?"+q.Encode(), nil, &raw, ""); err != nil {
		return Availability{}, err
	}
	return normalizeAvailability(raw), nil
}

func (c *HTTPClient) AvailableBulk(ctx context.Context, domains []string) ([]Availability, error) {
	body := map[string]any{"domains": domains, "checkType": "FAST"}
	var raw []availabilityAPI
	if err := c.do(ctx, http.MethodPost, "/v1/domains/available", body, &raw, ""); err != nil {
		return nil, err
	}
	out := make([]Availability, 0, len(raw))
	for _, item := range raw {
		out = append(out, normalizeAvailability(item))
	}
	return out, nil
}

type availabilityAPI struct {
	Domain     string      `json:"domain"`
	Available  bool        `json:"available"`
	Definitive bool        `json:"definitive,omitempty"`
	Price      interface{} `json:"price,omitempty"`
	Currency   string      `json:"currency,omitempty"`
}

func normalizeAvailability(in availabilityAPI) Availability {
	out := Availability{
		Domain:     in.Domain,
		Available:  in.Available,
		Definitive: in.Definitive,
		Currency:   in.Currency,
	}
	price, raw, unit := normalizeProviderPrice(in.Price)
	out.Price = price
	out.PriceRaw = raw
	out.PriceUnit = unit
	return out
}

// GoDaddy availability pricing is commonly reported in micro-units.
// We normalize to USD in `Price` and preserve provider value/unit for auditing.
func normalizeProviderPrice(v interface{}) (price float64, raw float64, unit string) {
	const micros = 1_000_000.0
	switch x := v.(type) {
	case nil:
		return 0, 0, ""
	case float64:
		raw = x
		if isWholeNumber(x) && x >= micros {
			return x / micros, x, "micros"
		}
		return x, x, "usd"
	case float32:
		f := float64(x)
		raw = f
		if isWholeNumber(f) && f >= micros {
			return f / micros, f, "micros"
		}
		return f, f, "usd"
	case int:
		f := float64(x)
		if f >= micros {
			return f / micros, f, "micros"
		}
		return f, f, "usd"
	case int64:
		f := float64(x)
		if f >= micros {
			return f / micros, f, "micros"
		}
		return f, f, "usd"
	case json.Number:
		if i, err := x.Int64(); err == nil {
			f := float64(i)
			if f >= micros {
				return f / micros, f, "micros"
			}
			return f, f, "usd"
		}
		if f, err := x.Float64(); err == nil {
			if isWholeNumber(f) && f >= micros {
				return f / micros, f, "micros"
			}
			return f, f, "usd"
		}
	case string:
		if s := strings.TrimSpace(x); s != "" {
			if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				f := float64(i)
				if f >= micros {
					return f / micros, f, "micros"
				}
				return f, f, "usd"
			}
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				if isWholeNumber(f) && f >= micros {
					return f / micros, f, "micros"
				}
				return f, f, "usd"
			}
		}
	}
	return 0, 0, ""
}

func isWholeNumber(v float64) bool {
	return math.Abs(v-math.Round(v)) < 1e-9
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

func (c *HTTPClient) ListOrders(ctx context.Context, limit, offset int) (OrdersPage, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	var raw struct {
		Orders []struct {
			OrderID   string `json:"orderId"`
			CreatedAt string `json:"createdAt"`
			Currency  string `json:"currency"`
			Items     []struct {
				Label string `json:"label"`
			} `json:"items"`
			Pricing struct {
				Total interface{} `json:"total"`
			} `json:"pricing"`
		} `json:"orders"`
		Pagination struct {
			First string `json:"first"`
			Last  string `json:"last"`
			Next  string `json:"next"`
			Total int    `json:"total"`
		} `json:"pagination"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/orders?"+q.Encode(), nil, &raw, ""); err != nil {
		return OrdersPage{}, err
	}
	out := OrdersPage{
		Orders: make([]Order, 0, len(raw.Orders)),
		Pagination: Pagination{
			First:  raw.Pagination.First,
			Last:   raw.Pagination.Last,
			Next:   raw.Pagination.Next,
			Total:  raw.Pagination.Total,
			Limit:  limit,
			Offset: offset,
		},
	}
	for _, o := range raw.Orders {
		price, rawPrice, unit := normalizeProviderPrice(o.Pricing.Total)
		items := make([]OrderItem, 0, len(o.Items))
		for _, item := range o.Items {
			items = append(items, OrderItem{Label: item.Label})
		}
		out.Orders = append(out.Orders, Order{
			OrderID:   o.OrderID,
			CreatedAt: o.CreatedAt,
			Currency:  o.Currency,
			Items:     items,
			Pricing: OrderPricing{
				Total:     price,
				TotalRaw:  rawPrice,
				TotalUnit: unit,
			},
		})
	}
	return out, nil
}

func (c *HTTPClient) ListSubscriptions(ctx context.Context, limit, offset int) (SubscriptionsPage, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	var raw struct {
		Subscriptions []struct {
			SubscriptionID string `json:"subscriptionId"`
			Status         string `json:"status"`
			Label          string `json:"label"`
			CreatedAt      string `json:"createdAt"`
			ExpiresAt      string `json:"expiresAt"`
			Renewable      bool   `json:"renewable"`
			RenewAuto      bool   `json:"renewAuto"`
			Product        struct {
				Namespace       string `json:"namespace"`
				ProductGroupKey string `json:"productGroupKey"`
			} `json:"product"`
			Billing struct {
				Status  string `json:"status"`
				RenewAt string `json:"renewAt"`
			} `json:"billing"`
		} `json:"subscriptions"`
		Pagination struct {
			First string `json:"first"`
			Last  string `json:"last"`
			Next  string `json:"next"`
			Total int    `json:"total"`
		} `json:"pagination"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/subscriptions?"+q.Encode(), nil, &raw, ""); err != nil {
		return SubscriptionsPage{}, err
	}
	out := SubscriptionsPage{
		Subscriptions: make([]Subscription, 0, len(raw.Subscriptions)),
		Pagination: Pagination{
			First:  raw.Pagination.First,
			Last:   raw.Pagination.Last,
			Next:   raw.Pagination.Next,
			Total:  raw.Pagination.Total,
			Limit:  limit,
			Offset: offset,
		},
	}
	for _, s := range raw.Subscriptions {
		out.Subscriptions = append(out.Subscriptions, Subscription{
			SubscriptionID: s.SubscriptionID,
			Status:         s.Status,
			Label:          s.Label,
			CreatedAt:      s.CreatedAt,
			ExpiresAt:      s.ExpiresAt,
			Renewable:      s.Renewable,
			RenewAuto:      s.RenewAuto,
			Product: SubscriptionProduct{
				Namespace:       s.Product.Namespace,
				ProductGroupKey: s.Product.ProductGroupKey,
			},
			Billing: SubscriptionBilling{
				Status:  s.Billing.Status,
				RenewAt: s.Billing.RenewAt,
			},
		})
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

func (c *HTTPClient) ResolveCustomerID(ctx context.Context, shopperID string) (string, error) {
	if strings.TrimSpace(shopperID) == "" {
		return "", &apperr.AppError{Code: apperr.CodeValidation, Message: "shopper_id is required"}
	}
	var out struct {
		CustomerID string `json:"customerId"`
	}
	q := url.Values{}
	q.Set("includes", "customerId")
	if err := c.do(ctx, http.MethodGet, "/v1/shoppers/"+url.PathEscape(shopperID)+"?"+q.Encode(), nil, &out, ""); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.CustomerID) == "" {
		return "", &apperr.AppError{Code: apperr.CodeProvider, Message: "customerId not present in shopper response"}
	}
	return out.CustomerID, nil
}

func (c *HTTPClient) V2Get(ctx context.Context, path string, query url.Values, out any) error {
	p := path
	if query != nil && len(query) > 0 {
		sep := "?"
		if strings.Contains(p, "?") {
			sep = "&"
		}
		p = p + sep + query.Encode()
	}
	return c.do(ctx, http.MethodGet, p, nil, out, "")
}

func (c *HTTPClient) V2Post(ctx context.Context, path string, body any, out any, idempotencyKey string) error {
	return c.do(ctx, http.MethodPost, path, body, out, idempotencyKey)
}

func (c *HTTPClient) V2Put(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodPut, path, body, out, "")
}

func (c *HTTPClient) V2Patch(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodPatch, path, body, out, "")
}

func (c *HTTPClient) DomainDetailV2(ctx context.Context, customerID, domain string, includes []string) (map[string]any, error) {
	q := url.Values{}
	for _, include := range includes {
		if strings.TrimSpace(include) != "" {
			q.Add("includes", include)
		}
	}
	path := "/v2/customers/" + url.PathEscape(customerID) + "/domains/" + url.PathEscape(domain)
	var out map[string]any
	if err := c.V2Get(ctx, path, q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *HTTPClient) DomainDetailV1(ctx context.Context, domain string) (map[string]any, error) {
	var out map[string]any
	if err := c.do(ctx, http.MethodGet, "/v1/domains/"+url.PathEscape(domain), nil, &out, ""); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *HTTPClient) RenewV2(ctx context.Context, customerID, domain string, req RenewV2Request, idempotencyKey string) (RenewResult, error) {
	path := "/v2/customers/" + url.PathEscape(customerID) + "/domains/" + url.PathEscape(domain) + "/renew"
	body := map[string]any{
		"expires": req.Expires,
		"consent": req.Consent,
	}
	if req.Period > 0 {
		body["period"] = req.Period
	}
	var out struct {
		Price    interface{} `json:"price"`
		Currency string      `json:"currency"`
		OrderID  string      `json:"orderId"`
	}
	if err := c.V2Post(ctx, path, body, &out, idempotencyKey); err != nil {
		return RenewResult{}, err
	}
	price, _, _ := normalizeProviderPrice(out.Price)
	return RenewResult{
		Domain:   domain,
		Price:    price,
		Currency: out.Currency,
		OrderID:  out.OrderID,
	}, nil
}

func (c *HTTPClient) SetNameserversV2(ctx context.Context, customerID, domain string, nameservers []string) error {
	path := "/v2/customers/" + url.PathEscape(customerID) + "/domains/" + url.PathEscape(domain) + "/nameServers"
	body := map[string]any{"nameServers": nameservers}
	return c.V2Put(ctx, path, body, nil)
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
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "sso-key "+c.apiKey+":"+c.apiSecret)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("X-Idempotency-Key", idempotencyKey)
	}

	// #nosec G704 -- base URL is validated to approved GoDaddy/loopback hosts in validateBaseURL.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &apperr.AppError{Code: apperr.CodeProvider, Message: "provider request failed", Retryable: true, Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil {
			return nil
		}
		limited := io.LimitReader(resp.Body, responseLimitFor(method, path))
		if err := json.NewDecoder(limited).Decode(out); err != nil && err != io.EOF {
			return &apperr.AppError{Code: apperr.CodeProvider, Message: "failed decoding provider response", Cause: err}
		}
		return nil
	}

	var raw map[string]any
	_ = json.NewDecoder(io.LimitReader(resp.Body, errorResponseLimitBytes)).Decode(&raw)
	if resp.StatusCode == 429 {
		return &apperr.AppError{Code: apperr.CodeRateLimited, Message: "provider rate limited", Retryable: true, Details: raw}
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return &apperr.AppError{Code: apperr.CodeAuth, Message: "provider authentication failed", Details: raw}
	}
	return &apperr.AppError{Code: apperr.CodeProvider, Message: "provider returned non-success status", Details: map[string]any{"status": resp.StatusCode, "provider": raw}}
}

func responseLimitFor(method, path string) int64 {
	cleanPath := path
	if idx := strings.Index(cleanPath, "?"); idx >= 0 {
		cleanPath = cleanPath[:idx]
	}
	switch {
	case method == http.MethodPost && cleanPath == "/v1/domains/available":
		return bulkResponseLimitBytes
	case method == http.MethodGet && cleanPath == "/v1/orders":
		return bulkResponseLimitBytes
	case method == http.MethodGet && cleanPath == "/v1/subscriptions":
		return bulkResponseLimitBytes
	case method == http.MethodGet && cleanPath == "/v1/domains":
		return bulkResponseLimitBytes
	default:
		return smallResponseLimitBytes
	}
}
