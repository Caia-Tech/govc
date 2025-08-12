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