package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLogLevel_String(t *testing.T) {
	testCases := []struct {
		level    LogLevel
		expected string
	}{
		{DebugLevel, "DEBUG"},
		{InfoLevel, "INFO"},
		{WarnLevel, "WARN"},
		{ErrorLevel, "ERROR"},
		{FatalLevel, "FATAL"},
		{LogLevel(999), "UNKNOWN"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.level.String()
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer

	testCases := []struct {
		name           string
		config         Config
		expectedLevel  LogLevel
		expectedOutput string
		expectedComp   string
	}{
		{
			name: "default config",
			config: Config{
				Level:     InfoLevel,
				Output:    &buf,
				Component: "test",
			},
			expectedLevel:  InfoLevel,
			expectedOutput: "&buf",
			expectedComp:   "test",
		},
		{
			name: "nil output defaults to stdout",
			config: Config{
				Level:     DebugLevel,
				Output:    nil,
				Component: "debug-test",
			},
			expectedLevel: DebugLevel,
			expectedComp:  "debug-test",
		},
		{
			name: "empty component",
			config: Config{
				Level:  ErrorLevel,
				Output: &buf,
			},
			expectedLevel: ErrorLevel,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logger := NewLogger(tc.config)

			if logger == nil {
				t.Fatal("NewLogger returned nil")
			}

			if logger.level != tc.expectedLevel {
				t.Errorf("Expected level %v, got %v", tc.expectedLevel, logger.level)
			}

			if logger.component != tc.expectedComp {
				t.Errorf("Expected component '%s', got '%s'", tc.expectedComp, logger.component)
			}

			if logger.output == nil {
				t.Error("Output should not be nil")
			}

			if logger.fields == nil {
				t.Error("Fields should be initialized")
			}
		})
	}
}

func TestLogger_WithField(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:     InfoLevel,
		Output:    &buf,
		Component: "test",
	})

	// Add first field
	logger1 := logger.WithField("key1", "value1")
	if logger1 == logger {
		t.Error("WithField should return a new logger instance")
	}

	// Add second field
	logger2 := logger1.WithField("key2", 42)

	// Check that fields are preserved and added correctly
	if len(logger.fields) != 0 {
		t.Error("Original logger should have no fields")
	}

	if len(logger1.fields) != 1 {
		t.Errorf("Logger1 should have 1 field, got %d", len(logger1.fields))
	}

	if logger1.fields["key1"] != "value1" {
		t.Errorf("Expected key1='value1', got %v", logger1.fields["key1"])
	}

	if len(logger2.fields) != 2 {
		t.Errorf("Logger2 should have 2 fields, got %d", len(logger2.fields))
	}

	if logger2.fields["key1"] != "value1" {
		t.Errorf("Expected key1='value1', got %v", logger2.fields["key1"])
	}

	if logger2.fields["key2"] != 42 {
		t.Errorf("Expected key2=42, got %v", logger2.fields["key2"])
	}
}

func TestLogger_WithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:     InfoLevel,
		Output:    &buf,
		Component: "test",
	})

	fields := map[string]interface{}{
		"str_field":  "string_value",
		"int_field":  123,
		"bool_field": true,
	}

	newLogger := logger.WithFields(fields)

	if newLogger == logger {
		t.Error("WithFields should return a new logger instance")
	}

	if len(newLogger.fields) != 3 {
		t.Errorf("Expected 3 fields, got %d", len(newLogger.fields))
	}

	for key, expectedValue := range fields {
		if newLogger.fields[key] != expectedValue {
			t.Errorf("Expected %s=%v, got %v", key, expectedValue, newLogger.fields[key])
		}
	}

	// Test adding fields to an already field-populated logger
	baseLogger := logger.WithField("base", "value")
	combinedLogger := baseLogger.WithFields(fields)

	if len(combinedLogger.fields) != 4 {
		t.Errorf("Expected 4 fields in combined logger, got %d", len(combinedLogger.fields))
	}

	if combinedLogger.fields["base"] != "value" {
		t.Error("Base field should be preserved")
	}
}

func TestLogger_WithComponent(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:     InfoLevel,
		Output:    &buf,
		Component: "original",
	})

	newLogger := logger.WithComponent("new-component")

	if newLogger.component != "new-component" {
		t.Errorf("Expected component 'new-component', got '%s'", newLogger.component)
	}

	if logger.component != "original" {
		t.Error("Original logger component should not be modified")
	}
}

