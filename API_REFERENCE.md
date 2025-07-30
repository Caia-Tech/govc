# govc API Reference

## Core Types

### Repository

The main interface for interacting with a govc repository.

```go
type Repository interface {
    // Initialization
    Init() error
    Clone(url string) error
    
    // File operations
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, content []byte, mode os.FileMode) error
    DeleteFile(path string) error
    MoveFile(oldPath, newPath string) error
    ListFiles(path string) ([]string, error)
    FileExists(path string) bool
    
    // Git operations
    Add(path string) error
    Commit(message string, opts *CommitOptions) (*Commit, error)
    Branch(name string) error
    Checkout(ref string) error
    Merge(branch string, opts *MergeOptions) error
    DeleteBranch(name string) error
    ListBranches() ([]*Branch, error)
    GetCurrentBranch() (*Branch, error)
    
    // Tags
    Tag(name string, opts *TagOptions) error
    ListTags() ([]*Tag, error)
    DeleteTag(name string) error
    
    // History
    Log(opts *LogOptions) ([]*Commit, error)
    LogFile(path string, opts *LogOptions) ([]*Commit, error)
    GetCommit(hash string) (*Commit, error)
    Diff(from, to string) (string, error)
    Blame(path string) ([]*BlameLine, error)
    
    // Stashing
    Stash(opts *StashOptions) (string, error)
    StashApply(id string) error
    StashDrop(id string) error
    ListStashes() ([]*Stash, error)
    
    // Advanced operations
    CherryPick(hash string) error
    Revert(hash string) error
    Rebase(branch string, opts *RebaseOptions) error
    Reset(hash string, mode ResetMode) error
    
    // Parallel realities
    CreateParallelReality(name string) (*ParallelReality, error)
    ListParallelRealities() ([]*ParallelReality, error)
    MergeParallelReality(name string) error
    DeleteParallelReality(name string) error
    
    // Time travel
    TimeTravel(hash string) error
    TimeTravelToDate(date time.Time) error
    GetTimelinePosition() *TimelinePosition
    
    // Transactions
    BeginTransaction() (Transaction, error)
    
    // Hooks
    RegisterHook(event HookEvent, handler HookHandler) error
    
    // Search
    SearchContent(query string, opts *SearchOptions) ([]*SearchResult, error)
    SearchCommits(query string, opts *SearchOptions) ([]*Commit, error)
    Grep(pattern string, opts *GrepOptions) ([]*GrepResult, error)
    
    // Status
    GetStatus() (*Status, error)
    GetStatistics() *Statistics
    
    // Configuration
    EnableCache(opts CacheOptions)
}
```

### Commit

Represents a commit in the repository.

```go
type Commit struct {
    Hash      string
    Message   string
    Author    string
    Email     string
    Timestamp time.Time
    Parents   []string
    Tree      string
}
```

### Branch

Represents a branch in the repository.

```go
type Branch struct {
    Name      string
    Hash      string
    IsRemote  bool
    IsCurrent bool
    Upstream  string
}
```

### Tag

Represents a tag in the repository.

```go
type Tag struct {
    Name      string
    Hash      string
    Message   string
    Tagger    string
    Timestamp time.Time
}
```

### Transaction

Interface for atomic operations.

```go
type Transaction interface {
    WriteFile(path string, content []byte, mode os.FileMode) error
    DeleteFile(path string) error
    MoveFile(oldPath, newPath string) error
    Add(path string) error
    Commit() error
    Rollback() error
}
```

### ParallelReality

Represents an isolated workspace for experimentation.

```go
type ParallelReality struct {
    Name        string
    BaseCommit  string
    Created     time.Time
    Description string
    
    // Same methods as Repository
    WriteFile(path string, content []byte, mode os.FileMode) error
    Add(path string) error
    Commit(message string, opts *CommitOptions) (*Commit, error)
    // ... other Repository methods
}
```

## Option Types

### RepositoryOptions

Options for creating a repository.

```go
type RepositoryOptions struct {
    // Storage path (empty for in-memory)
    Path string
    
    // Use in-memory storage
    InMemory bool
    
    // Enable compression
    Compressed bool
    
    // Enable encryption
    Encrypted bool
    EncryptionKey []byte
    
    // Performance options
    CacheSize      int
    MaxParallelOps int
    
    // Advanced options
    DisableHooks   bool
    StrictMode     bool
}
```

### CommitOptions

Options for creating commits.

