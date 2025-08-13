// Package govc provides a memory-first version control system
package govc

import (
	"github.com/Caia-Tech/govc/internal/repository"
	"github.com/Caia-Tech/govc/pkg/object"
)

// Repository is the main interface for govc operations
type Repository = repository.Repository

// NewRepository creates a new govc repository
func NewRepository() *Repository {
	return repository.New()
}

// LoadRepository loads or creates a repository at the specified path
func LoadRepository(repoPath string) (*Repository, error) {
	return repository.LoadRepository(repoPath)
}

// Core types re-exported for public API
type (
	Commit = object.Commit
	Author = object.Author
	Object = object.Object
	Blob   = object.Blob
	Tree   = object.Tree
)

// TransactionalCommit re-exported for public API
type TransactionalCommit = repository.TransactionalCommit

// Additional types for compatibility
type (
	ParallelReality = repository.ParallelReality
	CommitEvent     = repository.CommitEvent
	EventBus        = repository.EventBus
)

// Event represents a generic repository event
type Event interface {
	Type() string
	Data() interface{}
}

// Legacy function names for compatibility
func New() *Repository {
	return NewRepository()
}

func Open(path string) (*Repository, error) {
	return LoadRepository(path)
}

func Init(path string) (*Repository, error) {
	return LoadRepository(path)
}

// Config types
type Config struct {
	Author Author
}

type ConfigAuthor = Author

// NewWithConfig creates repository with config
func NewWithConfig(cfg Config) *Repository {
	return NewRepository()
}