package optimize

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// StorageOptimizer optimizes storage operations for govc
type StorageOptimizer struct {
	cache           *LRUCache
	compressor      *AdaptiveCompressor
	batcher         *WriteBatcher
	indexer         *HashIndexer
	deduplicator    *Deduplicator
	
	stats StorageStats
	mu    sync.RWMutex
}

// StorageStats tracks storage performance metrics
type StorageStats struct {
	CacheHits        atomic.Uint64
	CacheMisses      atomic.Uint64
	BytesCompressed  atomic.Uint64
	BytesSaved       atomic.Uint64
	DedupeHits       atomic.Uint64
	BatchedWrites    atomic.Uint64
	TotalWrites      atomic.Uint64
}

// NewStorageOptimizer creates a new storage optimizer
func NewStorageOptimizer(cacheSize int) *StorageOptimizer {
	return &StorageOptimizer{
		cache:        NewLRUCache(cacheSize),
		compressor:   NewAdaptiveCompressor(),
		batcher:      NewWriteBatcher(100, 10*time.Millisecond),
		indexer:      NewHashIndexer(),
		deduplicator: NewDeduplicator(),
	}
}

// LRUCache implements a thread-safe LRU cache
type LRUCache struct {
	capacity int
	items    map[string]*cacheItem
	head     *cacheItem
	tail     *cacheItem
	mu       sync.RWMutex
	
	hits   atomic.Uint64
	misses atomic.Uint64
}

type cacheItem struct {
	key   string
	value []byte
	size  int
	prev  *cacheItem
	next  *cacheItem
	freq  int
	lastAccess time.Time
}

// NewLRUCache creates a new LRU cache
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		items:    make(map[string]*cacheItem),
	}
}

// Get retrieves an item from cache
func (c *LRUCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	item, exists := c.items[key]
	c.mu.RUnlock()
	
	if !exists {
		c.misses.Add(1)
		return nil, false
	}
	
	c.mu.Lock()
	c.moveToFront(item)
	item.freq++
	item.lastAccess = time.Now()
	c.mu.Unlock()
	
	c.hits.Add(1)
	return item.value, true
}

// Put adds an item to cache
func (c *LRUCache) Put(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if item, exists := c.items[key]; exists {
		// Update existing item
		item.value = value
		item.size = len(value)
		c.moveToFront(item)
		return
	}
	
	// Add new item
	item := &cacheItem{
		key:        key,
		value:      value,
		size:       len(value),
		lastAccess: time.Now(),
	}
	
	c.items[key] = item
	c.addToFront(item)
	
	// Evict if over capacity
	if len(c.items) > c.capacity {
		c.evictLRU()
	}
}

func (c *LRUCache) moveToFront(item *cacheItem) {
	if item == c.head {
		return
	}
	
	// Remove from current position
	if item.prev != nil {
		item.prev.next = item.next
	}
	if item.next != nil {
		item.next.prev = item.prev
	}
	if item == c.tail {
		c.tail = item.prev
	}
	
	// Add to front
	item.prev = nil
	item.next = c.head
	if c.head != nil {
		c.head.prev = item
	}
	c.head = item
	if c.tail == nil {
		c.tail = item
	}
}

func (c *LRUCache) addToFront(item *cacheItem) {
	item.next = c.head
	item.prev = nil
	if c.head != nil {
		c.head.prev = item
	}
	c.head = item
	if c.tail == nil {
		c.tail = item
	}
}

func (c *LRUCache) evictLRU() {
	if c.tail == nil {
		return
	}
	
	delete(c.items, c.tail.key)
	
	if c.tail.prev != nil {
		c.tail.prev.next = nil
		c.tail = c.tail.prev
	} else {
		c.head = nil
		c.tail = nil
	}
}

// AdaptiveCompressor chooses optimal compression based on data
type AdaptiveCompressor struct {
	threshold int
	level     int
	mu        sync.RWMutex
	
	stats struct {
		totalIn  atomic.Uint64
		totalOut atomic.Uint64
	}
}

