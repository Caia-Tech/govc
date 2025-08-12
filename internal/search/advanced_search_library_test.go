package search

import (
	"testing"
	
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLibraryIntegration_AdvancedSearchAccessibility tests that all advanced search functions 
// are accessible from the Repository interface for library consumers
func TestLibraryIntegration_AdvancedSearchAccessibility(t *testing.T) {
	// Create a new repository (this is how library users would start)
	repo := NewRepository()
	
	// Initialize advanced search
	err := repo.InitializeAdvancedSearch()
	require.NoError(t, err)
	
	// Test that all advanced search methods are accessible
	t.Run("FullTextSearchAccessible", func(t *testing.T) {
		// This tests the method is accessible and has correct signature
		response, err := repo.FullTextSearch(&FullTextSearchRequest{
			Query: "test",
			Limit: 10,
		})
		// Method should be accessible and return valid response (even if empty)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, 0, len(response.Results)) // Empty repo, no results
	})
	
	t.Run("SQLQueryAccessible", func(t *testing.T) {
		// Test SQL query method accessibility
		result, err := repo.ExecuteSQLQuery("SELECT * FROM files")
		// Method should be accessible and return valid response (even if empty)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, len(result.Rows)) // Empty repo, no results
	})
	
	t.Run("SearchAggregationAccessible", func(t *testing.T) {
		// Test aggregation method accessibility
		response, err := repo.SearchWithAggregation(&AggregationRequest{
			Query: &FullTextSearchRequest{Query: "*"},
			GroupBy: []string{"extension"},
		})
		// Method should be accessible and return valid response (even if empty)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, 0, len(response.Groups)) // Empty repo, no groups
	})
	
	t.Run("SearchStatisticsAccessible", func(t *testing.T) {
		// Test statistics method accessibility
		stats, err := repo.GetSearchIndexStatistics()
		require.NoError(t, err)
		
		// Should return valid statistics even for empty index
		assert.NotNil(t, stats)
		assert.Contains(t, stats, "total_documents")
		assert.Equal(t, 0, stats["total_documents"])
	})
	
	t.Run("SearchSuggestionsAccessible", func(t *testing.T) {
		// Test suggestions method accessibility
		suggestions, err := repo.GetSearchSuggestions("te", 5)
		require.NoError(t, err)
		
		// Should return empty list for empty index
		assert.NotNil(t, suggestions)
		assert.Equal(t, 0, len(suggestions))
	})
	
	t.Run("RebuildIndexAccessible", func(t *testing.T) {
		// Test index rebuild method accessibility
		err := repo.RebuildSearchIndex()
		require.NoError(t, err)
	})
	
	t.Run("ConvenienceMethodsAccessible", func(t *testing.T) {
		// Test convenience methods are accessible
		response, err := repo.SearchCode("test", []string{"go"}, 10)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, 0, len(response.Results))
		
		response2, err := repo.SearchCodeWithHighlights("test", []string{"go"}, 10)
		require.NoError(t, err)
		assert.NotNil(t, response2)
		assert.Equal(t, 0, len(response2.Results))
		
		stats, err := repo.GetFileStatistics()
		require.NoError(t, err)
		assert.NotNil(t, stats)
		assert.Equal(t, 0, len(stats.Groups))
	})
}

// TestLibraryIntegration_AdvancedSearchTypes tests that all the required types
// are exported and accessible to library consumers
func TestLibraryIntegration_AdvancedSearchTypes(t *testing.T) {
	t.Run("RequestTypesAccessible", func(t *testing.T) {
		// Test that request types are accessible and can be constructed
		searchReq := &FullTextSearchRequest{
			Query:           "test query",
			FileTypes:       []string{"go", "js"},
			MaxSize:         1024000,
			MinScore:        0.1,
			IncludeContent:  true,
			HighlightLength: 200,
			Limit:           50,
			Offset:          0,
			SortBy:          "score",
		}
		assert.Equal(t, "test query", searchReq.Query)
		assert.Equal(t, []string{"go", "js"}, searchReq.FileTypes)
		
		aggReq := &AggregationRequest{
			Query:        searchReq,
			GroupBy:      []string{"extension"},
			Aggregations: []string{"count", "avg_size"},
			TimeRange:    "month",
		}
		assert.Equal(t, searchReq, aggReq.Query)
		assert.Equal(t, []string{"extension"}, aggReq.GroupBy)
	})
	
	t.Run("ResponseTypesAccessible", func(t *testing.T) {
		// Test that response types can be accessed (they should be returned by methods)
		repo := NewRepository()
		repo.InitializeAdvancedSearch()
		
		// Add a test document
		err := repo.advancedSearch.IndexDocument("test.go", []byte("package main\nfunc main() {}"))
		require.NoError(t, err)
		
		// Test response types are accessible
		response, err := repo.FullTextSearch(&FullTextSearchRequest{
			Query: "*", // Use wildcard to ensure we get results
			Limit: 10,
		})
		require.NoError(t, err)
		
		// Verify response structure is accessible
		assert.Greater(t, len(response.Results), 0)
		assert.Greater(t, response.Total, 0)
		assert.NotNil(t, response.Statistics)
		
		// Verify individual result structure is accessible
		result := response.Results[0]
		assert.NotNil(t, result.Document)
		assert.Greater(t, result.Score, 0.0)
		assert.Equal(t, "test.go", result.Document.Path)
	})
}

// TestLibraryIntegration_AdvancedSearchWorkflow tests a complete workflow
// that a library consumer might use
func TestLibraryIntegration_AdvancedSearchWorkflow(t *testing.T) {
	// Simulate a library user creating a repository and using advanced search
	repo := NewRepository()
	
	// Step 1: Initialize advanced search
	err := repo.InitializeAdvancedSearch()
	require.NoError(t, err)
	
	// Step 2: Add some content (simulate repository with files)
	testFiles := map[string]string{
		"main.go":      "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello World\")\n}",
		"utils.go":     "package main\n\nfunc helper() string {\n\treturn \"database connection\"\n}",
		"config.json":  "{\"database\": {\"host\": \"localhost\", \"port\": 5432}}",
		"README.md":    "# Project\n\nThis project demonstrates advanced search capabilities.",
	}
	
	for path, content := range testFiles {
		err := repo.advancedSearch.IndexDocument(path, []byte(content))
		require.NoError(t, err)
	}
	
	// Step 3: Perform full-text search
	searchResponse, err := repo.FullTextSearch(&FullTextSearchRequest{
		Query:          "database",
		IncludeContent: true,
		Limit:          10,
	})
	require.NoError(t, err)
	assert.Greater(t, len(searchResponse.Results), 0)
	
	// Step 4: Execute SQL query
	sqlResult, err := repo.ExecuteSQLQuery("SELECT path FROM files WHERE path LIKE '%.go'")
	require.NoError(t, err)
	assert.Equal(t, 2, len(sqlResult.Rows)) // main.go and utils.go
	
	// Step 5: Get file statistics
	stats, err := repo.GetFileStatistics()
	require.NoError(t, err)
	assert.Greater(t, len(stats.Groups), 0)
	
	// Step 6: Get search suggestions
	suggestions, err := repo.GetSearchSuggestions("data", 5)
	require.NoError(t, err)
	assert.Greater(t, len(suggestions), 0)
	
	// Step 7: Get index statistics
	indexStats, err := repo.GetSearchIndexStatistics()
	require.NoError(t, err)
	assert.Equal(t, 4, indexStats["total_documents"])
}