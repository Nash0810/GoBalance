package metrics

import (
	"context"
	"time"

	"github.com/Nash0810/gobalance/internal/backend"
	"github.com/Nash0810/gobalance/internal/retry"
)

// Exporter periodically updates metrics from system state
type Exporter struct {
	collector    *Collector
	pool         *backend.Pool
	retryBudget  *retry.Budget
}

// NewExporter creates a new metrics exporter
func NewExporter(collector *Collector, pool *backend.Pool, retryBudget *retry.Budget) *Exporter {
	return &Exporter{
		collector:   collector,
		pool:        pool,
		retryBudget: retryBudget,
	}
}

// Start begins the metrics export loop
func (e *Exporter) Start(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.export()
		}
	}
}

// export updates all gauge metrics
func (e *Exporter) export() {
	backends := e.pool.GetBackends()

	for _, b := range backends {
		backendHost := b.URL.Host

		// Backend state
		state := float64(b.GetState())
		e.collector.BackendState.WithLabelValues(backendHost).Set(state)

		// Active connections
		connections := float64(b.GetActiveRequests())
		e.collector.BackendConnections.WithLabelValues(backendHost).Set(connections)
	}

	// Retry budget
	if e.retryBudget != nil {
		tokens := float64(e.retryBudget.GetAvailable())
		e.collector.RetryBudgetTokens.Set(tokens)
	}
}
