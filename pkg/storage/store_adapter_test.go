package storage

import (
	"testing"

	"github.com/caiatech/govc/pkg/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreAdapter(t *testing.T) {
	// Use a real memory store
	backend := NewMemoryBackend()
	store := NewStore(backend)
	adapter := NewStoreAdapter(store)

	t.Run("Put and Get Blob", func(t *testing.T) {
		blob := object.NewBlob([]byte("test content"))

		hash, err := adapter.Put(blob)
		require.NoError(t, err)
		assert.NotEmpty(t, hash)
		assert.Equal(t, blob.Hash(), hash)

		retrieved, err := adapter.Get(hash)
		require.NoError(t, err)
		assert.Equal(t, blob, retrieved)
	})

	t.Run("Put nil object", func(t *testing.T) {
		hash, err := adapter.Put(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot store nil object")
		assert.Empty(t, hash)
	})

	t.Run("Get non-existent object", func(t *testing.T) {
		obj, err := adapter.Get("nonexistent")
		assert.Error(t, err)
		assert.Nil(t, obj)
	})

	t.Run("Exists", func(t *testing.T) {
		blob := object.NewBlob([]byte("exists test"))
		hash, _ := adapter.Put(blob)

		assert.True(t, adapter.Exists(hash))
		assert.False(t, adapter.Exists("nonexistent"))
	})

	t.Run("List", func(t *testing.T) {
		// Start with a fresh adapter
		backend := NewMemoryBackend()
		store := NewStore(backend)
		adapter = NewStoreAdapter(store)

		// Add multiple objects
		blob1 := object.NewBlob([]byte("blob1"))
		blob2 := object.NewBlob([]byte("blob2"))
		tree := object.NewTree()
		tree.AddEntry("100644", "test.txt", blob1.Hash())

		hash1, _ := adapter.Put(blob1)
		hash2, _ := adapter.Put(blob2)
		hash3, _ := adapter.Put(tree)

		hashes, err := adapter.List()
		require.NoError(t, err)
		assert.Len(t, hashes, 3)
		assert.Contains(t, hashes, hash1)
		assert.Contains(t, hashes, hash2)
		assert.Contains(t, hashes, hash3)
	})

	t.Run("Size", func(t *testing.T) {
		// Start with a fresh adapter
		backend := NewMemoryBackend()
		store := NewStore(backend)
		adapter = NewStoreAdapter(store)

		// Add objects with known sizes
		blob1 := object.NewBlob([]byte("hello"))    // 5 bytes content
		blob2 := object.NewBlob([]byte("world!!!")) // 8 bytes content

		adapter.Put(blob1)
		adapter.Put(blob2)

		size, err := adapter.Size()
		require.NoError(t, err)
		// Size should be greater than 0
		assert.Greater(t, size, int64(0))
		// The actual size depends on implementation details (compression, headers, etc)
		// Just verify it's reasonable
		assert.Less(t, size, int64(1000)) // Should be less than 1KB for these small blobs
	})

	t.Run("Close", func(t *testing.T) {
		err := adapter.Close()
		assert.NoError(t, err)
	})
}

func TestMemoryObjectStore(t *testing.T) {
	// Test the memory object store directly
	objStore := NewMemoryObjectStore()

	assert.NotNil(t, objStore)

	// Verify it works correctly
	blob := object.NewBlob([]byte("factory test"))
	hash, err := objStore.Put(blob)
	require.NoError(t, err)

	retrieved, err := objStore.Get(hash)
	require.NoError(t, err)
	assert.Equal(t, blob, retrieved)
}

func TestStoreAdapterWithDifferentObjectTypes(t *testing.T) {
	backend := NewMemoryBackend()
	store := NewStore(backend)
	adapter := NewStoreAdapter(store)

	t.Run("Tree object", func(t *testing.T) {
		tree := object.NewTree()
		// Use proper 40-character SHA1 hashes
		tree.AddEntry("100644", "file1.txt", "95d09f2b10159347eece71399a7e2e907ea3df4f")
		tree.AddEntry("040000", "subdir", "4b825dc642cb6eb9a060e54bf8d69288fbee4904")

		hash, err := adapter.Put(tree)
		require.NoError(t, err)
		assert.NotEmpty(t, hash)

		retrieved, err := adapter.Get(hash)
		require.NoError(t, err)

		retrievedTree, ok := retrieved.(*object.Tree)
		require.True(t, ok)
		assert.Len(t, retrievedTree.Entries, 2)
		// Tree entries might be sorted, check both exist
		var foundFile, foundDir bool
		for _, entry := range retrievedTree.Entries {
			if entry.Name == "file1.txt" {
				foundFile = true
				assert.Equal(t, "100644", entry.Mode)
			}
			if entry.Name == "subdir" {
				foundDir = true
				assert.Equal(t, "040000", entry.Mode)
			}
		}
		assert.True(t, foundFile, "file1.txt not found in tree")
		assert.True(t, foundDir, "subdir not found in tree")
	})

	t.Run("Commit object", func(t *testing.T) {
		author := object.Author{
			Name:  "Test User",
			Email: "test@example.com",
		}
		commit := object.NewCommit("tree123", author, "Test commit")
		commit.SetParent("parent456")

		hash, err := adapter.Put(commit)
		require.NoError(t, err)
		assert.NotEmpty(t, hash)

		retrieved, err := adapter.Get(hash)
		require.NoError(t, err)

		retrievedCommit, ok := retrieved.(*object.Commit)
		require.True(t, ok)
		assert.Equal(t, "Test commit", retrievedCommit.Message)
		assert.Equal(t, "tree123", retrievedCommit.TreeHash)
		assert.Equal(t, "parent456", retrievedCommit.ParentHash)
	})

	t.Run("Tag object", func(t *testing.T) {
		tagger := object.Author{
			Name:  "Tagger",
			Email: "tagger@example.com",
		}
		tag := object.NewTag("commit789", object.TypeCommit, "v1.0.0", tagger, "Release 1.0.0")

		hash, err := adapter.Put(tag)
		require.NoError(t, err)
		assert.NotEmpty(t, hash)

		retrieved, err := adapter.Get(hash)
		require.NoError(t, err)

		retrievedTag, ok := retrieved.(*object.Tag)
		require.True(t, ok)
		assert.Equal(t, "v1.0.0", retrievedTag.TagName)
		assert.Equal(t, "Release 1.0.0", retrievedTag.Message)
		assert.Equal(t, "commit789", retrievedTag.ObjectHash)
	})
}

func TestStoreAdapterConcurrency(t *testing.T) {
	backend := NewMemoryBackend()
	store := NewStore(backend)
	adapter := NewStoreAdapter(store)

	// Test concurrent operations
	done := make(chan bool)

	// Multiple writers
	for i := 0; i < 10; i++ {
		go func(n int) {
			blob := object.NewBlob([]byte(string(rune('a' + n))))
			_, err := adapter.Put(blob)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for writers
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all objects were stored
	hashes, _ := adapter.List()
	assert.Len(t, hashes, 10)

	// Multiple readers
	for _, hash := range hashes {
		go func(h string) {
			obj, err := adapter.Get(h)
			assert.NoError(t, err)
			assert.NotNil(t, obj)
			done <- true
		}(hash)
	}

	// Wait for readers
	for i := 0; i < len(hashes); i++ {
		<-done
	}
}

func TestStoreAdapterErrorCases(t *testing.T) {
	backend := NewMemoryBackend()
	store := NewStore(backend)
	adapter := NewStoreAdapter(store)

	t.Run("Invalid object hash format", func(t *testing.T) {
		// Try to get an object with invalid hash
		obj, err := adapter.Get("")
		assert.Error(t, err)
		assert.Nil(t, obj)
	})

	t.Run("Size calculation with mixed objects", func(t *testing.T) {
		// Add different types of objects
		blob := object.NewBlob([]byte("test blob"))
		tree := object.NewTree()
		tree.AddEntry("100644", "file.txt", "abc123")

		author := object.Author{Name: "Test", Email: "test@example.com"}
		commit := object.NewCommit("tree123", author, "Test commit")

		adapter.Put(blob)
		adapter.Put(tree)
		adapter.Put(commit)

		size, err := adapter.Size()
		require.NoError(t, err)
		assert.Greater(t, size, int64(0))
	})
}
