package compaction

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompaction_BasicIntegration tests basic compaction integration with repository
func TestCompaction_BasicIntegration(t *testing.T) {
	// Create repository and initialize garbage collector
	baseRepo := NewRepository()
	_ = NewConcurrentSafeRepository(baseRepo) // Create but don't use for this test
	
	// Initialize the repository's garbage collector
	if baseRepo.gc == nil {
		baseRepo.gc = NewGarbageCollector(baseRepo.store)
	}
	
	// Add some objects to the store
	for i := 0; i < 10; i++ {
		objName := fmt.Sprintf("test_object_%d", i)
		baseRepo.gc.AddReference(objName)
	}
	
	// Verify objects are tracked
	_ = baseRepo.gc.GetMemoryUsage() // Get stats but don't use variable
	assert.Equal(t, int64(10), int64(len(baseRepo.gc.refCounter)))
	
	// Remove some references to make them eligible for collection
	for i := 0; i < 5; i++ {
		objName := fmt.Sprintf("test_object_%d", i)
		baseRepo.gc.RemoveReference(objName)
	}
	
	// Force compaction
	err := baseRepo.gc.ForceCompaction(CompactOptions{Force: true})
	assert.NoError(t, err)
	
	// Check that compaction ran
	compactionStats := baseRepo.gc.GetStats()
	assert.Greater(t, compactionStats.TotalRuns, int64(0))
	
	// Should have only 5 referenced objects left
	assert.Equal(t, int64(5), int64(len(baseRepo.gc.refCounter)))
}

// TestCompaction_AutomaticCompaction tests automatic compaction background process
func TestCompaction_AutomaticCompaction(t *testing.T) {
	baseRepo := NewRepository()
	
	// Enable automatic compaction with aggressive settings for testing
	policy := CompactionPolicy{
		Enabled:            true,
		Interval:           100 * time.Millisecond,
		MemoryThreshold:    0.3, // Trigger at 30% fragmentation
		MinObjectAge:       10 * time.Millisecond,
		MaxObjectsPerCycle: 100,
	}
	
	err := baseRepo.EnableAutoCompaction(policy)
	require.NoError(t, err)
	
	// Add objects repeatedly to trigger automatic compaction
	for round := 0; round < 3; round++ {
		// Add 10 objects
		for i := 0; i < 10; i++ {
			objName := fmt.Sprintf("round_%d_object_%d", round, i)
			baseRepo.gc.AddReference(objName)
		}
		
		// Remove half to create fragmentation
		for i := 0; i < 5; i++ {
			objName := fmt.Sprintf("round_%d_object_%d", round, i)
			baseRepo.gc.RemoveReference(objName)
		}
		
		// Wait for automatic compaction to trigger
		time.Sleep(150 * time.Millisecond)
	}
	
	// Check that automatic compaction has run
	compactionStats := baseRepo.gc.GetStats()
	assert.Greater(t, compactionStats.TotalRuns, int64(0), "Automatic compaction should have run")
	
	// Cleanup
	err = baseRepo.gc.Stop()
	assert.NoError(t, err)
}

