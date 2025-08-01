#!/bin/bash

echo "Running govc test suite..."
echo "=========================="

# Test each package individually to get better error reporting
packages=(
    "."
    "./ai/embeddings"
    "./ai/llm"
    "./ai/search"
    "./pkg/object"
    "./pkg/refs"
    "./pkg/storage"
)

total_coverage=0
tested_packages=0

for pkg in "${packages[@]}"; do
    echo -e "\n📦 Testing $pkg..."
    if go test -cover -timeout 30s "$pkg" 2>&1 | tee test_output.tmp; then
        coverage=$(grep "coverage:" test_output.tmp | awk -F'coverage: ' '{print $2}' | awk '{print $1}' | sed 's/%//')
        if [ ! -z "$coverage" ]; then
            echo "✅ PASS - Coverage: $coverage%"
            # Extract just the number
            coverage_num=$(echo "$coverage" | grep -oE '[0-9]+\.?[0-9]*' | head -1)
            if [ ! -z "$coverage_num" ]; then
                total_coverage=$(echo "$total_coverage + $coverage_num" | bc -l)
                tested_packages=$((tested_packages + 1))
            fi
        else
            echo "✅ PASS - No coverage data"
        fi
    else
        echo "❌ FAIL"
    fi
done

rm -f test_output.tmp

echo -e "\n=========================="
echo "Summary:"
echo "=========================="
if [ $tested_packages -gt 0 ]; then
    avg_coverage=$(echo "scale=1; $total_coverage / $tested_packages" | bc)
    echo "Average coverage: $avg_coverage%"
fi
echo "Tested packages: $tested_packages"

# Run specific test files that should work
echo -e "\n📋 Running specific test files..."
test_files=(
    "govc_test.go"
    "basic_test.go"
    "export_test.go"
    "ai_features_test.go"
)

for file in "${test_files[@]}"; do
    if [ -f "$file" ]; then
        echo -e "\nTesting $file..."
        go test -v "./$file" 2>&1 | grep -E "(PASS|FAIL|---)" | head -10
    fi
done