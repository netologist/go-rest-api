# REST API Design & Maturity Guide (Principal Engineer Level)

> This document covers REST API maturity models, API-first methodology, documentation, best practices, HTTP status codes, authentication/authorization, throttling/rate-limiting, idempotency, KYC/KYB integrations, streaming, and tracking/observability — written at a principal/staff engineering depth, end to end.

---

## Table of Contents

1. [Richardson Maturity Model](#1-richardson-maturity-model)
2. [API-First Approach](#2-api-first-approach)
3. [API Documentation](#3-api-documentation)
4. [REST Best Practices](#4-rest-best-practices)
5. [HTTP Status Codes](#5-http-status-codes)
6. [Authentication](#6-authentication)
7. [Authorization](#7-authorization)
8. [Rate Limiting & Throttling](#8-rate-limiting--throttling)
9. [Idempotency](#9-idempotency)
10. [KYC / KYB Integrations](#10-kyc--kyb-integrations)
11. [Streaming (Real-Time Data)](#11-streaming-real-time-data)
12. [Tracking, Logging & Observability](#12-tracking-logging--observability)
13. [Error Handling](#13-error-handling)
14. [API Security (OWASP API Top 10)](#14-api-security-owasp-api-top-10)
15. [Versioning Strategies](#15-versioning-strategies)
16. [Caching](#16-caching)
17. [Testing & CI/CD](#17-testing--cicd)
18. [Additional Source: Zalando RESTful API Guidelines](#18-additional-source-notable-practices-from-the-zalando-restful-api-guidelines)
19. [Resilience Patterns & Distributed Systems](#19-resilience-patterns--distributed-systems)
20. [Golang + Chi Framework Example Case](#20-golang--chi-framework-example-case)
21. [Principal-Level Checklist](#21-principal-level-checklist)

---

## 1. Richardson Maturity Model

Defined by Leonard Richardson, this model classifies how "RESTful" an API is across four levels (0-3).

### Level 0 — The Swamp of POX (Plain Old XML/JSON)
- A single endpoint (e.g. `/api`), usually only `POST`.
- HTTP is used merely as a transport protocol, resembling SOAP/RPC.
- Example: `POST /api` body: `{"action": "getUser", "id": 5}`
- **Problem:** none of HTTP's semantics (verbs, status codes, caching) are leveraged.

### Level 1 — Resources
- Each entity gets its own URI: `/users/5`, `/orders/12`.
- Still typically a single HTTP method (`POST`) is used.
- **Gain:** resource-oriented thinking begins.

### Level 2 — HTTP Verbs
- `GET`, `POST`, `PUT`, `PATCH`, `DELETE` are used with correct semantics.
- Correct HTTP status codes (`200`, `201`, `404`, `409`, etc.) are returned.
- **This is considered "industry-standard REST" today.** Most production APIs sit here.

### Level 3 — HATEOAS (Hypermedia as the Engine of Application State)
- The response includes `links` describing the actions available on that resource.
- The client follows server-provided links instead of hardcoding URIs.

```json
{
  "id": 5,
  "status": "pending",
  "amount": 250.00,
  "_links": {
    "self": { "href": "/orders/5" },
    "cancel": { "href": "/orders/5/cancel", "method": "POST" },
    "pay": { "href": "/orders/5/pay", "method": "POST" }
  }
}
```

**Principal note:** HATEOAS is theoretically the "purest" form of REST, but in practice it adds complexity despite reducing client-server coupling. Most companies stop at Level 2 + good documentation (OpenAPI). HATEOAS pays off in domains with a real state machine — orders, payments, KYC flows.

---

## 2. API-First Approach

API-first is a development philosophy where the API contract is designed and approved **before** any code is written.

### Why API-First?
- Frontend, mobile, and partner teams can work in parallel against a **mock server** before the backend is finished.
- The contract becomes a cross-team agreement; breaking-change risk is caught early.
- Contract-driven tests (consumer-driven contracts, Pact, etc.) provide automated verification.

### The API-First Flow
1. **Design** — write the OpenAPI/AsyncAPI spec (Stoplight, Swagger Editor).
2. **Review** — review against architectural, security, and naming standards.
3. **Mock** — generate an automatic mock server from the spec (Prism, WireMock).
4. **Parallel Development** — frontend builds against the mock, backend builds against the spec.
5. **Contract Testing** — verify the real implementation conforms to the spec (Dredd, Schemathesis).
6. **Publish** — publish the spec to a developer portal (Backstage, Redoc, Stoplight).

### API-First vs Code-First
| Criterion | API-First | Code-First |
|---|---|---|
| Parallel work | High | Low |
| Consistency | High (centralized standard) | Varies by team |
| Speed (small project) | Slower at first | Fast |
| Long-term maintenance | Easier | Docs/code drift risk over time |

**Principal note:** API-first is nearly mandatory for large, multi-team systems consumed by external partners (banks, payment providers, KYC vendors).

---

## 3. API Documentation

### OpenAPI Specification (OAS 3.1)
The industry standard. At minimum it should include:
- `info`: title, version, contact, license
- `servers`: environment-specific (sandbox, prod)
- `paths`: request/response schemas per endpoint
- `components/schemas`: reusable models
- `components/securitySchemes`: auth mechanisms
- Example requests/responses (`examples`)
- Error models (see Section 13)

```yaml
openapi: 3.1.0
info:
  title: Payments API
  version: 2.1.0
  description: |
    Service for managing payment transactions.
servers:
  - url: https://api.example.com/v2
    description: Production
  - url: https://sandbox.api.example.com/v2
    description: Sandbox
paths:
  /payments/{paymentId}:
    get:
      summary: Retrieve payment details
      operationId: getPayment
      security:
        - oauth2: [payments:read]
      responses:
        '200':
          description: Success
        '404':
          description: Payment not found
```

### Components of Good Documentation
1. **Getting Started** — obtaining auth, first request, sandbox details.
2. **Reference** — auto-generated (Redoc/Swagger UI) endpoint reference.
3. **Guides / How-to** — common scenarios (webhook setup, idempotent retries, etc.).
4. **Changelog** — every version change, clearly marked breaking vs non-breaking.
5. **SDK/Code samples** — at minimum curl, followed by popular languages (JS, Python, Java).
6. **Postman/Insomnia Collection** — a ready-made shareable collection.
7. **Status Page** — uptime, incident history (statuspage.io-style).
8. **Rate limit and error code tables** — as a separate, searchable page.

**Principal note:** Documentation becomes worthless the moment it drifts out of sync with the code. Make the spec the **single source of truth** and generate documentation from it automatically (contract-as-code).

---

## 4. REST Best Practices

### Resource Naming
- Resource names should be **plural nouns**: `/users`, `/orders`, never verbs (`/getUsers` ❌).
- Hierarchy should reflect relationships: `/users/{userId}/orders/{orderId}`.
- Lowercase, kebab-case: `/order-items` (camelCase or snake_case in the body is a team-standard choice, not a URI concern).
- Verbs are only an acceptable exception for "action" endpoints: `/orders/{id}/cancel` (POST).

### HTTP Methods and Semantics
| Method | Purpose | Idempotent? | Safe? |
|---|---|---|---|
| GET | Read a resource | Yes | Yes |
| POST | Create a new resource / trigger an action | No | No |
| PUT | Fully replace/create a resource | Yes | No |
| PATCH | Partially update a resource | No (generally) | No |
| DELETE | Delete a resource | Yes | No |
| HEAD | Like GET but no body | Yes | Yes |
| OPTIONS | Discover supported methods | Yes | Yes |

### Filtering, Sorting, Pagination
```
GET /orders?status=paid&sort=-createdAt&page[size]=20&page[cursor]=eyJpZCI6MTB9
```
- **Cursor-based pagination** is more reliable than `offset` for large or changing datasets (no shifting when new records are inserted).
- `offset/limit` is acceptable for small, static datasets.
- Meta information should be returned in the response:
```json
{
  "data": [...],
  "meta": { "nextCursor": "eyJpZCI6MzB9", "hasMore": true }
}
```

### Partial Response / Field Selection
```
GET /users/5?fields=id,email,name
```

### Bulk Operations
- Endpoints like `POST /orders/bulk` let you process N resources in one request, avoiding the N-request problem.
- `207 Multi-Status` can be used for partial-success scenarios.

### Consistent Response Envelope
```json
{
  "data": { ... },
  "meta": { "requestId": "req_9f8e", "timestamp": "2026-08-26T10:00:00Z" },
  "errors": []
}
```

### Content Type and Negotiation
- `Content-Type: application/json; charset=utf-8` should always be explicit.
- `Accept` header can support version/format negotiation (`application/vnd.company.v2+json`).

### Naming Consistency
- Dates in ISO 8601: `2026-08-26T10:00:00Z`.
- Currency amounts should always be separate `amount` + `currency` fields (integer minor units — cents — not floats).
- Null vs missing-field behavior should be documented.

---

## 5. HTTP Status Codes

### 2xx — Success
| Code | Meaning | Usage |
|---|---|---|
| 200 OK | General success | Successful GET, PUT, PATCH |
| 201 Created | Resource created | After POST, with a `Location` header |
| 202 Accepted | Request accepted, processing async | Webhook-triggering, long-running operations |
| 204 No Content | Success, no body | DELETE, some PUT scenarios |
| 207 Multi-Status | Mixed results in bulk operation | Originated in WebDAV, used in bulk APIs |

### 3xx — Redirection
| Code | Meaning |
|---|---|
| 301 Moved Permanently | Resource permanently moved |
| 304 Not Modified | Cache is still valid (ETag/If-None-Match) |

### 4xx — Client Error
| Code | Meaning | When |
|---|---|---|
| 400 Bad Request | General client error | Validation error, malformed JSON |
| 401 Unauthorized | Authentication failed | Missing/invalid token |
| 403 Forbidden | Authenticated but not authorized | Insufficient scope/role |
| 404 Not Found | Resource doesn't exist | Wrong ID, deleted resource |
| 405 Method Not Allowed | Method not supported | `DELETE /users` (collection, not item) |
| 406 Not Acceptable | Content negotiation failed | Unsupported `Accept` |
| 409 Conflict | State conflict | Duplicate email, optimistic-lock failure |
| 410 Gone | Resource permanently removed | Deprecated endpoint |
| 412 Precondition Failed | `If-Match` mismatch | Optimistic concurrency |
| 413 Payload Too Large | Body size limit exceeded | |
| 415 Unsupported Media Type | Content-Type not supported | |
| 422 Unprocessable Entity | Syntactically valid, semantically invalid | Business rule violation (e.g. age < 18) |
| 429 Too Many Requests | Rate limit exceeded | Sent with `Retry-After` header |

### 5xx — Server Error
| Code | Meaning |
|---|---|
| 500 Internal Server Error | Unexpected error |
| 502 Bad Gateway | Upstream service returned an invalid response |
| 503 Service Unavailable | Service temporarily unavailable (maintenance, overload) |
| 504 Gateway Timeout | Upstream service timed out |

**Principal note:** `400 vs 422` is commonly confused: 400 means "I couldn't understand/parse the request," 422 means "I understood the request, but it violates a business rule." Keeping this distinction consistent simplifies error-handling logic for consuming teams.

---

## 6. Authentication

### Methods
| Method | Use Case | Note |
|---|---|---|
| API Key | Server-to-server, low sensitivity | In a header (`X-API-Key`), never in a URL query string |
| Basic Auth | Legacy, internal systems | HTTPS always mandatory |
| OAuth 2.0 (Client Credentials) | Server-to-server, partner integrations | Most common enterprise standard |
| OAuth 2.0 (Authorization Code + PKCE) | Delegated user authorization (web/mobile) | PKCE should be treated as mandatory for mobile/SPA |
| JWT (Bearer Token) | Stateless auth | `exp`, `iat`, `aud`, `iss` claims must be validated |
| mTLS (Mutual TLS) | High-security financial APIs | Common in banking/Open Banking standards |
| HMAC Signature | Webhook verification | Body + secret signature, replay-attack resistant |

### JWT Validation Checklist
- Signature algorithm must be whitelisted (`alg: none` must never be accepted — a critical vulnerability).
- `exp` (expiry) and `nbf` (not before) must be checked.
- `aud` (audience) must be verified to belong to your own service.
- Short-lived access tokens (5-15 min) + refresh token rotation.
- Refresh tokens should be single-use (rotation); if reuse is detected, the entire token chain should be revoked.

### Header Standards
```
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...
```

**Principal note:** Authentication answers "who are you?"; authorization answers "what can you do?" Conflating the two leads to misuse of 401/403 codes.

---

## 7. Authorization

### Models
- **RBAC (Role-Based Access Control):** Users are assigned roles (admin, operator, viewer); permissions attach to roles.
- **ABAC (Attribute-Based Access Control):** Decisions are based on attributes of the user, resource, and environment (e.g. "can only see transactions from their own branch").
- **ReBAC (Relationship-Based):** Google Zanzibar-style, based on a relationship graph (e.g. "are you the owner of this document?").
- **Scope-based (OAuth2):** Granular scopes like `payments:read`, `payments:write`, `kyc:approve`, designed around least-privilege.

### Endpoint-Level Authorization Design
```
GET /accounts/{id}/transactions   → scope: accounts:read
POST /accounts/{id}/transactions  → scope: accounts:write
POST /kyc/verifications/{id}/approve → scope: kyc:approve (compliance role only)
```

### Principal-Level Recommendations
- Always make the authorization decision **on the backend**, via a centralized policy engine (OPA/Open Policy Agent, Cedar); avoid `if role == 'admin'` checks scattered throughout code.
- Missing **object-level authorization** (IDOR — Insecure Direct Object Reference) is the #1 risk in OWASP API Top 10: when handling `GET /orders/123`, always verify that order 123 truly belongs to the requesting user.
- Audit logging: record who accessed what resource, when, and with what permission — mandatory especially for KYC/financial data.

---

## 8. Rate Limiting & Throttling

### Rate Limiting vs Throttling
- **Rate Limiting:** Caps the maximum number of requests allowed within a given time window (quota logic).
- **Throttling:** Controls the *rate* at which requests are processed (slowing down, queuing); typically kicks in under system overload.

### Algorithms
| Algorithm | Description | Pro | Con |
|---|---|---|---|
| Fixed Window | Counter within a fixed time window | Simple | Burst risk (up to 2x limit) at window boundaries |
| Sliding Window Log | Each request's timestamp is stored | Very precise | High memory cost |
| Sliding Window Counter | Smoothed version of fixed window | Good balance | Approximate result |
| Token Bucket | A bucket is refilled with tokens at a steady rate; each request consumes a token | Allows bursts, flexible | Requires parameter tuning |
| Leaky Bucket | Requests are "leaked" out at a constant rate | Fixes the output rate | Doesn't allow bursts |

**Principal note:** API gateways (Kong, Envoy, AWS API Gateway) generally favor **Token Bucket** because it allows legitimate burst traffic (e.g. batch synchronization) while preserving the long-term average.

### Response Header Standards (IETF draft + common practice)
```
RateLimit-Limit: 1000
RateLimit-Remaining: 42
RateLimit-Reset: 1735300000
Retry-After: 30
```
- On limit exceeded, return **429 Too Many Requests** + `Retry-After`.

### Layered Rate Limiting
1. **Global** (IP-based across the whole API, DDoS protection — usually at the WAF/CDN layer).
2. **Per API Key / Client** (partner-specific quota, per contract).
3. **Per User** (user-level, abuse prevention).
4. **Per Endpoint** (expensive endpoints — e.g. `/kyc/verify` is limited more strictly since it incurs third-party OCR costs).

### Post-Quota-Exceeded UX
- A clear error message + `Retry-After`.
- "Burst credit" or a "grace window" can be defined for key partners.
- Rate-limit metrics should be transparently exposed on the customer dashboard (developer portal).

---

## 9. Idempotency

### Why Is It Needed?
Network interruptions, timeouts, and retry mechanisms can cause the same `POST` request to be sent multiple times. This is especially critical in **payments, money transfers, and order creation**, where duplicate processing (double-charging) is a serious risk.

### The Idempotency Key Pattern
```
POST /payments
Idempotency-Key: 8f9e2b1a-3c4d-4e5f-9a1b-2c3d4e5f6a7b
```

**Server-side flow:**
1. Check whether the `Idempotency-Key` + client ID combination has been seen before (typically Redis/DB, TTL 24-48 hours).
2. If not seen: execute the operation, store the result keyed by the key, return the response.
3. If the same key arrives again with the **same body**: don't re-execute; return the original response as-is (usually `200`/`201` with the original payload).
4. If the same key arrives with a **different body**: return `422 Unprocessable Entity` or `409 Conflict` (key-reuse error).
5. If the operation is still in progress (concurrent retry): return `409 Conflict` or `425 Too Early`.

### Idempotent Methods vs Idempotency Key
- `GET`, `PUT`, `DELETE` are already idempotent by HTTP semantics (sending the same request N times should produce the same result).
- `POST` is not idempotent by nature; the idempotency key pattern was specifically designed for `POST`.

### Storage and Scaling
- Keys should be kept in a distributed cache (Redis), along with the response body.
- Key TTL should be set based on business needs (typically 24-72 hours).
- Storing the idempotency key together with a request hash cleanly separates the "same key, different body" scenario.

---

## 10. KYC / KYB Integrations

### Definitions
- **KYC (Know Your Customer):** Verification of an individual customer's identity (ID document, face match/liveness, address verification).
- **KYB (Know Your Business):** Verification of a legal entity (trade registry, UBO — Ultimate Beneficial Owner — identification, authorized signatories).

### Typical API Flow (Asynchronous Nature Is Critical)
KYC/KYB verification is generally **asynchronous, not synchronous**, because third-party services (OCR, sanctions-list screening, biometric matching) can take seconds to minutes.

```
POST /kyc/verifications
  → 202 Accepted, { "verificationId": "kyc_123", "status": "pending" }

GET /kyc/verifications/kyc_123
  → { "status": "pending" | "approved" | "rejected" | "manual_review" }

Webhook: POST {callback_url}
  → { "verificationId": "kyc_123", "status": "approved", "eventType": "kyc.verification.completed" }
```

### State Machine
```
pending → in_review → (approved | rejected | manual_review)
manual_review → (approved | rejected)
approved → expired (if periodic re-KYC is required)
```

### Critical Design Criteria
1. **PII Minimization:** Responses should never return unnecessary PII; raw ID numbers or document photos must never appear in logs (masking is mandatory).
2. **Encryption:** Documents (ID photo, passport) must be encrypted at rest and in transit; stored in a separate, tightly access-controlled "vault" service.
3. **Data Retention:** Retention and deletion policy must reflect local regulation (GDPR, and country-specific financial regulators — e.g. FinCEN, FCA) and be reflected in the API design (`DELETE /kyc/subjects/{id}` — "right to erasure").
4. **Audit Trail:** An immutable log of who approved/rejected which KYC record, when, and with what outcome.
5. **Sanction/PEP Screening Integration:** Can be modeled as a separate sub-resource: `GET /kyc/verifications/{id}/screening-results`.
6. **Idempotency:** Idempotency keys should be used on repeated KYC-initiation requests for the same user (prevents duplicate applications/charges).
7. **Rate Limiting:** KYC/KYB endpoints are typically expensive due to third-party costs (each call is billed), so they should be tightly rate-limited with abuse (fraud-driven repeated attempts) detection.
8. **Webhook Reliability:** Since webhook delivery isn't guaranteed, consumers should be able to confirm status via `GET /kyc/verifications/{id}` polling (a webhook + polling hybrid is the best practice).
9. **Versioned Rules:** KYC risk rules (e.g. the high-risk country list) change over time; which rule set was used for which decision at which date must be recorded (for regulatory audits).
10. **Multi-Stage Review (Manual Review):** Alongside automated approve/reject, a "manual_review" state requires a separate case-management API (`GET /kyc/reviews?status=pending`).

### Regulatory Context
- KYC/KYB is typically part of **AML (Anti-Money Laundering)** regulation.
- Authorities such as FinCEN (US), the FCA (UK), and the EU's AMLD (Anti-Money Laundering Directive) directly influence API design (data retention, reporting obligations).
- While an API designer doesn't need to master the legal details of these regulations, they must work with the compliance team to clarify **"how long specific data must be retained"** and **"which transactions must be reportable."**

---

## 11. Streaming (Real-Time Data)

REST's classic request/response model falls short for scenarios requiring real-time or continuous data flow (price updates, transaction notifications, log streaming).

### Comparison of Options
| Method | Direction | Use Case | Note |
|---|---|---|---|
| Polling | Client → Server (repeated) | Simple, low frequency | Wasteful, latency |
| Long Polling | Client → Server (held open) | Medium frequency, legacy compatibility | Server resource consumption |
| SSE (Server-Sent Events) | Server → Client (one-way) | Notifications, live feeds | Over HTTP, simple, automatic reconnect |
| WebSocket | Bidirectional, full-duplex | Trading, chat, live collaboration | More complex, stateful connection management |
| Webhooks | Server → Server (one-way, event-driven) | System-to-system integration (KYC result, payment status) | The most common B2B integration pattern |
| gRPC Streaming | Bidirectional, binary | High-performance inter-microservice communication | Not REST, but often used alongside it |

### SSE Example
```
GET /accounts/123/events
Accept: text/event-stream

data: {"event": "balance.updated", "amount": 1500.00}

data: {"event": "transaction.created", "id": "tx_1"}
```

### Webhook Design Best Practices
1. **Signing:** Every webhook body should be signed with HMAC-SHA256 (`X-Signature` header); the receiver must never act on a request before verifying the signature.
2. **Retry Policy:** If the receiver doesn't return 2xx, retry with exponential backoff (e.g. 1min, 5min, 30min, 2h, 24h); after a bounded number of attempts, move to a "dead letter" queue.
3. **Idempotency:** Every event should have a unique `eventId`; the receiver must store this ID to avoid reprocessing the same event (webhooks guarantee "at-least-once" delivery, not "exactly-once").
4. **No Ordering Guarantee:** Events should not be assumed to arrive in order; each event should carry its own timestamp/version information.
5. **Event Versioning:** Versioned event types like `eventType: "kyc.verification.completed.v2"` allow payload-schema changes without breaking consumers.
6. **Replay Endpoint:** `POST /webhooks/{eventId}/replay` — allow manually re-triggering an event if the receiving system missed it.
7. **Subscription Management:** `POST /webhook-subscriptions` should let consumers manage which event types they subscribe to.

### Chunked Transfer / Streaming Response
- `Transfer-Encoding: chunked` can be used for large file exports or token-based responses (as in AI/LLM streaming).
- Client-side backpressure handling (the server slowing down when the client can't keep up) should be designed for.

---

## 12. Tracking, Logging & Observability

### The Three Pillars of Observability
1. **Logging** — discrete events (structured/JSON logs).
2. **Metrics** — numerical time-series data (latency, error rate, throughput).
3. **Tracing** — a request's end-to-end journey across systems (distributed tracing).

### Correlation ID / Request ID
Every request should be tracked with a **single identifier** carried across system boundaries:
```
X-Request-Id: 7f3e9c2a-1b4d-4a5e-8c6f-9d0e1f2a3b4c
```
- If the client doesn't send one, the gateway/first service should generate it.
- It must be propagated identically to all downstream service calls (and logs).
- It should also be returned in the response header — clients can share this ID when raising support tickets, simplifying troubleshooting.

### Distributed Tracing (OpenTelemetry)
- **Trace ID:** Represents the entire end-to-end request.
- **Span ID:** Represents each individual service/step's own sub-operation.
- W3C Trace Context standard: `traceparent` header (`00-{trace-id}-{span-id}-{flags}`).
- OpenTelemetry has become the industry standard (compatible with Jaeger, Zipkin, Datadog, Grafana Tempo).

### Structured Logging
```json
{
  "timestamp": "2026-08-26T10:15:32.451Z",
  "level": "ERROR",
  "service": "payments-api",
  "requestId": "7f3e9c2a-...",
  "traceId": "abc123",
  "userId": "usr_789",
  "endpoint": "POST /payments",
  "statusCode": 422,
  "latencyMs": 145,
  "message": "Insufficient funds"
}
```
- PII must never be logged in raw form (masking: `user@***.com`, `**** **** **** 1234`).

### Metrics (Golden Signals)
1. **Latency** — p50, p95, p99 response time.
2. **Traffic** — RPS (requests per second).
3. **Errors** — 4xx/5xx rate.
4. **Saturation** — system resource saturation (CPU, connection pool, etc.).

### API Analytics / Usage Tracking
- Which client uses which endpoint, and how often — critical for partner billing and capacity planning.
- Should be reflected in dashboards via `X-Client-Id` or API-key-based segmentation.

### Alerting
- SLO (Service Level Objective)-based alerting: e.g. "alert if p99 latency exceeds 500ms for more than 5 minutes" — an **error budget** approach is preferable to raw thresholds.

---

## 13. Error Handling

### RFC 7807 — Problem Details for HTTP APIs
A standard, machine-readable error format:
```json
{
  "type": "https://api.example.com/errors/insufficient-funds",
  "title": "Insufficient Funds",
  "status": 422,
  "detail": "The account balance does not cover this transaction.",
  "instance": "/payments/789",
  "requestId": "7f3e9c2a-1b4d-4a5e-8c6f-9d0e1f2a3b4c",
  "errors": [
    { "field": "amount", "code": "EXCEEDS_BALANCE" }
  ]
}
```

### Principles
- Error messages should be processable by **both humans and machines** (`code` field fixed/enum, `message` human-facing).
- Sensitive details (stack traces, DB error messages) must never be returned in production for security reasons.
- Every error should include a `requestId` — for matching against logs when handling support requests.
- On validation errors, **all** invalid fields should be listed in a single response (so the user isn't stuck trial-and-erroring one field at a time).

---

## 14. API Security (OWASP API Top 10 — 2023)

1. **Broken Object Level Authorization (BOLA/IDOR)** — the most critical risk; ownership must be checked on every resource access.
2. **Broken Authentication** — weak token management, lack of brute-force protection.
3. **Broken Object Property Level Authorization** — a user being able to modify fields (e.g. `role`, `balance`) via PATCH that they shouldn't be able to touch.
4. **Unrestricted Resource Consumption** — missing rate limiting/quotas (see Section 8).
5. **Broken Function Level Authorization** — regular users able to reach admin endpoints.
6. **Unrestricted Access to Sensitive Business Flows** — bots/automation abusing a business flow (e.g. ticket purchasing, registration).
7. **Server Side Request Forgery (SSRF)** — a server calling a user-supplied URL without validation.
8. **Security Misconfiguration** — default credentials, unnecessary open endpoints, overly detailed error messages.
9. **Improper Inventory Management** — old (v1) endpoints that are no longer used but remain open and unmonitored — "shadow API" risk.
10. **Unsafe Consumption of APIs** — blind trust in data from third-party APIs (e.g. a KYC vendor); input validation must always be applied.

### Additional Security Controls
- **Input validation:** strict, schema-based (JSON Schema) validation, whitelist approach.
- **TLS 1.2+ mandatory**, weak cipher suites disabled.
- **CORS** policy limited to the minimum necessary origins.
- **Secrets** must never live in code/repo (use Vault, KMS).
- **Security headers:** `Strict-Transport-Security`, `X-Content-Type-Options: nosniff`.

---

## 15. Versioning Strategies

| Strategy | Example | Pro | Con |
|---|---|---|---|
| URI Path | `/v2/orders` | Visible, cache-friendly | URI pollution |
| Header | `Accept: application/vnd.company.v2+json` | Clean URI | Lower discoverability |
| Query Param | `/orders?version=2` | Simple | Caching complexity |

### Breaking vs Non-Breaking Changes
- **Non-breaking:** adding a new optional field, adding a new endpoint.
- **Breaking:** removing/renaming a field, changing a type, adding a required field, changing behavior.
- Breaking changes always require a new major version + a **deprecation policy** for the old version (e.g. 12 months of support + `Sunset` header + `Deprecation` header).

```
Deprecation: true
Sunset: Sat, 31 Dec 2026 23:59:59 GMT
Link: <https://api.example.com/v3/orders>; rel="successor-version"
```

---

## 16. Caching

- `ETag` + `If-None-Match`: conditional requests based on a content hash, saving bandwidth with `304 Not Modified`.
- `Cache-Control: private, max-age=60` — use `private` for user-specific data, `public` for shareable data.
- `Last-Modified` + `If-Modified-Since`: a time-based alternative.
- Sensitive endpoints such as financial/KYC data typically use `Cache-Control: no-store`.

---

## 17. Testing & CI/CD

- **Contract Testing:** automated validation against the OpenAPI spec (Schemathesis, Dredd).
- **Consumer-Driven Contracts:** using Pact to guarantee the provider meets consumer expectations.
- **Fuzz Testing:** resilience testing with unexpected/malformed input.
- **Load/Performance Testing:** SLO verification with k6, Gatling.
- **Security Testing:** automated scanning with OWASP ZAP, Burp Suite.
- **Spec linting in CI:** style/standard checks (naming conventions, required fields) with tools like Spectral.
- **Backward Compatibility Gate:** automatically diff the spec on each PR and block the merge if a breaking change is detected.

---

## 18. Additional Source: Notable Practices from the Zalando RESTful API Guidelines

The [Zalando RESTful API and Event Guidelines](https://opensource.zalando.com/restful-api-guidelines/) is an open-source, highly detailed standard set matured over years at a large e-commerce company running hundreds of microservices. Here are the notable additional practices that don't overlap with the sections above:

### API Meta Information and the "API Audience" Concept
- Every API spec should be tagged with `x-api-id` (a permanent, unique UUID) and `x-audience` (target audience) metadata.
- An **audience classification** is recommended as best practice: `component-internal` → `business-unit-internal` → `company-internal` → `external-partner` → `external-public`. This classification is used to scale documentation depth, review rigor, and authorization requirements — an API exposed to external partners requires a far stricter contract and backward-compatibility guarantee than an internal service.
- Semantic Versioning (major.minor.patch) is recommended for API spec versioning; avoiding pre-release/build metadata is advised.

### JSON Field Naming Standard
- Zalando uses **snake_case** for property names (`sales_order_number`, `billing_address`), aligning with companies like GitHub, Twitter, and Stack Exchange. Regardless of your own team's camelCase/snake_case preference, **consistency** is the critical point.
- Enum values should be defined as `UPPER_SNAKE_CASE` strings (e.g. `PENDING_REVIEW`), visually distinguishing them from regular fields.
- `null` and an "absent field" should carry the exact same semantics — expecting clients to interpret the two differently is highly error-prone. The one exception is JSON Merge Patch, where `null` explicitly means "delete this field."
- Boolean fields should never use `null`; if a third state is genuinely needed (`YES`/`NO`/`UNKNOWN`), switch to an enum instead.
- Empty arrays should always be returned as `[]`, never `null`.

### Common Reusable Data Models
- **Money object:** kept as two separate fields — `amount` (decimal, e.g. `99.95`) + `currency` (ISO 4217, e.g. `"EUR"`) — and never converted to `float`/`double` (precision-loss risk); exact types like Java's `BigDecimal` should be used.
- The Money object should **not be extended via inheritance** (e.g. adding `discounted_amount` violates the Liskov Substitution Principle); composition is preferred instead — separate Money objects like `price` and `discounted_price`.
- **Address/Addressee objects:** should be defined as a reusable schema with standard field names such as `salutation`, `first_name`, `last_name`, `street`, `city`, `zip`, `country_code` (ISO 3166-1 alpha-2).

### PATCH Strategy (Clarified Preference Order)
Zalando prioritizes three approaches for partial updates:
1. **JSON Merge Patch** (`application/merge-patch+json`, RFC 7396) — the first choice for simple scenarios.
2. **JSON Patch** (`application/json-patch+json`, RFC 6902) — for more complex scenarios like updating a single element within a large collection.
3. If neither is sufficient, an explicitly documented `POST` may be used.
Only **one** of these three approaches should be chosen per endpoint — mixing them creates complexity on the client side.

### A Caution on UUID Usage
- UUIDs allow collision-free ID generation in distributed systems, but they're hard for humans to remember, awkward in logging/debugging, and carry a 36-character bandwidth cost.
- For low-volume but widely-referenced "master data" such as brand IDs or attribute IDs, short, server-generated meaningful IDs are preferable to UUIDs.
- Sequential, strictly increasing numeric IDs can leak sensitive information (e.g. total order volume) to unauthorized clients — so IDs should always be kept as `string` type, not numbers.
- If a creation-time-sortable ID is needed (e.g. for cursor-based pagination), **ULID** (Universally Unique Lexicographically Sortable Identifier) can be considered instead of UUID.

### Proprietary HTTP Headers
- **`X-Flow-ID`:** corresponds to the `X-Request-Id` in our guide above; recommended as a mandatory header for tracing a request across all services, propagated identically to all downstream calls.
- **`Idempotency-Key`:** matches exactly the pattern described in Section 9; defined as an optional but recommended header.
- **`Prefer`:** lets the client request specific processing behavior from the server (e.g. `Prefer: respond-async` to request asynchronous instead of synchronous processing, or `Prefer: return=minimal` to ask that the created resource not be returned in the body).
- **Kebab-case with capitalized words:** the recommended format for HTTP header names (e.g. `X-Flow-ID`, not `X-flow-id`).

### The Problem JSON (RFC 7807/9457) Requirement
- The `application/problem+json` content type should be adopted as the standard for error responses; this aligns exactly with the RFC 7807 format given in Section 13 of this guide.
- Never leaking stack traces into responses in production is emphasized as a separate, explicit rule.

### Pagination Clarifications
- **Cursor-based pagination** is explicitly preferred over offset-based pagination; the cursor should be an **opaque** value that clients should never attempt to inspect or construct (typically encoding an encrypted page position + direction + filter information).
- Including a total record count (`total_count`) in the response should be avoided where possible — computing this figure over large tables is expensive and rarely serves a genuine business need.
- Standard query parameter names: `q` (search), `sort` (with `+`/`-` prefix for direction), `fields` (partial field selection), `embed` (sub-resource embedding), `offset`, `cursor`, `limit`.

### API Compatibility Philosophy
- Postel's Law (the Robustness Principle) is adopted as a central principle: **"Be conservative in what you send, be liberal in what you accept."**
- Adding a new optional field is treated as a "compatible extension" and considered normal API evolution; it is the **client's responsibility** to be designed to ignore new fields/enum values it doesn't recognize.
- **Media type versioning** (`Accept: application/vnd.company.v2+json`) is preferred over URL-based versioning; where possible, avoiding versioning entirely by moving forward only with backward-compatible changes is the best option.

### An Additional Perspective on Event/Webhook Design
- Events should be treated as part of the API and go through the same review process; an "event schema" should be versioned and kept backward-compatible just like an "API contract."
- Distinguishing between two event categories is useful: **general business-process events** (e.g. "order created") vs **data change events** (e.g. "this record was updated") — for the latter, ordering guarantees (events for the same resource must be processed in order) become more critical.
- Event consumers should be resilient to duplicate deliveries of the same event and designed to be idempotent against out-of-order events — this aligns exactly with the webhook principles in Section 11.

> **Note:** The Zalando guidelines contain some dependencies specific to internal processes (internal registries, internal review tooling, etc.) that aren't directly applicable outside the company; however, the general principles (naming, versioning, pagination, PATCH strategy, common data objects) can be adopted directly regardless of organization. Full details: https://opensource.zalando.com/restful-api-guidelines/

---

## 19. Resilience Patterns & Distributed Systems

Designing a single service correctly isn't enough — in production, services depend on each other over a network, and the network is **unreliable** (packet loss, latency, partial outages). This section covers the core patterns that make APIs resilient in distributed systems.

### Backpressure
This occurs when a service can't process incoming requests as fast as they're being produced (e.g. if a stream consumer processes more slowly than the producer, queues balloon and memory can be exhausted).
- **In reactive flows (streaming/queue consumption):** the consumer should signal how much data it can accept based on its own processing capacity (e.g. `request(n)`) — TCP's own flow control is an example of this same principle.
- **The practical REST API equivalent:** rate limiting (Section 8), monitoring queue-depth metrics, and signaling "I can't accept this right now" via `503 Service Unavailable` + `Retry-After`.
- **Load shedding:** deliberately rejecting low-priority requests when the system is overloaded, in order to protect critical operations.

### Circuit Breaker
If a downstream service keeps failing, retrying every request over and over (and timing out) wears down both the calling service and the downstream service even further ("cascading failure").
- **Closed:** normal operation; requests pass through and the error rate is monitored.
- **Open:** once an error threshold is exceeded (e.g. 50% errors over the last 10 seconds), the circuit opens; requests are never sent downstream and an error/fallback is returned directly.
- **Half-Open:** after a set waiting period, a limited number of test requests are allowed through; if they succeed, the circuit returns to `Closed`, otherwise it goes back to `Open`.
- Netflix Hystrix (now unmaintained) popularized this pattern; today libraries like **resilience4j** (Java), **Polly** (.NET), and **gobreaker** (Go) are used instead.

### Retry — Exponential Backoff + Jitter
- Immediately retrying a failed request compounds the problem during a moment of temporary congestion (the thundering herd problem).
- **Exponential backoff:** the wait time doubles on each attempt (1s, 2s, 4s, 8s...).
- **Jitter (randomness):** a random value is added to the wait time to prevent all clients from retrying at the exact same moment (strategies like "full jitter" or "decorrelated jitter").
- **Only idempotent operations, or those using an idempotency key, should be retried** (see Section 9) — otherwise, retrying increases the risk of duplicate processing.
- The maximum number of retry attempts and the total time budget (deadline) must always be bounded.

### Timeout Management
- Every outbound call must have an **explicit timeout**; a client waiting "forever" can exhaust its own thread/connection pool and lock up the whole service.
- **Deadline propagation:** a request's total time budget (e.g. 2 seconds) should be shared across every downstream call in the chain (Go's `context.Context` + deadline mechanism is ideal for this).

### Bulkhead Pattern
- Like the watertight compartments of a ship, **separate resource pools** (thread pool, connection pool) are defined for different downstream dependencies.
- The goal: prevent a slow or crashed dependency from affecting all other operations within the same process (via exhaustion of a shared thread pool).

### Graceful Degradation & Fallback
- If a non-critical feature (e.g. a "recommended products" service) goes down, the main function (e.g. "create order") shouldn't be blocked — a **simplified response** (empty list, stale cached data) should be returned instead.
- Rather than silently returning incomplete data, the fallback strategy can transparently flag the response as "degraded" (e.g. `meta.degraded: true`).

### Health Checks: Liveness / Readiness / Startup
Three distinct health check types popularized by the Kubernetes ecosystem:
- **Liveness:** "Is this process alive, or is it deadlocked/hung?" — if it fails, the orchestrator restarts the container.
- **Readiness:** "Is this instance currently ready to receive traffic?" (e.g. not ready if the DB connection hasn't been established yet) — if it fails, the load balancer should stop routing traffic, but the container isn't restarted.
- **Startup:** for slow-starting services, this prevents the liveness check from triggering too early and causing an unnecessary restart loop.
- Typically exposed as separate endpoints: `GET /healthz` (liveness) and `GET /readyz` (readiness).

### Saga Pattern (Distributed Transaction Management)
In a microservices architecture, you can't spread a single ACID transaction across multiple services; instead, a chain of **local transactions + compensating actions** is used.
- **Choreography:** each service performs its own operation and publishes an event; the next service listens for that event and performs its own operation. There's no central coordinator, but tracking the overall flow can become harder.
- **Orchestration:** a central "saga orchestrator" triggers each step in sequence and manages compensating steps on failure (e.g. if "charge payment" fails, "release the inventory reservation").
- Example: an order-creation saga → reserve inventory → charge payment → create shipment. If payment fails → cancel the inventory reservation (compensating action).

### Outbox Pattern
- When a service needs to both write to its own database and publish an event (the "dual write" problem), there's a risk of inconsistency between the two (written to DB but event not sent, or vice versa).
- **Solution:** the event is written into an "outbox" table in the same database, within the same transaction (atomic with the DB write). A separate background process (polling, or CDC — Change Data Capture, e.g. Debezium) reads this table and publishes the event to the actual message queue (Kafka, SQS).

### CQRS and Event Sourcing (Brief Overview)
- **CQRS (Command Query Responsibility Segregation):** separating the write (command) and read (query) models; in systems with heavy read traffic, this allows the read side to scale independently with optimized read models.
- **Event Sourcing:** instead of storing an entity's current state, storing the full ordered sequence of events (changes) applied to that entity; the current state is derived by "replaying" these events. This fits naturally in domains with heavy audit/regulatory requirements (finance, KYC).

### The CAP Theorem and Consistency Models (Core Concepts)
- **CAP Theorem:** during a network partition, a distributed system must trade off between Consistency and Availability; both cannot be guaranteed simultaneously.
- **Strong consistency:** every read sees the most recently written data (but this may require sacrificing availability or latency).
- **Eventual consistency:** the system becomes consistent over time; brief periods of inconsistency are accepted (used in most distributed caches, replicas, and event-driven architectures).
- API designers should clearly document which endpoints operate under "strong" versus "eventual" consistency — especially to avoid surprising clients in "read-after-write" scenarios.

### Service Mesh (Sidecar Pattern)
- Instead of embedding cross-cutting network concerns like mTLS, retry, circuit breaking, rate limiting, and tracing into application code, a **sidecar proxy** (like Envoy) is placed alongside each service, moving these responsibilities into the infrastructure layer.
- Service mesh solutions like Istio and Linkerd allow most of the resilience patterns discussed in this section (retry, circuit breaking, timeout, mTLS) to be applied as centralized policy, without any code changes.

---

## 20. Golang + Chi Framework Example Case

This section combines **Idempotency-Key**, **SSE streaming**, a **standard controller (handler) structure**, **query string / form parameters**, **429 rate limiting**, **RFC 7807 error handling**, **retry + circuit breaker (resilience)**, and **observability (request ID, structured logging)** into a single, realistic Go + [go-chi/chi](https://github.com/go-chi/chi) example.

> The full code was also generated as separate files (see the file list at the end of this message): `go.mod`, `main.go`, `problem.go`, `idempotency.go`, `ratelimit.go`, `resilience.go`, `handlers.go`. The most instructive pieces are shown below.

### Project Structure
```
api-example/
├── go.mod
├── main.go            # router setup, middleware chain, server start
├── problem.go          # RFC 7807 Problem Details helpers
├── idempotency.go      # Idempotency-Key middleware (in-memory store)
├── ratelimit.go        # Token bucket-based rate limiter middleware
├── resilience.go       # Retry (backoff+jitter) and a simple Circuit Breaker
└── handlers.go         # OrderHandler: standard controller structure
```

### 1) `go.mod`
```go
module api-example

go 1.22

require github.com/go-chi/chi/v5 v5.0.12
```

### 2) `problem.go` — RFC 7807 Error Handling
```go
package main

import (
	"encoding/json"
	"net/http"
)

// Problem implements the RFC 7807 (Problem Details for HTTP APIs) format.
type Problem struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail,omitempty"`
	Instance  string       `json:"instance,omitempty"`
	RequestID string       `json:"requestId,omitempty"`
	Errors    []FieldError `json:"errors,omitempty"`
}

type FieldError struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

// WriteProblem writes a standard error response and sets the Content-Type
// to application/problem+json.
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, problemType, title, detail string, fieldErrors ...FieldError) {
	p := Problem{
		Type:      problemType,
		Title:     title,
		Status:    status,
		Detail:    detail,
		Instance:  r.URL.Path,
		RequestID: GetRequestID(r.Context()),
		Errors:    fieldErrors,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

// Common error shortcuts
func BadRequest(w http.ResponseWriter, r *http.Request, detail string, fieldErrors ...FieldError) {
	WriteProblem(w, r, http.StatusBadRequest, "https://api.example.com/errors/bad-request", "Bad Request", detail, fieldErrors...)
}

func NotFound(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusNotFound, "https://api.example.com/errors/not-found", "Not Found", detail)
}

func Conflict(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusConflict, "https://api.example.com/errors/conflict", "Conflict", detail)
}

func UnprocessableEntity(w http.ResponseWriter, r *http.Request, detail string, fieldErrors ...FieldError) {
	WriteProblem(w, r, http.StatusUnprocessableEntity, "https://api.example.com/errors/unprocessable-entity", "Unprocessable Entity", detail, fieldErrors...)
}

func TooManyRequests(w http.ResponseWriter, r *http.Request, retryAfterSeconds int) {
	w.Header().Set("Retry-After", itoa(retryAfterSeconds))
	WriteProblem(w, r, http.StatusTooManyRequests, "https://api.example.com/errors/rate-limited", "Too Many Requests",
		"Rate limit exceeded, please retry after the specified duration.")
}

func InternalError(w http.ResponseWriter, r *http.Request) {
	// Stack traces / internal details must NEVER leak to the client in production.
	WriteProblem(w, r, http.StatusInternalServerError, "https://api.example.com/errors/internal", "Internal Server Error",
		"An unexpected error occurred.")
}

func itoa(n int) string {
	return http.StatusText(0) // simplified placeholder; the real file uses strconv.Itoa
}
```

> **Note:** the `itoa` function above is simplified for readability; the actual file uses `strconv.Itoa` (see the delivered file).

### 3) `idempotency.go` — Idempotency-Key Middleware
```go
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

type idempotencyRecord struct {
	requestHash string
	statusCode  int
	body        []byte
	headers     http.Header
	createdAt   time.Time
	inFlight    bool
}

// IdempotencyStore should be replaced with Redis in production (for TTL support).
type IdempotencyStore struct {
	mu      sync.Mutex
	records map[string]*idempotencyRecord
	ttl     time.Duration
}

func NewIdempotencyStore(ttl time.Duration) *IdempotencyStore {
	return &IdempotencyStore{records: make(map[string]*idempotencyRecord), ttl: ttl}
}

// Middleware enforces the Idempotency-Key header for non-idempotent methods
// like POST/PATCH, and replays the previous response verbatim on repeated requests.
func (s *IdempotencyStore) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPatch {
			next.ServeHTTP(w, r)
			return
		}

		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			BadRequest(w, r, "The 'Idempotency-Key' header is required for this endpoint.")
			return
		}

		bodyBytes, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		hash := sha256.Sum256(bodyBytes)
		reqHash := hex.EncodeToString(hash[:])

		s.mu.Lock()
		rec, exists := s.records[key]
		if exists {
			if rec.inFlight {
				s.mu.Unlock()
				Conflict(w, r, "A request with the same Idempotency-Key is still being processed.")
				return
			}
			if rec.requestHash != reqHash {
				s.mu.Unlock()
				UnprocessableEntity(w, r, "This Idempotency-Key was previously used with a different request body.")
				return
			}
			// Same key + same body: replay the previous response verbatim.
			for k, vv := range rec.headers {
				for _, v := range vv {
					w.Header().Add(k, v)
				}
			}
			w.Header().Set("X-Idempotent-Replayed", "true")
			w.WriteHeader(rec.statusCode)
			_, _ = w.Write(rec.body)
			s.mu.Unlock()
			return
		}

		// New record: mark it "in-flight" (to catch concurrent retries).
		rec = &idempotencyRecord{requestHash: reqHash, createdAt: time.Now(), inFlight: true}
		s.records[key] = rec
		s.mu.Unlock()

		rw := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK, body: &bytes.Buffer{}}
		next.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), idemKeyCtxKey{}, key)))

		s.mu.Lock()
		rec.inFlight = false
		rec.statusCode = rw.statusCode
		rec.body = rw.body.Bytes()
		rec.headers = rw.Header().Clone()
		s.mu.Unlock()

		// Clean up the record after TTL (simplified; production should use Redis TTL).
		go func(k string) {
			time.Sleep(s.ttl)
			s.mu.Lock()
			delete(s.records, k)
			s.mu.Unlock()
		}(key)
	})
}

type idemKeyCtxKey struct{}

// responseRecorder captures the downstream handler's response
// (so it can be stored in the idempotency store).
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

var _ = json.Marshal // (used elsewhere in the full example file)
```

### 4) `ratelimit.go` — Token Bucket-Based 429 Handling
```go
package main

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// TokenBucket implements the classic token bucket algorithm.
type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	refillRate float64 // tokens added per second
	lastRefill time.Time
}

func NewTokenBucket(capacity, refillRatePerSecond float64) *TokenBucket {
	return &TokenBucket{tokens: capacity, capacity: capacity, refillRate: refillRatePerSecond, lastRefill: time.Now()}
}

func (b *TokenBucket) Allow() (bool, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = min(b.capacity, b.tokens+elapsed*b.refillRate)
	b.lastRefill = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	// Compute when the next token will be available (for Retry-After).
	wait := time.Duration((1 - b.tokens) / b.refillRate * float64(time.Second))
	return false, wait
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// RateLimiterStore keeps a separate bucket per client (API key or IP).
type RateLimiterStore struct {
	mu      sync.Mutex
	buckets map[string]*TokenBucket
	cap     float64
	refill  float64
}

func NewRateLimiterStore(capacity, refillRatePerSecond float64) *RateLimiterStore {
	return &RateLimiterStore{buckets: make(map[string]*TokenBucket), cap: capacity, refill: refillRatePerSecond}
}

func (s *RateLimiterStore) getBucket(clientID string) *TokenBucket {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.buckets[clientID]
	if !ok {
		b = NewTokenBucket(s.cap, s.refill)
		s.buckets[clientID] = b
	}
	return b
}

// Middleware applies per-client layered rate limiting and returns
// 429 + Retry-After + RateLimit-* headers when the limit is exceeded.
func (s *RateLimiterStore) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID := r.Header.Get("X-API-Key")
		if clientID == "" {
			// RemoteAddr includes an ephemeral port that differs per
			// connection, so it must be reduced to just the IP —
			// otherwise every connection lands in its own bucket and
			// the limiter never actually triggers.
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			clientID = host
		}

		bucket := s.getBucket(clientID)
		allowed, wait := bucket.Allow()

		w.Header().Set("RateLimit-Limit", strconv.Itoa(int(s.cap)))
		if allowed {
			bucket.mu.Lock()
			remaining := int(bucket.tokens)
			bucket.mu.Unlock()
			w.Header().Set("RateLimit-Remaining", strconv.Itoa(remaining))
			next.ServeHTTP(w, r)
			return
		}

		retryAfterSeconds := int(wait.Seconds()) + 1
		w.Header().Set("RateLimit-Remaining", "0")
		TooManyRequests(w, r, retryAfterSeconds)
	})
}
```

### 5) `resilience.go` — Retry (Backoff+Jitter) and a Simple Circuit Breaker
```go
package main

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

// RetryWithBackoff retries the given function with exponential backoff + jitter.
// Only errors marked as "retryable" by the caller are retried.
func RetryWithBackoff(ctx context.Context, maxAttempts int, baseDelay time.Duration, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err // deadline exceeded/canceled context — stop retrying
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !errors.Is(lastErr, ErrRetryable) {
			return lastErr // non-idempotent / permanent error: don't retry
		}

		// Exponential backoff + full jitter
		backoff := float64(baseDelay) * math.Pow(2, float64(attempt))
		jittered := time.Duration(rand.Float64() * backoff)

		select {
		case <-time.After(jittered):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return lastErr
}

var ErrRetryable = errors.New("retryable error")

// --- Simple Circuit Breaker ---

type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

type CircuitBreaker struct {
	mu               sync.Mutex
	state            CircuitState
	failureCount     int
	failureThreshold int
	openedAt         time.Time
	resetTimeout     time.Duration
}

func NewCircuitBreaker(failureThreshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{state: StateClosed, failureThreshold: failureThreshold, resetTimeout: resetTimeout}
}

var ErrCircuitOpen = errors.New("circuit breaker is open: downstream service is not currently being called")

func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	if cb.state == StateOpen {
		if time.Since(cb.openedAt) > cb.resetTimeout {
			cb.state = StateHalfOpen // allow a test request through
		} else {
			cb.mu.Unlock()
			return ErrCircuitOpen
		}
	}
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()
	if err != nil {
		cb.failureCount++
		if cb.state == StateHalfOpen || cb.failureCount >= cb.failureThreshold {
			cb.state = StateOpen
			cb.openedAt = time.Now()
		}
		return err
	}

	// Successful call: close the circuit, reset counters.
	cb.state = StateClosed
	cb.failureCount = 0
	return nil
}
```

### 6) `handlers.go` — Standard Controller Structure, Query/Form Params, SSE Streaming
```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// Order is our domain model.
type Order struct {
	ID        string  `json:"id"`
	Status    string  `json:"status"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	CreatedAt string  `json:"createdAt"`
}

// OrderHandler implements the "standard controller" pattern: dependencies
// (repository, downstream client, resilience components) are injected as
// struct fields, and each HTTP endpoint is defined as a method.
type OrderHandler struct {
	breaker *CircuitBreaker
	// repo    OrderRepository // in a real project, inject the DB/repository layer here
}

func NewOrderHandler(breaker *CircuitBreaker) *OrderHandler {
	return &OrderHandler{breaker: breaker}
}

// ---- GET /orders — Query String Parameters (filter, sort, pagination) ----
func (h *OrderHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	status := q.Get("status")   // ?status=paid
	sortParam := q.Get("sort")  // ?sort=-createdAt
	limit := 20
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		} else {
			BadRequest(w, r, "The 'limit' parameter must be an integer between 1 and 100.")
			return
		}
	}
	cursor := q.Get("cursor") // opaque cursor, must never be constructed by the client

	// --- In a real project: repo.List(ctx, status, sortParam, limit, cursor) ---
	orders := []Order{
		{ID: "ord_1", Status: "paid", Amount: 120.50, Currency: "USD", CreatedAt: time.Now().Format(time.RFC3339)},
	}

	resp := map[string]interface{}{
		"data": orders,
		"meta": map[string]interface{}{
			"requestId":    GetRequestID(r.Context()),
			"filters":      map[string]string{"status": status, "sort": sortParam},
			"nextCursor":   "",
			"hasMore":      false,
			"appliedLimit": limit,
			"cursorEcho":   cursor,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- GET /orders/{id} — Path Parameter ----
func (h *OrderHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		BadRequest(w, r, "The order ID is required.")
		return
	}
	// --- In a real project: order, err := repo.FindByID(ctx, id) ---
	if id != "ord_1" {
		NotFound(w, r, fmt.Sprintf("No order found with ID '%s'.", id))
		return
	}
	order := Order{ID: id, Status: "paid", Amount: 120.50, Currency: "USD", CreatedAt: time.Now().Format(time.RFC3339)}
	writeJSON(w, http.StatusOK, order)
}

// ---- POST /orders — JSON Body + Idempotency-Key (applied via middleware chain) ----
type createOrderRequest struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

func (h *OrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, r, "The request body is not valid JSON.")
		return
	}

	var fieldErrors []FieldError
	if req.Amount <= 0 {
		fieldErrors = append(fieldErrors, FieldError{Field: "amount", Code: "MUST_BE_POSITIVE"})
	}
	if req.Currency == "" {
		fieldErrors = append(fieldErrors, FieldError{Field: "currency", Code: "REQUIRED"})
	}
	if len(fieldErrors) > 0 {
		UnprocessableEntity(w, r, "Validation error.", fieldErrors...)
		return
	}

	// --- Resilient call to a downstream payment service (retry + circuit breaker) ---
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	err := h.breaker.Execute(func() error {
		return RetryWithBackoff(ctx, 3, 100*time.Millisecond, func() error {
			return callPaymentService(ctx, req.Amount, req.Currency)
		})
	})
	if err != nil {
		if err == ErrCircuitOpen {
			WriteProblem(w, r, http.StatusServiceUnavailable,
				"https://api.example.com/errors/downstream-unavailable",
				"Service Unavailable", "The payment service is temporarily unavailable, please try again later.")
			return
		}
		InternalError(w, r)
		return
	}

	order := Order{ID: "ord_new", Status: "pending", Amount: req.Amount, Currency: req.Currency, CreatedAt: time.Now().Format(time.RFC3339)}
	w.Header().Set("Location", "/orders/"+order.ID)
	writeJSON(w, http.StatusCreated, order)
}

// ---- POST /orders/form — application/x-www-form-urlencoded Body ----
func (h *OrderHandler) CreateFromForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		BadRequest(w, r, "The form data could not be parsed.")
		return
	}

	amountStr := r.FormValue("amount")
	currency := r.FormValue("currency")

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		UnprocessableEntity(w, r, "Validation error.",
			FieldError{Field: "amount", Code: "INVALID_OR_MISSING"})
		return
	}
	if currency == "" {
		UnprocessableEntity(w, r, "Validation error.",
			FieldError{Field: "currency", Code: "REQUIRED"})
		return
	}

	order := Order{ID: "ord_form_1", Status: "pending", Amount: amount, Currency: currency, CreatedAt: time.Now().Format(time.RFC3339)}
	w.Header().Set("Location", "/orders/"+order.ID)
	writeJSON(w, http.StatusCreated, order)
}

// ---- GET /orders/{id}/events — SSE (Server-Sent Events) Streaming ----
func (h *OrderHandler) Events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		InternalError(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			// Client closed the connection; end the goroutine cleanly.
			return
		case t := <-ticker.C:
			event := map[string]string{"event": "order.status.updated", "timestamp": t.Format(time.RFC3339)}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// callPaymentService simulates an example downstream service call.
func callPaymentService(ctx context.Context, amount float64, currency string) error {
	// In a real project, an http.Client would call downstream, and
	// 5xx / timeout responses would be wrapped with ErrRetryable.
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
```

### 7) `main.go` — Router Setup, Middleware Chain, Observability
```go
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type requestIDCtxKey struct{}

func GetRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// generateRequestID returns a random 16-hex-char fallback ID, used when the
// caller doesn't supply an X-Request-Id.
func generateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// RequestIDMiddleware reads or generates the X-Request-Id / X-Flow-ID header,
// adds it to the context and the response header (the foundation of distributed tracing).
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = generateRequestID()
		}
		w.Header().Set("X-Request-Id", reqID)
		ctx := context.WithValue(r.Context(), requestIDCtxKey{}, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// StructuredLoggingMiddleware writes every request as a structured (JSON) log.
func StructuredLoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info("http_request",
				"requestId", GetRequestID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"latencyMs", time.Since(start).Milliseconds(),
			)
		})
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	idemStore := NewIdempotencyStore(48 * time.Hour)
	rateLimiter := NewRateLimiterStore(100, 10) // capacity of 100 tokens, refilled at 10/second
	breaker := NewCircuitBreaker(5, 30*time.Second)
	orderHandler := NewOrderHandler(breaker)

	r := chi.NewRouter()

	// --- Global middleware chain (order matters!) ---
	r.Use(RequestIDMiddleware)                  // 1. Assign a request ID to every request
	r.Use(StructuredLoggingMiddleware(logger))  // 2. Structured logging
	r.Use(middleware.Recoverer)                 // 3. Recover from panics, convert to 500
	r.Use(rateLimiter.Middleware)                // 4. Rate limiting (429)
	r.Use(middleware.Timeout(5 * time.Second))  // 5. Global deadline per request

	// --- Health check endpoints (Kubernetes liveness/readiness) ---
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"alive"}`))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// In a real project: check DB/cache connectivity.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	// --- /orders resource group ---
	r.Route("/orders", func(orders chi.Router) {
		orders.Get("/", orderHandler.List) // query string: ?status=paid&sort=-createdAt&limit=20

		// Idempotency-Key is required only on non-idempotent (POST/PATCH) endpoints.
		orders.With(idemStore.Middleware).Post("/", orderHandler.Create)
		orders.With(idemStore.Middleware).Post("/form", orderHandler.CreateFromForm)

		orders.Get("/{id}", orderHandler.Get)
		orders.Get("/{id}/events", orderHandler.Events) // SSE streaming
	})

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("server_starting", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server_failed", "error", err.Error())
		os.Exit(1)
	}
}
```

### Example Usage (curl)
```bash
# Filtering/pagination via query string
curl "http://localhost:8080/orders?status=paid&sort=-createdAt&limit=10"

# Creating an order with a JSON body + Idempotency-Key
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: 8f9e2b1a-3c4d-4e5f-9a1b-2c3d4e5f6a7b" \
  -d '{"amount": 150.00, "currency": "USD"}'

# Resending the same Idempotency-Key + same body → same response, no reprocessing
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: 8f9e2b1a-3c4d-4e5f-9a1b-2c3d4e5f6a7b" \
  -d '{"amount": 150.00, "currency": "USD"}'

# Creating an order with a form-urlencoded body
curl -X POST http://localhost:8080/orders/form \
  -H "Idempotency-Key: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f" \
  -d "amount=200.00&currency=EUR"

# SSE stream (the connection stays open, an event arrives every 2 seconds)
curl -N http://localhost:8080/orders/ord_1/events

# Rate limit test (send enough requests quickly and you'll get 429 + Retry-After)
for i in $(seq 1 150); do curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/orders; done
```

**Principal note:** this example is for teaching purposes; before taking it to production you'll want to add: a Redis-backed idempotency/rate-limit store (for multi-instance consistency), real OpenTelemetry integration (`otelhttp` middleware), a real downstream HTTP client (with context deadlines and retryable-error classification), and configurable (env/config) rate-limit values.

---

## 21. Principal-Level Checklist

- [ ] Was the API spec (OpenAPI) designed and reviewed before any code was written?
- [ ] Are resource naming and HTTP method semantics consistent?
- [ ] Are correct HTTP status codes and the RFC 7807 error format defined for every endpoint?
- [ ] Are authentication (OAuth2/mTLS) and object-level authorization (IDOR protection) implemented?
- [ ] Are rate-limiting headers (`RateLimit-*`, `Retry-After`) and a layered limit strategy in place?
- [ ] Do critical `POST` endpoints (payments, KYC initiation) support idempotency keys?
- [ ] Are KYC/KYB flows modeled asynchronously (pending/webhook/polling)? Are PII masking and retention policies defined?
- [ ] Was the right streaming method (SSE/WebSocket/Webhook) chosen for real-time needs?
- [ ] Is a correlation ID / distributed trace propagated end-to-end?
- [ ] Is a versioning and deprecation policy (Sunset header) defined?
- [ ] Are caching headers (ETag, Cache-Control) used correctly?
- [ ] Have OWASP API Top 10 controls (BOLA, function-level auth, SSRF, etc.) been applied?
- [ ] Is contract testing and a breaking-change gate set up in CI?
- [ ] Are circuit breaker + retry (backoff+jitter) + timeout/deadline propagation applied on critical downstream calls?
- [ ] Are liveness/readiness health check endpoints defined?
- [ ] Is a saga/compensating-action strategy defined for multi-step business processes?

---

*This guide is prepared as a starting reference at the principal/staff engineering level; institution-specific regulatory requirements (GDPR, FinCEN, PCI-DSS, local financial regulators, etc.) must always be verified with the legal/compliance team.*
