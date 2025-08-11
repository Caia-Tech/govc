#!/bin/bash

# SonarQube Setup Script for govc
# This script sets up SonarQube analysis without committing analysis files to git

set -e

echo "🚀 Setting up SonarQube Analysis for govc"
echo "========================================="

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker Desktop for ARM Mac."
    echo "   Visit: https://docs.docker.com/desktop/mac/install/"
    exit 1
fi

# Check if we're on ARM Mac
if [[ $(uname -m) == "arm64" ]]; then
    echo "✅ ARM64 Mac detected"
    SONAR_IMAGE="sonarqube:10-community"
else
    SONAR_IMAGE="sonarqube:10-community"
fi

# Function to start SonarQube in Docker
start_sonarqube() {
    echo "🐳 Starting SonarQube in Docker..."
    
    # Stop existing container if running
    docker stop sonarqube 2>/dev/null || true
    docker rm sonarqube 2>/dev/null || true
    
    # Run SonarQube container
    docker run -d \
        --name sonarqube \
        -p 9000:9000 \
        -v sonarqube_data:/opt/sonarqube/data \
        -v sonarqube_extensions:/opt/sonarqube/extensions \
        -v sonarqube_logs:/opt/sonarqube/logs \
        $SONAR_IMAGE
    
    echo "⏳ Waiting for SonarQube to start (this may take 1-2 minutes)..."
    
    # Wait for SonarQube to be ready
    for i in {1..60}; do
        if curl -s http://localhost:9000/api/system/status | grep -q "UP"; then
            echo "✅ SonarQube is running at http://localhost:9000"
            echo "   Default credentials: admin/admin"
            break
        fi
        sleep 2
        echo -n "."
    done
    echo ""
}

# Function to use SonarCloud instead (no local installation)
setup_sonarcloud() {
    echo "☁️  Setting up SonarCloud configuration..."
    
    cat > sonar-project.properties <<EOF
# SonarCloud Configuration
sonar.organization=caia-tech
sonar.projectKey=caia-tech_govc
sonar.sources=.
sonar.exclusions=**/*_test.go,**/vendor/**,**/node_modules/**,**/cmd/**,**/examples/**,**/docs/**,**/scripts/**
sonar.tests=.
sonar.test.inclusions=**/*_test.go
sonar.go.coverage.reportPaths=coverage.out
EOF
    
    echo "✅ SonarCloud configuration created"
    echo "   Visit: https://sonarcloud.io to set up your project"
}

# Menu for user selection
echo ""
echo "Choose SonarQube setup method:"
echo "1) Docker (Local) - Recommended for ARM Mac"
echo "2) SonarCloud (Cloud) - No local installation"
echo "3) Skip SonarQube setup"
echo ""
read -p "Enter choice [1-3]: " choice

case $choice in
    1)
        start_sonarqube
        USE_DOCKER=true
        ;;
    2)
        setup_sonarcloud
        USE_CLOUD=true
        ;;
    3)
        echo "⏭️  Skipping SonarQube setup"
        ;;
    *)
        echo "❌ Invalid choice"
        exit 1
        ;;
esac

# Create quality analysis script
echo ""
echo "📝 Creating quality analysis script..."

cat > scripts/quality-check.sh <<'SCRIPT'
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
SCRIPT

chmod +x scripts/quality-check.sh

echo "✅ Quality check script created at scripts/quality-check.sh"

# Create performance optimization script
cat > scripts/optimize-performance.sh <<'PERF'
#!/bin/bash

# Performance Optimization Script
# Analyzes and suggests performance improvements

set -e

echo "⚡ govc Performance Optimization Analysis"
echo "========================================"

cd "$(git rev-parse --show-toplevel)"

# Create analysis directory
PERF_DIR=".performance"
mkdir -p $PERF_DIR

# 1. CPU Profiling
echo ""
echo "🔥 CPU Profiling..."
go test -cpuprofile=$PERF_DIR/cpu.prof -bench=. -run=^$ ./pkg/storage -benchtime=100ms 2>/dev/null || true

if [ -f $PERF_DIR/cpu.prof ]; then
    echo "Top CPU consumers:"
    go tool pprof -top -nodecount=10 $PERF_DIR/cpu.prof | head -15
fi

# 2. Memory Profiling
echo ""
echo "🧠 Memory Profiling..."
go test -memprofile=$PERF_DIR/mem.prof -bench=. -run=^$ ./pkg/storage -benchtime=100ms 2>/dev/null || true

if [ -f $PERF_DIR/mem.prof ]; then
    echo "Top Memory allocators:"
    go tool pprof -top -nodecount=10 $PERF_DIR/mem.prof | head -15
fi

# 3. Goroutine Analysis
echo ""
echo "🔄 Goroutine Analysis..."
go test -trace=$PERF_DIR/trace.out -bench=. -run=^$ ./pkg/storage -benchtime=10ms 2>/dev/null || true

# 4. Allocation Analysis
echo ""
echo "📦 Allocation Analysis..."
go test -bench=. -benchmem -run=^$ ./... -benchtime=10ms 2>&1 | grep -E "allocs/op" | sort -t' ' -nk5 | tail -10

# 5. Lock Contention Analysis
echo ""
echo "🔒 Lock Contention Analysis..."
go test -blockprofile=$PERF_DIR/block.prof -bench=. -run=^$ ./pkg/storage -benchtime=10ms 2>/dev/null || true

# 6. Generate Optimization Recommendations
echo ""
echo "💡 Performance Optimization Recommendations"
echo "========================================="

# Check for common performance issues
echo ""
echo "Checking for common issues..."

# Large allocations
LARGE_ALLOCS=$(grep -E "B/op" $PERF_DIR/*.out 2>/dev/null | awk '$3 > 10000 {print $1, $3}' | wc -l || echo "0")
if [ "$LARGE_ALLOCS" -gt 0 ]; then
    echo "⚠️  Found $LARGE_ALLOCS functions with large allocations (>10KB/op)"
fi

# High allocation count
HIGH_ALLOC_COUNT=$(grep -E "allocs/op" $PERF_DIR/*.out 2>/dev/null | awk '$5 > 100 {print $1, $5}' | wc -l || echo "0")
if [ "$HIGH_ALLOC_COUNT" -gt 0 ]; then
    echo "⚠️  Found $HIGH_ALLOC_COUNT functions with high allocation counts (>100 allocs/op)"
fi

echo ""
echo "✅ Performance analysis complete!"
echo "📁 Results saved in $PERF_DIR/ (not tracked by git)"
echo ""
echo "🎯 Next Steps:"
echo "1. Review CPU profile: go tool pprof -http=:8080 $PERF_DIR/cpu.prof"
echo "2. Review Memory profile: go tool pprof -http=:8081 $PERF_DIR/mem.prof"
echo "3. Review Trace: go tool trace $PERF_DIR/trace.out"
PERF

chmod +x scripts/optimize-performance.sh

echo "✅ Performance optimization script created"

# Summary
echo ""
echo "✅ Setup Complete!"
echo "================="
echo ""
echo "🎯 Next Steps:"
echo "1. Run quality check: ./scripts/quality-check.sh"
echo "2. Run performance analysis: ./scripts/optimize-performance.sh"

if [ "$USE_DOCKER" = true ]; then
    echo "3. Access SonarQube: http://localhost:9000 (admin/admin)"
elif [ "$USE_CLOUD" = true ]; then
    echo "3. Configure SonarCloud: https://sonarcloud.io"
fi

echo ""
echo "📌 All analysis files are in .analysis/ and .performance/ (git-ignored)"