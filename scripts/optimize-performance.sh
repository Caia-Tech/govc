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
