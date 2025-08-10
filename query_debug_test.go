package govc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryEngine_Debug(t *testing.T) {
	// Create a completely isolated repository for debugging
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	t.Logf("Created new repository")
	
	// Add one simple test file
	content := []byte(`{"name": "test", "version": 1}`)
	hash, err := repo.store.StoreBlob(content)
	require.NoError(t, err)
	
	repo.staging.Add("config.json", hash)
	t.Logf("Added config.json to staging with hash: %s", hash)
	
	// Create a commit
	commit, err := repo.Commit("Add config file")
	require.NoError(t, err)
	t.Logf("Created commit: %s with message: %s", commit.Hash(), commit.Message)
	
	// Wait for async indexing to complete after commit
	time.Sleep(100 * time.Millisecond)
	
	// Check what's in the file index
	repo.queryEngine.fileIndex.mu.RLock()
	t.Logf("Files in index: %d", len(repo.queryEngine.fileIndex.pathIndex))
	for path, entry := range repo.queryEngine.fileIndex.pathIndex {
		t.Logf("  File: %s -> %s (size: %d)", path, entry.Hash, entry.Size)
	}
	repo.queryEngine.fileIndex.mu.RUnlock()
	
	// Check what's in the commit index
	repo.queryEngine.commitIndex.mu.RLock()
	t.Logf("Commits in index: %d", len(repo.queryEngine.commitIndex.hashIndex))
	for hash, entry := range repo.queryEngine.commitIndex.hashIndex {
		t.Logf("  Commit: %s -> %s", hash, entry.Message)
	}
	repo.queryEngine.commitIndex.mu.RUnlock()
	
	// Test exact path query
	t.Run("ExactPathQuery", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:  QueryByPath,
			Query: "config.json",
		})
		require.NoError(t, err)
		t.Logf("Exact path query result: %d files", len(result.Files))
		for i, file := range result.Files {
			t.Logf("  File %d: %s", i, file.Path)
		}
		assert.Equal(t, 1, len(result.Files))
		if len(result.Files) > 0 {
			assert.Equal(t, "config.json", result.Files[0].Path)
		}
	})
	
	// Test pattern query
	t.Run("PatternQuery", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:  QueryByPattern,
			Query: "*.json",
		})
		require.NoError(t, err)
		t.Logf("Pattern query result: %d files", len(result.Files))
		for i, file := range result.Files {
			t.Logf("  File %d: %s", i, file.Path)
		}
		assert.Equal(t, 1, len(result.Files))
		if len(result.Files) > 0 {
			assert.Equal(t, "config.json", result.Files[0].Path)
		}
	})
	
	// Test commit query
	t.Run("CommitQuery", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:  QueryByCommit,
			Query: "config",
		})
		require.NoError(t, err)
		t.Logf("Commit query result: %d commits", len(result.Commits))
		for i, commit := range result.Commits {
			t.Logf("  Commit %d: %s - %s", i, commit.Hash, commit.Message)
		}
		// We expect at least 1 commit containing "config"
		assert.True(t, len(result.Commits) >= 1)
		
		// Check that our commit is in there
		found := false
		for _, commit := range result.Commits {
			if commit.Message == "Add config file" {
				found = true
				break
			}
		}
		assert.True(t, found, "Should find our commit")
	})
}