package health

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Nash0810/gobalance/internal/backend"
	"github.com/Nash0810/gobalance/internal/config"
	"github.com/Nash0810/gobalance/internal/logging"
	"github.com/Nash0810/gobalance/internal/metrics"
)

// ActiveChecker performs periodic health checks on backends
type ActiveChecker struct {
	pool       *backend.Pool
	config     config.HealthCheckConfig
	client     *http.Client
	collector  *metrics.Collector   // Prometheus metrics
	logger     *logging.Logger      // Structured logger
}

// NewActiveChecker creates a new active health checker
func NewActiveChecker(pool *backend.Pool, cfg config.HealthCheckConfig, 
	collector *metrics.Collector, logger *logging.Logger) *ActiveChecker {
	return &ActiveChecker{
		pool:      pool,
		config:    cfg,
		client:    &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		collector: collector,
		logger:    logger,
	}
}

// Start begins the health check loop (runs in background goroutine)
func (ac *ActiveChecker) Start(ctx context.Context) {
	if !ac.config.Enabled {
		ac.logger.Info("active_health_checks_disabled")
		return
	}

	ticker := time.NewTicker(time.Duration(ac.config.Interval) * time.Second)
	defer ticker.Stop()

	ac.logger.Info("active_health_checker_started",
		"interval_seconds", ac.config.Interval,
		"timeout_seconds", ac.config.Timeout)

	// Run initial check immediately
	ac.checkAllBackends()

	for {
		select {
		case <-ctx.Done():
			ac.logger.Info("active_health_checker_stopped")
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
	startTime := time.Now()

	resp, err := ac.client.Get(url)
	duration := time.Since(startTime).Seconds()

	if ac.collector != nil {
		ac.collector.HealthCheckTotal.WithLabelValues(b.URL.Host, "attempt").Inc()
		ac.collector.HealthCheckDuration.WithLabelValues(b.URL.Host).Observe(duration)
	}

	if err != nil {
		// Check failed
		ac.handleFailure(b, err)
		if ac.collector != nil {
			ac.collector.HealthCheckTotal.WithLabelValues(b.URL.Host, "failure").Inc()
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Check succeeded
		ac.handleSuccess(b)
		if ac.collector != nil {
			ac.collector.HealthCheckTotal.WithLabelValues(b.URL.Host, "success").Inc()
		}
	} else {
		// Non-2xx status code
		ac.handleFailure(b, fmt.Errorf("status code: %d", resp.StatusCode))
		if ac.collector != nil {
			ac.collector.HealthCheckTotal.WithLabelValues(b.URL.Host, "failure").Inc()
		}
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
			ac.logger.Info("health_check_passed_state_transition",
				"backend", b.URL.Host,
				"old_state", currentState,
				"new_state", "HEALTHY",
				"consecutive_successes", metrics.ConsecutiveSuccesses)
			b.SetState(backend.Healthy)
		}
	}
}

// handleFailure processes failed health check
func (ac *ActiveChecker) handleFailure(b *backend.Backend, err error) {
	b.RecordHealthCheckFailure()
	metrics := b.GetHealthMetrics()
	currentState := b.GetState()

	ac.logger.Warn("health_check_failed",
		"backend", b.URL.Host,
		"error", err.Error(),
		"consecutive_failures", metrics.ConsecutiveFailures)

	// State transition: HEALTHY → UNHEALTHY
	if currentState == backend.Healthy {
		if metrics.ConsecutiveFailures >= ac.config.UnhealthyThreshold {
			ac.logger.Warn("health_state_transition",
				"backend", b.URL.Host,
				"old_state", currentState,
				"new_state", "UNHEALTHY",
				"consecutive_failures", metrics.ConsecutiveFailures)
			b.SetState(backend.Unhealthy)
		}
	}
}
