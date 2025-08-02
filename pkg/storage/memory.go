package storage

import (
	"fmt"
	"sort"
	"sync"

	"github.com/caiatech/govc/pkg/object"
)

// MemoryObjectStore implements ObjectStore in memory
type MemoryObjectStore struct {
	objects map[string]object.Object
	mu      sync.RWMutex
}

// NewMemoryObjectStore creates a new in-memory object store
func NewMemoryObjectStore() *MemoryObjectStore {
	return &MemoryObjectStore{
		objects: make(map[string]object.Object),
	}
}

func (s *MemoryObjectStore) Get(hash string) (object.Object, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	obj, exists := s.objects[hash]
	if !exists {
		return nil, NotFoundError(fmt.Sprintf("object %s not found", hash))
	}
	
	return obj, nil
}

func (s *MemoryObjectStore) Put(obj object.Object) (string, error) {
	if obj == nil {
		return "", InvalidObjectError("cannot store nil object")
	}
	
	hash := obj.Hash()
	if hash == "" {
		return "", InvalidObjectError("object has empty hash")
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.objects[hash] = obj
	return hash, nil
}

func (s *MemoryObjectStore) Exists(hash string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	_, exists := s.objects[hash]
	return exists
}

func (s *MemoryObjectStore) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	hashes := make([]string, 0, len(s.objects))
	for hash := range s.objects {
		hashes = append(hashes, hash)
	}
	
	sort.Strings(hashes)
	return hashes, nil
}

func (s *MemoryObjectStore) Size() (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Rough size calculation - in a real implementation we'd track bytes
	return int64(len(s.objects)), nil
}

func (s *MemoryObjectStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Clear the objects map to help with garbage collection
	s.objects = nil
	return nil
}

// MemoryRefStore implements RefStore in memory
type MemoryRefStore struct {
	refs map[string]string // ref name -> commit hash
	head string            // what HEAD points to
	mu   sync.RWMutex
}

// NewMemoryRefStore creates a new in-memory reference store
func NewMemoryRefStore() *MemoryRefStore {
	return &MemoryRefStore{
		refs: make(map[string]string),
		head: "refs/heads/main", // Default to main branch
	}
}

func (s *MemoryRefStore) GetRef(name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	hash, exists := s.refs[name]
	if !exists {
		return "", NotFoundError(fmt.Sprintf("reference %s not found", name))
	}
	
	return hash, nil
}

func (s *MemoryRefStore) UpdateRef(name string, hash string) error {
	if name == "" {
		return InvalidObjectError("reference name cannot be empty")
	}
	if hash == "" {
		return InvalidObjectError("hash cannot be empty")
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.refs[name] = hash
	return nil
}

func (s *MemoryRefStore) DeleteRef(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Idempotent operation - deleting non-existent ref is not an error
	delete(s.refs, name)
	return nil
}

func (s *MemoryRefStore) ListRefs() (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Return a copy to prevent external modification
	refs := make(map[string]string, len(s.refs))
	for name, hash := range s.refs {
		refs[name] = hash
	}
	
	return refs, nil
}

func (s *MemoryRefStore) GetHEAD() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return s.head, nil
}

func (s *MemoryRefStore) SetHEAD(target string) error {
	if target == "" {
		return InvalidObjectError("HEAD target cannot be empty")
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.head = target
	return nil
}

func (s *MemoryRefStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.refs = nil
	return nil
}

// MemoryWorkingStorage implements WorkingStorage in memory
type MemoryWorkingStorage struct {
	files map[string][]byte
	mu    sync.RWMutex
}

// NewMemoryWorkingStorage creates a new in-memory working storage
func NewMemoryWorkingStorage() *MemoryWorkingStorage {
	return &MemoryWorkingStorage{
		files: make(map[string][]byte),
	}
}

func (s *MemoryWorkingStorage) Read(path string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	data, exists := s.files[path]
	if !exists {
		return nil, NotFoundError(fmt.Sprintf("file %s not found", path))
	}
	
	// Return a copy to prevent external modification
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

func (s *MemoryWorkingStorage) Write(path string, data []byte) error {
	if path == "" {
		return InvalidObjectError("file path cannot be empty")
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Store a copy to prevent external modification
	fileCopy := make([]byte, len(data))
	copy(fileCopy, data)
	s.files[path] = fileCopy
	
	return nil
}

func (s *MemoryWorkingStorage) Delete(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, exists := s.files[path]; !exists {
		return NotFoundError(fmt.Sprintf("file %s not found", path))
	}
	
	delete(s.files, path)
	return nil
}

func (s *MemoryWorkingStorage) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	paths := make([]string, 0, len(s.files))
	for path := range s.files {
		paths = append(paths, path)
	}
	
	sort.Strings(paths)
	return paths, nil
}

func (s *MemoryWorkingStorage) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Clear all files
	for path := range s.files {
		delete(s.files, path)
	}
	
	return nil
}

func (s *MemoryWorkingStorage) Exists(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	_, exists := s.files[path]
	return exists
}

func (s *MemoryWorkingStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.files = nil
	return nil
}

// MemoryStorageFactory creates memory-based storage instances
type MemoryStorageFactory struct{}

// NewMemoryStorageFactory creates a new memory storage factory
func NewMemoryStorageFactory() *MemoryStorageFactory {
	return &MemoryStorageFactory{}
}

func (f *MemoryStorageFactory) CreateObjectStore(config ObjectStoreConfig) (ObjectStore, error) {
	if config.Type != "memory" && config.Type != "" {
		return nil, fmt.Errorf("unsupported object store type: %s", config.Type)
	}
	return NewMemoryObjectStore(), nil
}

func (f *MemoryStorageFactory) CreateRefStore(config RefStoreConfig) (RefStore, error) {
	if config.Type != "memory" && config.Type != "" {
		return nil, fmt.Errorf("unsupported ref store type: %s", config.Type)
	}
	return NewMemoryRefStore(), nil
}

func (f *MemoryStorageFactory) CreateWorkingStorage(config WorkingStorageConfig) (WorkingStorage, error) {
	if config.Type != "memory" && config.Type != "" {
		return nil, fmt.Errorf("unsupported working storage type: %s", config.Type)
	}
	return NewMemoryWorkingStorage(), nil
}