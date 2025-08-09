package badger

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/caiatech/govc/datastore"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBadgerStore(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "govc-badger-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	
	dbPath := filepath.Join(tmpDir, "badger")
	
	config := datastore.Config{
		Type:       datastore.TypeBadger,
		Connection: dbPath,
		Options: map[string]interface{}{
			"sync_writes":       false,
			"compression":       true,
			"memory_table_size": 64 << 20,
		},
	}
	
	store, err := New(config)
	require.NoError(t, err)
	
	err = store.Initialize(config)
	require.NoError(t, err)
	defer store.Close()
	
	t.Run("ObjectStore", func(t *testing.T) {
		testObjectStore(t, store)
	})
	
	t.Run("MetadataStore", func(t *testing.T) {
		testMetadataStore(t, store)
	})
	
	t.Run("Transactions", func(t *testing.T) {
		testTransactions(t, store)
	})
	
	t.Run("Metrics", func(t *testing.T) {
		testMetrics(t, store)
	})
}

func testObjectStore(t *testing.T, store *BadgerStore) {
	objStore := store.ObjectStore()
	
	// Test single object operations
	t.Run("SingleObject", func(t *testing.T) {
		hash := "badger-test-hash"
		data := []byte("badger test data")
		
		// Put object
		err := objStore.PutObject(hash, data)
		assert.NoError(t, err)
		
		// Get object
		retrieved, err := objStore.GetObject(hash)
		assert.NoError(t, err)
		assert.Equal(t, data, retrieved)
		
		// Check existence
		exists, err := objStore.HasObject(hash)
		assert.NoError(t, err)
		assert.True(t, exists)
		
		// Get size
		size, err := objStore.GetObjectSize(hash)
		assert.NoError(t, err)
		assert.Equal(t, int64(len(data)), size)
		
		// Delete object
		err = objStore.DeleteObject(hash)
		assert.NoError(t, err)
		
		// Check non-existence
		exists, err = objStore.HasObject(hash)
		assert.NoError(t, err)
		assert.False(t, exists)
		
		// Get non-existent object
		_, err = objStore.GetObject(hash)
		assert.ErrorIs(t, err, datastore.ErrNotFound)
	})
	
	// Test batch operations
	t.Run("BatchOperations", func(t *testing.T) {
		objects := map[string][]byte{
			"batch1": []byte("data1"),
			"batch2": []byte("data2"),
			"batch3": []byte("data3"),
		}
		
		// Put multiple objects
		err := objStore.PutObjects(objects)
		assert.NoError(t, err)
		
		// Get multiple objects
		hashes := []string{"batch1", "batch2", "batch3"}
		retrieved, err := objStore.GetObjects(hashes)
		assert.NoError(t, err)
		assert.Equal(t, objects, retrieved)
		
		// List objects
		list, err := objStore.ListObjects("batch", 10)
		assert.NoError(t, err)
		assert.Len(t, list, 3)
		
		// Count objects
		count, err := objStore.CountObjects()
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, count, int64(3))
		
		// Get storage size
		size, err := objStore.GetStorageSize()
		assert.NoError(t, err)
		assert.Greater(t, size, int64(0))
		
		// Delete multiple objects
		err = objStore.DeleteObjects(hashes)
		assert.NoError(t, err)
		
		// Verify deletion
		for _, hash := range hashes {
			exists, err := objStore.HasObject(hash)
			assert.NoError(t, err)
			assert.False(t, exists)
		}
	})
	
	// Test iteration
	t.Run("Iteration", func(t *testing.T) {
		// Add test objects
		testObjects := map[string][]byte{
			"iterate1": []byte("iterdata1"),
			"iterate2": []byte("iterdata2"),
			"iterate3": []byte("iterdata3"),
		}
		
		err := objStore.PutObjects(testObjects)
		assert.NoError(t, err)
		
		// Iterate over objects
		visited := make(map[string][]byte)
		err = objStore.IterateObjects("iterate", func(hash string, data []byte) error {
			visited[hash] = data
			return nil
		})
		assert.NoError(t, err)
		
		// Check all test objects were visited
		for hash, data := range testObjects {
			assert.Contains(t, visited, hash)
			assert.Equal(t, data, visited[hash])
		}
		
		// Clean up
		hashes := make([]string, 0, len(testObjects))
		for hash := range testObjects {
			hashes = append(hashes, hash)
		}
		objStore.DeleteObjects(hashes)
	})
}

