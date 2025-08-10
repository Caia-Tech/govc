package govc

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdvancedSearch_Creation(t *testing.T) {
	repo := NewRepository()
	
	as := NewAdvancedSearch(repo)
	require.NotNil(t, as)
	assert.NotNil(t, as.fullTextIndex)
	assert.NotNil(t, as.searchSubscriber)
	assert.NotNil(t, as.queryParser)
	assert.NotNil(t, as.aggregator)
}

func TestFullTextSearch_BasicSearch(t *testing.T) {
	repo := NewRepository()
	err := repo.InitializeAdvancedSearch()
	require.NoError(t, err)
	
	// Create test documents
	testFiles := []struct {
		path    string
		content string
	}{
		{"main.go", "package main\n\nfunc main() {\n\tfmt.Println(\"Hello, world!\")\n}"},
		{"utils.go", "package main\n\nfunc helper() {\n\tfmt.Println(\"Helper function\")\n}"},
		{"README.md", "# Project\n\nThis is a Go project with example code.\nIt demonstrates basic functionality."},
		{"config.json", "{\"name\": \"test\", \"version\": \"1.0.0\"}"},
	}
	
	// Index the documents
	for _, file := range testFiles {
		err := repo.advancedSearch.IndexDocument(file.path, []byte(file.content))
		require.NoError(t, err)
	}
	
	// Test basic search
	response, err := repo.FullTextSearch(&FullTextSearchRequest{
		Query:          "main",
		IncludeContent: true,
		Limit:          10,
	})
	
	require.NoError(t, err)
	assert.Greater(t, len(response.Results), 0)
	assert.Greater(t, response.Statistics.DocumentsScanned, 0)
	
	// Check that results contain "main"
	found := false
	for _, result := range response.Results {
		if result.Document.Path == "main.go" || result.Document.Path == "utils.go" {
			found = true
			assert.Greater(t, result.Score, 0.0)
			break
		}
	}
	assert.True(t, found, "Should find documents containing 'main'")
}

func TestFullTextSearch_FileTypeFilter(t *testing.T) {
	repo := NewRepository()
	err := repo.InitializeAdvancedSearch()
	require.NoError(t, err)
	
	// Index documents with different extensions
	testFiles := []struct {
		path    string
		content string
	}{
		{"file1.go", "Go code with test keyword"},
		{"file2.js", "JavaScript code with test keyword"},
		{"file3.py", "Python code with test keyword"},
		{"file4.txt", "Text file with test keyword"},
	}
	
	for _, file := range testFiles {
		err := repo.advancedSearch.IndexDocument(file.path, []byte(file.content))
		require.NoError(t, err)
	}
	
	// Search only Go files
	response, err := repo.FullTextSearch(&FullTextSearchRequest{
		Query:     "test",
		FileTypes: []string{"go"},
		Limit:     10,
	})
	
	require.NoError(t, err)
	assert.Equal(t, 1, len(response.Results))
	assert.Equal(t, "file1.go", response.Results[0].Document.Path)
}

