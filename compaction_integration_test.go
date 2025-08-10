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

// TestMemoryCompaction_HighFrequencyCommits simulates 72K commits/hour for sustained periods
func TestMemoryCompaction_HighFrequencyCommits(t *testing.T) {
	baseRepo := NewRepository()
	repo := NewConcurrentSafeRepository(baseRepo)
	err := repo.Initialize(context.Background())
	require.NoError(t, err)
	
	// Enable aggressive compaction for testing
	policy := CompactionPolicy{
		Enabled:            true,
		Interval:           100 * time.Millisecond, // Very frequent for testing
		MemoryThreshold:    0.5,                    // Trigger at 50%
		MinObjectAge:       10 * time.Millisecond,  // Very short age for testing
		MaxObjectsPerCycle: 100,
	}
	
	err = repo.EnableAutoCompaction(policy)
	require.NoError(t, err)
	
	// Simulate high-frequency commits (scaled down for test duration)
	// 72,000 commits/hour = 20 commits/second
	// Test with 100 commits over 5 seconds (20 commits/second)
	commitsPerSecond := 20
	testDurationSeconds := 5
	totalCommits := commitsPerSecond * testDurationSeconds
	
	startTime := time.Now()
	var wg sync.WaitGroup
	
	// Track memory usage during the test
	memoryStats := make([]MemoryStats, 0)
	var statsMu sync.Mutex
	
	// Start memory monitoring goroutine
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(testDurationSeconds+1)*time.Second)
	defer cancel()
	
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats := repo.GetMemoryUsage()
				statsMu.Lock()
				memoryStats = append(memoryStats, stats)
				statsMu.Unlock()
			}
		}
	}()
	
	// Execute high-frequency commits
	wg.Add(totalCommits)
	for i := 0; i < totalCommits; i++ {
		go func(commitNum int) {
			defer wg.Done()
			
			// Create a transaction with some data
			tx := repo.SafeTransaction()
			fileName := fmt.Sprintf("file_%d.txt", commitNum)
			content := fmt.Sprintf("content for commit %d at %v", commitNum, time.Now())
			
			err := tx.Add(fileName, []byte(content))
			if err != nil {
				t.Errorf("Failed to add file in commit %d: %v", commitNum, err)
				return
			}
			
			err = tx.Validate()
			if err != nil {
				t.Errorf("Failed to validate commit %d: %v", commitNum, err)
				return
			}
			
			_, err = tx.Commit(fmt.Sprintf("High frequency commit %d", commitNum))
			if err != nil {
				t.Errorf("Failed to commit %d: %v", commitNum, err)
				return
			}
		}(i)
		
		// Rate limiting to achieve target commits/second
		if i > 0 && i%commitsPerSecond == 0 {
			time.Sleep(1 * time.Second)
		}
	}
	
	// Wait for all commits to complete
	wg.Wait()
	duration := time.Since(startTime)
	
	// Wait a bit longer for final compaction
	time.Sleep(200 * time.Millisecond)
	cancel() // Stop monitoring
	
	// Verify performance
	actualRate := float64(totalCommits) / duration.Seconds()
	t.Logf("Achieved commit rate: %.1f commits/second", actualRate)
	assert.Greater(t, actualRate, 15.0, "Should achieve at least 15 commits/second")
	
	// Verify memory management
	finalStats := repo.GetMemoryUsage()
	compactionStats := repo.GetCompactionStats()
	
	t.Logf("Final memory stats: %+v", finalStats)
	t.Logf("Compaction stats: %+v", compactionStats)
	
	// Memory should be bounded (not grow linearly with commits)
	// With compaction, memory should not exceed a reasonable threshold
	maxExpectedObjects := int64(totalCommits / 2) // Compaction should reduce objects
	assert.LessOrEqual(t, finalStats.TotalObjects, maxExpectedObjects,
		"Memory should be bounded by compaction, got %d objects", finalStats.TotalObjects)
	
	// Compaction should have run multiple times
	assert.Greater(t, compactionStats.TotalRuns, int64(0), "Compaction should have run")
	
	// Check that memory didn't grow unboundedly
	statsMu.Lock()
	defer statsMu.Unlock()
	
	if len(memoryStats) > 2 {
		// Memory growth should level off due to compaction
		maxObjects := int64(0)
		for _, stats := range memoryStats {
			if stats.TotalObjects > maxObjects {
				maxObjects = stats.TotalObjects
			}
		}
		
		// Final memory should not be dramatically higher than peak
		// (allows for some growth but prevents unbounded growth)
		growthRatio := float64(finalStats.TotalObjects) / float64(maxObjects)
		assert.LessOrEqual(t, growthRatio, 2.0, "Memory growth should be bounded")
	}
}

