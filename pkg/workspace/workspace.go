package workspace

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/Caia-Tech/govc/pkg/object"
	"github.com/Caia-Tech/govc/pkg/storage"
)

// WorkingDirectory represents the working directory abstraction
// It can be backed by either memory or filesystem storage
type WorkingDirectory struct {
	storage storage.WorkingStorage
	mu      sync.RWMutex
}

// NewWorkingDirectory creates a new working directory with the given storage
func NewWorkingDirectory(storage storage.WorkingStorage) *WorkingDirectory {
	return &WorkingDirectory{
		storage: storage,
	}
}

// NewMemoryWorkingDirectory creates a memory-only working directory
func NewMemoryWorkingDirectory() *WorkingDirectory {
	return NewWorkingDirectory(storage.NewMemoryWorkingStorage())
}

// ReadFile reads a file from the working directory
func (wd *WorkingDirectory) ReadFile(path string) ([]byte, error) {
	wd.mu.RLock()
	defer wd.mu.RUnlock()
	return wd.storage.Read(path)
}

// WriteFile writes a file to the working directory
func (wd *WorkingDirectory) WriteFile(path string, content []byte) error {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	return wd.storage.Write(path, content)
}

// RemoveFile removes a file from the working directory
func (wd *WorkingDirectory) RemoveFile(path string) error {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	return wd.storage.Delete(path)
}

// ListFiles returns all files in the working directory
func (wd *WorkingDirectory) ListFiles() ([]string, error) {
	wd.mu.RLock()
	defer wd.mu.RUnlock()
	return wd.storage.List()
}

// MatchFiles returns files matching the given pattern
func (wd *WorkingDirectory) MatchFiles(pattern string) ([]string, error) {
	files, err := wd.ListFiles()
	if err != nil {
		return nil, err
	}

	var matched []string
	for _, file := range files {
		if pattern == "." || pattern == "*" {
			matched = append(matched, file)
		} else if match, _ := filepath.Match(pattern, file); match {
			matched = append(matched, file)
		}
	}

	return matched, nil
}

// Exists checks if a file exists in the working directory
func (wd *WorkingDirectory) Exists(path string) bool {
	wd.mu.RLock()
	defer wd.mu.RUnlock()
	return wd.storage.Exists(path)
}

// Clear removes all files from the working directory
func (wd *WorkingDirectory) Clear() error {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	return wd.storage.Clear()
}

// Close releases any resources held by the working directory
func (wd *WorkingDirectory) Close() error {
	return wd.storage.Close()
}

// Workspace coordinates between object storage, refs, and working directory
// It provides branch isolation by managing separate working directories per branch
type Workspace struct {
	objectStore storage.ObjectStore
	refStore    storage.RefStore
	workingDirs map[string]*WorkingDirectory // branch name -> working directory
	mu          sync.RWMutex
}

// NewWorkspace creates a new workspace with the given storage components
func NewWorkspace(objectStore storage.ObjectStore, refStore storage.RefStore) *Workspace {
	return &Workspace{
		objectStore: objectStore,
		refStore:    refStore,
		workingDirs: make(map[string]*WorkingDirectory),
	}
}

// GetWorkingDirectory returns the working directory for the given branch
// Creates a new one if it doesn't exist
func (ws *Workspace) GetWorkingDirectory(branch string) *WorkingDirectory {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if wd, exists := ws.workingDirs[branch]; exists {
		return wd
	}

	// Create new working directory for this branch
	wd := NewMemoryWorkingDirectory()
	ws.workingDirs[branch] = wd
	return wd
}

// CheckoutBranch switches to the specified branch and updates the working directory
func (ws *Workspace) CheckoutBranch(branchName string) error {
	// Get the commit hash for this branch
	commitHash, err := ws.refStore.GetRef("refs/heads/" + branchName)
	if err != nil {
		return fmt.Errorf("branch not found: %s", branchName)
	}

	// Get the working directory for this branch
	wd := ws.GetWorkingDirectory(branchName)

	// Load the tree from the commit
	err = ws.populateWorkingDirectory(wd, commitHash)
	if err != nil {
		return fmt.Errorf("failed to populate working directory: %v", err)
	}

	// Update HEAD
	return ws.refStore.SetHEAD("ref: refs/heads/" + branchName)
}

