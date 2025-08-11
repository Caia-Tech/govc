package workspace

import (
	"testing"

	"github.com/Caia-Tech/govc/pkg/object"
	"github.com/Caia-Tech/govc/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkingDirectory(t *testing.T) {
	wd := NewMemoryWorkingDirectory()
	defer wd.Close()

	// Test WriteFile and ReadFile
	content := []byte("test content")
	err := wd.WriteFile("test.txt", content)
	require.NoError(t, err)

	readContent, err := wd.ReadFile("test.txt")
	require.NoError(t, err)
	assert.Equal(t, content, readContent)

	// Test Exists
	assert.True(t, wd.Exists("test.txt"))
	assert.False(t, wd.Exists("nonexistent.txt"))

	// Test ListFiles
	files, err := wd.ListFiles()
	require.NoError(t, err)
	assert.Contains(t, files, "test.txt")

	// Test MatchFiles
	matched, err := wd.MatchFiles("*.txt")
	require.NoError(t, err)
	assert.Contains(t, matched, "test.txt")

	matched, err = wd.MatchFiles("*")
	require.NoError(t, err)
	assert.Contains(t, matched, "test.txt")

	// Test RemoveFile
	err = wd.RemoveFile("test.txt")
	require.NoError(t, err)
	assert.False(t, wd.Exists("test.txt"))

	// Test Clear
	wd.WriteFile("file1.txt", []byte("content1"))
	wd.WriteFile("file2.txt", []byte("content2"))

	err = wd.Clear()
	require.NoError(t, err)

	files, _ = wd.ListFiles()
	assert.Len(t, files, 0)
}

func TestWorkspace(t *testing.T) {
	// Set up storage components
	objectStore := storage.NewMemoryObjectStore()
	refStore := storage.NewMemoryRefStore()
	defer objectStore.Close()
	defer refStore.Close()

	// Create test objects
	blob := object.NewBlob([]byte("test file content"))
	blobHash, err := objectStore.Put(blob)
	require.NoError(t, err)

	tree := object.NewTree()
	tree.AddEntry("100644", "test.txt", blobHash)
	treeHash, err := objectStore.Put(tree)
	require.NoError(t, err)

	author := object.Author{
		Name:  "Test User",
		Email: "test@example.com",
	}
	commit := object.NewCommit(treeHash, author, "Initial commit")
	commitHash, err := objectStore.Put(commit)
	require.NoError(t, err)

	// Create main branch
	err = refStore.UpdateRef("refs/heads/main", commitHash)
	require.NoError(t, err)

	// Create workspace
	ws := NewWorkspace(objectStore, refStore)
	defer ws.Close()

	// Test GetWorkingDirectory
	wd1 := ws.GetWorkingDirectory("main")
	wd2 := ws.GetWorkingDirectory("main")
	assert.Equal(t, wd1, wd2) // Should return the same instance

	// Test different branches get different working directories
	wd3 := ws.GetWorkingDirectory("feature")
	// They should be different instances since they're for different branches
	assert.True(t, wd1 != wd3, "Different branches should have different working directories")

	// Test CheckoutBranch
	err = ws.CheckoutBranch("main")
	require.NoError(t, err)

	// Verify working directory was populated
	currentWd, err := ws.GetCurrentWorkingDirectory()
	require.NoError(t, err)

	content, err := currentWd.ReadFile("test.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("test file content"), content)

	// Test GetCurrentBranch
	branch, err := ws.GetCurrentBranch()
	require.NoError(t, err)
	assert.Equal(t, "main", branch)

	// Test CreateBranch
	err = ws.CreateBranch("feature", commitHash)
	require.NoError(t, err)

	// Switch to feature branch
	err = ws.CheckoutBranch("feature")
	require.NoError(t, err)

	branch, err = ws.GetCurrentBranch()
	require.NoError(t, err)
	assert.Equal(t, "feature", branch)

	// Verify feature branch has the same content
	featureWd, err := ws.GetCurrentWorkingDirectory()
	require.NoError(t, err)

	content, err = featureWd.ReadFile("test.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("test file content"), content)

	// Test branch isolation - modify feature branch
	err = featureWd.WriteFile("feature.txt", []byte("feature content"))
	require.NoError(t, err)

	// Switch back to main
	err = ws.CheckoutBranch("main")
	require.NoError(t, err)

	mainWd, err := ws.GetCurrentWorkingDirectory()
	require.NoError(t, err)

	// Main branch should not have the feature file
	assert.False(t, mainWd.Exists("feature.txt"))

	// But feature branch should still have it
	featureWd = ws.GetWorkingDirectory("feature")
	assert.True(t, featureWd.Exists("feature.txt"))

	// Test DeleteBranch
	err = ws.DeleteBranch("feature")
	require.NoError(t, err)

	// Working directory should be cleaned up
	ws.mu.RLock()
	_, exists := ws.workingDirs["feature"]
	ws.mu.RUnlock()
	assert.False(t, exists)
}

func TestWorkspaceErrors(t *testing.T) {
	objectStore := storage.NewMemoryObjectStore()
	refStore := storage.NewMemoryRefStore()
	defer objectStore.Close()
	defer refStore.Close()

	ws := NewWorkspace(objectStore, refStore)
	defer ws.Close()

	// Test CheckoutBranch with non-existent branch
	err := ws.CheckoutBranch("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "branch not found")

	// Test GetCurrentBranch when HEAD is detached
	err = refStore.SetHEAD("abc123")
	require.NoError(t, err)

	_, err = ws.GetCurrentBranch()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HEAD is detached")

	// Test DeleteBranch with non-existent branch (idempotent - should not error)
	err = ws.DeleteBranch("nonexistent")
	assert.NoError(t, err)
}
