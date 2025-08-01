package govc

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBasicRepository tests basic repository operations
func TestBasicRepository(t *testing.T) {
	repo := New()
	assert.NotNil(t, repo)
	assert.NotNil(t, repo.store)
	assert.NotNil(t, repo.staging)
	assert.NotNil(t, repo.refManager)
}

// TestBasicCommit tests basic commit operations
func TestBasicCommit(t *testing.T) {
	repo := New()
	
	// Write and commit a file
	err := repo.WriteFile("test.txt", []byte("hello world"))
	assert.NoError(t, err)
	
	err = repo.Add("test.txt")
	assert.NoError(t, err)
	
	commit, err := repo.Commit("Test commit")
	require.NoError(t, err)
	assert.NotEmpty(t, commit.Hash())
	assert.Equal(t, "Test commit", commit.Message)
}

// TestBranchOperations tests branch operations
func TestBranchOperations(t *testing.T) {
	repo := New()
	
	// Create initial commit
	repo.WriteFile("init.txt", []byte("initial"))
	repo.Add("init.txt")
	repo.Commit("Initial")
	
	// Create branch
	err := repo.Branch("feature").Create()
	assert.NoError(t, err)
	
	// List branches
	branches, err := repo.ListBranches()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(branches), 2)
	
	// Checkout branch
	err = repo.Checkout("feature")
	assert.NoError(t, err)
	
	// Get current branch
	current, err := repo.CurrentBranch()
	assert.NoError(t, err)
	assert.Equal(t, "feature", current)
}

// TestParallelReality tests parallel reality functionality
func TestParallelReality(t *testing.T) {
	repo := New()
	
	// Create reality
	pr := repo.ParallelReality("test")
	assert.NotNil(t, pr)
	assert.Equal(t, "parallel/test", pr.Name())
	
	// Apply changes
	err := pr.Apply(map[string][]byte{
		"config.yaml": []byte("test: true"),
	})
	assert.NoError(t, err)
}

// TestTransaction tests transactional commits
func TestTransaction(t *testing.T) {
	repo := New()
	
	// Create transaction
	tx := repo.Transaction()
	assert.NotNil(t, tx)
	
	// Add files
	tx.Add("file1.txt", []byte("content1"))
	tx.Add("file2.txt", []byte("content2"))
	
	// Validate
	err := tx.Validate()
	assert.NoError(t, err)
	
	// Commit
	commit, err := tx.Commit("Transaction commit")
	assert.NoError(t, err)
	assert.NotEmpty(t, commit.Hash())
}

// TestTimeTravel tests time travel functionality
func TestTimeTravel(t *testing.T) {
	repo := New()
	
	// Create commits
	repo.WriteFile("v1.txt", []byte("version 1"))
	repo.Add("v1.txt")
	commit1, _ := repo.Commit("Version 1")
	
	time.Sleep(10 * time.Millisecond)
	
	repo.WriteFile("v2.txt", []byte("version 2"))
	repo.Add("v2.txt")
	repo.Commit("Version 2")
	
	// Travel to first commit time
	snapshot := repo.TimeTravel(commit1.Author.Time)
	assert.NotNil(t, snapshot)
	
	// Read content at that time
	content, err := snapshot.Read("v1.txt")
	assert.NoError(t, err)
	assert.Equal(t, []byte("version 1"), content)
}

// TestConfiguration tests configuration
func TestConfiguration(t *testing.T) {
	repo := New()
	
	// Set config
	repo.SetConfig("user.name", "Test User")
	repo.SetConfig("user.email", "test@example.com")
	
	// Get config
	name := repo.GetConfig("user.name")
	assert.Equal(t, "Test User", name)
	
	email := repo.GetConfig("user.email")
	assert.Equal(t, "test@example.com", email)
}

// TestFileOperations tests file operations
func TestFileOperations(t *testing.T) {
	repo := New()
	
	// Write file
	err := repo.WriteFile("test.txt", []byte("hello"))
	assert.NoError(t, err)
	
	// Add and commit the file
	err = repo.Add("test.txt")
	assert.NoError(t, err)
	
	_, err = repo.Commit("Add test file")
	assert.NoError(t, err)
	
	// Read file
	content, err := repo.ReadFile("test.txt")
	assert.NoError(t, err)
	assert.Equal(t, []byte("hello"), content)
	
	// List files
	files, err := repo.ListFiles()
	assert.NoError(t, err)
	assert.Contains(t, files, "test.txt")
}

// TestStatus tests repository status
func TestStatus(t *testing.T) {
	repo := New()
	
	// Add untracked file
	repo.WriteFile("untracked.txt", []byte("content"))
	
	// Get status
	status, err := repo.Status()
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Contains(t, status.Untracked, "untracked.txt")
}

// TestLog tests commit log
func TestLog(t *testing.T) {
	repo := New()
	
	// Create commits
	for i := 0; i < 3; i++ {
		repo.WriteFile("file.txt", []byte(string(rune('A'+i))))
		repo.Add("file.txt")
		repo.Commit("Commit " + string(rune('A'+i)))
	}
	
	// Get log
	commits, err := repo.Log(2)
	assert.NoError(t, err)
	assert.Len(t, commits, 2)
}

// TestExport tests export functionality
func TestExport(t *testing.T) {
	repo := New()
	
	// Create content
	repo.WriteFile("export.txt", []byte("export me"))
	repo.Add("export.txt")
	repo.Commit("Export commit")
	
	// Export
	var buf bytes.Buffer
	err := repo.Export(&buf, "fast-export")
	assert.NoError(t, err)
	assert.Greater(t, buf.Len(), 0)
}

// TestStash tests stashing
func TestStash(t *testing.T) {
	repo := New()
	
	// Create initial commit
	repo.WriteFile("file.txt", []byte("initial"))
	repo.Add("file.txt")
	repo.Commit("Initial")
	
	// Make changes
	repo.WriteFile("file.txt", []byte("modified"))
	
	// Stash
	stash, err := repo.Stash("WIP", true)
	assert.NoError(t, err)
	assert.NotNil(t, stash)
	
	// File should be back to initial
	content, _ := repo.ReadFile("file.txt")
	assert.Equal(t, []byte("initial"), content)
}

// TestCherryPick tests cherry-pick
func TestCherryPick(t *testing.T) {
	repo := New()
	
	// Create base
	repo.WriteFile("base.txt", []byte("base"))
	repo.Add("base.txt")
	repo.Commit("Base")
	
	// Create branch with commit
	repo.Branch("feature").Create()
	repo.Checkout("feature")
	repo.WriteFile("cherry.txt", []byte("cherry"))
	repo.Add("cherry.txt")
	cherryCommit, _ := repo.Commit("Cherry commit")
	
	// Back to main
	repo.Checkout("main")
	
	// Cherry-pick
	picked, err := repo.CherryPick(cherryCommit.Hash())
	assert.NoError(t, err)
	assert.NotNil(t, picked)
}