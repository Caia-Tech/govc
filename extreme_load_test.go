package govc

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtremeLoad_ThousandConcurrentUsers tests 1000+ concurrent operations
func TestExtremeLoad_ThousandConcurrentUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping extreme load test in short mode")
	}

	manager := NewRepositoryManager()
	numUsers := 1000
	reposPerUser := 1
	operationsPerRepo := 5
	
	var wg sync.WaitGroup
	var successCount, errorCount int64
	
	startTime := time.Now()
	
	// Create 1000 concurrent users
	for userID := 0; userID < numUsers; userID++ {
		wg.Add(1)
		go func(uid int) {
			defer wg.Done()
			
			repoName := fmt.Sprintf("extreme_user_%d_repo", uid)
			
			// Create repository
			repo, err := manager.GetOrCreateRepository(repoName)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			
			// Perform multiple operations on this repository
			for op := 0; op < operationsPerRepo; op++ {
				tx := repo.SafeTransaction()
				
				// Create unique content for this user and operation
				content := []byte(fmt.Sprintf(`{
	"user_id": %d,
	"operation": %d,
	"timestamp": %d,
	"data": "extreme_load_test_data_for_user_%d_op_%d"
}`, uid, op, time.Now().UnixNano(), uid, op))
				
				path := fmt.Sprintf("user_data/op_%d.json", op)
				err = tx.Add(path, content)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					continue
				}
				
				err = tx.Validate()
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					continue
				}
				
				_, err = tx.Commit(fmt.Sprintf("User %d operation %d", uid, op))
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					continue
				}
				
				atomic.AddInt64(&successCount, 1)
			}
		}(userID)
	}
	
	wg.Wait()
	duration := time.Since(startTime)
	
	totalOperations := successCount + errorCount
	expectedOperations := int64(numUsers * reposPerUser * operationsPerRepo)
	
	t.Logf("Extreme Load Test Results:")
	t.Logf("  Concurrent users: %d", numUsers)
	t.Logf("  Operations per user: %d", operationsPerRepo)
	t.Logf("  Expected operations: %d", expectedOperations)
	t.Logf("  Successful operations: %d", successCount)
	t.Logf("  Failed operations: %d", errorCount)
	t.Logf("  Total duration: %v", duration)
	t.Logf("  Operations/second: %.2f", float64(successCount)/duration.Seconds())
	t.Logf("  Success rate: %.2f%%", float64(successCount)/float64(totalOperations)*100)
	
	assert.True(t, successCount > expectedOperations*8/10, "Should have > 80% success rate")
	assert.True(t, float64(successCount)/duration.Seconds() > 100, "Should achieve > 100 ops/sec")
	
	t.Logf("✅ Extreme load test passed with %d concurrent users", numUsers)
}

// TestExtremeLoad_MassiveBranchOperations tests branch creation under extreme load
func TestExtremeLoad_MassiveBranchOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping extreme load test in short mode")
	}

	manager := NewRepositoryManager()
	repo, err := manager.GetOrCreateRepository("branch_stress_repo")
	require.NoError(t, err)
	
	// Create initial commit
	tx := repo.SafeTransaction()
	err = tx.Add("README.md", []byte("Branch stress test repository"))
	require.NoError(t, err)
	err = tx.Validate()
	require.NoError(t, err)
	_, err = tx.Commit("Initial commit for branch stress test")
	require.NoError(t, err)
	
	numBranches := 2000
	var wg sync.WaitGroup
	var successCount, errorCount int64
	
	startTime := time.Now()
	
	// Create many branches concurrently
	for i := 0; i < numBranches; i++ {
		wg.Add(1)
		go func(branchNum int) {
			defer wg.Done()
			
			branchName := fmt.Sprintf("extreme_branch_%d", branchNum)
			
			err := repo.SafeCreateBranch(branchName, "")
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			
			// Switch to branch and make a commit
			err = repo.SafeCheckout(branchName)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			
			tx := repo.SafeTransaction()
			content := []byte(fmt.Sprintf("Content for branch %d", branchNum))
			path := fmt.Sprintf("branch_files/file_%d.txt", branchNum)
			
			err = tx.Add(path, content)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			
			err = tx.Validate()
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			
			_, err = tx.Commit(fmt.Sprintf("Commit on branch %d", branchNum))
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			
			atomic.AddInt64(&successCount, 1)
		}(i)
	}
	
	wg.Wait()
	duration := time.Since(startTime)
	
	totalOperations := successCount + errorCount
	
	t.Logf("Massive Branch Operations Test Results:")
	t.Logf("  Target branches: %d", numBranches)
	t.Logf("  Successful branch operations: %d", successCount)
	t.Logf("  Failed operations: %d", errorCount)
	t.Logf("  Total duration: %v", duration)
	t.Logf("  Branches/second: %.2f", float64(successCount)/duration.Seconds())
	t.Logf("  Success rate: %.2f%%", float64(successCount)/float64(totalOperations)*100)
	
	assert.True(t, successCount > int64(numBranches*8/10), "Should create > 80% of branches successfully")
	
	t.Logf("✅ Massive branch operations test passed with %d branches", successCount)
}

