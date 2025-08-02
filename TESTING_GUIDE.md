# 🧪 govc Testing Guide

> **Note**: Testing is a critical part of govc development. This guide documents our multi-layered testing approach and how to contribute to test coverage.

## Overview

govc employs a comprehensive testing strategy to ensure reliability during rapid development. Our testing philosophy emphasizes:

- **Comprehensive Coverage**: Every feature must have tests
- **Multiple Layers**: Unit, integration, and system tests
- **Continuous Verification**: Tests run on every change
- **Real-World Scenarios**: Tests simulate actual usage patterns

## Testing Architecture

### 1. Unit Tests

Located alongside source files as `*_test.go`:

```
repository_test.go     - Core repository operations
parallel_test.go       - Parallel reality features
storage/*_test.go      - Storage layer tests
refs/*_test.go         - Reference management tests
object/*_test.go       - Git object tests
```

**Current Coverage**: 52.1% overall

### 2. Integration Tests

Located in `api/*_test.go`:

```
api_comprehensive_test.go    - Full API workflow tests
api_git_integration_test.go  - Git compatibility tests
handlers_*_test.go          - Individual endpoint tests
```

**Focus**: End-to-end workflows and API behavior

### 3. System Tests

High-level tests that verify complete system behavior:

```
govc_test.go              - System-wide scenarios
examples_test.go          - Example-driven tests
basic_test.go             - Basic operations
```

## Running Tests

### Quick Test Commands

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run specific package tests
go test ./pkg/storage/...

# Run with verbose output
go test -v ./...

# Run specific test
go test -run TestStash

# Run with race detection
go test -race ./...
```

### Comprehensive Test Suite

We provide a comprehensive test runner:

```bash
# Run all tests with detailed reporting
./run_tests.sh

# Generate coverage report
./generate_coverage_report.sh

# Run specific test suites
./run_tests.sh --unit
./run_tests.sh --integration
./run_tests.sh --system
```

## Test Categories

### 1. Core Functionality Tests

Tests for fundamental VCS operations:

- `TestCommit` - Basic commit operations
- `TestBranch` - Branch management
- `TestMerge` - Merge operations
- `TestStash` - Stash functionality
- `TestCheckout` - Working tree updates

### 2. Memory-First Tests

Special tests for in-memory operations:

- `TestMemoryRepository` - Pure memory operations
- `TestParallelRealities` - Concurrent branch testing
- `TestTimeTravel` - Historical state access

### 3. Storage Layer Tests

Comprehensive storage testing:

- `TestObjectStore` - Object storage operations
- `TestRefStore` - Reference storage
- `TestWorkingStorage` - Working tree storage
- `TestStorageAdapters` - Adapter pattern tests

### 4. API Tests

RESTful API endpoint testing:

- `TestRepositoryAPI` - Repository management
- `TestFileAPI` - File operations
- `TestBranchAPI` - Branch operations
- `TestAuthAPI` - Authentication flows

## Writing Tests

### Test Structure

Follow this pattern for consistency:

```go
func TestFeatureName(t *testing.T) {
    // Setup
    repo := New()
    
    // Test subcases
    t.Run("successful case", func(t *testing.T) {
        // Arrange
        err := repo.WriteFile("test.txt", []byte("content"))
        require.NoError(t, err)
        
        // Act
        err = repo.Add("test.txt")
        
        // Assert
        assert.NoError(t, err)
        status := repo.Status()
        assert.Contains(t, status.Staged, "test.txt")
    })
    
    t.Run("error case", func(t *testing.T) {
        // Test error conditions
    })
}
```

### Testing Best Practices

1. **Use Table-Driven Tests** for multiple scenarios:

```go
tests := []struct {
    name     string
    input    string
    expected string
    wantErr  bool
}{
    {"valid input", "test", "result", false},
    {"empty input", "", "", true},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // Test implementation
    })
}
```

2. **Test Edge Cases**:
   - Empty repositories
   - Large files
   - Concurrent operations
   - Invalid inputs
   - Resource limits

3. **Use Test Helpers**:

```go
func setupTestRepo(t *testing.T) *Repository {
    t.Helper()
    repo := New()
    // Common setup
    return repo
}
```

4. **Clean Up Resources**:

```go
defer func() {
    // Cleanup
    repo.Close()
    os.RemoveAll(tempDir)
}()
```

## Test Coverage

### Current Coverage by Package

| Package | Coverage | Target | Status |
|---------|----------|--------|--------|
| govc (main) | 32.3% | 60% | 🟡 Improving |
| pkg/storage | 58.3% | 70% | 🟢 Good |
| pkg/refs | 60.5% | 70% | 🟢 Good |
| pkg/object | 54.4% | 70% | 🟡 Improving |
| api | Comprehensive | 80% | 🟢 Good |

### Viewing Coverage

```bash
# Generate HTML coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# View in terminal
go tool cover -func=coverage.out
```

## Continuous Integration

Tests run automatically on:

- Every push to main
- All pull requests
- Nightly builds
- Release candidates

### CI Test Matrix

- **Go Versions**: 1.20, 1.21, 1.22
- **Operating Systems**: Linux, macOS, Windows
- **Architectures**: amd64, arm64

## Recent Test Improvements

### August 2024 Test Fixes

- ✅ Fixed stash restoration for memory repositories
- ✅ Resolved checkout orphan branch creation
- ✅ Fixed time travel empty repository handling
- ✅ Eliminated repository operation deadlocks
- ✅ Corrected storage adapter idempotency

### Test Reliability Improvements

- Added deterministic timestamps in tests
- Fixed race conditions in parallel tests
- Improved test isolation
- Enhanced error message verification

## Debugging Tests

### Verbose Output

```bash
# Run with detailed logging
go test -v -run TestName

