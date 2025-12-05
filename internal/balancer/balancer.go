package balancer

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/Nash0810/gobalance/internal/backend"
	"github.com/Nash0810/gobalance/internal/health"
	"github.com/Nash0810/gobalance/internal/logging"
	"github.com/Nash0810/gobalance/internal/metrics"
	"github.com/Nash0810/gobalance/internal/retry"
)

// Balancer handles request routing
type Balancer struct {
	pool            *backend.Pool
	strategy        Strategy
	passiveTracker  *health.PassiveTracker
	retryPolicy     *retry.Policy
	circuitBreakers map[string]*health.CircuitBreaker // Per-backend circuit breakers
	cbMux           sync.RWMutex                      // Protects circuit breakers map
	collector       *metrics.Collector                // Prometheus metrics
	logger          *logging.Logger                   // Structured logger
}

// NewBalancer creates a new balancer instance
func NewBalancer(pool *backend.Pool, strategy Strategy, passiveTracker *health.PassiveTracker, retryPolicy *retry.Policy, collector *metrics.Collector, logger *logging.Logger) *Balancer {
	return &Balancer{
		pool:            pool,
		strategy:        strategy,
		passiveTracker:  passiveTracker,
		retryPolicy:     retryPolicy,
		circuitBreakers: make(map[string]*health.CircuitBreaker),
		collector:       collector,
		logger:          logger,
	}
}

// getCircuitBreaker gets or creates a circuit breaker for a backend
func (lb *Balancer) getCircuitBreaker(backend *backend.Backend) *health.CircuitBreaker {
	key := backend.URL.Host

	lb.cbMux.RLock()
	cb, exists := lb.circuitBreakers[key]
	lb.cbMux.RUnlock()

	if exists {
		return cb
	}

	lb.cbMux.Lock()
	defer lb.cbMux.Unlock()

	// Double-check after acquiring write lock
	if cb, exists := lb.circuitBreakers[key]; exists {
		return cb
	}

	// Create new circuit breaker
	cb = health.NewCircuitBreaker(key)
	lb.circuitBreakers[key] = cb
	return cb
}

// ServeHTTP implements http.Handler interface
// Incorporates FIX #2 (body buffering), FIX #4 (context propagation)
func (lb *Balancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Generate request ID
	requestID := uuid.New().String()
	r.Header.Set("X-Request-ID", requestID)

	startTime := time.Now()

	// FIX #2: Buffer request body for potential retries
	var bodyBytes []byte
	var err error
	if lb.retryPolicy != nil && r.Body != nil {
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			lb.logger.Error("failed_to_buffer_body",
				"request_id", requestID,
				"error", err.Error())
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		r.Body.Close()
		// Restore body for first attempt
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	maxAttempts := 1
	if lb.retryPolicy != nil {
		lb.retryPolicy.GetBudget().TrackRequest() // Track for adaptive budget
		maxAttempts = 3 // Allow up to 3 total attempts (original + 2 retries)
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// FIX #4: Check if client canceled request
		if r.Context().Err() != nil {
			lb.logger.Warn("client_canceled_request", "request_id", requestID)
			http.Error(w, "Request Canceled", 499)
			return
		}

		backend := lb.strategy.SelectBackend(lb.pool)

		if backend == nil {
			lb.logger.Error("no_healthy_backends_available", "request_id", requestID)
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

		// Get circuit breaker for this backend
		cb := lb.getCircuitBreaker(backend)
		backendHost := backend.URL.Host

		// Check circuit breaker
		if !cb.AllowRequest() {
			lb.logger.Warn("circuit_open",
				"request_id", requestID,
				"backend", backendHost,
				"attempt", attempt)

			if lb.collector != nil {
				lb.collector.RetriesTotal.WithLabelValues("circuit_open").Inc()
			}

			if attempt < maxAttempts {
				continue // Try different backend
			}
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

		lb.logger.Info("routing_request",
			"request_id", requestID,
			"backend", backendHost,
			"attempt", attempt,
			"method", r.Method,
			"path", r.URL.Path)

		backend.IncrementActiveRequests()
		if lb.collector != nil {
			lb.collector.ActiveRequests.WithLabelValues(backendHost).Inc()
		}

		// Create a custom response writer to capture errors
		crw := &captureResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// FIX #2: Restore body for retry attempts
		if bodyBytes != nil && attempt > 1 {
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// Forward request
		backend.ReverseProxy.ServeHTTP(crw, r)

		backend.DecrementActiveRequests()
		if lb.collector != nil {
			lb.collector.ActiveRequests.WithLabelValues(backendHost).Dec()
		}

		duration := time.Since(startTime).Seconds()
		statusStr := strconv.Itoa(crw.statusCode)

		// Record metrics
		if lb.collector != nil {
			lb.collector.RequestsTotal.WithLabelValues(backendHost, r.Method, statusStr).Inc()
			lb.collector.RequestDuration.WithLabelValues(backendHost, r.Method).Observe(duration)
		}

		// Check if request succeeded
		if crw.statusCode >= 500 {
			err := fmt.Errorf("status %d", crw.statusCode)
			lb.passiveTracker.RecordFailure(backend, err)
			cb.RecordFailure()

			lb.logger.Warn("request_failed",
				"request_id", requestID,
				"backend", backendHost,
				"status", crw.statusCode,
				"duration_ms", duration*1000)

			// Should retry?
			if lb.retryPolicy != nil && lb.retryPolicy.ShouldRetry(r, err, attempt) {
				if lb.collector != nil {
					lb.collector.RetriesTotal.WithLabelValues("server_error").Inc()
				}
				continue
			}

			return // Don't retry
		}

		// Success
		lb.passiveTracker.RecordSuccess(backend)
		cb.RecordSuccess()

		lb.logger.Info("request_completed",
			"request_id", requestID,
			"backend", backendHost,
			"status", crw.statusCode,
			"duration_ms", duration*1000)

		return
	}
}

// captureResponseWriter captures the status code (FIX #1: Added mutex for thread-safety)
type captureResponseWriter struct {
	http.ResponseWriter
	statusCode int
	mu         sync.Mutex
}

func (crw *captureResponseWriter) WriteHeader(code int) {
	crw.mu.Lock()
	crw.statusCode = code
	crw.mu.Unlock()
	crw.ResponseWriter.WriteHeader(code)
}
