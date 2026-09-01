# go-rest-api

A **production-quality REST API reference implementation** in Go — built from first principles, one concept at a time.

This project is a living tutorial: every file implements one production pattern (authentication, caching, idempotency, rate limiting, …) and is thoroughly commented explaining **why** each decision was made, not just **what** was coded. The companion `docs/` folder covers the theory for each topic at a principal-engineer depth.

---

## Table of Contents

1. [What This Project Covers](#what-this-project-covers)
2. [Architecture Overview](#architecture-overview)
3. [Libraries and Dependencies](#libraries-and-dependencies)
4. [Quick Start](#quick-start)
5. [Middleware Chain — Order Matters](#middleware-chain--order-matters)
6. [Richardson Maturity & HATEOAS](#richardson-maturity--hateoas)
7. [RFC 7807 Problem Details — Structured Error Responses](#rfc-7807-problem-details--structured-error-responses)
8. [HTTP Caching & Conditional Requests](#http-caching--conditional-requests)
9. [Authentication & Authorization](#authentication--authorization)
10. [Rate Limiting — Token Bucket](#rate-limiting--token-bucket)
11. [Idempotency](#idempotency)
12. [Content Negotiation](#content-negotiation)
13. [Cursor Pagination & Filtering](#cursor-pagination--filtering)
14. [Resilience — Retry & Circuit Breaker](#resilience--retry--circuit-breaker)
15. [Async Jobs — 202 Accepted Pattern](#async-jobs--202-accepted-pattern)
16. [Webhooks with HMAC Signatures](#webhooks-with-hmac-signatures)
17. [Security Headers & CORS](#security-headers--cors)
18. [Observability — Health, Metrics & Structured Logging](#observability--health-metrics--structured-logging)
19. [API Versioning & Deprecation](#api-versioning--deprecation)
20. [OpenAPI 3.1 & Swagger UI](#openapi-31--swagger-ui)
21. [Server-Sent Events (SSE) Streaming](#server-sent-events-sse-streaming)
22. [Optimistic Concurrency Control](#optimistic-concurrency-control)
23. [Project Layout](#project-layout)
24. [Running Tests](#running-tests)

---

## What This Project Covers

| File | Pattern / RFC | Key concept |
|---|---|---|
| `main.go` | Middleware chain, graceful shutdown | Request ID, structured logging, `chi` wiring |
| `handlers.go` | CRUD, SSE, form handling | Full `Order` resource lifecycle |
| `problem.go` | RFC 7807 | Structured error responses with `application/problem+json` |
| `caching.go` | RFC 9111, RFC 9110 | ETags, `If-None-Match`, `If-Match`, `Cache-Control` |
| `hateoas.go` | HAL, Richardson Level 3 | `_links`, state-driven hypermedia |
| `auth.go` | JWT HS256, API Keys, RBAC, Scopes | Authentication + Authorization middleware |
| `ratelimit.go` | Token bucket | Per-client rate limiting with `RateLimit-*` headers |
| `idempotency.go` | Idempotency-Key | Response caching and replay for non-idempotent methods |
| `content_negotiation.go` | RFC 9110 | Accept → JSON / XML / CSV response rendering |
| `pagination.go` | Cursor pagination | Opaque base64 cursors, multi-field sort, sparse fieldsets |
| `resilience.go` | Retry + circuit breaker | Exponential backoff with full jitter, three-state FSM |
| `async_jobs.go` | 202 Accepted | Long-running jobs with `Location` + `Retry-After` polling |
| `webhooks.go` | HMAC-SHA256 | Outbound delivery with retries + inbound signature verification |
| `security.go` | OWASP API Top 10 | Security headers, CORS, body limit, panic recovery |
| `observability.go` | OpenMetrics, `/healthz`, `/readyz` | Concurrent readiness probes, Prometheus-compatible metrics |
| `versioning.go` | RFC 8594, RFC 9745 | `Sunset` + `Deprecation` headers, vendor media-type version |
| `openapi.go` | OpenAPI 3.1 | Embedded spec + Swagger UI CDN |

---

## Architecture Overview

```
                      ┌─────────────────────────────────────────────────────┐
                      │               Global Middleware Chain                │
                      │  SecurityHeaders → CORS → BodyLimit → RequestID      │
                      │  → StructuredLog → PanicRecovery → Metrics → RateLimit│
                      │  → chi.Timeout(10s)                                  │
                      └────────────────────────┬────────────────────────────┘
                                               │
               ┌───────────────────────────────┼──────────────────────────────┐
               │                               │                              │
    ┌──────────▼──────────┐      ┌─────────────▼──────────┐   ┌──────────────▼──────┐
    │   /orders (CRUD)     │      │   /jobs (async)         │   │ /webhooks/incoming   │
    │  List · Get · Create │      │  Create · Poll · Cancel │   │ HMAC-SHA256 verified │
    │  Update · Pay · Cancel│      │  202 Accepted pattern   │   └─────────────────────┘
    │  SSE streaming        │      └────────────────────────┘
    └──────────────────────┘
           │ per-route middleware
           │ Idempotency-Key (POST)
           │ Content-Type validation
           │ AuthMiddleware + RBAC
```

One binary, zero external runtime dependencies — all patterns implemented against the Go standard library with a single routing dependency (`chi`).

---

## Libraries and Dependencies

### Production (`go.mod`)

| Package | Version | Purpose |
|---|---|---|
| `github.com/go-chi/chi/v5` | v5.0.12 | HTTP router with method-scoped middleware, URL parameters, route groups |

**Everything else is the Go standard library:** `net/http`, `crypto/hmac`, `crypto/sha256`, `log/slog`, `encoding/json`, `encoding/xml`, `encoding/csv`, `sync`, `math/rand/v2`.

> **Why chi and not the standard `net/http` mux?**
> Go 1.22 added method-prefixed patterns (`GET /orders/{id}`) and `r.PathValue("id")` to `net/http`,
> closing the biggest gap. Chi is still chosen here because it provides composable per-route middleware
> stacks (`r.With(...).Post(...)`) and a mature `middleware` sub-package (response writer wrapper,
> timeout, etc.) that would otherwise require bespoke code. It adds **zero transitive dependencies**
> and is trivially replaceable.

### Testing (`testing` stdlib only)

No testify, no gomock. All tests use table-driven patterns with `testing.T`.

---

## Quick Start

```bash
git clone https://github.com/netologist/go-rest-api
cd go-rest-api

# Run the server
go run .

# Server starts on :8080
# Swagger UI → http://localhost:8080/docs
# OpenAPI spec → http://localhost:8080/openapi.json
# Health → http://localhost:8080/healthz
# Readiness → http://localhost:8080/readyz
# Metrics → http://localhost:8080/metrics
```

```bash
# Get a JWT
curl -s -X POST http://localhost:8080/api/v1/auth/token \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","roles":["user"]}' | jq .

# Create an order (with idempotency key)
curl -s -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"item":"Widget Pro","amount":29.99,"currency":"USD"}' | jq .

# List orders (cursor pagination)
curl -s "http://localhost:8080/orders?limit=2&sort=-createdAt" | jq .

# Admin endpoint (requires Bearer token + admin role)
curl -s http://localhost:8080/admin/stats \
  -H "X-API-Key: secret_admin_key" | jq .

# Run all tests
go test ./... -v -count=1

# Run with race detector
go test ./... -race -count=1
```

---

## Middleware Chain — Order Matters

The order in which middlewares are registered defines the security and observability envelope of every request. Here is the chain from `main.go` with the rationale for each position:

```go
r.Use(SecurityHeadersMiddleware)               // 1. Applied first — headers set before any handler runs
r.Use(CORSMiddleware(DefaultCORSOptions()))    // 2. CORS — must run before auth so preflight OPTIONS returns early
r.Use(RequestBodyLimitMiddleware(1024 * 1024)) // 3. Body limit — bound I/O before reading body anywhere
r.Use(RequestIDMiddleware)                     // 4. Correlation ID injected into context for all downstream logs
r.Use(StructuredLoggingMiddleware(logger))     // 5. Logs AFTER response write to capture final status + latency
r.Use(ProblemRecoveryMiddleware(logger))       // 6. Panic → RFC 7807 500 — wraps everything below
r.Use(metrics.Middleware)                      // 7. Metrics after recovery so panics are counted
r.Use(rateLimiter.Middleware)                  // 8. Rate limit after ID is known (uses X-API-Key or remote IP)
r.Use(middleware.Timeout(10 * time.Second))    // 9. Context deadline — innermost, closest to business logic
```

> **Why structured logging after body limit but before panic recovery?**
> The logger wraps the `ResponseWriter` to capture the status code written by any middleware or handler below it.
> Panic recovery must be *inside* the logger so the logger can still write a meaningful log entry after a 500 is sent.

---

## Richardson Maturity & HATEOAS

Leonard Richardson's maturity model describes how "RESTful" an HTTP API is across four levels.

| Level | Name | Characteristic |
|---|---|---|
| 0 | Swamp of POX | Single endpoint, HTTP as transport, RPC-style |
| 1 | Resources | Each entity gets its own URI (`/orders/42`) |
| 2 | HTTP Verbs | Correct methods (`GET`, `POST`, `PUT`, `PATCH`, `DELETE`) + status codes |
| 3 | HATEOAS | Responses include links that drive next-state transitions |

This project implements **Level 3** via `hateoas.go`. Each order response includes `_links` describing only the transitions valid in its current state:

```go
// OrderStateLinks generates available state-transition links based on the
// current order status. This implements Richardson Maturity Level 3.
func OrderStateLinks(baseURL string, order Order) Links {
    orderURI := fmt.Sprintf("%s/orders/%s", baseURL, order.ID)
    links := Links{
        "self": Link{Href: orderURI, Method: http.MethodGet, Title: "Get order details"},
    }

    switch order.Status {
    case "pending":
        links["pay"]    = Link{Href: orderURI + "/pay",    Method: http.MethodPost, Title: "Pay for this order"}
        links["cancel"] = Link{Href: orderURI + "/cancel", Method: http.MethodPost, Title: "Cancel this order"}
        links["update"] = Link{Href: orderURI,             Method: http.MethodPut,  Title: "Update this order"}
    case "paid":
        links["cancel"] = Link{Href: orderURI + "/cancel", Method: http.MethodPost, Title: "Cancel and refund"}
    // "cancelled" and "completed" have no further transitions → only "self"
    }
    return links
}
```

A pending order response:

```json
{
  "data": { "id": "ord_abc123", "status": "pending", "amount": 99.00 },
  "_links": {
    "self":   { "href": "/orders/ord_abc123", "method": "GET" },
    "pay":    { "href": "/orders/ord_abc123/pay",    "method": "POST", "title": "Pay for this order" },
    "cancel": { "href": "/orders/ord_abc123/cancel", "method": "POST", "title": "Cancel this order" },
    "update": { "href": "/orders/ord_abc123",        "method": "PUT",  "title": "Update this order" }
  }
}
```

A client navigating the API never needs to know the URL structure — it follows links from the initial discovery resource.

---

## RFC 7807 Problem Details — Structured Error Responses

`problem.go` implements [RFC 7807](https://www.rfc-editor.org/rfc/rfc7807) (soon RFC 9457) — a standard machine-readable error format that replaces free-form `{"error": "something went wrong"}`.

```go
// Problem implements the RFC 7807 Problem Details format.
type Problem struct {
    Type       string       `json:"type"`             // URI identifying the problem class
    Title      string       `json:"title"`            // Human-readable summary (stable, not instance-specific)
    Status     int          `json:"status"`           // HTTP status code (mirrors the response status)
    Detail     string       `json:"detail"`           // Human-readable, instance-specific explanation
    Instance   string       `json:"instance"`         // URI of the specific request (e.g. request ID)
    FieldErrors []FieldError `json:"errors,omitempty"` // Field-level validation failures
}

type FieldError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
    Code    string `json:"code"`
}
```

Convenience helpers cover every standard status:

```go
// 400 Bad Request
BadRequest(w, r, "The 'amount' field must be a positive number.",
    FieldError{Field: "amount", Code: "must_be_positive", Message: "amount must be > 0"})

// 401 Unauthorized — RFC 7235 requires WWW-Authenticate
Unauthorized(w, r, "Bearer token expired.")

// 404 Not Found
NotFound(w, r, "Order ord_abc123 does not exist.")

// 409 Conflict
Conflict(w, r, "An idempotent request with this key is already in flight.")

// 412 Precondition Failed (ETag mismatch)
PreconditionFailed(w, r, "The order was modified since you last fetched it.")

// 422 Unprocessable Entity (semantically invalid)
UnprocessableEntity(w, r, "Cannot pay for a cancelled order.")

// 429 Too Many Requests (always add Retry-After)
TooManyRequests(w, r, 30) // retry after 30 seconds

// 500 Internal Server Error — never leak internals
InternalError(w, r)

// 503 Service Unavailable
ServiceUnavailable(w, r, "Upstream payment gateway is down.", 60)
```

Every error response sets `Content-Type: application/problem+json` and the body is identical in shape:

```json
{
  "type":     "https://api.example.com/errors/bad-request",
  "title":    "Bad Request",
  "status":   400,
  "detail":   "The 'amount' field must be a positive number.",
  "instance": "/orders#req-7a3f8b2c",
  "errors": [
    { "field": "amount", "code": "must_be_positive", "message": "amount must be > 0" }
  ]
}
```

---

## HTTP Caching & Conditional Requests

`caching.go` implements RFC 9111 (HTTP Caching) and RFC 9110 (HTTP Semantics) patterns.

### ETags

An ETag is a fingerprint for a resource version. The server computes it; the client stores it and sends it back on subsequent requests.

```go
// ComputeETag computes a strong ETag based on SHA-256 hash of payload.
// Strong ETags allow byte-range requests; use WeakETag when content may differ
// in insignificant ways (e.g. gzip vs identity encoding of the same body).
func ComputeETag(payload []byte) string {
    h := sha256.Sum256(payload)
    return `"` + hex.EncodeToString(h[:]) + `"`
}

// Order.ETag is a domain-level ETag derived from ID + UpdatedAt, avoiding
// the need to serialize the full body just to compute the fingerprint.
func (o *Order) ETag() string {
    h := sha256.Sum256([]byte(o.ID + o.UpdatedAt.String()))
    return `"` + hex.EncodeToString(h[:8]) + `"`
}
```

### Conditional GET — 304 Not Modified

```go
// GET /orders/{id}
func (h *OrderHandler) Get(w http.ResponseWriter, r *http.Request) {
    order := h.findOrder(id)
    etag := order.ETag()

    // If client sends If-None-Match: "<etag>" and it matches → save bandwidth
    if CheckIfNoneMatch(r, etag) {
        w.WriteHeader(http.StatusNotModified) // 304 — no body
        return
    }

    SetCacheHeaders(w, etag, order.UpdatedAt, CacheDirective{
        Private:        true,   // user-specific resource; do not cache in shared proxies
        MaxAge:         30 * time.Second,
        MustRevalidate: true,
    })
    writeJSON(w, http.StatusOK, order)
}
```

```
Client                              Server
  │  GET /orders/42                   │
  │  If-None-Match: "abc123"  ──────► │  ETag still "abc123"?
  │                           ◄──────  │  304 Not Modified (no body, saves bandwidth)
  │                                   │
  │  GET /orders/42                   │
  │  If-None-Match: "abc123"  ──────► │  Order updated, ETag is now "def456"
  │                           ◄──────  │  200 OK + full body + ETag: "def456"
```

### Optimistic Concurrency Control (If-Match)

For mutating requests (PUT / PATCH), the client must send the current ETag in `If-Match`. If the resource was modified by someone else in between, the server returns **412 Precondition Failed** — preventing lost updates.

```go
// PUT /orders/{id}
func (h *OrderHandler) Update(w http.ResponseWriter, r *http.Request) {
    order := h.findOrder(id)
    currentETag := order.ETag()

    // Reject if client doesn't own the latest version
    if !CheckIfMatch(r, currentETag) {
        PreconditionFailed(w, r,
            "The order has been modified since your last fetch. Re-fetch and try again.")
        return
    }

    // Safe to apply the update now
    applyUpdate(order, req)
    writeJSON(w, http.StatusOK, order)
}
```

### Cache-Control Directive Builder

```go
CacheDirective{
    Public:         true,
    MaxAge:         5 * time.Minute,
    SMaxAge:        1 * time.Minute,  // CDN TTL separate from browser TTL
    MustRevalidate: true,
}.String()
// → "public, max-age=300, s-maxage=60, must-revalidate"

CacheDirective{NoStore: true}.String()
// → "no-store"  (e.g. for auth endpoints)
```

---

## Authentication & Authorization

`auth.go` implements three authentication mechanisms in a single middleware stack.

### JWT (HS256) — Bearer tokens

```go
// JWTClaims holds standard + custom fields
type JWTClaims struct {
    Issuer    string   `json:"iss"`
    Subject   string   `json:"sub"`
    Email     string   `json:"email"`
    Roles     []string `json:"roles"`
    Scopes    []string `json:"scopes"`
    ExpiresAt int64    `json:"exp"`
    IssuedAt  int64    `json:"iat"`
}

// GenerateToken creates a signed HS256 JWT.
// Manual implementation (no third-party JWT library) so you can see exactly
// what the header, payload, and signature look like.
func (j *JWTAuthenticator) GenerateToken(claims JWTClaims) (string, error) {
    header := base64url(json.Marshal(map[string]string{"alg":"HS256","typ":"JWT"}))
    payload := base64url(json.Marshal(claims))
    signingInput := header + "." + payload
    sig := hmac.New(sha256.New, j.secretKey)
    sig.Write([]byte(signingInput))
    return signingInput + "." + base64url(sig.Sum(nil)), nil
}
```

### API Keys — `X-API-Key` header

```go
keyStore := NewAPIKeyStore(map[string]*AuthContext{
    "secret_admin_key": {
        Subject: "user_admin", Roles: []string{"admin", "user"},
        Scopes: []string{"orders:read", "orders:write", "admin"},
    },
    "secret_user_key": {
        Subject: "user_standard", Roles: []string{"user"},
        Scopes: []string{"orders:read"},
    },
})
```

### AuthMiddleware — unified inspection

```go
// AuthMiddleware checks Authorization: Bearer <jwt> first,
// then falls back to X-API-Key header.
// When optional=true, unauthenticated requests pass through
// (anonymous access allowed).
func AuthMiddleware(jwtAuth *JWTAuthenticator, keyStore *APIKeyStore, optional bool) func(http.Handler) http.Handler
```

### RBAC — Role-Based Access Control

```go
// Protects /admin/stats — requires the "admin" role
r.With(
    AuthMiddleware(jwtAuth, keyStore, false),
    RequireRoles("admin"),
).Get("/admin/stats", adminHandler)
```

### OAuth2 Scopes — fine-grained authorization

```go
// Requires the orders:write scope
r.With(
    AuthMiddleware(jwtAuth, keyStore, false),
    RequireScopes("orders:write"),
).Post("/orders", orderHandler.Create)
```

### Getting a token

```bash
curl -s -X POST http://localhost:8080/api/v1/auth/token \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","roles":["admin","user"]}'

# Response:
{
  "tokenType":   "Bearer",
  "accessToken": "eyJhbGc...",
  "expiresIn":   3600
}
```

---

## Rate Limiting — Token Bucket

`ratelimit.go` implements the token bucket algorithm: tokens refill at a constant rate up to a capacity, and each request consumes one. This allows short bursts while enforcing a long-term average rate.

```
 capacity = 100 tokens
 refill   = 10 tokens/second

 Timeline:
 t=0s   ████████████████████ 100 tokens (full)
 t=0s   50 requests arrive → 50 tokens consumed
 t=0s   ████████████         50 tokens remaining → all 50 pass
 t=1s   ██████████████       +10 refilled → 60 tokens
 t=1s   80 requests arrive → only 60 pass, 20 get 429
```

```go
type TokenBucket struct {
    mu         sync.Mutex
    tokens     float64
    capacity   float64
    refillRate float64    // tokens added per second
    lastRefill time.Time
}

// Allow reports whether a request may proceed and how long to wait if not.
func (b *TokenBucket) Allow() (bool, time.Duration) {
    b.mu.Lock()
    defer b.mu.Unlock()

    now := time.Now()
    elapsed := now.Sub(b.lastRefill).Seconds()
    b.tokens = minFloat(b.capacity, b.tokens + elapsed*b.refillRate)
    b.lastRefill = now

    if b.tokens >= 1 {
        b.tokens--
        return true, 0
    }
    wait := time.Duration((1-b.tokens)/b.refillRate * float64(time.Second))
    return false, wait
}
```

Per-client buckets are keyed by `X-API-Key` header (if present) or remote IP as a fallback:

```go
// RateLimiterStore keeps a separate token bucket per client.
type RateLimiterStore struct {
    mu      sync.Mutex
    buckets map[string]*TokenBucket
    cap     float64
    refill  float64
}
```

On success, the middleware sets standard rate-limit headers:

```
RateLimit-Limit:     100
RateLimit-Remaining: 73
```

On failure (429):

```
Retry-After: 3
Content-Type: application/problem+json
```

---

## Idempotency

`idempotency.go` ensures that POST or PATCH requests with the same `Idempotency-Key` header return the **exact same response** as the first successful call — without executing the handler a second time.

### Why this matters

A client places an order. The network times out before receiving the response. The client retries with the same key. Without idempotency, the order is created twice. With it, the second call returns the cached `201 Created` response instantly.

### How it works

```go
// Middleware intercepts at the request boundary:
// 1. Read Idempotency-Key header (required for POST/PATCH)
// 2. Hash request body with SHA-256 to detect body mismatches
// 3. If key seen before: replay cached status + headers + body
// 4. If key in-flight: return 409 Conflict
// 5. If key is new: mark in-flight, run handler, cache result, clear in-flight
func (s *IdempotencyStore) Middleware(next http.Handler) http.Handler
```

The response recorder captures the downstream handler's output:

```go
type responseRecorder struct {
    http.ResponseWriter
    statusCode int
    body       bytes.Buffer
}
```

### Key mismatch detection

If a client sends the same `Idempotency-Key` with a **different body**, it's a programming error. The middleware hashes the request body and rejects reuse with a different hash:

```go
requestHash := sha256hex(body)
if record.requestHash != requestHash {
    Conflict(w, r, "Idempotency key reused with a different request body.")
    return
}
```

### Usage

```bash
KEY=$(uuidgen)

# First call — executes the handler, caches result
curl -X POST http://localhost:8080/orders \
  -H "Idempotency-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d '{"item":"Widget","amount":9.99}' 
# → 201 Created, order_id: "ord_abc"

# Second call — replays exact same response (no DB write)
curl -X POST http://localhost:8080/orders \
  -H "Idempotency-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d '{"item":"Widget","amount":9.99}'
# → 201 Created, order_id: "ord_abc"  ← same response
```

---

## Content Negotiation

`content_negotiation.go` implements RFC 9110 §12 — the client declares what it can handle via `Accept`, the server picks the best match.

### Supported types

```go
const (
    MediaTypeJSON     = "application/json"
    MediaTypeXML      = "application/xml"
    MediaTypeCSV      = "text/csv"
    MediaTypeProblemJSON = "application/problem+json"
)
```

### Accept header parsing with quality factors

```
Accept: text/csv;q=0.8, application/xml;q=0.9, application/json
```

Parsed in descending quality order: JSON (1.0) → XML (0.9) → CSV (0.8).

```go
// ParseAcceptHeader parses the RFC 9110 Accept header.
// Returns specs ordered by quality factor (descending).
func ParseAcceptHeader(header string) []AcceptSpec

// NegotiateContentType picks the best server type matching client preferences.
// Returns "" when no match (caller should respond 406 Not Acceptable).
func NegotiateContentType(r *http.Request, supported []string) string
```

### Response rendering

```go
// RenderResponse writes data in the format negotiated via Accept header.
// Falls back to JSON when Accept is absent or wildcard (*/*).
func RenderResponse(w http.ResponseWriter, r *http.Request, status int, data any) {
    chosen := NegotiateContentType(r, []string{MediaTypeJSON, MediaTypeXML, MediaTypeCSV})
    switch chosen {
    case MediaTypeXML:
        w.Header().Set("Content-Type", MediaTypeXML)
        xml.NewEncoder(w).Encode(data)
    case MediaTypeCSV:
        renderCSV(w, data)
    default: // JSON fallback
        writeJSON(w, status, data)
    }
}
```

### Vendor media-type versioning

```
Accept: application/vnd.example.v2+json
```

```go
// VersionFromVendorMediaType extracts API version from vendor media type.
// "application/vnd.example.v2+json" → "v2"
func VersionFromVendorMediaType(acceptHeader string) string
```

### Content-Type validation middleware

```go
// Reject POST /orders with wrong Content-Type
orders.With(
    ValidateContentTypeMiddleware(MediaTypeJSON),
).Post("/", orderHandler.Create)

// Also supports form submissions
orders.With(
    ValidateContentTypeMiddleware("application/x-www-form-urlencoded"),
).Post("/form", orderHandler.CreateFromForm)
```

---

## Cursor Pagination & Filtering

`pagination.go` implements cursor-based pagination — the production-correct replacement for `LIMIT/OFFSET`.

### Why cursor pagination, not offset?

| | Offset | Cursor |
|---|---|---|
| Performance | Degrades with `OFFSET N` scans | O(1) seek to cursor position |
| Consistency | Items shift when inserting while paginating | Stable — new items don't shift pages |
| Parallel safety | Page N may have duplicates or gaps | Guaranteed no duplicates, no gaps |

### Opaque cursor token

The cursor is a base64-encoded JSON payload — opaque to the client, containing the sort key values needed for the next seek:

```go
type CursorPayload struct {
    ID        string    `json:"id"`
    CreatedAt time.Time `json:"createdAt"`
    Direction string    `json:"direction"` // "next" or "prev"
}

// EncodeCursor converts cursor state into an opaque base64 token.
func EncodeCursor(id string, createdAt time.Time, direction string) string {
    b, _ := json.Marshal(CursorPayload{ID: id, CreatedAt: createdAt, Direction: direction})
    return base64.URLEncoding.EncodeToString(b)
}
```

### Multi-field sort

```
GET /orders?sort=-createdAt,amount
```

```go
// ParseSortParam parses comma-separated sort strings like "-createdAt,amount,+status".
// "-" prefix = DESC, "+" or no prefix = ASC.
// Validates against allowedFields to prevent injection.
func ParseSortParam(sortStr string, allowedFields []string) ([]SortField, error)
```

### Sparse fieldsets

```
GET /orders?fields=id,amount,status
```

```go
// ApplySparseFieldset filters a struct or map to only the requested fields.
// Uses reflection to handle struct inputs, maps for pre-filtered data.
// If requestedFields is empty, the original item is returned unmodified.
func ApplySparseFieldset(item any, requestedFields []string) any
```

### Collection response shape

```json
{
  "_embedded": [
    { "id": "ord_1", "amount": 49.99, "status": "paid" },
    { "id": "ord_2", "amount": 19.99, "status": "pending" }
  ],
  "_links": {
    "self":  { "href": "/orders?limit=2&sort=-createdAt" },
    "next":  { "href": "/orders?limit=2&cursor=eyJpZ...&sort=-createdAt" },
    "first": { "href": "/orders?limit=2&sort=-createdAt" }
  },
  "meta": {
    "limit":   2,
    "sort":    "-createdAt",
    "hasMore": true
  }
}
```

---

## Resilience — Retry & Circuit Breaker

`resilience.go` implements two patterns for calling downstream services.

### Exponential backoff with full jitter

Full jitter randomises the wait so that a thundering-herd of retrying clients doesn't hit the recovering downstream simultaneously:

```go
// RetryWithBackoff retries fn using exponential backoff with full jitter.
// Only errors wrapping ErrRetryable are retried — validation errors and
// business errors are returned immediately to avoid duplicate side effects.
func RetryWithBackoff(ctx context.Context, maxAttempts int, baseDelay time.Duration, fn func() error) error {
    for attempt := range maxAttempts {
        err := fn()
        if err == nil {
            return nil
        }
        if !errors.Is(err, ErrRetryable) {
            return err   // non-retryable; fail fast
        }
        // Full jitter: sleep in [0, baseDelay * 2^attempt]
        cap := float64(baseDelay) * math.Pow(2, float64(attempt))
        sleep := time.Duration(rand.Float64() * cap)
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(sleep):
        }
    }
    return lastErr
}
```

### Circuit breaker — three-state FSM

```
  ┌─────────────────────────────────────────────────────┐
  │                                                     │
  │   Closed ──(N failures)──► Open ──(reset timeout)──► Half-Open
  │     ▲                                                    │
  │     └──────────────── (success) ──────────────────────── ┘
  │
  │   Closed:    all calls pass through
  │   Open:      all calls rejected immediately (fail fast)
  │   Half-Open: one probe call allowed; success → Closed, failure → Open
  └─────────────────────────────────────────────────────┘
```

```go
type CircuitBreaker struct {
    mu               sync.Mutex
    state            CircuitState    // StateClosed, StateOpen, StateHalfOpen
    failures         int
    failureThreshold int
    lastFailure      time.Time
    resetTimeout     time.Duration
}

// Execute runs fn if the circuit allows it.
func (cb *CircuitBreaker) Execute(fn func() error) error {
    cb.mu.Lock()
    // State transitions
    if cb.state == StateOpen {
        if time.Since(cb.lastFailure) > cb.resetTimeout {
            cb.state = StateHalfOpen
        } else {
            cb.mu.Unlock()
            return ErrCircuitOpen  // fail fast — don't call downstream
        }
    }
    cb.mu.Unlock()

    err := fn()

    cb.mu.Lock()
    defer cb.mu.Unlock()
    if err != nil {
        cb.failures++
        cb.lastFailure = time.Now()
        if cb.failures >= cb.failureThreshold || cb.state == StateHalfOpen {
            cb.state = StateOpen
        }
    } else {
        cb.failures = 0
        cb.state = StateClosed
    }
    return err
}
```

Usage in the order handler — the payment service call is wrapped in the circuit breaker:

```go
err := h.breaker.Execute(func() error {
    return callPaymentService(r.Context(), order.Amount, order.Currency)
})
if errors.Is(err, ErrCircuitOpen) {
    ServiceUnavailable(w, r, "Payment service is temporarily unavailable.", 30)
    return
}
```

---

## Async Jobs — 202 Accepted Pattern

`async_jobs.go` implements the "fire and poll" async pattern for long-running operations.

### The pattern

```
Client                          Server
  │  POST /jobs/export            │
  │  ──────────────────────────►  │  202 Accepted
  │  ◄──────────────────────────  │  Location: /jobs/job_abc
  │                               │  Retry-After: 5
  │  (wait 5 seconds)             │
  │                               │
  │  GET /jobs/job_abc            │
  │  ──────────────────────────►  │  200 OK
  │  ◄──────────────────────────  │  {"status":"running","progress":45}
  │                               │
  │  GET /jobs/job_abc            │
  │  ──────────────────────────►  │  200 OK  (or 303 See Other)
  │  ◄──────────────────────────  │  {"status":"completed","resultUrl":"..."}
```

### Job lifecycle

```go
type JobStatus string

const (
    JobPending   JobStatus = "pending"
    JobRunning   JobStatus = "running"
    JobCompleted JobStatus = "completed"
    JobFailed    JobStatus = "failed"
    JobCancelled JobStatus = "cancelled"
)

type Job struct {
    ID        string         `json:"id"`
    Type      string         `json:"type"`
    Status    JobStatus      `json:"status"`
    Progress  int            `json:"progress"`          // 0–100
    ResultURL string         `json:"resultUrl,omitempty"`
    Error     string         `json:"error,omitempty"`
    Metadata  map[string]any `json:"metadata,omitempty"`
    CreatedAt time.Time      `json:"createdAt"`
    UpdatedAt time.Time      `json:"updatedAt"`
}
```

### Create endpoint

```go
// POST /jobs/export → 202 Accepted
func (h *JobHandler) CreateExportJob(w http.ResponseWriter, r *http.Request) {
    job := h.store.Create("export", metadata)
    go runExportInBackground(h.store, job.ID) // fire and forget

    w.Header().Set("Location", "/jobs/"+job.ID)
    w.Header().Set("Retry-After", "5")
    writeJSON(w, http.StatusAccepted, job) // 202 — not 200, not 201
}
```

### Worker pool

```go
// RunBackgroundWorkerPool starts a bounded pool of goroutine workers.
// Prevents unbounded goroutine spawning under heavy load.
func RunBackgroundWorkerPool(ctx context.Context, numWorkers int, tasks <-chan func()) {
    for range numWorkers {
        go func() {
            for {
                select {
                case fn, ok := <-tasks:
                    if !ok { return }
                    fn()
                case <-ctx.Done():
                    return
                }
            }
        }()
    }
}
```

---

## Webhooks with HMAC Signatures

`webhooks.go` implements both outbound delivery and inbound verification.

### Outbound — signed delivery

Every outbound event is signed with HMAC-SHA256 over `"{timestamp}.{payload}"`:

```go
// ComputeWebhookSignature creates: HMAC-SHA256("{timestamp}.{payload}", secret)
func ComputeWebhookSignature(payload []byte, secret []byte, timestamp int64) string {
    mac := hmac.New(sha256.New, secret)
    mac.Write([]byte(fmt.Sprintf("%d.", timestamp)))
    mac.Write(payload)
    return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
```

Outgoing `POST` to subscriber URL includes headers:

```
X-Webhook-Timestamp: 1718000000
X-Webhook-Signature: sha256=3f7ab...
Content-Type: application/json
```

The `WebhookSender.Deliver` retries on transient errors with exponential backoff.

### Inbound — verification middleware

```go
// WebhookVerificationMiddleware validates inbound webhooks.
// Rejects payloads with:
//   - Missing or invalid signature
//   - Timestamp outside the tolerance window (replay attack prevention)
func WebhookVerificationMiddleware(secret []byte, tolerance time.Duration) func(http.Handler) http.Handler

// Replay attack prevention: tolerance = 5 minutes
r.With(
    WebhookVerificationMiddleware(webhookSecret, 5*time.Minute),
).Post("/webhooks/incoming", inboundHandler)
```

The tolerance window prevents replay attacks: a captured valid request can only be replayed within 5 minutes of the original timestamp.

---

## Security Headers & CORS

`security.go` implements OWASP-recommended security headers and CORS handling.

### Security headers

```go
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        h := w.Header()
        // Prevent clickjacking
        h.Set("X-Frame-Options", "DENY")
        // Disable browser MIME sniffing
        h.Set("X-Content-Type-Options", "nosniff")
        // Force HTTPS for 2 years
        h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
        // Restrictive CSP for API responses (no script/style needed)
        h.Set("Content-Security-Policy", "default-src 'none'")
        // No referrer on cross-origin requests
        h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
        // Disable sensitive browser features
        h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
        next.ServeHTTP(w, r)
    })
}
```

### CORS

```go
// DefaultCORSOptions returns production-ready CORS defaults.
func DefaultCORSOptions() CORSOptions {
    return CORSOptions{
        AllowedOrigins:   []string{"https://example.com", "https://www.example.com"},
        AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
        AllowedHeaders:   []string{"Authorization", "Content-Type", "Idempotency-Key", "X-API-Key"},
        ExposedHeaders:   []string{"X-Request-Id", "RateLimit-Limit", "RateLimit-Remaining"},
        AllowCredentials: false, // Never true with AllowedOrigins: ["*"]
        MaxAgeSeconds:    86400, // Preflight cache: 24h
    }
}
```

CORS middleware handles `OPTIONS` preflight requests immediately — no auth, no rate limiting applied to preflight.

### Body size limit

```go
// RequestBodyLimitMiddleware wraps r.Body with an io.LimitedReader.
// Prevents DoS via oversized payloads without buffering the whole body in memory.
func RequestBodyLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
            next.ServeHTTP(w, r)
        })
    }
}
```

### Panic recovery → RFC 7807

```go
// ProblemRecoveryMiddleware catches unexpected panics in any handler.
// Logs the full stack trace (server-side only) and returns a safe
// RFC 7807 Internal Server Error to the client — no stack trace leaked.
func ProblemRecoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler
```

---

## Observability — Health, Metrics & Structured Logging

`observability.go` implements three observability signals.

### Structured logging with `log/slog`

Go 1.21+ `log/slog` provides zero-allocation structured JSON logging. Every request generates one log line:

```go
logger.Info("http_request",
    "requestId", GetRequestID(r.Context()),
    "method",    r.Method,
    "path",      r.URL.Path,
    "status",    ww.Status(),
    "latencyMs", time.Since(start).Milliseconds(),
)
```

```json
{
  "time":      "2025-09-01T12:34:56.789Z",
  "level":     "INFO",
  "msg":       "http_request",
  "requestId": "a3f8b2c1",
  "method":    "POST",
  "path":      "/orders",
  "status":    201,
  "latencyMs": 12
}
```

### `/healthz` — liveness probe

```
GET /healthz → 200 {"status":"alive"}
```

Always returns 200 as long as the process is running. Used by Kubernetes liveness probes.

### `/readyz` — readiness probe

```go
// ReadinessChecker runs registered probes concurrently with a timeout.
type ReadinessChecker struct {
    probes  map[string]ProbeFunc
    version string
}

checker.RegisterProbe("database", func(ctx context.Context) error {
    return db.PingContext(ctx)
})
checker.RegisterProbe("payment_gateway", func(ctx context.Context) error {
    return pingPaymentGateway(ctx)
})
```

All probes run concurrently. If any probe fails, `/readyz` returns 503:

```json
{
  "status":  "degraded",
  "version": "1.0.0",
  "checks": {
    "database":        { "status": "healthy" },
    "payment_gateway": { "status": "unhealthy", "error": "connection refused" }
  }
}
```

### `/metrics` — Prometheus-compatible

```go
// APIMetrics tracks per-endpoint request counts and latency buckets.
// /metrics returns OpenMetrics format — compatible with Prometheus scraping.
func (m *APIMetrics) Handler() http.HandlerFunc
```

```
# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET",path="/orders",status="200"} 42

# HELP http_request_duration_ms HTTP request latency
# TYPE http_request_duration_ms histogram
http_request_duration_ms_bucket{le="10"} 38
http_request_duration_ms_bucket{le="50"} 41
http_request_duration_ms_bucket{le="+Inf"} 42
```

### Graceful shutdown

```go
// GracefulShutdown blocks until SIGINT or SIGTERM, then initiates a
// graceful shutdown with a configurable drain timeout.
func GracefulShutdown(srv *http.Server, logger *slog.Logger, shutdownTimeout time.Duration) {
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
    defer cancel()
    srv.Shutdown(ctx)
}
```

---

## API Versioning & Deprecation

`versioning.go` implements versioning via headers and vendor media types, plus RFC-compliant deprecation signalling.

### Versioning strategies

| Strategy | Example | Tradeoffs |
|---|---|---|
| URL path | `/api/v2/orders` | Simple, cache-friendly, visible in logs |
| Query param | `/orders?version=2` | Easy to test, not RESTful |
| Header | `X-API-Version: v2` | Clean URLs, harder to cache/test |
| Vendor media type | `Accept: application/vnd.example.v2+json` | Most RESTful, rarely used in practice |

This project implements header + vendor media type:

```go
// HeaderVersioningMiddleware extracts version from:
// 1. X-API-Version header
// 2. Accept-Version header
// 3. Vendor media type: application/vnd.example.v2+json
// Falls back to defaultVersion when none provided.
func HeaderVersioningMiddleware(defaultVersion string) func(http.Handler) http.Handler
```

### Deprecation headers (RFC 8594 + RFC 9745)

When an endpoint is deprecated, clients receive machine-readable headers in every response:

```go
r.With(DeprecationMiddleware(DeprecationConfig{
    DeprecationDate:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
    SunsetDate:       time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
    SuccessorVersion: "/orders",
    DocumentationURL: "https://api.example.com/docs/migrations/v1-to-v2",
})).Get("/legacy-orders", orderHandler.List)
```

Response headers:

```
Deprecation: @1735689600
Sunset:      Wed, 01 Jan 2027 00:00:00 GMT
Link: </orders>; rel="successor-version"
Link: <https://api.example.com/docs/migrations/v1-to-v2>; rel="deprecation"; type="text/html"
```

SDKs and API gateways can parse these headers to warn developers automatically.

---

## OpenAPI 3.1 & Swagger UI

`openapi.go` embeds the OpenAPI 3.1 specification as a Go string constant and serves both the raw JSON and an interactive Swagger UI:

```
GET /openapi.json  → OpenAPI 3.1 JSON document
GET /docs          → Swagger UI (loads spec from /openapi.json)
```

The Swagger UI is loaded from the unpkg CDN — no build step required:

```html
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
```

> **API-first design:** the OpenAPI spec is the contract. Write it first, implement second. The spec in `openapi.go` describes request/response shapes, authentication schemes, error responses, and example values — it is the source of truth for client SDK generation and integration tests.

---

## Server-Sent Events (SSE) Streaming

The `Events` handler on `GET /orders/{id}/events` demonstrates real-time streaming:

```go
// Events handles GET /orders/{id}/events — SSE streaming.
func (h *OrderHandler) Events(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    flusher, ok := w.(http.Flusher)
    if !ok {
        InternalError(w, r)
        return
    }

    ticker := time.NewTicker(2 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-r.Context().Done():
            return   // client disconnected — clean exit
        case t := <-ticker.C:
            fmt.Fprintf(w, "event: heartbeat\ndata: {\"ts\":%d}\n\n", t.Unix())
            flusher.Flush()
        }
    }
}
```

SSE is the correct choice for server-push scenarios where the client doesn't need to send data (order status updates, live pricing, job progress) — simpler than WebSocket, natively supported by browsers, works over HTTP/1.1.

---

## Optimistic Concurrency Control

PUT and PATCH on `/orders/{id}` require an `If-Match` header. This prevents the "lost update" problem when two clients modify the same resource concurrently:

```
Client A                         Client B                    Server
  │  GET /orders/42               │                            │
  │ ◄── ETag: "v3" ──────────────────────────────────────── ─ │
  │                               │  GET /orders/42            │
  │                               │ ◄── ETag: "v3" ─────────── │
  │  PUT /orders/42               │                            │
  │  If-Match: "v3"  ──────────────────────────────────────► │
  │ ◄── 200 OK, ETag: "v4" ─────────────────────────────────  │  (A's write succeeds, version → v4)
  │                               │                            │
  │                               │  PUT /orders/42            │
  │                               │  If-Match: "v3"  ─────────►│
  │                               │ ◄── 412 Precondition Failed │  (B's stale ETag rejected)
```

```go
if !CheckIfMatch(r, order.ETag()) {
    PreconditionFailed(w, r,
        "The order was modified since your last fetch. Re-fetch and retry.")
    return
}
```

---

## Project Layout

```
go-rest-api/
├── main.go                       Entry point: middleware chain, route wiring, graceful shutdown
├── handlers.go                   Order resource: List, Get, Create, Update, Pay, Cancel, Events
├── problem.go                    RFC 7807 structured errors for every HTTP status
├── caching.go                    ETags, Cache-Control, If-None-Match, If-Match (OCC)
├── hateoas.go                    HAL _links, Richardson Level 3, state-driven navigation
├── auth.go                       JWT HS256, API Keys, AuthMiddleware, RBAC, OAuth2 scopes
├── ratelimit.go                  Token bucket per-client rate limiter
├── idempotency.go                Idempotency-Key middleware with request-hash dedup
├── content_negotiation.go        Accept → JSON/XML/CSV negotiation, vendor media type
├── pagination.go                 Cursor pagination, multi-field sort, sparse fieldsets
├── resilience.go                 Retry with full-jitter backoff, three-state circuit breaker
├── async_jobs.go                 202 Accepted pattern, job store, worker pool
├── webhooks.go                   HMAC-SHA256 outbound signing + inbound verification middleware
├── security.go                   OWASP security headers, CORS, body limit, panic recovery
├── observability.go              /healthz, /readyz (concurrent probes), /metrics, graceful shutdown
├── versioning.go                 Deprecation (RFC 8594/RFC 9745) + header versioning
├── openapi.go                    OpenAPI 3.1 spec (embedded) + Swagger UI
├── *_test.go                     Table-driven tests for every module
├── docs/
│   ├── 01-richardson-maturity-and-hateoas.md
│   ├── 02-authentication-and-authorization.md
│   ├── 03-http-caching-and-conditional-requests.md
│   ├── 04-content-negotiation.md
│   ├── 05-pagination-filtering-and-sorting.md
│   ├── 06-idempotency.md
│   ├── 07-rate-limiting-and-throttling.md
│   ├── 08-resilience-and-circuit-breaker.md
│   ├── 09-error-handling-and-problem-details.md
│   ├── 10-asynchronous-jobs-and-batching.md
│   ├── 11-security-and-hardening.md
│   ├── 12-webhooks-and-event-delivery.md
│   ├── 13-observability-metrics-and-healthchecks.md
│   ├── 14-api-versioning-and-deprecation.md
│   └── 15-openapi-and-contract-first-design.md
├── go.mod
└── go.sum
```

---

## Running Tests

```bash
# All tests
go test ./... -v -count=1

# With race detector (mandatory before any commit)
go test ./... -race -count=1

# Single module
go test -run TestTokenBucket -v

# Coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### What the tests cover

| File | Test count | What's verified |
|---|---|---|
| `handlers_test.go` | 8 | CRUD lifecycle, 404, state transitions, SSE |
| `auth_test.go` | 7 | JWT round-trip, expiry, tamper detection, RBAC, scope |
| `caching_test.go` | 6 | ETag compute, If-None-Match 304, If-Match 412, Cache-Control serialization |
| `pagination_test.go` | 5 | Cursor encode/decode, sort parsing, sparse fieldsets |
| `ratelimit_test.go` | 4 | Allow/deny, refill, per-client isolation |
| `idempotency_test.go` | 5 | Replay, in-flight 409, body-mismatch 409, TTL expiry |
| `content_negotiation_test.go` | 5 | Accept parsing, quality ordering, 406, vendor type |
| `resilience_test.go` | 5 | Retry on retryable errors, skip on non-retryable, circuit transitions |
| `async_jobs_test.go` | 5 | 202 + Location header, poll, cancel, state machine |
| `webhooks_test.go` | 4 | Signature compute, verify, replay attack, tamper detect |
| `problem_test.go` | 6 | Status code, Content-Type, field errors, instance URI |
| `observability_test.go` | 4 | Healthz 200, readyz healthy/degraded, metrics counter |
| `hateoas_test.go` | 4 | Links per state, HAL wrapping, collection links |
| `versioning_test.go` | 3 | Deprecation headers, Sunset, Link rel |
| `main_test.go` | 3 | Full integration: router wiring, request ID, middleware chain |
| `security_test.go` | 4 | Security headers present, CORS preflight, body limit |

---

## Key Design Decisions

### 1. Zero external production dependencies (except `chi`)

Every pattern — JWT, HMAC, ETags, token bucket, circuit breaker — is implemented against the Go standard library. This makes the code maximally readable as a reference: no "magic" from a framework hides what's actually happening.

### 2. Single `package main`, single binary

All modules live in one package to eliminate cross-package import complexity. A production service would split `auth`, `ratelimit`, `idempotency`, etc. into separate packages, but for a reference/tutorial project the flat layout makes every file instantly navigable.

### 3. RFC-first comments

Every public function has a comment citing the relevant RFC. ETags reference RFC 9110; Cache-Control references RFC 9111; problem details reference RFC 7807; webhooks reference RFC 9421 conventions; versioning references RFC 8594 and RFC 9745. This makes the code self-documenting for engineers who need to look up the spec.

### 4. No panics in handler code

Every error path returns an appropriate RFC 7807 problem detail. `ProblemRecoveryMiddleware` exists as a backstop for truly unexpected panics in third-party code, not as an excuse to panic in business logic.

### 5. Concurrency safety everywhere

All shared state (`RateLimiterStore`, `IdempotencyStore`, `JobStore`, `CircuitBreaker`) uses `sync.Mutex`. Every `*_test.go` for concurrent components runs with `-race` in CI.
