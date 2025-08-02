package core

import (
	"fmt"
	"sync"

	"github.com/caiatech/govc/pkg/object"
	"github.com/caiatech/govc/pkg/refs"
	"github.com/caiatech/govc/pkg/storage"
)

// ObjectStoreAdapter adapts existing storage.Store to ObjectStore interface
type ObjectStoreAdapter struct {
	Store *storage.Store
}

func (a *ObjectStoreAdapter) Get(hash string) (object.Object, error) {
	return a.Store.GetObject(hash)
}

func (a *ObjectStoreAdapter) Put(obj object.Object) (string, error) {
	return a.Store.StoreObject(obj)
}

func (a *ObjectStoreAdapter) Exists(hash string) bool {
	_, err := a.Store.GetObject(hash)
	return err == nil
}

func (a *ObjectStoreAdapter) List() ([]string, error) {
	// This would need to be implemented in storage.Store
	return nil, fmt.Errorf("list not implemented")
}

func (a *ObjectStoreAdapter) Size() (int64, error) {
	// This would need to be implemented in storage.Store
	return 0, fmt.Errorf("size not implemented")
}

func (a *ObjectStoreAdapter) Close() error {
	// Storage.Store doesn't have Close
	return nil
}

// RefStoreAdapter adapts refs.RefManager to RefStore interface
type RefStoreAdapter struct {
	RefManager *refs.RefManager
	Store      refs.RefStore
}

func (a *RefStoreAdapter) GetRef(name string) (string, error) {
	return a.Store.GetRef(name)
}

func (a *RefStoreAdapter) UpdateRef(name, hash string) error {
	return a.Store.SetRef(name, hash)
}

func (a *RefStoreAdapter) DeleteRef(name string) error {
	return a.Store.DeleteRef(name)
}

func (a *RefStoreAdapter) ListRefs() (map[string]string, error) {
	refs, err := a.Store.ListRefs("")
	if err != nil {
		return nil, err
	}
	
	result := make(map[string]string)
	for _, ref := range refs {
		result[ref.Name] = ref.Hash
	}
	return result, nil
}

func (a *RefStoreAdapter) GetHEAD() (string, error) {
	return a.RefManager.GetHEAD()
}

func (a *RefStoreAdapter) SetHEAD(ref string) error {
	// Try to set as branch first
	if err := a.RefManager.SetHEADToBranch(ref); err == nil {
		return nil
	}
	// Otherwise set as detached
	return a.RefManager.SetHEADToCommit(ref)
}

func (a *RefStoreAdapter) Close() error {
	// RefManager doesn't have Close
	return nil
}

// MemoryObjectStore is a simple in-memory implementation of ObjectStore
type MemoryObjectStore struct {
	mu      sync.RWMutex
	objects map[string]object.Object
}

func NewMemoryObjectStore() *MemoryObjectStore {
	return &MemoryObjectStore{
		objects: make(map[string]object.Object),
	}
}

func (m *MemoryObjectStore) Get(hash string) (object.Object, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	obj, ok := m.objects[hash]
	if !ok {
		return nil, fmt.Errorf("object not found: %s", hash)
	}
	return obj, nil
}

func (m *MemoryObjectStore) Put(obj object.Object) (string, error) {
	hash := obj.Hash()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[hash] = obj
	return hash, nil
}

func (m *MemoryObjectStore) Exists(hash string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.objects[hash]
	return ok
}

func (m *MemoryObjectStore) List() ([]string, error) {
	hashes := make([]string, 0, len(m.objects))
	for hash := range m.objects {
		hashes = append(hashes, hash)
	}
	return hashes, nil
}

func (m *MemoryObjectStore) Size() (int64, error) {
	var size int64
	for _, obj := range m.objects {
		size += obj.Size()
	}
	return size, nil
}

func (m *MemoryObjectStore) Close() error {
	return nil
}

// MemoryRefStore is a simple in-memory implementation of RefStore
type MemoryRefStore struct {
	refs map[string]string
	head string
}

func NewMemoryRefStore() *MemoryRefStore {
	return &MemoryRefStore{
		refs: make(map[string]string),
		head: "refs/heads/main",
	}
}

