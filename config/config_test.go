package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Test server defaults
	if config.Server.Port != 8080 {
		t.Errorf("Expected default port 8080, got %d", config.Server.Port)
	}
	if config.Server.Host != "0.0.0.0" {
		t.Errorf("Expected default host 0.0.0.0, got %s", config.Server.Host)
	}
	if config.Server.MaxRepos != 1000 {
		t.Errorf("Expected default max repos 1000, got %d", config.Server.MaxRepos)
	}

	// Test auth defaults
	if !config.Auth.Enabled {
		t.Error("Expected auth to be enabled by default")
	}
	if config.Auth.JWT.TTL != 24*time.Hour {
		t.Errorf("Expected default JWT TTL 24h, got %v", config.Auth.JWT.TTL)
	}

	// Test pool defaults
	if config.Pool.MaxRepositories != 100 {
		t.Errorf("Expected default pool max repos 100, got %d", config.Pool.MaxRepositories)
	}

	// Test logging defaults
	if config.Logging.Level != "INFO" {
		t.Errorf("Expected default log level INFO, got %s", config.Logging.Level)
	}
	if config.Logging.Format != "json" {
		t.Errorf("Expected default log format json, got %s", config.Logging.Format)
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `
server:
  port: 9090
  host: "127.0.0.1"
  max_repos: 500

auth:
  enabled: false
  jwt:
    secret: "test-secret-for-testing-purposes-only"
    issuer: "test-issuer"
    ttl: "12h"

pool:
  max_repositories: 50
  max_idle_time: "15m"

logging:
  level: "DEBUG"
  format: "text"

metrics:
  enabled: false
  path: "/custom-metrics"
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	config, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test loaded values
	if config.Server.Port != 9090 {
		t.Errorf("Expected port 9090, got %d", config.Server.Port)
	}
	if config.Server.Host != "127.0.0.1" {
		t.Errorf("Expected host 127.0.0.1, got %s", config.Server.Host)
	}
	if config.Server.MaxRepos != 500 {
		t.Errorf("Expected max repos 500, got %d", config.Server.MaxRepos)
	}
	if config.Auth.Enabled {
		t.Error("Expected auth to be disabled")
	}
	if config.Pool.MaxRepositories != 50 {
		t.Errorf("Expected pool max repos 50, got %d", config.Pool.MaxRepositories)
	}
	if config.Logging.Level != "DEBUG" {
		t.Errorf("Expected log level DEBUG, got %s", config.Logging.Level)
	}
	if config.Metrics.Enabled {
		t.Error("Expected metrics to be disabled")
	}
}

func TestLoadConfigWithEnvVariables(t *testing.T) {
	// Set environment variables
	envVars := map[string]string{
		"GOVC_PORT":                    "7777",
		"GOVC_HOST":                    "192.168.1.1",
		"GOVC_JWT_SECRET":              "env-secret-for-testing-purposes-only",
		"GOVC_LOG_LEVEL":               "ERROR",
		"GOVC_POOL_MAX_REPOS":          "200",
		"GOVC_METRICS_ENABLED":         "false",
		"GOVC_STORAGE_TYPE":            "disk",
		"GOVC_STORAGE_DISK_PATH":       "/tmp/govc-test",
		"GOVC_DEBUG":                   "true",
	}

	// Set environment variables
	for key, value := range envVars {
		os.Setenv(key, value)
		defer os.Unsetenv(key)
	}

	config, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load config with env vars: %v", err)
	}

	// Test environment overrides
	if config.Server.Port != 7777 {
		t.Errorf("Expected port 7777 from env, got %d", config.Server.Port)
	}
	if config.Server.Host != "192.168.1.1" {
		t.Errorf("Expected host from env, got %s", config.Server.Host)
	}
	if config.Auth.JWT.Secret != "env-secret-for-testing-purposes-only" {
		t.Errorf("Expected JWT secret from env, got %s", config.Auth.JWT.Secret)
	}
	if config.Logging.Level != "ERROR" {
		t.Errorf("Expected log level ERROR from env, got %s", config.Logging.Level)
	}
	if config.Pool.MaxRepositories != 200 {
		t.Errorf("Expected pool max repos 200 from env, got %d", config.Pool.MaxRepositories)
	}
	if config.Metrics.Enabled {
		t.Error("Expected metrics disabled from env")
	}
	if config.Storage.Type != "disk" {
		t.Errorf("Expected storage type disk from env, got %s", config.Storage.Type)
	}
	if !config.Development.Debug {
		t.Error("Expected debug enabled from env")
	}
}

func TestLoadConfigWithEnvExpansion(t *testing.T) {
	// Set environment variable for expansion
	os.Setenv("TEST_SECRET", "expanded-secret-for-testing-purposes-only")
	defer os.Unsetenv("TEST_SECRET")

	// Create temporary config file with env expansion
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "expansion-config.yaml")

	configContent := `
auth:
  jwt:
    secret: "${TEST_SECRET}"
    issuer: "test-${USER:-unknown}"
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	config, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load config with expansion: %v", err)
	}

	if config.Auth.JWT.Secret != "expanded-secret-for-testing-purposes-only" {
		t.Errorf("Expected expanded secret, got %s", config.Auth.JWT.Secret)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		modifyFunc  func(*Config)
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid config",
			modifyFunc: func(c *Config) {
				c.Auth.JWT.Secret = "valid-secret-for-testing-purposes-only"
			},
			expectError: false,
		},
		{
			name: "Invalid port - too low",
			modifyFunc: func(c *Config) {
				c.Server.Port = 0
				c.Auth.JWT.Secret = "valid-secret-for-testing-purposes-only"
			},
			expectError: true,
			errorMsg:    "invalid server port",
		},
		{
			name: "Invalid port - too high",
			modifyFunc: func(c *Config) {
				c.Server.Port = 70000
				c.Auth.JWT.Secret = "valid-secret-for-testing-purposes-only"
			},
			expectError: true,
			errorMsg:    "invalid server port",
		},
		{
			name: "JWT secret too short",
			modifyFunc: func(c *Config) {
				c.Auth.JWT.Secret = "short"
			},
			expectError: true,
			errorMsg:    "JWT secret must be at least 32 characters",
		},
		{
			name: "Default JWT secret in production",
			modifyFunc: func(c *Config) {
				c.Auth.JWT.Secret = "default-secret-change-in-production"
			},
			expectError: true,
			errorMsg:    "JWT secret must be changed from default",
		},
		{
			name: "Invalid log level",
			modifyFunc: func(c *Config) {
				c.Logging.Level = "INVALID"
				c.Auth.JWT.Secret = "valid-secret-for-testing-purposes-only"
			},
			expectError: true,
			errorMsg:    "invalid log level",
		},
		{
			name: "Invalid log format",
			modifyFunc: func(c *Config) {
				c.Logging.Format = "invalid"
				c.Auth.JWT.Secret = "valid-secret-for-testing-purposes-only"
			},
			expectError: true,
			errorMsg:    "invalid log format",
		},
		{
			name: "Invalid storage type",
			modifyFunc: func(c *Config) {
				c.Storage.Type = "invalid"
				c.Auth.JWT.Secret = "valid-secret-for-testing-purposes-only"
			},
			expectError: true,
			errorMsg:    "invalid storage type",
		},
		{
			name: "Disk storage without path",
			modifyFunc: func(c *Config) {
				c.Storage.Type = "disk"
				c.Storage.DiskPath = ""
				c.Auth.JWT.Secret = "valid-secret-for-testing-purposes-only"
			},
			expectError: true,
			errorMsg:    "disk path is required",
		},
		{
			name: "Auth disabled - no validation needed",
			modifyFunc: func(c *Config) {
				c.Auth.Enabled = false
				c.Auth.JWT.Secret = ""
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			tt.modifyFunc(config)

			err := config.Validate()

			if tt.expectError {
				if err == nil {
					t.Error("Expected validation error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no validation error, got: %v", err)
				}
			}
		})
	}
}

