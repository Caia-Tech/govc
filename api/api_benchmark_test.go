package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Caia-Tech/govc/config"
	"github.com/gin-gonic/gin"
)

// setupBenchmarkServer creates a server for benchmarking
func setupBenchmarkServer() (*Server, *gin.Engine) {
	gin.SetMode(gin.ReleaseMode)
	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false
	cfg.Auth.JWT.Secret = "benchmark-secret-for-testing-purposes-only"
	cfg.Server.MaxRepos = 10000
	cfg.Pool.MaxRepositories = 1000
	cfg.Metrics.Enabled = false // Disable metrics for benchmarks

	server := NewServer(cfg)
	router := gin.New()
	server.RegisterRoutes(router)
	return server, router
}

// BenchmarkCreateRepository benchmarks repository creation
func BenchmarkCreateRepository(b *testing.B) {
	_, router := setupBenchmarkServer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := bytes.NewBufferString(fmt.Sprintf(`{"id": "bench-repo-%d", "memory_only": true}`, i))
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			b.Fatalf("Failed to create repository: %d", w.Code)
		}
	}
}

// BenchmarkGetRepository benchmarks repository retrieval
func BenchmarkGetRepository(b *testing.B) {
	_, router := setupBenchmarkServer()

	// Create a repository first
	body := bytes.NewBufferString(`{"id": "bench-get-repo", "memory_only": true}`)
	req := httptest.NewRequest("POST", "/api/v1/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/v1/repos/bench-get-repo", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			b.Fatalf("Failed to get repository: %d", w.Code)
		}
	}
}

// BenchmarkListRepositories benchmarks listing repositories
func BenchmarkListRepositories(b *testing.B) {
	_, router := setupBenchmarkServer()

	// Create multiple repositories
	for i := 0; i < 100; i++ {
		body := bytes.NewBufferString(fmt.Sprintf(`{"id": "list-repo-%d", "memory_only": true}`, i))
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/v1/repos", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			b.Fatalf("Failed to list repositories: %d", w.Code)
		}
	}
}

// BenchmarkAddFile benchmarks adding files to a repository
func BenchmarkAddFile(b *testing.B) {
	_, router := setupBenchmarkServer()

	// Create a repository
	body := bytes.NewBufferString(`{"id": "bench-add-repo", "memory_only": true}`)
	req := httptest.NewRequest("POST", "/api/v1/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := bytes.NewBufferString(fmt.Sprintf(
			`{"path": "file%d.txt", "content": "benchmark content %d"}`, i, i))
		req := httptest.NewRequest("POST", "/api/v1/repos/bench-add-repo/add", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			b.Fatalf("Failed to add file: %d", w.Code)
		}
	}
}

