# govc Architecture Refactoring Implementation Plan

## Overview

This plan details the steps to refactor govc's architecture based on the analysis in ARCHITECTURE_ANALYSIS.md. The refactoring will be done incrementally to maintain stability while addressing fundamental issues.

## Phase 1: Core Storage Abstraction (Days 1-5)

### Day 1: Define Storage Interfaces

**Files to create:**
- `pkg/storage/interfaces.go` - Core storage interfaces
- `pkg/storage/memory.go` - Memory implementation
- `pkg/storage/hybrid.go` - Memory-first with disk backup

**Tasks:**
1. Define ObjectStore interface
2. Define RefStore interface  
3. Define WorkingStorage interface
4. Create memory implementations
5. Write comprehensive tests

**Success Criteria:**
- Storage operations work through interfaces
- Memory implementation passes all tests
- No direct file system access in core

### Day 2: Refactor Object Storage

**Files to modify:**
- `pkg/storage/store.go` - Adapt to ObjectStore interface
- `repository.go` - Use ObjectStore instead of storage.Store

**Tasks:**
1. Make existing Store implement ObjectStore
2. Update Repository to use interface
3. Remove file system assumptions
4. Update tests

**Success Criteria:**
- Repository uses ObjectStore interface
- All object operations work in memory
- Existing tests pass

### Day 3: Refactor Reference Management

**Files to modify:**
- `pkg/refs/ref_manager.go` - Adapt to RefStore interface
- `repository.go` - Use RefStore interface

**Tasks:**
1. Make RefManager implement RefStore
2. Add memory-only ref storage
3. Update Repository to use interface
4. Test branch operations in memory

**Success Criteria:**
- References work through interface
- Branch operations work in memory
- No file system dependencies

### Day 4: Create WorkingDirectory Abstraction

**Files to create:**
- `working_directory.go` - New WorkingDirectory type
- `workspace.go` - Coordinator between repo and working dir

**Tasks:**
1. Create WorkingDirectory type
2. Implement WorkingStorage for memory
3. Create Workspace coordinator
4. Move staging area to Workspace

**Success Criteria:**
- Working directory is separate from Repository
- Can switch branches without state leakage
- Staging area works with new model

### Day 5: Fix Branch Isolation

**Files to modify:**
- `repository.go` - Remove worktree from Repository
- `api/handlers_branch.go` - Use Workspace for checkout

**Tasks:**
1. Remove worktree from Repository struct
2. Update checkout to use Workspace
3. Ensure working directory cleared on branch switch
4. Fix failing branch isolation tests

**Success Criteria:**
- Branch isolation tests pass
- No file leakage between branches
- Working directory properly synced

## Phase 2: Domain Model Cleanup (Days 6-8)

### Day 6: Create DTOs for Serialization

**Files to create:**
- `api/dto/repository.go` - Repository DTOs
- `api/dto/cluster.go` - Cluster DTOs
- `api/dto/mappers.go` - Domain to DTO mappers

**Tasks:**
1. Define serializable DTOs
2. Create mappers from domain objects
3. Update API handlers to use DTOs
4. Remove JSON tags from domain objects

**Success Criteria:**
- Domain objects have no JSON tags
- All API responses use DTOs
- Cluster can serialize state

### Day 7: Event System Implementation

**Files to create:**
- `pkg/events/interfaces.go` - Event interfaces
- `pkg/events/bus.go` - Event bus implementation
- `pkg/events/types.go` - Event type definitions

**Tasks:**
1. Define Event and EventBus interfaces
2. Implement in-memory event bus
3. Define repository event types
4. Add event publishing to Repository

**Success Criteria:**
- Events published for key operations
- Event bus handles concurrent publishers
- Can subscribe/unsubscribe dynamically

### Day 8: Clean Repository Interface

**Files to modify:**
- `repository.go` - Simplify Repository struct
- Create repository_builder.go - Builder pattern

**Tasks:**
1. Remove non-core fields from Repository
2. Create RepositoryBuilder
3. Update initialization code
4. Ensure backward compatibility

**Success Criteria:**
- Repository struct is clean
- Builder provides flexible initialization
- All tests still pass

## Phase 3: API v2 Design (Days 9-11)

### Day 9: API v2 Specification

**Files to create:**
- `api/v2/spec.yaml` - OpenAPI specification
- `api/v2/types.go` - Request/response types
- `api/v2/routes.go` - Route definitions

