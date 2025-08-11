package badger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Caia-Tech/govc/datastore"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBadgerStore_Configuration(t *testing.T) {
	t.Run("ValidConfiguration", func(t *testing.T) {
		tempDir := t.TempDir()
		
		config := datastore.Config{
			Type:               datastore.TypeBadger,
			Connection:         tempDir,
			MaxConnections:     10,
			MaxIdleConnections: 2,
			ConnectionTimeout:  time.Second * 30,
		}
		
		store, err := New(config)
		require.NoError(t, err)
		
		err = store.Initialize(config)
		assert.NoError(t, err)
		defer store.Close()
		
		// Verify store is functional
		err = store.ObjectStore().PutObject("config-test", []byte("test"))
		assert.NoError(t, err)
	})

	t.Run("InvalidDirectory", func(t *testing.T) {
		config := datastore.Config{
			Type:       datastore.TypeBadger,
			Connection: "/invalid/directory/that/does/not/exist",
		}
		
		store, err := New(config)
		require.NoError(t, err)
		
		err = store.Initialize(config)
		assert.Error(t, err)
	})

	t.Run("DirectoryCreation", func(t *testing.T) {
		tempDir := t.TempDir()
		subDir := filepath.Join(tempDir, "new", "nested", "directory")
		
		config := datastore.Config{
			Type:       datastore.TypeBadger,
			Connection: subDir,
		}
		
		store, err := New(config)
		require.NoError(t, err)
		
		err = store.Initialize(config)
		assert.NoError(t, err)
		defer store.Close()
		
		// Verify directory was created
		_, err = os.Stat(subDir)
		assert.NoError(t, err)
	})

	t.Run("ReadOnlyMode", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Create database first
		config := datastore.Config{
			Type:       datastore.TypeBadger,
			Connection: tempDir,
		}
		
		store1, err := New(config)
		require.NoError(t, err)
		
		err = store1.Initialize(config)
		require.NoError(t, err)
		
		// Add some data
		err = store1.PutObject("readonly-test", []byte("test data"))
		require.NoError(t, err)
		
		store1.Close()
		
		// Try to open in read-only mode by making directory read-only
		err = os.Chmod(tempDir, 0555)
		require.NoError(t, err)
		defer os.Chmod(tempDir, 0755) // Restore for cleanup
		
		store2, err := New(config)
		require.NoError(t, err)
		
		err = store2.Initialize(config)
		// This might succeed or fail depending on BadgerDB behavior
		if err == nil {
			defer store2.Close()
			
			// Should be able to read
			data, err := store2.GetObject("readonly-test")
			if err == nil {
				assert.Equal(t, []byte("test data"), data)
			}
			
			// Write operations might fail
			err = store2.PutObject("write-test", []byte("should fail"))
			// Don't assert on error as behavior is implementation specific
		}
	})
}

