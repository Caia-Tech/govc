package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Caia-Tech/govc/datastore"
)

// Register Redis adapter with the datastore factory
func init() {
	datastore.Register(datastore.TypeRedis, func(config datastore.Config) (datastore.DataStore, error) {
		return New(config)
	})
}

// RedisStore implements DataStore using Redis (primarily for caching and sessions)
type RedisStore struct {
	client      redis.UniversalClient
	config      datastore.Config
	metrics     datastore.Metrics
	objectStore *ObjectStore
	mu          sync.RWMutex
	closed      bool
}

// New creates a new Redis-backed datastore
func New(config datastore.Config) (*RedisStore, error) {
	if config.Connection == "" {
		config.Connection = "redis://localhost:6379"
	}

	// Parse Redis connection options
	opts, err := redis.ParseURL(config.Connection)
	if err != nil {
		return nil, fmt.Errorf("invalid Redis connection string: %w", err)
	}

	store := &RedisStore{
		config: config,
		metrics: datastore.Metrics{
			StartTime: time.Now(),
		},
	}

	// Create Redis client
	store.client = redis.NewClient(opts)

	return store, nil
}

// Initialize initializes the Redis connection
func (s *RedisStore) Initialize(config datastore.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// Initialize object store
	s.objectStore = NewObjectStore(s.client, config)

	return nil
}

// Close closes the Redis connection
func (s *RedisStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	if s.client != nil {
		return s.client.Close()
	}

	return nil
}

// HealthCheck verifies Redis is accessible
func (s *RedisStore) HealthCheck(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	if s.client == nil {
		return fmt.Errorf("Redis client not initialized")
	}

	return s.client.Ping(ctx).Err()
}

// ObjectStore returns the object store interface
func (s *RedisStore) ObjectStore() datastore.ObjectStore {
	return s.objectStore
}

// MetadataStore returns a stub metadata store (Redis is primarily for object caching)
func (s *RedisStore) MetadataStore() datastore.MetadataStore {
	return &StubMetadataStore{}
}

// BeginTx starts a new transaction (Redis has limited transaction support)
func (s *RedisStore) BeginTx(ctx context.Context, opts *datastore.TxOptions) (datastore.Transaction, error) {
	return NewRedisTransaction(s.client, ctx), nil
}

// GetMetrics returns store metrics
func (s *RedisStore) GetMetrics() datastore.Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := s.metrics
	metrics.Uptime = time.Since(s.metrics.StartTime)
	
	// Get Redis info for additional metrics
	if s.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		
		info := s.client.Info(ctx, "memory", "stats")
		if info.Err() == nil {
			// Parse basic info from Redis INFO command
			// In production, you'd parse specific metrics
			metrics.ActiveConnections = 1 // Simplified
		}
	}

	return metrics
}

// Type returns the datastore type
func (s *RedisStore) Type() string {
	return datastore.TypeRedis
}

// Info returns store information
func (s *RedisStore) Info() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info := map[string]interface{}{
		"type":       datastore.TypeRedis,
		"connection": s.config.Connection,
		"status":     "healthy",
	}

	if s.closed {
		info["status"] = "closed"
	}

	if s.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		
		if redisInfo := s.client.Info(ctx, "server"); redisInfo.Err() == nil {
			info["redis_info"] = "available"
		}
	}

	return info
}