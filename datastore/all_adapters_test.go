package datastore_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Caia-Tech/govc/datastore"
	"github.com/stretchr/testify/require"
	
	// Import all adapters to register them with the factory
	_ "github.com/Caia-Tech/govc/datastore/badger"
	_ "github.com/Caia-Tech/govc/datastore/memory"
	_ "github.com/Caia-Tech/govc/datastore/postgres"
	_ "github.com/Caia-Tech/govc/datastore/sqlite"
)

// TestAllAdaptersCompliance runs compliance tests on all datastore implementations
func TestAllAdaptersCompliance(t *testing.T) {
	// Memory Store
	t.Run("MemoryStore", func(t *testing.T) {
		config := datastore.Config{
			Type: datastore.TypeMemory,
		}
		
		store, err := datastore.Create(config)
		require.NoError(t, err)
		
		suite := datastore.ComplianceTestSuite{
			Store:  store,
			Config: config,
			Cleanup: func() {
				store.Close()
			},
		}
		
		datastore.RunComplianceTests(t, suite)
	})
	
	// SQLite Store
	t.Run("SQLiteStore", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "test.db")
		
		config := datastore.Config{
			Type:       datastore.TypeSQLite,
			Connection: dbPath,
		}
		
		store, err := datastore.Create(config)
		require.NoError(t, err)
		
		suite := datastore.ComplianceTestSuite{
			Store:  store,
			Config: config,
			Cleanup: func() {
				store.Close()
				os.RemoveAll(tempDir)
			},
		}
		
		datastore.RunComplianceTests(t, suite)
	})
	
	// BadgerDB Store
	t.Run("BadgerStore", func(t *testing.T) {
		tempDir := t.TempDir()
		
		config := datastore.Config{
			Type:       datastore.TypeBadger,
			Connection: tempDir,
		}
		
		store, err := datastore.Create(config)
		require.NoError(t, err)
		
		suite := datastore.ComplianceTestSuite{
			Store:  store,
			Config: config,
			Cleanup: func() {
				store.Close()
				os.RemoveAll(tempDir)
			},
		}
		
		datastore.RunComplianceTests(t, suite)
	})
	
	// PostgreSQL Store (only if available)
	t.Run("PostgresStore", func(t *testing.T) {
		connStr := os.Getenv("POSTGRES_TEST_URL")
		if connStr == "" {
			t.Skip("PostgreSQL not available, set POSTGRES_TEST_URL to run tests")
		}
		
		config := datastore.Config{
			Type:               datastore.TypePostgres,
			Connection:         connStr,
			MaxConnections:     10,
			MaxIdleConnections: 2,
		}
		
		store, err := datastore.Create(config)
		require.NoError(t, err)
		
		suite := datastore.ComplianceTestSuite{
			Store:  store,
			Config: config,
			Cleanup: func() {
				store.Close()
			},
		}
		
		datastore.RunComplianceTests(t, suite)
	})
}

// TestFactoryPattern tests the factory pattern for creating datastores
func TestFactoryPattern(t *testing.T) {
	tests := []struct {
		name   string
		config datastore.Config
		skip   bool
		skipMsg string
	}{
		{
			name: "Memory",
			config: datastore.Config{
				Type: datastore.TypeMemory,
			},
		},
		{
			name: "SQLite",
			config: datastore.Config{
				Type:       datastore.TypeSQLite,
				Connection: filepath.Join(t.TempDir(), "factory_test.db"),
			},
		},
		{
			name: "BadgerDB",
			config: datastore.Config{
				Type:       datastore.TypeBadger,
				Connection: t.TempDir(),
			},
		},
		{
			name: "PostgreSQL",
			config: datastore.Config{
				Type:       datastore.TypePostgres,
				Connection: os.Getenv("POSTGRES_TEST_URL"),
			},
			skip:    os.Getenv("POSTGRES_TEST_URL") == "",
			skipMsg: "PostgreSQL not available",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip(tt.skipMsg)
			}
			
			// Create store using factory
			store, err := datastore.Create(tt.config)
			require.NoError(t, err)
			require.NotNil(t, store)
			
			// Initialize
			err = store.Initialize(tt.config)
			require.NoError(t, err)
			
			// Verify type
			require.Equal(t, tt.config.Type, store.Type())
			
			// Basic health check
			ctx := context.Background()
			err = store.HealthCheck(ctx)
			require.NoError(t, err)
			
			// Clean up
			store.Close()
		})
	}
}

