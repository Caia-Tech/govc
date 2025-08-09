package govc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrentSafeRepository_Basic(t *testing.T) {
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)

	// Test initialization
	err := safeRepo.Initialize(context.Background())
	assert.NoError(t, err)
	assert.False(t, safeRepo.IsCorrupted())
}

func TestConcurrentSafeRepository_NilRepoHandling(t *testing.T) {
	// Test with nil repository - should create new one
	safeRepo := NewConcurrentSafeRepository(nil)
	assert.NotNil(t, safeRepo)
	assert.NotNil(t, safeRepo.Repository)

	err := safeRepo.Initialize(context.Background())
	assert.NoError(t, err)
}

func TestConcurrentSafeTransactionalCommit_BasicOperations(t *testing.T) {
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(context.Background()))

	// Create safe transaction
	tx := safeRepo.SafeTransaction()
	assert.NotNil(t, tx)

	// Test Add with valid data
	err := tx.Add("test/file.txt", []byte("Hello World"))
	assert.NoError(t, err)

	// Test Add with empty path (should fail)
	err = tx.Add("", []byte("data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path cannot be empty")

	// Test Add with nil content (should be converted to empty)
	err = tx.Add("empty.txt", nil)
	assert.NoError(t, err)

	// Test validation
	err = tx.Validate()
	assert.NoError(t, err)

	// Test commit
	_, err = tx.Commit("Test commit")
	assert.NoError(t, err)
}

func TestConcurrentSafeTransactionalCommit_CommittedTransactionErrors(t *testing.T) {
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(context.Background()))

	tx := safeRepo.SafeTransaction()
	require.NoError(t, tx.Add("file.txt", []byte("content")))
	require.NoError(t, tx.Validate())
	
	// Commit transaction
	_, err := tx.Commit("Initial commit")
	require.NoError(t, err)

	// Try to add to committed transaction
	err = tx.Add("another.txt", []byte("more content"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "committed transaction")

	// Try to commit again
	_, err = tx.Commit("Second commit")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already committed")
}

func TestConcurrentSafeTransactionalCommit_ValidationRequired(t *testing.T) {
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(context.Background()))

	tx := safeRepo.SafeTransaction()
	require.NoError(t, tx.Add("file.txt", []byte("content")))

	// Try to commit without validation
	_, err := tx.Commit("Unvalidated commit")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not validated")
}

func TestConcurrentSafeTransactionalCommit_Rollback(t *testing.T) {
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(context.Background()))

	tx := safeRepo.SafeTransaction()
	require.NoError(t, tx.Add("file1.txt", []byte("content1")))
	require.NoError(t, tx.Add("file2.txt", []byte("content2")))
	require.NoError(t, tx.Validate())

	// Rollback transaction
	err := tx.Rollback()
	assert.NoError(t, err)

	// Should be able to add again after rollback
	err = tx.Add("file3.txt", []byte("content3"))
	assert.NoError(t, err)

	// But validation state should be reset
	_, err = tx.Commit("After rollback")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not validated")
}

func TestConcurrentSafeTransactionalCommit_RollbackCommitted(t *testing.T) {
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(context.Background()))

	tx := safeRepo.SafeTransaction()
	require.NoError(t, tx.Add("file.txt", []byte("content")))
	require.NoError(t, tx.Validate())
	_, err := tx.Commit("Test commit")
	require.NoError(t, err)

	// Try to rollback committed transaction
	err = tx.Rollback()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rollback committed")
}

func TestConcurrentSafeRepository_BranchOperations(t *testing.T) {
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(context.Background()))

	// Test creating branch with empty name
	err := safeRepo.SafeCreateBranch("", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "branch name cannot be empty")

	// Test creating valid branch
	err = safeRepo.SafeCreateBranch("feature", "")
	assert.NoError(t, err)

	// Test checking out with empty name
	err = safeRepo.SafeCheckout("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "branch name cannot be empty")

	// Test valid checkout
	err = safeRepo.SafeCheckout("feature")
	assert.NoError(t, err)
}

func TestConcurrentSafeRepository_CorruptionHandling(t *testing.T) {
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)

	// Manually set corruption flag
	atomic.StoreInt32(&safeRepo.corrupted, 1)
	assert.True(t, safeRepo.IsCorrupted())

	// Operations should fail when corrupted
	tx := safeRepo.SafeTransaction()
	err := tx.Add("file.txt", []byte("content"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "corrupted")

	err = safeRepo.SafeCreateBranch("test", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "corrupted")

	err = safeRepo.SafeCheckout("main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "corrupted")
}

func TestConcurrentSafeRepository_CorruptionRecovery(t *testing.T) {
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)

	// Initialize first
	require.NoError(t, safeRepo.Initialize(context.Background()))

	// Manually corrupt
	atomic.StoreInt32(&safeRepo.corrupted, 1)
	assert.True(t, safeRepo.IsCorrupted())

	// Attempt recovery
	err := safeRepo.RecoverFromCorruption()
	assert.NoError(t, err)
	assert.False(t, safeRepo.IsCorrupted())
}

