# Pattern 14: API Versioning & RFC 8594 / RFC 9745 Deprecation

## 1. Executive Summary & Versioning Strategies

APIs evolve over time. While additive changes (new optional query params, new JSON fields) should be backward-compatible, breaking changes (removing fields, altering type semantics) require explicit versioning strategies.

### 4 Primary REST API Versioning Strategies

| Strategy | Syntax Example | Advantages | Disadvantages |
|---|---|---|---|
| **1. URI Path Versioning** | `/api/v1/orders`<br>`/api/v2/orders` | Explicit, easy to test in browsers, simple CDN routing. | Violates pure REST URI permanence (same resource has multiple URIs). **Industry favorite (Stripe, Twilio).** |
| **2. Custom Header Versioning** | `X-API-Version: 2026-08-29` | Clean URIs, supports date-based version pinning. | Harder to test directly in browser address bars, requires header forwarding in proxies. |
| **3. Vendor Media Type Versioning** | `Accept: application/vnd.app.v2+json` | Purest REST implementation (HATEOAS compliant). | Complex client configuration, proxy caching fragmentation. |
| **4. Query Parameter Versioning** | `/orders?v=2` | Simple to test. | Confuses caching proxies, easy to omit. |

---

## 2. API Deprecation & Sunset Standards

When an old API version is marked for retirement, servers should communicate deprecation programmatically per IETF RFC standards.

### Standards Overview
1. **RFC 9745 (The `Deprecation` HTTP Response Header):** Communicates that an endpoint is deprecated. The value is either `true` or an `@<unix-timestamp>` indicating the date of deprecation.
2. **RFC 8594 (The `Sunset` HTTP Response Header):** Communicates the exact date and time when the endpoint will be permanently decommissioned (returning `410 Gone` or `404 Not Found`). The value is an HTTP-date (RFC 1123).
3. **Link Headers:** Provide links to the successor version and migration guides.

```http
HTTP/1.1 200 OK
Deprecation: @1768435200
Sunset: Tue, 01 Jun 2027 00:00:00 GMT
Link: </api/v2/orders>; rel="successor-version"
Link: <https://api.example.com/docs/migrations/v1-to-v2>; rel="deprecation"; type="text/html"
```

---

## 3. Deprecation Middleware in Go

```go
type DeprecationConfig struct {
	DeprecationDate  time.Time
	SunsetDate       time.Time
	SuccessorVersion string
	DocumentationURL string
}

func DeprecationMiddleware(cfg DeprecationConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()

			if !cfg.DeprecationDate.IsZero() {
				h.Set("Deprecation", fmt.Sprintf("@%d", cfg.DeprecationDate.Unix()))
			} else {
				h.Set("Deprecation", "true")
			}

			if !cfg.SunsetDate.IsZero() {
				h.Set("Sunset", cfg.SunsetDate.UTC().Format(http.TimeFormat))
			}

			if cfg.SuccessorVersion != "" {
				h.Add("Link", fmt.Sprintf(`<%s>; rel="successor-version"`, cfg.SuccessorVersion))
			}
			if cfg.DocumentationURL != "" {
				h.Add("Link", fmt.Sprintf(`<%s>; rel="deprecation"; type="text/html"`, cfg.DocumentationURL))
			}

			next.ServeHTTP(w, r)
		})
	}
}
```

---

## 4. Principal Guidelines for Breaking Changes

1. **Additive Changes are Free:** Always add new optional fields or new endpoints without bumping the major API version.
2. **Give 12–24 Months Sunset Notice:** For public B2B APIs, provide at least 12 months between setting the `Sunset` header and turning off the endpoint.
3. **Monitor Deprecated Traffic:** Use Prometheus metrics to monitor traffic to deprecated endpoints before flipping the kill switch to `410 Gone`.
