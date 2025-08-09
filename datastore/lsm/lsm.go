package lsm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/caiatech/govc/datastore"
)

// Register LSM-tree adapter with the datastore factory
func init() {
	datastore.Register(datastore.TypeCustom, func(config datastore.Config) (datastore.DataStore, error) {
		return New(config)
	})
}

// LSMStore implements a Log-Structured Merge Tree optimized for Git objects
type LSMStore struct {
	config        datastore.Config
	path          string
	metrics       datastore.Metrics
	
	// Core LSM components
	memTable      *MemTable
	immutable     *MemTable
	levels        []*Level
	wal           *WriteAheadLog
	compactor     *Compactor
	
	// Performance optimizations
	bloomFilters  []*BloomFilter
	blockCache    *LRUCache
	
	// Synchronization
	mu            sync.RWMutex
	flushMu       sync.Mutex
	compactionMu  sync.Mutex
	closed        bool
	
	// Background processes
	flushCh       chan struct{}
	compactCh     chan int
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// LSMConfig holds LSM-tree specific configuration
type LSMConfig struct {
	// MemTable configuration
	MemTableSize    int64  // Size in bytes before flush
	MaxMemTables    int    // Max number of immutable memtables
	
	// Level configuration  
	MaxLevels       int    // Maximum number of levels (default: 7)
	LevelMultiplier int    // Size multiplier between levels (default: 10)
	Level0Files     int    // Max files in L0 before compaction (default: 4)
	
	// SSTable configuration
	SSTableSize     int64  // Target SSTable size in bytes
	BlockSize       int    // Block size for SSTables (default: 4KB)
	
	// Performance tuning
	BloomFilterBits int    // Bits per key for bloom filters (default: 10)
	BlockCacheSize  int64  // LRU cache size for blocks
	Compression     CompressionType
	
	// Compaction tuning
	CompactionThreads int  // Background compaction threads
	CompactionStyle   CompactionStyle
}

// CompressionType defines compression algorithm
type CompressionType int

const (
	NoCompression CompressionType = iota
	Snappy
	ZSTD
	LZ4
)

// CompactionStyle defines compaction strategy
type CompactionStyle int

const (
	LeveledCompaction CompactionStyle = iota
	UniversalCompaction
)

// New creates a new LSM-tree datastore
func New(config datastore.Config) (*LSMStore, error) {
	if config.Connection == "" {
		return nil, fmt.Errorf("LSM store requires a directory path")
	}

	// Parse LSM-specific configuration
	lsmConfig := parseLSMConfig(config)
	
	store := &LSMStore{
		config:      config,
		path:        config.Connection,
		metrics:     datastore.Metrics{StartTime: time.Now()},
		levels:      make([]*Level, lsmConfig.MaxLevels),
		flushCh:     make(chan struct{}, 1),
		compactCh:   make(chan int, lsmConfig.MaxLevels),
		stopCh:      make(chan struct{}),
	}
	
	return store, nil
}

// parseLSMConfig extracts LSM-specific configuration from generic config
func parseLSMConfig(config datastore.Config) LSMConfig {
	return LSMConfig{
		MemTableSize:      int64(config.GetIntOption("memtable_size", 64*1024*1024)), // 64MB
		MaxMemTables:      config.GetIntOption("max_memtables", 2),
		MaxLevels:         config.GetIntOption("max_levels", 7),
		LevelMultiplier:   config.GetIntOption("level_multiplier", 10),
		Level0Files:       config.GetIntOption("level0_files", 4),
		SSTableSize:       int64(config.GetIntOption("sstable_size", 256*1024*1024)), // 256MB
		BlockSize:         config.GetIntOption("block_size", 4*1024),          // 4KB
		BloomFilterBits:   config.GetIntOption("bloom_bits", 10),
		BlockCacheSize:    int64(config.GetIntOption("block_cache_size", 128*1024*1024)), // 128MB
		Compression:       CompressionType(config.GetIntOption("compression", int(Snappy))),
		CompactionThreads: config.GetIntOption("compaction_threads", 1),
		CompactionStyle:   CompactionStyle(config.GetIntOption("compaction_style", int(LeveledCompaction))),
	}
}

// Initialize initializes the LSM store
func (s *LSMStore) Initialize(config datastore.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.closed {
		return fmt.Errorf("store is closed")
	}
	
	lsmConfig := parseLSMConfig(config)
	
	// Initialize Write Ahead Log
	wal, err := NewWriteAheadLog(s.path)
	if err != nil {
		return fmt.Errorf("failed to initialize WAL: %w", err)
	}
	s.wal = wal
	
	// Initialize MemTable
	s.memTable = NewMemTable(lsmConfig.MemTableSize)
	
	// Initialize levels
	for i := 0; i < lsmConfig.MaxLevels; i++ {
		level, err := NewLevel(i, s.path, lsmConfig)
		if err != nil {
			return fmt.Errorf("failed to initialize level %d: %w", i, err)
		}
		s.levels[i] = level
	}
	
	// Initialize bloom filters
	s.bloomFilters = make([]*BloomFilter, lsmConfig.MaxLevels)
	for i := 0; i < lsmConfig.MaxLevels; i++ {
		s.bloomFilters[i] = NewBloomFilter(1000000, lsmConfig.BloomFilterBits)
	}
	
	// Initialize block cache
	s.blockCache = NewLRUCache(lsmConfig.BlockCacheSize)
	
	// Initialize compactor
	s.compactor = NewCompactor(s, lsmConfig)
	
	// Start background processes
	s.startBackgroundProcesses()
	
	// Recover from WAL if needed
	if err := s.recover(); err != nil {
		return fmt.Errorf("failed to recover from WAL: %w", err)
	}
	
	return nil
}

// startBackgroundProcesses starts flush and compaction goroutines
func (s *LSMStore) startBackgroundProcesses() {
	// Flush goroutine
	s.wg.Add(1)
	go s.flushWorker()
	
	// Compaction goroutine
	s.wg.Add(1) 
	go s.compactionWorker()
}

// flushWorker handles memtable flushes
func (s *LSMStore) flushWorker() {
	defer s.wg.Done()
	
	for {
		select {
		case <-s.flushCh:
			s.flushMemTable()
		case <-s.stopCh:
			return
		}
	}
}

// compactionWorker handles level compactions
func (s *LSMStore) compactionWorker() {
	defer s.wg.Done()
	
	for {
		select {
		case level := <-s.compactCh:
			s.compactLevel(level)
		case <-s.stopCh:
			return
		}
	}
}

// recover recovers from write-ahead log
func (s *LSMStore) recover() error {
	if s.wal == nil {
		return nil
	}
	
	entries, err := s.wal.ReadAll()
	if err != nil {
		return err
	}
	
	for _, entry := range entries {
		s.memTable.Put(entry.Key, entry.Value)
	}
	
	// Clear WAL after successful recovery
	return s.wal.Truncate()
}

// Close closes the LSM store
func (s *LSMStore) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	
	// Stop background processes
	close(s.stopCh)
	s.wg.Wait()
	
	// Flush final memtable (without holding the main mutex)
	s.flushMemTable()
	
	// Close WAL
	if s.wal != nil {
		s.wal.Close()
	}
	
	// Close levels
	for _, level := range s.levels {
		if level != nil {
			level.Close()
		}
	}
	
	return nil
}

