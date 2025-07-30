package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LogLevel represents the severity level of a log entry
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	case FatalLevel:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	Level       LogLevel               `json:"level"`
	Message     string                 `json:"message"`
	Fields      map[string]interface{} `json:"fields,omitempty"`
	Caller      string                 `json:"caller,omitempty"`
	RequestID   string                 `json:"request_id,omitempty"`
	UserID      string                 `json:"user_id,omitempty"`
	Component   string                 `json:"component,omitempty"`
	Operation   string                 `json:"operation,omitempty"`
	Duration    *int64                 `json:"duration_ms,omitempty"`
	Error       string                 `json:"error,omitempty"`
	HTTPMethod  string                 `json:"http_method,omitempty"`
	HTTPPath    string                 `json:"http_path,omitempty"`
	HTTPStatus  int                    `json:"http_status,omitempty"`
	HTTPLatency *int64                 `json:"http_latency_ms,omitempty"`
}

// Logger represents a structured logger
type Logger struct {
	level     LogLevel
	output    io.Writer
	component string
	fields    map[string]interface{}
	mu        sync.RWMutex
}

// Config represents logger configuration
type Config struct {
	Level       LogLevel
	Output      io.Writer
	Component   string
	EnableCaller bool
	TimeFormat  string
}

// NewLogger creates a new structured logger
func NewLogger(config Config) *Logger {
	if config.Output == nil {
		config.Output = os.Stdout
	}
	
	if config.TimeFormat == "" {
		config.TimeFormat = time.RFC3339
	}

	return &Logger{
		level:     config.Level,
		output:    config.Output,
		component: config.Component,
		fields:    make(map[string]interface{}),
	}
}

// WithField adds a field to the logger context
func (l *Logger) WithField(key string, value interface{}) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	newLogger := &Logger{
		level:     l.level,
		output:    l.output,
		component: l.component,
		fields:    make(map[string]interface{}),
	}

	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}

	// Add new field
	newLogger.fields[key] = value

	return newLogger
}

// WithFields adds multiple fields to the logger context
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	newLogger := &Logger{
		level:     l.level,
		output:    l.output,
		component: l.component,
		fields:    make(map[string]interface{}),
	}

	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}

	// Add new fields
	for k, v := range fields {
		newLogger.fields[k] = v
	}

	return newLogger
}

// WithComponent sets the component for this logger
func (l *Logger) WithComponent(component string) *Logger {
	newLogger := *l
	newLogger.component = component
	return &newLogger
}

// Debug logs a debug message
func (l *Logger) Debug(message string) {
	l.log(DebugLevel, message, nil)
}

// Debugf logs a formatted debug message
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(DebugLevel, fmt.Sprintf(format, args...), nil)
}

// Info logs an info message
func (l *Logger) Info(message string) {
	l.log(InfoLevel, message, nil)
}

// Infof logs a formatted info message
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(InfoLevel, fmt.Sprintf(format, args...), nil)
}

// Warn logs a warning message
func (l *Logger) Warn(message string) {
	l.log(WarnLevel, message, nil)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(WarnLevel, fmt.Sprintf(format, args...), nil)
}

// Error logs an error message
func (l *Logger) Error(message string) {
	l.log(ErrorLevel, message, nil)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(ErrorLevel, fmt.Sprintf(format, args...), nil)
}

// ErrorWithErr logs an error message with an error object
func (l *Logger) ErrorWithErr(message string, err error) {
	fields := map[string]interface{}{"error": err.Error()}
	l.log(ErrorLevel, message, fields)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(message string) {
	l.log(FatalLevel, message, nil)
	os.Exit(1)
}

// Fatalf logs a formatted fatal message and exits
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.log(FatalLevel, fmt.Sprintf(format, args...), nil)
	os.Exit(1)
}

