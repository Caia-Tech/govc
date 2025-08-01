package builder

import (
	"sync"
	"time"
)

// MemoryCache implements an in-memory build cache
type MemoryCache struct {
	entries map[string]*cacheEntry
	maxSize int64
	size    int64
	mu      sync.RWMutex
}

type cacheEntry struct {
	data      []byte
	size      int64
	hits      int
	lastAccess time.Time
	created    time.Time
}

// NewMemoryCache creates a new memory-based build cache
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{
		entries: make(map[string]*cacheEntry),
		maxSize: 1024 * 1024 * 1024, // 1GB default
	}
}

// Get retrieves data from the cache
func (c *MemoryCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		return nil, false
	}

	// Update access stats
	c.mu.Lock()
	entry.hits++
	entry.lastAccess = time.Now()
	c.mu.Unlock()

	return entry.data, true
}

// Set stores data in the cache
func (c *MemoryCache) Set(key string, data []byte) error {
	size := int64(len(data))

	c.mu.Lock()
	defer c.mu.Unlock()

	// If key already exists, subtract its old size
	if existing, exists := c.entries[key]; exists {
		c.size -= existing.size
	}

	// Check if we need to evict entries
	for c.size+size > c.maxSize && len(c.entries) > 0 {
		c.evictLRU()
	}

	// Store the new entry
	c.entries[key] = &cacheEntry{
		data:       data,
		size:       size,
		hits:       0,
		lastAccess: time.Now(),
		created:    time.Now(),
	}
	c.size += size

	return nil
}

// Has checks if a key exists in the cache
func (c *MemoryCache) Has(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.entries[key]
	return exists
}

// evictLRU removes the least recently used entry
func (c *MemoryCache) evictLRU() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.lastAccess.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastAccess
		}
	}

	if oldestKey != "" {
		entry := c.entries[oldestKey]
		c.size -= entry.size
		delete(c.entries, oldestKey)
	}
}

// SetMaxSize sets the maximum cache size
func (c *MemoryCache) SetMaxSize(size int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxSize = size
}

// Clear removes all entries from the cache
func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
	c.size = 0
}

// Stats returns cache statistics
func (c *MemoryCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalHits int
	for _, entry := range c.entries {
		totalHits += entry.hits
	}

	return CacheStats{
		Entries:  len(c.entries),
		Size:     c.size,
		MaxSize:  c.maxSize,
		HitCount: totalHits,
	}
}

// CacheStats contains cache statistics
type CacheStats struct {
	Entries  int
	Size     int64
	MaxSize  int64
	HitCount int
}