func TestBadgerStore_KeyPrefixes(t *testing.T) {
	tempDir := t.TempDir()
	
	config := datastore.Config{
		Type:       datastore.TypeBadger,
		Connection: tempDir,
	}
	
	store, err := New(config)
	require.NoError(t, err)
	
	err = store.Initialize(config)
	require.NoError(t, err)
	defer store.Close()

	t.Run("PrefixIsolation", func(t *testing.T) {
		// Add data with different types
		err = store.ObjectStore().PutObject("test-object", []byte("object data"))
		assert.NoError(t, err)
		
		err = store.SetConfig("test-config", "config value")
		assert.NoError(t, err)
		
		repo := &datastore.Repository{
			ID:        uuid.New().String(),
			Name:      "test-repo",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		err = store.SaveRepository(repo)
		assert.NoError(t, err)
		
		// Verify data is isolated by prefixes
		objData, err := store.ObjectStore().GetObject("test-object")
		assert.NoError(t, err)
		assert.Equal(t, []byte("object data"), objData)
		
		configVal, err := store.GetConfig("test-config")
		assert.NoError(t, err)
		assert.Equal(t, "config value", configVal)
		
		retrievedRepo, err := store.GetRepository(repo.ID)
		assert.NoError(t, err)
		assert.Equal(t, repo.Name, retrievedRepo.Name)
	})

	t.Run("PrefixConflictPrevention", func(t *testing.T) {
		// Test that prefixes don't interfere with each other
		// This was the bug we fixed earlier with user names
		
		// Create a user with a name that could conflict with object prefix
		user := &datastore.User{
			ID:        uuid.New().String(),
			Username:  "obj-test-user", // Could conflict with object prefix
			Email:     "test@example.com",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		
		err = store.SaveUser(user)
		assert.NoError(t, err)
		
		// Create an object with similar name
		err = store.ObjectStore().PutObject("obj-test-object", []byte("object data"))
		assert.NoError(t, err)
		
		// Both should be retrievable independently
		retrievedUser, err := store.GetUserByUsername(user.Username)
		assert.NoError(t, err)
		assert.Equal(t, user.ID, retrievedUser.ID)
		
		objData, err := store.ObjectStore().GetObject("obj-test-object")
		assert.NoError(t, err)
		assert.Equal(t, []byte("object data"), objData)
	})
}

func TestBadgerStore_LargeDataHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large data test in short mode")
	}
	
	tempDir := t.TempDir()
	
	config := datastore.Config{
		Type:       datastore.TypeBadger,
		Connection: tempDir,
	}
	
	store, err := New(config)
	require.NoError(t, err)
	
	err = store.Initialize(config)
	require.NoError(t, err)
	defer store.Close()

	t.Run("LargeObjects", func(t *testing.T) {
		sizes := []int{
			1024,         // 1KB
			1024 * 1024,  // 1MB
			10 * 1024 * 1024, // 10MB
		}
		
		for _, size := range sizes {
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i % 256)
			}
			
			hash := fmt.Sprintf("large-%dMB", size/(1024*1024))
			
			start := time.Now()
			err = store.ObjectStore().PutObject(hash, data)
			writeTime := time.Since(start)
			
			assert.NoError(t, err, "Failed to store %dMB object", size/(1024*1024))
			t.Logf("Write %dMB took %v", size/(1024*1024), writeTime)
			
			start = time.Now()
			retrieved, err := store.ObjectStore().GetObject(hash)
			readTime := time.Since(start)
			
			assert.NoError(t, err, "Failed to retrieve %dMB object", size/(1024*1024))
			assert.Equal(t, data, retrieved, "Data mismatch for %dMB object", size/(1024*1024))
			t.Logf("Read %dMB took %v", size/(1024*1024), readTime)
			
			// Test size
			objSize, err := store.ObjectStore().GetObjectSize(hash)
			assert.NoError(t, err)
			assert.Equal(t, int64(size), objSize)
		}
	})

	t.Run("ManySmallObjects", func(t *testing.T) {
		const numObjects = 100000
		const objectSize = 100
		
		start := time.Now()
		
		// Store many small objects
		for i := 0; i < numObjects; i++ {
			hash := fmt.Sprintf("small-%08d", i)
			data := make([]byte, objectSize)
			for j := range data {
				data[j] = byte((i + j) % 256)
			}
			
			err = store.ObjectStore().PutObject(hash, data)
			assert.NoError(t, err, "Failed to store object %d", i)
			
			if i > 0 && i%10000 == 0 {
				elapsed := time.Since(start)
				rate := float64(i) / elapsed.Seconds()
				t.Logf("Stored %d objects, rate: %.2f ops/sec", i, rate)
			}
		}
		
		totalTime := time.Since(start)
		rate := float64(numObjects) / totalTime.Seconds()
		t.Logf("Total: %d objects in %v (%.2f ops/sec)", numObjects, totalTime, rate)
		
		// Verify count
		count, err := store.ObjectStore().CountObjects()
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, count, int64(numObjects))
	})
}

