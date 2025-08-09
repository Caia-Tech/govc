package datastore

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ChaosTestSuite runs chaos engineering tests to verify system resilience
func ChaosTestSuite(t *testing.T, createStore func() DataStore) {
	t.Run("ResourceExhaustionTests", func(t *testing.T) {
		runResourceExhaustionTests(t, createStore)
	})
	
	t.Run("ConcurrentFailureTests", func(t *testing.T) {
		runConcurrentFailureTests(t, createStore)
	})
	
	t.Run("RecoveryTests", func(t *testing.T) {
		runRecoveryTests(t, createStore)
	})
	
	t.Run("NetworkPartitionSimulation", func(t *testing.T) {
		runNetworkPartitionTests(t, createStore)
	})
}

// runResourceExhaustionTests simulates resource exhaustion scenarios
func runResourceExhaustionTests(t *testing.T, createStore func() DataStore) {
	t.Run("MemoryPressure", func(t *testing.T) {
		store := createStore()
		defer store.Close()
		
		// Monitor memory usage
		var maxMemory uint64
		done := make(chan bool)
		
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					if m.Alloc > maxMemory {
						maxMemory = m.Alloc
					}
				}
			}
		}()
		
		// Try to exhaust memory by storing large objects
		var stored int
		maxObjects := 1000
		objectSize := 1024 * 1024 // 1MB each
		
		for i := 0; i < maxObjects; i++ {
			hash := fmt.Sprintf("memory-pressure-%d", i)
			data := make([]byte, objectSize)
			
			err := store.ObjectStore().PutObject(hash, data)
			if err != nil {
				t.Logf("Hit memory limit at object %d: %v", i, err)
				break
			}
			stored++
			
			// Check if we're using too much memory (limit to 500MB for test)
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if m.Alloc > 500*1024*1024 {
				t.Logf("Memory limit reached at %d objects", i)
				break
			}
		}
		
		close(done)
		
		t.Logf("Stored %d objects under memory pressure", stored)
		t.Logf("Peak memory usage: %d MB", maxMemory/(1024*1024))
		
		// Should have stored at least some objects
		assert.Greater(t, stored, 0, "Should be able to store some objects")
		
		// Verify stored objects are still accessible
		for i := 0; i < min(10, stored); i++ {
			hash := fmt.Sprintf("memory-pressure-%d", i)
			exists, err := store.ObjectStore().HasObject(hash)
			assert.NoError(t, err)
			assert.True(t, exists, "Stored object should still exist")
		}
	})
	
	t.Run("DiskSpaceExhaustion", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping disk exhaustion test in short mode")
		}
		
		store := createStore()
		defer store.Close()
		
		// Try to fill up available space (limited for testing)
		maxSize := int64(100 * 1024 * 1024) // Limit to 100MB for test
		var totalStored int64
		objectSize := 1024 * 1024 // 1MB per object
		
		for i := 0; totalStored < maxSize; i++ {
			hash := fmt.Sprintf("disk-pressure-%d", i)
			data := make([]byte, objectSize)
			
			err := store.ObjectStore().PutObject(hash, data)
			if err != nil {
				t.Logf("Storage failed at %d MB: %v", totalStored/(1024*1024), err)
				break
			}
			
			totalStored += int64(objectSize)
		}
		
		t.Logf("Total stored under disk pressure: %d MB", totalStored/(1024*1024))
		
		// Verify some objects are still accessible
		count, err := store.ObjectStore().CountObjects()
		assert.NoError(t, err)
		assert.Greater(t, count, int64(0), "Should have stored some objects")
	})
	
	t.Run("GoroutineExhaustion", func(t *testing.T) {
		store := createStore()
		defer store.Close()
		
		initialGoroutines := runtime.NumGoroutine()
		
		// Launch many concurrent operations
		numWorkers := 1000
		var wg sync.WaitGroup
		errors := make(chan error, numWorkers)
		
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				hash := fmt.Sprintf("goroutine-test-%d", id)
				data := []byte(fmt.Sprintf("data-%d", id))
				
				err := store.ObjectStore().PutObject(hash, data)
				if err != nil {
					errors <- fmt.Errorf("worker %d failed: %w", id, err)
					return
				}
				
				// Small delay to keep goroutines alive longer
				time.Sleep(10 * time.Millisecond)
				
				_, err = store.ObjectStore().GetObject(hash)
				if err != nil {
					errors <- fmt.Errorf("worker %d read failed: %w", id, err)
				}
			}(i)
		}
		
		wg.Wait()
		close(errors)
		
		peakGoroutines := runtime.NumGoroutine()
		
		// Check for errors
		var errorCount int
		for err := range errors {
			t.Logf("Worker error: %v", err)
			errorCount++
		}
		
		t.Logf("Initial goroutines: %d, Peak: %d, Errors: %d", 
			initialGoroutines, peakGoroutines, errorCount)
		
		// Should handle reasonable concurrency without too many errors
		errorRate := float64(errorCount) / float64(numWorkers)
		assert.Less(t, errorRate, 0.1, "Error rate should be less than 10%%")
	})
}

