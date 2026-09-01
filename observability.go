package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// HealthStatus represents the health state of the service or a dependency.
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "UP"
	StatusDegraded  HealthStatus = "DEGRADED"
	StatusUnhealthy HealthStatus = "DOWN"
)

// ComponentCheck represents the result of checking an individual dependency.
type ComponentCheck struct {
	Status    HealthStatus `json:"status"`
	LatencyMs int64        `json:"latencyMs"`
	Error     string       `json:"error,omitempty"`
}

// HealthCheckResponse is returned by the /readyz endpoint.
type HealthCheckResponse struct {
	Status     HealthStatus              `json:"status"`
	Timestamp  time.Time                 `json:"timestamp"`
	Version    string                    `json:"version"`
	Components map[string]ComponentCheck `json:"components"`
}

// ProbeFunc is a function that checks a subsystem (e.g., database, cache, downstream API).
type ProbeFunc func(ctx context.Context) error

// ReadinessChecker coordinates health probes across external dependencies.
type ReadinessChecker struct {
	mu      sync.RWMutex
	probes  map[string]ProbeFunc
	version string
}

// NewReadinessChecker creates a readiness coordinator.
func NewReadinessChecker(version string) *ReadinessChecker {
	return &ReadinessChecker{
		probes:  make(map[string]ProbeFunc),
		version: version,
	}
}

// RegisterProbe adds a new dependency probe.
func (c *ReadinessChecker) RegisterProbe(name string, probe ProbeFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.probes[name] = probe
}

// Check evaluates all registered probes concurrently with a timeout.
func (c *ReadinessChecker) Check(ctx context.Context) HealthCheckResponse {
	c.mu.RLock()
	probeList := make(map[string]ProbeFunc, len(c.probes))
	for k, v := range c.probes {
		probeList[k] = v
	}
	c.mu.RUnlock()

	resp := HealthCheckResponse{
		Status:     StatusHealthy,
		Timestamp:  time.Now().UTC(),
		Version:    c.version,
		Components: make(map[string]ComponentCheck, len(probeList)),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, probe := range probeList {
		wg.Add(1)
		go func(compName string, fn ProbeFunc) {
			defer wg.Done()
			start := time.Now()

			probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			err := fn(probeCtx)
			latency := time.Since(start).Milliseconds()

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				resp.Status = StatusUnhealthy
				resp.Components[compName] = ComponentCheck{
					Status:    StatusUnhealthy,
					LatencyMs: latency,
					Error:     err.Error(),
				}
			} else {
				resp.Components[compName] = ComponentCheck{
					Status:    StatusHealthy,
					LatencyMs: latency,
				}
			}
		}(name, probe)
	}

	wg.Wait()
	return resp
}

// Handler returns an HTTP handler for /readyz.
func (c *ReadinessChecker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := c.Check(r.Context())
		statusCode := http.StatusOK
		if result.Status == StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(result)
	}
}

// APIMetrics tracks in-memory metrics for HTTP requests.
type APIMetrics struct {
	inFlightRequests atomic.Int64
	totalRequests    atomic.Uint64
	status2xx        atomic.Uint64
	status3xx        atomic.Uint64
	status4xx        atomic.Uint64
	status5xx        atomic.Uint64
	totalLatencyMs   atomic.Uint64
}

// NewAPIMetrics creates a metrics collector.
func NewAPIMetrics() *APIMetrics {
	return &APIMetrics{}
}

// Middleware records metrics for each incoming request.
func (m *APIMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.inFlightRequests.Add(1)
		m.totalRequests.Add(1)
		start := time.Now()

		rw := &statusRecordingWriter{ResponseWriter: w, statusCode: http.StatusOK}
		defer func() {
			m.inFlightRequests.Add(-1)
			elapsed := time.Since(start).Milliseconds()
			m.totalLatencyMs.Add(uint64(elapsed))

			switch {
			case rw.statusCode >= 200 && rw.statusCode < 300:
				m.status2xx.Add(1)
			case rw.statusCode >= 300 && rw.statusCode < 400:
				m.status3xx.Add(1)
			case rw.statusCode >= 400 && rw.statusCode < 500:
				m.status4xx.Add(1)
			case rw.statusCode >= 500:
				m.status5xx.Add(1)
			}
		}()

		next.ServeHTTP(rw, r)
	})
}

// Handler returns a Prometheus/OpenMetrics formatted endpoint for /metrics.
func (m *APIMetrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		totReq := m.totalRequests.Load()
		totLat := m.totalLatencyMs.Load()
		var avgLat float64
		if totReq > 0 {
			avgLat = float64(totLat) / float64(totReq)
		}

		fmt.Fprintf(w, "# HELP http_requests_total Total number of HTTP requests\n")
		fmt.Fprintf(w, "# TYPE http_requests_total counter\n")
		fmt.Fprintf(w, "http_requests_total{status=\"2xx\"} %d\n", m.status2xx.Load())
		fmt.Fprintf(w, "http_requests_total{status=\"3xx\"} %d\n", m.status3xx.Load())
		fmt.Fprintf(w, "http_requests_total{status=\"4xx\"} %d\n", m.status4xx.Load())
		fmt.Fprintf(w, "http_requests_total{status=\"5xx\"} %d\n", m.status5xx.Load())
		fmt.Fprintf(w, "\n# HELP http_requests_in_flight Current number of active HTTP requests\n")
		fmt.Fprintf(w, "# TYPE http_requests_in_flight gauge\n")
		fmt.Fprintf(w, "http_requests_in_flight %d\n", m.inFlightRequests.Load())
		fmt.Fprintf(w, "\n# HELP http_request_duration_ms_avg Average request latency in milliseconds\n")
		fmt.Fprintf(w, "# TYPE http_request_duration_ms_avg gauge\n")
		fmt.Fprintf(w, "http_request_duration_ms_avg %.2f\n", avgLat)
	}
}

type statusRecordingWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusRecordingWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// GracefulShutdown blocks until an interrupt signal is received, then gracefully shuts down the server.
func GracefulShutdown(srv *http.Server, logger *slog.Logger, shutdownTimeout time.Duration) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	sig := <-quit
	logger.Info("shutdown_signal_received", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server_forced_shutdown", "error", err.Error())
	} else {
		logger.Info("server_gracefully_stopped")
	}
}
