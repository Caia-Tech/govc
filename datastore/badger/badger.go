// Package badger implements a BadgerDB-backed datastore for govc.
// BadgerDB is an embedded, persistent key-value database optimized for SSD.
// It's particularly well-suited for Git objects due to its LSM-tree architecture.
package badger

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Caia-Tech/govc/datastore"
	badgerdb "github.com/dgraph-io/badger/v4"
)

// BadgerStore implements datastore.DataStore using BadgerDB
type BadgerStore struct {
	db       *badgerdb.DB
	path     string
	config   datastore.Config
	
	// Metrics
	reads    atomic.Int64
	writes   atomic.Int64
	deletes  atomic.Int64
	
	// Lifecycle
	startTime time.Time
	closed    atomic.Bool
}

// Key prefixes for different data types
const (
	prefixObject     = "obj:"
	prefixRepo       = "repo:"
	prefixUser       = "user:"
	prefixUserByName = "uname:" // Changed to avoid prefix conflict
	prefixRef        = "ref:"
	prefixEvent      = "event:"
	prefixConfig     = "cfg:"
)

// init registers the BadgerDB store factory
func init() {
	datastore.Register(datastore.TypeBadger, func(config datastore.Config) (datastore.DataStore, error) {
		return New(config)
	})
}

// New creates a new BadgerDB store
func New(config datastore.Config) (*BadgerStore, error) {
	store := &BadgerStore{
		path:      config.Connection,
		config:    config,
		startTime: time.Now(),
	}
	
	return store, nil
}

// Initialize initializes the BadgerDB store
func (s *BadgerStore) Initialize(config datastore.Config) error {
	if s.db != nil {
		return nil // Already initialized
	}
	
	// Configure BadgerDB options
	opts := badgerdb.DefaultOptions(s.path)
	
	// Apply configuration options
	if v, ok := config.Options["sync_writes"].(bool); ok {
		opts.SyncWrites = v
	} else {
		opts.SyncWrites = false // Default to async for better performance
	}
	
	if v, ok := config.Options["compression"].(bool); ok && v {
		// Compression is enabled by default in v4
		opts.Logger = nil // Disable verbose logging
	}
	
	if v, ok := config.Options["value_log_max_entries"].(int); ok {
		opts.ValueLogMaxEntries = uint32(v)
	}
	
	if v, ok := config.Options["memory_table_size"].(int); ok {
		opts.MemTableSize = int64(v)
	} else {
		opts.MemTableSize = 64 << 20 // 64MB default
	}
	
	// Open database
	db, err := badgerdb.Open(opts)
	if err != nil {
		return fmt.Errorf("failed to open BadgerDB: %w", err)
	}
	
	s.db = db
	
	// Start garbage collection goroutine
	go s.runGC()
	
	return nil
}

// runGC runs periodic garbage collection
func (s *BadgerStore) runGC() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		if s.closed.Load() {
			return
		}
		
		err := s.db.RunValueLogGC(0.5)
		if err != nil && err != badgerdb.ErrNoRewrite {
			// Log error but don't stop GC
			fmt.Printf("BadgerDB GC error: %v\n", err)
		}
	}
}

// Close closes the BadgerDB store
func (s *BadgerStore) Close() error {
	if s.closed.Swap(true) {
		return fmt.Errorf("already closed")
	}
	
	if s.db != nil {
		return s.db.Close()
	}
	
	return nil
}

// HealthCheck checks if the store is healthy
func (s *BadgerStore) HealthCheck(ctx context.Context) error {
	if s.closed.Load() {
		return fmt.Errorf("store is closed")
	}
	
	// Try a simple read operation
	key := []byte(prefixConfig + "health_check")
	err := s.db.View(func(txn *badgerdb.Txn) error {
		_, err := txn.Get(key)
		if err == badgerdb.ErrKeyNotFound {
			return nil // Key not found is OK for health check
		}
		return err
	})
	
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	
	return nil
}

// Type returns the store type
func (s *BadgerStore) Type() string {
	return datastore.TypeBadger
}

// Info returns store information
func (s *BadgerStore) Info() map[string]interface{} {
	lsm, vlog := s.db.Size()
	
	info := map[string]interface{}{
		"type":           datastore.TypeBadger,
		"path":           s.path,
		"lsm_size":       lsm,
		"vlog_size":      vlog,
		"total_size":     lsm + vlog,
		"reads":          s.reads.Load(),
		"writes":         s.writes.Load(),
		"deletes":        s.deletes.Load(),
		"uptime":         time.Since(s.startTime),
	}
	
	// Count objects
	var objectCount int64
	prefix := []byte(prefixObject)
	s.db.View(func(txn *badgerdb.Txn) error {
		opts := badgerdb.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			objectCount++
		}
		return nil
	})
	info["objects"] = objectCount
	
	return info
}

// ObjectStore returns the object store interface
func (s *BadgerStore) ObjectStore() datastore.ObjectStore {
	return s
}

// MetadataStore returns the metadata store interface
func (s *BadgerStore) MetadataStore() datastore.MetadataStore {
	return s
}

// BeginTx begins a transaction
func (s *BadgerStore) BeginTx(ctx context.Context, opts *datastore.TxOptions) (datastore.Transaction, error) {
	var badgerTxn *badgerdb.Txn
	
	if opts != nil && opts.ReadOnly {
		badgerTxn = s.db.NewTransaction(false)
	} else {
		badgerTxn = s.db.NewTransaction(true)
	}
	
	return &badgerTransaction{
		store: s,
		txn:   badgerTxn,
		ctx:   ctx,
	}, nil
}

// GetMetrics returns store metrics
func (s *BadgerStore) GetMetrics() datastore.Metrics {
	lsm, vlog := s.db.Size()
	
	// Count objects
	var objectCount int64
	prefix := []byte(prefixObject)
	s.db.View(func(txn *badgerdb.Txn) error {
		opts := badgerdb.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			objectCount++
		}
		return nil
	})
	
	return datastore.Metrics{
		Reads:       s.reads.Load(),
		Writes:      s.writes.Load(),
		Deletes:     s.deletes.Load(),
		ObjectCount: objectCount,
		StorageSize: lsm + vlog,
		StartTime:   s.startTime,
		Uptime:      time.Since(s.startTime),
	}
}

// Helper functions

func makeObjectKey(hash string) []byte {
	return []byte(prefixObject + hash)
}

func makeRepoKey(id string) []byte {
	return []byte(prefixRepo + id)
}

func makeUserKey(id string) []byte {
	return []byte(prefixUser + id)
}

func makeUserByNameKey(username string) []byte {
	return []byte(prefixUserByName + username)
}

func makeRefKey(repoID, name string) []byte {
	return []byte(prefixRef + repoID + ":" + name)
}

func makeEventKey(id string) []byte {
	return []byte(prefixEvent + id)
}

func makeConfigKey(key string) []byte {
	return []byte(prefixConfig + key)
}

// marshalJSON marshals data to JSON
func marshalJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// unmarshalJSON unmarshals JSON data
func unmarshalJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}