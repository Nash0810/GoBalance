package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/Nash0810/gobalance/internal/backend"
	"github.com/Nash0810/gobalance/internal/balancer"
	"github.com/Nash0810/gobalance/internal/config"
	"github.com/Nash0810/gobalance/internal/health"
	"github.com/Nash0810/gobalance/internal/retry"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("configs/config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	// Parse backends
	parsedBackends, err := cfg.ParseBackends()
	if err != nil {
		log.Fatal(err)
	}

	// Create backend pool
	pool := backend.NewPool()
	for _, pb := range parsedBackends {
		b := backend.NewBackend(pb.URL)
		b.SetWeight(pb.Weight) // Set weight from config
		pool.AddBackend(b)
		log.Printf("Added backend: %s (weight: %d)", b.URL.String(), b.Weight)
	}

	if pool.Size() == 0 {
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
		log.Printf("Unknown strategy '%s', using round-robin", cfg.Strategy)
		strategy = balancer.NewRoundRobinStrategy()
	}
	log.Printf("Using strategy: %s", strategy.Name())

	// Start active health checker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	activeChecker := health.NewActiveChecker(pool, cfg.HealthCheck)
	go activeChecker.Start(ctx)

	// Create passive tracker
	passiveTracker := health.NewPassiveTracker(5) // 5 failures threshold

	// Create retry policy
	retryPolicy := retry.NewPolicy(cfg.Retry.MaxAttempts, cfg.Retry.BudgetPercent)
	if cfg.Retry.Enabled {
		log.Printf("Retry enabled: max_attempts=%d, budget=%d%%",
			cfg.Retry.MaxAttempts, cfg.Retry.BudgetPercent)
	}

	// Create balancer
	lb := balancer.NewBalancer(pool, strategy, passiveTracker, retryPolicy)

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Starting load balancer on %s", addr)
	log.Fatal(http.ListenAndServe(addr, lb))
}
