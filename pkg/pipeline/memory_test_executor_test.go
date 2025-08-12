package pipeline

import (
	"context"
	"testing"
	"time"
)

func TestMemoryTestExecutor(t *testing.T) {
	// Test virtual file system
	t.Run("VirtualFileSystem", func(t *testing.T) {
		vfs := NewVirtualFileSystem()
		
		sourceFiles := map[string][]byte{
			"main.go": []byte(`
package main

func Add(a, b int) int {
	return a + b
}

func Multiply(a, b int) int {
	return a * b
}
`),
		}
		
		testFiles := map[string][]byte{
			"main_test.go": []byte(`
package main

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Error("Add failed")
	}
}

func TestMultiply(t *testing.T) {
	if Multiply(3, 4) != 12 {
		t.Error("Multiply failed")
	}
}

func TestFailingExample(t *testing.T) {
	t.Error("This test always fails")
}

func TestSkipExample(t *testing.T) {
	t.Skip("This test is skipped")
}
`),
		}
		
		err := vfs.LoadFiles(sourceFiles, testFiles)
		if err != nil {
			t.Fatalf("Failed to load files: %v", err)
		}
		
		// Verify files are in memory
		content, err := vfs.ReadFile("main.go")
		if err != nil {
			t.Errorf("Failed to read main.go: %v", err)
		}
		if len(content) == 0 {
			t.Error("main.go content is empty")
		}
		
		// Verify test file
		testContent, err := vfs.ReadFile("main_test.go")
		if err != nil {
			t.Errorf("Failed to read main_test.go: %v", err)
		}
		if len(testContent) == 0 {
			t.Error("main_test.go content is empty")
		}
	})
	
	t.Run("MemoryCodeLoader", func(t *testing.T) {
		loader := NewMemoryCodeLoader()
		vfs := NewVirtualFileSystem()
		
		sourceFiles := map[string][]byte{
			"example.go": []byte(`package example

func Hello() string {
	return "Hello, World!"
}`),
			"example_test.go": []byte(`package example

import "testing"

func TestHello(t *testing.T) {
	if Hello() != "Hello, World!" {
		t.Error("Hello failed")
	}
}`),
		}
		
		vfs.LoadFiles(sourceFiles, nil)
		
		packages, err := loader.LoadFromMemory(vfs)
		if err != nil {
			t.Fatalf("Failed to load from memory: %v", err)
		}
		
		if len(packages) != 1 {
			t.Errorf("Expected 1 package, got %d", len(packages))
		}
		
		if _, exists := packages["example"]; !exists {
			t.Error("Package 'example' not found")
		}
	})
	
	t.Run("MemoryCompiler", func(t *testing.T) {
		compiler := NewMemoryCompiler()
		loader := NewMemoryCodeLoader()
		vfs := NewVirtualFileSystem()
		
		testFiles := map[string][]byte{
			"sample_test.go": []byte(`package sample

import "testing"

func TestOne(t *testing.T) {
	t.Log("Test one")
}

func TestTwo(t *testing.T) {
	t.Log("Test two")
}

func BenchmarkExample(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// Benchmark code
	}
}`),
		}
		
		vfs.LoadFiles(nil, testFiles)
		packages, _ := loader.LoadFromMemory(vfs)
		
		compiled, err := compiler.CompileTests(packages)
		if err != nil {
			t.Fatalf("Failed to compile tests: %v", err)
		}
		
		if len(compiled) == 0 {
			t.Error("No compiled code produced")
		}
		
		// Verify test functions were extracted
		for _, code := range compiled {
			if len(code.TestFuncs) < 2 {
				t.Errorf("Expected at least 2 test functions, got %d", len(code.TestFuncs))
			}
			
			// Check for specific test names
			foundTestOne := false
			foundTestTwo := false
			for _, fn := range code.TestFuncs {
				if fn.Name == "TestOne" {
					foundTestOne = true
				}
				if fn.Name == "TestTwo" {
					foundTestTwo = true
				}
			}
			
			if !foundTestOne {
				t.Error("TestOne not found in compiled code")
			}
			if !foundTestTwo {
				t.Error("TestTwo not found in compiled code")
			}
		}
	})
	
	t.Run("InMemoryT", func(t *testing.T) {
		// Test the in-memory testing.T implementation
		harness := NewTestHarness()
		inMemT := harness.NewTest("TestExample")
		
		// Test logging
		inMemT.Log("This is a log message")
		inMemT.Logf("Formatted: %d", 42)
		
		output := inMemT.Output()
		if output == "" {
			t.Error("Expected output from Log calls")
		}
		
		// Test failure
		inMemT.Error("This is an error")
		if !inMemT.Failed() {
			t.Error("Expected test to be marked as failed")
		}
		
		// Test status
		status := inMemT.Status()
		if status != "failed" {
			t.Errorf("Expected status 'failed', got '%s'", status)
		}
		
		// Test skip
		skippedT := harness.NewTest("TestSkipped")
		skippedT.skipped = true // Simulate skip
		if !skippedT.Skipped() {
			t.Error("Expected test to be marked as skipped")
		}
		if skippedT.Status() != "skipped" {
			t.Error("Expected status 'skipped'")
		}
	})
	
	t.Run("DirectMemoryTestExecution", func(t *testing.T) {
		dmte := &DirectMemoryTestExecution{}
		
		// Register test functions directly in memory
		dmte.RegisterTestFunction("TestPass", func(t *testing.T) {
			// This test passes
		})
		
		dmte.RegisterTestFunction("TestFail", func(t *testing.T) {
			t.Error("This test fails")
		})
		
		dmte.RegisterTestFunction("TestSkip", func(t *testing.T) {
			t.Skip("This test is skipped")
		})
		
		ctx := context.Background()
		result, err := dmte.ExecuteDirect(ctx)
		
		if err != nil {
			t.Fatalf("ExecuteDirect failed: %v", err)
		}
		
		if result.Passed != 1 {
			t.Errorf("Expected 1 passed test, got %d", result.Passed)
		}
		
		if result.Failed != 1 {
			t.Errorf("Expected 1 failed test, got %d", result.Failed)
		}
		
		if result.Skipped != 1 {
			t.Errorf("Expected 1 skipped test, got %d", result.Skipped)
		}
	})
	
	t.Run("FullMemoryExecution", func(t *testing.T) {
		executor := NewMemoryTestExecutor()
		
		sourceFiles := map[string][]byte{
			"calc.go": []byte(`package calc

func Square(n int) int {
	return n * n
}`),
		}
		
		testFiles := map[string][]byte{
			"calc_test.go": []byte(`package calc

import "testing"

func TestSquare(t *testing.T) {
	if Square(4) != 16 {
		t.Error("Square(4) should be 16")
	}
}

func TestSquareNegative(t *testing.T) {
	if Square(-3) != 9 {
		t.Error("Square(-3) should be 9")
	}
}`),
		}
		
		ctx := context.Background()
		result, err := executor.ExecuteTestsInMemory(ctx, sourceFiles, testFiles)
		
		if err != nil {
			t.Fatalf("ExecuteTestsInMemory failed: %v", err)
		}
		
		if result == nil {
			t.Fatal("Result is nil")
		}
		
		// The actual execution is simulated, but the infrastructure is in place
		t.Logf("Execution completed with %d tests", len(result.Tests))
	})
}

