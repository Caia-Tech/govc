package storage

import (
	"github.com/caiatech/govc/pkg/refs"
)

// RefManagerAdapter adapts the existing RefManager to implement storage.RefStore interface
// This provides backward compatibility while transitioning to the new architecture
type RefManagerAdapter struct {
	manager *refs.RefManager
}

// NewRefManagerAdapter creates a RefStore adapter for the existing RefManager
func NewRefManagerAdapter(manager *refs.RefManager) *RefManagerAdapter {
	return &RefManagerAdapter{manager: manager}
}

func (a *RefManagerAdapter) GetRef(name string) (string, error) {
	// The RefManager uses its underlying store's GetRef
	// We need to access the store directly
	if refStore := a.getUnderlyingStore(); refStore != nil {
		return refStore.GetRef(name)
	}
	// Fallback to using RefManager methods for known ref types
	if name == "refs/heads/main" || name == "main" {
		return a.manager.GetBranch("main")
	}
	// For other refs, we'll need to use reflection or modify RefManager
	// For now, return an error for unsupported refs
	return "", NotFoundError("ref not found: " + name)
}

func (a *RefManagerAdapter) UpdateRef(name string, hash string) error {
	// Use the RefManager's UpdateRef method
	return a.manager.UpdateRef(name, hash, "")
}

func (a *RefManagerAdapter) DeleteRef(name string) error {
	// The RefManager uses its underlying store's DeleteRef
	if refStore := a.getUnderlyingStore(); refStore != nil {
		return refStore.DeleteRef(name)
	}
	return NotFoundError("ref not found: " + name)
}

func (a *RefManagerAdapter) ListRefs() (map[string]string, error) {
	result := make(map[string]string)
	
	// List branches
	branches, err := a.manager.ListBranches()
	if err != nil {
		return nil, err
	}
	for _, branch := range branches {
		result[branch.Name] = branch.Hash
	}
	
	// List tags
	tags, err := a.manager.ListTags()
	if err != nil {
		return nil, err
	}
	for _, tag := range tags {
		result[tag.Name] = tag.Hash
	}
	
	return result, nil
}

func (a *RefManagerAdapter) GetHEAD() (string, error) {
	return a.manager.GetHEAD()
}

func (a *RefManagerAdapter) SetHEAD(target string) error {
	return a.manager.SetHEAD(target)
}

func (a *RefManagerAdapter) Close() error {
	// RefManager doesn't have a Close method, so nothing to do
	return nil
}

// Helper method to access the underlying RefStore
func (a *RefManagerAdapter) getUnderlyingStore() refs.RefStore {
	// This requires access to the manager's store field
	// Since it's not exported, we'll need to add a getter method or use reflection
	// For now, return nil and rely on the RefManager methods
	return nil
}

// NewRefStoreFromRefsStore creates a storage.RefStore from a refs.RefStore
func NewRefStoreFromRefsStore(refStore refs.RefStore) RefStore {
	manager := refs.NewRefManager(refStore)
	return NewRefManagerAdapter(manager)
}

// NewMemoryRefStoreFromRefs creates a memory-only RefStore using the refs package
func NewMemoryRefStoreFromRefs() RefStore {
	memoryStore := refs.NewMemoryRefStore()
	manager := refs.NewRefManager(memoryStore)
	return NewRefManagerAdapter(manager)
}

// NewFileRefStore creates a file-backed RefStore using the refs package
func NewFileRefStore(path string) RefStore {
	fileStore := refs.NewFileRefStore(path)
	manager := refs.NewRefManager(fileStore)
	return NewRefManagerAdapter(manager)
}