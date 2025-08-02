package main

import (
	"fmt"
	"log"

	"github.com/caiatech/govc"
)

func main() {
	// Method 1: QuickStart - Everything pre-configured
	quickStartExample()

	// Method 2: Custom setup - More control
	customSetupExample()

	// Method 3: Backward compatible
	backwardCompatibleExample()
}

func quickStartExample() {
	fmt.Println("=== QuickStart Example ===")
	
	// Create a complete in-memory Git environment
	qs := govc.NewQuickStartV2()
	
	// Configure user
	qs.Config.Set("user.name", "Quick Start")
	qs.Config.Set("user.email", "quick@example.com")
	
	// Create and commit a file
	err := qs.Workspace.WriteFile("hello.txt", []byte("Hello, World!"))
	if err != nil {
		log.Fatal(err)
	}
	
	err = qs.Operations.Add("hello.txt")
	if err != nil {
		log.Fatal(err)
	}
	
	hash, err := qs.Operations.Commit("Initial commit")
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Created commit: %s\n", hash)
	
	// Create a branch
	err = qs.Operations.Branch("feature")
	if err != nil {
		log.Fatal(err)
	}
	
	// List branches
	branches, err := qs.Repository.ListBranches()
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Println("Branches:")
	for _, branch := range branches {
		fmt.Printf("  - %s (%s)\n", branch.Name, branch.Hash[:7])
	}
	
	// Work with stashes
	err = qs.Workspace.WriteFile("hello.txt", []byte("Modified content"))
	if err != nil {
		log.Fatal(err)
	}
	
	stash, err := qs.Stash.Create("WIP: working on feature")
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Created stash: %s\n", stash.ID[:7])
	
	// Setup webhook
	webhook, err := qs.Webhooks.Register("https://example.com/hook", []string{"push"}, "secret")
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Registered webhook: %s\n", webhook.ID[:7])
	
	fmt.Println()
}

func customSetupExample() {
	fmt.Println("=== Custom Setup Example ===")
	
	// Import core package at the top of the file
	// import "github.com/caiatech/govc/pkg/core"
	
	// For this example, we'll use QuickStart to demonstrate
	// In real code, you would import core and use it directly
	qs := govc.NewQuickStartV2()
	
	// You can access individual components
	workspace := qs.Workspace
	config := qs.Config
	ops := qs.Operations
	
	// Already initialized by QuickStart
	// In real custom setup, you would call ops.Init()
	
	// Configure
	config.Set("user.name", "Custom User")
	config.Set("user.email", "custom@example.com")
	
	// Work with repository
	err := workspace.WriteFile("custom.txt", []byte("Custom setup"))
	if err != nil {
		log.Fatal(err)
	}
	
	err = ops.Add("custom.txt")
	if err != nil {
		log.Fatal(err)
	}
	
	hash, err := ops.Commit("Custom commit")
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Custom setup commit: %s\n", hash)
	
	// Show how to access the underlying stores
	fmt.Println("Repository uses clean architecture:")
	fmt.Printf("  - Immutable objects in ObjectStore\n")
	fmt.Printf("  - References in RefStore\n")
	fmt.Printf("  - Working files in WorkingStorage\n")
	fmt.Printf("  - Config in ConfigStore\n")
	fmt.Println()
}

func backwardCompatibleExample() {
	fmt.Println("=== Backward Compatible Example ===")
	
	// Use RepositoryV2 which has a similar API to the old Repository
	repo := govc.NewMemoryRepositoryV2()
	
	// Configure
	repo.SetConfig("user.name", "Legacy User")
	repo.SetConfig("user.email", "legacy@example.com")
	
	// Old-style operations still work
	err := repo.WriteFile("legacy.txt", []byte("Legacy API"))
	if err != nil {
		log.Fatal(err)
	}
	
	err = repo.Add("legacy.txt")
	if err != nil {
		log.Fatal(err)
	}
	
	// Commit (note: author parameter is ignored, uses config)
	hash, err := repo.Commit("Legacy commit", nil)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Legacy API commit: %s\n", hash)
	
	// Status
	status, err := repo.Status()
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Status: branch=%s, clean=%v\n", status.Branch, status.Clean())
	
	// Create branch
	err = repo.CreateBranch("legacy-feature")
	if err != nil {
		log.Fatal(err)
	}
	
	// List branches
	branches, err := repo.ListBranches()
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Println("Branches:", branches)
	fmt.Println()
}

// Example: Building a Git-like CLI
func cliExample() {
	// This shows how you might build a Git-like CLI using the new architecture
	_ = govc.NewQuickStartV2()
	
	// Parse command line arguments (pseudo-code)
	// cmd := os.Args[1]
	// switch cmd {
	// case "init":
	//     err := qs.Operations.Init()
	// case "add":
	//     err := qs.Operations.Add(os.Args[2:]...)
	// case "commit":
	//     hash, err := qs.Operations.Commit(os.Args[2])
	// case "status":
	//     status, err := qs.Operations.Status()
	//     // Print status
	// case "branch":
	//     if len(os.Args) > 2 {
	//         err := qs.Operations.Branch(os.Args[2])
	//     } else {
	//         branches, err := qs.Repository.ListBranches()
	//         // Print branches
	//     }
	// case "stash":
	//     stash, err := qs.Stash.Create("WIP")
	//     // Print stash info
	// }
}