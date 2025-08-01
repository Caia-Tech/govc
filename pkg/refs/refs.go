package refs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type RefType string

const (
	RefTypeBranch RefType = "branch"
	RefTypeTag    RefType = "tag"
	RefTypeRemote RefType = "remote"
)

type Ref struct {
	Name string
	Hash string
	Type RefType
}

type RefStore interface {
	GetRef(name string) (string, error)
	SetRef(name string, hash string) error
	DeleteRef(name string) error
	ListRefs(prefix string) ([]Ref, error)
	GetHEAD() (string, error)
	SetHEAD(ref string) error
}

// MemoryRefStore keeps all references in memory.
// In traditional Git, each ref is a file. Here, refs are just map entries.
// This is why operations like creating branches or switching between them
// are instant - no file I/O, just pointer updates.
type MemoryRefStore struct {
	refs map[string]string
	head string
	mu   sync.RWMutex
}

func NewMemoryRefStore() *MemoryRefStore {
	return &MemoryRefStore{
		refs: make(map[string]string),
		head: "refs/heads/main",
	}
}

func (m *MemoryRefStore) GetRef(name string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	fullName := m.normalizeRefName(name)
	hash, exists := m.refs[fullName]
	if !exists {
		return "", fmt.Errorf("ref not found: %s", name)
	}
	return hash, nil
}

func (m *MemoryRefStore) SetRef(name string, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fullName := m.normalizeRefName(name)
	m.refs[fullName] = hash
	return nil
}

func (m *MemoryRefStore) DeleteRef(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fullName := m.normalizeRefName(name)
	delete(m.refs, fullName)
	return nil
}

func (m *MemoryRefStore) ListRefs(prefix string) ([]Ref, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	refs := make([]Ref, 0)
	for name, hash := range m.refs {
		if prefix == "" || strings.HasPrefix(name, prefix) {
			refType := m.getRefType(name)
			refs = append(refs, Ref{
				Name: name,
				Hash: hash,
				Type: refType,
			})
		}
	}
	return refs, nil
}

func (m *MemoryRefStore) GetHEAD() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.head == "" {
		return "", fmt.Errorf("HEAD not set")
	}

	// Handle symbolic references (e.g., "ref: refs/heads/main")
	if strings.HasPrefix(m.head, "ref: ") {
		ref := strings.TrimPrefix(m.head, "ref: ")
		hash, exists := m.refs[ref]
		if !exists {
			return "", fmt.Errorf("HEAD points to non-existent ref: %s", ref)
		}
		return hash, nil
	}

	// Handle direct references that start with refs/
	if strings.HasPrefix(m.head, "refs/") {
		hash, exists := m.refs[m.head]
		if !exists {
			return "", fmt.Errorf("HEAD points to non-existent ref: %s", m.head)
		}
		return hash, nil
	}

	return m.head, nil
}

func (m *MemoryRefStore) SetHEAD(ref string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.head = ref
	return nil
}

func (m *MemoryRefStore) normalizeRefName(name string) string {
	if strings.HasPrefix(name, "refs/") {
		return name
	}
	if strings.Contains(name, "/") {
		return name
	}
	return "refs/heads/" + name
}

func (m *MemoryRefStore) getRefType(name string) RefType {
	if strings.HasPrefix(name, "refs/heads/") {
		return RefTypeBranch
	}
	if strings.HasPrefix(name, "refs/tags/") {
		return RefTypeTag
	}
	if strings.HasPrefix(name, "refs/remotes/") {
		return RefTypeRemote
	}
	return RefTypeBranch
}

type FileRefStore struct {
	path string
	mu   sync.RWMutex
}

func NewFileRefStore(path string) *FileRefStore {
	return &FileRefStore{
		path: path,
	}
}

func (f *FileRefStore) GetRef(name string) (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	fullName := f.normalizeRefName(name)
	refPath := filepath.Join(f.path, fullName)

	data, err := os.ReadFile(refPath)
	if err != nil {
		packedRefs, err := f.readPackedRefs()
		if err != nil {
			return "", fmt.Errorf("ref not found: %s", name)
		}
		hash, exists := packedRefs[fullName]
		if !exists {
			return "", fmt.Errorf("ref not found: %s", name)
		}
		return hash, nil
	}

	return strings.TrimSpace(string(data)), nil
}

func (f *FileRefStore) SetRef(name string, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	fullName := f.normalizeRefName(name)
	refPath := filepath.Join(f.path, fullName)
	dir := filepath.Dir(refPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create ref directory: %v", err)
	}

	return os.WriteFile(refPath, []byte(hash+"\n"), 0644)
}

func (f *FileRefStore) DeleteRef(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	fullName := f.normalizeRefName(name)
	refPath := filepath.Join(f.path, fullName)

	return os.Remove(refPath)
}

func (f *FileRefStore) ListRefs(prefix string) ([]Ref, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	refs := make([]Ref, 0)

	looseRefs, err := f.listLooseRefs(prefix)
	if err != nil {
		return nil, err
	}
	refs = append(refs, looseRefs...)

	packedRefs, err := f.readPackedRefs()
	if err == nil {
		for name, hash := range packedRefs {
			if prefix == "" || strings.HasPrefix(name, prefix) {
				refType := f.getRefType(name)
				refs = append(refs, Ref{
					Name: name,
					Hash: hash,
					Type: refType,
				})
			}
		}
	}

	return refs, nil
}

