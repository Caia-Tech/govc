package main

import (
	"fmt"
	"log"

	"github.com/Caia-Tech/govc"
)

func main() {
	// Create a new memory-first repository
	repo := govc.NewRepository()
	
	if repo == nil {
		log.Fatal("Failed to create repository")
	}
	
	fmt.Println("✓ Repository with search created successfully")
	
	// Test search functionality
	fmt.Println("\nTesting search functionality:")
	
	// Find files (should be empty initially)
	files, err := repo.FindFiles("*.go")
	if err != nil {
		log.Printf("FindFiles error: %v", err)
	} else {
		fmt.Printf("✓ Found %d files matching '*.go'\n", len(files))
	}
	
	// Test getting commits
	commits, err := repo.Log(10)
	if err != nil {
		log.Printf("Log error: %v", err) 
	} else {
		fmt.Printf("✓ Retrieved %d commits\n", len(commits))
	}
	
	// Test getting current branch
	branch, err := repo.GetCurrentBranch()
	if err != nil {
		log.Printf("GetCurrentBranch error: %v", err)
	} else {
		fmt.Printf("✓ Current branch: %s\n", branch)
	}
	
	fmt.Println("\n✅ Search functionality test completed!")
}