// HealthCheck verifies the LSM store is operational
func (s *LSMStore) HealthCheck(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.closed {
		return fmt.Errorf("store is closed")
	}
	
	// Check that memtable is accessible
	if s.memTable == nil {
		return fmt.Errorf("memtable not initialized")
	}
	
	// Check that levels are accessible
	for i, level := range s.levels {
		if level == nil {
			return fmt.Errorf("level %d not initialized", i)
		}
	}
	
	return nil
}

// ObjectStore returns the LSM object store interface
func (s *LSMStore) ObjectStore() datastore.ObjectStore {
	return &ObjectStore{lsm: s}
}

// MetadataStore returns a stub metadata store (LSM focuses on object storage)
func (s *LSMStore) MetadataStore() datastore.MetadataStore {
	return &StubMetadataStore{}
}

// BeginTx starts a new transaction (simplified for LSM)
func (s *LSMStore) BeginTx(ctx context.Context, opts *datastore.TxOptions) (datastore.Transaction, error) {
	return &LSMTransaction{lsm: s, ctx: ctx}, nil
}

// GetMetrics returns LSM store metrics
func (s *LSMStore) GetMetrics() datastore.Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	metrics := s.metrics
	metrics.Uptime = time.Since(s.metrics.StartTime)
	
	// Add LSM-specific metrics
	if s.memTable != nil {
		metrics.ObjectCount = int64(s.memTable.Size())
		metrics.StorageSize = s.memTable.MemoryUsage()
	}
	
	// Add level statistics
	for _, level := range s.levels {
		if level != nil {
			levelStats := level.Stats()
			metrics.ObjectCount += levelStats.NumKeys
			metrics.StorageSize += levelStats.DiskSize
		}
	}
	
	return metrics
}

// Type returns the datastore type
func (s *LSMStore) Type() string {
	return "lsm"
}

// Info returns LSM store information
func (s *LSMStore) Info() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	info := map[string]interface{}{
		"type":       "lsm",
		"path":       s.path,
		"status":     "healthy",
		"num_levels": len(s.levels),
	}
	
	if s.closed {
		info["status"] = "closed"
	}
	
	// Add level information
	levelInfo := make([]map[string]interface{}, len(s.levels))
	for i, level := range s.levels {
		if level != nil {
			stats := level.Stats()
			levelInfo[i] = map[string]interface{}{
				"level":     i,
				"num_files": stats.NumFiles,
				"num_keys":  stats.NumKeys,
				"disk_size": stats.DiskSize,
			}
		}
	}
	info["levels"] = levelInfo
	
	// Add memtable information
	if s.memTable != nil {
		info["memtable_size"] = s.memTable.Size()
		info["memtable_memory"] = s.memTable.MemoryUsage()
	}
	
	return info
}