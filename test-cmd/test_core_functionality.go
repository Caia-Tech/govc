package main

import (
	"fmt"
	"log"

	"github.com/Caia-Tech/govc"
)

func main() {
	fmt.Println("Testing core govc functionality...")
	
	// Test 1: Create repository
	fmt.Println("\n1. Testing repository creation...")
	repo := govc.NewRepository()
	if repo == nil {
		log.Fatal("Failed to create repository")
	}
	fmt.Println("✓ Repository created successfully")
	
	// Test 2: Test current branch
	fmt.Println("\n2. Testing current branch...")
	currentBranch, err := repo.GetCurrentBranch()
	if err != nil {
		log.Printf("Error getting current branch: %v", err)
	} else {
		fmt.Printf("✓ Current branch: %s\n", currentBranch)
	}
	
	// Test 3: List branches
	fmt.Println("\n3. Testing branch listing...")
	branches, err := repo.ListBranches()
	if err != nil {
		log.Printf("Error listing branches: %v", err)
	} else {
		fmt.Printf("✓ Found %d branches: %v\n", len(branches), branches)
	}
	
	// Test 4: Test staging area
	fmt.Println("\n4. Testing staging area...")
	stagingArea := repo.GetStagingArea()
	if stagingArea == nil {
		log.Printf("Staging area is nil")
	} else {
		fmt.Println("✓ Staging area available")
		
		// Test adding a file to staging
		stagingArea.Add("test.txt", []byte("test content"))
		stagedFiles, err := stagingArea.List()
		if err != nil {
			log.Printf("Error listing staged files: %v", err)
		} else {
			fmt.Printf("✓ Staged files: %v\n", stagedFiles)
		}
	}
	
	// Test 5: Test search functionality
	fmt.Println("\n5. Testing search functionality...")
	results, err := repo.FindFiles("*.txt")
	if err != nil {
		log.Printf("Error in search: %v", err)
	} else {
		fmt.Printf("✓ Search results: %v\n", results)
	}
	
	fmt.Println("\n🎉 Core functionality test completed!")
}