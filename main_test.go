package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFullAPIRouterIntegration(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	router, _, _ := BuildRouter(logger)

	t.Run("GET /healthz returns alive status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), `"alive"`) {
			t.Errorf("expected alive status, got %s", w.Body.String())
		}
		if w.Header().Get("X-Request-Id") == "" {
			t.Error("expected X-Request-Id correlation header")
		}
	})

	t.Run("GET /readyz returns UP status and component health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var health HealthCheckResponse
		if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
			t.Fatalf("failed to decode health check JSON: %v", err)
		}
		if health.Status != StatusHealthy {
			t.Errorf("expected status UP, got %s", health.Status)
		}
	})

	t.Run("GET /metrics returns Prometheus metric format", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "http_requests_total") {
			t.Errorf("expected http_requests_total metric in output, got %s", w.Body.String())
		}
	})

	t.Run("GET /openapi.json and GET /docs return OpenAPI spec and Swagger UI", func(t *testing.T) {
		reqSpec := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
		wSpec := httptest.NewRecorder()
		router.ServeHTTP(wSpec, reqSpec)
		if wSpec.Code != http.StatusOK || !strings.Contains(wSpec.Body.String(), `"openapi": "3.1.0"`) {
			t.Errorf("failed to fetch OpenAPI 3.1 spec")
		}

		reqDocs := httptest.NewRequest(http.MethodGet, "/docs", nil)
		wDocs := httptest.NewRecorder()
		router.ServeHTTP(wDocs, reqDocs)
		if wDocs.Code != http.StatusOK || !strings.Contains(wDocs.Body.String(), "swagger-ui") {
			t.Errorf("failed to fetch Swagger UI HTML")
		}
	})

	t.Run("POST /api/v1/auth/token generates valid JWT Bearer token used on /admin/stats", func(t *testing.T) {
		tokenReqBody := []byte(`{"email":"superadmin@example.com","roles":["admin"]}`)
		reqToken := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", bytes.NewReader(tokenReqBody))
		wToken := httptest.NewRecorder()
		router.ServeHTTP(wToken, reqToken)

		if wToken.Code != http.StatusOK {
			t.Fatalf("expected status 200 for token creation, got %d", wToken.Code)
		}

		var tokenResp struct {
			AccessToken string `json:"accessToken"`
		}
		if err := json.NewDecoder(wToken.Body).Decode(&tokenResp); err != nil || tokenResp.AccessToken == "" {
			t.Fatalf("failed to parse access token from response: %v", err)
		}

		// Access protected /admin/stats with Bearer token
		reqAdmin := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
		reqAdmin.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
		wAdmin := httptest.NewRecorder()
		router.ServeHTTP(wAdmin, reqAdmin)

		if wAdmin.Code != http.StatusOK {
			t.Fatalf("expected status 200 on /admin/stats with admin JWT token, got %d (%s)", wAdmin.Code, wAdmin.Body.String())
		}
	})

	t.Run("GET /legacy-orders returns Deprecation and Sunset headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/legacy-orders", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 on legacy endpoint, got %d", w.Code)
		}
		if w.Header().Get("Deprecation") == "" {
			t.Error("expected Deprecation header on legacy endpoint")
		}
		if w.Header().Get("Sunset") == "" {
			t.Error("expected Sunset header on legacy endpoint")
		}
	})

	t.Run("POST /webhooks/incoming verifies HMAC signature", func(t *testing.T) {
		webhookSecret := []byte("webhook-signing-secret-key-12345")
		body := []byte(`{"event":"order.refunded","id":"evt_999"}`)
		sig := ComputeWebhookSignature(body, webhookSecret, time.Now().Unix())

		req := httptest.NewRequest(http.MethodPost, "/webhooks/incoming", bytes.NewReader(body))
		req.Header.Set("X-Signature-256", sig)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 on signed webhook, got %d (%s)", w.Code, w.Body.String())
		}
	})
}
