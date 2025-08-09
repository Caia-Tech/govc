package mongodb

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caiatech/govc/datastore"
)

func TestMain(m *testing.M) {
	// Skip tests if MongoDB is not available
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		// MongoDB not available, skip all tests
		os.Exit(0)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	if err := client.Ping(ctx, nil); err != nil {
		client.Disconnect(context.Background())
		// MongoDB not available, skip all tests
		os.Exit(0)
	}
	client.Disconnect(context.Background())
	
	os.Exit(m.Run())
}

func setupTestMongoStore(t *testing.T) (*MongoStore, func()) {
	config := datastore.Config{
		Type:       datastore.TypeMongoDB,
		Connection: "mongodb://localhost:27017/govc_test", // Use test database
	}

	store, err := New(config)
	require.NoError(t, err)
	require.NotNil(t, store)

	err = store.Initialize(config)
	require.NoError(t, err)

	// Clean up function
	cleanup := func() {
		// Drop test database
		if store.database != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			store.database.Drop(ctx)
		}
		store.Close()
	}

	return store, cleanup
}

func TestMongoStore_Initialize(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
	defer cleanup()

	assert.Equal(t, datastore.TypeMongoDB, store.Type())
	
	// Test health check
	ctx := context.Background()
	err := store.HealthCheck(ctx)
	assert.NoError(t, err)
}

func TestMongoStore_ObjectStore_BasicOperations(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
	defer cleanup()

	objStore := store.ObjectStore()
	require.NotNil(t, objStore)

	// Test data
	hash := "test-hash-mongodb-123"
	data := []byte("test data for mongodb storage with some content")

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

func TestMongoStore_ObjectStore_BatchOperations(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
	defer cleanup()

	objStore := store.ObjectStore()

	// Test data
	objects := map[string][]byte{
		"mongo-hash1": []byte("mongodb data one"),
		"mongo-hash2": []byte("mongodb data two with more content"),
		"mongo-hash3": []byte("mongodb data three for testing"),
	}

	// Test PutObjects
	err := objStore.PutObjects(objects)
	assert.NoError(t, err)

	// Test GetObjects
	retrieved, err := objStore.GetObjects([]string{"mongo-hash1", "mongo-hash2", "mongo-hash3"})
	assert.NoError(t, err)
	assert.Len(t, retrieved, 3)
	assert.Equal(t, objects["mongo-hash1"], retrieved["mongo-hash1"])
	assert.Equal(t, objects["mongo-hash2"], retrieved["mongo-hash2"])
	assert.Equal(t, objects["mongo-hash3"], retrieved["mongo-hash3"])

	// Test DeleteObjects
	err = objStore.DeleteObjects([]string{"mongo-hash1", "mongo-hash3"})
	assert.NoError(t, err)

	// Verify partial deletion
	retrieved, err = objStore.GetObjects([]string{"mongo-hash1", "mongo-hash2", "mongo-hash3"})
	assert.NoError(t, err)
	assert.Len(t, retrieved, 1) // Only hash2 should remain
	assert.Equal(t, objects["mongo-hash2"], retrieved["mongo-hash2"])
}

func TestMongoStore_ObjectStore_ListOperations(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
	defer cleanup()

	objStore := store.ObjectStore()

	// Test data with different prefixes
	testData := map[string][]byte{
		"mongo-prefix1-hash1": []byte("mongo data 1"),
		"mongo-prefix1-hash2": []byte("mongo data 2"),
		"mongo-prefix2-hash1": []byte("mongo data 3"),
		"mongo-other-hash":    []byte("mongo data 4"),
	}

	// Put all test data
	err := objStore.PutObjects(testData)
	assert.NoError(t, err)

	// Wait for consistency
	time.Sleep(100 * time.Millisecond)

	// Test ListObjects with prefix
	hashes, err := objStore.ListObjects("mongo-prefix1-", 0)
	assert.NoError(t, err)
	assert.Len(t, hashes, 2)
	assert.Contains(t, hashes, "mongo-prefix1-hash1")
	assert.Contains(t, hashes, "mongo-prefix1-hash2")

	// Test ListObjects with limit
	hashes, err = objStore.ListObjects("", 2)
	assert.NoError(t, err)
	assert.LessOrEqual(t, len(hashes), 2)

	// Test CountObjects
	count, err := objStore.CountObjects()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(4))

	// Test GetStorageSize
	size, err := objStore.GetStorageSize()
	assert.NoError(t, err)
	assert.Greater(t, size, int64(0))
}

