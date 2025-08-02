package core

import (
	"io"
	"time"

	"github.com/caiatech/govc/pkg/object"
)

// ObjectStore defines the interface for storing immutable Git objects
type ObjectStore interface {
	// Get retrieves an object by its hash
	Get(hash string) (object.Object, error)
	
	// Put stores an object and returns its hash
	Put(obj object.Object) (string, error)
	
	// Exists checks if an object exists
	Exists(hash string) bool
	
	// List returns all object hashes
	List() ([]string, error)
	
	// Size returns the total size of stored objects
	Size() (int64, error)
	
	// Close releases any resources
	io.Closer
}

// RefStore defines the interface for managing references
type RefStore interface {
	// GetRef returns the hash a reference points to
	GetRef(name string) (string, error)
	
	// UpdateRef updates a reference to point to a new hash
	UpdateRef(name string, hash string) error
	
	// DeleteRef removes a reference
	DeleteRef(name string) error
	
	// ListRefs returns all references
	ListRefs() (map[string]string, error)
	
	// GetHEAD returns what HEAD points to
	GetHEAD() (string, error)
	
	// SetHEAD updates HEAD
	SetHEAD(target string) error
	
	// Close releases any resources
	io.Closer
}

// WorkingStorage defines the interface for working directory storage
type WorkingStorage interface {
	// Read reads a file from working directory
	Read(path string) ([]byte, error)
	
	// Write writes a file to working directory
	Write(path string, data []byte) error
	
	// Delete removes a file from working directory
	Delete(path string) error
	
	// List returns all files in working directory
	List() ([]string, error)
	
	// Clear removes all files
	Clear() error
	
	// Exists checks if a file exists
	Exists(path string) bool
	
	// Close releases any resources
	io.Closer
}

// ConfigStore defines the interface for configuration storage
type ConfigStore interface {
	// Get retrieves a config value
	Get(key string) (string, error)
	
	// Set stores a config value
	Set(key, value string) error
	
	// Delete removes a config value
	Delete(key string) error
	
	// List returns all config keys
	List() (map[string]string, error)
	
	// Close releases any resources
	io.Closer
}

// Repository represents the core immutable Git repository
// It only handles objects and references, no mutable state
type Repository struct {
	objects ObjectStore
	refs    RefStore
}

// NewRepository creates a new repository with the given stores
func NewRepository(objects ObjectStore, refs RefStore) *Repository {
	return &Repository{
		objects: objects,
		refs:    refs,
	}
}

// Objects returns the object store
func (r *Repository) Objects() ObjectStore {
	return r.objects
}

// Refs returns the reference store
func (r *Repository) Refs() RefStore {
	return r.refs
}

// Workspace represents the mutable working directory state
type Workspace struct {
	repo     *Repository
	working  WorkingStorage
	staging  *StagingArea
}

// NewWorkspace creates a new workspace
func NewWorkspace(repo *Repository, working WorkingStorage) *Workspace {
	return &Workspace{
		repo:    repo,
		working: working,
		staging: NewStagingArea(),
	}
}

// StagingArea represents the Git staging area (index)
type StagingArea struct {
	entries map[string]StagedEntry
}

// StagedEntry represents a file in the staging area
type StagedEntry struct {
	Hash string
	Mode string
}

// NewStagingArea creates a new empty staging area
func NewStagingArea() *StagingArea {
	return &StagingArea{
		entries: make(map[string]StagedEntry),
	}
}


// Event represents a repository event
type Event struct {
	Type      string
	Timestamp time.Time
	Data      interface{}
}

// EventHandler handles repository events
type EventHandler func(event Event) error

// EventManager manages repository events and webhooks
type EventManager struct {
	handlers []EventHandler
}

// NewEventManager creates a new event manager
func NewEventManager() *EventManager {
	return &EventManager{
		handlers: make([]EventHandler, 0),
	}
}

// Subscribe adds an event handler
func (em *EventManager) Subscribe(handler EventHandler) {
	em.handlers = append(em.handlers, handler)
}

// Publish sends an event to all handlers
func (em *EventManager) Publish(event Event) {
	for _, handler := range em.handlers {
		// Run handlers asynchronously
		go handler(event)
	}
}