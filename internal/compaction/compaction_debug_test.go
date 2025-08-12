package compaction

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompaction_Debug tests to understand why objects aren't being collected
func TestCompaction_Debug(t *testing.T) {
	baseRepo := NewRepository()
	
	// Initialize GC if needed
	if baseRepo.gc == nil {
		baseRepo.gc = NewGarbageCollector(baseRepo.store)
	}
	
	t.Logf("Initial state:")
	t.Logf("  Reference counter size: %d", len(baseRepo.gc.refCounter))
	
	// Add some objects
	for i := 0; i < 5; i++ {
		objName := fmt.Sprintf("debug_object_%d", i)
		baseRepo.gc.AddReference(objName)
		t.Logf("  Added reference to %s, count: %d", objName, baseRepo.gc.GetReferenceCount(objName))
	}
	
	t.Logf("After adding references:")
	t.Logf("  Reference counter size: %d", len(baseRepo.gc.refCounter))
	
	// Remove some references
	for i := 0; i < 3; i++ {
		objName := fmt.Sprintf("debug_object_%d", i)
		baseRepo.gc.RemoveReference(objName)
		t.Logf("  Removed reference from %s, count: %d", objName, baseRepo.gc.GetReferenceCount(objName))
	}
	
	t.Logf("After removing references:")
	t.Logf("  Reference counter size: %d", len(baseRepo.gc.refCounter))
	
	// Check what candidates are identified
	candidates := baseRepo.gc.identifyGarbageObjects()
	t.Logf("Garbage collection candidates: %v", candidates)
	
	// Check fragmentation ratio
	fragmentRatio := baseRepo.gc.calculateFragmentRatio()
	t.Logf("Fragment ratio: %.3f", fragmentRatio)
	
	// Check if compaction should run
	shouldCompact := baseRepo.gc.shouldCompact()
	t.Logf("Should compact: %t", shouldCompact)
	
	// Force compaction and see what happens
	initialStats := baseRepo.gc.GetStats()
	t.Logf("Initial compaction stats: runs=%d, collected=%d", initialStats.TotalRuns, initialStats.ObjectsCollected)
	
	err := baseRepo.gc.ForceCompaction(CompactOptions{Force: true})
	require.NoError(t, err)
	
	finalStats := baseRepo.gc.GetStats()
	t.Logf("Final compaction stats: runs=%d, collected=%d", finalStats.TotalRuns, finalStats.ObjectsCollected)
	t.Logf("Final reference counter size: %d", len(baseRepo.gc.refCounter))
	
	// Verify at least some collection happened
	assert.Greater(t, finalStats.TotalRuns, initialStats.TotalRuns, "Should have run compaction")
}