.PHONY: all build test bench clean install lint

# Default target
all: build

# Build the binary
build:
	go build -o govc ./cmd/govc

# Install globally
install:
	go install ./cmd/govc

# Run all tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -cover -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run benchmarks
bench:
	go test -bench=. -benchmem ./...

# Run memory-first specific benchmarks
bench-memory:
	go test -bench=Memory -benchmem ./...
	go test -bench=Parallel -benchmem ./...

# Clean build artifacts
clean:
	rm -f govc
	rm -f coverage.out coverage.html

# Run linter (requires golangci-lint)
lint:
	golangci-lint run ./...

# Format code
fmt:
	go fmt ./...

# Run tests with race detector
test-race:
	go test -race ./...

# Quick test - only run short tests
test-quick:
	go test -short ./...

# Test parallel reality features specifically
test-parallel:
	go test -v -run Parallel ./...

# Test memory-first benefits
test-memory-first:
	go test -v -run MemoryFirst ./...

# Generate test coverage report
coverage-report:
	@echo "Running tests with coverage..."
	@go test -coverprofile=coverage.out ./...
	@echo "\nCoverage by package:"
	@go tool cover -func=coverage.out
	@echo "\nGenerating HTML report..."
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Development mode - watch for changes and run tests
dev:
	@echo "Watching for changes..."
	@while true; do \
		inotifywait -e modify -r . --exclude .git 2>/dev/null || sleep 2; \
		clear; \
		echo "Running tests..."; \
		go test -short ./...; \
	done