func TestConcurrentSafeRepository_QuiescenceWaiting(t *testing.T) {
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(context.Background()))

	// Should complete immediately when no active operations
	err := safeRepo.WaitForQuiescence(100 * time.Millisecond)
	assert.NoError(t, err)

	// Simulate active operation
	atomic.AddInt64(&safeRepo.activeOps, 1)
	
	start := time.Now()
	err = safeRepo.WaitForQuiescence(50 * time.Millisecond)
	duration := time.Since(start)
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
	assert.True(t, duration >= 50*time.Millisecond)

	// Clean up
	atomic.AddInt64(&safeRepo.activeOps, -1)
}

// MMO Game Scenario Tests - These test the specific use cases mentioned in the bug report

func TestMMOGame_ConcurrentWorldCreation(t *testing.T) {
	// Test multiple concurrent world creations (the main issue from the bug report)
	manager := NewRepositoryManager()
	numWorlds := 10
	numGoroutines := 5

	var wg sync.WaitGroup
	errors := make(chan error, numWorlds*numGoroutines)

	// Multiple goroutines creating worlds concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numWorlds; j++ {
				worldName := fmt.Sprintf("world_%d_%d", goroutineID, j)
				repo, err := manager.GetOrCreateRepository(worldName)
				if err != nil {
					errors <- fmt.Errorf("failed to create world %s: %w", worldName, err)
					continue
				}

				// Simulate world initialization with commits
				tx := repo.SafeTransaction()
				err = tx.Add("world.dat", []byte(fmt.Sprintf("World data for %s", worldName)))
				if err != nil {
					errors <- fmt.Errorf("failed to add world data: %w", err)
					continue
				}

				err = tx.Validate()
				if err != nil {
					errors <- fmt.Errorf("failed to validate transaction: %w", err)
					continue
				}

				_, err = tx.Commit(fmt.Sprintf("Initialize %s", worldName))
				if err != nil {
					errors <- fmt.Errorf("failed to commit world: %w", err)
					continue
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Concurrent world creation error: %v", err)
	}
}

func TestMMOGame_ConcurrentUserOperations(t *testing.T) {
	// Test simultaneous user operations on the same repository
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(context.Background()))

	numUsers := 20
	operationsPerUser := 10
	var wg sync.WaitGroup
	errors := make(chan error, numUsers*operationsPerUser)

	// Multiple users performing operations concurrently
	for userID := 0; userID < numUsers; userID++ {
		wg.Add(1)
		go func(uid int) {
			defer wg.Done()
			for op := 0; op < operationsPerUser; op++ {
				tx := safeRepo.SafeTransaction()
				
				// User creates/updates their data
				userFile := fmt.Sprintf("users/%d/profile.json", uid)
				userData := []byte(fmt.Sprintf(`{"user_id": %d, "operation": %d, "timestamp": %d}`, uid, op, time.Now().UnixNano()))
				
				err := tx.Add(userFile, userData)
				if err != nil {
					errors <- fmt.Errorf("user %d op %d add failed: %w", uid, op, err)
					continue
				}

				err = tx.Validate()
				if err != nil {
					errors <- fmt.Errorf("user %d op %d validate failed: %w", uid, op, err)
					continue
				}

				_, err = tx.Commit(fmt.Sprintf("User %d operation %d", uid, op))
				if err != nil {
					errors <- fmt.Errorf("user %d op %d commit failed: %w", uid, op, err)
					continue
				}
			}
		}(userID)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent user operation error: %v", err)
	}
}