// BenchmarkCommit benchmarks commit operations
func BenchmarkCommit(b *testing.B) {
	_, router := setupBenchmarkServer()

	// Create a repository
	body := bytes.NewBufferString(`{"id": "bench-commit-repo", "memory_only": true}`)
	req := httptest.NewRequest("POST", "/api/v1/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Add a file
		body := bytes.NewBufferString(fmt.Sprintf(
			`{"path": "commit%d.txt", "content": "commit content %d"}`, i, i))
		req := httptest.NewRequest("POST", "/api/v1/repos/bench-commit-repo/add", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Commit
		body = bytes.NewBufferString(fmt.Sprintf(`{"message": "Commit %d"}`, i))
		req = httptest.NewRequest("POST", "/api/v1/repos/bench-commit-repo/commit", body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			b.Fatalf("Failed to commit: %d", w.Code)
		}
	}
}

// BenchmarkTransaction benchmarks transaction operations
func BenchmarkTransaction(b *testing.B) {
	_, router := setupBenchmarkServer()

	// Create a repository
	body := bytes.NewBufferString(`{"id": "bench-tx-repo", "memory_only": true}`)
	req := httptest.NewRequest("POST", "/api/v1/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Begin transaction
		req := httptest.NewRequest("POST", "/api/v1/repos/bench-tx-repo/transaction", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var txResp TransactionResponse
		json.Unmarshal(w.Body.Bytes(), &txResp)

		// Add files to transaction
		for j := 0; j < 5; j++ {
			body := bytes.NewBufferString(fmt.Sprintf(
				`{"path": "tx%d_%d.txt", "content": "transaction content"}`, i, j))
			req := httptest.NewRequest("POST",
				fmt.Sprintf("/api/v1/repos/bench-tx-repo/transaction/%s/add", txResp.ID), body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}

		// Commit transaction
		body = bytes.NewBufferString(fmt.Sprintf(`{"message": "Transaction %d"}`, i))
		req = httptest.NewRequest("POST",
			fmt.Sprintf("/api/v1/repos/bench-tx-repo/transaction/%s/commit", txResp.ID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			b.Fatalf("Failed to commit transaction: %d", w.Code)
		}
	}
}

// BenchmarkCreateBranch benchmarks branch creation
func BenchmarkCreateBranch(b *testing.B) {
	_, router := setupBenchmarkServer()

	// Create and initialize a repository
	body := bytes.NewBufferString(`{"id": "bench-branch-repo", "memory_only": true}`)
	req := httptest.NewRequest("POST", "/api/v1/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Add initial commit
	body = bytes.NewBufferString(`{"path": "README.md", "content": "# Benchmark"}`)
	req = httptest.NewRequest("POST", "/api/v1/repos/bench-branch-repo/add", body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body = bytes.NewBufferString(`{"message": "Initial commit"}`)
	req = httptest.NewRequest("POST", "/api/v1/repos/bench-branch-repo/commit", body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := bytes.NewBufferString(fmt.Sprintf(`{"name": "branch-%d"}`, i))
		req := httptest.NewRequest("POST", "/api/v1/repos/bench-branch-repo/branches", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			b.Fatalf("Failed to create branch: %d", w.Code)
		}
	}
}

// BenchmarkParallelRealities benchmarks parallel reality operations
func BenchmarkParallelRealities(b *testing.B) {
	_, router := setupBenchmarkServer()

	// Create and initialize a repository
	body := bytes.NewBufferString(`{"id": "bench-reality-repo", "memory_only": true}`)
	req := httptest.NewRequest("POST", "/api/v1/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Add initial commit
	body = bytes.NewBufferString(`{"path": "config.yaml", "content": "version: 1.0"}`)
	req = httptest.NewRequest("POST", "/api/v1/repos/bench-reality-repo/add", body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body = bytes.NewBufferString(`{"message": "Initial config"}`)
	req = httptest.NewRequest("POST", "/api/v1/repos/bench-reality-repo/commit", body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create parallel realities
		branches := []string{
			fmt.Sprintf("reality-%d-a", i),
			fmt.Sprintf("reality-%d-b", i),
			fmt.Sprintf("reality-%d-c", i),
		}
		branchesJSON, _ := json.Marshal(branches)
		body := bytes.NewBufferString(fmt.Sprintf(`{"branches": %s}`, branchesJSON))
		req := httptest.NewRequest("POST", "/api/v1/repos/bench-reality-repo/parallel-realities", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			b.Fatalf("Failed to create parallel realities: %d", w.Code)
		}
	}
}

// BenchmarkConcurrentRequests benchmarks concurrent request handling
func BenchmarkConcurrentRequests(b *testing.B) {
	_, router := setupBenchmarkServer()

	// Create test repositories
	for i := 0; i < 10; i++ {
		body := bytes.NewBufferString(fmt.Sprintf(`{"id": "concurrent-repo-%d", "memory_only": true}`, i))
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			repoID := fmt.Sprintf("concurrent-repo-%d", i%10)

			// Mix of operations
			switch i % 4 {
			case 0: // Get repository
				req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s", repoID), nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

			case 1: // Add file
				body := bytes.NewBufferString(fmt.Sprintf(
					`{"path": "file%d.txt", "content": "content"}`, i))
				req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

			case 2: // Get status
				req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

			case 3: // List branches
				req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
			}

			i++
		}
	})
}

// BenchmarkLargeFileOperations benchmarks operations with large files
func BenchmarkLargeFileOperations(b *testing.B) {
	_, router := setupBenchmarkServer()

	// Create a repository
	body := bytes.NewBufferString(`{"id": "bench-large-repo", "memory_only": true}`)
	req := httptest.NewRequest("POST", "/api/v1/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Create different sized content
	sizes := []int{
		1024,        // 1KB
		10 * 1024,   // 10KB
		100 * 1024,  // 100KB
		1024 * 1024, // 1MB
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size_%dKB", size/1024), func(b *testing.B) {
			content := strings.Repeat("A", size)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				body := bytes.NewBufferString(fmt.Sprintf(
					`{"path": "large%d_%d.txt", "content": "%s"}`, size, i, content))
				req := httptest.NewRequest("POST", "/api/v1/repos/bench-large-repo/add", body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				if w.Code != http.StatusOK {
					b.Fatalf("Failed to add large file: %d", w.Code)
				}
			}
		})
	}
}

// BenchmarkTransactionSize benchmarks transactions with varying numbers of files
func BenchmarkTransactionSize(b *testing.B) {
	_, router := setupBenchmarkServer()

	// Create a repository
	body := bytes.NewBufferString(`{"id": "bench-tx-size-repo", "memory_only": true}`)
	req := httptest.NewRequest("POST", "/api/v1/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	fileCounts := []int{1, 10, 50, 100}

	for _, count := range fileCounts {
		b.Run(fmt.Sprintf("Files_%d", count), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Begin transaction
				req := httptest.NewRequest("POST", "/api/v1/repos/bench-tx-size-repo/transaction", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				var txResp TransactionResponse
				json.Unmarshal(w.Body.Bytes(), &txResp)

				// Add files
				for j := 0; j < count; j++ {
					body := bytes.NewBufferString(fmt.Sprintf(
						`{"path": "tx%d_file%d.txt", "content": "content %d"}`, i, j, j))
					req := httptest.NewRequest("POST",
						fmt.Sprintf("/api/v1/repos/bench-tx-size-repo/transaction/%s/add", txResp.ID), body)
					req.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)
				}

				// Commit transaction
				body = bytes.NewBufferString(fmt.Sprintf(`{"message": "Transaction with %d files"}`, count))
				req = httptest.NewRequest("POST",
					fmt.Sprintf("/api/v1/repos/bench-tx-size-repo/transaction/%s/commit", txResp.ID), body)
				req.Header.Set("Content-Type", "application/json")
				w = httptest.NewRecorder()
				router.ServeHTTP(w, req)
			}
		})
	}
}

// BenchmarkMemoryUsage benchmarks memory usage patterns
func BenchmarkMemoryUsage(b *testing.B) {
	b.Run("RepositoryCreation", func(b *testing.B) {
		server, router := setupBenchmarkServer()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			body := bytes.NewBufferString(fmt.Sprintf(`{"id": "mem-repo-%d", "memory_only": true}`, i))
			req := httptest.NewRequest("POST", "/api/v1/repos", body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// Report memory stats
			if i%100 == 0 {
				server.mu.RLock()
				repoCount := server.repoPool.Size()
				txCount := len(server.transactions)
				server.mu.RUnlock()
				b.Logf("Repos: %d, Transactions: %d", repoCount, txCount)
			}
		}
	})

	b.Run("TransactionAccumulation", func(b *testing.B) {
		server, router := setupBenchmarkServer()

		// Create a repository
		body := bytes.NewBufferString(`{"id": "mem-tx-repo", "memory_only": true}`)
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		activeTransactions := make([]string, 0)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Create transaction
			req := httptest.NewRequest("POST", "/api/v1/repos/mem-tx-repo/transaction", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			var txResp TransactionResponse
			json.Unmarshal(w.Body.Bytes(), &txResp)
			activeTransactions = append(activeTransactions, txResp.ID)

			// Periodically commit some transactions
			if i%10 == 0 && len(activeTransactions) > 5 {
				txToCommit := activeTransactions[0]
				activeTransactions = activeTransactions[1:]

				body := bytes.NewBufferString(`{"message": "Memory benchmark commit"}`)
				req := httptest.NewRequest("POST",
					fmt.Sprintf("/api/v1/repos/mem-tx-repo/transaction/%s/commit", txToCommit), body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
			}

			// Report transaction count
			if i%50 == 0 {
				server.mu.RLock()
				txCount := len(server.transactions)
				server.mu.RUnlock()
				b.Logf("Active transactions in server: %d", txCount)
			}
		}
	})
}

// BenchmarkJSONSerialization benchmarks JSON encoding/decoding overhead
func BenchmarkJSONSerialization(b *testing.B) {
	_, router := setupBenchmarkServer()

	// Create a repository with history
	body := bytes.NewBufferString(`{"id": "bench-json-repo", "memory_only": true}`)
	req := httptest.NewRequest("POST", "/api/v1/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Add commits
	for i := 0; i < 100; i++ {
		body := bytes.NewBufferString(fmt.Sprintf(
			`{"path": "file%d.txt", "content": "content %d"}`, i, i))
		req := httptest.NewRequest("POST", "/api/v1/repos/bench-json-repo/add", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if i%10 == 0 {
			body = bytes.NewBufferString(fmt.Sprintf(`{"message": "Commit batch %d"}`, i/10))
			req = httptest.NewRequest("POST", "/api/v1/repos/bench-json-repo/commit", body)
			req.Header.Set("Content-Type", "application/json")
			w = httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}
	}

	b.Run("GetLog", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/api/v1/repos/bench-json-repo/log", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				b.Fatalf("Failed to get log: %d", w.Code)
			}
		}
	})

	b.Run("GetStatus", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/api/v1/repos/bench-json-repo/status", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				b.Fatalf("Failed to get status: %d", w.Code)
			}
		}
	})
}

// BenchmarkRouting benchmarks the routing overhead
func BenchmarkRouting(b *testing.B) {
	_, router := setupBenchmarkServer()

	routes := []string{
		"/health",
		"/api/v1/repos",
		"/api/v1/repos/test-repo",
		"/api/v1/repos/test-repo/status",
		"/api/v1/repos/test-repo/branches",
		"/api/v1/repos/test-repo/log",
	}

	for _, route := range routes {
		b.Run(strings.ReplaceAll(route, "/", "_"), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("GET", route, nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
			}
		})
	}
}

// BenchmarkMiddleware benchmarks middleware overhead
func BenchmarkMiddleware(b *testing.B) {
	b.Run("WithoutAuth", func(b *testing.B) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		cfg.Auth.JWT.Secret = "benchmark-secret-for-testing-purposes-only"
		cfg.Server.MaxRepos = 10000
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/health", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}
	})

	b.Run("WithAuth", func(b *testing.B) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = true
		cfg.Auth.JWT.Secret = "benchmark-secret-for-testing-purposes-only"
		cfg.Server.MaxRepos = 10000
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/api/v1/repos", nil)
			req.Header.Set("Authorization", "Bearer test-token")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}
	})

	b.Run("RateLimiting", func(b *testing.B) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		cfg.Auth.JWT.Secret = "benchmark-secret-for-testing-purposes-only"
		cfg.Server.MaxRepos = 10000
		server := NewServer(cfg)
		router := gin.New()
		router.Use(RateLimitMiddleware(1000)) // High limit for benchmark
		server.RegisterRoutes(router)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/health", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}
	})
}
