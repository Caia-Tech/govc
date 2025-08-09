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

# Dashboard-specific targets
.PHONY: dashboard-test dashboard-bench dashboard-dev dashboard-build

# Test dashboard components
dashboard-test:
	@echo "Running dashboard tests..."
	go test -v ./web/...
	go test -v ./web/tests/...

# Run dashboard benchmarks
dashboard-bench:
	@echo "Running dashboard benchmarks..."
	go test -bench="BenchmarkDashboard" -benchmem ./web/...

# Dashboard development mode with live reload
dashboard-dev:
	@echo "Starting dashboard in development mode..."
	go build -o govc-server ./cmd/govc-server
	./govc-server --config config.example.yaml &
	@echo "Dashboard running at http://localhost:8080/dashboard"
	@echo "Press Ctrl+C to stop"
	@trap 'kill $$!' INT; wait

# Build dashboard with embedded assets
dashboard-build:
	@echo "Building dashboard server..."
	go build -tags embed -o govc-server ./cmd/govc-server
	@echo "Dashboard server built: ./govc-server"

# Run dashboard integration tests
dashboard-integration:
	@echo "Running dashboard integration tests..."
	go test -v -tags=integration ./web/tests/...

# Check dashboard health
dashboard-health:
	@echo "Checking dashboard health..."
	@curl -s http://localhost:8080/api/v1/dashboard/overview > /dev/null && echo "✅ Dashboard API is healthy" || echo "❌ Dashboard API is not responding"
	@curl -s http://localhost:8080/dashboard > /dev/null && echo "✅ Dashboard UI is healthy" || echo "❌ Dashboard UI is not responding"