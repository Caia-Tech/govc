package govc_test

import (
	"fmt"
	"log"
	"time"

	"github.com/caiatech/govc"
)

// Example demonstrates basic govc usage as a library
func Example() {
	// Create a memory-first repository
	repo := govc.New()

	// Start a transaction
	tx := repo.Transaction()
	tx.Add("config.yaml", []byte("version: 1.0"))
	tx.Add("app.conf", []byte("debug: false"))

	// Validate before committing
	if err := tx.Validate(); err != nil {
		log.Fatal(err)
	}

	// Commit the transaction
	commit, err := tx.Commit("Initial configuration")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Created commit %s\n", commit.Hash()[:7])
}

// ExampleRepository_ParallelRealities demonstrates testing multiple configurations
func ExampleRepository_ParallelRealities() {
	repo := govc.New()

	// Create parallel realities for A/B testing
	realities := repo.ParallelRealities([]string{
		"config-aggressive-caching",
		"config-moderate-caching",
		"config-no-caching",
	})

	// Apply different configurations
	realities[0].Apply(map[string][]byte{
		"cache.conf": []byte("ttl: 3600\nsize: 10GB"),
	})
	realities[1].Apply(map[string][]byte{
		"cache.conf": []byte("ttl: 1800\nsize: 5GB"),
	})
	realities[2].Apply(map[string][]byte{
		"cache.conf": []byte("enabled: false"),
	})

	// Test each configuration
	for i, reality := range realities {
		result := reality.Benchmark()
		fmt.Printf("Configuration %d ready for testing\n", i)
		_ = result.Better() // Would evaluate performance
	}

	// Output:
	// Configuration 0 ready for testing
	// Configuration 1 ready for testing
	// Configuration 2 ready for testing
}

// ExampleRepository_Transaction demonstrates safe transactional commits
func ExampleRepository_Transaction() {
	repo := govc.New()

	// Start a transaction for multiple related changes
	tx := repo.Transaction()

	// Add multiple configuration files
	tx.Add("database.yaml", []byte("host: localhost\nport: 5432"))
	tx.Add("redis.yaml", []byte("host: localhost\nport: 6379"))
	tx.Add("app.yaml", []byte("workers: 4\ntimeout: 30"))

	// Validate all changes together
	if err := tx.Validate(); err != nil {
		// Rollback if validation fails
		tx.Rollback()
		fmt.Println("Validation failed, changes rolled back")
		return
	}

	// Commit atomically
	commit, _ := tx.Commit("Update service configurations")
	fmt.Printf("All configurations updated in commit %s\n", commit.Hash()[:7])
}

// ExampleRepository_Watch demonstrates reactive infrastructure
func ExampleRepository_Watch() {
	repo := govc.New()

	// Set up event handler
	repo.Watch(func(event govc.Event) {
		// React to different types of commits
		switch {
		case event.Message == "EMERGENCY_ROLLBACK":
			fmt.Printf("Emergency rollback triggered by %s\n", event.Author)
		case event.Author == "ci-bot":
			fmt.Printf("Automated deployment: %s\n", event.Message)
		default:
			fmt.Printf("Commit by %s: %s\n", event.Author, event.Message)
		}
	})

	// Simulate some commits
	tx := repo.Transaction()
	tx.Add("status.txt", []byte("emergency"))
	tx.Validate()
	tx.Commit("EMERGENCY_ROLLBACK")

	// In practice, the watch handler would trigger actual rollback procedures
}

// ExampleRepository_TimeTravel demonstrates debugging with time travel
func ExampleRepository_TimeTravel() {
	repo := govc.New()

	// Create some history with explicit time delays
	tx1 := repo.Transaction()
	tx1.Add("config.yaml", []byte("version: 1.0\nstable: true"))
	tx1.Validate()
	commit1, _ := tx1.Commit("Stable configuration")
	
	// Wait to ensure different timestamps
	time.Sleep(10 * time.Millisecond)

	tx2 := repo.Transaction()
	tx2.Add("config.yaml", []byte("version: 2.0\nstable: false"))
	tx2.Validate()
	tx2.Commit("Experimental configuration")
	
	// Travel back to stable version
	targetTime := commit1.Author.Time.Add(1 * time.Millisecond)
	
	snapshot := repo.TimeTravel(targetTime)
	if snapshot != nil {
		content, _ := snapshot.Read("config.yaml")
		fmt.Printf("Configuration at stable point:\n%s\n", content)
	}

	// Output:
	// Configuration at stable point:
	// version: 1.0
	// stable: true
}