func TestMMOGame_HighFrequencyAssetSaving(t *testing.T) {
	// Test high-frequency asset and role saving
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(context.Background()))

	numAssets := 100
	saveFrequency := 50 // saves per asset
	var wg sync.WaitGroup
	errors := make(chan error, numAssets*saveFrequency)

	// High-frequency asset saving
	for assetID := 0; assetID < numAssets; assetID++ {
		wg.Add(1)
		go func(aid int) {
			defer wg.Done()
			for save := 0; save < saveFrequency; save++ {
				tx := safeRepo.SafeTransaction()
				
				// Save asset state
				assetPath := fmt.Sprintf("assets/%d/state.dat", aid)
				assetData := []byte(fmt.Sprintf("Asset %d state #%d at %d", aid, save, time.Now().UnixNano()))
				
				err := tx.Add(assetPath, assetData)
				if err != nil {
					errors <- fmt.Errorf("asset %d save %d failed: %w", aid, save, err)
					continue
				}

				// Also save role data
				rolePath := fmt.Sprintf("roles/%d/role_%d.json", aid, save%10)
				roleData := []byte(fmt.Sprintf(`{"asset_id": %d, "role": "role_%d", "save": %d}`, aid, save%10, save))
				
				err = tx.Add(rolePath, roleData)
				if err != nil {
					errors <- fmt.Errorf("role %d save %d failed: %w", aid, save, err)
					continue
				}

				err = tx.Validate()
				if err != nil {
					errors <- fmt.Errorf("asset %d save %d validate failed: %w", aid, save, err)
					continue
				}

				_, err = tx.Commit(fmt.Sprintf("Save asset %d #%d", aid, save))
				if err != nil {
					errors <- fmt.Errorf("asset %d save %d commit failed: %w", aid, save, err)
					continue
				}
			}
		}(assetID)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("High-frequency asset saving error: %v", err)
	}
}

func TestMMOGame_ConcurrentChatPersistence(t *testing.T) {
	// Test concurrent chat message persistence
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(context.Background()))

	numChannels := 5
	messagesPerChannel := 100
	var wg sync.WaitGroup
	errors := make(chan error, numChannels*messagesPerChannel)

	// Multiple chat channels with concurrent message persistence
	for channelID := 0; channelID < numChannels; channelID++ {
		wg.Add(1)
		go func(cid int) {
			defer wg.Done()
			for msgID := 0; msgID < messagesPerChannel; msgID++ {
				tx := safeRepo.SafeTransaction()
				
				// Persist chat message
				msgPath := fmt.Sprintf("chat/%d/%d.json", cid, msgID)
				msgData := []byte(fmt.Sprintf(`{"channel": %d, "message_id": %d, "content": "Message %d in channel %d", "timestamp": %d}`, cid, msgID, msgID, cid, time.Now().UnixNano()))
				
				err := tx.Add(msgPath, msgData)
				if err != nil {
					errors <- fmt.Errorf("chat channel %d msg %d failed: %w", cid, msgID, err)
					continue
				}

				err = tx.Validate()
				if err != nil {
					errors <- fmt.Errorf("chat channel %d msg %d validate failed: %w", cid, msgID, err)
					continue
				}

				_, err = tx.Commit(fmt.Sprintf("Chat message %d in channel %d", msgID, cid))
				if err != nil {
					errors <- fmt.Errorf("chat channel %d msg %d commit failed: %w", cid, msgID, err)
					continue
				}
			}
		}(channelID)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent chat persistence error: %v", err)
	}
}

func TestRepositoryManager_ConcurrentAccess(t *testing.T) {
	manager := NewRepositoryManager()
	
	// Test concurrent access to same repository name
	repoName := "shared_repo"
	numGoroutines := 10
	var wg sync.WaitGroup
	repos := make(chan *ConcurrentSafeRepository, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			repo, err := manager.GetOrCreateRepository(repoName)
			if err != nil {
				t.Errorf("Failed to get repository: %v", err)
				return
			}
			repos <- repo
		}()
	}

	wg.Wait()
	close(repos)

	// All goroutines should get the same repository instance
	var firstRepo *ConcurrentSafeRepository
	count := 0
	for repo := range repos {
		count++
		if firstRepo == nil {
			firstRepo = repo
		} else {
			assert.Equal(t, firstRepo, repo, "All goroutines should get the same repository instance")
		}
	}
	assert.Equal(t, numGoroutines, count)
}

func TestRepositoryManager_RemoveRepository(t *testing.T) {
	manager := NewRepositoryManager()
	
	// Create a repository
	repo, err := manager.GetOrCreateRepository("test_repo")
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Remove it
	err = manager.RemoveRepository("test_repo")
	assert.NoError(t, err)

	// Create again - should be a new instance
	newRepo, err := manager.GetOrCreateRepository("test_repo")
	require.NoError(t, err)
	require.NotNil(t, newRepo)

	// Should be different instances
	assert.NotEqual(t, repo, newRepo)
}

func TestRepositoryManager_RemoveWithActiveOperations(t *testing.T) {
	manager := NewRepositoryManager()
	
	repo, err := manager.GetOrCreateRepository("active_repo")
	require.NoError(t, err)

	// Simulate active operation
	atomic.AddInt64(&repo.activeOps, 1)
	
	// Attempt to remove - should timeout
	err = manager.RemoveRepository("active_repo")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "active")

	// Clean up active operation
	atomic.AddInt64(&repo.activeOps, -1)

	// Now removal should work
	err = manager.RemoveRepository("active_repo")
	assert.NoError(t, err)
}

