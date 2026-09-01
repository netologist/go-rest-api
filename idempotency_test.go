package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdempotencyStore(t *testing.T) {
	t.Parallel()

	store := NewIdempotencyStore(1 * time.Second)
	var executionCount atomic.Int32

	handler := store.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executionCount.Add(1)
		w.Header().Set("Custom-Header", "custom-value")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"res_123"}`))
	}))

	t.Run("GET requests bypass idempotency key check", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/orders", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", w.Code)
		}
	})

	t.Run("POST without Idempotency-Key returns 400 Bad Request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader([]byte(`{"amount":100}`)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("First POST succeeds and second identical POST replays cached response", func(t *testing.T) {
		execStart := executionCount.Load()
		body := []byte(`{"amount":250,"currency":"USD"}`)

		// 1st Call
		req1 := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
		req1.Header.Set("Idempotency-Key", "key_alpha_1")
		w1 := httptest.NewRecorder()
		handler.ServeHTTP(w1, req1)

		if w1.Code != http.StatusCreated {
			t.Fatalf("1st call: expected status 201, got %d", w1.Code)
		}
		if w1.Header().Get("Custom-Header") != "custom-value" {
			t.Errorf("expected Custom-Header to be preserved")
		}
		if w1.Header().Get("X-Idempotent-Replayed") != "" {
			t.Errorf("1st call should not have X-Idempotent-Replayed header")
		}

		// 2nd Call with exact same key and body
		req2 := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
		req2.Header.Set("Idempotency-Key", "key_alpha_1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		if w2.Code != http.StatusCreated {
			t.Fatalf("2nd call: expected status 201, got %d", w2.Code)
		}
		if w2.Header().Get("X-Idempotent-Replayed") != "true" {
			t.Errorf("2nd call expected X-Idempotent-Replayed: true")
		}
		if w2.Body.String() != `{"id":"res_123"}` {
			t.Errorf("replayed body mismatch: got %s", w2.Body.String())
		}
		if executionCount.Load() != execStart+1 {
			t.Errorf("handler was executed %d times, expected exactly 1 execution", executionCount.Load()-execStart)
		}
	})

	t.Run("Reusing same key with different body returns 422 Unprocessable Entity", func(t *testing.T) {
		key := "key_payload_mismatch"
		body1 := []byte(`{"amount":100}`)
		body2 := []byte(`{"amount":200}`)

		req1 := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body1))
		req1.Header.Set("Idempotency-Key", key)
		w1 := httptest.NewRecorder()
		handler.ServeHTTP(w1, req1)

		req2 := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body2))
		req2.Header.Set("Idempotency-Key", key)
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		if w2.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected status 422 for payload mismatch under same key, got %d", w2.Code)
		}
	})

	t.Run("Concurrent requests with same key trigger 409 Conflict for second caller", func(t *testing.T) {
		key := "key_concurrent"
		slowHandler := store.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))

		var wg sync.WaitGroup
		var statusCodes [2]int
		body := []byte(`{"amount":50}`)

		wg.Add(2)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
			req.Header.Set("Idempotency-Key", key)
			w := httptest.NewRecorder()
			slowHandler.ServeHTTP(w, req)
			statusCodes[0] = w.Code
		}()

		go func() {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond) // Ensure first goroutine acquired lock and is inFlight
			req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
			req.Header.Set("Idempotency-Key", key)
			w := httptest.NewRecorder()
			slowHandler.ServeHTTP(w, req)
			statusCodes[1] = w.Code
		}()

		wg.Wait()

		// One should succeed (200), the other should get 409 Conflict
		has200 := statusCodes[0] == http.StatusOK || statusCodes[1] == http.StatusOK
		has409 := statusCodes[0] == http.StatusConflict || statusCodes[1] == http.StatusConflict
		if !has200 || !has409 {
			t.Errorf("expected one 200 OK and one 409 Conflict, got %v", statusCodes)
		}
	})
}
