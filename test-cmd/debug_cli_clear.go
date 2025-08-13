package main

import (
	"fmt"
	
	"github.com/Caia-Tech/govc/internal/repository"
)

func main() {
	fmt.Println("🔍 Debug CLI Clear Issue")
	
	// Load the same repository the CLI uses
	repo, err := repository.LoadRepository("/Users/owner/Desktop/caiatech/software/govc")
	if err != nil {
		fmt.Printf("❌ Failed to load repository: %v\n", err)
		return
	}
	
	staging := repo.GetStagingArea()
	
	// Check current staging area status
	files, _ := staging.List()
	fmt.Printf("Files currently staged: %v\n", files)
	
	if len(files) > 0 {
		fmt.Println("\n🧹 Manually clearing staging area...")
		staging.Clear()
		
		// Check if cleared
		files, _ = staging.List()
		fmt.Printf("Files after manual clear: %v\n", files)
		
		// Reload and check persistence
		repo2, err := repository.LoadRepository("/Users/owner/Desktop/caiatech/software/govc")
		if err != nil {
			fmt.Printf("❌ Failed to reload repository: %v\n", err)
			return
		}
		
		staging2 := repo2.GetStagingArea()
		files2, _ := staging2.List()
		fmt.Printf("Files after reload: %v\n", files2)
		
		if len(files2) == 0 {
			fmt.Println("✅ Manual clear worked!")
		} else {
			fmt.Println("❌ Manual clear didn't persist")
		}
	} else {
		fmt.Println("✅ Staging area is already clear")
	}
}