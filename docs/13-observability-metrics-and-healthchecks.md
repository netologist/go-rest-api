# Pattern 13: Observability, Metrics & Health Probes

## 1. Executive Summary & Cloud-Native Requirements

In containerized, cloud-native environments (Kubernetes, AWS ECS, Nomad), automated orchestrators manage the lifecycle of your REST API containers based on health probes.

Furthermore, production operations require real-time visibility into traffic volume, status code distributions, latency percentiles, and active connections via standard metrics (Prometheus / OpenMetrics) and distributed correlation tracing (`X-Request-Id`).

---

## 2. Kubernetes-Style Health Probes: Liveness vs Readiness

```
1. Liveness Probe (/healthz): "Is the Go process deadlocked or crashed?"
   - If fails (5xx / timeout) -> Kubernetes restarts the container pod.
   - Invariant: Never check external databases in /healthz. If DB is down, restarting all API pods causes cascading fail-loops.

2. Readiness Probe (/readyz): "Is the container ready to accept incoming user traffic?"
   - If fails (503) -> Kubernetes removes the pod from load balancer routing, but does NOT restart the process.
   - Invariant: Concurrently check critical dependencies (Database, Redis, Downstream Gateways).
```

### Readiness Checker Implementation in Go

```go
type ReadinessChecker struct {
	mu      sync.RWMutex
	probes  map[string]ProbeFunc
	version string
}

func (c *ReadinessChecker) Check(ctx context.Context) HealthCheckResponse {
	// Evaluates all registered dependency probes concurrently with timeout
	// Returns 200 OK with UP status if all pass, or 503 Service Unavailable if any fail.
}
```

```json
{
  "status": "UP",
  "timestamp": "2026-08-29T14:30:00Z",
  "version": "1.0.0",
  "components": {
    "database": { "status": "UP", "latencyMs": 2 },
    "payment_gateway": { "status": "UP", "latencyMs": 45 }
  }
}
```

---

## 3. Prometheus / OpenMetrics Collector

Standard Prometheus endpoint (`GET /metrics`) exposing counters and gauges:

```
# HELP http_requests_total Total number of HTTP requests
# TYPE http_requests_total counter
http_requests_total{status="2xx"} 1420
http_requests_total{status="4xx"} 18
http_requests_total{status="5xx"} 2

# HELP http_requests_in_flight Current active HTTP connections
# TYPE http_requests_in_flight gauge
http_requests_in_flight 4

# HELP http_request_duration_ms_avg Average request latency in milliseconds
# TYPE http_request_duration_ms_avg gauge
http_request_duration_ms_avg 12.40
```

---

## 4. Distributed Tracing & Correlation IDs (`X-Request-Id`)

Every inbound request must either inherit or generate a unique correlation identifier:
- Propagate `X-Request-Id` to all downstream HTTP/gRPC calls.
- Include `requestId` in every structured log line (`slog.Info`, `slog.Error`).
- Include `requestId` in RFC 7807 Problem Details error responses.

```go
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
```

---

## 5. Graceful Shutdown (Zero-Downtime Deployments)

When Kubernetes terminates a pod (`SIGTERM`), the API must stop accepting new connections and allow in-flight requests to complete cleanly before exiting.

```go
func GracefulShutdown(srv *http.Server, logger *slog.Logger, shutdownTimeout time.Duration) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	sig := <-quit
	logger.Info("shutdown_signal_received", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server_forced_shutdown", "error", err.Error())
	}
}
```
