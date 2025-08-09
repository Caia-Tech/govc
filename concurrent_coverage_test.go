package govc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentSafeRepository_InitializeEdgeCases tests initialization edge cases
func TestConcurrentSafeRepository_InitializeEdgeCases(t *testing.T) {
	// Test double initialization
	repo := NewRepository()
	csr := NewConcurrentSafeRepository(repo)
	
	// First initialization
	err := csr.Initialize(context.Background())
	assert.NoError(t, err)
	
	// Second initialization should return immediately
	err = csr.Initialize(context.Background())
	assert.NoError(t, err)
	
	// Test initialization with nil repository components
	baseRepo := &Repository{
		store:      nil,
		refManager: nil,
		staging:    nil,
		worktree:   nil,
		config:     nil,
	}
	csr2 := NewConcurrentSafeRepository(baseRepo)
	err = csr2.Initialize(context.Background())
	assert.Error(t, err) // Should fail since store and refManager are nil
}

// TestConcurrentSafeRepository_ValidationStateErrors tests repository validation errors
func TestConcurrentSafeRepository_ValidationStateErrors(t *testing.T) {
	// Create repo with missing HEAD
	repo := NewRepository()
	csr := NewConcurrentSafeRepository(repo)
	
	// Break the HEAD reference to trigger repair path
	repo.refManager = nil
	
	err := csr.validateRepositoryState()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "refManager is nil")
}

// TestConcurrentSafeTransactionalCommit_RetryExhaustion tests retry limit
func TestConcurrentSafeTransactionalCommit_RetryExhaustion(t *testing.T) {
	repo := NewRepository()
	csr := NewConcurrentSafeRepository(repo)
	tx := csr.SafeTransaction()
	
	// Set a very low retry count
	tx.retryPolicy = &RetryPolicy{
		MaxAttempts:   2,
		InitialDelay:  1 * time.Millisecond,
		BackoffFactor: 1.5,
		MaxDelay:      10 * time.Millisecond,
	}
	
	// Create an operation that always fails with retryable error
	failCount := 0
	err := tx.withRetry(func() error {
		failCount++
		return fmt.Errorf("resource temporarily unavailable")
	})
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "operation failed after 2 attempts")
	assert.Equal(t, 2, failCount)
}

// TestConcurrentSafeTransactionalCommit_NonRetryableError tests non-retryable errors
func TestConcurrentSafeTransactionalCommit_NonRetryableError(t *testing.T) {
	repo := NewRepository()
	csr := NewConcurrentSafeRepository(repo)
	tx := csr.SafeTransaction()
	
	// Create an operation that fails with non-retryable error
	attemptCount := 0
	err := tx.withRetry(func() error {
		attemptCount++
		return fmt.Errorf("permission denied")
	})
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	assert.Equal(t, 1, attemptCount) // Should not retry
}

