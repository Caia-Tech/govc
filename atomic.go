package govc

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Caia-Tech/govc/pkg/object"
)

// AtomicTransactionState represents the state of an atomic transaction
type AtomicTransactionState int32

const (
	TxnPending AtomicTransactionState = iota
	TxnPreparing
	TxnPrepared  
	TxnCommitting
	TxnCommitted
	TxnAborting
	TxnAborted
	TxnConflicted
)

// String returns the string representation of the transaction state
func (s AtomicTransactionState) String() string {
	switch s {
	case TxnPending:
		return "Pending"
	case TxnPreparing:
		return "Preparing"
	case TxnPrepared:
		return "Prepared"
	case TxnCommitting:
		return "Committing"
	case TxnCommitted:
		return "Committed"
	case TxnAborting:
		return "Aborting"
	case TxnAborted:
		return "Aborted"
	case TxnConflicted:
		return "Conflicted"
	default:
		return "Unknown"
	}
}

// AtomicTransaction provides ACID guarantees for repository operations
// This is critical for high-concurrency scenarios where race conditions
// would otherwise cause data corruption or inconsistent state.
type AtomicTransaction struct {
	id          string
	repo        *Repository
	state       int32 // AtomicTransactionState (atomic access)
	startTime   time.Time
	prepareTime time.Time
	commitTime  time.Time
	
	// Transaction isolation
	readSnapshot   *RepositorySnapshot
	writeSet       map[string]*AtomicFileOp
	conflictSet    map[string]string // path -> conflicting transaction ID
	dependencies   []string          // transaction IDs this depends on
	
	// Atomic operations tracking
	operations     []*AtomicOperation
	compensations  []*AtomicOperation // for rollback
	
	// Concurrency control
	mu             sync.RWMutex
	preparedCh     chan struct{}
	completedCh    chan struct{}
	
	// Metadata
	message        string
	author         object.Author
	tags           map[string]string
	retryCount     int32
	maxRetries     int32
}

// AtomicFileOp represents an atomic file operation within a transaction
type AtomicFileOp struct {
	Path        string
	Type        FileOpType
	OldContent  []byte
	NewContent  []byte
	OldHash     string
	NewHash     string
	Timestamp   time.Time
	Checksum    string
}

// FileOpType represents the type of file operation
type FileOpType int

const (
	FileOpCreate FileOpType = iota
	FileOpUpdate
	FileOpDelete
	FileOpRename
	FileOpCopy
)

// String returns the string representation of the file operation type
func (t FileOpType) String() string {
	switch t {
	case FileOpCreate:
		return "Create"
	case FileOpUpdate:
		return "Update"
	case FileOpDelete:
		return "Delete"
	case FileOpRename:
		return "Rename"
	case FileOpCopy:
		return "Copy"
	default:
		return "Unknown"
	}
}

// AtomicOperation represents a single atomic operation that can be undone
type AtomicOperation struct {
	Type        string
	Description string
	Execute     func() error
	Compensate  func() error // Rollback operation
	Executed    bool
	Error       error
	Timestamp   time.Time
}

// RepositorySnapshot represents a consistent point-in-time view
type RepositorySnapshot struct {
	CommitHash   string
	Timestamp    time.Time
	FileStates   map[string]string // path -> hash
	BranchRefs   map[string]string // branch -> commit hash
	TagRefs      map[string]string // tag -> commit hash
}

// AtomicTransactionManager manages concurrent atomic transactions
type AtomicTransactionManager struct {
	activeTxns     map[string]*AtomicTransaction
	committedTxns  map[string]*AtomicTransaction
	conflictGraph  *ConflictGraph
	mu             sync.RWMutex
	nextTxnID      int64
	
	// Performance metrics
	totalTxns      int64
	committedCount int64
	abortedCount   int64
	conflictCount  int64
}

// ConflictGraph tracks dependencies and conflicts between transactions
type ConflictGraph struct {
	edges    map[string][]string // transaction ID -> list of conflicting transaction IDs
	deadlock *DeadlockDetector
	mu       sync.RWMutex
}

// DeadlockDetector detects circular dependencies in the conflict graph
type DeadlockDetector struct {
	cycles   [][]string
	visited  map[string]bool
	stack    []string
	mu       sync.Mutex
}

