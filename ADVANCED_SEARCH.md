# GoVC Advanced Search Library

This document explains how to use GoVC's advanced search capabilities as a Go library.

## Overview

GoVC provides powerful advanced search functionality that goes far beyond the basic Query Engine:

- **Full-text search with TF-IDF scoring**: Relevance-ranked document search
- **SQL-like query interface**: Familiar query syntax for complex searches
- **Real-time search subscriptions**: Get notified when search results change
- **Search aggregation and analytics**: Group and analyze search results
- **Auto-completion suggestions**: Provide search suggestions to users

## Quick Start

```go
package main

import (
    "log"
    "github.com/caiatech/govc"
)

func main() {
    // Create a new repository
    repo := govc.NewRepository()
    
    // Initialize advanced search
    if err := repo.InitializeAdvancedSearch(); err != nil {
        log.Fatal(err)
    }
    
    // Perform a full-text search
    response, err := repo.FullTextSearch(&govc.FullTextSearchRequest{
        Query: "database connection",
        Limit: 10,
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Process results
    for _, result := range response.Results {
        log.Printf("Found: %s (score: %.3f)", result.Document.Path, result.Score)
    }
}
```

## Core Functions

### 1. Full-Text Search

Perform advanced text search with relevance scoring:

```go
response, err := repo.FullTextSearch(&govc.FullTextSearchRequest{
    Query:           "error handling database",
    FileTypes:       []string{"go", "js"},     // Filter by file type
    MaxSize:         1024000,                  // Max file size (bytes)
    MinScore:        0.1,                      // Minimum relevance score
    IncludeContent:  true,                     // Include full content
    HighlightLength: 200,                      // Highlight snippet length
    Limit:           50,                       // Max results
    Offset:          0,                        // Pagination offset
    SortBy:          "score",                  // Sort by: "score", "date", "size"
})
```

### 2. SQL-Like Queries

Execute SQL-like queries for structured data access:

```go
// Find all Go files larger than 1KB
result, err := repo.ExecuteSQLQuery(`
    SELECT path, size FROM files 
    WHERE path LIKE '%.go' AND size > 1000 
    ORDER BY size DESC LIMIT 10
`)

// Count commits by author
result, err := repo.ExecuteSQLQuery(`
    SELECT author, COUNT(*) as commits 
    FROM commits 
    GROUP BY author 
    ORDER BY COUNT(*) DESC
`)

// Search file content
result, err := repo.ExecuteSQLQuery(`
    SELECT path FROM content 
    WHERE content CONTAINS 'database' 
    OR content CONTAINS 'sql'
`)
```

### 3. Search Aggregation

Analyze search results with grouping and metrics:

```go
response, err := repo.SearchWithAggregation(&govc.AggregationRequest{
    Query: &govc.FullTextSearchRequest{Query: "*"},
    GroupBy: []string{"extension"},           // Group by file extension
    Aggregations: []string{"count", "avg_size", "total_size"},
    TimeRange: "month",                       // Optional: "hour", "day", "week", "month", "year"
})

// Process aggregated results
for _, group := range response.Groups {
    fmt.Printf("%s files: %d (avg size: %.1f bytes)\n", 
        group.GroupValue, group.Count, group.Metrics["avg_size"])
}
```

### 4. Search Statistics

Get insights into the search index:

```go
stats, err := repo.GetSearchIndexStatistics()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Total documents: %v\n", stats["total_documents"])
fmt.Printf("Total tokens: %v\n", stats["total_tokens"])
fmt.Printf("Average document size: %.1f bytes\n", stats["average_doc_size"])

// Get top tokens
topTokens := stats["top_tokens"].([]map[string]interface{})
for _, token := range topTokens {
    fmt.Printf("Token: %s (frequency: %v)\n", token["token"], token["frequency"])
}
```

### 5. Search Suggestions

Provide auto-completion for search queries:

```go
suggestions, err := repo.GetSearchSuggestions("data", 10)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Suggestions for 'data': %v\n", suggestions)
// Output: ["database", "datastore", "data_processing", ...]
```

### 6. Real-Time Search Subscriptions

Get notified when search results change:

