package main

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// ErrRetryable marks an error as safe to retry (e.g. a transient network
// error or a 5xx from a downstream service). Wrap downstream errors with
// fmt.Errorf("...: %w", ErrRetryable) or errors.Join to make them retryable.
var ErrRetryable = errors.New("retryable error")

// RetryWithBackoff retries fn using exponential backoff with full jitter.
// Only errors matching ErrRetryable are retried; anything else (including
// validation errors) is returned immediately, since blindly retrying a
// non-idempotent, non-transient failure would risk duplicate side effects.
func RetryWithBackoff(ctx context.Context, maxAttempts int, baseDelay time.Duration, fn func() error) error {
	var lastErr error
	for attempt := range maxAttempts {
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !errors.Is(lastErr, ErrRetryable) {
			return lastErr
		}

		backoff := float64(baseDelay) * math.Pow(2, float64(attempt))
		jittered := time.Duration(rand.Float64() * backoff)

		select {
		case <-time.After(jittered):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return lastErr
}

// CircuitState represents the state of a CircuitBreaker.
type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

// ErrCircuitOpen is returned when a call is rejected because the breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker is open: downstream service is not currently being called")

// CircuitBreaker is a minimal, dependency-free circuit breaker suitable for
// wrapping calls to a single downstream dependency. For production use,
// consider a maintained library (e.g. sony/gobreaker) with better metrics
// and sliding-window failure tracking.
type CircuitBreaker struct {
	mu               sync.Mutex
	state            CircuitState
	failureCount     int
	failureThreshold int
	openedAt         time.Time
	resetTimeout     time.Duration
}

func NewCircuitBreaker(failureThreshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{state: StateClosed, failureThreshold: failureThreshold, resetTimeout: resetTimeout}
}

// Execute runs fn if the circuit allows it, and updates the circuit's state
// based on the outcome.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	if cb.state == StateOpen {
		if time.Since(cb.openedAt) > cb.resetTimeout {
			cb.state = StateHalfOpen
		} else {
			cb.mu.Unlock()
			return ErrCircuitOpen
		}
	}
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()
	if err != nil {
		cb.failureCount++
		if cb.state == StateHalfOpen || cb.failureCount >= cb.failureThreshold {
			cb.state = StateOpen
			cb.openedAt = time.Now()
		}
		return err
	}

	cb.state = StateClosed
	cb.failureCount = 0
	return nil
}
