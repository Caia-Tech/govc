package mongodb

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/Caia-Tech/govc/datastore"
)

// Register MongoDB adapter with the datastore factory
func init() {
	datastore.Register(datastore.TypeMongoDB, func(config datastore.Config) (datastore.DataStore, error) {
		return New(config)
	})
}

// MongoStore implements DataStore using MongoDB
type MongoStore struct {
	client       *mongo.Client
	database     *mongo.Database
	config       datastore.Config
	metrics      datastore.Metrics
	objectStore  *ObjectStore
	metadataStore *MetadataStore
	mu           sync.RWMutex
	closed       bool
}

// New creates a new MongoDB-backed datastore
func New(config datastore.Config) (*MongoStore, error) {
	if config.Connection == "" {
		config.Connection = "mongodb://localhost:27017/govc"
	}

	store := &MongoStore{
		config: config,
		metrics: datastore.Metrics{
			StartTime: time.Now(),
		},
	}

	return store, nil
}

// Initialize initializes the MongoDB connection
func (s *MongoStore) Initialize(config datastore.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	// Parse MongoDB options
	clientOptions := options.Client().ApplyURI(config.Connection)
	
	// Apply connection pool settings
	if config.MaxConnections > 0 {
		clientOptions.SetMaxPoolSize(uint64(config.MaxConnections))
	}
	if config.MaxIdleConnections > 0 {
		clientOptions.SetMinPoolSize(uint64(config.MaxIdleConnections))
	}
	if config.ConnectionTimeout > 0 {
		clientOptions.SetConnectTimeout(config.ConnectionTimeout)
	}

	// Apply additional options from config
	if readPreference := config.GetStringOption("read_preference", ""); readPreference != "" {
		// Would set read preference based on string value
		// This is simplified for this implementation
	}

	// Create MongoDB client
	client, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	s.client = client

	// Get database name from connection string or use default
	dbName := "govc"
	// For simplicity, extract from connection string manually or use default
	// In production, would use proper URI parsing
	s.database = client.Database(dbName)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Ping(ctx, nil); err != nil {
		s.client.Disconnect(context.Background())
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	// Initialize stores
	s.objectStore = NewObjectStore(s.database, config)
	s.metadataStore = NewMetadataStore(s.database, config)

	return nil
}

// Close closes the MongoDB connection
func (s *MongoStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	if s.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.client.Disconnect(ctx)
	}

	return nil
}

// HealthCheck verifies MongoDB is accessible
func (s *MongoStore) HealthCheck(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	if s.client == nil {
		return fmt.Errorf("MongoDB client not initialized")
	}

	return s.client.Ping(ctx, nil)
}

// ObjectStore returns the object store interface
func (s *MongoStore) ObjectStore() datastore.ObjectStore {
	return s.objectStore
}

// MetadataStore returns the metadata store interface
func (s *MongoStore) MetadataStore() datastore.MetadataStore {
	return s.metadataStore
}

// BeginTx starts a new transaction
func (s *MongoStore) BeginTx(ctx context.Context, opts *datastore.TxOptions) (datastore.Transaction, error) {
	if s.client == nil {
		return nil, fmt.Errorf("MongoDB client not initialized")
	}

	// Start MongoDB session
	session, err := s.client.StartSession()
	if err != nil {
		return nil, fmt.Errorf("failed to start MongoDB session: %w", err)
	}

	return NewMongoTransaction(session, s.database, ctx)
}

// GetMetrics returns store metrics
func (s *MongoStore) GetMetrics() datastore.Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := s.metrics
	metrics.Uptime = time.Since(s.metrics.StartTime)

	// Get MongoDB server status for additional metrics
	if s.client != nil && s.database != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		var result map[string]interface{}
		err := s.database.RunCommand(ctx, map[string]interface{}{"serverStatus": 1}).Decode(&result)
		if err == nil {
			// Parse connection metrics
			if connections, ok := result["connections"].(map[string]interface{}); ok {
				if current, ok := connections["current"].(int32); ok {
					metrics.ActiveConnections = int(current)
				}
				if totalCreated, ok := connections["totalCreated"].(int64); ok {
					metrics.TotalConnections = totalCreated
				}
			}
		}
	}

	return metrics
}

// Type returns the datastore type
func (s *MongoStore) Type() string {
	return datastore.TypeMongoDB
}

// Info returns store information
func (s *MongoStore) Info() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info := map[string]interface{}{
		"type":       datastore.TypeMongoDB,
		"connection": s.config.Connection,
		"status":     "healthy",
	}

	if s.closed {
		info["status"] = "closed"
	}

	if s.client != nil && s.database != nil {
		info["database"] = s.database.Name()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		var result map[string]interface{}
		err := s.database.RunCommand(ctx, map[string]interface{}{"buildInfo": 1}).Decode(&result)
		if err == nil {
			if version, ok := result["version"].(string); ok {
				info["server_version"] = version
			}
		}
	}

	return info
}