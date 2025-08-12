package search

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryEngine_Creation(t *testing.T) {
	repo := NewRepository()
	require.NotNil(t, repo.queryEngine)
	
	stats := repo.GetQueryEngineStats()
	assert.Equal(t, int64(0), stats.TotalQueries)
	assert.Equal(t, int64(0), stats.CacheHits)
	assert.Equal(t, int64(0), stats.IndexHits)
}

func TestQueryEngine_FileIndexing(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Add some test files
	testFiles := map[string][]byte{
		"config.json":       []byte(`{"name": "test", "version": 1}`),
		"src/main.go":       []byte(`package main\nfunc main() { fmt.Println("hello") }`),
		"docs/README.md":    []byte(`# Test Project\nThis is a test.`),
		"data/users.yaml":   []byte(`users:\n  - name: alice\n  - name: bob`),
	}
	
	for path, content := range testFiles {
		hash, err := repo.store.StoreBlob(content)
		require.NoError(t, err)
		repo.staging.Add(path, hash)
	}
	
	// Create a commit to trigger indexing
	_, err := repo.Commit("Add test files")
	require.NoError(t, err)
	
	// Wait for indexing to complete
	time.Sleep(100 * time.Millisecond)
	
	// Test path-based queries
	t.Run("QueryByPath", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:  QueryByPath,
			Query: "config.json",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, len(result.Files))
		assert.Equal(t, "config.json", result.Files[0].Path)
		assert.True(t, result.IndexUsed)
	})
	
	// Test pattern-based queries
	t.Run("QueryByPattern", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:  QueryByPattern,
			Query: "*.json",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, len(result.Files))
		assert.Equal(t, "config.json", result.Files[0].Path)
	})
	
	t.Run("QueryByPatternDirectory", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:  QueryByPattern,
			Query: "src/*",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, len(result.Files))
		assert.Equal(t, "src/main.go", result.Files[0].Path)
	})
	
	// Test content-based queries
	t.Run("QueryByContent", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:  QueryByContent,
			Query: "hello",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, len(result.Files))
		assert.Equal(t, "src/main.go", result.Files[0].Path)
	})
	
	t.Run("QueryByContentMultipleWords", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:  QueryByContent,
			Query: "test project",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, len(result.Files))
		assert.Equal(t, "docs/README.md", result.Files[0].Path)
	})
}

func TestQueryEngine_CommitIndexing(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Create several test commits
	commits := []struct {
		files   map[string][]byte
		message string
	}{
		{
			files:   map[string][]byte{"file1.txt": []byte("content 1")},
			message: "Initial commit with feature A",
		},
		{
			files:   map[string][]byte{"file2.txt": []byte("content 2")},
			message: "Add feature B implementation",
		},
		{
			files:   map[string][]byte{"file3.txt": []byte("content 3")},
			message: "Bug fix for issue #123",
		},
	}
	
	for _, commit := range commits {
		for path, content := range commit.files {
			hash, err := repo.store.StoreBlob(content)
			require.NoError(t, err)
			repo.staging.Add(path, hash)
		}
		_, err := repo.Commit(commit.message)
		require.NoError(t, err)
	}
	
	// Wait for indexing
	time.Sleep(100 * time.Millisecond)
	
	// Test commit message queries
	t.Run("QueryByCommitMessage", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:  QueryByCommit,
			Query: "feature",
		})
		require.NoError(t, err)
		assert.Equal(t, 2, len(result.Commits)) // Two commits have "feature" in message
	})
	
	t.Run("QueryByCommitMessageSpecific", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:  QueryByCommit,
			Query: "bug fix",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, len(result.Commits))
		assert.Contains(t, strings.ToLower(result.Commits[0].Message), "bug fix")
	})
}

