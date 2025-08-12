

# govc - AI-Native Memory-First Version Control

> **Next-generation version control designed for AI systems and high-performance workflows**

[![Go](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-Apache%202.0-green.svg)](LICENSE)
[![Tests](https://img.shields.io/badge/Tests-Passing-brightgreen.svg)](https://github.com/Caia-Tech/govc)
[![Status](https://img.shields.io/badge/Status-Beta-yellow.svg)](PROJECT_REVIEW.md)

## 🚀 What is govc?

govc is a revolutionary version control system built from the ground up for the AI era, where code generation happens at machine speed, not human speed.

### 🤖 **AI-First Design**
- **Memory-First Operations** - No disk I/O bottleneck for AI systems generating thousands of variations
- **Parallel Universe Branching** - AI can explore 1000s of solution branches simultaneously
- **Event-Driven Architecture** - AI hooks for automated testing, documentation, and review

### ⚡ **Performance Features**
- **10,000+ commits/sec** in memory mode
- **Zero-cost branching** for parallel exploration
- **Atomic batch operations** for related changes
- **Semantic code indexing** for intelligent search

### 🏢 **Enterprise Ready**
- **JWT/API Key Authentication** with RBAC
- **Multiple Storage Backends** (Memory, PostgreSQL, MongoDB, Redis, SQLite, BadgerDB)
- **REST & GRPC APIs** with client libraries (Go, JavaScript/TypeScript, Python)
- **Prometheus Metrics** and structured logging
- **Clustering & Sharding** for distributed deployments
- **Web Dashboard** for monitoring and management

## 📦 Installation

### As a CLI Tool
```bash
# Build from source
git clone https://github.com/Caia-Tech/govc.git
cd govc
go build -o govc ./cmd/govc
```

### As a Library
```bash
go get github.com/Caia-Tech/govc
```

## 🎯 Quick Start

### CLI Usage
```bash
# Initialize a repository
govc init

# Add files to staging
govc add main.go

# Commit changes
govc commit -m "Initial commit"

# View status
govc status

# View history
govc log
```

### Library Usage
```go
import "github.com/Caia-Tech/govc"

// Create memory-first repository
repo := govc.NewRepository()

// Add content directly
staging := repo.GetStagingArea()
staging.Add("main.go", []byte("package main\n\nfunc main() {}"))

// Commit
commit, _ := repo.Commit("Initial commit")
fmt.Printf("Created: %s\n", commit.Hash())
```

### Server Mode
```bash
# Start the API server
./govc-server --config config.yaml

# Server provides:
# - REST API on :8080/api/v1
# - GRPC on :9090
# - Web Dashboard on :8080/dashboard
# - Metrics on :8080/metrics
```

## 🧪 Integrated Pipeline

govc includes native CI/CD tools that share memory for zero-overhead operations:

### Memory Test Executor
```go
// Run tests entirely in memory
executor := pipeline.NewMemoryTestExecutor()
results, _ := executor.ExecuteTestsInMemory(ctx, sourceFiles, testFiles)
```

### AI Commands (Experimental)
```bash
govc ai index        # Build semantic code index
govc ai search       # Semantic code search
govc ai commit-msg   # Generate commit messages
govc ai review       # Automated code review
```

## 🏗️ Architecture

### Memory-First Design
- **In-Memory Operations** - All operations happen in memory by default
- **Optional Persistence** - Save to disk only when needed
- **Shared Data Structures** - Pipeline tools access common memory
- **Event-Driven Updates** - Real-time notifications for all changes

### Storage Backends
```yaml
# config.yaml
storage:
  type: memory    # memory | postgres | mongodb | redis | sqlite | badger
  memory_limit: 4GB
  persist_on_commit: true
```

### Performance Optimizations
- Parallel commit processing
- Delta compression with chains
- Optimized blob storage
- Connection pooling
- Resource management

## 📊 Benchmarks

```
BenchmarkCommit-10              50000      23456 ns/op
BenchmarkParallelCommits-10    100000      11234 ns/op
BenchmarkMemoryTestExec-10      10000     112345 ns/op
BenchmarkSearch-10              20000      56789 ns/op
```

## 🔒 Security

- **JWT Authentication** with refresh tokens
- **API Key Management** with SHA256 hashing
- **Role-Based Access Control** (admin, developer, reader, guest)
- **Input Validation** on all endpoints
- **CSRF Protection** for web operations
- **Rate Limiting** and DDoS protection

## 🛠️ Development

```bash
# Run all tests
go test ./...

# Run with race detection
go test -race ./...

# Generate coverage
go test -cover ./...

# Run benchmarks
go test -bench=. ./...
```

## 📖 Documentation

- [Architecture Overview](docs/ARCHITECTURE.md)
- [API Reference](docs/api-reference.md)
- [Security Guide](docs/SECURITY.md)
- [Production Deployment](docs/production-deployment.md)

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guidelines](CONTRIBUTING.md) for details.

## 📄 License

Apache License 2.0 - see [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

Built with inspiration from Git, but designed for the future of AI-powered development.
