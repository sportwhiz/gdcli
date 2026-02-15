package rate

import (
	"context"
	"errors"
	"testing"
)

func TestRetryEventuallySucceeds(t *testing.T) {
	count := 0
	err := Retry(context.Background(), 3, func() (bool, error) {
		count++
		if count < 3 {
			return true, errors.New("temp")
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("retry should succeed: %v", err)
	}
}