func TestQueryEngine_SortingAndPagination(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Create test files with different sizes and names
	testFiles := map[string][]byte{
		"a_large.txt":  make([]byte, 1000), // 1000 bytes
		"b_medium.txt": make([]byte, 500),  // 500 bytes
		"c_small.txt":  make([]byte, 100),  // 100 bytes
		"d_tiny.txt":   make([]byte, 50),   // 50 bytes
	}
	
	for path, content := range testFiles {
		hash, err := repo.store.StoreBlob(content)
		require.NoError(t, err)
		repo.staging.Add(path, hash)
	}
	_, err := repo.Commit("Add files with different sizes")
	require.NoError(t, err)
	
	time.Sleep(100 * time.Millisecond)
	
	// Test sorting by path
	t.Run("SortByPath", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:      QueryByPattern,
			Query:     "*.txt",
			SortBy:    "path",
			SortOrder: "asc",
		})
		require.NoError(t, err)
		assert.Equal(t, 4, len(result.Files))
		assert.Equal(t, "a_large.txt", result.Files[0].Path)
		assert.Equal(t, "d_tiny.txt", result.Files[3].Path)
	})
	
	// Test sorting by size
	t.Run("SortBySize", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:      QueryByPattern,
			Query:     "*.txt",
			SortBy:    "size",
			SortOrder: "desc",
		})
		require.NoError(t, err)
		assert.Equal(t, 4, len(result.Files))
		assert.Equal(t, "a_large.txt", result.Files[0].Path) // Largest first
		assert.Equal(t, "d_tiny.txt", result.Files[3].Path)  // Smallest last
		assert.True(t, result.Files[0].Size >= result.Files[1].Size)
	})
	
	// Test pagination
	t.Run("Pagination", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:   QueryByPattern,
			Query:  "*.txt",
			Limit:  2,
			Offset: 0,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, len(result.Files))
		assert.Equal(t, 4, result.Total) // Total should still be 4
		
		// Get next page
		result2, err := repo.Query(&QueryRequest{
			Type:   QueryByPattern,
			Query:  "*.txt",
			Limit:  2,
			Offset: 2,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, len(result2.Files))
		
		// Ensure different files in each page
		assert.NotEqual(t, result.Files[0].Path, result2.Files[0].Path)
	})
}

func TestQueryEngine_Caching(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Add test file
	hash, err := repo.store.StoreBlob([]byte("test content"))
	require.NoError(t, err)
	repo.staging.Add("test.txt", hash)
	_, err = repo.Commit("Add test file")
	require.NoError(t, err)
	
	time.Sleep(100 * time.Millisecond)
	
	// First query - should miss cache
	result1, err := repo.Query(&QueryRequest{
		Type:  QueryByPath,
		Query: "test.txt",
	})
	require.NoError(t, err)
	assert.False(t, result1.CacheHit)
	
	// Second identical query - should hit cache
	result2, err := repo.Query(&QueryRequest{
		Type:  QueryByPath,
		Query: "test.txt",
	})
	require.NoError(t, err)
	assert.True(t, result2.CacheHit)
	
	// Verify cache statistics
	stats := repo.GetQueryEngineStats()
	assert.True(t, stats.CacheStats.Hits > 0)
	assert.True(t, stats.CacheStats.HitRate > 0)
}

func TestQueryEngine_HashQueries(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Add test file
	content := []byte("test content for hash query")
	hash, err := repo.store.StoreBlob(content)
	require.NoError(t, err)
	repo.staging.Add("hashtest.txt", hash)
	commit, err := repo.Commit("Add file for hash test")
	require.NoError(t, err)
	
	time.Sleep(100 * time.Millisecond)
	
	// Test file hash query
	t.Run("QueryByFileHash", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:  QueryByHash,
			Query: hash,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, len(result.Files))
		assert.Equal(t, "hashtest.txt", result.Files[0].Path)
		assert.Equal(t, hash, result.Files[0].Hash)
	})
	
	// Test commit hash query
	t.Run("QueryByCommitHash", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:  QueryByHash,
			Query: commit.Hash(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, len(result.Commits))
		assert.Equal(t, commit.Hash(), result.Commits[0].Hash)
	})
}

