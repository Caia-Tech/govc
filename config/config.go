package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete application configuration
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Auth        AuthConfig        `yaml:"auth"`
	Pool        PoolConfig        `yaml:"pool"`
	Logging     LoggingConfig     `yaml:"logging"`
	Metrics     MetricsConfig     `yaml:"metrics"`
	Storage     StorageConfig     `yaml:"storage"`
	Development DevelopmentConfig `yaml:"development"`
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Host           string        `yaml:"host"`
	Port           int           `yaml:"port"`
	ReadTimeout    time.Duration `yaml:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout"`
	IdleTimeout    time.Duration `yaml:"idle_timeout"`
	RequestTimeout time.Duration `yaml:"request_timeout"`
	MaxRepos       int           `yaml:"max_repos"`
	MaxRequestSize int64         `yaml:"max_request_size"`
	TrustedProxy   []string      `yaml:"trusted_proxy"`
}

// AuthConfig contains authentication configuration
type AuthConfig struct {
	Enabled bool      `yaml:"enabled"`
	JWT     JWTConfig `yaml:"jwt"`
}

// JWTConfig contains JWT-specific configuration
type JWTConfig struct {
	Secret string        `yaml:"secret"`
	Issuer string        `yaml:"issuer"`
	TTL    time.Duration `yaml:"ttl"`
}

// PoolConfig contains repository pool configuration
type PoolConfig struct {
	MaxRepositories int           `yaml:"max_repositories"`
	MaxIdleTime     time.Duration `yaml:"max_idle_time"`
	CleanupInterval time.Duration `yaml:"cleanup_interval"`
	EnableMetrics   bool          `yaml:"enable_metrics"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level     string `yaml:"level"`
	Format    string `yaml:"format"`
	Component string `yaml:"component"`
	Output    string `yaml:"output"`
}

// MetricsConfig contains metrics configuration
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// StorageConfig contains storage configuration
type StorageConfig struct {
	Type         string `yaml:"type"`          // memory, disk, hybrid
	DiskPath     string `yaml:"disk_path"`     // Path for disk storage
	MaxMemoryMB  int    `yaml:"max_memory_mb"` // Max memory usage in MB
	CacheEnabled bool   `yaml:"cache_enabled"`
	CacheTTL     time.Duration `yaml:"cache_ttl"`
}

// DevelopmentConfig contains development-specific settings
type DevelopmentConfig struct {
	Debug              bool     `yaml:"debug"`
	ProfilerEnabled    bool     `yaml:"profiler_enabled"`
	CORSEnabled        bool     `yaml:"cors_enabled"`
	AllowedOrigins     []string `yaml:"allowed_origins"`
	UseNewArchitecture bool     `yaml:"use_new_architecture"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:           "0.0.0.0",
			Port:           8080,
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
			IdleTimeout:    60 * time.Second,
			RequestTimeout: 25 * time.Second, // Slightly less than WriteTimeout
			MaxRepos:       1000,
			MaxRequestSize: 10 * 1024 * 1024, // 10MB
			TrustedProxy:   []string{},
		},
		Auth: AuthConfig{
			Enabled: true,
			JWT: JWTConfig{
				Secret: "default-secret-change-in-production",
				Issuer: "govc-server",
				TTL:    24 * time.Hour,
			},
		},
		Pool: PoolConfig{
			MaxRepositories: 100,
			MaxIdleTime:     30 * time.Minute,
			CleanupInterval: 5 * time.Minute,
			EnableMetrics:   true,
		},
		Logging: LoggingConfig{
			Level:     "INFO",
			Format:    "json",
			Component: "govc",
			Output:    "stdout",
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
		Storage: StorageConfig{
			Type:         "memory",
			DiskPath:     "/var/lib/govc",
			MaxMemoryMB:  1024,
			CacheEnabled: true,
			CacheTTL:     1 * time.Hour,
		},
		Development: DevelopmentConfig{
			Debug:              false,
			ProfilerEnabled:    false,
			CORSEnabled:        false,
			AllowedOrigins:     []string{"http://localhost:3000"},
			UseNewArchitecture: true, // Default to new architecture
		},
	}
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configFile string) (*Config, error) {
	config := DefaultConfig()

	// Load from YAML file if provided and exists
	if configFile != "" {
		if err := loadFromFile(config, configFile); err != nil {
			// Don't fail if file doesn't exist, just use defaults
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to load config file %s: %w", configFile, err)
			}
		}
	}

	// Override with environment variables
	if err := loadFromEnv(config); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// loadFromFile loads configuration from YAML file
