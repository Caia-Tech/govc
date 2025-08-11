package storage

import (
	"github.com/Caia-Tech/govc/pkg/object"
)

// StoreAdapter adapts the existing Store to implement ObjectStore interface
// This provides backward compatibility while transitioning to the new architecture
type StoreAdapter struct {
	store *Store
}

// NewStoreAdapter creates an ObjectStore adapter for the existing Store
func NewStoreAdapter(store *Store) *StoreAdapter {
	return &StoreAdapter{store: store}
}

func (a *StoreAdapter) Get(hash string) (object.Object, error) {
	return a.store.GetObject(hash)
}

func (a *StoreAdapter) Put(obj object.Object) (string, error) {
	if obj == nil {
		return "", InvalidObjectError("cannot store nil object")
	}
	return a.store.StoreObject(obj)
}

func (a *StoreAdapter) Exists(hash string) bool {
	return a.store.HasObject(hash)
}

func (a *StoreAdapter) List() ([]string, error) {
	return a.store.ListObjects()
}

func (a *StoreAdapter) Size() (int64, error) {
	// Return number of objects in cache as a rough size estimate
	hashes, err := a.store.cache.ListObjects()
	if err != nil {
		return 0, err
	}
	return int64(len(hashes)), nil
}

func (a *StoreAdapter) Close() error {
	// Store doesn't have a Close method, so nothing to do
	return nil
}

// NewObjectStoreFromBackend creates an ObjectStore from a Backend
func NewObjectStoreFromBackend(backend Backend) ObjectStore {
	store := NewStore(backend)
	return NewStoreAdapter(store)
}

// NewMemoryObjectStoreFromStore creates a memory-only ObjectStore
func NewMemoryObjectStoreFromStore() ObjectStore {
	backend := NewMemoryBackend()
	store := NewStore(backend)
	return NewStoreAdapter(store)
}

// NewFileObjectStore creates a file-backed ObjectStore
func NewFileObjectStore(path string) ObjectStore {
	backend := NewFileBackend(path)
	store := NewStore(backend)
	return NewStoreAdapter(store)
}
