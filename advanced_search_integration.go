package govc

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Advanced Search Integration Methods for Repository

// InitializeAdvancedSearch initializes the advanced search system
func (r *Repository) InitializeAdvancedSearch() error {
	if r.advancedSearch != nil {
		return nil // Already initialized
	}
	
	r.advancedSearch = NewAdvancedSearch(r)
	
	// Subscribe to repository events to keep search index updated
	if r.eventBus != nil {
		r.eventBus.Subscribe("file.*", func(path string, oldValue, newValue []byte) {
			event := &RepositoryEvent{
				Event: "file.updated",
				Data: map[string]interface{}{
					"path": path,
				},
			}
			r.handleSearchIndexUpdate(event)
		})
		
		r.eventBus.Subscribe("commit.*", func(path string, oldValue, newValue []byte) {
			event := &RepositoryEvent{
				Event: "commit.created",
				Data: map[string]interface{}{
					"path": path,
				},
			}
			r.handleSearchIndexUpdate(event)
		})
	}
	
	return nil
}

// FullTextSearch performs advanced full-text search
func (r *Repository) FullTextSearch(req *FullTextSearchRequest) (*FullTextSearchResponse, error) {
	if err := r.InitializeAdvancedSearch(); err != nil {
		return nil, err
	}
	
	return r.advancedSearch.FullTextSearch(req)
}

// ExecuteSQLQuery executes a SQL-like query
func (r *Repository) ExecuteSQLQuery(sqlQuery string) (*QueryExecutionResult, error) {
	if err := r.InitializeAdvancedSearch(); err != nil {
		return nil, err
	}
	
	// Parse the SQL query
	parsed, err := r.advancedSearch.queryParser.ParseQuery(sqlQuery)
	if err != nil {
		return nil, fmt.Errorf("query parse error: %w", err)
	}
	
	// Execute the parsed query
	return r.executeQuery(parsed, sqlQuery)
}

// SearchWithAggregation performs search with aggregation
func (r *Repository) SearchWithAggregation(req *AggregationRequest) (*AggregationResponse, error) {
	if err := r.InitializeAdvancedSearch(); err != nil {
		return nil, err
	}
	
	return r.advancedSearch.aggregator.Aggregate(r.advancedSearch, req)
}

// SubscribeToSearch creates a real-time search subscription
func (r *Repository) SubscribeToSearch(ctx context.Context, query *FullTextSearchRequest, options SubscriptionOptions) (*SearchSubscription, error) {
	if err := r.InitializeAdvancedSearch(); err != nil {
		return nil, err
	}
	
	return r.advancedSearch.searchSubscriber.Subscribe(ctx, query, options)
}

// UnsubscribeFromSearch removes a search subscription
func (r *Repository) UnsubscribeFromSearch(subscriptionID string) error {
	if r.advancedSearch == nil {
		return fmt.Errorf("advanced search not initialized")
	}
	
	return r.advancedSearch.searchSubscriber.Unsubscribe(subscriptionID)
}

// GetSearchSubscription returns a search subscription
func (r *Repository) GetSearchSubscription(subscriptionID string) (*SearchSubscription, error) {
	if r.advancedSearch == nil {
		return nil, fmt.Errorf("advanced search not initialized")
	}
	
	return r.advancedSearch.searchSubscriber.GetSubscription(subscriptionID)
}

// ListSearchSubscriptions returns all active search subscriptions
func (r *Repository) ListSearchSubscriptions() []SearchSubscription {
	if r.advancedSearch == nil {
		return []SearchSubscription{}
	}
	
	return r.advancedSearch.searchSubscriber.ListSubscriptions()
}

// GetSearchSuggestions provides query auto-completion suggestions
func (r *Repository) GetSearchSuggestions(partialQuery string, limit int) ([]string, error) {
	if err := r.InitializeAdvancedSearch(); err != nil {
		return nil, err
	}
	
	return r.advancedSearch.SearchSuggestions(partialQuery, limit), nil
}

// GetSearchIndexStatistics returns search index statistics
func (r *Repository) GetSearchIndexStatistics() (map[string]interface{}, error) {
	if err := r.InitializeAdvancedSearch(); err != nil {
		return nil, err
	}
	
	return r.advancedSearch.GetIndexStatistics(), nil
}

