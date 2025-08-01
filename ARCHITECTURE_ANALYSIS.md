# govc Architecture Analysis

## Executive Summary

This document analyzes the current architecture of govc based on test failures and code review. It identifies fundamental architectural issues and proposes solutions for creating a robust, scalable memory-first Git implementation.

## Current State Analysis

### Test Results Summary
- **12 packages passing tests**
- **6 packages with failures**
  - API: Complex integration tests failing
  - Cluster: Serialization and concurrency issues
  - Tests: E2E failures due to architectural issues
  - Examples: Outdated API usage

### Core Architectural Problems Identified

#### 1. Working Directory State Management

**Problem**: govc conflates Git's immutable object model with mutable working directory state. Branches try to contain working directory state, breaking isolation when switching branches.

**Evidence**: 
- `Makefile` exists on main branch when it shouldn't
- Files from feature branches leak into main
- Test: `TestCompleteGitWorkflow/Verify_main_branch_state` fails

**Root Cause**: The `Repository` struct mixes concerns:
```go
type Repository struct {
    store      *storage.Store      // Immutable objects (good)
    worktree   *Worktree          // Mutable state (mixed concern)
    staging    *StagingArea       // Mutable state (mixed concern)
    // ...
}
```

#### 2. Serialization Architecture

**Problem**: Domain objects contain non-serializable fields (channels, mutexes, interfaces), causing cluster state persistence to fail.

**Evidence**:
- Cluster tests panic when trying to marshal Repository
- Node struct contains `repositories map[string]*govc.Repository`
- JSON marshaling fails on channels and mutexes

**Root Cause**: No separation between domain objects and data transfer objects (DTOs).

#### 3. Memory-First vs File System Reality

**Problem**: Despite claiming to be "memory-first", the codebase has deep file system assumptions throughout.

**Evidence**:
- `Worktree` assumes file paths exist
- Tests use file operations directly
- Memory mode is implemented as special case rather than default

**Root Cause**: File system operations are baked into core abstractions rather than being one implementation of a storage interface.

#### 4. API Consistency and Evolution

**Problem**: The API has evolved organically without clear versioning or consistency.

**Evidence**:
- Mixed response formats (some return `changes`, others `staged`/`modified`)
- Backward compatibility hacks added ad-hoc
- Different endpoints use different parameter names (`q` vs `query`)

**Root Cause**: No API design guidelines or versioning strategy.

#### 5. Feature Creep and Lack of Focus

**Problem**: govc tries to be multiple systems at once, diluting its core value proposition.

**Evidence**:
- ~9000 lines of AI code removed
- Container system mixed with Git implementation
- Distributed system features incomplete

**Root Cause**: No clear architectural boundaries or plugin system.

## Proposed Architecture

### Design Principles

1. **Separation of Concerns**: Each component has a single, well-defined responsibility
2. **Interface-Based Design**: Depend on abstractions, not concrete implementations
3. **Event-Driven Architecture**: Components communicate through events, not direct coupling
4. **True Memory-First**: Memory is the default, file system is an implementation detail
5. **Plugin Architecture**: Non-core features are optional plugins

### Component Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        API Layer (v1/v2)                     │
├─────────────────────────────────────────────────────────────┤
│                     Core Git Engine                          │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐       │
│  │  Repository  │  │   Working   │  │   Staging    │       │
│  │  (Immutable) │  │  Directory  │  │    Area      │       │
│  └─────────────┘  └─────────────┘  └──────────────┘       │
├─────────────────────────────────────────────────────────────┤
│                    Storage Abstraction                       │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐       │
│  │   Object    │  │     Ref     │  │   Working    │       │
│  │    Store    │  │    Store    │  │   Storage    │       │
│  └─────────────┘  └─────────────┘  └──────────────┘       │
├─────────────────────────────────────────────────────────────┤
│                    Event Bus                                 │
├─────────────────────────────────────────────────────────────┤
│                    Plugin System                             │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐       │
│  │  Container  │  │   Cluster   │  │     AI       │       │
│  │   Plugin    │  │   Plugin    │  │   Plugin     │       │
│  └─────────────┘  └─────────────┘  └──────────────┘       │
└─────────────────────────────────────────────────────────────┘
```

### Core Abstractions

#### Storage Layer
```go
// Object storage for immutable Git objects
type ObjectStore interface {
    Get(hash string) (Object, error)
    Put(obj Object) (string, error)
    Exists(hash string) bool
}

