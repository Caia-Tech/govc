package main

import (
	"fmt"
	"log"
	"os"

	"github.com/caiatech/govc"
)

func main() {
	// Example 1: Basic repository operations
	basicExample()

	// Example 2: Working with branches
	branchExample()

	// Example 3: File operations
	fileOperationsExample()

	// Example 4: Commit history
	historyExample()
}

func basicExample() {
	fmt.Println("=== Basic Repository Example ===")

	// Create a new in-memory repository
	repo, err := govc.NewRepository()
	if err != nil {
		log.Fatal("Failed to create repository:", err)
	}

	// Initialize the repository
	if err := repo.Init(); err != nil {
		log.Fatal("Failed to initialize repository:", err)
	}

	// Create and write a file
	content := []byte("# govc Example\n\nThis is a demonstration of govc capabilities.")
	if err := repo.WriteFile("README.md", content, 0644); err != nil {
		log.Fatal("Failed to write file:", err)
	}

	// Stage the file
	if err := repo.Add("README.md"); err != nil {
		log.Fatal("Failed to stage file:", err)
	}

	// Create a commit
	commit, err := repo.Commit("Initial commit", &govc.CommitOptions{
		Author: "Example User <user@example.com>",
	})
	if err != nil {
		log.Fatal("Failed to create commit:", err)
	}

	fmt.Printf("Created commit: %s\n", commit.Hash)
	fmt.Printf("Commit message: %s\n", commit.Message)
	fmt.Printf("Author: %s\n", commit.Author)
	fmt.Println()
}

func branchExample() {
	fmt.Println("=== Branch Operations Example ===")

	repo, _ := govc.NewRepository()
	repo.Init()

	// Create initial commit
	repo.WriteFile("main.go", []byte("package main\n\nfunc main() {}\n"), 0644)
	repo.Add("main.go")
	repo.Commit("Initial commit", nil)

	// Create a new branch
	if err := repo.Branch("feature/new-feature"); err != nil {
		log.Fatal("Failed to create branch:", err)
	}

	// Switch to the new branch
	if err := repo.Checkout("feature/new-feature"); err != nil {
		log.Fatal("Failed to checkout branch:", err)
	}

	// Make changes on the feature branch
	repo.WriteFile("feature.go", []byte("package main\n\n// New feature code\n"), 0644)
	repo.Add("feature.go")
	commit, _ := repo.Commit("Add new feature", nil)

	fmt.Printf("Created feature commit: %s\n", commit.Hash)

	// List all branches
	branches, err := repo.ListBranches()
	if err != nil {
		log.Fatal("Failed to list branches:", err)
	}

	fmt.Println("Branches:")
	for _, branch := range branches {
		fmt.Printf("  - %s\n", branch.Name)
	}

	// Switch back to main
	repo.Checkout("main")

	// Merge the feature branch
	if err := repo.Merge("feature/new-feature", &govc.MergeOptions{
		Message: "Merge feature/new-feature into main",
	}); err != nil {
		log.Fatal("Failed to merge:", err)
	}

	fmt.Println("Successfully merged feature branch!")
	fmt.Println()
}

func fileOperationsExample() {
	fmt.Println("=== File Operations Example ===")

	repo, _ := govc.NewRepository()
	repo.Init()

	// Create directory structure
	files := map[string]string{
		"src/main.go":      "package main\n\nfunc main() {\n\tprintln(\"Hello, govc!\")\n}\n",
		"src/utils.go":     "package main\n\n// Utility functions\n",
		"test/main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestMain(t *testing.T) {}\n",
		"README.md":        "# My Project\n\nBuilt with govc\n",
		".gitignore":       "*.tmp\n*.log\n",
	}

	// Write multiple files
	for path, content := range files {
		if err := repo.WriteFile(path, []byte(content), 0644); err != nil {
			log.Fatal("Failed to write file:", err)
		}
	}

	// List files in src directory
	srcFiles, err := repo.ListFiles("src/")
	if err != nil {
		log.Fatal("Failed to list files:", err)
	}

	fmt.Println("Files in src/:")
	for _, file := range srcFiles {
		fmt.Printf("  - %s\n", file)
	}

	// Read a file
	content, err := repo.ReadFile("src/main.go")
	if err != nil {
		log.Fatal("Failed to read file:", err)
	}

	fmt.Printf("\nContent of src/main.go:\n%s\n", string(content))

	// Move a file
	if err := repo.MoveFile("src/utils.go", "src/helpers.go"); err != nil {
		log.Fatal("Failed to move file:", err)
	}

	fmt.Println("Moved src/utils.go to src/helpers.go")

	// Stage all changes
	repo.Add(".")
	repo.Commit("Add project structure", nil)
	fmt.Println()
}

