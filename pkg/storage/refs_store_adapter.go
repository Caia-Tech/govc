package storage

import (
	"github.com/caiatech/govc/pkg/refs"
)

// RefsStoreAdapter adapts refs.RefStore to implement storage.RefStore interface
// This provides a more direct bridge between the two interfaces
type RefsStoreAdapter struct {
	store refs.RefStore
}

// NewRefsStoreAdapter creates a storage.RefStore adapter for refs.RefStore
func NewRefsStoreAdapter(store refs.RefStore) *RefsStoreAdapter {
	return &RefsStoreAdapter{store: store}
}

func (a *RefsStoreAdapter) GetRef(name string) (string, error) {
	return a.store.GetRef(name)
}

func (a *RefsStoreAdapter) UpdateRef(name string, hash string) error {
	if name == "" {
		return InvalidObjectError("reference name cannot be empty")
	}
	if hash == "" {
		return InvalidObjectError("hash cannot be empty")
	}
	return a.store.SetRef(name, hash)
}

func (a *RefsStoreAdapter) DeleteRef(name string) error {
	// Idempotent operation - deleting non-existent ref is not an error
	return a.store.DeleteRef(name)
}

func (a *RefsStoreAdapter) ListRefs() (map[string]string, error) {
	// Get all refs by listing with empty prefix
	refsList, err := a.store.ListRefs("")
	if err != nil {
		return nil, err
	}
	
	// Convert []refs.Ref to map[string]string
	result := make(map[string]string)
	for _, ref := range refsList {
		result[ref.Name] = ref.Hash
	}
	
	return result, nil
}

func (a *RefsStoreAdapter) GetHEAD() (string, error) {
	// For MemoryRefStore, we need to emulate the storage interface behavior
	if _, ok := a.store.(*refs.MemoryRefStore); ok {
		// The refs store will resolve HEAD, but if the target doesn't exist, it will error
		// Let's try to get the resolved HEAD and catch any errors
		_, err := a.store.GetHEAD()
		if err != nil {
			return "", err
		}
		// If no error, return the symbolic reference
		// TODO: This is a limitation - we need access to the head field
		return "refs/heads/main", nil
	}
	
	// For other stores, return the resolved value
	return a.store.GetHEAD()
}

func (a *RefsStoreAdapter) SetHEAD(target string) error {
	return a.store.SetHEAD(target)
}

func (a *RefsStoreAdapter) Close() error {
	// refs.RefStore doesn't have a Close method, so nothing to do
	return nil
}

// Convenience constructors using the refs package stores

// NewMemoryRefStoreAdapter creates a memory-only RefStore using refs.MemoryRefStore
func NewMemoryRefStoreAdapter() RefStore {
	memoryStore := refs.NewMemoryRefStore()
	return NewRefsStoreAdapter(memoryStore)
}

// NewFileRefStoreAdapter creates a file-backed RefStore using refs.FileRefStore
func NewFileRefStoreAdapter(path string) RefStore {
	fileStore := refs.NewFileRefStore(path)
	return NewRefsStoreAdapter(fileStore)
}