func TestConfigMethods(t *testing.T) {
	config := DefaultConfig()
	config.Server.Host = "localhost"
	config.Server.Port = 9000

	// Test Address method
	expectedAddr := "localhost:9000"
	if addr := config.Address(); addr != expectedAddr {
		t.Errorf("Expected address %s, got %s", expectedAddr, addr)
	}

	// Test IsDevelopment method
	if config.IsDevelopment() {
		t.Error("Expected development mode to be false by default")
	}

	config.Development.Debug = true
	if !config.IsDevelopment() {
		t.Error("Expected development mode to be true when debug is enabled")
	}

	// Test GetLogLevel method
	testCases := []struct {
		level    string
		expected LogLevel
	}{
		{"DEBUG", DebugLevel},
		{"INFO", InfoLevel},
		{"WARN", WarnLevel},
		{"ERROR", ErrorLevel},
		{"FATAL", FatalLevel},
		{"INVALID", InfoLevel}, // Should default to INFO
	}

	for _, tc := range testCases {
		config.Logging.Level = tc.level
		if actual := config.GetLogLevel(); actual != tc.expected {
			t.Errorf("Expected log level %v for %s, got %v", tc.expected, tc.level, actual)
		}
	}
}

func TestLogLevelString(t *testing.T) {
	testCases := []struct {
		level    LogLevel
		expected string
	}{
		{DebugLevel, "DEBUG"},
		{InfoLevel, "INFO"},
		{WarnLevel, "WARN"},
		{ErrorLevel, "ERROR"},
		{FatalLevel, "FATAL"},
		{LogLevel(999), "UNKNOWN"}, // Invalid level
	}

	for _, tc := range testCases {
		if actual := tc.level.String(); actual != tc.expected {
			t.Errorf("Expected string %s for level %v, got %s", tc.expected, tc.level, actual)
		}
	}
}

