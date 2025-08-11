package redis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Caia-Tech/govc/datastore"
)

func TestMain(m *testing.M) {
	// Skip tests if Redis is not available
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	if err := client.Ping(ctx).Err(); err != nil {
		// Redis not available, skip all tests
		os.Exit(0)
	}
	client.Close()
	
	os.Exit(m.Run())
}

func setupTestRedisStore(t *testing.T) (*RedisStore, func()) {
	config := datastore.Config{
		Type:       datastore.TypeRedis,
		Connection: "redis://localhost:6379/15", // Use DB 15 for testing
	}

	store, err := New(config)
	require.NoError(t, err)
	require.NotNil(t, store)

	err = store.Initialize(config)
	require.NoError(t, err)

	// Clean up function
	cleanup := func() {
		// Clear test database
		ctx := context.Background()
		store.client.FlushDB(ctx)
		store.Close()
	}

	return store, cleanup
}

func TestRedisStore_Initialize(t *testing.T) {
	store, cleanup := setupTestRedisStore(t)
	defer cleanup()

	assert.Equal(t, datastore.TypeRedis, store.Type())
	
	// Test health check
	ctx := context.Background()
	err := store.HealthCheck(ctx)
	assert.NoError(t, err)
}

func TestRedisStore_ObjectStore_BasicOperations(t *testing.T) {
	store, cleanup := setupTestRedisStore(t)
	defer cleanup()

	objStore := store.ObjectStore()
	require.NotNil(t, objStore)

	// Test data
	hash := "test-hash-123"
	data := []byte("test data for redis storage")

	// Test PutObject
	err := objStore.PutObject(hash, data)
	assert.NoError(t, err)

	// Test HasObject
	exists, err := objStore.HasObject(hash)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Test GetObject
	retrieved, err := objStore.GetObject(hash)
	assert.NoError(t, err)
	assert.Equal(t, data, retrieved)

	// Test GetObjectSize
	size, err := objStore.GetObjectSize(hash)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(data)), size)

	// Test DeleteObject
	err = objStore.DeleteObject(hash)
	assert.NoError(t, err)

	// Verify deletion
	exists, err = objStore.HasObject(hash)
	assert.NoError(t, err)
	assert.False(t, exists)

	// Test getting non-existent object
	_, err = objStore.GetObject(hash)
	assert.Equal(t, datastore.ErrNotFound, err)
}

func TestRedisStore_ObjectStore_BatchOperations(t *testing.T) {
	store, cleanup := setupTestRedisStore(t)
	defer cleanup()

	objStore := store.ObjectStore()

	// Test data
	objects := map[string][]byte{
		"hash1": []byte("data one"),
		"hash2": []byte("data two"),
		"hash3": []byte("data three"),
	}

	// Test PutObjects
	err := objStore.PutObjects(objects)
	assert.NoError(t, err)

	// Test GetObjects
	retrieved, err := objStore.GetObjects([]string{"hash1", "hash2", "hash3"})
	assert.NoError(t, err)
	assert.Len(t, retrieved, 3)
	assert.Equal(t, objects["hash1"], retrieved["hash1"])
	assert.Equal(t, objects["hash2"], retrieved["hash2"])
	assert.Equal(t, objects["hash3"], retrieved["hash3"])

	// Test DeleteObjects
	err = objStore.DeleteObjects([]string{"hash1", "hash3"})
	assert.NoError(t, err)

	// Verify partial deletion
	retrieved, err = objStore.GetObjects([]string{"hash1", "hash2", "hash3"})
	assert.NoError(t, err)
	assert.Len(t, retrieved, 1) // Only hash2 should remain
	assert.Equal(t, objects["hash2"], retrieved["hash2"])
}

func TestRedisStore_ObjectStore_ListOperations(t *testing.T) {
	store, cleanup := setupTestRedisStore(t)
	defer cleanup()

	objStore := store.ObjectStore()

	// Test data with different prefixes
	testData := map[string][]byte{
		"prefix1-hash1": []byte("data 1"),
		"prefix1-hash2": []byte("data 2"),
		"prefix2-hash1": []byte("data 3"),
		"other-hash":    []byte("data 4"),
	}

	// Put all test data
	err := objStore.PutObjects(testData)
	assert.NoError(t, err)

	// Test ListObjects with prefix
	hashes, err := objStore.ListObjects("prefix1-", 0)
	assert.NoError(t, err)
	assert.Len(t, hashes, 2)
	assert.Contains(t, hashes, "prefix1-hash1")
	assert.Contains(t, hashes, "prefix1-hash2")

	// Test ListObjects with limit
	hashes, err = objStore.ListObjects("", 2)
	assert.NoError(t, err)
	assert.Len(t, hashes, 2)

	// Test CountObjects
	count, err := objStore.CountObjects()
	assert.NoError(t, err)
	assert.Equal(t, int64(4), count)

	// Test GetStorageSize
	size, err := objStore.GetStorageSize()
	assert.NoError(t, err)
	assert.Greater(t, size, int64(0))
}

