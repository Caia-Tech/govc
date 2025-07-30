# Govc Go Client Library

The official Go client library for interacting with govc servers.

## Installation

```bash
go get github.com/caiatech/govc/client/go/govc
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/caiatech/govc/client/go/govc"
)

func main() {
    // Create a client
    client, err := govc.NewClient("https://govc.example.com")
    if err != nil {
        log.Fatal(err)
    }

    // Authenticate (if required)
    auth := client.Auth()
    resp, err := auth.Login(context.Background(), "username", "password")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Logged in successfully, token expires at: %v\n", resp.ExpiresAt)

    // Create a repository
    repo, err := client.CreateRepo(context.Background(), "my-project", &govc.CreateRepoOptions{
        MemoryOnly: true,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Add a file
    err = repo.AddFile(context.Background(), "README.md", "# My Project\n\nWelcome!")
    if err != nil {
        log.Fatal(err)
    }

    // Commit changes
    commit, err := repo.Commit(context.Background(), "Initial commit", &govc.Author{
        Name:  "John Doe",
        Email: "john@example.com",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Created commit: %s\n", commit.Hash)
}
```

## Authentication

The client supports both JWT tokens and API keys:

### JWT Authentication

```go
// Login with username/password
auth := client.Auth()
resp, err := auth.Login(ctx, "username", "password")

// The token is automatically stored in the client
// You can also set it manually:
client.SetToken("your-jwt-token")

// Refresh token before expiry
resp, err = auth.RefreshToken(ctx)
```

### API Key Authentication

```go
// Create an API key (requires JWT auth first)
keyResp, err := auth.CreateAPIKey(ctx, "ci-key", []string{"repo:read", "repo:write"})

// Use the API key
client.SetAPIKey(keyResp.Key)

// Or create a new client with API key
client, _ := govc.NewClient("https://govc.example.com",
    govc.WithAPIKey("your-api-key"))
```

## Repository Operations

### Basic Operations

```go
// Get a repository
repo, err := client.GetRepo(ctx, "my-project")

// List all repositories
repos, err := client.ListRepos(ctx)

// Delete a repository
err = client.DeleteRepo(ctx, "old-project")
```

### Working with Files

```go
// Add files to staging
err = repo.AddFile(ctx, "src/main.go", "package main\n\nfunc main() {}")

// Write files directly
err = repo.WriteFile(ctx, "config.yaml", "version: 1.0")

// Read files
content, err := repo.ReadFile(ctx, "README.md")

// Get repository status
status, err := repo.Status(ctx)
fmt.Printf("Branch: %s, Staged: %d files\n", status.Branch, len(status.Staged))

// Commit changes
commit, err := repo.Commit(ctx, "Add main.go", nil) // nil uses default author
```

### Branch Management

```go
// List branches
branches, err := repo.ListBranches(ctx)

// Create a new branch
err = repo.CreateBranch(ctx, "feature/new-feature", "main")

// Checkout a branch
err = repo.Checkout(ctx, "feature/new-feature")

// Merge branches
err = repo.Merge(ctx, "feature/new-feature", "main")
```

### Tags

```go
// List tags
tags, err := repo.ListTags(ctx)

// Create a tag
err = repo.CreateTag(ctx, "v1.0.0", "Release version 1.0.0")
```

## Transactions

Transactions provide atomic operations:

```go
// Start a transaction
tx, err := repo.Transaction(ctx)

// Add multiple files
err = tx.Add(ctx, "file1.txt", "Content 1")
err = tx.Add(ctx, "file2.txt", "Content 2")

// Validate the transaction
err = tx.Validate(ctx)

// Commit if valid
commit, err := tx.Commit(ctx, "Add multiple files atomically")

// Or rollback
err = tx.Rollback(ctx)
```

## Advanced Features

### Parallel Realities

Test multiple configurations simultaneously:

```go
// Create parallel realities
realities, err := repo.CreateParallelRealities(ctx, []string{
    "config-a",
    "config-b",
    "config-c",
})

// Benchmark a reality
result, err := repo.BenchmarkReality(ctx, "config-a")
fmt.Printf("Reality %s performed %v\n", result.Reality, result.Better)
```

### Time Travel

Access historical snapshots:

```go
// Get repository state at a specific time
snapshot, err := repo.TimeTravel(ctx, "2024-01-01T12:00:00Z")

// Access files from that point in time
for _, file := range snapshot.Files {
    fmt.Printf("File: %s\n", file.Path)
}
```

## Error Handling

The client returns typed errors:

```go
repo, err := client.GetRepo(ctx, "non-existent")
if err != nil {
    if errResp, ok := err.(*govc.ErrorResponse); ok {
        fmt.Printf("Error code: %s, message: %s\n", errResp.Code, errResp.Error)
    }
}
```

## Configuration Options

```go
// Create client with custom options
client, err := govc.NewClient("https://govc.example.com",
    govc.WithHTTPClient(&http.Client{
        Timeout: 60 * time.Second,
    }),
    govc.WithToken("your-token"),
    govc.WithUserAgent("my-app/1.0"),
)
```

## Context Support

All methods accept a context for cancellation and timeouts:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

repo, err := client.CreateRepo(ctx, "my-project", nil)
```

## Examples

See the [examples](examples/) directory for more complete examples:

- `basic/` - Basic repository operations
- `auth/` - Authentication examples
- `transactions/` - Using transactions
- `parallel/` - Parallel realities example
- `ci/` - CI/CD integration example

## API Reference

For complete API documentation, see the [GoDoc](https://pkg.go.dev/github.com/caiatech/govc/client/go/govc).

## Contributing

Contributions are welcome! Please read our [Contributing Guide](../../../CONTRIBUTING.md) for details.

## License

This client library is part of the govc project and is licensed under the same terms.