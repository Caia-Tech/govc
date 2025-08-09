package govc

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStress_HighVolumeOperations tests the system under high concurrent load
func TestStress_HighVolumeOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	// Use concurrent safe repository for stress testing
	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(nil))

	// Test configuration
	numGoroutines := 20
	duration := 10 * time.Second

	var totalOperations int64
	var errors int64
	var wg sync.WaitGroup
	done := make(chan struct{})

	// Start timer
	go func() {
		time.Sleep(duration)
		close(done)
	}()

	// Memory statistics before
	var memBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	// Writer goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			operations := 0
			
			for {
				select {
				case <-done:
					t.Logf("Worker %d completed %d operations", workerID, operations)
					atomic.AddInt64(&totalOperations, int64(operations))
					return
				default:
					// Create a transaction
					tx := safeRepo.SafeTransaction()
					
					// Add multiple files in this transaction
					for j := 0; j < 5; j++ {
						path := fmt.Sprintf("stress/%d/%d/file_%d.txt", workerID, operations, j)
						content := []byte(fmt.Sprintf("Stress test content from worker %d, operation %d, file %d", workerID, operations, j))
						
						err := tx.Add(path, content)
						if err != nil {
							atomic.AddInt64(&errors, 1)
							continue
						}
					}
					
					// Validate and commit
					err := tx.Validate()
					if err != nil {
						atomic.AddInt64(&errors, 1)
						continue
					}
					
					_, err = tx.Commit(fmt.Sprintf("Stress commit %d from worker %d", operations, workerID))
					if err != nil {
						atomic.AddInt64(&errors, 1)
						continue
					}
					
					operations++
				}
			}
		}(i)
	}

	wg.Wait()
	
	// Memory statistics after
	var memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memAfter)

	// Results
	errorCount := atomic.LoadInt64(&errors)
	totalOps := atomic.LoadInt64(&totalOperations)
	
	t.Logf("Stress Test Results:")
	t.Logf("  Total Operations: %d", totalOps)
	t.Logf("  Total Errors: %d", errorCount)
	t.Logf("  Operations/second: %.2f", float64(totalOps)/duration.Seconds())
	t.Logf("  Memory Usage: %d KB -> %d KB (diff: %d KB)", 
		memBefore.Alloc/1024, memAfter.Alloc/1024, (memAfter.Alloc-memBefore.Alloc)/1024)
	t.Logf("  Goroutines: %d", runtime.NumGoroutine())

	assert.True(t, totalOps > 0, "Should have completed some operations")
	assert.True(t, errorCount < totalOps/10, "Error rate should be less than 10%")
}

// TestStress_ConcurrentBranching tests concurrent branch operations
func TestStress_ConcurrentBranching(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	manager := NewRepositoryManager()
	numRepos := 5
	branchesPerRepo := 50
	
	var wg sync.WaitGroup
	var errors int64

	for repoID := 0; repoID < numRepos; repoID++ {
		wg.Add(1)
		go func(rid int) {
			defer wg.Done()
			
			repoName := fmt.Sprintf("stress_repo_%d", rid)
			safeRepo, err := manager.GetOrCreateRepository(repoName)
			if err != nil {
				atomic.AddInt64(&errors, 1)
				return
			}
			
			// Create initial commit
			tx := safeRepo.SafeTransaction()
			err = tx.Add("README.md", []byte(fmt.Sprintf("Repository %d", rid)))
			if err != nil {
				atomic.AddInt64(&errors, 1)
				return
			}
			
			err = tx.Validate()
			if err != nil {
				atomic.AddInt64(&errors, 1)
				return
			}
			
			_, err = tx.Commit("Initial commit")
			if err != nil {
				atomic.AddInt64(&errors, 1)
				return
			}

			// Create many branches concurrently
			var branchWg sync.WaitGroup
			for b := 0; b < branchesPerRepo; b++ {
				branchWg.Add(1)
				go func(branchID int) {
					defer branchWg.Done()
					
					branchName := fmt.Sprintf("branch_%d_%d", rid, branchID)
					err := safeRepo.SafeCreateBranch(branchName, "")
					if err != nil {
						atomic.AddInt64(&errors, 1)
						return
					}
					
					// Switch to branch and make commits
					err = safeRepo.SafeCheckout(branchName)
					if err != nil {
						atomic.AddInt64(&errors, 1)
						return
					}
					
					// Create commits on this branch
					for c := 0; c < 3; c++ {
						tx := safeRepo.SafeTransaction()
						path := fmt.Sprintf("branch_files/%s/file_%d.txt", branchName, c)
						content := []byte(fmt.Sprintf("Content from %s commit %d", branchName, c))
						
						err = tx.Add(path, content)
						if err == nil {
							err = tx.Validate()
							if err == nil {
								_, err = tx.Commit(fmt.Sprintf("Commit %d on %s", c, branchName))
							}
						}
						
						if err != nil {
							atomic.AddInt64(&errors, 1)
						}
					}
				}(b)
			}
			branchWg.Wait()
		}(repoID)
	}

	wg.Wait()
	
	errorCount := atomic.LoadInt64(&errors)
	totalBranches := numRepos * branchesPerRepo
	totalCommits := totalBranches * 3
	
	t.Logf("Branch Stress Test Results:")
	t.Logf("  Repositories: %d", numRepos)
	t.Logf("  Branches per repo: %d", branchesPerRepo)
	t.Logf("  Total branches: %d", totalBranches)
	t.Logf("  Expected commits: %d", totalCommits)
	t.Logf("  Errors: %d", errorCount)
	t.Logf("  Success rate: %.2f%%", float64(totalCommits-int(errorCount))/float64(totalCommits)*100)

	assert.True(t, errorCount < int64(totalCommits)/20, "Error rate should be less than 5%")
}