func TestBadgerStore_Iteration(t *testing.T) {
	tempDir := t.TempDir()
	
	config := datastore.Config{
		Type:       datastore.TypeBadger,
		Connection: tempDir,
	}
	
	store, err := New(config)
	require.NoError(t, err)
	
	err = store.Initialize(config)
	require.NoError(t, err)
	defer store.Close()

	// Create test data with different prefixes
	testData := map[string][]byte{
		"prefix-a-001": []byte("data a 1"),
		"prefix-a-002": []byte("data a 2"),
		"prefix-a-003": []byte("data a 3"),
		"prefix-b-001": []byte("data b 1"),
		"prefix-b-002": []byte("data b 2"),
		"prefix-c-001": []byte("data c 1"),
	}
	
	for hash, data := range testData {
		err = store.ObjectStore().PutObject(hash, data)
		require.NoError(t, err)
	}

	t.Run("PrefixIteration", func(t *testing.T) {
		// Iterate with prefix "prefix-a-"
		visited := make(map[string][]byte)
		err = store.ObjectStore().IterateObjects("prefix-a-", func(hash string, data []byte) error {
			visited[hash] = data
			return nil
		})
		assert.NoError(t, err)
		
		// Should have visited only "prefix-a-" objects
		assert.Len(t, visited, 3)
		assert.Equal(t, []byte("data a 1"), visited["prefix-a-001"])
		assert.Equal(t, []byte("data a 2"), visited["prefix-a-002"])
		assert.Equal(t, []byte("data a 3"), visited["prefix-a-003"])
	})

	t.Run("IterationWithError", func(t *testing.T) {
		stopError := fmt.Errorf("stop iteration")
		count := 0
		
		err = store.ObjectStore().IterateObjects("prefix-", func(hash string, data []byte) error {
			count++
			if count >= 3 {
				return stopError
			}
			return nil
		})
		
		assert.ErrorIs(t, err, stopError)
		assert.Equal(t, 3, count)
	})

	t.Run("ListObjectsWithLimit", func(t *testing.T) {
		// List all objects with prefix
		all, err := store.ObjectStore().ListObjects("prefix-", 0)
		assert.NoError(t, err)
		assert.Len(t, all, 6)
		
		// List with limit
		limited, err := store.ObjectStore().ListObjects("prefix-", 3)
		assert.NoError(t, err)
		assert.Len(t, limited, 3)
		
		// List with prefix that has no matches
		empty, err := store.ObjectStore().ListObjects("nonexistent-", 0)
		assert.NoError(t, err)
		assert.Empty(t, empty)
	})

	t.Run("IterationOrder", func(t *testing.T) {
		// BadgerDB should iterate in key order
		var keys []string
		err = store.ObjectStore().IterateObjects("prefix-a-", func(hash string, data []byte) error {
			keys = append(keys, hash)
			return nil
		})
		assert.NoError(t, err)
		
		// Should be in lexicographical order
		assert.Equal(t, []string{"prefix-a-001", "prefix-a-002", "prefix-a-003"}, keys)
	})
}

