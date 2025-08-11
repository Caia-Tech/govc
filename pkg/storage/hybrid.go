package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/Caia-Tech/govc/pkg/object"
)

// HybridObjectStore implements memory-first storage with disk backup
// Objects are kept in memory for speed but also persisted to disk
type HybridObjectStore struct {
	memory     *MemoryObjectStore
	diskPath   string
	maxMemory  int64
	usedMemory int64
	mu         sync.RWMutex
}

// NewHybridObjectStore creates a memory-first object store with disk backup
func NewHybridObjectStore(diskPath string, maxMemory int64) (*HybridObjectStore, error) {
	if err := os.MkdirAll(filepath.Join(diskPath, "objects"), 0755); err != nil {
		return nil, IOError("failed to create objects directory", err)
	}

	return &HybridObjectStore{
		memory:    NewMemoryObjectStore(),
		diskPath:  diskPath,
		maxMemory: maxMemory,
	}, nil
}

func (s *HybridObjectStore) Get(hash string) (object.Object, error) {
	// Try memory first
	obj, err := s.memory.Get(hash)
	if err == nil {
		return obj, nil
	}

	// If not in memory, try disk
	obj, err = s.loadFromDisk(hash)
	if err != nil {
		return nil, err
	}

	// Load into memory for future access
	s.memory.Put(obj)
	atomic.AddInt64(&s.usedMemory, s.estimateObjectSize(obj))

	return obj, nil
}

func (s *HybridObjectStore) Put(obj object.Object) (string, error) {
	if obj == nil {
		return "", InvalidObjectError("cannot store nil object")
	}

	hash := obj.Hash()

	// Always store in memory first
	_, err := s.memory.Put(obj)
	if err != nil {
		return "", err
	}

	// Update memory usage
	size := s.estimateObjectSize(obj)
	newUsage := atomic.AddInt64(&s.usedMemory, size)

	// If over memory limit, evict some objects
	if newUsage > s.maxMemory {
		s.evictLRU()
	}

	// Also persist to disk
	if err := s.saveToDisk(hash, obj); err != nil {
		// Log error but don't fail - memory storage succeeded
		// In a production system, you'd want proper logging here
	}

	return hash, nil
}

func (s *HybridObjectStore) Exists(hash string) bool {
	// Check memory first
	if s.memory.Exists(hash) {
		return true
	}

	// Check disk
	objPath := filepath.Join(s.diskPath, "objects", hash[:2], hash[2:])
	_, err := os.Stat(objPath)
	return err == nil
}

func (s *HybridObjectStore) List() ([]string, error) {
	// Start with memory objects
	memoryHashes, err := s.memory.List()
	if err != nil {
		return nil, err
	}

	// Add disk objects
	hashSet := make(map[string]bool)
	for _, hash := range memoryHashes {
		hashSet[hash] = true
	}

	// Walk disk objects directory
	objectsDir := filepath.Join(s.diskPath, "objects")
	err = filepath.Walk(objectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			// Reconstruct hash from directory structure
			rel, _ := filepath.Rel(objectsDir, path)
			if len(rel) >= 3 {
				hash := filepath.Dir(rel) + filepath.Base(rel)
				hashSet[hash] = true
			}
		}
		return nil
	})

	if err != nil {
		// Return memory hashes if disk walk fails
		return memoryHashes, nil
	}

	// Convert back to slice
	result := make([]string, 0, len(hashSet))
	for hash := range hashSet {
		result = append(result, hash)
	}

	return result, nil
}

func (s *HybridObjectStore) Size() (int64, error) {
	return atomic.LoadInt64(&s.usedMemory), nil
}

func (s *HybridObjectStore) Close() error {
	s.memory.Close()
	return nil
}

// Helper methods

func (s *HybridObjectStore) loadFromDisk(hash string) (object.Object, error) {
	objPath := filepath.Join(s.diskPath, "objects", hash[:2], hash[2:])

	data, err := os.ReadFile(objPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NotFoundError(fmt.Sprintf("object %s not found", hash))
		}
		return nil, IOError(fmt.Sprintf("failed to read object %s", hash), err)
	}

	// Deserialize object based on type
	// This is a simplified implementation - in reality you'd need proper deserialization
	obj, err := object.Deserialize(data)
	if err != nil {
		return nil, InvalidObjectError(fmt.Sprintf("failed to deserialize object %s", hash))
	}

	return obj, nil
}

func (s *HybridObjectStore) saveToDisk(hash string, obj object.Object) error {
	objDir := filepath.Join(s.diskPath, "objects", hash[:2])
	if err := os.MkdirAll(objDir, 0755); err != nil {
		return IOError("failed to create object directory", err)
	}

	objPath := filepath.Join(objDir, hash[2:])

	// Serialize object
	data, err := obj.Serialize()
	if err != nil {
		return IOError("failed to serialize object", err)
	}

	return os.WriteFile(objPath, data, 0644)
}

func (s *HybridObjectStore) estimateObjectSize(obj object.Object) int64 {
	// Rough estimate - in production you'd want more accurate sizing
	data, err := obj.Serialize()
	if err != nil {
		return 1024 // fallback estimate
	}
	return int64(len(data))
}

func (s *HybridObjectStore) evictLRU() {
	// Simplified LRU eviction - remove half the objects
	// In production you'd want proper LRU tracking
	s.mu.Lock()
	defer s.mu.Unlock()

	hashes, _ := s.memory.List()
	evictCount := len(hashes) / 2

	for i := 0; i < evictCount && i < len(hashes); i++ {
		hash := hashes[i]
		if obj, err := s.memory.Get(hash); err == nil {
			size := s.estimateObjectSize(obj)
			atomic.AddInt64(&s.usedMemory, -size)
		}

		// Remove from memory (but keep on disk)
		s.memory.objects[hash] = nil
		delete(s.memory.objects, hash)
	}
}

// HybridStorageFactory creates hybrid storage instances
type HybridStorageFactory struct {
	basePath string
}

// NewHybridStorageFactory creates a new hybrid storage factory
func NewHybridStorageFactory(basePath string) *HybridStorageFactory {
	return &HybridStorageFactory{basePath: basePath}
}

func (f *HybridStorageFactory) CreateObjectStore(config ObjectStoreConfig) (ObjectStore, error) {
	if config.Type != "hybrid" {
		return nil, fmt.Errorf("unsupported object store type: %s", config.Type)
	}

	path := config.Path
	if path == "" {
		path = filepath.Join(f.basePath, "objects")
	}

	maxMemory := config.MaxMemory
	if maxMemory <= 0 {
		maxMemory = 100 * 1024 * 1024 // 100MB default
	}

	return NewHybridObjectStore(path, maxMemory)
}

func (f *HybridStorageFactory) CreateRefStore(config RefStoreConfig) (RefStore, error) {
	// For now, refs are always in memory for speed
	// Could be extended to hybrid as well
	return NewMemoryRefStore(), nil
}

func (f *HybridStorageFactory) CreateWorkingStorage(config WorkingStorageConfig) (WorkingStorage, error) {
	// Working storage is typically memory-based for performance
	return NewMemoryWorkingStorage(), nil
}
