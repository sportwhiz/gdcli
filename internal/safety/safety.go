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
	ts, err := store.LoadTokens()
	if err != nil {
		return store.ConfirmToken{}, err
	}
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
	if err := store.SaveTokens(ts); err != nil {
		return store.ConfirmToken{}, err
	}
	return t, nil
}

func ValidateAndUseToken(tokenID, domain string, now time.Time) (store.ConfirmToken, error) {
	ts, err := store.LoadTokens()
	if err != nil {
		return store.ConfirmToken{}, err
	}
	for i := range ts.Tokens {
		t := &ts.Tokens[i]
		if t.TokenID != tokenID {
			continue
		}
		if t.Domain != domain {
			return store.ConfirmToken{}, &apperr.AppError{Code: apperr.CodeConfirmation, Message: "token domain mismatch"}
		}
		if t.Used {
			return store.ConfirmToken{}, &apperr.AppError{Code: apperr.CodeConfirmation, Message: "confirmation token already used"}
		}
		if now.UTC().After(t.ExpiresAt) {
			return store.ConfirmToken{}, &apperr.AppError{Code: apperr.CodeConfirmation, Message: "confirmation token expired"}
		}
		t.Used = true
		if err := store.SaveTokens(ts); err != nil {
			return store.ConfirmToken{}, err
		}
		return *t, nil
	}
	return store.ConfirmToken{}, &apperr.AppError{Code: apperr.CodeConfirmation, Message: "confirmation token not found"}
}

func RequireAutoEnabled(autoEnabled bool, ackHash string) error {
	if !autoEnabled || ackHash == "" {
		return &apperr.AppError{Code: apperr.CodeSafety, Message: "auto-purchase is not enabled"}
	}
	return nil
}
