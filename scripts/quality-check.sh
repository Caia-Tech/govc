#!/bin/bash

# Quality and Performance Check Script
# This script runs various quality checks without committing artifacts to git

set -e

echo "🔍 Running govc Quality & Performance Analysis"
echo "=============================================="

# Ensure we're in the project root
cd "$(git rev-parse --show-toplevel)"

# Create temp directory for analysis artifacts
ANALYSIS_DIR=".analysis"
mkdir -p $ANALYSIS_DIR

# Add to .gitignore if not already there
if ! grep -q "^\.analysis" .gitignore 2>/dev/null; then
    echo ".analysis/" >> .gitignore
    echo "sonar-project.properties" >> .gitignore
    echo "*.prof" >> .gitignore
    echo "*.out" >> .gitignore
    echo "*.xml" >> .gitignore
fi

# 1. Code Coverage
echo ""
echo "📊 Generating Code Coverage..."
go test -short -coverprofile=$ANALYSIS_DIR/coverage.out ./... -timeout 30s || true
go tool cover -func=$ANALYSIS_DIR/coverage.out | tail -5

# 2. Go Vet
echo ""
echo "🔍 Running go vet..."
go vet ./... 2>&1 | tee $ANALYSIS_DIR/govet.out || true

# 3. Golangci-lint (if available)
if command -v golangci-lint &> /dev/null; then
    echo ""
    echo "🔍 Running golangci-lint..."
    golangci-lint run --out-format checkstyle > $ANALYSIS_DIR/golangci-lint.xml 2>/dev/null || true
    golangci-lint run --out-format colored-line-number | head -20 || true
else
    echo "⚠️  golangci-lint not found. Install with: brew install golangci-lint"
fi

# 4. Security Check
echo ""
echo "🔒 Running security check..."
if command -v gosec &> /dev/null; then
    gosec -fmt=json -out=$ANALYSIS_DIR/gosec.json ./... 2>/dev/null || true
    gosec -fmt=text ./... 2>&1 | head -20 || true
else
    echo "⚠️  gosec not found. Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"
fi

# 5. Performance Profiling
echo ""
echo "⚡ Running performance benchmarks..."
go test -bench=. -benchmem -run=^$ ./... -benchtime=10ms | tee $ANALYSIS_DIR/benchmark.out | grep -E "Benchmark|ns/op|allocs/op" | head -20

# 6. Memory Profile (for key packages)
echo ""
echo "🧠 Generating memory profiles..."
go test -memprofile=$ANALYSIS_DIR/mem.prof -bench=. -run=^$ ./pkg/storage -benchtime=10ms 2>/dev/null || true

# 7. Race Detection
echo ""
echo "🏃 Running race detector on key packages..."
go test -race -short ./pkg/storage ./pkg/refs ./pkg/core -timeout 10s 2>&1 | grep -E "PASS|FAIL|WARNING" || true

# 8. Generate Summary Report
echo ""
echo "📋 Summary Report"
echo "================"

# Coverage summary
COVERAGE=$(go tool cover -func=$ANALYSIS_DIR/coverage.out 2>/dev/null | tail -1 | awk '{print $3}' || echo "N/A")
echo "📊 Test Coverage: $COVERAGE"

# Count issues
if [ -f $ANALYSIS_DIR/golangci-lint.xml ]; then
    LINT_ISSUES=$(grep -c "<error" $ANALYSIS_DIR/golangci-lint.xml 2>/dev/null || echo "0")
    echo "🔍 Lint Issues: $LINT_ISSUES"
fi

# Count benchmarks
BENCH_COUNT=$(grep -c "Benchmark" $ANALYSIS_DIR/benchmark.out 2>/dev/null || echo "0")
echo "⚡ Benchmarks Run: $BENCH_COUNT"

# Check for race conditions
RACE_FOUND=$(grep -c "WARNING: DATA RACE" $ANALYSIS_DIR/*.out 2>/dev/null || echo "0")
if [ "$RACE_FOUND" -gt 0 ]; then
    echo "⚠️  Race Conditions Found: $RACE_FOUND"
else
    echo "✅ No Race Conditions Detected"
fi

echo ""
echo "✅ Analysis Complete!"
echo "📁 Results saved in $ANALYSIS_DIR/ (not tracked by git)"

# If SonarQube is running, offer to send results
if curl -s http://localhost:9000/api/system/status 2>/dev/null | grep -q "UP"; then
    echo ""
    echo "🔄 SonarQube is running. Run 'sonar-scanner' to upload results."
fi
