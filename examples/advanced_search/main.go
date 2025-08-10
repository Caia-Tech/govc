package main

import (
	"fmt"
	"log"

	"github.com/caiatech/govc"
)

func main() {
	fmt.Println("GoVC Advanced Search Library Example")
	fmt.Println("=====================================")

	// Step 1: Create a new repository
	repo := govc.NewRepository()
	
	// Step 2: Initialize advanced search capabilities
	if err := repo.InitializeAdvancedSearch(); err != nil {
		log.Fatalf("Failed to initialize advanced search: %v", err)
	}
	fmt.Println("✓ Advanced search initialized")

	// Step 3: Simulate adding some documents to the repository
	// In a real scenario, these would come from actual files in the repository
	testFiles := map[string]string{
		"main.go": `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
	database := setupDatabase()
	defer database.Close()
}

func setupDatabase() *Database {
	return &Database{host: "localhost"}
}`,

		"utils/helper.go": `package utils

import "database/sql"

func ConnectDatabase() (*sql.DB, error) {
	return sql.Open("postgres", "host=localhost")
}

func QueryUsers(db *sql.DB) ([]User, error) {
	// Database query logic here
	return nil, nil
}`,

		"config.json": `{
	"database": {
		"host": "localhost",
		"port": 5432,
		"name": "myapp"
	},
	"server": {
		"port": 8080
	}
}`,

		"README.md": `# MyApp

A sample application demonstrating database connectivity.

## Features
- Database connection management
- User management
- Configuration handling
`,
	}

	// Index the documents
	// Note: In a real application, this would happen automatically when files are added
	// For this demo, we'll manually index the documents
	for path, content := range testFiles {
		// We need to access the advanced search instance through the repo internals
		// In practice, documents are indexed automatically when files are committed
		if repo.GetAdvancedSearch() != nil {
			if err := repo.GetAdvancedSearch().IndexDocument(path, []byte(content)); err != nil {
				log.Printf("Failed to index %s: %v", path, err)
			}
		}
	}
	fmt.Printf("✓ Indexed %d documents\n", len(testFiles))

	// Step 4: Demonstrate full-text search
	fmt.Println("\n--- Full-Text Search Examples ---")
	
	// Search for "database"
	searchResponse, err := repo.FullTextSearch(&govc.FullTextSearchRequest{
		Query:          "database",
		IncludeContent: false,
		Limit:          10,
	})
	if err != nil {
		log.Printf("Search failed: %v", err)
	} else {
		fmt.Printf("Search for 'database' found %d results:\n", len(searchResponse.Results))
		for i, result := range searchResponse.Results {
			fmt.Printf("  %d. %s (score: %.3f) - %s\n", 
				i+1, result.Document.Path, result.Score, result.MatchReason)
		}
	}

	// Step 5: Demonstrate SQL-like queries
	fmt.Println("\n--- SQL Query Examples ---")
	
	// Find all Go files
	sqlResult, err := repo.ExecuteSQLQuery("SELECT path FROM files WHERE path LIKE '%.go'")
	if err != nil {
		log.Printf("SQL query failed: %v", err)
	} else {
		fmt.Printf("Found %d Go files:\n", len(sqlResult.Rows))
		for i, row := range sqlResult.Rows {
			fmt.Printf("  %d. %s\n", i+1, row["path"])
		}
	}

	// Step 6: Demonstrate search aggregation
	fmt.Println("\n--- Search Aggregation Examples ---")
	
	aggResponse, err := repo.SearchWithAggregation(&govc.AggregationRequest{
		Query: &govc.FullTextSearchRequest{Query: "*"},
		GroupBy: []string{"extension"},
		Aggregations: []string{"count", "avg_size"},
	})
	if err != nil {
		log.Printf("Aggregation failed: %v", err)
	} else {
		fmt.Printf("File statistics by extension:\n")
		for _, group := range aggResponse.Groups {
			fmt.Printf("  %s: %d files\n", group.GroupValue, group.Count)
		}
	}

	// Step 7: Get search index statistics
	fmt.Println("\n--- Search Index Statistics ---")
	
	stats, err := repo.GetSearchIndexStatistics()
	if err != nil {
		log.Printf("Failed to get statistics: %v", err)
	} else {
		fmt.Printf("Total documents: %v\n", stats["total_documents"])
		fmt.Printf("Total tokens: %v\n", stats["total_tokens"])
		fmt.Printf("Average document size: %.1f bytes\n", stats["average_doc_size"])
	}

	// Step 8: Get search suggestions
	fmt.Println("\n--- Search Suggestions ---")
	
	suggestions, err := repo.GetSearchSuggestions("data", 5)
	if err != nil {
		log.Printf("Failed to get suggestions: %v", err)
	} else {
		fmt.Printf("Suggestions for 'data': %v\n", suggestions)
	}

	fmt.Println("\n✓ Advanced search library demonstration complete!")
}