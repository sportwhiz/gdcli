package safety

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	apperr "github.com/sportwhiz/gdcli/internal/errors"
	"github.com/sportwhiz/gdcli/internal/store"
)

const (
	AckPhrase = "I UNDERSTAND PURCHASES ARE FINAL"
	TokenTTL  = 10 * time.Minute
)

func HashAcknowledgment(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func EnableAutoPurchase(ack string) (string, error) {
	if ack != AckPhrase {
		return "", &apperr.AppError{
			Code:    apperr.CodeSafety,
			Message: "invalid acknowledgment phrase",
			Details: map[string]any{"required": AckPhrase},
		}
	}
	return HashAcknowledgment(ack), nil
}

func IssueToken(domain string, price float64, currency, operationKey string, now time.Time) (store.ConfirmToken, error) {
	raw := sha256.Sum256([]byte(domain + "|" + operationKey + "|" + now.UTC().Format(time.RFC3339Nano)))
	tokenID := hex.EncodeToString(raw[:16])
	var issued store.ConfirmToken
	err := store.LoadAndSaveTokens(func(ts *store.TokenStore) error {
		pruneTokens(ts, now)
		t := store.ConfirmToken{
			TokenID:      tokenID,
			Domain:       domain,
			QuotedPrice:  price,
			Currency:     currency,
			IssuedAt:     now.UTC(),
			ExpiresAt:    now.UTC().Add(TokenTTL),
			Used:         false,
			OperationKey: operationKey,
		}
		ts.Tokens = append(ts.Tokens, t)
		issued = t
		return nil
	})
	if err != nil {
		return store.ConfirmToken{}, err
	}
	return issued, nil
}

func pruneTokens(ts *store.TokenStore, now time.Time) {
	cutoff := now.UTC()
	kept := ts.Tokens[:0]
	for _, tok := range ts.Tokens {
		if tok.Used {
			continue
		}
		if cutoff.After(tok.ExpiresAt) {
			continue
		}
		kept = append(kept, tok)
	}
	ts.Tokens = kept
}

func ValidateAndUseToken(tokenID, domain string, now time.Time) (store.ConfirmToken, error) {
	var used store.ConfirmToken
	var found bool
	err := store.LoadAndSaveTokens(func(ts *store.TokenStore) error {
		pruneTokens(ts, now)
		for i := range ts.Tokens {
			t := &ts.Tokens[i]
			if t.TokenID != tokenID {
				continue
			}
			found = true
			if t.Domain != domain {
				return &apperr.AppError{Code: apperr.CodeConfirmation, Message: "token domain mismatch"}
			}
			if t.Used {
				return &apperr.AppError{Code: apperr.CodeConfirmation, Message: "confirmation token already used"}
			}
			if now.UTC().After(t.ExpiresAt) {
				return &apperr.AppError{Code: apperr.CodeConfirmation, Message: "confirmation token expired"}
			}
			t.Used = true
			used = *t
			return nil
		}
		return nil
	})
	if err != nil {
		return store.ConfirmToken{}, err
	}
	if !found {
		return store.ConfirmToken{}, &apperr.AppError{Code: apperr.CodeConfirmation, Message: "confirmation token not found"}
	}
	return used, nil
}

func RequireAutoEnabled(autoEnabled bool, ackHash string) error {
	if !autoEnabled || ackHash == "" {
		return &apperr.AppError{Code: apperr.CodeSafety, Message: "auto-purchase is not enabled"}
	}
	return nil
}