// TestCompaction_MemoryGrowthPrevention tests that compaction prevents unbounded memory growth
func TestCompaction_MemoryGrowthPrevention(t *testing.T) {
	baseRepo := NewRepository()
	
	// Enable compaction
	policy := CompactionPolicy{
		Enabled:            true,
		Interval:           50 * time.Millisecond,
		MemoryThreshold:    0.4,
		MinObjectAge:       20 * time.Millisecond,
		MaxObjectsPerCycle: 50,
	}
	
	err := baseRepo.EnableAutoCompaction(policy)
	require.NoError(t, err)
	
	initialStats := baseRepo.gc.GetMemoryUsage()
	
	// Simulate high-frequency object creation and deletion
	for cycle := 0; cycle < 10; cycle++ {
		// Create 20 objects
		for i := 0; i < 20; i++ {
			objName := fmt.Sprintf("cycle_%d_obj_%d", cycle, i)
			baseRepo.gc.AddReference(objName)
		}
		
		// Delete 15 objects (create some fragmentation)
		for i := 0; i < 15; i++ {
			objName := fmt.Sprintf("cycle_%d_obj_%d", cycle, i)
			baseRepo.gc.RemoveReference(objName)
		}
		
		// Small delay to allow compaction
		time.Sleep(60 * time.Millisecond)
	}
	
	// Wait for final compaction
	time.Sleep(100 * time.Millisecond)
	
	_ = baseRepo.gc.GetMemoryUsage() // Get stats but don't use variable
	compactionStats := baseRepo.gc.GetStats()
	
	t.Logf("Initial objects: %d, Final reference count: %d", initialStats.TotalObjects, int64(len(baseRepo.gc.refCounter)))
	t.Logf("Compaction runs: %d, Objects collected: %d", compactionStats.TotalRuns, compactionStats.ObjectsCollected)
	
	// Memory should be bounded (not grow unboundedly)
	// With 10 cycles * 5 net objects per cycle = 50 objects maximum expected
	assert.LessOrEqual(t, int64(len(baseRepo.gc.refCounter)), int64(60), 
		"Reference counter should be bounded by compaction")
	
	// Compaction should have run multiple times
	assert.Greater(t, compactionStats.TotalRuns, int64(3), "Multiple compaction cycles should have run")
	assert.Greater(t, compactionStats.ObjectsCollected, int64(50), "Should have collected many objects")
	
	// Cleanup
	err = baseRepo.gc.Stop()
	assert.NoError(t, err)
}

// TestCompaction_ConcurrentAccess tests compaction with concurrent reference operations
func TestCompaction_ConcurrentAccess(t *testing.T) {
	baseRepo := NewRepository()
	
	// Enable compaction
	policy := CompactionPolicy{
		Enabled:            true,
		Interval:           30 * time.Millisecond,
		MemoryThreshold:    0.3,
		MinObjectAge:       10 * time.Millisecond,
		MaxObjectsPerCycle: 25,
	}
	
	err := baseRepo.EnableAutoCompaction(policy)
	require.NoError(t, err)
	
	// Run concurrent operations for 1 second
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	
	// Start reference adding goroutines
	for i := 0; i < 5; i++ {
		go func(workerID int) {
			counter := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					objName := fmt.Sprintf("worker_%d_obj_%d", workerID, counter)
					baseRepo.gc.AddReference(objName)
					counter++
					time.Sleep(5 * time.Millisecond)
				}
			}
		}(i)
	}
	
	// Start reference removing goroutines
	for i := 0; i < 3; i++ {
		go func(workerID int) {
			counter := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// Remove objects from previous iterations
					if counter > 10 {
						objName := fmt.Sprintf("worker_%d_obj_%d", workerID, counter-10)
						baseRepo.gc.RemoveReference(objName)
					}
					counter++
					time.Sleep(8 * time.Millisecond)
				}
			}
		}(i)
	}
	
	// Wait for test completion
	<-ctx.Done()
	
	// Wait for final compaction
	time.Sleep(100 * time.Millisecond)
	
	// Verify system stability
	_ = baseRepo.gc.GetMemoryUsage() // Get stats but don't use variable
	compactionStats := baseRepo.gc.GetStats()
	
	t.Logf("Concurrent test results:")
	t.Logf("  Final reference count: %d", len(baseRepo.gc.refCounter))
	t.Logf("  Compaction runs: %d", compactionStats.TotalRuns)
	t.Logf("  Objects collected: %d", compactionStats.ObjectsCollected)
	t.Logf("  Error count: %d", compactionStats.ErrorCount)
	
	// Should have run compaction multiple times without errors
	assert.Greater(t, compactionStats.TotalRuns, int64(5), "Should run multiple compactions")
	assert.Equal(t, int64(0), compactionStats.ErrorCount, "No errors should occur during concurrent access")
	
	// Memory should be reasonable (not unbounded growth)
	// With 5 writers * 50 ops + some overhead from timing, 800 is a reasonable upper bound
	assert.LessOrEqual(t, int64(len(baseRepo.gc.refCounter)), int64(800), 
		"Memory should remain bounded under concurrent access")
	
	// Cleanup
	err = baseRepo.gc.Stop()
	assert.NoError(t, err)
}