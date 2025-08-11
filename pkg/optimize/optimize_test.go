package optimize

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMemoryPool(t *testing.T) {
	pool := NewMemoryPool()
	
	// Test getting and putting buffers
	buf1 := pool.Get(1024)
	if len(buf1) != 1024 {
		t.Errorf("Expected buffer size 1024, got %d", len(buf1))
	}
	
	// Write some data
	copy(buf1, []byte("test data"))
	
	// Return to pool
	pool.Put(buf1)
	
	// Get another buffer (should reuse)
	buf2 := pool.Get(1024)
	if buf2[0] != 0 {
		t.Error("Buffer not cleared after return to pool")
	}
	
	// Test efficiency
	stats := pool.Stats()
	if stats.TotalGets < 2 {
		t.Error("Pool not tracking gets")
	}
}

func TestStreamProcessor(t *testing.T) {
	pool := NewMemoryPool()
	sp := NewStreamProcessor(pool, 1024, 2)
	
	// Create test data
	input := bytes.NewBuffer([]byte("Hello, World! This is a test of the streaming processor."))
	output := &bytes.Buffer{}
	
	// Process with uppercase transformation
	err := sp.Process(context.Background(), input, output, func(chunk []byte) ([]byte, error) {
		result := make([]byte, len(chunk))
		for i, b := range chunk {
			if b >= 'a' && b <= 'z' {
				result[i] = b - 32 // Convert to uppercase
			} else {
				result[i] = b
			}
		}
		return result, nil
	})
	
	if err != nil {
		t.Fatalf("Processing failed: %v", err)
	}
	
	expected := "HELLO, WORLD! THIS IS A TEST OF THE STREAMING PROCESSOR."
	if output.String() != expected {
		t.Errorf("Expected %q, got %q", expected, output.String())
	}
}

func TestConcurrentLimiter(t *testing.T) {
	limiter := NewConcurrentLimiter(2) // Max 2 concurrent operations
	
	start := time.Now()
	results := make(chan int, 4)
	
	// Start 4 operations with max 2 concurrent
	for i := 0; i < 4; i++ {
		i := i
		go func() {
			limiter.Do(context.Background(), func() error {
				time.Sleep(100 * time.Millisecond)
				results <- i
				return nil
			})
		}()
	}
	
	// Collect results
	for i := 0; i < 4; i++ {
		<-results
	}
	
	elapsed := time.Since(start)
	// With max 2 concurrent, 4 operations of 100ms should take ~200ms
	if elapsed < 200*time.Millisecond {
		t.Error("Operations ran too fast, limiter not working")
	}
	if elapsed > 300*time.Millisecond {
		t.Error("Operations took too long")
	}
	
	stats := limiter.Stats()
	if stats.TotalCompleted != 4 {
		t.Errorf("Expected 4 completed operations, got %d", stats.TotalCompleted)
	}
}

func TestWorkerPool(t *testing.T) {
	pool := NewWorkerPool(2, 4)
	
	results := make(chan int, 10)
	
	// Submit 10 jobs
	for i := 0; i < 10; i++ {
		i := i
		pool.Submit(func() {
			results <- i * 2
		})
	}
	
	// Collect results
	sum := 0
	for i := 0; i < 10; i++ {
		sum += <-results
	}
	
	// 0*2 + 1*2 + ... + 9*2 = 2 * (0+1+...+9) = 2 * 45 = 90
	if sum != 90 {
		t.Errorf("Expected sum 90, got %d", sum)
	}
	
	// Shutdown pool
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	
	if err := pool.Shutdown(ctx); err != nil {
		t.Errorf("Pool shutdown failed: %v", err)
	}
}

func TestBatchProcessor(t *testing.T) {
	bp := NewBatchProcessor(3, 2) // Batch size 3, max 2 concurrent
	
	// Create 10 items
	items := make([]interface{}, 10)
	for i := 0; i < 10; i++ {
		items[i] = i
	}
	
	sum := 0
	err := bp.Process(context.Background(), items, func(batch []interface{}) error {
		for _, item := range batch {
			sum += item.(int)
		}
		return nil
	})
	
	if err != nil {
		t.Fatalf("Batch processing failed: %v", err)
	}
	
	// Sum of 0..9 = 45
	if sum != 45 {
		t.Errorf("Expected sum 45, got %d", sum)
	}
}

func TestStringBuilder(t *testing.T) {
	pool := NewMemoryPool()
	
	sb := pool.NewStringBuilder(10)
	defer sb.Release()
	
	// Write some data
	sb.Write([]byte("Hello"))
	sb.Write([]byte(", "))
	sb.Write([]byte("World!"))
	
	result := sb.String()
	if result != "Hello, World!" {
		t.Errorf("Expected 'Hello, World!', got %q", result)
	}
}

func TestChunkedReader(t *testing.T) {
	pool := NewMemoryPool()
	sp := NewStreamProcessor(pool, 10, 1)
	
	input := bytes.NewBufferString("This is a test of chunked reading")
	reader := sp.NewChunkedReader(input)
	
	var chunks []string
	for {
		chunk, err := reader.ReadChunk()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		chunks = append(chunks, string(chunk))
		pool.Put(chunk) // Return chunk to pool
	}
	
	if len(chunks) == 0 {
		t.Error("No chunks read")
	}
	
	// Reconstruct and verify
	result := strings.Join(chunks, "")
	expected := "This is a test of chunked reading"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func BenchmarkMemoryPool(b *testing.B) {
	pool := NewMemoryPool()
	
	b.Run("WithPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf := pool.Get(4096)
			// Simulate some work
			for j := 0; j < 100; j++ {
				buf[j] = byte(j)
			}
			pool.Put(buf)
		}
	})
	
	b.Run("WithoutPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf := make([]byte, 4096)
			// Simulate some work
			for j := 0; j < 100; j++ {
				buf[j] = byte(j)
			}
		}
	})
}

func BenchmarkConcurrentLimiter(b *testing.B) {
	limiter := NewConcurrentLimiter(10)
	ctx := context.Background()
	
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.Do(ctx, func() error {
				// Simulate work
				time.Sleep(1 * time.Microsecond)
				return nil
			})
		}
	})
}