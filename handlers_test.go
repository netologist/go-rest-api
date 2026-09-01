package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func setupTestOrderRouter() (*chi.Mux, *OrderHandler) {
	cb := NewCircuitBreaker(5, 10*time.Second)
	h := NewOrderHandler(cb)

	r := chi.NewRouter()
	r.Get("/orders", h.List)
	r.Post("/orders", h.Create)
	r.Post("/orders/form", h.CreateFromForm)
	r.Get("/orders/{id}", h.Get)
	r.Put("/orders/{id}", h.Update)
	r.Post("/orders/{id}/pay", h.Pay)
	r.Post("/orders/{id}/cancel", h.Cancel)
	r.Get("/orders/{id}/events", h.Events)

	return r, h
}

func TestOrderHandlerIntegration(t *testing.T) {
	t.Parallel()

	r, _ := setupTestOrderRouter()

	t.Run("GET /orders returns list of orders with pagination meta", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/orders?limit=10", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode list response: %v", err)
		}
		if _, ok := resp["data"]; !ok {
			t.Error("expected 'data' array in response")
		}
		if _, ok := resp["meta"]; !ok {
			t.Error("expected 'meta' object in response")
		}
	})

	t.Run("GET /orders with sparse fieldsets returns only selected fields", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/orders?fields=id,status", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var resp struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(resp.Data) > 0 {
			first := resp.Data[0]
			if _, ok := first["id"]; !ok {
				t.Error("expected 'id' field in sparse result")
			}
			if _, ok := first["status"]; !ok {
				t.Error("expected 'status' field in sparse result")
			}
			if _, ok := first["amount"]; ok {
				t.Error("expected 'amount' to be filtered out in sparse result")
			}
		}
	})

	t.Run("GET /orders with HAL JSON returns hypermedia collection", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/orders", nil)
		req.Header.Set("Accept", "application/hal+json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		if !strings.Contains(w.Header().Get("Content-Type"), "application/hal+json") {
			t.Errorf("expected Content-Type application/hal+json, got %s", w.Header().Get("Content-Type"))
		}

		var hal HalCollection
		if err := json.NewDecoder(w.Body).Decode(&hal); err != nil {
			t.Fatalf("failed to decode HAL collection: %v", err)
		}
		if len(hal.Embedded) == 0 {
			t.Error("expected _embedded items in HAL collection")
		}
	})

	t.Run("GET /orders/ord_1 returns order with ETag", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/orders/ord_1", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		etag := w.Header().Get("ETag")
		if etag == "" {
			t.Fatal("expected ETag header on GET /orders/ord_1")
		}

		// Conditional GET: 304 Not Modified
		reqCond := httptest.NewRequest(http.MethodGet, "/orders/ord_1", nil)
		reqCond.Header.Set("If-None-Match", etag)
		wCond := httptest.NewRecorder()
		r.ServeHTTP(wCond, reqCond)

		if wCond.Code != http.StatusNotModified {
			t.Fatalf("expected status 304 Not Modified, got %d", wCond.Code)
		}
	})

	t.Run("POST /orders creates new order and returns 201 Created", func(t *testing.T) {
		body := []byte(`{"amount":349.00,"currency":"USD"}`)
		req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201 Created, got %d (%s)", w.Code, w.Body.String())
		}
		location := w.Header().Get("Location")
		if location == "" || !strings.HasPrefix(location, "/orders/ord_") {
			t.Errorf("expected Location header starting with /orders/ord_, got %s", location)
		}
	})

	t.Run("POST /orders/form creates order from form URL-encoded body", func(t *testing.T) {
		formData := "amount=89.50&currency=EUR"
		req := httptest.NewRequest(http.MethodPost, "/orders/form", strings.NewReader(formData))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201 Created from form, got %d", w.Code)
		}
	})

	t.Run("PUT /orders/ord_1 with Optimistic Concurrency Control (If-Match)", func(t *testing.T) {
		// 1. Get current ETag
		getReq := httptest.NewRequest(http.MethodGet, "/orders/ord_1", nil)
		getW := httptest.NewRecorder()
		r.ServeHTTP(getW, getReq)
		currentETag := getW.Header().Get("ETag")

		// 2. Update with matching If-Match -> 200 OK
		updateBody := []byte(`{"amount":199.95}`)
		putReq := httptest.NewRequest(http.MethodPut, "/orders/ord_1", bytes.NewReader(updateBody))
		putReq.Header.Set("If-Match", currentETag)
		putW := httptest.NewRecorder()
		r.ServeHTTP(putW, putReq)

		if putW.Code != http.StatusOK {
			t.Fatalf("expected status 200 on valid If-Match update, got %d (%s)", putW.Code, putW.Body.String())
		}
		newETag := putW.Header().Get("ETag")
		if newETag == currentETag {
			t.Error("expected ETag to change after successful update")
		}

		// 3. Stale update with old ETag -> 412 Precondition Failed
		staleReq := httptest.NewRequest(http.MethodPut, "/orders/ord_1", bytes.NewReader(updateBody))
		staleReq.Header.Set("If-Match", currentETag) // Old ETag
		staleW := httptest.NewRecorder()
		r.ServeHTTP(staleW, staleReq)

		if staleW.Code != http.StatusPreconditionFailed {
			t.Fatalf("expected status 412 Precondition Failed on stale update, got %d", staleW.Code)
		}
	})

	t.Run("POST /orders/{id}/pay state transition", func(t *testing.T) {
		// Pay ord_1 (status is pending) -> 200 OK
		req := httptest.NewRequest(http.MethodPost, "/orders/ord_1/pay", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 on paying pending order, got %d", w.Code)
		}

		// Pay again -> 409 Conflict (already paid)
		req2 := httptest.NewRequest(http.MethodPost, "/orders/ord_1/pay", nil)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)

		if w2.Code != http.StatusConflict {
			t.Fatalf("expected status 409 Conflict when paying already-paid order, got %d", w2.Code)
		}
	})

	t.Run("GET /orders/{id}/events SSE headers", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		req := httptest.NewRequest(http.MethodGet, "/orders/ord_1/events", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Content-Type") != "text/event-stream" {
			t.Errorf("expected Content-Type text/event-stream, got %s", w.Header().Get("Content-Type"))
		}
		if w.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("expected Cache-Control no-cache, got %s", w.Header().Get("Cache-Control"))
		}
	})
}
