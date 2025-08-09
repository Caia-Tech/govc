package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/caiatech/govc/datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore(t *testing.T) {
	config := datastore.DefaultConfig(datastore.TypeMemory)
	store := New(config)
	defer store.Close()

	err := store.Initialize(config)
	require.NoError(t, err)

	t.Run("ObjectStore", func(t *testing.T) {
		testObjectStore(t, store)
	})

	t.Run("MetadataStore", func(t *testing.T) {
		testMetadataStore(t, store)
	})

	t.Run("Transaction", func(t *testing.T) {
		testTransaction(t, store)
	})

	t.Run("Metrics", func(t *testing.T) {
		testMetrics(t, store)
	})
}

func testObjectStore(t *testing.T, store *MemoryStore) {
	objectStore := store.ObjectStore()

	// Test PutObject and GetObject
	hash := "abc123"
	data := []byte("test data")
	
	err := objectStore.PutObject(hash, data)
	assert.NoError(t, err)

	retrieved, err := objectStore.GetObject(hash)
	assert.NoError(t, err)
	assert.Equal(t, data, retrieved)

	// Test HasObject
	exists, err := objectStore.HasObject(hash)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Test ListObjects
	err = objectStore.PutObject("abc456", []byte("more data"))
	assert.NoError(t, err)

	objects, err := objectStore.ListObjects("abc", 10)
	assert.NoError(t, err)
	assert.Len(t, objects, 2)

	// Test DeleteObject
	err = objectStore.DeleteObject(hash)
	assert.NoError(t, err)

	exists, err = objectStore.HasObject(hash)
	assert.NoError(t, err)
	assert.False(t, exists)

	// Test batch operations
	batch := map[string][]byte{
		"hash1": []byte("data1"),
		"hash2": []byte("data2"),
		"hash3": []byte("data3"),
	}

	err = objectStore.PutObjects(batch)
	assert.NoError(t, err)

	results, err := objectStore.GetObjects([]string{"hash1", "hash2", "hash3"})
	assert.NoError(t, err)
	assert.Len(t, results, 3)
	assert.Equal(t, batch["hash1"], results["hash1"])
}

func testMetadataStore(t *testing.T, store *MemoryStore) {
	metaStore := store.MetadataStore()

	// Test Repository operations
	repo := &datastore.Repository{
		Name:        "test-repo",
		Description: "Test repository",
		IsPrivate:   false,
	}

	err := metaStore.SaveRepository(repo)
	assert.NoError(t, err)
	assert.NotEmpty(t, repo.ID)

	retrieved, err := metaStore.GetRepository(repo.ID)
	assert.NoError(t, err)
	assert.Equal(t, repo.Name, retrieved.Name)

	// Test User operations
	user := &datastore.User{
		Username: "testuser",
		Email:    "test@example.com",
		FullName: "Test User",
		IsActive: true,
	}

	err = metaStore.SaveUser(user)
	assert.NoError(t, err)
	assert.NotEmpty(t, user.ID)

	retrievedUser, err := metaStore.GetUser(user.ID)
	assert.NoError(t, err)
	assert.Equal(t, user.Username, retrievedUser.Username)

	// Test GetUserByUsername
	byUsername, err := metaStore.GetUserByUsername("testuser")
	assert.NoError(t, err)
	assert.Equal(t, user.ID, byUsername.ID)

	// Test Reference operations
	ref := &datastore.Reference{
		Name: "refs/heads/main",
		Hash: "abc123",
		Type: datastore.RefTypeBranch,
	}

	err = metaStore.SaveRef(repo.ID, ref)
	assert.NoError(t, err)

	retrievedRef, err := metaStore.GetRef(repo.ID, "refs/heads/main")
	assert.NoError(t, err)
	assert.Equal(t, ref.Hash, retrievedRef.Hash)

	// Test Audit logging
	event := &datastore.AuditEvent{
		UserID:   user.ID,
		Username: user.Username,
		Action:   "repository.create",
		Resource: "repository",
		ResourceID: repo.ID,
		Success:  true,
	}

	err = metaStore.LogEvent(event)
	assert.NoError(t, err)

	events, err := metaStore.QueryEvents(datastore.EventFilter{
		UserID: user.ID,
		Limit:  10,
	})
	assert.NoError(t, err)
	assert.Len(t, events, 1)

	// Test Configuration
	err = metaStore.SetConfig("test.key", "test.value")
	assert.NoError(t, err)

	value, err := metaStore.GetConfig("test.key")
	assert.NoError(t, err)
	assert.Equal(t, "test.value", value)
}

func testTransaction(t *testing.T, store *MemoryStore) {
	ctx := context.Background()
	
	// Test successful transaction
	tx, err := store.BeginTx(ctx, nil)
	require.NoError(t, err)

	err = tx.PutObject("tx_test", []byte("transaction data"))
	assert.NoError(t, err)

	// Data should not be visible before commit
	_, err = store.ObjectStore().GetObject("tx_test")
	assert.Error(t, err)

	err = tx.Commit()
	assert.NoError(t, err)

	// Data should be visible after commit
	data, err := store.ObjectStore().GetObject("tx_test")
	assert.NoError(t, err)
	assert.Equal(t, []byte("transaction data"), data)

	// Test rollback
	tx2, err := store.BeginTx(ctx, nil)
	require.NoError(t, err)

	err = tx2.PutObject("tx_test2", []byte("rollback data"))
	assert.NoError(t, err)

	err = tx2.Rollback()
	assert.NoError(t, err)

	// Data should not exist after rollback
	_, err = store.ObjectStore().GetObject("tx_test2")
	assert.Error(t, err)
}

func testMetrics(t *testing.T, store *MemoryStore) {
	metrics := store.GetMetrics()
	
	assert.Greater(t, metrics.Reads, int64(0))
	assert.Greater(t, metrics.Writes, int64(0))
	assert.NotZero(t, metrics.StartTime)
	assert.Greater(t, metrics.Uptime, time.Duration(0))
}

func BenchmarkMemoryStore(b *testing.B) {
	config := datastore.DefaultConfig(datastore.TypeMemory)
	store := New(config)
	defer store.Close()

	b.Run("PutObject", func(b *testing.B) {
		data := []byte("benchmark data")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash := fmt.Sprintf("hash%d", i)
			store.ObjectStore().PutObject(hash, data)
		}
	})

	b.Run("GetObject", func(b *testing.B) {
		// Prepare data
		for i := 0; i < 1000; i++ {
			hash := fmt.Sprintf("bench%d", i)
			store.ObjectStore().PutObject(hash, []byte("data"))
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash := fmt.Sprintf("bench%d", i%1000)
			store.ObjectStore().GetObject(hash)
		}
	})

	b.Run("Transaction", func(b *testing.B) {
		ctx := context.Background()
		data := []byte("transaction data")
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tx, _ := store.BeginTx(ctx, nil)
			tx.PutObject(fmt.Sprintf("tx%d", i), data)
			tx.Commit()
		}
	})
}