func TestRetryPolicy_ExponentialBackoff(t *testing.T) {
	policy := DefaultRetryPolicy()
	assert.Equal(t, 3, policy.MaxAttempts)
	assert.Equal(t, 10*time.Millisecond, policy.InitialDelay)
	assert.Equal(t, 2.0, policy.BackoffFactor)
	assert.Equal(t, 1*time.Second, policy.MaxDelay)
}

func TestConcurrentSafeTransactionalCommit_RetryLogic(t *testing.T) {
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(context.Background()))

	tx := safeRepo.SafeTransaction()

	// Test retryable error detection
	retryableErr := fmt.Errorf("invalid argument")
	assert.True(t, tx.isRetryableError(retryableErr))

	nonRetryableErr := fmt.Errorf("permanent failure")
	assert.False(t, tx.isRetryableError(nonRetryableErr))

	// Test nil error
	assert.False(t, tx.isRetryableError(nil))
}

func TestConcurrentSafeTransactionalCommit_FileValidation(t *testing.T) {
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(context.Background()))

	tx := safeRepo.SafeTransaction()

	// Test large file validation (should fail)
	largeData := make([]byte, 200*1024*1024) // 200MB
	require.NoError(t, tx.Add("large.dat", largeData))
	
	err := tx.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

// Stress test to validate the fix for parallel.go:177 race condition
func TestParallelCommitRaceConditionFix(t *testing.T) {
	// This test specifically targets the race condition that was reported
	// at parallel.go:177 in TransactionalCommit.Add()
	
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(context.Background()))

	numGoroutines := 50
	commitsPerGoroutine := 20
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*commitsPerGoroutine)

	// Multiple goroutines doing the exact operation that was failing
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < commitsPerGoroutine; j++ {
				tx := safeRepo.SafeTransaction()
				
				// This was the problematic line: tc.repo.store.StoreBlob(content)
				// Our fix ensures proper nil checking and locking
				path := fmt.Sprintf("file_%d_%d.txt", goroutineID, j)
				content := []byte(fmt.Sprintf("Content from goroutine %d, commit %d", goroutineID, j))
				
				err := tx.Add(path, content) // This line was causing nil pointer dereference
				if err != nil {
					errors <- fmt.Errorf("goroutine %d commit %d: %w", goroutineID, j, err)
					continue
				}

				err = tx.Validate()
				if err != nil {
					errors <- fmt.Errorf("goroutine %d commit %d validate: %w", goroutineID, j, err)
					continue
				}

				_, err = tx.Commit(fmt.Sprintf("Commit %d_%d", goroutineID, j))
				if err != nil {
					errors <- fmt.Errorf("goroutine %d commit %d final: %w", goroutineID, j, err)
					continue
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// If our fix is working, there should be no errors
	errorCount := 0
	for err := range errors {
		t.Errorf("Race condition still exists: %v", err)
		errorCount++
	}

	if errorCount == 0 {
		t.Logf("SUCCESS: Race condition fix validated - %d concurrent operations completed without errors", numGoroutines*commitsPerGoroutine)
	}
}

// Benchmark tests to measure performance impact of safety measures
func BenchmarkConcurrentSafeRepository_BasicOperations(b *testing.B) {
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(b, safeRepo.Initialize(context.Background()))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tx := safeRepo.SafeTransaction()
			path := fmt.Sprintf("bench_file_%d.txt", i)
			content := []byte(fmt.Sprintf("Benchmark content %d", i))
			
			tx.Add(path, content)
			tx.Validate()
			tx.Commit(fmt.Sprintf("Benchmark commit %d", i))
			i++
		}
	})
}

func BenchmarkConcurrentSafeRepository_vs_Original(b *testing.B) {
	// Compare performance of original vs safe implementation
	
	b.Run("Original", func(b *testing.B) {
		repo := NewRepository()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tx := repo.Transaction()
			tx.Add(fmt.Sprintf("file_%d.txt", i), []byte(fmt.Sprintf("content_%d", i)))
			tx.Validate()
			tx.Commit(fmt.Sprintf("commit_%d", i))
		}
	})

	b.Run("ConcurrentSafe", func(b *testing.B) {
		repo := NewRepository()
		safeRepo := NewConcurrentSafeRepository(repo)
		require.NoError(b, safeRepo.Initialize(context.Background()))
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tx := safeRepo.SafeTransaction()
			tx.Add(fmt.Sprintf("file_%d.txt", i), []byte(fmt.Sprintf("content_%d", i)))
			tx.Validate()
			tx.Commit(fmt.Sprintf("commit_%d", i))
		}
	})
}