// RebuildSearchIndex forces a rebuild of the search index
func (r *Repository) RebuildSearchIndex() error {
	if r.advancedSearch != nil {
		// Clear existing index and rebuild
		r.advancedSearch.fullTextIndex.mu.Lock()
		r.advancedSearch.fullTextIndex.documentIndex = make(map[string]*SearchDocument)
		r.advancedSearch.fullTextIndex.tokenIndex = make(map[string][]string)
		r.advancedSearch.fullTextIndex.termFrequency = make(map[string]map[string]int)
		r.advancedSearch.fullTextIndex.documentFreq = make(map[string]int)
		r.advancedSearch.fullTextIndex.totalDocuments = 0
		r.advancedSearch.fullTextIndex.mu.Unlock()
		
		// Rebuild in background
		go r.advancedSearch.buildInitialIndex()
	}
	
	return nil
}

// executeQuery executes a parsed SQL query
func (r *Repository) executeQuery(parsed *ParsedQuery, originalSQL string) (*QueryExecutionResult, error) {
	start := time.Now()
	
	// Convert parsed query to appropriate search request
	switch parsed.FromType {
	case "files":
		return r.executeFileQuery(parsed, originalSQL, start)
	case "commits":
		return r.executeCommitQuery(parsed, originalSQL, start)
	case "content":
		return r.executeContentQuery(parsed, originalSQL, start)
	default:
		return nil, fmt.Errorf("unsupported table: %s", parsed.FromType)
	}
}

// executeFileQuery executes a query on files
func (r *Repository) executeFileQuery(parsed *ParsedQuery, originalSQL string, start time.Time) (*QueryExecutionResult, error) {
	// Build search request from parsed query
	searchReq := &FullTextSearchRequest{
		Query:  "*", // Search all documents by default
		Limit:  parsed.Limit,
		Offset: parsed.Offset,
	}
	
	var pathPattern string
	
	// Handle WHERE clauses
	if parsed.WhereClause != nil {
		for _, condition := range parsed.WhereClause.Conditions {
			switch strings.ToLower(condition.Field) {
			case "path":
				if condition.Operator == "LIKE" {
					// Store path pattern for filtering later
					pattern := strings.ReplaceAll(condition.Value.(string), "%", "*")
					pattern = strings.ReplaceAll(pattern, "_", "?")
					pathPattern = pattern
				}
			case "size":
				if condition.Operator == ">" {
					if size, ok := condition.Value.(int); ok {
						searchReq.MaxSize = int64(size + 1)
					}
				}
			case "content":
				if condition.Operator == "CONTAINS" {
					searchReq.Query = condition.Value.(string)
				}
			}
		}
	}
	
	// Execute search
	var results []map[string]interface{}
	var total int
	
	if len(parsed.Aggregations) > 0 {
		// Handle aggregation query
		aggReq := &AggregationRequest{
			Query:        searchReq,
			GroupBy:      parsed.GroupBy,
			Aggregations: []string{},
		}
		
		for _, agg := range parsed.Aggregations {
			aggReq.Aggregations = append(aggReq.Aggregations, strings.ToLower(agg.Function))
		}
		
		aggResponse, err := r.SearchWithAggregation(aggReq)
		if err != nil {
			return nil, err
		}
		
		// Convert aggregation results to rows
		if len(parsed.GroupBy) > 0 {
			// Grouped results
			for _, group := range aggResponse.Groups {
				row := make(map[string]interface{})
				row[parsed.GroupBy[0]] = group.GroupValue
				for _, agg := range parsed.Aggregations {
					key := strings.ToLower(agg.Function)
					if agg.Alias != "" {
						key = agg.Alias
					}
					row[key] = group.Metrics[key]
				}
				results = append(results, row)
			}
		} else {
			// Non-grouped aggregation
			row := make(map[string]interface{})
			for _, agg := range parsed.Aggregations {
				key := strings.ToLower(agg.Function)
				if agg.Alias != "" {
					key = agg.Alias
				}
				row[key] = aggResponse.Summary.Metrics[key]
			}
			results = append(results, row)
		}
		
		total = len(results)
	} else {
		// Handle regular query
		searchResponse, err := r.FullTextSearch(searchReq)
		if err != nil {
			return nil, err
		}
		
		// Convert search results to rows
		for _, result := range searchResponse.Results {
			// Apply path pattern filter if specified
			if pathPattern != "" {
				matched, _ := filepath.Match(pathPattern, result.Document.Path)
				if !matched {
					continue // Skip this result
				}
			}
			
			row := make(map[string]interface{})
			
			// Add requested fields
			for _, field := range parsed.SelectFields {
				switch strings.ToLower(field) {
				case "*":
					// Add all available fields
					row["path"] = result.Document.Path
					row["size"] = result.Document.Size
					row["last_modified"] = result.Document.LastModified
					row["score"] = result.Score
					row["content"] = result.Document.Content
				case "path":
					row["path"] = result.Document.Path
				case "size":
					row["size"] = result.Document.Size
				case "last_modified":
					row["last_modified"] = result.Document.LastModified
				case "score":
					row["score"] = result.Score
				case "content":
					row["content"] = result.Document.Content
				}
			}
			
			results = append(results, row)
		}
		
		total = searchResponse.Total
	}
	
	return &QueryExecutionResult{
		Rows:        results,
		Total:       total,
		QueryTime:   time.Since(start),
		ExecutedSQL: originalSQL,
	}, nil
}

