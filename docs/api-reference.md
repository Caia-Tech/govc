# govc REST API Reference

## Overview

The govc REST API provides a complete Git implementation accessible via HTTP. All operations are memory-first for maximum performance, with optional disk persistence.

**Base URL**: `https://api.govc.example.com`  
**API Version**: `v1`  
**Full Base Path**: `https://api.govc.example.com/api/v1`

## Table of Contents

- [Authentication](#authentication)
- [Repository Operations](#repository-operations)
- [File Operations](#file-operations)
- [Branch Operations](#branch-operations)
- [Commit Operations](#commit-operations)
- [Tag Operations](#tag-operations)
- [Advanced Git Operations](#advanced-git-operations)
- [Search Operations](#search-operations)
- [Hooks & Events](#hooks--events)
- [User Management](#user-management)
- [Health & Monitoring](#health--monitoring)
- [Error Responses](#error-responses)

## Authentication

### Login

Authenticate with username and password to receive a JWT token.

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "username": "alice",
  "password": "secret123"
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_at": "2024-01-16T15:30:00Z",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "username": "alice",
    "email": "alice@example.com"
  }
}
```

### Get Current User

```http
GET /api/v1/auth/me
Authorization: Bearer <token>
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "username": "alice",
  "email": "alice@example.com",
  "is_active": true,
  "is_admin": true,
  "roles": ["admin", "developer"],
  "created_at": "2024-01-01T10:00:00Z"
}
```

### API Key Management

#### Create API Key

```http
POST /api/v1/auth/keys
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "CI Pipeline"
}
```

**Response:**
```json
{
  "id": "660e8400-e29b-41d4-a716-446655440001",
  "name": "CI Pipeline",
  "key": "gk_live_abcdefghijklmnopqrstuvwxyz1234567890",
  "created_at": "2024-01-15T10:30:00Z"
}
```

#### List API Keys

```http
GET /api/v1/auth/keys
Authorization: Bearer <token>
```

#### Delete API Key

```http
DELETE /api/v1/auth/keys/{key_id}
Authorization: Bearer <token>
```

## Repository Operations

### Create Repository

```http
POST /api/v1/repos
Authorization: Bearer <token>
Content-Type: application/json

{
  "id": "my-project",
  "memory_only": false,
  "description": "Main project repository"
}
```

**Response:**
```json
{
  "id": "my-project",
  "memory_only": false,
  "description": "Main project repository",
  "created_at": "2024-01-15T10:00:00Z",
  "updated_at": "2024-01-15T10:00:00Z"
}
```

### List Repositories

```http
GET /api/v1/repos
Authorization: Bearer <token>
```

**Query Parameters:**
- `memory_only` (boolean): Filter by storage type
- `limit` (integer): Maximum results (default: 100)
- `offset` (integer): Pagination offset

### Get Repository

```http
GET /api/v1/repos/{repo_id}
Authorization: Bearer <token>
```

### Delete Repository

```http
DELETE /api/v1/repos/{repo_id}
Authorization: Bearer <token>
```

### Clone Repository

```http
POST /api/v1/repos/{repo_id}/clone
Authorization: Bearer <token>
Content-Type: application/json

{
  "target_id": "my-project-clone"
}
```

## File Operations

### Read File

```http
GET /api/v1/repos/{repo_id}/read/{path}
Authorization: Bearer <token>
```

**Query Parameters:**
- `ref` (string): Branch, tag, or commit hash (default: current HEAD)

**Response:**
```json
{
  "path": "src/main.go",
  "content": "package main\n\nfunc main() {\n    // ...\n}",
  "size": 1234,
  "mode": "100644",
  "hash": "abc123def456..."
}
```

### List Directory

```http
GET /api/v1/repos/{repo_id}/tree/{path}
Authorization: Bearer <token>
```

**Query Parameters:**
- `ref` (string): Branch, tag, or commit hash
- `recursive` (boolean): List recursively

**Response:**
```json
{
  "path": "src",
  "entries": [
    {
      "name": "main.go",
      "type": "file",
      "mode": "100644",
      "size": 1234,
      "hash": "abc123..."
    },
    {
      "name": "utils",
      "type": "directory",
      "mode": "040000"
    }
  ]
}
```

### Add/Update File

```http
POST /api/v1/repos/{repo_id}/add
Authorization: Bearer <token>
Content-Type: application/json

{
  "path": "src/new-file.go",
  "content": "package main\n\n// New file content",
  "mode": "100644"
}
```

### Remove File

```http
DELETE /api/v1/repos/{repo_id}/remove/{path}
Authorization: Bearer <token>
```

### Move/Rename File

```http
POST /api/v1/repos/{repo_id}/move
Authorization: Bearer <token>
Content-Type: application/json

{
  "from": "old-path.txt",
  "to": "new-path.txt"
}
```

### Get File Diff

```http
GET /api/v1/repos/{repo_id}/diff
Authorization: Bearer <token>
```

**Query Parameters:**
- `from` (string): Source commit/branch
- `to` (string): Target commit/branch (default: HEAD)
- `path` (string): Specific file path (optional)

### Get File Blame

```http
GET /api/v1/repos/{repo_id}/blame/{path}
Authorization: Bearer <token>
```

## Branch Operations

### List Branches

```http
GET /api/v1/repos/{repo_id}/branches
Authorization: Bearer <token>
```

**Response:**
```json
{
  "branches": [
    {
      "name": "main",
      "commit": "abc123def456...",
      "protected": true,
      "is_default": true
    },
    {
      "name": "feature/new-feature",
      "commit": "def456ghi789...",
      "protected": false,
      "is_default": false
    }
  ]
}
```

### Create Branch

```http
POST /api/v1/repos/{repo_id}/branches
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "feature/new-feature",
  "from": "main"
}
```

### Delete Branch

```http
DELETE /api/v1/repos/{repo_id}/branches/{branch_name}
Authorization: Bearer <token>
```

### Get Current Branch

```http
GET /api/v1/repos/{repo_id}/branch
Authorization: Bearer <token>
```

## Commit Operations

### Create Commit

```http
POST /api/v1/repos/{repo_id}/commit
Authorization: Bearer <token>
Content-Type: application/json

{
  "message": "Add new feature",
  "author": {
    "name": "Alice Smith",
    "email": "alice@example.com"
  },
  "parents": ["abc123def456..."]
}
```

### List Commits (Log)

```http
GET /api/v1/repos/{repo_id}/log
Authorization: Bearer <token>
```

**Query Parameters:**
- `branch` (string): Branch name
- `limit` (integer): Maximum commits
- `offset` (integer): Skip commits
- `since` (string): ISO 8601 date
- `until` (string): ISO 8601 date
- `author` (string): Filter by author

### Get Commit

```http
GET /api/v1/repos/{repo_id}/commits/{commit_hash}
Authorization: Bearer <token>
```

### Cherry Pick

```http
POST /api/v1/repos/{repo_id}/cherry-pick
Authorization: Bearer <token>
Content-Type: application/json

{
  "commit": "abc123def456...",
  "branch": "feature/target-branch"
}
```

### Revert Commit

```http
POST /api/v1/repos/{repo_id}/revert
Authorization: Bearer <token>
Content-Type: application/json

{
  "commit": "abc123def456...",
  "message": "Revert: Original commit message"
}
```

## Tag Operations

### List Tags

```http
GET /api/v1/repos/{repo_id}/tags
Authorization: Bearer <token>
```

### Create Tag

```http
POST /api/v1/repos/{repo_id}/tags
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "v1.0.0",
  "commit": "abc123def456...",
  "message": "Release version 1.0.0",
  "tagger": {
    "name": "Alice Smith",
    "email": "alice@example.com"
  }
}
```

### Delete Tag

```http
DELETE /api/v1/repos/{repo_id}/tags/{tag_name}
Authorization: Bearer <token>
```

## Advanced Git Operations

### Create Transaction

Start an atomic transaction for multiple operations.

```http
POST /api/v1/repos/{repo_id}/transaction
Authorization: Bearer <token>
```

**Response:**
```json
{
  "id": "tx_123456789",
  "created_at": "2024-01-15T10:00:00Z",
  "expires_at": "2024-01-15T10:05:00Z"
}
```

### Transaction Operations

All file operations can be performed within a transaction by adding the transaction ID:

```http
POST /api/v1/repos/{repo_id}/add?tx=tx_123456789
DELETE /api/v1/repos/{repo_id}/remove/{path}?tx=tx_123456789
```

### Commit Transaction

```http
POST /api/v1/repos/{repo_id}/transaction/{tx_id}/commit
Authorization: Bearer <token>
Content-Type: application/json

{
  "message": "Atomic commit of multiple changes",
  "author": {
    "name": "Alice Smith",
    "email": "alice@example.com"
  }
}
```

### Rollback Transaction

```http
POST /api/v1/repos/{repo_id}/transaction/{tx_id}/rollback
Authorization: Bearer <token>
```

### Stash Operations

#### Create Stash

```http
POST /api/v1/repos/{repo_id}/stash
Authorization: Bearer <token>
Content-Type: application/json

{
  "message": "WIP: Working on feature"
}
```

#### List Stashes

```http
GET /api/v1/repos/{repo_id}/stash
Authorization: Bearer <token>
```

#### Apply Stash

```http
POST /api/v1/repos/{repo_id}/stash/{stash_id}/apply
Authorization: Bearer <token>
```

#### Drop Stash

```http
DELETE /api/v1/repos/{repo_id}/stash/{stash_id}
Authorization: Bearer <token>
```

### Merge

```http
POST /api/v1/repos/{repo_id}/merge
Authorization: Bearer <token>
Content-Type: application/json

{
  "source": "feature/new-feature",
  "target": "main",
  "message": "Merge feature/new-feature into main",
  "strategy": "recursive"
}
```

### Rebase

```http
POST /api/v1/repos/{repo_id}/rebase
Authorization: Bearer <token>
Content-Type: application/json

{
  "branch": "feature/my-feature",
  "onto": "main",
  "interactive": false
}
```

### Reset

```http
POST /api/v1/repos/{repo_id}/reset
Authorization: Bearer <token>
Content-Type: application/json

{
  "commit": "abc123def456...",
  "mode": "hard"
}
```

**Reset Modes:**
- `soft`: Move HEAD only
- `mixed`: Move HEAD and reset index (default)
- `hard`: Move HEAD, reset index and working tree

### Parallel Realities

Create isolated branch universes for testing.

```http
POST /api/v1/repos/{repo_id}/parallel-realities
Authorization: Bearer <token>
Content-Type: application/json

{
  "branches": ["test-1", "test-2", "test-3"],
  "base": "main"
}
```

### Time Travel

Access repository state at any point in time.

```http
GET /api/v1/repos/{repo_id}/time-travel
Authorization: Bearer <token>
```

**Query Parameters:**
- `timestamp` (string): ISO 8601 timestamp
- `branch` (string): Specific branch

## Search Operations

### Search Commits

```http
GET /api/v1/repos/{repo_id}/search/commits
Authorization: Bearer <token>
```

**Query Parameters:**
- `q` (string): Search query
- `author` (string): Filter by author
- `since` (string): Start date
- `until` (string): End date

### Search Content

```http
GET /api/v1/repos/{repo_id}/search/content
Authorization: Bearer <token>
```

**Query Parameters:**
- `q` (string): Search query
- `path` (string): Path pattern
- `branch` (string): Search in branch

### Search Files

```http
GET /api/v1/repos/{repo_id}/search/files
Authorization: Bearer <token>
```

**Query Parameters:**
- `q` (string): Filename pattern
- `type` (string): File type filter

### Advanced Pattern Search (Grep)

```http
POST /api/v1/repos/{repo_id}/grep
Authorization: Bearer <token>
Content-Type: application/json

{
  "pattern": "TODO|FIXME",
  "paths": ["src/**/*.go"],
  "case_sensitive": false,
  "regex": true
}
```

## Hooks & Events

### Register Webhook

```http
POST /api/v1/repos/{repo_id}/hooks
Authorization: Bearer <token>
Content-Type: application/json

{
  "url": "https://example.com/webhook",
  "events": ["push", "commit", "branch_create"],
  "secret": "webhook-secret",
  "active": true
}
```

### List Webhooks

```http
GET /api/v1/repos/{repo_id}/hooks
Authorization: Bearer <token>
```

### Delete Webhook

```http
DELETE /api/v1/repos/{repo_id}/hooks/{hook_id}
Authorization: Bearer <token>
```

### Event Stream (SSE)

```http
GET /api/v1/repos/{repo_id}/events
Authorization: Bearer <token>
Accept: text/event-stream
```

**Event Types:**
- `commit`: New commit created
- `branch`: Branch created/deleted
- `tag`: Tag created/deleted
- `merge`: Merge completed
- `push`: Changes pushed

**Example Event:**
```
event: commit
data: {"repository":"my-project","commit":"abc123...","message":"Add feature","author":"alice"}
```

## User Management

### List Users

```http
GET /api/v1/users
Authorization: Bearer <token>
```

**Required Permission:** `system:admin`

### Create User

```http
POST /api/v1/users
Authorization: Bearer <token>
Content-Type: application/json

{
  "username": "bob",
  "email": "bob@example.com",
  "password": "secure-password",
  "is_admin": false,
  "roles": ["developer"]
}
```

### Get User

```http
GET /api/v1/users/{username}
Authorization: Bearer <token>
```

### Update User

```http
PATCH /api/v1/users/{username}
Authorization: Bearer <token>
Content-Type: application/json

{
  "email": "newemail@example.com",
  "is_active": true
}
```

### Delete User

```http
DELETE /api/v1/users/{username}
Authorization: Bearer <token>
```

### Change Password

```http
POST /api/v1/users/{username}/password
Authorization: Bearer <token>
Content-Type: application/json

{
  "current_password": "old-password",
  "new_password": "new-secure-password"
}
```

### Add Role

```http
POST /api/v1/users/{username}/roles
Authorization: Bearer <token>
Content-Type: application/json

{
  "role": "reviewer"
}
```

### Remove Role

```http
DELETE /api/v1/users/{username}/roles/{role}
Authorization: Bearer <token>
```

## Health & Monitoring

### Health Check

```http
GET /health
```

**Response:**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime": 3600
}
```

### Liveness Check

```http
GET /health/live
```

### Readiness Check

```http
GET /health/ready
```

### Prometheus Metrics

```http
GET /metrics
```

**Example Output:**
```
# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET",path="/api/v1/repos",status="200"} 1234

# HELP http_request_duration_seconds HTTP request latency
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.1"} 1234
```

## Error Responses

All errors follow a consistent format:

```json
{
  "error": {
    "code": "REPO_NOT_FOUND",
    "message": "Repository not found",
    "details": {
      "repository": "my-project"
    }
  }
}
```

### Common Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `UNAUTHORIZED` | 401 | Missing or invalid authentication |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `NOT_FOUND` | 404 | Resource not found |
| `REPO_NOT_FOUND` | 404 | Repository not found |
| `BRANCH_NOT_FOUND` | 404 | Branch not found |
| `CONFLICT` | 409 | Operation conflicts with current state |
| `VALIDATION_ERROR` | 400 | Invalid request data |
| `INTERNAL_ERROR` | 500 | Internal server error |

### Rate Limiting

Rate limit information is included in response headers:

```
X-RateLimit-Limit: 5000
X-RateLimit-Remaining: 4999
X-RateLimit-Reset: 1642329600
```

## Authentication Methods

### Bearer Token (JWT)

```http
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

### API Key

```http
X-API-Key: gk_live_abcdefghijklmnopqrstuvwxyz1234567890
```

## Pagination

List endpoints support pagination:

```http
GET /api/v1/repos?limit=20&offset=40
```

Response includes pagination metadata:

```json
{
  "data": [...],
  "pagination": {
    "total": 150,
    "limit": 20,
    "offset": 40,
    "has_more": true
  }
}
```

## Versioning

The API version is included in the URL path:
- Current: `/api/v1`
- Future: `/api/v2`

Version information is also available in response headers:
```
X-API-Version: v1
```

## CORS Support

The API supports CORS for browser-based clients:

```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS
Access-Control-Allow-Headers: Authorization, Content-Type, X-API-Key
```

## Request Limits

- Maximum request size: 10MB
- Maximum file size: 100MB
- Transaction timeout: 5 minutes
- Maximum parallel realities: 100
- Maximum stash entries: 50

## Performance Considerations

1. **Memory Operations**: All operations are memory-first for speed
2. **Caching**: Responses include cache headers where appropriate
3. **Compression**: Responses are gzip compressed
4. **Connection Pooling**: Keep-alive connections are supported
5. **Batch Operations**: Use transactions for multiple operations

## SDK Support

Official SDKs are available for:
- Go: `github.com/caiatech/govc/client/go`
- JavaScript/TypeScript: `@caiatech/govc-client`
- Python: Coming soon

## Example: Complete Workflow

```bash
# 1. Authenticate
curl -X POST https://api.govc.example.com/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"secret"}'

# 2. Create repository
curl -X POST https://api.govc.example.com/api/v1/repos \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":"my-project","memory_only":false}'

# 3. Start transaction
TX_ID=$(curl -X POST https://api.govc.example.com/api/v1/repos/my-project/transaction \
  -H "Authorization: Bearer $TOKEN" | jq -r .id)

# 4. Add files
curl -X POST "https://api.govc.example.com/api/v1/repos/my-project/add?tx=$TX_ID" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"README.md","content":"# My Project"}'

# 5. Commit transaction
curl -X POST "https://api.govc.example.com/api/v1/repos/my-project/transaction/$TX_ID/commit" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message":"Initial commit","author":{"name":"Alice","email":"alice@example.com"}}'
```