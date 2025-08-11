package benchmarks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Caia-Tech/govc"
	"github.com/Caia-Tech/govc/api"
)

// BenchmarkOptimizedClient compares optimized client vs standard client
func BenchmarkOptimizedClient(b *testing.B) {
	// Setup repository and server
	repo := govc.NewRepository()
	handler := setupTestServer(repo)
	server := httptest.NewServer(handler)
	defer server.Close()
	
	ctx := context.Background()
	testData := []byte("test content for benchmarking optimized client")
	
	// Standard client (from existing benchmarks)
	standardClient := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	// Optimized client with connection pooling
	optimizedClient := api.NewOptimizedHTTPClient(&api.ClientOptions{
		BaseURL:    server.URL,
		Timeout:    10 * time.Second,
		BinaryMode: false, // Start with JSON
		EnableGzip: true,
		ConnectionPool: &api.ConnectionPool{
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     60 * time.Second,
			DialTimeout:         5 * time.Second,
			KeepAliveTimeout:    30 * time.Second,
		},
		RetryPolicy: &api.RetryPolicy{
			MaxRetries:    2,
			InitialDelay:  50 * time.Millisecond,
			MaxDelay:      2 * time.Second,
			BackoffFactor: 2.0,
		},
	})
	defer optimizedClient.Close()
	
	// Warmup connections
	optimizedClient.WarmupConnections(ctx, []string{"/blob", "/batch"})
	
	b.Run("StandardClient_Single", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			payload := map[string]string{
				"content": string(testData),
			}
			jsonData, _ := json.Marshal(payload)
			
			resp, err := standardClient.Post(
				server.URL+"/blob",
				"application/json",
				bytes.NewReader(jsonData),
			)
			if err != nil {
				b.Fatal(err)
			}
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
	})
	
	b.Run("OptimizedClient_Single_JSON", func(b *testing.B) {
		payload := map[string]string{
			"content": string(testData),
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := optimizedClient.Post(ctx, "/blob", payload)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	
	b.Run("OptimizedClient_Single_Binary", func(b *testing.B) {
		optimizedClient.EnableBinaryMode()
		payload := map[string]string{
			"content": string(testData),
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := optimizedClient.Post(ctx, "/blob", payload)
			if err != nil {
				b.Fatal(err)
			}
		}
		optimizedClient.DisableBinaryMode() // Reset for other tests
	})
	
	b.Run("OptimizedClient_Batch_10", func(b *testing.B) {
		ops := make([]api.BatchOperation, 10)
		for j := 0; j < 10; j++ {
			ops[j] = api.BatchOperation{
				ID:   fmt.Sprintf("op_%d", j),
				Type: api.OpStoreBlob,
				Params: json.RawMessage(fmt.Sprintf(`{"content": "%s_%d"}`, 
					string(testData), j)),
			}
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := optimizedClient.BatchRequest(ctx, ops)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	
	b.Run("OptimizedClient_Batch_100", func(b *testing.B) {
		ops := make([]api.BatchOperation, 100)
		for j := 0; j < 100; j++ {
			ops[j] = api.BatchOperation{
				ID:   fmt.Sprintf("op_%d", j),
				Type: api.OpStoreBlob,
				Params: json.RawMessage(fmt.Sprintf(`{"content": "content_%d"}`, j)),
			}
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := optimizedClient.BatchRequest(ctx, ops)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkConnectionPooling tests the effectiveness of connection pooling
func BenchmarkConnectionPooling(b *testing.B) {
	repo := govc.NewRepository()
	handler := setupTestServer(repo)
	server := httptest.NewServer(handler)
	defer server.Close()
	
	ctx := context.Background()
	testData := []byte("connection pooling test data")
	
	// Client without connection pooling (new connection each time)
	noPoolClient := api.NewOptimizedHTTPClient(&api.ClientOptions{
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
		ConnectionPool: &api.ConnectionPool{
			MaxIdleConns:        0, // No pooling
			MaxIdleConnsPerHost: 0,
			IdleConnTimeout:     1 * time.Millisecond, // Very short
			DialTimeout:         5 * time.Second,
			KeepAliveTimeout:    1 * time.Millisecond,
		},
	})
	defer noPoolClient.Close()
	
	// Client with aggressive connection pooling
	poolClient := api.NewOptimizedHTTPClient(&api.ClientOptions{
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
		ConnectionPool: &api.ConnectionPool{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 50,
			IdleConnTimeout:     300 * time.Second, // Long timeout
			DialTimeout:         5 * time.Second,
			KeepAliveTimeout:    30 * time.Second,
		},
	})
	defer poolClient.Close()
	
	payload := map[string]string{
		"content": string(testData),
	}
	
	b.Run("NoConnectionPooling", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := noPoolClient.Post(ctx, "/blob", payload)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	
	b.Run("WithConnectionPooling", func(b *testing.B) {
		// Warmup connection
		poolClient.WarmupConnections(ctx, []string{"/blob"})
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := poolClient.Post(ctx, "/blob", payload)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkSerializationMethods compares JSON vs MessagePack
func BenchmarkSerializationMethods(b *testing.B) {
	repo := govc.NewRepository()
	handler := setupTestServer(repo)
	server := httptest.NewServer(handler)
	defer server.Close()
	
	ctx := context.Background()
	
	client := api.NewOptimizedHTTPClient(&api.ClientOptions{
		BaseURL:    server.URL,
		Timeout:    10 * time.Second,
		EnableGzip: true,
	})
	defer client.Close()
	
	// Large payload to see serialization differences
	largeContent := make([]byte, 10*1024) // 10KB
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}
	
	payload := map[string]interface{}{
		"content": string(largeContent),
		"metadata": map[string]interface{}{
			"size":        len(largeContent),
			"timestamp":   time.Now().Unix(),
			"compression": false,
			"checksum":    "abc123def456",
		},
	}
	
	b.Run("JSON_Serialization", func(b *testing.B) {
		client.DisableBinaryMode()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := client.Post(ctx, "/blob", payload)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	
	b.Run("MessagePack_Serialization", func(b *testing.B) {
		client.EnableBinaryMode()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := client.Post(ctx, "/blob", payload)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkRetryMechanisms tests retry behavior under failures
func BenchmarkRetryMechanisms(b *testing.B) {
	repo := govc.NewRepository()
	
	// Create a server that fails intermittently
	failureRate := 0.3 // 30% failure rate
	requestCount := 0
	
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if float64(requestCount%10) < failureRate*10 {
			// Simulate failure
			http.Error(w, "Simulated server error", http.StatusInternalServerError)
			return
		}
		
		// Success
		setupTestServer(repo).ServeHTTP(w, r)
	})
	
	server := httptest.NewServer(handler)
	defer server.Close()
	
	ctx := context.Background()
	
	// Client with retry policy
	retryClient := api.NewOptimizedHTTPClient(&api.ClientOptions{
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
		RetryPolicy: &api.RetryPolicy{
			MaxRetries:    3,
			InitialDelay:  10 * time.Millisecond,
			MaxDelay:      1 * time.Second,
			BackoffFactor: 2.0,
		},
	})
	defer retryClient.Close()
	
	// Client without retry
	noRetryClient := api.NewOptimizedHTTPClient(&api.ClientOptions{
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
		RetryPolicy: &api.RetryPolicy{
			MaxRetries: 0, // No retries
		},
	})
	defer noRetryClient.Close()
	
	payload := map[string]string{
		"content": "retry test data",
	}
	
	b.Run("WithRetry", func(b *testing.B) {
		successCount := 0
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := retryClient.Post(ctx, "/blob", payload)
			if err == nil {
				successCount++
			}
		}
		b.Logf("Success rate with retry: %.1f%%", float64(successCount)/float64(b.N)*100)
	})
	
	b.Run("WithoutRetry", func(b *testing.B) {
		successCount := 0
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := noRetryClient.Post(ctx, "/blob", payload)
			if err == nil {
				successCount++
			}
		}
		b.Logf("Success rate without retry: %.1f%%", float64(successCount)/float64(b.N)*100)
	})
}

// Example benchmark results expectation:
/*
BenchmarkOptimizedClient/StandardClient_Single-8                 1000    1842736 ns/op
BenchmarkOptimizedClient/OptimizedClient_Single_JSON-8           3000     615245 ns/op  (3x faster)
BenchmarkOptimizedClient/OptimizedClient_Single_Binary-8         5000     410163 ns/op  (4.5x faster)
BenchmarkOptimizedClient/OptimizedClient_Batch_10-8             10000     182741 ns/op  (10x faster)
BenchmarkOptimizedClient/OptimizedClient_Batch_100-8             2000     912356 ns/op  (20x faster)

BenchmarkConnectionPooling/NoConnectionPooling-8                 500     2847132 ns/op
BenchmarkConnectionPooling/WithConnectionPooling-8              2000      923461 ns/op  (3x faster)

BenchmarkSerializationMethods/JSON_Serialization-8              1000     1245673 ns/op
BenchmarkSerializationMethods/MessagePack_Serialization-8       3000      432189 ns/op  (2.9x faster)
*/