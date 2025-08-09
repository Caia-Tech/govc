package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/caiatech/govc/datastore"
	"github.com/caiatech/govc/datastore/badger"
	"github.com/caiatech/govc/datastore/memory"
	"github.com/caiatech/govc/datastore/postgres"
	"github.com/caiatech/govc/datastore/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// IntegrationTestSuite runs tests that verify interoperability between different adapters
func IntegrationTestSuite(t *testing.T) {
	stores := createAllStores(t)
	defer cleanupStores(stores)
	
	t.Run("DataMigrationTests", func(t *testing.T) {
		runDataMigrationTests(t, stores)
	})
	
	t.Run("ConsistencyTests", func(t *testing.T) {
		runConsistencyTests(t, stores)
	})
	
	t.Run("BackupRestoreTests", func(t *testing.T) {
		runBackupRestoreTests(t, stores)
	})
	
	t.Run("PerformanceComparison", func(t *testing.T) {
		runPerformanceComparison(t, stores)
	})
	
	t.Run("FeatureCompatibilityMatrix", func(t *testing.T) {
		runFeatureCompatibilityTests(t, stores)
	})
}

// createAllStores creates instances of all available datastores for testing
func createAllStores(t *testing.T) map[string]datastore.DataStore {
	stores := make(map[string]datastore.DataStore)
	
	// Memory Store
	memConfig := datastore.Config{Type: datastore.TypeMemory}
	memStore := memory.New(memConfig)
	if err := memStore.Initialize(memConfig); err == nil {
		stores["Memory"] = memStore
	} else {
		t.Logf("Failed to create Memory store: %v", err)
	}
	
	// SQLite Store
	tempDir := t.TempDir()
	sqliteConfig := datastore.Config{
		Type:       datastore.TypeSQLite,
		Connection: filepath.Join(tempDir, "integration_test.db"),
	}
	sqliteStore, err := sqlite.New(sqliteConfig)
	if err == nil {
		if err := sqliteStore.Initialize(sqliteConfig); err == nil {
			stores["SQLite"] = sqliteStore
		} else {
			t.Logf("Failed to initialize SQLite store: %v", err)
		}
	} else {
		t.Logf("Failed to create SQLite store: %v", err)
	}
	
	// BadgerDB Store
	badgerConfig := datastore.Config{
		Type:       datastore.TypeBadger,
		Connection: filepath.Join(tempDir, "badger_integration"),
	}
	badgerStore, err := badger.New(badgerConfig)
	if err == nil {
		if err := badgerStore.Initialize(badgerConfig); err == nil {
			stores["BadgerDB"] = badgerStore
		} else {
			t.Logf("Failed to initialize BadgerDB store: %v", err)
		}
	} else {
		t.Logf("Failed to create BadgerDB store: %v", err)
	}
	
	// PostgreSQL Store (only if available)
	pgConfig := datastore.Config{
		Type:               datastore.TypePostgres,
		Connection:         os.Getenv("POSTGRES_TEST_URL"),
		MaxConnections:     5,
		MaxIdleConnections: 2,
		ConnectionTimeout:  10 * time.Second,
	}
	if pgConfig.Connection == "" {
		pgConfig.Connection = "postgres://postgres:postgres@localhost:5432/govc_integration_test?sslmode=disable"
	}
	
	pgStore, err := postgres.New(pgConfig)
	if err == nil {
		if err := pgStore.Initialize(pgConfig); err == nil {
			stores["PostgreSQL"] = pgStore
			t.Logf("PostgreSQL store available for integration tests")
		} else {
			t.Logf("PostgreSQL not available for integration tests: %v", err)
		}
	} else {
		t.Logf("Failed to create PostgreSQL store: %v", err)
	}
	
	t.Logf("Created %d stores for integration testing: %v", len(stores), getStoreNames(stores))
	return stores
}

// cleanupStores properly closes all stores
func cleanupStores(stores map[string]datastore.DataStore) {
	for name, store := range stores {
		if store != nil {
			store.Close()
		}
		_ = name // Avoid unused variable warning
	}
}

// getStoreNames returns a slice of store names
func getStoreNames(stores map[string]datastore.DataStore) []string {
	names := make([]string, 0, len(stores))
	for name := range stores {
		names = append(names, name)
	}
	return names
}

