# govc - Memory-First Git: A Reality Engine for Infrastructure

**govc isn't just Git written in Go. It's Git reimagined as a memory-first reality engine for infrastructure that can exist in multiple states simultaneously.**

## Why govc? The Paradigm Shift

Traditional Git is filesystem-bound. Every operation touches disk. This made sense in 2005 when Git was designed for Linux kernel development. But what if we rethink version control for modern infrastructure?

**The key insight**: When Git operations happen in memory instead of on disk, the performance characteristics completely change what's possible. Branches become lightweight parallel realities. Commits become transactional state changes. History becomes a traversable timeline of system states.

## What This Enables

### 1. Instant Parallel Realities
Creating 1000 branches takes milliseconds, not seconds. Each branch is an isolated reality where you can test changes without affecting others.

```go
// Test multiple configurations in parallel
results := make(chan TestResult)
for _, config := range configs {
    go func(c Config) {
        branch := repo.ParallelReality(c.Name)
        branch.Apply(c)
        results <- branch.Evaluate()
    }(config)
}
```

### 2. Transactional Infrastructure Changes
Apply changes, validate them, and only commit if everything checks out. True rollback before anything hits persistent state.

```go
tx := repo.BeginTransaction()
tx.Add("nginx.conf", newConfig)
tx.Add("ssl/cert.pem", newCert)

if err := tx.Validate(); err != nil {
    return err // Nothing was written
}

commit := tx.Commit("Update nginx with new SSL cert")
```

### 3. Git as Event Stream
React to infrastructure changes in real-time. Every commit becomes an event you can process, transform, or route.

```go
repo.Watch(func(event CommitEvent) {
    if event.Changes("*.tf") {
        terraform.Plan(event.Branch())
    }
    if event.Author() == "automation" {
        audit.Log(event)
    }
})
```

### 4. Parallel Universe Testing
Each test runs in its own isolated branch. Tests can't interfere with each other, even when running simultaneously.

```go
suite.ParallelTest(func(t *Test, branch *IsolatedBranch) {
    // This test has its own reality
    branch.Apply(TestConfig{
        Database: "test-db-" + t.Name,
        Cache: "isolated",
    })
    // Changes exist only in this branch
})
```

## Installation

```bash
go install github.com/caia-tech/govc/cmd/govc@latest
```

Or use as a library:
```go
import "github.com/caia-tech/govc"
```

## Quick Start

### CLI: Infrastructure as Versioned State
```bash
# Initialize a reality engine for your infrastructure
govc init

# Create parallel realities for testing
govc branch staging
govc branch canary

# Apply changes transactionally
govc checkout staging
govc add configs/
govc commit -m "Test new load balancer config"

# Instant rollback by switching realities
govc checkout main  # Previous state restored instantly
```

### Library: Memory-First API
```go
// Initialize an in-memory repository
repo := govc.NewRepository()

// Create isolated branches for parallel testing
futures := repo.ParallelRealities([]string{
    "optimize-cpu",
    "optimize-memory", 
    "optimize-network",
})

// Test configurations in parallel universes
for _, future := range futures {
    go func(branch *ParallelReality) {
        branch.Apply(optimization)
        score := branch.Benchmark()
        if score.Better() {
            repo.Merge(branch, repo.Main())
        }
    }(future)
}
```

## Architecture: Memory-First by Design

```
┌─────────────────────────────────────────────────┐
│                 Application                      │
├─────────────────────────────────────────────────┤
│              govc Memory Layer                   │
│  ┌─────────────┐ ┌──────────────┐ ┌──────────┐ │
│  │  Parallel   │ │ Transactional│ │  Event   │ │
│  │ Realities   │ │   Commits    │ │  Stream  │ │
│  └─────────────┘ └──────────────┘ └──────────┘ │
├─────────────────────────────────────────────────┤
│           In-Memory Object Store                 │
│  ┌────────┐ ┌────────┐ ┌────────┐ ┌─────────┐  │
│  │ Blobs  │ │ Trees  │ │Commits │ │  Refs   │  │
│  └────────┘ └────────┘ └────────┘ └─────────┘  │
├─────────────────────────────────────────────────┤
│        Optional Persistence Layer                │
└─────────────────────────────────────────────────┘
```

Key design principles:
- **Memory-first**: All operations happen in memory
- **Persistence-optional**: Disk is just for durability, not operation
- **Branch-as-reality**: Each branch is an isolated universe
- **Commit-as-event**: Every commit triggers reactive flows
- **Parallel-by-default**: Operations designed for concurrency

## Real-World Use Cases

### Configuration Management Without Fear
```go
// Test infrastructure changes in isolation
test := repo.IsolatedBranch("test-redis-upgrade")
test.Apply(RedisConfig{Version: "7.0"})

if err := test.Validate(); err == nil {
    // Merge only if validation passes
    repo.Merge(test, repo.Main())
}
```

### A/B Testing Infrastructure
```go
// Run two configurations simultaneously
a := repo.ParallelReality("config-a")
b := repo.ParallelReality("config-b")

a.Apply(ConfigA{})
b.Apply(ConfigB{})

// Route traffic and measure
metrics := RouteTraffic(a: 50, b: 50)

// Winner becomes the new reality
if metrics.A.Better() {
    repo.SetMain(a)
}
```

### Time-Travel Debugging
```go
// Replay system state at incident time
incident := time.Parse("2024-01-15 03:42:00")
snapshot := repo.TimeTravel(incident)

// Examine state at that moment
fmt.Printf("Config was: %v\n", snapshot.Read("app.conf"))
fmt.Printf("Deployed by: %v\n", snapshot.LastCommit().Author)
```

## Future Vision: Distributed Reality Sync

We're building toward a future where multiple govc nodes can share realities:

```go
// Node A: Create a reality
reality := nodeA.ParallelReality("experiment")
reality.Apply(changes)

// Node B: Subscribe to that reality
nodeB.Subscribe(nodeA, "experiment")

// Both nodes now share the same reality
// Changes sync in real-time
```

This enables:
- Distributed configuration testing
- Multi-region infrastructure rollouts
- Consensus-based infrastructure changes
- Reality merging across data centers

## Examples

See the `examples/` directory for practical demonstrations:
- `examples/infrastructure/` - Config management patterns
- `examples/parallel-testing/` - Test isolation strategies  
- `examples/event-stream/` - Reactive infrastructure
- `examples/distributed/` - Multi-node synchronization

## Contributing

govc is more than a project - it's a new way of thinking about infrastructure state. We welcome contributions that push this vision forward.

## License

Apache License 2.0 - see LICENSE file

---

*govc is a [Caia Tech](https://caia.tech) project, reimagining version control for the infrastructure age.*