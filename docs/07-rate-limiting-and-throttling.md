# Pattern 07: Rate Limiting & Throttling

## 1. Executive Summary & Core Motivations

Rate limiting protects backend systems, third-party downstream APIs, and shared infrastructure from:
1. **Denial of Service (DoS) & Brute Force Attacks.**
2. **Cascading Failure & Resource Starvation (Noisy Neighbor Problem).**
3. **Monetization & API Tiering** (Free vs Enterprise tier quotas).

When a client exceeds its allotted quota, the API responds with **`429 Too Many Requests`** along with standard HTTP response headers indicating current limits and retry timing.

---

## 2. Rate Limiting Algorithms Comparison

```
1. Token Bucket: Allows legitimate bursts, refills tokens smoothly over time.
2. Leaky Bucket: Smooths traffic to a strictly constant outbound rate.
3. Fixed Window: Prone to the 2x burst boundary vulnerability at window resets.
4. Sliding Window Log / Counter: High accuracy, prevents window-boundary spikes.
```

| Algorithm | Burst Handling | Memory Efficiency | Implementation Complexity | Best For |
|---|---|---|---|---|
| **Token Bucket** | Excellent (Allows bursts up to bucket capacity) | High ($O(1)$ per client) | Low | **Standard for general-purpose REST APIs (AWS, Stripe).** |
| **Leaky Bucket** | None (Smooths requests into a constant queue rate) | High | Moderate | Outbound packet shaping, strict queue processing. |
| **Fixed Window** | Poor (Allows double burst at boundary) | Very High | Very Low | Basic hourly/daily quota checks. |
| **Sliding Window Counter** | Good (Weighted combination of current + previous window) | High | Moderate | High-accuracy tier limits. |

---

## 3. Standard HTTP Headers (IETF RateLimit Specification)

Modern APIs use standard rate limiting headers:

```http
HTTP/1.1 200 OK
RateLimit-Limit: 100
RateLimit-Remaining: 84
RateLimit-Reset: 1724932800

--- (When Exceeded) ---

HTTP/1.1 429 Too Many Requests
Retry-After: 12
RateLimit-Limit: 100
RateLimit-Remaining: 0
Content-Type: application/problem+json

{
  "type": "https://api.example.com/errors/rate-limited",
  "title": "Too Many Requests",
  "status": 429,
  "detail": "Rate limit exceeded, please retry after 12 seconds."
}
```

---

## 4. Token Bucket Implementation in Go

```go
type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	refillRate float64 // tokens refilled per second
	lastRefill time.Time
}

func (b *TokenBucket) Allow() (bool, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = minFloat(b.capacity, b.tokens+elapsed*b.refillRate)
	b.lastRefill = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	// Calculate wait time until 1 token is available
	wait := time.Duration((1 - b.tokens) / b.refillRate * float64(time.Second))
	return false, wait
}
```

---

## 5. Client Identification & Security Hardening

To apply per-client limits correctly:
1. **Authenticated Requests:** Identify clients by `X-API-Key` or JWT `sub` (User ID / Client ID).
2. **Unauthenticated Requests:** Identify clients by IP address.
   - **Crucial Invariant:** Remote IP addresses from `net.SplitHostPort(r.RemoteAddr)` must strip ephemeral TCP ports, otherwise every new connection lands in its own bucket and rate limiting will never trigger.
   - **Reverse Proxy Caution:** When running behind a reverse proxy/load balancer (Cloudflare, AWS ALB, NGINX), inspect trusted `X-Forwarded-For` or `CF-Connecting-IP` headers only from verified proxy CIDRs to prevent IP spoofing attacks.
