package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// WebhookEvent represents an outbound event notification delivered to subscribers.
type WebhookEvent struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`      // e.g. "order.created", "order.paid"
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

// ComputeWebhookSignature creates an HMAC-SHA256 signature over "<timestamp>.<payload>".
func ComputeWebhookSignature(payload []byte, secret []byte, timestamp int64) string {
	mac := hmac.New(sha256.New, secret)
	signedContent := fmt.Sprintf("%d.%s", timestamp, string(payload))
	mac.Write([]byte(signedContent))
	return "t=" + strconv.FormatInt(timestamp, 10) + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhookSignature verifies an inbound HMAC-SHA256 signature and guards against replay attacks.
func VerifyWebhookSignature(payload []byte, secret []byte, signatureHeader string, tolerance time.Duration) error {
	if signatureHeader == "" {
		return errors.New("missing webhook signature header")
	}

	var timestampStr, sigHex string
	parts := bytes.Split([]byte(signatureHeader), []byte(","))
	for _, p := range parts {
		kv := bytes.SplitN(p, []byte("="), 2)
		if len(kv) == 2 {
			k := string(bytes.TrimSpace(kv[0]))
			v := string(bytes.TrimSpace(kv[1]))
			if k == "t" {
				timestampStr = v
			} else if k == "v1" {
				sigHex = v
			}
		}
	}

	if timestampStr == "" || sigHex == "" {
		return errors.New("invalid signature header format: expected t=<timestamp>,v1=<signature>")
	}

	ts, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return errors.New("invalid timestamp in signature")
	}

	// Replay attack check: verify timestamp is within acceptable tolerance
	eventTime := time.Unix(ts, 0)
	if tolerance > 0 && time.Since(eventTime).Abs() > tolerance {
		return fmt.Errorf("webhook signature expired: event timestamp %s is older than tolerance %v", eventTime, tolerance)
	}

	expectedSigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return errors.New("invalid hex encoding in signature")
	}

	mac := hmac.New(sha256.New, secret)
	signedContent := fmt.Sprintf("%d.%s", ts, string(payload))
	mac.Write([]byte(signedContent))
	actualSig := mac.Sum(nil)

	if !hmac.Equal(expectedSigBytes, actualSig) {
		return errors.New("webhook signature mismatch")
	}

	return nil
}

// WebhookSender delivers webhook events to registered endpoint URLs with exponential backoff.
type WebhookSender struct {
	client  *http.Client
	secret  []byte
	maxTries int
}

// NewWebhookSender creates a webhook dispatcher.
func NewWebhookSender(secret []byte, maxRetries int) *WebhookSender {
	return &WebhookSender{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		secret:  secret,
		maxTries: maxRetries,
	}
}

// Deliver sends a signed WebhookEvent to a target URL, retrying transient network errors.
func (s *WebhookSender) Deliver(ctx context.Context, targetURL string, event WebhookEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	timestamp := time.Now().Unix()
	signature := ComputeWebhookSignature(payload, s.secret, timestamp)

	var lastErr error
	for attempt := range s.maxTries {
		if err := ctx.Err(); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("X-Signature-256", signature)
		req.Header.Set("X-Webhook-Event", event.Type)
		req.Header.Set("X-Webhook-ID", event.ID)

		resp, err := s.client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil // Successfully delivered
			}
			lastErr = fmt.Errorf("webhook endpoint returned status %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		// Exponential backoff with jitter
		backoff := float64(100*time.Millisecond) * math.Pow(2, float64(attempt))
		jitter := time.Duration(rand.Float64() * backoff)
		select {
		case <-time.After(jitter):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("failed to deliver webhook after %d attempts: %w", s.maxTries, lastErr)
}

// WebhookVerificationMiddleware validates inbound webhooks before passing them to the handler.
func WebhookVerificationMiddleware(secret []byte, tolerance time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sig := r.Header.Get("X-Signature-256")
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				BadRequest(w, r, "Failed to read request body.")
				return
			}
			// Restore request body for downstream handlers
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			if err := VerifyWebhookSignature(bodyBytes, secret, sig, tolerance); err != nil {
				Unauthorized(w, r, "Webhook verification failed: "+err.Error())
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
