package api

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/caiatech/govc"
	"github.com/caiatech/govc/logging"
	"github.com/caiatech/govc/pkg/core"
	"github.com/caiatech/govc/pkg/refs"
	"github.com/caiatech/govc/pkg/storage"
)

// RepositoryFactory creates repositories using the new architecture
type RepositoryFactory struct {
	mu sync.RWMutex
	// Cache of components for each repository
	repositories map[string]*RepositoryComponents
	logger       *logging.Logger
}

// RepositoryComponents holds all components for a repository
type RepositoryComponents struct {
	Repository *core.CleanRepository
	Workspace  *core.CleanWorkspace
	Operations *core.Operations
	Config     *core.Config
	Stash      *core.StashManager
	Webhooks   *core.WebhookManager

	// Legacy wrapper for backward compatibility
	LegacyRepo *govc.RepositoryV2
}

// NewRepositoryFactory creates a new repository factory
func NewRepositoryFactory(logger *logging.Logger) *RepositoryFactory {
	return &RepositoryFactory{
		repositories: make(map[string]*RepositoryComponents),
		logger:       logger.WithComponent("repository-factory"),
	}
}

// CreateRepository creates a new repository with the specified ID and path
func (rf *RepositoryFactory) CreateRepository(id string, path string, memoryOnly bool) (*RepositoryComponents, error) {
	start := time.Now()
	
	rf.logger.WithFields(map[string]interface{}{
		"repo_id":     id,
		"path":        path,
		"memory_only": memoryOnly,
	}).Info("Creating repository in factory")
	
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// Check if already exists
	if _, exists := rf.repositories[id]; exists {
		rf.logger.WithField("repo_id", id).Warn("Attempted to create existing repository")
		return nil, fmt.Errorf("repository already exists: %s", id)
	}

	var components *RepositoryComponents
	var err error

	if memoryOnly {
		components, err = rf.createMemoryRepository(id)
	} else {
		components, err = rf.createFileRepository(id, path)
	}

	if err != nil {
		rf.logger.WithFields(map[string]interface{}{
			"repo_id":     id,
			"path":        path,
			"memory_only": memoryOnly,
			"error":       err.Error(),
			"duration_ms": time.Since(start).Milliseconds(),
		}).Error("Failed to create repository in factory")
		return nil, err
	}

	rf.repositories[id] = components
	
	rf.logger.WithFields(map[string]interface{}{
		"repo_id":     id,
		"path":        path,
		"memory_only": memoryOnly,
		"duration_ms": time.Since(start).Milliseconds(),
	}).Info("Repository created successfully in factory")
	
	return components, nil
}

// GetRepository retrieves an existing repository
func (rf *RepositoryFactory) GetRepository(id string) (*RepositoryComponents, error) {
	rf.mu.RLock()
	defer rf.mu.RUnlock()

	components, exists := rf.repositories[id]
	if !exists {
		return nil, fmt.Errorf("repository not found: %s", id)
	}

	return components, nil
}

// DeleteRepository removes a repository
func (rf *RepositoryFactory) DeleteRepository(id string) error {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	delete(rf.repositories, id)
	return nil
}

// ListRepositories returns all repository IDs
func (rf *RepositoryFactory) ListRepositories() []string {
	rf.mu.RLock()
	defer rf.mu.RUnlock()

	ids := make([]string, 0, len(rf.repositories))
	for id := range rf.repositories {
		ids = append(ids, id)
	}
	return ids
}

// createMemoryRepository creates a pure in-memory repository
func (rf *RepositoryFactory) createMemoryRepository(id string) (*RepositoryComponents, error) {
	// Create stores
	objects := core.NewMemoryObjectStore()
	refStore := core.NewMemoryRefStore()
	working := core.NewMemoryWorkingStorage()
	configStore := core.NewMemoryConfigStore()

	// Create components
	repo := core.NewCleanRepository(objects, refStore)
	workspace := core.NewCleanWorkspace(repo, working)
	config := core.NewConfig(configStore)
	ops := core.NewOperations(repo, workspace, config)

	// Create managers
	stash := core.NewStashManager(repo, workspace)
	webhooks := core.NewWebhookManager()

	// Initialize repository
	if err := ops.Init(); err != nil {
		return nil, err
	}

	// Create legacy wrapper
	legacy := &govc.RepositoryV2{
		// This would need proper initialization
		// For now, we'll use the components directly
	}

	return &RepositoryComponents{
		Repository: repo,
		Workspace:  workspace,
		Operations: ops,
		Config:     config,
		Stash:      stash,
		Webhooks:   webhooks,
		LegacyRepo: legacy,
	}, nil
}

// createFileRepository creates a file-backed repository
func (rf *RepositoryFactory) createFileRepository(id string, path string) (*RepositoryComponents, error) {
	// Create directories
	gitDir := filepath.Join(path, ".govc")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .govc directory: %v", err)
	}

	objectsDir := filepath.Join(gitDir, "objects")
	if err := os.MkdirAll(objectsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create objects directory: %v", err)
	}

	refsDir := filepath.Join(gitDir, "refs", "heads")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create refs directory: %v", err)
	}

	// Create storage backends
	backend := storage.NewFileBackend(gitDir)
	store := storage.NewStore(backend)
	objects := &core.ObjectStoreAdapter{Store: store}

	fileRefStore := refs.NewFileRefStore(gitDir)
	refManager := refs.NewRefManager(fileRefStore)
	refStore := &core.RefStoreAdapter{RefManager: refManager, Store: fileRefStore}

	// For now, use memory working storage even for file repos
	// TODO: Implement file-based working storage
	working := core.NewMemoryWorkingStorage()
	configStore := core.NewMemoryConfigStore()

	// Create components
	repo := core.NewCleanRepository(objects, refStore)
	workspace := core.NewCleanWorkspace(repo, working)
	config := core.NewConfig(configStore)
	ops := core.NewOperations(repo, workspace, config)

	// Create managers
	stash := core.NewStashManager(repo, workspace)
	webhooks := core.NewWebhookManager()

	// Initialize repository
	if err := ops.Init(); err != nil {
		return nil, err
	}

	// Create legacy wrapper
	legacy := &govc.RepositoryV2{
		// This would need proper initialization
	}

	return &RepositoryComponents{
		Repository: repo,
		Workspace:  workspace,
		Operations: ops,
		Config:     config,
		Stash:      stash,
		Webhooks:   webhooks,
		LegacyRepo: legacy,
	}, nil
}

// OpenRepository opens an existing repository
func (rf *RepositoryFactory) OpenRepository(id string, path string) (*RepositoryComponents, error) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// Check if already loaded
	if components, exists := rf.repositories[id]; exists {
		return components, nil
	}

	// Check if path exists
	gitDir := filepath.Join(path, ".govc")
	if _, err := os.Stat(gitDir); err != nil {
		return nil, fmt.Errorf("not a govc repository: %v", err)
	}

	// Create components
	components, err := rf.createFileRepository(id, path)
	if err != nil {
		return nil, err
	}

	rf.repositories[id] = components
	return components, nil
}
