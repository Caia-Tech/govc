package compaction

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/Caia-Tech/govc/pkg/storage"
)

// TestGarbageCollector_BasicCompaction tests basic compaction functionality
func TestGarbageCollector_BasicCompaction(t *testing.T) {
	store := storage.NewStore(storage.NewMemoryBackend())
	gc := NewGarbageCollector(store)
	
	// Add some objects with references
	gc.AddReference("obj1")
	gc.AddReference("obj2")
	gc.AddReference("obj1") // obj1 now has 2 references
	
	// Verify reference counts
	assert.Equal(t, int64(2), gc.GetReferenceCount("obj1"))
	assert.Equal(t, int64(1), gc.GetReferenceCount("obj2"))
	assert.Equal(t, int64(0), gc.GetReferenceCount("nonexistent"))
	
	// Remove one reference from obj1
	gc.RemoveReference("obj1")
	assert.Equal(t, int64(1), gc.GetReferenceCount("obj1"))
	
	// Remove all references
	gc.RemoveReference("obj1")
	gc.RemoveReference("obj2")
	
	// Both should be gone
	assert.Equal(t, int64(0), gc.GetReferenceCount("obj1"))
	assert.Equal(t, int64(0), gc.GetReferenceCount("obj2"))
}

// TestGarbageCollector_ReferenceCounting tests reference counting edge cases
func TestGarbageCollector_ReferenceCounting(t *testing.T) {
	store := storage.NewStore(storage.NewMemoryBackend())
	gc := NewGarbageCollector(store)
	
	// Test adding multiple references to same object
	for i := 0; i < 5; i++ {
		gc.AddReference("test_obj")
	}
	assert.Equal(t, int64(5), gc.GetReferenceCount("test_obj"))
	
	// Test removing more references than exist
	for i := 0; i < 10; i++ {
		gc.RemoveReference("test_obj")
	}
	assert.Equal(t, int64(0), gc.GetReferenceCount("test_obj"))
	
	// Test removing non-existent reference
	gc.RemoveReference("nonexistent")
	assert.Equal(t, int64(0), gc.GetReferenceCount("nonexistent"))
}

// TestGarbageCollector_PolicyConfiguration tests policy configuration
func TestGarbageCollector_PolicyConfiguration(t *testing.T) {
	store := storage.NewStore(storage.NewMemoryBackend())
	gc := NewGarbageCollector(store)
	
	// Test default policy
	defaultPolicy := DefaultCompactionPolicy()
	assert.True(t, defaultPolicy.Enabled)
	assert.Equal(t, 5*time.Minute, defaultPolicy.Interval)
	assert.Equal(t, 0.8, defaultPolicy.MemoryThreshold)
	
	// Test setting custom policy
	customPolicy := CompactionPolicy{
		Enabled:            false,
		Interval:           10 * time.Minute,
		MemoryThreshold:    0.9,
		MinObjectAge:       30 * time.Minute,
		MaxObjectsPerCycle: 500,
	}
	
	gc.SetPolicy(customPolicy)
	// We can't directly access the policy, but we can test that it was set
	// by checking if compaction is disabled
	assert.False(t, gc.shouldCompact())
}

// TestGarbageCollector_ConcurrentAccess tests concurrent access to reference counter
func TestGarbageCollector_ConcurrentAccess(t *testing.T) {
	store := storage.NewStore(storage.NewMemoryBackend())
	gc := NewGarbageCollector(store)
	
	var wg sync.WaitGroup
	numGoroutines := 100
	referencesPerGoroutine := 100
	
	// Concurrently add references
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < referencesPerGoroutine; j++ {
				gc.AddReference("concurrent_obj")
			}
		}(i)
	}
	wg.Wait()
	
	// Should have exactly numGoroutines * referencesPerGoroutine references
	expectedRefs := int64(numGoroutines * referencesPerGoroutine)
	assert.Equal(t, expectedRefs, gc.GetReferenceCount("concurrent_obj"))
	
	// Concurrently remove references
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < referencesPerGoroutine; j++ {
				gc.RemoveReference("concurrent_obj")
			}
		}(i)
	}
	wg.Wait()
	
	// Should be back to 0
	assert.Equal(t, int64(0), gc.GetReferenceCount("concurrent_obj"))
}

// TestGarbageCollector_EmptyRepository tests compaction on empty repository
func TestGarbageCollector_EmptyRepository(t *testing.T) {
	store := storage.NewStore(storage.NewMemoryBackend())
	gc := NewGarbageCollector(store)
	
	// Test compaction with no objects
	err := gc.ForceCompaction(CompactOptions{Force: true})
	assert.NoError(t, err)
	
	// Test memory stats with empty repo
	stats := gc.GetMemoryUsage()
	assert.Equal(t, int64(0), stats.TotalObjects)
	assert.Equal(t, int64(0), stats.TotalBytes)
	assert.Equal(t, 0.0, stats.FragmentRatio)
}

