# Pattern 12: Webhooks, Event Delivery & HMAC Signatures

## 1. Executive Summary & Core Concept

While standard REST APIs follow a **pull model** (clients continuously poll for updates), **Webhooks** implement an asynchronous **push model** (the server notifies subscribers over HTTP `POST` when domain events occur, e.g. `order.paid`, `refund.created`).

```
[REST API] ---- Event: "order.paid" ----> HTTP POST https://customer.com/webhook
                                          Headers:
                                            X-Signature-256: t=1724932800,v1=a9f4...
                                            X-Webhook-Event: order.paid
```

---

## 2. Cryptographic Security & Replay Attack Defense

Webhook endpoints are publicly reachable on the open internet. To ensure authenticity and integrity, webhook senders must sign payloads using **HMAC-SHA256**.

### Two Critical Vulnerabilities & Defenses

1. **Payload Tampering:** An attacker in the middle alters the payload (e.g. changing order amount).
   - **Defense:** HMAC-SHA256 signature calculated over the payload bytes with a shared secret.
2. **Replay Attacks:** An attacker intercepts a legitimate, signed webhook request (e.g. `user.credited $100`) and resends the exact same HTTP request multiple times.
   - **Defense:** **Timestamped Signatures** ($t = \text{unix\_timestamp}$). The signature is calculated over `$timestamp . $payload`. The receiver rejects any webhook whose timestamp is older than a tolerance window (e.g. 5 minutes).

```
Signature Header: X-Signature-256: t=1724932800,v1=9b7c8d9e2a...
Signed Content:   1724932800.{"id":"evt_1","type":"order.paid"}
```

---

## 3. HMAC Signature Implementation in Go

### Generating Webhook Signatures

```go
func ComputeWebhookSignature(payload []byte, secret []byte, timestamp int64) string {
	mac := hmac.New(sha256.New, secret)
	signedContent := fmt.Sprintf("%d.%s", timestamp, string(payload))
	mac.Write([]byte(signedContent))
	return "t=" + strconv.FormatInt(timestamp, 10) + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}
```

### Inbound Verification Middleware (Constant-Time Compare)

```go
func VerifyWebhookSignature(payload []byte, secret []byte, signatureHeader string, tolerance time.Duration) error {
	ts, sigHex, err := parseSignatureHeader(signatureHeader)
	if err != nil {
		return err
	}

	// 1. Replay attack check
	eventTime := time.Unix(ts, 0)
	if tolerance > 0 && time.Since(eventTime).Abs() > tolerance {
		return errors.New("webhook signature expired")
	}

	// 2. Cryptographic verification with constant-time comparison
	expectedSigBytes, _ := hex.DecodeString(sigHex)
	mac := hmac.New(sha256.New, secret)
	signedContent := fmt.Sprintf("%d.%s", ts, string(payload))
	mac.Write([]byte(signedContent))
	actualSig := mac.Sum(nil)

	if !hmac.Equal(expectedSigBytes, actualSig) {
		return errors.New("webhook signature mismatch")
	}
	return nil
}
```

---

## 4. Reliable Event Delivery with Exponential Backoff

Recipient servers may experience temporary downtime or network issues. The webhook sender must retry deliveries using exponential backoff:

```go
func (s *WebhookSender) Deliver(ctx context.Context, targetURL string, event WebhookEvent) error {
	payload, _ := json.Marshal(event)
	timestamp := time.Now().Unix()
	signature := ComputeWebhookSignature(payload, s.secret, timestamp)

	for attempt := range s.maxTries {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
		req.Header.Set("X-Signature-256", signature)
		req.Header.Set("X-Webhook-Event", event.Type)

		resp, err := s.client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			_ = resp.Body.Close()
			return nil // Delivered successfully
		}

		// Randomized exponential backoff
		backoff := float64(100*time.Millisecond) * math.Pow(2, float64(attempt))
		jitter := time.Duration(rand.Float64() * backoff)
		time.Sleep(jitter)
	}
	return errors.New("webhook delivery exhausted all retries")
}
```
