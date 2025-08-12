package repository

import (
	"github.com/Caia-Tech/govc/pkg/core"
	"github.com/Caia-Tech/govc/pkg/refs"
	"github.com/Caia-Tech/govc/pkg/storage"
)

// NewMemoryRepositoryV2Helper creates a pure in-memory repository using clean architecture
// (Actual implementation is in repository_v2.go)
func NewMemoryRepositoryV2Helper() *RepositoryV2 {
	return NewMemoryRepositoryV2()
}

// QuickStartV2 provides a simple way to create an in-memory repository with all components
type QuickStartV2 struct {
	Repository *core.CleanRepository
	Workspace  *core.CleanWorkspace
	Operations *core.Operations
	Config     *core.Config
	Stash      *core.StashManager
	Webhooks   *core.WebhookManager
}

// NewQuickStartV2 creates a complete in-memory Git environment
func NewQuickStartV2() *QuickStartV2 {
	// Create stores
	objects := core.NewMemoryObjectStore()
	refs := core.NewMemoryRefStore()
	working := core.NewMemoryWorkingStorage()
	config := core.NewMemoryConfigStore()

	// Create components
	repo := core.NewCleanRepository(objects, refs)
	workspace := core.NewCleanWorkspace(repo, working)
	cfg := core.NewConfig(config)
	ops := core.NewOperations(repo, workspace, cfg)

	// Create managers
	stash := core.NewStashManager(repo, workspace)
	webhooks := core.NewWebhookManager()

	// Initialize repository
	ops.Init()

	return &QuickStartV2{
		Repository: repo,
		Workspace:  workspace,
		Operations: ops,
		Config:     cfg,
		Stash:      stash,
		Webhooks:   webhooks,
	}
}

// NewFileBackedQuickStartV2 creates a file-backed Git environment
func NewFileBackedQuickStartV2(path string) (*QuickStartV2, error) {
	// Setup directories
	gitDir := path + "/.govc"

	// Create storage backends
	backend := storage.NewFileBackend(gitDir)
	store := storage.NewStore(backend)
	objects := &core.ObjectStoreAdapter{Store: store}

	fileRefStore := refs.NewFileRefStore(gitDir)
	refManager := refs.NewRefManager(fileRefStore)
	refStore := &core.RefStoreAdapter{RefManager: refManager, Store: fileRefStore}

	working := core.NewFileWorkingStorage(path)
	config := core.NewMemoryConfigStore() // Config can still be in memory

	// Create components
	repo := core.NewCleanRepository(objects, refStore)
	workspace := core.NewCleanWorkspace(repo, working)
	cfg := core.NewConfig(config)
	ops := core.NewOperations(repo, workspace, cfg)

	// Create managers
	stash := core.NewStashManager(repo, workspace)
	webhooks := core.NewWebhookManager()

	// Initialize repository
	if err := ops.Init(); err != nil {
		return nil, err
	}

	return &QuickStartV2{
		Repository: repo,
		Workspace:  workspace,
		Operations: ops,
		Config:     cfg,
		Stash:      stash,
		Webhooks:   webhooks,
	}, nil
}