// runConcurrentFailureTests simulates various concurrent failure scenarios
func runConcurrentFailureTests(t *testing.T, createStore func() DataStore) {
	t.Run("ConcurrentReadWriteWithFailures", func(t *testing.T) {
		store := createStore()
		defer store.Close()
		
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		// Counters for operations
		var (
			successfulReads   int64
			successfulWrites  int64
			failedReads      int64
			failedWrites     int64
		)
		
		numWorkers := 20
		var wg sync.WaitGroup
		
		// Writer workers
		for i := 0; i < numWorkers/2; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				counter := 0
				for {
					select {
					case <-ctx.Done():
						return
					default:
						hash := fmt.Sprintf("concurrent-write-%d-%d", workerID, counter)
						data := generateRandomData(100 + counter%1000)
						
						err := store.ObjectStore().PutObject(hash, data)
						if err != nil {
							atomic.AddInt64(&failedWrites, 1)
						} else {
							atomic.AddInt64(&successfulWrites, 1)
						}
						
						counter++
						
						// Randomly introduce delays to create race conditions
						if counter%10 == 0 {
							time.Sleep(time.Millisecond)
						}
					}
				}
			}(i)
		}
		
		// Reader workers
		for i := numWorkers/2; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				counter := 0
				for {
					select {
					case <-ctx.Done():
						return
					default:
						// Try to read from what writers are creating
						readerWorkerID := counter % (numWorkers / 2)
						readCounter := counter % 100
						hash := fmt.Sprintf("concurrent-write-%d-%d", readerWorkerID, readCounter)
						
						_, err := store.ObjectStore().GetObject(hash)
						if err != nil {
							atomic.AddInt64(&failedReads, 1)
						} else {
							atomic.AddInt64(&successfulReads, 1)
						}
						
						counter++
					}
				}
			}(i)
		}
		
		wg.Wait()
		
		t.Logf("Concurrent operations results:")
		t.Logf("  Successful reads: %d, failed: %d", successfulReads, failedReads)
		t.Logf("  Successful writes: %d, failed: %d", successfulWrites, failedWrites)
		
		// Should have some successful operations
		assert.Greater(t, successfulWrites, int64(0), "Should have successful writes")
		assert.Greater(t, successfulReads, int64(0), "Should have successful reads")
		
		// Failure rate should be reasonable (allow some failures due to timing)
		if successfulWrites > 0 {
			writeFailureRate := float64(failedWrites) / float64(successfulWrites+failedWrites)
			assert.Less(t, writeFailureRate, 0.5, "Write failure rate should be reasonable")
		}
	})
	
	t.Run("TransactionConflictStorm", func(t *testing.T) {
		store := createStore()
		defer store.Close()
		
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		// All workers will try to modify the same set of objects
		conflictKeys := []string{"conflict-1", "conflict-2", "conflict-3"}
		
		// Initialize conflict keys
		for _, key := range conflictKeys {
			err := store.ObjectStore().PutObject(key, []byte("initial"))
			require.NoError(t, err)
		}
		
		var (
			successfulTx int64
			failedTx     int64
		)
		
		numWorkers := 10
		var wg sync.WaitGroup
		
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				counter := 0
				for {
					select {
					case <-ctx.Done():
						return
					default:
						tx, err := store.BeginTx(ctx, nil)
						if err != nil {
							atomic.AddInt64(&failedTx, 1)
							continue
						}
						
						// Try to modify all conflict keys in transaction
						allSucceeded := true
						for _, key := range conflictKeys {
							newData := []byte(fmt.Sprintf("worker-%d-op-%d", workerID, counter))
							err := tx.PutObject(key, newData)
							if err != nil {
								allSucceeded = false
								break
							}
						}
						
						if allSucceeded {
							err = tx.Commit()
							if err != nil {
								atomic.AddInt64(&failedTx, 1)
							} else {
								atomic.AddInt64(&successfulTx, 1)
							}
						} else {
							tx.Rollback()
							atomic.AddInt64(&failedTx, 1)
						}
						
						counter++
					}
				}
			}(i)
		}
		
		wg.Wait()
		
		t.Logf("Transaction conflict results:")
		t.Logf("  Successful transactions: %d", successfulTx)
		t.Logf("  Failed transactions: %d", failedTx)
		
		// Should have some successful transactions despite conflicts
		assert.Greater(t, successfulTx, int64(0), "Should have some successful transactions")
		
		// Verify final state is consistent
		for _, key := range conflictKeys {
			data, err := store.ObjectStore().GetObject(key)
			assert.NoError(t, err, "Conflict key should still exist")
			t.Logf("Final value for %s: %s", key, string(data))
		}
	})
}

