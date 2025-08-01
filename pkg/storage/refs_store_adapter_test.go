package storage

import (
	"testing"

	"github.com/caiatech/govc/pkg/refs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefManagerAdapter(t *testing.T) {
	// Create a real RefManager with memory ref store
	memRefStore := refs.NewMemoryRefStore()
	refManager := refs.NewRefManager(memRefStore)
	adapter := NewRefManagerAdapter(refManager)

	t.Run("Basic ref operations", func(t *testing.T) {
		// Update a ref
		err := adapter.UpdateRef("refs/heads/main", "abc123")
		require.NoError(t, err)

		// Get the ref
		hash, err := adapter.GetRef("refs/heads/main")
		require.NoError(t, err)
		assert.Equal(t, "abc123", hash)

		// Delete the ref
		err = adapter.DeleteRef("refs/heads/main")
		require.NoError(t, err)

		// Verify it's deleted
		_, err = adapter.GetRef("refs/heads/main")
		assert.Error(t, err)
	})

	t.Run("HEAD operations", func(t *testing.T) {
		// Set HEAD to a branch
		err := adapter.SetHEAD("refs/heads/main")
		require.NoError(t, err)

		// Get HEAD
		head, err := adapter.GetHEAD()
		require.NoError(t, err)
		assert.Equal(t, "refs/heads/main", head)

		// Set HEAD to a commit
		err = adapter.SetHEAD("xyz789")
		require.NoError(t, err)

		head, err = adapter.GetHEAD()
		require.NoError(t, err)
		assert.Equal(t, "xyz789", head)
	})

	t.Run("List refs", func(t *testing.T) {
		// Create multiple refs
		adapter.UpdateRef("refs/heads/main", "aaa111")
		adapter.UpdateRef("refs/heads/feature", "bbb222")
		adapter.UpdateRef("refs/tags/v1.0", "ccc333")

		refs, err := adapter.ListRefs()
		require.NoError(t, err)
		assert.Greater(t, len(refs), 0)
		
		// Check if our refs are in the list
		assert.Equal(t, "aaa111", refs["refs/heads/main"])
		assert.Equal(t, "bbb222", refs["refs/heads/feature"])
		assert.Equal(t, "ccc333", refs["refs/tags/v1.0"])
	})

	t.Run("Close", func(t *testing.T) {
		err := adapter.Close()
		assert.NoError(t, err)
	})
}

func TestMemoryRefStore(t *testing.T) {
	refStore := NewMemoryRefStore()
	defer refStore.Close()

	t.Run("Basic operations", func(t *testing.T) {
		// Update ref
		err := refStore.UpdateRef("refs/heads/main", "abc123")
		require.NoError(t, err)

		// Get ref
		hash, err := refStore.GetRef("refs/heads/main")
		require.NoError(t, err)
		assert.Equal(t, "abc123", hash)

		// Delete ref
		err = refStore.DeleteRef("refs/heads/main")
		require.NoError(t, err)

		// Verify deletion
		_, err = refStore.GetRef("refs/heads/main")
		assert.Error(t, err)
	})

	t.Run("HEAD operations", func(t *testing.T) {
		// Set HEAD
		err := refStore.SetHEAD("refs/heads/main")
		require.NoError(t, err)

		// Get HEAD
		head, err := refStore.GetHEAD()
		require.NoError(t, err)
		assert.Equal(t, "refs/heads/main", head)
	})

	t.Run("List refs", func(t *testing.T) {
		// Add refs
		refStore.UpdateRef("refs/heads/main", "111")
		refStore.UpdateRef("refs/heads/dev", "222")
		refStore.UpdateRef("refs/tags/v1", "333")

		refs, err := refStore.ListRefs()
		require.NoError(t, err)
		assert.Len(t, refs, 3)
	})

	t.Run("Concurrent access", func(t *testing.T) {
		done := make(chan bool)

		// Multiple writers
		for i := 0; i < 10; i++ {
			go func(n int) {
				refName := "refs/heads/branch" + string(rune('0'+n))
				hash := "hash" + string(rune('0'+n))
				err := refStore.UpdateRef(refName, hash)
				assert.NoError(t, err)
				done <- true
			}(i)
		}

		// Wait for writers
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify all refs
		refs, _ := refStore.ListRefs()
		assert.GreaterOrEqual(t, len(refs), 10)
	})
}