// TestExtremeLoad_HighFrequencyReads tests extremely high read frequency
func TestExtremeLoad_HighFrequencyReads(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping extreme load test in short mode")
	}

	manager := NewRepositoryManager()
	repo, err := manager.GetOrCreateRepository("read_stress_repo")
	require.NoError(t, err)
	
	// Create initial data
	tx := repo.SafeTransaction()
	for i := 0; i < 100; i++ {
		path := fmt.Sprintf("data/file_%d.txt", i)
		content := []byte(fmt.Sprintf("Initial content for file %d", i))
		err = tx.Add(path, content)
		require.NoError(t, err)
	}
	err = tx.Validate()
	require.NoError(t, err)
	_, err = tx.Commit("Initial data for read stress test")
	require.NoError(t, err)
	
	duration := 10 * time.Second
	numReaders := 100
	var wg sync.WaitGroup
	var totalReads int64
	done := make(chan struct{})
	
	// Start timer
	go func() {
		time.Sleep(duration)
		close(done)
	}()
	
	startTime := time.Now()
	
	// Create many concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			count := 0
			
			for {
				select {
				case <-done:
					atomic.AddInt64(&totalReads, int64(count))
					return
				default:
					// Perform various read operations
					switch count % 4 {
					case 0:
						files := repo.SafeListStagedFiles()
						_ = files
					case 1:
						files := repo.SafeGetStagedFiles()
						_ = files
					case 2:
						_ = repo.IsCorrupted()
					case 3:
						tx := repo.SafeTransaction()
						_ = tx
					}
					count++
				}
			}
		}(i)
	}
	
	// One writer to ensure there are concurrent writes
	wg.Add(1)
	go func() {
		defer wg.Done()
		writeCount := 0
		
		for {
			select {
			case <-done:
				return
			default:
				tx := repo.SafeTransaction()
				path := fmt.Sprintf("writes/write_%d.txt", writeCount)
				content := []byte(fmt.Sprintf("Write content %d", writeCount))
				
				err := tx.Add(path, content)
				if err == nil {
					err = tx.Validate()
					if err == nil {
						_, err = tx.Commit(fmt.Sprintf("Write %d", writeCount))
					}
				}
				
				if err == nil {
					writeCount++
				}
				
				// Small delay to not overwhelm writers
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()
	
	wg.Wait()
	actualDuration := time.Since(startTime)
	
	readsPerSecond := float64(totalReads) / actualDuration.Seconds()
	
	t.Logf("High Frequency Reads Test Results:")
	t.Logf("  Concurrent readers: %d", numReaders)
	t.Logf("  Test duration: %v", duration)
	t.Logf("  Total reads: %d", totalReads)
	t.Logf("  Reads/second: %.2f", readsPerSecond)
	t.Logf("  Reads per reader per second: %.2f", readsPerSecond/float64(numReaders))
	
	assert.True(t, totalReads > 10000, "Should achieve > 10,000 reads")
	assert.True(t, readsPerSecond > 1000, "Should achieve > 1,000 reads/second")
	
	t.Logf("✅ High frequency reads test passed with %.0f reads/second", readsPerSecond)
}

// TestExtremeLoad_ConcurrentRepositoryCreation tests massive concurrent repository creation
func TestExtremeLoad_ConcurrentRepositoryCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping extreme load test in short mode")
	}

	manager := NewRepositoryManager()
	numRepositories := 500
	var wg sync.WaitGroup
	var successCount, errorCount int64
	
	startTime := time.Now()
	
	// Create many repositories concurrently
	for i := 0; i < numRepositories; i++ {
		wg.Add(1)
		go func(repoNum int) {
			defer wg.Done()
			
			repoName := fmt.Sprintf("concurrent_repo_%d", repoNum)
			
			repo, err := manager.GetOrCreateRepository(repoName)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			
			// Initialize each repository with some content
			tx := repo.SafeTransaction()
			content := []byte(fmt.Sprintf(`{
	"repository_id": %d,
	"name": "%s",
	"created_at": "%s"
}`, repoNum, repoName, time.Now().Format(time.RFC3339)))
			
			err = tx.Add("repo.json", content)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			
			err = tx.Validate()
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			
			_, err = tx.Commit(fmt.Sprintf("Initialize repository %d", repoNum))
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			
			// Verify repository is not corrupted
			if repo.IsCorrupted() {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			
			atomic.AddInt64(&successCount, 1)
		}(i)
	}
	
	wg.Wait()
	duration := time.Since(startTime)
	
	totalOperations := successCount + errorCount
	
	t.Logf("Concurrent Repository Creation Test Results:")
	t.Logf("  Target repositories: %d", numRepositories)
	t.Logf("  Successful creations: %d", successCount)
	t.Logf("  Failed creations: %d", errorCount)
	t.Logf("  Total duration: %v", duration)
	t.Logf("  Repositories/second: %.2f", float64(successCount)/duration.Seconds())
	t.Logf("  Success rate: %.2f%%", float64(successCount)/float64(totalOperations)*100)
	
	assert.True(t, successCount > int64(numRepositories*9/10), "Should create > 90% of repositories successfully")
	
	// Clean up repositories
	for i := 0; i < int(successCount); i++ {
		repoName := fmt.Sprintf("concurrent_repo_%d", i)
		err := manager.RemoveRepository(repoName)
		assert.NoError(t, err)
	}
	
	t.Logf("✅ Concurrent repository creation test passed with %d repositories", successCount)
}