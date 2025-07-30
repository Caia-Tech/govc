# govc REST API

Memory-first Git operations over HTTP. This REST API exposes govc's unique features like parallel realities, transactional commits, and time travel through a simple HTTP interface.

## Quick Start

### Running the Server

```bash
# Run with default settings
go run cmd/govc-server/main.go

# Run with custom port
go run cmd/govc-server/main.go -port 3000

# Run with authentication enabled
go run cmd/govc-server/main.go -auth

# Run with debug mode
go run cmd/govc-server/main.go -debug
```

### Docker

```bash
# Build the image
docker build -t govc-server .

# Run the container
docker run -p 8080:8080 govc-server

# Run with environment variables
docker run -p 8080:8080 \
  -e PORT=3000 \
  -e MAX_REPOS=500 \
  -e ENABLE_AUTH=true \
  govc-server
```

## API Endpoints

### Repository Management

#### Create Repository
```bash
POST /api/v1/repos
Content-Type: application/json

{
  "id": "my-repo",
  "memory_only": true
}
```

#### Get Repository
```bash
GET /api/v1/repos/{repo_id}
```

#### List Repositories
```bash
GET /api/v1/repos
```

#### Delete Repository
```bash
DELETE /api/v1/repos/{repo_id}
```

### Basic Git Operations

#### Add File
```bash
POST /api/v1/repos/{repo_id}/add
Content-Type: application/json

{
  "path": "README.md",
  "content": "# My Project"
}
```

#### Commit
```bash
POST /api/v1/repos/{repo_id}/commit
Content-Type: application/json

{
  "message": "Initial commit",
  "author": "John Doe",
  "email": "john@example.com"
}
```

#### Get Log
```bash
GET /api/v1/repos/{repo_id}/log?limit=50
```

#### Get Status
```bash
GET /api/v1/repos/{repo_id}/status
```

#### Show Commit
```bash
GET /api/v1/repos/{repo_id}/show/{commit_sha}
```

### Branch Operations

#### List Branches
```bash
GET /api/v1/repos/{repo_id}/branches
```

#### Create Branch
```bash
POST /api/v1/repos/{repo_id}/branches
Content-Type: application/json

{
  "name": "feature-x",
  "from": "main"
}
```

#### Checkout Branch
```bash
POST /api/v1/repos/{repo_id}/checkout
Content-Type: application/json

{
  "branch": "feature-x"
}
```

#### Merge Branches
```bash
POST /api/v1/repos/{repo_id}/merge
Content-Type: application/json

{
  "from": "feature-x",
  "to": "main"
}
```

### Memory-First Features

#### Transactions

```bash
# Begin transaction
POST /api/v1/repos/{repo_id}/transaction
Response: {"id": "tx_abc123", "repo_id": "my-repo", "created_at": "..."}

# Add to transaction
POST /api/v1/repos/{repo_id}/transaction/{tx_id}/add
Content-Type: application/json
{
  "path": "config.yaml",
  "content": "version: 2.0"
}

# Validate transaction
POST /api/v1/repos/{repo_id}/transaction/{tx_id}/validate

# Commit transaction
POST /api/v1/repos/{repo_id}/transaction/{tx_id}/commit
Content-Type: application/json
{
  "message": "Update configuration"
}

# Or rollback
POST /api/v1/repos/{repo_id}/transaction/{tx_id}/rollback
```

#### Parallel Realities

```bash
# Create parallel realities
POST /api/v1/repos/{repo_id}/parallel-realities
Content-Type: application/json
{
  "branches": ["test-config-a", "test-config-b", "test-config-c"]
}

# List realities
GET /api/v1/repos/{repo_id}/parallel-realities

# Apply changes to a reality
POST /api/v1/repos/{repo_id}/parallel-realities/{reality}/apply
Content-Type: application/json
{
  "changes": {
    "nginx.conf": "worker_processes 4;",
    "redis.conf": "maxmemory 2gb"
  }
}

# Benchmark a reality
GET /api/v1/repos/{repo_id}/parallel-realities/{reality}/benchmark
```

