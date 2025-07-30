# Using govc as a Library

govc is designed as a library-first Git implementation. The CLI is just one way to use it.

## Installation

```bash
go get github.com/caiatech/govc
```

## Quick Start

```go
import "github.com/caiatech/govc"

// Create a memory-first repository
repo := govc.New()

// Start a transaction
tx := repo.Transaction()
tx.Add("config.yaml", []byte("version: 1.0"))
tx.Validate()
tx.Commit("Initial commit")
```

## Key Concepts

### 1. Memory-First by Default

```go
// This creates a repository entirely in memory
// No disk I/O unless you explicitly save
repo := govc.New()
```

### 2. Transactional Commits

```go
tx := repo.Transaction()
tx.Add("file1.txt", []byte("content"))
tx.Add("file2.txt", []byte("content"))

// Validate before committing
if err := tx.Validate(); err != nil {
    tx.Rollback() // Nothing was written
    return err
}

commit := tx.Commit("Safe commit")
```

### 3. Parallel Realities

```go
// Create isolated universes for testing
realities := repo.ParallelRealities([]string{
    "experiment-1",
    "experiment-2",
    "experiment-3",
})

// Each reality is completely isolated
for _, reality := range realities {
    reality.Apply(differentConfig)
    result := reality.Benchmark()
    // Test without affecting other realities
}
```

### 4. Event-Driven Infrastructure

```go
// React to repository changes
repo.Watch(func(event govc.Event) {
    if event.Author == "automation" {
        // Trigger deployment
    }
    if strings.Contains(event.Message, "EMERGENCY") {
        // Rollback immediately
    }
})
```

## Common Patterns

### Infrastructure Configuration Testing

```go
func testConfigurations(repo *govc.Repository) {
    configs := []Config{
        {Name: "high-memory", Memory: "16GB", CPU: 4},
        {Name: "high-cpu", Memory: "8GB", CPU: 8},
        {Name: "balanced", Memory: "12GB", CPU: 6},
    }

    // Test all in parallel
    realities := repo.ParallelRealities(getNames(configs))
    
    results := make([]Result, len(configs))
    for i, reality := range realities {
        reality.Apply(configs[i].ToBytes())
        results[i] = runBenchmark(reality)
    }

    // Apply best configuration
    best := findBest(results)
    repo.Merge(best.Reality, "main")
}
```

### A/B Testing

```go
func abTest(repo *govc.Repository) {
    // Create two realities
    a := repo.ParallelReality("config-a")
    b := repo.ParallelReality("config-b")

    // Apply different configurations
    a.Apply(configA)
    b.Apply(configB)

    // Route traffic 50/50
    router.Route("50%", a)
    router.Route("50%", b)

    // Monitor and pick winner
    time.Sleep(1 * time.Hour)
    
    if metrics.A > metrics.B {
        repo.Merge(a.Name(), "main")
    } else {
        repo.Merge(b.Name(), "main")
    }
}
```

### Time Travel Debugging

```go
func debugIncident(repo *govc.Repository, incidentTime time.Time) {
    // Go back to the incident time
    snapshot := repo.TimeTravel(incidentTime)
    
    // Examine state at that moment
    config, _ := snapshot.Read("app.conf")
    fmt.Printf("Config was: %s\n", config)
    
    // Find who made the last change
    commit := snapshot.LastCommit()
    fmt.Printf("Last change by: %s\n", commit.Author.Name)
}
```

## API Reference

### Repository Creation

- `govc.New()` - Create memory-first repository
- `govc.Init(path)` - Initialize with file persistence
- `govc.Open(path)` - Open existing repository
- `govc.NewWithConfig(config)` - Create with custom config

### Core Operations

- `repo.Transaction()` - Start transactional commit
- `repo.Add(pattern)` - Stage files
- `repo.Commit(message)` - Create commit
- `repo.Branch(name)` - Create branch builder
- `repo.Checkout(ref)` - Switch branches
- `repo.Merge(from, to)` - Merge branches

### Memory-First Features

- `repo.ParallelReality(name)` - Create isolated universe
- `repo.ParallelRealities(names)` - Create multiple universes
- `repo.Watch(handler)` - Subscribe to events
- `repo.TimeTravel(time)` - Get historical snapshot

### Reality Operations

- `reality.Apply(changes)` - Apply changes to reality
- `reality.Benchmark()` - Run performance tests
- `reality.Evaluate()` - Get reality metrics

## Best Practices

1. **Use Memory-First for Testing**: Create repositories with `govc.New()` for testing and experimentation.

2. **Leverage Parallel Realities**: Test multiple configurations simultaneously instead of sequentially.

3. **Validate Before Committing**: Always use transactions and validate before committing.

4. **React to Events**: Use `Watch()` to build reactive systems that respond to changes.

5. **Time Travel for Debugging**: Use `TimeTravel()` to examine past states when debugging issues.

## Performance Tips

- Memory operations are ~1000x faster than disk
- Create thousands of branches instantly
- Run parallel tests without setup/teardown
- No cleanup needed - just discard realities

## See Also

- [Full API Documentation](https://pkg.go.dev/github.com/caiatech/govc)
- [Infrastructure Examples](../infrastructure/)
- [Parallel Testing Guide](../parallel-testing/)