# Phase 1: Core API Completion - Detailed Task Breakdown

## Week 1: File Operations & Basic Features

### Day 1-2: File Operations
- [ ] Implement `GET /api/v1/repos/{repo_id}/read/*path`
  - Read file content at current HEAD
  - Support reading from specific commit/branch via query params
  - Return base64 for binary files
  - Tests: reading existing files, non-existent files, binary files
  
- [ ] Implement `GET /api/v1/repos/{repo_id}/tree/*path`
  - List directory contents
  - Include file metadata (size, mode, last modified)
  - Support recursive listing
  - Tests: root directory, subdirectories, empty directories

- [ ] Implement `DELETE /api/v1/repos/{repo_id}/remove/*path`
  - Remove files from staging
  - Support batch removal
  - Tests: remove single file, remove directory, remove non-existent

- [ ] Implement `POST /api/v1/repos/{repo_id}/move`
  ```json
  {
    "from": "old/path/file.txt",
    "to": "new/path/file.txt"
  }
  ```
  - Move/rename files
  - Preserve history
  - Tests: rename file, move to different directory, invalid paths

### Day 3-4: Diff & Blame
- [ ] Implement `GET /api/v1/repos/{repo_id}/diff`
  - Diff between commits: `?from=abc123&to=def456`
  - Diff between branches: `?from=main&to=feature`
  - Diff working directory: `?from=HEAD`
  - Support different formats (unified, raw, name-only)
  - Tests: commit diff, branch diff, working directory diff

- [ ] Implement `GET /api/v1/repos/{repo_id}/blame/*path`
  - Line-by-line commit attribution
  - Include commit hash, author, timestamp, message
  - Support blame at specific commit
  - Tests: blame current file, blame at historical commit

### Day 5: Stash Operations
- [ ] Implement stash data structure
  ```go
  type Stash struct {
      ID        string
      Message   string
      Timestamp time.Time
      Changes   map[string][]byte
      Branch    string
  }
  ```

- [ ] Implement `POST /api/v1/repos/{repo_id}/stash`
  - Save current working directory changes
  - Optional stash message
  - Tests: stash changes, stash with message

- [ ] Implement `GET /api/v1/repos/{repo_id}/stash/list`
  - List all stashes with metadata
  - Tests: list empty, list multiple stashes

- [ ] Implement `POST /api/v1/repos/{repo_id}/stash/apply/{stash_id}`
  - Apply stashed changes
  - Handle conflicts
  - Option to drop stash after apply
  - Tests: apply clean, apply with conflicts

## Week 2: Advanced Git Operations

### Day 6-7: Cherry-pick & Revert
- [ ] Implement `POST /api/v1/repos/{repo_id}/cherry-pick`
  ```json
  {
    "commits": ["abc123", "def456"],
    "strategy": "recursive"
  }
  ```
  - Apply specific commits to current branch
  - Support multiple commits
  - Handle conflicts
  - Tests: single commit, multiple commits, with conflicts

- [ ] Implement `POST /api/v1/repos/{repo_id}/revert`
  ```json
  {
    "commits": ["abc123"],
    "no_commit": false
  }
  ```
  - Create revert commits
  - Support reverting merge commits
  - Tests: revert regular commit, revert merge commit

### Day 8-9: Reset & Rebase
- [ ] Implement `POST /api/v1/repos/{repo_id}/reset`
  ```json
  {
    "target": "abc123",
    "mode": "hard|soft|mixed"
  }
  ```
  - Reset to specific commit
  - Support different reset modes
  - Tests: hard reset, soft reset, mixed reset

- [ ] Implement `POST /api/v1/repos/{repo_id}/rebase`
  ```json
  {
    "onto": "main",
    "from": "feature-branch",
    "interactive": false
  }
  ```
  - Basic rebase functionality
  - Interactive rebase support
  - Conflict resolution
  - Tests: clean rebase, rebase with conflicts

### Day 10: Advanced Transaction Features
- [ ] Add transaction squashing
  ```go
  func (tc *TransactionalCommit) Squash(message string) (*object.Commit, error)
  ```

- [ ] Add transaction amending
  ```go
  func (tc *TransactionalCommit) Amend(commitHash string) error
  ```

- [ ] Add dry-run validation
  - Validate without committing
  - Return potential conflicts
  - Tests: dry run success, dry run with issues

## Week 3: Search, Hooks & Polish

### Day 11-12: Search Implementation
- [ ] Create search index structure
  ```go
  type SearchIndex struct {
      Commits  map[string]*CommitIndex
      Content  map[string]*ContentIndex
      Files    *FileIndex
  }
  ```

- [ ] Implement `GET /api/v1/repos/{repo_id}/search/commits`
  - Search by message, author, date range
  - Support regex patterns
  - Pagination
  - Tests: search by message, by author, by date

