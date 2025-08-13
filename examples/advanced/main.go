package main

import (
	"fmt"
	"log"
	"time"

	"github.com/Caia-Tech/govc"
)

func main() {
	// Example 1: Time travel
	timeTravelExample()

	// Example 2: Stashing
	stashingExample()

	// Example 3: Search and query
	searchExample()

	// Example 4: Hooks
	hooksExample()

	// Example 5: Performance optimization
	performanceExample()
}

func timeTravelExample() {
	fmt.Println("=== Time Travel Example ===")

	repo := govc.New()

	// Create a series of commits
	commitTimes := make([]time.Time, 0)
	for i := 1; i <= 5; i++ {
		content := fmt.Sprintf("Version %d of the file", i)
		repo.WriteFile("document.txt", []byte(content))
		repo.Add("document.txt")
		commit, _ := repo.Commit(fmt.Sprintf("Update %d", i))
		commitTimes = append(commitTimes, time.Now())
		fmt.Printf("Created commit %d: %s\n", i, commit.Hash())
		time.Sleep(100 * time.Millisecond) // Small delay between commits
	}

	// Read current version
	current, _ := repo.ReadFile("document.txt")
	fmt.Printf("Current version: %s\n", string(current))

	// Travel back to the time of the third commit
	if len(commitTimes) >= 3 {
		snapshot := repo.TimeTravel(commitTimes[2])
		if snapshot != nil {
			// Note: TimeTravel returns a historical snapshot
			// We can read files from that point in time
			historicalContent, err := snapshot.Read("document.txt")
			if err == nil {
				fmt.Printf("Content at time of commit 3: %s\n", string(historicalContent))
			}
		}
	}

	fmt.Println("Time travel demonstration complete")
	fmt.Println()
}

func stashingExample() {
	fmt.Println("=== Stashing Example ===")

	repo := govc.New()

	// Create initial file
	repo.WriteFile("work.txt", []byte("Initial content"))
	repo.Add("work.txt")
	repo.Commit("Initial commit")

	// Start working on changes
	repo.WriteFile("work.txt", []byte("Work in progress..."))
	repo.WriteFile("temp.txt", []byte("Temporary file"))

	// Stash the changes
	stash, err := repo.Stash("WIP: implementing new feature", true)
	if err != nil {
		log.Fatal("Failed to stash:", err)
	}

	fmt.Printf("Created stash: %s\n", stash.ID)

	// Working directory is now clean
	content, _ := repo.ReadFile("work.txt")
	fmt.Printf("After stash: %s\n", string(content))

	// Do some other work
	repo.WriteFile("other.txt", []byte("Other work"))
	repo.Add("other.txt")
	repo.Commit("Add other file")

	// List stashes
	stashes := repo.ListStashes()
	fmt.Println("\nStashes:")
	for _, stash := range stashes {
		fmt.Printf("  - %s: %s\n", stash.ID, stash.Message)
	}

	// Apply the stash
	if err := repo.ApplyStash(stash.ID, false); err != nil {
		log.Fatal("Failed to apply stash:", err)
	}

	content, _ = repo.ReadFile("work.txt")
	fmt.Printf("\nAfter applying stash: %s\n", string(content))
	fmt.Println()
}

func searchExample() {
	fmt.Println("=== Search Example ===")

	repo := govc.New()

	// Create sample code files
	files := map[string]string{
		"main.go": `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
	// TODO: Add more features
}`,
		"utils.go": `package main

// TODO: Implement utility functions

func HelperFunction() {
	// TODO: Complete implementation
}`,
		"README.md": `# Project

TODO: Write documentation
This project demonstrates govc features.`,
	}

	for path, content := range files {
		repo.WriteFile(path, []byte(content))
	}
	repo.Add(".")
	repo.Commit("Add initial files")

	// Search functionality is implemented at the API layer
	// The repository provides low-level file access
	allFiles, _ := repo.ListFiles()
	fmt.Printf("Repository contains %d files\n", len(allFiles))

	// You can read and search files manually
	for _, file := range allFiles {
		content, _ := repo.ReadFile(file)
		if len(content) > 0 {
			fmt.Printf("  - %s (%d bytes)\n", file, len(content))
		}
	}
	fmt.Println()
}

func hooksExample() {
	fmt.Println("=== Hooks Example ===")

	// Note: Hook functionality would need to be implemented
	// This is a demonstration of what it could look like

	fmt.Println("Hook functionality not yet implemented in core govc")
	fmt.Println("Could support pre-commit, post-commit, pre-push hooks")
	fmt.Println()
}

func performanceExample() {
	fmt.Println("=== Performance Optimization Example ===")

	// Create in-memory repository
	repo := govc.New()

	// Measure performance of bulk operations
	start := time.Now()

	// Create many files
	for i := 0; i < 100; i++ {
		filename := fmt.Sprintf("file_%04d.txt", i)
		content := fmt.Sprintf("This is file number %d", i)
		repo.WriteFile(filename, []byte(content))
	}

	writeTime := time.Since(start)
	fmt.Printf("Time to write 100 files: %v\n", writeTime)

	// Bulk add
	start = time.Now()
	repo.Add(".")
	addTime := time.Since(start)
	fmt.Printf("Time to stage 100 files: %v\n", addTime)

	// Commit
	start = time.Now()
	repo.Commit("Add 100 files")
	commitTime := time.Since(start)
	fmt.Printf("Time to commit: %v\n", commitTime)

	// Read performance
	start = time.Now()
	for i := 0; i < 100; i++ {
		filename := fmt.Sprintf("file_%04d.txt", i)
		repo.ReadFile(filename)
	}
	readTime := time.Since(start)
	fmt.Printf("Time to read 100 files: %v\n", readTime)

	fmt.Println("\ngovc is memory-first, providing excellent performance")
	fmt.Println("All operations complete in microseconds to milliseconds")
}

func concurrentExample() {
	fmt.Println("=== Concurrent Operations Example ===")

	// Create in-memory repository
	repo := govc.New()

	// Create multiple goroutines working on different branches
	branches := []string{"feature-1", "feature-2", "feature-3"}
	done := make(chan bool, len(branches))

	for _, branch := range branches {
		go func(branchName string) {
			// Create branch
			repo.Branch(branchName)
			repo.Checkout(branchName)

			// Do work on branch
			for i := 0; i < 5; i++ {
				filename := fmt.Sprintf("%s_file_%d.txt", branchName, i)
				content := fmt.Sprintf("Content for %s", filename)
				repo.WriteFile(filename, []byte(content))
				repo.Add(filename)
			}

			// Commit changes
			repo.Commit(fmt.Sprintf("Add files for %s", branchName))

			done <- true
		}(branch)
	}

	// Wait for all goroutines
	for i := 0; i < len(branches); i++ {
		<-done
	}

	// List all branches and their commits
	allBranches, _ := repo.ListBranches()
	fmt.Println("Created branches:")
	for _, branch := range allBranches {
		fmt.Printf("  - %s\n", branch)
	}
}