#### Time Travel

```bash
# Get repository state at timestamp
GET /api/v1/repos/{repo_id}/time-travel/{unix_timestamp}

# Read file at specific time
GET /api/v1/repos/{repo_id}/time-travel/{unix_timestamp}/read/path/to/file
```

## Authentication

When authentication is enabled (`-auth` flag), include a Bearer token:

```bash
curl -H "Authorization: Bearer your-token-here" \
  http://localhost:8080/api/v1/repos
```

## Error Responses

All errors follow this format:

```json
{
  "error": "Human readable error message",
  "code": "ERROR_CODE",
  "details": {}
}
```

Common error codes:
- `REPO_NOT_FOUND` - Repository doesn't exist
- `INVALID_REQUEST` - Request validation failed
- `MAX_REPOS_REACHED` - Repository limit exceeded
- `UNAUTHORIZED` - Missing or invalid auth token
- `RATE_LIMIT_EXCEEDED` - Too many requests

## Example: Infrastructure Testing Workflow

```bash
# 1. Create repository
curl -X POST http://localhost:8080/api/v1/repos \
  -H "Content-Type: application/json" \
  -d '{"id": "infra-test", "memory_only": true}'

# 2. Create parallel realities for testing
curl -X POST http://localhost:8080/api/v1/repos/infra-test/parallel-realities \
  -H "Content-Type: application/json" \
  -d '{"branches": ["high-memory", "balanced", "low-cost"]}'

# 3. Apply different configurations
curl -X POST http://localhost:8080/api/v1/repos/infra-test/parallel-realities/high-memory/apply \
  -H "Content-Type: application/json" \
  -d '{"changes": {"config.yaml": "memory: 16GB\ncpu: 8"}}'

# 4. Benchmark and compare
curl http://localhost:8080/api/v1/repos/infra-test/parallel-realities/high-memory/benchmark

# 5. Merge winning configuration
curl -X POST http://localhost:8080/api/v1/repos/infra-test/merge \
  -H "Content-Type: application/json" \
  -d '{"from": "parallel/high-memory", "to": "main"}'
```

## Client Libraries

### JavaScript/Node.js
```javascript
const GovcClient = require('@caia-tech/govc-client');

const client = new GovcClient({
  baseURL: 'http://localhost:8080',
  token: 'your-token'
});

// Create repository
const repo = await client.createRepo({
  id: 'my-project',
  memoryOnly: true
});

// Create transaction
const tx = await repo.beginTransaction();
await tx.add('config.yaml', 'version: 1.0');
await tx.add('app.conf', 'debug: false');
await tx.commit('Initial configuration');
```

### Python
```python
from govc_client import GovcClient

client = GovcClient(
    base_url='http://localhost:8080',
    token='your-token'
)

# Create repository
repo = client.create_repo(id='my-project', memory_only=True)

# Test configurations in parallel
realities = repo.parallel_realities(['config-a', 'config-b'])
for reality in realities:
    reality.apply(changes={'db.conf': 'pool_size: 50'})
    result = reality.benchmark()
    print(f"{reality.name}: {result}")
```

## Performance Considerations

- Repositories are kept in memory by default
- Use `memory_only: false` for persistence to disk
- The server can handle thousands of concurrent operations
- Consider implementing repository LRU eviction for large deployments

## Security

- Always use authentication in production
- Run behind a reverse proxy (nginx, caddy) for TLS
- Implement rate limiting for public APIs
- Validate and sanitize file paths to prevent traversal attacks

## Monitoring

Health check endpoint provides basic metrics:

```bash
GET /health
Response:
{
  "status": "healthy",
  "repos": 42,
  "transactions": 3,
  "max_repos": 1000
}
```

For production, consider adding:
- Prometheus metrics endpoint
- Structured logging
- Distributed tracing
- Repository size limits