// LogOperation logs the start and end of an operation with duration
func (l *Logger) LogOperation(operation string, fn func() error) error {
	start := time.Now()
	
	l.WithField("operation", operation).Info("Operation started")
	
	err := fn()
	duration := time.Since(start).Milliseconds()
	
	logger := l.WithFields(map[string]interface{}{
		"operation": operation,
		"duration_ms": duration,
	})
	
	if err != nil {
		logger.WithField("error", err.Error()).Error("Operation failed")
		return err
	}
	
	logger.Info("Operation completed")
	return nil
}

// log writes a log entry
func (l *Logger) log(level LogLevel, message string, additionalFields map[string]interface{}) {
	if level < l.level {
		return
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Fields:    make(map[string]interface{}),
		Component: l.component,
	}

	// Add existing fields
	for k, v := range l.fields {
		entry.Fields[k] = v
	}

	// Add additional fields
	for k, v := range additionalFields {
		entry.Fields[k] = v
	}

	// Add caller information for errors and above
	if level >= ErrorLevel {
		if caller := getCaller(); caller != "" {
			entry.Caller = caller
		}
	}

	// Format and write the log entry
	formatted := l.formatEntry(entry)
	l.output.Write([]byte(formatted + "\n"))
}

// formatEntry formats a log entry as JSON
func (l *Logger) formatEntry(entry LogEntry) string {
	// Simple JSON formatting (in production, you'd use encoding/json)
	timestamp := entry.Timestamp.Format(time.RFC3339)
	level := entry.Level.String()
	
	var parts []string
	parts = append(parts, fmt.Sprintf(`"timestamp":"%s"`, timestamp))
	parts = append(parts, fmt.Sprintf(`"level":"%s"`, level))
	parts = append(parts, fmt.Sprintf(`"message":"%s"`, escapeJSON(entry.Message)))
	
	if entry.Component != "" {
		parts = append(parts, fmt.Sprintf(`"component":"%s"`, entry.Component))
	}
	
	if entry.Caller != "" {
		parts = append(parts, fmt.Sprintf(`"caller":"%s"`, entry.Caller))
	}
	
	if entry.RequestID != "" {
		parts = append(parts, fmt.Sprintf(`"request_id":"%s"`, entry.RequestID))
	}
	
	if entry.UserID != "" {
		parts = append(parts, fmt.Sprintf(`"user_id":"%s"`, entry.UserID))
	}
	
	if entry.Operation != "" {
		parts = append(parts, fmt.Sprintf(`"operation":"%s"`, entry.Operation))
	}
	
	if entry.Duration != nil {
		parts = append(parts, fmt.Sprintf(`"duration_ms":%d`, *entry.Duration))
	}
	
	if entry.Error != "" {
		parts = append(parts, fmt.Sprintf(`"error":"%s"`, escapeJSON(entry.Error)))
	}
	
	if entry.HTTPMethod != "" {
		parts = append(parts, fmt.Sprintf(`"http_method":"%s"`, entry.HTTPMethod))
	}
	
	if entry.HTTPPath != "" {
		parts = append(parts, fmt.Sprintf(`"http_path":"%s"`, entry.HTTPPath))
	}
	
	if entry.HTTPStatus != 0 {
		parts = append(parts, fmt.Sprintf(`"http_status":%d`, entry.HTTPStatus))
	}
	
	if entry.HTTPLatency != nil {
		parts = append(parts, fmt.Sprintf(`"http_latency_ms":%d`, *entry.HTTPLatency))
	}
	
	// Add custom fields
	if len(entry.Fields) > 0 {
		var fieldParts []string
		for k, v := range entry.Fields {
			switch val := v.(type) {
			case string:
				fieldParts = append(fieldParts, fmt.Sprintf(`"%s":"%s"`, k, escapeJSON(val)))
			case int, int32, int64:
				fieldParts = append(fieldParts, fmt.Sprintf(`"%s":%v`, k, val))
			case float32, float64:
				fieldParts = append(fieldParts, fmt.Sprintf(`"%s":%v`, k, val))
			case bool:
				fieldParts = append(fieldParts, fmt.Sprintf(`"%s":%t`, k, val))
			default:
				fieldParts = append(fieldParts, fmt.Sprintf(`"%s":"%v"`, k, val))
			}
		}
		if len(fieldParts) > 0 {
			parts = append(parts, strings.Join(fieldParts, ","))
		}
	}
	
	return "{" + strings.Join(parts, ",") + "}"
}