func (f *FileRefStore) GetHEAD() (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	headPath := filepath.Join(f.path, "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return "", fmt.Errorf("failed to read HEAD: %v", err)
	}

	content := strings.TrimSpace(string(data))
	if strings.HasPrefix(content, "ref: ") {
		ref := strings.TrimPrefix(content, "ref: ")
		return f.GetRef(ref)
	}

	return content, nil
}

func (f *FileRefStore) SetHEAD(ref string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	headPath := filepath.Join(f.path, "HEAD")
	content := ref
	if strings.HasPrefix(ref, "refs/") {
		content = "ref: " + ref
	}
	return os.WriteFile(headPath, []byte(content+"\n"), 0644)
}

func (f *FileRefStore) normalizeRefName(name string) string {
	if strings.HasPrefix(name, "refs/") {
		return name
	}
	if strings.Contains(name, "/") {
		return name
	}
	return "refs/heads/" + name
}

func (f *FileRefStore) getRefType(name string) RefType {
	if strings.HasPrefix(name, "refs/heads/") {
		return RefTypeBranch
	}
	if strings.HasPrefix(name, "refs/tags/") {
		return RefTypeTag
	}
	if strings.HasPrefix(name, "refs/remotes/") {
		return RefTypeRemote
	}
	return RefTypeBranch
}

func (f *FileRefStore) listLooseRefs(prefix string) ([]Ref, error) {
	refs := make([]Ref, 0)
	refsDir := filepath.Join(f.path, "refs")

	err := filepath.Walk(refsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(f.path, path)
		if err != nil {
			return err
		}

		if prefix != "" && !strings.HasPrefix(relPath, prefix) {
			return nil
		}

		hash, err := f.GetRef(relPath)
		if err != nil {
			return nil
		}

		refType := f.getRefType(relPath)
		refs = append(refs, Ref{
			Name: relPath,
			Hash: hash,
			Type: refType,
		})

		return nil
	})

	return refs, err
}

func (f *FileRefStore) readPackedRefs() (map[string]string, error) {
	packedPath := filepath.Join(f.path, "packed-refs")
	data, err := os.ReadFile(packedPath)
	if err != nil {
		return nil, err
	}

	refs := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) == 2 {
			refs[parts[1]] = parts[0]
		}
	}

	return refs, nil
}

type RefManager struct {
	store RefStore
}

func NewRefManager(store RefStore) *RefManager {
	return &RefManager{
		store: store,
	}
}

func (r *RefManager) CreateBranch(name string, commitHash string) error {
	return r.store.SetRef("refs/heads/"+name, commitHash)
}

func (r *RefManager) DeleteBranch(name string) error {
	return r.store.DeleteRef("refs/heads/" + name)
}

func (r *RefManager) ListBranches() ([]Ref, error) {
	return r.store.ListRefs("refs/heads/")
}

func (r *RefManager) GetBranch(name string) (string, error) {
	return r.store.GetRef("refs/heads/" + name)
}

func (r *RefManager) CreateTag(name string, objectHash string) error {
	return r.store.SetRef("refs/tags/"+name, objectHash)
}

func (r *RefManager) DeleteTag(name string) error {
	return r.store.DeleteRef("refs/tags/" + name)
}

func (r *RefManager) ListTags() ([]Ref, error) {
	return r.store.ListRefs("refs/tags/")
}

func (r *RefManager) GetTag(name string) (string, error) {
	return r.store.GetRef("refs/tags/" + name)
}

func (r *RefManager) GetHEAD() (string, error) {
	return r.store.GetHEAD()
}

func (r *RefManager) SetHEAD(ref string) error {
	return r.store.SetHEAD(ref)
}

func (r *RefManager) SetHEADToBranch(branchName string) error {
	return r.store.SetHEAD("ref: refs/heads/" + branchName)
}

func (r *RefManager) SetHEADToCommit(commitHash string) error {
	return r.store.SetHEAD(commitHash)
}

func (r *RefManager) GetCurrentBranch() (string, error) {
	// We need to get the raw HEAD value, not the resolved one
	// For MemoryRefStore, we need to access the head field directly
	if memStore, ok := r.store.(*MemoryRefStore); ok {
		memStore.mu.RLock()
		head := memStore.head
		memStore.mu.RUnlock()
		
		if strings.HasPrefix(head, "ref: refs/heads/") {
			return strings.TrimPrefix(head, "ref: refs/heads/"), nil
		}
		return "", fmt.Errorf("HEAD is detached")
	}
	
	// For FileRefStore, read HEAD file directly
	if fileStore, ok := r.store.(*FileRefStore); ok {
		fileStore.mu.RLock()
		defer fileStore.mu.RUnlock()
		
		headPath := filepath.Join(fileStore.path, "HEAD")
		data, err := os.ReadFile(headPath)
		if err != nil {
			return "", fmt.Errorf("failed to read HEAD: %v", err)
		}
		
		content := strings.TrimSpace(string(data))
		if strings.HasPrefix(content, "ref: refs/heads/") {
			return strings.TrimPrefix(content, "ref: refs/heads/"), nil
		}
		return "", fmt.Errorf("HEAD is detached")
	}
	
	return "", fmt.Errorf("unknown ref store type")
}

func (r *RefManager) UpdateRef(name string, newHash string, oldHash string) error {
	if oldHash != "" {
		currentHash, err := r.store.GetRef(name)
		if err != nil {
			return err
		}
		if currentHash != oldHash {
			return fmt.Errorf("ref %s has changed", name)
		}
	}
	return r.store.SetRef(name, newHash)
}