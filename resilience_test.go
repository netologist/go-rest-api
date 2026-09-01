package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryWithBackoff(t *testing.T) {
	t.Parallel()

	t.Run("Non-retryable error fails immediately without retrying", func(t *testing.T) {
		var attempts int
		err := RetryWithBackoff(context.Background(), 5, 10*time.Millisecond, func() error {
			attempts++
			return errors.New("permanent validation error")
		})

		if attempts != 1 {
			t.Errorf("expected exactly 1 attempt for non-retryable error, got %d", attempts)
		}
		if err == nil || err.Error() != "permanent validation error" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Retryable error succeeds on third attempt", func(t *testing.T) {
		var attempts int
		err := RetryWithBackoff(context.Background(), 5, 1*time.Millisecond, func() error {
			attempts++
			if attempts < 3 {
				return fmt.Errorf("transient network blip: %w", ErrRetryable)
			}
			return nil
		})

		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
		if err != nil {
			t.Errorf("expected nil error after successful retry, got %v", err)
		}
	})

	t.Run("Context cancellation aborts retry loop promptly", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		var attempts int
		err := RetryWithBackoff(ctx, 10, 50*time.Millisecond, func() error {
			attempts++
			return fmt.Errorf("fail: %w", ErrRetryable)
		})

		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Errorf("expected context deadline exceeded error, got %v", err)
		}
	})
}

func TestCircuitBreaker(t *testing.T) {
	t.Parallel()

	cb := NewCircuitBreaker(3, 50*time.Millisecond)

	// In Closed state, successful calls succeed
	err := cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("expected nil error in closed state, got %v", err)
	}

	// Trigger 3 failures -> trips breaker to StateOpen
	for range 3 {
		_ = cb.Execute(func() error { return errors.New("downstream 500 error") })
	}

	// 4th call should immediately fail with ErrCircuitOpen without executing the function
	var executed atomic.Bool
	err = cb.Execute(func() error {
		executed.Store(true)
		return nil
	})

	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
	if executed.Load() {
		t.Fatal("circuit breaker was open but executed protected function")
	}

	// Wait for resetTimeout -> transitions to StateHalfOpen
	time.Sleep(60 * time.Millisecond)

	// In HalfOpen state, a successful call resets breaker to StateClosed
	err = cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("expected call in half-open state to succeed, got %v", err)
	}

	// Verify breaker is now back to Closed and accepts calls
	err = cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("expected breaker to be Closed, got %v", err)
	}
}
