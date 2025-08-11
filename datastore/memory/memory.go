// Package memory implements an in-memory datastore for govc.
// This is the fastest implementation but provides no persistence.
// Perfect for development, testing, and temporary operations.
package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Caia-Tech/govc/datastore"
	"github.com/google/uuid"
)

// MemoryStore implements datastore.DataStore using in-memory maps
type MemoryStore struct {
	// Object storage
	objects   map[string][]byte
	objectsMu sync.RWMutex

	// Repository metadata
	repositories   map[string]*datastore.Repository
	repositoriesMu sync.RWMutex

	// Users
	users        map[string]*datastore.User
	usersByName  map[string]*datastore.User
	usersByEmail map[string]*datastore.User
	usersMu      sync.RWMutex

	// References (key: repoID:refName)
	refs   map[string]*datastore.Reference
	refsMu sync.RWMutex

	// Audit events
	events   []*datastore.AuditEvent
	eventsMu sync.RWMutex

	// Configuration
	config   map[string]string
	configMu sync.RWMutex

	// Metrics
	reads   atomic.Int64
	writes  atomic.Int64
	deletes atomic.Int64

	// Settings
	maxSize   int64
	currentSize atomic.Int64

	// Lifecycle
	startTime time.Time
	closed    atomic.Bool
}

// init registers the memory store factory
func init() {
	datastore.Register(datastore.TypeMemory, func(config datastore.Config) (datastore.DataStore, error) {
		return New(config), nil
	})
}

// New creates a new memory store
func New(config datastore.Config) *MemoryStore {
	maxSize := config.GetIntOption("max_size", 1024*1024*1024) // Default 1GB

	return &MemoryStore{
		objects:      make(map[string][]byte),
		repositories: make(map[string]*datastore.Repository),
		users:        make(map[string]*datastore.User),
		usersByName:  make(map[string]*datastore.User),
		usersByEmail: make(map[string]*datastore.User),
		refs:         make(map[string]*datastore.Reference),
		events:       make([]*datastore.AuditEvent, 0),
		config:       make(map[string]string),
		maxSize:      int64(maxSize),
		startTime:    time.Now(),
	}
}

// Initialize initializes the memory store
func (m *MemoryStore) Initialize(config datastore.Config) error {
	// Memory store doesn't need initialization
	return nil
}

// Close closes the memory store
func (m *MemoryStore) Close() error {
	if m.closed.Swap(true) {
		return fmt.Errorf("already closed")
	}

	// Clear all data
	m.objectsMu.Lock()
	m.objects = nil
	m.objectsMu.Unlock()

	m.repositoriesMu.Lock()
	m.repositories = nil
	m.repositoriesMu.Unlock()

	m.usersMu.Lock()
	m.users = nil
	m.usersByName = nil
	m.usersByEmail = nil
	m.usersMu.Unlock()

	return nil
}

// HealthCheck checks if the store is healthy
func (m *MemoryStore) HealthCheck(ctx context.Context) error {
	if m.closed.Load() {
		return fmt.Errorf("store is closed")
	}

	// Check memory usage
	if m.currentSize.Load() > m.maxSize {
		return fmt.Errorf("memory limit exceeded: %d > %d", m.currentSize.Load(), m.maxSize)
	}

	return nil
}

// Type returns the store type
func (m *MemoryStore) Type() string {
	return datastore.TypeMemory
}

// Info returns store information
func (m *MemoryStore) Info() map[string]interface{} {
	m.objectsMu.RLock()
	objectCount := len(m.objects)
	m.objectsMu.RUnlock()

	m.repositoriesMu.RLock()
	repoCount := len(m.repositories)
	m.repositoriesMu.RUnlock()

	m.usersMu.RLock()
	userCount := len(m.users)
	m.usersMu.RUnlock()

	return map[string]interface{}{
		"type":         datastore.TypeMemory,
		"objects":      objectCount,
		"repositories": repoCount,
		"users":        userCount,
		"memory_used":  m.currentSize.Load(),
		"memory_limit": m.maxSize,
		"uptime":       time.Since(m.startTime),
	}
}

// ObjectStore returns the object store interface
func (m *MemoryStore) ObjectStore() datastore.ObjectStore {
	return m
}

// MetadataStore returns the metadata store interface
func (m *MemoryStore) MetadataStore() datastore.MetadataStore {
	return m
}

// BeginTx begins a transaction
func (m *MemoryStore) BeginTx(ctx context.Context, opts *datastore.TxOptions) (datastore.Transaction, error) {
	// Create a transaction wrapper
	tx := &memoryTransaction{
		store:     m,
		ctx:       ctx,
		opts:      opts,
		mutations: make(map[string]interface{}),
		deleted:   make(map[string]bool),
	}
	return tx, nil
}

// GetMetrics returns store metrics
func (m *MemoryStore) GetMetrics() datastore.Metrics {
	uptime := time.Since(m.startTime)

	m.objectsMu.RLock()
	objectCount := int64(len(m.objects))
	m.objectsMu.RUnlock()

	return datastore.Metrics{
		Reads:        m.reads.Load(),
		Writes:       m.writes.Load(),
		Deletes:      m.deletes.Load(),
		ObjectCount:  objectCount,
		StorageSize:  m.currentSize.Load(),
		StartTime:    m.startTime,
		Uptime:       uptime,
	}
}

// ObjectStore implementation

// GetObject retrieves an object by hash
func (m *MemoryStore) GetObject(hash string) ([]byte, error) {
	m.reads.Add(1)

	m.objectsMu.RLock()
	defer m.objectsMu.RUnlock()

	if data, exists := m.objects[hash]; exists {
		// Return a copy to prevent external modifications
		result := make([]byte, len(data))
		copy(result, data)
		return result, nil
	}

	return nil, datastore.ErrNotFound
}

