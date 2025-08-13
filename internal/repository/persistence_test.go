package repository

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRepository_CreatesNewRepo(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "govc-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Load repository (should create new one)
	repo, err := LoadRepository(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Check that .govc directory was created
	govcDir := filepath.Join(tmpDir, ".govc")
	_, err = os.Stat(govcDir)
	assert.NoError(t, err, ".govc directory should exist")

	// Check subdirectories
	objectsDir := filepath.Join(govcDir, "objects")
	_, err = os.Stat(objectsDir)
	assert.NoError(t, err, "objects directory should exist")

	refsDir := filepath.Join(govcDir, "refs", "heads")
	_, err = os.Stat(refsDir)
	assert.NoError(t, err, "refs/heads directory should exist")

	// Check HEAD file
	headFile := filepath.Join(govcDir, "HEAD")
	_, err = os.Stat(headFile)
	assert.NoError(t, err, "HEAD file should exist")
}

func TestLoadRepository_OpensExistingRepo(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "govc-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// First load - creates repository
	repo1, err := LoadRepository(tmpDir)
	require.NoError(t, err)

	// Add a file and commit
	staging := repo1.GetStagingArea()
	staging.Add("test.txt", []byte("test content"))
	commit1, err := repo1.Commit("First commit")
	require.NoError(t, err)
	commitHash := commit1.Hash()

	// Second load - should open existing repository
	repo2, err := LoadRepository(tmpDir)
	require.NoError(t, err)

	// Verify we can retrieve the commit from the reopened repo
	retrieved, err := repo2.GetCommit(commitHash)
	assert.NoError(t, err)
	assert.Equal(t, "First commit", retrieved.Message)
}

func TestRepositoryPersistence_AcrossSessions(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "govc-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Session 1: Create repo and make commits
	repo1, err := LoadRepository(tmpDir)
	require.NoError(t, err)

	// Make first commit
	staging1 := repo1.GetStagingArea()
	staging1.Add("file1.txt", []byte("content1"))
	commit1, err := repo1.Commit("Commit 1")
	require.NoError(t, err)
	hash1 := commit1.Hash()

	// Make second commit
	staging1.Add("file2.txt", []byte("content2"))
	commit2, err := repo1.Commit("Commit 2")
	require.NoError(t, err)
	hash2 := commit2.Hash()

	// Session 2: Open repo and verify commits exist
	repo2, err := LoadRepository(tmpDir)
	require.NoError(t, err)

	// Check log returns both commits
	commits, err := repo2.Log(10)
	require.NoError(t, err)
	require.Len(t, commits, 2, "Should have 2 commits")

	// Verify commit order (newest first)
	assert.Equal(t, hash2, commits[0].Hash())
	assert.Equal(t, "Commit 2", commits[0].Message)
	assert.Equal(t, hash1, commits[1].Hash())
	assert.Equal(t, "Commit 1", commits[1].Message)

	// Verify parent chain
	assert.Equal(t, hash1, commits[0].ParentHash)
	assert.Equal(t, "", commits[1].ParentHash) // First commit has no parent
}

func TestBranchPersistence(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "govc-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create repo and make a commit on main
	repo1, err := LoadRepository(tmpDir)
	require.NoError(t, err)

	staging := repo1.GetStagingArea()
	staging.Add("main.txt", []byte("main content"))
	mainCommit, err := repo1.Commit("Main commit")
	require.NoError(t, err)

	// Create feature branch
	err = repo1.CreateBranch("feature", mainCommit.Hash())
	require.NoError(t, err)

	// Switch to feature branch
	err = repo1.Checkout("feature")
	require.NoError(t, err)

	// Make a commit on feature
	staging.Add("feature.txt", []byte("feature content"))
	_, err = repo1.Commit("Feature commit")
	require.NoError(t, err)

	// Open repository in new session
	repo2, err := LoadRepository(tmpDir)
	require.NoError(t, err)

	// List branches
	branches, err := repo2.ListBranches()
	require.NoError(t, err)
	assert.Len(t, branches, 2, "Should have 2 branches")

	// Check feature branch exists
	branchFound := false
	for _, branch := range branches {
		if branch == "feature" {
			branchFound = true
			break
		}
	}
	assert.True(t, branchFound, "Feature branch should exist")
}

func TestStagingAreaPersistence(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "govc-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Session 1: Stage some files
	repo1, err := LoadRepository(tmpDir)
	require.NoError(t, err)

	staging1 := repo1.GetStagingArea()
	staging1.Add("staged1.txt", []byte("staged content 1"))
	staging1.Add("staged2.txt", []byte("staged content 2"))

	// Save staging area
	err = repo1.Save()
	require.NoError(t, err)

	// Session 2: Open repo and check staging area persisted
	repo2, err := LoadRepository(tmpDir)
	require.NoError(t, err)

	staging2 := repo2.GetStagingArea()
	
	// Check staged files are still there
	files, err := staging2.List()
	require.NoError(t, err)
	assert.Len(t, files, 2, "Should have 2 staged files")
	
	// Check specific files
	for _, file := range files {
		if file == "staged1.txt" || file == "staged2.txt" {
			hash, err := staging2.GetFileHash(file)
			require.NoError(t, err)
			assert.NotEmpty(t, hash)
		}
	}

	// Commit the staged files
	commit, err := repo2.Commit("Commit staged files")
	require.NoError(t, err)
	assert.NotNil(t, commit)

	// Check staging area is now empty
	files, err = staging2.List()
	require.NoError(t, err)
	assert.Len(t, files, 0, "Staging area should be empty after commit")
}

func TestHEADPersistence(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "govc-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create repo
	repo1, err := LoadRepository(tmpDir)
	require.NoError(t, err)

	// Make a commit
	staging := repo1.GetStagingArea()
	staging.Add("test.txt", []byte("test"))
	commit1, err := repo1.Commit("First commit")
	require.NoError(t, err)

	// Create and checkout feature branch
	err = repo1.CreateBranch("feature", commit1.Hash())
	require.NoError(t, err)
	err = repo1.Checkout("feature")
	require.NoError(t, err)

	// Open repo in new session
	repo2, err := LoadRepository(tmpDir)
	require.NoError(t, err)

	// Check current branch is feature
	currentBranch, err := repo2.GetCurrentBranch()
	require.NoError(t, err)
	assert.Equal(t, "feature", currentBranch)

	// Check current commit is correct
	currentCommit, err := repo2.CurrentCommit()
	require.NoError(t, err)
	assert.Equal(t, commit1.Hash(), currentCommit.Hash())
}

func TestEmptyBranchHandling(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "govc-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create repo (main branch has no commits)
	repo, err := LoadRepository(tmpDir)
	require.NoError(t, err)

	// CurrentCommit should return error for empty repo
	_, err = repo.CurrentCommit()
	assert.Error(t, err, "Should error when no commits exist")

	// Log should return empty list, not error
	commits, err := repo.Log(10)
	assert.NoError(t, err)
	assert.Len(t, commits, 0)

	// Status should work on empty repo
	status, err := repo.Status()
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, "main", status.Branch)
}

