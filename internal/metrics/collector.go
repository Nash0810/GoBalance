package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Collector holds all Prometheus metrics
type Collector struct {
	// Request metrics
	RequestsTotal       *prometheus.CounterVec
	RequestDuration     *prometheus.HistogramVec
	ActiveRequests      *prometheus.GaugeVec

	// Backend metrics
	BackendState        *prometheus.GaugeVec
	BackendConnections  *prometheus.GaugeVec
	CircuitBreakerState *prometheus.GaugeVec

	// Health check metrics
	HealthCheckTotal    *prometheus.CounterVec
	HealthCheckDuration *prometheus.HistogramVec

	// Retry metrics
	RetriesTotal        *prometheus.CounterVec
	RetryBudgetTokens   prometheus.Gauge
}

// NewCollector creates and registers all metrics
func NewCollector() *Collector {
	return &Collector{
		RequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gobalance_requests_total",
				Help: "Total number of requests",
			},
			[]string{"backend", "method", "status"},
		),

		RequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gobalance_request_duration_seconds",
				Help:    "Request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"backend", "method"},
		),

		ActiveRequests: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gobalance_active_requests",
				Help: "Number of active requests per backend",
			},
			[]string{"backend"},
		),

		BackendState: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gobalance_backend_state",
				Help: "Backend health state (0=DOWN, 1=UNHEALTHY, 2=DRAINING, 3=HEALTHY)",
			},
			[]string{"backend"},
		),

		BackendConnections: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gobalance_backend_connections",
				Help: "Active connections per backend",
			},
			[]string{"backend"},
		),

		CircuitBreakerState: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gobalance_circuit_breaker_state",
				Help: "Circuit breaker state (0=CLOSED, 1=HALF_OPEN, 2=OPEN)",
			},
			[]string{"backend"},
		),

		HealthCheckTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gobalance_health_checks_total",
				Help: "Total number of health checks",
			},
			[]string{"backend", "result"},
		),

		HealthCheckDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gobalance_health_check_duration_seconds",
				Help:    "Health check duration in seconds",
				Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
			},
			[]string{"backend"},
		),

		RetriesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gobalance_retries_total",
				Help: "Total number of retries",
			},
			[]string{"reason"},
		),

		RetryBudgetTokens: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "gobalance_retry_budget_tokens",
				Help: "Available retry budget tokens",
			},
		),
	}
}
