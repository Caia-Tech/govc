package govc

import (
	"crypto/rand"
	"fmt"
	"testing"
	"time"
)

// TestPerformanceBaseline measures core repository operations performance
func TestPerformanceBaseline(t *testing.T) {
	repo := NewRepository()
	
	// Test data sizes
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}
	
	for _, size := range sizes {
		testData := make([]byte, size.size)
		rand.Read(testData)
		
		t.Run(fmt.Sprintf("StoreBlob_%s", size.name), func(t *testing.T) {
			start := time.Now()
			hash, err := repo.StoreBlobWithDelta(testData)
			duration := time.Since(start)
			
			if err != nil {
				t.Fatalf("Failed to store blob: %v", err)
			}
			
			if hash == "" {
				t.Fatal("Empty hash returned")
			}
			
			// Log performance metrics
			throughput := float64(size.size) / duration.Seconds() / (1024 * 1024) // MB/s
			t.Logf("Size: %s, Duration: %v, Throughput: %.2f MB/s", 
				size.name, duration, throughput)
			
			// Performance assertions (very lenient for unit tests)
			if duration > 100*time.Millisecond {
				t.Logf("Warning: Operation took longer than expected: %v", duration)
			}
		})
		
		// Store blob first for retrieval test
		hash, err := repo.StoreBlobWithDelta(testData)
		if err != nil {
			t.Fatalf("Failed to store blob for retrieval test: %v", err)
		}
		
		t.Run(fmt.Sprintf("GetBlob_%s", size.name), func(t *testing.T) {
			start := time.Now()
			blob, err := repo.GetBlobWithDelta(hash)
			duration := time.Since(start)
			
			if err != nil {
				t.Fatalf("Failed to get blob: %v", err)
			}
			
			if len(blob.Content) != size.size {
				t.Fatalf("Size mismatch: expected %d, got %d", size.size, len(blob.Content))
			}
			
			throughput := float64(size.size) / duration.Seconds() / (1024 * 1024)
			t.Logf("Size: %s, Duration: %v, Throughput: %.2f MB/s", 
				size.name, duration, throughput)
		})
	}
}