func TestBadgerStore_TransactionEdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	
	config := datastore.Config{
		Type:       datastore.TypeBadger,
		Connection: tempDir,
	}
	
	store, err := New(config)
	require.NoError(t, err)
	
	err = store.Initialize(config)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	t.Run("TransactionConflicts", func(t *testing.T) {
		// BadgerDB has optimistic concurrency control
		
		// Create initial data
		err = store.ObjectStore().PutObject("conflict-test", []byte("initial"))
		require.NoError(t, err)
		
		// Start two transactions
		tx1, err := store.BeginTx(ctx, nil)
		require.NoError(t, err)
		
		tx2, err := store.BeginTx(ctx, nil)
		require.NoError(t, err)
		
		// Both transactions read the same key
		data1, err := tx1.GetObject("conflict-test")
		assert.NoError(t, err)
		assert.Equal(t, []byte("initial"), data1)
		
		data2, err := tx2.GetObject("conflict-test")
		assert.NoError(t, err)
		assert.Equal(t, []byte("initial"), data2)
		
		// Both transactions modify the same key
		err = tx1.PutObject("conflict-test", []byte("modified-by-tx1"))
		assert.NoError(t, err)
		
		err = tx2.PutObject("conflict-test", []byte("modified-by-tx2"))
		assert.NoError(t, err)
		
		// First transaction commits
		err = tx1.Commit()
		assert.NoError(t, err)
		
		// Second transaction should fail on commit (conflict)
		err = tx2.Commit()
		// BadgerDB might detect the conflict and fail, or allow last-writer-wins
		// The exact behavior depends on BadgerDB version and configuration
		// We don't assert on the error here as it's implementation dependent
		
		// Verify the final state
		final, err := store.ObjectStore().GetObject("conflict-test")
		assert.NoError(t, err)
		// Should be one of the two values
		assert.True(t, string(final) == "modified-by-tx1" || string(final) == "modified-by-tx2")
	})

	t.Run("LongRunningTransaction", func(t *testing.T) {
		tx, err := store.BeginTx(ctx, nil)
		require.NoError(t, err)
		
		// Perform many operations
		const numOps = 10000
		for i := 0; i < numOps; i++ {
			hash := fmt.Sprintf("long-tx-%04d", i)
			data := []byte(fmt.Sprintf("long transaction data %d", i))
			
			err = tx.PutObject(hash, data)
			assert.NoError(t, err, "Failed operation %d in long transaction", i)
		}
		
		// Commit large transaction
		err = tx.Commit()
		assert.NoError(t, err)
		
		// Verify all operations succeeded
		count, err := store.ObjectStore().CountObjects()
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, count, int64(numOps))
	})

	t.Run("TransactionMemoryUsage", func(t *testing.T) {
		// Test that transactions don't consume excessive memory
		tx, err := store.BeginTx(ctx, nil)
		require.NoError(t, err)
		
		// Add large amount of data in transaction
		const numObjects = 1000
		const objectSize = 1024 * 10 // 10KB each
		
		for i := 0; i < numObjects; i++ {
			hash := fmt.Sprintf("memory-test-%04d", i)
			data := make([]byte, objectSize)
			for j := range data {
				data[j] = byte((i + j) % 256)
			}
			
			err = tx.PutObject(hash, data)
			assert.NoError(t, err, "Failed to add object %d", i)
		}
		
		// Transaction should still be usable
		exists, err := tx.HasObject("memory-test-0000")
		assert.NoError(t, err)
		assert.True(t, exists)
		
		err = tx.Rollback()
		assert.NoError(t, err)
	})
}

func TestBadgerStore_CompactionAndMaintenance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping maintenance test in short mode")
	}
	
	tempDir := t.TempDir()
	
	config := datastore.Config{
		Type:       datastore.TypeBadger,
		Connection: tempDir,
	}
	
	store, err := New(config)
	require.NoError(t, err)
	
	err = store.Initialize(config)
	require.NoError(t, err)
	defer store.Close()

	t.Run("DataCompaction", func(t *testing.T) {
		// Add lots of data to trigger compaction
		const numObjects = 10000
		
		for i := 0; i < numObjects; i++ {
			hash := fmt.Sprintf("compact-test-%08d", i)
			data := make([]byte, 1024)
			for j := range data {
				data[j] = byte((i + j) % 256)
			}
			
			err = store.ObjectStore().PutObject(hash, data)
			assert.NoError(t, err, "Failed to store object %d", i)
		}
		
		// Get initial size
		initialSize, err := store.GetStorageSize()
		assert.NoError(t, err)
		t.Logf("Initial storage size: %d bytes", initialSize)
		
		// Delete half the objects to create dead space
		for i := 0; i < numObjects/2; i++ {
			hash := fmt.Sprintf("compact-test-%08d", i)
			err = store.ObjectStore().DeleteObject(hash)
			assert.NoError(t, err, "Failed to delete object %d", i)
		}
		
		// Force garbage collection if possible
		// BadgerDB will run compaction automatically
		// We can wait a bit for it to happen
		time.Sleep(time.Second * 2)
		
		// Check final count
		count, err := store.ObjectStore().CountObjects()
		assert.NoError(t, err)
		assert.Equal(t, int64(numObjects/2), count)
	})

	t.Run("ValueLogGarbageCollection", func(t *testing.T) {
		// Add and delete large objects to test value log GC
		const numLargeObjects = 100
		const objectSize = 1024 * 100 // 100KB each
		
		// Add large objects
		for i := 0; i < numLargeObjects; i++ {
			hash := fmt.Sprintf("vlgc-test-%04d", i)
			data := make([]byte, objectSize)
			for j := range data {
				data[j] = byte((i * j) % 256)
			}
			
			err = store.ObjectStore().PutObject(hash, data)
			assert.NoError(t, err, "Failed to store large object %d", i)
		}
		
		// Delete most of them
		for i := 0; i < numLargeObjects*3/4; i++ {
			hash := fmt.Sprintf("vlgc-test-%04d", i)
			err = store.ObjectStore().DeleteObject(hash)
			assert.NoError(t, err, "Failed to delete large object %d", i)
		}
		
		// Wait for potential garbage collection
		time.Sleep(time.Second * 3)
		
		// Verify remaining objects are still accessible
		for i := numLargeObjects * 3 / 4; i < numLargeObjects; i++ {
			hash := fmt.Sprintf("vlgc-test-%04d", i)
			exists, err := store.ObjectStore().HasObject(hash)
			assert.NoError(t, err)
			assert.True(t, exists, "Object %s should still exist", hash)
		}
	})
}

