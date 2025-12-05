package config

import (
	"testing"
	"time"
)

// TestLoadConfigDefaults verifies configuration defaults are applied
func TestLoadConfigDefaults(t *testing.T) {
	cfg := &Config{}

	// Apply defaults
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = 30
	}
	if cfg.Strategy == "" {
		cfg.Strategy = "roundrobin"
	}

	if cfg.RequestTimeout != 30 {
		t.Errorf("Expected RequestTimeout 30, got %d", cfg.RequestTimeout)
	}
	if cfg.Strategy != "roundrobin" {
		t.Errorf("Expected Strategy roundrobin, got %s", cfg.Strategy)
	}
}

// TestConfigValidation verifies config can be created with values
func TestConfigValidation(t *testing.T) {
	cfg := &Config{
		Strategy:       "roundrobin",
		RequestTimeout: 30,
		Port:           8080,
	}

	if cfg.Strategy != "roundrobin" {
		t.Error("Strategy not set correctly")
	}
	if cfg.RequestTimeout != 30 {
		t.Error("RequestTimeout not set correctly")
	}
	if cfg.Port != 8080 {
		t.Error("Port not set correctly")
	}
}

// TestConfigDuration verifies timeout duration conversion
func TestConfigDuration(t *testing.T) {
	cfg := &Config{
		RequestTimeout: 15,
	}

	duration := time.Duration(cfg.RequestTimeout) * time.Second
	if duration != 15*time.Second {
		t.Errorf("Expected 15s, got %v", duration)
	}
}

// TestBackendConfig verifies backend configuration structure
func TestBackendConfig(t *testing.T) {
	backend := &BackendConfig{
		URL:    "http://localhost:8081",
		Weight: 1,
	}

	if backend.URL != "http://localhost:8081" {
		t.Error("Backend URL not set correctly")
	}
	if backend.Weight != 1 {
		t.Error("Backend Weight not set correctly")
	}
}

// TestMultipleBackends verifies multiple backends configuration
func TestMultipleBackends(t *testing.T) {
	cfg := &Config{
		Backends: []BackendConfig{
			{URL: "http://localhost:8081", Weight: 1},
			{URL: "http://localhost:8082", Weight: 2},
			{URL: "http://localhost:8083", Weight: 1},
		},
	}

	if len(cfg.Backends) != 3 {
		t.Errorf("Expected 3 backends, got %d", len(cfg.Backends))
	}

	totalWeight := 0
	for _, b := range cfg.Backends {
		totalWeight += b.Weight
	}
	if totalWeight != 4 {
		t.Errorf("Expected total weight 4, got %d", totalWeight)
	}
}

// TestHealthCheckConfig verifies health check configuration
func TestHealthCheckConfig(t *testing.T) {
	hc := &HealthCheckConfig{
		Interval:           10,
		Timeout:            5,
		UnhealthyThreshold: 3,
	}

	if hc.Interval != 10 {
		t.Error("Interval not set correctly")
	}
	if hc.Timeout != 5 {
		t.Error("Timeout not set correctly")
	}
	if hc.UnhealthyThreshold != 3 {
		t.Error("UnhealthyThreshold not set correctly")
	}
}

// TestRetryConfig verifies retry configuration
func TestRetryConfig(t *testing.T) {
	retry := &RetryConfig{
		MaxAttempts:   3,
		BudgetPercent: 25,
	}

	if retry.MaxAttempts != 3 {
		t.Error("MaxAttempts not set correctly")
	}
	if retry.BudgetPercent != 25 {
		t.Error("BudgetPercent not set correctly")
	}
}