```go
type CommitOptions struct {
    // Author information
    Author string
    Email  string
    
    // Committer (defaults to author)
    Committer      string
    CommitterEmail string
    
    // GPG signing
    SigningKey string
    
    // Parent commits (for merge commits)
    Parents []string
    
    // Timestamp (defaults to now)
    Timestamp time.Time
    
    // Skip hooks
    SkipHooks bool
}
```

### MergeOptions

Options for merging branches.

```go
type MergeOptions struct {
    // Merge strategy
    Strategy MergeStrategy
    
    // Commit message
    Message string
    
    // Fast-forward only
    FFOnly bool
    
    // No fast-forward
    NoFF bool
    
    // Squash commits
    Squash bool
    
    // Conflict resolution
    ConflictResolution ConflictStrategy
}
```

### LogOptions

Options for retrieving commit history.

```go
type LogOptions struct {
    // Maximum number of commits
    MaxCount int
    
    // Skip commits
    Skip int
    
    // Filter by author
    Author string
    
    // Date range
    Since time.Time
    Until time.Time
    
    // Path filter
    Path string
    
    // Include merge commits
    IncludeMerges bool
    
    // Reverse order
    Reverse bool
}
```

### SearchOptions

Options for searching content and commits.

```go
type SearchOptions struct {
    // File pattern filter
    FilePattern string
    
    // Ignore case
    IgnoreCase bool
    
    // Include binary files
    IncludeBinary bool
    
    // Context lines
    Context int
    
    // Maximum results
    MaxResults int
    
    // Date range
    Since time.Time
    Until time.Time
    
    // Author filter
    Author string
}
```

### GrepOptions

Options for grep operations.

```go
type GrepOptions struct {
    // Use regex
    Regex bool
    
    // Case insensitive
    IgnoreCase bool
    
    // Show line numbers
    LineNumbers bool
    
    // Context lines
    BeforeContext int
    AfterContext  int
    
    // File filter
    Include []string
    Exclude []string
}
```

### StashOptions

Options for stashing changes.

```go
type StashOptions struct {
    // Stash message
    Message string
    
    // Include untracked files
    IncludeUntracked bool
    
    // Keep index
    KeepIndex bool
}
```

### TagOptions

Options for creating tags.

```go
type TagOptions struct {
    // Tag message
    Message string
    
    // Target commit
    Commit string
    
    // GPG sign
    Sign bool
    
    // Force (overwrite existing)
    Force bool
}
```

### CacheOptions

Options for enabling cache.

```go
type CacheOptions struct {
    // Cache size (number of objects)
    Size int
    
    // Time to live
    TTL time.Duration
    
    // Cache statistics
    EnableStats bool
}
```

## Enums

### MergeStrategy

```go
type MergeStrategy string

const (
    MergeStrategyRecursive MergeStrategy = "recursive"
    MergeStrategyOurs      MergeStrategy = "ours"
    MergeStrategyTheirs    MergeStrategy = "theirs"
    MergeStrategyOctopus   MergeStrategy = "octopus"
)
```

### ResetMode

```go
type ResetMode string

const (
    ResetSoft  ResetMode = "soft"
    ResetMixed ResetMode = "mixed"
    ResetHard  ResetMode = "hard"
)
```

### HookEvent

```go
type HookEvent string

const (
    HookPreCommit  HookEvent = "pre-commit"
    HookPostCommit HookEvent = "post-commit"
    HookPrePush    HookEvent = "pre-push"
    HookPostPush   HookEvent = "post-push"
    HookPreMerge   HookEvent = "pre-merge"
    HookPostMerge  HookEvent = "post-merge"
)
```

### ConflictStrategy

```go
type ConflictStrategy string

const (
    ConflictManual    ConflictStrategy = "manual"
    ConflictOurs      ConflictStrategy = "ours"
    ConflictTheirs    ConflictStrategy = "theirs"
    ConflictUnion     ConflictStrategy = "union"
)
```

## Result Types

### Status

Repository status information.

```go
type Status struct {
    Branch    string
    Staged    []FileStatus
    Modified  []FileStatus
    Untracked []FileStatus
    Conflicts []FileStatus
    Ahead     int
    Behind    int
}

type FileStatus struct {
    Path   string
    Status string // "added", "modified", "deleted", "renamed"
    OldPath string // For renames
}
```

### Statistics

Repository statistics.

```go
type Statistics struct {
    ObjectCount   int
    MemoryUsage   int64
    DiskUsage     int64
    CacheHits     int64
    CacheMisses   int64
    Commits       int
    Branches      int
    Tags          int
    Files         int
}
```