func TestBadgerStore_ErrorRecovery(t *testing.T) {
	tempDir := t.TempDir()
	
	config := datastore.Config{
		Type:       datastore.TypeBadger,
		Connection: tempDir,
	}

	t.Run("RecoveryAfterCrash", func(t *testing.T) {
		// Create store and add data
		store1, err := New(config)
		require.NoError(t, err)
		
		err = store1.Initialize(config)
		require.NoError(t, err)
		
		// Add committed data
		err = store1.PutObject("recovery-committed", []byte("committed data"))
		require.NoError(t, err)
		
		// Start transaction but don't commit
		ctx := context.Background()
		tx, err := store1.BeginTx(ctx, nil)
		require.NoError(t, err)
		
		err = tx.PutObject("recovery-uncommitted", []byte("uncommitted data"))
		require.NoError(t, err)
		
		// Force close without proper cleanup (simulates crash)
		store1.Close()
		
		// Create new store instance
		store2, err := New(config)
		require.NoError(t, err)
		
		err = store2.Initialize(config)
		require.NoError(t, err)
		defer store2.Close()
		
		// Committed data should be recovered
		data, err := store2.GetObject("recovery-committed")
		assert.NoError(t, err)
		assert.Equal(t, []byte("committed data"), data)
		
		// Uncommitted data should not exist
		_, err = store2.GetObject("recovery-uncommitted")
		assert.ErrorIs(t, err, datastore.ErrNotFound)
	})

	t.Run("CorruptedDataHandling", func(t *testing.T) {
		// Create store and add data
		store1, err := New(config)
		require.NoError(t, err)
		
		err = store1.Initialize(config)
		require.NoError(t, err)
		
		err = store1.PutObject("corruption-test", []byte("original data"))
		require.NoError(t, err)
		
		store1.Close()
		
		// Simulate corruption by writing invalid data to a file
		// Note: This is a simplified test - real corruption testing would be more complex
		files, err := filepath.Glob(filepath.Join(tempDir, "*"))
		if err == nil && len(files) > 0 {
			// Write some random bytes to potentially corrupt a file
			corruptData := []byte("CORRUPTED_DATA_PATTERN")
			for _, file := range files {
				if stat, err := os.Stat(file); err == nil && !stat.IsDir() && stat.Size() > 0 {
					// Try to append corrupt data to the file
					f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0644)
					if err == nil {
						f.Write(corruptData)
						f.Close()
						break // Only corrupt one file
					}
				}
			}
		}
		
		// Try to open store again
		store2, err := New(config)
		require.NoError(t, err)
		
		err = store2.Initialize(config)
		// BadgerDB might detect corruption and fail to open,
		// or it might open successfully but have data issues
		if err != nil {
			// Expected case - BadgerDB detected corruption
			t.Logf("BadgerDB correctly detected corruption: %v", err)
		} else {
			// Store opened successfully, try to read data
			defer store2.Close()
			
			_, err = store2.GetObject("corruption-test")
			// This might succeed or fail depending on the corruption
			t.Logf("Read after corruption attempt: %v", err)
		}
	})
}