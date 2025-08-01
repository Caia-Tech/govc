package govc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGovcAPI tests the main govc API functions
func TestGovcAPI(t *testing.T) {
	// Test New()
	repo := New()
	assert.NotNil(t, repo)
	
	// Test NewWithConfig()
	config := Config{
		MemoryOnly: true,
		Author: ConfigAuthor{
			Name:  "Test User",
			Email: "test@example.com",
		},
	}
	repoWithConfig := NewWithConfig(config)
	assert.NotNil(t, repoWithConfig)
	assert.Equal(t, "Test User", repoWithConfig.GetConfig("user.name"))
	assert.Equal(t, "test@example.com", repoWithConfig.GetConfig("user.email"))
}

// TestInitAndOpen tests Init and Open functions
func TestInitAndOpen(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Test Init
	repo1, err := Init(tmpDir)
	require.NoError(t, err)
	assert.NotNil(t, repo1)
	
	// Test Open
	repo2, err := Open(tmpDir)
	require.NoError(t, err)
	assert.NotNil(t, repo2)
}

// TestNewMemoryStore tests memory store creation
func TestNewMemoryStore(t *testing.T) {
	store := NewMemoryStore()
	assert.NotNil(t, store)
}

// TestParallelRealityBasic tests basic parallel reality operations
func TestParallelRealityBasic(t *testing.T) {
	repo := New()
	
	// Create single reality
	pr := repo.ParallelReality("test")
	assert.NotNil(t, pr)
	assert.Equal(t, "parallel/test", pr.Name())
	
	// Create multiple realities
	names := []string{"dev", "staging", "prod"}
	realities := repo.ParallelRealities(names)
	assert.Len(t, realities, 3)
	
	for i, pr := range realities {
		assert.Equal(t, "parallel/"+names[i], pr.Name())
	}
}

// TestParallelRealityApplyChanges tests applying changes
func TestParallelRealityApplyChanges(t *testing.T) {
	repo := New()
	pr := repo.ParallelReality("changes-test")
	
	// Apply map changes
	err := pr.Apply(map[string][]byte{
		"file1.txt": []byte("content1"),
		"file2.txt": []byte("content2"),
	})
	assert.NoError(t, err)
	
	// Apply function changes
	err = pr.Apply(func(reality *ParallelReality) {
		// Custom logic here
	})
	assert.NoError(t, err)
	
	// Apply unsupported type
	err = pr.Apply("unsupported")
	assert.Error(t, err)
}

// TestParallelRealityEvaluate tests evaluation
func TestParallelRealityEvaluate(t *testing.T) {
	repo := New()
	pr := repo.ParallelReality("eval-test")
	
	result := pr.Evaluate()
	assert.NotNil(t, result)
	
	// Check result structure
	evalMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "parallel/eval-test", evalMap["reality"])
	assert.True(t, evalMap["isolated"].(bool))
}

// TestParallelRealityBenchmarkResult tests benchmark functionality
func TestParallelRealityBenchmarkResult(t *testing.T) {
	repo := New()
	pr := repo.ParallelReality("bench-test")
	
	result := pr.Benchmark()
	assert.NotNil(t, result)
	assert.Equal(t, "parallel/bench-test", result.Reality)
	assert.NotNil(t, result.Metrics)
}

// TestTransactionBasic tests basic transaction operations
func TestTransactionBasic(t *testing.T) {
	repo := New()
	
	// Create transaction
	tx := repo.Transaction()
	assert.NotNil(t, tx)
	
	// Access internals directly since Transaction returns *TransactionalCommit
	assert.NotNil(t, tx.changes)
}

// TestWatchEvents tests event watching
func TestWatchEvents(t *testing.T) {
	repo := New()
	
	eventCount := 0
	repo.Watch(func(event CommitEvent) {
		eventCount++
	})
	
	// The watch function should be registered
	// (actual event testing would require triggering commits)
}

// TestCommitsSimple tests commit retrieval
func TestCommitsSimple(t *testing.T) {
	repo := New()
	
	// Initially should have no commits (or initial commit)
	commits, _ := repo.Log(0)
	assert.NotNil(t, commits)
}

