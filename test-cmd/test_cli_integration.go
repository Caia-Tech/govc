package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Caia-Tech/govc"
)

func main() {
	fmt.Println("Testing CLI integration issues...")
	
	// Test 1: Repository persistence across calls
	fmt.Println("\n1. Testing repository persistence:")
	
	// Create first repository
	repo1 := govc.NewRepository()
	stagingArea1 := repo1.GetStagingArea()
	err := stagingArea1.Add("test.txt", []byte("test content"))
	if err != nil {
		log.Printf("Error adding to repo1: %v", err)
	}
	
	staged1, err := stagingArea1.List()
	if err != nil {
		log.Printf("Error listing staged files in repo1: %v", err)
	} else {
		fmt.Printf("Repo1 staged files: %v\n", staged1)
	}
	
	// Create second repository (simulating what openRepo() does)
	repo2 := govc.NewRepository()
	stagingArea2 := repo2.GetStagingArea()
	staged2, err := stagingArea2.List()
	if err != nil {
		log.Printf("Error listing staged files in repo2: %v", err)
	} else {
		fmt.Printf("Repo2 staged files: %v\n", staged2)
	}
	
	if len(staged1) > 0 && len(staged2) == 0 {
		fmt.Println("❌ ISSUE CONFIRMED: Repositories are not persistent across instances")
	} else {
		fmt.Println("✅ Repositories share state")
	}
	
	// Test 2: Memory vs file-based persistence
	fmt.Println("\n2. Testing memory vs file persistence:")
	
	// Check if there's any file-based persistence
	if _, err := os.Stat(".govc"); os.IsNotExist(err) {
		fmt.Println("❌ No .govc directory found - repositories are purely in-memory")
	} else {
		fmt.Println("✅ .govc directory exists - may have file persistence")
	}
	
	fmt.Println("\n🔧 SOLUTION NEEDED:")
	fmt.Println("- Implement persistent repository storage")
	fmt.Println("- Make openRepo() load from persistent storage instead of creating new instances")
	fmt.Println("- Ensure staging area survives across CLI calls")
}