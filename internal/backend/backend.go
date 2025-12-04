package backend

import (
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// Backend represents a single backend server
type Backend struct {
	URL            *url.URL               // Backend URL
	alive          bool                   // Health status (protected by mutex)
	state          HealthState            // Current health state
	metrics        HealthMetrics          // Health check metrics
	mux            sync.RWMutex           // Protects 'alive', 'state', 'metrics'
	ReverseProxy   *httputil.ReverseProxy // HTTP proxy
	ActiveRequests int64                  // Active request count (atomic)
	Weight         int                    // Weight for weighted strategies (1-100)
}

// NewBackend creates a new backend instance
func NewBackend(u *url.URL) *Backend {
	return &Backend{
		URL:            u,
		alive:          true,
		state:          Healthy,
		metrics:        HealthMetrics{},
		ReverseProxy:   httputil.NewSingleHostReverseProxy(u),
		ActiveRequests: 0,
		Weight:         1, // Default weight
	}
}

// IsAlive returns the backend's health status (thread-safe)
func (b *Backend) IsAlive() bool {
	b.mux.RLock()
	defer b.mux.RUnlock()
	return b.alive
}

// SetAlive sets the backend's health status (thread-safe)
func (b *Backend) SetAlive(alive bool) {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.alive = alive
}

// GetState returns the current health state (thread-safe)
func (b *Backend) GetState() HealthState {
	b.mux.RLock()
	defer b.mux.RUnlock()
	return b.state
}

// SetState sets the health state (thread-safe)
func (b *Backend) SetState(state HealthState) {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.state = state

	// Update alive flag based on state
	b.alive = (state == Healthy)
}

// RecordHealthCheckSuccess records a successful health check
func (b *Backend) RecordHealthCheckSuccess() {
	b.mux.Lock()
	defer b.mux.Unlock()

	b.metrics.ConsecutiveSuccesses++
	b.metrics.ConsecutiveFailures = 0
	b.metrics.LastCheck = time.Now()
	b.metrics.LastSuccess = time.Now()
}

// RecordHealthCheckFailure records a failed health check
func (b *Backend) RecordHealthCheckFailure() {
	b.mux.Lock()
	defer b.mux.Unlock()

	b.metrics.ConsecutiveFailures++
	b.metrics.ConsecutiveSuccesses = 0
	b.metrics.LastCheck = time.Now()
	b.metrics.LastFailure = time.Now()
}

// GetHealthMetrics returns a copy of health metrics (thread-safe)
func (b *Backend) GetHealthMetrics() HealthMetrics {
	b.mux.RLock()
	defer b.mux.RUnlock()
	return b.metrics
}

// IncrementActiveRequests atomically increments active request count
func (b *Backend) IncrementActiveRequests() {
	atomic.AddInt64(&b.ActiveRequests, 1)
}

// DecrementActiveRequests atomically decrements active request count
func (b *Backend) DecrementActiveRequests() {
	atomic.AddInt64(&b.ActiveRequests, -1)
}

// GetActiveRequests atomically reads active request count
func (b *Backend) GetActiveRequests() int64 {
	return atomic.LoadInt64(&b.ActiveRequests)
}

// SetWeight sets the backend weight
func (b *Backend) SetWeight(weight int) {
	if weight < 1 {
		weight = 1
	}
	if weight > 100 {
		weight = 100
	}
	b.Weight = weight
}
