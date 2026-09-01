package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// generateRequestID returns a random 16-hex-char ID as fallback when X-Request-Id is absent.
func generateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type requestIDCtxKey struct{}

// GetRequestID retrieves the per-request correlation ID from context.
func GetRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// RequestIDMiddleware reads or generates an X-Request-Id correlation token.
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

// StructuredLoggingMiddleware writes one structured JSON log per completed HTTP request.
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

// BuildRouter constructs and wires all middlewares and endpoint routes.
func BuildRouter(logger *slog.Logger) (*chi.Mux, *OrderHandler, *JobStore) {
	idemStore := NewIdempotencyStore(48 * time.Hour)
	rateLimiter := NewRateLimiterStore(100, 10)
	breaker := NewCircuitBreaker(5, 30*time.Second)
	orderHandler := NewOrderHandler(breaker)
	jobStore := NewJobStore()
	jobHandler := NewJobHandler(jobStore)
	metrics := NewAPIMetrics()

	// Authentication setup
	jwtSecret := []byte("super-secret-key-at-least-32-chars-long!")
	jwtAuth := NewJWTAuthenticator(jwtSecret, "https://api.example.com")
	keyStore := NewAPIKeyStore(map[string]*AuthContext{
		"secret_admin_key": {
			Subject:    "user_admin",
			Email:      "admin@example.com",
			Roles:      []string{"admin", "user"},
			Scopes:     []string{"orders:read", "orders:write", "admin"},
			ClientID:   "admin_cli",
			AuthMethod: "api_key",
		},
		"secret_user_key": {
			Subject:    "user_standard",
			Email:      "customer@example.com",
			Roles:      []string{"user"},
			Scopes:     []string{"orders:read"},
			ClientID:   "web_app",
			AuthMethod: "api_key",
		},
	})

	// Health & readiness probes
	readinessChecker := NewReadinessChecker("1.0.0")
	readinessChecker.RegisterProbe("database", func(ctx context.Context) error {
		return nil // Simulated healthy DB
	})
	readinessChecker.RegisterProbe("payment_gateway", func(ctx context.Context) error {
		return nil // Simulated healthy payment provider
	})

	r := chi.NewRouter()

	// --- Global Middleware Chain (Order matters!) ---
	r.Use(SecurityHeadersMiddleware)                // 1. OWASP Security headers
	r.Use(CORSMiddleware(DefaultCORSOptions()))     // 2. CORS headers & preflight
	r.Use(RequestBodyLimitMiddleware(1024 * 1024))  // 3. Body size limit (1MB)
	r.Use(RequestIDMiddleware)                      // 4. Correlation ID (X-Request-Id)
	r.Use(StructuredLoggingMiddleware(logger))      // 5. Structured JSON access logger
	r.Use(ProblemRecoveryMiddleware(logger))        // 6. Panic recovery -> RFC 7807 500
	r.Use(metrics.Middleware)                       // 7. Prometheus metrics collector
	r.Use(rateLimiter.Middleware)                   // 8. Token Bucket rate limiting
	r.Use(middleware.Timeout(10 * time.Second))     // 9. Per-request context deadline

	// --- API Documentation (OpenAPI 3.1 & Swagger UI) ---
	r.Get("/openapi.json", ServeOpenAPISpec)
	r.Get("/docs", ServeSwaggerUI)

	// --- Observability & Probes ---
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"alive"}`))
	})
	r.Get("/readyz", readinessChecker.Handler())
	r.Get("/metrics", metrics.Handler())

	// --- Auth & Token Generation Demo Endpoint ---
	r.Post("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email string   `json:"email"`
			Roles []string `json:"roles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			BadRequest(w, r, "Invalid token request body.")
			return
		}
		if req.Email == "" {
			req.Email = "demo@example.com"
		}
		if len(req.Roles) == 0 {
			req.Roles = []string{"user"}
		}

		token, err := jwtAuth.GenerateToken(JWTClaims{
			Issuer:    "https://api.example.com",
			Subject:   "usr_" + generateRequestID()[:8],
			Email:     req.Email,
			Roles:     req.Roles,
			Scopes:    []string{"orders:read", "orders:write"},
			ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
			IssuedAt:  time.Now().Unix(),
		})
		if err != nil {
			InternalError(w, r)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"tokenType":   "Bearer",
			"accessToken": token,
			"expiresIn":   3600,
		})
	})

	// --- Orders Resource Group (/orders) ---
	r.Route("/orders", func(orders chi.Router) {
		// Content negotiation, filtering, cursor pagination & HATEOAS collection
		orders.Get("/", orderHandler.List)

		// Create orders with Idempotency-Key & Content-Type validation
		orders.With(
			idemStore.Middleware,
			ValidateContentTypeMiddleware(MediaTypeJSON),
		).Post("/", orderHandler.Create)

		// Form URL-encoded creation
		orders.With(
			idemStore.Middleware,
			ValidateContentTypeMiddleware("application/x-www-form-urlencoded"),
		).Post("/form", orderHandler.CreateFromForm)

		// Single resource operations
		orders.Get("/{id}", orderHandler.Get)
		orders.Put("/{id}", orderHandler.Update)   // Optimistic Concurrency Control (If-Match)
		orders.Patch("/{id}", orderHandler.Update) // Partial update with OCC
		orders.Post("/{id}/pay", orderHandler.Pay) // State transition to paid
		orders.Post("/{id}/cancel", orderHandler.Cancel) // State transition to cancelled
		orders.Get("/{id}/events", orderHandler.Events)  // SSE Streaming
	})

	// --- Asynchronous Long-Running Jobs (/jobs) ---
	r.Route("/jobs", func(jobs chi.Router) {
		jobs.Post("/export", jobHandler.CreateExportJob)
		jobs.Get("/{id}", jobHandler.GetJob)
		jobs.Delete("/{id}", jobHandler.CancelJob)
	})

	// --- Inbound Webhook Receiver with HMAC Signature Verification ---
	webhookSecret := []byte("webhook-signing-secret-key-12345")
	r.With(
		WebhookVerificationMiddleware(webhookSecret, 5*time.Minute),
	).Post("/webhooks/incoming", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		logger.Info("webhook_received", "payloadLength", len(body))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"received":true}`))
	})

	// --- Deprecated Endpoint Example (RFC 8594 / RFC 9745) ---
	sunsetDate := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	deprecationDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.With(DeprecationMiddleware(DeprecationConfig{
		DeprecationDate:  deprecationDate,
		SunsetDate:       sunsetDate,
		SuccessorVersion: "/orders",
		DocumentationURL: "https://api.example.com/docs/migrations/v1-to-v2",
	})).Get("/legacy-orders", orderHandler.List)

	// --- Protected Admin Route (RBAC) ---
	r.With(
		AuthMiddleware(jwtAuth, keyStore, false),
		RequireRoles("admin"),
	).Get("/admin/stats", func(w http.ResponseWriter, r *http.Request) {
		user, _ := GetAuthUser(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"message": "Welcome to protected admin panel",
			"admin":   user.Email,
		})
	})

	return r, orderHandler, jobStore
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	router, _, _ := BuildRouter(logger)

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("server_starting", "addr", srv.Addr)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server_failed", "error", err.Error())
			os.Exit(1)
		}
	}()

	GracefulShutdown(srv, logger, 10*time.Second)
}