func TestMemoryHeapAndStack(t *testing.T) {
	t.Run("MemoryHeap", func(t *testing.T) {
		heap := NewMemoryHeap()
		
		// Simulate allocation
		ptr := uintptr(0x1000)
		heap.allocations[ptr] = &Allocation{
			Ptr:  ptr,
			Size: 1024,
			Type: "[]byte",
			Time: time.Now(),
		}
		heap.totalSize = 1024
		
		if len(heap.allocations) != 1 {
			t.Error("Expected 1 allocation")
		}
		
		if heap.totalSize != 1024 {
			t.Errorf("Expected total size 1024, got %d", heap.totalSize)
		}
	})
	
	t.Run("MemoryStack", func(t *testing.T) {
		stack := NewMemoryStack()
		
		// Push frame
		frame := StackFrame{
			Function: "TestFunction",
			File:     "test.go",
			Line:     42,
			Locals:   map[string]interface{}{"x": 10},
		}
		
		stack.frames = append(stack.frames, frame)
		stack.size++
		
		if stack.size != 1 {
			t.Errorf("Expected stack size 1, got %d", stack.size)
		}
		
		if len(stack.frames) != 1 {
			t.Error("Expected 1 frame")
		}
	})
}

func TestCompileMetadata(t *testing.T) {
	metadata := &CompileMetadata{
		Package:     "test",
		TestCount:   5,
		CompileTime: time.Millisecond * 100,
		MemoryUsage: 4096,
	}
	
	if metadata.Package != "test" {
		t.Errorf("Expected package 'test', got '%s'", metadata.Package)
	}
	
	if metadata.TestCount != 5 {
		t.Errorf("Expected 5 tests, got %d", metadata.TestCount)
	}
	
	if metadata.MemoryUsage != 4096 {
		t.Errorf("Expected memory usage 4096, got %d", metadata.MemoryUsage)
	}
}

func BenchmarkVirtualFileSystem(b *testing.B) {
	vfs := NewVirtualFileSystem()
	
	files := map[string][]byte{
		"file1.go": []byte("package main"),
		"file2.go": []byte("package main"),
		"file3.go": []byte("package main"),
	}
	
	vfs.LoadFiles(files, nil)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = vfs.ReadFile("file1.go")
	}
}

func BenchmarkMemoryCodeLoader(b *testing.B) {
	loader := NewMemoryCodeLoader()
	vfs := NewVirtualFileSystem()
	
	files := map[string][]byte{
		"test.go": []byte(`package test

func Example() {
	// Function body
}`),
	}
	
	vfs.LoadFiles(files, nil)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loader.LoadFromMemory(vfs)
	}
}

func BenchmarkDirectExecution(b *testing.B) {
	dmte := &DirectMemoryTestExecution{}
	
	dmte.RegisterTestFunction("BenchTest", func(t *testing.T) {
		// Simple test
		x := 1 + 1
		if x != 2 {
			t.Error("Math is broken")
		}
	})
	
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = dmte.ExecuteDirect(ctx)
	}
}