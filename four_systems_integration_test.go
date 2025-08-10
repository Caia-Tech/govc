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

// TestFourSystemsIntegration tests all four critical systems working together:
// 1. Memory Compaction (GarbageCollector)
// 2. Pub/Sub System (EventBus) 
// 3. Atomic Operations (AtomicTransactionManager)
// 4. Query Engine
func TestFourSystemsIntegration_BasicWorkflow(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Start garbage collection
	repo.gc.Start(context.Background())
	defer repo.gc.Stop()
	
	var eventCount int64
	var commitHashes []string
	var mu sync.Mutex
	
	// Subscribe to events (Pub/Sub System)
	unsubscribe := repo.OnCommit(func(commitHash string, files []string) {
		atomic.AddInt64(&eventCount, 1)
		mu.Lock()
		commitHashes = append(commitHashes, commitHash)
		mu.Unlock()
		t.Logf("Event received: Commit %s with %d files", commitHash[:8], len(files))
	})
	defer unsubscribe()
	
	// Perform atomic operations that create searchable content
	testData := []struct {
		filename string
		content  string
		message  string
	}{
		{"frontend/App.js", "import React from 'react';\nexport default App;", "Add main App component"},
		{"frontend/Button.js", "import React from 'react';\nexport default Button;", "Add Button component"},
		{"backend/auth.go", "package main\nfunc authenticate() {}", "Add authentication handler"},
		{"backend/users.go", "package main\nfunc getUsers() {}", "Add users handler"}, 
		{"config/database.json", `{"host": "localhost", "port": 5432}`, "Add database config"},
		{"docs/README.md", "# Project\nThis is a test project", "Add documentation"},
		{"tests/auth_test.go", "package main\nfunc TestAuth() {}", "Add auth tests"},
	}
	
	// Use Atomic Operations for each file
	for i, data := range testData {
		t.Logf("Creating file %d: %s", i+1, data.filename)
		
		_, err := repo.AtomicCreateFile(data.filename, []byte(data.content), data.message)
		require.NoError(t, err)
		
		// Add some garbage collection candidates  
		objectHash := fmt.Sprintf("temp_object_%d", i)
		repo.gc.AddReference(objectHash)
		if i%2 == 0 {
			repo.gc.RemoveReference(objectHash) // Make every other one a GC candidate
		}
	}
	
	// Wait for events and indexing to propagate
	time.Sleep(200 * time.Millisecond)
	
	// Test 1: Verify Pub/Sub System received all events
	assert.Equal(t, int64(len(testData)), atomic.LoadInt64(&eventCount))
	mu.Lock()
	assert.Equal(t, len(testData), len(commitHashes))
	mu.Unlock()
	
	// Test 2: Verify Atomic Operations statistics
	stats := repo.GetTransactionStats()
	assert.Equal(t, int64(len(testData)), stats.CommittedTransactions)
	assert.Equal(t, int64(0), stats.AbortedTransactions)
	
	// Test 3: Test Query Engine functionality
	t.Run("QueryEngine_FilePatterns", func(t *testing.T) {
		// Find JavaScript files
		jsFiles, err := repo.FindFiles("*.js")
		require.NoError(t, err)
		assert.Equal(t, 2, len(jsFiles), "Should find 2 JS files")
		
		// Find Go files
		goFiles, err := repo.FindFiles("*.go")
		require.NoError(t, err)
		assert.Equal(t, 2, len(goFiles), "Should find 2 Go files")
		
		// Find files in backend directory
		backendFiles, err := repo.FindFiles("backend/*")
		require.NoError(t, err)
		assert.Equal(t, 2, len(backendFiles), "Should find 2 backend files")
	})
	
	t.Run("QueryEngine_ContentSearch", func(t *testing.T) {
		// Search for React imports
		reactFiles, err := repo.SearchFileContent("React")
		require.NoError(t, err)
		assert.Equal(t, 2, len(reactFiles), "Should find 2 files containing 'React'")
		
		// Search for authentication
		authFiles, err := repo.SearchFileContent("authenticate")
		require.NoError(t, err)
		assert.Equal(t, 1, len(authFiles), "Should find 1 file with authenticate")
	})
	
	t.Run("QueryEngine_CommitSearch", func(t *testing.T) {
		// Search commits by message
		addCommits, err := repo.SearchCommitMessages("Add")
		require.NoError(t, err)
		assert.True(t, len(addCommits) >= 5, "Should find commits with 'Add'")
		
		// Find commits by author (System)
		systemCommits, err := repo.FindCommitsByAuthor("System")
		require.NoError(t, err)
		assert.Equal(t, len(testData), len(systemCommits), "Should find all commits by System")
	})
	
	// Test 4: Force garbage collection and verify Memory Compaction
	repo.gc.ForceCompaction(CompactOptions{Force: true})
	
	gcStats := repo.gc.GetStats()
	assert.GreaterOrEqual(t, gcStats.TotalRuns, int64(1), "Should have run GC at least once")
	assert.GreaterOrEqual(t, gcStats.ObjectsCollected, int64(1), "Should have collected objects")
	
	// Test 5: Verify Event Bus statistics
	eventBusStats := repo.eventBus.GetStats()
	assert.GreaterOrEqual(t, eventBusStats.TotalEvents, int64(len(testData)))
	assert.GreaterOrEqual(t, eventBusStats.TotalDelivered, int64(len(testData)))
	assert.Equal(t, int64(0), eventBusStats.TotalErrors)
	
	// Test 6: Verify Query Engine statistics
	queryStats := repo.GetQueryEngineStats()
	assert.True(t, queryStats.TotalQueries > 0, "Should have processed queries")
	assert.True(t, queryStats.FileIndexSize >= len(testData), "Should have indexed all files")
	assert.True(t, queryStats.CommitIndexSize >= len(testData), "Should have indexed all commits")
	
	t.Logf("✅ Four Systems Integration Results:")
	t.Logf("  Atomic Operations: %d commits, %d aborts", stats.CommittedTransactions, stats.AbortedTransactions)
	t.Logf("  Pub/Sub Events: %d published, %d delivered, %d errors", eventBusStats.TotalEvents, eventBusStats.TotalDelivered, eventBusStats.TotalErrors)
	t.Logf("  Query Engine: %d queries, %d files indexed, %d commits indexed", queryStats.TotalQueries, queryStats.FileIndexSize, queryStats.CommitIndexSize)
	t.Logf("  Memory Compaction: %d runs, %d objects collected", gcStats.TotalRuns, gcStats.ObjectsCollected)
}