// getCaller returns the file and line number of the caller
func getCaller() string {
	// Skip getCaller -> log -> public method -> actual caller
	_, file, line, ok := runtime.Caller(4)
	if !ok {
		return ""
	}
	
	// Get just the filename, not the full path
	filename := filepath.Base(file)
	return fmt.Sprintf("%s:%d", filename, line)
}

// escapeJSON escapes a string for JSON
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

// Default logger instance
var defaultLogger = NewLogger(Config{
	Level:     InfoLevel,
	Component: "govc",
})

// SetDefaultLogger sets the default logger
func SetDefaultLogger(logger *Logger) {
	defaultLogger = logger
}

// GetDefaultLogger returns the default logger
func GetDefaultLogger() *Logger {
	return defaultLogger
}

// Package-level convenience functions
func Debug(message string) {
	defaultLogger.Debug(message)
}

func Debugf(format string, args ...interface{}) {
	defaultLogger.Debugf(format, args...)
}

func Info(message string) {
	defaultLogger.Info(message)
}

func Infof(format string, args ...interface{}) {
	defaultLogger.Infof(format, args...)
}

func Warn(message string) {
	defaultLogger.Warn(message)
}

func Warnf(format string, args ...interface{}) {
	defaultLogger.Warnf(format, args...)
}

func Error(message string) {
	defaultLogger.Error(message)
}

func Errorf(format string, args ...interface{}) {
	defaultLogger.Errorf(format, args...)
}

func ErrorWithErr(message string, err error) {
	defaultLogger.ErrorWithErr(message, err)
}

func Fatal(message string) {
	defaultLogger.Fatal(message)
}

func Fatalf(format string, args ...interface{}) {
	defaultLogger.Fatalf(format, args...)
}

// Gin middleware for request logging
func GinLogger(logger *Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		
		// Generate request ID
		requestID := generateRequestID()
		c.Set("request_id", requestID)
		
		// Process request
		c.Next()
		
		// Calculate latency
		latency := time.Since(start).Milliseconds()
		status := c.Writer.Status()
		
		// Get user ID if available
		userID := ""
		if user, exists := c.Get("user"); exists {
			// Try to extract user ID from the auth context
			if userData, ok := user.(map[string]interface{}); ok {
				if id, exists := userData["id"]; exists {
					if idStr, ok := id.(string); ok {
						userID = idStr
					}
				}
			}
		}
		
		// Log the request
		logFields := map[string]interface{}{
			"request_id":     requestID,
			"http_method":    method,
			"http_path":      path,
			"http_status":    status,
			"http_latency_ms": latency,
		}
		
		if userID != "" {
			logFields["user_id"] = userID
		}
		
		logLevel := InfoLevel
		if status >= 400 && status < 500 {
			logLevel = WarnLevel
		} else if status >= 500 {
			logLevel = ErrorLevel
		}
		
		logger.WithFields(logFields).log(logLevel, "HTTP request processed", nil)
	}
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
}

// GetRequestID extracts request ID from gin context
func GetRequestID(c *gin.Context) string {
	if requestID, exists := c.Get("request_id"); exists {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return ""
}

// GetUserID extracts user ID from gin context
func GetUserID(c *gin.Context) string {
	if user, exists := c.Get("user"); exists {
		// Try to extract user ID from the auth context
		if userData, ok := user.(map[string]interface{}); ok {
			if id, exists := userData["id"]; exists {
				if idStr, ok := id.(string); ok {
					return idStr
				}
			}
		}
	}
	return ""
}