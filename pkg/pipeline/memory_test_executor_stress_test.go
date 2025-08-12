package pipeline

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMemoryExecutorEdgeCases tests edge cases and failure modes
func TestMemoryExecutorEdgeCases(t *testing.T) {
	t.Run("NilInputs", func(t *testing.T) {
		executor := NewMemoryTestExecutor()
		
		// Test with nil source files
		result, err := executor.ExecuteTestsInMemory(context.Background(), nil, nil)
		if err != nil {
			t.Errorf("Should handle nil inputs gracefully, got error: %v", err)
		}
		if result == nil {
			t.Error("Should return empty result, not nil")
		}
		
		// Test with empty maps
		result, err = executor.ExecuteTestsInMemory(context.Background(), 
			map[string][]byte{}, map[string][]byte{})
		if err != nil {
			t.Errorf("Should handle empty inputs gracefully, got error: %v", err)
		}
	})
	
	t.Run("MalformedGoCode", func(t *testing.T) {
		executor := NewMemoryTestExecutor()
		
		testCases := []struct {
			name        string
			sourceFiles map[string][]byte
			testFiles   map[string][]byte
			shouldError bool
		}{
			{
				name: "Invalid syntax",
				sourceFiles: map[string][]byte{
					"broken.go": []byte("package main\n\nfunc {{{ invalid syntax"),
				},
				shouldError: true,
			},
			{
				name: "Missing package declaration",
				sourceFiles: map[string][]byte{
					"nopackage.go": []byte("func Test() {}"),
				},
				shouldError: true,
			},
			{
				name: "Unicode in code",
				sourceFiles: map[string][]byte{
					"unicode.go": []byte("package main\n\n// 你好世界\nfunc Test() {}"),
				},
				shouldError: false, // Should handle unicode
			},
			{
				name: "Binary data instead of code",
				sourceFiles: map[string][]byte{
					"binary.go": []byte{0xFF, 0xFE, 0x00, 0x01, 0x02, 0x03},
				},
				shouldError: true,
			},
			{
				name: "Circular imports",
				sourceFiles: map[string][]byte{
					"a/a.go": []byte(`package a
import "b"
func A() { b.B() }`),
					"b/b.go": []byte(`package b
import "a"
func B() { a.A() }`),
				},
				shouldError: false, // Parser should handle, compiler would fail
			},
		}
		
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := context.Background()
				result, err := executor.ExecuteTestsInMemory(ctx, tc.sourceFiles, tc.testFiles)
				
				if tc.shouldError && err == nil {
					t.Error("Expected error for malformed code, got none")
				}
				if !tc.shouldError && err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if !tc.shouldError && result == nil {
					t.Error("Expected result for valid code")
				}
			})
		}
	})
	
	t.Run("VeryLargeFiles", func(t *testing.T) {
		executor := NewMemoryTestExecutor()
		
		// Create a very large test file (10MB)
		var builder strings.Builder
		builder.WriteString("package large\n\n")
		builder.WriteString("import \"testing\"\n\n")
		
		for i := 0; i < 100000; i++ {
			builder.WriteString(fmt.Sprintf(`
func Test%d(t *testing.T) {
	// Test function %d with some comments to make it larger
	x := %d + %d
	if x != %d {
		t.Error("Math failed")
	}
}
`, i, i, i, i, i*2))
		}
		
		largeFile := []byte(builder.String())
		t.Logf("Created large file of %d bytes (~%d MB)", len(largeFile), len(largeFile)/1024/1024)
		
		testFiles := map[string][]byte{
			"large_test.go": largeFile,
		}
		
		startTime := time.Now()
		ctx := context.Background()
		result, err := executor.ExecuteTestsInMemory(ctx, nil, testFiles)
		duration := time.Since(startTime)
		
		if err != nil {
			t.Errorf("Failed to handle large file: %v", err)
		}
		if result == nil {
			t.Error("Expected result for large file")
		}
		
		t.Logf("Processing large file took %v", duration)
		if duration > time.Second*10 {
			t.Error("Processing took too long, possible performance issue")
		}
	})
	
	t.Run("MemoryExhaustion", func(t *testing.T) {
		// Track memory before test
		var memStatsBefore runtime.MemStats
		runtime.ReadMemStats(&memStatsBefore)
		
		executor := NewMemoryTestExecutor()
		
		// Try to allocate many large files
		files := make(map[string][]byte)
		const fileSize = 1024 * 1024 // 1MB per file
		const numFiles = 100
		
		for i := 0; i < numFiles; i++ {
			data := make([]byte, fileSize)
			for j := range data {
				data[j] = byte(j % 256)
			}
			files[fmt.Sprintf("file%d.go", i)] = data
		}
		
		t.Logf("Created %d files of %d bytes each", numFiles, fileSize)
		
		// This should handle the memory pressure gracefully
		ctx := context.Background()
		_, err := executor.ExecuteTestsInMemory(ctx, files, nil)
		
		// Track memory after test
		var memStatsAfter runtime.MemStats
		runtime.ReadMemStats(&memStatsAfter)
		
		memoryGrowth := memStatsAfter.Alloc - memStatsBefore.Alloc
		t.Logf("Memory growth: %d bytes (~%d MB)", memoryGrowth, memoryGrowth/1024/1024)
		
		// Force garbage collection
		runtime.GC()
		runtime.GC() // Run twice to ensure cleanup
		
		var memStatsGC runtime.MemStats
		runtime.ReadMemStats(&memStatsGC)
		
		var memoryAfterGC int64
		if memStatsGC.Alloc > memStatsBefore.Alloc {
			memoryAfterGC = int64(memStatsGC.Alloc - memStatsBefore.Alloc)
		} else {
			memoryAfterGC = -int64(memStatsBefore.Alloc - memStatsGC.Alloc)
		}
		t.Logf("Memory after GC: %d bytes (~%d MB)", memoryAfterGC, memoryAfterGC/1024/1024)
		
		// Check for memory leaks - after GC, memory should be mostly recovered
		if memoryAfterGC > int64(memoryGrowth/2) {
			t.Error("Possible memory leak detected - memory not released after GC")
		}
		
		if err != nil {
			// Memory exhaustion errors are acceptable
			t.Logf("Memory exhaustion handled: %v", err)
		}
	})
	
	t.Run("ConcurrentAccess", func(t *testing.T) {
		vfs := NewVirtualFileSystem()
		
		// Load initial files
		files := map[string][]byte{
			"concurrent.go": []byte("package main\nfunc Test() {}"),
		}
		vfs.LoadFiles(files, nil)
		
		// Test concurrent reads and writes
		var wg sync.WaitGroup
		errors := make(chan error, 100)
		
		// Multiple readers
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					_, err := vfs.ReadFile("concurrent.go")
					if err != nil {
						errors <- fmt.Errorf("reader %d: %v", id, err)
					}
				}
			}(i)
		}
		
		// Multiple writers
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					newFiles := map[string][]byte{
						fmt.Sprintf("file_%d_%d.go", id, j): []byte(fmt.Sprintf("package test%d", id)),
					}
					err := vfs.LoadFiles(newFiles, nil)
					if err != nil {
						errors <- fmt.Errorf("writer %d: %v", id, err)
					}
				}
			}(i)
		}
		
		// Wait for all goroutines
		wg.Wait()
		close(errors)
		
		// Check for errors
		errorCount := 0
		for err := range errors {
			t.Errorf("Concurrent access error: %v", err)
			errorCount++
			if errorCount > 10 {
				t.Fatal("Too many concurrent access errors")
			}
		}
	})
	
	t.Run("InfiniteRecursion", func(t *testing.T) {
		harness := NewTestHarness()
		inMemT := harness.NewTest("TestInfiniteRecursion")
		
		// Create a function that would cause infinite recursion
		var recursiveTest func(depth int)
		recursiveTest = func(depth int) {
			if depth > 10000 {
				// Should never reach here in normal execution
				inMemT.Error("Infinite recursion not prevented")
				return
			}
			// This would cause stack overflow if not handled
			recursiveTest(depth + 1)
		}
		
		// Execute with protection
		done := make(chan bool)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					// Expected - we should recover from stack overflow
					t.Logf("Correctly caught recursion panic: %v", r)
				}
				done <- true
			}()
			recursiveTest(0)
		}()
		
		select {
		case <-done:
			// Completed (with panic recovery)
		case <-time.After(time.Second):
			t.Error("Infinite recursion caused timeout")
		}
	})
	
	t.Run("PanicHandling", func(t *testing.T) {
		dmte := &DirectMemoryTestExecution{}
		
		// Register a test that panics
		dmte.RegisterTestFunction("TestPanic", func(t *testing.T) {
			panic("intentional panic for testing")
		})
		
		ctx := context.Background()
		result, err := dmte.ExecuteDirect(ctx)
		
		// Should handle panic gracefully
		if err != nil {
			t.Errorf("Panic should be caught, not returned as error: %v", err)
		}
		if result == nil {
			t.Fatal("Should return result even with panic")
		}
		
		// The test should be marked as failed due to panic
		if result.Failed != 1 {
			t.Error("Test with panic should be marked as failed")
		}
	})
	
	t.Run("ContextCancellation", func(t *testing.T) {
		executor := NewMemoryTestExecutor()
		
		// Create a context that cancels immediately
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		
		testFiles := map[string][]byte{
			"test.go": []byte(`package test
import "testing"
func TestSomething(t *testing.T) {
	t.Log("Should not execute")
}`),
		}
		
		result, err := executor.ExecuteTestsInMemory(ctx, nil, testFiles)
		
		// Should handle cancellation gracefully
		if err == nil || !strings.Contains(err.Error(), "context") {
			t.Error("Expected context cancellation error")
		}
		if result != nil && len(result.Tests) > 0 {
			t.Error("Should not execute tests with cancelled context")
		}
	})
	
	t.Run("TimeoutHandling", func(t *testing.T) {
		dmte := &DirectMemoryTestExecution{}
		
		// Register a test that takes too long
		dmte.RegisterTestFunction("TestSlow", func(t *testing.T) {
			time.Sleep(time.Second * 10)
		})
		
		// Use a context with short timeout
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		defer cancel()
		
		startTime := time.Now()
		result, err := dmte.ExecuteDirect(ctx)
		duration := time.Since(startTime)
		
		if duration > time.Second {
			t.Error("Timeout not respected, took too long")
		}
		
		if err != nil {
			t.Logf("Timeout handled with error: %v", err)
		}
		if result == nil {
			t.Fatal("Should return result even with timeout")
		}
	})
	
	t.Run("InvalidTestNames", func(t *testing.T) {
		compiler := NewMemoryCompiler()
		loader := NewMemoryCodeLoader()
		vfs := NewVirtualFileSystem()
		
		// Test with various invalid test names
		testFiles := map[string][]byte{
			"invalid_test.go": []byte(`package test
import "testing"

func test_lowercase(t *testing.T) {} // Should not be detected
func NotTest(t *testing.T) {}        // Should not be detected
func Test(t *testing.T) {}            // Valid but minimal
func Test_ValidUnderscore(t *testing.T) {} // Valid
func Test123Numbers(t *testing.T) {} // Valid
func TestΩUnicode(t *testing.T) {}   // Valid with unicode
func Benchmark123(b *testing.B) {}   // Valid benchmark
func ExampleΩ() {}                   // Valid example
`),
		}
		
		vfs.LoadFiles(nil, testFiles)
		packages, err := loader.LoadFromMemory(vfs)
		if err != nil {
			t.Fatalf("Failed to load: %v", err)
		}
		
		compiled, err := compiler.CompileTests(packages)
		if err != nil {
			t.Fatalf("Failed to compile: %v", err)
		}
		
		// Count detected tests
		totalTests := 0
		for _, code := range compiled {
			totalTests += len(code.TestFuncs)
		}
		
		// Should detect only valid test functions
		expectedTests := 6 // Test, Test_ValidUnderscore, Test123Numbers, TestΩUnicode, Benchmark123, ExampleΩ
		if totalTests != expectedTests {
			t.Errorf("Expected %d valid tests, found %d", expectedTests, totalTests)
			for _, code := range compiled {
				for _, fn := range code.TestFuncs {
					t.Logf("Found test: %s", fn.Name)
				}
			}
		}
	})
}

