package storage

import (
	"github.com/caiatech/govc/pkg/object"
)

// ObjectStore handles immutable Git objects (commits, trees, blobs)
// This is the core of Git's content-addressable storage
type ObjectStore interface {
	// Get retrieves an object by its hash
	Get(hash string) (object.Object, error)
	
	// Put stores an object and returns its hash
	Put(obj object.Object) (string, error)
	
	// Exists checks if an object exists without retrieving it
	Exists(hash string) bool
	
	// List returns all object hashes (for debugging/maintenance)
	List() ([]string, error)
	
	// Size returns the storage size metrics
	Size() (int64, error)
	
	// Close releases any resources held by the store
	Close() error
}

// RefStore handles mutable references (branches, tags, HEAD)
// These are pointers to commits in the object store
type RefStore interface {
	// GetRef returns the hash that a reference points to
	GetRef(name string) (string, error)
	
	// UpdateRef updates a reference to point to a new hash
	UpdateRef(name string, hash string) error
	
	// DeleteRef removes a reference
	DeleteRef(name string) error
	
	// ListRefs returns all references with their target hashes
	ListRefs() (map[string]string, error)
	
	// GetHEAD returns what HEAD points to (branch name or commit hash)
	GetHEAD() (string, error)
	
	// SetHEAD updates HEAD to point to a branch or commit
	SetHEAD(target string) error
	
	// Close releases any resources held by the store
	Close() error
}

// WorkingStorage handles mutable working directory content
// This is separate from Git objects and can be cleared/restored
type WorkingStorage interface {
	// Read retrieves file content from the working directory
	Read(path string) ([]byte, error)
	
	// Write stores file content in the working directory
	Write(path string, data []byte) error
	
	// Delete removes a file from the working directory
	Delete(path string) error
	
	// List returns all files in the working directory
	List() ([]string, error)
	
	// Clear removes all files from the working directory
	Clear() error
	
	// Exists checks if a file exists
	Exists(path string) bool
	
	// Close releases any resources held by the storage
	Close() error
}

// StorageFactory creates storage instances
// This allows different storage backends (memory, disk, hybrid)
type StorageFactory interface {
	// CreateObjectStore creates an object store with the given configuration
	CreateObjectStore(config ObjectStoreConfig) (ObjectStore, error)
	
	// CreateRefStore creates a reference store with the given configuration
	CreateRefStore(config RefStoreConfig) (RefStore, error)
	
	// CreateWorkingStorage creates working storage with the given configuration
	CreateWorkingStorage(config WorkingStorageConfig) (WorkingStorage, error)
}

// Configuration types for different storage backends

type ObjectStoreConfig struct {
	Type        string                 // "memory", "disk", "hybrid"
	Path        string                // For disk storage
	MaxMemory   int64                 // For hybrid storage
	Options     map[string]interface{} // Backend-specific options
}

type RefStoreConfig struct {
	Type        string                 // "memory", "disk", "hybrid"
	Path        string                // For disk storage
	Options     map[string]interface{} // Backend-specific options
}

type WorkingStorageConfig struct {
	Type        string                 // "memory", "disk", "hybrid"
	Path        string                // For disk storage
	Options     map[string]interface{} // Backend-specific options
}

// Errors for storage operations
type Error struct {
	Type    string // "not_found", "invalid_object", "io_error", etc.
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// Common error constructors
func NotFoundError(message string) *Error {
	return &Error{Type: "not_found", Message: message}
}

func InvalidObjectError(message string) *Error {
	return &Error{Type: "invalid_object", Message: message}
}

func IOError(message string, cause error) *Error {
	return &Error{Type: "io_error", Message: message, Cause: cause}
}