func (m *MemoryRefStore) GetRef(name string) (string, error) {
	hash, ok := m.refs[name]
	if !ok {
		return "", fmt.Errorf("ref not found: %s", name)
	}
	return hash, nil
}

func (m *MemoryRefStore) UpdateRef(name, hash string) error {
	m.refs[name] = hash
	return nil
}

func (m *MemoryRefStore) DeleteRef(name string) error {
	delete(m.refs, name)
	return nil
}

func (m *MemoryRefStore) ListRefs() (map[string]string, error) {
	result := make(map[string]string)
	for k, v := range m.refs {
		result[k] = v
	}
	return result, nil
}

func (m *MemoryRefStore) GetHEAD() (string, error) {
	return m.head, nil
}

func (m *MemoryRefStore) SetHEAD(ref string) error {
	m.head = ref
	return nil
}

func (m *MemoryRefStore) Close() error {
	return nil
}

// MemoryWorkingStorage is an in-memory implementation of WorkingStorage
type MemoryWorkingStorage struct {
	mu    sync.RWMutex
	files map[string][]byte
}

func NewMemoryWorkingStorage() *MemoryWorkingStorage {
	return &MemoryWorkingStorage{
		files: make(map[string][]byte),
	}
}

func (m *MemoryWorkingStorage) Read(path string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	content, ok := m.files[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return content, nil
}

func (m *MemoryWorkingStorage) Write(path string, content []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = content
	return nil
}

func (m *MemoryWorkingStorage) Delete(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, path)
	return nil
}

func (m *MemoryWorkingStorage) List() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	paths := make([]string, 0, len(m.files))
	for path := range m.files {
		paths = append(paths, path)
	}
	return paths, nil
}

func (m *MemoryWorkingStorage) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files = make(map[string][]byte)
	return nil
}

func (m *MemoryWorkingStorage) Exists(path string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.files[path]
	return ok
}

func (m *MemoryWorkingStorage) Close() error {
	return nil
}

// FileWorkingStorage is a file-based implementation of WorkingStorage
type FileWorkingStorage struct {
	root string
}

func NewFileWorkingStorage(root string) *FileWorkingStorage {
	return &FileWorkingStorage{root: root}
}

func (f *FileWorkingStorage) Read(path string) ([]byte, error) {
	// Implementation would read from filesystem
	return nil, fmt.Errorf("not implemented")
}

func (f *FileWorkingStorage) Write(path string, content []byte) error {
	// Implementation would write to filesystem
	return fmt.Errorf("not implemented")
}

func (f *FileWorkingStorage) Delete(path string) error {
	// Implementation would delete from filesystem
	return fmt.Errorf("not implemented")
}

func (f *FileWorkingStorage) List() ([]string, error) {
	// Implementation would list files
	return nil, fmt.Errorf("not implemented")
}

func (f *FileWorkingStorage) Clear() error {
	// Implementation would clear working directory
	return fmt.Errorf("not implemented")
}

func (f *FileWorkingStorage) Exists(path string) bool {
	// Implementation would check filesystem
	return false
}

func (f *FileWorkingStorage) Close() error {
	return nil
}

// MemoryConfigStore is an in-memory config store
type MemoryConfigStore struct {
	values map[string]string
}

func NewMemoryConfigStore() *MemoryConfigStore {
	return &MemoryConfigStore{
		values: make(map[string]string),
	}
}

func (m *MemoryConfigStore) Get(key string) (string, error) {
	value, ok := m.values[key]
	if !ok {
		return "", fmt.Errorf("key not found: %s", key)
	}
	return value, nil
}

func (m *MemoryConfigStore) Set(key, value string) error {
	m.values[key] = value
	return nil
}

func (m *MemoryConfigStore) Delete(key string) error {
	delete(m.values, key)
	return nil
}

func (m *MemoryConfigStore) List() (map[string]string, error) {
	result := make(map[string]string)
	for k, v := range m.values {
		result[k] = v
	}
	return result, nil
}

func (m *MemoryConfigStore) Close() error {
	return nil
}