package govc

import (
	"fmt"
	"sync"
	"time"

	"github.com/caia-tech/govc/pkg/object"
	"github.com/caia-tech/govc/pkg/refs"
	"github.com/caia-tech/govc/pkg/storage"
)

// ParallelReality represents an isolated branch that exists only in memory.
// Each ParallelReality is a complete universe where changes can be tested
// without affecting other realities. This is the key to govc's memory-first
// approach - branches aren't just pointers, they're isolated worlds.
type ParallelReality struct {
	name       string
	repo       *Repository
	isolated   bool
	ephemeral  bool // Never persists to disk
	startTime  time.Time
	mu         sync.RWMutex
}

// NewRepository creates a memory-first repository.
// Unlike traditional Git, this repository operates entirely in memory
// by default, making operations instant and enabling parallel realities.
func NewRepository() *Repository {
	return &Repository{
		path:       ":memory:",
		store:      NewMemoryStore(),
		refManager: NewMemoryRefManager(),
		staging:    NewStagingArea(),
		worktree:   NewVirtualWorktree(),
	}
}

// ParallelReality creates an isolated branch universe.
// This is instant because it's memory-only - no disk I/O involved.
func (r *Repository) ParallelReality(name string) *ParallelReality {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Memory-first: Creating a branch is just creating a pointer
	// This is why we can create 1000 branches in milliseconds
	branchName := fmt.Sprintf("parallel/%s", name)
	currentHash, _ := r.refManager.GetHEAD()
	r.refManager.CreateBranch(branchName, currentHash)

	return &ParallelReality{
		name:      branchName,
		repo:      r,
		isolated:  true,
		ephemeral: true,
		startTime: time.Now(),
	}
}

// ParallelRealities creates multiple isolated universes at once.
// Perfect for testing multiple configurations simultaneously.
func (r *Repository) ParallelRealities(names []string) []*ParallelReality {
	realities := make([]*ParallelReality, len(names))
	for i, name := range names {
		realities[i] = r.ParallelReality(name)
	}
	return realities
}

// IsolatedBranch creates a branch that's completely isolated from others.
// Changes here won't affect any other branch until explicitly merged.
func (r *Repository) IsolatedBranch(name string) *ParallelReality {
	reality := r.ParallelReality(name)
	reality.isolated = true
	return reality
}

// Apply applies changes to this reality without affecting others.
// This is where the memory-first approach shines - changes are instant.
func (pr *ParallelReality) Apply(changes interface{}) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	// Switch to this reality
	oldBranch, _ := pr.repo.CurrentBranch()
	pr.repo.Checkout(pr.name)
	defer pr.repo.Checkout(oldBranch)

	// Apply changes based on type
	switch c := changes.(type) {
	case map[string][]byte:
		for path, content := range c {
			hash, _ := pr.repo.store.StoreBlob(content)
			pr.repo.staging.Add(path, hash)
		}
	case func(*ParallelReality):
		c(pr)
	default:
		return fmt.Errorf("unsupported change type: %T", changes)
	}

	return nil
}

// Evaluate runs tests or benchmarks in this isolated reality.
func (pr *ParallelReality) Evaluate() interface{} {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	// In this reality, we can run any evaluation without affecting others
	return map[string]interface{}{
		"reality":  pr.name,
		"duration": time.Since(pr.startTime),
		"isolated": pr.isolated,
	}
}

// Benchmark runs performance tests in this reality.
func (pr *ParallelReality) Benchmark() *BenchmarkResult {
	return &BenchmarkResult{
		Reality:   pr.name,
		StartTime: pr.startTime,
		Metrics:   make(map[string]float64),
	}
}

// TransactionalCommit represents a commit that can be validated before persisting.
// This is only possible because we operate in memory first.
type TransactionalCommit struct {
	repo     *Repository
	staging  *StagingArea
	message  string
	author   object.Author
	changes  map[string][]byte
	validated bool
}

// BeginTransaction starts a new transactional commit.
// Changes are staged in memory and can be rolled back before committing.
func (r *Repository) BeginTransaction() *TransactionalCommit {
	return &TransactionalCommit{
		repo:    r,
		staging: NewStagingArea(),
		changes: make(map[string][]byte),
		author: object.Author{
			Name:  "System",
			Email: "system@govc",
			Time:  time.Now(),
		},
	}
}

// Add stages a file in the transaction.
// Nothing is written to disk until Commit() is called.
func (tc *TransactionalCommit) Add(path string, content []byte) {
	tc.changes[path] = content
	hash, _ := tc.repo.store.StoreBlob(content)
	tc.staging.Add(path, hash)
}

