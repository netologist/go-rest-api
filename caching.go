package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CacheDirective models standard HTTP Cache-Control headers (RFC 9111).
type CacheDirective struct {
	MaxAge         time.Duration
	SMaxAge        time.Duration
	Public         bool
	Private        bool
	NoCache        bool
	NoStore        bool
	MustRevalidate bool
	Immutable      bool
}

// String serializes CacheDirective into a valid Cache-Control header value.
func (c CacheDirective) String() string {
	if c.NoStore {
		return "no-store, no-cache, must-revalidate"
	}
	if c.NoCache {
		return "no-cache"
	}

	var parts []string
	if c.Public {
		parts = append(parts, "public")
	} else if c.Private {
		parts = append(parts, "private")
	}

	if c.MaxAge > 0 {
		parts = append(parts, fmt.Sprintf("max-age=%d", int(c.MaxAge.Seconds())))
	}
	if c.SMaxAge > 0 {
		parts = append(parts, fmt.Sprintf("s-maxage=%d", int(c.SMaxAge.Seconds())))
	}
	if c.MustRevalidate {
		parts = append(parts, "must-revalidate")
	}
	if c.Immutable {
		parts = append(parts, "immutable")
	}

	if len(parts) == 0 {
		return "private, no-cache"
	}
	return strings.Join(parts, ", ")
}

// ComputeETag computes a strong ETag based on SHA-256 hash of payload.
func ComputeETag(payload []byte) string {
	h := sha256.Sum256(payload)
	return fmt.Sprintf(`"%s"`, hex.EncodeToString(h[:16]))
}

// ComputeWeakETag computes a weak ETag (prefixed with W/).
func ComputeWeakETag(payload []byte) string {
	h := sha256.Sum256(payload)
	return fmt.Sprintf(`W/"%s"`, hex.EncodeToString(h[:16]))
}

// CheckIfNoneMatch checks whether the inbound If-None-Match header matches the resource ETag.
// If it matches, the client already has the freshest version, so a 304 Not Modified should be sent.
func CheckIfNoneMatch(r *http.Request, etag string) bool {
	header := r.Header.Get("If-None-Match")
	if header == "" {
		return false
	}

	// Wildcard match
	if header == "*" {
		return true
	}

	// Clean weak tags comparison if needed
	cleanETag := strings.TrimPrefix(etag, "W/")
	tags := strings.Split(header, ",")
	for _, tag := range tags {
		t := strings.TrimSpace(tag)
		tClean := strings.TrimPrefix(t, "W/")
		if tClean == cleanETag || t == etag {
			return true
		}
	}
	return false
}

// CheckIfMatch implements Optimistic Concurrency Control (OCC) per RFC 9110.
// For mutating requests (PUT/PATCH/DELETE), the client provides If-Match: "<current-etag>".
// If the server's current ETag does not match, a 412 Precondition Failed must be returned to prevent lost updates.
func CheckIfMatch(r *http.Request, currentETag string) bool {
	header := r.Header.Get("If-Match")
	if header == "" {
		// If-Match is optional unless enforced by API business rules.
		return true
	}

	if header == "*" {
		return currentETag != ""
	}

	cleanCurrent := strings.TrimPrefix(currentETag, "W/")
	tags := strings.Split(header, ",")
	for _, tag := range tags {
		t := strings.TrimSpace(tag)
		tClean := strings.TrimPrefix(t, "W/")
		if tClean == cleanCurrent || t == currentETag {
			return true
		}
	}
	return false
}

// SetCacheHeaders sets standard caching headers on the HTTP response.
func SetCacheHeaders(w http.ResponseWriter, etag string, lastModified time.Time, directive CacheDirective) {
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	if !lastModified.IsZero() {
		w.Header().Set("Last-Modified", lastModified.UTC().Format(http.TimeFormat))
	}
	w.Header().Set("Cache-Control", directive.String())
	w.Header().Add("Vary", "Accept, Accept-Encoding")
}

// CheckIfModifiedSince checks whether the resource was modified since the client's cached timestamp.
func CheckIfModifiedSince(r *http.Request, lastModified time.Time) bool {
	header := r.Header.Get("If-Modified-Since")
	if header == "" || lastModified.IsZero() {
		return false
	}

	t, err := http.ParseTime(header)
	if err != nil {
		return false
	}

	// Truncate to second precision as HTTP date format only supports second granularity
	return !lastModified.UTC().Truncate(time.Second).After(t.UTC())
}
