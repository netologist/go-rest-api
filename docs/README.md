# Enterprise REST API Design Patterns & Architecture Blueprint

Welcome to the comprehensive documentation of production-grade REST API patterns implemented in Go. This repository serves as a reference architecture and implementation guide for building scalable, resilient, secure, and observable RESTful APIs at a Principal / Staff Engineer level.

---

## Pattern Documentation Index

Each architectural pattern has its own dedicated in-depth guide with specifications, real-world trade-offs, security implications, and Go code walkthroughs:

| # | Pattern Guide | Core RFCs & Standards | Summary |
|---|---|---|---|
| **01** | [Richardson Maturity & HATEOAS](./01-richardson-maturity-and-hateoas.md) | RFC 8288, HAL | Levels 0–3, Hypermedia as the Engine of Application State, dynamic state transitions. |
| **02** | [Authentication & Authorization](./02-authentication-and-authorization.md) | RFC 7519, RFC 6749, RFC 7235 | JWT Bearer validation, API keys, RBAC, OAuth2 Scopes, 401 vs 403. |
| **03** | [HTTP Caching & Conditional Requests](./03-http-caching-and-conditional-requests.md) | RFC 9111, RFC 9110, RFC 7232 | Strong/Weak ETags, 304 Not Modified, Optimistic Concurrency Control (412 Precondition Failed), Cache-Control. |
| **04** | [Content Negotiation & Media Types](./04-content-negotiation.md) | RFC 9110 | `Accept` header quality factors (`q`), JSON/XML/CSV rendering, 406 Not Acceptable, 415 Unsupported Media Type, Vendor media types. |
| **05** | [Pagination, Filtering & Sorting](./05-pagination-filtering-and-sorting.md) | Standard Best Practices | Opaque Cursor-based pagination, multi-field sorting (`?sort=-createdAt,amount`), sparse fieldsets (`?fields=`), keyset vs offset trade-offs. |
| **06** | [Idempotency Pattern](./06-idempotency.md) | IETF Idempotency-Key Draft | Safe retries for POST/PATCH, SHA-256 payload hashing, in-flight locks (409), replaying responses, key reuse protection. |
| **07** | [Rate Limiting & Throttling](./07-rate-limiting-and-throttling.md) | IETF RateLimit Headers | Token Bucket algorithm, per-client isolation, 429 Too Many Requests, `Retry-After`, `RateLimit-Limit/Remaining`. |
| **08** | [Resilience & Circuit Breaker](./08-resilience-and-circuit-breaker.md) | Distributed Patterns | Exponential backoff with full jitter, `ErrRetryable` classification, Circuit Breaker state machine (Closed, Open, Half-Open). |
| **09** | [Error Handling & Problem Details](./09-error-handling-and-problem-details.md) | RFC 7807 / RFC 9457 | `application/problem+json`, structured field-level errors, standard status code mappings, security hardening against stack leaks. |
| **10** | [Asynchronous Long-Running Jobs](./10-asynchronous-jobs-and-batching.md) | RFC 7240 | 202 Accepted pattern, `Location` polling header, `Retry-After`, background worker pools, job cancellation. |
| **11** | [Security & API Hardening](./11-security-and-hardening.md) | OWASP API Security Top 10 | Security Headers (HSTS, CSP, XFO, nosniff), CORS preflight handling, `MaxBytesReader` (413 Payload Too Large), Panic recovery. |
| **12** | [Webhooks & Event Signatures](./12-webhooks-and-event-delivery.md) | HMAC-SHA256 Standard | Outbound event delivery with exponential backoff, HMAC-SHA256 signatures, timestamp replay attack defense, inbound verification. |
| **13** | [Observability, Health & Metrics](./13-observability-metrics-and-healthchecks.md) | Cloud-Native Standards | Liveness (`/healthz`), Readiness (`/readyz`) with concurrent dependency probes, Prometheus `/metrics`, `X-Request-Id` correlation, graceful shutdown. |
| **14** | [API Versioning & Deprecation](./14-api-versioning-and-deprecation.md) | RFC 8594, RFC 9745 | URI/Header/Vendor versioning, `Sunset` header, `Deprecation` header, `Link: <url>; rel="successor-version"`. |
| **15** | [OpenAPI 3.1 & Contract Testing](./15-openapi-and-contract-first-design.md) | OpenAPI 3.1.0 | Interactive Swagger UI (`/docs`), OpenAPI JSON specification (`/openapi.json`), API-first lifecycle, consumer-driven contracts. |

---

## Quick Start

### Running the API Server

```bash
go run .
```

The server will start on `http://localhost:8080` with the following active endpoints:

- **Interactive API Documentation:** `http://localhost:8080/docs`
- **OpenAPI 3.1 JSON Specification:** `http://localhost:8080/openapi.json`
- **Liveness Probe:** `GET /healthz`
- **Readiness Probe (with dependency health):** `GET /readyz`
- **Prometheus Metrics:** `GET /metrics`
- **List Orders (with cursor pagination, sort, fields):** `GET /orders`
- **Create Order (with Idempotency-Key):** `POST /orders`
- **Async Job Dispatcher:** `POST /jobs/export`
- **Poll Async Job:** `GET /jobs/{id}`
- **Webhooks Receiver (HMAC verified):** `POST /webhooks/incoming`

### Running the Full Test Suite

```bash
# Run all unit and integration tests with Go race detector
go test -v -race ./...

# Check statement test coverage
go test -cover ./...
```
