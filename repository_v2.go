package govc

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/caiatech/govc/pkg/core"
	"github.com/caiatech/govc/pkg/object"
	"github.com/caiatech/govc/pkg/refs"
	"github.com/caiatech/govc/pkg/storage"
)

// RepositoryV2 is the new version using clean architecture
// This separates concerns between immutable repository operations
// and mutable workspace operations
type RepositoryV2 struct {
	path       string
	repo       *core.CleanRepository
	workspace  *core.CleanWorkspace
	operations *core.Operations
	config     *core.Config

	// Legacy features to be extracted
	// stashes    []*Stash
	// webhooks   map[string]*Webhook
	// events     chan *RepositoryEvent
	mu sync.RWMutex
}

// InitRepositoryV2 creates a new repository with clean architecture
func InitRepositoryV2(path string) (*RepositoryV2, error) {
	var objects core.ObjectStore
	var refStore core.RefStore
	var working core.WorkingStorage

	if path == ":memory:" {
		// Pure memory operation
		objects = core.NewMemoryObjectStore()
		refStore = core.NewMemoryRefStore()
		working = core.NewMemoryWorkingStorage()
	} else {
		// File-backed operation
		gitDir := filepath.Join(path, ".govc")
		if err := os.MkdirAll(gitDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create .govc directory: %v", err)
		}

		objectsDir := filepath.Join(gitDir, "objects")
		if err := os.MkdirAll(objectsDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create objects directory: %v", err)
		}

		refsDir := filepath.Join(gitDir, "refs", "heads")
		if err := os.MkdirAll(refsDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create refs directory: %v", err)
		}

		// Use adapters for existing implementations
		backend := storage.NewFileBackend(gitDir)
		store := storage.NewStore(backend)
		objects = &core.ObjectStoreAdapter{Store: store}

		fileRefStore := refs.NewFileRefStore(gitDir)
		refManager := refs.NewRefManager(fileRefStore)
		refStore = &core.RefStoreAdapter{RefManager: refManager, Store: fileRefStore}

		working = core.NewFileWorkingStorage(path)
	}

	// Create clean architecture components
	repo := core.NewCleanRepository(objects, refStore)
	workspace := core.NewCleanWorkspace(repo, working)
	config := core.NewConfig(core.NewMemoryConfigStore())
	operations := core.NewOperations(repo, workspace, config)

	// Initialize repository
	if err := operations.Init(); err != nil {
		return nil, err
	}

	return &RepositoryV2{
		path:       path,
		repo:       repo,
		workspace:  workspace,
		operations: operations,
		config:     config,
		// stashes:    make([]*Stash, 0),
		// webhooks:   make(map[string]*Webhook),
		// events:     make(chan *RepositoryEvent, 100),
	}, nil
}

// OpenRepositoryV2 opens an existing repository
func OpenRepositoryV2(path string) (*RepositoryV2, error) {
	if path == ":memory:" {
		return nil, fmt.Errorf("cannot open memory repository")
	}

	gitDir := filepath.Join(path, ".govc")
	if _, err := os.Stat(gitDir); err != nil {
		return nil, fmt.Errorf("not a govc repository: %v", err)
	}

	// Reuse initialization logic
	return InitRepositoryV2(path)
}

// NewMemoryRepositoryV2 creates a pure in-memory repository
func NewMemoryRepositoryV2() *RepositoryV2 {
	repo, _ := InitRepositoryV2(":memory:")
	return repo
}

// Adapter methods to maintain backward compatibility
func (r *RepositoryV2) Add(paths ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.operations.Add(paths...)
}

func (r *RepositoryV2) Commit(message string, author *object.Author) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Set author config if provided
	if author != nil {
		r.config.Set("user.name", author.Name)
		r.config.Set("user.email", author.Email)
	}

	return r.operations.Commit(message)
}

func (r *RepositoryV2) CreateBranch(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.operations.Branch(name)
}

func (r *RepositoryV2) Checkout(ref string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.operations.Checkout(ref)
}

func (r *RepositoryV2) Status() (*core.Status, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.operations.Status()
}

func (r *RepositoryV2) Log(limit int) ([]*object.Commit, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.operations.Log(limit)
}

func (r *RepositoryV2) CreateTag(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.operations.Tag(name)
}

func (r *RepositoryV2) ListBranches() ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	branches, err := r.repo.ListBranches()
	if err != nil {
		return nil, err
	}

	names := make([]string, len(branches))
	for i, b := range branches {
		names[i] = b.Name
	}
	return names, nil
}

func (r *RepositoryV2) ListTags() ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tags, err := r.repo.ListTags()
	if err != nil {
		return nil, err
	}

	names := make([]string, len(tags))
	for i, t := range tags {
		names[i] = t.Name
	}
	return names, nil
}

// Merge performs a merge (fast-forward only for now)
func (r *RepositoryV2) Merge(branch, into string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Checkout target branch
	if err := r.operations.Checkout(into); err != nil {
		return err
	}

	// Merge source branch
	_, err := r.operations.Merge(branch, fmt.Sprintf("Merge branch '%s' into %s", branch, into))
	return err
}

// File operations
func (r *RepositoryV2) ReadFile(path string) ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.workspace.ReadFile(path)
}

func (r *RepositoryV2) WriteFile(path string, content []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.workspace.WriteFile(path, content)
}

// Config operations
func (r *RepositoryV2) SetConfig(key, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config.Set(key, value)
}

func (r *RepositoryV2) GetConfig(key string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, _ := r.config.Get(key)
	return value
}

// IsMemoryOnly returns true if this is a pure memory repository
func (r *RepositoryV2) IsMemoryOnly() bool {
	return r.path == ":memory:"
}

// GetPath returns the repository path
func (r *RepositoryV2) GetPath() string {
	return r.path
}