// executeCommitQuery executes a query on commits
func (r *Repository) executeCommitQuery(parsed *ParsedQuery, originalSQL string, start time.Time) (*QueryExecutionResult, error) {
	// Build query request
	req := &QueryRequest{
		Type:      QueryByCommit,
		Limit:     parsed.Limit,
		Offset:    parsed.Offset,
		SortBy:    "date",
		SortOrder: "desc",
	}
	
	// Handle WHERE clauses
	if parsed.WhereClause != nil {
		for _, condition := range parsed.WhereClause.Conditions {
			switch strings.ToLower(condition.Field) {
			case "author":
				req.Type = QueryByAuthor
				req.Query = condition.Value.(string)
			case "message":
				if condition.Operator == "CONTAINS" {
					req.Query = condition.Value.(string)
				}
			}
		}
	}
	
	// Execute query
	queryResult, err := r.Query(req)
	if err != nil {
		return nil, err
	}
	
	// Convert to result format
	var results []map[string]interface{}
	
	if len(parsed.Aggregations) > 0 {
		// Handle aggregations (e.g., COUNT(*), GROUP BY author)
		if len(parsed.GroupBy) > 0 && parsed.GroupBy[0] == "author" {
			authorCounts := make(map[string]int)
			for _, commit := range queryResult.Commits {
				authorCounts[commit.Author]++
			}
			
			for author, count := range authorCounts {
				row := map[string]interface{}{
					"author": author,
					"count":  count,
				}
				results = append(results, row)
			}
		} else {
			// Simple aggregation
			row := map[string]interface{}{
				"count": len(queryResult.Commits),
			}
			results = append(results, row)
		}
	} else {
		// Regular results
		for _, commit := range queryResult.Commits {
			row := make(map[string]interface{})
			
			for _, field := range parsed.SelectFields {
				switch strings.ToLower(field) {
				case "*":
					row["hash"] = commit.Hash
					row["author"] = commit.Author
					row["message"] = commit.Message
					row["timestamp"] = commit.Timestamp
				case "hash":
					row["hash"] = commit.Hash
				case "author":
					row["author"] = commit.Author
				case "message":
					row["message"] = commit.Message
				case "timestamp":
					row["timestamp"] = commit.Timestamp
				}
			}
			
			results = append(results, row)
		}
	}
	
	return &QueryExecutionResult{
		Rows:        results,
		Total:       len(results),
		QueryTime:   time.Since(start),
		ExecutedSQL: originalSQL,
	}, nil
}

// executeContentQuery executes a query on file content
func (r *Repository) executeContentQuery(parsed *ParsedQuery, originalSQL string, start time.Time) (*QueryExecutionResult, error) {
	// Convert to full-text search
	searchReq := &FullTextSearchRequest{
		IncludeContent: true,
		Limit:          parsed.Limit,
		Offset:         parsed.Offset,
	}
	
	// Handle WHERE clauses
	if parsed.WhereClause != nil {
		for _, condition := range parsed.WhereClause.Conditions {
			if condition.Field == "content" && condition.Operator == "CONTAINS" {
				searchReq.Query = condition.Value.(string)
			}
		}
	}
	
	// Execute search
	searchResponse, err := r.FullTextSearch(searchReq)
	if err != nil {
		return nil, err
	}
	
	// Convert results
	var results []map[string]interface{}
	
	for _, result := range searchResponse.Results {
		row := make(map[string]interface{})
		
		for _, field := range parsed.SelectFields {
			switch strings.ToLower(field) {
			case "*":
				row["path"] = result.Document.Path
				row["content"] = result.Document.Content
				row["score"] = result.Score
			case "path":
				row["path"] = result.Document.Path
			case "content":
				row["content"] = result.Document.Content
			case "score":
				row["score"] = result.Score
			}
		}
		
		results = append(results, row)
	}
	
	return &QueryExecutionResult{
		Rows:        results,
		Total:       searchResponse.Total,
		QueryTime:   time.Since(start),
		ExecutedSQL: originalSQL,
	}, nil
}