// NewAdaptiveCompressor creates an adaptive compressor
func NewAdaptiveCompressor() *AdaptiveCompressor {
	return &AdaptiveCompressor{
		threshold: 1024, // Don't compress below 1KB
		level:     zlib.DefaultCompression,
	}
}

// Compress compresses data if beneficial
func (ac *AdaptiveCompressor) Compress(data []byte) ([]byte, bool) {
	if len(data) < ac.threshold {
		return data, false
	}
	
	ac.stats.totalIn.Add(uint64(len(data)))
	
	var buf bytes.Buffer
	w, err := zlib.NewWriterLevel(&buf, ac.level)
	if err != nil {
		return data, false
	}
	
	if _, err := w.Write(data); err != nil {
		w.Close()
		return data, false
	}
	w.Close()
	
	compressed := buf.Bytes()
	
	// Only use compression if it saves space
	if len(compressed) >= len(data)*9/10 { // At least 10% savings
		return data, false
	}
	
	ac.stats.totalOut.Add(uint64(len(compressed)))
	return compressed, true
}

// Decompress decompresses data
func (ac *AdaptiveCompressor) Decompress(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	
	return io.ReadAll(r)
}

// WriteBatcher batches multiple writes for efficiency
type WriteBatcher struct {
	batchSize     int
	flushInterval time.Duration
	
	pending []WriteRequest
	mu      sync.Mutex
	flush   chan struct{}
	done    chan struct{}
	
	writer BatchWriter
}

// WriteRequest represents a batched write
type WriteRequest struct {
	Key   string
	Value []byte
	Done  chan error
}

// BatchWriter processes batched writes
type BatchWriter interface {
	WriteBatch(requests []WriteRequest) error
}

// NewWriteBatcher creates a write batcher
func NewWriteBatcher(batchSize int, flushInterval time.Duration) *WriteBatcher {
	wb := &WriteBatcher{
		batchSize:     batchSize,
		flushInterval: flushInterval,
		pending:       make([]WriteRequest, 0, batchSize),
		flush:         make(chan struct{}, 1),
		done:          make(chan struct{}),
	}
	
	go wb.run()
	return wb
}

// Write adds a write request to the batch
func (wb *WriteBatcher) Write(key string, value []byte) error {
	done := make(chan error, 1)
	
	wb.mu.Lock()
	wb.pending = append(wb.pending, WriteRequest{
		Key:   key,
		Value: value,
		Done:  done,
	})
	
	shouldFlush := len(wb.pending) >= wb.batchSize
	wb.mu.Unlock()
	
	if shouldFlush {
		select {
		case wb.flush <- struct{}{}:
		default:
		}
	}
	
	return <-done
}

func (wb *WriteBatcher) run() {
	ticker := time.NewTicker(wb.flushInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			wb.flushPending()
		case <-wb.flush:
			wb.flushPending()
		case <-wb.done:
			wb.flushPending()
			return
		}
	}
}

func (wb *WriteBatcher) flushPending() {
	wb.mu.Lock()
	if len(wb.pending) == 0 {
		wb.mu.Unlock()
		return
	}
	
	batch := wb.pending
	wb.pending = make([]WriteRequest, 0, wb.batchSize)
	wb.mu.Unlock()
	
	var err error
	if wb.writer != nil {
		err = wb.writer.WriteBatch(batch)
	}
	
	// Signal completion
	for _, req := range batch {
		req.Done <- err
	}
}

// Close flushes pending writes and stops the batcher
func (wb *WriteBatcher) Close() {
	close(wb.done)
}

// HashIndexer provides fast hash-based lookups
type HashIndexer struct {
	index map[string]IndexEntry
	mu    sync.RWMutex
}

// IndexEntry stores indexed data location
type IndexEntry struct {
	Offset int64
	Size   int64
	Hash   string
	Time   time.Time
}

// NewHashIndexer creates a hash indexer
func NewHashIndexer() *HashIndexer {
	return &HashIndexer{
		index: make(map[string]IndexEntry),
	}
}

