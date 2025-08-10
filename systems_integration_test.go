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

// TestIntegration_MemoryCompactionPubSubAtomic tests that Memory Compaction, Pub/Sub, and Atomic Operations work together
func TestIntegration_MemoryCompactionPubSubAtomic(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()

	// Start garbage collection
	repo.gc.Start(context.Background())
	defer repo.gc.Stop()

	var eventCount int64
	var commitHashes []string
	var mu sync.Mutex

	// Subscribe to events
	unsubscribe := repo.OnCommit(func(commitHash string, files []string) {
		atomic.AddInt64(&eventCount, 1)
		mu.Lock()
		commitHashes = append(commitHashes, commitHash)
		mu.Unlock()
	})
	defer unsubscribe()

	// Perform atomic operations that should trigger events and memory management
	numOperations := 100
	for i := 0; i < numOperations; i++ {
		filename := fmt.Sprintf("integration_test_%d.txt", i)
		content := fmt.Sprintf("Integration test content %d", i)
		message := fmt.Sprintf("Integration commit %d", i)

		// Use atomic operations
		_, err := repo.AtomicCreateFile(filename, []byte(content), message)
		require.NoError(t, err)

		// Every 10 operations, add some references for garbage collection
		if i%10 == 0 {
			objectHash := fmt.Sprintf("integration_object_%d", i)
			repo.gc.AddReference(objectHash)
			repo.gc.RemoveReference(objectHash) // This should make it a GC candidate
		}
	}

	// Wait for events to propagate
	time.Sleep(100 * time.Millisecond)

	// Verify events were received
	assert.Equal(t, int64(numOperations), atomic.LoadInt64(&eventCount))

	mu.Lock()
	assert.Equal(t, numOperations, len(commitHashes))
	mu.Unlock()

	// Verify transaction statistics
	stats := repo.GetTransactionStats()
	assert.Equal(t, int64(numOperations), stats.CommittedTransactions)
	assert.Equal(t, int64(0), stats.AbortedTransactions)

	// Verify event bus statistics
	eventBusStats := repo.eventBus.GetStats()
	assert.GreaterOrEqual(t, eventBusStats.TotalEvents, int64(numOperations))

	// Force garbage collection
	repo.gc.ForceCompaction(CompactOptions{})

	// Verify garbage collection stats
	gcStats := repo.gc.GetStats()
	assert.GreaterOrEqual(t, gcStats.TotalRuns, int64(1))

	t.Logf("Integration test completed successfully:")
	t.Logf("  Transactions: %d committed, %d aborted", stats.CommittedTransactions, stats.AbortedTransactions)
	t.Logf("  Events: %d published, %d delivered", eventBusStats.TotalEvents, eventBusStats.TotalDelivered)
	t.Logf("  Garbage Collection: %d runs, %d objects collected", gcStats.TotalRuns, gcStats.ObjectsCollected)
}

