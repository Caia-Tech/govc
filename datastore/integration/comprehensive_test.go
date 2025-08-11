package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Caia-Tech/govc/datastore"
	"github.com/Caia-Tech/govc/datastore/badger"
	"github.com/Caia-Tech/govc/datastore/memory"
	"github.com/Caia-Tech/govc/datastore/postgres"
	"github.com/Caia-Tech/govc/datastore/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllStoresComprehensive runs the complete comprehensive test suite on all available stores
func TestAllStoresComprehensive(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping comprehensive tests in short mode")
	}

	stores := createTestStores(t)
	defer cleanupTestStores(stores)

	for storeName, store := range stores {
		t.Run(storeName, func(t *testing.T) {
			t.Parallel() // Run store tests in parallel
			runComprehensiveTestSuite(t, store, storeName)
		})
	}

	// Run integration tests that require multiple stores
	if len(stores) > 1 {
		t.Run("Integration", func(t *testing.T) {
			IntegrationTestSuite(t)
		})
	}
}

// createTestStores creates all available datastore instances for testing
func createTestStores(t *testing.T) map[string]datastore.DataStore {
	stores := make(map[string]datastore.DataStore)
	tempDir := t.TempDir()

	// Memory Store - Always available
	memConfig := datastore.Config{Type: datastore.TypeMemory}
	memStore := memory.New(memConfig)
	if err := memStore.Initialize(memConfig); err == nil {
		stores["Memory"] = memStore
		t.Logf("✓ Memory store created successfully")
	} else {
		t.Errorf("Failed to create Memory store: %v", err)
	}

	// SQLite Store - Should always work
	sqliteConfig := datastore.Config{
		Type:       datastore.TypeSQLite,
		Connection: filepath.Join(tempDir, "comprehensive_test.db"),
	}
	sqliteStore, err := sqlite.New(sqliteConfig)
	if err == nil {
		if err := sqliteStore.Initialize(sqliteConfig); err == nil {
			stores["SQLite"] = sqliteStore
			t.Logf("✓ SQLite store created successfully")
		} else {
			t.Errorf("Failed to initialize SQLite store: %v", err)
		}
	} else {
		t.Errorf("Failed to create SQLite store: %v", err)
	}

	// BadgerDB Store
	badgerConfig := datastore.Config{
		Type:       datastore.TypeBadger,
		Connection: filepath.Join(tempDir, "comprehensive_badger"),
	}
	badgerStore, err := badger.New(badgerConfig)
	if err == nil {
		if err := badgerStore.Initialize(badgerConfig); err == nil {
			stores["BadgerDB"] = badgerStore
			t.Logf("✓ BadgerDB store created successfully")
		} else {
			t.Logf("⚠ Failed to initialize BadgerDB store: %v", err)
		}
	} else {
		t.Logf("⚠ Failed to create BadgerDB store: %v", err)
	}

	// PostgreSQL Store - Optional
	pgURL := os.Getenv("POSTGRES_TEST_URL")
	if pgURL == "" {
		pgURL = "postgres://postgres:postgres@localhost:5432/govc_comprehensive_test?sslmode=disable"
	}

	pgConfig := datastore.Config{
		Type:               datastore.TypePostgres,
		Connection:         pgURL,
		MaxConnections:     5,
		MaxIdleConnections: 2,
		ConnectionTimeout:  10 * time.Second,
	}

	pgStore, err := postgres.New(pgConfig)
	if err == nil {
		if err := pgStore.Initialize(pgConfig); err == nil {
			stores["PostgreSQL"] = pgStore
			t.Logf("✓ PostgreSQL store created successfully")
		} else {
			t.Logf("⚠ PostgreSQL not available: %v", err)
		}
	} else {
		t.Logf("⚠ Failed to create PostgreSQL store: %v", err)
	}

	t.Logf("Created %d stores for comprehensive testing", len(stores))
	return stores
}

// cleanupTestStores closes all test stores
func cleanupTestStores(stores map[string]datastore.DataStore) {
	for name, store := range stores {
		if store != nil {
			store.Close()
		}
		_ = name // Avoid unused variable
	}
}