func TestRedisStore_ObjectStore_IterateObjects(t *testing.T) {
	store, cleanup := setupTestRedisStore(t)
	defer cleanup()

	objStore := store.ObjectStore()

	// Test data
	testData := map[string][]byte{
		"iter-1": []byte("iteration data 1"),
		"iter-2": []byte("iteration data 2"),
		"iter-3": []byte("iteration data 3"),
	}

	err := objStore.PutObjects(testData)
	assert.NoError(t, err)

	// Test iteration
	visited := make(map[string][]byte)
	err = objStore.IterateObjects("iter-", func(hash string, data []byte) error {
		visited[hash] = data
		return nil
	})

	assert.NoError(t, err)
	assert.Len(t, visited, 3)
	assert.Equal(t, testData["iter-1"], visited["iter-1"])
	assert.Equal(t, testData["iter-2"], visited["iter-2"])
	assert.Equal(t, testData["iter-3"], visited["iter-3"])
}

func TestRedisStore_Transaction_Basic(t *testing.T) {
	store, cleanup := setupTestRedisStore(t)
	defer cleanup()

	ctx := context.Background()
	
	// Begin transaction
	tx, err := store.BeginTx(ctx, &datastore.TxOptions{})
	assert.NoError(t, err)
	require.NotNil(t, tx)

	// Test transaction methods exist (they should return ErrNotImplemented for most operations)
	err = tx.PutObject("tx-hash", []byte("tx data"))
	assert.NoError(t, err) // This should work (writes to pipeline)

	// Most read operations should fail
	_, err = tx.GetObject("tx-hash")
	assert.Error(t, err) // Should fail for Redis transactions

	// Test commit
	err = tx.Commit()
	assert.NoError(t, err)

	// After commit, we should see the data in the store
	data, err := store.ObjectStore().GetObject("tx-hash")
	assert.NoError(t, err)
	assert.Equal(t, []byte("tx data"), data)
}

func TestRedisStore_Transaction_Rollback(t *testing.T) {
	store, cleanup := setupTestRedisStore(t)
	defer cleanup()

	ctx := context.Background()

	// Begin transaction
	tx, err := store.BeginTx(ctx, &datastore.TxOptions{})
	assert.NoError(t, err)

	// Add some data
	err = tx.PutObject("rollback-hash", []byte("rollback data"))
	assert.NoError(t, err)

	// Rollback
	err = tx.Rollback()
	assert.NoError(t, err)

	// Data should not exist
	_, err = store.ObjectStore().GetObject("rollback-hash")
	assert.Equal(t, datastore.ErrNotFound, err)
}

func TestRedisStore_Metrics(t *testing.T) {
	store, cleanup := setupTestRedisStore(t)
	defer cleanup()

	metrics := store.GetMetrics()
	assert.Greater(t, metrics.Uptime, time.Duration(0))
	assert.Equal(t, 1, metrics.ActiveConnections)
}

func TestRedisStore_Info(t *testing.T) {
	store, cleanup := setupTestRedisStore(t)
	defer cleanup()

	info := store.Info()
	assert.Equal(t, datastore.TypeRedis, info["type"])
	assert.Contains(t, info, "connection")
	assert.Equal(t, "healthy", info["status"])
}

func TestRedisStore_InvalidOperations(t *testing.T) {
	store, cleanup := setupTestRedisStore(t)
	defer cleanup()

	objStore := store.ObjectStore()

	// Test invalid hash
	err := objStore.PutObject("", []byte("data"))
	assert.Equal(t, datastore.ErrInvalidData, err)

	_, err = objStore.GetObject("")
	assert.Equal(t, datastore.ErrInvalidData, err)

	exists, err := objStore.HasObject("")
	assert.Equal(t, datastore.ErrInvalidData, err)
	assert.False(t, exists)

	err = objStore.DeleteObject("")
	assert.Equal(t, datastore.ErrInvalidData, err)

	_, err = objStore.GetObjectSize("")
	assert.Equal(t, datastore.ErrInvalidData, err)
}

func TestRedisStore_MetadataStore_NotImplemented(t *testing.T) {
	store, cleanup := setupTestRedisStore(t)
	defer cleanup()

	metaStore := store.MetadataStore()
	require.NotNil(t, metaStore)

	// All metadata operations should return ErrNotImplemented for Redis
	err := metaStore.SaveRepository(&datastore.Repository{})
	assert.Equal(t, datastore.ErrNotImplemented, err)

	_, err = metaStore.GetRepository("test")
	assert.Equal(t, datastore.ErrNotImplemented, err)

	_, err = metaStore.ListRepositories(datastore.RepositoryFilter{})
	assert.Equal(t, datastore.ErrNotImplemented, err)

	err = metaStore.DeleteRepository("test")
	assert.Equal(t, datastore.ErrNotImplemented, err)
}

func TestRedisStore_Close(t *testing.T) {
	config := datastore.Config{
		Type:       datastore.TypeRedis,
		Connection: "redis://localhost:6379/15",
	}

	store, err := New(config)
	require.NoError(t, err)
	
	err = store.Initialize(config)
	require.NoError(t, err)

	// Close should work
	err = store.Close()
	assert.NoError(t, err)

	// Second close should also work (idempotent)
	err = store.Close()
	assert.NoError(t, err)

	// Operations after close should fail
	ctx := context.Background()
	err = store.HealthCheck(ctx)
	assert.Error(t, err)
}