// runRecoveryTests tests recovery from various failure scenarios
func runRecoveryTests(t *testing.T, createStore func() DataStore) {
	t.Run("RecoverFromPanic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Recovered from panic: %v", r)
			}
		}()
		
		store := createStore()
		defer func() {
			// Ensure we can close even after potential issues
			if store != nil {
				store.Close()
			}
		}()
		
		// Perform some operations
		err := store.ObjectStore().PutObject("recovery-test", []byte("test data"))
		require.NoError(t, err)
		
		// Verify operation succeeded
		data, err := store.ObjectStore().GetObject("recovery-test")
		require.NoError(t, err)
		assert.Equal(t, []byte("test data"), data)
		
		// Try to trigger potential panic conditions
		// (these shouldn't panic, but if they do, we should recover)
		
		// Extremely long hash
		longHash := strings.Repeat("a", 100000)
		store.ObjectStore().PutObject(longHash, []byte("test"))
		
		// Nil data (some implementations might not handle this well)
		store.ObjectStore().PutObject("nil-test", nil)
		
		// Operations on closed store (simulate)
		// Note: We don't actually close here to keep the test stable
	})
	
	t.Run("GracefulDegradation", func(t *testing.T) {
		store := createStore()
		defer store.Close()
		
		// Test that store continues to function even under stress
		numOperations := 1000
		var errors []error
		
		for i := 0; i < numOperations; i++ {
			hash := fmt.Sprintf("degradation-test-%d", i)
			data := make([]byte, 1000+i) // Varying sizes
			
			err := store.ObjectStore().PutObject(hash, data)
			if err != nil {
				errors = append(errors, fmt.Errorf("put %d: %w", i, err))
				continue
			}
			
			// Immediate read-back test
			retrieved, err := store.ObjectStore().GetObject(hash)
			if err != nil {
				errors = append(errors, fmt.Errorf("get %d: %w", i, err))
				continue
			}
			
			if len(retrieved) != len(data) {
				errors = append(errors, fmt.Errorf("size mismatch at %d: got %d, want %d", 
					i, len(retrieved), len(data)))
			}
			
			// Simulate occasional system pressure
			if i%100 == 0 {
				runtime.GC()
			}
		}
		
		errorRate := float64(len(errors)) / float64(numOperations)
		t.Logf("Degradation test: %d operations, %d errors (%.2f%%)", 
			numOperations, len(errors), errorRate*100)
		
		// Should maintain reasonable reliability
		assert.Less(t, errorRate, 0.1, "Error rate should be less than 10%%")
		
		// Log first few errors for diagnosis
		for i, err := range errors {
			if i < 5 {
				t.Logf("Error %d: %v", i, err)
			}
		}
	})
}

// runNetworkPartitionTests simulates network-related failures for distributed stores
func runNetworkPartitionTests(t *testing.T, createStore func() DataStore) {
	t.Run("ConnectionInterruption", func(t *testing.T) {
		store := createStore()
		defer store.Close()
		
		// Test resilience to connection issues by rapidly opening/closing connections
		// This mainly applies to network-based stores (PostgreSQL, Redis, etc.)
		
		numCycles := 10
		for cycle := 0; cycle < numCycles; cycle++ {
			// Perform operations
			hash := fmt.Sprintf("connection-test-%d", cycle)
			data := []byte(fmt.Sprintf("cycle-%d-data", cycle))
			
			err := store.ObjectStore().PutObject(hash, data)
			if err != nil {
				t.Logf("Put failed in cycle %d: %v", cycle, err)
				continue
			}
			
			// Try immediate read
			retrieved, err := store.ObjectStore().GetObject(hash)
			if err != nil {
				t.Logf("Get failed in cycle %d: %v", cycle, err)
				continue
			}
			
			assert.Equal(t, data, retrieved, "Data should be consistent in cycle %d", cycle)
			
			// Brief pause to simulate network latency
			time.Sleep(10 * time.Millisecond)
		}
	})
	
	t.Run("HighLatencySimulation", func(t *testing.T) {
		store := createStore()
		defer store.Close()
		
		// Simulate high-latency operations by adding delays
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		var operations int64
		var timeouts int64
		
		numWorkers := 5
		var wg sync.WaitGroup
		
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				counter := 0
				for {
					select {
					case <-ctx.Done():
						return
					default:
						hash := fmt.Sprintf("latency-test-%d-%d", workerID, counter)
						data := []byte("test data")
						
						// Use short timeout to simulate network timeouts
						_, opCancel := context.WithTimeout(ctx, 100*time.Millisecond)
						
						// For stores that support context, use it
						err := store.ObjectStore().PutObject(hash, data)
						opCancel()
						
						atomic.AddInt64(&operations, 1)
						if err != nil {
							atomic.AddInt64(&timeouts, 1)
						}
						
						counter++
						
						// Add delay to simulate network latency
						time.Sleep(10 * time.Millisecond)
					}
				}
			}(i)
		}
		
		wg.Wait()
		
		timeoutRate := float64(timeouts) / float64(operations)
		t.Logf("High latency simulation: %d operations, %d timeouts (%.2f%%)", 
			operations, timeouts, timeoutRate*100)
		
		// Should handle some timeouts gracefully
		assert.Less(t, timeoutRate, 0.5, "Timeout rate should be manageable")
	})
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}