func TestLoadConfigNonexistentFile(t *testing.T) {
	// Set environment variable to provide a valid JWT secret
	os.Setenv("GOVC_JWT_SECRET", "test-secret-for-nonexistent-file-test-purposes-only")
	defer os.Unsetenv("GOVC_JWT_SECRET")

	// Loading non-existent file should use defaults with env overrides
	config, err := LoadConfig("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Loading nonexistent config file should not error: %v", err)
	}

	// Should have default values
	if config.Server.Port != 8080 {
		t.Errorf("Expected default port when file doesn't exist, got %d", config.Server.Port)
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	// Create temporary invalid YAML file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "invalid.yaml")

	invalidYAML := `
server:
  port: invalid_port
  host: [invalid_structure
`

	err := os.WriteFile(configFile, []byte(invalidYAML), 0644)
	if err != nil {
		t.Fatalf("Failed to write invalid config file: %v", err)
	}

	_, err = LoadConfig(configFile)
	if err == nil {
		t.Error("Expected error when loading invalid YAML")
	}
}

func TestConfigWithCompleteValues(t *testing.T) {
	config := &Config{
		Server: ServerConfig{
			Host:         "example.com",
			Port:         8443,
			ReadTimeout:  45 * time.Second,
			WriteTimeout: 45 * time.Second,
			IdleTimeout:  120 * time.Second,
			MaxRepos:     2000,
			TrustedProxy: []string{"192.168.1.0/24", "10.0.0.0/8"},
		},
		Auth: AuthConfig{
			Enabled: true,
			JWT: JWTConfig{
				Secret: "super-secret-jwt-key-for-testing-purposes-only-not-for-production",
				Issuer: "govc-test",
				TTL:    8 * time.Hour,
			},
		},
		Pool: PoolConfig{
			MaxRepositories: 150,
			MaxIdleTime:     45 * time.Minute,
			CleanupInterval: 10 * time.Minute,
			EnableMetrics:   true,
		},
		Logging: LoggingConfig{
			Level:     "WARN",
			Format:    "text",
			Component: "govc-test",
			Output:    "stderr",
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/custom-metrics",
		},
		Storage: StorageConfig{
			Type:         "hybrid",
			DiskPath:     "/opt/govc/data",
			MaxMemoryMB:  2048,
			CacheEnabled: true,
			CacheTTL:     2 * time.Hour,
		},
		Development: DevelopmentConfig{
			Debug:           true,
			ProfilerEnabled: true,
			CORSEnabled:     true,
			AllowedOrigins:  []string{"http://localhost:3000", "https://dev.example.com"},
		},
	}

	err := config.Validate()
	if err != nil {
		t.Errorf("Valid complete config failed validation: %v", err)
	}

	// Test all helper methods
	if config.Address() != "example.com:8443" {
		t.Errorf("Unexpected address: %s", config.Address())
	}

	if !config.IsDevelopment() {
		t.Error("Expected development mode to be true")
	}

	if config.GetLogLevel() != WarnLevel {
		t.Errorf("Expected WARN level, got %v", config.GetLogLevel())
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		 func() bool {
			 for i := 1; i <= len(s)-len(substr); i++ {
				 if s[i:i+len(substr)] == substr {
					 return true
				 }
			 }
			 return false
		 }())))
}

func BenchmarkLoadConfig(b *testing.B) {
	// Create temporary config file
	tmpDir := b.TempDir()
	configFile := filepath.Join(tmpDir, "bench-config.yaml")

	configContent := `
server:
  port: 8080
  host: "0.0.0.0"
auth:
  jwt:
    secret: "benchmark-secret-for-testing-purposes-only"
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		b.Fatalf("Failed to write benchmark config file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LoadConfig(configFile)
		if err != nil {
			b.Fatalf("Config load failed: %v", err)
		}
	}
}

func BenchmarkConfigValidation(b *testing.B) {
	config := DefaultConfig()
	config.Auth.JWT.Secret = "benchmark-secret-for-testing-purposes-only"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := config.Validate()
		if err != nil {
			b.Fatalf("Config validation failed: %v", err)
		}
	}
}