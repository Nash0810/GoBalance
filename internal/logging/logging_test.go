package logging

import (
	"testing"
)

// TestLoggerCreation verifies logger can be created with prefix
func TestLoggerCreation(t *testing.T) {
	logger := NewLogger("test")
	if logger == nil {
		t.Error("Logger creation failed")
	}
	if logger.prefix != "test" {
		t.Errorf("Expected prefix 'test', got '%s'", logger.prefix)
	}
}

// TestLoggerInfo verifies info logging doesn't panic
func TestLoggerInfo(t *testing.T) {
	logger := NewLogger("test")
	// Should not panic
	logger.Info("test message", "key", "value")
}

// TestLoggerWarn verifies warn logging doesn't panic
func TestLoggerWarn(t *testing.T) {
	logger := NewLogger("test")
	// Should not panic
	logger.Warn("test warning", "key", "value")
}

// TestLoggerError verifies error logging doesn't panic
func TestLoggerError(t *testing.T) {
	logger := NewLogger("test")
	// Should not panic
	logger.Error("test error", "key", "value")
}

// TestLoggerMultipleKeyValues verifies multiple key-value pairs
func TestLoggerMultipleKeyValues(t *testing.T) {
	logger := NewLogger("balancer")
	// Should not panic with multiple key-value pairs
	logger.Info("request processed", "id", "abc123", "status", 200, "duration", "45ms")
}