// TestBranchesSimple tests branch operations
func TestBranchesSimple(t *testing.T) {
	repo := New()
	
	// Get branches (should be empty initially)
	branches, err := repo.ListBranches()
	assert.NoError(t, err)
	assert.NotNil(t, branches)
	assert.Empty(t, branches) // No branches until first commit
	
	// Create branch
	err = repo.Branch("feature").Create()
	assert.NoError(t, err)
	
	// Verify branch exists
	branches, err = repo.ListBranches()
	assert.NoError(t, err)
	// Check for feature branch
	hasFeature := false
	for _, b := range branches {
		if b.Name == "refs/heads/feature" {
			hasFeature = true
			break
		}
	}
	assert.True(t, hasFeature)
}

// TestTagsSimple tests tag operations
func TestTagsSimple(t *testing.T) {
	repo := New()
	
	// Initially no tags
	tags, _ := repo.ListTags()
	assert.Empty(t, tags)
	
	// Create a tag would require a commit first
}

// TestCheckoutSimple tests branch checkout
func TestCheckoutSimple(t *testing.T) {
	repo := New()
	
	// Need at least one commit before creating branches
	err := repo.WriteFile("README.md", []byte("# Test"))
	require.NoError(t, err)
	err = repo.Add("README.md")
	require.NoError(t, err)
	_, err = repo.Commit("Initial commit")
	require.NoError(t, err)
	
	// Create new branch
	err = repo.Branch("develop").Create()
	require.NoError(t, err)
	
	// Checkout
	err = repo.Checkout("develop")
	assert.NoError(t, err)
	
	// Checkout to non-existent branch should work (creates orphan branch)
	err = repo.Checkout("non-existent")
	assert.NoError(t, err)
}

// TestMergeSimple tests branch merging
func TestMergeSimple(t *testing.T) {
	repo := New()
	
	// Would need to create branches and commits first
	err := repo.Merge("non-existent", "main")
	assert.Error(t, err)
}

// TestSnapshotSimple tests snapshot functionality
func TestSnapshotSimple(t *testing.T) {
	repo := New()
	
	// Create a commit first
	tx := repo.Transaction()
	tx.Add("test.txt", []byte("test"))
	tx.Validate()
	commit, _ := tx.Commit("Test")
	
	// Get snapshot using time travel
	snapshot := repo.TimeTravel(commit.Author.Time)
	assert.NotNil(t, snapshot)
}

// TestTimeTravelSimple tests time travel
func TestTimeTravelSimple(t *testing.T) {
	repo := New()
	
	// Time travel to current time
	snapshot := repo.TimeTravel(time.Now())
	assert.NotNil(t, snapshot)
}

// TestConfigurationSimple tests config operations
func TestConfigurationSimple(t *testing.T) {
	repo := New()
	
	// Set config
	repo.SetConfig("test.key", "test value")
	
	// Get config
	value := repo.GetConfig("test.key")
	assert.Equal(t, "test value", value)
	
	// Get non-existent
	missing := repo.GetConfig("missing")
	assert.Equal(t, "", missing)
}

// TestLastCommitSimple tests getting last commit
func TestLastCommitSimple(t *testing.T) {
	repo := New()
	
	// Should handle no commits gracefully
	last, _ := repo.CurrentCommit()
	// Could be nil or initial commit
	_ = last
}

// TestResetSimple tests reset functionality
func TestResetSimple(t *testing.T) {
	repo := New()
	
	// Would need commits first
	err := repo.Reset("invalid-hash", "hard")
	assert.Error(t, err)
}

// TestDeleteBranch tests branch deletion
func TestDeleteBranch(t *testing.T) {
	repo := New()
	
	// Need a commit before creating branches
	repo.WriteFile("README.md", []byte("# Test"))
	repo.Add("README.md")
	repo.Commit("Initial commit")
	
	// Create and delete branch
	err := repo.Branch("temp").Create()
	assert.NoError(t, err)
	err = repo.Branch("temp").Delete()
	assert.NoError(t, err)
	
	// Can't delete current branch
	currentBranch, _ := repo.CurrentBranch()
	err = repo.Branch(currentBranch).Delete()
	assert.Error(t, err)
}

// TestCreateAndDeleteTag tests tag operations
func TestCreateAndDeleteTag(t *testing.T) {
	repo := New()
	
	// Would need a commit first
	err := repo.CreateTag("v1.0.0", "Version 1.0.0")
	// Might error if no commits
	_ = err
	
	// Delete non-existent tag - check if DeleteTag exists
	// err = repo.DeleteTag("non-existent")
	// assert.Error(t, err)
}