// NewAtomicTransactionManager creates a new transaction manager
func NewAtomicTransactionManager() *AtomicTransactionManager {
	return &AtomicTransactionManager{
		activeTxns:    make(map[string]*AtomicTransaction),
		committedTxns: make(map[string]*AtomicTransaction),
		conflictGraph: &ConflictGraph{
			edges: make(map[string][]string),
			deadlock: &DeadlockDetector{
				visited: make(map[string]bool),
				stack:   make([]string, 0),
			},
		},
	}
}

// BeginTransaction starts a new atomic transaction
func (tm *AtomicTransactionManager) BeginTransaction(repo *Repository) *AtomicTransaction {
	txnID := tm.generateTransactionID()
	
	// Create snapshot of current repository state
	snapshot := tm.createSnapshot(repo)
	
	txn := &AtomicTransaction{
		id:            txnID,
		repo:          repo,
		state:         int32(TxnPending),
		startTime:     time.Now(),
		readSnapshot:  snapshot,
		writeSet:      make(map[string]*AtomicFileOp),
		conflictSet:   make(map[string]string),
		dependencies:  make([]string, 0),
		operations:    make([]*AtomicOperation, 0),
		compensations: make([]*AtomicOperation, 0),
		preparedCh:    make(chan struct{}),
		completedCh:   make(chan struct{}),
		tags:          make(map[string]string),
		maxRetries:    3,
		author: object.Author{
			Name:  "System",
			Email: "system@govc",
			Time:  time.Now(),
		},
	}
	
	tm.mu.Lock()
	tm.activeTxns[txnID] = txn
	atomic.AddInt64(&tm.totalTxns, 1)
	tm.mu.Unlock()
	
	return txn
}

// AtomicFileCreate atomically creates a new file
func (txn *AtomicTransaction) AtomicFileCreate(path string, content []byte) error {
	if !txn.ensureState(TxnPending, TxnPreparing) {
		return fmt.Errorf("transaction %s not in valid state for file operations: %s", 
			txn.id, AtomicTransactionState(atomic.LoadInt32(&txn.state)))
	}
	
	txn.mu.Lock()
	defer txn.mu.Unlock()
	
	// Check for conflicts with other transactions
	if conflictID, exists := txn.conflictSet[path]; exists {
		return fmt.Errorf("path %s conflicts with transaction %s", path, conflictID)
	}
	
	// Check if file already exists in write set
	if _, exists := txn.writeSet[path]; exists {
		return fmt.Errorf("path %s already modified in this transaction", path)
	}
	
	// Store content using delta compression when beneficial
	hash, err := txn.repo.StoreBlobWithDelta(content)
	if err != nil {
		return fmt.Errorf("failed to store blob for %s: %w", path, err)
	}
	
	// Create file operation
	fileOp := &AtomicFileOp{
		Path:       path,
		Type:       FileOpCreate,
		NewContent: content,
		NewHash:    hash,
		Timestamp:  time.Now(),
		Checksum:   calculateChecksum(content),
	}
	
	txn.writeSet[path] = fileOp
	
	// Create atomic operation with compensation
	op := &AtomicOperation{
		Type:        "FileCreate",
		Description: fmt.Sprintf("Create file %s", path),
		Execute: func() error {
			txn.repo.staging.Add(path, hash)
			return nil
		},
		Compensate: func() error {
			txn.repo.staging.Remove(path)
			return nil
		},
		Timestamp: time.Now(),
	}
	
	txn.operations = append(txn.operations, op)
	
	return nil
}

// AtomicFileUpdate atomically updates an existing file
func (txn *AtomicTransaction) AtomicFileUpdate(path string, content []byte) error {
	if !txn.ensureState(TxnPending, TxnPreparing) {
		return fmt.Errorf("transaction %s not in valid state for file operations: %s", 
			txn.id, AtomicTransactionState(atomic.LoadInt32(&txn.state)))
	}
	
	txn.mu.Lock()
	defer txn.mu.Unlock()
	
	// Check for conflicts
	if conflictID, exists := txn.conflictSet[path]; exists {
		return fmt.Errorf("path %s conflicts with transaction %s", path, conflictID)
	}
	
	// Get current file state from snapshot
	oldHash, exists := txn.readSnapshot.FileStates[path]
	var oldContent []byte
	if exists {
		if blob, err := txn.repo.store.GetBlob(oldHash); err == nil {
			oldContent = blob.Content
		}
	}
	
	// Store new content using delta compression when beneficial
	newHash, err := txn.repo.StoreBlobWithDelta(content)
	if err != nil {
		return fmt.Errorf("failed to store blob for %s: %w", path, err)
	}
	
	// Create file operation
	fileOp := &AtomicFileOp{
		Path:       path,
		Type:       FileOpUpdate,
		OldContent: oldContent,
		NewContent: content,
		OldHash:    oldHash,
		NewHash:    newHash,
		Timestamp:  time.Now(),
		Checksum:   calculateChecksum(content),
	}
	
	txn.writeSet[path] = fileOp
	
	// Create atomic operation with compensation
	op := &AtomicOperation{
		Type:        "FileUpdate",
		Description: fmt.Sprintf("Update file %s", path),
		Execute: func() error {
			txn.repo.staging.Add(path, newHash)
			return nil
		},
		Compensate: func() error {
			if oldHash != "" {
				txn.repo.staging.Add(path, oldHash)
			} else {
				txn.repo.staging.Remove(path)
			}
			return nil
		},
		Timestamp: time.Now(),
	}
	
	txn.operations = append(txn.operations, op)
	
	return nil
}