// ExampleRepository_Branch demonstrates branch operations
func ExampleRepository_Branch() {
	repo := govc.New()

	// Initialize with main branch
	tx := repo.Transaction()
	tx.Add("version.txt", []byte("1.0"))
	tx.Validate()
	tx.Commit("Initial version")

	// Create feature branch
	err := repo.Branch("feature/new-api").Create()
	if err != nil {
		log.Fatal(err)
	}

	// Create and checkout hotfix branch
	err = repo.Branch("hotfix/security-patch").Checkout()
	if err != nil {
		log.Fatal(err)
	}

	// Make changes on hotfix branch
	hotfixTx := repo.Transaction()
	hotfixTx.Add("security.patch", []byte("CVE-2024-001 fixed"))
	hotfixTx.Validate()
	hotfixTx.Commit("Apply security patch")

	fmt.Println("Hotfix branch created and patch applied")

	// Output:
	// Hotfix branch created and patch applied
}

// ExampleNew demonstrates creating different repository types
func ExampleNew() {
	// Memory-first repository (recommended)
	memRepo := govc.New()
	fmt.Println("Memory repository created")

	// Repository with custom configuration
	configuredRepo := govc.NewWithConfig(govc.Config{
		MemoryOnly: true,
		Author: govc.ConfigAuthor{
			Name:  "Infrastructure Bot",
			Email: "bot@example.com",
		},
	})
	fmt.Println("Configured repository created")

	_ = memRepo
	_ = configuredRepo

	// Output:
	// Memory repository created
	// Configured repository created
}

// ExampleInit demonstrates file-persisted repository
func ExampleInit() {
	// Initialize repository with file persistence
	repo, err := govc.Init("/tmp/my-infrastructure")
	if err != nil {
		log.Fatal(err)
	}

	// Can be reopened later
	repo2, err := govc.Open("/tmp/my-infrastructure")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Repository initialized and reopened")
	_ = repo
	_ = repo2

	// Output:
	// Repository initialized and reopened
}

// Example_infrastructureTesting shows a complete infrastructure testing workflow
func Example_infrastructureTesting() {
	// Create repository for infrastructure state
	repo := govc.New()

	// Define configurations to test
	configs := map[string]map[string][]byte{
		"high-performance": {
			"nginx.conf": []byte("worker_processes auto;\nworker_connections 4096;"),
			"cache.conf": []byte("size=10GB\nttl=3600"),
		},
		"balanced": {
			"nginx.conf": []byte("worker_processes 4;\nworker_connections 1024;"),
			"cache.conf": []byte("size=5GB\nttl=1800"),
		},
		"low-resource": {
			"nginx.conf": []byte("worker_processes 2;\nworker_connections 512;"),
			"cache.conf": []byte("size=1GB\nttl=900"),
		},
	}

	// Test all configurations in parallel
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	realities := repo.ParallelRealities(names)

	// Apply configurations
	i := 0
	for name, config := range configs {
		realities[i].Apply(config)
		fmt.Printf("Testing %s configuration\n", name)
		i++
	}

	// In practice, you would:
	// 1. Run performance tests on each reality
	// 2. Collect metrics
	// 3. Choose the best configuration
	// 4. Merge it to main branch

	// Output:
	// Testing high-performance configuration
	// Testing balanced configuration
	// Testing low-resource configuration
}

// Example_disasterRecovery demonstrates instant rollback capabilities
func Example_disasterRecovery() {
	repo := govc.New()

	// Create backup points
	backups := []string{"backup-1", "backup-2", "backup-3"}
	
	for i, backup := range backups {
		tx := repo.Transaction()
		tx.Add("system-state.json", []byte(fmt.Sprintf(`{"version": %d, "healthy": true}`, i+1)))
		tx.Validate()
		commit, _ := tx.Commit(backup)
		fmt.Printf("Created %s at %s\n", backup, commit.Hash()[:7])
	}

	// Simulate disaster
	disasterTx := repo.Transaction()
	disasterTx.Add("system-state.json", []byte(`{"version": 999, "healthy": false, "error": "critical failure"}`))
	disasterTx.Validate()
	disasterTx.Commit("System failure")

	// Instant rollback
	fmt.Println("Disaster detected! Rolling back to backup-3...")
	// In practice: repo.Checkout("backup-3")

	// Output:
	// Created backup-1 at [a-f0-9]{7}
	// Created backup-2 at [a-f0-9]{7}
	// Created backup-3 at [a-f0-9]{7}
	// Disaster detected! Rolling back to backup-3...
}