package builder

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMemoryCache(t *testing.T) {
	cache := NewMemoryCache()
	
	assert.NotNil(t, cache)
	assert.NotNil(t, cache.entries)
	assert.Equal(t, int64(1024*1024*1024), cache.maxSize) // 1GB default
	assert.Equal(t, int64(0), cache.size)
}

func TestCacheGetSet(t *testing.T) {
	cache := NewMemoryCache()
	
	t.Run("Set and Get", func(t *testing.T) {
		key := "test-key"
		data := []byte("test data")
		
		// Set data
		err := cache.Set(key, data)
		require.NoError(t, err)
		
		// Get data
		retrieved, found := cache.Get(key)
		assert.True(t, found)
		assert.Equal(t, data, retrieved)
		
		// Verify cache size increased
		assert.Equal(t, int64(len(data)), cache.size)
	})
	
	t.Run("Get Non-existent Key", func(t *testing.T) {
		data, found := cache.Get("non-existent")
		assert.False(t, found)
		assert.Nil(t, data)
	})
	
	t.Run("Update Existing Key", func(t *testing.T) {
		key := "update-key"
		data1 := []byte("original data")
		data2 := []byte("updated data")
		
		// Set initial data
		err := cache.Set(key, data1)
		require.NoError(t, err)
		initialSize := cache.size
		
		// Update with new data
		err = cache.Set(key, data2)
		require.NoError(t, err)
		
		// Verify data was updated
		retrieved, found := cache.Get(key)
		assert.True(t, found)
		assert.Equal(t, data2, retrieved)
		
		// Verify size was adjusted correctly
		expectedSize := initialSize - int64(len(data1)) + int64(len(data2))
		assert.Equal(t, expectedSize, cache.size)
	})
}

func TestCacheHas(t *testing.T) {
	cache := NewMemoryCache()
	
	key := "exists-key"
	data := []byte("some data")
	
	// Check before setting
	assert.False(t, cache.Has(key))
	
	// Set and check
	err := cache.Set(key, data)
	require.NoError(t, err)
	assert.True(t, cache.Has(key))
}

func TestCacheEviction(t *testing.T) {
	cache := NewMemoryCache()
	cache.SetMaxSize(100) // Small cache for testing
	
	// Add entries that will exceed max size
	entries := []struct {
		key  string
		data []byte
	}{
		{"key1", make([]byte, 30)},
		{"key2", make([]byte, 30)},
		{"key3", make([]byte, 30)},
		{"key4", make([]byte, 30)}, // This should trigger eviction
	}
	
	// Add entries and track which ones get evicted
	for _, entry := range entries {
		err := cache.Set(entry.key, entry.data)
		require.NoError(t, err)
		
		// Small delay to ensure different access times
		time.Sleep(10 * time.Millisecond)
	}
	
	// Cache size should not exceed max
	assert.LessOrEqual(t, cache.size, cache.maxSize)
	
	// At least one of the first entries should be evicted
	evicted := 0
	for i := 0; i < 3; i++ {
		if !cache.Has(entries[i].key) {
			evicted++
		}
	}
	assert.Greater(t, evicted, 0)
	
	// Last entry should still be in cache
	assert.True(t, cache.Has("key4"))
}

func TestCacheLRUEviction(t *testing.T) {
	cache := NewMemoryCache()
	cache.SetMaxSize(100)
	
	// Add three entries
	cache.Set("old", make([]byte, 30))
	time.Sleep(10 * time.Millisecond)
	
	cache.Set("middle", make([]byte, 30))
	time.Sleep(10 * time.Millisecond)
	
	cache.Set("new", make([]byte, 30))
	time.Sleep(10 * time.Millisecond)
	
	// Access "old" to make it recently used
	cache.Get("old")
	
	// Add another entry to trigger eviction
	cache.Set("newest", make([]byte, 40))
	
	// "middle" should be evicted (least recently used)
	assert.False(t, cache.Has("middle"))
	assert.True(t, cache.Has("old"))
	assert.True(t, cache.Has("new"))
	assert.True(t, cache.Has("newest"))
}

func TestCacheHitTracking(t *testing.T) {
	cache := NewMemoryCache()
	
	key := "hit-test"
	data := []byte("test data")
	
	// Set data
	err := cache.Set(key, data)
	require.NoError(t, err)
	
	// Access multiple times
	for i := 0; i < 5; i++ {
		_, found := cache.Get(key)
		assert.True(t, found)
	}
	
	// Check hit count through stats
	stats := cache.Stats()
	assert.Equal(t, 5, stats.HitCount)
	assert.Equal(t, 1, stats.Entries)
}

