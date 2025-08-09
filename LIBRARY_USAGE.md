# Using govc as a Go Library

## Installation

```bash
go get github.com/caiatech/govc
```

## Quick Start

### Basic Usage

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/caiatech/govc"
)

func main() {
    // Create a memory-first repository
    repo := govc.New()
    
    // Add files
    repo.Add("README.md", []byte("# My Project"))
    repo.Add("main.go", []byte("package main\n\nfunc main() {}"))
    
    // Commit
    commit, err := repo.Commit("Initial commit")
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Created commit: %s\n", commit.Hash())
}
```

### With Configuration

```go
package main

import (
    "github.com/caiatech/govc"
)

func main() {
    // Create with custom configuration
    repo := govc.NewWithConfig(govc.Config{
        MemoryOnly:  true,
        EventStream: true,
        Author: govc.ConfigAuthor{
            Name:  "John Doe",
            Email: "john@example.com",
        },
    })
    
    // Use the repository
    repo.Add("file.txt", []byte("content"))
    repo.Commit("Add file")
}
```

## Core API

### Repository Creation

```go
// Memory-only repository (fastest)
repo := govc.New()

// Repository with file persistence
repo, err := govc.Init("/path/to/repo")

// Open existing repository
repo, err := govc.Open("/path/to/repo")

// With configuration
repo := govc.NewWithConfig(govc.Config{
    MemoryOnly:   true,
    EventStream:  true,
    MaxRealities: 100,
})
```

### Basic Git Operations

```go
// Add files
err := repo.Add("file.txt", []byte("content"))
err := repo.AddFile("path/to/file.txt")

// Commit
commit, err := repo.Commit("Commit message")

// Status
status, err := repo.Status()
fmt.Printf("Branch: %s\n", status.Branch)
fmt.Printf("Staged: %v\n", status.Staged)
fmt.Printf("Modified: %v\n", status.Modified)

// Log
commits, err := repo.Log(10) // Last 10 commits
for _, c := range commits {
    fmt.Printf("%s - %s\n", c.Hash()[:7], c.Message())
}

// Branches
branches, err := repo.ListBranches()
err = repo.CreateBranch("feature")
err = repo.Checkout("feature")
err = repo.Merge("main")

// Tags
err = repo.CreateTag("v1.0.0", "Release version 1.0.0")
tags, err := repo.ListTags()
```

### Advanced Features

#### Parallel Realities

```go
// Create isolated branch universes for testing
realities := repo.ParallelRealities([]string{"test-a", "test-b", "test-c"})

// Work in each reality independently
for _, reality := range realities {
    reality.Apply(map[string][]byte{
        "config.yaml": []byte("version: test"),
    })
    
    score := reality.Benchmark()
    fmt.Printf("Reality %s score: %f\n", reality.Name(), score)
}

// Merge the best one
best := realities[0]
repo.MergeReality(best)
```

#### Transactional Commits

```go
// Start a transaction
tx := repo.Transaction()

// Add multiple changes
tx.Add("file1.txt", []byte("content1"))
tx.Add("file2.txt", []byte("content2"))
tx.Add("file3.txt", []byte("content3"))

// Validate before committing
if err := tx.Validate(); err != nil {
    tx.Rollback()
    return err
}

// Commit if valid
commit, err := tx.Commit("Transactional commit")
```

#### Event Streaming

```go
// Watch for repository events
events := repo.Watch()

go func() {
    for event := range events {
        fmt.Printf("Event: %s - %s\n", event.Type, event.Message)
        
        // React to specific events
        switch event.Type {
        case govc.EventCommit:
            fmt.Printf("New commit: %s\n", event.CommitHash)
        case govc.EventBranch:
            fmt.Printf("Branch operation: %s\n", event.BranchName)
        }
    }
}()
```

#### Time Travel

```go
// Create a snapshot at current state
snapshot := repo.TimeTravel(time.Now())