// TestStress_MemoryPressure tests behavior under memory pressure
func TestStress_MemoryPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	repo := NewRepository() 
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(nil))

	// Create large commits to put pressure on memory
	numLargeCommits := 50
	fileSizeKB := 100 // 100KB per file
	filesPerCommit := 20

	var wg sync.WaitGroup
	var errors int64

	for i := 0; i < numLargeCommits; i++ {
		wg.Add(1)
		go func(commitID int) {
			defer wg.Done()
			
			tx := safeRepo.SafeTransaction()
			
			// Create large files
			for f := 0; f < filesPerCommit; f++ {
				path := fmt.Sprintf("large/commit_%d/file_%d.txt", commitID, f)
				
				// Create content with repeated patterns to test memory usage
				content := make([]byte, fileSizeKB*1024)
				pattern := []byte(fmt.Sprintf("COMMIT_%d_FILE_%d_", commitID, f))
				
				for j := 0; j < len(content); j++ {
					content[j] = pattern[j%len(pattern)]
				}
				
				err := tx.Add(path, content)
				if err != nil {
					atomic.AddInt64(&errors, 1)
					return
				}
			}
			
			err := tx.Validate()
			if err != nil {
				atomic.AddInt64(&errors, 1)
				return
			}
			
			_, err = tx.Commit(fmt.Sprintf("Large commit %d", commitID))
			if err != nil {
				atomic.AddInt64(&errors, 1)
			}
		}(i)
	}

	wg.Wait()
	
	var memStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memStats)
	
	errorCount := atomic.LoadInt64(&errors)
	expectedSize := numLargeCommits * filesPerCommit * fileSizeKB // KB
	
	t.Logf("Memory Pressure Test Results:")
	t.Logf("  Large commits: %d", numLargeCommits)
	t.Logf("  Files per commit: %d", filesPerCommit)
	t.Logf("  File size: %d KB", fileSizeKB)
	t.Logf("  Expected total data: %d KB", expectedSize)
	t.Logf("  Memory allocated: %d KB", memStats.Alloc/1024)
	t.Logf("  Errors: %d", errorCount)
	t.Logf("  Success rate: %.2f%%", float64(numLargeCommits-int(errorCount))/float64(numLargeCommits)*100)

	assert.True(t, errorCount < int64(numLargeCommits)/10, "Error rate should be less than 10% even under memory pressure")
}

