package memory

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/Caia-Tech/govc/datastore"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	config := datastore.Config{Type: datastore.TypeMemory}
	store := New(config)
	err := store.Initialize(config)
	require.NoError(t, err)
	defer store.Close()

	t.Run("LightStress", func(t *testing.T) {
		stressConfig := datastore.StressTestConfig{
			Duration:           5 * time.Second,
			NumWorkers:         runtime.NumCPU(),
			OperationsPerSec:   50,
			MaxObjectSize:      1024,
			MinObjectSize:      100,
			ReadWriteRatio:     0.7,
			TransactionPercent: 0.3,
		}

		metrics := datastore.StressTestSuite(t, store, stressConfig)
		datastore.ValidateStressTestResults(t, metrics, stressConfig)
	})

	t.Run("HighConcurrency", func(t *testing.T) {
		stressConfig := datastore.StressTestConfig{
			Duration:           10 * time.Second,
			NumWorkers:         runtime.NumCPU() * 4,
			OperationsPerSec:   100,
			MaxObjectSize:      512,
			MinObjectSize:      50,
			ReadWriteRatio:     0.8,
			TransactionPercent: 0.1,
		}

		metrics := datastore.StressTestSuite(t, store, stressConfig)
		datastore.ValidateStressTestResults(t, metrics, stressConfig)
	})

	t.Run("LargeObjects", func(t *testing.T) {
		stressConfig := datastore.StressTestConfig{
			Duration:           8 * time.Second,
			NumWorkers:         2,
			OperationsPerSec:   10,
			MaxObjectSize:      1024 * 1024, // 1MB
			MinObjectSize:      512 * 1024,  // 512KB
			ReadWriteRatio:     0.5,
			TransactionPercent: 0.2,
		}

		metrics := datastore.StressTestSuite(t, store, stressConfig)
		datastore.ValidateStressTestResults(t, metrics, stressConfig)
	})

	t.Run("TransactionHeavy", func(t *testing.T) {
		stressConfig := datastore.StressTestConfig{
			Duration:           6 * time.Second,
			NumWorkers:         runtime.NumCPU(),
			OperationsPerSec:   30,
			MaxObjectSize:      2048,
			MinObjectSize:      100,
			ReadWriteRatio:     0.6,
			TransactionPercent: 0.8, // 80% of operations in transactions
		}

		metrics := datastore.StressTestSuite(t, store, stressConfig)
		datastore.ValidateStressTestResults(t, metrics, stressConfig)
	})
}

func TestMemoryStore_MemoryLeakTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}

	config := datastore.Config{Type: datastore.TypeMemory}

	// Run multiple cycles to detect memory leaks
	for cycle := 0; cycle < 5; cycle++ {
		t.Run(fmt.Sprintf("Cycle%d", cycle), func(t *testing.T) {
			var m1, m2 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m1)

			// Create store and perform operations
			store := New(config)
			err := store.Initialize(config)
			require.NoError(t, err)

			// Perform many operations
			for i := 0; i < 10000; i++ {
				data := make([]byte, 1024)
				err := store.ObjectStore().PutObject(fmt.Sprintf("leak-test-%d", i), data)
				require.NoError(t, err)
			}

			// Close store
			store.Close()

			// Force garbage collection
			runtime.GC()
			runtime.GC() // Call twice to ensure cleanup
			runtime.ReadMemStats(&m2)

			memoryGrowth := m2.Alloc - m1.Alloc
			t.Logf("Memory growth in cycle %d: %d bytes", cycle, memoryGrowth)

			// Memory growth should be reasonable (less than 100MB per cycle)
			require.Less(t, memoryGrowth, uint64(100*1024*1024),
				"Excessive memory growth detected, possible memory leak")
		})
	}
}

func TestMemoryStore_ResourceExhaustion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resource exhaustion test in short mode")
	}

	config := datastore.Config{Type: datastore.TypeMemory}
	store := New(config)
	err := store.Initialize(config)
	require.NoError(t, err)
	defer store.Close()

	t.Run("MaxObjectsTest", func(t *testing.T) {
		// Try to store many objects until we hit limits
		maxObjects := 100000
		var lastError error

		for i := 0; i < maxObjects; i++ {
			hash := fmt.Sprintf("exhaust-test-%d", i)
			data := make([]byte, 1024) // 1KB each
			err := store.ObjectStore().PutObject(hash, data)

			if err != nil {
				lastError = err
				t.Logf("Hit limit at %d objects: %v", i, err)
				break
			}

			// Log progress every 10k objects
			if i%10000 == 0 && i > 0 {
				t.Logf("Stored %d objects successfully", i)
			}
		}

		// Should be able to store a reasonable number of objects
		count, err := store.ObjectStore().CountObjects()
		require.NoError(t, err)
		require.Greater(t, count, int64(50000), "Should be able to store at least 50k objects")

		if lastError != nil {
			t.Logf("Final error: %v", lastError)
		}
	})
}

func BenchmarkMemoryStore_Operations(b *testing.B) {
	config := datastore.Config{Type: datastore.TypeMemory}
	store := New(config)
	err := store.Initialize(config)
	require.NoError(b, err)
	defer store.Close()

	// Pre-populate with test data
	for i := 0; i < 1000; i++ {
		hash := fmt.Sprintf("bench-data-%d", i)
		data := make([]byte, 512)
		err := store.ObjectStore().PutObject(hash, data)
		require.NoError(b, err)
	}

	b.Run("PutObject", func(b *testing.B) {
		data := make([]byte, 512)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash := fmt.Sprintf("bench-put-%d", i)
			store.ObjectStore().PutObject(hash, data)
		}
	})

	b.Run("GetObject", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash := fmt.Sprintf("bench-data-%d", i%1000)
			store.ObjectStore().GetObject(hash)
		}
	})

	b.Run("HasObject", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash := fmt.Sprintf("bench-data-%d", i%1000)
			store.ObjectStore().HasObject(hash)
		}
	})

	b.Run("DeleteObject", func(b *testing.B) {
		// Prepare data for deletion
		for i := 0; i < b.N; i++ {
			hash := fmt.Sprintf("bench-delete-%d", i)
			data := make([]byte, 512)
			store.ObjectStore().PutObject(hash, data)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash := fmt.Sprintf("bench-delete-%d", i)
			store.ObjectStore().DeleteObject(hash)
		}
	})
}