package core

import (
	"fmt"
	"log"
	"os"
)

// DiagnosticLogger provides detailed logging for debugging
type DiagnosticLogger struct {
	enabled bool
	prefix  string
}

// NewDiagnosticLogger creates a diagnostic logger
func NewDiagnosticLogger(prefix string) *DiagnosticLogger {
	// Enable diagnostics via environment variable
	enabled := os.Getenv("GOVC_DEBUG") == "1"
	return &DiagnosticLogger{
		enabled: enabled,
		prefix:  prefix,
	}
}

// Log logs a diagnostic message
func (d *DiagnosticLogger) Log(format string, args ...interface{}) {
	if !d.enabled {
		return
	}
	msg := fmt.Sprintf(format, args...)
	log.Printf("[%s] %s", d.prefix, msg)
}

// LogError logs an error
func (d *DiagnosticLogger) LogError(operation string, err error) {
	if !d.enabled {
		return
	}
	if err != nil {
		log.Printf("[%s] ERROR in %s: %v", d.prefix, operation, err)
	}
}

// LogData logs data with a label
func (d *DiagnosticLogger) LogData(label string, data interface{}) {
	if !d.enabled {
		return
	}
	log.Printf("[%s] %s: %+v", d.prefix, label, data)
}