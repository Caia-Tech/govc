package main

import (
	"fmt"
	"log"
	"strings"

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
	repo := govc.New()

	// Create and write a file
	content := []byte("# govc Example\n\nThis is a demonstration of govc capabilities.")
	if err := repo.WriteFile("README.md", content); err != nil {
		log.Fatal("Failed to write file:", err)
	}

	// Stage the file
	if err := repo.Add("README.md"); err != nil {
		log.Fatal("Failed to stage file:", err)
	}

	// Create a commit
	commit, err := repo.Commit("Initial commit")
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

	repo := govc.New()

	// Create initial commit
	repo.WriteFile("main.go", []byte("package main\n\nfunc main() {}\n"))
	repo.Add("main.go")
	repo.Commit("Initial commit")

	// Create a new branch
	if err := repo.Branch("feature/new-feature"); err != nil {
		log.Fatal("Failed to create branch:", err)
	}

	// Switch to the new branch
	if err := repo.Checkout("feature/new-feature"); err != nil {
		log.Fatal("Failed to checkout branch:", err)
	}

	// Make changes on the feature branch
	repo.WriteFile("feature.go", []byte("package main\n\n// New feature code\n"))
	repo.Add("feature.go")
	commit, _ := repo.Commit("Add new feature")

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
	if err := repo.Merge("feature/new-feature", "Merge feature branch"); err != nil {
		log.Fatal("Failed to merge:", err)
	}

	fmt.Println("Successfully merged feature branch!")
	fmt.Println()
}

func fileOperationsExample() {
	fmt.Println("=== File Operations Example ===")

	repo := govc.New()

	// Create directory structure
	files := map[string]string{
		"src/main.go":      "package main\n\nfunc main() {\n\tprintln(\"Hello, govc!\")\n}\n",
		"src/utils.go":     "package main\n\n// Utility functions\n",
		"test/main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestMain(t *testing.T) {}\n",
		"README.md":        "# My Project\n\nBuilt with govc\n",
		".gitignore":       "*.tmp\n*.commits\n",
	}

	// Write multiple files
	for path, content := range files {
		if err := repo.WriteFile(path, []byte(content)); err != nil {
			log.Fatal("Failed to write file:", err)
		}
	}

	// List all files
	srcFiles, err := repo.ListFiles()
	if err != nil {
		log.Fatal("Failed to list files:", err)
	}

	fmt.Println("Files in repository:")
	for _, file := range srcFiles {
		if strings.HasPrefix(file, "src/") {
			fmt.Printf("  - %s\n", file)
		}
	}

	// Read a file
	content, err := repo.ReadFile("src/main.go")
	if err != nil {
		log.Fatal("Failed to read file:", err)
	}

	fmt.Printf("\nContent of src/main.go:\n%s\n", string(content))

	// Note: File moving would be done by deleting and re-adding
	// repo.DeleteFile("src/utils.go")
	// repo.WriteFile("src/helpers.go", content)
	fmt.Println("Note: Use DeleteFile + WriteFile to move files")

	// Stage all changes
	repo.Add(".")
	repo.Commit("Add project structure")
	fmt.Println()
}

func historyExample() {
	fmt.Println("=== Commit History Example ===")

	repo := govc.New()

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
		repo.WriteFile(c.file, []byte(c.content))
		repo.Add(c.file)
		repo.Commit(c.message)
	}

	// Get commit history
	commitList, err := repo.Log(0) // 0 means get all commits
	if err != nil {
		log.Fatal("Failed to get commits:", err)
	}

	fmt.Println("Commit History:")
	for i, commit := range commitList {
		fmt.Printf("%d. %s - %s\n", i+1, commit.Hash()[:8], commit.Message)
	}

	// Get diff between commits
	if len(commitList) >= 2 {
		diff, err := repo.Diff(commitList[1].Hash(), commitList[0].Hash(), "")
		if err != nil {
			log.Fatal("Failed to get diff:", err)
		}

		fmt.Printf("\nDiff between last two commits:\n%s\n", diff)
	}

	// Note: File-specific history would need to be filtered from full log
	fmt.Println("\nNote: Filter commit log to get file-specific history")
}

func transactionExample() {
	fmt.Println("=== Transaction Example ===")

	repo := govc.New()

	// Use the Transaction() method to get a transactional commit
	tx := repo.Transaction()

	// Make multiple changes atomically
	tx.Add("config.json", []byte(`{"version": "1.0"}`))
	tx.Add("data.json", []byte(`{"items": []}`))

	// Validate before committing
	if err := tx.Validate(); err != nil {
		tx.Rollback()
		log.Fatal("Validation failed:", err)
	}

	// Commit the transaction
	if _, err := tx.Commit("Add configuration files"); err != nil {
		log.Fatal("Failed to commit transaction:", err)
	}

	fmt.Println("Transaction completed successfully!")
}

func parallelRealityExample() {
	fmt.Println("=== Parallel Reality Example ===")

	repo := govc.New()

	// Create base state
	repo.WriteFile("main.go", []byte("package main\n\n// Original code\n"))
	repo.Add("main.go")
	repo.Commit("Initial version")

	// Create parallel realities for experimentation
	realities := repo.ParallelRealities([]string{"experiment-1", "experiment-2"})

	// Make experimental changes in first reality
	pr := realities[0]
	pr.Apply(map[string][]byte{
		"experimental.go": []byte("package main\n\n// Experimental feature\n"),
	})
	// Note: Complete parallel reality implementation would go here
	fmt.Println("Parallel realities created for testing")
}