// TestGarbageCollector_MemoryStats tests memory statistics calculation
func TestGarbageCollector_MemoryStats(t *testing.T) {
	store := storage.NewStore(storage.NewMemoryBackend())
	gc := NewGarbageCollector(store)
	
	// Add some objects with different reference states
	gc.AddReference("referenced_obj")
	gc.refCounter["unreferenced_obj"] = 0 // Simulate unreferenced object
	
	stats := gc.GetMemoryUsage()
	
	// Should have 2 total objects
	assert.Equal(t, int64(2), stats.TotalObjects)
	
	// Fragment ratio should be 0.5 (1 of 2 objects unreferenced)
	assert.Equal(t, 0.5, stats.FragmentRatio)
}

// TestGarbageCollector_StartStop tests start/stop functionality
func TestGarbageCollector_StartStop(t *testing.T) {
	store := storage.NewStore(storage.NewMemoryBackend())
	gc := NewGarbageCollector(store)
	
	// Test starting GC
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	err := gc.Start(ctx)
	assert.NoError(t, err)
	
	// Test starting already running GC
	err = gc.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
	
	// Test stopping GC
	err = gc.Stop()
	assert.NoError(t, err)
	
	// Test stopping already stopped GC
	err = gc.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

// TestGarbageCollector_ForceCompaction tests force compaction
func TestGarbageCollector_ForceCompaction(t *testing.T) {
	store := storage.NewStore(storage.NewMemoryBackend())
	gc := NewGarbageCollector(store)
	
	// Add some unreferenced objects
	gc.refCounter["obj1"] = 0
	gc.refCounter["obj2"] = 0
	gc.refCounter["obj3"] = 1 // This one is referenced
	
	initialStats := gc.GetStats()
	
	// Force compaction
	err := gc.ForceCompaction(CompactOptions{Force: true})
	assert.NoError(t, err)
	
	// Check that stats were updated
	newStats := gc.GetStats()
	assert.Greater(t, newStats.TotalRuns, initialStats.TotalRuns)
	
	// Referenced object should still exist
	assert.Equal(t, int64(1), gc.GetReferenceCount("obj3"))
}

// TestGarbageCollector_CompactionStats tests compaction statistics tracking
func TestGarbageCollector_CompactionStats(t *testing.T) {
	store := storage.NewStore(storage.NewMemoryBackend())
	gc := NewGarbageCollector(store)
	
	initialStats := gc.GetStats()
	assert.Equal(t, int64(0), initialStats.TotalRuns)
	assert.Equal(t, int64(0), initialStats.ObjectsCollected)
	assert.Equal(t, int64(0), initialStats.BytesFreed)
	
	// Perform compaction
	gc.refCounter["unreferenced"] = 0
	err := gc.ForceCompaction(CompactOptions{})
	assert.NoError(t, err)
	
	newStats := gc.GetStats()
	assert.Equal(t, int64(1), newStats.TotalRuns)
	assert.Greater(t, newStats.LastRunTime, time.Duration(0))
}

// TestGarbageCollector_CorruptedReferences tests handling of corrupted reference data
func TestGarbageCollector_CorruptedReferences(t *testing.T) {
	store := storage.NewStore(storage.NewMemoryBackend())
	gc := NewGarbageCollector(store)
	
	// Simulate corrupted reference counter with negative values
	gc.mu.Lock()
	gc.refCounter["corrupted_obj"] = -5
	gc.mu.Unlock()
	
	// GetReferenceCount should handle this gracefully
	count := gc.GetReferenceCount("corrupted_obj")
	assert.Equal(t, int64(-5), count) // Returns actual value, let caller handle
	
	// Compaction should still work
	err := gc.ForceCompaction(CompactOptions{Force: true})
	assert.NoError(t, err)
}

// TestGarbageCollector_HighFrequencyOperations tests performance under high frequency
func TestGarbageCollector_HighFrequencyOperations(t *testing.T) {
	store := storage.NewStore(storage.NewMemoryBackend())
	gc := NewGarbageCollector(store)
	
	// Simulate high-frequency reference operations
	startTime := time.Now()
	operations := 10000
	
	for i := 0; i < operations; i++ {
		objectID := "obj_" + string(rune(i%100)) // 100 different objects
		gc.AddReference(objectID)
		if i%2 == 0 {
			gc.RemoveReference(objectID)
		}
	}
	
	duration := time.Since(startTime)
	opsPerSecond := float64(operations) / duration.Seconds()
	
	// Should handle at least 100K operations per second
	assert.Greater(t, opsPerSecond, 100000.0, "Reference operations too slow: %.0f ops/sec", opsPerSecond)
	
	// Memory usage should be reasonable
	stats := gc.GetMemoryUsage()
	assert.LessOrEqual(t, stats.TotalObjects, int64(100)) // Should not exceed 100 objects
}