// AtomicFileDelete atomically deletes a file
func (txn *AtomicTransaction) AtomicFileDelete(path string) error {
	if !txn.ensureState(TxnPending, TxnPreparing) {
		return fmt.Errorf("transaction %s not in valid state for file operations: %s", 
			txn.id, AtomicTransactionState(atomic.LoadInt32(&txn.state)))
	}
	
	txn.mu.Lock()
	defer txn.mu.Unlock()
	
	// Check for conflicts
	if conflictID, exists := txn.conflictSet[path]; exists {
		return fmt.Errorf("path %s conflicts with transaction %s", path, conflictID)
	}
	
	// Get current file state from snapshot or write set
	oldHash := ""
	var oldContent []byte
	
	// First check if file was created in this transaction
	if fileOp, inWriteSet := txn.writeSet[path]; inWriteSet {
		if fileOp.Type == FileOpCreate || fileOp.Type == FileOpUpdate {
			oldHash = fileOp.NewHash
			oldContent = fileOp.NewContent
		} else {
			return fmt.Errorf("file %s already being deleted in this transaction", path)
		}
	} else {
		// Check snapshot
		var exists bool
		oldHash, exists = txn.readSnapshot.FileStates[path]
		if !exists {
			return fmt.Errorf("file %s does not exist", path)
		}
		
		// Get content from store
		if blob, err := txn.repo.store.GetBlob(oldHash); err == nil {
			oldContent = blob.Content
		}
	}
	
	// Create file operation
	fileOp := &AtomicFileOp{
		Path:       path,
		Type:       FileOpDelete,
		OldContent: oldContent,
		OldHash:    oldHash,
		Timestamp:  time.Now(),
	}
	
	txn.writeSet[path] = fileOp
	
	// Create atomic operation with compensation
	op := &AtomicOperation{
		Type:        "FileDelete",
		Description: fmt.Sprintf("Delete file %s", path),
		Execute: func() error {
			txn.repo.staging.Remove(path)
			return nil
		},
		Compensate: func() error {
			txn.repo.staging.Add(path, oldHash)
			return nil
		},
		Timestamp: time.Now(),
	}
	
	txn.operations = append(txn.operations, op)
	
	return nil
}

// Prepare prepares the transaction for commit (2-phase commit protocol)
func (txn *AtomicTransaction) Prepare() error {
	if !txn.ensureState(TxnPending, TxnPreparing) {
		return fmt.Errorf("transaction %s not in valid state for prepare: %s", 
			txn.id, AtomicTransactionState(atomic.LoadInt32(&txn.state)))
	}
	
	atomic.StoreInt32(&txn.state, int32(TxnPreparing))
	txn.prepareTime = time.Now()
	
	// Detect conflicts with other active transactions
	if err := txn.detectConflicts(); err != nil {
		atomic.StoreInt32(&txn.state, int32(TxnConflicted))
		return err
	}
	
	// Validate all operations
	for _, op := range txn.operations {
		if err := op.Execute(); err != nil {
			// Rollback executed operations
			txn.rollback()
			atomic.StoreInt32(&txn.state, int32(TxnAborted))
			return fmt.Errorf("operation failed during prepare: %w", err)
		}
		op.Executed = true
	}
	
	atomic.StoreInt32(&txn.state, int32(TxnPrepared))
	close(txn.preparedCh)
	
	return nil
}

