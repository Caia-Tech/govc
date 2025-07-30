package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/caiatech/govc/client/go/govc"
)

func main() {
	// Get server URL from environment or use default
	serverURL := os.Getenv("GOVC_SERVER")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	// Create client
	client, err := govc.NewClient(serverURL)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()

	// Check server health
	fmt.Println("Checking server health...")
	health, err := client.HealthCheck(ctx)
	if err != nil {
		log.Printf("Warning: Health check failed: %v", err)
	} else {
		fmt.Printf("Server status: %s, version: %s\n", health.Status, health.Version)
	}

	// Create a repository
	fmt.Println("\nCreating repository...")
	repo, err := client.CreateRepo(ctx, "example-repo", &govc.CreateRepoOptions{
		MemoryOnly: true,
	})
	if err != nil {
		log.Fatalf("Failed to create repository: %v", err)
	}
	fmt.Printf("Created repository: %s\n", repo.ID)

	// Add some files
	fmt.Println("\nAdding files...")
	files := map[string]string{
		"README.md":   "# Example Repository\n\nThis is a demo repository created with govc Go client.",
		"main.go":     "package main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"Hello, govc!\")\n}",
		".gitignore":  "*.exe\n*.dll\n*.so\n*.dylib\nvendor/\n",
	}

	for path, content := range files {
		if err := repo.AddFile(ctx, path, content); err != nil {
			log.Printf("Failed to add %s: %v", path, err)
		} else {
			fmt.Printf("Added: %s\n", path)
		}
	}

	// Check status
	fmt.Println("\nRepository status:")
	status, err := repo.Status(ctx)
	if err != nil {
		log.Printf("Failed to get status: %v", err)
	} else {
		fmt.Printf("Branch: %s\n", status.Branch)
		fmt.Printf("Staged files: %v\n", status.Staged)
		fmt.Printf("Clean: %v\n", status.Clean)
	}

	// Commit changes
	fmt.Println("\nCommitting changes...")
	commit, err := repo.Commit(ctx, "Initial commit", &govc.Author{
		Name:  "Example User",
		Email: "user@example.com",
	})
	if err != nil {
		log.Fatalf("Failed to commit: %v", err)
	}
	fmt.Printf("Created commit: %s\n", commit.Hash)

	// Create a feature branch
	fmt.Println("\nCreating feature branch...")
	if err := repo.CreateBranch(ctx, "feature/awesome", ""); err != nil {
		log.Printf("Failed to create branch: %v", err)
	}

	// List branches
	branches, err := repo.ListBranches(ctx)
	if err != nil {
		log.Printf("Failed to list branches: %v", err)
	} else {
		fmt.Println("\nBranches:")
		for _, branch := range branches {
			marker := ""
			if branch.IsCurrent {
				marker = " *"
			}
			fmt.Printf("  %s%s\n", branch.Name, marker)
		}
	}

	// Get commit log
	fmt.Println("\nCommit log:")
	commits, err := repo.Log(ctx, 10)
	if err != nil {
		log.Printf("Failed to get log: %v", err)
	} else {
		for _, c := range commits {
			fmt.Printf("  %s - %s (%s)\n", c.Hash[:7], c.Message, c.Author)
		}
	}

	// Cleanup (optional)
	fmt.Println("\nCleaning up...")
	if err := client.DeleteRepo(ctx, repo.ID); err != nil {
		log.Printf("Failed to delete repository: %v", err)
	} else {
		fmt.Println("Repository deleted successfully")
	}
}