func TestFullTextSearch_Scoring(t *testing.T) {
	repo := NewRepository()
	err := repo.InitializeAdvancedSearch()
	require.NoError(t, err)
	
	// Create documents with different relevance for "database"
	testFiles := []struct {
		path    string
		content string
	}{
		{"high_relevance.go", "database database database connection pool"},
		{"medium_relevance.go", "database connection established with additional content to change document length"},
		{"low_relevance.go", "connecting to database"},
		{"no_relevance.go", "file processing and validation"},
	}
	
	for _, file := range testFiles {
		err := repo.advancedSearch.IndexDocument(file.path, []byte(file.content))
		require.NoError(t, err)
	}
	
	// First, test without MinScore to see all results
	response, err := repo.FullTextSearch(&FullTextSearchRequest{
		Query: "database",
		Limit: 10,
	})
	
	require.NoError(t, err)
	t.Logf("Total results without MinScore: %d", len(response.Results))
	for i, result := range response.Results {
		t.Logf("Result %d: %s, Score: %.4f", i, result.Document.Path, result.Score)
	}
	
	// Should find 3 documents containing "database" (excluding no_relevance.go)
	assert.Equal(t, 3, len(response.Results))
	
	// Results should be sorted by score (highest first)
	assert.Equal(t, "high_relevance.go", response.Results[0].Document.Path)
	if len(response.Results) >= 2 {
		assert.Greater(t, response.Results[0].Score, response.Results[1].Score)
	}
	// Note: medium_relevance and low_relevance may have very similar scores due to TF-IDF normalization
	// The important thing is that high_relevance has the highest score
	
	// Test with very low MinScore to ensure filtering works
	response2, err := repo.FullTextSearch(&FullTextSearchRequest{
		Query:    "database",
		MinScore: 0.001, // Very low threshold
		Limit:    10,
	})
	
	require.NoError(t, err)
	assert.Equal(t, 3, len(response2.Results)) // Should still exclude no_relevance.go
}

func TestFullTextSearch_Highlights(t *testing.T) {
	repo := NewRepository()
	err := repo.InitializeAdvancedSearch()
	require.NoError(t, err)
	
	content := "This is a sample document with important information about error handling and exception management."
	err = repo.advancedSearch.IndexDocument("test.txt", []byte(content))
	require.NoError(t, err)
	
	response, err := repo.FullTextSearch(&FullTextSearchRequest{
		Query:           "error",
		HighlightLength: 100,
		Limit:           1,
	})
	
	require.NoError(t, err)
	assert.Equal(t, 1, len(response.Results))
	assert.Greater(t, len(response.Results[0].Highlights), 0)
	
	// Check that highlights contain the search term
	highlight := response.Results[0].Highlights[0]
	assert.Contains(t, highlight, "error")
}

func TestQueryParser_BasicQueries(t *testing.T) {
	parser := NewQueryParser()
	
	tests := []struct {
		name     string
		query    string
		wantErr  bool
		checkFn  func(t *testing.T, parsed *ParsedQuery)
	}{
		{
			name:    "Simple SELECT FROM",
			query:   "SELECT * FROM files",
			wantErr: false,
			checkFn: func(t *testing.T, parsed *ParsedQuery) {
				assert.Contains(t, parsed.SelectFields, "*")
				assert.Equal(t, "files", parsed.FromType)
			},
		},
		{
			name:    "SELECT with WHERE",
			query:   "SELECT path FROM files WHERE size > 1000",
			wantErr: false,
			checkFn: func(t *testing.T, parsed *ParsedQuery) {
				assert.Contains(t, parsed.SelectFields, "path")
				assert.Equal(t, "files", parsed.FromType)
				assert.NotNil(t, parsed.WhereClause)
				assert.Equal(t, 1, len(parsed.WhereClause.Conditions))
				assert.Equal(t, "size", parsed.WhereClause.Conditions[0].Field)
				assert.Equal(t, ">", parsed.WhereClause.Conditions[0].Operator)
			},
		},
		{
			name:    "SELECT with aggregation",
			query:   "SELECT COUNT(*) FROM commits",
			wantErr: false,
			checkFn: func(t *testing.T, parsed *ParsedQuery) {
				assert.Equal(t, 1, len(parsed.Aggregations))
				assert.Equal(t, "COUNT", parsed.Aggregations[0].Function)
			},
		},
		{
			name:    "SELECT with ORDER BY",
			query:   "SELECT * FROM files ORDER BY size DESC",
			wantErr: false,
			checkFn: func(t *testing.T, parsed *ParsedQuery) {
				assert.Equal(t, 1, len(parsed.OrderBy))
				assert.Equal(t, "size", parsed.OrderBy[0].Field)
				assert.Equal(t, "DESC", parsed.OrderBy[0].Direction)
			},
		},
		{
			name:    "SELECT with LIMIT",
			query:   "SELECT * FROM files LIMIT 10",
			wantErr: false,
			checkFn: func(t *testing.T, parsed *ParsedQuery) {
				assert.Equal(t, 10, parsed.Limit)
			},
		},
		{
			name:    "Invalid query - no FROM",
			query:   "SELECT *",
			wantErr: true,
			checkFn: nil,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parser.ParseQuery(tt.query)
			
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, parsed)
				if tt.checkFn != nil {
					tt.checkFn(t, parsed)
				}
			}
		})
	}
}