// TestFourSystemsIntegration_HighConcurrency tests all systems under high concurrent load
func TestFourSystemsIntegration_HighConcurrency(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Start garbage collection
	repo.gc.Start(context.Background())
	defer repo.gc.Stop()
	
	var totalEvents int64
	var totalQueries int64
	
	// Subscribe to events
	unsubscribe := repo.OnCommit(func(commitHash string, files []string) {
		atomic.AddInt64(&totalEvents, 1)
	})
	defer unsubscribe()
	
	numWorkers := 20
	operationsPerWorker := 25
	var wg sync.WaitGroup
	
	t.Logf("Starting high concurrency test: %d workers, %d ops each", numWorkers, operationsPerWorker)
	
	// Launch concurrent workers performing atomic operations
	for workerID := 0; workerID < numWorkers; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			for i := 0; i < operationsPerWorker; i++ {
				// Create files with atomic operations
				filename := fmt.Sprintf("worker_%d/file_%d.txt", id, i)
				content := fmt.Sprintf("Worker %d content %d with keywords: test data worker%d", id, i, id)
				message := fmt.Sprintf("Worker %d operation %d", id, i)
				
				_, err := repo.AtomicCreateFile(filename, []byte(content), message)
				if err != nil {
					t.Logf("Worker %d operation %d failed: %v", id, i, err)
					continue
				}
				
				// Add GC work
				objectHash := fmt.Sprintf("worker_%d_object_%d", id, i)
				repo.gc.AddReference(objectHash)
				if i%3 == 0 {
					repo.gc.RemoveReference(objectHash)
				}
				
				// Perform some queries to test Query Engine under load
				if i%5 == 0 {
					go func() {
						atomic.AddInt64(&totalQueries, 1)
						repo.FindFiles("*.txt")
					}()
				}
			}
		}(workerID)
	}
	
	// Launch concurrent query workers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			for j := 0; j < 20; j++ {
				atomic.AddInt64(&totalQueries, 1)
				
				// Various query types
				switch j % 4 {
				case 0:
					repo.FindFiles("*.txt")
				case 1:
					repo.SearchFileContent("worker")
				case 2:
					repo.SearchCommitMessages("Worker")
				case 3:
					repo.GetRecentCommits(10)
				}
				
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}
	
	wg.Wait()
	
	// Wait for all async operations to complete
	time.Sleep(300 * time.Millisecond)
	
	// Force garbage collection
	repo.gc.ForceCompaction(CompactOptions{Force: true})
	
	// Verify results
	expectedOperations := int64(numWorkers * operationsPerWorker)
	
	// Check atomic operations
	stats := repo.GetTransactionStats()
	assert.Equal(t, expectedOperations, stats.CommittedTransactions)
	assert.Equal(t, int64(0), stats.AbortedTransactions)
	
	// Check events
	assert.GreaterOrEqual(t, atomic.LoadInt64(&totalEvents), expectedOperations)
	
	// Check event bus
	eventBusStats := repo.eventBus.GetStats()
	assert.GreaterOrEqual(t, eventBusStats.TotalEvents, expectedOperations)
	assert.GreaterOrEqual(t, eventBusStats.TotalDelivered, expectedOperations)
	assert.Equal(t, int64(0), eventBusStats.TotalErrors, "Should have no event errors")
	
	// Check queries
	assert.GreaterOrEqual(t, atomic.LoadInt64(&totalQueries), int64(100), "Should have processed many queries")
	
	// Check query engine
	queryStats := repo.GetQueryEngineStats()
	assert.GreaterOrEqual(t, queryStats.TotalQueries, int64(100))
	assert.GreaterOrEqual(t, int64(queryStats.FileIndexSize), expectedOperations)
	assert.GreaterOrEqual(t, int64(queryStats.CommitIndexSize), expectedOperations)
	
	// Check garbage collection
	gcStats := repo.gc.GetStats()
	assert.GreaterOrEqual(t, gcStats.TotalRuns, int64(1))
	
	t.Logf("✅ High Concurrency Integration Results:")
	t.Logf("  Workers: %d, Operations per worker: %d", numWorkers, operationsPerWorker)
	t.Logf("  Atomic Operations: %d commits, %d aborts", stats.CommittedTransactions, stats.AbortedTransactions)
	t.Logf("  Pub/Sub Events: %d published, %d delivered, %d errors", eventBusStats.TotalEvents, eventBusStats.TotalDelivered, eventBusStats.TotalErrors)
	t.Logf("  Query Engine: %d queries processed, %d files indexed", queryStats.TotalQueries, queryStats.FileIndexSize)
	t.Logf("  Memory Compaction: %d runs, %d objects collected", gcStats.TotalRuns, gcStats.ObjectsCollected)
	t.Logf("  Query Performance: %.2fμs average", float64(queryStats.AverageQueryTime.Nanoseconds())/1000)
	t.Logf("  Cache Hit Rate: %.1f%%", queryStats.CacheStats.HitRate*100)
}