// Go back to a specific point
pastSnapshot := repo.TimeTravel(time.Now().Add(-24 * time.Hour))

// Read file from the past
content, err := pastSnapshot.ReadFile("config.yaml")

// Compare states
diff := repo.DiffSnapshots(pastSnapshot, snapshot)
```

## API Client Usage

### Creating a Client

```go
package main

import (
    "github.com/caiatech/govc/client/go/govc"
)

func main() {
    // Create client
    client := govc.NewClient("http://localhost:8080", govc.ClientOptions{
        APIKey: "your-api-key",
        Timeout: 30 * time.Second,
    })
    
    // List repositories
    repos, err := client.ListRepositories()
    if err != nil {
        log.Fatal(err)
    }
    
    // Create a repository
    repo, err := client.CreateRepository("my-repo", govc.RepositoryOptions{
        MemoryOnly: true,
    })
    
    // Work with the repository
    err = repo.AddFile("README.md", []byte("# Hello"))
    commit, err := repo.Commit("Initial commit")
}
```

### Authentication

```go
// With JWT token
client := govc.NewClient("http://localhost:8080", govc.ClientOptions{
    Token: "jwt-token-here",
})

// With API key
client := govc.NewClient("http://localhost:8080", govc.ClientOptions{
    APIKey: "api-key-here",
})

// Login to get token
token, err := client.Login("username", "password")
client.SetToken(token)
```

## Integration Examples

### Web Application

```go
package main

import (
    "net/http"
    "github.com/caiatech/govc"
    "github.com/gin-gonic/gin"
)

type GitService struct {
    repo *govc.Repository
}

func NewGitService() *GitService {
    return &GitService{
        repo: govc.New(),
    }
}