func loadFromFile(config *Config, filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	// Expand environment variables in YAML content
	expanded := os.ExpandEnv(string(data))

	return yaml.Unmarshal([]byte(expanded), config)
}

// loadFromEnv loads configuration from environment variables
func loadFromEnv(config *Config) error {
	// Server configuration
	if host := os.Getenv("GOVC_HOST"); host != "" {
		config.Server.Host = host
	}
	if port := os.Getenv("GOVC_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Server.Port = p
		}
	}
	if timeout := os.Getenv("GOVC_READ_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			config.Server.ReadTimeout = d
		}
	}
	if timeout := os.Getenv("GOVC_WRITE_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			config.Server.WriteTimeout = d
		}
	}
	if maxRepos := os.Getenv("GOVC_MAX_REPOS"); maxRepos != "" {
		if m, err := strconv.Atoi(maxRepos); err == nil {
			config.Server.MaxRepos = m
		}
	}

	// Auth configuration
	if enabled := os.Getenv("GOVC_AUTH_ENABLED"); enabled != "" {
		config.Auth.Enabled = strings.ToLower(enabled) == "true"
	}
	if secret := os.Getenv("GOVC_JWT_SECRET"); secret != "" {
		config.Auth.JWT.Secret = secret
	}
	if issuer := os.Getenv("GOVC_JWT_ISSUER"); issuer != "" {
		config.Auth.JWT.Issuer = issuer
	}
	if ttl := os.Getenv("GOVC_JWT_TTL"); ttl != "" {
		if d, err := time.ParseDuration(ttl); err == nil {
			config.Auth.JWT.TTL = d
		}
	}

	// Pool configuration
	if maxRepos := os.Getenv("GOVC_POOL_MAX_REPOS"); maxRepos != "" {
		if m, err := strconv.Atoi(maxRepos); err == nil {
			config.Pool.MaxRepositories = m
		}
	}
	if idleTime := os.Getenv("GOVC_POOL_IDLE_TIME"); idleTime != "" {
		if d, err := time.ParseDuration(idleTime); err == nil {
			config.Pool.MaxIdleTime = d
		}
	}
	if cleanupInterval := os.Getenv("GOVC_POOL_CLEANUP_INTERVAL"); cleanupInterval != "" {
		if d, err := time.ParseDuration(cleanupInterval); err == nil {
			config.Pool.CleanupInterval = d
		}
	}

	// Logging configuration
	if level := os.Getenv("GOVC_LOG_LEVEL"); level != "" {
		config.Logging.Level = strings.ToUpper(level)
	}
	if format := os.Getenv("GOVC_LOG_FORMAT"); format != "" {
		config.Logging.Format = strings.ToLower(format)
	}
	if component := os.Getenv("GOVC_LOG_COMPONENT"); component != "" {
		config.Logging.Component = component
	}

	// Metrics configuration
	if enabled := os.Getenv("GOVC_METRICS_ENABLED"); enabled != "" {
		config.Metrics.Enabled = strings.ToLower(enabled) == "true"
	}
	if path := os.Getenv("GOVC_METRICS_PATH"); path != "" {
		config.Metrics.Path = path
	}

	// Storage configuration
	if storageType := os.Getenv("GOVC_STORAGE_TYPE"); storageType != "" {
		config.Storage.Type = strings.ToLower(storageType)
	}
	if diskPath := os.Getenv("GOVC_STORAGE_DISK_PATH"); diskPath != "" {
		config.Storage.DiskPath = diskPath
	}
	if maxMemory := os.Getenv("GOVC_STORAGE_MAX_MEMORY_MB"); maxMemory != "" {
		if m, err := strconv.Atoi(maxMemory); err == nil {
			config.Storage.MaxMemoryMB = m
		}
	}

	// Development configuration
	if debug := os.Getenv("GOVC_DEBUG"); debug != "" {
		config.Development.Debug = strings.ToLower(debug) == "true"
	}
	if profiler := os.Getenv("GOVC_PROFILER_ENABLED"); profiler != "" {
		config.Development.ProfilerEnabled = strings.ToLower(profiler) == "true"
	}
	if cors := os.Getenv("GOVC_CORS_ENABLED"); cors != "" {
		config.Development.CORSEnabled = strings.ToLower(cors) == "true"
	}

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Server validation
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.Server.ReadTimeout <= 0 {
		return fmt.Errorf("read timeout must be positive")
	}
	if c.Server.WriteTimeout <= 0 {
		return fmt.Errorf("write timeout must be positive")
	}
	if c.Server.MaxRepos <= 0 {
		return fmt.Errorf("max repositories must be positive")
	}

	// Auth validation
	if c.Auth.Enabled {
		if c.Auth.JWT.Secret == "" {
			return fmt.Errorf("JWT secret is required when auth is enabled")
		}
		if c.Auth.JWT.Secret == "default-secret-change-in-production" {
			return fmt.Errorf("JWT secret must be changed from default value in production")
		}
		if len(c.Auth.JWT.Secret) < 32 {
			return fmt.Errorf("JWT secret must be at least 32 characters long")
		}
		if c.Auth.JWT.Issuer == "" {
			return fmt.Errorf("JWT issuer is required")
		}
		if c.Auth.JWT.TTL <= 0 {
			return fmt.Errorf("JWT TTL must be positive")
		}
	}

	// Pool validation
	if c.Pool.MaxRepositories <= 0 {
		return fmt.Errorf("pool max repositories must be positive")
	}
	if c.Pool.MaxIdleTime <= 0 {
		return fmt.Errorf("pool max idle time must be positive")
	}
	if c.Pool.CleanupInterval <= 0 {
		return fmt.Errorf("pool cleanup interval must be positive")
	}

	// Logging validation
	validLevels := map[string]bool{
		"DEBUG": true, "INFO": true, "WARN": true, "ERROR": true, "FATAL": true,
	}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}
	validFormats := map[string]bool{
		"json": true, "text": true,
	}
	if !validFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format: %s", c.Logging.Format)
	}

	// Storage validation
	validStorageTypes := map[string]bool{
		"memory": true, "disk": true, "hybrid": true,
	}
	if !validStorageTypes[c.Storage.Type] {
		return fmt.Errorf("invalid storage type: %s", c.Storage.Type)
	}
	if (c.Storage.Type == "disk" || c.Storage.Type == "hybrid") && c.Storage.DiskPath == "" {
		return fmt.Errorf("disk path is required for disk/hybrid storage")
	}
	if c.Storage.MaxMemoryMB <= 0 {
		return fmt.Errorf("max memory must be positive")
	}

	return nil
}

// Address returns the server address
func (c *Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.Development.Debug
}

// GetLogLevel returns the logging level as a structured enum
func (c *Config) GetLogLevel() LogLevel {
	switch c.Logging.Level {
	case "DEBUG":
		return DebugLevel
	case "INFO":
		return InfoLevel
	case "WARN":
		return WarnLevel
	case "ERROR":
		return ErrorLevel
	case "FATAL":
		return FatalLevel
	default:
		return InfoLevel
	}
}

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