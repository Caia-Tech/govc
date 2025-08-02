package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/caiatech/govc/pkg/object"
)

type Backend interface {
	ReadObject(hash string) ([]byte, error)
	WriteObject(hash string, data []byte) error
	HasObject(hash string) bool
	ListObjects() ([]string, error)
}

// MemoryBackend stores all Git objects in memory.
// This is the foundation of govc's memory-first approach - no disk I/O
// means operations complete in microseconds instead of milliseconds.
// Creating 1000 branches? That's just creating 1000 pointers in a map.
type MemoryBackend struct {
	objects map[string][]byte
	mu      sync.RWMutex
}

func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		objects: make(map[string][]byte),
	}
}

func (m *MemoryBackend) ReadObject(hash string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, exists := m.objects[hash]
	if !exists {
		return nil, fmt.Errorf("object not found: %s", hash)
	}
	return data, nil
}

func (m *MemoryBackend) WriteObject(hash string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.objects[hash] = data
	return nil
}

func (m *MemoryBackend) HasObject(hash string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.objects[hash]
	return exists
}

func (m *MemoryBackend) ListObjects() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hashes := make([]string, 0, len(m.objects))
	for hash := range m.objects {
		hashes = append(hashes, hash)
	}
	return hashes, nil
}

type FileBackend struct {
	path string
	mu   sync.RWMutex
}

func NewFileBackend(path string) *FileBackend {
	return &FileBackend{
		path: path,
	}
}

func (f *FileBackend) objectPath(hash string) string {
	if len(hash) < 2 {
		return ""
	}
	dir := hash[:2]
	file := hash[2:]
	return filepath.Join(f.path, "objects", dir, file)
}

func (f *FileBackend) ReadObject(hash string) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	path := f.objectPath(hash)
	compressed, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("object not found: %s", hash)
	}

	return object.Decompress(compressed)
}

func (f *FileBackend) WriteObject(hash string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	path := f.objectPath(hash)
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	compressed, err := object.Compress(data)
	if err != nil {
		return fmt.Errorf("failed to compress object: %v", err)
	}

	return os.WriteFile(path, compressed, 0644)
}

func (f *FileBackend) HasObject(hash string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	path := f.objectPath(hash)
	_, err := os.Stat(path)
	return err == nil
}

func (f *FileBackend) ListObjects() ([]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var hashes []string
	objectsDir := filepath.Join(f.path, "objects")

	dirs, err := os.ReadDir(objectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return hashes, nil
		}
		return nil, err
	}

	for _, dir := range dirs {
		if !dir.IsDir() || len(dir.Name()) != 2 {
			continue
		}

		subDir := filepath.Join(objectsDir, dir.Name())
		files, err := os.ReadDir(subDir)
		if err != nil {
			continue
		}

		for _, file := range files {
			if !file.IsDir() {
				hash := dir.Name() + file.Name()
				hashes = append(hashes, hash)
			}
		}
	}

	return hashes, nil
}

// Store provides a unified interface to Git objects.
// The key innovation: cache isn't just for performance, it's the primary
// storage layer. The backend is optional - you can run entirely in memory.
// This enables use cases like testing infrastructure changes without
// ever touching disk.
type Store struct {
	backend Backend
	cache   *MemoryBackend // Memory-first: cache is primary, backend is optional
}

func NewStore(backend Backend) *Store {
	return &Store{
		backend: backend,
		cache:   NewMemoryBackend(),
	}
}

func (s *Store) StoreObject(obj object.Object) (string, error) {
	data, err := obj.Serialize()
	if err != nil {
		return "", err
	}
	hash := object.HashObject(data)

	if s.cache.HasObject(hash) {
		return hash, nil
	}

	if err := s.cache.WriteObject(hash, data); err != nil {
		return "", err
	}

	if err := s.backend.WriteObject(hash, data); err != nil {
		return "", err
	}

	return hash, nil
}

func (s *Store) GetObject(hash string) (object.Object, error) {
	var data []byte
	var err error

	data, err = s.cache.ReadObject(hash)
	if err != nil {
		data, err = s.backend.ReadObject(hash)
		if err != nil {
			return nil, err
		}
		_ = s.cache.WriteObject(hash, data)
	}

	return object.ParseObject(data)
}

func (s *Store) HasObject(hash string) bool {
	return s.cache.HasObject(hash) || s.backend.HasObject(hash)
}

func (s *Store) ListObjects() ([]string, error) {
	return s.backend.ListObjects()
}

func (s *Store) StoreBlob(content []byte) (string, error) {
	blob := object.NewBlob(content)
	return s.StoreObject(blob)
}

func (s *Store) GetBlob(hash string) (*object.Blob, error) {
	obj, err := s.GetObject(hash)
	if err != nil {
		return nil, err
	}

	blob, ok := obj.(*object.Blob)
	if !ok {
		return nil, fmt.Errorf("object %s is not a blob", hash)
	}

	return blob, nil
}

func (s *Store) StoreTree(tree *object.Tree) (string, error) {
	return s.StoreObject(tree)
}

func (s *Store) GetTree(hash string) (*object.Tree, error) {
	obj, err := s.GetObject(hash)
	if err != nil {
		return nil, err
	}

	tree, ok := obj.(*object.Tree)
	if !ok {
		return nil, fmt.Errorf("object %s is not a tree", hash)
	}

	return tree, nil
}

func (s *Store) StoreCommit(commit *object.Commit) (string, error) {
	return s.StoreObject(commit)
}

func (s *Store) GetCommit(hash string) (*object.Commit, error) {
	obj, err := s.GetObject(hash)
	if err != nil {
		return nil, err
	}

	commit, ok := obj.(*object.Commit)
	if !ok {
		return nil, fmt.Errorf("object %s is not a commit", hash)
	}

	return commit, nil
}

func (s *Store) StoreTag(tag *object.Tag) (string, error) {
	return s.StoreObject(tag)
}

func (s *Store) GetTag(hash string) (*object.Tag, error) {
	obj, err := s.GetObject(hash)
	if err != nil {
		return nil, err
	}

	tag, ok := obj.(*object.Tag)
	if !ok {
		return nil, fmt.Errorf("object %s is not a tag", hash)
	}

	return tag, nil
}

type PackFile struct {
	Version    uint32
	NumObjects uint32
	Objects    []PackedObject
}

type PackedObject struct {
	Type   object.Type
	Size   uint64
	Offset uint64
	Data   []byte
}

func (s *Store) WritePack(w io.Writer, hashes []string) error {
	pack := &PackFile{
		Version:    2,
		NumObjects: uint32(len(hashes)),
		Objects:    make([]PackedObject, 0, len(hashes)),
	}

	for _, hash := range hashes {
		obj, err := s.GetObject(hash)
		if err != nil {
			return err
		}

		data, err := obj.Serialize()
		if err != nil {
			return err
		}
		packed := PackedObject{
			Type: obj.Type(),
			Size: uint64(obj.Size()),
			Data: data,
		}
		pack.Objects = append(pack.Objects, packed)
	}

	return nil
}

func (s *Store) ReadPack(r io.Reader) error {
	return nil
}