// runDataMigrationTests tests migrating data between different store types
func runDataMigrationTests(t *testing.T, stores map[string]datastore.DataStore) {
	if len(stores) < 2 {
		t.Skip("Need at least 2 stores for migration testing")
	}
	
	// Create test dataset
	testData := generateTestDataset(100)
	
	// Test all combinations of source -> destination migrations
	storeNames := getStoreNames(stores)
	for i, sourceName := range storeNames {
		for j, destName := range storeNames {
			if i >= j { // Skip same store and avoid duplicate combinations
				continue
			}
			
			t.Run(fmt.Sprintf("%s_to_%s", sourceName, destName), func(t *testing.T) {
				sourceStore := stores[sourceName]
				destStore := stores[destName]
				
				// Clear destination store
				clearStore(t, destStore)
				
				// Populate source store
				populateStore(t, sourceStore, testData)
				
				// Perform migration
				start := time.Now()
				migrationErrors := migrateData(t, sourceStore, destStore)
				migrationTime := time.Since(start)
				
				t.Logf("Migration %s -> %s took %v with %d errors", 
					sourceName, destName, migrationTime, len(migrationErrors))
				
				// Verify all data was migrated correctly
				verifyMigration(t, sourceStore, destStore, testData)
				
				// Log any migration errors
				for _, err := range migrationErrors {
					t.Logf("Migration error: %v", err)
				}
				
				// Migration should have few or no errors
				errorRate := float64(len(migrationErrors)) / float64(len(testData))
				assert.Less(t, errorRate, 0.05, "Migration error rate should be less than 5%%")
			})
		}
	}
}

// generateTestDataset creates a diverse test dataset
func generateTestDataset(count int) map[string][]byte {
	dataset := make(map[string][]byte)
	
	for i := 0; i < count; i++ {
		var data []byte
		var hash string
		
		switch i % 5 {
		case 0: // Small text data
			hash = fmt.Sprintf("small-text-%d", i)
			data = []byte(fmt.Sprintf("Small text data item %d", i))
		case 1: // Binary data
			hash = fmt.Sprintf("binary-data-%d", i)
			data = make([]byte, 256)
			for j := range data {
				data[j] = byte(j % 256)
			}
		case 2: // Large data
			hash = fmt.Sprintf("large-data-%d", i)
			data = make([]byte, 10240) // 10KB
			for j := range data {
				data[j] = byte((i + j) % 256)
			}
		case 3: // Unicode data
			hash = fmt.Sprintf("unicode-data-%d", i)
			data = []byte(fmt.Sprintf("Unicode test: 🌍🚀💻 Item %d with émojis", i))
		case 4: // Empty data
			hash = fmt.Sprintf("empty-data-%d", i)
			data = []byte{}
		}
		
		dataset[hash] = data
	}
	
	return dataset
}

// populateStore fills a store with test data
func populateStore(t *testing.T, store datastore.DataStore, data map[string][]byte) {
	for hash, value := range data {
		err := store.ObjectStore().PutObject(hash, value)
		require.NoError(t, err, "Failed to populate store with test data")
	}
}

// clearStore removes all objects from a store
func clearStore(t *testing.T, store datastore.DataStore) {
	// Get all objects
	hashes, err := store.ObjectStore().ListObjects("", 0)
	if err != nil {
		t.Logf("Warning: Could not list objects to clear store: %v", err)
		return
	}
	
	// Delete all objects
	for _, hash := range hashes {
		err := store.ObjectStore().DeleteObject(hash)
		if err != nil {
			t.Logf("Warning: Could not delete object %s during cleanup: %v", hash, err)
		}
	}
}

// migrateData migrates all data from source to destination store
func migrateData(t *testing.T, source, dest datastore.DataStore) []error {
	var errors []error
	
	// Get all objects from source
	hashes, err := source.ObjectStore().ListObjects("", 0)
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to list source objects: %w", err))
		return errors
	}
	
	// Copy each object
	for _, hash := range hashes {
		data, err := source.ObjectStore().GetObject(hash)
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to get object %s: %w", hash, err))
			continue
		}
		
		err = dest.ObjectStore().PutObject(hash, data)
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to put object %s: %w", hash, err))
		}
	}
	
	return errors
}

// verifyMigration checks that all data was migrated correctly
func verifyMigration(t *testing.T, source, dest datastore.DataStore, originalData map[string][]byte) {
	// Check that all original data exists in destination
	for hash, expectedData := range originalData {
		destData, err := dest.ObjectStore().GetObject(hash)
		assert.NoError(t, err, "Object %s should exist in destination", hash)
		if err == nil {
			assert.Equal(t, expectedData, destData, "Data should match for object %s", hash)
		}
	}
	
	// Check that destination doesn't have extra data
	destHashes, err := dest.ObjectStore().ListObjects("", 0)
	if err == nil {
		assert.Equal(t, len(originalData), len(destHashes), 
			"Destination should have same number of objects as source")
	}
}

