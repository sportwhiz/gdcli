package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type suggestion struct {
	Domain string  `json:"domain"`
	Score  float64 `json:"score"`
}

type availability struct {
	Domain    string  `json:"domain"`
	Available bool    `json:"available"`
	Price     float64 `json:"price"`
	Currency  string  `json:"currency"`
}

type purchaseResult struct {
	Domain   string  `json:"domain"`
	Price    float64 `json:"price"`
	Currency string  `json:"currency"`
	OrderID  string  `json:"order_id"`
}

type renewResult struct {
	Domain   string  `json:"domain"`
	Price    float64 `json:"price"`
	Currency string  `json:"currency"`
	OrderID  string  `json:"order_id"`
}

type portfolioDomain struct {
	Domain  string `json:"domain"`
	Expires string `json:"expires"`
}

type dnsRecord struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Data string `json:"data"`
	TTL  int    `json:"ttl,omitempty"`
}

type state struct {
	mu           sync.Mutex
	portfolio    []portfolioDomain
	availability map[string]availability
	nameservers  map[string][]string
	records      map[string][]dnsRecord
	orderCounter int
}

func main() {
	s := &state{
		portfolio: []portfolioDomain{
			{Domain: "alpha.com", Expires: "2026-12-31"},
			{Domain: "brand.ai", Expires: "2026-03-20"},
		},
		availability: map[string]availability{
			"example.com": {Domain: "example.com", Available: true, Price: 12.99, Currency: "USD"},
			"taken.com":   {Domain: "taken.com", Available: false, Price: 0, Currency: "USD"},
		},
		nameservers: map[string][]string{
			"alpha.com": {"ns1.notafternic.com", "ns2.notafternic.com"},
			"brand.ai":  {"ns1.afternic.com", "ns2.afternic.com"},
		},
		records: map[string][]dnsRecord{
			"alpha.com": {{Type: "A", Name: "@", Data: "1.2.3.4", TTL: 600}},
			"brand.ai":  {{Type: "A", Name: "@", Data: "5.6.7.8", TTL: 600}, {Type: "TXT", Name: "@", Data: "verify=ok", TTL: 600}},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/domains/suggest", s.handleSuggest)
	mux.HandleFunc("/v1/domains/available", s.handleAvailable)
	mux.HandleFunc("/v1/domains/purchase", s.handlePurchase)
	mux.HandleFunc("/v1/domains", s.handleDomains)
	mux.HandleFunc("/v1/domains/", s.handleDomainSub)

	addr := ":8787"
	log.Printf("mock godaddy listening on %s", addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func (s *state) handleSuggest(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "query required"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 5
	}
	out := make([]suggestion, 0, limit)
	for i := 0; i < limit; i++ {
		sfx := ".com"
		if i%2 == 1 {
			sfx = ".ai"
		}
		out = append(out, suggestion{Domain: strings.ReplaceAll(strings.ToLower(query), " ", "") + strconv.Itoa(i+1) + sfx, Score: 0.95 - float64(i)*0.03})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *state) handleAvailable(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch r.Method {
	case http.MethodGet:
		domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
		if domain == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": "domain required"})
			return
		}
		if a, ok := s.availability[domain]; ok {
			writeJSON(w, http.StatusOK, a)
			return
		}
		writeJSON(w, http.StatusOK, availability{Domain: domain, Available: true, Price: 12.99, Currency: "USD"})
	case http.MethodPost:
		var req struct {
			Domains []string `json:"domains"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid json"})
			return
		}
		out := make([]availability, 0, len(req.Domains))
		for _, d := range req.Domains {
			d = strings.ToLower(strings.TrimSpace(d))
			if a, ok := s.availability[d]; ok {
				out = append(out, a)
				continue
			}
			out = append(out, availability{Domain: d, Available: true, Price: 12.99, Currency: "USD"})
		}
		writeJSON(w, http.StatusOK, out)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "method not allowed"})
	}
}

func (s *state) handlePurchase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "method not allowed"})
		return
	}
	var req struct {
		Domain string `json:"domain"`
		Period int    `json:"period"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid json"})
		return
	}
	if req.Period <= 0 {
		req.Period = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d := strings.ToLower(strings.TrimSpace(req.Domain))
	if a, ok := s.availability[d]; ok && !a.Available {
		writeJSON(w, http.StatusConflict, map[string]any{"message": "domain not available"})
		return
	}
	s.orderCounter++
	writeJSON(w, http.StatusOK, purchaseResult{Domain: d, Price: 12.99 * float64(req.Period), Currency: "USD", OrderID: "mock-order-" + strconv.Itoa(s.orderCounter)})
}

func (s *state) handleDomains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "method not allowed"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, s.portfolio)
}

func (s *state) handleDomainSub(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/domains/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "not found"})
		return
	}
	domain := strings.ToLower(strings.TrimSpace(parts[0]))
	if domain == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "domain required"})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			ns := s.nameservers[domain]
			if len(ns) == 0 {
				ns = []string{"ns1.notafternic.com", "ns2.notafternic.com"}
			}
			writeJSON(w, http.StatusOK, map[string]any{"nameServers": ns})
		case http.MethodPatch:
			var req struct {
				NameServers []string `json:"nameServers"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid json"})
				return
			}
			s.nameservers[domain] = req.NameServers
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "method not allowed"})
		}
		return
	}

	if len(parts) == 2 && parts[1] == "renew" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "method not allowed"})
			return
		}
		s.orderCounter++
		writeJSON(w, http.StatusOK, renewResult{Domain: domain, Price: 12.99, Currency: "USD", OrderID: "mock-renew-" + strconv.Itoa(s.orderCounter)})
		return
	}

	if len(parts) == 2 && parts[1] == "records" {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.records[domain])
		case http.MethodPut:
			var req []dnsRecord
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid json"})
				return
			}
			s.records[domain] = req
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "method not allowed"})
		}
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]any{"message": "not found"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
