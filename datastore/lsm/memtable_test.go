package lsm

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemTable_New(t *testing.T) {
	maxSize := int64(64 * 1024 * 1024) // 64MB
	mt := NewMemTable(maxSize)

	assert.NotNil(t, mt)
	assert.Equal(t, maxSize, mt.maxSize)
	assert.Equal(t, int64(0), mt.currentSize)
	assert.Equal(t, 0, mt.Size())
	assert.False(t, mt.IsFull())
	assert.False(t, mt.IsSealed())
	assert.NotNil(t, mt.data)
	assert.True(t, time.Since(mt.createdAt) < time.Second)
}

func TestMemTable_Put(t *testing.T) {
	mt := NewMemTable(1024)

	// Test basic put
	key := "test_key_123"
	value := []byte("test value data")

	err := mt.Put(key, value)
	assert.NoError(t, err)
	assert.Equal(t, 1, mt.Size())
	assert.True(t, mt.MemoryUsage() > 0)

	// Test get
	retrieved, found := mt.Get(key)
	assert.True(t, found)
	assert.Equal(t, value, retrieved)

	// Test has
	assert.True(t, mt.Has(key))

	// Test update existing key
	newValue := []byte("updated test value data")
	err = mt.Put(key, newValue)
	assert.NoError(t, err)
	assert.Equal(t, 1, mt.Size()) // Size should remain 1

	retrieved, found = mt.Get(key)
	assert.True(t, found)
	assert.Equal(t, newValue, retrieved)
}

func TestMemTable_Get(t *testing.T) {
	mt := NewMemTable(1024)

	// Test get non-existent key
	_, found := mt.Get("non_existent")
	assert.False(t, found)

	// Test get after put
	key := "test_key"
	value := []byte("test value")
	
	err := mt.Put(key, value)
	require.NoError(t, err)

	retrieved, found := mt.Get(key)
	assert.True(t, found)
	assert.Equal(t, value, retrieved)

	// Verify returned value is a copy
	retrieved[0] = 'X'
	original, _ := mt.Get(key)
	assert.NotEqual(t, retrieved[0], original[0])
}

func TestMemTable_Delete(t *testing.T) {
	mt := NewMemTable(1024)

	key := "test_key"
	value := []byte("test value")

	// Put a key
	err := mt.Put(key, value)
	require.NoError(t, err)
	assert.True(t, mt.Has(key))

	// Delete the key
	err = mt.Delete(key)
	assert.NoError(t, err)
	
	// Key should no longer be accessible
	assert.False(t, mt.Has(key))
	_, found := mt.Get(key)
	assert.False(t, found)

	// Size should still include tombstone
	assert.Equal(t, 1, mt.Size())
	assert.True(t, mt.MemoryUsage() > 0)

	// Delete non-existent key should still work
	err = mt.Delete("non_existent")
	assert.NoError(t, err)
	assert.Equal(t, 2, mt.Size()) // Tombstone added
}

func TestMemTable_Has(t *testing.T) {
	mt := NewMemTable(1024)

	key := "test_key"
	assert.False(t, mt.Has(key))

	// After put
	err := mt.Put(key, []byte("value"))
	require.NoError(t, err)
	assert.True(t, mt.Has(key))

	// After delete
	err = mt.Delete(key)
	require.NoError(t, err)
	assert.False(t, mt.Has(key))
}

func TestMemTable_Size(t *testing.T) {
	mt := NewMemTable(1024)
	assert.Equal(t, 0, mt.Size())

	// Add entries
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key_%d", i)
		value := []byte(fmt.Sprintf("value_%d", i))
		err := mt.Put(key, value)
		require.NoError(t, err)
		assert.Equal(t, i+1, mt.Size())
	}

	// Delete an entry (size should stay the same due to tombstone)
	err := mt.Delete("key_0")
	require.NoError(t, err)
	assert.Equal(t, 10, mt.Size())

	// Update an entry (size should stay the same)
	err = mt.Put("key_1", []byte("new_value_1"))
	require.NoError(t, err)
	assert.Equal(t, 10, mt.Size())
}

func TestMemTable_MemoryUsage(t *testing.T) {
	mt := NewMemTable(1024)
	assert.Equal(t, int64(0), mt.MemoryUsage())

	key := "test_key"
	value := []byte("test_value")
	
	err := mt.Put(key, value)
	require.NoError(t, err)

	usage := mt.MemoryUsage()
	assert.True(t, usage > 0)
	assert.True(t, usage >= int64(len(key)+len(value)))

	// Add more data
	bigValue := make([]byte, 100)
	err = mt.Put("big_key", bigValue)
	require.NoError(t, err)

	newUsage := mt.MemoryUsage()
	assert.True(t, newUsage > usage)
}

