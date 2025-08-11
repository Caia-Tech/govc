package core

import (
	"fmt"
	"time"

	"github.com/Caia-Tech/govc/pkg/object"
	"github.com/google/uuid"
)

// StashManager manages repository stashes
type StashManager struct {
	repo      *CleanRepository
	workspace *CleanWorkspace
	stashes   []*Stash
}

// NewStashManager creates a new stash manager
func NewStashManager(repo *CleanRepository, workspace *CleanWorkspace) *StashManager {
	return &StashManager{
		repo:      repo,
		workspace: workspace,
		stashes:   make([]*Stash, 0),
	}
}

// Stash represents a saved workspace state
type Stash struct {
	ID           string
	Message      string
	Author       object.Author
	TreeHash     string // Hash of the stashed tree
	ParentCommit string // Commit hash when stash was created
	Timestamp    time.Time
	Changes      map[string][]byte // Unstaged changes
	Index        map[string]string // Staged changes (path -> hash)
	Untracked    map[string][]byte // Untracked files
}

// Create creates a new stash
func (sm *StashManager) Create(message string, includeUntracked bool) (*Stash, error) {
	// Get current HEAD
	head, err := sm.repo.GetHEAD()
	if err != nil {
		return nil, fmt.Errorf("cannot stash without commits")
	}

	// Get current branch
	currentBranch := sm.workspace.branch
	currentCommit, err := sm.repo.GetBranch(currentBranch)
	if err != nil {
		currentCommit = head
	}

	// Get status
	status, err := sm.workspace.Status()
	if err != nil {
		return nil, err
	}

	if status.Clean() {
		return nil, fmt.Errorf("no local changes to stash")
	}

	// Save staged changes
	staging := sm.workspace.GetStagingArea()
	index := make(map[string]string)
	for path, entry := range staging.entries {
		index[path] = entry.Hash
	}

	// Save unstaged changes
	changes := make(map[string][]byte)
	for _, path := range status.Modified {
		content, err := sm.workspace.ReadFile(path)
		if err == nil {
			changes[path] = content
		}
	}

	// Save untracked files if requested
	untrackedFiles := make(map[string][]byte)
	if includeUntracked {
		for _, path := range status.Untracked {
			content, err := sm.workspace.ReadFile(path)
			if err == nil {
				untrackedFiles[path] = content
			}
		}
	}

	// Create tree from current state
	tree := sm.buildTreeFromWorkspace()
	treeHash, err := sm.repo.objects.Put(tree)
	if err != nil {
		return nil, err
	}

	// Create stash
	stash := &Stash{
		ID:      uuid.New().String(),
		Message: message,
		Author: object.Author{
			Name:  "Stash",
			Email: "stash@govc",
			Time:  time.Now(),
		},
		TreeHash:     treeHash,
		ParentCommit: currentCommit,
		Timestamp:    time.Now(),
		Changes:      changes,
		Index:        index,
		Untracked:    untrackedFiles,
	}

	sm.stashes = append(sm.stashes, stash)

	// Reset workspace to clean state
	sm.workspace.Reset()

	// Also need to restore files to their committed state
	// Get the committed tree
	commit, _ := sm.repo.GetCommit(currentCommit)
	var committedFiles map[string]string
	if commit != nil {
		tree, _ := sm.repo.GetTree(commit.TreeHash)
		if tree != nil {
			committedFiles = make(map[string]string)
			for _, entry := range tree.Entries {
				committedFiles[entry.Name] = entry.Hash
			}
		}
	}

	// Restore modified files to committed state
	for path := range changes {
		if hash, exists := committedFiles[path]; exists {
			// Restore original content
			blob, err := sm.repo.GetBlob(hash)
			if err == nil {
				sm.workspace.WriteFile(path, blob.Content)
			}
		}
	}

	// Remove newly added files that were staged
	for path := range index {
		if _, exists := committedFiles[path]; !exists {
			// This was a new file, remove it
			sm.workspace.working.Delete(path)
		}
	}

	// Remove untracked files if they were included
	for path := range untrackedFiles {
		sm.workspace.working.Delete(path)
	}

	return stash, nil
}

// List returns all stashes
func (sm *StashManager) List() []*Stash {
	return sm.stashes
}

// Get returns a specific stash
func (sm *StashManager) Get(id string) (*Stash, error) {
	for _, stash := range sm.stashes {
		if stash.ID == id {
			return stash, nil
		}
	}
	return nil, fmt.Errorf("stash not found: %s", id)
}

// Apply applies a stash to the current workspace
func (sm *StashManager) Apply(id string) error {
	stash, err := sm.Get(id)
	if err != nil {
		return err
	}

	// Check if workspace is clean
	status, err := sm.workspace.Status()
	if err != nil {
		return err
	}

	if !status.Clean() {
		return fmt.Errorf("cannot apply stash: workspace has uncommitted changes")
	}

	// Restore staged changes
	for path, hash := range stash.Index {
		// The object already exists in the repository
		sm.workspace.staging.entries[path] = StagedEntry{
			Hash: hash,
			Mode: "100644",
		}
	}

	// Restore unstaged changes
	for path, content := range stash.Changes {
		if err := sm.workspace.WriteFile(path, content); err != nil {
			return err
		}
	}

	// Restore untracked files
	for path, content := range stash.Untracked {
		if err := sm.workspace.WriteFile(path, content); err != nil {
			return err
		}
	}

	return nil
}

// Drop removes a stash
func (sm *StashManager) Drop(id string) error {
	for i, stash := range sm.stashes {
		if stash.ID == id {
			// Remove from slice
			sm.stashes = append(sm.stashes[:i], sm.stashes[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("stash not found: %s", id)
}

// Pop applies and drops a stash
func (sm *StashManager) Pop(id string) error {
	if err := sm.Apply(id); err != nil {
		return err
	}
	return sm.Drop(id)
}

// buildTreeFromWorkspace creates a tree from the current workspace state
func (sm *StashManager) buildTreeFromWorkspace() *object.Tree {
	staging := sm.workspace.GetStagingArea()
	entries := make([]object.TreeEntry, 0, len(staging.entries))

	for path, staged := range staging.entries {
		entries = append(entries, object.TreeEntry{
			Mode: staged.Mode,
			Name: path,
			Hash: staged.Hash,
		})
	}

	tree := object.NewTree()
	tree.Entries = entries
	return tree
}