// runComprehensiveTestSuite runs all test categories for a single store
func runComprehensiveTestSuite(t *testing.T, store datastore.DataStore, storeName string) {
	t.Logf("🧪 Running comprehensive test suite for %s", storeName)

	// 1. Basic compliance tests (already covered in unit tests)
	t.Run("BasicCompliance", func(t *testing.T) {
		// Basic compliance testing - just verify store works
		err := store.ObjectStore().PutObject("compliance-test", []byte("test data"))
		require.NoError(t, err)
		
		data, err := store.ObjectStore().GetObject("compliance-test")
		require.NoError(t, err)
		assert.Equal(t, []byte("test data"), data)
	})

	// 2. Fuzzing and property-based tests
	t.Run("FuzzTests", func(t *testing.T) {
		datastore.FuzzTestSuite(t, store)
	})

	// 3. Security penetration tests
	t.Run("SecurityTests", func(t *testing.T) {
		datastore.SecurityTestSuite(t, store)
	})

	// 4. Stress tests
	t.Run("StressTests", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping stress tests in short mode")
		}
		
		stressConfig := datastore.DefaultStressConfig()
		// Adjust for comprehensive testing
		stressConfig.Duration = 30 * time.Second
		stressConfig.NumWorkers = min(8, runtime.NumCPU()*2)
		stressConfig.OperationsPerSec = 200
		
		metrics := datastore.StressTestSuite(t, store, stressConfig)
		datastore.ValidateStressTestResults(t, metrics, stressConfig)
	})

	// 5. Chaos engineering tests
	t.Run("ChaosTests", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping chaos tests in short mode")
		}
		
		createStoreFunc := func() datastore.DataStore {
			// For chaos tests, we need to create fresh instances
			return createFreshStore(t, storeName)
		}
		datastore.ChaosTestSuite(t, createStoreFunc)
	})

	// 6. Edge case and boundary tests
	t.Run("EdgeCases", func(t *testing.T) {
		runEdgeCaseTests(t, store)
	})

	// 7. Performance regression tests
	t.Run("PerformanceRegression", func(t *testing.T) {
		runPerformanceRegressionTests(t, store, storeName)
	})

	// 8. Resource usage tests
	t.Run("ResourceUsage", func(t *testing.T) {
		runResourceUsageTests(t, store)
	})

	// 9. Data integrity verification
	t.Run("DataIntegrity", func(t *testing.T) {
		runDataIntegrityTests(t, store)
	})

	// 10. Concurrency edge cases
	t.Run("ConcurrencyEdgeCases", func(t *testing.T) {
		runConcurrencyEdgeCaseTests(t, store)
	})

	t.Logf("✅ Comprehensive test suite completed for %s", storeName)
}

// createFreshStore creates a new store instance for testing
func createFreshStore(t *testing.T, storeName string) datastore.DataStore {
	tempDir := t.TempDir()
	
	switch storeName {
	case "Memory":
		config := datastore.Config{Type: datastore.TypeMemory}
		store := memory.New(config)
		err := store.Initialize(config)
		require.NoError(t, err)
		return store
		
	case "SQLite":
		config := datastore.Config{
			Type:       datastore.TypeSQLite,
			Connection: filepath.Join(tempDir, "chaos_test.db"),
		}
		store, err := sqlite.New(config)
		require.NoError(t, err)
		err = store.Initialize(config)
		require.NoError(t, err)
		return store
		
	case "BadgerDB":
		config := datastore.Config{
			Type:       datastore.TypeBadger,
			Connection: filepath.Join(tempDir, "chaos_badger"),
		}
		store, err := badger.New(config)
		require.NoError(t, err)
		err = store.Initialize(config)
		require.NoError(t, err)
		return store
		
	case "PostgreSQL":
		config := datastore.Config{
			Type:               datastore.TypePostgres,
			Connection:         os.Getenv("POSTGRES_TEST_URL"),
			MaxConnections:     5,
			MaxIdleConnections: 2,
			ConnectionTimeout:  10 * time.Second,
		}
		if config.Connection == "" {
			config.Connection = "postgres://postgres:postgres@localhost:5432/govc_chaos_test?sslmode=disable"
		}
		store, err := postgres.New(config)
		require.NoError(t, err)
		err = store.Initialize(config)
		require.NoError(t, err)
		return store
		
	default:
		t.Fatalf("Unknown store type: %s", storeName)
		return nil
	}
}

