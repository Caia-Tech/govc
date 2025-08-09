package govc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caiatech/govc/pkg/object"
)

// ConcurrentSafeRepository wraps Repository with thread-safe operations
type ConcurrentSafeRepository struct {
	*Repository
	globalMutex    sync.RWMutex        // Global repo lock
	transactionMux sync.Mutex          // Transaction serialization
	storeMux       sync.RWMutex        // Store operations lock
	refMux         sync.RWMutex        // Reference operations lock
	branchMux      sync.RWMutex        // Branch operations lock
	initialized    int32               // Atomic flag for initialization
	corrupted      int32               // Atomic flag for corruption state
	activeOps      int64               // Active operation counter
	retryPolicy    *RetryPolicy        // Retry configuration
}

// RetryPolicy defines retry behavior for transient failures
type RetryPolicy struct {
	MaxAttempts   int
	InitialDelay  time.Duration
	BackoffFactor float64
	MaxDelay      time.Duration
}

// DefaultRetryPolicy provides sensible defaults for retry behavior
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:   3,
		InitialDelay:  10 * time.Millisecond,
		BackoffFactor: 2.0,
		MaxDelay:      1 * time.Second,
	}
}

// NewConcurrentSafeRepository creates a thread-safe wrapper around Repository
func NewConcurrentSafeRepository(repo *Repository) *ConcurrentSafeRepository {
	if repo == nil {
		repo = NewRepository()
	}
	
	return &ConcurrentSafeRepository{
		Repository:  repo,
		retryPolicy: DefaultRetryPolicy(),
	}
}

// Initialize ensures the repository is properly initialized with corruption checks
func (csr *ConcurrentSafeRepository) Initialize(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&csr.initialized, 0, 1) {
		return nil // Already initialized
	}

	csr.globalMutex.Lock()
	defer csr.globalMutex.Unlock()

	return csr.initializeUnlocked()
}

// initializeUnlocked performs initialization without locking (must be called while holding globalMutex)
func (csr *ConcurrentSafeRepository) initializeUnlocked() error {
	// Validate repository state
	if err := csr.validateRepositoryState(); err != nil {
		atomic.StoreInt32(&csr.corrupted, 1)
		return fmt.Errorf("repository validation failed: %w", err)
	}

	// Initialize components with error recovery
	if err := csr.initializeComponents(); err != nil {
		atomic.StoreInt32(&csr.corrupted, 1)
		return fmt.Errorf("component initialization failed: %w", err)
	}

	return nil
}

// validateRepositoryState checks for corruption and repairs if possible
func (csr *ConcurrentSafeRepository) validateRepositoryState() error {
	// Check if store is accessible
	if csr.Repository.store == nil {
		return fmt.Errorf("store is nil")
	}

	// Check if ref manager is accessible
	if csr.Repository.refManager == nil {
		return fmt.Errorf("refManager is nil")
	}

	// Validate HEAD reference
	if _, err := csr.Repository.refManager.GetHEAD(); err != nil {
		// Try to repair by creating default main branch
		if err := csr.Repository.refManager.CreateBranch("main", ""); err != nil {
			return fmt.Errorf("failed to create default main branch: %w", err)
		}
		if err := csr.Repository.refManager.SetHEADToBranch("main"); err != nil {
			return fmt.Errorf("failed to set HEAD to main: %w", err)
		}
	}

	return nil
}

// initializeComponents safely initializes repository components
func (csr *ConcurrentSafeRepository) initializeComponents() error {
	// Initialize staging area if needed
	if csr.Repository.staging == nil {
		csr.Repository.staging = NewStagingArea()
	}

	// Initialize worktree if needed
	if csr.Repository.worktree == nil {
		csr.Repository.worktree = &Worktree{
			path:  ":memory:",
			files: make(map[string][]byte),
		}
	}

	// Initialize config if needed
	if csr.Repository.config == nil {
		csr.Repository.config = make(map[string]string)
	}

	return nil
}

// SafeTransaction creates a thread-safe transactional commit
func (csr *ConcurrentSafeRepository) SafeTransaction() *ConcurrentSafeTransactionalCommit {
	return &ConcurrentSafeTransactionalCommit{
		parent:      csr,
		staging:     NewStagingArea(),
		changes:     make(map[string][]byte),
		author:      object.Author{Name: "System", Email: "system@govc", Time: time.Now()},
		retryPolicy: csr.retryPolicy,
	}
}

// ConcurrentSafeTransactionalCommit provides thread-safe transactional operations
type ConcurrentSafeTransactionalCommit struct {
	parent      *ConcurrentSafeRepository
	staging     *StagingArea
	message     string
	author      object.Author
	changes     map[string][]byte
	validated   bool
	committed   bool
	mu          sync.Mutex
	retryPolicy *RetryPolicy
}