- [ ] Implement `GET /api/v1/repos/{repo_id}/search/content`
  - Full-text search in file content
  - Support regex and glob patterns
  - Return context lines
  - Tests: simple search, regex search, case sensitivity

- [ ] Implement `GET /api/v1/repos/{repo_id}/search/files`
  - Search by filename patterns
  - Support wildcards
  - Tests: exact match, wildcard match

### Day 13: Hooks System
- [ ] Design hook system
  ```go
  type Hook struct {
      ID       string
      Type     HookType
      URL      string
      Events   []string
      Secret   string
      Active   bool
  }
  ```

- [ ] Implement `POST /api/v1/repos/{repo_id}/hooks`
  - Register webhooks
  - Validate webhook URL
  - Tests: create hook, invalid URL

- [ ] Implement `GET /api/v1/repos/{repo_id}/hooks`
  - List all hooks
  - Tests: list hooks, empty list

- [ ] Implement `DELETE /api/v1/repos/{repo_id}/hooks/{hook_id}`
  - Remove webhook
  - Tests: delete existing, delete non-existent

- [ ] Implement hook triggering
  - Pre-commit validation
  - Post-commit notifications
  - Error handling and retries

### Day 14: Event Streaming
- [ ] Implement `GET /api/v1/repos/{repo_id}/events` (SSE)
  - Real-time event streaming
  - Event types: commit, branch, tag, push
  - Client reconnection support
  - Tests: event delivery, reconnection

### Day 15: Testing & Documentation
- [ ] Write comprehensive integration tests
- [ ] Update API documentation
- [ ] Add example code for each endpoint
- [ ] Performance benchmarks for new endpoints
- [ ] Update Postman/Insomnia collections

## Code Organization

### New Files to Create:
```
api/
├── handlers_file.go      # File operations
├── handlers_diff.go      # Diff and blame
├── handlers_stash.go     # Stash operations
├── handlers_advanced.go  # Cherry-pick, revert, reset, rebase
├── handlers_search.go    # Search functionality
├── handlers_hooks.go     # Webhooks
├── handlers_events.go    # SSE events
├── search/
│   ├── index.go         # Search index
│   ├── commits.go       # Commit search
│   ├── content.go       # Content search
│   └── files.go         # File search
├── hooks/
│   ├── manager.go       # Hook management
│   ├── trigger.go       # Hook triggering
│   └── delivery.go      # Webhook delivery
└── events/
    ├── emitter.go       # Event emitter
    └── stream.go        # SSE stream handler
```

### New Types to Add:
```go
// types.go additions
type ReadFileRequest struct {
    Path   string `json:"path"`
    Ref    string `json:"ref,omitempty"`
}

type DiffRequest struct {
    From   string `json:"from"`
    To     string `json:"to"`
    Format string `json:"format,omitempty"`
}

type StashRequest struct {
    Message string `json:"message,omitempty"`
}

type CherryPickRequest struct {
    Commits  []string `json:"commits"`
    Strategy string   `json:"strategy,omitempty"`
}

type RebaseRequest struct {
    Onto        string `json:"onto"`
    From        string `json:"from,omitempty"`
    Interactive bool   `json:"interactive"`
}

type SearchRequest struct {
    Query   string `json:"query"`
    Type    string `json:"type"`
    Options map[string]interface{} `json:"options,omitempty"`
}
```

## Testing Strategy

### Unit Tests
- Each handler function gets its own test file
- Mock the repository layer for isolated testing
- Test error conditions and edge cases

### Integration Tests
- Full workflow tests for each feature
- Test interaction between features
- Performance benchmarks

### Example Test Cases:
```go
// Test stash workflow
func TestStashWorkflow(t *testing.T) {
    // 1. Create repo
    // 2. Add files
    // 3. Make changes
    // 4. Stash changes
    // 5. Verify working directory clean
    // 6. Apply stash
    // 7. Verify changes restored
}

// Test search functionality
func TestSearchIntegration(t *testing.T) {
    // 1. Create repo with multiple commits
    // 2. Search by commit message
    // 3. Search by file content
    // 4. Search by filename
    // 5. Verify results accuracy
}
```

## Performance Targets

- File read: < 10ms for files under 1MB
- Diff generation: < 50ms for typical commits
- Search: < 100ms for repos with 10k commits
- Stash operations: < 20ms
- Cherry-pick: < 100ms per commit
- Event streaming: < 5ms latency

## Definition of Done

- [ ] All endpoints implemented with tests
- [ ] API documentation updated
- [ ] Performance benchmarks pass
- [ ] No race conditions (verified with -race)
- [ ] Code coverage > 80%
- [ ] Example code for each feature
- [ ] Error handling consistent
- [ ] Logging added for debugging

## Next Phase Preview

After completing Phase 1, we'll have a feature-complete Git API. Phase 2 will focus on:
- Authentication and authorization
- Production infrastructure
- Monitoring and observability
- Performance optimization
- Deployment readiness

This sets the foundation for building client libraries and advanced features in subsequent phases.