func (s *GitService) HandleCommit(c *gin.Context) {
    var req struct {
        Files   map[string]string `json:"files"`
        Message string            `json:"message"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // Add files
    for path, content := range req.Files {
        s.repo.Add(path, []byte(content))
    }
    
    // Commit
    commit, err := s.repo.Commit(req.Message)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{
        "hash": commit.Hash(),
        "message": "Commit created successfully",
    })
}
```

### CI/CD Pipeline

```go
package main

import (
    "github.com/caiatech/govc"
)

func runCIPipeline(repo *govc.Repository) error {
    // Create parallel realities for different test scenarios
    realities := repo.ParallelRealities([]string{
        "unit-tests",
        "integration-tests",
        "performance-tests",
    })
    
    results := make(chan TestResult, len(realities))
    
    // Run tests in parallel
    for _, reality := range realities {
        go func(r *govc.ParallelReality) {
            // Apply test configuration
            r.Apply(getTestConfig(r.Name()))
            
            // Run tests
            result := runTests(r)
            results <- result
        }(reality)
    }
    
    // Collect results
    allPassed := true
    for i := 0; i < len(realities); i++ {
        result := <-results
        if !result.Passed {
            allPassed = false
        }
    }
    
    if allPassed {
        // Merge to main
        return repo.Merge("main")
    }
    
    return fmt.Errorf("tests failed")
}
```

### Infrastructure as Code

```go
package main

import (
    "github.com/caiatech/govc"
)

type InfraManager struct {
    repo *govc.Repository
}

func (m *InfraManager) DeployCanary(config Config) error {
    // Create canary branch
    canary := m.repo.ParallelReality("canary-" + config.Version)
    
    // Apply new configuration
    canary.Apply(map[string][]byte{
        "k8s/deployment.yaml": config.ToYAML(),
        "terraform/main.tf":   config.ToTerraform(),
    })
    
    // Validate configuration
    if err := canary.Validate(); err != nil {
        return err
    }
    
    // Deploy to 10% of traffic
    if err := deployCanary(canary, 0.1); err != nil {
        canary.Destroy()
        return err
    }
    
    // Monitor metrics
    metrics := monitorCanary(canary, 5*time.Minute)
    
    if metrics.ErrorRate < 0.01 {
        // Promote canary
        m.repo.MergeReality(canary)
        return deployFull(m.repo)
    }
    
    // Rollback
    canary.Destroy()
    return fmt.Errorf("canary failed with error rate: %f", metrics.ErrorRate)
}
```

## Best Practices

### 1. Resource Management

```go
// Always close repositories when done
repo, err := govc.Open("/path/to/repo")
if err != nil {
    return err
}
defer repo.Close()
```

### 2. Error Handling

```go
// Check all errors
commit, err := repo.Commit("message")
if err != nil {
    // Handle specific error types
    switch err {
    case govc.ErrNoChanges:
        // Nothing to commit
        return nil
    case govc.ErrConflict:
        // Handle merge conflict
        return resolveMergeConflict(repo)
    default:
        return fmt.Errorf("commit failed: %w", err)
    }
}
```

### 3. Transaction Pattern

```go
func atomicUpdate(repo *govc.Repository, updates map[string][]byte) error {
    tx := repo.Transaction()
    defer tx.Rollback() // Rollback if not committed
    
    for path, content := range updates {
        if err := tx.Add(path, content); err != nil {
            return err
        }
    }
    
    if err := tx.Validate(); err != nil {
        return err
    }
    
    _, err := tx.Commit("Atomic update")
    return err
}
```

### 4. Concurrent Operations

```go
// Use parallel realities for concurrent work
func processConcurrently(repo *govc.Repository, items []Item) error {
    var wg sync.WaitGroup
    errors := make(chan error, len(items))
    
    for _, item := range items {
        wg.Add(1)
        go func(item Item) {
            defer wg.Done()
            
            // Each goroutine gets its own reality
            reality := repo.ParallelReality(item.ID)
            defer reality.Destroy()
            
            if err := processItem(reality, item); err != nil {
                errors <- err
                return
            }
            
            // Merge successful changes
            if err := repo.MergeReality(reality); err != nil {
                errors <- err
            }
        }(item)
    }
    
    wg.Wait()
    close(errors)
    
    // Check for errors
    for err := range errors {
        if err != nil {
            return err
        }
    }
    
    return nil
}
```

## Package Structure

When importing govc, you have access to these packages:

```go
import (
    "github.com/caiatech/govc"           // Main library
    "github.com/caiatech/govc/client/go/govc" // API client
    "github.com/caiatech/govc/pkg/object"     // Git objects
    "github.com/caiatech/govc/pkg/refs"       // References
    "github.com/caiatech/govc/pkg/storage"    // Storage backends
)
```

## Type Reference

### Core Types

```go
// Repository operations
type Repository struct { ... }
type ParallelReality struct { ... }
type TransactionalCommit struct { ... }
type HistoricalSnapshot struct { ... }

// Git objects
type Commit struct { ... }
type Tree struct { ... }
type Blob struct { ... }
type Tag struct { ... }

// Events
type CommitEvent struct { ... }
type RepositoryEvent struct { ... }

// Status
type Status struct {
    Branch    string
    Staged    []string
    Modified  []string
    Untracked []string
}
```

## Migration from go-git

If you're migrating from go-git, here's a comparison:

```go
// go-git
import "github.com/go-git/go-git/v5"
repo, _ := git.PlainOpen(".")
w, _ := repo.Worktree()
w.Add("file.txt")
w.Commit("message", &git.CommitOptions{})

// govc (simpler API)
import "github.com/caiatech/govc"
repo, _ := govc.Open(".")
repo.Add("file.txt")
repo.Commit("message")
```

## Performance Tips

1. **Use memory-only repos for testing** - 40x faster than disk
2. **Batch operations** - Use transactions for multiple changes
3. **Parallel realities** - Process independent changes concurrently
4. **Event streaming** - React to changes instead of polling

## Support

- Documentation: https://github.com/caiatech/govc
- Issues: https://github.com/caiatech/govc/issues
- Examples: https://github.com/caiatech/govc/tree/main/examples

## License

Apache 2.0 - See LICENSE file