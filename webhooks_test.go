package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebhookSignatures(t *testing.T) {
	t.Parallel()

	secret := []byte("test-webhook-secret-key-12345")
	payload := []byte(`{"id":"evt_1","type":"order.created","amount":100}`)
	now := time.Now().Unix()

	sigHeader := ComputeWebhookSignature(payload, secret, now)

	t.Run("Valid signature verifies successfully", func(t *testing.T) {
		err := VerifyWebhookSignature(payload, secret, sigHeader, 5*time.Minute)
		if err != nil {
			t.Fatalf("expected signature verification to pass, got: %v", err)
		}
	})

	t.Run("Tampered payload fails verification", func(t *testing.T) {
		tampered := []byte(`{"id":"evt_1","type":"order.created","amount":999}`)
		err := VerifyWebhookSignature(tampered, secret, sigHeader, 5*time.Minute)
		if err == nil {
			t.Fatal("expected signature mismatch error on tampered payload, got nil")
		}
	})

	t.Run("Expired timestamp fails verification (replay attack prevention)", func(t *testing.T) {
		oldTimestamp := time.Now().Add(-10 * time.Minute).Unix()
		oldSigHeader := ComputeWebhookSignature(payload, secret, oldTimestamp)

		err := VerifyWebhookSignature(payload, secret, oldSigHeader, 5*time.Minute)
		if err == nil {
			t.Fatal("expected timestamp expired error, got nil")
		}
	})

	t.Run("Incorrect secret fails verification", func(t *testing.T) {
		wrongSecret := []byte("wrong-secret-key-54321")
		err := VerifyWebhookSignature(payload, wrongSecret, sigHeader, 5*time.Minute)
		if err == nil {
			t.Fatal("expected signature mismatch error with wrong secret, got nil")
		}
	})
}

func TestWebhookVerificationMiddleware(t *testing.T) {
	t.Parallel()

	secret := []byte("middleware-secret-key")
	mw := WebhookVerificationMiddleware(secret, 5*time.Minute)

	handlerCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Missing signature header returns 401 Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewReader([]byte(`{}`)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401 Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Valid webhook passes middleware", func(t *testing.T) {
		body := []byte(`{"event":"payment.succeeded"}`)
		sig := ComputeWebhookSignature(body, secret, time.Now().Unix())

		req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewReader(body))
		req.Header.Set("X-Signature-256", sig)
		w := httptest.NewRecorder()

		handlerCalled = false
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 OK, got %d", w.Code)
		}
		if !handlerCalled {
			t.Error("expected downstream handler to be called")
		}
	})
}

func TestWebhookSenderDelivery(t *testing.T) {
	t.Parallel()

	secret := []byte("sender-secret-12345")
	sender := NewWebhookSender(secret, 3)

	var serverReceived bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Signature-256") != "" && r.Header.Get("X-Webhook-Event") == "order.paid" {
			serverReceived = true
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	event := WebhookEvent{
		ID:        "evt_delivery_1",
		Type:      "order.paid",
		Timestamp: time.Now().UTC(),
		Data:      map[string]any{"orderId": "ord_100"},
	}

	err := sender.Deliver(context.Background(), ts.URL, event)
	if err != nil {
		t.Fatalf("failed to deliver webhook: %v", err)
	}
	if !serverReceived {
		t.Error("test server did not receive webhook")
	}
}