func TestMemTable_IsFull(t *testing.T) {
	smallSize := int64(200) // Small max size
	mt := NewMemTable(smallSize)
	assert.False(t, mt.IsFull())

	// Add data until full
	key := "k1"
	value := make([]byte, 50) // Small value
	
	err := mt.Put(key, value)
	require.NoError(t, err)
	
	// Should not be full yet (key + value + 64 overhead = ~120 bytes)
	assert.False(t, mt.IsFull())

	// Add more data to exceed limit
	key2 := "k2"
	value2 := make([]byte, 50)
	
	err = mt.Put(key2, value2)
	require.NoError(t, err)

	// Should be full now (total ~240 bytes > 200)
	assert.True(t, mt.IsFull())
}

func TestMemTable_Seal(t *testing.T) {
	mt := NewMemTable(1024)
	assert.False(t, mt.IsSealed())

	// Seal the memtable
	mt.Seal()
	assert.True(t, mt.IsSealed())

	// Should not be able to write to sealed memtable
	err := mt.Put("key", []byte("value"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sealed")

	err = mt.Delete("key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sealed")
}

func TestMemTable_Iterator(t *testing.T) {
	mt := NewMemTable(1024)

	// Add test data
	testData := map[string][]byte{
		"key_c": []byte("value_c"),
		"key_a": []byte("value_a"),
		"key_b": []byte("value_b"),
		"key_d": []byte("value_d"),
	}

	for key, value := range testData {
		err := mt.Put(key, value)
		require.NoError(t, err)
	}

	// Test iterator
	iter := mt.NewIterator()
	assert.NotNil(t, iter)

	// Collect all entries
	var entries []*Entry
	for iter.Valid() {
		entries = append(entries, iter.Entry())
		iter.Next()
	}

	// Should have all entries
	assert.Len(t, entries, len(testData))

	// Should be sorted by key
	for i := 1; i < len(entries); i++ {
		assert.True(t, entries[i-1].Key < entries[i].Key)
	}

	// Verify content
	for _, entry := range entries {
		expected := testData[entry.Key]
		assert.Equal(t, expected, entry.Value)
	}
}

func TestMemTable_IteratorSeek(t *testing.T) {
	mt := NewMemTable(1024)

	// Add sorted test data
	keys := []string{"key_10", "key_20", "key_30", "key_40", "key_50"}
	for _, key := range keys {
		err := mt.Put(key, []byte("value_"+key))
		require.NoError(t, err)
	}

	iter := mt.NewIterator()

	// Test SeekToFirst
	iter.SeekToFirst()
	assert.True(t, iter.Valid())
	assert.Equal(t, "key_10", iter.Key())

	// Test SeekToLast
	iter.SeekToLast()
	assert.True(t, iter.Valid())
	assert.Equal(t, "key_50", iter.Key())

	// Test Seek to specific key
	iter.Seek("key_25")
	assert.True(t, iter.Valid())
	assert.Equal(t, "key_30", iter.Key()) // Should find first key >= "key_25"

	// Test seek to exact key
	iter.Seek("key_30")
	assert.True(t, iter.Valid())
	assert.Equal(t, "key_30", iter.Key())

	// Test seek beyond last key
	iter.Seek("key_99")
	assert.False(t, iter.Valid())
}

func TestMemTable_IteratorMethods(t *testing.T) {
	mt := NewMemTable(1024)

	// Empty iterator
	iter := mt.NewIterator()
	assert.False(t, iter.Valid())
	assert.Equal(t, "", iter.Key())
	assert.Nil(t, iter.Value())
	assert.Nil(t, iter.Entry())

	// Add data
	key := "test_key"
	value := []byte("test_value")
	err := mt.Put(key, value)
	require.NoError(t, err)

	// Test with data
	iter = mt.NewIterator()
	assert.True(t, iter.Valid())
	assert.Equal(t, key, iter.Key())
	assert.Equal(t, value, iter.Value())
	
	entry := iter.Entry()
	assert.NotNil(t, entry)
	assert.Equal(t, key, entry.Key)
	assert.Equal(t, value, entry.Value)
	assert.False(t, entry.Deleted)

	// Move past end
	iter.Next()
	assert.False(t, iter.Valid())
}

func TestMemTable_Range(t *testing.T) {
	mt := NewMemTable(1024)

	// Add test data with various prefixes
	testData := map[string][]byte{
		"blob_001": []byte("blob data 1"),
		"blob_002": []byte("blob data 2"),
		"tree_001": []byte("tree data 1"),
		"tree_002": []byte("tree data 2"),
		"commit_001": []byte("commit data 1"),
	}

	for key, value := range testData {
		err := mt.Put(key, value)
		require.NoError(t, err)
	}

	// Test range iteration - all keys
	var allKeys []string
	mt.Range("", "", func(key string, value []byte, deleted bool) bool {
		if !deleted {
			allKeys = append(allKeys, key)
		}
		return true
	})

	assert.Len(t, allKeys, len(testData))
	sort.Strings(allKeys) // Range should return in sorted order

	// Test range iteration with prefix (blob_)
	var blobKeys []string
	mt.Range("blob_", "blob~", func(key string, value []byte, deleted bool) bool {
		if !deleted && strings.HasPrefix(key, "blob_") {
			blobKeys = append(blobKeys, key)
		}
		return true
	})

	assert.Len(t, blobKeys, 2)

	// Test early termination
	var limitedKeys []string
	mt.Range("", "", func(key string, value []byte, deleted bool) bool {
		if !deleted {
			limitedKeys = append(limitedKeys, key)
		}
		return len(limitedKeys) < 2 // Stop after 2 keys
	})

	assert.Len(t, limitedKeys, 2)

	// Test with deleted entries
	err := mt.Delete("blob_001")
	require.NoError(t, err)

	var activeKeys []string
	var deletedKeys []string
	mt.Range("", "", func(key string, value []byte, deleted bool) bool {
		if deleted {
			deletedKeys = append(deletedKeys, key)
		} else {
			activeKeys = append(activeKeys, key)
		}
		return true
	})

	assert.Contains(t, deletedKeys, "blob_001")
	assert.Len(t, activeKeys, len(testData)-1)
}

func TestMemTable_Stats(t *testing.T) {
	mt := NewMemTable(1024)

	// Empty stats
	stats := mt.Stats()
	assert.Equal(t, 0, stats.NumEntries)
	assert.Equal(t, int64(0), stats.MemoryUsage)
	assert.Equal(t, int64(1024), stats.MaxSize)
	assert.False(t, stats.Sealed)
	assert.Equal(t, 0.0, stats.AverageKeySize)
	assert.Equal(t, 0.0, stats.AverageValueSize)

	// Add some data
	testData := map[string][]byte{
		"short": []byte("val"),
		"medium_key": []byte("medium_value"),
		"longer_key_name": []byte("much_longer_value_content"),
	}

	for key, value := range testData {
		err := mt.Put(key, value)
		require.NoError(t, err)
	}

	// Stats with data
	stats = mt.Stats()
	assert.Equal(t, len(testData), stats.NumEntries)
	assert.True(t, stats.MemoryUsage > 0)
	assert.True(t, stats.AverageKeySize > 0)
	assert.True(t, stats.AverageValueSize > 0)
	assert.False(t, stats.Sealed)

	// Seal and check stats
	mt.Seal()
	stats = mt.Stats()
	assert.True(t, stats.Sealed)

	// Delete one entry and check stats
	err := mt.Delete("short") // This should work because delete happens before seal
	// Actually, it won't work because we sealed it. Let's test before sealing:

	mt2 := NewMemTable(1024)
	for key, value := range testData {
		err := mt2.Put(key, value)
		require.NoError(t, err)
	}

	err = mt2.Delete("short")
	require.NoError(t, err)

	stats = mt2.Stats()
	assert.Equal(t, len(testData), stats.NumEntries) // Tombstone still counts
	// Average value size should be different due to deleted entry
}

func TestMemTable_Flush(t *testing.T) {
	mt := NewMemTable(1024)

	// Add data
	err := mt.Put("key1", []byte("value1"))
	require.NoError(t, err)
	err = mt.Put("key2", []byte("value2"))
	require.NoError(t, err)

	// Cannot flush unsealed memtable
	_, err = mt.Flush()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsealed")

	// Seal and flush
	mt.Seal()
	iter, err := mt.Flush()
	assert.NoError(t, err)
	assert.NotNil(t, iter)

	// Verify iterator works
	count := 0
	for iter.Valid() {
		count++
		iter.Next()
	}
	assert.Equal(t, 2, count)
}

func TestMemTable_MergeMemTables(t *testing.T) {
	// Create multiple memtables
	mt1 := NewMemTable(512)
	mt2 := NewMemTable(512)
	mt3 := NewMemTable(512)

	// Add data with some overlaps
	err := mt1.Put("key1", []byte("value1_mt1"))
	require.NoError(t, err)
	err = mt1.Put("key2", []byte("value2_mt1"))
	require.NoError(t, err)

	// Wait a bit to ensure different timestamps
	time.Sleep(time.Millisecond)

	err = mt2.Put("key2", []byte("value2_mt2")) // Overwrite key2
	require.NoError(t, err)
	err = mt2.Put("key3", []byte("value3_mt2"))
	require.NoError(t, err)

	time.Sleep(time.Millisecond)

	err = mt3.Put("key4", []byte("value4_mt3"))
	require.NoError(t, err)

	// Merge tables
	merged, err := MergeMemTables([]*MemTable{mt1, mt2, mt3})
	assert.NoError(t, err)
	assert.NotNil(t, merged)
	assert.True(t, merged.IsSealed())

	// Verify merged content
	value1, found := merged.Get("key1")
	assert.True(t, found)
	assert.Equal(t, []byte("value1_mt1"), value1)

	// key2 should have the newer value from mt2
	value2, found := merged.Get("key2")
	assert.True(t, found)
	assert.Equal(t, []byte("value2_mt2"), value2)

	value3, found := merged.Get("key3")
	assert.True(t, found)
	assert.Equal(t, []byte("value3_mt2"), value3)

	value4, found := merged.Get("key4")
	assert.True(t, found)
	assert.Equal(t, []byte("value4_mt3"), value4)

	// Test with empty slice
	_, err = MergeMemTables([]*MemTable{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no memtables")

	// Test with single memtable
	single, err := MergeMemTables([]*MemTable{mt1})
	assert.NoError(t, err)
	assert.NotNil(t, single)
}

func TestMemTable_Concurrent(t *testing.T) {
	mt := NewMemTable(10 * 1024) // 10KB

	numGoroutines := 10
	numOpsPerGoroutine := 100
	done := make(chan bool, numGoroutines)

	// Concurrent writes and reads
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < numOpsPerGoroutine; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				value := []byte(fmt.Sprintf("value_%d_%d", id, j))

				// Write
				err := mt.Put(key, value)
				if err != nil {
					// Might get error if memtable becomes sealed or full
					continue
				}

				// Read
				retrieved, found := mt.Get(key)
				if found {
					assert.Equal(t, value, retrieved)
				}

				// Check existence
				exists := mt.Has(key)
				assert.Equal(t, found, exists)
			}
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify final state
	assert.True(t, mt.Size() > 0)
	assert.True(t, mt.MemoryUsage() > 0)
}

func TestMemTable_LargeData(t *testing.T) {
	mt := NewMemTable(10 * 1024 * 1024) // 10MB

	// Test with large values
	largeValue := make([]byte, 100*1024) // 100KB
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	key := "large_data_key"
	err := mt.Put(key, largeValue)
	assert.NoError(t, err)

	retrieved, found := mt.Get(key)
	assert.True(t, found)
	assert.Equal(t, largeValue, retrieved)

	// Verify memory usage
	usage := mt.MemoryUsage()
	assert.True(t, usage >= int64(len(largeValue)))

	// Test iterator with large data
	iter := mt.NewIterator()
	assert.True(t, iter.Valid())
	assert.Equal(t, key, iter.Key())
	assert.Equal(t, largeValue, iter.Value())
}

func TestMemTable_DeletedEntries(t *testing.T) {
	mt := NewMemTable(1024)

	// Add and delete entries
	keys := []string{"key1", "key2", "key3"}
	for _, key := range keys {
		err := mt.Put(key, []byte("value_"+key))
		require.NoError(t, err)
	}

	// Delete some keys
	err := mt.Delete("key2")
	require.NoError(t, err)

	// Test iterator includes tombstones
	iter := mt.NewIterator()
	count := 0
	deletedCount := 0
	for iter.Valid() {
		entry := iter.Entry()
		if entry.Deleted {
			deletedCount++
		}
		count++
		iter.Next()
	}

	assert.Equal(t, 3, count)
	assert.Equal(t, 1, deletedCount)

	// Test range iteration with deleted entries
	activeCount := 0
	tombstoneCount := 0
	mt.Range("", "", func(key string, value []byte, deleted bool) bool {
		if deleted {
			tombstoneCount++
		} else {
			activeCount++
		}
		return true
	})

	assert.Equal(t, 2, activeCount)
	assert.Equal(t, 1, tombstoneCount)
}

// Benchmark tests
func BenchmarkMemTable_Put(b *testing.B) {
	mt := NewMemTable(100 * 1024 * 1024) // 100MB
	value := make([]byte, 1024) // 1KB value

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench_key_%d", i)
			mt.Put(key, value)
			i++
		}
	})
}

func BenchmarkMemTable_Get(b *testing.B) {
	mt := NewMemTable(100 * 1024 * 1024) // 100MB
	value := make([]byte, 1024) // 1KB value

	// Populate with data
	numKeys := 10000
	keys := make([]string, numKeys)
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("bench_key_%d", i)
		keys[i] = key
		mt.Put(key, value)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := keys[i%numKeys]
			mt.Get(key)
			i++
		}
	})
}

func BenchmarkMemTable_Iterator(b *testing.B) {
	mt := NewMemTable(100 * 1024 * 1024) // 100MB
	value := make([]byte, 100) // 100B value

	// Populate with data
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("bench_key_%04d", i)
		mt.Put(key, value)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		iter := mt.NewIterator()
		for iter.Valid() {
			iter.Key()
			iter.Value()
			iter.Next()
		}
	}
}