**Tasks:**
1. Design consistent API patterns
2. Create OpenAPI specification
3. Define request/response types
4. Plan migration strategy

**Success Criteria:**
- Complete OpenAPI spec
- Consistent naming and patterns
- Clear migration path from v1

### Day 10: Implement Core v2 Endpoints

**Files to create:**
- `api/v2/handlers_repo.go` - Repository handlers
- `api/v2/handlers_git.go` - Git operation handlers
- `api/v2/middleware.go` - v2 specific middleware

**Tasks:**
1. Implement repository CRUD
2. Implement Git operations
3. Use consistent response format
4. Add proper error handling

**Success Criteria:**
- Core endpoints working
- Consistent response format
- Proper HTTP status codes

### Day 11: API Versioning Infrastructure

**Files to create:**
- `api/version_negotiation.go` - Version selection
- `api/v1/adapter.go` - v1 to v2 adapter

**Tasks:**
1. Implement version negotiation
2. Create v1 compatibility layer
3. Update router for versioning
4. Test both API versions

**Success Criteria:**
- Version negotiation works
- v1 API still functional
- Can use both versions simultaneously

## Phase 4: Plugin Architecture (Days 12-14)

### Day 12: Plugin System Core

**Files to create:**
- `pkg/plugin/interfaces.go` - Plugin interfaces
- `pkg/plugin/manager.go` - Plugin manager
- `pkg/plugin/loader.go` - Plugin loader

**Tasks:**
1. Define Plugin interface
2. Create plugin manager
3. Implement plugin lifecycle
4. Add plugin configuration

**Success Criteria:**
- Plugin system initialized
- Can load/unload plugins
- Plugin lifecycle works

### Day 13: Extract Container System

**Files to create:**
- `plugins/container/plugin.go` - Container plugin
- `plugins/container/README.md` - Plugin documentation

**Tasks:**
1. Move container code to plugin
2. Implement plugin interface
3. Update API routes
4. Test as plugin

**Success Criteria:**
- Container system works as plugin
- Can enable/disable container features
- No container code in core

### Day 14: Extract Cluster Features

**Files to create:**
- `plugins/cluster/plugin.go` - Cluster plugin
- `plugins/cluster/README.md` - Plugin documentation

**Tasks:**
1. Move cluster code to plugin
2. Fix serialization issues
3. Use event bus for distribution
4. Test cluster as plugin

**Success Criteria:**
- Cluster works as plugin
- Serialization issues resolved
- Can run with/without cluster

## Phase 5: Testing and Documentation (Days 15-16)

### Day 15: Comprehensive Testing

**Tasks:**
1. Fix all failing tests
2. Add integration tests for new architecture
3. Performance benchmarks
4. Load testing for plugins

**Success Criteria:**
- All tests pass
- Performance improved
- No regression in functionality

### Day 16: Documentation Update

**Files to update:**
- `README.md` - Updated architecture
- `API_REFERENCE.md` - v2 API documentation
- `PLUGIN_DEVELOPMENT.md` - Plugin guide

**Tasks:**
1. Update architecture documentation
2. Document v2 API
3. Create plugin development guide
4. Migration guide from v1

**Success Criteria:**
- Clear documentation
- API fully documented
- Plugin development guide complete

## Implementation Guidelines

### Principles
1. **Incremental Changes**: Each day's work should leave the system functional
2. **Test-Driven**: Write tests before implementation
3. **Backward Compatible**: Don't break existing functionality
4. **Performance Focus**: Benchmark critical paths

### Testing Strategy
- Unit tests for each new component
- Integration tests for workflows
- Benchmark tests for performance
- Load tests for concurrency

### Code Review Process
1. Self-review against architecture principles
2. Ensure tests pass
3. Check for backward compatibility
4. Verify documentation updated

### Risk Management
- Keep changes small and focused
- Maintain v1 API during transition
- Feature flags for new functionality
- Rollback plan for each phase

## Success Metrics

1. **Code Quality**
   - All tests passing
   - No architectural workarounds
   - Clean separation of concerns

2. **Performance**
   - Memory operations faster than disk
   - Reduced memory footprint
   - Better concurrency handling

3. **Maintainability**
   - Clear module boundaries
   - Consistent patterns
   - Comprehensive documentation

4. **Extensibility**
   - Working plugin system
   - Easy to add features
   - No core modifications needed

## Conclusion

This plan provides a systematic approach to refactoring govc's architecture. By following this incremental approach, we can address fundamental issues while maintaining stability and backward compatibility.