func TestLogger_LogLevels(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:     DebugLevel,
		Output:    &buf,
		Component: "test",
	})

	testCases := []struct {
		name        string
		logFunc     func()
		expectedLog bool
		level       LogLevel
	}{
		{
			name: "debug message",
			logFunc: func() {
				logger.Debug("debug message")
			},
			expectedLog: true,
			level:       DebugLevel,
		},
		{
			name: "info message",
			logFunc: func() {
				logger.Info("info message")
			},
			expectedLog: true,
			level:       InfoLevel,
		},
		{
			name: "warn message",
			logFunc: func() {
				logger.Warn("warn message")
			},
			expectedLog: true,
			level:       WarnLevel,
		},
		{
			name: "error message",
			logFunc: func() {
				logger.Error("error message")
			},
			expectedLog: true,
			level:       ErrorLevel,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()
			tc.logFunc()

			output := buf.String()
			if tc.expectedLog {
				if output == "" {
					t.Error("Expected log output but got none")
				}

				// Parse JSON to verify structure
				var entry map[string]interface{}
				if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
					t.Errorf("Failed to parse log output as JSON: %v", err)
				}

				if entry["level"] != tc.level.String() {
					t.Errorf("Expected level %s, got %v", tc.level.String(), entry["level"])
				}

				if entry["component"] != "test" {
					t.Errorf("Expected component 'test', got %v", entry["component"])
				}
			} else {
				if output != "" {
					t.Errorf("Expected no log output but got: %s", output)
				}
			}
		})
	}
}

func TestLogger_FormattedLogs(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:     DebugLevel,
		Output:    &buf,
		Component: "test",
	})

	testCases := []struct {
		name        string
		logFunc     func()
		expectedMsg string
	}{
		{
			name: "debugf",
			logFunc: func() {
				logger.Debugf("debug %s %d", "test", 42)
			},
			expectedMsg: "debug test 42",
		},
		{
			name: "infof",
			logFunc: func() {
				logger.Infof("info %s %d", "formatted", 123)
			},
			expectedMsg: "info formatted 123",
		},
		{
			name: "warnf",
			logFunc: func() {
				logger.Warnf("warn %v", true)
			},
			expectedMsg: "warn true",
		},
		{
			name: "errorf",
			logFunc: func() {
				logger.Errorf("error %f", 3.14)
			},
			expectedMsg: "error 3.140000",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()
			tc.logFunc()

			output := buf.String()
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
				t.Errorf("Failed to parse log output as JSON: %v", err)
			}

			if entry["message"] != tc.expectedMsg {
				t.Errorf("Expected message '%s', got '%v'", tc.expectedMsg, entry["message"])
			}
		})
	}
}

func TestLogger_ErrorWithErr(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:     ErrorLevel,
		Output:    &buf,
		Component: "test",
	})

	testErr := errors.New("test error")
	logger.ErrorWithErr("operation failed", testErr)

	output := buf.String()
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
		t.Errorf("Failed to parse log output as JSON: %v", err)
	}

	if entry["message"] != "operation failed" {
		t.Errorf("Expected message 'operation failed', got '%v'", entry["message"])
	}

	if entry["error"] != "test error" {
		t.Errorf("Expected error 'test error', got '%v'", entry["error"])
	}
}

func TestLogger_LogLevel_Filtering(t *testing.T) {
	var buf bytes.Buffer

	// Create logger with WARN level - should filter out DEBUG and INFO
	logger := NewLogger(Config{
		Level:     WarnLevel,
		Output:    &buf,
		Component: "test",
	})

	testCases := []struct {
		name      string
		logFunc   func()
		shouldLog bool
	}{
		{
			name: "debug should be filtered",
			logFunc: func() {
				logger.Debug("debug message")
			},
			shouldLog: false,
		},
		{
			name: "info should be filtered",
			logFunc: func() {
				logger.Info("info message")
			},
			shouldLog: false,
		},
		{
			name: "warn should log",
			logFunc: func() {
				logger.Warn("warn message")
			},
			shouldLog: true,
		},
		{
			name: "error should log",
			logFunc: func() {
				logger.Error("error message")
			},
			shouldLog: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()
			tc.logFunc()

			output := buf.String()
			if tc.shouldLog {
				if output == "" {
					t.Error("Expected log output but got none")
				}
			} else {
				if output != "" {
					t.Errorf("Expected no output but got: %s", output)
				}
			}
		})
	}
}

