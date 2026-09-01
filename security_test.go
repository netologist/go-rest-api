package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	t.Parallel()

	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	headers := w.Header()
	if headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options: nosniff, got %s", headers.Get("X-Content-Type-Options"))
	}
	if headers.Get("X-Frame-Options") != "DENY" {
		t.Errorf("expected X-Frame-Options: DENY, got %s", headers.Get("X-Frame-Options"))
	}
	if !strings.Contains(headers.Get("Strict-Transport-Security"), "max-age=") {
		t.Errorf("expected Strict-Transport-Security header, got %s", headers.Get("Strict-Transport-Security"))
	}
	if !strings.Contains(headers.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Errorf("expected CSP header, got %s", headers.Get("Content-Security-Policy"))
	}
}

func TestCORSMiddleware(t *testing.T) {
	t.Parallel()

	opts := DefaultCORSOptions()
	opts.AllowedOrigins = []string{"https://app.example.com"}
	opts.AllowCredentials = true

	handler := CORSMiddleware(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
	}))

	t.Run("Preflight OPTIONS request from allowed origin returns 204 No Content with CORS headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/orders", nil)
		req.Header.Set("Origin", "https://app.example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected status 204 No Content for preflight, got %d", w.Code)
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
			t.Errorf("expected Access-Control-Allow-Origin https://app.example.com, got %s", w.Header().Get("Access-Control-Allow-Origin"))
		}
		if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Error("expected Access-Control-Allow-Credentials: true")
		}
		if !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "POST") {
			t.Errorf("expected Allow-Methods to contain POST, got %s", w.Header().Get("Access-Control-Allow-Methods"))
		}
	})

	t.Run("Disallowed origin does not get CORS headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/orders", nil)
		req.Header.Set("Origin", "https://evil.attacker.com")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("disallowed origin should not receive Access-Control-Allow-Origin, got %s", w.Header().Get("Access-Control-Allow-Origin"))
		}
	})
}

func TestRequestBodyLimitMiddleware(t *testing.T) {
	t.Parallel()

	// 50 bytes limit
	handler := RequestBodyLimitMiddleware(50)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			PayloadTooLarge(w, r, "Request payload exceeds size limit.")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Body within limit succeeds", func(t *testing.T) {
		smallBody := []byte(`{"message":"hello"}`)
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(smallBody))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 for small body, got %d", w.Code)
		}
	})

	t.Run("Body exceeding limit fails with 413 Payload Too Large", func(t *testing.T) {
		largeBody := make([]byte, 200) // Exceeds 50 bytes
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(largeBody))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("expected status 413 Payload Too Large, got %d", w.Code)
		}
	})
}

func TestProblemRecoveryMiddleware(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := ProblemRecoveryMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("simulated critical crash")
	}))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 on recovered panic, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/problem+json" {
		t.Errorf("expected application/problem+json Content-Type, got %s", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), "Internal Server Error") {
		t.Errorf("expected RFC 7807 problem details in body, got %s", w.Body.String())
	}
}
