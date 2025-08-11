#!/bin/bash

# Performance Regression Testing Script
# Automatically detects performance regressions in govc

set -e

echo "🚀 govc Performance Regression Testing"
echo "======================================"
echo ""

# Configuration
BASELINE_FILE=".performance/baseline.json"
RESULTS_FILE=".performance/results.json"
REGRESSION_THRESHOLD=10 # Percentage threshold for regression detection

# Create performance directory
mkdir -p .performance

# Function to run benchmarks and capture results
run_benchmarks() {
    local output_file=$1
    local test_name=$2
    
    echo "📊 Running $test_name benchmarks..."
    
    # Run Go benchmarks
    go test -bench=. -benchmem -benchtime=10s -json ./... 2>/dev/null | \
        jq -s '[.[] | select(.Action == "output" and .Output | contains("Benchmark"))] | 
        map({
            name: (.Output | split("\t")[0] | ltrimstr("Benchmark")),
            ns_op: (.Output | capture("(?<ns>[0-9.]+) ns/op") | .ns | tonumber),
            bytes_op: (.Output | capture("(?<bytes>[0-9]+) B/op") | .bytes | tonumber),
            allocs_op: (.Output | capture("(?<allocs>[0-9]+) allocs/op") | .allocs | tonumber)
        }) | map(select(.name != null))' > "$output_file" 2>/dev/null || true
}