func TestLogger_LogOperation(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:     InfoLevel,
		Output:    &buf,
		Component: "test",
	})

	// Test successful operation
	t.Run("successful operation", func(t *testing.T) {
		buf.Reset()

		err := logger.LogOperation("test_operation", func() error {
			time.Sleep(10 * time.Millisecond)
			return nil
		})

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")

		if len(lines) != 2 {
			t.Errorf("Expected 2 log lines (start and end), got %d", len(lines))
		}

		// Check start log
		var startEntry map[string]interface{}
		if err := json.Unmarshal([]byte(lines[0]), &startEntry); err != nil {
			t.Errorf("Failed to parse start log: %v", err)
		}

		if startEntry["message"] != "Operation started" {
			t.Errorf("Expected start message, got '%v'", startEntry["message"])
		}

		if startEntry["operation"] != "test_operation" {
			t.Errorf("Expected operation 'test_operation', got '%v'", startEntry["operation"])
		}

		// Check end log
		var endEntry map[string]interface{}
		if err := json.Unmarshal([]byte(lines[1]), &endEntry); err != nil {
			t.Errorf("Failed to parse end log: %v", err)
		}

		if endEntry["message"] != "Operation completed" {
			t.Errorf("Expected completion message, got '%v'", endEntry["message"])
		}

		if endEntry["operation"] != "test_operation" {
			t.Errorf("Expected operation 'test_operation', got '%v'", endEntry["operation"])
		}

		// Check duration exists and is reasonable
		duration, exists := endEntry["duration_ms"]
		if !exists {
			t.Error("Expected duration_ms field")
		}

		if durationFloat, ok := duration.(float64); ok {
			if durationFloat < 10 {
				t.Errorf("Expected duration >= 10ms, got %f", durationFloat)
			}
		}
	})

	// Test failed operation
	t.Run("failed operation", func(t *testing.T) {
		buf.Reset()

		testErr := errors.New("operation failed")
		err := logger.LogOperation("failing_operation", func() error {
			return testErr
		})

		if err != testErr {
			t.Errorf("Expected original error to be returned, got: %v", err)
		}

		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")

		if len(lines) != 2 {
			t.Errorf("Expected 2 log lines (start and error), got %d", len(lines))
		}

		// Check error log
		var errorEntry map[string]interface{}
		if err := json.Unmarshal([]byte(lines[1]), &errorEntry); err != nil {
			t.Errorf("Failed to parse error log: %v", err)
		}

		if errorEntry["message"] != "Operation failed" {
			t.Errorf("Expected failure message, got '%v'", errorEntry["message"])
		}

		if errorEntry["error"] != "operation failed" {
			t.Errorf("Expected error 'operation failed', got '%v'", errorEntry["error"])
		}
	})
}

func TestLogger_JSONFormatting(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:     InfoLevel,
		Output:    &buf,
		Component: "test",
	})

	// Test with various field types
	logger.WithFields(map[string]interface{}{
		"string_field": "test value",
		"int_field":    42,
		"float_field":  3.14,
		"bool_field":   true,
		"nil_field":    nil,
	}).Info("test message")

	output := buf.String()
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
		t.Errorf("Failed to parse log output as JSON: %v", err)
	}

	expectedFields := map[string]interface{}{
		"string_field": "test value",
		"int_field":    float64(42), // JSON unmarshals numbers as float64
		"float_field":  3.14,
		"bool_field":   true,
	}

	for key, expected := range expectedFields {
		if entry[key] != expected {
			t.Errorf("Expected %s=%v, got %v", key, expected, entry[key])
		}
	}
}

func TestEscapeJSON(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{`simple string`, `simple string`},
		{`string with "quotes"`, `string with \"quotes\"`},
		{`string with \backslash`, `string with \\backslash`},
		{"string with\nnewline", `string with\nnewline`},
		{"string with\ttab", `string with\ttab`},
		{"string with\rcarriage return", `string with\rcarriage return`},
		{`complex "string" with\nspecial\tchars\r`, `complex \"string\" with\\nspecial\\tchars\\r`},
	}

	for _, tc := range testCases {
		result := escapeJSON(tc.input)
		if result != tc.expected {
			t.Errorf("escapeJSON(%q): expected %q, got %q", tc.input, tc.expected, result)
		}
	}
}

func TestGinLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:     InfoLevel,
		Output:    &buf,
		Component: "http",
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(GinLogger(logger))

	// Add test routes
	router.GET("/success", func(c *gin.Context) {
		time.Sleep(10 * time.Millisecond)
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	router.GET("/client-error", func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
	})

	router.GET("/server-error", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server error"})
	})

	testCases := []struct {
		name           string
		path           string
		expectedStatus int
		expectedLevel  string
	}{
		{
			name:           "successful request",
			path:           "/success",
			expectedStatus: 200,
			expectedLevel:  "INFO",
		},
		{
			name:           "client error request",
			path:           "/client-error",
			expectedStatus: 400,
			expectedLevel:  "WARN",
		},
		{
			name:           "server error request",
			path:           "/server-error",
			expectedStatus: 500,
			expectedLevel:  "ERROR",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()

			req := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}

			output := buf.String()
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
				t.Errorf("Failed to parse log output as JSON: %v", err)
			}

			if entry["level"] != tc.expectedLevel {
				t.Errorf("Expected level %s, got %v", tc.expectedLevel, entry["level"])
			}

			if entry["http_method"] != "GET" {
				t.Errorf("Expected http_method 'GET', got %v", entry["http_method"])
			}

			if entry["http_path"] != tc.path {
				t.Errorf("Expected http_path '%s', got %v", tc.path, entry["http_path"])
			}

			if entry["http_status"] != float64(tc.expectedStatus) {
				t.Errorf("Expected http_status %d, got %v", tc.expectedStatus, entry["http_status"])
			}

			if _, exists := entry["request_id"]; !exists {
				t.Error("Expected request_id field")
			}

			if _, exists := entry["http_latency_ms"]; !exists {
				t.Error("Expected http_latency_ms field")
			}
		})
	}
}

func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	time.Sleep(1 * time.Millisecond) // Ensure different timestamp
	id2 := generateRequestID()

	if id1 == "" {
		t.Error("Request ID should not be empty")
	}

	if id2 == "" {
		t.Error("Request ID should not be empty")
	}

	if id1 == id2 {
		t.Error("Request IDs should be unique")
	}

	// Should contain process ID and timestamp
	if !strings.Contains(id1, "-") {
		t.Error("Request ID should contain separator")
	}
}

func TestGetRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	// Test when no request ID is set
	id := GetRequestID(c)
	if id != "" {
		t.Errorf("Expected empty request ID, got '%s'", id)
	}

	// Test when request ID is set
	expectedID := "test-request-id"
	c.Set("request_id", expectedID)

	id = GetRequestID(c)
	if id != expectedID {
		t.Errorf("Expected request ID '%s', got '%s'", expectedID, id)
	}

	// Test when request ID is not a string
	c.Set("request_id", 123)
	id = GetRequestID(c)
	if id != "" {
		t.Errorf("Expected empty request ID for non-string value, got '%s'", id)
	}
}

func TestGetUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	// Test when no user is set
	userID := GetUserID(c)
	if userID != "" {
		t.Errorf("Expected empty user ID, got '%s'", userID)
	}

	// Test when user is set correctly
	userData := map[string]interface{}{
		"id":   "test-user-123",
		"name": "Test User",
	}
	c.Set("user", userData)

	userID = GetUserID(c)
	if userID != "test-user-123" {
		t.Errorf("Expected user ID 'test-user-123', got '%s'", userID)
	}

	// Test when user data doesn't have id field
	userDataNoID := map[string]interface{}{
		"name": "Test User",
	}
	c.Set("user", userDataNoID)

	userID = GetUserID(c)
	if userID != "" {
		t.Errorf("Expected empty user ID when no id field, got '%s'", userID)
	}

	// Test when user is not a map
	c.Set("user", "not-a-map")
	userID = GetUserID(c)
	if userID != "" {
		t.Errorf("Expected empty user ID for non-map user, got '%s'", userID)
	}
}

