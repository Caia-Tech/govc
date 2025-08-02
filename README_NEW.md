# govc - Memory-First Git Infrastructure

> **⚠️ ACTIVE DEVELOPMENT WARNING**: This project is under heavy development and is subject to significant changes. APIs, features, and behaviors may change without notice. See [DEVELOPMENT_STATUS.md](DEVELOPMENT_STATUS.md) for current status.

**A complete Git implementation in Go with enterprise production infrastructure, designed for high-performance, memory-first operations and scalable Git server deployments.**

[![Go](https://img.shields.io/badge/Go-1.20+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-Apache%202.0-green.svg)](LICENSE)
[![Tests](https://img.shields.io/badge/Tests-Passing-brightgreen.svg)](https://github.com/caiatech/govc)
[![Status](https://img.shields.io/badge/Status-Alpha-orange.svg)](DEVELOPMENT_STATUS.md)
[![Coverage](https://img.shields.io/badge/Coverage-52%25-yellow.svg)](TESTING_GUIDE.md)

## 🚧 Development Notice

**govc is currently in ALPHA stage**. We are actively developing and testing this system with multiple layers of verification:

- **Continuous Testing**: Every feature undergoes unit, integration, and system testing
- **Rapid Iteration**: We release updates frequently as we refine the architecture
- **Community Feedback**: Your input shapes the direction of development
- **Production Path**: We're building towards production readiness with careful consideration

**Please read [DEVELOPMENT_STATUS.md](DEVELOPMENT_STATUS.md) before using govc in any capacity.**

## 🏗️ Architecture Update

We've recently completed a major architectural refactoring to improve modularity and testability:

- **Clean Architecture**: Separated concerns into distinct, testable components
- **Interface-Based Design**: Easy to mock and test
- **Pluggable Storage**: Mix and match storage backends
- **Backward Compatible**: Migration path from old to new architecture

See [MIGRATION_GUIDE.md](MIGRATION_GUIDE.md) for details on migrating to the new architecture.

## 🚀 What is govc?

govc is a complete Git implementation written in pure Go, featuring:

- **🏎️ Memory-First Architecture** - Operations happen in memory with optional persistence
- **🧩 Clean Architecture** - Modular design with separated concerns
- **🔐 Enterprise Security** - JWT/API key authentication with role-based access control  
- **📊 Production Monitoring** - Prometheus metrics and structured logging
- **🏊 Resource Management** - Connection pooling and efficient resource handling
- **⚡ High Performance** - Optimized for speed with comprehensive benchmarking
- **🔧 REST API** - Complete HTTP API for Git operations
- **🧪 Parallel Testing** - Isolated branch realities for concurrent testing

## ⚡ Quick Start

### Library Usage (New Clean Architecture)

```go
package main

import (
    "fmt"
    "github.com/caiatech/govc"
)

func main() {
    // Quick start with everything pre-configured
    qs := govc.NewQuickStart()
    
    // Configure user
    qs.Config.Set("user.name", "John Doe")
    qs.Config.Set("user.email", "john@example.com")
    
    // Create and commit a file
    err := qs.Workspace.WriteFile("README.md", []byte("# My Project"))
    if err != nil {
        panic(err)
    }
    
    err = qs.Operations.Add("README.md")
    if err != nil {
        panic(err)
    }
    
    hash, err := qs.Operations.Commit("Initial commit")
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Created commit: %s\n", hash)
}
```

### API Server

```bash
# Start the server
govc-server --config config.yaml

# Use the API
curl -X POST http://localhost:8080/api/v1/repos \
  -H "Content-Type: application/json" \
  -d '{"id": "my-repo", "memory_only": true}'
```

## 📚 Documentation

### Core Documentation
- [README.md](README.md) - This file
- [DEVELOPMENT_STATUS.md](DEVELOPMENT_STATUS.md) - Current development status and warnings
- [MIGRATION_GUIDE.md](MIGRATION_GUIDE.md) - Guide for migrating to clean architecture
- [CHANGELOG.md](CHANGELOG.md) - Version history and changes

### Development
- [CONTRIBUTING.md](CONTRIBUTING.md) - How to contribute
- [TESTING_GUIDE.md](TESTING_GUIDE.md) - Testing strategy and guidelines
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) - System architecture

### API & Integration
- [API_REFERENCE.md](API_REFERENCE.md) - Complete API documentation
- [INTEGRATION.md](INTEGRATION.md) - Integration patterns and examples
- [docs/production-deployment.md](docs/production-deployment.md) - Production deployment guide

### Examples
- [examples/clean_architecture/](examples/clean_architecture/) - Clean architecture examples
- [examples/basic/](examples/basic/) - Basic usage examples
- [examples/advanced/](examples/advanced/) - Advanced features

## 🏗️ Architecture

govc uses a clean architecture with separated concerns:

```
┌─────────────────────────────────────────────────┐
│                   API Layer                      │
│        (HTTP handlers, authentication)           │
└─────────────────────────────────────────────────┘
                        │
┌─────────────────────────────────────────────────┐
│               Operations Layer                   │
│     (High-level Git operations, workflows)       │
└─────────────────────────────────────────────────┘
                        │
┌─────────────────────────────────────────────────┐
│                 Core Layer                       │
│  ┌─────────────┐ ┌──────────────┐ ┌──────────┐ │
│  │ Repository  │ │  Workspace   │ │  Config  │ │
│  │ (Immutable) │ │  (Mutable)   │ │          │ │
│  └─────────────┘ └──────────────┘ └──────────┘ │
│  ┌─────────────┐ ┌──────────────┐              │
│  │StashManager │ │WebhookManager│              │
│  └─────────────┘ └──────────────┘              │
└─────────────────────────────────────────────────┘
                        │
┌─────────────────────────────────────────────────┐
│               Storage Layer                      │
│  ┌────────────┐ ┌────────┐ ┌────────────────┐  │
│  │ObjectStore │ │RefStore│ │WorkingStorage  │  │
│  └────────────┘ └────────┘ └────────────────┘  │
└─────────────────────────────────────────────────┘
```

## 🔐 Security Features

- **JWT Authentication** - Secure token-based authentication
- **API Key Management** - Long-lived API keys for automation
- **Role-Based Access Control** - Fine-grained permissions
- **Webhook Security** - HMAC signatures for webhook payloads
- **Repository Isolation** - Complete isolation between repositories

## 📊 Monitoring & Metrics

- **Prometheus Metrics** - Built-in metrics endpoint
- **Structured Logging** - JSON logging with levels
- **Health Checks** - Liveness and readiness probes
- **Performance Tracking** - Operation timing and resource usage

## 🚀 Performance

- **Memory-First Operations** - All operations in memory by default
- **Lazy Loading** - Objects loaded only when needed
- **Connection Pooling** - Efficient resource management
- **Parallel Operations** - Concurrent branch operations
- **Optimized Storage** - Efficient object storage format

## 📦 Installation

### Go Library

```bash
go get github.com/caiatech/govc
```

### Server Binary

```bash
go install github.com/caiatech/govc/cmd/govc-server@latest
```

### Docker

```bash
docker run -p 8080:8080 caiatech/govc-server:latest
```

## ⚙️ Configuration

Create a `config.yaml`:

```yaml
server:
  port: 8080
  max_request_size: 10485760  # 10MB
  request_timeout: 30s

auth:
  enabled: true
  jwt:
    secret: your-secret-key
    issuer: govc
    ttl: 24h

metrics:
  enabled: true
  
pool:
  max_repositories: 1000
  max_idle_time: 30m
  cleanup_interval: 5m
```

## 🧪 Testing

```bash
# Run all tests
./run_tests.sh

# Run specific test suite
go test ./pkg/core -v

# Run benchmarks
go test -bench=. ./benchmarks
```

## 🤝 Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Key areas for contribution:
- Storage backend implementations
- Performance optimizations
- Additional Git features
- Documentation improvements
- Test coverage

## 📄 License

govc is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

## 🙏 Acknowledgments

- The Git project for the inspiration and specification
- The Go community for excellent tooling
- All contributors who help improve govc

---

**Note**: This is an active development project. Features and APIs are subject to change. Always refer to the latest documentation.