// TestRaceConditions tests for race conditions in concurrent execution
func TestRaceConditions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race condition test in short mode")
	}
	
	t.Run("ConcurrentTestExecution", func(t *testing.T) {
		dmte := &DirectMemoryTestExecution{}
		
		// Register multiple tests
		for i := 0; i < 100; i++ {
			name := fmt.Sprintf("Test%d", i)
			dmte.RegisterTestFunction(name, func(t *testing.T) {
				// Simulate some work
				time.Sleep(time.Microsecond)
			})
		}
		
		// Execute concurrently from multiple goroutines
		var wg sync.WaitGroup
		errors := make(chan error, 10)
		
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx := context.Background()
				_, err := dmte.ExecuteDirect(ctx)
				if err != nil {
					errors <- err
				}
			}()
		}
		
		wg.Wait()
		close(errors)
		
		for err := range errors {
			t.Errorf("Race condition error: %v", err)
		}
	})
	
	t.Run("SharedMemoryCorruption", func(t *testing.T) {
		vfs := NewVirtualFileSystem()
		
		// Initial data
		originalData := []byte("package main\nfunc Test() {}")
		vfs.LoadFiles(map[string][]byte{"shared.go": originalData}, nil)
		
		// Multiple goroutines reading and modifying
		var wg sync.WaitGroup
		corrupted := false
		
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				for j := 0; j < 100; j++ {
					// Read
					data, err := vfs.ReadFile("shared.go")
					if err != nil {
						t.Errorf("Read error: %v", err)
						return
					}
					
					// Verify data integrity
					if !strings.Contains(string(data), "package") {
						corrupted = true
						t.Error("Data corruption detected")
						return
					}
					
					// Modify (simulate updates)
					if j%10 == 0 {
						newData := []byte(fmt.Sprintf("package main\n// Modified by %d\nfunc Test() {}", id))
						vfs.LoadFiles(map[string][]byte{"shared.go": newData}, nil)
					}
				}
			}(i)
		}
		
		wg.Wait()
		
		if corrupted {
			t.Fatal("Memory corruption detected in concurrent access")
		}
	})
}