// runEdgeCaseTests tests various edge cases not covered elsewhere
func runEdgeCaseTests(t *testing.T, store datastore.DataStore) {
	t.Run("ExtremelyLongHashes", func(t *testing.T) {
		longHash := strings.Repeat("a", 10000)
		data := []byte("test data for long hash")
		
		err := store.ObjectStore().PutObject(longHash, data)
		// Don't require success, just that it doesn't crash
		t.Logf("Long hash result: %v", err)
	})
	
	t.Run("SpecialCharacterHashes", func(t *testing.T) {
		specialHashes := []string{
			"hash\x00with\x00nulls",
			"hash\nwith\nnewlines",
			"hash\twith\ttabs",
			"hash with spaces",
			"hash/with/slashes",
			"hash\\with\\backslashes",
			"hash.with.dots",
			"hash-with-dashes",
			"hash_with_underscores",
		}
		
		for _, hash := range specialHashes {
			data := []byte("test data")
			err := store.ObjectStore().PutObject(hash, data)
			t.Logf("Special hash %q result: %v", hash, err)
			
			if err == nil {
				retrieved, err2 := store.ObjectStore().GetObject(hash)
				if err2 == nil {
					assert.Equal(t, data, retrieved)
				}
			}
		}
	})
	
	t.Run("ConcurrentSameKey", func(t *testing.T) {
		const numWorkers = 10
		const iterations = 100
		
		var wg sync.WaitGroup
		errors := make(chan error, numWorkers*iterations)
		
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				for j := 0; j < iterations; j++ {
					// All workers write to the same key
					data := []byte(fmt.Sprintf("worker-%d-iteration-%d", workerID, j))
					err := store.ObjectStore().PutObject("concurrent-same-key", data)
					if err != nil {
						errors <- err
					}
				}
			}(i)
		}
		
		wg.Wait()
		close(errors)
		
		errorCount := 0
		for err := range errors {
			errorCount++
			if errorCount <= 5 { // Log first few errors
				t.Logf("Concurrent same key error: %v", err)
			}
		}
		
		t.Logf("Concurrent same key errors: %d/%d", errorCount, numWorkers*iterations)
		
		// Should handle concurrent access reasonably well
		errorRate := float64(errorCount) / float64(numWorkers*iterations)
		assert.Less(t, errorRate, 0.5, "Error rate should be manageable")
	})
}

// runPerformanceRegressionTests checks for performance regressions
func runPerformanceRegressionTests(t *testing.T, store datastore.DataStore, storeName string) {
	// Baseline performance expectations (adjust based on your requirements)
	baselineExpectations := map[string]struct {
		minOpsPerSec float64
		maxAvgLatency time.Duration
	}{
		"Memory":     {minOpsPerSec: 10000, maxAvgLatency: 1 * time.Millisecond},
		"SQLite":     {minOpsPerSec: 1000, maxAvgLatency: 10 * time.Millisecond},
		"BadgerDB":   {minOpsPerSec: 2000, maxAvgLatency: 5 * time.Millisecond},
		"PostgreSQL": {minOpsPerSec: 500, maxAvgLatency: 20 * time.Millisecond},
	}
	
	baseline, exists := baselineExpectations[storeName]
	if !exists {
		t.Logf("No baseline expectations for %s, skipping regression test", storeName)
		return
	}
	
	// Run performance measurement
	stressConfig := datastore.StressTestConfig{
		Duration:           5 * time.Second,
		NumWorkers:         2,
		OperationsPerSec:   100,
		MaxObjectSize:      1024,
		MinObjectSize:      512,
		ReadWriteRatio:     0.7,
		TransactionPercent: 0.1,
	}
	
	metrics := datastore.StressTestSuite(t, store, stressConfig)
	
	// Check against baseline
	if metrics.ThroughputOpsPerSec < baseline.minOpsPerSec {
		t.Logf("⚠ PERFORMANCE REGRESSION: %s throughput %.2f ops/sec below baseline %.2f", 
			storeName, metrics.ThroughputOpsPerSec, baseline.minOpsPerSec)
	}
	
	if metrics.AverageLatency > baseline.maxAvgLatency {
		t.Logf("⚠ PERFORMANCE REGRESSION: %s latency %v above baseline %v", 
			storeName, metrics.AverageLatency, baseline.maxAvgLatency)
	}
	
	t.Logf("Performance check %s: %.2f ops/sec (baseline: %.2f), latency: %v (baseline: %v)", 
		storeName, metrics.ThroughputOpsPerSec, baseline.minOpsPerSec,
		metrics.AverageLatency, baseline.maxAvgLatency)
}

