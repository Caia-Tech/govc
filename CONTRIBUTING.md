# Contributing to govc

Thank you for your interest in contributing to govc! As an actively evolving project, we welcome contributions while maintaining high standards for code quality and testing.

## 🚧 Development Status

**Important**: govc is in alpha stage. This means:
- APIs may change between versions
- Your contributions might need updates as the project evolves
- We prioritize learning and experimentation over stability
- Breaking changes are documented but may happen frequently

## 📋 Before You Contribute

1. **Read the Documentation**:
   - [DEVELOPMENT_STATUS.md](DEVELOPMENT_STATUS.md) - Current project status
   - [TESTING_GUIDE.md](TESTING_GUIDE.md) - Testing requirements
   - [API_REFERENCE.md](API_REFERENCE.md) - Current API documentation

2. **Check Existing Work**:
   - Browse [open issues](https://github.com/Caia-Tech/govc/issues)
   - Review [pull requests](https://github.com/Caia-Tech/govc/pulls)
   - Join [discussions](https://github.com/Caia-Tech/govc/discussions)

3. **Understand the Architecture**:
   - Memory-first design principles
   - Storage abstraction layers
   - Testing requirements

## 🛠️ Development Setup

### Prerequisites

- Go 1.20 or higher
- Git
- Make (optional but recommended)

### Setup Steps

```bash
# Clone the repository
git clone https://github.com/Caia-Tech/govc.git
cd govc

# Install dependencies
go mod download

# Run tests to verify setup
go test ./...

# Run with race detection
go test -race ./...
```

## 🧪 Testing Requirements

**All contributions MUST include tests**. We maintain multiple testing layers:

### 1. Unit Tests
For new functions or methods:
```go
func TestYourFeature(t *testing.T) {
    // Test implementation
}
```

### 2. Integration Tests
For API changes or complex features:
```go
func TestYourFeatureIntegration(t *testing.T) {
    // End-to-end test
}
```

### 3. Benchmarks
For performance-critical code:
```go
func BenchmarkYourFeature(b *testing.B) {
    // Benchmark implementation
}
```

### Running Tests

```bash
# All tests
make test

# Specific package
go test ./pkg/storage/...

# With coverage
make test-coverage

# Benchmarks
go test -bench=. ./...
```

## 📝 Contribution Process

### 1. Small Changes (Bug Fixes, Documentation)

1. Fork the repository
2. Create a feature branch: `git checkout -b fix-description`
3. Make your changes
4. Add tests for your changes
5. Run all tests: `go test ./...`
6. Commit with clear message: `git commit -m "fix: clear description"`
7. Push to your fork: `git push origin fix-description`
8. Create a Pull Request

### 2. Large Changes (New Features, Architecture)

1. **Discuss First**: Open an issue or discussion
2. Wait for maintainer feedback
3. Follow the small changes process
4. Be prepared for multiple review rounds

## 💻 Code Standards

### Go Code Style

We follow standard Go conventions:

```go
// Package comment explains the purpose
package storage

// ExportedType is documented with godoc comments
type ExportedType struct {
    // ExportedField is documented
    ExportedField string
    
    privateField string
}

// Method does something and is documented
func (e *ExportedType) Method() error {
    // Implementation
    return nil
}
```

### Error Handling

Always handle errors explicitly:

```go
// Good
data, err := ReadFile(path)
if err != nil {
    return fmt.Errorf("reading file %s: %w", path, err)
}

// Bad
data, _ := ReadFile(path)  // Never ignore errors
```

### Testing Patterns

Use table-driven tests:

```go
tests := []struct {
    name    string
    input   string
    want    string
    wantErr bool
}{
    {"empty input", "", "", true},
    {"valid input", "test", "TEST", false},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        got, err := YourFunction(tt.input)
        if (err != nil) != tt.wantErr {
            t.Errorf("YourFunction() error = %v, wantErr %v", err, tt.wantErr)
        }
        if got != tt.want {
            t.Errorf("YourFunction() = %v, want %v", got, tt.want)
        }
    })
}
```

## 📊 Pull Request Guidelines

### PR Title Format

Use conventional commits:
- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation only
- `style:` Code style changes
- `refactor:` Code refactoring
- `test:` Test additions/changes
- `chore:` Maintenance tasks

### PR Description Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix (non-breaking change)
- [ ] New feature (non-breaking change)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update

## Testing
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] New tests added
- [ ] Coverage maintained or improved

## Checklist
- [ ] My code follows the project style
- [ ] I have performed a self-review
- [ ] I have added tests
- [ ] I have updated documentation
- [ ] My changes generate no new warnings
```

## 🐛 Reporting Issues

### Bug Reports

Include:
1. Go version: `go version`
2. OS and architecture
3. Steps to reproduce
4. Expected behavior
5. Actual behavior
6. Error messages/logs

### Feature Requests

Include:
1. Use case description
2. Proposed solution
3. Alternative solutions considered
4. Impact on existing features

## 🤝 Code Review Process

1. **Automated Checks**: CI must pass
2. **Test Coverage**: Must maintain or improve
3. **Documentation**: Updates for API changes
4. **Performance**: No significant regressions
5. **Security**: No new vulnerabilities

## 🚀 Release Process

We follow semantic versioning during alpha:
- `0.0.x` - Patch releases (bug fixes)
- `0.x.0` - Minor releases (features, possible breaking changes)
- `1.0.0` - First stable release (future)

## 📚 Learning Resources

- [Go Documentation](https://golang.org/doc/)
- [Git Internals](https://git-scm.com/book/en/v2/Git-Internals-Plumbing-and-Porcelain)
- [Test-Driven Development](https://github.com/golang/go/wiki/TestComments)

## 🎯 Current Priorities

Check our [project board](https://github.com/Caia-Tech/govc/projects) for:
- High-priority issues
- Good first issues
- Help wanted tasks

## 💬 Communication

- **Issues**: Bug reports and feature requests
- **Discussions**: Architecture and design decisions
- **Pull Requests**: Code contributions

## 🙏 Recognition

Contributors are recognized in:
- [CHANGELOG.md](CHANGELOG.md)
- Release notes
- Project documentation

Thank you for helping make govc better! Your contributions, no matter how small, are valued and appreciated.

---

**Remember**: The goal is to build something innovative together. Don't be afraid to experiment, but always test your experiments thoroughly!