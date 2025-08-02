package storage

import (
	"errors"
	"testing"

	"github.com/caiatech/govc/pkg/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test edge cases for storage interfaces

func TestObjectStoreEdgeCases(t *testing.T) {
	store := NewMemoryObjectStore()
	defer store.Close()

	t.Run("Empty hash", func(t *testing.T) {
		obj, err := store.Get("")
		assert.Error(t, err)
		assert.Nil(t, obj)
	})

	t.Run("Very long hash", func(t *testing.T) {
		longHash := string(make([]byte, 1000))
		obj, err := store.Get(longHash)
		assert.Error(t, err)
		assert.Nil(t, obj)
	})

	t.Run("Store empty blob", func(t *testing.T) {
		emptyBlob := object.NewBlob([]byte{})
		hash, err := store.Put(emptyBlob)
		require.NoError(t, err)
		assert.NotEmpty(t, hash)

		// Should be able to retrieve it
		retrieved, err := store.Get(hash)
		require.NoError(t, err)
		blob, ok := retrieved.(*object.Blob)
		require.True(t, ok)
		assert.Empty(t, blob.Content)
	})

	t.Run("Store large blob", func(t *testing.T) {
		// Create a 1MB blob
		largeContent := make([]byte, 1024*1024)
		for i := range largeContent {
			largeContent[i] = byte(i % 256)
		}
		largeBlob := object.NewBlob(largeContent)

		hash, err := store.Put(largeBlob)
		require.NoError(t, err)

		retrieved, err := store.Get(hash)
		require.NoError(t, err)
		assert.Equal(t, largeBlob, retrieved)
	})

	t.Run("List when empty", func(t *testing.T) {
		emptyStore := NewMemoryObjectStore()
		defer emptyStore.Close()

		hashes, err := emptyStore.List()
		require.NoError(t, err)
		assert.Empty(t, hashes)
	})

	t.Run("Size when empty", func(t *testing.T) {
		emptyStore := NewMemoryObjectStore()
		defer emptyStore.Close()

		size, err := emptyStore.Size()
		require.NoError(t, err)
		assert.Equal(t, int64(0), size)
	})
}

func TestRefStoreEdgeCases(t *testing.T) {
	store := NewMemoryRefStore()
	defer store.Close()

	t.Run("Empty ref name", func(t *testing.T) {
		err := store.UpdateRef("", "abc123")
		// Should fail - empty ref names are invalid
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reference name cannot be empty")

		_, err = store.GetRef("")
		// Getting empty ref should also fail
		assert.Error(t, err)
	})

	t.Run("Special characters in ref name", func(t *testing.T) {
		specialRefs := []string{
			"refs/heads/feature/ABC-123",
			"refs/heads/user@example.com",
			"refs/heads/release-1.0",
			"refs/heads/feature_branch",
		}

		for _, refName := range specialRefs {
			err := store.UpdateRef(refName, "hash123")
			require.NoError(t, err, "Failed to update ref: %s", refName)

			hash, err := store.GetRef(refName)
			require.NoError(t, err, "Failed to get ref: %s", refName)
			assert.Equal(t, "hash123", hash)
		}
	})

	t.Run("Update ref multiple times", func(t *testing.T) {
		refName := "refs/heads/evolving"

		// Update multiple times
		for i := 0; i < 10; i++ {
			hash := string(rune('a' + i))
			err := store.UpdateRef(refName, hash)
			require.NoError(t, err)

			retrieved, err := store.GetRef(refName)
			require.NoError(t, err)
			assert.Equal(t, hash, retrieved)
		}
	})

	t.Run("Delete non-existent ref", func(t *testing.T) {
		err := store.DeleteRef("refs/heads/doesnotexist")
		// Should succeed - idempotent operation
		assert.NoError(t, err)
	})

	t.Run("HEAD not set initially", func(t *testing.T) {
		freshStore := NewMemoryRefStore()
		defer freshStore.Close()

		head, err := freshStore.GetHEAD()
		// HEAD should default to refs/heads/main
		require.NoError(t, err)
		assert.Equal(t, "refs/heads/main", head)
	})

	t.Run("Set HEAD to empty", func(t *testing.T) {
		err := store.SetHEAD("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HEAD target cannot be empty")
	})
}

func TestWorkingStorageEdgeCases(t *testing.T) {
	store := NewMemoryWorkingStorage()

	t.Run("Path operations", func(t *testing.T) {
		content := []byte("test content")

		// Write and read
		err := store.Write("file.txt", content)
		require.NoError(t, err)

		retrieved, err := store.Read("file.txt")
		require.NoError(t, err)
		assert.Equal(t, content, retrieved)
	})

	t.Run("Deep directory structure", func(t *testing.T) {
		deepPath := "a/b/c/d/e/f/g/h/i/j/k/file.txt"
		content := []byte("deep file")

		err := store.Write(deepPath, content)
		require.NoError(t, err)

		retrieved, err := store.Read(deepPath)
		require.NoError(t, err)
		assert.Equal(t, content, retrieved)
	})

	t.Run("Delete operations", func(t *testing.T) {
		// Create files
		store.Write("dir/file1.txt", []byte("content1"))
		store.Write("dir/file2.txt", []byte("content2"))

		// Delete one file
		err := store.Delete("dir/file1.txt")
		assert.NoError(t, err)

		// file1 should be gone, file2 should remain
		_, err = store.Read("dir/file1.txt")
		assert.Error(t, err)

		content, err := store.Read("dir/file2.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("content2"), content)
	})

	t.Run("List files", func(t *testing.T) {
		// Clear and add files
		store.Clear()
		store.Write("test.txt", []byte("1"))
		store.Write("test.md", []byte("2"))
		store.Write("dir/test.txt", []byte("3"))

		files, err := store.List()
		require.NoError(t, err)
		assert.Len(t, files, 3)
		assert.Contains(t, files, "test.txt")
		assert.Contains(t, files, "test.md")
		assert.Contains(t, files, "dir/test.txt")
	})

	t.Run("Clear operation", func(t *testing.T) {
		// Add files
		store.Write("file1.txt", []byte("1"))
		store.Write("file2.txt", []byte("2"))

		// Clear
		err := store.Clear()
		require.NoError(t, err)

		// Should be empty
		files, err := store.List()
		require.NoError(t, err)
		assert.Empty(t, files)
	})
}

// Test error conditions
type FailingObjectStore struct{}

func (f *FailingObjectStore) Get(hash string) (object.Object, error) {
	return nil, errors.New("get failed")
}

func (f *FailingObjectStore) Put(obj object.Object) (string, error) {
	return "", errors.New("put failed")
}

func (f *FailingObjectStore) Exists(hash string) bool {
	return false
}

func (f *FailingObjectStore) List() ([]string, error) {
	return nil, errors.New("list failed")
}

func (f *FailingObjectStore) Size() (int64, error) {
	return 0, errors.New("size failed")
}

func (f *FailingObjectStore) Close() error {
	return errors.New("close failed")
}

func TestErrorPropagation(t *testing.T) {
	failStore := &FailingObjectStore{}

	t.Run("Get error", func(t *testing.T) {
		obj, err := failStore.Get("any")
		assert.Error(t, err)
		assert.Nil(t, obj)
		assert.Contains(t, err.Error(), "get failed")
	})

	t.Run("Put error", func(t *testing.T) {
		blob := object.NewBlob([]byte("test"))
		hash, err := failStore.Put(blob)
		assert.Error(t, err)
		assert.Empty(t, hash)
		assert.Contains(t, err.Error(), "put failed")
	})

	t.Run("List error", func(t *testing.T) {
		hashes, err := failStore.List()
		assert.Error(t, err)
		assert.Nil(t, hashes)
		assert.Contains(t, err.Error(), "list failed")
	})

	t.Run("Size error", func(t *testing.T) {
		size, err := failStore.Size()
		assert.Error(t, err)
		assert.Equal(t, int64(0), size)
		assert.Contains(t, err.Error(), "size failed")
	})

	t.Run("Close error", func(t *testing.T) {
		err := failStore.Close()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "close failed")
	})
}
