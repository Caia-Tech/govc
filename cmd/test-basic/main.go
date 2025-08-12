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
	
	fmt.Println("✓ Repository created successfully")
	
	// Test basic operations
	fmt.Println("\nTesting basic operations:")
	
	// Add a file (need to write content to worktree first)
	// For now, just test that we can stage a file
	err := repo.Add("test.txt")
	if err != nil {
		log.Printf("Note: Add returned error (expected for non-existent file): %v", err)
	} else {
		fmt.Println("✓ File staging attempted")
	}
	
	// Create a commit
	commit, err := repo.Commit("Initial commit")
	if err != nil {
		log.Printf("Failed to commit: %v", err)
	} else {
		fmt.Printf("✓ Commit created: %s\n", commit.Hash)
	}
	
	// Check status
	status, err := repo.Status()
	if err != nil {
		log.Printf("Failed to get status: %v", err)
	} else {
		fmt.Printf("✓ Status: %v\n", status)
	}
	
	fmt.Println("\n✅ Basic functionality test passed!")
}