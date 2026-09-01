package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadinessChecker(t *testing.T) {
	t.Parallel()

	t.Run("All healthy probes return 200 OK with UP status", func(t *testing.T) {
		checker := NewReadinessChecker("1.0.0")
		checker.RegisterProbe("db", func(ctx context.Context) error { return nil })
		checker.RegisterProbe("redis", func(ctx context.Context) error { return nil })

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		checker.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var resp HealthCheckResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Status != StatusHealthy {
			t.Errorf("expected status UP, got %s", resp.Status)
		}
		if len(resp.Components) != 2 {
			t.Errorf("expected 2 components, got %d", len(resp.Components))
		}
	})

	t.Run("Failing probe returns 503 Service Unavailable with DOWN status", func(t *testing.T) {
		checker := NewReadinessChecker("1.0.0")
		checker.RegisterProbe("db", func(ctx context.Context) error { return nil })
		checker.RegisterProbe("payment_provider", func(ctx context.Context) error {
			return errors.New("connection timeout to payment provider")
		})

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		checker.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected status 503 Service Unavailable, got %d", w.Code)
		}

		var resp HealthCheckResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Status != StatusUnhealthy {
			t.Errorf("expected status DOWN, got %s", resp.Status)
		}
		if resp.Components["payment_provider"].Status != StatusUnhealthy {
			t.Errorf("expected payment_provider to be DOWN, got %s", resp.Components["payment_provider"].Status)
		}
	})
}

func TestAPIMetricsMiddleware(t *testing.T) {
	t.Parallel()

	metrics := NewAPIMetrics()
	handler := metrics.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/success" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	// Send requests
	req1 := httptest.NewRequest(http.MethodGet, "/success", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/missing", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	// Check metrics endpoint
	reqMetrics := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	wMetrics := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(wMetrics, reqMetrics)

	metricsBody := wMetrics.Body.String()
	if !strings.Contains(metricsBody, `http_requests_total{status="2xx"} 1`) {
		t.Errorf("expected status 2xx count of 1 in metrics, got:\n%s", metricsBody)
	}
	if !strings.Contains(metricsBody, `http_requests_total{status="4xx"} 1`) {
		t.Errorf("expected status 4xx count of 1 in metrics, got:\n%s", metricsBody)
	}
}
