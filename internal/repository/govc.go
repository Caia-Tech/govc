// Package govc provides a memory-first Git implementation that enables
// parallel realities, transactional commits, and reactive infrastructure.
//
// govc reimagines Git as a memory-first reality engine where branches are
// lightweight parallel universes and commits trigger reactive events.
//
// Basic usage:
//
//	repo := govc.New()
//
//	// Create parallel realities for testing
//	realities := repo.ParallelRealities([]string{"test-a", "test-b", "test-c"})
//
//	// Transactional commits
//	tx := repo.Transaction()
//	tx.Add("config.yaml", []byte("version: 2.0"))
//	if err := tx.Validate(); err == nil {
//	    tx.Commit("Updated config")
//	}
//
//	// Watch for changes
//	repo.Watch(func(event CommitEvent) {
//	    fmt.Printf("New commit: %s\n", event.Message)
//	})
package repository

import (
	"github.com/Caia-Tech/govc/pkg/object"
	"github.com/Caia-Tech/govc/pkg/refs"
	"github.com/Caia-Tech/govc/pkg/storage"
)

// New creates a memory-first repository that operates entirely in RAM.
// This is the recommended way to use govc for infrastructure management.
func New() *Repository {
	return NewRepository()
}

// Init initializes a new repository at the given path with optional persistence.
// Use this when you need Git compatibility or want to persist state.
func Init(path string) (*Repository, error) {
	return InitRepository(path)
}

// Open opens an existing repository from disk.
func Open(path string) (*Repository, error) {
	return OpenRepository(path)
}

// NewMemoryStore creates a pure in-memory object store.
// Useful for testing or temporary operations.
func NewMemoryStore() *storage.Store {
	backend := storage.NewMemoryBackend()
	return storage.NewStore(backend)
}

// Config holds repository configuration options.
type Config struct {
	// Memory-first operation (no disk I/O)
	MemoryOnly bool

	// Enable event streaming
	EventStream bool

	// Maximum parallel realities
	MaxRealities int

	// Author information
	Author ConfigAuthor
}

// ConfigAuthor holds author configuration.
type ConfigAuthor struct {
	Name  string
	Email string
}

// NewWithConfig creates a repository with custom configuration.
func NewWithConfig(config Config) *Repository {
	repo := NewRepository()

	if config.Author.Name != "" {
		repo.SetConfig("user.name", config.Author.Name)
	}
	if config.Author.Email != "" {
		repo.SetConfig("user.email", config.Author.Email)
	}

	return repo
}

// Reality represents a parallel reality (isolated branch universe).
// This is the key abstraction that enables testing multiple states simultaneously.
type Reality = ParallelReality

// Transaction represents a transactional commit that can be validated
// before persisting. This ensures infrastructure changes are safe.
type Transaction = TransactionalCommit

// Event represents a commit event in the repository.
// Use this to build reactive infrastructure systems.
type Event = CommitEvent

// Snapshot represents a point-in-time view of the repository.
// Use TimeTravel() to create snapshots for debugging or analysis.
type Snapshot = HistoricalSnapshot

// Object types re-exported for library users
type (
	Blob   = object.Blob
	Tree   = object.Tree
	Commit = object.Commit
	Tag    = object.Tag
	Author = object.Author
)

// Reference types re-exported for library users
type (
	Ref        = refs.Ref
	RefManager = refs.RefManager
)

// QuickStart provides a simple example of using govc as a library.
func QuickStart() {
	// Create a memory-first repository
	repo := New()

	// Start a transaction
	tx := repo.Transaction()
	tx.Add("config.yaml", []byte("version: 1.0"))
	tx.Add("app.yaml", []byte("name: myapp"))

	// Validate before committing
	if err := tx.Validate(); err != nil {
		// Rollback if validation fails
		tx.Rollback()
		return
	}

	// Commit the transaction
	tx.Commit("Initial configuration")

	// Create parallel realities for testing
	realities := repo.ParallelRealities([]string{"staging", "production"})

	// Test changes in staging
	staging := realities[0]
	staging.Apply(map[string][]byte{
		"config.yaml": []byte("version: 2.0-beta"),
	})

	// If tests pass, merge to main
	if staging.Benchmark().Better() {
		repo.Merge(staging.Name(), "main")
	}
}
