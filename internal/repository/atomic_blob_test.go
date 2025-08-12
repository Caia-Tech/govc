package repository

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAtomicBlobStorage tests that blobs are properly stored and retrievable
func TestAtomicBlobStorage(t *testing.T) {
	t.Run("DirectBlobVerification", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Create files using AtomicMultiFileUpdate
		files := map[string][]byte{
			"test1.txt": []byte("Content for test 1"),
			"test2.txt": []byte("Content for test 2"),
			"test3.txt": []byte("Content for test 3"),
		}

		// Perform atomic update
		commit, err := repo.AtomicMultiFileUpdate(files, "Test atomic update")
		require.NoError(t, err)
		require.NotNil(t, commit)

		// Get the tree from the commit
		tree, err := repo.store.GetTree(commit.TreeHash)
		require.NoError(t, err, "Failed to get tree from commit")

		// Verify each blob in the tree can be retrieved
		for _, entry := range tree.Entries {
			t.Logf("Checking entry: %s with hash: %s", entry.Name, entry.Hash)
			
			// Try to get the blob directly from the store
			blob, err := repo.store.GetBlob(entry.Hash)
			if err != nil {
				t.Errorf("Failed to get blob for %s (hash: %s): %v", entry.Name, entry.Hash, err)
				
				// Also try with delta-aware retrieval
				blob, err = repo.GetBlobWithDelta(entry.Hash)
				if err != nil {
					t.Errorf("Also failed with GetBlobWithDelta for %s: %v", entry.Name, err)
				} else {
					t.Logf("GetBlobWithDelta succeeded for %s", entry.Name)
					assert.Equal(t, files[entry.Name], blob.Content)
				}
			} else {
				// Verify content matches
				expectedContent, exists := files[entry.Name]
				assert.True(t, exists, "Unexpected file in tree: %s", entry.Name)
				assert.Equal(t, expectedContent, blob.Content, "Content mismatch for %s", entry.Name)
			}
		}

		// Also verify via ReadFile
		for path, expectedContent := range files {
			content, err := repo.ReadFile(path)
			require.NoError(t, err, "Failed to read file %s", path)
			assert.Equal(t, expectedContent, content)
		}
	})

	t.Run("TransactionBlobStorage", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Use transaction directly
		txn := repo.BeginTransaction()
		
		// Add files
		files := map[string][]byte{
			"txn1.txt": []byte("Transaction content 1"),
			"txn2.txt": []byte("Transaction content 2"),
		}

		for path, content := range files {
			err := txn.AtomicFileCreate(path, content)
			require.NoError(t, err)
		}

		// Prepare and commit
		err := txn.Prepare()
		require.NoError(t, err)

		commit, err := txn.Commit("Transaction commit")
		require.NoError(t, err)
		require.NotNil(t, commit)

		// Verify blobs are stored
		tree, err := repo.store.GetTree(commit.TreeHash)
		require.NoError(t, err)

		for _, entry := range tree.Entries {
			blob, err := repo.store.GetBlob(entry.Hash)
			require.NoError(t, err, "Blob not found for %s (hash: %s)", entry.Name, entry.Hash)
			
			expectedContent := files[entry.Name]
			assert.Equal(t, expectedContent, blob.Content)
		}
	})

	t.Run("BlobStorageAfterRepoRecreation", func(t *testing.T) {
		// This simulates the issue where blobs might not persist
		repo := NewRepository()
		defer repo.eventBus.Stop()

		files := map[string][]byte{
			"persist1.txt": []byte("Should persist 1"),
			"persist2.txt": []byte("Should persist 2"),
		}

		commit, err := repo.AtomicMultiFileUpdate(files, "Persistence test")
		require.NoError(t, err)
		require.NotNil(t, commit)

		commitHash := commit.Hash()
		treeHash := commit.TreeHash

		// Simulate getting the commit later (without recreating repo since it's in-memory)
		// Get the commit
		retrievedCommit, err := repo.store.GetCommit(commitHash)
		require.NoError(t, err, "Failed to retrieve commit")
		assert.Equal(t, treeHash, retrievedCommit.TreeHash)

		// Get the tree
		tree, err := repo.store.GetTree(treeHash)
		require.NoError(t, err, "Failed to retrieve tree")

		// Try to get each blob
		successCount := 0
		for _, entry := range tree.Entries {
			blob, err := repo.store.GetBlob(entry.Hash)
			if err == nil {
				successCount++
				t.Logf("Successfully retrieved blob for %s", entry.Name)
				assert.Equal(t, files[entry.Name], blob.Content)
			} else {
				t.Errorf("Failed to retrieve blob for %s (hash: %s): %v", 
					entry.Name, entry.Hash, err)
			}
		}

		assert.Equal(t, len(files), successCount, 
			"Not all blobs were retrievable. Got %d/%d", successCount, len(files))
	})

	t.Run("LargeNumberOfFiles", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Create many files
		fileCount := 50
		files := make(map[string][]byte)
		for i := 0; i < fileCount; i++ {
			path := fmt.Sprintf("file%03d.txt", i)
			content := fmt.Sprintf("Content for file %d with some extra text", i)
			files[path] = []byte(content)
		}

		// Atomic update with many files
		commit, err := repo.AtomicMultiFileUpdate(files, "Many files test")
		require.NoError(t, err)
		require.NotNil(t, commit)

		// Get tree and verify all blobs
		tree, err := repo.store.GetTree(commit.TreeHash)
		require.NoError(t, err)
		assert.Equal(t, fileCount, len(tree.Entries))

		// Verify each blob
		for _, entry := range tree.Entries {
			blob, err := repo.store.GetBlob(entry.Hash)
			require.NoError(t, err, "Failed to get blob for %s", entry.Name)
			
			expectedContent := files[entry.Name]
			assert.Equal(t, expectedContent, blob.Content)
		}

		// Also verify via ReadFile
		for path, expectedContent := range files {
			content, err := repo.ReadFile(path)
			require.NoError(t, err, "Failed to read %s", path)
			assert.Equal(t, expectedContent, content)
		}
	})
}