func TestCommitChainIntegrity(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "govc-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	repo, err := LoadRepository(tmpDir)
	require.NoError(t, err)

	// Create chain of commits
	var previousHash string
	for i := 1; i <= 5; i++ {
		staging := repo.GetStagingArea()
		filename := filepath.Join("file", string(rune(i+'0')), ".txt")
		staging.Add(filename, []byte("content"))
		
		commit, err := repo.Commit(string(rune(i+'0')))
		require.NoError(t, err)

		// Verify parent is set correctly
		if i == 1 {
			assert.Equal(t, "", commit.ParentHash, "First commit should have no parent")
		} else {
			assert.Equal(t, previousHash, commit.ParentHash, "Commit should point to previous commit")
		}
		
		previousHash = commit.Hash()
	}

	// Verify log returns commits in correct order
	commits, err := repo.Log(10)
	require.NoError(t, err)
	require.Len(t, commits, 5)

	// Check commits are in reverse chronological order
	for i := 0; i < 5; i++ {
		expectedMessage := string(rune(5 - i + '0'))
		assert.Equal(t, expectedMessage, commits[i].Message)
	}

	// Verify parent chain
	for i := 0; i < 4; i++ {
		assert.Equal(t, commits[i+1].Hash(), commits[i].ParentHash)
	}
	assert.Equal(t, "", commits[4].ParentHash, "Oldest commit should have no parent")
}

func TestInitRepository_CreatesFileStructure(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "govc-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Initialize repository
	repo, err := InitRepository(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Verify file structure
	govcDir := filepath.Join(tmpDir, ".govc")
	assert.DirExists(t, govcDir)
	assert.DirExists(t, filepath.Join(govcDir, "objects"))
	assert.DirExists(t, filepath.Join(govcDir, "refs", "heads"))
	assert.FileExists(t, filepath.Join(govcDir, "HEAD"))

	// Verify HEAD points to main branch
	headContent, err := os.ReadFile(filepath.Join(govcDir, "HEAD"))
	require.NoError(t, err)
	assert.Equal(t, "ref: refs/heads/main", string(headContent))
}

func TestOpenRepository_FailsOnNonExistent(t *testing.T) {
	// Try to open non-existent repository
	tmpDir, err := os.MkdirTemp("", "govc-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	nonExistentPath := filepath.Join(tmpDir, "does-not-exist")
	_, err = OpenRepository(nonExistentPath)
	assert.Error(t, err, "Should fail when opening non-existent repository")
}