// TestMemoryCompaction_ConcurrentOperations tests compaction with concurrent read/write operations
func TestMemoryCompaction_ConcurrentOperations(t *testing.T) {
	baseRepo := NewRepository()
	repo := NewConcurrentSafeRepository(baseRepo)
	err := repo.Initialize(context.Background())
	require.NoError(t, err)
	
	// Enable compaction
	policy := CompactionPolicy{
		Enabled:            true,
		Interval:           50 * time.Millisecond,
		MemoryThreshold:    0.3,
		MinObjectAge:       10 * time.Millisecond,
		MaxObjectsPerCycle: 50,
	}
	
	err = repo.EnableAutoCompaction(policy)
	require.NoError(t, err)
	
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	var wg sync.WaitGroup
	numWriters := 10
	numReaders := 5
	opsPerWorker := 50
	
	// Start writer goroutines
	wg.Add(numWriters)
	for i := 0; i < numWriters; i++ {
		go func(writerID int) {
			defer wg.Done()
			
			for j := 0; j < opsPerWorker; j++ {
				select {
				case <-ctx.Done():
					return
				default:
					tx := repo.SafeTransaction()
					fileName := fmt.Sprintf("writer_%d_file_%d.txt", writerID, j)
					content := fmt.Sprintf("data from writer %d operation %d", writerID, j)
					
					err := tx.Add(fileName, []byte(content))
					if err != nil {
						continue // Skip this operation
					}
					
					err = tx.Validate()
					if err != nil {
						continue
					}
					
					_, err = tx.Commit(fmt.Sprintf("Writer %d commit %d", writerID, j))
					if err != nil {
						continue
					}
					
					time.Sleep(10 * time.Millisecond) // Small delay between operations
				}
			}
		}(i)
	}
	
	// Start reader goroutines
	wg.Add(numReaders)
	for i := 0; i < numReaders; i++ {
		go func(readerID int) {
			defer wg.Done()
			
			for j := 0; j < opsPerWorker*2; j++ { // More reads than writes
				select {
				case <-ctx.Done():
					return
				default:
					// Try to read files
					files, _ := repo.Repository.ListFiles()
					if len(files) > 0 {
						// Read a random file
						fileName := files[j%len(files)]
						_, _ = repo.Repository.ReadFile(fileName)
					}
					
					// Get status
					_, _ = repo.Repository.Status()
					
					time.Sleep(5 * time.Millisecond) // Frequent reads
				}
			}
		}(i)
	}
	
	// Wait for completion
	wg.Wait()
	
	// Verify system stability
	finalStats := repo.GetMemoryUsage()
	compactionStats := repo.GetCompactionStats()
	
	t.Logf("Concurrent test - Final stats: %+v", finalStats)
	t.Logf("Concurrent test - Compaction: %+v", compactionStats)
	
	// System should remain stable
	assert.Greater(t, compactionStats.TotalRuns, int64(0), "Compaction should have run during concurrent operations")
	assert.Equal(t, int64(0), compactionStats.ErrorCount, "No compaction errors should occur")
	
	// Memory should be reasonable
	expectedMaxObjects := int64(numWriters * opsPerWorker)
	assert.LessOrEqual(t, finalStats.TotalObjects, expectedMaxObjects*2, // Allow some overhead
		"Memory usage should remain reasonable under concurrent load")
}