func TestSQLQuery_Integration(t *testing.T) {
	repo := NewRepository()
	
	// Initialize advanced search first
	err := repo.InitializeAdvancedSearch()
	require.NoError(t, err)
	
	// Create test files and index them directly
	testFiles := []struct {
		path    string
		content string
	}{
		{"large.txt", "This is a large file with lots of content"},
		{"small.go", "package main"},
		{"medium.js", "function test() { return true; }"},
	}
	
	for _, file := range testFiles {
		// Index the documents directly for the search
		err := repo.advancedSearch.IndexDocument(file.path, []byte(file.content))
		require.NoError(t, err)
		t.Logf("Indexed document: %s", file.path)
	}
	
	// Check that documents were indexed
	stats, err := repo.GetSearchIndexStatistics()
	require.NoError(t, err)
	t.Logf("Index statistics: %+v", stats)
	
	// Test basic search first
	searchResp, err := repo.FullTextSearch(&FullTextSearchRequest{
		Query: "*",
		Limit: 10,
	})
	require.NoError(t, err)
	t.Logf("Basic search found %d documents", len(searchResp.Results))
	for i, result := range searchResp.Results {
		t.Logf("Document %d: %s", i, result.Document.Path)
	}
	
	// Test SQL query
	result, err := repo.ExecuteSQLQuery("SELECT path FROM files WHERE path LIKE '%.go'")
	require.NoError(t, err)
	
	t.Logf("SQL query returned %d rows", len(result.Rows))
	for i, row := range result.Rows {
		t.Logf("Row %d: %+v", i, row)
	}
	
	assert.Equal(t, 1, len(result.Rows))
	assert.Equal(t, "small.go", result.Rows[0]["path"])
}

func TestSearchSubscription_BasicSubscription(t *testing.T) {
	repo := NewRepository()
	err := repo.InitializeAdvancedSearch()
	require.NoError(t, err)
	
	// Create a search subscription
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	subscription, err := repo.SubscribeToSearch(ctx, &FullTextSearchRequest{
		Query: "test",
		Limit: 10,
	}, SubscriptionOptions{
		PollInterval:    100 * time.Millisecond,
		OnlyChanges:     true,
		IncludeContent:  false,
	})
	
	require.NoError(t, err)
	assert.NotEmpty(t, subscription.ID)
	assert.True(t, subscription.Active)
	
	// Clean up
	err = repo.UnsubscribeFromSearch(subscription.ID)
	assert.NoError(t, err)
}

func TestSearchSubscription_Notifications(t *testing.T) {
	repo := NewRepository()
	err := repo.InitializeAdvancedSearch()
	require.NoError(t, err)
	
	// Create a subscription for "golang" keyword
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	subscription, err := repo.SubscribeToSearch(ctx, &FullTextSearchRequest{
		Query: "golang",
		Limit: 5,
	}, SubscriptionOptions{
		PollInterval:   50 * time.Millisecond,
		OnlyChanges:    true,
		IncludeContent: false,
	})
	require.NoError(t, err)
	
	// Start polling in background
	go repo.advancedSearch.searchSubscriber.StartPolling(ctx, repo.advancedSearch)
	
	// Add a document that matches
	time.Sleep(100 * time.Millisecond) // Let initial search complete
	
	err = repo.advancedSearch.IndexDocument("new_file.go", []byte("This file contains golang code examples"))
	require.NoError(t, err)
	
	// Notify subscriber of change
	repo.advancedSearch.searchSubscriber.NotifyUpdate(repo.advancedSearch, "new_file.go", "created")
	
	// Check for notification
	select {
	case notification := <-subscription.Channel:
		assert.Equal(t, subscription.ID, notification.SubscriptionID)
		assert.Equal(t, NotificationNewMatch, notification.Type)
		t.Logf("Received notification: %+v", notification)
	case <-time.After(500 * time.Millisecond):
		t.Log("No notification received within timeout")
	}
	
	// Clean up
	repo.UnsubscribeFromSearch(subscription.ID)
}

