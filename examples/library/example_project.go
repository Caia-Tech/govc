package main

import (
	"fmt"
	"log"
	
	"github.com/caiatech/govc"
)

func main() {
	// Example 1: Basic in-memory repository
	basicExample()
	
	// Example 2: Transactional commits
	transactionExample()
	
	// Example 3: Parallel realities
	parallelExample()
	
	// Example 4: Event-driven
	eventExample()
}

func basicExample() {
	fmt.Println("\n=== Basic Repository Example ===")
	
	// Create a memory-first repository
	repo := govc.New()
	
	// Add some files
	repo.Add("README.md", []byte("# My Project\n\nThis is a test project."))
	repo.Add("main.go", []byte("package main\n\nfunc main() {\n\t// TODO\n}"))
	repo.Add(".gitignore", []byte("*.exe\n*.dll\n*.so\n*.dylib"))
	
	// Create a commit
	commit, err := repo.Commit("Initial project structure")
	if err != nil {
		log.Printf("Error committing: %v", err)
		return
	}
	
	fmt.Printf("Created commit: %s\n", commit.Hash()[:8])
	
	// Check status
	status, _ := repo.Status()
	fmt.Printf("Current branch: %s\n", status.Branch)
	fmt.Printf("Files in repo: %d\n", len(status.Tracked))
}

func transactionExample() {
	fmt.Println("\n=== Transaction Example ===")
	
	repo := govc.New()
	
	// Start a transaction
	tx := repo.Transaction()
	
	// Add multiple files atomically
	files := map[string]string{
		"src/server.go":    "package main\n\n// Server code",
		"src/client.go":    "package main\n\n// Client code",
		"src/shared.go":    "package main\n\n// Shared code",
		"config/app.yaml":  "name: myapp\nversion: 1.0",
		"config/test.yaml": "environment: test",
	}
	
	for path, content := range files {
		tx.Add(path, []byte(content))
	}
	
	// Validate before committing
	if err := tx.Validate(); err != nil {
		fmt.Printf("Validation failed: %v\n", err)
		tx.Rollback()
		return
	}
	
	// Commit the transaction
	commit, err := tx.Commit("Add application structure")
	if err != nil {
		fmt.Printf("Transaction failed: %v\n", err)
		return
	}
	
	fmt.Printf("Transaction committed: %s\n", commit.Hash()[:8])
	fmt.Printf("Files added: %d\n", len(files))
}

func parallelExample() {
	fmt.Println("\n=== Parallel Realities Example ===")
	
	repo := govc.New()
	
	// Add base configuration
	repo.Add("config.yaml", []byte("version: 1.0\nfeatures:\n  - base"))
	repo.Commit("Base configuration")
	
	// Create parallel realities for testing different configurations
	scenarios := []string{"small", "medium", "large"}
	realities := repo.ParallelRealities(scenarios)
	
	// Test different configurations in parallel
	results := make(map[string]float64)
	
	for i, reality := range realities {
		// Apply different configurations
		config := fmt.Sprintf("version: 1.0\nsize: %s\nworkers: %d", scenarios[i], (i+1)*2)
		reality.Apply(map[string][]byte{
			"config.yaml": []byte(config),
		})
		
		// Simulate performance testing
		score := float64(100 - (i * 10)) // Mock score
		results[reality.Name()] = score
		
		fmt.Printf("Reality %s: score %.2f\n", reality.Name(), score)
	}
	
	// Choose the best configuration
	var bestReality *govc.ParallelReality
	var bestScore float64
	
	for i, reality := range realities {
		if score := results[reality.Name()]; score > bestScore {
			bestScore = score
			bestReality = realities[i]
		}
	}
	
	// Merge the best configuration
	if bestReality != nil {
		fmt.Printf("Merging best reality: %s (score: %.2f)\n", bestReality.Name(), bestScore)
		// In real implementation: repo.MergeReality(bestReality)
	}
}

func eventExample() {
	fmt.Println("\n=== Event-Driven Example ===")
	
	repo := govc.NewWithConfig(govc.Config{
		EventStream: true,
		Author: govc.ConfigAuthor{
			Name:  "Event Bot",
			Email: "bot@example.com",
		},
	})
	
	// Watch for events
	events := repo.Watch(func(event govc.CommitEvent) {
		fmt.Printf("Event: %s on branch %s\n", event.Message, event.Branch)
		
		// React to specific patterns
		if contains(event.Changes, "config.yaml") {
			fmt.Println("  → Configuration changed, triggering reload...")
		}
		if contains(event.Changes, ".go") {
			fmt.Println("  → Go files changed, triggering rebuild...")
		}
	})
	
	// Simulate some changes
	repo.Add("config.yaml", []byte("version: 2.0"))
	repo.Commit("Update configuration")
	
	repo.Add("main.go", []byte("package main\n\n// Updated"))
	repo.Commit("Update main.go")
	
	// In real app, events would be processed asynchronously
	_ = events
}

func contains(slice []string, substr string) bool {
	for _, s := range slice {
		if len(s) >= len(substr) && s[len(s)-len(substr):] == substr {
			return true
		}
	}
	return false
}