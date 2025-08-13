package main

import (
	"fmt"
	"os"
	
	"github.com/Caia-Tech/govc/internal/repository"
)

func main() {
	fmt.Println("🧪 Testing Commit Clear Functionality")
	
	// Create temporary directory
	tempDir := "/tmp/test_commit_clear"
	os.RemoveAll(tempDir)
	os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)
	
	// Load repository
	repo, err := repository.LoadRepository(tempDir)
	if err != nil {
		fmt.Printf("❌ Failed to load repository: %v\n", err)
		return
	}
	
	staging := repo.GetStagingArea()
	
	// Add files to staging
	fmt.Println("\n1. Adding files to staging area...")
	staging.Add("file1.txt", []byte("content1"))
	staging.Add("file2.txt", []byte("content2"))
	
	files, _ := staging.List()
	fmt.Printf("   Files staged: %v\n", files)
	
	// Create commit
	fmt.Println("\n2. Creating commit...")
	commit, err := repo.Commit("Test commit")
	if err != nil {
		fmt.Printf("❌ Failed to create commit: %v\n", err)
		return
	}
	fmt.Printf("   Commit created: %s\n", commit.Hash()[:8])
	
	// Check staging area after commit
	files, _ = staging.List()
	fmt.Printf("   Files staged after commit: %v\n", files)
	
	// Reload repository to check persistence
	fmt.Println("\n3. Reloading repository to check persistence...")
	repo2, err := repository.LoadRepository(tempDir)
	if err != nil {
		fmt.Printf("❌ Failed to reload repository: %v\n", err)
		return
	}
	
	staging2 := repo2.GetStagingArea()
	files2, _ := staging2.List()
	fmt.Printf("   Files staged after reload: %v\n", files2)
	
	// Final result
	if len(files) == 0 && len(files2) == 0 {
		fmt.Println("\n✅ SUCCESS: Staging area properly cleared after commit!")
	} else {
		fmt.Println("\n❌ FAILURE: Staging area not cleared after commit")
		fmt.Printf("   In-memory: %d files, Persistent: %d files\n", len(files), len(files2))
	}
}