// runConsistencyTests verifies that all stores behave consistently
func runConsistencyTests(t *testing.T, stores map[string]datastore.DataStore) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T, store datastore.DataStore) interface{}
	}{
		{
			name: "EmptyDataHandling",
			testFunc: func(t *testing.T, store datastore.DataStore) interface{} {
				err := store.ObjectStore().PutObject("empty-test", []byte{})
				require.NoError(t, err)
				
				data, err := store.ObjectStore().GetObject("empty-test")
				require.NoError(t, err)
				return len(data)
			},
		},
		{
			name: "NonExistentObjectBehavior",
			testFunc: func(t *testing.T, store datastore.DataStore) interface{} {
				_, err := store.ObjectStore().GetObject("definitely-does-not-exist")
				return err.Error()
			},
		},
		{
			name: "LargeObjectHandling",
			testFunc: func(t *testing.T, store datastore.DataStore) interface{} {
				largeData := make([]byte, 100000) // 100KB
				for i := range largeData {
					largeData[i] = byte(i % 256)
				}
				
				err := store.ObjectStore().PutObject("large-test", largeData)
				if err != nil {
					return fmt.Sprintf("Error: %v", err)
				}
				
				retrieved, err := store.ObjectStore().GetObject("large-test")
				if err != nil {
					return fmt.Sprintf("Error: %v", err)
				}
				
				return len(retrieved) == len(largeData) && bytesEqual(largeData[:100], retrieved[:100])
			},
		},
		{
			name: "UnicodeHandling",
			testFunc: func(t *testing.T, store datastore.DataStore) interface{} {
				unicodeData := []byte("Hello 🌍 World 测试 🚀")
				
				err := store.ObjectStore().PutObject("unicode-test", unicodeData)
				require.NoError(t, err)
				
				retrieved, err := store.ObjectStore().GetObject("unicode-test")
				require.NoError(t, err)
				
				return bytesEqual(unicodeData, retrieved)
			},
		},
	}
	
	// Run each test case against all stores and compare results
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := make(map[string]interface{})
			
			// Execute test on each store
			for name, store := range stores {
				result := tc.testFunc(t, store)
				results[name] = result
				t.Logf("%s result for %s: %v", tc.name, name, result)
			}
			
			// Check consistency across stores
			var firstResult interface{}
			var firstName string
			for name, result := range results {
				if firstResult == nil {
					firstResult = result
					firstName = name
				} else {
					// For error messages, we allow some variation
					if tc.name == "NonExistentObjectBehavior" {
						// All should be error messages, but exact text may vary
						assert.IsType(t, "", result, "All stores should return string error messages")
					} else {
						assert.Equal(t, firstResult, result, 
							"Store %s result should match %s result", name, firstName)
					}
				}
			}
		})
	}
}

// runBackupRestoreTests tests backup/restore functionality between stores
func runBackupRestoreTests(t *testing.T, stores map[string]datastore.DataStore) {
	if len(stores) < 2 {
		t.Skip("Need at least 2 stores for backup/restore testing")
	}
	
	// Create test data
	testData := generateTestDataset(50)
	
	storeNames := getStoreNames(stores)
	
	for _, sourceName := range storeNames {
		t.Run(fmt.Sprintf("BackupRestore_%s", sourceName), func(t *testing.T) {
			sourceStore := stores[sourceName]
			
			// Populate source store
			populateStore(t, sourceStore, testData)
			
			// Create "backup" by copying to all other stores
			backups := make(map[string]datastore.DataStore)
			for _, destName := range storeNames {
				if destName != sourceName {
					destStore := stores[destName]
					clearStore(t, destStore)
					
					errors := migrateData(t, sourceStore, destStore)
					assert.Empty(t, errors, "Backup to %s should be successful", destName)
					
					backups[destName] = destStore
				}
			}
			
			// Simulate data loss in source (clear some data)
			corruptedData := make(map[string][]byte)
			counter := 0
			for hash := range testData {
				if counter%3 == 0 { // Corrupt every third item
					corruptedData[hash] = testData[hash]
					err := sourceStore.ObjectStore().DeleteObject(hash)
					require.NoError(t, err)
				}
				counter++
			}
			
			// Verify data loss
			for hash := range corruptedData {
				_, err := sourceStore.ObjectStore().GetObject(hash)
				assert.ErrorIs(t, err, datastore.ErrNotFound, "Object should be deleted")
			}
			
			// Restore from each backup and verify
			for backupName, backupStore := range backups {
				t.Run(fmt.Sprintf("RestoreFrom_%s", backupName), func(t *testing.T) {
					// Restore corrupted data from backup
					for hash := range corruptedData {
						data, err := backupStore.ObjectStore().GetObject(hash)
						require.NoError(t, err, "Backup should contain the data")
						
						err = sourceStore.ObjectStore().PutObject(hash, data)
						require.NoError(t, err, "Should be able to restore data")
					}
					
					// Verify restoration
					for hash, expectedData := range corruptedData {
						restoredData, err := sourceStore.ObjectStore().GetObject(hash)
						require.NoError(t, err, "Restored object should exist")
						assert.Equal(t, expectedData, restoredData, "Restored data should match original")
					}
				})
			}
		})
	}
}

