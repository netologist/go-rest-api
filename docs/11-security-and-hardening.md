# Pattern 11: Security & API Hardening (OWASP API Top 10)

## 1. Executive Summary & Threat Landscape

REST APIs are prime targets for automated attacks, credential stuffing, data scraping, and denial-of-service attempts. Securing production REST APIs requires defense-in-depth across multiple OSI layers.

The **OWASP API Security Top 10** highlights the most critical risks:
- **API1: Broken Object Level Authorization (BOLA / IDOR)**
- **API2: Broken Authentication**
- **API4: Unrestricted Resource Consumption (DoS / Large Payloads)**
- **API5: Broken Function Level Authorization (Admin access bypass)**
- **API7: Server-Side Request Forgery (SSRF)**
- **API8: Security Misconfiguration (CORS, Missing Headers)**

---

## 2. Request Body Size Limiting (Defending Against DoS / OOM)

Without request body bounds, an attacker can stream gigabytes of garbage data to a JSON endpoint, exhausting server RAM and crashing the process with an Out-of-Memory (OOM) panic.

### Go Implementation with `http.MaxBytesReader`

```go
func RequestBodyLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil || r.Body == http.NoBody {
				next.ServeHTTP(w, r)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
```

If the client payload exceeds `maxBytes` (e.g. 1MB), `MaxBytesReader` terminates reads and sets an error, allowing the API to respond with **`413 Payload Too Large`**.

---

## 3. Production HTTP Security Headers

```go
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()

		// Prevent MIME-type confusion / sniffing attacks
		h.Set("X-Content-Type-Options", "nosniff")

		// Prevent Clickjacking via iframes
		h.Set("X-Frame-Options", "DENY")

		// Force HTTPS connection via HSTS (2 years + subdomains + preload)
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

		// Restrict scripts and resource loading
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

		// Restrict Referer header leakage
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Disable unused browser device features
		h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=(), payment=()")

		next.ServeHTTP(w, r)
	})
}
```

---

## 4. Cross-Origin Resource Sharing (CORS) Hardening

CORS is enforced by browsers to protect users from malicious cross-origin requests.

```go
type CORSOptions struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAgeSeconds    int
}
```

### Critical CORS Rules
1. **Never use `Access-Control-Allow-Origin: *` with `AllowCredentials: true`.**
2. **Handle Preflight `OPTIONS` requests cleanly:** Return `204 No Content` with cached max-age (`Access-Control-Max-Age: 86400`) to avoid repetitive preflight roundtrips.
3. **Expose Custom Headers:** Headers like `ETag`, `X-Request-Id`, `Retry-After`, and `RateLimit-Remaining` cannot be read by browser JavaScript unless explicitly declared in `Access-Control-Expose-Headers`.

---

## 5. Panic Recovery with RFC 7807

Uncaught panics must never drop the TCP connection abruptly or leak internal Go stack traces to clients.

```go
func ProblemRecoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					stack := string(debug.Stack())
					logger.Error("panic_recovered",
						"requestId", GetRequestID(r.Context()),
						"method", r.Method,
						"path", r.URL.Path,
						"error", fmt.Sprintf("%v", rec),
						"stack", stack,
					)
					InternalError(w, r) // Returns RFC 7807 500 JSON
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
```
