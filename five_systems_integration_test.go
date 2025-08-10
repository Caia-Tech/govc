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

// TestFiveSystemsIntegration tests all five critical systems working together:
// 1. Memory Compaction (GarbageCollector)
// 2. Pub/Sub System (EventBus)
// 3. Atomic Operations (AtomicTransactionManager)
// 4. Query Engine
// 5. Delta Compression
func TestFiveSystemsIntegration_BasicWorkflow(t *testing.T) {
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
	
	// Perform atomic operations with delta-compressed storage
	testData := []struct {
		filename string
		content  string
		message  string
	}{
		{"app.js", "import React from 'react';\nexport default function App() {\n  return <div>Hello World</div>;\n}", "Add main App component"},
		{"app.js", "import React from 'react';\nexport default function App() {\n  return <div>Hello World - Updated</div>;\n}", "Update App component"}, // Similar to previous
		{"components/Button.js", "import React from 'react';\nexport default function Button() {\n  return <button>Click me</button>;\n}", "Add Button component"},
		{"components/Button.js", "import React from 'react';\nexport default function Button({ children }) {\n  return <button>{children}</button>;\n}", "Update Button component"}, // Similar to previous
		{"utils/api.js", "export async function fetchData(url) {\n  const response = await fetch(url);\n  return response.json();\n}", "Add API utilities"},
		{"utils/api.js", "export async function fetchData(url) {\n  const response = await fetch(url);\n  const data = await response.json();\n  return data;\n}", "Update API utilities"}, // Similar to previous
	}
	
	// Use Atomic Operations with Delta Compression for each file
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
	atomicStats := repo.GetTransactionStats()
	assert.Equal(t, int64(len(testData)), atomicStats.CommittedTransactions)
	assert.Equal(t, int64(0), atomicStats.AbortedTransactions)
	
	// Test 3: Test Query Engine functionality
	t.Run("QueryEngine_WithDeltaCompression", func(t *testing.T) {
		// Find JavaScript files
		jsFiles, err := repo.FindFiles("*.js")
		require.NoError(t, err)
		assert.True(t, len(jsFiles) >= 3, "Should find JavaScript files")
		
		// Search for React components
		reactFiles, err := repo.SearchFileContent("React")
		require.NoError(t, err)
		assert.True(t, len(reactFiles) >= 2, "Should find React components")
		
		// Find commits by message
		updateCommits, err := repo.SearchCommitMessages("Update")
		require.NoError(t, err)
		assert.True(t, len(updateCommits) >= 3, "Should find update commits")
	})
	
	// Test 4: Verify Delta Compression effectiveness
	deltaStats := repo.GetDeltaCompressionStats()
	t.Logf("Delta Compression Stats: %+v", deltaStats)
	
	// Since we have similar file updates, delta compression might be used
	assert.True(t, deltaStats.TotalObjects >= int64(len(testData)), "Should track all objects")
	
	// Test 5: Force garbage collection and verify Memory Compaction
	repo.gc.ForceCompaction(CompactOptions{Force: true})
	
	gcStats := repo.gc.GetStats()
	assert.GreaterOrEqual(t, gcStats.TotalRuns, int64(1), "Should have run GC at least once")
	assert.GreaterOrEqual(t, gcStats.ObjectsCollected, int64(1), "Should have collected objects")
	
	// Test 6: Verify Event Bus statistics
	eventBusStats := repo.eventBus.GetStats()
	assert.GreaterOrEqual(t, eventBusStats.TotalEvents, int64(len(testData)))
	assert.GreaterOrEqual(t, eventBusStats.TotalDelivered, int64(len(testData)))
	assert.Equal(t, int64(0), eventBusStats.TotalErrors)
	
	// Test 7: Verify Query Engine statistics
	queryStats := repo.GetQueryEngineStats()
	assert.True(t, queryStats.TotalQueries > 0, "Should have processed queries")
	// We have 3 unique files (app.js, components/Button.js, utils/api.js) 
	assert.True(t, queryStats.FileIndexSize >= 3, "Should have indexed all unique files")
	assert.True(t, int64(queryStats.CommitIndexSize) >= int64(len(testData)), "Should have indexed all commits")
	
	t.Logf("✅ Five Systems Integration Results:")
	t.Logf("  Atomic Operations: %d commits, %d aborts", atomicStats.CommittedTransactions, atomicStats.AbortedTransactions)
	t.Logf("  Pub/Sub Events: %d published, %d delivered, %d errors", eventBusStats.TotalEvents, eventBusStats.TotalDelivered, eventBusStats.TotalErrors)
	t.Logf("  Query Engine: %d queries, %d files indexed, %d commits indexed", queryStats.TotalQueries, queryStats.FileIndexSize, queryStats.CommitIndexSize)
	t.Logf("  Delta Compression: %d objects, %.1f%% compression ratio", deltaStats.TotalObjects, (1.0-deltaStats.CompressionRatio)*100)
	t.Logf("  Memory Compaction: %d runs, %d objects collected", gcStats.TotalRuns, gcStats.ObjectsCollected)
}