# Function to compare results
compare_results() {
    local baseline=$1
    local current=$2
    
    echo ""
    echo "📈 Comparing performance..."
    echo ""
    
    # Use jq to compare results
    jq -r --slurpfile current "$current" '
        . as $baseline |
        $current[0] as $curr |
        
        # Create lookup maps
        ($baseline | map({(.name): .}) | add) as $base_map |
        ($curr | map({(.name): .}) | add) as $curr_map |
        
        # Compare each benchmark
        $curr | map(
            .name as $name |
            if $base_map[$name] then
                {
                    name: $name,
                    ns_op: {
                        baseline: $base_map[$name].ns_op,
                        current: .ns_op,
                        change: (((.ns_op - $base_map[$name].ns_op) / $base_map[$name].ns_op) * 100)
                    },
                    bytes_op: {
                        baseline: ($base_map[$name].bytes_op // 0),
                        current: (.bytes_op // 0),
                        change: if $base_map[$name].bytes_op then
                            ((((.bytes_op // 0) - $base_map[$name].bytes_op) / $base_map[$name].bytes_op) * 100)
                        else 0 end
                    },
                    allocs_op: {
                        baseline: ($base_map[$name].allocs_op // 0),
                        current: (.allocs_op // 0),
                        change: if $base_map[$name].allocs_op then
                            ((((.allocs_op // 0) - $base_map[$name].allocs_op) / $base_map[$name].allocs_op) * 100)
                        else 0 end
                    }
                }
            else
                {
                    name: $name,
                    status: "NEW"
                }
            end
        ) |
        
        # Format output
        map(
            if .status == "NEW" then
                "🆕 \(.name): NEW BENCHMARK"
            else
                (if .ns_op.change > 10 then "❌" 
                elif .ns_op.change > 5 then "⚠️"
                elif .ns_op.change < -5 then "✅"
                else "➡️" end) as $icon |
                
                "\($icon) \(.name):\n" +
                "    Time:   \(.ns_op.baseline | tostring) ns → \(.ns_op.current | tostring) ns (\(.ns_op.change | tostring | .[0:5])%)\n" +
                "    Memory: \(.bytes_op.baseline | tostring) B → \(.bytes_op.current | tostring) B (\(.bytes_op.change | tostring | .[0:5])%)\n" +
                "    Allocs: \(.allocs_op.baseline | tostring) → \(.allocs_op.current | tostring) (\(.allocs_op.change | tostring | .[0:5])%)"
            end
        ) | .[]
    ' "$baseline" || echo "No baseline found"
}

# Function to check for regressions
check_regressions() {
    local baseline=$1
    local current=$2
    local threshold=$3
    
    # Count regressions
    local regressions=$(jq -r --slurpfile current "$current" --arg threshold "$threshold" '
        . as $baseline |
        $current[0] as $curr |
        
        ($baseline | map({(.name): .}) | add) as $base_map |
        ($curr | map({(.name): .}) | add) as $curr_map |
        
        $curr | map(
            .name as $name |
            if $base_map[$name] then
                if (((.ns_op - $base_map[$name].ns_op) / $base_map[$name].ns_op) * 100) > ($threshold | tonumber) then
                    1
                else
                    0
                end
            else
                0
            end
        ) | add
    ' "$baseline" 2>/dev/null || echo "0")
    
    echo "$regressions"
}

# Function to generate HTML report
generate_html_report() {
    local baseline=$1
    local current=$2
    local output=".performance/report.html"
    
    cat > "$output" <<EOF
<!DOCTYPE html>
<html>
<head>
    <title>Performance Regression Report</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            margin: 20px;
            background: #f5f5f5;
        }
        h1 { color: #333; }
        .summary {
            background: white;
            padding: 20px;
            border-radius: 8px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        table {
            width: 100%;
            background: white;
            border-collapse: collapse;
            border-radius: 8px;
            overflow: hidden;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        th {
            background: #4CAF50;
            color: white;
            padding: 12px;
            text-align: left;
        }
        td {
            padding: 12px;
            border-bottom: 1px solid #ddd;
        }
        .regression { background: #ffebee; }
        .improvement { background: #e8f5e9; }
        .neutral { background: white; }
        .change-positive { color: #d32f2f; }
        .change-negative { color: #388e3c; }
        .change-neutral { color: #757575; }
    </style>
</head>
<body>
    <h1>🚀 govc Performance Regression Report</h1>
    <div class="summary">
        <h2>Summary</h2>
        <p>Generated: $(date)</p>
        <p>Threshold: ${REGRESSION_THRESHOLD}%</p>
    </div>
    <table>
        <thead>
            <tr>
                <th>Benchmark</th>
                <th>Baseline (ns/op)</th>
                <th>Current (ns/op)</th>
                <th>Change (%)</th>
                <th>Memory (B/op)</th>
                <th>Allocations</th>
                <th>Status</th>
            </tr>
        </thead>
        <tbody>
EOF

    # Generate table rows
    jq -r --slurpfile current "$current" '
        . as $baseline |
        $current[0] as $curr |
        
        ($baseline | map({(.name): .}) | add) as $base_map |
        ($curr | map({(.name): .}) | add) as $curr_map |
        
        $curr | map(
            .name as $name |
            if $base_map[$name] then
                {
                    name: $name,
                    baseline_ns: $base_map[$name].ns_op,
                    current_ns: .ns_op,
                    change: (((.ns_op - $base_map[$name].ns_op) / $base_map[$name].ns_op) * 100),
                    current_bytes: (.bytes_op // 0),
                    current_allocs: (.allocs_op // 0)
                }
            else
                {
                    name: $name,
                    status: "NEW"
                }
            end
        ) |
        map(
            if .status == "NEW" then
                "<tr class=\"neutral\"><td>\(.name)</td><td>-</td><td>-</td><td>NEW</td><td>-</td><td>-</td><td>🆕 New</td></tr>"
            else
                (if .change > 10 then "regression"
                elif .change < -5 then "improvement"
                else "neutral" end) as $class |
                
                (if .change > 10 then "change-positive"
                elif .change < -5 then "change-negative"
                else "change-neutral" end) as $change_class |
                
                (if .change > 10 then "❌ Regression"
                elif .change < -5 then "✅ Improvement"
                else "➡️ No Change" end) as $status |
                
                "<tr class=\"\($class)\">
                    <td>\(.name)</td>
                    <td>\(.baseline_ns)</td>
                    <td>\(.current_ns)</td>
                    <td class=\"\($change_class)\">\(.change | tostring | .[0:6])%</td>
                    <td>\(.current_bytes)</td>
                    <td>\(.current_allocs)</td>
                    <td>\($status)</td>
                </tr>"
            end
        ) | .[]
    ' "$baseline" >> "$output" 2>/dev/null || echo "<tr><td colspan=\"7\">No baseline data</td></tr>" >> "$output"

    cat >> "$output" <<EOF
        </tbody>
    </table>
</body>
</html>
EOF

    echo "📄 HTML report generated: $output"
}

# Main execution
main() {
    case "${1:-test}" in
        baseline)
            echo "📐 Creating performance baseline..."
            run_benchmarks "$BASELINE_FILE" "baseline"
            echo "✅ Baseline created: $BASELINE_FILE"
            ;;
            
        test)
            if [ ! -f "$BASELINE_FILE" ]; then
                echo "⚠️  No baseline found. Creating one now..."
                run_benchmarks "$BASELINE_FILE" "baseline"
                echo ""
            fi
            
            # Run current benchmarks
            run_benchmarks "$RESULTS_FILE" "current"
            
            # Compare results
            compare_results "$BASELINE_FILE" "$RESULTS_FILE"
            
            # Check for regressions
            regression_count=$(check_regressions "$BASELINE_FILE" "$RESULTS_FILE" "$REGRESSION_THRESHOLD")
            
            echo ""
            echo "======================================"
            
            if [ "$regression_count" -gt 0 ]; then
                echo "❌ Found $regression_count performance regression(s) exceeding ${REGRESSION_THRESHOLD}% threshold"
                generate_html_report "$BASELINE_FILE" "$RESULTS_FILE"
                exit 1
            else
                echo "✅ No performance regressions detected"
                generate_html_report "$BASELINE_FILE" "$RESULTS_FILE"
            fi
            ;;
            
        update)
            echo "🔄 Updating baseline with current performance..."
            run_benchmarks "$BASELINE_FILE" "new baseline"
            echo "✅ Baseline updated"
            ;;
            
        report)
            if [ ! -f "$BASELINE_FILE" ] || [ ! -f "$RESULTS_FILE" ]; then
                echo "❌ Missing baseline or results file"
                exit 1
            fi
            generate_html_report "$BASELINE_FILE" "$RESULTS_FILE"
            ;;
            
        clean)
            echo "🧹 Cleaning performance data..."
            rm -rf .performance
            echo "✅ Cleaned"
            ;;
            
        *)
            echo "Usage: $0 {baseline|test|update|report|clean}"
            echo ""
            echo "  baseline - Create performance baseline"
            echo "  test     - Run tests and check for regressions (default)"
            echo "  update   - Update baseline with current performance"
            echo "  report   - Generate HTML report"
            echo "  clean    - Remove all performance data"
            exit 1
            ;;
    esac
}

# Check for required tools
if ! command -v jq &> /dev/null; then
    echo "❌ jq is required but not installed. Install with: brew install jq"
    exit 1
fi

# Run main function
main "$@"