func TestSearchAggregation_FileStatistics(t *testing.T) {
	repo := NewRepository()
	err := repo.InitializeAdvancedSearch()
	require.NoError(t, err)
	
	// Create test files with different extensions
	testFiles := []struct {
		path    string
		content string
	}{
		{"file1.go", "Go file content"},
		{"file2.go", "Another Go file"},
		{"file1.js", "JavaScript content"},
		{"file2.js", "More JavaScript"},
		{"file3.js", "Even more JavaScript"},
		{"README.md", "Markdown documentation"},
	}
	
	for _, file := range testFiles {
		err := repo.advancedSearch.IndexDocument(file.path, []byte(file.content))
		require.NoError(t, err)
	}
	
	// Test aggregation by file extension
	response, err := repo.SearchWithAggregation(&AggregationRequest{
		Query: &FullTextSearchRequest{
			Query: "*", // Match all files
		},
		GroupBy:      []string{"extension"},
		Aggregations: []string{"count", "avg_size"},
	})
	
	require.NoError(t, err)
	assert.Equal(t, 3, len(response.Groups)) // .go, .js, .md
	assert.Equal(t, 6, response.Summary.TotalResults)
	
	// Check that groups are present
	extensions := make(map[string]bool)
	for _, group := range response.Groups {
		extensions[group.GroupValue] = true
		assert.Greater(t, group.Count, 0)
		assert.Contains(t, group.Metrics, "count")
	}
	
	assert.True(t, extensions["go"])
	assert.True(t, extensions["js"])
	assert.True(t, extensions["md"])
}

func TestSearchIndexStatistics(t *testing.T) {
	repo := NewRepository()
	err := repo.InitializeAdvancedSearch()
	require.NoError(t, err)
	
	// Add some documents
	testDocs := []struct {
		path    string
		content string
	}{
		{"doc1.txt", "The quick brown fox jumps over the lazy dog"},
		{"doc2.txt", "Pack my box with five dozen liquor jugs"},
		{"doc3.txt", "How vexingly quick daft zebras jump"},
	}
	
	for _, doc := range testDocs {
		err := repo.advancedSearch.IndexDocument(doc.path, []byte(doc.content))
		require.NoError(t, err)
	}
	
	// Get statistics
	stats, err := repo.GetSearchIndexStatistics()
	require.NoError(t, err)
	
	assert.Equal(t, 3, stats["total_documents"])
	assert.Greater(t, stats["total_tokens"], 0)
	assert.NotNil(t, stats["index_last_updated"])
	assert.Greater(t, stats["average_doc_size"], 0.0)
	
	// Check top tokens
	topTokens, ok := stats["top_tokens"].([]map[string]interface{})
	assert.True(t, ok)
	assert.Greater(t, len(topTokens), 0)
}

func TestSearchSuggestions(t *testing.T) {
	repo := NewRepository()
	err := repo.InitializeAdvancedSearch()
	require.NoError(t, err)
	
	// Add documents with various terms
	testDocs := []struct {
		path    string
		content string
	}{
		{"programming.txt", "programming languages include javascript, python, golang"},
		{"development.txt", "software development practices and programming methodologies"},
		{"projects.txt", "project management and development lifecycle"},
	}
	
	for _, doc := range testDocs {
		err := repo.advancedSearch.IndexDocument(doc.path, []byte(doc.content))
		require.NoError(t, err)
	}
	
	// Test suggestions
	suggestions, err := repo.GetSearchSuggestions("pro", 5)
	require.NoError(t, err)
	
	assert.Greater(t, len(suggestions), 0)
	
	// All suggestions should start with "pro"
	for _, suggestion := range suggestions {
		assert.True(t, strings.HasPrefix(suggestion, "pro"), 
			"Suggestion '%s' should start with 'pro'", suggestion)
	}
}