// TestStress_RapidCommits tests very rapid commit operations
func TestStress_RapidCommits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(nil))

	numWorkers := 10
	commitsPerWorker := 200
	
	var wg sync.WaitGroup
	var totalCommits int64
	var errors int64
	
	start := time.Now()

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for c := 0; c < commitsPerWorker; c++ {
				tx := safeRepo.SafeTransaction()
				
				path := fmt.Sprintf("rapid/%d/commit_%d.txt", workerID, c)
				content := []byte(fmt.Sprintf("Rapid commit %d from worker %d at %d", c, workerID, time.Now().UnixNano()))
				
				err := tx.Add(path, content)
				if err != nil {
					atomic.AddInt64(&errors, 1)
					continue
				}
				
				err = tx.Validate()
				if err != nil {
					atomic.AddInt64(&errors, 1)
					continue
				}
				
				_, err = tx.Commit(fmt.Sprintf("Rapid %d_%d", workerID, c))
				if err != nil {
					atomic.AddInt64(&errors, 1)
					continue
				}
				
				atomic.AddInt64(&totalCommits, 1)
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)
	
	commits := atomic.LoadInt64(&totalCommits)
	errorCount := atomic.LoadInt64(&errors)
	
	t.Logf("Rapid Commits Test Results:")
	t.Logf("  Workers: %d", numWorkers)
	t.Logf("  Commits per worker: %d", commitsPerWorker)
	t.Logf("  Total successful commits: %d", commits)
	t.Logf("  Total errors: %d", errorCount)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Commits/second: %.2f", float64(commits)/duration.Seconds())
	t.Logf("  Success rate: %.2f%%", float64(commits)/float64(numWorkers*commitsPerWorker)*100)

	assert.True(t, commits > int64(numWorkers*commitsPerWorker/2), "Should complete at least 50% of commits")
	assert.True(t, float64(commits)/duration.Seconds() > 10, "Should achieve at least 10 commits/second")
}

// TestStress_ConcurrentReadWrite tests concurrent read and write operations
func TestStress_ConcurrentReadWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	repo := NewRepository()
	safeRepo := NewConcurrentSafeRepository(repo)
	require.NoError(t, safeRepo.Initialize(nil))

	// Create initial data
	tx := safeRepo.SafeTransaction()
	for i := 0; i < 100; i++ {
		path := fmt.Sprintf("data/file_%d.txt", i)
		content := []byte(fmt.Sprintf("Initial content %d", i))
		require.NoError(t, tx.Add(path, content))
	}
	require.NoError(t, tx.Validate())
	_, err := tx.Commit("Initial data")
	require.NoError(t, err)

	duration := 5 * time.Second
	var wg sync.WaitGroup
	done := make(chan struct{})
	var readOps, writeOps, errors int64

	// Start timer
	go func() {
		time.Sleep(duration)
		close(done)
	}()

	// Reader goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			count := 0
			
			for {
				select {
				case <-done:
					t.Logf("Reader %d performed %d operations", readerID, count)
					atomic.AddInt64(&readOps, int64(count))
					return
				default:
					// Random read operations - use only thread-safe methods
					switch count % 3 {
					case 0:
						// List staged files safely
						files := safeRepo.SafeListStagedFiles()
						_ = files
					case 1:
						// Check if repository is corrupted
						_ = safeRepo.IsCorrupted()
					case 2:
						// Get staged files safely
						files := safeRepo.SafeGetStagedFiles()
						_ = files
					}
					count++
				}
			}
		}(i)
	}

	// Writer goroutines
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			count := 0
			
			for {
				select {
				case <-done:
					t.Logf("Writer %d performed %d operations", writerID, count)
					atomic.AddInt64(&writeOps, int64(count))
					return
				default:
					tx := safeRepo.SafeTransaction()
					path := fmt.Sprintf("writer_%d/operation_%d.txt", writerID, count)
					content := []byte(fmt.Sprintf("Writer %d operation %d at %d", writerID, count, time.Now().UnixNano()))
					
					err := tx.Add(path, content)
					if err == nil {
						err = tx.Validate()
						if err == nil {
							_, err = tx.Commit(fmt.Sprintf("Writer %d op %d", writerID, count))
						}
					}
					
					if err != nil {
						atomic.AddInt64(&errors, 1)
					}
					count++
				}
			}
		}(i)
	}

	wg.Wait()
	
	reads := atomic.LoadInt64(&readOps)
	writes := atomic.LoadInt64(&writeOps)
	errorCount := atomic.LoadInt64(&errors)
	
	t.Logf("Concurrent Read/Write Test Results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Read operations: %d (%.2f/sec)", reads, float64(reads)/duration.Seconds())
	t.Logf("  Write operations: %d (%.2f/sec)", writes, float64(writes)/duration.Seconds())
	t.Logf("  Total operations: %d", reads+writes)
	t.Logf("  Errors: %d", errorCount)
	t.Logf("  Error rate: %.2f%%", float64(errorCount)/float64(reads+writes)*100)

	assert.True(t, reads > 0, "Should have completed read operations")
	assert.True(t, writes > 0, "Should have completed write operations")
	assert.True(t, errorCount < (reads+writes)/20, "Error rate should be less than 5%")
}