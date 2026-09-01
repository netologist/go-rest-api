package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTokenBucket(t *testing.T) {
	t.Parallel()

	tb := NewTokenBucket(3, 10) // capacity 3, refill 10 tokens/sec

	// Consume capacity
	for i := range 3 {
		allowed, wait := tb.Allow()
		if !allowed {
			t.Fatalf("request %d should have been allowed", i+1)
		}
		if wait != 0 {
			t.Errorf("wait duration should be 0 for allowed request, got %v", wait)
		}
	}

	// 4th request must be rejected
	allowed, wait := tb.Allow()
	if allowed {
		t.Fatal("4th request should have been rejected")
	}
	if wait <= 0 {
		t.Errorf("wait duration should be positive, got %v", wait)
	}

	// Wait for refill
	time.Sleep(120 * time.Millisecond) // ~1 token refilled
	allowed, _ = tb.Allow()
	if !allowed {
		t.Fatal("request after refill should have been allowed")
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	t.Parallel()

	store := NewRateLimiterStore(2, 5) // 2 tokens capacity

	handler := store.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	// 1st request with API key "client_1"
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.Header.Set("X-API-Key", "client_1")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w1.Code)
	}
	if w1.Header().Get("RateLimit-Limit") != "2" {
		t.Errorf("expected RateLimit-Limit 2, got %s", w1.Header().Get("RateLimit-Limit"))
	}

	// 2nd request
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set("X-API-Key", "client_1")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w2.Code)
	}

	// 3rd request from same client -> 429
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.Header.Set("X-API-Key", "client_1")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	if w3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", w3.Code)
	}
	if w3.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}

	// Different client ("client_2") should have separate bucket and succeed
	reqOther := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqOther.Header.Set("X-API-Key", "client_2")
	wOther := httptest.NewRecorder()
	handler.ServeHTTP(wOther, reqOther)
	if wOther.Code != http.StatusOK {
		t.Fatalf("expected status 200 for isolated client, got %d", wOther.Code)
	}
}
