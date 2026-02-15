package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sportwhiz/gdcli/internal/config"
)

const CacheFile = "update_check.json"

type Cache struct {
	LastCheckedAt   time.Time `json:"last_checked_at"`
	CurrentVersion  string    `json:"current_version"`
	LatestVersion   string    `json:"latest_version,omitempty"`
	UpdateAvailable *bool     `json:"update_available,omitempty"`
	ReleaseURL      string    `json:"release_url,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
}

func LoadCache() (*Cache, error) {
	path, err := cachePath()
	if err != nil {
		return nil, err
	}
	path = filepath.Clean(path)
	// #nosec G304 -- path is scoped to ~/.gdcli with fixed filename.
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var c Cache
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func SaveCache(c *Cache) error {
	path, err := cachePath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func ShouldCheck(now, lastChecked time.Time, interval time.Duration) bool {
	if interval <= 0 {
		return true
	}
	if lastChecked.IsZero() {
		return true
	}
	return now.Sub(lastChecked) >= interval
}

func IsDisabledByEnv() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("GDCLI_DISABLE_UPDATE_CHECK")))
	return v == "1" || v == "true" || v == "yes"
}

func cachePath() (string, error) {
	dir, err := config.EnsureDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, CacheFile), nil
}
