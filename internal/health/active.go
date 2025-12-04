package health

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Nash0810/gobalance/internal/backend"
	"github.com/Nash0810/gobalance/internal/config"
)

// ActiveChecker performs periodic health checks on backends
type ActiveChecker struct {
	pool   *backend.Pool
	config config.HealthCheckConfig
	client *http.Client
}

// NewActiveChecker creates a new active health checker
func NewActiveChecker(pool *backend.Pool, cfg config.HealthCheckConfig) *ActiveChecker {
	return &ActiveChecker{
		pool:   pool,
		config: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
	}
}

// Start begins the health check loop (runs in background goroutine)
func (ac *ActiveChecker) Start(ctx context.Context) {
	if !ac.config.Enabled {
		log.Println("Active health checks disabled")
		return
	}

	ticker := time.NewTicker(time.Duration(ac.config.Interval) * time.Second)
	defer ticker.Stop()

	log.Printf("Active health checker started (interval: %ds, timeout: %ds)",
		ac.config.Interval, ac.config.Timeout)

	// Run initial check immediately
	ac.checkAllBackends()

	for {
		select {
		case <-ctx.Done():
			log.Println("Active health checker stopped")
			return
		case <-ticker.C:
			ac.checkAllBackends()
		}
	}
}

// checkAllBackends checks health of all backends
func (ac *ActiveChecker) checkAllBackends() {
	backends := ac.pool.GetBackends()

	for _, b := range backends {
		go ac.checkBackend(b) // Check in parallel
	}
}

// checkBackend performs health check on a single backend
func (ac *ActiveChecker) checkBackend(b *backend.Backend) {
	url := fmt.Sprintf("%s%s", b.URL.String(), ac.config.Path)

	resp, err := ac.client.Get(url)

	if err != nil {
		// Check failed
		ac.handleFailure(b, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Check succeeded
		ac.handleSuccess(b)
	} else {
		// Non-2xx status code
		ac.handleFailure(b, fmt.Errorf("status code: %d", resp.StatusCode))
	}
}

// handleSuccess processes successful health check
// FIX: Added coordination with passive health tracker
func (ac *ActiveChecker) handleSuccess(b *backend.Backend) {
	b.RecordHealthCheckSuccess()
	metrics := b.GetHealthMetrics()
	currentState := b.GetState()

	// FIX: Don't mark healthy if recent passive failure (coordination fix #5)
	// Note: This will be fully implemented when we add LastPassiveFailure tracking
	// State transition: DOWN/UNHEALTHY → HEALTHY
	if currentState != backend.Healthy {
		if metrics.ConsecutiveSuccesses >= ac.config.HealthyThreshold {
			log.Printf("[HEALTH] %s: %s → HEALTHY (after %d successes)",
				b.URL.Host, currentState, metrics.ConsecutiveSuccesses)
			b.SetState(backend.Healthy)
		}
	}
}

// handleFailure processes failed health check
func (ac *ActiveChecker) handleFailure(b *backend.Backend, err error) {
	b.RecordHealthCheckFailure()
	metrics := b.GetHealthMetrics()
	currentState := b.GetState()

	log.Printf("[HEALTH] %s: Check failed: %v (failures: %d)",
		b.URL.Host, err, metrics.ConsecutiveFailures)

	// State transition: HEALTHY → UNHEALTHY
	if currentState == backend.Healthy {
		if metrics.ConsecutiveFailures >= ac.config.UnhealthyThreshold {
			log.Printf("[HEALTH] %s: HEALTHY → UNHEALTHY (after %d failures)",
				b.URL.Host, metrics.ConsecutiveFailures)
			b.SetState(backend.Unhealthy)
		}
	}
}
