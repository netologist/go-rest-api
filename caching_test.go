package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestETagAndConditionalRequests(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"id":"ord_100","status":"pending","amount":150.00}`)
	strongETag := ComputeETag(payload)
	weakETag := ComputeWeakETag(payload)

	if !strings.HasPrefix(strongETag, `"`) || !strings.HasSuffix(strongETag, `"`) {
		t.Errorf("strong ETag must be enclosed in quotes, got %s", strongETag)
	}
	if !strings.HasPrefix(weakETag, `W/"`) {
		t.Errorf("weak ETag must start with W/\", got %s", weakETag)
	}

	t.Run("CheckIfNoneMatch matches identical ETag", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/orders/1", nil)
		req.Header.Set("If-None-Match", strongETag)

		if !CheckIfNoneMatch(req, strongETag) {
			t.Errorf("expected CheckIfNoneMatch to return true for matching tag %s", strongETag)
		}
	})

	t.Run("CheckIfNoneMatch matches wildcard *", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/orders/1", nil)
		req.Header.Set("If-None-Match", "*")

		if !CheckIfNoneMatch(req, strongETag) {
			t.Error("expected CheckIfNoneMatch to return true for *")
		}
	})

	t.Run("CheckIfNoneMatch returns false for mismatching tag", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/orders/1", nil)
		req.Header.Set("If-None-Match", `"different-tag-1234"`)

		if CheckIfNoneMatch(req, strongETag) {
			t.Error("expected CheckIfNoneMatch to return false for mismatching tag")
		}
	})

	t.Run("CheckIfMatch returns true for matching ETag on update", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/orders/1", nil)
		req.Header.Set("If-Match", strongETag)

		if !CheckIfMatch(req, strongETag) {
			t.Error("expected CheckIfMatch to return true for matching tag")
		}
	})

	t.Run("CheckIfMatch returns false for stale ETag (mid-air collision prevented)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/orders/1", nil)
		req.Header.Set("If-Match", `"stale-etag-9999"`)

		if CheckIfMatch(req, strongETag) {
			t.Error("expected CheckIfMatch to return false for stale tag")
		}
	})
}

func TestCacheDirectiveAndHeaders(t *testing.T) {
	t.Parallel()

	directive := CacheDirective{
		Public:         true,
		MaxAge:         60 * time.Second,
		MustRevalidate: true,
	}

	headerVal := directive.String()
	if !strings.Contains(headerVal, "public") || !strings.Contains(headerVal, "max-age=60") || !strings.Contains(headerVal, "must-revalidate") {
		t.Errorf("unexpected Cache-Control header: %s", headerVal)
	}

	w := httptest.NewRecorder()
	lastMod := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	SetCacheHeaders(w, `"etag-123"`, lastMod, directive)

	if w.Header().Get("ETag") != `"etag-123"` {
		t.Errorf("expected ETag header, got %s", w.Header().Get("ETag"))
	}
	if w.Header().Get("Last-Modified") == "" {
		t.Error("expected Last-Modified header")
	}
	if !strings.Contains(w.Header().Get("Vary"), "Accept") {
		t.Error("expected Vary header to contain Accept")
	}
}