// runPerformanceComparison runs comparative performance tests
func runPerformanceComparison(t *testing.T, stores map[string]datastore.DataStore) {
	if len(stores) < 2 {
		t.Skip("Need at least 2 stores for performance comparison")
	}
	
	// Run performance test suite
	datastore.PerformanceTestSuite(t, stores)
}

// runFeatureCompatibilityTests creates a compatibility matrix
func runFeatureCompatibilityTests(t *testing.T, stores map[string]datastore.DataStore) {
	features := []struct {
		name     string
		testFunc func(datastore.DataStore) bool
	}{
		{
			name: "Transactions",
			testFunc: func(store datastore.DataStore) bool {
				ctx := context.Background()
				tx, err := store.BeginTx(ctx, nil)
				if err != nil {
					return false
				}
				tx.Rollback()
				return true
			},
		},
		{
			name: "BatchOperations",
			testFunc: func(store datastore.DataStore) bool {
				objects := map[string][]byte{
					"batch1": []byte("data1"),
					"batch2": []byte("data2"),
				}
				err := store.ObjectStore().PutObjects(objects)
				return err == nil
			},
		},
		{
			name: "Iteration",
			testFunc: func(store datastore.DataStore) bool {
				// Put test object
				store.ObjectStore().PutObject("iter-test", []byte("data"))
				
				// Try iteration
				err := store.ObjectStore().IterateObjects("iter", func(hash string, data []byte) error {
					return nil
				})
				return err == nil
			},
		},
		{
			name: "ObjectCounting",
			testFunc: func(store datastore.DataStore) bool {
				store.ObjectStore().PutObject("count-test", []byte("data"))
				_, err := store.ObjectStore().CountObjects()
				return err == nil
			},
		},
		{
			name: "SizeReporting",
			testFunc: func(store datastore.DataStore) bool {
				store.ObjectStore().PutObject("size-test", []byte("data"))
				_, err := store.ObjectStore().GetObjectSize("size-test")
				return err == nil
			},
		},
		{
			name: "HealthChecks",
			testFunc: func(store datastore.DataStore) bool {
				ctx := context.Background()
				err := store.HealthCheck(ctx)
				return err == nil
			},
		},
		{
			name: "Metrics",
			testFunc: func(store datastore.DataStore) bool {
				metrics := store.GetMetrics()
				return metrics.StartTime != time.Time{}
			},
		},
		{
			name: "InfoReporting",
			testFunc: func(store datastore.DataStore) bool {
				info := store.Info()
				return len(info) > 0
			},
		},
	}
	
	// Build compatibility matrix
	t.Log("\n=== FEATURE COMPATIBILITY MATRIX ===")
	
	// Print header
	header := fmt.Sprintf("%-20s", "Feature")
	for name := range stores {
		header += fmt.Sprintf("%-12s", name)
	}
	t.Log(header)
	
	// Test each feature
	for _, feature := range features {
		row := fmt.Sprintf("%-20s", feature.name)
		
		for _, store := range stores {
			supported := feature.testFunc(store)
			if supported {
				row += fmt.Sprintf("%-12s", "✓")
			} else {
				row += fmt.Sprintf("%-12s", "✗")
			}
		}
		
		t.Log(row)
	}
}

// Helper function to check if bytes are equal (avoiding import)
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}