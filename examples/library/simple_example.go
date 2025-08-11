// simple_example.go - Minimal example of using govc as a library
package main

import (
	"fmt"
	"log"

	"github.com/Caia-Tech/govc"
)

func main() {
	// Create an in-memory Git repository
	repo := govc.New()

	// Add some files
	files := map[string]string{
		"README.md":   "# My Project\n\nBuilt with govc",
		"main.go":     "package main\n\nfunc main() {}",
		".gitignore":  "bin/\n*.tmp",
		"Makefile":    "build:\n\tgo build -o bin/app",
	}

	for path, content := range files {
		err := repo.Add(path, []byte(content))
		if err != nil {
			log.Fatalf("Failed to add %s: %v", path, err)
		}
	}

	// Create a commit
	commit, err := repo.Commit("Initial project setup")
	if err != nil {
		log.Fatalf("Failed to commit: %v", err)
	}

	fmt.Printf("✓ Created repository with commit %s\n", commit.Hash()[:8])

	// Check the status
	status, err := repo.Status()
	if err != nil {
		log.Fatalf("Failed to get status: %v", err)
	}

	fmt.Printf("✓ Current branch: %s\n", status.Branch)
	fmt.Printf("✓ Files tracked: %d\n", len(files))

	// Create a new branch
	err = repo.CreateBranch("feature/awesome")
	if err != nil {
		log.Fatalf("Failed to create branch: %v", err)
	}

	// Switch to the new branch
	err = repo.Checkout("feature/awesome")
	if err != nil {
		log.Fatalf("Failed to checkout branch: %v", err)
	}

	// Make changes on the feature branch
	repo.Add("feature.go", []byte("package main\n\n// New feature"))
	commit2, err := repo.Commit("Add new feature")
	if err != nil {
		log.Fatalf("Failed to commit: %v", err)
	}

	fmt.Printf("✓ Created feature commit %s\n", commit2.Hash()[:8])

	// List all branches
	branches, err := repo.ListBranches()
	if err != nil {
		log.Fatalf("Failed to list branches: %v", err)
	}

	fmt.Println("\n📋 Repository branches:")
	for _, branch := range branches {
		fmt.Printf("  - %s\n", branch.Name)
	}

	// Get commit history
	commits, err := repo.Log(5)
	if err != nil {
		log.Fatalf("Failed to get log: %v", err)
	}

	fmt.Println("\n📜 Recent commits:")
	for _, c := range commits {
		fmt.Printf("  %s - %s\n", c.Hash()[:8], c.Message())
	}

	fmt.Println("\n✅ govc library example completed successfully!")
}