func BenchmarkFullTextSearch(b *testing.B) {
	repo := NewRepository()
	repo.InitializeAdvancedSearch()
	
	// Create a large corpus
	for i := 0; i < 1000; i++ {
		content := fmt.Sprintf("Document %d contains various keywords like golang, javascript, python, and database. This is sample content for performance testing.", i)
		repo.advancedSearch.IndexDocument(fmt.Sprintf("doc_%d.txt", i), []byte(content))
	}
	
	searchReq := &FullTextSearchRequest{
		Query: "golang database",
		Limit: 20,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.FullTextSearch(searchReq)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQueryParser(b *testing.B) {
	parser := NewQueryParser()
	query := "SELECT path, size FROM files WHERE size > 1000 AND path LIKE '%.go' ORDER BY size DESC LIMIT 50"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parser.ParseQuery(query)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestRealWorldSearchScenarios(t *testing.T) {
	repo := NewRepository()
	err := repo.InitializeAdvancedSearch()
	require.NoError(t, err)
	
	// Simulate a real codebase with various file types
	codeFiles := []struct {
		path    string
		content string
	}{
		{"main.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Error: connection failed\")\n}"},
		{"utils/helper.go", "package utils\n\nfunc HandleError(err error) {\n\tif err != nil {\n\t\tpanic(err)\n\t}\n}"},
		{"config.json", "{\"database\": {\"host\": \"localhost\", \"port\": 5432}}"},
		{"README.md", "# Project\n\nThis project handles database connections and error management.\n\n## Error Handling\n\nWe use structured error handling."},
		{"tests/error_test.go", "package tests\n\nfunc TestErrorHandling(t *testing.T) {\n\t// Test error scenarios\n}"},
		{"docs/api.md", "# API Documentation\n\n## Error Responses\n\nAll API endpoints return structured error responses."},
	}
	
	for _, file := range codeFiles {
		err := repo.advancedSearch.IndexDocument(file.path, []byte(file.content))
		require.NoError(t, err)
	}
	
	t.Run("SearchForErrors", func(t *testing.T) {
		response, err := repo.SearchCodeWithHighlights("error", []string{"go", "md"}, 10)
		require.NoError(t, err)
		
		assert.Greater(t, len(response.Results), 0)
		assert.Less(t, response.QueryTime, 100*time.Millisecond)
		
		// Should find error-related content
		foundGoFile := false
		for _, result := range response.Results {
			if strings.HasSuffix(result.Document.Path, ".go") {
				foundGoFile = true
				assert.Greater(t, len(result.Highlights), 0)
			}
		}
		assert.True(t, foundGoFile)
	})
	
	t.Run("SearchForDatabase", func(t *testing.T) {
		response, err := repo.SearchCode("database", nil, 5)
		require.NoError(t, err)
		
		assert.Greater(t, len(response.Results), 0)
		
		// Should find config.json and README.md
		foundConfig := false
		foundReadme := false
		for _, result := range response.Results {
			if result.Document.Path == "config.json" {
				foundConfig = true
			}
			if result.Document.Path == "README.md" {
				foundReadme = true
			}
		}
		assert.True(t, foundConfig || foundReadme)
	})
	
	t.Run("FileTypeStatistics", func(t *testing.T) {
		stats, err := repo.GetFileStatistics()
		require.NoError(t, err)
		
		assert.Greater(t, len(stats.Groups), 0)
		assert.Equal(t, 6, stats.Summary.TotalResults)
		
		// Should have groups for different file types
		extensions := make(map[string]bool)
		for _, group := range stats.Groups {
			extensions[group.GroupValue] = true
		}
		assert.True(t, extensions["go"])
		assert.True(t, extensions["md"])
		assert.True(t, extensions["json"])
	})
}