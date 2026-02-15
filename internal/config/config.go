package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DirName    = ".gdcli"
	ConfigName = "config.json"
)

type Config struct {
	APIEnvironment      string  `json:"api_environment"`
	ShopperID           string  `json:"shopper_id,omitempty"`
	CustomerID          string  `json:"customer_id,omitempty"`
	CustomerIDResolved  string  `json:"customer_id_resolved_at,omitempty"`
	CustomerIDSource    string  `json:"customer_id_source,omitempty"`
	AutoPurchaseEnabled bool    `json:"auto_purchase_enabled"`
	AcknowledgmentHash  string  `json:"acknowledgment_hash,omitempty"`
	MaxPricePerDomain   float64 `json:"max_price_per_domain"`
	MaxDailySpend       float64 `json:"max_daily_spend"`
	MaxDomainsPerDay    int     `json:"max_domains_per_day"`
	DefaultYears        int     `json:"default_years"`
	DefaultDNSTemplate  string  `json:"default_dns_template"`
	OutputDefault       string  `json:"output_default"`
}

func Default() *Config {
	return &Config{
		APIEnvironment:      "prod",
		AutoPurchaseEnabled: false,
		MaxPricePerDomain:   25,
		MaxDailySpend:       100,
		MaxDomainsPerDay:    5,
		DefaultYears:        1,
		DefaultDNSTemplate:  "afternic-nameservers",
		OutputDefault:       "json",
	}
}

func HomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, DirName), nil
}

func Path() (string, error) {
	home, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ConfigName), nil
}

func EnsureDir() (string, error) {
	dir, err := HomeDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	path = filepath.Clean(path)
	// #nosec G304 -- path is derived from user home + fixed filename.
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Default()
			if saveErr := Save(cfg); saveErr != nil {
				return nil, fmt.Errorf("initialize config: %w", saveErr)
			}
			return cfg, nil
		}
		return nil, err
	}
	cfg := Default()
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Save(cfg *Config) error {
	if _, err := EnsureDir(); err != nil {
		return err
	}
	path, err := Path()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}