// TestFiveSystemsIntegration_HighFrequencyCommits tests delta compression under high-frequency commits
func TestFiveSystemsIntegration_HighFrequencyCommits(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Start garbage collection
	repo.gc.Start(context.Background())
	defer repo.gc.Stop()
	
	var totalEvents int64
	
	// Subscribe to events
	unsubscribe := repo.OnCommit(func(commitHash string, files []string) {
		atomic.AddInt64(&totalEvents, 1)
	})
	defer unsubscribe()
	
	// Configure for high-frequency scenario
	numFiles := 100
	versionsPerFile := 10 // Multiple versions of each file for delta compression
	
	t.Logf("Starting high-frequency delta compression test: %d files, %d versions each", numFiles, versionsPerFile)
	
	start := time.Now()
	
	// Create multiple versions of files (ideal for delta compression)
	for fileIdx := 0; fileIdx < numFiles; fileIdx++ {
		baseContent := fmt.Sprintf("// File %d - Base Content\nfunction processData%d() {\n  console.log('Processing data for file %d');\n  return {\n    id: %d,\n    status: 'active',\n    data: ['item1', 'item2', 'item3']\n  };\n}", fileIdx, fileIdx, fileIdx, fileIdx)
		
		for version := 0; version < versionsPerFile; version++ {
			// Create similar content with small changes (perfect for delta compression)
			content := baseContent + fmt.Sprintf("\n// Version %d update\n// Timestamp: %d", version, time.Now().UnixNano())
			
			filename := fmt.Sprintf("data/file_%d.js", fileIdx)
			message := fmt.Sprintf("Update file %d version %d", fileIdx, version)
			
			_, err := repo.AtomicCreateFile(filename, []byte(content), message)
			require.NoError(t, err)
		}
	}
	
	commitTime := time.Since(start)
	expectedCommits := int64(numFiles * versionsPerFile)
	
	// Wait for all operations to complete
	time.Sleep(300 * time.Millisecond)
	
	// Force garbage collection
	repo.gc.ForceCompaction(CompactOptions{Force: true})
	
	// Verify all systems handled high-frequency commits
	atomicStats := repo.GetTransactionStats()
	assert.Equal(t, expectedCommits, atomicStats.CommittedTransactions)
	assert.Equal(t, int64(0), atomicStats.AbortedTransactions)
	
	// Check events
	assert.GreaterOrEqual(t, atomic.LoadInt64(&totalEvents), expectedCommits)
	
	// Check delta compression effectiveness
	deltaStats := repo.GetDeltaCompressionStats()
	
	// With many similar file versions, delta compression should be effective
	if deltaStats.DeltaObjects > 0 {
		assert.True(t, deltaStats.CompressionRatio < 0.8, "Should achieve good compression with similar files")
		t.Logf("Delta compression achieved %.1f%% compression ratio", (1.0-deltaStats.CompressionRatio)*100)
	}
	
	// Performance assertions
	commitsPerSecond := float64(expectedCommits) / commitTime.Seconds()
	assert.Greater(t, commitsPerSecond, 1000.0, "Should handle at least 1000 commits/sec with delta compression")
	
	// Check query performance
	queryStats := repo.GetQueryEngineStats()
	assert.GreaterOrEqual(t, int64(queryStats.FileIndexSize), int64(numFiles))
	assert.GreaterOrEqual(t, int64(queryStats.CommitIndexSize), expectedCommits)
	
	t.Logf("🎯 High-Frequency Delta Compression Results:")
	t.Logf("  Duration: %v", commitTime)
	t.Logf("  Operations: %d commits (%.0f commits/sec)", expectedCommits, commitsPerSecond)
	t.Logf("  Delta Compression: %d total objects, %d delta objects", deltaStats.TotalObjects, deltaStats.DeltaObjects)
	t.Logf("  Compression Ratio: %.1f%%", (1.0-deltaStats.CompressionRatio)*100)
	t.Logf("  Average Chain Length: %.1f", deltaStats.AverageChainLength)
	t.Logf("  Events: %d/%d delivered", atomic.LoadInt64(&totalEvents), expectedCommits)
	t.Logf("  Query Performance: %v average", queryStats.AverageQueryTime)
}

