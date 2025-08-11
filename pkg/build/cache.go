package build

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"sync"
	"time"
)

// BuildCache manages caching of build results
type BuildCache interface {
	// Get retrieves cached build result
	Get(key CacheKey) (*BuildResult, bool)

	// Put stores build result in cache
	Put(key CacheKey, result *BuildResult) error

	// Invalidate removes cached result
	Invalidate(key CacheKey) error

	// Clear removes all cached results
	Clear() error

	// Stats returns cache statistics
	Stats() CacheStats
}

// CacheKey identifies a unique build configuration
type CacheKey struct {
	Plugin       string      `json:"plugin"`
	Config       BuildConfig `json:"config"`
	FilesHash    string      `json:"files_hash"`
	Dependencies []string    `json:"dependencies,omitempty"`
}

// String returns string representation of cache key
func (k CacheKey) String() string {
	data, _ := json.Marshal(k)
	hasher := sha256.New()
	hasher.Write(data)
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

// CacheStats contains cache statistics
type CacheStats struct {
	Hits        int64     `json:"hits"`
	Misses      int64     `json:"misses"`
	Entries     int       `json:"entries"`
	Size        int64     `json:"size"`
	HitRate     float64   `json:"hit_rate"`
	LastCleanup time.Time `json:"last_cleanup"`
}

// MemoryCache implements BuildCache in memory
type MemoryCache struct {
	cache       map[string]*CacheEntry
	maxEntries  int
	maxSize     int64
	ttl         time.Duration
	hits        int64
	misses      int64
	currentSize int64
	mu          sync.RWMutex
}

// CacheEntry represents a cached build result
type CacheEntry struct {
	Key       CacheKey
	Result    *BuildResult
	Size      int64
	CreatedAt time.Time
	AccessedAt time.Time
	HitCount  int
}

// NewMemoryCache creates a new in-memory build cache
func NewMemoryCache() *MemoryCache {
	return NewMemoryCacheWithOptions(1000, 1<<30, 1*time.Hour) // 1000 entries, 1GB, 1 hour TTL
}

// NewMemoryCacheWithOptions creates cache with custom options
func NewMemoryCacheWithOptions(maxEntries int, maxSize int64, ttl time.Duration) *MemoryCache {
	cache := &MemoryCache{
		cache:      make(map[string]*CacheEntry),
		maxEntries: maxEntries,
		maxSize:    maxSize,
		ttl:        ttl,
	}

	// Start cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Get retrieves cached build result
func (c *MemoryCache) Get(key CacheKey) (*BuildResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keyStr := key.String()
	entry, exists := c.cache[keyStr]
	if !exists {
		c.misses++
		return nil, false
	}

	// Check if entry is expired
	if time.Since(entry.CreatedAt) > c.ttl {
		// Entry expired
		c.misses++
		return nil, false
	}

	// Update access time and hit count
	entry.AccessedAt = time.Now()
	entry.HitCount++
	c.hits++

	// Return a copy to prevent mutations
	resultCopy := c.copyResult(entry.Result)
	return resultCopy, true
}

// Put stores build result in cache
func (c *MemoryCache) Put(key CacheKey, result *BuildResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Calculate entry size
	size := c.calculateResultSize(result)

	// Check if we need to evict entries
	if len(c.cache) >= c.maxEntries || c.currentSize+size > c.maxSize {
		c.evictLRU()
	}

	keyStr := key.String()
	entry := &CacheEntry{
		Key:        key,
		Result:     c.copyResult(result),
		Size:       size,
		CreatedAt:  time.Now(),
		AccessedAt: time.Now(),
		HitCount:   0,
	}

	// Remove old entry if exists
	if old, exists := c.cache[keyStr]; exists {
		c.currentSize -= old.Size
	}

	c.cache[keyStr] = entry
	c.currentSize += size

	return nil
}

// Invalidate removes cached result
func (c *MemoryCache) Invalidate(key CacheKey) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	keyStr := key.String()
	if entry, exists := c.cache[keyStr]; exists {
		c.currentSize -= entry.Size
		delete(c.cache, keyStr)
	}

	return nil
}

// Clear removes all cached results
func (c *MemoryCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*CacheEntry)
	c.currentSize = 0
	c.hits = 0
	c.misses = 0

	return nil
}

