package lsm

import (
	"container/list"
	"sync"
)

// LRUCache implements a simple LRU cache for block caching
type LRUCache struct {
	capacity int64
	size     int64
	items    map[string]*list.Element
	evictList *list.List
	mu       sync.RWMutex
}

// CacheItem represents an item in the cache
type CacheItem struct {
	key   string
	value []byte
	size  int64
}

// NewLRUCache creates a new LRU cache
func NewLRUCache(capacity int64) *LRUCache {
	return &LRUCache{
		capacity:  capacity,
		size:      0,
		items:     make(map[string]*list.Element),
		evictList: list.New(),
	}
}

// Get retrieves an item from the cache
func (c *LRUCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if elem, exists := c.items[key]; exists {
		c.evictList.MoveToFront(elem)
		item := elem.Value.(*CacheItem)
		
		// Return a copy
		result := make([]byte, len(item.value))
		copy(result, item.value)
		return result, true
	}
	
	return nil, false
}

// Put adds an item to the cache
func (c *LRUCache) Put(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	itemSize := int64(len(key) + len(value))
	
	// Check if item already exists
	if elem, exists := c.items[key]; exists {
		c.evictList.MoveToFront(elem)
		item := elem.Value.(*CacheItem)
		c.size -= item.size
		
		// Update item
		item.value = make([]byte, len(value))
		copy(item.value, value)
		item.size = itemSize
		c.size += itemSize
		
		c.evictOldest()
		return
	}
	
	// Add new item
	item := &CacheItem{
		key:   key,
		value: make([]byte, len(value)),
		size:  itemSize,
	}
	copy(item.value, value)
	
	elem := c.evictList.PushFront(item)
	c.items[key] = elem
	c.size += itemSize
	
	c.evictOldest()
}

// evictOldest removes oldest items if over capacity
func (c *LRUCache) evictOldest() {
	for c.size > c.capacity {
		elem := c.evictList.Back()
		if elem == nil {
			break
		}
		
		c.evictList.Remove(elem)
		item := elem.Value.(*CacheItem)
		delete(c.items, item.key)
		c.size -= item.size
	}
}

// Clear clears all items from the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.items = make(map[string]*list.Element)
	c.evictList.Init()
	c.size = 0
}

// Size returns the current size of the cache
func (c *LRUCache) Size() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.size
}

// Len returns the number of items in the cache
func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}