// Commit commits the transaction atomically
func (txn *AtomicTransaction) Commit(message string) (*object.Commit, error) {
	if !txn.ensureState(TxnPrepared, TxnCommitting) {
		return nil, fmt.Errorf("transaction %s not in valid state for commit: %s", 
			txn.id, AtomicTransactionState(atomic.LoadInt32(&txn.state)))
	}
	
	atomic.StoreInt32(&txn.state, int32(TxnCommitting))
	txn.message = message
	txn.author.Time = time.Now()
	
	// Perform the actual commit
	commit, err := txn.repo.Commit(message)
	if err != nil {
		// Rollback on commit failure
		txn.rollback()
		atomic.StoreInt32(&txn.state, int32(TxnAborted))
		return nil, fmt.Errorf("commit failed: %w", err)
	}
	
	txn.commitTime = time.Now()
	atomic.StoreInt32(&txn.state, int32(TxnCommitted))
	
	// Update statistics
	atomic.AddInt64(&txn.repo.txnManager.committedCount, 1)
	
	// Move transaction from active to committed
	txn.repo.txnManager.mu.Lock()
	delete(txn.repo.txnManager.activeTxns, txn.id)
	txn.repo.txnManager.committedTxns[txn.id] = txn
	txn.repo.txnManager.mu.Unlock()
	
	// Publish commit event
	if txn.repo.eventBus != nil {
		files := make([]string, 0, len(txn.writeSet))
		for path := range txn.writeSet {
			files = append(files, path)
		}
		txn.repo.publishCommitEvent(commit.Hash(), files)
	}
	
	close(txn.completedCh)
	return commit, nil
}

// Abort aborts the transaction and rolls back all operations
func (txn *AtomicTransaction) Abort() error {
	currentState := AtomicTransactionState(atomic.LoadInt32(&txn.state))
	if currentState == TxnCommitted {
		return fmt.Errorf("cannot abort committed transaction %s", txn.id)
	}
	
	atomic.StoreInt32(&txn.state, int32(TxnAborting))
	
	if err := txn.rollback(); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}
	
	atomic.StoreInt32(&txn.state, int32(TxnAborted))
	
	// Update statistics
	atomic.AddInt64(&txn.repo.txnManager.abortedCount, 1)
	
	// Remove transaction from active list
	txn.repo.txnManager.mu.Lock()
	delete(txn.repo.txnManager.activeTxns, txn.id)
	txn.repo.txnManager.mu.Unlock()
	
	close(txn.completedCh)
	
	return nil
}

// rollback executes all compensation operations
func (txn *AtomicTransaction) rollback() error {
	var lastErr error
	
	// Execute compensations in reverse order
	for i := len(txn.operations) - 1; i >= 0; i-- {
		op := txn.operations[i]
		if op.Executed && op.Compensate != nil {
			if err := op.Compensate(); err != nil {
				lastErr = err
			}
		}
	}
	
	return lastErr
}

// detectConflicts detects conflicts with other active transactions
func (txn *AtomicTransaction) detectConflicts() error {
	txn.repo.txnManager.mu.RLock()
	defer txn.repo.txnManager.mu.RUnlock()
	
	for otherID, other := range txn.repo.txnManager.activeTxns {
		if otherID == txn.id {
			continue
		}
		
		// Only check for conflicts with transactions that are actively preparing or prepared
		otherState := AtomicTransactionState(atomic.LoadInt32(&other.state))
		if otherState != TxnPreparing && otherState != TxnPrepared {
			continue
		}
		
		other.mu.RLock()
		// Check for write-write conflicts
		for path := range txn.writeSet {
			if _, exists := other.writeSet[path]; exists {
				txn.conflictSet[path] = otherID
				other.mu.RUnlock()
				return fmt.Errorf("write-write conflict on path %s with transaction %s", path, otherID)
			}
		}
		other.mu.RUnlock()
	}
	
	return nil
}

// ensureState ensures the transaction is in one of the expected states
func (txn *AtomicTransaction) ensureState(expected ...AtomicTransactionState) bool {
	current := AtomicTransactionState(atomic.LoadInt32(&txn.state))
	for _, state := range expected {
		if current == state {
			return true
		}
	}
	return false
}

// generateTransactionID generates a unique transaction ID
func (tm *AtomicTransactionManager) generateTransactionID() string {
	id := atomic.AddInt64(&tm.nextTxnID, 1)
	return fmt.Sprintf("txn_%d_%d", time.Now().UnixNano(), id)
}