// TestFourSystemsIntegration_RealWorldScenario simulates a realistic application scenario
func TestFourSystemsIntegration_RealWorldScenario(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Start garbage collection  
	repo.gc.Start(context.Background())
	defer repo.gc.Stop()
	
	// Simulate building a web application with all systems active
	t.Log("Simulating real-world application development scenario...")
	
	var eventCount int64
	unsubscribe := repo.OnCommit(func(commitHash string, files []string) {
		atomic.AddInt64(&eventCount, 1)
		t.Logf("📝 Commit %s: %d files changed", commitHash[:8], len(files))
	})
	defer unsubscribe()
	
	// Phase 1: Initial project structure
	t.Log("Phase 1: Creating initial project structure")
	initialFiles := map[string]string{
		"package.json":           `{"name": "webapp", "version": "1.0.0"}`,
		"src/index.js":           `import App from './App';\nReactDOM.render(<App />, root);`,
		"src/App.js":             `import React from 'react';\nexport default function App() {}`,
		"src/components/Header.js": `import React from 'react';\nexport default Header;`,
		"src/utils/auth.js":      `export function authenticate(token) {}`,
		"server/index.js":        `const express = require('express');\nconst app = express();`,
		"server/routes/auth.js":  `module.exports = function(app) {}`,
		"config/database.js":     `module.exports = { host: 'localhost' };`,
	}
	
	updates := map[string][]byte{}
	for path, content := range initialFiles {
		updates[path] = []byte(content)
	}
	
	_, err := repo.AtomicMultiFileUpdate(updates, "Initial project setup")
	require.NoError(t, err)
	
	time.Sleep(100 * time.Millisecond)
	
	// Phase 2: Feature development with queries
	t.Log("Phase 2: Developing features with real-time queries")
	
	// Developer searches for React components
	reactFiles, err := repo.FindFiles("*.js")
	require.NoError(t, err)
	t.Logf("Found %d JavaScript files", len(reactFiles))
	
	// Search for authentication related code
	authFiles, err := repo.SearchFileContent("auth")
	require.NoError(t, err)
	t.Logf("Found %d files containing 'auth'", len(authFiles))
	
	// Add new feature files
	featureUpdates := map[string][]byte{
		"src/components/Login.js":    []byte(`import React from 'react';\nexport default Login;`),
		"src/components/Dashboard.js": []byte(`import React from 'react';\nexport default Dashboard;`),
		"server/routes/users.js":     []byte(`module.exports = function(app) { /* users API */ };`),
		"tests/auth.test.js":         []byte(`describe('Authentication', () => { /* tests */ });`),
	}
	
	_, err = repo.AtomicMultiFileUpdate(featureUpdates, "Add authentication features")
	require.NoError(t, err)
	
	time.Sleep(100 * time.Millisecond)
	
	// Phase 3: Bug fixes and refactoring
	t.Log("Phase 3: Bug fixes and code refactoring")
	
	// Update existing files
	_, err = repo.AtomicUpdateFile("src/utils/auth.js", 
		[]byte(`export function authenticate(token) {\n  return validateToken(token);\n}`), 
		"Fix authentication validation")
	require.NoError(t, err)
	
	_, err = repo.AtomicUpdateFile("src/App.js",
		[]byte(`import React from 'react';\nimport Header from './components/Header';\nexport default function App() { return <div><Header /></div>; }`),
		"Integrate Header component")
	require.NoError(t, err)
	
	time.Sleep(100 * time.Millisecond)
	
	// Phase 4: Performance testing with queries
	t.Log("Phase 4: Performance testing and monitoring")
	
	startTime := time.Now()
	
	// Simulate developer workflow with concurrent queries
	var wg sync.WaitGroup
	queryTypes := []string{"components", "routes", "auth", "tests", "config"}
	
	for _, queryType := range queryTypes {
		wg.Add(1)
		go func(qt string) {
			defer wg.Done()
			
			switch qt {
			case "components":
				repo.FindFiles("src/components/*.js")
			case "routes": 
				repo.FindFiles("server/routes/*.js")
			case "auth":
				repo.SearchFileContent("auth")
			case "tests":
				repo.FindFiles("tests/*.js")
			case "config":
				repo.FindFiles("config/*")
			}
		}(queryType)
	}
	
	wg.Wait()
	queryTime := time.Since(startTime)
	
	// Force garbage collection
	repo.gc.ForceCompaction(CompactOptions{Force: true})
	
	// Verify all systems worked correctly
	totalFiles := len(initialFiles) + len(featureUpdates)
	expectedCommits := 4 // initial + features + 2 bug fixes
	
	// Check atomic operations
	stats := repo.GetTransactionStats()
	assert.Equal(t, int64(expectedCommits), stats.CommittedTransactions)
	assert.Equal(t, int64(0), stats.AbortedTransactions)
	
	// Check events
	assert.Equal(t, int64(expectedCommits), atomic.LoadInt64(&eventCount))
	
	// Check query engine
	queryStats := repo.GetQueryEngineStats()
	assert.True(t, int64(queryStats.FileIndexSize) >= int64(totalFiles))
	assert.True(t, queryStats.TotalQueries >= int64(len(queryTypes)))
	
	// Check garbage collection
	gcStats := repo.gc.GetStats()
	assert.GreaterOrEqual(t, gcStats.TotalRuns, int64(1))
	
	// Performance assertions
	assert.True(t, queryTime < 100*time.Millisecond, "All queries should complete quickly")
	assert.True(t, queryStats.AverageQueryTime < 1*time.Millisecond, "Individual queries should be fast")
	
	t.Logf("✅ Real-World Scenario Results:")
	t.Logf("  Total Files: %d", totalFiles)
	t.Logf("  Total Commits: %d", expectedCommits)
	t.Logf("  Query Performance: All %d queries completed in %v", len(queryTypes), queryTime)
	t.Logf("  Average Query Time: %v", queryStats.AverageQueryTime)
	t.Logf("  Cache Hit Rate: %.1f%%", queryStats.CacheStats.HitRate*100)
	t.Logf("  Event Delivery: 100%% success rate")
	t.Logf("  Memory Management: %d GC runs", gcStats.TotalRuns)
	t.Logf("  System Status: All four systems integrated successfully! 🎉")
}