```go
ctx := context.Background()
subscription, err := repo.SubscribeToSearch(ctx, &govc.FullTextSearchRequest{
    Query: "critical error",
    Limit: 20,
}, govc.SubscriptionOptions{
    PollInterval:    5 * time.Second,
    OnlyChanges:     true,
    IncludeContent:  false,
})

if err != nil {
    log.Fatal(err)
}

// Listen for notifications
go func() {
    for notification := range subscription.Channel {
        switch notification.Type {
        case govc.NotificationNewMatch:
            fmt.Printf("New match: %s\n", notification.NewResult.Document.Path)
        case govc.NotificationRemovedMatch:
            fmt.Printf("Removed match: %s\n", notification.RemovedPath)
        case govc.NotificationUpdatedMatch:
            fmt.Printf("Updated match: %s\n", notification.UpdatedResult.Document.Path)
        }
    }
}()

// Clean up when done
defer repo.UnsubscribeFromSearch(subscription.ID)
```

## Convenience Methods

GoVC provides several convenience methods for common search operations:

```go
// Search code files with syntax highlighting
response, err := repo.SearchCodeWithHighlights("error handling", []string{"go", "js"}, 10)

// Find large files
response, err := repo.SearchLargeFiles(1024000, 20) // Files > 1MB

// Get file statistics by type
stats, err := repo.GetFileStatistics()

// Get commit statistics by author
stats, err := repo.GetAuthorStatistics()

// Search recent changes
response, err := repo.SearchRecentChanges(24, 50) // Last 24 hours
```

## Advanced Usage

### Manual Document Indexing

For advanced use cases, you can manually index documents:

```go
advancedSearch := repo.GetAdvancedSearch()
if advancedSearch != nil {
    err := advancedSearch.IndexDocument("custom.txt", []byte("custom content"))
    if err != nil {
        log.Printf("Failed to index document: %v", err)
    }
}
```

### Rebuilding the Search Index

Force a complete rebuild of the search index:

```go
err := repo.RebuildSearchIndex()
if err != nil {
    log.Printf("Failed to rebuild index: %v", err)
}
```

## Types Reference

### Core Request Types

- `FullTextSearchRequest`: Configure full-text search parameters
- `AggregationRequest`: Configure search aggregation and analytics
- `SubscriptionOptions`: Configure real-time search subscriptions

### Core Response Types

- `FullTextSearchResponse`: Full-text search results with scoring
- `AggregationResponse`: Aggregated search results with metrics
- `QueryExecutionResult`: SQL query execution results
- `SearchResult`: Individual search result with document and score
- `SearchSubscription`: Active search subscription

### Notification Types

- `NotificationNewMatch`: New file matches the search
- `NotificationUpdatedMatch`: Existing match was updated
- `NotificationRemovedMatch`: File no longer matches
- `NotificationFullRefresh`: Complete result set changed
- `NotificationError`: Error occurred

## Performance Considerations

1. **Indexing**: Documents are automatically indexed when added to the repository
2. **Memory Usage**: The search index is kept in memory for fast access
3. **Query Performance**: TF-IDF scoring provides sub-millisecond search times
4. **Subscription Polling**: Default polling interval is 5 seconds (configurable)

## Error Handling

All search methods return errors that should be handled appropriately:

```go
response, err := repo.FullTextSearch(req)
if err != nil {
    switch {
    case strings.Contains(err.Error(), "not initialized"):
        // Advanced search not initialized
        log.Printf("Please call InitializeAdvancedSearch() first")
    case strings.Contains(err.Error(), "invalid query"):
        // Invalid search query
        log.Printf("Please check your search query syntax")
    default:
        // Other errors
        log.Printf("Search failed: %v", err)
    }
    return
}
```

## Examples

See the complete example in `examples/advanced_search/main.go` for a full demonstration of all features.

## API Endpoints

When using GoVC as a server, the advanced search features are also available via HTTP API:

- `POST /api/v1/repos/:repo_id/search/fulltext` - Full-text search
- `POST /api/v1/repos/:repo_id/search/sql` - SQL queries  
- `POST /api/v1/repos/:repo_id/search/aggregate` - Search aggregation
- `GET /api/v1/repos/:repo_id/search/statistics` - Index statistics
- `GET /api/v1/repos/:repo_id/search/suggestions?q=term` - Search suggestions
- `POST /api/v1/repos/:repo_id/search/rebuild` - Rebuild search index

See the API documentation for detailed endpoint specifications.