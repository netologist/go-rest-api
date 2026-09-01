package main

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// DeprecationConfig defines API endpoint sunset and deprecation policies per RFC 8594 / RFC 9745.
type DeprecationConfig struct {
	DeprecationDate  time.Time // When the endpoint became officially deprecated
	SunsetDate       time.Time // When the endpoint will be permanently turned off
	SuccessorVersion string    // e.g. "/api/v2/orders"
	DocumentationURL string    // Migration guide URL
}

// DeprecationMiddleware attaches RFC 8594 (Sunset Header) and RFC 9745 (Deprecation Header) to responses.
func DeprecationMiddleware(cfg DeprecationConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()

			// Deprecation header: formatted as Unix timestamp (@<seconds>) or "true"
			if !cfg.DeprecationDate.IsZero() {
				h.Set("Deprecation", fmt.Sprintf("@%d", cfg.DeprecationDate.Unix()))
			} else {
				h.Set("Deprecation", "true")
			}

			// Sunset header: HTTP-date format (RFC 1123)
			if !cfg.SunsetDate.IsZero() {
				h.Set("Sunset", cfg.SunsetDate.UTC().Format(http.TimeFormat))
			}

			// Link headers for successor version and deprecation docs
			var links []string
			if cfg.SuccessorVersion != "" {
				links = append(links, fmt.Sprintf(`<%s>; rel="successor-version"`, cfg.SuccessorVersion))
			}
			if cfg.DocumentationURL != "" {
				links = append(links, fmt.Sprintf(`<%s>; rel="deprecation"; type="text/html"`, cfg.DocumentationURL))
			}
			for _, link := range links {
				h.Add("Link", link)
			}

			next.ServeHTTP(w, r)
		})
	}
}

type apiVersionCtxKey struct{}

// WithAPIVersion stores the negotiated API version in request context.
func WithAPIVersion(ctx context.Context, version string) context.Context {
	return context.WithValue(ctx, apiVersionCtxKey{}, version)
}

// GetAPIVersion retrieves the API version from request context.
func GetAPIVersion(ctx context.Context) string {
	if v, ok := ctx.Value(apiVersionCtxKey{}).(string); ok {
		return v
	}
	return "v1"
}

// HeaderVersioningMiddleware extracts version from X-API-Version or Accept header.
func HeaderVersioningMiddleware(defaultVersion string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			version := r.Header.Get("X-API-Version")
			if version == "" {
				version = r.Header.Get("Accept-Version")
			}
			if version == "" {
				// Check vendor media type if present
				version = VersionFromVendorMediaType(r.Header.Get("Accept"))
			}
			if version == "" {
				version = defaultVersion
			}

			w.Header().Set("X-API-Version-Selected", version)
			next.ServeHTTP(w, r.WithContext(WithAPIVersion(r.Context(), version)))
		})
	}
}
