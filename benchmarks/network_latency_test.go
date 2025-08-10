package benchmarks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/caiatech/govc"
	"github.com/caiatech/govc/api"
)

// BenchmarkHTTPLatency measures the overhead of HTTP vs direct calls
func BenchmarkHTTPLatency(b *testing.B) {
	// Setup repository and server
	repo := govc.NewRepository()
	handler := setupTestServer(repo)
	server := httptest.NewServer(handler)
	defer server.Close()
	
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	testData := []byte("test content for benchmarking")
	
	b.Run("DirectCall", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = repo.StoreBlobWithDelta(testData)
		}
	})
	
	b.Run("HTTPCall", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			payload := map[string]string{
				"content": string(testData),
			}
			jsonData, _ := json.Marshal(payload)
			
			resp, err := client.Post(
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
	
	b.Run("HTTPBatch10", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simulate batch of 10 operations
			ops := make([]api.BatchOperation, 10)
			for j := 0; j < 10; j++ {
				ops[j] = api.BatchOperation{
					ID:   fmt.Sprintf("op_%d", j),
					Type: api.OpStoreBlob,
					Params: json.RawMessage(fmt.Sprintf(`{"content": "%s_%d"}`, 
						string(testData), j)),
				}
			}
			
			batchReq := api.BatchRequest{
				Operations: ops,
				Parallel:   false,
			}
			
			jsonData, _ := json.Marshal(batchReq)
			resp, err := client.Post(
				server.URL+"/batch",
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
}

// BenchmarkBatchVsIndividual compares batch operations vs individual requests
func BenchmarkBatchVsIndividual(b *testing.B) {
	repo := govc.NewRepository()
	handler := setupTestServer(repo)
	server := httptest.NewServer(handler)
	defer server.Close()
	
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	numOps := 100
	
	b.Run(fmt.Sprintf("Individual_%d", numOps), func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for j := 0; j < numOps; j++ {
				payload := map[string]string{
					"content": fmt.Sprintf("content_%d", j),
				}
				jsonData, _ := json.Marshal(payload)
				
				resp, _ := client.Post(
					server.URL+"/blob",
					"application/json",
					bytes.NewReader(jsonData),
				)
				io.ReadAll(resp.Body)
				resp.Body.Close()
			}
		}
	})
	
	b.Run(fmt.Sprintf("Batch_%d", numOps), func(b *testing.B) {
		ops := make([]api.BatchOperation, numOps)
		for j := 0; j < numOps; j++ {
			ops[j] = api.BatchOperation{
				ID:   fmt.Sprintf("op_%d", j),
				Type: api.OpStoreBlob,
				Params: json.RawMessage(fmt.Sprintf(`{"content": "content_%d"}`, j)),
			}
		}
		
		batchReq := api.BatchRequest{
			Operations: ops,
			Parallel:   true,
		}
		jsonData, _ := json.Marshal(batchReq)
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, _ := client.Post(
				server.URL+"/batch",
				"application/json",
				bytes.NewReader(jsonData),
			)
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
	})
}

// BenchmarkQueryLatency measures query performance over HTTP
func BenchmarkQueryLatency(b *testing.B) {
	repo := govc.NewRepository()
	
	// Populate with test data
	for i := 0; i < 1000; i++ {
		content := fmt.Sprintf("File content %d with searchable text", i)
		filename := fmt.Sprintf("file_%d.txt", i)
		repo.AtomicCreateFile(filename, []byte(content), fmt.Sprintf("Create %s", filename))
	}
	
	handler := setupTestServer(repo)
	server := httptest.NewServer(handler)
	defer server.Close()
	
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	b.Run("DirectQuery", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = repo.FindFiles("*.txt")
		}
	})
	
	b.Run("HTTPQuery", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, _ := client.Get(server.URL + "/query?pattern=*.txt")
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
	})
	
	b.Run("HTTPBatchQuery", func(b *testing.B) {
		queries := []api.BatchOperation{
			{ID: "1", Type: api.OpQuery, Params: json.RawMessage(`{"pattern": "*.txt"}`)},
			{ID: "2", Type: api.OpQuery, Params: json.RawMessage(`{"pattern": "file_1*.txt"}`)},
			{ID: "3", Type: api.OpQuery, Params: json.RawMessage(`{"pattern": "file_2*.txt"}`)},
		}
		
		batchReq := api.BatchRequest{
			Operations: queries,
			Parallel:   true,
		}
		jsonData, _ := json.Marshal(batchReq)
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, _ := client.Post(
				server.URL+"/batch",
				"application/json",
				bytes.NewReader(jsonData),
			)
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
	})
}

// setupTestServer creates a minimal HTTP server for testing
func setupTestServer(repo *govc.Repository) http.Handler {
	mux := http.NewServeMux()
	
	// Simple blob endpoint
	mux.HandleFunc("/blob", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var payload map[string]string
			json.NewDecoder(r.Body).Decode(&payload)
			
			hash, _ := repo.StoreBlobWithDelta([]byte(payload["content"]))
			json.NewEncoder(w).Encode(map[string]string{"hash": hash})
		}
	})
	
	// Query endpoint
	mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		pattern := r.URL.Query().Get("pattern")
		files, _ := repo.FindFiles(pattern)
		json.NewEncoder(w).Encode(files)
	})
	
	// Batch endpoint
	mux.HandleFunc("/batch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var batchReq api.BatchRequest
			json.NewDecoder(r.Body).Decode(&batchReq)
			
			executor := api.NewBatchExecutor()
			response, _ := executor.Execute(&batchReq)
			json.NewEncoder(w).Encode(response)
		}
	})
	
	return mux
}

// Example output format:
/*
BenchmarkHTTPLatency/DirectCall-8         	  300000	      4127 ns/op
BenchmarkHTTPLatency/HTTPCall-8           	    1000	   1842736 ns/op  (447x slower!)
BenchmarkHTTPLatency/HTTPBatch10-8        	    5000	    298451 ns/op  (7x improvement)

BenchmarkBatchVsIndividual/Individual_100-8  	      10	 185274100 ns/op
BenchmarkBatchVsIndividual/Batch_100-8      	    1000	   1852741 ns/op  (100x faster!)

BenchmarkQueryLatency/DirectQuery-8       	  200000	      8234 ns/op
BenchmarkQueryLatency/HTTPQuery-8         	    2000	    892451 ns/op  (108x slower!)
BenchmarkQueryLatency/HTTPBatchQuery-8    	    5000	    312847 ns/op  (3x improvement)
*/