// createSnapshot creates a consistent snapshot of the repository
func (tm *AtomicTransactionManager) createSnapshot(repo *Repository) *RepositorySnapshot {
	snapshot := &RepositorySnapshot{
		Timestamp:  time.Now(),
		FileStates: make(map[string]string),
		BranchRefs: make(map[string]string),
		TagRefs:    make(map[string]string),
	}
	
	// Get current commit hash
	if hash, err := repo.refManager.GetHEAD(); err == nil {
		snapshot.CommitHash = hash
		
		// Get file states from current commit
		if hash != "" { // Only if we have commits
			if commit, err := repo.store.GetCommit(hash); err == nil {
				if tree, err := repo.store.GetTree(commit.TreeHash); err == nil {
					tm.collectFileStates(tree, "", snapshot.FileStates)
				}
			}
		}
	}
	
	// Also include staging area files for current state
	if repo.staging != nil {
		repo.staging.mu.RLock()
		for path, hash := range repo.staging.files {
			snapshot.FileStates[path] = hash
		}
		repo.staging.mu.RUnlock()
	}
	
	return snapshot
}

// collectFileStates recursively collects file states from a tree
func (tm *AtomicTransactionManager) collectFileStates(tree *object.Tree, prefix string, fileStates map[string]string) {
	for _, entry := range tree.Entries {
		fullPath := entry.Name
		if prefix != "" {
			fullPath = prefix + "/" + entry.Name
		}
		
		if entry.Mode == "040000" { // Directory
			if subTree, err := tm.getTreeFromAnyRepo(entry.Hash); err == nil {
				tm.collectFileStates(subTree, fullPath, fileStates)
			}
		} else { // File
			fileStates[fullPath] = entry.Hash
		}
	}
}

// getTreeFromAnyRepo is a helper to get trees (we'll use the first available repo's store)
func (tm *AtomicTransactionManager) getTreeFromAnyRepo(hash string) (*object.Tree, error) {
	// For now, we'll use a simple approach since we need access to the store
	// In a production system, this would be more sophisticated
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	for _, txn := range tm.activeTxns {
		if tree, err := txn.repo.store.GetTree(hash); err == nil {
			return tree, nil
		}
	}
	
	return nil, fmt.Errorf("tree not found: %s", hash)
}

// GetStats returns transaction manager statistics
func (tm *AtomicTransactionManager) GetStats() TransactionStats {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	return TransactionStats{
		TotalTransactions:     atomic.LoadInt64(&tm.totalTxns),
		ActiveTransactions:    int64(len(tm.activeTxns)),
		CommittedTransactions: atomic.LoadInt64(&tm.committedCount),
		AbortedTransactions:   atomic.LoadInt64(&tm.abortedCount),
		ConflictCount:         atomic.LoadInt64(&tm.conflictCount),
	}
}

// TransactionStats contains transaction manager statistics
type TransactionStats struct {
	TotalTransactions     int64 `json:"total_transactions"`
	ActiveTransactions    int64 `json:"active_transactions"`
	CommittedTransactions int64 `json:"committed_transactions"`
	AbortedTransactions   int64 `json:"aborted_transactions"`
	ConflictCount         int64 `json:"conflict_count"`
}

// calculateChecksum calculates a checksum for content integrity
func calculateChecksum(content []byte) string {
	// Simple checksum for now - could be enhanced with proper hashing
	sum := 0
	for _, b := range content {
		sum += int(b)
	}
	return fmt.Sprintf("checksum_%d", sum)
}

// GetTransactionID returns the transaction ID
func (txn *AtomicTransaction) GetTransactionID() string {
	return txn.id
}

// GetState returns the current transaction state
func (txn *AtomicTransaction) GetState() AtomicTransactionState {
	return AtomicTransactionState(atomic.LoadInt32(&txn.state))
}

// IsActive returns true if the transaction is active (not committed or aborted)
func (txn *AtomicTransaction) IsActive() bool {
	state := AtomicTransactionState(atomic.LoadInt32(&txn.state))
	return state == TxnPending || state == TxnPreparing || state == TxnPrepared || state == TxnCommitting
}

// WaitForCompletion waits for the transaction to complete (commit or abort)
func (txn *AtomicTransaction) WaitForCompletion() AtomicTransactionState {
	<-txn.completedCh
	return AtomicTransactionState(atomic.LoadInt32(&txn.state))
}

// GetWriteSet returns a copy of the transaction's write set
func (txn *AtomicTransaction) GetWriteSet() map[string]*AtomicFileOp {
	txn.mu.RLock()
	defer txn.mu.RUnlock()
	
	writeSet := make(map[string]*AtomicFileOp)
	for path, op := range txn.writeSet {
		writeSet[path] = op
	}
	return writeSet
}