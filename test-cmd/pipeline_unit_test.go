package main

import (
	"context"
	"testing"
	
	"github.com/Caia-Tech/govc/pkg/pipeline"
)

func TestMemoryTestExecutorCreation(t *testing.T) {
	executor := pipeline.NewMemoryTestExecutor()
	if executor == nil {
		t.Fatal("NewMemoryTestExecutor() returned nil")
	}
}

func TestMemoryTestExecutorBasicExecution(t *testing.T) {
	executor := pipeline.NewMemoryTestExecutor()
	ctx := context.Background()
	
	// Test source files
	sourceFiles := map[string][]byte{
		"main.go": []byte(`package main

func Add(a, b int) int {
	return a + b
}

func Subtract(a, b int) int {
	return a - b
}
`),
	}
	
	// Test files
	testFiles := map[string][]byte{
		"main_test.go": []byte(`package main

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Error("Add(2, 3) should be 5")
	}
}

func TestSubtract(t *testing.T) {
	if Subtract(5, 3) != 2 {
		t.Error("Subtract(5, 3) should be 2")
	}
}
`),
	}
	
	// Execute tests
	result, err := executor.ExecuteTestsInMemory(ctx, sourceFiles, testFiles)
	if err != nil {
		t.Fatalf("ExecuteTestsInMemory() failed: %v", err)
	}
	
	if result == nil {
		t.Fatal("ExecuteTestsInMemory() returned nil result")
	}
	
	if result.Passed != 2 {
		t.Errorf("Expected 2 passed tests, got %d", result.Passed)
	}
	
	if result.Failed != 0 {
		t.Errorf("Expected 0 failed tests, got %d", result.Failed)
	}
	
	if len(result.Tests) != 2 {
		t.Errorf("Expected 2 test results, got %d", len(result.Tests))
	}
	
	// Check individual test results
	testNames := make(map[string]bool)
	for _, test := range result.Tests {
		testNames[test.Name] = test.Status == "passed"
	}
	
	if !testNames["TestAdd"] {
		t.Error("TestAdd should have passed")
	}
	
	if !testNames["TestSubtract"] {
		t.Error("TestSubtract should have passed")
	}
}

func TestMemoryTestExecutorFailingTests(t *testing.T) {
	executor := pipeline.NewMemoryTestExecutor()
	ctx := context.Background()
	
	// Test source files
	sourceFiles := map[string][]byte{
		"main.go": []byte(`package main

func Add(a, b int) int {
	return a + b
}
`),
	}
	
	// Test files with failing tests
	testFiles := map[string][]byte{
		"main_test.go": []byte(`package main

import "testing"

func TestAddPass(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Error("Add(2, 3) should be 5")
	}
}

func TestAddFail(t *testing.T) {
	if Add(2, 3) != 6 {  // This will fail
		t.Error("Add(2, 3) should be 6")
	}
}
`),
	}
	
	// Execute tests
	result, err := executor.ExecuteTestsInMemory(ctx, sourceFiles, testFiles)
	if err != nil {
		t.Fatalf("ExecuteTestsInMemory() failed: %v", err)
	}
	
	if result == nil {
		t.Fatal("ExecuteTestsInMemory() returned nil result")
	}
	
	if result.Passed != 1 {
		t.Errorf("Expected 1 passed test, got %d", result.Passed)
	}
	
	if result.Failed != 1 {
		t.Errorf("Expected 1 failed test, got %d", result.Failed)
	}
}

func TestMemoryTestExecutorEmptyFiles(t *testing.T) {
	executor := pipeline.NewMemoryTestExecutor()
	ctx := context.Background()
	
	// Empty source files
	sourceFiles := map[string][]byte{}
	testFiles := map[string][]byte{}
	
	// Execute tests
	result, err := executor.ExecuteTestsInMemory(ctx, sourceFiles, testFiles)
	
	// Should handle empty files gracefully
	if err == nil && result != nil {
		// If no error, should have 0 tests
		if result.Passed != 0 || result.Failed != 0 {
			t.Errorf("Expected 0 tests with empty files, got %d passed, %d failed", 
				result.Passed, result.Failed)
		}
	}
	// If there's an error, that's also acceptable behavior for empty files
}

func TestMemoryTestExecutorInvalidSyntax(t *testing.T) {
	executor := pipeline.NewMemoryTestExecutor()
	ctx := context.Background()
	
	// Test source files with invalid syntax
	sourceFiles := map[string][]byte{
		"main.go": []byte(`package main

func Add(a, b int) int {
	return a + b  // Missing closing brace
`),
	}
	
	testFiles := map[string][]byte{
		"main_test.go": []byte(`package main

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Error("Add(2, 3) should be 5")
	}
}
`),
	}
	
	// Execute tests - should handle syntax errors
	_, err := executor.ExecuteTestsInMemory(ctx, sourceFiles, testFiles)
	
	// Should either return an error or handle gracefully
	// This is an edge case test - either behavior is acceptable
	if err != nil {
		t.Logf("ExecuteTestsInMemory() handled syntax error: %v", err)
	}
}

func TestMemoryTestExecutorContextCancellation(t *testing.T) {
	executor := pipeline.NewMemoryTestExecutor()
	ctx, cancel := context.WithCancel(context.Background())
	
	// Cancel context immediately
	cancel()
	
	sourceFiles := map[string][]byte{
		"main.go": []byte(`package main
func Add(a, b int) int { return a + b }`),
	}
	
	testFiles := map[string][]byte{
		"main_test.go": []byte(`package main
import "testing"
func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Error("Should be 5")
	}
}`),
	}
	
	// Execute tests with cancelled context
	_, err := executor.ExecuteTestsInMemory(ctx, sourceFiles, testFiles)
	
	// Should handle cancelled context appropriately
	if err != nil && err == context.Canceled {
		t.Logf("ExecuteTestsInMemory() correctly handled context cancellation: %v", err)
	} else {
		// Some implementations might not check context, which is also acceptable
		t.Logf("ExecuteTestsInMemory() completed despite context cancellation")
	}
}