func TestCacheClear(t *testing.T) {
	cache := NewMemoryCache()
	
	// Add multiple entries
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key%d", i)
		data := []byte(fmt.Sprintf("data%d", i))
		err := cache.Set(key, data)
		require.NoError(t, err)
	}
	
	// Verify entries exist
	assert.Equal(t, 10, len(cache.entries))
	assert.Greater(t, cache.size, int64(0))
	
	// Clear cache
	cache.Clear()
	
	// Verify cache is empty
	assert.Equal(t, 0, len(cache.entries))
	assert.Equal(t, int64(0), cache.size)
	
	// Verify entries are gone
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key%d", i)
		assert.False(t, cache.Has(key))
	}
}

func TestCacheStats(t *testing.T) {
	cache := NewMemoryCache()
	cache.SetMaxSize(1000)
	
	// Add entries and generate hits
	entries := map[string]int{
		"key1": 3,
		"key2": 5,
		"key3": 2,
	}
	
	for key, hits := range entries {
		data := []byte(fmt.Sprintf("data for %s", key))
		err := cache.Set(key, data)
		require.NoError(t, err)
		
		// Generate hits
		for i := 0; i < hits; i++ {
			cache.Get(key)
		}
	}
	
	stats := cache.Stats()
	assert.Equal(t, 3, stats.Entries)
	assert.Equal(t, 10, stats.HitCount) // 3 + 5 + 2
	assert.Greater(t, stats.Size, int64(0))
	assert.Equal(t, int64(1000), stats.MaxSize)
}

func TestCacheConcurrency(t *testing.T) {
	cache := NewMemoryCache()
	cache.SetMaxSize(10000)
	
	// Number of goroutines and operations
	numGoroutines := 50
	opsPerGoroutine := 100
	
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			for j := 0; j < opsPerGoroutine; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				data := []byte(fmt.Sprintf("data-%d-%d", id, j))
				
				// Set
				if err := cache.Set(key, data); err != nil {
					errors <- err
					return
				}
				
				// Get
				retrieved, found := cache.Get(key)
				if !found {
					errors <- fmt.Errorf("key not found: %s", key)
					return
				}
				if string(retrieved) != string(data) {
					errors <- fmt.Errorf("data mismatch for key: %s", key)
					return
				}
				
				// Has
				if !cache.Has(key) {
					errors <- fmt.Errorf("Has returned false for existing key: %s", key)
					return
				}
			}
		}(i)
	}
	
	// Wait for all goroutines
	wg.Wait()
	close(errors)
	
	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}
	
	// Verify cache state
	assert.LessOrEqual(t, cache.size, cache.maxSize)
}

func TestCacheWithZeroSizeData(t *testing.T) {
	cache := NewMemoryCache()
	
	// Set empty data
	key := "empty"
	err := cache.Set(key, []byte{})
	require.NoError(t, err)
	
	// Should still be retrievable
	data, found := cache.Get(key)
	assert.True(t, found)
	assert.Empty(t, data)
	assert.True(t, cache.Has(key))
}

func TestCacheMaxSizeAdjustment(t *testing.T) {
	cache := NewMemoryCache()
	
	// Fill cache with data
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key%d", i)
		data := make([]byte, 100)
		err := cache.Set(key, data)
		require.NoError(t, err)
	}
	
	originalSize := cache.size
	
	// Reduce max size
	cache.SetMaxSize(500)
	
	// Add new data to trigger eviction
	err := cache.Set("trigger", make([]byte, 100))
	require.NoError(t, err)
	
	// Cache size should respect new limit
	assert.LessOrEqual(t, cache.size, cache.maxSize)
	assert.Less(t, cache.size, originalSize)
}

func TestCacheEntryMetadata(t *testing.T) {
	cache := NewMemoryCache()
	
	key := "metadata-test"
	data := []byte("test data")
	
	// Set data
	err := cache.Set(key, data)
	require.NoError(t, err)
	
	// Access entry directly to check metadata
	cache.mu.RLock()
	entry, exists := cache.entries[key]
	cache.mu.RUnlock()
	
	assert.True(t, exists)
	assert.Equal(t, int64(len(data)), entry.size)
	assert.Equal(t, 0, entry.hits) // Not incremented on Set
	assert.NotZero(t, entry.created)
	assert.NotZero(t, entry.lastAccess)
	
	// Get to increment hits
	cache.Get(key)
	
	cache.mu.RLock()
	assert.Equal(t, 1, entry.hits)
	cache.mu.RUnlock()
}

func TestEvictionWithEqualAccessTimes(t *testing.T) {
	cache := NewMemoryCache()
	cache.SetMaxSize(100)
	
	// Mock scenario where all entries have same access time
	now := time.Now()
	
	cache.mu.Lock()
	// Manually create entries with same access time
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("key%d", i)
		cache.entries[key] = &cacheEntry{
			data:       make([]byte, 30),
			size:       30,
			lastAccess: now,
			created:    now,
		}
		cache.size += 30
	}
	cache.mu.Unlock()
	
	// Add new entry to trigger eviction
	err := cache.Set("new", make([]byte, 40))
	require.NoError(t, err)
	
	// Should have evicted one entry
	assert.Equal(t, 3, len(cache.entries))
	assert.LessOrEqual(t, cache.size, cache.maxSize)
}