func TestMongoStore_ObjectStore_IterateObjects(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
	defer cleanup()

	objStore := store.ObjectStore()

	// Test data
	testData := map[string][]byte{
		"iter-mongo-1": []byte("iteration data 1 for mongodb"),
		"iter-mongo-2": []byte("iteration data 2 for mongodb"),
		"iter-mongo-3": []byte("iteration data 3 for mongodb"),
	}

	err := objStore.PutObjects(testData)
	assert.NoError(t, err)

	// Wait for consistency
	time.Sleep(100 * time.Millisecond)

	// Test iteration
	visited := make(map[string][]byte)
	err = objStore.IterateObjects("iter-mongo-", func(hash string, data []byte) error {
		visited[hash] = data
		return nil
	})

	assert.NoError(t, err)
	assert.Len(t, visited, 3)
	assert.Equal(t, testData["iter-mongo-1"], visited["iter-mongo-1"])
	assert.Equal(t, testData["iter-mongo-2"], visited["iter-mongo-2"])
	assert.Equal(t, testData["iter-mongo-3"], visited["iter-mongo-3"])
}

func TestMongoStore_MetadataStore_Repositories(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
	defer cleanup()

	metaStore := store.MetadataStore()
	require.NotNil(t, metaStore)

	// Test repository
	repo := &datastore.Repository{
		ID:          "test-repo-mongodb",
		Name:        "Test MongoDB Repository",
		Description: "A test repository for MongoDB adapter",
		Path:        "/test/path",
		IsPrivate:   false,
		Size:        1024,
		CommitCount: 42,
		BranchCount: 3,
		Metadata: map[string]interface{}{
			"test_field": "test_value",
		},
	}

	// Test SaveRepository
	err := metaStore.SaveRepository(repo)
	assert.NoError(t, err)

	// Test GetRepository
	retrieved, err := metaStore.GetRepository(repo.ID)
	assert.NoError(t, err)
	assert.Equal(t, repo.ID, retrieved.ID)
	assert.Equal(t, repo.Name, retrieved.Name)
	assert.Equal(t, repo.Description, retrieved.Description)

	// Test ListRepositories
	repos, err := metaStore.ListRepositories(datastore.RepositoryFilter{})
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(repos), 1)

	// Test UpdateRepository
	updates := map[string]interface{}{
		"description": "Updated description",
		"size":        2048,
	}
	err = metaStore.UpdateRepository(repo.ID, updates)
	assert.NoError(t, err)

	// Verify update
	updated, err := metaStore.GetRepository(repo.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated description", updated.Description)
	assert.Equal(t, int64(2048), updated.Size)

	// Test DeleteRepository
	err = metaStore.DeleteRepository(repo.ID)
	assert.NoError(t, err)

	// Verify deletion
	_, err = metaStore.GetRepository(repo.ID)
	assert.Equal(t, datastore.ErrNotFound, err)
}

func TestMongoStore_MetadataStore_Users(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
	defer cleanup()

	metaStore := store.MetadataStore()

	// Test user
	user := &datastore.User{
		ID:       "test-user-mongodb",
		Username: "testuser",
		Email:    "test@example.com",
		FullName: "Test User",
		IsActive: true,
		IsAdmin:  false,
		Metadata: map[string]interface{}{
			"department": "engineering",
		},
	}

	// Test SaveUser
	err := metaStore.SaveUser(user)
	assert.NoError(t, err)

	// Test GetUser
	retrieved, err := metaStore.GetUser(user.ID)
	assert.NoError(t, err)
	assert.Equal(t, user.ID, retrieved.ID)
	assert.Equal(t, user.Username, retrieved.Username)
	assert.Equal(t, user.Email, retrieved.Email)

	// Test GetUserByUsername
	byUsername, err := metaStore.GetUserByUsername(user.Username)
	assert.NoError(t, err)
	assert.Equal(t, user.ID, byUsername.ID)

	// Test ListUsers
	users, err := metaStore.ListUsers(datastore.UserFilter{})
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(users), 1)

	// Test UpdateUser
	updates := map[string]interface{}{
		"full_name": "Updated User",
		"is_admin":  true,
	}
	err = metaStore.UpdateUser(user.ID, updates)
	assert.NoError(t, err)

	// Test DeleteUser
	err = metaStore.DeleteUser(user.ID)
	assert.NoError(t, err)

	// Verify deletion
	_, err = metaStore.GetUser(user.ID)
	assert.Equal(t, datastore.ErrNotFound, err)
}