# With additional debug output
DEBUG=1 go test -v ./...
```

### Test Timeouts

For long-running tests:

```bash
go test -timeout 30s ./...
```

### Race Detection

Always test with race detection before committing:

```bash
go test -race ./...
```

## Contributing Tests

When contributing, ensure:

1. **New Features**: Include comprehensive tests
2. **Bug Fixes**: Add regression tests
3. **API Changes**: Update integration tests
4. **Performance**: Include benchmarks

### Test Checklist

- [ ] Unit tests for new functions
- [ ] Integration tests for API changes
- [ ] Edge case coverage
- [ ] Error condition testing
- [ ] Documentation updates
- [ ] No test flakiness

## Performance Testing

### Benchmarks

Run performance benchmarks:

```bash
go test -bench=. ./...
go test -bench=BenchmarkCommit -benchmem
```

### Load Testing

For API endpoints:

```bash
# Using Apache Bench
ab -n 1000 -c 10 http://localhost:8080/api/v1/repos

# Using k6
k6 run scripts/load-test.js
```

## Test Maintenance

### Fixing Flaky Tests

1. Identify through repeated runs:
   ```bash
   go test -count=10 -run TestName
   ```

2. Common causes:
   - Time-dependent logic
   - Shared state
   - Resource contention
   - Network dependencies

### Test Refactoring

Regularly review and refactor tests:

- Remove redundant tests
- Consolidate similar tests
- Update deprecated patterns
- Improve test names

## Future Testing Goals

### Short Term (Q3 2024)
- [ ] Achieve 70% code coverage
- [ ] Eliminate all flaky tests
- [ ] Add fuzzing tests
- [ ] Improve integration test speed

### Long Term
- [ ] 80%+ code coverage
- [ ] Property-based testing
- [ ] Chaos engineering tests
- [ ] Performance regression detection

---

**Remember**: Tests are not just about coverage—they're about confidence in the system's behavior. Write tests that give you confidence to refactor and evolve the codebase.