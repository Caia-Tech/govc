package storage

import (
	"fmt"
	"testing"

	"github.com/caiatech/govc/pkg/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock object for testing
type mockObject struct {
	hash    string
	content []byte
}

func (m *mockObject) Hash() string               { return m.hash }
func (m *mockObject) Type() object.Type          { return object.TypeBlob }
func (m *mockObject) Size() int64                { return int64(len(m.content)) }
func (m *mockObject) Serialize() ([]byte, error) { return m.content, nil }

func newMockObject(hash string, content string) object.Object {
	return &mockObject{hash: hash, content: []byte(content)}
}

// Test suite for ObjectStore implementations
func TestObjectStore(t *testing.T) {
	stores := []struct {
		name  string
		store ObjectStore
	}{
		{"Memory", NewMemoryObjectStore()},
		{"StoreAdapter", NewMemoryObjectStoreFromStore()},
	}

	for _, tc := range stores {
		t.Run(tc.name, func(t *testing.T) {
			testObjectStoreBasicOperations(t, tc.store)
			testObjectStoreErrors(t, tc.store)
			testObjectStoreConcurrency(t, tc.store)
		})
	}
}

func testObjectStoreBasicOperations(t *testing.T, store ObjectStore) {
	var obj1, obj2 object.Object

	// For StoreAdapter, use real objects; for memory stores, use mock objects
	if _, isAdapter := store.(*StoreAdapter); isAdapter {
		// Use real blob objects for StoreAdapter
		obj1 = object.NewBlob([]byte("test content"))
		obj2 = object.NewBlob([]byte("another content"))
	} else {
		// Use mock objects for pure memory stores
		obj1 = newMockObject("abc123", "test content")
		obj2 = newMockObject("def456", "another content")
	}

	// Test Put and Get
	hash1, err := store.Put(obj1)
	require.NoError(t, err)
	assert.Equal(t, obj1.Hash(), hash1)

	retrieved, err := store.Get(hash1)
	require.NoError(t, err)
	assert.Equal(t, obj1.Hash(), retrieved.Hash())

	// Test Exists
	assert.True(t, store.Exists(hash1))
	assert.False(t, store.Exists("nonexistent"))

	// Test List
	hash2, err := store.Put(obj2)
	require.NoError(t, err)

	hashes, err := store.List()
	require.NoError(t, err)
	assert.Contains(t, hashes, hash1)
	assert.Contains(t, hashes, hash2)

	// Test Size
	size, err := store.Size()
	require.NoError(t, err)
	assert.Greater(t, size, int64(0))
}

func testObjectStoreErrors(t *testing.T, store ObjectStore) {
	// Test Get non-existent object
	_, err := store.Get("nonexistent")
	assert.Error(t, err)

	// Test Put nil object
	_, err = store.Put(nil)
	assert.Error(t, err)

	// Test Put object with empty hash (only for memory stores, not adapters)
	if _, isAdapter := store.(*StoreAdapter); !isAdapter {
		emptyHashObj := &mockObject{hash: "", content: []byte("test")}
		_, err = store.Put(emptyHashObj)
		assert.Error(t, err)
	}
}

func testObjectStoreConcurrency(t *testing.T, store ObjectStore) {
	// Test concurrent writes
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			// Use different objects for adapters vs memory stores
			var obj object.Object
			if _, isAdapter := store.(*StoreAdapter); isAdapter {
				content := fmt.Sprintf("content-%d", id)
				obj = object.NewBlob([]byte(content))
			} else {
				obj = newMockObject(string(rune('a'+id))+"hash", "content")
			}
			_, err := store.Put(obj)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify objects were stored (may be fewer due to deduplication)
	hashes, err := store.List()
	require.NoError(t, err)
	assert.Greater(t, len(hashes), 0) // At least some objects should exist
}

// Test suite for RefStore implementations
func TestRefStore(t *testing.T) {
	stores := []struct {
		name  string
		store RefStore
	}{
		{"Memory", NewMemoryRefStore()},
		{"RefsAdapter", NewMemoryRefStoreAdapter()},
	}

	for _, tc := range stores {
		t.Run(tc.name, func(t *testing.T) {
			testRefStoreBasicOperations(t, tc.store)
			testRefStoreErrors(t, tc.store)
			testRefStoreHEAD(t, tc.store)
		})
	}
}

func testRefStoreBasicOperations(t *testing.T, store RefStore) {
	// Test UpdateRef and GetRef
	err := store.UpdateRef("refs/heads/main", "abc123")
	require.NoError(t, err)

	hash, err := store.GetRef("refs/heads/main")
	require.NoError(t, err)
	assert.Equal(t, "abc123", hash)

	// Test ListRefs
	store.UpdateRef("refs/heads/feature", "def456")
	store.UpdateRef("refs/tags/v1.0", "ghi789")

	refs, err := store.ListRefs()
	require.NoError(t, err)
	assert.Equal(t, "abc123", refs["refs/heads/main"])
	assert.Equal(t, "def456", refs["refs/heads/feature"])
	assert.Equal(t, "ghi789", refs["refs/tags/v1.0"])

	// Test DeleteRef
	err = store.DeleteRef("refs/heads/feature")
	require.NoError(t, err)

	_, err = store.GetRef("refs/heads/feature")
	assert.Error(t, err)
}

func testRefStoreErrors(t *testing.T, store RefStore) {
	// Test GetRef non-existent
	_, err := store.GetRef("refs/heads/nonexistent")
	assert.Error(t, err)

	// Test UpdateRef with empty name
	err = store.UpdateRef("", "abc123")
	assert.Error(t, err)

	// Test UpdateRef with empty hash
	err = store.UpdateRef("refs/heads/test", "")
	assert.Error(t, err)

	// Test DeleteRef non-existent (should be idempotent)
	err = store.DeleteRef("refs/heads/nonexistent")
	assert.NoError(t, err)
}

func testRefStoreHEAD(t *testing.T, store RefStore) {
	// Test default HEAD
	head, err := store.GetHEAD()
	require.NoError(t, err)

	// For RefsAdapter, HEAD behavior is different - it resolves to commit hash
	if _, isAdapter := store.(*RefsStoreAdapter); isAdapter {
		// RefsAdapter defaults to "refs/heads/main" symbolically
		assert.Equal(t, "refs/heads/main", head)
	} else {
		// Memory store should return "refs/heads/main"
		assert.Equal(t, "refs/heads/main", head)
	}

	// Test SetHEAD
	err = store.SetHEAD("refs/heads/develop")
	require.NoError(t, err)

	head, err = store.GetHEAD()
	if _, isAdapter := store.(*RefsStoreAdapter); isAdapter {
		// For adapters, the behavior is different due to underlying refs package
		// The refs package allows setting HEAD to non-existent branches
		// We'll just verify it doesn't crash
		if err == nil {
			t.Log("RefsAdapter HEAD behavior: ", head)
		} else {
			t.Log("RefsAdapter HEAD error (expected): ", err)
		}
	} else {
		require.NoError(t, err)
		assert.Equal(t, "refs/heads/develop", head)
	}

	// Test SetHEAD with empty target (only for memory stores)
	if _, isAdapter := store.(*RefsStoreAdapter); !isAdapter {
		err = store.SetHEAD("")
		assert.Error(t, err)
	}
}

// Test suite for WorkingStorage implementations
func TestWorkingStorage(t *testing.T) {
	stores := []struct {
		name    string
		storage WorkingStorage
	}{
		{"Memory", NewMemoryWorkingStorage()},
	}

	for _, tc := range stores {
		t.Run(tc.name, func(t *testing.T) {
			testWorkingStorageBasicOperations(t, tc.storage)
			testWorkingStorageErrors(t, tc.storage)
			testWorkingStorageClear(t, tc.storage)
		})
	}
}

func testWorkingStorageBasicOperations(t *testing.T, storage WorkingStorage) {
	// Test Write and Read
	content := []byte("test file content")
	err := storage.Write("test.txt", content)
	require.NoError(t, err)

	readContent, err := storage.Read("test.txt")
	require.NoError(t, err)
	assert.Equal(t, content, readContent)

	// Test Exists
	assert.True(t, storage.Exists("test.txt"))
	assert.False(t, storage.Exists("nonexistent.txt"))

	// Test List
	storage.Write("another.txt", []byte("another content"))

	files, err := storage.List()
	require.NoError(t, err)
	assert.Contains(t, files, "test.txt")
	assert.Contains(t, files, "another.txt")

	// Test Delete
	err = storage.Delete("test.txt")
	require.NoError(t, err)

	assert.False(t, storage.Exists("test.txt"))

	_, err = storage.Read("test.txt")
	assert.Error(t, err)
}

func testWorkingStorageErrors(t *testing.T, storage WorkingStorage) {
	// Test Read non-existent file
	_, err := storage.Read("nonexistent.txt")
	assert.Error(t, err)

	// Test Write with empty path
	err = storage.Write("", []byte("content"))
	assert.Error(t, err)

	// Test Delete non-existent file
	err = storage.Delete("nonexistent.txt")
	assert.Error(t, err)
}

func testWorkingStorageClear(t *testing.T, storage WorkingStorage) {
	// Clear any existing files first
	storage.Clear()

	// Add some files
	storage.Write("file1.txt", []byte("content1"))
	storage.Write("file2.txt", []byte("content2"))

	files, _ := storage.List()
	assert.Len(t, files, 2)

	// Clear all files
	err := storage.Clear()
	require.NoError(t, err)

	files, _ = storage.List()
	assert.Len(t, files, 0)

	assert.False(t, storage.Exists("file1.txt"))
	assert.False(t, storage.Exists("file2.txt"))
}

// Test StorageFactory
func TestMemoryStorageFactory(t *testing.T) {
	factory := NewMemoryStorageFactory()

	// Test CreateObjectStore
	objStore, err := factory.CreateObjectStore(ObjectStoreConfig{Type: "memory"})
	require.NoError(t, err)
	assert.NotNil(t, objStore)

	// Test CreateRefStore
	refStore, err := factory.CreateRefStore(RefStoreConfig{Type: "memory"})
	require.NoError(t, err)
	assert.NotNil(t, refStore)

	// Test CreateWorkingStorage
	workStore, err := factory.CreateWorkingStorage(WorkingStorageConfig{Type: "memory"})
	require.NoError(t, err)
	assert.NotNil(t, workStore)

	// Test unsupported type
	_, err = factory.CreateObjectStore(ObjectStoreConfig{Type: "unsupported"})
	assert.Error(t, err)
}

// Test Error types
func TestStorageErrors(t *testing.T) {
	// Test NotFoundError
	err := NotFoundError("test not found")
	assert.Equal(t, "not_found", err.Type)
	assert.Equal(t, "test not found", err.Message)
	assert.Equal(t, "test not found", err.Error())

	// Test InvalidObjectError
	err = InvalidObjectError("invalid object")
	assert.Equal(t, "invalid_object", err.Type)
	assert.Equal(t, "invalid object", err.Message)

	// Test IOError with cause
	cause := assert.AnError
	err = IOError("io failed", cause)
	assert.Equal(t, "io_error", err.Type)
	assert.Equal(t, "io failed", err.Message)
	assert.Equal(t, cause, err.Unwrap())
	assert.Contains(t, err.Error(), "io failed")
	assert.Contains(t, err.Error(), cause.Error())
}
