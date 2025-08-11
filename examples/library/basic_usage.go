package main

import (
	"fmt"
	"log"

	"github.com/Caia-Tech/govc"
)

func main() {
	// Example 1: Memory-first repository (recommended)
	memoryFirstExample()

	// Example 2: Repository with file persistence
	filePersistenceExample()

	// Example 3: Advanced configuration
	advancedExample()
}

func memoryFirstExample() {
	fmt.Println("=== Memory-First Repository ===")

	// Create a pure in-memory repository
	// All operations happen in RAM - instant and parallel
	repo := govc.New()

	// Start a transaction
	tx := repo.Transaction()
	tx.Add("config.yaml", []byte("version: 1.0\nname: myapp"))
	tx.Add("data.json", []byte(`{"status": "active"}`))

	// Validate before committing
	if err := tx.Validate(); err != nil {
		log.Printf("Validation failed: %v", err)
		tx.Rollback()
		return
	}

	// Commit the transaction
	commit, err := tx.Commit("Initial configuration")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Created commit: %s\n", commit.Hash()[:7])

	// Create branches instantly
	repo.Branch("feature-x").Create()
	repo.Branch("hotfix").Create()

	// List branches
	branches, _ := repo.ListBranches()
	fmt.Printf("Branches: %d\n", len(branches))
}

func filePersistenceExample() {
	fmt.Println("\n=== File Persistence Repository ===")

	// Initialize repository with file persistence
	repo, err := govc.Init("/tmp/myrepo")
	if err != nil {
		log.Fatal(err)
	}

	// Add files from disk
	err = repo.Add("*.go")
	if err != nil {
		log.Printf("Add error: %v", err)
	}

	// Commit changes
	commit, err := repo.Commit("Add Go files")
	if err != nil {
		log.Printf("Commit error: %v", err)
	} else {
		fmt.Printf("Committed: %s\n", commit.Message)
	}

	// Open existing repository
	_, err = govc.Open("/tmp/myrepo")
	if err != nil {
		log.Printf("Open error: %v", err)
	} else {
		fmt.Println("Successfully opened repository")
	}
}

func advancedExample() {
	fmt.Println("\n=== Advanced Features ===")

	// Create repository with configuration
	repo := govc.NewWithConfig(govc.Config{
		MemoryOnly: true,
		Author: govc.ConfigAuthor{
			Name:  "John Doe",
			Email: "john@example.com",
		},
	})

	// Create parallel realities for testing
	realities := repo.ParallelRealities([]string{
		"test-config-a",
		"test-config-b",
		"test-config-c",
	})

	fmt.Printf("Created %d parallel realities\n", len(realities))

	// Apply different configurations to each reality
	realities[0].Apply(map[string][]byte{
		"config.yaml": []byte("cache: enabled\nsize: large"),
	})

	realities[1].Apply(map[string][]byte{
		"config.yaml": []byte("cache: disabled\nsize: small"),
	})

	realities[2].Apply(map[string][]byte{
		"config.yaml": []byte("cache: enabled\nsize: medium"),
	})

	// Benchmark each reality
	for i, reality := range realities {
		result := reality.Benchmark()
		fmt.Printf("Reality %d performance: %v\n", i, result.Better())
	}

	// Watch for events
	repo.Watch(func(event govc.Event) {
		fmt.Printf("Event: %s by %s\n", event.Message, event.Author)
	})

	// Time travel
	snapshot := repo.TimeTravel(realities[0].Benchmark().StartTime)
	if snapshot != nil {
		fmt.Println("Successfully traveled to the past!")
	}
}

// Example: Infrastructure testing pattern
func infrastructureTestingPattern() {
	repo := govc.New()

	// Test multiple database configurations
	configs := []struct {
		name string
		data map[string][]byte
	}{
		{
			"postgres-config",
			map[string][]byte{
				"db.conf": []byte("type: postgres\npool: 50"),
			},
		},
		{
			"mysql-config",
			map[string][]byte{
				"db.conf": []byte("type: mysql\npool: 100"),
			},
		},
	}

	// Test all configurations in parallel
	realities := make([]*govc.Reality, 0)
	for _, cfg := range configs {
		reality := repo.ParallelReality(cfg.name)
		reality.Apply(cfg.data)
		realities = append(realities, reality)
	}

	// Find best performing configuration
	var bestConfig string
	var bestScore float64

	for i, reality := range realities {
		reality.Benchmark()
		score := 100.0 // Simulated score
		if score > bestScore {
			bestScore = score
			bestConfig = configs[i].name
		}
	}

	fmt.Printf("Best configuration: %s\n", bestConfig)

	// Merge winning configuration to main
	repo.Merge(bestConfig, "main")
}