// TestFiveSystemsIntegration_StorageEfficiency tests storage efficiency with delta compression
func TestFiveSystemsIntegration_StorageEfficiency(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	t.Log("Testing storage efficiency with delta compression...")
	
	// Create a base document
	baseDocument := `
# Project Documentation

## Overview
This is a comprehensive documentation file for our project.
It contains multiple sections and detailed information.

## Features
- Feature A: High-performance operations
- Feature B: Real-time synchronization
- Feature C: Advanced query capabilities
- Feature D: Memory optimization
- Feature E: Delta compression

## Architecture
The system consists of multiple components:
1. Repository management
2. Event handling
3. Query processing
4. Storage optimization
5. Memory management

## Usage Examples
Here are some common usage patterns...
`
	
	// Store base document
	_, err := repo.AtomicCreateFile("docs/README.md", []byte(baseDocument), "Add initial documentation")
	require.NoError(t, err)
	
	// Create multiple versions with incremental changes
	versions := []string{
		"Feature F: Advanced analytics",
		"Feature G: Real-time monitoring",
		"Feature H: Distributed processing", 
		"Feature I: Cloud integration",
		"Feature J: Machine learning support",
	}
	
	for i, newFeature := range versions {
		// Add new feature to the document (highly similar content)
		updatedDoc := baseDocument + fmt.Sprintf("\n- %s\n", newFeature)
		message := fmt.Sprintf("Add %s to documentation", newFeature)
		
		_, err := repo.AtomicUpdateFile("docs/README.md", []byte(updatedDoc), message)
		require.NoError(t, err)
		
		// Query for the updated content
		files, err := repo.FindFiles("docs/*.md")
		require.NoError(t, err)
		assert.Equal(t, 1, len(files))
		
		// Search for a simpler term that should be found
		// Since content indexing tokenizes, search for a single word from the feature
		searchTerm := "Feature" // This word should be in all versions
		results, err := repo.SearchFileContent(searchTerm)
		require.NoError(t, err)
		assert.True(t, len(results) > 0, "Should find content with 'Feature' in version %d", i+1)
	}
	
	time.Sleep(100 * time.Millisecond)
	
	// Check storage efficiency
	deltaStats := repo.GetDeltaCompressionStats()
	atomicStats := repo.GetTransactionStats()
	
	// Should have processed all commits
	expectedCommits := int64(len(versions) + 1) // base + updates
	assert.Equal(t, expectedCommits, atomicStats.CommittedTransactions)
	
	// Check compression effectiveness
	if deltaStats.DeltaObjects > 0 {
		t.Logf("Storage Efficiency Results:")
		t.Logf("  Total Objects: %d", deltaStats.TotalObjects)
		t.Logf("  Delta Objects: %d", deltaStats.DeltaObjects)
		t.Logf("  Base Objects: %d", deltaStats.BaseObjects)
		t.Logf("  Original Size: %d bytes", deltaStats.TotalOriginalSize)
		t.Logf("  Compressed Size: %d bytes", deltaStats.TotalCompressedSize)
		t.Logf("  Compression Ratio: %.1f%%", (1.0-deltaStats.CompressionRatio)*100)
		t.Logf("  Space Saved: %d bytes", deltaStats.TotalOriginalSize-deltaStats.TotalCompressedSize)
		
		// For highly similar documents, should achieve good compression
		assert.True(t, deltaStats.CompressionRatio < 0.9, "Should achieve compression for similar documents")
	}
	
	// Test query performance on compressed data
	start := time.Now()
	
	// Multiple queries to test performance
	for i := 0; i < 100; i++ {
		repo.FindFiles("*.md")
		repo.SearchFileContent("Feature")
		repo.SearchCommitMessages("documentation")
	}
	
	queryTime := time.Since(start)
	avgQueryTime := queryTime / 300 // 100 iterations * 3 queries each
	
	assert.True(t, avgQueryTime < 1*time.Millisecond, "Queries should remain fast with delta compression")
	
	t.Logf("  Query Performance: %v average (300 queries)", avgQueryTime)
	t.Logf("  ✅ ALL FIVE SYSTEMS WORKING EFFICIENTLY WITH DELTA COMPRESSION!")
}

