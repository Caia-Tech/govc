package datastore

import (
	"fmt"
	"time"
)

// Config represents the configuration for a datastore
type Config struct {
	Type       string                 `yaml:"type" json:"type"`
	Connection string                 `yaml:"connection" json:"connection"`
	Options    map[string]interface{} `yaml:"options" json:"options"`
	
	// Advanced configurations
	Cache       *CacheConfig       `yaml:"cache" json:"cache"`
	Sharding    *ShardingConfig    `yaml:"sharding" json:"sharding"`
	Replication *ReplicationConfig `yaml:"replication" json:"replication"`
	
	// Connection pool settings
	MaxConnections     int           `yaml:"max_connections" json:"max_connections"`
	MaxIdleConnections int           `yaml:"max_idle_connections" json:"max_idle_connections"`
	ConnectionTimeout  time.Duration `yaml:"connection_timeout" json:"connection_timeout"`
	
	// Performance settings
	BatchSize        int  `yaml:"batch_size" json:"batch_size"`
	EnableCompression bool `yaml:"enable_compression" json:"enable_compression"`
	EnableEncryption  bool `yaml:"enable_encryption" json:"enable_encryption"`
}

// CacheConfig configures caching layer
type CacheConfig struct {
	Enabled  bool          `yaml:"enabled" json:"enabled"`
	Type     string        `yaml:"type" json:"type"` // "memory", "redis"
	Size     int64         `yaml:"size" json:"size"` // Max cache size in bytes
	TTL      time.Duration `yaml:"ttl" json:"ttl"`
	Eviction string        `yaml:"eviction" json:"eviction"` // "lru", "lfu", "fifo"
}

// ShardingConfig configures data sharding
type ShardingConfig struct {
	Enabled      bool     `yaml:"enabled" json:"enabled"`
	ShardCount   int      `yaml:"shard_count" json:"shard_count"`
	ShardKey     string   `yaml:"shard_key" json:"shard_key"`
	ShardServers []string `yaml:"shard_servers" json:"shard_servers"`
}

// ReplicationConfig configures data replication
type ReplicationConfig struct {
	Enabled       bool     `yaml:"enabled" json:"enabled"`
	Mode          string   `yaml:"mode" json:"mode"` // "master-slave", "multi-master"
	Replicas      []string `yaml:"replicas" json:"replicas"`
	SyncMode      string   `yaml:"sync_mode" json:"sync_mode"` // "async", "sync", "semi-sync"
	FailoverAuto  bool     `yaml:"failover_auto" json:"failover_auto"`
}

// DatabaseType constants
const (
	TypeMemory   = "memory"
	TypeSQLite   = "sqlite"
	TypeBadger   = "badger"
	TypePostgres = "postgres"
	TypeMongoDB  = "mongodb"
	TypeRedis    = "redis"
	TypeCustom   = "custom"
	TypeBolt     = "bolt"
)

// DefaultConfig returns default configuration for a database type
func DefaultConfig(dbType string) Config {
	switch dbType {
	case TypeMemory:
		return Config{
			Type: TypeMemory,
			Options: map[string]interface{}{
				"max_size": 1024 * 1024 * 1024, // 1GB
			},
		}
		
	case TypeSQLite:
		return Config{
			Type:       TypeSQLite,
			Connection: "./data/govc.db",
			Options: map[string]interface{}{
				"journal_mode": "WAL",
				"synchronous":  "NORMAL",
				"cache_size":   10000,
				"foreign_keys": true,
			},
		}
		
	case TypeBadger:
		return Config{
			Type:       TypeBadger,
			Connection: "./data/badger",
			Options: map[string]interface{}{
				"compression":          "snappy",
				"value_log_file_size": 1024 * 1024 * 1024, // 1GB
				"value_log_max_entries": 1000000,
				"sync_writes":          false,
			},
		}
		
	case TypePostgres:
		return Config{
			Type:               TypePostgres,
			Connection:         "postgres://localhost/govc?sslmode=disable",
			MaxConnections:     25,
			MaxIdleConnections: 5,
			ConnectionTimeout:  30 * time.Second,
			Options: map[string]interface{}{
				"pool_size":      25,
				"ssl_mode":       "prefer",
				"statement_cache_capacity": 100,
			},
		}
		
	case TypeMongoDB:
		return Config{
			Type:               TypeMongoDB,
			Connection:         "mongodb://localhost:27017/govc",
			MaxConnections:     50,
			ConnectionTimeout:  30 * time.Second,
			Options: map[string]interface{}{
				"replica_set":     "",
				"write_concern":   "majority",
				"read_preference": "primaryPreferred",
				"use_gridfs":      true,
			},
		}
		
	case TypeRedis:
		return Config{
			Type:               TypeRedis,
			Connection:         "redis://localhost:6379/0",
			MaxConnections:     10,
			MaxIdleConnections: 5,
			ConnectionTimeout:  10 * time.Second,
			Options: map[string]interface{}{
				"password":      "",
				"max_retries":   3,
				"pool_size":     10,
				"read_timeout":  "3s",
				"write_timeout": "3s",
			},
		}
		
	default:
		return Config{
			Type: dbType,
		}
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Type == "" {
		return fmt.Errorf("database type is required")
	}
	
	switch c.Type {
	case TypeMemory:
		// Memory doesn't need connection string
		
	case TypeSQLite, TypeBadger:
		if c.Connection == "" {
			return fmt.Errorf("%s requires a file path", c.Type)
		}
		
	case TypePostgres, TypeMongoDB, TypeRedis:
		if c.Connection == "" {
			return fmt.Errorf("%s requires a connection string", c.Type)
		}
		
	default:
		// Custom types handle their own validation
	}
	
	// Validate connection pool settings
	if c.MaxConnections < 0 {
		return fmt.Errorf("max_connections cannot be negative")
	}
	if c.MaxIdleConnections < 0 {
		return fmt.Errorf("max_idle_connections cannot be negative")
	}
	if c.MaxIdleConnections > c.MaxConnections {
		return fmt.Errorf("max_idle_connections cannot exceed max_connections")
	}
	if c.ConnectionTimeout < 0 {
		return fmt.Errorf("connection_timeout must be positive")
	}
	
	return nil
}

// GetOption retrieves a typed option value
func (c *Config) GetOption(key string, defaultValue interface{}) interface{} {
	if c.Options == nil {
		return defaultValue
	}
	
	if value, exists := c.Options[key]; exists {
		return value
	}
	
	return defaultValue
}

// GetStringOption retrieves a string option
func (c *Config) GetStringOption(key string, defaultValue string) string {
	value := c.GetOption(key, defaultValue)
	if str, ok := value.(string); ok {
		return str
	}
	return defaultValue
}

// GetIntOption retrieves an integer option
func (c *Config) GetIntOption(key string, defaultValue int) int {
	value := c.GetOption(key, defaultValue)
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return defaultValue
	}
}

// GetBoolOption retrieves a boolean option
func (c *Config) GetBoolOption(key string, defaultValue bool) bool {
	value := c.GetOption(key, defaultValue)
	if b, ok := value.(bool); ok {
		return b
	}
	return defaultValue
}

// GetDurationOption retrieves a duration option
func (c *Config) GetDurationOption(key string, defaultValue time.Duration) time.Duration {
	value := c.GetOption(key, defaultValue)
	switch v := value.(type) {
	case time.Duration:
		return v
	case string:
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	case int64:
		return time.Duration(v)
	}
	return defaultValue
}