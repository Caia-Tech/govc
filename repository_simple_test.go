package govc

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRepository_SimpleOperations tests basic repository functionality
func TestRepository_SimpleOperations(t *testing.T) {
	t.Run("CreateAndCommit", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Write a file to the worktree
		err := repo.WriteFile("test.txt", []byte("Hello, World!"))
		require.NoError(t, err)

		// Add the file to staging
		err = repo.Add("test.txt")
		require.NoError(t, err)

		// Commit
		commit, err := repo.Commit("Initial commit")
		require.NoError(t, err)
		assert.NotNil(t, commit)
		assert.Equal(t, "Initial commit", commit.Message)
		assert.NotEmpty(t, commit.Hash())
	})

	t.Run("ReadAfterCommit", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		content := []byte("Test content for reading")
		
		// Write and commit a file
		err := repo.WriteFile("read_test.txt", content)
		require.NoError(t, err)
		
		err = repo.Add("read_test.txt")
		require.NoError(t, err)
		
		_, err = repo.Commit("Add read test file")
		require.NoError(t, err)

		// Read the file
		readContent, err := repo.ReadFile("read_test.txt")
		require.NoError(t, err)
		assert.Equal(t, content, readContent)
	})

	t.Run("MultipleFiles", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		files := map[string][]byte{
			"file1.txt": []byte("Content 1"),
			"file2.txt": []byte("Content 2"),
			"file3.txt": []byte("Content 3"),
		}

		// Write all files
		for path, content := range files {
			err := repo.WriteFile(path, content)
			require.NoError(t, err)
		}

		// Add all files
		for path := range files {
			err := repo.Add(path)
			require.NoError(t, err)
		}

		// Commit
		_, err := repo.Commit("Add multiple files")
		require.NoError(t, err)

		// Verify all files can be read
		for path, expectedContent := range files {
			content, err := repo.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, expectedContent, content)
		}
	})

	t.Run("BranchOperations", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Create initial commit
		err := repo.WriteFile("main.txt", []byte("main content"))
		require.NoError(t, err)
		err = repo.Add("main.txt")
		require.NoError(t, err)
		_, err = repo.Commit("Main commit")
		require.NoError(t, err)

		// Create new branch
		err = repo.Branch("feature").Create()
		require.NoError(t, err)

		// Checkout feature branch
		err = repo.Checkout("feature")
		require.NoError(t, err)

		// Add new file on feature branch
		err = repo.WriteFile("feature.txt", []byte("feature content"))
		require.NoError(t, err)
		err = repo.Add("feature.txt")
		require.NoError(t, err)
		_, err = repo.Commit("Feature commit")
		require.NoError(t, err)

		// Verify feature file exists
		content, err := repo.ReadFile("feature.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("feature content"), content)

		// Switch back to main
		err = repo.Checkout("main")
		require.NoError(t, err)

		// Feature file should not exist on main
		_, err = repo.ReadFile("feature.txt")
		assert.Error(t, err)
	})

	t.Run("StashOperations", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Create uncommitted changes
		err := repo.WriteFile("stash.txt", []byte("Stashed content"))
		require.NoError(t, err)
		err = repo.Add("stash.txt")
		require.NoError(t, err)

		// Stash changes (include untracked files)
		stash, err := repo.Stash("Work in progress", true)
		require.NoError(t, err)
		assert.NotNil(t, stash)
		assert.Equal(t, "Work in progress", stash.Message)

		// List stashes to get the stash ID
		stashes := repo.ListStashes()
		require.NotEmpty(t, stashes)
		
		// Apply stash using ID
		err = repo.ApplyStash(stashes[0].ID, false)
		require.NoError(t, err)
	})
}

// TestRepository_AtomicOperations tests atomic file operations
func TestRepository_AtomicOperations(t *testing.T) {
	t.Run("AtomicMultiFileUpdate", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Create files using AtomicMultiFileUpdate
		files := map[string][]byte{
			"atomic1.txt": []byte("Atomic content 1"),
			"atomic2.txt": []byte("Atomic content 2"),
			"atomic3.txt": []byte("Atomic content 3"),
		}

		commit, err := repo.AtomicMultiFileUpdate(files, "Atomic update")
		require.NoError(t, err)
		assert.NotNil(t, commit)

		// Verify all files are readable
		for path, expectedContent := range files {
			content, err := repo.ReadFile(path)
			require.NoError(t, err, "Failed to read %s", path)
			assert.Equal(t, expectedContent, content)
		}
	})

	t.Run("AtomicCreateFile", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Create single file atomically
		content := []byte("Single atomic file")
		commit, err := repo.AtomicCreateFile("single.txt", content, "Create single file")
		require.NoError(t, err)
		assert.NotNil(t, commit)

		// Read and verify
		readContent, err := repo.ReadFile("single.txt")
		require.NoError(t, err)
		assert.Equal(t, content, readContent)
	})

	t.Run("AtomicUpdateFile", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Create initial file
		initialContent := []byte("Initial")
		_, err := repo.AtomicCreateFile("update.txt", initialContent, "Create file")
		require.NoError(t, err)

		// Update atomically
		updatedContent := []byte("Updated")
		commit, err := repo.AtomicUpdateFile("update.txt", updatedContent, "Update file")
		require.NoError(t, err)
		assert.NotNil(t, commit)

		// Verify update
		readContent, err := repo.ReadFile("update.txt")
		require.NoError(t, err)
		assert.Equal(t, updatedContent, readContent)
	})
}