func TestDefaultLogger(t *testing.T) {
	// Test default logger functions
	defaultLogger := GetDefaultLogger()
	if defaultLogger == nil {
		t.Fatal("Default logger should not be nil")
	}

	// Test setting new default logger
	var buf bytes.Buffer
	newLogger := NewLogger(Config{
		Level:     DebugLevel,
		Output:    &buf,
		Component: "new-default",
	})

	SetDefaultLogger(newLogger)

	if GetDefaultLogger() != newLogger {
		t.Error("Default logger should have been updated")
	}

	// Test package-level functions use default logger
	Debug("debug test")
	output := buf.String()

	if output == "" {
		t.Error("Package-level Debug should produce output")
	}

	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
		t.Errorf("Failed to parse debug output: %v", err)
	}

	if entry["level"] != "DEBUG" {
		t.Errorf("Expected DEBUG level, got %v", entry["level"])
	}

	if entry["component"] != "new-default" {
		t.Errorf("Expected component 'new-default', got %v", entry["component"])
	}
}

func TestPackageLevelFunctions(t *testing.T) {
	var buf bytes.Buffer
	testLogger := NewLogger(Config{
		Level:     DebugLevel,
		Output:    &buf,
		Component: "package-test",
	})

	// Save original default logger
	originalLogger := GetDefaultLogger()
	defer SetDefaultLogger(originalLogger)

	// Set test logger as default
	SetDefaultLogger(testLogger)

	testCases := []struct {
		name    string
		logFunc func()
		level   string
		message string
	}{
		{
			name: "Debug",
			logFunc: func() {
				Debug("debug message")
			},
			level:   "DEBUG",
			message: "debug message",
		},
		{
			name: "Debugf",
			logFunc: func() {
				Debugf("debug %s", "formatted")
			},
			level:   "DEBUG",
			message: "debug formatted",
		},
		{
			name: "Info",
			logFunc: func() {
				Info("info message")
			},
			level:   "INFO",
			message: "info message",
		},
		{
			name: "Infof",
			logFunc: func() {
				Infof("info %d", 42)
			},
			level:   "INFO",
			message: "info 42",
		},
		{
			name: "Warn",
			logFunc: func() {
				Warn("warn message")
			},
			level:   "WARN",
			message: "warn message",
		},
		{
			name: "Warnf",
			logFunc: func() {
				Warnf("warn %v", true)
			},
			level:   "WARN",
			message: "warn true",
		},
		{
			name: "Error",
			logFunc: func() {
				Error("error message")
			},
			level:   "ERROR",
			message: "error message",
		},
		{
			name: "Errorf",
			logFunc: func() {
				Errorf("error %f", 3.14)
			},
			level:   "ERROR",
			message: "error 3.140000",
		},
		{
			name: "ErrorWithErr",
			logFunc: func() {
				ErrorWithErr("operation failed", errors.New("test error"))
			},
			level:   "ERROR",
			message: "operation failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()
			tc.logFunc()

			output := buf.String()
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
				t.Errorf("Failed to parse log output: %v", err)
			}

			if entry["level"] != tc.level {
				t.Errorf("Expected level %s, got %v", tc.level, entry["level"])
			}

			if entry["message"] != tc.message {
				t.Errorf("Expected message '%s', got %v", tc.message, entry["message"])
			}
		})
	}
}

// Note: Testing Fatal and Fatalf would require special handling since they call os.Exit()
// In a real test suite, you might use a testing framework that can capture os.Exit calls
// or refactor the code to make the exit behavior injectable for testing

func BenchmarkLogger_Info(b *testing.B) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  InfoLevel,
		Output: &buf,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message")
		buf.Reset() // Reset to avoid growing buffer
	}
}

func BenchmarkLogger_WithField(b *testing.B) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  InfoLevel,
		Output: &buf,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newLogger := logger.WithField("key", "value")
		newLogger.Info("benchmark message")
		buf.Reset()
	}
}

func BenchmarkLogger_WithFields(b *testing.B) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  InfoLevel,
		Output: &buf,
	})

	fields := map[string]interface{}{
		"field1": "value1",
		"field2": 42,
		"field3": true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newLogger := logger.WithFields(fields)
		newLogger.Info("benchmark message")
		buf.Reset()
	}
}

func BenchmarkGinLogger(b *testing.B) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  InfoLevel,
		Output: &buf,
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(GinLogger(logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		buf.Reset()
	}
}