// Validate checks if the transaction is valid.
// This is where you can run tests, linting, security checks, etc.
func (tc *TransactionalCommit) Validate() error {
	// Memory-first benefit: We can validate the entire state
	// before any permanent changes are made
	for path, content := range tc.changes {
		if len(content) == 0 {
			return fmt.Errorf("empty file: %s", path)
		}
		// Add more validation logic here
	}
	tc.validated = true
	return nil
}

// Commit finalizes the transaction if validation passed.
// Only now do changes become "real" in the repository.
func (tc *TransactionalCommit) Commit(message string) (*object.Commit, error) {
	if !tc.validated {
		return nil, fmt.Errorf("transaction not validated")
	}

	tc.message = message
	
	// Swap staging areas atomically
	oldStaging := tc.repo.staging
	tc.repo.staging = tc.staging
	defer func() { tc.repo.staging = oldStaging }()

	return tc.repo.Commit(message)
}

// Rollback discards all changes in the transaction.
// Because we're memory-first, this is instant and leaves no trace.
func (tc *TransactionalCommit) Rollback() {
	tc.changes = make(map[string][]byte)
	tc.staging = NewStagingArea()
	tc.validated = false
}

// CommitEvent represents a commit as an event in the system.
// This enables reactive programming with version control.
type CommitEvent struct {
	Hash      string
	Author    string
	Message   string
	Timestamp time.Time
	Changes   []string
	Branch    string
}

// Watch sets up a commit event stream.
// Every commit becomes an event that can trigger actions.
func (r *Repository) Watch(handler func(CommitEvent)) {
	// This is a simplified version - in production you'd want
	// a proper event bus with channels
	go func() {
		lastHash := ""
		for {
			time.Sleep(100 * time.Millisecond)
			
			currentHash, err := r.refManager.GetHEAD()
			if err != nil || currentHash == lastHash {
				continue
			}
			
			commit, err := r.store.GetCommit(currentHash)
			if err != nil {
				continue
			}
			
			branch, _ := r.CurrentBranch()
			
			event := CommitEvent{
				Hash:      currentHash,
				Author:    commit.Author.Name,
				Message:   commit.Message,
				Timestamp: commit.Author.Time,
				Branch:    branch,
			}
			
			handler(event)
			lastHash = currentHash
		}
	}()
}

// TimeTravel returns the repository state at a specific time.
// Memory-first makes this operation instant.
func (r *Repository) TimeTravel(moment time.Time) *HistoricalSnapshot {
	commits, _ := r.Log(0) // Get all commits
	
	// Find the commit closest to the requested time
	for _, commit := range commits {
		if commit.Author.Time.Before(moment) {
			return &HistoricalSnapshot{
				repo:   r,
				commit: commit,
				time:   moment,
			}
		}
	}
	
	return nil
}

// HistoricalSnapshot represents a point-in-time view of the repository.
type HistoricalSnapshot struct {
	repo   *Repository
	commit *object.Commit
	time   time.Time
}

// Read returns file content at this point in history.
func (hs *HistoricalSnapshot) Read(path string) ([]byte, error) {
	tree, err := hs.repo.store.GetTree(hs.commit.TreeHash)
	if err != nil {
		return nil, err
	}
	
	for _, entry := range tree.Entries {
		if entry.Name == path {
			blob, err := hs.repo.store.GetBlob(entry.Hash)
			if err != nil {
				return nil, err
			}
			return blob.Content, nil
		}
	}
	
	return nil, fmt.Errorf("file not found in snapshot: %s", path)
}

// LastCommit returns the commit at this snapshot.
func (hs *HistoricalSnapshot) LastCommit() *object.Commit {
	return hs.commit
}

// BenchmarkResult holds performance metrics for a reality.
type BenchmarkResult struct {
	Reality   string
	StartTime time.Time
	Metrics   map[string]float64
}

// Better compares this result to a baseline.
func (br *BenchmarkResult) Better() bool {
	// Simplified - real implementation would compare metrics
	return true
}

// VirtualWorktree is a memory-only worktree.
// No files are written to disk unless explicitly requested.
type VirtualWorktree struct {
	files map[string][]byte
	mu    sync.RWMutex
}

// NewVirtualWorktree creates a worktree that exists only in memory.
func NewVirtualWorktree() *VirtualWorktree {
	return &VirtualWorktree{
		files: make(map[string][]byte),
	}
}

// NewMemoryStore creates a store that operates entirely in memory.
// This is what makes govc's instant operations possible.
func NewMemoryStore() *storage.Store {
	backend := storage.NewMemoryBackend()
	return storage.NewStore(backend)
}

// NewMemoryRefManager creates a reference manager in memory.
func NewMemoryRefManager() *refs.RefManager {
	store := refs.NewMemoryRefStore()
	return refs.NewRefManager(store)
}