// TestParallelReality_Simple tests basic parallel reality functionality
func TestParallelReality_Simple(t *testing.T) {
	t.Run("CreateReality", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Create initial commit
		err := repo.WriteFile("base.txt", []byte("base content"))
		require.NoError(t, err)
		err = repo.Add("base.txt")
		require.NoError(t, err)
		_, err = repo.Commit("Base commit")
		require.NoError(t, err)

		// Create parallel reality
		reality := repo.ParallelReality("test-reality")
		assert.NotNil(t, reality)
		assert.Equal(t, "parallel/test-reality", reality.name)
		assert.True(t, reality.isolated)
		assert.True(t, reality.ephemeral)
	})

	t.Run("MultipleRealities", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Create multiple realities
		names := []string{"reality-a", "reality-b", "reality-c"}
		realities := repo.ParallelRealities(names)
		
		assert.Len(t, realities, 3)
		for i, reality := range realities {
			assert.NotNil(t, reality)
			assert.Equal(t, fmt.Sprintf("parallel/%s", names[i]), reality.name)
		}

		// Verify branches exist
		branches, err := repo.ListBranches()
		require.NoError(t, err)
		
		// Convert refs to branch names
		branchNames := make([]string, len(branches))
		for i, ref := range branches {
			branchNames[i] = ref.Name
		}
		
		for _, name := range names {
			assert.Contains(t, branchNames, fmt.Sprintf("refs/heads/parallel/%s", name))
		}
	})

	t.Run("RealityPerformance", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Measure time to create many realities
		start := time.Now()
		count := 100
		names := make([]string, count)
		for i := 0; i < count; i++ {
			names[i] = fmt.Sprintf("perf-%d", i)
		}

		realities := repo.ParallelRealities(names)
		duration := time.Since(start)

		// Should be very fast
		assert.Len(t, realities, count)
		assert.Less(t, duration, 500*time.Millisecond)
		t.Logf("Created %d realities in %v", count, duration)
	})
}

// TestRepository_EdgeCases tests edge cases and error conditions
func TestRepository_EdgeCases(t *testing.T) {
	t.Run("EmptyCommit", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Try to commit without any changes
		// Some repositories allow empty commits
		commit, err := repo.Commit("Empty commit")
		
		// Either it errors or allows empty commit
		if err != nil {
			assert.Contains(t, err.Error(), "nothing")
			assert.Nil(t, commit)
		} else {
			// Empty commit is allowed
			assert.NotNil(t, commit)
			assert.Equal(t, "Empty commit", commit.Message)
		}
	})

	t.Run("CheckoutNonExistentBranch", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		err := repo.Checkout("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "branch not found")
	})

	t.Run("EmptyFile", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Add empty file
		err := repo.WriteFile("empty.txt", []byte{})
		require.NoError(t, err)
		err = repo.Add("empty.txt")
		require.NoError(t, err)
		_, err = repo.Commit("Add empty file")
		require.NoError(t, err)

		// Read empty file
		content, err := repo.ReadFile("empty.txt")
		require.NoError(t, err)
		assert.Empty(t, content)
	})

	t.Run("LargeFile", func(t *testing.T) {
		repo := NewRepository()
		defer repo.eventBus.Stop()

		// Create 100KB file
		largeContent := make([]byte, 100*1024)
		for i := range largeContent {
			largeContent[i] = byte(i % 256)
		}

		// Add and commit
		err := repo.WriteFile("large.bin", largeContent)
		require.NoError(t, err)
		err = repo.Add("large.bin")
		require.NoError(t, err)
		
		start := time.Now()
		_, err = repo.Commit("Add large file")
		require.NoError(t, err)
		duration := time.Since(start)

		// Should be fast
		assert.Less(t, duration, 500*time.Millisecond)

		// Read and verify
		content, err := repo.ReadFile("large.bin")
		require.NoError(t, err)
		assert.Equal(t, len(largeContent), len(content))
	})
}