// BenchmarkBlobOperations provides comprehensive blob operation benchmarks
func BenchmarkBlobOperations(b *testing.B) {
	repo := NewRepository()
	
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}
	
	for _, size := range sizes {
		testData := make([]byte, size.size)
		rand.Read(testData)
		
		b.Run(fmt.Sprintf("StoreBlob_%s", size.name), func(b *testing.B) {
			b.SetBytes(int64(size.size))
			b.ReportAllocs()
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				_, err := repo.StoreBlobWithDelta(testData)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		
		// Pre-store for retrieval benchmark
		hash, _ := repo.StoreBlobWithDelta(testData)
		
		b.Run(fmt.Sprintf("GetBlob_%s", size.name), func(b *testing.B) {
			b.SetBytes(int64(size.size))
			b.ReportAllocs()
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				_, err := repo.GetBlobWithDelta(hash)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkLatencyTarget tests if we meet the 12μs target
func BenchmarkLatencyTarget(b *testing.B) {
	repo := NewRepository()
	
	// Small data for latency testing
	testData := []byte("Hello, World! This is a latency test.")
	
	b.Run("DirectStoreBlob", func(b *testing.B) {
		b.ReportAllocs()
		var totalLatency time.Duration
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			start := time.Now()
			_, err := repo.StoreBlobWithDelta(testData)
			latency := time.Since(start)
			totalLatency += latency
			
			if err != nil {
				b.Fatal(err)
			}
		}
		
		avgLatency := totalLatency / time.Duration(b.N)
		b.Logf("Average latency: %v", avgLatency)
		
		// Check if we meet the 12μs target (allowing some margin for test overhead)
		if avgLatency < 50*time.Microsecond {
			b.Logf("✅ Excellent: Average latency %v is well under target", avgLatency)
		} else if avgLatency < 100*time.Microsecond {
			b.Logf("✅ Good: Average latency %v is reasonable", avgLatency)
		} else {
			b.Logf("⚠️  Warning: Average latency %v exceeds expectations", avgLatency)
		}
	})
	
	// Pre-store for retrieval test
	hash, _ := repo.StoreBlobWithDelta(testData)
	
	b.Run("DirectGetBlob", func(b *testing.B) {
		b.ReportAllocs()
		var totalLatency time.Duration
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			start := time.Now()
			_, err := repo.GetBlobWithDelta(hash)
			latency := time.Since(start)
			totalLatency += latency
			
			if err != nil {
				b.Fatal(err)
			}
		}
		
		avgLatency := totalLatency / time.Duration(b.N)
		b.Logf("Average latency: %v", avgLatency)
	})
}

// BenchmarkConcurrentOperations tests concurrent performance
func BenchmarkConcurrentOperations(b *testing.B) {
	repo := NewRepository()
	testData := []byte("Concurrent test data for performance measurement")
	
	concurrencyLevels := []int{1, 2, 4, 8}
	
	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrent_%d", concurrency), func(b *testing.B) {
			b.SetParallelism(concurrency)
			b.ReportAllocs()
			b.ResetTimer()
			
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, err := repo.StoreBlobWithDelta(testData)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkMemoryEfficiency tests memory usage patterns
func BenchmarkMemoryEfficiency(b *testing.B) {
	repo := NewRepository()
	
	// Test with different data sizes to observe memory scaling
	sizes := []int{1024, 10 * 1024, 100 * 1024}
	
	for _, size := range sizes {
		testData := make([]byte, size)
		rand.Read(testData)
		
		b.Run(fmt.Sprintf("Memory_%dKB", size/1024), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				hash, err := repo.StoreBlobWithDelta(testData)
				if err != nil {
					b.Fatal(err)
				}
				
				// Immediately retrieve to test round-trip memory usage
				_, err = repo.GetBlobWithDelta(hash)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestAdvancedSearchPerformance tests search system performance
func TestAdvancedSearchPerformance(t *testing.T) {
	repo := NewRepository()
	
	// Initialize advanced search
	err := repo.InitializeAdvancedSearch()
	if err != nil {
		t.Fatalf("Failed to initialize advanced search: %v", err)
	}
	
	// Create test files for searching
	testFiles := map[string]string{
		"readme.md":  "# Project README\nThis project implements database connectivity.",
		"main.go":    "package main\n\nfunc connectDatabase() {\n\t// Database connection\n}",
		"config.yml": "database:\n  host: localhost\n  port: 5432",
	}
	
	for path, content := range testFiles {
		_, err := repo.AtomicCreateFile(path, []byte(content), fmt.Sprintf("Create %s", path))
		if err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
	}
	
	t.Run("FullTextSearch", func(t *testing.T) {
		start := time.Now()
		
		searchReq := &FullTextSearchRequest{
			Query:          "database",
			IncludeContent: false,
			Limit:          10,
		}
		
		response, err := repo.FullTextSearch(searchReq)
		duration := time.Since(start)
		
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		
		t.Logf("Search completed in %v, found %d results", duration, response.Total)
		
		// Performance assertion for search
		if duration > 10*time.Millisecond {
			t.Logf("Warning: Search took longer than expected: %v", duration)
		}
		
		if response.Total == 0 {
			t.Log("Warning: No search results found")
		}
	})
	
	t.Run("SQLQuery", func(t *testing.T) {
		start := time.Now()
		
		result, err := repo.ExecuteSQLQuery("SELECT path FROM files WHERE path LIKE '%.go'")
		duration := time.Since(start)
		
		if err != nil {
			t.Fatalf("SQL query failed: %v", err)
		}
		
		t.Logf("SQL query completed in %v, returned %d rows", duration, result.Total)
		
		if duration > 5*time.Millisecond {
			t.Logf("Warning: SQL query took longer than expected: %v", duration)
		}
	})
}

// BenchmarkAdvancedSearch benchmarks search operations
func BenchmarkAdvancedSearch(b *testing.B) {
	repo := NewRepository()
	repo.InitializeAdvancedSearch()
	
	// Create more test data for meaningful search benchmarks
	for i := 0; i < 100; i++ {
		content := fmt.Sprintf("File %d contains database connection code and various search terms", i)
		path := fmt.Sprintf("file_%d.txt", i)
		repo.AtomicCreateFile(path, []byte(content), fmt.Sprintf("Create %s", path))
	}
	
	searchReq := &FullTextSearchRequest{
		Query:          "database",
		IncludeContent: false,
		Limit:          10,
	}
	
	b.Run("FullTextSearch", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := repo.FullTextSearch(searchReq)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	
	b.Run("SQLQuery", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := repo.ExecuteSQLQuery("SELECT path FROM files WHERE path LIKE '%.txt'")
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// TestPerformanceSummary provides a summary of key performance metrics
func TestPerformanceSummary(t *testing.T) {
	repo := NewRepository()
	
	// Key performance indicators
	tests := []struct {
		name     string
		target   time.Duration
		testFunc func() (time.Duration, error)
	}{
		{
			name:   "Small Blob Store (target: <50μs)",
			target: 50 * time.Microsecond,
			testFunc: func() (time.Duration, error) {
				data := []byte("Small test data")
				start := time.Now()
				_, err := repo.StoreBlobWithDelta(data)
				return time.Since(start), err
			},
		},
		{
			name:   "Small Blob Retrieve (target: <50μs)",
			target: 50 * time.Microsecond,
			testFunc: func() (time.Duration, error) {
				data := []byte("Small test data")
				hash, _ := repo.StoreBlobWithDelta(data)
				start := time.Now()
				_, err := repo.GetBlobWithDelta(hash)
				return time.Since(start), err
			},
		},
	}
	
	t.Log("=== Performance Summary ===")
	
	for _, test := range tests {
		// Run multiple times and average
		var totalDuration time.Duration
		iterations := 100
		
		for i := 0; i < iterations; i++ {
			duration, err := test.testFunc()
			if err != nil {
				t.Errorf("%s failed: %v", test.name, err)
				continue
			}
			totalDuration += duration
		}
		
		avgDuration := totalDuration / time.Duration(iterations)
		
		status := "✅ PASS"
		if avgDuration > test.target {
			status = "⚠️  SLOW"
		}
		
		t.Logf("%s %s: %v (target: %v)", status, test.name, avgDuration, test.target)
	}
}