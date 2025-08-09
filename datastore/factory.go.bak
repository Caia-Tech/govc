package datastore

import (
	"fmt"
	"sync"
)

// Factory is a function that creates a DataStore instance
type Factory func(config Config) (DataStore, error)

// Registry manages datastore factories
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// global registry instance
var globalRegistry = &Registry{
	factories: make(map[string]Factory),
}

// Register registers a new datastore factory
func Register(name string, factory Factory) {
	globalRegistry.Register(name, factory)
}

// Create creates a new datastore instance
func Create(config Config) (DataStore, error) {
	return globalRegistry.Create(config)
}

// ListTypes returns all registered datastore types
func ListTypes() []string {
	return globalRegistry.ListTypes()
}

// Register registers a factory for a datastore type
func (r *Registry) Register(name string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if factory == nil {
		panic("datastore: Register factory is nil")
	}
	
	if _, exists := r.factories[name]; exists {
		panic(fmt.Sprintf("datastore: Register called twice for factory %s", name))
	}
	
	r.factories[name] = factory
}

// Create creates a new datastore using the registered factory
func (r *Registry) Create(config Config) (DataStore, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	r.mu.RLock()
	factory, exists := r.factories[config.Type]
	r.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("unknown datastore type: %s", config.Type)
	}
	
	// Create the datastore
	ds, err := factory(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s datastore: %w", config.Type, err)
	}
	
	// Initialize the datastore
	if err := ds.Initialize(config); err != nil {
		// Try to close if initialization failed
		_ = ds.Close()
		return nil, fmt.Errorf("failed to initialize %s datastore: %w", config.Type, err)
	}
	
	// Wrap with decorators if needed
	ds = wrapWithCache(ds, config)
	ds = wrapWithMetrics(ds, config)
	ds = wrapWithCompression(ds, config)
	
	return ds, nil
}

// ListTypes returns all registered datastore types
func (r *Registry) ListTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	types := make([]string, 0, len(r.factories))
	for name := range r.factories {
		types = append(types, name)
	}
	return types
}

// wrapWithCache wraps the datastore with caching if configured
func wrapWithCache(ds DataStore, config Config) DataStore {
	if config.Cache != nil && config.Cache.Enabled {
		// TODO: Implement caching decorator
		// return NewCachedDataStore(ds, config.Cache)
	}
	return ds
}

// wrapWithMetrics wraps the datastore with metrics collection
func wrapWithMetrics(ds DataStore, config Config) DataStore {
	// TODO: Implement metrics decorator
	// return NewMetricsDataStore(ds)
	return ds
}

// wrapWithCompression wraps the datastore with compression
func wrapWithCompression(ds DataStore, config Config) DataStore {
	if config.EnableCompression {
		// TODO: Implement compression decorator
		// return NewCompressedDataStore(ds)
	}
	return ds
}

// MultiStore allows using different stores for different data types
type MultiStore struct {
	objects  DataStore
	metadata DataStore
	cache    DataStore
}

// NewMultiStore creates a multi-store configuration
func NewMultiStore(objectsConfig, metadataConfig, cacheConfig Config) (*MultiStore, error) {
	ms := &MultiStore{}
	
	// Create object store
	if objectsConfig.Type != "" {
		objects, err := Create(objectsConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create object store: %w", err)
		}
		ms.objects = objects
	}
	
	// Create metadata store
	if metadataConfig.Type != "" {
		metadata, err := Create(metadataConfig)
		if err != nil {
			// Clean up object store if metadata fails
			if ms.objects != nil {
				_ = ms.objects.Close()
			}
			return nil, fmt.Errorf("failed to create metadata store: %w", err)
		}
		ms.metadata = metadata
	}
	
	// Create cache store
	if cacheConfig.Type != "" {
		cache, err := Create(cacheConfig)
		if err != nil {
			// Clean up previous stores
			if ms.objects != nil {
				_ = ms.objects.Close()
			}
			if ms.metadata != nil {
				_ = ms.metadata.Close()
			}
			return nil, fmt.Errorf("failed to create cache store: %w", err)
		}
		ms.cache = cache
	}
	
	// Default to same store if not specified
	if ms.metadata == nil && ms.objects != nil {
		ms.metadata = ms.objects
	}
	if ms.objects == nil && ms.metadata != nil {
		ms.objects = ms.metadata
	}
	
	return ms, nil
}

// CreateFromYAML creates a datastore from YAML configuration
func CreateFromYAML(yamlConfig string) (DataStore, error) {
	// TODO: Parse YAML and create datastore
	// This would use yaml.Unmarshal to parse the config
	// and then call Create with the parsed Config
	return nil, fmt.Errorf("not implemented yet")
}

// CreateFromJSON creates a datastore from JSON configuration
func CreateFromJSON(jsonConfig string) (DataStore, error) {
	// TODO: Parse JSON and create datastore
	// This would use json.Unmarshal to parse the config
	// and then call Create with the parsed Config
	return nil, fmt.Errorf("not implemented yet")
}

// CreateFromEnv creates a datastore from environment variables
func CreateFromEnv() (DataStore, error) {
	// TODO: Read environment variables and create config
	// Example env vars:
	// GOVC_DATASTORE_TYPE=postgres
	// GOVC_DATASTORE_CONNECTION=postgres://localhost/govc
	// GOVC_DATASTORE_MAX_CONNECTIONS=25
	return nil, fmt.Errorf("not implemented yet")
}