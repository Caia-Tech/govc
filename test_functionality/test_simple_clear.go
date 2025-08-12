package main

import (
	"fmt"
	"os"
	
	"github.com/Caia-Tech/govc/internal/repository"
)

func main() {
	fmt.Println("🧪 Testing Simple Staging Area Clear")
	
	// Create temporary directory
	tempDir := "/tmp/test_simple_clear"
	os.RemoveAll(tempDir)
	os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)
	
	// Create persistent staging area
	staging := repository.NewStagingAreaWithPath(tempDir)
	
	// Add files
	fmt.Println("\n1. Adding files...")
	staging.Add("file1.txt", []byte("content1"))
	staging.Add("file2.txt", []byte("content2"))
	
	files, _ := staging.List()
	fmt.Printf("   Before clear: %v\n", files)
	
	// Save to disk
	err := staging.Save(tempDir)
	if err != nil {
		fmt.Printf("❌ Save failed: %v\n", err)
		return
	}
	fmt.Println("   Saved to disk")
	
	// Clear staging area  
	fmt.Println("\n2. Clearing staging area...")
	staging.Clear()
	
	files, _ = staging.List()
	fmt.Printf("   After clear (in-memory): %v\n", files)
	
	// Load new staging area to check persistence
	fmt.Println("\n3. Loading new staging area...")
	newStaging := repository.NewStagingAreaWithPath(tempDir)
	err = newStaging.Load(tempDir)
	if err != nil {
		fmt.Printf("❌ Load failed: %v\n", err)
		return
	}
	
	files2, _ := newStaging.List()
	fmt.Printf("   After reload from disk: %v\n", files2)
	
	if len(files2) == 0 {
		fmt.Println("✅ Fix works! Staging area properly cleared on disk")
	} else {
		fmt.Println("❌ Fix didn't work - files still on disk")
	}
}