// Add adds an entry to the index
func (hi *HashIndexer) Add(key string, offset, size int64) {
	hash := sha256.Sum256([]byte(key))
	
	hi.mu.Lock()
	hi.index[key] = IndexEntry{
		Offset: offset,
		Size:   size,
		Hash:   hex.EncodeToString(hash[:]),
		Time:   time.Now(),
	}
	hi.mu.Unlock()
}

// Get retrieves an entry from the index
func (hi *HashIndexer) Get(key string) (IndexEntry, bool) {
	hi.mu.RLock()
	entry, exists := hi.index[key]
	hi.mu.RUnlock()
	return entry, exists
}

// Deduplicator eliminates duplicate data storage
type Deduplicator struct {
	seen  map[string]string // hash -> original key
	mu    sync.RWMutex
	hits  atomic.Uint64
	total atomic.Uint64
}

// NewDeduplicator creates a deduplicator
func NewDeduplicator() *Deduplicator {
	return &Deduplicator{
		seen: make(map[string]string),
	}
}

// CheckDuplicate checks if data is duplicate
func (d *Deduplicator) CheckDuplicate(data []byte) (string, bool) {
	d.total.Add(1)
	
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])
	
	d.mu.RLock()
	original, exists := d.seen[hashStr]
	d.mu.RUnlock()
	
	if exists {
		d.hits.Add(1)
		return original, true
	}
	
	return hashStr, false
}

// Record records a new unique entry
func (d *Deduplicator) Record(hash, key string) {
	d.mu.Lock()
	d.seen[hash] = key
	d.mu.Unlock()
}

// OptimizedStore wraps a store with optimizations
type OptimizedStore struct {
	optimizer *StorageOptimizer
	backend   StorageBackend
}

// StorageBackend interface for underlying storage
type StorageBackend interface {
	Read(key string) ([]byte, error)
	Write(key string, data []byte) error
	Delete(key string) error
}

// NewOptimizedStore creates an optimized store
func NewOptimizedStore(backend StorageBackend, cacheSize int) *OptimizedStore {
	return &OptimizedStore{
		optimizer: NewStorageOptimizer(cacheSize),
		backend:   backend,
	}
}

// Read reads with caching
func (os *OptimizedStore) Read(key string) ([]byte, error) {
	// Check cache first
	if data, hit := os.optimizer.cache.Get(key); hit {
		os.optimizer.stats.CacheHits.Add(1)
		return data, nil
	}
	
	os.optimizer.stats.CacheMisses.Add(1)
	
	// Read from backend
	data, err := os.backend.Read(key)
	if err != nil {
		return nil, err
	}
	
	// Cache for next time
	os.optimizer.cache.Put(key, data)
	
	return data, nil
}

// Write writes with optimization
func (os *OptimizedStore) Write(key string, data []byte) error {
	os.optimizer.stats.TotalWrites.Add(1)
	
	// Check for duplicates
	if hash, isDupe := os.optimizer.deduplicator.CheckDuplicate(data); isDupe {
		os.optimizer.stats.DedupeHits.Add(1)
		// Just reference the existing data
		return nil
	} else {
		os.optimizer.deduplicator.Record(hash, key)
	}
	
	// Compress if beneficial
	compressed, wasCompressed := os.optimizer.compressor.Compress(data)
	if wasCompressed {
		saved := len(data) - len(compressed)
		os.optimizer.stats.BytesSaved.Add(uint64(saved))
		os.optimizer.stats.BytesCompressed.Add(uint64(len(data)))
	}
	
	// Update cache
	os.optimizer.cache.Put(key, data)
	
	// Write to backend
	return os.backend.Write(key, compressed)
}

// BulkOptimizer optimizes bulk operations
type BulkOptimizer struct {
	parallel   int
	chunkSize  int
	limiter    *ConcurrentLimiter
	pool       *MemoryPool
}

// NewBulkOptimizer creates a bulk optimizer
func NewBulkOptimizer(parallel, chunkSize int) *BulkOptimizer {
	return &BulkOptimizer{
		parallel:  parallel,
		chunkSize: chunkSize,
		limiter:   NewConcurrentLimiter(parallel),
		pool:      NewMemoryPool(),
	}
}