// handleSearchIndexUpdate handles repository events to keep search index updated
func (r *Repository) handleSearchIndexUpdate(event *RepositoryEvent) {
	if r.advancedSearch == nil {
		return
	}
	
	switch event.Event {
	case "file.created", "file.updated":
		if data, ok := event.Data.(map[string]interface{}); ok {
			if filePath, ok := data["path"].(string); ok {
			// Re-index the file
			go func() {
				// Get file content
				files, err := r.FindFiles(filePath)
				if err == nil && len(files) > 0 {
					blob, err := r.GetBlobWithDelta(files[0].Hash)
					if err == nil {
						r.advancedSearch.IndexDocument(filePath, blob.Content)
					}
				}
				
				// Notify subscriptions
				r.advancedSearch.searchSubscriber.NotifyUpdate(r.advancedSearch, filePath, event.Event)
			}()
			}
		}
	case "file.deleted":
		if data, ok := event.Data.(map[string]interface{}); ok {
			if filePath, ok := data["path"].(string); ok {
			// Remove from index
			go func() {
				r.advancedSearch.fullTextIndex.mu.Lock()
				if doc, exists := r.advancedSearch.fullTextIndex.documentIndex[filePath]; exists {
					r.advancedSearch.removeDocumentFromIndex(filePath, doc)
				}
				r.advancedSearch.fullTextIndex.mu.Unlock()
				
				// Notify subscriptions
				r.advancedSearch.searchSubscriber.NotifyUpdate(r.advancedSearch, filePath, event.Event)
			}()
			}
		}
	}
}

// Convenience methods for common advanced search operations

// SearchCode searches for code patterns
func (r *Repository) SearchCode(query string, fileTypes []string, limit int) (*FullTextSearchResponse, error) {
	return r.FullTextSearch(&FullTextSearchRequest{
		Query:          query,
		FileTypes:      fileTypes,
		IncludeContent: false,
		Limit:          limit,
		SortBy:         "score",
	})
}

// SearchCodeWithHighlights searches for code with syntax highlighting
func (r *Repository) SearchCodeWithHighlights(query string, fileTypes []string, limit int) (*FullTextSearchResponse, error) {
	return r.FullTextSearch(&FullTextSearchRequest{
		Query:           query,
		FileTypes:       fileTypes,
		IncludeContent:  true,
		HighlightLength: 300,
		Limit:           limit,
		SortBy:          "score",
	})
}

// SearchLargeFiles finds large files in the repository
func (r *Repository) SearchLargeFiles(minSizeBytes int64, limit int) (*FullTextSearchResponse, error) {
	return r.FullTextSearch(&FullTextSearchRequest{
		Query:          "*",
		MaxSize:        0, // No max size filter
		IncludeContent: false,
		Limit:          limit,
		SortBy:         "size",
	})
}

// GetFileStatistics returns file statistics by type
func (r *Repository) GetFileStatistics() (*AggregationResponse, error) {
	return r.SearchWithAggregation(&AggregationRequest{
		Query: &FullTextSearchRequest{
			Query: "*",
		},
		GroupBy:      []string{"extension"},
		Aggregations: []string{"count", "avg_size", "total_size"},
	})
}

// GetAuthorStatistics returns commit statistics by author
func (r *Repository) GetAuthorStatistics() (*QueryExecutionResult, error) {
	return r.ExecuteSQLQuery("SELECT author, COUNT(*) as commits FROM commits GROUP BY author ORDER BY COUNT(*) DESC")
}

// GetAdvancedSearch returns the advanced search instance for direct access
// This is useful for advanced use cases where direct indexing is needed
func (r *Repository) GetAdvancedSearch() *AdvancedSearch {
	return r.advancedSearch
}

// SearchRecentChanges finds recently changed files
func (r *Repository) SearchRecentChanges(hours int, limit int) (*FullTextSearchResponse, error) {
	return r.FullTextSearch(&FullTextSearchRequest{
		Query:          "*",
		IncludeContent: false,
		Limit:          limit,
		SortBy:         "date",
	})
}