// Stats returns cache statistics
func (c *MemoryCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Hits:    c.hits,
		Misses:  c.misses,
		Entries: len(c.cache),
		Size:    c.currentSize,
		HitRate: hitRate,
	}
}

// evictLRU removes least recently used entries
func (c *MemoryCache) evictLRU() {
	// Find LRU entry
	var lruKey string
	var lruTime time.Time

	for key, entry := range c.cache {
		if lruTime.IsZero() || entry.AccessedAt.Before(lruTime) {
			lruKey = key
			lruTime = entry.AccessedAt
		}
	}

	// Remove LRU entry
	if lruKey != "" {
		if entry, exists := c.cache[lruKey]; exists {
			c.currentSize -= entry.Size
			delete(c.cache, lruKey)
		}
	}
}

// cleanupLoop periodically removes expired entries
func (c *MemoryCache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes expired entries
func (c *MemoryCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.cache {
		if now.Sub(entry.CreatedAt) > c.ttl {
			c.currentSize -= entry.Size
			delete(c.cache, key)
		}
	}
}

// calculateResultSize estimates the size of a build result
func (c *MemoryCache) calculateResultSize(result *BuildResult) int64 {
	size := int64(0)

	// Calculate artifacts size
	for _, artifact := range result.Artifacts {
		size += int64(len(artifact.Content))
		size += int64(len(artifact.Name))
		size += artifact.Size
	}

	// Estimate metadata size
	if result.Metadata != nil {
		data, _ := json.Marshal(result.Metadata)
		size += int64(len(data))
	}

	// Estimate output size
	if result.Output != nil {
		size += int64(len(result.Output.Stdout))
		size += int64(len(result.Output.Stderr))
	}

	return size
}

// copyResult creates a deep copy of build result
func (c *MemoryCache) copyResult(result *BuildResult) *BuildResult {
	if result == nil {
		return nil
	}

	copy := &BuildResult{
		Success:  result.Success,
		Duration: result.Duration,
		CacheHit: result.CacheHit,
		Error:    result.Error,
	}

	// Copy artifacts
	if result.Artifacts != nil {
		copy.Artifacts = make([]BuildArtifact, len(result.Artifacts))
		for i, artifact := range result.Artifacts {
			artifactCopy := artifact
			if artifact.Content != nil {
				artifactCopy.Content = make([]byte, len(artifact.Content))
				// Use builtin copy function - avoid variable name conflict
				for i := range artifact.Content {
					artifactCopy.Content[i] = artifact.Content[i]
				}
			}
			copy.Artifacts[i] = artifactCopy
		}
	}

	// Copy metadata
	if result.Metadata != nil {
		data, _ := json.Marshal(result.Metadata)
		var metadata BuildMetadata
		json.Unmarshal(data, &metadata)
		copy.Metadata = &metadata
	}

	// Copy output
	if result.Output != nil {
		copy.Output = &BuildOutput{
			Stdout:   result.Output.Stdout,
			Stderr:   result.Output.Stderr,
			Warnings: append([]string{}, result.Output.Warnings...),
			Errors:   append([]string{}, result.Output.Errors...),
			Info:     append([]string{}, result.Output.Info...),
		}
	}

	// Copy memory stats
	if result.MemoryStats != nil {
		copy.MemoryStats = &MemoryStats{
			PeakUsage:    result.MemoryStats.PeakUsage,
			AverageUsage: result.MemoryStats.AverageUsage,
			FinalUsage:   result.MemoryStats.FinalUsage,
			Allocations:  result.MemoryStats.Allocations,
			Frees:        result.MemoryStats.Frees,
		}
	}

	return copy
}

// DistributedCache implements distributed caching
type DistributedCache struct {
	local  *MemoryCache
	remote RemoteCacheClient
	mu     sync.RWMutex
}

// RemoteCacheClient interface for remote cache backends
type RemoteCacheClient interface {
	Get(key string) ([]byte, error)
	Put(key string, value []byte, ttl time.Duration) error
	Delete(key string) error
}

// NewDistributedCache creates a distributed cache
func NewDistributedCache(remote RemoteCacheClient) *DistributedCache {
	return &DistributedCache{
		local:  NewMemoryCache(),
		remote: remote,
	}
}

// Get retrieves from local cache first, then remote
func (d *DistributedCache) Get(key CacheKey) (*BuildResult, bool) {
	// Check local cache first
	if result, hit := d.local.Get(key); hit {
		return result, true
	}

	// Check remote cache
	keyStr := key.String()
	data, err := d.remote.Get(keyStr)
	if err != nil || data == nil {
		return nil, false
	}

	// Deserialize result
	var result BuildResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false
	}

	// Store in local cache
	d.local.Put(key, &result)

	return &result, true
}