// Add safely stages a file in the transaction with proper error handling
func (cstc *ConcurrentSafeTransactionalCommit) Add(path string, content []byte) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if content == nil {
		content = []byte{} // Allow empty files but not nil content
	}

	cstc.mu.Lock()
	defer cstc.mu.Unlock()

	if cstc.committed {
		return fmt.Errorf("cannot add to committed transaction")
	}

	// Check for repository corruption
	if atomic.LoadInt32(&cstc.parent.corrupted) == 1 {
		return fmt.Errorf("repository is corrupted")
	}

	// Increment active operations counter
	atomic.AddInt64(&cstc.parent.activeOps, 1)
	defer atomic.AddInt64(&cstc.parent.activeOps, -1)

	// Use retry logic for transient failures
	return cstc.withRetry(func() error {
		// Safely acquire store lock
		cstc.parent.storeMux.Lock()
		defer cstc.parent.storeMux.Unlock()

		// Validate store before use
		if cstc.parent.Repository.store == nil {
			return fmt.Errorf("store is nil")
		}

		// Store blob with error handling
		hash, err := cstc.parent.Repository.store.StoreBlob(content)
		if err != nil {
			return fmt.Errorf("failed to store blob for %s: %w", path, err)
		}

		// Update staging and changes atomically
		cstc.changes[path] = content
		if cstc.staging == nil {
			cstc.staging = NewStagingArea()
		}
		cstc.staging.Add(path, hash)

		return nil
	})
}

// withRetry executes an operation with retry logic for transient failures
func (cstc *ConcurrentSafeTransactionalCommit) withRetry(operation func() error) error {
	var lastErr error
	delay := cstc.retryPolicy.InitialDelay

	for attempt := 0; attempt < cstc.retryPolicy.MaxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(delay)
			delay = time.Duration(float64(delay) * cstc.retryPolicy.BackoffFactor)
			if delay > cstc.retryPolicy.MaxDelay {
				delay = cstc.retryPolicy.MaxDelay
			}
		}

		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !cstc.isRetryableError(err) {
			break
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", cstc.retryPolicy.MaxAttempts, lastErr)
}

// isRetryableError determines if an error should trigger a retry
func (cstc *ConcurrentSafeTransactionalCommit) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Common transient errors that should be retried
	retryableErrors := []string{
		"invalid argument",
		"resource temporarily unavailable",
		"connection refused",
		"timeout",
		"temporary failure",
	}

	for _, retryable := range retryableErrors {
		if containsIgnoreCase(errStr, retryable) {
			return true
		}
	}

	return false
}

// Validate performs comprehensive validation with atomic state checking
func (cstc *ConcurrentSafeTransactionalCommit) Validate() error {
	cstc.mu.Lock()
	defer cstc.mu.Unlock()

	if cstc.committed {
		return fmt.Errorf("transaction already committed")
	}

	// Check repository state
	if atomic.LoadInt32(&cstc.parent.corrupted) == 1 {
		return fmt.Errorf("repository is corrupted")
	}

	// Validate all changes
	for path, content := range cstc.changes {
		if len(content) == 0 {
			// Allow empty files but warn
			continue
		}
		
		// Add custom validation logic here
		if err := cstc.validateFileContent(path, content); err != nil {
			return fmt.Errorf("validation failed for %s: %w", path, err)
		}
	}

	cstc.validated = true
	return nil
}

// validateFileContent performs content-specific validation
func (cstc *ConcurrentSafeTransactionalCommit) validateFileContent(path string, content []byte) error {
	// Basic validation - can be extended based on file type
	if len(content) > 100*1024*1024 { // 100MB limit
		return fmt.Errorf("file too large: %d bytes", len(content))
	}

	// Add more validation rules as needed
	return nil
}