// BenchmarkDatastores benchmarks all datastore implementations
func BenchmarkDatastores(b *testing.B) {
	benchmarks := []struct {
		name  string
		setup func() (datastore.DataStore, func())
		skip  bool
	}{
		{
			name: "Memory",
			setup: func() (datastore.DataStore, func()) {
				config := datastore.Config{Type: datastore.TypeMemory}
				store, _ := datastore.Create(config)
				return store, func() { store.Close() }
			},
		},
		{
			name: "SQLite",
			setup: func() (datastore.DataStore, func()) {
				tempDir := b.TempDir()
				config := datastore.Config{
					Type:       datastore.TypeSQLite,
					Connection: filepath.Join(tempDir, "bench.db"),
				}
				store, _ := datastore.Create(config)
				return store, func() {
					store.Close()
					os.RemoveAll(tempDir)
				}
			},
		},
		{
			name: "BadgerDB",
			setup: func() (datastore.DataStore, func()) {
				tempDir := b.TempDir()
				config := datastore.Config{
					Type:       datastore.TypeBadger,
					Connection: tempDir,
				}
				store, _ := datastore.Create(config)
				return store, func() {
					store.Close()
					os.RemoveAll(tempDir)
				}
			},
		},
		{
			name: "PostgreSQL",
			setup: func() (datastore.DataStore, func()) {
				connStr := os.Getenv("POSTGRES_TEST_URL")
				if connStr == "" {
					return nil, func() {}
				}
				config := datastore.Config{
					Type:       datastore.TypePostgres,
					Connection: connStr,
				}
				store, _ := datastore.Create(config)
				return store, func() { store.Close() }
			},
			skip: os.Getenv("POSTGRES_TEST_URL") == "",
		},
	}
	
	// Benchmark scenarios
	scenarios := []struct {
		name string
		test func(b *testing.B, store datastore.DataStore)
	}{
		{
			name: "PutObject",
			test: benchmarkPutObject,
		},
		{
			name: "GetObject",
			test: benchmarkGetObject,
		},
		{
			name: "BatchPut",
			test: benchmarkBatchPut,
		},
		{
			name: "Transaction",
			test: benchmarkTransaction,
		},
	}
	
	for _, bench := range benchmarks {
		if bench.skip {
			continue
		}
		
		b.Run(bench.name, func(b *testing.B) {
			store, cleanup := bench.setup()
			if store == nil {
				b.Skip("Store not available")
			}
			defer cleanup()
			
			for _, scenario := range scenarios {
				b.Run(scenario.name, func(b *testing.B) {
					scenario.test(b, store)
				})
			}
		})
	}
}

func benchmarkPutObject(b *testing.B, store datastore.DataStore) {
	data := []byte("benchmark test data that is somewhat realistic in size for a git object")
	objStore := store.ObjectStore()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hash := fmt.Sprintf("bench-put-%d-%d", time.Now().UnixNano(), i)
		err := objStore.PutObject(hash, data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkGetObject(b *testing.B, store datastore.DataStore) {
	// Prepare test data
	objStore := store.ObjectStore()
	hash := "bench-get-object"
	data := []byte("benchmark test data for reading")
	
	err := objStore.PutObject(hash, data)
	if err != nil {
		b.Fatal(err)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		retrieved, err := objStore.GetObject(hash)
		if err != nil {
			b.Fatal(err)
		}
		if len(retrieved) != len(data) {
			b.Fatal("data mismatch")
		}
	}
}

func benchmarkBatchPut(b *testing.B, store datastore.DataStore) {
	objStore := store.ObjectStore()
	
	// Prepare batch
	batchSize := 100
	objects := make(map[string][]byte, batchSize)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Prepare unique batch
		for j := 0; j < batchSize; j++ {
			hash := fmt.Sprintf("batch-%d-%d", i, j)
			objects[hash] = []byte(fmt.Sprintf("data %d", j))
		}
		
		err := objStore.PutObjects(objects)
		if err != nil {
			b.Fatal(err)
		}
		
		// Clear for next iteration
		for k := range objects {
			delete(objects, k)
		}
	}
}

func benchmarkTransaction(b *testing.B, store datastore.DataStore) {
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, err := store.BeginTx(ctx, nil)
		if err != nil {
			b.Fatal(err)
		}
		
		// Perform some operations
		hash := fmt.Sprintf("tx-bench-%d", i)
		data := []byte("transaction benchmark data")
		
		err = tx.PutObject(hash, data)
		if err != nil {
			tx.Rollback()
			b.Fatal(err)
		}
		
		err = tx.Commit()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestConcurrentAccess tests concurrent access patterns
func TestConcurrentAccess(t *testing.T) {
	stores := []struct {
		name   string
		config datastore.Config
		skip   bool
	}{
		{
			name:   "Memory",
			config: datastore.Config{Type: datastore.TypeMemory},
		},
		{
			name: "SQLite",
			config: datastore.Config{
				Type:       datastore.TypeSQLite,
				Connection: filepath.Join(t.TempDir(), "concurrent.db"),
			},
		},
		{
			name: "BadgerDB",
			config: datastore.Config{
				Type:       datastore.TypeBadger,
				Connection: t.TempDir(),
			},
		},
	}
	
	for _, st := range stores {
		t.Run(st.name, func(t *testing.T) {
			if st.skip {
				t.Skip()
			}
			
			store, err := datastore.Create(st.config)
			require.NoError(t, err)
			
			err = store.Initialize(st.config)
			require.NoError(t, err)
			defer store.Close()
			
			// Run concurrent operations
			const numGoroutines = 10
			const opsPerGoroutine = 100
			
			errChan := make(chan error, numGoroutines)
			doneChan := make(chan bool, numGoroutines)
			
			for i := 0; i < numGoroutines; i++ {
				go func(id int) {
					objStore := store.ObjectStore()
					
					for j := 0; j < opsPerGoroutine; j++ {
						hash := fmt.Sprintf("concurrent-%d-%d", id, j)
						data := []byte(fmt.Sprintf("data %d %d", id, j))
						
						// Write
						if err := objStore.PutObject(hash, data); err != nil {
							errChan <- err
							return
						}
						
						// Read
						retrieved, err := objStore.GetObject(hash)
						if err != nil {
							errChan <- err
							return
						}
						
						// Verify
						if string(retrieved) != string(data) {
							errChan <- fmt.Errorf("data mismatch")
							return
						}
						
						// Delete
						if err := objStore.DeleteObject(hash); err != nil {
							errChan <- err
							return
						}
					}
					
					doneChan <- true
				}(i)
			}
			
			// Wait for completion
			for i := 0; i < numGoroutines; i++ {
				select {
				case err := <-errChan:
					t.Fatal(err)
				case <-doneChan:
					// Success
				case <-time.After(30 * time.Second):
					t.Fatal("timeout waiting for goroutines")
				}
			}
		})
	}
}