func TestMongoStore_MetadataStore_References(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
	defer cleanup()

	metaStore := store.MetadataStore()

	repoID := "test-repo-refs"
	ref := &datastore.Reference{
		Name:      "refs/heads/main",
		Hash:      "abc123def456",
		Type:      datastore.RefTypeBranch,
		UpdatedBy: "test-user",
	}

	// Test SaveRef
	err := metaStore.SaveRef(repoID, ref)
	assert.NoError(t, err)

	// Test GetRef
	retrieved, err := metaStore.GetRef(repoID, ref.Name)
	assert.NoError(t, err)
	assert.Equal(t, ref.Name, retrieved.Name)
	assert.Equal(t, ref.Hash, retrieved.Hash)
	assert.Equal(t, ref.Type, retrieved.Type)

	// Test ListRefs
	refs, err := metaStore.ListRefs(repoID, "")
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(refs), 1)

	// Test UpdateRef
	newHash := "def456abc123"
	err = metaStore.UpdateRef(repoID, ref.Name, newHash)
	assert.NoError(t, err)

	// Verify update
	updated, err := metaStore.GetRef(repoID, ref.Name)
	assert.NoError(t, err)
	assert.Equal(t, newHash, updated.Hash)

	// Test DeleteRef
	err = metaStore.DeleteRef(repoID, ref.Name)
	assert.NoError(t, err)

	// Verify deletion
	_, err = metaStore.GetRef(repoID, ref.Name)
	assert.Equal(t, datastore.ErrNotFound, err)
}

func TestMongoStore_MetadataStore_Configuration(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
	defer cleanup()

	metaStore := store.MetadataStore()

	key := "test.config.key"
	value := "test config value"

	// Test SetConfig
	err := metaStore.SetConfig(key, value)
	assert.NoError(t, err)

	// Test GetConfig
	retrieved, err := metaStore.GetConfig(key)
	assert.NoError(t, err)
	assert.Equal(t, value, retrieved)

	// Test GetAllConfig
	allConfig, err := metaStore.GetAllConfig()
	assert.NoError(t, err)
	assert.Contains(t, allConfig, key)
	assert.Equal(t, value, allConfig[key])

	// Test DeleteConfig
	err = metaStore.DeleteConfig(key)
	assert.NoError(t, err)

	// Verify deletion
	_, err = metaStore.GetConfig(key)
	assert.Equal(t, datastore.ErrNotFound, err)
}

func TestMongoStore_Transaction_Basic(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
	defer cleanup()

	ctx := context.Background()
	
	// Begin transaction
	tx, err := store.BeginTx(ctx, &datastore.TxOptions{})
	assert.NoError(t, err)
	require.NotNil(t, tx)

	// Test transactional operations
	err = tx.PutObject("tx-mongo-hash", []byte("transactional data"))
	assert.NoError(t, err)

	// Test commit
	err = tx.Commit()
	assert.NoError(t, err)

	// After commit, we should see the data in the store
	data, err := store.ObjectStore().GetObject("tx-mongo-hash")
	assert.NoError(t, err)
	assert.Equal(t, []byte("transactional data"), data)
}

func TestMongoStore_Transaction_Rollback(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
	defer cleanup()

	ctx := context.Background()

	// Begin transaction
	tx, err := store.BeginTx(ctx, &datastore.TxOptions{})
	assert.NoError(t, err)

	// Add some data
	err = tx.PutObject("rollback-mongo-hash", []byte("rollback data"))
	assert.NoError(t, err)

	// Rollback
	err = tx.Rollback()
	assert.NoError(t, err)

	// Data should not exist
	_, err = store.ObjectStore().GetObject("rollback-mongo-hash")
	assert.Equal(t, datastore.ErrNotFound, err)
}

func TestMongoStore_Metrics(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
	defer cleanup()

	metrics := store.GetMetrics()
	assert.Greater(t, metrics.Uptime, time.Duration(0))
	assert.GreaterOrEqual(t, metrics.ActiveConnections, 0)
}

func TestMongoStore_Info(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
	defer cleanup()

	info := store.Info()
	assert.Equal(t, datastore.TypeMongoDB, info["type"])
	assert.Contains(t, info, "connection")
	assert.Equal(t, "healthy", info["status"])
	assert.Contains(t, info, "database")
}

func TestMongoStore_InvalidOperations(t *testing.T) {
	store, cleanup := setupTestMongoStore(t)
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

func TestMongoStore_Close(t *testing.T) {
	config := datastore.Config{
		Type:       datastore.TypeMongoDB,
		Connection: "mongodb://localhost:27017/govc_test",
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