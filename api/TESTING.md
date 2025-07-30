# govc REST API Test Suite

This directory contains a comprehensive test suite for the govc REST API server, including unit tests, integration tests, benchmarks, and stress tests.

## Test Files

### 1. `api_test.go` - Basic Unit Tests
- Repository CRUD operations
- Branch operations
- Transaction handling
- Parallel realities
- Health check endpoint

### 2. `api_comprehensive_test.go` - Edge Cases and Error Handling
- **Repository Edge Cases**: Empty IDs, invalid JSON, duplicates, special characters
- **Git Operations Edge Cases**: Path traversal, large files, empty commits
- **Branch Operations Complex**: Invalid branch names, duplicate branches
- **Transaction Concurrency**: Concurrent transaction creation and modifications
- **Parallel Realities Complex**: Multiple realities with different configurations
- **Time Travel Features**: Past/future state queries
- **Middleware Testing**: Rate limiting, authentication
- **Server Limits**: Repository count limits
- **Error Response Formats**: Consistent error handling

### 3. `api_integration_test.go` - Complex Workflows
- **Complete Workflow**: Full repo lifecycle from creation to merge
- **Transactional Workflow**: Multi-file transactions with validation
- **Parallel Reality Workflow**: A/B testing scenario
- **Concurrent Operations**: Simultaneous file additions and branch operations
- **Error Recovery**: Transaction rollback and invalid operation recovery

### 4. `api_benchmark_test.go` - Performance Benchmarks
- **Basic Operations**: Create, get, list repositories
- **Git Operations**: Add files, commits
- **Transactions**: Full transaction lifecycle
- **Branch Operations**: Create branches
- **Parallel Realities**: Creation and configuration
- **Concurrent Requests**: Mixed operations under load
- **Large Files**: Performance with varying file sizes
- **Transaction Sizes**: Performance with many files
- **Memory Usage**: Repository and transaction accumulation
- **JSON Serialization**: Encoding/decoding overhead
- **Routing**: Path routing performance
- **Middleware**: Authentication and rate limiting overhead

### 5. `api_stress_test.go` - Stress and Load Tests
- **Concurrent Repositories**: 100 repos with 50 ops each
- **Transaction Stress**: Heavy transaction load
- **Memory Leaks**: Repository lifecycle under sustained load
- **Parallel Realities Stress**: Many realities with changes
- **Burst Load**: Handling sudden traffic spikes
- **Long Running**: 60-second sustained load test

## Running Tests

### Run all tests (excluding stress tests):
```bash
go test -v -short
```

### Run specific test categories:
```bash
# Edge cases
go test -v -run TestRepositoryEdgeCases

# Integration tests
go test -v -run TestCompleteWorkflow

# Benchmarks
go test -bench=. -benchtime=10s -run=^$

# Stress tests (takes longer)
go test -v -run TestStress
```

### Run with race detection:
```bash
go test -race -short
```

### Generate coverage report:
```bash
go test -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Known Issues

1. **Concurrent Map Writes**: The underlying govc library has a race condition in `TransactionalCommit.Add()` when multiple goroutines add files to the same transaction. This needs to be fixed in the core library.

2. **Branch Creation Validation**: Creating branches from non-existent branches currently succeeds when it should fail.

3. **Duplicate Branch Names**: Creating duplicate branch names is allowed when it should return a conflict error.

## Test Configuration

Tests use the following configuration:
- Memory-only repositories (no disk persistence)
- No authentication by default
- High repository limits for stress tests
- Gin test mode for cleaner output

## Adding New Tests

When adding new tests:
1. Use subtests with `t.Run()` for better organization
2. Create helper functions for common operations
3. Clean up resources after tests
4. Use meaningful test names that describe the scenario
5. Add stress tests for new features that might have concurrency issues

## Performance Baselines

Current benchmark results on Apple M2 Pro:
- Repository creation: ~X ops/sec
- File additions: ~X ops/sec
- Commits: ~X ops/sec
- Transactions: ~X ops/sec
- Concurrent operations: ~X ops/sec

These baselines should be monitored for performance regressions.