package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sportwhiz/gdcli/internal/config"
)

const (
	OperationsFile = "operations.jsonl"
	TokensFile     = "confirm_tokens.json"
)

type Operation struct {
	OperationID string    `json:"operation_id"`
	Type        string    `json:"type"`
	Domain      string    `json:"domain"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"`
}

type ConfirmToken struct {
	TokenID      string    `json:"token_id"`
	Domain       string    `json:"domain"`
	QuotedPrice  float64   `json:"quoted_price"`
	Currency     string    `json:"currency"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	Used         bool      `json:"used"`
	OperationKey string    `json:"operation_key"`
}

type TokenStore struct {
	Tokens []ConfirmToken `json:"tokens"`
}

func operationsPath() (string, error) {
	d, err := config.EnsureDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, OperationsFile), nil
}

func tokensPath() (string, error) {
	d, err := config.EnsureDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, TokensFile), nil
}

func AppendOperation(op Operation) error {
	path, err := operationsPath()
	if err != nil {
		return err
	}
	path = filepath.Clean(path)
	// #nosec G304 -- path is scoped to ~/.gdcli with fixed filename.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(op)
}

func ReadOperations() ([]Operation, error) {
	path, err := operationsPath()
	if err != nil {
		return nil, err
	}
	path = filepath.Clean(path)
	// #nosec G304 -- path is scoped to ~/.gdcli with fixed filename.
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var ops []Operation
	s := bufio.NewScanner(f)
	for s.Scan() {
		var op Operation
		if err := json.Unmarshal(s.Bytes(), &op); err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return ops, nil
}

func LoadTokens() (*TokenStore, error) {
	path, err := tokensPath()
	if err != nil {
		return nil, err
	}
	path = filepath.Clean(path)
	// #nosec G304 -- path is scoped to ~/.gdcli with fixed filename.
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &TokenStore{}, nil
		}
		return nil, err
	}
	var ts TokenStore
	if err := json.Unmarshal(b, &ts); err != nil {
		return nil, err
	}
	return &ts, nil
}

func SaveTokens(ts *TokenStore) error {
	path, err := tokensPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func LoadAndSaveTokens(mutator func(*TokenStore) error) error {
	path, err := tokensPath()
	if err != nil {
		return err
	}
	path = filepath.Clean(path)
	// #nosec G304 -- path is scoped to ~/.gdcli with fixed filename.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return err
	}
	defer func() { _ = unlockFile(f) }()

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	ts := &TokenStore{}
	if len(b) > 0 {
		if err := json.Unmarshal(b, ts); err != nil {
			return err
		}
	}
	if err := mutator(ts); err != nil {
		return err
	}
	out, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, err := f.Write(out); err != nil {
		return err
	}
	return f.Sync()
}