// TestMemoryLeaks specifically tests for memory leaks
func TestMemoryLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}
	
	t.Run("RepeatedExecutions", func(t *testing.T) {
		executor := NewMemoryTestExecutor()
		
		// Baseline memory
		runtime.GC()
		var baseline runtime.MemStats
		runtime.ReadMemStats(&baseline)
		
		// Run many executions
		const iterations = 1000
		for i := 0; i < iterations; i++ {
			testFiles := map[string][]byte{
				fmt.Sprintf("test%d.go", i): []byte(fmt.Sprintf(`package test%d
import "testing"
func Test%d(t *testing.T) {
	t.Log("Test %d")
}`, i, i, i)),
			}
			
			ctx := context.Background()
			executor.ExecuteTestsInMemory(ctx, nil, testFiles)
			
			// Periodic GC to check for leaks
			if i%100 == 0 {
				runtime.GC()
			}
		}
		
		// Final memory check
		runtime.GC()
		runtime.GC() // Double GC to ensure cleanup
		var final runtime.MemStats
		runtime.ReadMemStats(&final)
		
		var memoryGrowthMB int64
		if final.Alloc > baseline.Alloc {
			memoryGrowth := final.Alloc - baseline.Alloc
			memoryGrowthMB = int64(memoryGrowth / 1024 / 1024)
		} else {
			// Memory decreased (good!)
			memoryGrowthMB = -int64((baseline.Alloc - final.Alloc) / 1024 / 1024)
		}
		
		t.Logf("Memory growth after %d iterations: %d MB", iterations, memoryGrowthMB)
		
		// Allow some growth but flag potential leaks
		maxAcceptableGrowthMB := int64(50)
		if memoryGrowthMB > maxAcceptableGrowthMB {
			t.Errorf("Excessive memory growth detected: %d MB (max acceptable: %d MB)", 
				memoryGrowthMB, maxAcceptableGrowthMB)
		}
	})
	
	t.Run("LargeFileCleanup", func(t *testing.T) {
		vfs := NewVirtualFileSystem()
		
		// Track initial memory
		runtime.GC()
		var initial runtime.MemStats
		runtime.ReadMemStats(&initial)
		
		// Load large files
		const fileSize = 10 * 1024 * 1024 // 10MB
		const numFiles = 10
		
		for i := 0; i < numFiles; i++ {
			data := make([]byte, fileSize)
			vfs.LoadFiles(map[string][]byte{
				fmt.Sprintf("large%d.go", i): data,
			}, nil)
		}
		
		// Check memory after loading
		var afterLoad runtime.MemStats
		runtime.ReadMemStats(&afterLoad)
		growthMB := (afterLoad.Alloc - initial.Alloc) / 1024 / 1024
		t.Logf("Memory after loading %d files: +%d MB", numFiles, growthMB)
		
		// Clear the VFS
		vfs = NewVirtualFileSystem() // Create new instance, old should be GC'd
		
		// Force GC and check memory is released
		runtime.GC()
		runtime.GC()
		
		var afterGC runtime.MemStats
		runtime.ReadMemStats(&afterGC)
		
		var remainingMB int64
		if afterGC.Alloc > initial.Alloc {
			remainingGrowth := afterGC.Alloc - initial.Alloc
			remainingMB = int64(remainingGrowth / 1024 / 1024)
		} else {
			remainingMB = -int64((initial.Alloc - afterGC.Alloc) / 1024 / 1024)
		}
		
		t.Logf("Memory after cleanup: %+d MB", remainingMB)
		
		// Memory should be mostly released
		if remainingMB > 10 {
			t.Errorf("Memory not properly released after cleanup: %d MB still allocated", remainingMB)
		}
	})
}

