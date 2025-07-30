# govc Quick Start Guide

Get up and running with govc in 5 minutes!

## Installation

```bash
go get github.com/caiatech/govc
```

## Basic Example

```go
package main

import (
    "fmt"
    "log"
    "github.com/caiatech/govc"
)

func main() {
    // Create repository
    repo, err := govc.NewRepository()
    if err != nil {
        log.Fatal(err)
    }
    
    // Initialize
    repo.Init()
    
    // Create a file
    repo.WriteFile("hello.txt", []byte("Hello, govc!"), 0644)
    
    // Stage and commit
    repo.Add("hello.txt")
    commit, _ := repo.Commit("First commit", nil)
    
    fmt.Printf("Created commit: %s\n", commit.Hash)
}
```

## Key Features

### 1. In-Memory Operations (40x faster than disk)
```go
// All operations happen in memory by default
repo, _ := govc.NewRepository()
```

### 2. Branching
```go
// Create and switch branches
repo.Branch("feature")
repo.Checkout("feature")
```

### 3. Parallel Realities
```go
// Experiment without affecting main
pr, _ := repo.CreateParallelReality("experiment")
pr.WriteFile("test.go", []byte("experimental code"), 0644)
```

### 4. Time Travel
```go
// Go back to any point in history
repo.TimeTravel(commitHash)
```

### 5. Transactions
```go
// Atomic operations
tx, _ := repo.BeginTransaction()
tx.WriteFile("file1.txt", []byte("data1"), 0644)
tx.WriteFile("file2.txt", []byte("data2"), 0644)
tx.Commit() // All or nothing
```

## Next Steps

- [Full Integration Guide](INTEGRATION.md) - Comprehensive integration documentation
- [API Reference](API_REFERENCE.md) - Complete API documentation
- [Examples](examples/) - Working code examples
- [Server Setup](README.md#server-mode) - Run govc as a server

## Common Use Cases

### Configuration Management
```go
// Track infrastructure configs
repo.WriteFile("nginx.conf", nginxConfig, 0644)
repo.Add("nginx.conf")
repo.Commit("Update nginx config", nil)
```

### A/B Testing
```go
// Test different versions
realities := repo.ParallelRealities([]string{"variant-a", "variant-b"})
// Run experiments in isolation
```

### Instant Rollback
```go
// Something went wrong?
repo.TimeTravel(lastGoodCommit)
```

## Performance Tips

1. **Use transactions** for multiple operations
2. **Enable caching** for read-heavy workloads
3. **Use parallel realities** instead of branches for experiments
4. **Keep repositories focused** - one repo per logical unit

## Getting Help

- Issues: https://github.com/caiatech/govc/issues
- Docs: https://docs.caiatech.com/govc
- Examples: [examples/](examples/)