// Commit finalizes the transaction with full synchronization
func (cstc *ConcurrentSafeTransactionalCommit) Commit(message string) (*object.Commit, error) {
	cstc.mu.Lock()
	defer cstc.mu.Unlock()

	if cstc.committed {
		return nil, fmt.Errorf("transaction already committed")
	}

	if !cstc.validated {
		return nil, fmt.Errorf("transaction not validated")
	}

	// Check repository state
	if atomic.LoadInt32(&cstc.parent.corrupted) == 1 {
		return nil, fmt.Errorf("repository is corrupted")
	}

	// Use global transaction lock to serialize commits
	cstc.parent.transactionMux.Lock()
	defer cstc.parent.transactionMux.Unlock()

	// Increment active operations
	atomic.AddInt64(&cstc.parent.activeOps, 1)
	defer atomic.AddInt64(&cstc.parent.activeOps, -1)

	cstc.message = message
	cstc.author.Time = time.Now()

	// Perform atomic staging area swap
	var finalCommit *object.Commit
	err := cstc.withRetry(func() error {
		cstc.parent.globalMutex.Lock()
		defer cstc.parent.globalMutex.Unlock()

		// Validate repository components before commit
		if cstc.parent.Repository.store == nil {
			return fmt.Errorf("store is nil")
		}
		if cstc.parent.Repository.refManager == nil {
			return fmt.Errorf("refManager is nil")
		}

		// Swap staging areas atomically
		oldStaging := cstc.parent.Repository.staging
		cstc.parent.Repository.staging = cstc.staging
		
		defer func() {
			// Restore old staging on error
			if !cstc.committed {
				cstc.parent.Repository.staging = oldStaging
			}
		}()

		// Perform the actual commit
		commit, commitErr := cstc.parent.Repository.Commit(message)
		if commitErr != nil {
			return fmt.Errorf("commit failed: %w", commitErr)
		}

		finalCommit = commit
		cstc.committed = true
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	return finalCommit, nil
}

// Rollback discards all changes safely
func (cstc *ConcurrentSafeTransactionalCommit) Rollback() error {
	cstc.mu.Lock()
	defer cstc.mu.Unlock()

	if cstc.committed {
		return fmt.Errorf("cannot rollback committed transaction")
	}

	// Clear all state
	cstc.changes = make(map[string][]byte)
	cstc.staging = NewStagingArea()
	cstc.validated = false

	return nil
}

// SafeCreateBranch creates a branch with proper synchronization
func (csr *ConcurrentSafeRepository) SafeCreateBranch(name, startPoint string) error {
	if name == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	// Check repository state
	if atomic.LoadInt32(&csr.corrupted) == 1 {
		return fmt.Errorf("repository is corrupted")
	}

	csr.branchMux.Lock()
	defer csr.branchMux.Unlock()

	atomic.AddInt64(&csr.activeOps, 1)
	defer atomic.AddInt64(&csr.activeOps, -1)

	return csr.withRetry(func() error {
		if csr.Repository.refManager == nil {
			return fmt.Errorf("refManager is nil")
		}

		return csr.Repository.refManager.CreateBranch(name, startPoint)
	})
}

// SafeCheckout performs a safe branch checkout with validation
func (csr *ConcurrentSafeRepository) SafeCheckout(branch string) error {
	if branch == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	// Check repository state
	if atomic.LoadInt32(&csr.corrupted) == 1 {
		return fmt.Errorf("repository is corrupted")
	}

	csr.branchMux.Lock()
	defer csr.branchMux.Unlock()

	atomic.AddInt64(&csr.activeOps, 1)
	defer atomic.AddInt64(&csr.activeOps, -1)

	return csr.withRetry(func() error {
		if csr.Repository.refManager == nil {
			return fmt.Errorf("refManager is nil")
		}

		return csr.Repository.Checkout(branch)
	})
}

// WaitForQuiescence waits for all active operations to complete
func (csr *ConcurrentSafeRepository) WaitForQuiescence(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&csr.activeOps) == 0 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	
	return fmt.Errorf("timeout waiting for operations to complete")
}

// IsCorrupted returns true if the repository is in a corrupted state
func (csr *ConcurrentSafeRepository) IsCorrupted() bool {
	return atomic.LoadInt32(&csr.corrupted) == 1
}

// RecoverFromCorruption attempts to recover from corruption
func (csr *ConcurrentSafeRepository) RecoverFromCorruption() error {
	csr.globalMutex.Lock()
	defer csr.globalMutex.Unlock()

	// Wait for active operations to complete
	if err := csr.WaitForQuiescence(5 * time.Second); err != nil {
		return fmt.Errorf("cannot recover while operations are active: %w", err)
	}

	// Reset corruption flag
	atomic.StoreInt32(&csr.corrupted, 0)

	// Re-initialize repository (we're already holding the lock)
	atomic.StoreInt32(&csr.initialized, 0)
	return csr.initializeUnlocked()
}

// withRetry provides retry logic for repository operations
func (csr *ConcurrentSafeRepository) withRetry(operation func() error) error {
	return (&ConcurrentSafeTransactionalCommit{
		parent:      csr,
		retryPolicy: csr.retryPolicy,
	}).withRetry(operation)
}

// Helper function for case-insensitive string containment check
func containsIgnoreCase(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}

// RepositoryManager manages multiple concurrent-safe repositories
type RepositoryManager struct {
	repositories map[string]*ConcurrentSafeRepository
	mu           sync.RWMutex
}

// NewRepositoryManager creates a new repository manager
func NewRepositoryManager() *RepositoryManager {
	return &RepositoryManager{
		repositories: make(map[string]*ConcurrentSafeRepository),
	}
}

// GetOrCreateRepository safely gets or creates a repository
func (rm *RepositoryManager) GetOrCreateRepository(name string) (*ConcurrentSafeRepository, error) {
	if name == "" {
		return nil, fmt.Errorf("repository name cannot be empty")
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	if repo, exists := rm.repositories[name]; exists {
		return repo, nil
	}

	// Create new repository
	baseRepo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(baseRepo)
	
	// Initialize the repository
	if err := safeRepo.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize repository %s: %w", name, err)
	}

	rm.repositories[name] = safeRepo
	return safeRepo, nil
}

// RemoveRepository safely removes a repository
func (rm *RepositoryManager) RemoveRepository(name string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if repo, exists := rm.repositories[name]; exists {
		// Wait for operations to complete
		if err := repo.WaitForQuiescence(5 * time.Second); err != nil {
			return fmt.Errorf("cannot remove repository while operations are active: %w", err)
		}
		delete(rm.repositories, name)
	}

	return nil
}