// TestIntegration_ConcurrentOperationsAllSystems tests concurrent operations across all systems
func TestIntegration_ConcurrentOperationsAllSystems(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()

	// Start garbage collection
	repo.gc.Start(context.Background())
	defer repo.gc.Stop()

	var totalEvents int64
	var totalTransactions int64
	numWorkers := 10
	operationsPerWorker := 50

	// Subscribe to events
	unsubscribe := repo.OnCommit(func(commitHash string, files []string) {
		atomic.AddInt64(&totalEvents, 1)
	})
	defer unsubscribe()

	var wg sync.WaitGroup

	// Launch concurrent workers
	for workerID := 0; workerID < numWorkers; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for i := 0; i < operationsPerWorker; i++ {
				// Use atomic multi-file operations
				updates := map[string][]byte{
					fmt.Sprintf("worker_%d_config_%d.json", id, i):   []byte(fmt.Sprintf(`{"worker": %d, "iteration": %d}`, id, i)),
					fmt.Sprintf("worker_%d_data_%d.txt", id, i):     []byte(fmt.Sprintf("Data from worker %d, iteration %d", id, i)),
					fmt.Sprintf("worker_%d_metadata_%d.yaml", id, i): []byte(fmt.Sprintf("worker: %d\niteration: %d", id, i)),
				}

				_, err := repo.AtomicMultiFileUpdate(updates, fmt.Sprintf("Worker %d operation %d", id, i))
				if err == nil {
					atomic.AddInt64(&totalTransactions, 1)
				}

				// Add some garbage collection work
				objectHash := fmt.Sprintf("worker_%d_object_%d", id, i)
				repo.gc.AddReference(objectHash)
				if i%5 == 0 {
					repo.gc.RemoveReference(objectHash) // Some objects become GC candidates
				}
			}
		}(workerID)
	}

	wg.Wait()

	// Wait for events to propagate
	time.Sleep(200 * time.Millisecond)

	// Force garbage collection
	repo.gc.ForceCompaction(CompactOptions{})

	// Verify results
	expectedTransactions := int64(numWorkers * operationsPerWorker)
	
	assert.Equal(t, expectedTransactions, atomic.LoadInt64(&totalTransactions))
	assert.GreaterOrEqual(t, atomic.LoadInt64(&totalEvents), expectedTransactions)

	// Verify all systems are functioning
	stats := repo.GetTransactionStats()
	assert.Equal(t, expectedTransactions, stats.CommittedTransactions)

	eventBusStats := repo.eventBus.GetStats()
	assert.GreaterOrEqual(t, eventBusStats.TotalEvents, expectedTransactions)
	assert.GreaterOrEqual(t, eventBusStats.TotalDelivered, expectedTransactions)

	gcStats := repo.gc.GetStats()
	assert.GreaterOrEqual(t, gcStats.TotalRuns, int64(1))

	t.Logf("Concurrent integration test completed:")
	t.Logf("  Workers: %d, Operations per worker: %d", numWorkers, operationsPerWorker)
	t.Logf("  Total transactions: %d", stats.CommittedTransactions)
	t.Logf("  Total events: %d published, %d delivered", eventBusStats.TotalEvents, eventBusStats.TotalDelivered)
	t.Logf("  GC runs: %d, objects collected: %d", gcStats.TotalRuns, gcStats.ObjectsCollected)
}

// TestIntegration_HighFrequencyWithAllSystems tests high-frequency operations with all systems active
func TestIntegration_HighFrequencyWithAllSystems(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()

	// Start garbage collection
	repo.gc.Start(context.Background())
	defer repo.gc.Stop()

	var eventCount int64

	// Subscribe to events
	unsubscribe := repo.OnCommit(func(commitHash string, files []string) {
		atomic.AddInt64(&eventCount, 1)
	})
	defer unsubscribe()

	// High frequency operations
	numOperations := 1000
	start := time.Now()

	for i := 0; i < numOperations; i++ {
		filename := fmt.Sprintf("high_freq_%d.txt", i)
		content := fmt.Sprintf("High frequency content %d", i)
		message := fmt.Sprintf("High freq commit %d", i)

		_, err := repo.AtomicCreateFile(filename, []byte(content), message)
		require.NoError(t, err)

		// Add some GC work
		if i%100 == 0 {
			objectHash := fmt.Sprintf("high_freq_object_%d", i)
			repo.gc.AddReference(objectHash)
			repo.gc.RemoveReference(objectHash)
		}
	}

	duration := time.Since(start)
	opsPerSecond := float64(numOperations) / duration.Seconds()

	// Wait for events to propagate
	time.Sleep(100 * time.Millisecond)

	// Force garbage collection
	repo.gc.ForceCompaction(CompactOptions{})

	// Verify performance and correctness
	assert.Greater(t, opsPerSecond, 100.0) // Should handle at least 100 ops/sec

	stats := repo.GetTransactionStats()
	assert.Equal(t, int64(numOperations), stats.CommittedTransactions)

	eventBusStats := repo.eventBus.GetStats()
	assert.GreaterOrEqual(t, eventBusStats.TotalEvents, int64(numOperations))
	assert.Equal(t, int64(numOperations), atomic.LoadInt64(&eventCount))

	gcStats := repo.gc.GetStats()
	assert.GreaterOrEqual(t, gcStats.TotalRuns, int64(1))

	t.Logf("High frequency integration test:")
	t.Logf("  Operations: %d in %v (%.2f ops/sec)", numOperations, duration, opsPerSecond)
	t.Logf("  All transactions committed successfully: %d", stats.CommittedTransactions)
	t.Logf("  All events delivered: %d", atomic.LoadInt64(&eventCount))
	t.Logf("  Garbage collection: %d runs", gcStats.TotalRuns)
}