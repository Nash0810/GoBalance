package balancer

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/Nash0810/gobalance/internal/backend"
	"github.com/Nash0810/gobalance/internal/health"
)

// Balancer handles request routing
type Balancer struct {
	pool           *backend.Pool
	strategy       Strategy
	passiveTracker *health.PassiveTracker
}

// NewBalancer creates a new balancer instance
func NewBalancer(pool *backend.Pool, strategy Strategy, passiveTracker *health.PassiveTracker) *Balancer {
	return &Balancer{
		pool:           pool,
		strategy:       strategy,
		passiveTracker: passiveTracker,
	}
}

// ServeHTTP implements http.Handler interface
func (lb *Balancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	backend := lb.strategy.SelectBackend(lb.pool)

	if backend == nil {
		log.Printf("No healthy backends available")
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	log.Printf("Routing request to: %s (state: %s)",
		backend.URL.String(), backend.GetState())

	backend.IncrementActiveRequests()
	defer backend.DecrementActiveRequests()

	// Create a custom response writer to capture errors
	crw := &captureResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	// Forward request
	backend.ReverseProxy.ServeHTTP(crw, r)

	// Track success/failure
	if crw.statusCode >= 500 {
		lb.passiveTracker.RecordFailure(backend, fmt.Errorf("status %d", crw.statusCode))
	} else {
		lb.passiveTracker.RecordSuccess(backend)
	}
}

// captureResponseWriter captures the status code (FIX: Added mutex for thread-safety)
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
