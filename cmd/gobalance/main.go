package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/Nash0810/gobalance/internal/backend"
	"github.com/Nash0810/gobalance/internal/balancer"
	"github.com/Nash0810/gobalance/internal/config"
	"github.com/Nash0810/gobalance/internal/health"
	"github.com/Nash0810/gobalance/internal/logging"
	"github.com/Nash0810/gobalance/internal/metrics"
	"github.com/Nash0810/gobalance/internal/retry"
)

func main() {
	// Create logger
	logger := logging.NewLogger("gobalance")
	logger.Info("starting_load_balancer")

	// Load configuration
	cfg, err := config.LoadConfig("configs/config.yaml")
	if err != nil {
		logger.Error("failed_to_load_config", "error", err.Error())
		log.Fatal(err)
	}

	// Parse backends
	parsedBackends, err := cfg.ParseBackends()
	if err != nil {
		logger.Error("failed_to_parse_backends", "error", err.Error())
		log.Fatal(err)
	}

	// Create metrics collector
	collector := metrics.NewCollector()

	// Create backend pool
	pool := backend.NewPool()
	for _, pb := range parsedBackends {
		b := backend.NewBackend(pb.URL)
		b.SetWeight(pb.Weight) // Set weight from config
		pool.AddBackend(b)
		logger.Info("backend_added",
			"url", b.URL.String(),
			"weight", b.Weight)
	}

	if pool.Size() == 0 {
		logger.Error("no_backends_configured")
		log.Fatal("No backends configured")
	}

	// Create strategy based on config
	var strategy balancer.Strategy
	switch cfg.Strategy {
	case "round-robin":
		strategy = balancer.NewRoundRobinStrategy()
	case "weighted-round-robin":
		strategy = balancer.NewWeightedRoundRobinStrategy()
	case "least-connections":
		strategy = balancer.NewLeastConnectionsStrategy()
	default:
		logger.Warn("unknown_strategy_using_roundrobin",
			"strategy", cfg.Strategy)
		strategy = balancer.NewRoundRobinStrategy()
	}
	logger.Info("strategy_selected",
		"strategy", strategy.Name())

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start active health checker
	activeChecker := health.NewActiveChecker(pool, cfg.HealthCheck, collector, logger)
	go activeChecker.Start(ctx)

	// Create passive tracker
	passiveTracker := health.NewPassiveTracker(5) // 5 failures threshold

	// Create retry policy
	retryPolicy := retry.NewPolicy(cfg.Retry.MaxAttempts, cfg.Retry.BudgetPercent)
	if cfg.Retry.Enabled {
		logger.Info("retry_enabled",
			"max_attempts", cfg.Retry.MaxAttempts,
			"budget_percent", cfg.Retry.BudgetPercent)
	}

	// Log request timeout configuration (FIX #8)
	logger.Info("request_timeout_configured",
		"timeout_seconds", cfg.RequestTimeout)

	// Create balancer with metrics, logging, and timeout
	requestTimeout := time.Duration(cfg.RequestTimeout) * time.Second
	lb := balancer.NewBalancer(pool, strategy, passiveTracker, retryPolicy, requestTimeout, collector, logger)

	// Start metrics exporter
	exporter := metrics.NewExporter(collector, pool, retryPolicy.GetBudget())
	go exporter.Start(ctx)

	// Start config watcher for hot reload
	configWatcher, err := config.NewWatcher("configs/config.yaml", logger, func(newCfg *config.Config) error {
		logger.Info("applying_config_reload")

		// Parse new backends
		newBackends, err := newCfg.ParseBackends()
		if err != nil {
			return err
		}

		// Create new backend instances
		var backends []*backend.Backend
		for _, pb := range newBackends {
			b := backend.NewBackend(pb.URL)
			b.SetWeight(pb.Weight)
			backends = append(backends, b)
			logger.Info("new_backend_configured",
				"url", b.URL.String(),
				"weight", b.Weight)
		}

		// Replace backends in pool (preserves health state of existing backends)
		pool.ReplaceBackends(backends)

		logger.Info("backends_reloaded", "count", len(backends))
		return nil
	})
	if err != nil {
		logger.Error("failed_to_create_config_watcher", "error", err.Error())
	} else {
		go configWatcher.Start(ctx)
	}

	// Create HTTP server
	mux := http.NewServeMux()

	// Main proxy handler
	mux.Handle("/", lb)

	// Metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Health endpoint for load balancer itself
	mux.HandleFunc("/lb-health", func(w http.ResponseWriter, r *http.Request) {
		backends := pool.GetHealthyBackends()
		if len(backends) == 0 {
			http.Error(w, "No healthy backends", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","healthy_backends":%d}`, len(backends))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in background
	go func() {
		logger.Info("server_starting",
			"addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server_error", "error", err.Error())
			log.Fatal(err)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	logger.Info("shutdown_signal_received")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown_error", "error", err.Error())
	}

	// Cancel background contexts
	cancel()

	logger.Info("shutdown_complete")
}
