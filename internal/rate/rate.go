package rate

import (
	"context"
	"math/rand"
	"sync"
	"time"

	apperr "github.com/sportwhiz/gdcli/internal/errors"
)

type Limiter struct {
	interval time.Duration
	last     time.Time
	mu       sync.Mutex
}

func NewLimiter(rpm int) *Limiter {
	if rpm <= 0 {
		rpm = 55
	}
	return &Limiter{interval: time.Minute / time.Duration(rpm)}
}

func (l *Limiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	next := l.last.Add(l.interval)
	if next.Before(now) {
		next = now
	}
	l.last = next
	l.mu.Unlock()

	wait := time.Until(next)
	if wait <= 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func Retry(ctx context.Context, attempts int, fn func() (bool, error)) error {
	if attempts < 1 {
		attempts = 1
	}
	base := 250 * time.Millisecond
	for i := 0; i < attempts; i++ {
		retryable, err := fn()
		if err == nil {
			return nil
		}
		if !retryable {
			return err
		}
		if i == attempts-1 {
			return &apperr.AppError{Code: apperr.CodeRateLimited, Message: "request exhausted retries", Retryable: true, Cause: err}
		}
		jitter := time.Duration(rand.Intn(250)) * time.Millisecond
		wait := base*(1<<i) + jitter
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil
}