// TestFourSystemsIntegration_StressTest pushes all systems to their limits
func TestFourSystemsIntegration_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}
	
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Start garbage collection
	repo.gc.Start(context.Background())
	defer repo.gc.Stop()
	
	t.Log("🔥 Starting four-systems stress test...")
	
	var totalEvents int64
	unsubscribe := repo.OnCommit(func(commitHash string, files []string) {
		atomic.AddInt64(&totalEvents, 1)
	})
	defer unsubscribe()
	
	// Stress test parameters
	numWorkers := 50
	operationsPerWorker := 50
	numQueryWorkers := 10
	queriesPerWorker := 100
	
	startTime := time.Now()
	
	var wg sync.WaitGroup
	
	// Launch atomic operation workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for j := 0; j < operationsPerWorker; j++ {
				filename := fmt.Sprintf("stress/%d/file_%d.txt", workerID, j)
				content := fmt.Sprintf("Stress test content from worker %d, operation %d\nThis is test data for performance evaluation", workerID, j)
				message := fmt.Sprintf("Stress test W%d O%d", workerID, j)
				
				_, err := repo.AtomicCreateFile(filename, []byte(content), message)
				if err != nil {
					t.Errorf("Worker %d operation %d failed: %v", workerID, j, err)
					return
				}
				
				// Add GC pressure
				for k := 0; k < 3; k++ {
					objHash := fmt.Sprintf("stress_%d_%d_%d", workerID, j, k)
					repo.gc.AddReference(objHash)
					if k%2 == 0 {
						repo.gc.RemoveReference(objHash)
					}
				}
			}
		}(i)
	}
	
	// Launch query workers
	for i := 0; i < numQueryWorkers; i++ {
		wg.Add(1)
		go func(queryWorkerID int) {
			defer wg.Done()
			
			for j := 0; j < queriesPerWorker; j++ {
				switch j % 5 {
				case 0:
					repo.FindFiles("*.txt")
				case 1:
					repo.SearchFileContent("stress")
				case 2:
					repo.FindFiles("stress/*")
				case 3:
					repo.SearchCommitMessages("Stress")
				case 4:
					repo.GetLargestFiles(10)
				}
				
				time.Sleep(5 * time.Millisecond) // Small delay to prevent overwhelming
			}
		}(i)
	}
	
	wg.Wait()
	
	duration := time.Since(startTime)
	
	// Wait for all async operations
	time.Sleep(500 * time.Millisecond)
	
	// Force final garbage collection
	repo.gc.ForceCompaction(CompactOptions{Force: true})
	
	// Calculate expected results
	expectedOperations := int64(numWorkers * operationsPerWorker)
	expectedQueries := int64(numQueryWorkers * queriesPerWorker)
	
	// Verify all systems performed correctly under stress
	stats := repo.GetTransactionStats()
	assert.Equal(t, expectedOperations, stats.CommittedTransactions, "All atomic operations should succeed")
	assert.Equal(t, int64(0), stats.AbortedTransactions, "No operations should abort under stress")
	
	eventBusStats := repo.eventBus.GetStats()
	assert.GreaterOrEqual(t, eventBusStats.TotalEvents, expectedOperations, "All events should be published")
	assert.GreaterOrEqual(t, eventBusStats.TotalDelivered, expectedOperations, "All events should be delivered")
	assert.Equal(t, int64(0), eventBusStats.TotalErrors, "No events should be dropped under stress")
	
	queryStats := repo.GetQueryEngineStats()
	assert.GreaterOrEqual(t, queryStats.TotalQueries, expectedQueries, "All queries should be processed")
	assert.GreaterOrEqual(t, int64(queryStats.FileIndexSize), expectedOperations, "All files should be indexed")
	assert.GreaterOrEqual(t, int64(queryStats.CommitIndexSize), expectedOperations, "All commits should be indexed")
	
	gcStats := repo.gc.GetStats()
	assert.GreaterOrEqual(t, gcStats.TotalRuns, int64(1), "Garbage collection should run")
	assert.GreaterOrEqual(t, gcStats.ObjectsCollected, int64(100), "Should collect many objects")
	
	// Performance assertions
	opsPerSecond := float64(expectedOperations) / duration.Seconds()
	assert.Greater(t, opsPerSecond, 500.0, "Should handle at least 500 ops/sec under stress")
	assert.Less(t, queryStats.AverageQueryTime, 10*time.Millisecond, "Queries should remain fast under stress")
	
	t.Logf("🎯 Stress Test Results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Workers: %d atomic + %d query workers", numWorkers, numQueryWorkers)
	t.Logf("  Operations: %d atomic operations (%.0f ops/sec)", expectedOperations, opsPerSecond)
	t.Logf("  Queries: %d queries processed", queryStats.TotalQueries)
	t.Logf("  Events: %d/%d delivered (%.1f%% success)", eventBusStats.TotalDelivered, eventBusStats.TotalEvents, 
		float64(eventBusStats.TotalDelivered)/float64(eventBusStats.TotalEvents)*100)
	t.Logf("  Query Performance: %v average (%.1f%% cache hit)", queryStats.AverageQueryTime, queryStats.CacheStats.HitRate*100)
	t.Logf("  Memory Management: %d GC runs, %d objects collected", gcStats.TotalRuns, gcStats.ObjectsCollected)
	t.Logf("  ✅ ALL FOUR SYSTEMS PASSED STRESS TEST!")
}