// TestPerformanceRegression checks for performance regressions
func TestPerformanceRegression(t *testing.T) {
	t.Run("LoadTimeRegression", func(t *testing.T) {
		loader := NewMemoryCodeLoader()
		vfs := NewVirtualFileSystem()
		
		// Create a moderate-sized codebase
		var files = make(map[string][]byte)
		for i := 0; i < 100; i++ {
			files[fmt.Sprintf("file%d.go", i)] = []byte(fmt.Sprintf(`package test
// File %d
func Function%d() {
	// Some code
}`, i, i))
		}
		
		vfs.LoadFiles(files, nil)
		
		// Measure load time
		start := time.Now()
		_, err := loader.LoadFromMemory(vfs)
		duration := time.Since(start)
		
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		
		t.Logf("Loading %d files took %v", len(files), duration)
		
		// Check for regression - should be very fast
		maxAcceptable := time.Millisecond * 100
		if duration > maxAcceptable {
			t.Errorf("Performance regression: loading took %v (max acceptable: %v)", 
				duration, maxAcceptable)
		}
	})
	
	t.Run("CompileTimeRegression", func(t *testing.T) {
		compiler := NewMemoryCompiler()
		loader := NewMemoryCodeLoader()
		vfs := NewVirtualFileSystem()
		
		// Create test files with many test functions
		testCode := `package test
import "testing"
`
		for i := 0; i < 100; i++ {
			testCode += fmt.Sprintf(`
func Test%d(t *testing.T) {
	if 1+1 != 2 {
		t.Error("Math is broken")
	}
}
`, i)
		}
		
		vfs.LoadFiles(nil, map[string][]byte{"test.go": []byte(testCode)})
		packages, _ := loader.LoadFromMemory(vfs)
		
		// Measure compile time
		start := time.Now()
		compiled, err := compiler.CompileTests(packages)
		duration := time.Since(start)
		
		if err != nil {
			t.Fatalf("Compile failed: %v", err)
		}
		
		t.Logf("Compiling %d test functions took %v", len(compiled), duration)
		
		// Check for regression
		maxAcceptable := time.Millisecond * 50
		if duration > maxAcceptable {
			t.Errorf("Performance regression: compilation took %v (max acceptable: %v)", 
				duration, maxAcceptable)
		}
	})
}

