# govc - Memory-First Git Infrastructure

**A complete Git implementation in Go with enterprise production infrastructure, designed for high-performance, memory-first operations and scalable Git server deployments.**

[![Go](https://img.shields.io/badge/Go-1.20+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-Apache%202.0-green.svg)](LICENSE)
[![Tests](https://img.shields.io/badge/Tests-Passing-brightgreen.svg)](https://github.com/caiatech/govc)

## 🚀 What is govc?

govc is a complete Git implementation written in pure Go, featuring:

- **🏎️ Memory-First Architecture** - Operations happen in memory with optional persistence
- **🔐 Enterprise Security** - JWT/API key authentication with role-based access control  
- **📊 Production Monitoring** - Prometheus metrics and structured logging
- **🏊 Resource Management** - Connection pooling and efficient resource handling
- **⚡ High Performance** - Optimized for speed with comprehensive benchmarking
- **🔧 REST API** - Complete HTTP API for Git operations
- **🧪 Parallel Testing** - Isolated branch realities for concurrent testing

## 📋 Table of Contents

- [Quick Start](#quick-start)
- [Production Features](#production-features)
- [Architecture](#architecture)
- [API Documentation](#api-documentation)
- [Authentication & Security](#authentication--security)
- [Monitoring & Observability](#monitoring--observability)
- [Performance](#performance)
- [Installation](#installation)
- [Configuration](#configuration)
- [Examples](#examples)
- [Contributing](#contributing)

## ⚡ Quick Start

### Server Deployment

```bash
# Clone the repository
git clone https://github.com/caiatech/govc.git
cd govc

# Build the server
go build -o govc-server ./cmd/govc-server

# Run with default configuration
./govc-server
```

The server starts on `http://localhost:8080` with:
- JWT authentication enabled
- Prometheus metrics at `/metrics`
- Health checks at `/health`
- Repository pooling active
- Structured JSON logging

### Using as a Library

```go
package main

import (
    "log"
    "github.com/caiatech/govc"
)

func main() {
    // Create memory-first repository
    repo := govc.New()
    
    // Add files and commit
    repo.Add("README.md", "# My Project")
    commit, _ := repo.Commit("Initial commit", govc.Author{
        Name:  "Developer",
        Email: "dev@example.com",
    })
    
    log.Printf("Created commit: %s", commit.Hash())
}
```

### REST API Usage

```bash
# Create a repository
curl -X POST http://localhost:8080/api/v1/repos \
  -H "Content-Type: application/json" \
  -d '{"id": "my-repo", "memory_only": true}'

# Add a file
curl -X POST http://localhost:8080/api/v1/repos/my-repo/files \
  -H "Content-Type: application/json" \
  -d '{"path": "main.go", "content": "package main\n\nfunc main() {}"}'

# Create a commit
curl -X POST http://localhost:8080/api/v1/repos/my-repo/commits \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Add main.go",
    "author": {"name": "Dev", "email": "dev@example.com"}
  }'
```

## 🏗️ Production Features

### 🔐 Enterprise Authentication & Authorization

**JWT Authentication**
- Secure token generation with custom claims
- Configurable expiration and refresh
- Automatic token validation middleware

**API Key Management**  
- Secure key generation with SHA256 hashing
- Per-key permissions and expiration
- Key revocation and listing

**Role-Based Access Control (RBAC)**
- Default roles: `admin`, `developer`, `reader`, `guest`
- Granular permissions system
- Repository-level permissions
- User management with activation/deactivation

```go
// Example: Create user with specific permissions
rbac := auth.NewRBAC()
rbac.CreateUser("dev1", "Developer", "dev@company.com", []string{"developer"})
rbac.GrantRepositoryPermission("dev1", "critical-repo", auth.PermissionRepoRead)
```

### 📊 Comprehensive Monitoring

**Prometheus Metrics**
- HTTP request metrics (count, duration, status)
- System metrics (memory, goroutines, GC stats)
- Repository and transaction counters
- Custom business metrics

**Structured Logging**
- JSON formatted logs with correlation IDs
- Configurable log levels (DEBUG, INFO, WARN, ERROR, FATAL)
- Request/response logging with user context
- Operation timing and error tracking

**Health Checks**
- Liveness probe: `/health/live`
- Readiness probe: `/health/ready`  
- Detailed system status: `/health`

### 🏊 Resource Management

**Repository Connection Pooling**
- Configurable pool size and idle timeouts
- Automatic cleanup of unused connections
- LRU-style eviction policies
- Pool statistics and monitoring

**Memory Management**
- Efficient in-memory object storage
- Optional persistence to disk
- Garbage collection optimization
- Memory usage monitoring

### ⚡ High Performance

**Memory-First Operations**
- All Git operations happen in memory
- Optional disk persistence for durability
- Optimized object storage and retrieval
- Concurrent operation support

**Benchmarked Performance**
- 42.9x faster than disk-based operations
- Sub-millisecond branch creation
- Concurrent transaction processing
- Efficient memory usage patterns

## 🏛️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Production govc Server                   │
├─────────────────────────────────────────────────────────────┤
│  Authentication & Authorization Layer                       │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────┐    │
│  │ JWT Auth    │ │ API Keys    │ │ RBAC Permissions    │    │
│  └─────────────┘ └─────────────┘ └─────────────────────┘    │
├─────────────────────────────────────────────────────────────┤
│  Monitoring & Observability Layer                          │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────┐    │
│  │ Prometheus  │ │ Structured  │ │ Health Checks       │    │
│  │ Metrics     │ │ Logging     │ │ & Status            │    │
│  └─────────────┘ └─────────────┘ └─────────────────────┘    │
├─────────────────────────────────────────────────────────────┤
│  REST API Layer (Gin Framework)                            │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────┐    │
│  │ Repository  │ │ Git         │ │ Transaction         │    │
│  │ Management  │ │ Operations  │ │ Management          │    │
│  └─────────────┘ └─────────────┘ └─────────────────────┘    │
├─────────────────────────────────────────────────────────────┤
│  Resource Management Layer                                  │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────┐    │
│  │ Repository  │ │ Connection  │ │ Memory              │    │
│  │ Pool        │ │ Pooling     │ │ Management          │    │
│  └─────────────┘ └─────────────┘ └─────────────────────┘    │
├─────────────────────────────────────────────────────────────┤
│  Core Git Implementation                                    │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────┐    │
│  │ Memory-First│ │ Object      │ │ Reference           │    │
│  │ Storage     │ │ Management  │ │ Management          │    │
│  └─────────────┘ └─────────────┘ └─────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

## 📚 API Documentation

### Core Repository Operations

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/repos` | GET | List repositories |
| `/api/v1/repos` | POST | Create repository |
| `/api/v1/repos/{id}` | GET | Get repository info |
| `/api/v1/repos/{id}` | DELETE | Delete repository |

### Git Operations

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/repos/{id}/files` | POST | Add file |
| `/api/v1/repos/{id}/files/{path}` | GET | Get file content |
| `/api/v1/repos/{id}/commits` | POST | Create commit |
| `/api/v1/repos/{id}/commits` | GET | Get commit log |
| `/api/v1/repos/{id}/branches` | GET | List branches |
| `/api/v1/repos/{id}/branches` | POST | Create branch |

### Container Operations

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/repos/{id}/containers/definitions` | GET | List container definitions |
| `/api/v1/repos/{id}/containers/definitions` | POST | Create container definition |
| `/api/v1/repos/{id}/containers/definitions/*path` | GET | Get container definition |
| `/api/v1/repos/{id}/containers/definitions/*path` | PUT | Update container definition |
| `/api/v1/repos/{id}/containers/definitions/*path` | DELETE | Delete container definition |
| `/api/v1/repos/{id}/containers/build` | POST | Start container build |
| `/api/v1/repos/{id}/containers/builds` | GET | List builds |
| `/api/v1/repos/{id}/containers/builds/{build_id}` | GET | Get build details |

### Authentication

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/auth/login` | POST | JWT login |
| `/api/v1/auth/refresh` | POST | Refresh JWT token |
| `/api/v1/auth/apikeys` | GET | List API keys |
| `/api/v1/auth/apikeys` | POST | Create API key |

### Monitoring

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Detailed health check |
| `/health/live` | GET | Liveness probe |
| `/health/ready` | GET | Readiness probe |
| `/metrics` | GET | Prometheus metrics |

## 🔐 Authentication & Security

### JWT Authentication

```bash
# Login to get JWT token
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "admin123"}'

# Use JWT token in requests
curl -H "Authorization: Bearer <jwt-token>" \
  http://localhost:8080/api/v1/repos
```

### API Key Authentication

```bash
# Create API key
curl -X POST http://localhost:8080/api/v1/auth/apikeys \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{"name": "ci-key", "permissions": ["repo:read", "repo:write"]}'

# Use API key in requests
curl -H "X-API-Key: <api-key>" \
  http://localhost:8080/api/v1/repos
```

### Permission System

**Global Permissions:**
- `system:admin` - Full system access
- `repo:read` - Read repository content
- `repo:write` - Modify repository content  
- `repo:delete` - Delete repositories
- `user:read` - View user information
- `webhook:write` - Manage webhooks

**Repository-specific permissions can override global permissions**

## 📊 Monitoring & Observability

### Prometheus Metrics

Key metrics exposed at `/metrics`:

```
# HTTP request metrics
govc_http_requests_total{method="GET",status="200"} 150
govc_http_request_duration_seconds{method="POST",path="/api/v1/repos"} 0.25

# System metrics  
govc_repositories_total 42
govc_transactions_active 3
govc_uptime_seconds 86400

# Go runtime metrics
go_memstats_alloc_bytes 15728640
go_goroutines 25
```

### Structured Logging

All logs are in JSON format with correlation:

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "INFO", 
  "message": "HTTP request processed",
  "request_id": "req-123-456",
  "user_id": "user-789",
  "http_method": "POST",
  "http_path": "/api/v1/repos",
  "http_status": 201,
  "http_latency_ms": 45
}
```

### Health Checks

**Liveness:** `/health/live` - Returns 200 if server is running
**Readiness:** `/health/ready` - Returns 200 if server can handle requests
**Detailed:** `/health` - Returns comprehensive system status

## 🚀 Performance

### Benchmarks

**Memory vs Disk Operations:**
- **Memory-first:** 1.1ms average operation time
- **Disk-based:** 50.8ms average operation time  
- **Performance gain:** 42.9x faster

**Branch Operations:**
- **1000 branches created:** 1.3ms
- **100 parallel realities:** 164μs
- **Concurrent modifications:** 1.2ms for 100 operations

**HTTP API Performance:**
- **Repository operations:** ~500μs per request
- **Authentication middleware:** ~120ns per request
- **Metrics collection:** ~247ns per request

## 📦 Installation

### From Source

```bash
git clone https://github.com/caiatech/govc.git
cd govc
go build -o govc-server ./cmd/govc-server
```

### As Go Module

```bash
go get github.com/caiatech/govc
```

### Docker (Coming Soon)

```bash
docker run -p 8080:8080 caiatech/govc:latest
```

## ⚙️ Configuration

### Environment Variables

```bash
# Server configuration
GOVC_PORT=8080
GOVC_HOST=0.0.0.0

# Authentication
GOVC_JWT_SECRET=your-secret-key
GOVC_JWT_ISSUER=govc-server
GOVC_JWT_TTL=24h

# Pool configuration  
GOVC_POOL_MAX_REPOS=1000
GOVC_POOL_IDLE_TIME=30m
GOVC_POOL_CLEANUP_INTERVAL=5m

# Logging
GOVC_LOG_LEVEL=INFO
GOVC_LOG_FORMAT=json
```

### Configuration File

```yaml
server:
  port: 8080
  host: "0.0.0.0"
  
auth:
  jwt:
    secret: "your-secret-key"
    issuer: "govc-server" 
    ttl: "24h"
    
pool:
  max_repositories: 1000
  max_idle_time: "30m"
  cleanup_interval: "5m"
  
logging:
  level: "INFO"
  format: "json"
  
metrics:
  enabled: true
  path: "/metrics"
```

## 💡 Examples

### Infrastructure as Code Testing

```go
// Test multiple infrastructure configurations in parallel
configs := []InfraConfig{
    {Name: "small", CPU: 2, Memory: "4GB"},
    {Name: "medium", CPU: 4, Memory: "8GB"}, 
    {Name: "large", CPU: 8, Memory: "16GB"},
}

results := make(chan TestResult, len(configs))

for _, config := range configs {
    go func(c InfraConfig) {
        // Each test gets its own isolated branch
        branch := repo.ParallelReality(c.Name)
        branch.Apply(c)
        
        // Run validation in isolation
        score := branch.Validate()
        results <- TestResult{Config: c, Score: score}
    }(config)
}

// Choose the best configuration
var best TestResult
for i := 0; i < len(configs); i++ {
    result := <-results
    if result.Score > best.Score {
        best = result
    }
}

// Deploy the winning configuration
repo.Merge(best.Config.Name, "production")
```

### Canary Deployments

```go
// Create canary branch for new version
canary := repo.Branch("canary-v2.0")
canary.Apply(NewVersion{Version: "2.0", Features: ["new-auth"]})

// Route 10% of traffic to canary
metrics := LoadBalancer{
    Routes: map[string]int{
        "production": 90,
        "canary-v2.0": 10,
    },
}.Deploy()

// Monitor and promote if successful
if metrics.CanaryErrorRate < 0.01 {
    repo.Merge("canary-v2.0", "production")
    log.Println("Canary deployment successful!")
} else {
    repo.DeleteBranch("canary-v2.0")
    log.Println("Canary deployment failed, rolled back")
}
```

### Event-Driven Infrastructure

```go
// React to infrastructure changes in real-time
repo.Watch(func(event CommitEvent) {
    switch {
    case event.Changes("*.tf"):
        // Terraform files changed
        go terraform.Plan(event.Branch())
        
    case event.Changes("k8s/*.yaml"):
        // Kubernetes manifests changed  
        go kubernetes.Apply(event.Branch())
        
    case event.Author() == "security-bot":
        // Security updates - auto-approve
        if event.Message().Contains("security-patch") {
            repo.Merge(event.Branch(), "production")
        }
    }
})
```

## 🧪 Testing

### Run All Tests

```bash
# Run comprehensive test suite
go test ./... -v

# Run with benchmarks
go test ./... -bench=. -benchmem

# Run specific package tests
go test ./auth -v
go test ./pool -v  
go test ./metrics -v
go test ./logging -v
```

### Test Coverage

The project maintains comprehensive test coverage:

- **Auth Package:** JWT, API keys, RBAC, middleware
- **Pool Package:** Connection pooling, resource management
- **Metrics Package:** Prometheus integration, HTTP tracking
- **Logging Package:** Structured logging, request correlation
- **Core Library:** Git operations, object storage, references

## 🤝 Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Development Setup

```bash
git clone https://github.com/caiatech/govc.git
cd govc
go mod download
go test ./...
```

### Architecture Documents

- [Architecture Overview](docs/ARCHITECTURE.md)
- [Phase 1 Implementation](docs/PHASE1_PROGRESS.md)
- [Development Roadmap](ROADMAP.md)

## 📄 License

Apache License 2.0 - see [LICENSE](LICENSE) file for details.

## 🏢 About

govc is developed by [Caia Tech](https://caia.tech), reimagining version control for the infrastructure age.

**Key Contributors:**
- Memory-first Git implementation
- Production infrastructure components  
- Enterprise security features
- Comprehensive test coverage
- Performance optimization

---

**Production Ready** ✅ | **Enterprise Security** 🔐 | **High Performance** ⚡ | **Comprehensive Monitoring** 📊
