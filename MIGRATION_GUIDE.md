# Architecture Migration Guide

This guide helps you migrate from the monolithic `Repository` struct to the new clean architecture.

## Overview

The new architecture separates concerns into distinct components:

- **Core Repository** (`pkg/core/CleanRepository`) - Immutable Git objects and references only
- **Workspace** (`pkg/core/CleanWorkspace`) - Mutable working directory and staging area
- **Config** (`pkg/core/Config`) - Configuration management
- **StashManager** (`pkg/core/StashManager`) - Stash operations
- **WebhookManager** (`pkg/core/WebhookManager`) - Webhook management

## Key Benefits

1. **Separation of Concerns** - Each component has a single responsibility
2. **Testability** - Interface-based design makes testing easier
3. **Flexibility** - Mix and match storage backends (memory, file, hybrid)
4. **Modularity** - Use only the components you need

## Migration Steps

### 1. Replace Repository Creation

**Old:**
```go
// Memory repository
repo := govc.NewMemoryRepository()

// File repository
repo, err := govc.InitRepository("/path/to/repo")
repo, err := govc.OpenRepository("/path/to/repo")
```

**New:**
```go
import "github.com/caiatech/govc/pkg/core"

// Memory repository
objects := core.NewMemoryObjectStore()
refs := core.NewMemoryRefStore()
repo := core.NewCleanRepository(objects, refs)

// With workspace
working := core.NewMemoryWorkingStorage()
workspace := core.NewCleanWorkspace(repo, working)

// With operations helper
config := core.NewConfig(core.NewMemoryConfigStore())
ops := core.NewOperations(repo, workspace, config)
```

### 2. Update Basic Operations

**Old:**
```go
// Add files
err := repo.Add("file.txt")

// Commit
hash, err := repo.Commit("message", &object.Author{...})

// Status
status, err := repo.Status()

// Branches
err := repo.CreateBranch("feature")
err := repo.Checkout("feature")
branches, err := repo.ListBranches()
```

**New:**
```go
// Add files
err := ops.Add("file.txt")

// Commit
config.Set("user.name", "Name")
config.Set("user.email", "email@example.com")
hash, err := ops.Commit("message")

// Status
status, err := ops.Status()

// Branches
err := ops.Branch("feature")
err := ops.Checkout("feature")
branches, err := repo.ListBranches()
```

### 3. Migrate Stash Operations

**Old:**
```go
// Stash operations were part of Repository
stash, err := repo.Stash("WIP")
stashes := repo.ListStashes()
err := repo.ApplyStash(stashID)
```

**New:**
```go
// Create separate stash manager
stashMgr := core.NewStashManager(repo, workspace)

// Use stash manager
stash, err := stashMgr.Create("WIP")
stashes := stashMgr.List()
err := stashMgr.Apply(stashID)
err := stashMgr.Pop(stashID)
```

### 4. Migrate Webhook Operations

**Old:**
```go
// Webhooks were part of Repository
hook, err := repo.RegisterWebhook(url, events, secret)
hooks := repo.ListWebhooks()
repo.TriggerWebhook(event, data)
```

**New:**
```go
// Create separate webhook manager
webhookMgr := core.NewWebhookManager()

// Use webhook manager
hook, err := webhookMgr.Register(url, events, secret)
hooks := webhookMgr.List()
webhookMgr.Trigger(event, repoName, data)
```

### 5. Use Adapters for Existing Code

If you need backward compatibility:

```go
// Use RepositoryV2 as a drop-in replacement
repo, err := govc.InitRepositoryV2("/path/to/repo")

// It implements most of the old interface
err := repo.Add("file.txt")
hash, err := repo.Commit("message", author)
status, err := repo.Status()
```

## Storage Backends

The new architecture supports pluggable storage:

### Memory Storage
```go
objects := core.NewMemoryObjectStore()
refs := core.NewMemoryRefStore()
working := core.NewMemoryWorkingStorage()
config := core.NewMemoryConfigStore()
```

### File Storage (using adapters)
```go
// Adapt existing storage implementations
backend := storage.NewFileBackend(gitDir)
store := storage.NewStore(backend)
objects := &core.ObjectStoreAdapter{Store: store}

fileRefStore := refs.NewFileRefStore(gitDir)
refManager := refs.NewRefManager(fileRefStore)
refs := &core.RefStoreAdapter{RefManager: refManager, Store: fileRefStore}
```

### Custom Storage
Implement the interfaces:
- `core.ObjectStore`
- `core.RefStore`
- `core.WorkingStorage`
- `core.ConfigStore`

## Best Practices

1. **Use Operations for High-Level Tasks**
   - The `Operations` struct provides Git-like commands
   - Handles coordination between components

2. **Separate Storage from Logic**
   - Keep business logic in managers
   - Storage implementations should be simple

3. **Test with Memory Storage**
   - Fast and deterministic
   - No filesystem cleanup needed

4. **Use Interfaces**
   - Depend on interfaces, not concrete types
   - Makes testing and swapping implementations easy

## Example: Complete Migration

```go
package main

import (
    "fmt"
    "github.com/caiatech/govc/pkg/core"
)

func main() {
    // Create in-memory repository
    objects := core.NewMemoryObjectStore()
    refs := core.NewMemoryRefStore()
    working := core.NewMemoryWorkingStorage()
    config := core.NewMemoryConfigStore()
    
    // Create components
    repo := core.NewCleanRepository(objects, refs)
    workspace := core.NewCleanWorkspace(repo, working)
    cfg := core.NewConfig(config)
    ops := core.NewOperations(repo, workspace, cfg)
    
    // Initialize
    if err := ops.Init(); err != nil {
        panic(err)
    }
    
    // Configure
    cfg.Set("user.name", "Example User")
    cfg.Set("user.email", "user@example.com")
    
    // Create file
    if err := workspace.WriteFile("README.md", []byte("# Example")); err != nil {
        panic(err)
    }
    
    // Add and commit
    if err := ops.Add("README.md"); err != nil {
        panic(err)
    }
    
    hash, err := ops.Commit("Initial commit")
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Created commit: %s\n", hash)
    
    // Create stash manager if needed
    stashMgr := core.NewStashManager(repo, workspace)
    
    // Create webhook manager if needed
    webhookMgr := core.NewWebhookManager()
    
    // Your application logic here...
}
```

## Troubleshooting

### Import Errors
Make sure to update imports:
```go
// Old
import "github.com/caiatech/govc"

// New (add as needed)
import (
    "github.com/caiatech/govc/pkg/core"
    "github.com/caiatech/govc/pkg/object"
)
```

### Missing Methods
Some methods may have moved:
- Stash operations → `StashManager`
- Webhook operations → `WebhookManager`
- Config operations → `Config`

### Type Mismatches
The new `Status` type is in `core` package:
```go
var status *core.Status
```

## Future Plans

1. **Remove Legacy Code** - Once migration is complete
2. **Add More Storage Backends** - S3, database, etc.
3. **Improve Performance** - Optimize hot paths
4. **Add Features** - Shallow clones, partial checkouts

For questions or issues, please open an issue on GitHub.