package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadConfig reads YAML file and parses it into Config struct
func LoadConfig(filepath string) (*Config, error) {
	// Read file
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if config.Port == 0 {
		config.Port = 8080
	}
	if len(config.Backends) == 0 {
		return nil, fmt.Errorf("no backends configured")
	}

	// Health check defaults
	if config.HealthCheck.Interval == 0 {
		config.HealthCheck.Interval = 5
	}
	if config.HealthCheck.Timeout == 0 {
		config.HealthCheck.Timeout = 3
	}
	if config.HealthCheck.HealthyThreshold == 0 {
		config.HealthCheck.HealthyThreshold = 2
	}
	if config.HealthCheck.UnhealthyThreshold == 0 {
		config.HealthCheck.UnhealthyThreshold = 3
	}
	if config.HealthCheck.Path == "" {
		config.HealthCheck.Path = "/health"
	}

	return &config, nil
}