// runResourceUsageTests monitors resource usage patterns
func runResourceUsageTests(t *testing.T, store datastore.DataStore) {
	var initialMem, finalMem runtime.MemStats
	
	runtime.GC()
	runtime.ReadMemStats(&initialMem)
	
	// Perform various operations
	for i := 0; i < 1000; i++ {
		hash := fmt.Sprintf("resource-test-%d", i)
		data := make([]byte, 1024)
		
		err := store.ObjectStore().PutObject(hash, data)
		require.NoError(t, err)
		
		_, err = store.ObjectStore().GetObject(hash)
		require.NoError(t, err)
		
		if i%10 == 0 {
			err = store.ObjectStore().DeleteObject(hash)
			require.NoError(t, err)
		}
	}
	
	runtime.GC()
	runtime.ReadMemStats(&finalMem)
	
	memoryGrowth := finalMem.Alloc - initialMem.Alloc
	t.Logf("Resource usage: Memory growth = %d bytes (%d MB)", 
		memoryGrowth, memoryGrowth/(1024*1024))
	
	// Memory growth should be reasonable
	assert.Less(t, memoryGrowth, uint64(100*1024*1024), 
		"Memory growth should be less than 100MB")
}

// runDataIntegrityTests verifies data integrity under various conditions
func runDataIntegrityTests(t *testing.T, store datastore.DataStore) {
	t.Run("HashCollisionHandling", func(t *testing.T) {
		// Test with hashes that might cause internal collisions
		testHashes := []string{
			"0000000000000000000000000000000000000000",
			"1111111111111111111111111111111111111111",
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"ffffffffffffffffffffffffffffffffffffffff",
		}
		
		for i, hash := range testHashes {
			data := []byte(fmt.Sprintf("collision test data %d", i))
			
			err := store.ObjectStore().PutObject(hash, data)
			require.NoError(t, err)
			
			retrieved, err := store.ObjectStore().GetObject(hash)
			require.NoError(t, err)
			assert.Equal(t, data, retrieved, "Data integrity failed for hash %s", hash)
		}
	})
	
	t.Run("DataConsistencyAfterErrors", func(t *testing.T) {
		// Store valid data
		validData := []byte("valid test data")
		err := store.ObjectStore().PutObject("consistency-test", validData)
		require.NoError(t, err)
		
		// Try operations that might cause errors
		store.ObjectStore().PutObject("", []byte("empty hash"))                    // Invalid hash
		store.ObjectStore().PutObject("consistency-test", make([]byte, 100*1024*1024)) // Potentially too large
		store.ObjectStore().DeleteObject("non-existent-key")                       // Non-existent delete
		
		// Original data should still be intact
		retrieved, err := store.ObjectStore().GetObject("consistency-test")
		require.NoError(t, err)
		assert.Equal(t, validData, retrieved, "Data should remain consistent after errors")
	})
}

// runConcurrencyEdgeCaseTests tests edge cases in concurrent scenarios
func runConcurrencyEdgeCaseTests(t *testing.T, store datastore.DataStore) {
	t.Run("ReadWhileDeleting", func(t *testing.T) {
		// Store initial data
		for i := 0; i < 100; i++ {
			hash := fmt.Sprintf("read-delete-test-%d", i)
			data := []byte(fmt.Sprintf("data %d", i))
			err := store.ObjectStore().PutObject(hash, data)
			require.NoError(t, err)
		}
		
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		
		var wg sync.WaitGroup
		var readErrors, deleteErrors int64
		
		// Reader goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					for i := 0; i < 100; i++ {
						hash := fmt.Sprintf("read-delete-test-%d", i)
						_, err := store.ObjectStore().GetObject(hash)
						if err != nil && err != datastore.ErrNotFound {
							atomic.AddInt64(&readErrors, 1)
						}
					}
				}
			}
		}()
		
		// Deleter goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					for i := 0; i < 100; i++ {
						hash := fmt.Sprintf("read-delete-test-%d", i)
						err := store.ObjectStore().DeleteObject(hash)
						if err != nil && err != datastore.ErrNotFound {
							atomic.AddInt64(&deleteErrors, 1)
						}
					}
				}
			}
		}()
		
		wg.Wait()
		
		t.Logf("Concurrent read/delete: read errors=%d, delete errors=%d", readErrors, deleteErrors)
		
		// Some errors are expected, but not too many
		assert.Less(t, readErrors, int64(1000), "Read errors should be bounded")
		assert.Less(t, deleteErrors, int64(1000), "Delete errors should be bounded")
	})
}