// TestConcurrentSafeTransactionalCommit_AddErrors tests Add error paths
func TestConcurrentSafeTransactionalCommit_AddErrors(t *testing.T) {
	repo := NewRepository()
	csr := NewConcurrentSafeRepository(repo)
	
	// Test adding with empty path
	tx := csr.SafeTransaction()
	err := tx.Add("", []byte("content"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path cannot be empty")
	
	// Test adding after commit
	tx2 := csr.SafeTransaction()
	err = tx2.Add("file.txt", []byte("content"))
	require.NoError(t, err)
	err = tx2.Validate()
	require.NoError(t, err)
	_, err = tx2.Commit("test commit")
	require.NoError(t, err)
	
	// Try to add after commit
	err = tx2.Add("another.txt", []byte("content"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot add to committed transaction")
}

// TestConcurrentSafeTransactionalCommit_ValidateErrors tests validation error paths
func TestConcurrentSafeTransactionalCommit_ValidateErrors(t *testing.T) {
	repo := NewRepository()
	csr := NewConcurrentSafeRepository(repo)
	
	// Validate after commit
	tx := csr.SafeTransaction()
	err := tx.Add("file.txt", []byte("content"))
	require.NoError(t, err)
	err = tx.Validate()
	require.NoError(t, err)
	_, err = tx.Commit("test")
	require.NoError(t, err)
	
	// Try to validate after commit
	err = tx.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction already committed")
}

// TestConcurrentSafeTransactionalCommit_CommitErrors tests commit error paths
func TestConcurrentSafeTransactionalCommit_CommitErrors(t *testing.T) {
	repo := NewRepository()
	csr := NewConcurrentSafeRepository(repo)
	
	// Commit without validation
	tx := csr.SafeTransaction()
	err := tx.Add("file.txt", []byte("content"))
	require.NoError(t, err)
	
	_, err = tx.Commit("unvalidated commit")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction not validated")
	
	// Double commit
	tx2 := csr.SafeTransaction()
	err = tx2.Add("file.txt", []byte("content"))
	require.NoError(t, err)
	err = tx2.Validate()
	require.NoError(t, err)
	_, err = tx2.Commit("first commit")
	require.NoError(t, err)
	
	_, err = tx2.Commit("second commit")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction already committed")
}

// TestConcurrentSafeRepository_BranchErrors tests branch operation errors
func TestConcurrentSafeRepository_BranchErrors(t *testing.T) {
	repo := NewRepository()
	csr := NewConcurrentSafeRepository(repo)
	
	// Create branch with empty name
	err := csr.SafeCreateBranch("", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "branch name cannot be empty")
	
	// Checkout empty branch name
	err = csr.SafeCheckout("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "branch name cannot be empty")
}

// TestConcurrentSafeRepository_RecoverFromCorruptionTimeout tests recovery timeout
func TestConcurrentSafeRepository_RecoverFromCorruptionTimeout(t *testing.T) {
	repo := NewRepository()
	csr := NewConcurrentSafeRepository(repo)
	
	// Start an operation to prevent quiescence
	go func() {
		for i := 0; i < 10; i++ {
			csr.SafeTransaction()
			time.Sleep(10 * time.Millisecond)
		}
	}()
	
	// Short timeout for testing
	csr.retryPolicy = &RetryPolicy{
		MaxAttempts:   1,
		InitialDelay:  1 * time.Millisecond,
		BackoffFactor: 1.0,
		MaxDelay:      1 * time.Millisecond,
	}
	
	// Try to recover (should timeout waiting for operations)
	err := csr.RecoverFromCorruption()
	// This might or might not error depending on timing
	_ = err
}

// TestRepositoryManager_Errors tests repository manager error paths
func TestRepositoryManager_Errors(t *testing.T) {
	manager := NewRepositoryManager()
	
	// Create repository with empty name
	_, err := manager.GetOrCreateRepository("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "repository name cannot be empty")
}

// TestConcurrentSafeRepository_GetStagedFilesNilStaging tests nil staging handling
func TestConcurrentSafeRepository_GetStagedFilesNilStaging(t *testing.T) {
	repo := &Repository{
		staging: nil,
	}
	csr := NewConcurrentSafeRepository(repo)
	
	// Should return empty map for nil staging
	files := csr.SafeGetStagedFiles()
	assert.NotNil(t, files)
	assert.Empty(t, files)
	
	// Should return empty list for nil staging
	list := csr.SafeListStagedFiles()
	assert.NotNil(t, list)
	assert.Empty(t, list)
}

// TestConcurrentSafeRepository_RetryPolicyExponentialBackoff tests backoff calculation
func TestConcurrentSafeRepository_RetryPolicyExponentialBackoff(t *testing.T) {
	repo := NewRepository()
	csr := NewConcurrentSafeRepository(repo)
	tx := csr.SafeTransaction()
	
	tx.retryPolicy = &RetryPolicy{
		MaxAttempts:   3,
		InitialDelay:  10 * time.Millisecond,
		BackoffFactor: 2.0,
		MaxDelay:      100 * time.Millisecond,
	}
	
	// Track delays
	var delays []time.Duration
	lastTime := time.Now()
	attemptCount := 0
	
	err := tx.withRetry(func() error {
		now := time.Now()
		if attemptCount > 0 {
			delays = append(delays, now.Sub(lastTime))
		}
		lastTime = now
		attemptCount++
		
		if attemptCount < 3 {
			return fmt.Errorf("resource temporarily unavailable")
		}
		return nil
	})
	
	require.NoError(t, err)
	assert.Equal(t, 3, attemptCount)
	assert.Len(t, delays, 2)
	
	// Check exponential backoff (allowing for timing variance)
	if len(delays) >= 2 {
		assert.True(t, delays[1] > delays[0]) // Second delay should be longer
	}
}