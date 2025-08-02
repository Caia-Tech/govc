package core

import (
	"time"
	
	"github.com/caiatech/govc/pkg/object"
	"github.com/caiatech/govc/pkg/refs"
	"github.com/caiatech/govc/pkg/storage"
)

// MigrateRepository shows how to migrate from old Repository to new architecture
// This is a temporary helper during refactoring
func MigrateRepository(old interface{}) (*Repository, *Workspace, error) {
	// This would extract the stores from the old repository
	// and create the new clean separation
	
	// For now, return a simple example
	objStore := storage.NewMemoryObjectStore()
	refStore := storage.NewMemoryRefStore()
	
	repo := NewRepository(objStore, refStore)
	
	workingStore := storage.NewMemoryWorkingStorage()
	workspace := NewWorkspace(repo, workingStore)
	
	return repo, workspace, nil
}

// GitOperations provides high-level Git operations using the clean architecture
type GitOperations struct {
	repo      *Repository
	workspace *Workspace
	config    *Config
	events    *EventManager
}

// NewGitOperations creates a new Git operations handler
func NewGitOperations(repo *Repository, workspace *Workspace, config *Config) *GitOperations {
	return &GitOperations{
		repo:      repo,
		workspace: workspace,
		config:    config,
		events:    NewEventManager(),
	}
}

// Add stages a file (example of how operations would work)
func (g *GitOperations) Add(path string) error {
	// Read from working directory
	content, err := g.workspace.working.Read(path)
	if err != nil {
		return err
	}
	
	// Create blob object
	blob := object.NewBlob(content)
	
	// Store in object database
	hash, err := g.repo.objects.Put(blob)
	if err != nil {
		return err
	}
	
	// Add to staging area
	g.workspace.staging.entries[path] = StagedEntry{
		Hash: hash,
		Mode: "100644",
	}
	
	// Publish event
	g.events.Publish(Event{
		Type: "file.staged",
		Data: map[string]string{
			"path": path,
			"hash": hash,
		},
	})
	
	return nil
}

// Commit creates a new commit (example of clean separation)
func (g *GitOperations) Commit(message string) (string, error) {
	// Build tree from staging area
	tree := g.buildTreeFromStaging()
	
	// Store tree object
	treeHash, err := g.repo.objects.Put(tree)
	if err != nil {
		return "", err
	}
	
	// Get parent commit
	parentHash := ""
	headRef, err := g.repo.refs.GetHEAD()
	if err == nil && headRef != "" {
		// Resolve ref to commit hash
		if ref, err := g.repo.refs.GetRef(headRef); err == nil {
			parentHash = ref
		}
	}
	
	// Create commit object
	authorName, _ := g.config.store.Get("user.name")
	authorEmail, _ := g.config.store.Get("user.email")
	author := object.Author{
		Name:  authorName,
		Email: authorEmail,
		Time:  time.Now(),
	}
	
	commit := object.NewCommit(treeHash, author, message)
	if parentHash != "" {
		commit.SetParent(parentHash)
	}
	
	// Store commit
	commitHash, err := g.repo.objects.Put(commit)
	if err != nil {
		return "", err
	}
	
	// Update HEAD
	err = g.repo.refs.UpdateRef("HEAD", commitHash)
	if err != nil {
		return "", err
	}
	
	// Clear staging area
	g.workspace.staging = NewStagingArea()
	
	// Publish event
	g.events.Publish(Event{
		Type: "commit.created",
		Data: map[string]string{
			"hash":    commitHash,
			"message": message,
		},
	})
	
	return commitHash, nil
}

func (g *GitOperations) buildTreeFromStaging() *object.Tree {
	entries := make([]object.TreeEntry, 0, len(g.workspace.staging.entries))
	
	for path, staged := range g.workspace.staging.entries {
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

// Example of how to use adapters during migration
func AdaptOldRefManager(oldRefManager *refs.RefManager) RefStore {
	return storage.NewRefManagerAdapter(oldRefManager)
}

// Example of how to adapt old store
func AdaptOldStore(oldStore *storage.Store) ObjectStore {
	return storage.NewStoreAdapter(oldStore)
}