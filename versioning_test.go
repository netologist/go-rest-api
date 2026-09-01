package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeprecationMiddleware(t *testing.T) {
	t.Parallel()

	sunsetDate := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	deprecationDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	cfg := DeprecationConfig{
		DeprecationDate:  deprecationDate,
		SunsetDate:       sunsetDate,
		SuccessorVersion: "/api/v2/orders",
		DocumentationURL: "https://api.example.com/migrations/orders-v2",
	}

	handler := DeprecationMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Deprecation") != "@1768435200" { // Unix for 2026-01-15T00:00:00Z
		t.Errorf("expected Deprecation header @1768435200, got %s", w.Header().Get("Deprecation"))
	}
	if !strings.Contains(w.Header().Get("Sunset"), "2027") {
		t.Errorf("expected Sunset header with 2027, got %s", w.Header().Get("Sunset"))
	}

	links := w.Header().Values("Link")
	var hasSuccessor, hasDepDocs bool
	for _, l := range links {
		if strings.Contains(l, `rel="successor-version"`) {
			hasSuccessor = true
		}
		if strings.Contains(l, `rel="deprecation"`) {
			hasDepDocs = true
		}
	}

	if !hasSuccessor {
		t.Error("missing Link header with rel=successor-version")
	}
	if !hasDepDocs {
		t.Error("missing Link header with rel=deprecation")
	}
}

func TestHeaderVersioningMiddleware(t *testing.T) {
	t.Parallel()

	handler := HeaderVersioningMiddleware("v1")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := GetAPIVersion(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("version:" + v))
	}))

	t.Run("Default fallback version when no header present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Body.String() != "version:v1" {
			t.Errorf("expected default version:v1, got %s", w.Body.String())
		}
	})

	t.Run("X-API-Version header selects version", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-API-Version", "v2.5")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Body.String() != "version:v2.5" {
			t.Errorf("expected version:v2.5, got %s", w.Body.String())
		}
		if w.Header().Get("X-API-Version-Selected") != "v2.5" {
			t.Errorf("expected echo header X-API-Version-Selected: v2.5")
		}
	})
}
