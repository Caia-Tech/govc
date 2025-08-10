package govc

import (
	"fmt"

	"github.com/caiatech/govc/pkg/object"
)

// BeginTransaction starts a new atomic transaction
func (r *Repository) BeginTransaction() *AtomicTransaction {
	return r.txnManager.BeginTransaction(r)
}

// AtomicCommit performs an atomic commit with the given file operations
// This is a convenience method that creates a transaction, performs operations, and commits
func (r *Repository) AtomicCommit(message string, operations func(*AtomicTransaction) error) (*object.Commit, error) {
	// Start a new transaction
	txn := r.BeginTransaction()
	defer func() {
		if txn.IsActive() {
			txn.Abort()
		}
	}()
	
	// Perform operations
	if err := operations(txn); err != nil {
		return nil, fmt.Errorf("transaction operations failed: %w", err)
	}
	
	// Prepare the transaction
	if err := txn.Prepare(); err != nil {
		return nil, fmt.Errorf("transaction prepare failed: %w", err)
	}
	
	// Commit the transaction
	return txn.Commit(message)
}

// AtomicFileOperations performs multiple file operations atomically
func (r *Repository) AtomicFileOperations(message string, ops []AtomicFileOperation) (*object.Commit, error) {
	return r.AtomicCommit(message, func(txn *AtomicTransaction) error {
		for _, op := range ops {
			var err error
			switch op.Type {
			case FileOpCreate:
				err = txn.AtomicFileCreate(op.Path, op.Content)
			case FileOpUpdate:
				err = txn.AtomicFileUpdate(op.Path, op.Content)
			case FileOpDelete:
				err = txn.AtomicFileDelete(op.Path)
			default:
				return fmt.Errorf("unsupported operation type: %s", op.Type)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// AtomicFileOperation represents a file operation to be performed atomically
type AtomicFileOperation struct {
	Type    FileOpType
	Path    string
	Content []byte
}

// GetTransactionStats returns statistics about the transaction manager
func (r *Repository) GetTransactionStats() TransactionStats {
	return r.txnManager.GetStats()
}

// GetActiveTransactions returns a list of currently active transaction IDs
func (r *Repository) GetActiveTransactions() []string {
	r.txnManager.mu.RLock()
	defer r.txnManager.mu.RUnlock()
	
	txnIDs := make([]string, 0, len(r.txnManager.activeTxns))
	for id := range r.txnManager.activeTxns {
		txnIDs = append(txnIDs, id)
	}
	return txnIDs
}

// WaitForTransaction waits for a specific transaction to complete
func (r *Repository) WaitForTransaction(txnID string) (AtomicTransactionState, error) {
	r.txnManager.mu.RLock()
	txn, exists := r.txnManager.activeTxns[txnID]
	r.txnManager.mu.RUnlock()
	
	if !exists {
		// Check if it's in committed transactions
		r.txnManager.mu.RLock()
		txn, exists = r.txnManager.committedTxns[txnID]
		r.txnManager.mu.RUnlock()
		
		if exists {
			return TxnCommitted, nil
		}
		
		return TxnAborted, fmt.Errorf("transaction %s not found", txnID)
	}
	
	return txn.WaitForCompletion(), nil
}

// AbortTransaction aborts a specific transaction
func (r *Repository) AbortTransaction(txnID string) error {
	r.txnManager.mu.RLock()
	txn, exists := r.txnManager.activeTxns[txnID]
	r.txnManager.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("transaction %s not found or not active", txnID)
	}
	
	return txn.Abort()
}

// BatchAtomicCommit performs multiple atomic commits in sequence
// This is useful for applying a series of related changes that each need to be atomic
func (r *Repository) BatchAtomicCommit(operations []BatchOperation) ([]*object.Commit, error) {
	commits := make([]*object.Commit, 0, len(operations))
	
	for i, op := range operations {
		commit, err := r.AtomicCommit(op.Message, op.Operations)
		if err != nil {
			return commits, fmt.Errorf("batch operation %d failed: %w", i, err)
		}
		commits = append(commits, commit)
	}
	
	return commits, nil
}

// BatchOperation represents a batch of atomic operations with a commit message
type BatchOperation struct {
	Message    string
	Operations func(*AtomicTransaction) error
}

// AtomicCreateFile is a convenience method for atomically creating a single file
func (r *Repository) AtomicCreateFile(path string, content []byte, message string) (*object.Commit, error) {
	return r.AtomicCommit(message, func(txn *AtomicTransaction) error {
		return txn.AtomicFileCreate(path, content)
	})
}

// AtomicUpdateFile is a convenience method for atomically updating a single file
func (r *Repository) AtomicUpdateFile(path string, content []byte, message string) (*object.Commit, error) {
	return r.AtomicCommit(message, func(txn *AtomicTransaction) error {
		return txn.AtomicFileUpdate(path, content)
	})
}

// AtomicDeleteFile is a convenience method for atomically deleting a single file
func (r *Repository) AtomicDeleteFile(path string, message string) (*object.Commit, error) {
	return r.AtomicCommit(message, func(txn *AtomicTransaction) error {
		return txn.AtomicFileDelete(path)
	})
}

// AtomicMultiFileUpdate atomically updates multiple files in a single commit
func (r *Repository) AtomicMultiFileUpdate(updates map[string][]byte, message string) (*object.Commit, error) {
	return r.AtomicCommit(message, func(txn *AtomicTransaction) error {
		for path, content := range updates {
			if err := txn.AtomicFileUpdate(path, content); err != nil {
				return err
			}
		}
		return nil
	})
}

// IsTransactionActive checks if a transaction is currently active
func (r *Repository) IsTransactionActive(txnID string) bool {
	r.txnManager.mu.RLock()
	defer r.txnManager.mu.RUnlock()
	
	txn, exists := r.txnManager.activeTxns[txnID]
	return exists && txn.IsActive()
}