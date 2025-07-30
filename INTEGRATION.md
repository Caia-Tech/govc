# govc Integration Guide

This guide explains how to integrate govc into your Go projects as a high-performance, memory-first Git implementation.

## Table of Contents
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Core Concepts](#core-concepts)
- [Basic Usage](#basic-usage)
- [Advanced Features](#advanced-features)
- [API Reference](#api-reference)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Installation

```bash
go get github.com/caiatech/govc
```

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/caiatech/govc"
)

func main() {
    // Create a new repository
    repo, err := govc.NewRepository()
    if err != nil {
        log.Fatal(err)
    }
    
    // Initialize the repository
    if err := repo.Init(); err != nil {
        log.Fatal(err)
    }
    
    // Add a file
    if err := repo.WriteFile("README.md", []byte("# My Project"), 0644); err != nil {
        log.Fatal(err)
    }
    
    // Stage and commit
    if err := repo.Add("README.md"); err != nil {
        log.Fatal(err)
    }
    
    commit, err := repo.Commit("Initial commit", &govc.CommitOptions{
        Author: "John Doe <john@example.com>",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Created commit: %s\n", commit.Hash)
}
```

## Core Concepts

### Repository

The `Repository` is the main interface for interacting with govc:

```go
type Repository interface {
    // Initialization
    Init() error
    Clone(url string) error
    
    // File operations
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, content []byte, mode os.FileMode) error
    DeleteFile(path string) error
    ListFiles(path string) ([]string, error)
    
    // Git operations
    Add(path string) error
    Commit(message string, opts *CommitOptions) (*Commit, error)
    Branch(name string) error
    Checkout(ref string) error
    Merge(branch string, opts *MergeOptions) error
    
    // Advanced features
    CreateParallelReality(name string) (*ParallelReality, error)
    TimeTravel(commitHash string) error
}
```

### Transactions

Use transactions for atomic operations:

```go
tx, err := repo.BeginTransaction()
if err != nil {
    return err
}
defer tx.Rollback() // Rollback if not committed

// Multiple operations in a transaction
if err := tx.WriteFile("file1.txt", data1, 0644); err != nil {
    return err
}
if err := tx.WriteFile("file2.txt", data2, 0644); err != nil {
    return err
}

// Commit the transaction
if err := tx.Commit(); err != nil {
    return err
}
```

## Basic Usage

### Creating and Managing Repositories

```go
// Create in-memory repository
repo, err := govc.NewRepository()

// Create with custom storage path
repo, err := govc.NewRepositoryWithPath("/path/to/storage")

// Create with options
repo, err := govc.NewRepositoryWithOptions(govc.RepositoryOptions{
    Path:       "/path/to/storage",
    InMemory:   true,
    Compressed: true,
})
```

### Working with Files

```go
// Write a file
err := repo.WriteFile("src/main.go", []byte(sourceCode), 0644)

// Read a file
content, err := repo.ReadFile("src/main.go")

// List files in directory
files, err := repo.ListFiles("src/")

// Delete a file
err := repo.DeleteFile("old-file.txt")

// Move/rename a file
err := repo.MoveFile("old-name.txt", "new-name.txt")
```

### Branching and Merging

```go
// Create a new branch
err := repo.Branch("feature/new-feature")

// Switch branches
err := repo.Checkout("feature/new-feature")

// List branches
branches, err := repo.ListBranches()

// Merge branches
err := repo.Merge("feature/new-feature", &govc.MergeOptions{
    Strategy: govc.MergeStrategyRecursive,
    Message:  "Merge feature branch",
})

// Delete branch
err := repo.DeleteBranch("feature/old-feature")
```

### Working with Commits

```go
// Get commit history
commits, err := repo.Log(&govc.LogOptions{
    MaxCount: 100,
    Since:    time.Now().AddDate(0, -1, 0), // Last month
})

// Get specific commit
commit, err := repo.GetCommit("abc123...")

// Cherry-pick a commit
err := repo.CherryPick("abc123...")

// Revert a commit
err := repo.Revert("abc123...")
```

### Tags

```go
// Create a tag
err := repo.Tag("v1.0.0", &govc.TagOptions{
    Message: "Version 1.0.0",
    Commit:  "abc123...",
})

// List tags
tags, err := repo.ListTags()

// Delete tag
err := repo.DeleteTag("old-tag")
```

## Advanced Features

### Parallel Realities

Create and manage parallel versions of your repository:

```go
// Create a parallel reality
pr, err := repo.CreateParallelReality("experiment-1")

// Work in parallel reality
pr.WriteFile("experimental.go", []byte(code), 0644)
pr.Commit("Experimental changes", nil)

// Merge parallel reality back
err = repo.MergeParallelReality("experiment-1")

// List parallel realities
realities, err := repo.ListParallelRealities()
```

### Time Travel

Navigate through repository history:

```go
// Travel to specific commit
err := repo.TimeTravel("abc123...")

// Travel to specific date
err := repo.TimeTravelToDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

// Get current timeline position
position := repo.GetTimelinePosition()
```

### Stashing

Save and restore work in progress:

```go
// Stash changes
stashID, err := repo.Stash(&govc.StashOptions{
    Message: "WIP: implementing feature X",
})

// List stashes
stashes, err := repo.ListStashes()

// Apply stash
err := repo.StashApply(stashID)

// Drop stash
err := repo.StashDrop(stashID)
```

### Hooks

Register hooks for repository events:

```go
// Pre-commit hook
repo.RegisterHook(govc.HookPreCommit, func(ctx govc.HookContext) error {
    // Validate commit
    files := ctx.GetStagedFiles()
    for _, file := range files {
        if err := validateFile(file); err != nil {
            return fmt.Errorf("validation failed: %w", err)
        }
    }
    return nil
})

// Post-commit hook
repo.RegisterHook(govc.HookPostCommit, func(ctx govc.HookContext) error {
    commit := ctx.GetCommit()
    fmt.Printf("Created commit: %s\n", commit.Hash)
    return nil
})
```

### Search and Query

```go
// Search commits
results, err := repo.SearchCommits("bug fix", &govc.SearchOptions{
    Author: "john@example.com",
    Since:  time.Now().AddDate(0, -3, 0),
})

// Search file content
matches, err := repo.SearchContent("TODO", &govc.SearchOptions{
    FilePattern: "*.go",
    IgnoreCase:  true,
})

// Advanced grep
results, err := repo.Grep("func.*Error", &govc.GrepOptions{
    Regex:       true,
    LineNumbers: true,
})
```

## API Reference

### Repository Options

```go
type RepositoryOptions struct {
    // Storage path (empty for in-memory)
    Path string
    
    // Use in-memory storage
    InMemory bool
    
    // Enable compression
    Compressed bool
    
    // Enable encryption
    Encrypted bool
    EncryptionKey []byte
    
    // Performance options
    CacheSize int
    MaxParallelOps int
}
```

### Commit Options

```go
type CommitOptions struct {
    // Author information
    Author string
    Email  string
    
    // Committer (defaults to author)
    Committer string
    CommitterEmail string
    
    // GPG signing
    SigningKey string
    
    // Parent commits (for merge commits)
    Parents []string
    
    // Timestamp (defaults to now)
    Timestamp time.Time
}
```

### Merge Options

```go
type MergeOptions struct {
    // Merge strategy
    Strategy MergeStrategy
    
    // Commit message
    Message string
    
    // Fast-forward only
    FFOnly bool
    
    // No fast-forward
    NoFF bool
    
    // Squash commits
    Squash bool
}
```

## Best Practices

### 1. Use Transactions for Multiple Operations

```go
func updateMultipleFiles(repo govc.Repository, updates map[string][]byte) error {
    tx, err := repo.BeginTransaction()
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    for path, content := range updates {
        if err := tx.WriteFile(path, content, 0644); err != nil {
            return err
        }
    }
    
    return tx.Commit()
}
```

### 2. Handle Large Files Efficiently

```go
// Use streaming for large files
reader, err := repo.OpenFile("large-file.bin")
if err != nil {
    return err
}
defer reader.Close()

// Process in chunks
buffer := make([]byte, 1024*1024) // 1MB chunks
for {
    n, err := reader.Read(buffer)
    if err == io.EOF {
        break
    }
    if err != nil {
        return err
    }
    processChunk(buffer[:n])
}
```

### 3. Implement Proper Error Handling

```go
if err := repo.Checkout("main"); err != nil {
    switch {
    case errors.Is(err, govc.ErrBranchNotFound):
        // Handle missing branch
        return fmt.Errorf("branch 'main' does not exist")
    case errors.Is(err, govc.ErrConflict):
        // Handle merge conflicts
        conflicts := err.(*govc.ConflictError).Conflicts
        return handleConflicts(conflicts)
    default:
        return fmt.Errorf("checkout failed: %w", err)
    }
}
```

### 4. Use Connection Pooling

```go
// Create a repository pool for high-concurrency scenarios
pool := govc.NewRepositoryPool(govc.PoolOptions{
    MaxSize:     100,
    MaxIdleTime: 30 * time.Minute,
})

// Get repository from pool
repo, err := pool.Get("repo-id")
if err != nil {
    return err
}
defer pool.Put(repo) // Return to pool when done
```

## Troubleshooting

### Common Issues

1. **Out of Memory Errors**
   ```go
   // Use disk-based storage for large repositories
   repo, err := govc.NewRepositoryWithOptions(govc.RepositoryOptions{
       Path:     "/path/to/storage",
       InMemory: false,
   })
   ```

2. **Concurrent Access Issues**
   ```go
   // Use repository locking
   lock := repo.Lock()
   defer lock.Unlock()
   
   // Perform operations safely
   ```

3. **Performance Issues**
   ```go
   // Enable caching
   repo.EnableCache(govc.CacheOptions{
       Size: 1000, // Cache 1000 objects
       TTL:  5 * time.Minute,
   })
   ```

### Debug Logging

```go
// Enable debug logging
govc.SetLogLevel(govc.LogLevelDebug)

// Custom logger
govc.SetLogger(log.New(os.Stderr, "[govc] ", log.LstdFlags))
```

## Migration from Git

### Import Existing Repository

```go
// Import from Git
importer := govc.NewGitImporter()
repo, err := importer.Import("/path/to/git/repo")

// Import with progress tracking
repo, err := importer.ImportWithProgress("/path/to/git/repo", func(progress float64) {
    fmt.Printf("Import progress: %.2f%%\n", progress*100)
})
```

### Export to Git

```go
// Export to Git format
exporter := govc.NewGitExporter()
err := exporter.Export(repo, "/path/to/export")

// Export specific branch
err := exporter.ExportBranch(repo, "main", "/path/to/export")
```

## Performance Considerations

1. **Memory Usage**: In-memory repositories are 40x faster but consume more RAM
2. **Compression**: Reduces storage by 60-80% with minimal performance impact
3. **Caching**: Significantly improves read performance for frequently accessed objects
4. **Parallel Operations**: Use goroutines for independent operations

## Support

- GitHub Issues: https://github.com/caiatech/govc/issues
- Documentation: https://docs.caiatech.com/govc
- Examples: See the `examples/` directory