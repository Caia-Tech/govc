package main

import (
	"fmt"
	"log"
	"time"

	"github.com/caiatech/govc"
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

	repo, _ := govc.NewRepository()
	repo.Init()

	// Create a series of commits
	commits := make([]string, 0)
	for i := 1; i <= 5; i++ {
		content := fmt.Sprintf("Version %d of the file", i)
		repo.WriteFile("document.txt", []byte(content), 0644)
		repo.Add("document.txt")
		commit, _ := repo.Commit(fmt.Sprintf("Update %d", i), nil)
		commits = append(commits, commit.Hash)
		time.Sleep(100 * time.Millisecond) // Small delay between commits
	}

	// Read current version
	current, _ := repo.ReadFile("document.txt")
	fmt.Printf("Current version: %s\n", string(current))

	// Travel back to the third commit
	if err := repo.TimeTravel(commits[2]); err != nil {
		log.Fatal("Failed to time travel:", err)
	}

	// Read historical version
	historical, _ := repo.ReadFile("document.txt")
	fmt.Printf("Version at commit 3: %s\n", string(historical))

	// Travel to a specific date
	targetDate := time.Now().Add(-2 * time.Minute)
	if err := repo.TimeTravelToDate(targetDate); err != nil {
		log.Fatal("Failed to time travel to date:", err)
	}

	// Return to present
	repo.TimeTravel(commits[len(commits)-1])
	fmt.Println("Returned to present")
	fmt.Println()
}

func stashingExample() {
	fmt.Println("=== Stashing Example ===")

	repo, _ := govc.NewRepository()
	repo.Init()

	// Create initial file
	repo.WriteFile("work.txt", []byte("Initial content"), 0644)
	repo.Add("work.txt")
	repo.Commit("Initial commit", nil)

	// Start working on changes
	repo.WriteFile("work.txt", []byte("Work in progress..."), 0644)
	repo.WriteFile("temp.txt", []byte("Temporary file"), 0644)

	// Stash the changes
	stashID, err := repo.Stash(&govc.StashOptions{
		Message: "WIP: implementing new feature",
	})
	if err != nil {
		log.Fatal("Failed to stash:", err)
	}

	fmt.Printf("Created stash: %s\n", stashID)

	// Working directory is now clean
	content, _ := repo.ReadFile("work.txt")
	fmt.Printf("After stash: %s\n", string(content))

	// Do some other work
	repo.WriteFile("other.txt", []byte("Other work"), 0644)
	repo.Add("other.txt")
	repo.Commit("Add other file", nil)

	// List stashes
	stashes, _ := repo.ListStashes()
	fmt.Println("\nStashes:")
	for _, stash := range stashes {
		fmt.Printf("  - %s: %s\n", stash.ID, stash.Message)
	}

	// Apply the stash
	if err := repo.StashApply(stashID); err != nil {
		log.Fatal("Failed to apply stash:", err)
	}

	content, _ = repo.ReadFile("work.txt")
	fmt.Printf("\nAfter applying stash: %s\n", string(content))
	fmt.Println()
}

func searchExample() {
	fmt.Println("=== Search Example ===")

	repo, _ := govc.NewRepository()
	repo.Init()

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
		repo.WriteFile(path, []byte(content), 0644)
	}
	repo.Add(".")
	repo.Commit("Add initial files", nil)

	// Search for TODO comments
	results, err := repo.SearchContent("TODO", &govc.SearchOptions{
		FilePattern: "*",
		IgnoreCase:  false,
	})
	if err != nil {
		log.Fatal("Failed to search:", err)
	}

	fmt.Println("TODO items found:")
	for _, match := range results {
		fmt.Printf("  %s:%d: %s\n", match.File, match.Line, match.Content)
	}

	// Search using regex
	funcResults, err := repo.Grep(`func\s+\w+`, &govc.GrepOptions{
		Regex:       true,
		LineNumbers: true,
	})
	if err != nil {
		log.Fatal("Failed to grep:", err)
	}

	fmt.Println("\nFunction definitions:")
	for _, match := range funcResults {
		fmt.Printf("  %s:%d: %s\n", match.File, match.Line, match.Match)
	}

	// Search commits
	repo.WriteFile("bugfix.go", []byte("// Fixed critical bug"), 0644)
	repo.Add("bugfix.go")
	repo.Commit("Fix: resolve critical bug in parser", nil)

	commitResults, err := repo.SearchCommits("bug", &govc.SearchOptions{
		Since: time.Now().Add(-1 * time.Hour),
	})
	if err != nil {
		log.Fatal("Failed to search commits:", err)
	}

	fmt.Println("\nCommits mentioning 'bug':")
	for _, commit := range commitResults {
		fmt.Printf("  %s: %s\n", commit.Hash[:8], commit.Message)
	}
	fmt.Println()
}

