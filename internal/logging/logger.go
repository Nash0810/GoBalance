package logging

import (
	"fmt"
	"log"
	"time"
)

// Logger provides structured logging
type Logger struct {
	prefix string
}

// NewLogger creates a new logger with prefix
func NewLogger(prefix string) *Logger {
	return &Logger{prefix: prefix}
}

// Info logs informational message with key-value pairs
func (l *Logger) Info(msg string, keysAndValues ...interface{}) {
	l.log("INFO", msg, keysAndValues...)
}

// Warn logs warning message
func (l *Logger) Warn(msg string, keysAndValues ...interface{}) {
	l.log("WARN", msg, keysAndValues...)
}

// Error logs error message
func (l *Logger) Error(msg string, keysAndValues ...interface{}) {
	l.log("ERROR", msg, keysAndValues...)
}

// log formats and outputs log message
func (l *Logger) log(level string, msg string, keysAndValues ...interface{}) {
	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z07:00")

	output := fmt.Sprintf("%s [%s] %s: %s", timestamp, level, l.prefix, msg)

	// Append key-value pairs
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := keysAndValues[i]
			value := keysAndValues[i+1]
			output += fmt.Sprintf(" %v=%v", key, value)
		}
	}

	log.Println(output)
}