// TestMemoryCompaction_LongRunningProcess tests compaction over extended periods
func TestMemoryCompaction_LongRunningProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running test in short mode")
	}
	
	baseRepo := NewRepository()
	repo := NewConcurrentSafeRepository(baseRepo)
	err := repo.Initialize(context.Background())
	require.NoError(t, err)
	
	// Enable conservative compaction for long-running test
	policy := CompactionPolicy{
		Enabled:            true,
		Interval:           500 * time.Millisecond,
		MemoryThreshold:    0.6,
		MinObjectAge:       100 * time.Millisecond,
		MaxObjectsPerCycle: 200,
	}
	
	err = repo.EnableAutoCompaction(policy)
	require.NoError(t, err)
	
	// Run for 30 seconds with continuous commits
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	var commitCount int64
	var wg sync.WaitGroup
	
	// Continuous commit goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		ticker := time.NewTicker(50 * time.Millisecond) // 20 commits/second
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				count := atomic.AddInt64(&commitCount, 1)
				
				tx := repo.SafeTransaction()
				fileName := fmt.Sprintf("long_running_%d.txt", count)
				content := fmt.Sprintf("long running commit %d at %v", count, time.Now())
				
				err := tx.Add(fileName, []byte(content))
				if err != nil {
					continue
				}
				
				err = tx.Validate()
				if err != nil {
					continue
				}
				
				_, err = tx.Commit(fmt.Sprintf("Long running commit %d", count))
				if err != nil {
					continue
				}
			}
		}
	}()
	
	// Memory monitoring goroutine
	memoryReadings := make([]int64, 0)
	var readingsMu sync.Mutex
	
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats := repo.GetMemoryUsage()
				readingsMu.Lock()
				memoryReadings = append(memoryReadings, stats.TotalObjects)
				readingsMu.Unlock()
				
				t.Logf("Memory reading: %d objects, %d bytes, %.2f fragment ratio",
					stats.TotalObjects, stats.TotalBytes, stats.FragmentRatio)
			}
		}
	}()
	
	wg.Wait()
	
	finalCommitCount := atomic.LoadInt64(&commitCount)
	finalStats := repo.GetMemoryUsage()
	compactionStats := repo.GetCompactionStats()
	
	t.Logf("Long-running test completed:")
	t.Logf("  Total commits: %d", finalCommitCount)
	t.Logf("  Final memory: %+v", finalStats)
	t.Logf("  Compaction stats: %+v", compactionStats)
	
	// Verify sustained operation
	assert.Greater(t, finalCommitCount, int64(400), "Should achieve sustained commit rate")
	assert.Greater(t, compactionStats.TotalRuns, int64(5), "Compaction should run regularly")
	assert.Equal(t, int64(0), compactionStats.ErrorCount, "No compaction errors in long run")
	
	// Verify memory stability - memory should not grow linearly with commits
	readingsMu.Lock()
	defer readingsMu.Unlock()
	
	if len(memoryReadings) > 5 {
		// Check that memory stabilized and didn't grow unboundedly
		firstReading := memoryReadings[2]  // Skip first few readings for stabilization
		lastReading := memoryReadings[len(memoryReadings)-1]
		
		// Memory growth should be sub-linear
		memoryGrowthRatio := float64(lastReading) / float64(firstReading)
		commitGrowthRatio := float64(finalCommitCount) / float64(firstReading)
		
		t.Logf("Memory growth ratio: %.2f, Commit growth ratio: %.2f", 
			memoryGrowthRatio, commitGrowthRatio)
		
		// Memory should grow much slower than commits due to compaction
		assert.Less(t, memoryGrowthRatio, commitGrowthRatio/2,
			"Memory growth should be sub-linear compared to commit growth")
	}
}