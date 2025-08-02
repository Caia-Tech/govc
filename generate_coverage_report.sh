#!/bin/bash

echo "=== Test Coverage Report ==="
echo "Generated on: $(date)"
echo ""

# Test each major package and collect coverage
packages=(
    "./pkg/object"
    "./pkg/refs"
    "./pkg/storage"
    "./pkg/workspace"
    "./api"
    "./auth"
    "./cluster"
    "./container"
    "./pool"
    "./importexport"
    "."
)

total_coverage=0
count=0

echo "Package Coverage Summary:"
echo "========================"

for pkg in "${packages[@]}"; do
    if [[ -d "${pkg#./}" ]] || [[ "$pkg" == "." ]]; then
        echo -n "Testing $pkg... "
        
        # Run tests with coverage
        output=$(go test -cover "$pkg" 2>&1)
        
        if echo "$output" | grep -q "coverage:"; then
            coverage=$(echo "$output" | grep -o 'coverage: [0-9.]*%' | grep -o '[0-9.]*')
            echo "coverage: ${coverage}%"
            
            # Add to total for average calculation
            if [[ -n "$coverage" ]] && [[ "$coverage" != "0.0" ]]; then
                total_coverage=$(echo "$total_coverage + $coverage" | bc)
                count=$((count + 1))
            fi
        else
            echo "FAILED or no tests"
        fi
    fi
done

echo ""
echo "========================"
if [[ $count -gt 0 ]]; then
    average=$(echo "scale=1; $total_coverage / $count" | bc)
    echo "Average Coverage: ${average}%"
else
    echo "No test coverage data available"
fi

# Generate detailed coverage profile
echo ""
echo "Generating detailed coverage profile..."
go test -coverprofile=full_coverage.out -covermode=atomic ./... 2>/dev/null || true

if [[ -f full_coverage.out ]]; then
    echo ""
    echo "Top 10 least covered files:"
    echo "=========================="
    go tool cover -func=full_coverage.out | grep -v "100.0%" | sort -k3 -n | head -10
    
    echo ""
    echo "Coverage by directory:"
    echo "===================="
    go tool cover -func=full_coverage.out | grep -E "^github.com/caiatech/govc/[^/]+/" | awk -F'/' '{print $4}' | sort | uniq -c | sort -nr
fi

echo ""
echo "Report complete. HTML report: coverage.html"