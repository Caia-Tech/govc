#!/bin/bash

echo "=== govc Test Report ==="
echo "Date: $(date)"
echo ""

# Function to run tests for a package
test_package() {
    local pkg=$1
    local name=$2
    echo -n "Testing $name... "
    
    if output=$(go test "$pkg" -short 2>&1); then
        echo "✅ PASS"
        return 0
    else
        echo "❌ FAIL"
        # Show first few lines of error
        echo "$output" | grep -E "(FAIL|Error:|panic:)" | head -3 | sed 's/^/  /'
        return 1
    fi
}

# Track results
passed=0
failed=0

echo "Core Packages:"
echo "============="
test_package "./pkg/object" "pkg/object" && ((passed++)) || ((failed++))
test_package "./pkg/refs" "pkg/refs" && ((passed++)) || ((failed++))
test_package "./pkg/storage" "pkg/storage" && ((passed++)) || ((failed++))
test_package "./pkg/workspace" "pkg/workspace" && ((passed++)) || ((failed++))

echo ""
echo "Main Packages:"
echo "============="
test_package "." "govc (main)" && ((passed++)) || ((failed++))
test_package "./auth" "auth" && ((passed++)) || ((failed++))
test_package "./pool" "pool" && ((passed++)) || ((failed++))
test_package "./container" "container" && ((passed++)) || ((failed++))
test_package "./importexport" "importexport" && ((passed++)) || ((failed++))

echo ""
echo "API & Integration:"
echo "=================="
test_package "./api" "api" && ((passed++)) || ((failed++))
test_package "./cluster" "cluster" && ((passed++)) || ((failed++))

echo ""
echo "Summary:"
echo "========"
total=$((passed + failed))
echo "Total packages tested: $total"
echo "Passed: $passed ($(( (passed * 100) / total ))%)"
echo "Failed: $failed ($(( (failed * 100) / total ))%)"

if [ $failed -eq 0 ]; then
    echo ""
    echo "🎉 All tests passed!"
    exit 0
else
    echo ""
    echo "⚠️  Some tests failed. Please review the errors above."
    exit 1
fi