package main

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// TokenBucket implements the classic token bucket algorithm: tokens refill at
// a constant rate up to a capacity, and each allowed request consumes one.
// This allows legitimate bursts while preserving a long-term average rate.
type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	refillRate float64 // tokens added per second
	lastRefill time.Time
}

func NewTokenBucket(capacity, refillRatePerSecond float64) *TokenBucket {
	return &TokenBucket{tokens: capacity, capacity: capacity, refillRate: refillRatePerSecond, lastRefill: time.Now()}
}

// Allow reports whether a request may proceed. If not, it returns how long
// the caller should wait before the next token becomes available.
func (b *TokenBucket) Allow() (bool, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = minFloat(b.capacity, b.tokens+elapsed*b.refillRate)
	b.lastRefill = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / b.refillRate * float64(time.Second))
	return false, wait
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// RateLimiterStore keeps a separate token bucket per client, identified by
// API key (preferred) or, failing that, remote address.
type RateLimiterStore struct {
	mu      sync.Mutex
	buckets map[string]*TokenBucket
	cap     float64
	refill  float64
}

func NewRateLimiterStore(capacity, refillRatePerSecond float64) *RateLimiterStore {
	return &RateLimiterStore{buckets: make(map[string]*TokenBucket), cap: capacity, refill: refillRatePerSecond}
}

func (s *RateLimiterStore) getBucket(clientID string) *TokenBucket {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.buckets[clientID]
	if !ok {
		b = NewTokenBucket(s.cap, s.refill)
		s.buckets[clientID] = b
	}
	return b
}

// Middleware applies per-client rate limiting and, on success, sets
// RateLimit-Limit / RateLimit-Remaining headers. On failure it returns
// 429 Too Many Requests with a Retry-After header via TooManyRequests.
func (s *RateLimiterStore) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID := r.Header.Get("X-API-Key")
		if clientID == "" {
			// RemoteAddr includes an ephemeral port that differs per TCP
			// connection, so strip it down to just the IP — otherwise every
			// connection would land in its own bucket and the limiter would
			// never actually trigger.
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			clientID = host
		}

		bucket := s.getBucket(clientID)
		allowed, wait := bucket.Allow()

		w.Header().Set("RateLimit-Limit", strconv.Itoa(int(s.cap)))
		if allowed {
			bucket.mu.Lock()
			remaining := int(bucket.tokens)
			bucket.mu.Unlock()
			w.Header().Set("RateLimit-Remaining", strconv.Itoa(remaining))
			next.ServeHTTP(w, r)
			return
		}

		retryAfterSeconds := int(wait.Seconds()) + 1
		w.Header().Set("RateLimit-Remaining", "0")
		TooManyRequests(w, r, retryAfterSeconds)
	})
}