// TestStoreBlobWithDelta tests the StoreBlobWithDelta function directly
func TestStoreBlobWithDelta(t *testing.T) {
	t.Run("DirectStorageAndRetrieval", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		content := []byte("Test content for direct storage")
		
		// Store blob
		hash, err := repo.StoreBlobWithDelta(content)
		require.NoError(t, err)
		assert.NotEmpty(t, hash)

		// Retrieve via store.GetBlob
		blob, err := repo.store.GetBlob(hash)
		require.NoError(t, err, "Failed to retrieve blob with hash %s", hash)
		assert.Equal(t, content, blob.Content)

		// Also retrieve via GetBlobWithDelta
		blobDelta, err := repo.GetBlobWithDelta(hash)
		require.NoError(t, err)
		assert.Equal(t, content, blobDelta.Content)
	})

	t.Run("MultipleStorageOfSameContent", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		content := []byte("Duplicate content test")
		
		// Store same content multiple times
		hash1, err := repo.StoreBlobWithDelta(content)
		require.NoError(t, err)
		
		hash2, err := repo.StoreBlobWithDelta(content)
		require.NoError(t, err)
		
		// Should get same hash (deduplication)
		assert.Equal(t, hash1, hash2)

		// Should still be retrievable
		blob, err := repo.store.GetBlob(hash1)
		require.NoError(t, err)
		assert.Equal(t, content, blob.Content)
	})

	t.Run("VariousContentSizes", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		testCases := []struct {
			name    string
			content []byte
		}{
			{"empty", []byte{}},
			{"small", []byte("a")},
			{"medium", []byte("This is a medium-sized content string for testing")},
			{"large", make([]byte, 10000)}, // 10KB
		}

		// Fill large content
		for i := range testCases[3].content {
			testCases[3].content[i] = byte(i % 256)
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				hash, err := repo.StoreBlobWithDelta(tc.content)
				require.NoError(t, err)

				blob, err := repo.store.GetBlob(hash)
				require.NoError(t, err, "Failed to retrieve %s content", tc.name)
				assert.Equal(t, tc.content, blob.Content)
			})
		}
	})
}