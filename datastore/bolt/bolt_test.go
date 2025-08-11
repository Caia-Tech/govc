package bolt

import (
	"path/filepath"
	"testing"

	"github.com/Caia-Tech/govc/datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoltStore(t *testing.T) {
	tempDir := t.TempDir()
	config := datastore.Config{
		Type:       datastore.TypeBolt,
		Connection: filepath.Join(tempDir, "test.db"),
	}

	store, err := New(config)
	require.NoError(t, err)
	defer store.Close()

	err = store.Initialize(config)
	require.NoError(t, err)

	t.Run("ObjectStore", func(t *testing.T) {
		t.Run("SingleObject", func(t *testing.T) {
			hash := "test-hash-123"
			data := []byte("test data")

			// Store object
			err := store.ObjectStore().PutObject(hash, data)
			require.NoError(t, err)

			// Check if exists
			exists, err := store.ObjectStore().HasObject(hash)
			require.NoError(t, err)
			assert.True(t, exists)

			// Retrieve object
			retrieved, err := store.ObjectStore().GetObject(hash)
			require.NoError(t, err)
			assert.Equal(t, data, retrieved)

			// Get size
			size, err := store.ObjectStore().GetObjectSize(hash)
			require.NoError(t, err)
			assert.Equal(t, int64(len(data)), size)

			// Delete object
			err = store.ObjectStore().DeleteObject(hash)
			require.NoError(t, err)

			// Check no longer exists
			exists, err = store.ObjectStore().HasObject(hash)
			require.NoError(t, err)
			assert.False(t, exists)
		})

		t.Run("BatchOperations", func(t *testing.T) {
			objects := map[string][]byte{
				"batch1": []byte("data1"),
				"batch2": []byte("data2"),
				"batch3": []byte("data3"),
			}

			// Store multiple objects
			err := store.ObjectStore().PutObjects(objects)
			require.NoError(t, err)

			// Retrieve multiple objects
			retrieved, err := store.ObjectStore().GetObjects([]string{"batch1", "batch2", "batch3"})
			require.NoError(t, err)
			assert.Equal(t, objects, retrieved)

			// List objects
			hashes, err := store.ObjectStore().ListObjects("batch", 0)
			require.NoError(t, err)
			assert.Len(t, hashes, 3)

			// Count objects
			count, err := store.ObjectStore().CountObjects()
			require.NoError(t, err)
			assert.Equal(t, int64(3), count)

			// Delete all
			err = store.ObjectStore().DeleteObjects([]string{"batch1", "batch2", "batch3"})
			require.NoError(t, err)

			// Verify deletion
			count, err = store.ObjectStore().CountObjects()
			require.NoError(t, err)
			assert.Equal(t, int64(0), count)
		})
	})

	t.Run("MetadataStore", func(t *testing.T) {
		t.Run("Repository", func(t *testing.T) {
			repo := &datastore.Repository{
				Name:        "test-repo",
				Owner:       "test-owner",
				Description: "Test repository",
				Status:      "active",
			}

			// Create repository
			err := store.MetadataStore().CreateRepository(repo)
			require.NoError(t, err)

			// Get repository
			retrieved, err := store.MetadataStore().GetRepository("test-repo")
			require.NoError(t, err)
			assert.Equal(t, repo.Name, retrieved.Name)
			assert.Equal(t, repo.Owner, retrieved.Owner)

			// Update repository
			repo.Description = "Updated description"
			err = store.MetadataStore().UpdateRepository(repo)
			require.NoError(t, err)

			// Verify update
			retrieved, err = store.MetadataStore().GetRepository("test-repo")
			require.NoError(t, err)
			assert.Equal(t, "Updated description", retrieved.Description)

			// Delete repository
			err = store.MetadataStore().DeleteRepository("test-repo")
			require.NoError(t, err)

			// Verify deletion
			_, err = store.MetadataStore().GetRepository("test-repo")
			assert.ErrorIs(t, err, datastore.ErrNotFound)
		})
	})

	t.Run("Metrics", func(t *testing.T) {
		metrics := store.GetMetrics()
		assert.Equal(t, datastore.TypeBolt, store.Type())
		assert.NotZero(t, metrics.StartTime)
		assert.NotZero(t, metrics.Uptime)
	})
}

func TestBoltStoreHealthCheck(t *testing.T) {
	tempDir := t.TempDir()
	config := datastore.Config{
		Type:       datastore.TypeBolt,
		Connection: filepath.Join(tempDir, "health_test.db"),
	}

	store, err := New(config)
	require.NoError(t, err)
	defer store.Close()

	err = store.Initialize(config)
	require.NoError(t, err)

	// Health check should pass
	err = store.HealthCheck(nil)
	assert.NoError(t, err)
}