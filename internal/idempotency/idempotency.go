package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/sportwhiz/gdcli/internal/store"
)

func OperationKey(opType, domain string, amount float64, now time.Time) string {
	day := now.UTC().Format("2006-01-02")
	raw := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%.2f|%s", opType, domain, amount, day)))
	return hex.EncodeToString(raw[:16])
}

func AlreadySucceeded(operationKey string) (bool, error) {
	ops, err := store.ReadOperations()
	if err != nil {
		return false, err
	}
	for _, op := range ops {
		if op.OperationID == operationKey && op.Status == "succeeded" {
			return true, nil
		}
	}
	return false, nil
}
