package main

import (
	"fmt"
	"os"
	
	"github.com/Caia-Tech/govc/internal/repository"
)

func main() {
	fmt.Println("🧪 Testing Staging Area Clear Issue")
	
	// Create temporary directory
	tempDir := "/tmp/test_clear_staging"
	os.RemoveAll(tempDir)
	os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)
	
	// Load repository (creates persistent staging area)
	repo, err := repository.LoadRepository(tempDir)
	if err != nil {
		fmt.Printf("❌ Failed to load repository: %v\n", err)
		return
	}
	
	staging := repo.GetStagingArea()
	
	// Add some files
	fmt.Println("\n1. Adding files to staging area...")
	staging.Add("file1.txt", []byte("content1"))
	staging.Add("file2.txt", []byte("content2"))
	
	files, _ := staging.List()
	fmt.Printf("   Staged files: %v\n", files)
	
	// Create a commit (this should clear staging area)
	fmt.Println("\n2. Creating commit...")
	commit, err := repo.Commit("Test commit")
	if err != nil {
		fmt.Printf("❌ Failed to create commit: %v\n", err)
		return
	}
	fmt.Printf("   Commit created: %s\n", commit.Hash()[:8])
	
	// Check if staging area is cleared in memory
	files, _ = staging.List()
	fmt.Printf("   Staged files after commit: %v\n", files)
	
	if len(files) == 0 {
		fmt.Println("✅ In-memory staging area cleared correctly")
	} else {
		fmt.Println("❌ In-memory staging area NOT cleared")
	}
	
	// Now load the repository again to check persistence
	fmt.Println("\n3. Reloading repository to check persistence...")
	repo2, err := repository.LoadRepository(tempDir)
	if err != nil {
		fmt.Printf("❌ Failed to reload repository: %v\n", err)
		return
	}
	
	staging2 := repo2.GetStagingArea()
	files2, _ := staging2.List()
	fmt.Printf("   Staged files after reload: %v\n", files2)
	
	if len(files2) == 0 {
		fmt.Println("✅ Persistent staging area cleared correctly")
	} else {
		fmt.Println("❌ Persistent staging area NOT cleared - this is the bug!")
		fmt.Println("   The .govc/staging.json file still contains old data")
	}
	
	fmt.Println("\n📊 SUMMARY:")
	fmt.Printf("   In-memory clear works: %t\n", len(files) == 0)
	fmt.Printf("   Persistent clear works: %t\n", len(files2) == 0)
	
	if len(files) == 0 && len(files2) == 0 {
		fmt.Println("🎉 Staging area clear works correctly!")
	} else {
		fmt.Println("🐛 Bug confirmed: staging area not cleared after commit")
	}
}