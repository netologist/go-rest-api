package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// IdempotencyStore is an in-memory implementation for demonstration purposes.
// In production this should be backed by Redis (or similar) with a real TTL,
// so that idempotency is respected across multiple service instances.
type IdempotencyStore struct {
	mu      sync.Mutex
	records map[string]*idempotencyRecord
	ttl     time.Duration
}

func NewIdempotencyStore(ttl time.Duration) *IdempotencyStore {
	return &IdempotencyStore{records: make(map[string]*idempotencyRecord), ttl: ttl}
}

type idemKeyCtxKey struct{}

// Middleware enforces the Idempotency-Key header on non-idempotent methods
// (POST/PATCH) and replays the previous response verbatim when the same key
// and request body are seen again. Differing bodies under the same key are
// rejected as a key-reuse error.
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

		// New key: mark it in-flight so concurrent retries with the same key
		// are told to wait/conflict instead of double-processing.
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

		// Expire the record after TTL. In production, rely on Redis TTL instead
		// of an in-process goroutine (which won't survive a restart or scale-out).
		go func(k string) {
			time.Sleep(s.ttl)
			s.mu.Lock()
			delete(s.records, k)
			s.mu.Unlock()
		}(key)
	})
}

// responseRecorder captures the downstream handler's status code and body
// so the idempotency store can persist and later replay the exact response.
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