// populateWorkingDirectory loads files from a commit into the working directory
func (ws *Workspace) populateWorkingDirectory(wd *WorkingDirectory, commitHash string) error {
	// Get the commit object
	commitObj, err := ws.objectStore.Get(commitHash)
	if err != nil {
		return fmt.Errorf("failed to get commit %s: %v", commitHash, err)
	}

	commit, ok := commitObj.(*object.Commit)
	if !ok {
		return fmt.Errorf("object %s is not a commit", commitHash)
	}

	// Get the tree object
	treeObj, err := ws.objectStore.Get(commit.TreeHash)
	if err != nil {
		return fmt.Errorf("failed to get tree %s: %v", commit.TreeHash, err)
	}

	tree, ok := treeObj.(*object.Tree)
	if !ok {
		return fmt.Errorf("object %s is not a tree", commit.TreeHash)
	}

	// Clear the working directory first
	wd.Clear()

	// Load files from the tree
	for _, entry := range tree.Entries {
		blobObj, err := ws.objectStore.Get(entry.Hash)
		if err != nil {
			return fmt.Errorf("failed to get blob %s: %v", entry.Hash, err)
		}

		blob, ok := blobObj.(*object.Blob)
		if !ok {
			return fmt.Errorf("object %s is not a blob", entry.Hash)
		}

		err = wd.WriteFile(entry.Name, blob.Content)
		if err != nil {
			return fmt.Errorf("failed to write file %s: %v", entry.Name, err)
		}
	}

	return nil
}

// GetCurrentBranch returns the name of the current branch
func (ws *Workspace) GetCurrentBranch() (string, error) {
	head, err := ws.refStore.GetHEAD()
	if err != nil {
		return "", err
	}

	// Handle symbolic references with "ref: " prefix
	if head == "ref: refs/heads/main" {
		return "main", nil
	}
	if len(head) > 5 && head[:5] == "ref: " {
		ref := head[5:] // Remove "ref: " prefix
		if len(ref) > len("refs/heads/") && ref[:len("refs/heads/")] == "refs/heads/" {
			return ref[len("refs/heads/"):], nil
		}
	}

	// Handle direct symbolic references
	if head == "refs/heads/main" {
		return "main", nil
	}
	if len(head) > len("refs/heads/") && head[:len("refs/heads/")] == "refs/heads/" {
		return head[len("refs/heads/"):], nil
	}

	// Check if it's a hex hash (detached HEAD) - can be any length
	// If it doesn't start with refs/ and isn't a simple branch name, assume it's a hash
	if len(head) >= 4 && isHexString(head) {
		return "", fmt.Errorf("HEAD is detached")
	}

	// For other cases, assume it's a branch name without the refs/heads/ prefix
	return head, nil
}

// GetCurrentWorkingDirectory returns the working directory for the current branch
func (ws *Workspace) GetCurrentWorkingDirectory() (*WorkingDirectory, error) {
	branch, err := ws.GetCurrentBranch()
	if err != nil {
		return nil, err
	}

	return ws.GetWorkingDirectory(branch), nil
}

// CreateBranch creates a new branch from the current commit
func (ws *Workspace) CreateBranch(branchName string, fromCommit string) error {
	err := ws.refStore.UpdateRef("refs/heads/"+branchName, fromCommit)
	if err != nil {
		return err
	}

	// Create a working directory for the new branch
	wd := ws.GetWorkingDirectory(branchName)

	// Populate it with the commit's content
	return ws.populateWorkingDirectory(wd, fromCommit)
}

// DeleteBranch removes a branch and its working directory
func (ws *Workspace) DeleteBranch(branchName string) error {
	err := ws.refStore.DeleteRef("refs/heads/" + branchName)
	if err != nil {
		return err
	}

	// Clean up the working directory
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if wd, exists := ws.workingDirs[branchName]; exists {
		wd.Close()
		delete(ws.workingDirs, branchName)
	}

	return nil
}

// Close releases all resources held by the workspace
func (ws *Workspace) Close() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Close all working directories
	for _, wd := range ws.workingDirs {
		wd.Close()
	}
	ws.workingDirs = make(map[string]*WorkingDirectory)

	// Close storage components
	ws.objectStore.Close()
	ws.refStore.Close()

	return nil
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}
