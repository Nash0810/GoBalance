package config

import (
	"net/url"
)

// Config represents the load balancer configuration
type Config struct {
	Port           int               `yaml:"port"`            // Load balancer port
	Backends       []BackendConfig   `yaml:"backends"`        // Backend URLs with weights
	Strategy       string            `yaml:"strategy"`        // Load balancing strategy
	RequestTimeout int               `yaml:"request_timeout"` // Per-request timeout in seconds
	HealthCheck    HealthCheckConfig `yaml:"health_check"`    // Health check configuration
	Retry          RetryConfig       `yaml:"retry"`           // Retry configuration
}

// BackendConfig represents a single backend configuration
type BackendConfig struct {
	URL    string `yaml:"url"`            // Backend URL
	Weight int    `yaml:"weight,omitempty"` // Optional weight
}

// HealthCheckConfig defines health check parameters
type HealthCheckConfig struct {
	Enabled            bool   `yaml:"enabled"`             // Enable health checks
	Interval           int    `yaml:"interval"`            // Seconds between checks
	Timeout            int    `yaml:"timeout"`             // Check timeout in seconds
	HealthyThreshold   int    `yaml:"healthy_threshold"`   // Successes needed to mark healthy
	UnhealthyThreshold int    `yaml:"unhealthy_threshold"` // Failures needed to mark unhealthy
	Path               string `yaml:"path"`                // Health check endpoint path
}

// RetryConfig defines retry behavior
type RetryConfig struct {
	Enabled       bool `yaml:"enabled"`        // Enable retries
	MaxAttempts   int  `yaml:"max_attempts"`   // Total attempts (original + retries)
	BudgetPercent int  `yaml:"budget_percent"` // % of requests that can be retries
}

// ParsedBackend represents a backend with parsed URL
type ParsedBackend struct {
	URL    *url.URL
	Weight int
}

// ParseBackends converts BackendConfig to ParsedBackend
func (c *Config) ParseBackends() ([]*ParsedBackend, error) {
	var backends []*ParsedBackend
	for _, bc := range c.Backends {
		u, err := url.Parse(bc.URL)
		if err != nil {
			return nil, err
		}

		weight := bc.Weight
		if weight == 0 {
			weight = 1 // Default weight
		}

		backends = append(backends, &ParsedBackend{
			URL:    u,
			Weight: weight,
		})
	}
	return backends, nil
}