### SearchResult

Content search result.

```go
type SearchResult struct {
    File    string
    Line    int
    Column  int
    Content string
    Context []string
}
```

### GrepResult

Grep operation result.

```go
type GrepResult struct {
    File       string
    Line       int
    Match      string
    BeforeContext []string
    AfterContext  []string
}
```

### BlameLine

Blame information for a line.

```go
type BlameLine struct {
    Line       int
    Content    string
    CommitHash string
    Author     string
    Timestamp  time.Time
}
```

### Stash

Stashed changes.

```go
type Stash struct {
    ID        string
    Message   string
    Timestamp time.Time
    Branch    string
}
```

### TimelinePosition

Current position in repository timeline.

```go
type TimelinePosition struct {
    Commit    string
    Branch    string
    Timestamp time.Time
    IsDetached bool
}
```

## Hook Context

Context passed to hook handlers.

```go
type HookContext interface {
    GetEvent() HookEvent
    GetRepository() Repository
    GetCommit() *Commit
    GetStagedFiles() []FileInfo
    GetBranch() string
    GetUser() string
}

type FileInfo struct {
    Name string
    Size int64
    Mode os.FileMode
}
```

## Error Types

```go
var (
    ErrNotInitialized   = errors.New("repository not initialized")
    ErrBranchNotFound   = errors.New("branch not found")
    ErrCommitNotFound   = errors.New("commit not found")
    ErrFileNotFound     = errors.New("file not found")
    ErrConflict         = errors.New("merge conflict")
    ErrNoChanges        = errors.New("no changes to commit")
    ErrInvalidRef       = errors.New("invalid reference")
    ErrStashNotFound    = errors.New("stash not found")
    ErrHookFailed       = errors.New("hook execution failed")
    ErrTransactionFailed = errors.New("transaction failed")
)

// ConflictError provides details about merge conflicts
type ConflictError struct {
    Conflicts []ConflictInfo
}

type ConflictInfo struct {
    Path  string
    Ours  []byte
    Theirs []byte
    Base  []byte
}
```

## Constructor Functions

```go
// Create a new in-memory repository
func NewRepository() (Repository, error)

// Create a repository with specific path
func NewRepositoryWithPath(path string) (Repository, error)

// Create a repository with options
func NewRepositoryWithOptions(opts RepositoryOptions) (Repository, error)

// Create a Git importer
func NewGitImporter() *GitImporter

// Create a Git exporter
func NewGitExporter() *GitExporter
```

## Import/Export

### GitImporter

```go
type GitImporter struct {
    // Import a Git repository
    Import(gitPath string) (Repository, error)
    
    // Import with progress callback
    ImportWithProgress(gitPath string, progress func(float64)) (Repository, error)
    
    // Import specific branch
    ImportBranch(gitPath, branch string) (Repository, error)
}
```

### GitExporter

```go
type GitExporter struct {
    // Export to Git format
    Export(repo Repository, path string) error
    
    // Export specific branch
    ExportBranch(repo Repository, branch, path string) error
    
    // Export with options
    ExportWithOptions(repo Repository, path string, opts ExportOptions) error
}

type ExportOptions struct {
    Branches []string
    Tags     []string
    Bare     bool
    Force    bool
}
```

## Pool Management

### RepositoryPool

```go
type RepositoryPool struct {
    // Get repository from pool
    Get(id string) (Repository, error)
    
    // Return repository to pool
    Put(repo Repository)
    
    // Remove from pool
    Remove(id string)
    
    // Get statistics
    GetStatistics() PoolStatistics
    
    // Start pool
    Start()
    
    // Stop pool
    Stop()
}

type PoolOptions struct {
    MaxSize         int
    MaxIdleTime     time.Duration
    CleanupInterval time.Duration
}

type PoolStatistics struct {
    ActiveCount  int
    IdleCount    int
    TotalCreated int64
    CacheHits    int64
    CacheMisses  int64
}

// Create a new repository pool
func NewRepositoryPool(opts PoolOptions) *RepositoryPool
```

## Client Libraries

### Go Client

See `client/go/govc` package for the Go client implementation.

### JavaScript/TypeScript Client

See `client/js` directory for the JavaScript/TypeScript SDK.

## Server API

### REST Endpoints

See `api/` directory for the complete REST API implementation including:
- Repository management
- File operations
- Git operations
- Search and query
- Hooks and events
- Authentication and authorization

## Examples

See the `examples/` directory for complete working examples demonstrating all features.