// Put stores in both local and remote cache
func (d *DistributedCache) Put(key CacheKey, result *BuildResult) error {
	// Store in local cache
	if err := d.local.Put(key, result); err != nil {
		return err
	}

	// Serialize and store in remote cache
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	keyStr := key.String()
	return d.remote.Put(keyStr, data, 1*time.Hour)
}

// FileHasher calculates hash of multiple files
type FileHasher struct {
	hasher hash.Hash
	files  []string
}

// NewFileHasher creates a new file hasher
func NewFileHasher() *FileHasher {
	return &FileHasher{
		hasher: sha256.New(),
		files:  make([]string, 0),
	}
}

// Add adds a file to the hash
func (h *FileHasher) Add(filename string, content []byte) {
	h.hasher.Write([]byte(filename))
	h.hasher.Write(content)
	h.files = append(h.files, filename)
}

// Hash returns the computed hash
func (h *FileHasher) Hash() string {
	return fmt.Sprintf("%x", h.hasher.Sum(nil))
}

// Files returns the list of files hashed
func (h *FileHasher) Files() []string {
	return h.files
}

// CacheKeyBuilder helps build cache keys
type CacheKeyBuilder struct {
	plugin   string
	config   BuildConfig
	files    map[string]string
	deps     []string
}

// NewCacheKeyBuilder creates a new cache key builder
func NewCacheKeyBuilder(plugin string) *CacheKeyBuilder {
	return &CacheKeyBuilder{
		plugin: plugin,
		files:  make(map[string]string),
		deps:   make([]string, 0),
	}
}

// WithConfig sets the build config
func (b *CacheKeyBuilder) WithConfig(config BuildConfig) *CacheKeyBuilder {
	b.config = config
	return b
}

// WithFile adds a file to the key
func (b *CacheKeyBuilder) WithFile(path string, content []byte) *CacheKeyBuilder {
	hasher := sha256.New()
	hasher.Write(content)
	b.files[path] = fmt.Sprintf("%x", hasher.Sum(nil))
	return b
}

// WithDependency adds a dependency
func (b *CacheKeyBuilder) WithDependency(dep string) *CacheKeyBuilder {
	b.deps = append(b.deps, dep)
	return b
}

// Build creates the cache key
func (b *CacheKeyBuilder) Build() CacheKey {
	// Calculate files hash
	filesData, _ := json.Marshal(b.files)
	hasher := sha256.New()
	hasher.Write(filesData)
	filesHash := fmt.Sprintf("%x", hasher.Sum(nil))

	return CacheKey{
		Plugin:       b.plugin,
		Config:       b.config,
		FilesHash:    filesHash,
		Dependencies: b.deps,
	}
}