// ProcessBulk processes items in optimized bulk operations
func (bo *BulkOptimizer) ProcessBulk(ctx context.Context, items []interface{}, processor func([]interface{}) error) error {
	chunks := make([][]interface{}, 0)
	
	// Split into chunks
	for i := 0; i < len(items); i += bo.chunkSize {
		end := i + bo.chunkSize
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[i:end])
	}
	
	// Process chunks in parallel
	errChan := make(chan error, len(chunks))
	var wg sync.WaitGroup
	
	for _, chunk := range chunks {
		chunk := chunk // Capture
		wg.Add(1)
		
		go func() {
			defer wg.Done()
			err := bo.limiter.Do(ctx, func() error {
				return processor(chunk)
			})
			if err != nil {
				errChan <- err
			}
		}()
	}
	
	wg.Wait()
	close(errChan)
	
	// Check for errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	
	return nil
}

// PackedFormat provides efficient binary packing
type PackedFormat struct {
	Version uint32
	Entries []PackedEntry
}

// PackedEntry is a packed storage entry
type PackedEntry struct {
	Type   uint8
	Flags  uint8
	Size   uint32
	Hash   [32]byte
	Data   []byte
}

// Pack packs entries into binary format
func Pack(entries []PackedEntry) ([]byte, error) {
	buf := new(bytes.Buffer)
	
	// Write header
	binary.Write(buf, binary.LittleEndian, uint32(1)) // Version
	binary.Write(buf, binary.LittleEndian, uint32(len(entries)))
	
	// Write entries
	for _, entry := range entries {
		binary.Write(buf, binary.LittleEndian, entry.Type)
		binary.Write(buf, binary.LittleEndian, entry.Flags)
		binary.Write(buf, binary.LittleEndian, entry.Size)
		buf.Write(entry.Hash[:])
		buf.Write(entry.Data)
	}
	
	return buf.Bytes(), nil
}

// Unpack unpacks binary format
func Unpack(data []byte) (*PackedFormat, error) {
	buf := bytes.NewReader(data)
	pf := &PackedFormat{}
	
	// Read header
	binary.Read(buf, binary.LittleEndian, &pf.Version)
	var count uint32
	binary.Read(buf, binary.LittleEndian, &count)
	
	// Read entries
	pf.Entries = make([]PackedEntry, count)
	for i := range pf.Entries {
		binary.Read(buf, binary.LittleEndian, &pf.Entries[i].Type)
		binary.Read(buf, binary.LittleEndian, &pf.Entries[i].Flags)
		binary.Read(buf, binary.LittleEndian, &pf.Entries[i].Size)
		buf.Read(pf.Entries[i].Hash[:])
		
		pf.Entries[i].Data = make([]byte, pf.Entries[i].Size)
		buf.Read(pf.Entries[i].Data)
	}
	
	return pf, nil
}

// Stats returns optimizer statistics
func (so *StorageOptimizer) Stats() map[string]interface{} {
	return map[string]interface{}{
		"cache_hits":       so.stats.CacheHits.Load(),
		"cache_misses":     so.stats.CacheMisses.Load(),
		"bytes_compressed": so.stats.BytesCompressed.Load(),
		"bytes_saved":      so.stats.BytesSaved.Load(),
		"dedupe_hits":      so.stats.DedupeHits.Load(),
		"batched_writes":   so.stats.BatchedWrites.Load(),
		"total_writes":     so.stats.TotalWrites.Load(),
		"cache_hit_ratio":  float64(so.stats.CacheHits.Load()) / float64(so.stats.CacheHits.Load()+so.stats.CacheMisses.Load()),
		"compression_ratio": float64(so.stats.BytesSaved.Load()) / float64(so.stats.BytesCompressed.Load()),
		"dedupe_ratio":     float64(so.stats.DedupeHits.Load()) / float64(so.stats.TotalWrites.Load()),
	}
}