// Reference storage for branches/tags
type RefStore interface {
    GetRef(name string) (string, error)
    UpdateRef(name string, hash string) error
    DeleteRef(name string) error
    ListRefs() ([]Ref, error)
}

// Working directory storage
type WorkingStorage interface {
    Read(path string) ([]byte, error)
    Write(path string, data []byte) error
    Delete(path string) error
    List() ([]string, error)
    Clear() error
}
```

#### Domain Model
```go
// Pure domain object - no I/O, no serialization
type Repository struct {
    objects ObjectStore
    refs    RefStore
}

// Manages mutable working directory state
type WorkingDirectory struct {
    storage WorkingStorage
    base    string // commit hash we're based on
}

// Coordinates between repository and working directory
type Workspace struct {
    repo    *Repository
    workdir *WorkingDirectory
    staging *StagingArea
}
```

#### Event System
```go
type Event interface {
    Type() string
    Timestamp() time.Time
    Data() interface{}
}

type EventBus interface {
    Publish(event Event) error
    Subscribe(eventType string, handler func(Event)) (unsubscribe func())
}
```

#### Plugin Architecture
```go
type Plugin interface {
    Name() string
    Version() string
    Initialize(core Core) error
    Shutdown() error
}

type Core interface {
    Repositories() RepositoryManager
    Events() EventBus
    Storage() StorageFactory
}
```

### API Design

#### Versioning Strategy
- `/api/v1/*` - Current API with compatibility fixes
- `/api/v2/*` - New consistent API
- Version negotiation via Accept header

#### Consistent Response Format
```go
type Response[T any] struct {
    Data    T              `json:"data"`
    Error   *ErrorInfo     `json:"error,omitempty"`
    Meta    *MetaInfo      `json:"meta,omitempty"`
}

type ErrorInfo struct {
    Code    string         `json:"code"`
    Message string         `json:"message"`
    Details interface{}    `json:"details,omitempty"`
}
```

## Implementation Phases

### Phase 1: Core Refactoring (Week 1-2)
1. Implement storage abstractions
2. Separate Repository from WorkingDirectory
3. Create Workspace coordinator
4. Fix branch isolation issue

### Phase 2: API Stabilization (Week 3)
1. Design v2 API specification
2. Implement DTOs for serialization
3. Add OpenAPI documentation
4. Create migration guide

### Phase 3: Plugin Architecture (Week 4)
1. Define plugin interfaces
2. Extract container system to plugin
3. Move cluster features to plugin
4. Create plugin loader/manager

### Phase 4: Testing and Migration (Week 5)
1. Comprehensive integration tests
2. Performance benchmarks
3. Migration tools for existing code
4. Documentation updates

## Benefits of This Architecture

1. **Solves Branch Isolation**: Clear separation between immutable objects and working state
2. **Enables Proper Serialization**: DTOs separate from domain objects
3. **True Memory-First**: Storage is abstracted, memory is default
4. **Extensible**: Plugins allow features without core complexity
5. **Maintainable**: Clear boundaries and single responsibilities
6. **Testable**: Interfaces allow easy mocking and testing

## Risks and Mitigation

### Risk: Breaking Changes
**Mitigation**: 
- Maintain v1 API with adapters
- Provide migration tools
- Extensive documentation

### Risk: Performance Regression
**Mitigation**:
- Benchmark critical paths
- Memory-first design should improve performance
- Profile and optimize hot paths

### Risk: Plugin Complexity
**Mitigation**:
- Start with simple plugin interface
- Provide plugin SDK and examples
- Core features remain in core

## Success Criteria

1. All tests pass without architectural workarounds
2. Branch isolation works correctly
3. Cluster can serialize state properly
4. API v2 is consistent and documented
5. Plugins can extend functionality without modifying core
6. Performance improves for memory operations

## Conclusion

The current architecture has fundamental issues that can't be fixed with patches. This proposed architecture addresses root causes while maintaining the memory-first vision of govc. The phased implementation allows incremental progress while maintaining stability.