func testMetadataStore(t *testing.T, store *BadgerStore) {
	metaStore := store.MetadataStore()
	
	// Test repository operations
	t.Run("Repository", func(t *testing.T) {
		repo := &datastore.Repository{
			ID:          uuid.New().String(),
			Name:        "badger-test-repo",
			Description: "Test repository for BadgerDB",
			Path:        "/badger/test/repo",
			IsPrivate:   false,
			Metadata:    map[string]interface{}{"engine": "badger"},
			Size:        2048,
			CommitCount: 20,
			BranchCount: 5,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		
		// Save repository
		err := metaStore.SaveRepository(repo)
		assert.NoError(t, err)
		
		// Get repository
		retrieved, err := metaStore.GetRepository(repo.ID)
		assert.NoError(t, err)
		assert.Equal(t, repo.Name, retrieved.Name)
		assert.Equal(t, repo.Description, retrieved.Description)
		
		// Update repository
		err = metaStore.UpdateRepository(repo.ID, map[string]interface{}{
			"description": "Updated BadgerDB description",
			"is_private":  true,
		})
		assert.NoError(t, err)
		
		// Verify update
		updated, err := metaStore.GetRepository(repo.ID)
		assert.NoError(t, err)
		assert.Equal(t, "Updated BadgerDB description", updated.Description)
		assert.True(t, updated.IsPrivate)
		
		// List repositories
		repos, err := metaStore.ListRepositories(datastore.RepositoryFilter{
			Limit: 10,
		})
		assert.NoError(t, err)
		assert.NotEmpty(t, repos)
		
		// Delete repository
		err = metaStore.DeleteRepository(repo.ID)
		assert.NoError(t, err)
		
		// Verify deletion
		_, err = metaStore.GetRepository(repo.ID)
		assert.ErrorIs(t, err, datastore.ErrNotFound)
	})
	
	// Test user operations
	t.Run("User", func(t *testing.T) {
		now := time.Now()
		user := &datastore.User{
			ID:          uuid.New().String(),
			Username:    "badgeruser",
			Email:       "badger@example.com",
			FullName:    "Badger User",
			IsActive:    true,
			IsAdmin:     false,
			Metadata:    map[string]interface{}{"database": "badger"},
			CreatedAt:   now,
			UpdatedAt:   now,
			LastLoginAt: &now,
		}
		
		// Save user
		err := metaStore.SaveUser(user)
		assert.NoError(t, err)
		
		// Get user
		retrieved, err := metaStore.GetUser(user.ID)
		assert.NoError(t, err)
		assert.Equal(t, user.Username, retrieved.Username)
		assert.Equal(t, user.Email, retrieved.Email)
		
		// Get user by username
		byUsername, err := metaStore.GetUserByUsername(user.Username)
		assert.NoError(t, err)
		assert.Equal(t, user.ID, byUsername.ID)
		
		// Update user
		err = metaStore.UpdateUser(user.ID, map[string]interface{}{
			"email":    "newbadger@example.com",
			"is_admin": true,
		})
		assert.NoError(t, err)
		
		// Verify update
		updated, err := metaStore.GetUser(user.ID)
		assert.NoError(t, err)
		assert.Equal(t, "newbadger@example.com", updated.Email)
		assert.True(t, updated.IsAdmin)
		
		// List users
		users, err := metaStore.ListUsers(datastore.UserFilter{
			Limit: 10,
		})
		assert.NoError(t, err)
		assert.NotEmpty(t, users)
		
		// Delete user
		err = metaStore.DeleteUser(user.ID)
		assert.NoError(t, err)
		
		// Verify deletion
		_, err = metaStore.GetUser(user.ID)
		assert.ErrorIs(t, err, datastore.ErrNotFound)
		
		// Username mapping should also be deleted
		_, err = metaStore.GetUserByUsername(user.Username)
		assert.ErrorIs(t, err, datastore.ErrNotFound)
	})
	
	// Test reference operations
	t.Run("Reference", func(t *testing.T) {
		// First create a repository for the reference
		repo := &datastore.Repository{
			ID:        uuid.New().String(),
			Name:      "badger-ref-repo",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		
		err := metaStore.SaveRepository(repo)
		require.NoError(t, err)
		defer metaStore.DeleteRepository(repo.ID)
		
		ref := &datastore.Reference{
			Name:      "refs/heads/badger",
			Hash:      "badger123hash",
			Type:      datastore.RefTypeBranch,
			UpdatedAt: time.Now(),
			UpdatedBy: "badgeruser",
		}
		
		// Save reference
		err = metaStore.SaveRef(repo.ID, ref)
		assert.NoError(t, err)
		
		// Get reference
		retrieved, err := metaStore.GetRef(repo.ID, ref.Name)
		assert.NoError(t, err)
		assert.Equal(t, ref.Hash, retrieved.Hash)
		assert.Equal(t, ref.Type, retrieved.Type)
		
		// Update reference
		err = metaStore.UpdateRef(repo.ID, ref.Name, "newBadgerHash")
		assert.NoError(t, err)
		
		// Verify update
		updated, err := metaStore.GetRef(repo.ID, ref.Name)
		assert.NoError(t, err)
		assert.Equal(t, "newBadgerHash", updated.Hash)
		
		// List references
		refs, err := metaStore.ListRefs(repo.ID, datastore.RefTypeBranch)
		assert.NoError(t, err)
		assert.NotEmpty(t, refs)
		
		// Delete reference
		err = metaStore.DeleteRef(repo.ID, ref.Name)
		assert.NoError(t, err)
		
		// Verify deletion
		_, err = metaStore.GetRef(repo.ID, ref.Name)
		assert.ErrorIs(t, err, datastore.ErrNotFound)
	})
	
	// Test audit events
	t.Run("AuditEvents", func(t *testing.T) {
		event := &datastore.AuditEvent{
			ID:         uuid.New().String(),
			Timestamp:  time.Now(),
			UserID:     "badgeruser123",
			Username:   "badgeruser",
			Action:     "badger_action",
			Resource:   "badger_resource",
			ResourceID: "resource123",
			Details:    map[string]interface{}{"db": "badger"},
			IPAddress:  "192.168.1.1",
			UserAgent:  "badger-agent",
			Success:    true,
			ErrorMsg:   "",
		}
		
		// Log event
		err := metaStore.LogEvent(event)
		assert.NoError(t, err)
		
		// Query events
		events, err := metaStore.QueryEvents(datastore.EventFilter{
			UserID: "badgeruser123",
			Limit:  10,
		})
		assert.NoError(t, err)
		assert.NotEmpty(t, events)
		
		// Count events
		count, err := metaStore.CountEvents(datastore.EventFilter{
			UserID: "badgeruser123",
		})
		assert.NoError(t, err)
		assert.Greater(t, count, int64(0))
	})
	
	// Test configuration
	t.Run("Configuration", func(t *testing.T) {
		// Set config
		err := metaStore.SetConfig("badger.test.key", "badger.test.value")
		assert.NoError(t, err)
		
		// Get config
		value, err := metaStore.GetConfig("badger.test.key")
		assert.NoError(t, err)
		assert.Equal(t, "badger.test.value", value)
		
		// Get all config
		config, err := metaStore.GetAllConfig()
		assert.NoError(t, err)
		assert.Contains(t, config, "badger.test.key")
		assert.Equal(t, "badger.test.value", config["badger.test.key"])
		
		// Delete config
		err = metaStore.DeleteConfig("badger.test.key")
		assert.NoError(t, err)
		
		// Verify deletion
		_, err = metaStore.GetConfig("badger.test.key")
		assert.ErrorIs(t, err, datastore.ErrNotFound)
	})
}

func testTransactions(t *testing.T, store *BadgerStore) {
	ctx := context.Background()
	
	// Test successful transaction
	t.Run("SuccessfulTransaction", func(t *testing.T) {
		tx, err := store.BeginTx(ctx, nil)
		require.NoError(t, err)
		
		// Perform operations
		hash := "badger-tx-test"
		data := []byte("badger transaction test data")
		
		err = tx.PutObject(hash, data)
		assert.NoError(t, err)
		
		// Commit
		err = tx.Commit()
		assert.NoError(t, err)
		
		// Verify data was persisted
		retrieved, err := store.ObjectStore().GetObject(hash)
		assert.NoError(t, err)
		assert.Equal(t, data, retrieved)
		
		// Clean up
		store.ObjectStore().DeleteObject(hash)
	})
	
	// Test rollback
	t.Run("RollbackTransaction", func(t *testing.T) {
		tx, err := store.BeginTx(ctx, nil)
		require.NoError(t, err)
		
		// Perform operations
		hash := "badger-rollback-test"
		data := []byte("badger rollback test data")
		
		err = tx.PutObject(hash, data)
		assert.NoError(t, err)
		
		// Rollback
		err = tx.Rollback()
		assert.NoError(t, err)
		
		// Verify data was not persisted
		_, err = store.ObjectStore().GetObject(hash)
		assert.ErrorIs(t, err, datastore.ErrNotFound)
	})
	
	// Test read-only transaction
	t.Run("ReadOnlyTransaction", func(t *testing.T) {
		// Add test data
		hash := "badger-readonly-test"
		data := []byte("badger readonly data")
		err := store.ObjectStore().PutObject(hash, data)
		require.NoError(t, err)
		defer store.ObjectStore().DeleteObject(hash)
		
		// Begin read-only transaction
		tx, err := store.BeginTx(ctx, &datastore.TxOptions{
			ReadOnly: true,
		})
		require.NoError(t, err)
		
		// Read should work
		retrieved, err := tx.GetObject(hash)
		assert.NoError(t, err)
		assert.Equal(t, data, retrieved)
		
		// Write should fail in read-only transaction
		err = tx.PutObject("should-fail", []byte("data"))
		assert.Error(t, err)
		
		// Commit read-only transaction
		err = tx.Commit()
		assert.NoError(t, err)
	})
}

func testMetrics(t *testing.T, store *BadgerStore) {
	metrics := store.GetMetrics()
	
	assert.Greater(t, metrics.Reads, int64(0))
	assert.Greater(t, metrics.Writes, int64(0))
	assert.GreaterOrEqual(t, metrics.Deletes, int64(0))
	assert.GreaterOrEqual(t, metrics.ObjectCount, int64(0))
	assert.GreaterOrEqual(t, metrics.StorageSize, int64(0))
	assert.NotZero(t, metrics.StartTime)
	assert.Greater(t, metrics.Uptime, time.Duration(0))
}

func TestBadgerStoreHealthCheck(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "govc-badger-health-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	
	dbPath := filepath.Join(tmpDir, "health")
	
	config := datastore.Config{
		Type:       datastore.TypeBadger,
		Connection: dbPath,
	}
	
	store, err := New(config)
	require.NoError(t, err)
	
	err = store.Initialize(config)
	require.NoError(t, err)
	defer store.Close()
	
	// Health check should succeed
	ctx := context.Background()
	err = store.HealthCheck(ctx)
	assert.NoError(t, err)
	
	// Close store
	err = store.Close()
	assert.NoError(t, err)
	
	// Health check should fail after close
	err = store.HealthCheck(ctx)
	assert.Error(t, err)
}

func TestBadgerStoreInfo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "govc-badger-info-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	
	dbPath := filepath.Join(tmpDir, "info")
	
	config := datastore.Config{
		Type:       datastore.TypeBadger,
		Connection: dbPath,
	}
	
	store, err := New(config)
	require.NoError(t, err)
	
	err = store.Initialize(config)
	require.NoError(t, err)
	defer store.Close()
	
	info := store.Info()
	
	assert.Equal(t, datastore.TypeBadger, info["type"])
	assert.Equal(t, dbPath, info["path"])
	assert.Contains(t, info, "lsm_size")
	assert.Contains(t, info, "vlog_size")
	assert.Contains(t, info, "total_size")
	assert.Contains(t, info, "objects")
	assert.Contains(t, info, "uptime")
}

func BenchmarkBadgerStore(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "govc-badger-bench-*")
	require.NoError(b, err)
	defer os.RemoveAll(tmpDir)
	
	dbPath := filepath.Join(tmpDir, "bench")
	
	config := datastore.Config{
		Type:       datastore.TypeBadger,
		Connection: dbPath,
		Options: map[string]interface{}{
			"sync_writes":       false,
			"compression":       false, // Disable for benchmarks
			"memory_table_size": 128 << 20, // 128MB for benchmarks
		},
	}
	
	store, err := New(config)
	require.NoError(b, err)
	
	err = store.Initialize(config)
	require.NoError(b, err)
	defer store.Close()
	
	// Prepare test data
	testData := make([]byte, 1024) // 1KB object
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	
	b.Run("PutObject", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hash := uuid.New().String()
			err := store.ObjectStore().PutObject(hash, testData)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	
	// Add some objects for read benchmarks
	testHashes := make([]string, 100)
	for i := 0; i < 100; i++ {
		testHashes[i] = uuid.New().String()
		store.ObjectStore().PutObject(testHashes[i], testData)
	}
	
	b.Run("GetObject", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hash := testHashes[i%100]
			_, err := store.ObjectStore().GetObject(hash)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	
	b.Run("HasObject", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hash := testHashes[i%100]
			_, err := store.ObjectStore().HasObject(hash)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	
	b.Run("Transaction", func(b *testing.B) {
		ctx := context.Background()
		for i := 0; i < b.N; i++ {
			tx, err := store.BeginTx(ctx, nil)
			if err != nil {
				b.Fatal(err)
			}
			
			hash := uuid.New().String()
			err = tx.PutObject(hash, testData)
			if err != nil {
				b.Fatal(err)
			}
			
			err = tx.Commit()
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}