func hooksExample() {
	fmt.Println("=== Hooks Example ===")

	repo, _ := govc.NewRepository()
	repo.Init()

	// Register pre-commit hook
	repo.RegisterHook(govc.HookPreCommit, func(ctx govc.HookContext) error {
		fmt.Println("Pre-commit hook: validating files...")
		
		stagedFiles := ctx.GetStagedFiles()
		for _, file := range stagedFiles {
			if file.Name == "forbidden.txt" {
				return fmt.Errorf("cannot commit forbidden.txt")
			}
			
			// Check file size
			if file.Size > 1024*1024 { // 1MB
				return fmt.Errorf("file %s is too large (%d bytes)", file.Name, file.Size)
			}
		}
		
		fmt.Println("Pre-commit validation passed!")
		return nil
	})

	// Register post-commit hook
	repo.RegisterHook(govc.HookPostCommit, func(ctx govc.HookContext) error {
		commit := ctx.GetCommit()
		fmt.Printf("Post-commit hook: Created commit %s\n", commit.Hash[:8])
		
		// Could trigger CI/CD, notifications, etc.
		return nil
	})

	// Register pre-push hook
	repo.RegisterHook(govc.HookPrePush, func(ctx govc.HookContext) error {
		fmt.Println("Pre-push hook: running tests...")
		// Simulate test execution
		time.Sleep(100 * time.Millisecond)
		fmt.Println("All tests passed!")
		return nil
	})

	// Test the hooks
	repo.WriteFile("allowed.txt", []byte("This is allowed"), 0644)
	repo.Add("allowed.txt")
	
	if _, err := repo.Commit("Add allowed file", nil); err != nil {
		log.Fatal("Failed to commit:", err)
	}

	// Try to commit forbidden file
	repo.WriteFile("forbidden.txt", []byte("This is forbidden"), 0644)
	repo.Add("forbidden.txt")
	
	if _, err := repo.Commit("Try to add forbidden file", nil); err != nil {
		fmt.Printf("Commit blocked by hook: %v\n", err)
	}
	fmt.Println()
}

func performanceExample() {
	fmt.Println("=== Performance Optimization Example ===")

	// Create repository with performance options
	repo, err := govc.NewRepositoryWithOptions(govc.RepositoryOptions{
		InMemory:       true,
		Compressed:     true,
		CacheSize:      1000,
		MaxParallelOps: 4,
	})
	if err != nil {
		log.Fatal("Failed to create repository:", err)
	}

	repo.Init()

	// Enable caching
	repo.EnableCache(govc.CacheOptions{
		Size: 1000,
		TTL:  5 * time.Minute,
	})

	// Measure performance of bulk operations
	start := time.Now()
	
	// Create many files
	for i := 0; i < 100; i++ {
		filename := fmt.Sprintf("file_%04d.txt", i)
		content := fmt.Sprintf("This is file number %d", i)
		repo.WriteFile(filename, []byte(content), 0644)
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
	repo.Commit("Add 100 files", nil)
	commitTime := time.Since(start)
	fmt.Printf("Time to commit: %v\n", commitTime)

	// Read performance (with cache)
	start = time.Now()
	for i := 0; i < 100; i++ {
		filename := fmt.Sprintf("file_%04d.txt", i)
		repo.ReadFile(filename)
	}
	readTime := time.Since(start)
	fmt.Printf("Time to read 100 files (cached): %v\n", readTime)

	// Use transaction for atomic operations
	start = time.Now()
	tx, _ := repo.BeginTransaction()
	
	for i := 100; i < 200; i++ {
		filename := fmt.Sprintf("file_%04d.txt", i)
		content := fmt.Sprintf("This is file number %d", i)
		tx.WriteFile(filename, []byte(content), 0644)
	}
	
	tx.Commit()
	txTime := time.Since(start)
	fmt.Printf("Time for transaction with 100 files: %v\n", txTime)

	// Repository statistics
	stats := repo.GetStatistics()
	fmt.Printf("\nRepository Statistics:\n")
	fmt.Printf("  Total objects: %d\n", stats.ObjectCount)
	fmt.Printf("  Memory usage: %d bytes\n", stats.MemoryUsage)
	fmt.Printf("  Cache hits: %d\n", stats.CacheHits)
	fmt.Printf("  Cache misses: %d\n", stats.CacheMisses)
}

func concurrentExample() {
	fmt.Println("=== Concurrent Operations Example ===")

	repo, _ := govc.NewRepositoryWithOptions(govc.RepositoryOptions{
		MaxParallelOps: 10,
		InMemory:       true,
	})
	repo.Init()

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
				repo.WriteFile(filename, []byte(content), 0644)
				repo.Add(filename)
			}

			// Commit changes
			repo.Commit(fmt.Sprintf("Add files for %s", branchName), nil)

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
		fmt.Printf("  - %s\n", branch.Name)
	}
}