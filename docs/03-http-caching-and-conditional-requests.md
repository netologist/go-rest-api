# Pattern 03: HTTP Caching, Conditional Requests & Optimistic Concurrency Control

## 1. Executive Summary & Specification Standards

HTTP Caching (RFC 9111) and Conditional Requests (RFC 9110 / RFC 7232) form the bedrock of scalable web services. They deliver two critical capabilities:

1. **Bandwidth & Latency Optimization (Conditional GET):** Enables clients and intermediate proxies (CDNs) to validate whether cached representations are still fresh without re-transmitting the entire payload (`304 Not Modified`).
2. **Preventing Lost Updates (Optimistic Concurrency Control):** Ensures that concurrent mutations (`PUT`, `PATCH`, `DELETE`) do not overwrite changes made by another client mid-air (`412 Precondition Failed`).

---

## 2. Entity Tags (ETags): Strong vs Weak

An **ETag (Entity Tag)** is an opaque string identifier assigned by the web server to a specific version of a resource.

```
Strong ETag:  "686897696a7c876b7e"    (Byte-for-byte identical content)
Weak ETag:    W/"686897696a7c876b7e"  (Semantically equivalent content)
```

- **Strong ETags:** Guarantees that two representations are identical at the byte level. Required for range requests (`206 Partial Content`).
- **Weak ETags (prefixed with `W/`):** Indicates that two representations are semantically equivalent even if minor formatting differences exist (e.g. whitespace, ordering of JSON keys).

### ETag Calculation in Go

```go
func ComputeETag(payload []byte) string {
	h := sha256.Sum256(payload)
	return fmt.Sprintf(`"%s"`, hex.EncodeToString(h[:16]))
}
```

---

## 3. Conditional GET Flow (RFC 7232)

```
Client                                      Server
  |                                           |
  |--- GET /orders/1 ------------------------>|
  |<-- 200 OK [ETag: "v1", Body: {...}] ------|
  |                                           |
  | (Later: client wants to refresh data)    |
  |--- GET /orders/1 [If-None-Match: "v1"] -->|
  |                                           | (Resource hash matches "v1")
  |<-- 304 Not Modified [ETag: "v1"] ---------| (Empty body: 0 payload bytes transmitted)
```

### Go Implementation

```go
func (h *OrderHandler) Get(w http.ResponseWriter, r *http.Request) {
	order := h.getOrder(id)
	etag := order.ETag()

	// If-None-Match header check
	if CheckIfNoneMatch(r, etag) {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	SetCacheHeaders(w, etag, order.UpdatedAt, CacheDirective{
		MaxAge:         60 * time.Second,
		MustRevalidate: true,
		Private:        true,
	})
	writeJSON(w, http.StatusOK, order)
}
```

---

## 4. Optimistic Concurrency Control (OCC) with `If-Match`

In distributed multi-user environments, two users might fetch the same order and attempt to update it concurrently. Without concurrency controls, the second update silently overwrites the first (the **Lost Update Problem**).

### OCC Workflow

```
Client A                 Client B                 Server (Order v1)
   |                        |                            |
   |-- GET /orders/1 ------>|                            |--> ETag: "v1"
   |                        |-- GET /orders/1 ---------->|--> ETag: "v1"
   |                        |                            |
   |-- PUT (If-Match: "v1")->|                           |--> Matches! Update to v2 (ETag: "v2")
   |<- 200 OK (ETag: "v2")--|                            |
   |                        |                            |
   |                        |-- PUT (If-Match: "v1")---->|--> Mismatch! (Server is v2, client sent v1)
   |                        |<- 412 Precondition Failed -|   (Update rejected!)
```

### Go Implementation

```go
func (h *OrderHandler) Update(w http.ResponseWriter, r *http.Request) {
	order := h.findOrder(id)
	currentETag := order.ETag()

	// Enforce optimistic concurrency control
	if !CheckIfMatch(r, currentETag) {
		PreconditionFailed(w, r, "Resource was modified by another request. Fetch latest state and retry.")
		return
	}

	// Apply mutation safely...
	order.Version++
	order.UpdatedAt = time.Now().UTC()
	w.Header().Set("ETag", order.ETag())
	writeJSON(w, http.StatusOK, order)
}
```

---

## 5. Cache-Control Directives Reference

| Directive | Meaning |
|---|---|
| `public` | Response may be cached by any cache (browsers, CDNs, corporate proxies). |
| `private` | Response is personalized to a single user and MUST NOT be cached by shared caches (CDNs). |
| `no-cache` | Cache may store response, but MUST validate with server (`If-None-Match`) before serving it. |
| `no-store` | Response must NEVER be stored anywhere on disk or memory (sensitive financial/PII data). |
| `max-age=N` | Max time in seconds the representation is considered fresh. |
| `must-revalidate` | Once expired, stale copies must not be served without contacting the origin server. |
| `Vary: Accept, Origin` | Tells intermediate caches that representations vary based on incoming request headers. |
