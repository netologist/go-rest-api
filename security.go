package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
)

// SecurityHeadersMiddleware adds OWASP-recommended HTTP security headers to all responses.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()

		// Prevent MIME-sniffing
		h.Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking via iframes
		h.Set("X-Frame-Options", "DENY")

		// Strict Transport Security (HSTS) - 2 years + subdomains + preload
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

		// Restrict resource loading to secure origins only
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

		// Restrict Referer header leakage
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Disable browser features not needed by REST APIs
		h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=(), payment=()")

		next.ServeHTTP(w, r)
	})
}

// CORSOptions configures Cross-Origin Resource Sharing rules.
type CORSOptions struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAgeSeconds    int
}

// DefaultCORSOptions returns sensible, production-ready CORS defaults.
func DefaultCORSOptions() CORSOptions {
	return CORSOptions{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{
			"Accept", "Authorization", "Content-Type", "X-Request-Id",
			"X-API-Key", "Idempotency-Key", "If-Match", "If-None-Match",
		},
		ExposedHeaders: []string{
			"X-Request-Id", "ETag", "Location", "Retry-After",
			"RateLimit-Limit", "RateLimit-Remaining", "X-Idempotent-Replayed",
		},
		AllowCredentials: false,
		MaxAgeSeconds:    86400, // 24 hours
	}
}

// CORSMiddleware handles cross-origin browser requests and preflight OPTIONS queries.
func CORSMiddleware(opts CORSOptions) func(http.Handler) http.Handler {
	allowedOriginSet := make(map[string]bool)
	wildcard := false
	for _, o := range opts.AllowedOrigins {
		if o == "*" {
			wildcard = true
		}
		allowedOriginSet[o] = true
	}

	methodsStr := strings.Join(opts.AllowedMethods, ", ")
	headersStr := strings.Join(opts.AllowedHeaders, ", ")
	exposedStr := strings.Join(opts.ExposedHeaders, ", ")
	maxAgeStr := strconv.Itoa(opts.MaxAgeSeconds)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Validate origin
			if wildcard {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if allowedOriginSet[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			} else {
				// Origin not allowed
				next.ServeHTTP(w, r)
				return
			}

			if opts.AllowCredentials && !wildcard {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if exposedStr != "" {
				w.Header().Set("Access-Control-Expose-Headers", exposedStr)
			}

			// Handle preflight OPTIONS request
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", methodsStr)
				w.Header().Set("Access-Control-Allow-Headers", headersStr)
				w.Header().Set("Access-Control-Max-Age", maxAgeStr)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequestBodyLimitMiddleware bounds the maximum readable size of request payloads to prevent DoS.
func RequestBodyLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil || r.Body == http.NoBody {
				next.ServeHTTP(w, r)
				return
			}

			// Wrap body reader with MaxBytesReader
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// ProblemRecoveryMiddleware catches unexpected panics and renders an RFC 7807 Internal Error response.
func ProblemRecoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					// Check for http.ErrAbortHandler
					if rec == http.ErrAbortHandler {
						panic(rec)
					}

					var err error
					switch x := rec.(type) {
					case string:
						err = errors.New(x)
					case error:
						err = x
					default:
						err = fmt.Errorf("unknown panic: %v", rec)
					}

					stack := string(debug.Stack())
					logger.Error("panic_recovered",
						"requestId", GetRequestID(r.Context()),
						"method", r.Method,
						"path", r.URL.Path,
						"error", err.Error(),
						"stack", stack,
					)

					// Write RFC 7807 500 response
					InternalError(w, r)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
