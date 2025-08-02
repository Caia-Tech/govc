# govc Architectural Refactoring Plan

## Goal
Simplify and focus the govc codebase on its core value proposition: memory-first Git operations.

## Current Issues

### 1. Repository Struct Violations
The `Repository` struct currently mixes multiple concerns:
```go
type Repository struct {
    // Core (keep)
    store      *storage.Store      // Immutable object storage ✓
    refManager *refs.RefManager    // Reference management ✓
    
    // Working state (extract)
    staging    *StagingArea        // Should be in Workspace
    worktree   *Worktree          // Should be in Workspace
    
    // Configuration (extract)
    config     map[string]string   // Should be in Config service
    
    // External integrations (extract)
    webhooks   map[string]*Webhook // Should be in EventManager
    events     chan *RepositoryEvent
    
    // Other
    stashes    []*Stash           // Consider if this belongs here
    mu         sync.RWMutex       // Keep for thread safety
}
```

### 2. Features to Remove/Extract
- Container system (not core to VCS)
- Distributed/cluster features (can be a plugin later)
- Complex event systems (webhooks can be simpler)

## Refactoring Steps

### Phase 1: Create Clean Architecture

#### 1.1 Core Repository (Immutable)
```go
// Core repository only handles immutable Git objects
type Repository struct {
    objects ObjectStore  // Blobs, trees, commits, tags
    refs    RefStore    // Branches, tags, HEAD
}
```

#### 1.2 Workspace (Mutable State)
```go
// Workspace manages the mutable working directory
type Workspace struct {
    repo     *Repository
    worktree WorkingStorage
    staging  *StagingArea
}
```

#### 1.3 Configuration
```go
// Config manages repository configuration
type Config struct {
    store ConfigStore
}
```

#### 1.4 Event System (Optional)
```go
// EventManager handles webhooks and events
type EventManager struct {
    webhooks map[string]Webhook
    events   chan Event
}
```

### Phase 2: Remove Non-Essential Features

1. **Remove container system**
   - Move to separate repository/plugin
   - Not core to VCS functionality

2. **Simplify distributed features**
   - Extract cluster/raft to plugin
   - Keep core simple

3. **Streamline API**
   - Focus on Git-compatible operations
   - Remove experimental endpoints

### Phase 3: Improve Storage Abstraction

Current storage is file-system biased. Need true abstraction:

```go
type ObjectStore interface {
    Get(hash string) (Object, error)
    Put(obj Object) (string, error)
    Exists(hash string) bool
}

type RefStore interface {
    GetRef(name string) (string, error)
    UpdateRef(name, hash string) error
    DeleteRef(name string) error
}

type WorkingStorage interface {
    Read(path string) ([]byte, error)
    Write(path string, data []byte) error
    Delete(path string) error
    List() ([]string, error)
}
```

### Phase 4: Consolidate Documentation

Reduce from 21 files to essential docs:
- README.md
- CONTRIBUTING.md
- API.md
- ARCHITECTURE.md
- CHANGELOG.md

## Implementation Order

1. **Week 1**: Create new clean structures
   - Design interfaces
   - Create workspace package
   - Create config package

2. **Week 2**: Migrate functionality
   - Move working tree operations to Workspace
   - Move config operations to Config
   - Simplify event system

3. **Week 3**: Remove non-essential features
   - Extract container system
   - Remove complex distributed features
   - Simplify API surface

4. **Week 4**: Testing and documentation
   - Update all tests
   - Consolidate documentation
   - Performance benchmarks

## Success Criteria

1. **Simpler codebase**: <50 files, <20k LOC
2. **Better separation**: Clear boundaries between concerns
3. **Higher test coverage**: >70% for core functionality
4. **Faster operations**: No performance regression
5. **Cleaner API**: Consistent, Git-compatible interface

## Risks and Mitigations

1. **Breaking changes**: Version as v0.4.0 with clear migration guide
2. **Feature removal**: Move to plugins, not delete
3. **Performance**: Benchmark before/after each change
4. **Compatibility**: Maintain Git-compatible API surface