func historyExample() {
	fmt.Println("=== Commit History Example ===")

	repo, _ := govc.NewRepository()
	repo.Init()

	// Create multiple commits
	commits := []struct {
		file    string
		content string
		message string
	}{
		{"file1.txt", "Version 1", "Add file1"},
		{"file2.txt", "Version 1", "Add file2"},
		{"file1.txt", "Version 2", "Update file1"},
		{"file3.txt", "Version 1", "Add file3"},
		{"file2.txt", "Version 2", "Update file2"},
	}

	for _, c := range commits {
		repo.WriteFile(c.file, []byte(c.content), 0644)
		repo.Add(c.file)
		repo.Commit(c.message, nil)
	}

	// Get commit history
	log, err := repo.Log(&govc.LogOptions{
		MaxCount: 10,
	})
	if err != nil {
		log.Fatal("Failed to get log:", err)
	}

	fmt.Println("Commit History:")
	for i, commit := range log {
		fmt.Printf("%d. %s - %s\n", i+1, commit.Hash[:8], commit.Message)
	}

	// Get diff between commits
	if len(log) >= 2 {
		diff, err := repo.Diff(log[1].Hash, log[0].Hash)
		if err != nil {
			log.Fatal("Failed to get diff:", err)
		}

		fmt.Printf("\nDiff between last two commits:\n%s\n", diff)
	}

	// Show specific file history
	fileLog, err := repo.LogFile("file1.txt", &govc.LogOptions{})
	if err != nil {
		log.Fatal("Failed to get file log:", err)
	}

	fmt.Println("\nHistory of file1.txt:")
	for _, commit := range fileLog {
		fmt.Printf("  - %s: %s\n", commit.Hash[:8], commit.Message)
	}
}

func transactionExample() {
	fmt.Println("=== Transaction Example ===")

	repo, _ := govc.NewRepository()
	repo.Init()

	// Start a transaction
	tx, err := repo.BeginTransaction()
	if err != nil {
		log.Fatal("Failed to begin transaction:", err)
	}

	// Make multiple changes atomically
	if err := tx.WriteFile("config.json", []byte(`{"version": "1.0"}`), 0644); err != nil {
		tx.Rollback()
		log.Fatal("Failed to write config:", err)
	}

	if err := tx.WriteFile("data.json", []byte(`{"items": []}`), 0644); err != nil {
		tx.Rollback()
		log.Fatal("Failed to write data:", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		log.Fatal("Failed to commit transaction:", err)
	}

	fmt.Println("Transaction completed successfully!")
}

func parallelRealityExample() {
	fmt.Println("=== Parallel Reality Example ===")

	repo, _ := govc.NewRepository()
	repo.Init()

	// Create base state
	repo.WriteFile("main.go", []byte("package main\n\n// Original code\n"), 0644)
	repo.Add("main.go")
	repo.Commit("Initial version", nil)

	// Create a parallel reality for experimentation
	pr, err := repo.CreateParallelReality("experiment-1")
	if err != nil {
		log.Fatal("Failed to create parallel reality:", err)
	}

	// Make experimental changes
	pr.WriteFile("experimental.go", []byte("package main\n\n// Experimental feature\n"), 0644)
	pr.WriteFile("main.go", []byte("package main\n\n// Modified code\n"), 0644)
	pr.Add(".")
	pr.Commit("Experimental changes", nil)

	// Create another parallel reality
	pr2, _ := repo.CreateParallelReality("experiment-2")
	pr2.WriteFile("alternative.go", []byte("package main\n\n// Alternative approach\n"), 0644)
	pr2.Add("alternative.go")
	pr2.Commit("Alternative implementation", nil)

	// List parallel realities
	realities, _ := repo.ListParallelRealities()
	fmt.Println("Parallel Realities:")
	for _, reality := range realities {
		fmt.Printf("  - %s\n", reality.Name)
	}

	// Merge successful experiment
	if err := repo.MergeParallelReality("experiment-1"); err != nil {
		log.Fatal("Failed to merge parallel reality:", err)
	}

	fmt.Println("Successfully merged experiment-1!")
}