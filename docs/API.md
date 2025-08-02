# API Documentation

## Overview

The govc REST API provides comprehensive Git operations through HTTP endpoints. This document covers all available endpoints, request/response formats, authentication, and examples.

## Table of Contents

- [Authentication](#authentication)
- [API Versioning](#api-versioning)
- [Error Handling](#error-handling)
- [Repository Management](#repository-management)
- [Git Operations](#git-operations)
- [File Operations](#file-operations)
- [Branch Operations](#branch-operations)
- [Advanced Features](#advanced-features)
- [Monitoring Endpoints](#monitoring-endpoints)

## Authentication

### JWT Authentication

Obtain a JWT token by logging in:

```bash
POST /api/v1/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123"
}

Response:
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_at": "2024-01-16T10:00:00Z",
  "user": {
    "id": "user-123",
    "username": "admin",
    "roles": ["admin"]
  }
}
```

Use the token in subsequent requests:

```bash
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

### API Key Authentication

Create an API key:

```bash
POST /api/v1/apikeys
Authorization: Bearer <jwt-token>
Content-Type: application/json

{
  "name": "CI/CD Pipeline",
  "permissions": ["repo:read", "repo:write"],
  "expires_at": "2024-12-31T23:59:59Z"
}

Response:
{
  "key": "gv_1234567890abcdef",
  "key_id": "key-123",
  "name": "CI/CD Pipeline",
  "created_at": "2024-01-15T10:00:00Z"
}
```

Use the API key:

```bash
X-API-Key: gv_1234567890abcdef
```

## API Versioning

The API uses URL versioning:

- **v1**: `/api/v1/*` - Original API
- **v2**: `/api/v2/*` - Clean architecture implementation

Both versions are maintained for backward compatibility.

## Error Handling

All errors follow a consistent format:

```json
{
  "error": "Human-readable error message",
  "code": "MACHINE_READABLE_CODE",
  "details": {
    "field": "repo_id",
    "value": "invalid-id!"
  }
}
```

Common error codes:

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVALID_REQUEST` | 400 | Malformed request |
| `INVALID_REPO_ID` | 400 | Invalid repository identifier |
| `INVALID_FILE_PATH` | 400 | Invalid or unsafe file path |
| `REPO_NOT_FOUND` | 404 | Repository doesn't exist |
| `REPO_EXISTS` | 409 | Repository already exists |
| `UNAUTHORIZED` | 401 | Missing or invalid authentication |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `MAX_REPOS_REACHED` | 503 | Repository limit exceeded |

## Repository Management

### Create Repository

```bash
POST /api/v2/repos
Content-Type: application/json

{
  "id": "my-project",
  "memory_only": false,
  "description": "My awesome project"
}

Response: 201 Created
{
  "id": "my-project",
  "path": "/storage/repos/my-project",
  "current_branch": "main",
  "created_at": "2024-01-15T10:00:00Z"
}
```

**Validation Rules:**
- Repository ID: 1-64 characters, alphanumeric with hyphens/underscores
- Cannot use reserved names: `admin`, `api`, `health`, `metrics`
- Must start with alphanumeric character

### List Repositories

```bash
GET /api/v2/repos

Response: 200 OK
{
  "repositories": [
    {
      "id": "my-project",
      "path": "/storage/repos/my-project",
      "current_branch": "main",
      "created_at": "2024-01-15T10:00:00Z"
    }
  ],
  "count": 1
}
```

### Get Repository

```bash
GET /api/v2/repos/{repo_id}

Response: 200 OK
{
  "id": "my-project",
  "path": "/storage/repos/my-project",
  "current_branch": "main",
  "created_at": "2024-01-15T10:00:00Z"
}
```

### Delete Repository

```bash
DELETE /api/v2/repos/{repo_id}

Response: 200 OK
{
  "message": "repository my-project deleted"
}
```

## Git Operations

### Add Files

```bash
POST /api/v2/repos/{repo_id}/files
Content-Type: application/json

{
  "path": "src/main.go",
  "content": "package main\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}"
}

Response: 201 Created
{
  "path": "src/main.go",
  "size": 58,
  "added": true
}
```

**Validation:**
- Path cannot contain `..` (path traversal)
- Path cannot be absolute
- Path cannot contain null bytes
- Maximum file size: 50MB

### Read File

```bash
GET /api/v2/repos/{repo_id}/files?path=src/main.go

Response: 200 OK
{
  "path": "src/main.go",
  "content": "package main\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}",
  "size": 58,
  "mode": "100644"
}
```

### Create Commit

```bash
POST /api/v2/repos/{repo_id}/commits
Content-Type: application/json

{
  "message": "Initial commit",
  "author": "John Doe",
  "email": "john@example.com"
}

Response: 201 Created
{
  "hash": "abc123def456...",
  "message": "Initial commit",
  "author": {
    "name": "John Doe",
    "email": "john@example.com"
  },
  "timestamp": "2024-01-15T10:30:00Z",
  "parents": [],
  "tree": "tree123..."
}
```

**Validation:**
- Message: Required, max 100,000 characters
- Email: Must be valid email format
- Author: Max 100 characters

### Get Commit Log

```bash
GET /api/v2/repos/{repo_id}/commits?limit=10&offset=0

Response: 200 OK
{
  "commits": [
    {
      "hash": "abc123def456...",
      "message": "Initial commit",
      "author": {
        "name": "John Doe",
        "email": "john@example.com"
      },
      "timestamp": "2024-01-15T10:30:00Z"
    }
  ],
  "total": 1,
  "has_more": false
}
```

### Get Repository Status

```bash
GET /api/v2/repos/{repo_id}/status

Response: 200 OK
{
  "branch": "main",
  "clean": false,
  "staged": ["src/main.go"],
  "modified": ["README.md"],
  "untracked": ["test.txt"]
}
```

## File Operations

### List Directory Tree

```bash
GET /api/v2/repos/{repo_id}/tree?path=src

Response: 200 OK
{
  "path": "src",
  "entries": [
    {
      "name": "main.go",
      "type": "file",
      "size": 58,
      "mode": "100644"
    },
    {
      "name": "utils",
      "type": "directory"
    }
  ]
}
```

### Move/Rename File

```bash
POST /api/v2/repos/{repo_id}/files/move
Content-Type: application/json

{
  "from": "old-name.txt",
  "to": "new-name.txt"
}

Response: 200 OK
{
  "from": "old-name.txt",
  "to": "new-name.txt",
  "moved": true
}
```

### Delete File

```bash
DELETE /api/v2/repos/{repo_id}/files?path=unwanted.txt

Response: 200 OK
{
  "path": "unwanted.txt",
  "removed": true
}
```

## Branch Operations

### Create Branch

```bash
POST /api/v2/repos/{repo_id}/branches
Content-Type: application/json

{
  "name": "feature/new-feature",
  "from": "main"
}

Response: 201 Created
{
  "name": "feature/new-feature",
  "commit": "abc123def456...",
  "created": true
}
```

**Validation:**
- Cannot contain spaces, `~`, `^`, `:`, `?`, `*`, `[`, `@{`, `\`
- Cannot start or end with hyphen
- Cannot contain consecutive dots

### List Branches

```bash
GET /api/v2/repos/{repo_id}/branches

Response: 200 OK
{
  "branches": [
    {
      "name": "main",
      "commit": "abc123...",
      "is_current": true
    },
    {
      "name": "feature/new-feature",
      "commit": "def456...",
      "is_current": false
    }
  ],
  "current": "main"
}
```

### Switch Branch

```bash
POST /api/v2/repos/{repo_id}/checkout
Content-Type: application/json

{
  "branch": "feature/new-feature"
}

Response: 200 OK
{
  "previous": "main",
  "current": "feature/new-feature"
}
```

### Merge Branch

```bash
POST /api/v2/repos/{repo_id}/merge
Content-Type: application/json

{
  "source": "feature/new-feature",
  "message": "Merge feature/new-feature into main"
}

Response: 200 OK
{
  "commit": "merge123...",
  "message": "Merge feature/new-feature into main",
  "merged": true
}
```

## Advanced Features

### Transactions

Start a transaction for atomic operations:

```bash
POST /api/v1/repos/{repo_id}/transaction

Response: 201 Created
{
  "transaction_id": "tx-123",
  "expires_at": "2024-01-15T11:00:00Z"
}
```

Add files to transaction:

```bash
POST /api/v1/repos/{repo_id}/transaction/{tx_id}/add
Content-Type: application/json

{
  "files": [
    {"path": "file1.txt", "content": "content1"},
    {"path": "file2.txt", "content": "content2"}
  ]
}
```

Commit transaction:

```bash
POST /api/v1/repos/{repo_id}/transaction/{tx_id}/commit
Content-Type: application/json

{
  "message": "Atomic commit",
  "author": "John Doe",
  "email": "john@example.com"
}
```

### Search

Search commits:

```bash
GET /api/v1/repos/{repo_id}/search/commits?q=fix&limit=10

Response: 200 OK
{
  "commits": [...],
  "total": 25
}
```

Search file content:

```bash
GET /api/v1/repos/{repo_id}/search/content?q=TODO&path=*.go

Response: 200 OK
{
  "matches": [
    {
      "file": "src/main.go",
      "line": 42,
      "content": "// TODO: Implement this feature"
    }
  ]
}
```

### Hooks

Register a webhook:

```bash
POST /api/v1/repos/{repo_id}/hooks
Content-Type: application/json

{
  "url": "https://example.com/webhook",
  "events": ["commit", "branch_create"],
  "secret": "webhook-secret"
}

Response: 201 Created
{
  "id": "hook-123",
  "url": "https://example.com/webhook",
  "events": ["commit", "branch_create"],
  "active": true
}
```

## Monitoring Endpoints

### Health Check

```bash
GET /health

Response: 200 OK
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:00:00Z",
  "version": "1.0.0",
  "uptime": 3600,
  "checks": {
    "database": "ok",
    "storage": "ok",
    "memory": {
      "used_mb": 150,
      "limit_mb": 1024
    }
  }
}
```

### Prometheus Metrics

```bash
GET /metrics

Response: 200 OK
# HELP govc_http_requests_total Total HTTP requests
# TYPE govc_http_requests_total counter
govc_http_requests_total{method="GET",status="200"} 1523

# HELP govc_repositories_total Total number of repositories
# TYPE govc_repositories_total gauge
govc_repositories_total 42
```

### API Version

```bash
GET /version

Response: 200 OK
{
  "version": "1.0.0",
  "git_commit": "abc123",
  "build_time": "2024-01-15T09:00:00Z",
  "go_version": "1.21"
}
```

## Rate Limiting

API requests are rate limited:

- **Default**: 60 requests per minute per IP
- **Authenticated**: 300 requests per minute per user
- **Headers**: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`

Example response when rate limited:

```bash
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 60
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1705320000

{
  "error": "Rate limit exceeded",
  "code": "RATE_LIMIT_EXCEEDED",
  "retry_after": 45
}
```

## Request Examples

### Complete Workflow Example

```bash
# 1. Create repository
curl -X POST http://localhost:8080/api/v2/repos \
  -H "Content-Type: application/json" \
  -d '{"id": "my-app"}'

# 2. Add files
curl -X POST http://localhost:8080/api/v2/repos/my-app/files \
  -H "Content-Type: application/json" \
  -d '{"path": "README.md", "content": "# My App"}'

curl -X POST http://localhost:8080/api/v2/repos/my-app/files \
  -H "Content-Type: application/json" \
  -d '{"path": "main.go", "content": "package main..."}'

# 3. Create commit
curl -X POST http://localhost:8080/api/v2/repos/my-app/commits \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Initial commit",
    "author": "Developer",
    "email": "dev@example.com"
  }'

# 4. Create feature branch
curl -X POST http://localhost:8080/api/v2/repos/my-app/branches \
  -H "Content-Type: application/json" \
  -d '{"name": "feature/new-feature"}'

# 5. Switch to feature branch
curl -X POST http://localhost:8080/api/v2/repos/my-app/checkout \
  -H "Content-Type: application/json" \
  -d '{"branch": "feature/new-feature"}'

# 6. Make changes and commit
curl -X POST http://localhost:8080/api/v2/repos/my-app/files \
  -H "Content-Type: application/json" \
  -d '{"path": "feature.go", "content": "package main..."}'

curl -X POST http://localhost:8080/api/v2/repos/my-app/commits \
  -H "Content-Type: application/json" \
  -d '{"message": "Add new feature", "author": "Developer", "email": "dev@example.com"}'

# 7. Merge back to main
curl -X POST http://localhost:8080/api/v2/repos/my-app/checkout \
  -H "Content-Type: application/json" \
  -d '{"branch": "main"}'

curl -X POST http://localhost:8080/api/v2/repos/my-app/merge \
  -H "Content-Type: application/json" \
  -d '{"source": "feature/new-feature", "message": "Merge new feature"}'
```

## SDK Support

Official SDKs are available:

- **Go**: `github.com/caiatech/govc/sdk/go`
- **JavaScript/TypeScript**: `@caiatech/govc`
- **Python**: Coming soon
- **Ruby**: Coming soon

Example using the Go SDK:

```go
import "github.com/caiatech/govc/sdk/go"

client := govc.NewClient("http://localhost:8080", "api-key")

// Create repository
repo, err := client.CreateRepository("my-app", false)

// Add file
err = repo.AddFile("main.go", "package main...")

// Commit
commit, err := repo.Commit("Initial commit", "Dev", "dev@example.com")
```