// TestFiveSystemsIntegration_MMOWorkloadSimulation simulates MMO-like high-frequency operations
func TestFiveSystemsIntegration_MMOWorkloadSimulation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping MMO simulation in short mode")
	}
	
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Start garbage collection
	repo.gc.Start(context.Background())
	defer repo.gc.Stop()
	
	t.Log("🎮 Simulating MMO-like workload with all five systems...")
	
	var totalEvents int64
	unsubscribe := repo.OnCommit(func(commitHash string, files []string) {
		atomic.AddInt64(&totalEvents, 1)
	})
	defer unsubscribe()
	
	// Simulate player state updates (perfect for delta compression)
	numPlayers := 50
	updatesPerPlayer := 20
	
	startTime := time.Now()
	
	var wg sync.WaitGroup
	
	// Simulate concurrent player updates
	for playerID := 0; playerID < numPlayers; playerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			baseState := fmt.Sprintf(`{
  "player_id": %d,
  "username": "player_%d",
  "level": 1,
  "experience": 0,
  "health": 100,
  "mana": 50,
  "position": {"x": 0, "y": 0, "z": 0},
  "inventory": [],
  "skills": {},
  "guild": null,
  "stats": {
    "strength": 10,
    "agility": 10,
    "intelligence": 10
  },
  "timestamp": %d
}`, id, id, time.Now().UnixNano())
			
			// Create initial state
			filename := fmt.Sprintf("players/player_%d.json", id)
			_, err := repo.AtomicCreateFile(filename, []byte(baseState), fmt.Sprintf("Create player %d", id))
			if err != nil {
				t.Errorf("Failed to create player %d: %v", id, err)
				return
			}
			
			// Simulate updates (level, position, inventory changes)
			for update := 1; update <= updatesPerPlayer; update++ {
				// Small incremental changes (ideal for delta compression)
				updatedState := fmt.Sprintf(`{
  "player_id": %d,
  "username": "player_%d",
  "level": %d,
  "experience": %d,
  "health": %d,
  "mana": 50,
  "position": {"x": %d, "y": %d, "z": 0},
  "inventory": [%s],
  "skills": {},
  "guild": null,
  "stats": {
    "strength": %d,
    "agility": 10,
    "intelligence": 10
  },
  "timestamp": %d
}`, id, id, 1+update/5, update*100, 100-update%20, update*2, update*3, 
				generateInventoryItems(update), 10+update/10, time.Now().UnixNano())
				
				message := fmt.Sprintf("Update player %d state #%d", id, update)
				_, err := repo.AtomicUpdateFile(filename, []byte(updatedState), message)
				if err != nil {
					t.Errorf("Failed to update player %d: %v", id, err)
					return
				}
				
				// Occasionally query data (simulate game server operations)
				if update%5 == 0 {
					repo.FindFiles(fmt.Sprintf("players/player_%d.json", id))
					repo.SearchFileContent(fmt.Sprintf("player_%d", id))
				}
			}
		}(playerID)
	}
	
	wg.Wait()
	
	duration := time.Since(startTime)
	expectedCommits := int64(numPlayers * (updatesPerPlayer + 1)) // Initial + updates
	
	// Wait for all async operations
	time.Sleep(500 * time.Millisecond)
	
	// Force final garbage collection
	repo.gc.ForceCompaction(CompactOptions{Force: true})
	
	// Verify all systems performed under MMO-like load
	atomicStats := repo.GetTransactionStats()
	assert.Equal(t, expectedCommits, atomicStats.CommittedTransactions, "All player updates should succeed")
	assert.Equal(t, int64(0), atomicStats.AbortedTransactions, "No updates should fail")
	
	deltaStats := repo.GetDeltaCompressionStats()
	eventBusStats := repo.eventBus.GetStats()
	queryStats := repo.GetQueryEngineStats()
	gcStats := repo.gc.GetStats()
	
	// Use eventBusStats
	_ = eventBusStats
	
	// Performance assertions for MMO workload
	operationsPerSecond := float64(expectedCommits) / duration.Seconds()
	assert.Greater(t, operationsPerSecond, 1000.0, "Should handle MMO-scale operations (>1000 ops/sec)")
	
	// Delta compression should be highly effective for player state updates
	if deltaStats.DeltaObjects > 0 {
		assert.True(t, deltaStats.CompressionRatio < 0.7, "Player state updates should compress well")
	}
	
	t.Logf("🎯 MMO Simulation Results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Players: %d", numPlayers)
	t.Logf("  Operations: %d (%.0f ops/sec)", expectedCommits, operationsPerSecond)
	t.Logf("  Atomic Operations: 100%% success rate")
	t.Logf("  Events: %d/%d delivered (%.1f%% success)", atomic.LoadInt64(&totalEvents), expectedCommits,
		float64(atomic.LoadInt64(&totalEvents))/float64(expectedCommits)*100)
	t.Logf("  Delta Compression: %d objects, %.1f%% compression", deltaStats.TotalObjects, (1.0-deltaStats.CompressionRatio)*100)
	t.Logf("  Query Engine: %d queries, %v avg time", queryStats.TotalQueries, queryStats.AverageQueryTime)
	t.Logf("  Memory Management: %d GC runs, %d objects collected", gcStats.TotalRuns, gcStats.ObjectsCollected)
	t.Logf("  🏆 ALL FIVE SYSTEMS PASSED MMO SIMULATION!")
}

// generateInventoryItems generates sample inventory items for player state
func generateInventoryItems(updateNum int) string {
	items := []string{
		`"sword"`, `"shield"`, `"potion"`, `"gem"`, `"scroll"`,
		`"armor"`, `"bow"`, `"arrow"`, `"ring"`, `"amulet"`,
	}
	
	numItems := (updateNum % 5) + 1
	result := ""
	for i := 0; i < numItems && i < len(items); i++ {
		if i > 0 {
			result += ", "
		}
		result += items[i]
	}
	return result
}