func TestQueryEngine_RepositoryConvenienceMethods(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Setup test data
	testFiles := map[string][]byte{
		"config.json":     []byte(`{"name": "test"}`),
		"src/main.go":     []byte(`package main\nfunc main() {}`),
		"docs/README.md":  []byte(`# Test\nDocumentation`),
		"large_file.txt":  make([]byte, 2000), // 2KB file
	}
	
	for path, content := range testFiles {
		hash, err := repo.store.StoreBlob(content)
		require.NoError(t, err)
		repo.staging.Add(path, hash)
	}
	_, err := repo.Commit("Setup test files")
	require.NoError(t, err)
	
	time.Sleep(100 * time.Millisecond)
	
	// Test convenience methods
	t.Run("FindFiles", func(t *testing.T) {
		files, err := repo.FindFiles("*.json")
		require.NoError(t, err)
		assert.Equal(t, 1, len(files))
		assert.Equal(t, "config.json", files[0].Path)
	})
	
	t.Run("FindFileByPath", func(t *testing.T) {
		file, err := repo.FindFileByPath("src/main.go")
		require.NoError(t, err)
		assert.Equal(t, "src/main.go", file.Path)
		
		// Test non-existent file
		_, err = repo.FindFileByPath("nonexistent.txt")
		assert.Error(t, err)
	})
	
	t.Run("SearchFileContent", func(t *testing.T) {
		files, err := repo.SearchFileContent("main")
		require.NoError(t, err)
		assert.Equal(t, 1, len(files))
		assert.Equal(t, "src/main.go", files[0].Path)
	})
	
	t.Run("GetLargestFiles", func(t *testing.T) {
		files, err := repo.GetLargestFiles(2)
		require.NoError(t, err)
		assert.Equal(t, 2, len(files))
		// Should be sorted by size, largest first
		assert.True(t, files[0].Size >= files[1].Size)
	})
}

func TestQueryEngine_QueryBuilder(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Setup test data
	hash, err := repo.store.StoreBlob([]byte("test content"))
	require.NoError(t, err)
	repo.staging.Add("test.txt", hash)
	_, err = repo.Commit("Add test file")
	require.NoError(t, err)
	
	time.Sleep(100 * time.Millisecond)
	
	// Test fluent query builder interface
	t.Run("FluentInterface", func(t *testing.T) {
		files, err := repo.NewQueryBuilder().
			Files().
			WithPattern("*.txt").
			WithLimit(10).
			SortByPath().
			ExecuteFiles()
		require.NoError(t, err)
		assert.Equal(t, 1, len(files))
		assert.Equal(t, "test.txt", files[0].Path)
	})
	
	t.Run("QueryBuilderWithFilters", func(t *testing.T) {
		result, err := repo.NewQueryBuilder().
			Files().
			WithPattern("*").
			WithFilter("min_size", 0).
			WithLimit(5).
			Execute()
		require.NoError(t, err)
		assert.True(t, len(result.Files) > 0)
	})
}

func TestQueryEngine_PerformanceAndStats(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Add multiple files to test performance
	for i := 0; i < 100; i++ {
		content := []byte(fmt.Sprintf("test content %d", i))
		hash, err := repo.store.StoreBlob(content)
		require.NoError(t, err)
		repo.staging.Add(fmt.Sprintf("file%d.txt", i), hash)
	}
	_, err := repo.Commit("Add 100 test files")
	require.NoError(t, err)
	
	time.Sleep(200 * time.Millisecond) // Wait for indexing
	
	// Perform multiple queries
	for i := 0; i < 10; i++ {
		_, err := repo.Query(&QueryRequest{
			Type:  QueryByPattern,
			Query: "*.txt",
		})
		require.NoError(t, err)
	}
	
	// Check statistics
	stats := repo.GetQueryEngineStats()
	assert.True(t, stats.TotalQueries > 0)
	assert.True(t, stats.FileIndexSize >= 100) // Should have indexed 100 files
	assert.True(t, stats.AverageQueryTime > 0)
	
	t.Logf("Query Engine Stats:")
	t.Logf("  Total Queries: %d", stats.TotalQueries)
	t.Logf("  Cache Hits: %d", stats.CacheHits)
	t.Logf("  Average Query Time: %v", stats.AverageQueryTime)
	t.Logf("  File Index Size: %d", stats.FileIndexSize)
	t.Logf("  Cache Hit Rate: %.2f%%", stats.CacheStats.HitRate*100)
}

