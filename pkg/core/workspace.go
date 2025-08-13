package core

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync"

	"github.com/Caia-Tech/govc/pkg/object"
)

// CleanWorkspace manages the mutable working directory state
// Separated from repository to maintain clean architecture
type CleanWorkspace struct {
	mu      sync.RWMutex
	repo    *CleanRepository
	working WorkingStorage
	staging *CleanStagingArea
	branch  string // Current branch
	diag    *DiagnosticLogger
}

// NewCleanWorkspace creates a new workspace
func NewCleanWorkspace(repo *CleanRepository, working WorkingStorage) *CleanWorkspace {
	return &CleanWorkspace{
		repo:    repo,
		working: working,
		staging: NewCleanStagingArea(),
		branch:  "main", // Default branch
		diag:    NewDiagnosticLogger("Workspace"),
	}
}

// CleanStagingArea represents the Git index/staging area
type CleanStagingArea struct {
	mu      sync.RWMutex
	entries map[string]StagedEntry
	removed map[string]bool // Track removed files
}

// NewCleanStagingArea creates a new staging area
func NewCleanStagingArea() *CleanStagingArea {
	return &CleanStagingArea{
		entries: make(map[string]StagedEntry),
		removed: make(map[string]bool),
	}
}

// Add stages a file
func (w *CleanWorkspace) Add(path string) error {
	// Check if file exists in working directory
	content, err := w.working.Read(path)
	if err != nil {
		return fmt.Errorf("cannot read file %s: %w", path, err)
	}

	// Create blob
	blob := object.NewBlob(content)
	hash, err := w.repo.objects.Put(blob)
	if err != nil {
		return fmt.Errorf("cannot store blob: %w", err)
	}

	// Add to staging
	w.staging.mu.Lock()
	w.staging.entries[path] = StagedEntry{
		Hash: hash,
		Mode: "100644", // Regular file
	}

	// Remove from removed list if it was there
	delete(w.staging.removed, path)
	w.staging.mu.Unlock()

	return nil
}

// Remove stages a file removal
func (w *CleanWorkspace) Remove(path string) error {
	// Mark as removed
	w.staging.mu.Lock()
	w.staging.removed[path] = true

	// Remove from staged entries
	delete(w.staging.entries, path)
	w.staging.mu.Unlock()

	// Remove from working directory
	return w.working.Delete(path)
}

// Status returns the current workspace status
func (w *CleanWorkspace) Status() (*Status, error) {
	w.mu.RLock()
	currentBranch := w.branch
	w.mu.RUnlock()

	status := &Status{
		Branch:    currentBranch,
		Staged:    []string{},
		Modified:  []string{},
		Untracked: []string{},
	}

	// Get current commit tree
	head, err := w.repo.GetBranch(currentBranch)
	if err != nil {
		// New repository, no commits yet
		head = ""
	}

	var currentTree *object.Tree
	if head != "" {
		commit, err := w.repo.GetCommit(head)
		if err == nil {
			currentTree, _ = w.repo.GetTree(commit.TreeHash)
		}
	}

	// Build map of files in current commit
	committedFiles := make(map[string]string)
	if currentTree != nil {
		for _, entry := range currentTree.Entries {
			committedFiles[entry.Name] = entry.Hash
		}
	}

	// Check staged files
	w.staging.mu.RLock()
	for path := range w.staging.entries {
		status.Staged = append(status.Staged, path)
	}
	for path := range w.staging.removed {
		status.Staged = append(status.Staged, path+" (deleted)")
	}
	w.staging.mu.RUnlock()

	// Check working directory files
	workingFiles, _ := w.working.List()
	for _, path := range workingFiles {
		// Skip if already staged
		w.staging.mu.RLock()
		_, isStaged := w.staging.entries[path]
		w.staging.mu.RUnlock()
		if isStaged {
			continue
		}

		// Check if file is tracked
		if commitHash, isTracked := committedFiles[path]; isTracked {
			// File is tracked, check if modified
			content, err := w.working.Read(path)
			if err == nil {
				blob := object.NewBlob(content)
				if blob.Hash() != commitHash {
					status.Modified = append(status.Modified, path)
				}
			}
		} else {
			// File is untracked
			status.Untracked = append(status.Untracked, path)
		}
	}

	// Sort for consistent output
	sort.Strings(status.Staged)
	sort.Strings(status.Modified)
	sort.Strings(status.Untracked)

	return status, nil
}

// Checkout switches branches and updates working directory
func (w *CleanWorkspace) Checkout(branch string) error {
	w.diag.Log("Checkout: Starting checkout to branch %s", branch)
	
	// Get branch commit
	commitHash, err := w.repo.GetBranch(branch)
	if err != nil {
		w.diag.LogError(fmt.Sprintf("GetBranch %s", branch), err)
		return fmt.Errorf("branch not found: %s", branch)
	}
	w.diag.Log("Checkout: Branch %s points to commit %s", branch, commitHash)

	// Get commit
	commit, err := w.repo.GetCommit(commitHash)
	if err != nil {
		w.diag.LogError("GetCommit", err)
		return err
	}
	w.diag.Log("Checkout: Got commit with tree %s", commit.TreeHash)

	// Get tree
	tree, err := w.repo.GetTree(commit.TreeHash)
	if err != nil {
		w.diag.LogError("GetTree", err)
		return err
	}
	w.diag.Log("Checkout: Tree has %d entries", len(tree.Entries))

	// Clear working directory
	if err := w.working.Clear(); err != nil {
		return err
	}

	// Restore files from tree
	for _, entry := range tree.Entries {
		blob, err := w.repo.GetBlob(entry.Hash)
		if err != nil {
			return err
		}

		if err := w.working.Write(entry.Name, blob.Content); err != nil {
			return err
		}
	}

	// Update current branch and staging area
	w.mu.Lock()
	w.branch = branch
	w.staging = NewCleanStagingArea()
	w.mu.Unlock()

	return nil
}

// Reset resets staging area
func (w *CleanWorkspace) Reset() {
	w.mu.Lock()
	w.staging = NewCleanStagingArea()
	w.mu.Unlock()
}

// GetStagingArea returns the current staging area
func (w *CleanWorkspace) GetStagingArea() *CleanStagingArea {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.staging
}

// Status represents the workspace status
type Status struct {
	Branch    string
	Staged    []string
	Modified  []string
	Untracked []string
}

// Clean checks if working directory is clean
func (s *Status) Clean() bool {
	return len(s.Staged) == 0 && len(s.Modified) == 0 && len(s.Untracked) == 0
}

// ReadFile reads a file from working directory
func (w *CleanWorkspace) ReadFile(path string) ([]byte, error) {
	return w.working.Read(path)
}

// WriteFile writes a file to working directory
func (w *CleanWorkspace) WriteFile(path string, content []byte) error {
	// Ensure directory exists for file-based storage
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		// This is a simplified version - real implementation would handle this better
		// For memory storage, directories don't matter
	}

	return w.working.Write(path, content)
}

// ListFiles returns all files in working directory
func (w *CleanWorkspace) ListFiles() ([]string, error) {
	return w.working.List()
}