// BenchmarkStressTest performs stress testing benchmarks
func BenchmarkStressTest(b *testing.B) {
	b.Run("ManySmallFiles", func(b *testing.B) {
		vfs := NewVirtualFileSystem()
		
		// Create many small files
		files := make(map[string][]byte)
		for i := 0; i < 1000; i++ {
			files[fmt.Sprintf("small%d.go", i)] = []byte("package main")
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			vfs.LoadFiles(files, nil)
		}
	})
	
	b.Run("FewLargeFiles", func(b *testing.B) {
		vfs := NewVirtualFileSystem()
		
		// Create few large files
		files := make(map[string][]byte)
		for i := 0; i < 10; i++ {
			data := make([]byte, 1024*1024) // 1MB each
			files[fmt.Sprintf("large%d.go", i)] = data
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			vfs.LoadFiles(files, nil)
		}
	})
	
	b.Run("ConcurrentReads", func(b *testing.B) {
		vfs := NewVirtualFileSystem()
		vfs.LoadFiles(map[string][]byte{
			"file.go": []byte("package main"),
		}, nil)
		
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, _ = vfs.ReadFile("file.go")
			}
		})
	})
	
	b.Run("ConcurrentWrites", func(b *testing.B) {
		vfs := NewVirtualFileSystem()
		
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				vfs.LoadFiles(map[string][]byte{
					fmt.Sprintf("file%d.go", i): []byte("package main"),
				}, nil)
				i++
			}
		})
	})
}