func TestQueryEngine_ComplexQueries(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Setup complex test scenario
	testFiles := map[string][]byte{
		"frontend/src/components/Button.js":     []byte(`import React from 'react'; export default Button`),
		"frontend/src/components/Modal.js":      []byte(`import React from 'react'; export default Modal`),
		"backend/src/controllers/auth.go":       []byte(`package controllers\nfunc AuthHandler() {}`),
		"backend/src/controllers/users.go":      []byte(`package controllers\nfunc UsersHandler() {}`),
		"config/database.json":                  []byte(`{"host": "localhost", "port": 5432}`),
		"config/app.yaml":                       []byte(`name: myapp\nversion: 1.0.0`),
		"docs/api/auth.md":                      []byte(`# Authentication API\nDescribes auth endpoints`),
		"docs/api/users.md":                     []byte(`# Users API\nDescribes user endpoints`),
	}
	
	for path, content := range testFiles {
		hash, err := repo.store.StoreBlob(content)
		require.NoError(t, err)
		repo.staging.Add(path, hash)
	}
	_, err := repo.Commit("Add complex project structure")
	require.NoError(t, err)
	
	time.Sleep(200 * time.Millisecond)
	
	// Test complex pattern queries
	t.Run("FindJavaScriptFiles", func(t *testing.T) {
		files, err := repo.FindFiles("*.js")
		require.NoError(t, err)
		assert.Equal(t, 2, len(files))
		for _, file := range files {
			assert.True(t, strings.HasSuffix(file.Path, ".js"))
		}
	})
	
	t.Run("FindConfigFiles", func(t *testing.T) {
		// This would need actual implementation of multi-pattern matching
		jsonFiles, err := repo.FindFiles("*.json")
		require.NoError(t, err)
		yamlFiles, err := repo.FindFiles("*.yaml")
		require.NoError(t, err)
		
		totalConfigFiles := len(jsonFiles) + len(yamlFiles)
		assert.Equal(t, 2, totalConfigFiles)
	})
	
	t.Run("FindByDirectory", func(t *testing.T) {
		backendFiles, err := repo.FindFiles("backend/*")
		require.NoError(t, err)
		assert.True(t, len(backendFiles) >= 2)
		for _, file := range backendFiles {
			assert.True(t, strings.HasPrefix(file.Path, "backend/"))
		}
	})
	
	t.Run("ContentSearchAcrossFiles", func(t *testing.T) {
		files, err := repo.SearchFileContent("React")
		require.NoError(t, err)
		assert.Equal(t, 2, len(files))
		for _, file := range files {
			assert.True(t, strings.HasSuffix(file.Path, ".js"))
		}
	})
}

func TestQueryEngine_ErrorHandling(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Test invalid query types
	t.Run("InvalidQueryType", func(t *testing.T) {
		_, err := repo.Query(&QueryRequest{
			Type:  QueryType(999), // Invalid type
			Query: "test",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported query type")
	})
	
	// Test queries on empty repository
	t.Run("QueriesOnEmptyRepo", func(t *testing.T) {
		result, err := repo.Query(&QueryRequest{
			Type:  QueryByPath,
			Query: "nonexistent.txt",
		})
		require.NoError(t, err)
		assert.Equal(t, 0, len(result.Files))
		assert.Equal(t, 0, result.Total)
	})
	
	// Test invalid date range
	t.Run("InvalidDateRange", func(t *testing.T) {
		_, err := repo.Query(&QueryRequest{
			Type: QueryByDate,
			Filters: map[string]interface{}{
				"start_date": "invalid-date-format",
			},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid start_date format")
	})
}

func TestQueryEngine_ConcurrentAccess(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	// Add test data
	hash, err := repo.store.StoreBlob([]byte("concurrent test content"))
	require.NoError(t, err)
	repo.staging.Add("concurrent.txt", hash)
	_, err = repo.Commit("Add test file for concurrent access")
	require.NoError(t, err)
	
	time.Sleep(100 * time.Millisecond)
	
	// Run concurrent queries
	numGoroutines := 50
	results := make(chan *QueryResult, numGoroutines)
	errors := make(chan error, numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		go func() {
			result, err := repo.Query(&QueryRequest{
				Type:  QueryByPath,
				Query: "concurrent.txt",
			})
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}()
	}
	
	// Collect results
	successCount := 0
	errorCount := 0
	
	for i := 0; i < numGoroutines; i++ {
		select {
		case result := <-results:
			assert.Equal(t, 1, len(result.Files))
			assert.Equal(t, "concurrent.txt", result.Files[0].Path)
			successCount++
		case err := <-errors:
			t.Logf("Concurrent query error: %v", err)
			errorCount++
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent queries")
		}
	}
	
	assert.Equal(t, numGoroutines, successCount+errorCount)
	assert.True(t, successCount > errorCount*10) // Expect mostly successes
	
	// Verify no data corruption
	stats := repo.GetQueryEngineStats()
	assert.True(t, stats.TotalQueries >= int64(successCount))
}