// PutObject stores an object
func (m *MemoryStore) PutObject(hash string, data []byte) error {
	m.writes.Add(1)

	// Check size limit
	if int64(len(data)) > m.maxSize {
		return fmt.Errorf("object too large: %d > %d", len(data), m.maxSize)
	}

	m.objectsMu.Lock()
	defer m.objectsMu.Unlock()

	// Check if object already exists
	if existing, exists := m.objects[hash]; exists {
		// Update size tracking
		m.currentSize.Add(-int64(len(existing)))
	}

	// Store a copy to prevent external modifications
	stored := make([]byte, len(data))
	copy(stored, data)
	m.objects[hash] = stored
	m.currentSize.Add(int64(len(data)))

	return nil
}

// DeleteObject deletes an object
func (m *MemoryStore) DeleteObject(hash string) error {
	m.deletes.Add(1)

	m.objectsMu.Lock()
	defer m.objectsMu.Unlock()

	if data, exists := m.objects[hash]; exists {
		delete(m.objects, hash)
		m.currentSize.Add(-int64(len(data)))
		return nil
	}

	return datastore.ErrNotFound
}

// HasObject checks if an object exists
func (m *MemoryStore) HasObject(hash string) (bool, error) {
	m.reads.Add(1)

	m.objectsMu.RLock()
	defer m.objectsMu.RUnlock()

	_, exists := m.objects[hash]
	return exists, nil
}

// ListObjects lists objects with a prefix
func (m *MemoryStore) ListObjects(prefix string, limit int) ([]string, error) {
	m.reads.Add(1)

	m.objectsMu.RLock()
	defer m.objectsMu.RUnlock()

	var results []string
	for hash := range m.objects {
		if strings.HasPrefix(hash, prefix) {
			results = append(results, hash)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// IterateObjects iterates over objects with a prefix
func (m *MemoryStore) IterateObjects(prefix string, fn func(hash string, data []byte) error) error {
	m.reads.Add(1)

	m.objectsMu.RLock()
	defer m.objectsMu.RUnlock()

	for hash, data := range m.objects {
		if strings.HasPrefix(hash, prefix) {
			if err := fn(hash, data); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetObjects retrieves multiple objects
func (m *MemoryStore) GetObjects(hashes []string) (map[string][]byte, error) {
	m.reads.Add(int64(len(hashes)))

	m.objectsMu.RLock()
	defer m.objectsMu.RUnlock()

	results := make(map[string][]byte)
	for _, hash := range hashes {
		if data, exists := m.objects[hash]; exists {
			// Return a copy
			copied := make([]byte, len(data))
			copy(copied, data)
			results[hash] = copied
		}
	}

	return results, nil
}

// PutObjects stores multiple objects
func (m *MemoryStore) PutObjects(objects map[string][]byte) error {
	m.writes.Add(int64(len(objects)))

	// Calculate total size
	totalSize := int64(0)
	for _, data := range objects {
		totalSize += int64(len(data))
	}

	if totalSize > m.maxSize {
		return fmt.Errorf("batch too large: %d > %d", totalSize, m.maxSize)
	}

	m.objectsMu.Lock()
	defer m.objectsMu.Unlock()

	for hash, data := range objects {
		// Store a copy
		stored := make([]byte, len(data))
		copy(stored, data)
		
		// Update size tracking
		if existing, exists := m.objects[hash]; exists {
			m.currentSize.Add(-int64(len(existing)))
		}
		
		m.objects[hash] = stored
		m.currentSize.Add(int64(len(data)))
	}

	return nil
}

// DeleteObjects deletes multiple objects
func (m *MemoryStore) DeleteObjects(hashes []string) error {
	m.deletes.Add(int64(len(hashes)))

	m.objectsMu.Lock()
	defer m.objectsMu.Unlock()

	for _, hash := range hashes {
		if data, exists := m.objects[hash]; exists {
			delete(m.objects, hash)
			m.currentSize.Add(-int64(len(data)))
		}
	}

	return nil
}

// GetObjectSize returns the size of an object
func (m *MemoryStore) GetObjectSize(hash string) (int64, error) {
	m.reads.Add(1)

	m.objectsMu.RLock()
	defer m.objectsMu.RUnlock()

	if data, exists := m.objects[hash]; exists {
		return int64(len(data)), nil
	}

	return 0, datastore.ErrNotFound
}

// CountObjects returns the number of objects
func (m *MemoryStore) CountObjects() (int64, error) {
	m.objectsMu.RLock()
	defer m.objectsMu.RUnlock()

	return int64(len(m.objects)), nil
}

// GetStorageSize returns the total storage size
func (m *MemoryStore) GetStorageSize() (int64, error) {
	return m.currentSize.Load(), nil
}

// MetadataStore implementation

// SaveRepository saves a repository
func (m *MemoryStore) SaveRepository(repo *datastore.Repository) error {
	if repo.ID == "" {
		repo.ID = uuid.New().String()
	}
	if repo.CreatedAt.IsZero() {
		repo.CreatedAt = time.Now()
	}
	repo.UpdatedAt = time.Now()

	m.repositoriesMu.Lock()
	defer m.repositoriesMu.Unlock()

	m.repositories[repo.ID] = repo
	m.writes.Add(1)
	return nil
}

// GetRepository retrieves a repository
func (m *MemoryStore) GetRepository(id string) (*datastore.Repository, error) {
	m.repositoriesMu.RLock()
	defer m.repositoriesMu.RUnlock()

	m.reads.Add(1)
	if repo, exists := m.repositories[id]; exists {
		return repo, nil
	}
	return nil, datastore.ErrNotFound
}

// Additional methods would continue here...
// This is a partial implementation showing the pattern