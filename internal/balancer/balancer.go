package balancer

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/Nash0810/gobalance/internal/backend"
	"github.com/Nash0810/gobalance/internal/health"
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
}

// NewBalancer creates a new balancer instance
func NewBalancer(pool *backend.Pool, strategy Strategy, passiveTracker *health.PassiveTracker, retryPolicy *retry.Policy) *Balancer {
	return &Balancer{
		pool:            pool,
		strategy:        strategy,
		passiveTracker:  passiveTracker,
		retryPolicy:     retryPolicy,
		circuitBreakers: make(map[string]*health.CircuitBreaker),
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
	// FIX #2: Buffer request body for potential retries
	var bodyBytes []byte
	var err error
	if lb.retryPolicy != nil && r.Body != nil {
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Failed to buffer request body: %v", err)
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
			log.Printf("[REQUEST] Client canceled request")
			http.Error(w, "Request Canceled", 499)
			return
		}

		backend := lb.strategy.SelectBackend(lb.pool)

		if backend == nil {
			log.Printf("No healthy backends available")
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

		// Get circuit breaker for this backend
		cb := lb.getCircuitBreaker(backend)

		// Check circuit breaker
		if !cb.AllowRequest() {
			log.Printf("[CIRCUIT] %s: Circuit OPEN, trying next backend", backend.URL.Host)
			if attempt < maxAttempts {
				continue // Try different backend
			}
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

		log.Printf("Routing request to: %s (attempt %d, state: %s, circuit: %s)",
			backend.URL.String(), attempt, backend.GetState(), cb.GetState())

		backend.IncrementActiveRequests()

		// Create a custom response writer to capture errors
		crw := &captureResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// FIX #2: Restore body for retry attempts
		if bodyBytes != nil && attempt > 1 {
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// Forward request
		backend.ReverseProxy.ServeHTTP(crw, r)

		backend.DecrementActiveRequests()

		// Check if request succeeded
		if crw.statusCode >= 500 {
			err := fmt.Errorf("status %d", crw.statusCode)
			lb.passiveTracker.RecordFailure(backend, err)
			cb.RecordFailure()

			// Should retry?
			if lb.retryPolicy != nil && lb.retryPolicy.ShouldRetry(r, err, attempt) {
				log.Printf("[RETRY] Retrying request (attempt %d)", attempt+1)
				continue
			}

			return // Don't retry
		}

		// Success
		lb.passiveTracker.RecordSuccess(backend)
		cb.RecordSuccess()
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
