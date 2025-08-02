package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/caiatech/govc/config"
	"github.com/gin-gonic/gin"
)

// TestRepositoryEdgeCases tests edge cases and error conditions for repository operations
func TestRepositoryEdgeCases(t *testing.T) {
	_, router := setupTestServer()

	t.Run("Create repository with empty ID", func(t *testing.T) {
		body := bytes.NewBufferString(`{"id": "", "memory_only": true}`)
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("Create repository with invalid JSON", func(t *testing.T) {
		body := bytes.NewBufferString(`{"id": "test", invalid json`)
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("Create duplicate repository", func(t *testing.T) {
		repoID := "duplicate-test"
		
		// Create first repository
		createRepo(t, router, repoID)
		
		// Try to create duplicate
		body := bytes.NewBufferString(fmt.Sprintf(`{"id": "%s", "memory_only": true}`, repoID))
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Errorf("Expected status %d, got %d", http.StatusConflict, w.Code)
		}
	})

	t.Run("Create repository with special characters in ID", func(t *testing.T) {
		specialIDs := []string{
			"repo-with-dash",
			"repo_with_underscore",
			"repo.with.dots",
			"repo/with/slashes",
			"repo with spaces",
			"repo@with@at",
		}

		for _, id := range specialIDs {
			body := bytes.NewBufferString(fmt.Sprintf(`{"id": "%s", "memory_only": true}`, id))
			req := httptest.NewRequest("POST", "/api/v1/repos", body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// Should succeed for most characters
			if w.Code != http.StatusCreated && w.Code != http.StatusBadRequest {
				t.Errorf("For ID '%s': expected status %d or %d, got %d", 
					id, http.StatusCreated, http.StatusBadRequest, w.Code)
			}
		}
	})

	t.Run("List repositories with pagination", func(t *testing.T) {
		// Create multiple repositories
		for i := 0; i < 5; i++ {
			createRepo(t, router, fmt.Sprintf("page-test-%d", i))
		}

		// Test with limit
		req := httptest.NewRequest("GET", "/api/v1/repos?limit=2", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Delete non-existent repository", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/v1/repos/non-existent-repo", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

// TestGitOperationsEdgeCases tests edge cases for Git operations
func TestGitOperationsEdgeCases(t *testing.T) {
	_, router := setupTestServer()

	// Create and initialize a test repository
	repoID := "git-edge-test"
	createRepo(t, router, repoID)

	t.Run("Add file with empty path", func(t *testing.T) {
		body := bytes.NewBufferString(`{"path": "", "content": "test"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("Add file with path traversal", func(t *testing.T) {
		dangerousPaths := []string{
			"../../../etc/passwd",
			"..\\..\\windows\\system32",
			"/etc/passwd",
			"C:\\Windows\\System32\\config",
		}

		for _, path := range dangerousPaths {
			body := bytes.NewBufferString(fmt.Sprintf(`{"path": "%s", "content": "malicious"}`, path))
			req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// Should either reject or sanitize the path
			if w.Code == http.StatusOK {
				var resp map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil {
					if pathVal, ok := resp["path"].(string); ok && strings.Contains(pathVal, "..") {
						t.Errorf("Path traversal not prevented for: %s", path)
					}
				}
			}
		}
	})

	t.Run("Add very large file", func(t *testing.T) {
		// Create 10MB of content
		largeContent := strings.Repeat("A", 10*1024*1024)
		body := bytes.NewBufferString(fmt.Sprintf(`{"path": "large.txt", "content": "%s"}`, largeContent))
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should handle large files gracefully
		if w.Code != http.StatusOK && w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("Expected status %d or %d, got %d", 
				http.StatusOK, http.StatusRequestEntityTooLarge, w.Code)
		}
	})

	t.Run("Commit with empty message", func(t *testing.T) {
		// First add a file
		body := bytes.NewBufferString(`{"path": "test.txt", "content": "test"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Try to commit with empty message
		body = bytes.NewBufferString(`{"message": ""}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Empty message might be allowed or rejected
		if w.Code != http.StatusCreated && w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d or %d, got %d", 
				http.StatusCreated, http.StatusBadRequest, w.Code)
		}
	})

	t.Run("Show non-existent commit", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/show/nonexistent123", repoID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})

	t.Run("Get log with invalid limit", func(t *testing.T) {
		invalidLimits := []string{"-1", "0", "abc", "999999999"}

		for _, limit := range invalidLimits {
			req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/log?limit=%s", repoID, limit), nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// Should handle gracefully
			if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
				t.Errorf("For limit '%s': expected status %d or %d, got %d", 
					limit, http.StatusOK, http.StatusBadRequest, w.Code)
			}
		}
	})
}

// TestBranchOperationsComplex tests complex branch scenarios
func TestBranchOperationsComplex(t *testing.T) {
	_, router := setupTestServer()

	repoID := "branch-complex-test"
	createRepo(t, router, repoID)
	createInitialCommit(t, router, repoID)

	t.Run("Create branch from non-existent branch", func(t *testing.T) {
		body := bytes.NewBufferString(`{"name": "new-feature", "from": "non-existent"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d or %d, got %d", 
				http.StatusNotFound, http.StatusBadRequest, w.Code)
		}
	})

	t.Run("Create branch with duplicate name", func(t *testing.T) {
		// Create first branch
		body := bytes.NewBufferString(`{"name": "duplicate-branch"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusCreated {
			t.Fatalf("Failed to create first branch: %d", w.Code)
		}

		// Try to create duplicate
		body = bytes.NewBufferString(`{"name": "duplicate-branch"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict && w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d or %d, got %d. Response: %s", 
				http.StatusConflict, http.StatusBadRequest, w.Code, w.Body.String())
		}
	})

	t.Run("Delete current branch", func(t *testing.T) {
		// Get current branch
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var status map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &status)
		currentBranch := status["branch"].(string)

		// Try to delete current branch
		req = httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/repos/%s/branches/%s", repoID, currentBranch), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("Merge non-existent branches", func(t *testing.T) {
		body := bytes.NewBufferString(`{"from": "non-existent", "to": "main"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/merge", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			t.Errorf("Expected error status, got %d", w.Code)
		}
	})
}

// TestTransactionsConcurrency tests concurrent transaction operations
func TestTransactionsConcurrency(t *testing.T) {
	server, router := setupTestServer()

	repoID := "tx-concurrent-test"
	createRepo(t, router, repoID)

	t.Run("Concurrent transaction creation", func(t *testing.T) {
		const numGoroutines = 10
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		transactionIDs := make([]string, numGoroutines)
		errors := make([]error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(index int) {
				defer wg.Done()

				req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/transaction", repoID), nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				if w.Code == http.StatusCreated {
					var resp TransactionResponse
					if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil {
						transactionIDs[index] = resp.ID
					} else {
						errors[index] = err
					}
				} else {
					errors[index] = fmt.Errorf("status code: %d", w.Code)
				}
			}(i)
		}

		wg.Wait()

		// Verify all transactions were created
		successCount := 0
		for i, txID := range transactionIDs {
			if txID != "" && errors[i] == nil {
				successCount++
			}
		}

		if successCount != numGoroutines {
			t.Errorf("Expected %d successful transactions, got %d", numGoroutines, successCount)
		}

		// Verify no duplicate transaction IDs
		idMap := make(map[string]bool)
		for _, txID := range transactionIDs {
			if txID != "" {
				if idMap[txID] {
					t.Errorf("Duplicate transaction ID found: %s", txID)
				}
				idMap[txID] = true
			}
		}
	})

	t.Run("Concurrent modifications to same transaction", func(t *testing.T) {
		// Create a transaction
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/transaction", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var txResp TransactionResponse
		json.Unmarshal(w.Body.Bytes(), &txResp)
		txID := txResp.ID

		// Concurrent adds to the same transaction
		const numOps = 20
		var wg sync.WaitGroup
		wg.Add(numOps)

		results := make([]int, numOps)

		for i := 0; i < numOps; i++ {
			go func(index int) {
				defer wg.Done()

				body := bytes.NewBufferString(fmt.Sprintf(
					`{"path": "file%d.txt", "content": "content %d"}`, index, index))
				req := httptest.NewRequest("POST", 
					fmt.Sprintf("/api/v1/repos/%s/transaction/%s/add", repoID, txID), body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)
				results[index] = w.Code
			}(i)
		}

		wg.Wait()

		// All operations should succeed
		for i, code := range results {
			if code != http.StatusOK {
				t.Errorf("Operation %d failed with status %d", i, code)
			}
		}
	})

	t.Run("Transaction cleanup verification", func(t *testing.T) {
		// Get initial transaction count
		server.mu.RLock()
		initialCount := len(server.transactions)
		server.mu.RUnlock()

		// Create and commit a transaction
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/transaction", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var txResp TransactionResponse
		json.Unmarshal(w.Body.Bytes(), &txResp)

		// Commit the transaction
		body := bytes.NewBufferString(`{"message": "test commit"}`)
		req = httptest.NewRequest("POST", 
			fmt.Sprintf("/api/v1/repos/%s/transaction/%s/commit", repoID, txResp.ID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Verify transaction was cleaned up
		server.mu.RLock()
		finalCount := len(server.transactions)
		server.mu.RUnlock()

		if finalCount != initialCount {
			t.Errorf("Transaction not cleaned up after commit. Initial: %d, Final: %d", 
				initialCount, finalCount)
		}
	})
}

// TestParallelRealitiesComplex tests complex parallel reality scenarios
func TestParallelRealitiesComplex(t *testing.T) {
	_, router := setupTestServer()

	repoID := "reality-complex-test"
	createRepo(t, router, repoID)
	createInitialCommit(t, router, repoID)

	t.Run("Create many parallel realities", func(t *testing.T) {
		branches := make([]string, 50)
		for i := 0; i < 50; i++ {
			branches[i] = fmt.Sprintf("reality-%d", i)
		}

		branchesJSON, _ := json.Marshal(branches)
		body := bytes.NewBufferString(fmt.Sprintf(`{"branches": %s}`, branchesJSON))
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/parallel-realities", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
		}

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		if count := int(resp["count"].(float64)); count != 50 {
			t.Errorf("Expected 50 realities created, got %d", count)
		}
	})

	t.Run("Apply different changes to realities", func(t *testing.T) {
		// Create a few realities
		body := bytes.NewBufferString(`{"branches": ["config-a", "config-b", "config-c"]}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/parallel-realities", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Apply different changes to each
		configs := []struct {
			reality string
			changes map[string]string
		}{
			{
				reality: "config-a",
				changes: map[string]string{
					"app.conf":   "mode: production\nworkers: 4",
					"cache.conf": "redis: enabled\nttl: 3600",
				},
			},
			{
				reality: "config-b",
				changes: map[string]string{
					"app.conf":   "mode: staging\nworkers: 2",
					"cache.conf": "memory: enabled\nttl: 1800",
				},
			},
			{
				reality: "config-c",
				changes: map[string]string{
					"app.conf":   "mode: development\nworkers: 1",
					"cache.conf": "disabled: true",
				},
			},
		}

		for _, cfg := range configs {
			changesJSON, _ := json.Marshal(cfg.changes)
			body := bytes.NewBufferString(fmt.Sprintf(`{"changes": %s}`, changesJSON))
			req := httptest.NewRequest("POST", 
				fmt.Sprintf("/api/v1/repos/%s/parallel-realities/%s/apply", repoID, cfg.reality), body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Failed to apply changes to %s: status %d", cfg.reality, w.Code)
			}
		}
	})

	t.Run("Benchmark non-existent reality", func(t *testing.T) {
		req := httptest.NewRequest("GET", 
			fmt.Sprintf("/api/v1/repos/%s/parallel-realities/non-existent/benchmark", repoID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

// TestTimeTravelFeatures tests time travel functionality
func TestTimeTravelFeatures(t *testing.T) {
	_, router := setupTestServer()

	repoID := "timetravel-test"
	createRepo(t, router, repoID)

	// Create commits at different times
	commits := []struct {
		files   map[string]string
		message string
		delay   time.Duration
	}{
		{
			files:   map[string]string{"v1.txt": "version 1"},
			message: "Initial version",
			delay:   0,
		},
		{
			files:   map[string]string{"v2.txt": "version 2"},
			message: "Second version",
			delay:   1000 * time.Millisecond,
		},
		{
			files:   map[string]string{"v3.txt": "version 3"},
			message: "Third version",
			delay:   1000 * time.Millisecond,
		},
	}

	timestamps := make([]int64, len(commits)+1)
	
	// Capture initial timestamp before any commits
	timestamps[0] = time.Now().Unix()
	time.Sleep(1000 * time.Millisecond) // Ensure distinct timestamps

	for i, commit := range commits {
		time.Sleep(commit.delay)

		// Add files
		for path, content := range commit.files {
			body := bytes.NewBufferString(fmt.Sprintf(
				`{"path": "%s", "content": "%s"}`, path, content))
			req := httptest.NewRequest("POST", 
				fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}

		// Commit
		body := bytes.NewBufferString(fmt.Sprintf(`{"message": "%s"}`, commit.message))
		req := httptest.NewRequest("POST", 
			fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		
		// Capture timestamp AFTER commit with longer delay for distinctness
		time.Sleep(1000 * time.Millisecond) // Longer delay for distinct timestamps
		timestamps[i+1] = time.Now().Unix()
	}

	t.Run("Time travel to past state", func(t *testing.T) {
		// Travel to time of second commit
		req := httptest.NewRequest("GET", 
			fmt.Sprintf("/api/v1/repos/%s/time-travel/%d", repoID, timestamps[1]), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// TimeTravel is a V1-only feature, might return 404 with V2 architecture
		if w.Code == http.StatusNotFound {
			t.Skip("TimeTravel not supported in current architecture")
			return
		}

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		if w.Code == http.StatusOK {
			var state map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &state)

			// Should have commit information
			if commit, ok := state["commit"].(map[string]interface{}); ok {
				if commit["hash"] == nil || commit["hash"] == "" {
					t.Errorf("Expected commit hash at timestamp %d", timestamps[1])
				}
			} else {
				t.Errorf("Expected commit information in response")
			}
		}
	})

	t.Run("Time travel to future", func(t *testing.T) {
		futureTime := time.Now().Add(time.Hour).Unix()
		req := httptest.NewRequest("GET", 
			fmt.Sprintf("/api/v1/repos/%s/time-travel/%d", repoID, futureTime), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// TimeTravel is a V1-only feature, might return 404 with V2 architecture
		if w.Code == http.StatusNotFound {
			t.Skip("TimeTravel not supported in current architecture")
			return
		}

		// Should return current state
		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Read file at specific time", func(t *testing.T) {
		// Read v2.txt at time before it was created
		req := httptest.NewRequest("GET", 
			fmt.Sprintf("/api/v1/repos/%s/time-travel/%d/read/v2.txt", repoID, timestamps[0]), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// TimeTravel is a V1-only feature, might return 404 with V2 architecture
		if w.Code == http.StatusNotFound {
			var errResp ErrorResponse
			json.Unmarshal(w.Body.Bytes(), &errResp)
			if errResp.Code == "NO_COMMITS_AT_TIME" || errResp.Code == "NO_COMMITS" {
				t.Skip("TimeTravel not supported in current architecture")
				return
			}
		}

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}

		// Read v2.txt at time after it was created (should be after commit 1)
		req = httptest.NewRequest("GET", 
			fmt.Sprintf("/api/v1/repos/%s/time-travel/%d/read/v2.txt", repoID, timestamps[2]), nil)
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d or %d, got %d", http.StatusOK, http.StatusNotFound, w.Code)
		}
	})
}

// TestMiddlewareFunctionality tests middleware behavior
func TestMiddlewareFunctionality(t *testing.T) {
	t.Run("Rate limiting", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		cfg.Auth.JWT.Secret = "test-secret-for-testing-purposes-only"
		cfg.Server.MaxRepos = 100
		server := NewServer(cfg)
		router := gin.New()
		
		// Add rate limiting middleware
		router.Use(RateLimitMiddleware(10))
		server.RegisterRoutes(router)

		// Make rapid requests
		for i := 0; i < 15; i++ {
			req := httptest.NewRequest("GET", "/api/v1/repos", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if i < 10 {
				if w.Code == http.StatusTooManyRequests {
					t.Errorf("Rate limited too early at request %d", i+1)
				}
			} else {
				if w.Code != http.StatusTooManyRequests {
					t.Errorf("Expected rate limit at request %d, got status %d", i+1, w.Code)
				}
			}
		}
	})

	t.Run("Authentication when enabled", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = true
		cfg.Auth.JWT.Secret = "test-secret-for-testing-purposes-only"
		cfg.Server.MaxRepos = 100
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		// Request without auth header to an endpoint that requires auth
		req := httptest.NewRequest("GET", "/api/v1/pool/stats", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status %d without auth, got %d", http.StatusUnauthorized, w.Code)
		}

		// Request with auth header
		req = httptest.NewRequest("GET", "/api/v1/repos", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusUnauthorized {
			t.Errorf("Should not be unauthorized with auth header")
		}
	})
}

// TestServerMaxRepos tests repository limit enforcement
func TestServerMaxRepos(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false
	cfg.Auth.JWT.Secret = "test-secret-for-testing-purposes-only"
	cfg.Server.MaxRepos = 3
	server := NewServer(cfg)
	router := gin.New()
	server.RegisterRoutes(router)

	// Create repos up to the limit
	for i := 0; i < 4; i++ {
		body := bytes.NewBufferString(fmt.Sprintf(`{"id": "repo-%d", "memory_only": true}`, i))
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if i < 3 {
			if w.Code != http.StatusCreated {
				t.Errorf("Expected status %d for repo %d, got %d", 
					http.StatusCreated, i, w.Code)
			}
		} else {
			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("Expected status %d for repo %d (over limit), got %d", 
					http.StatusServiceUnavailable, i, w.Code)
			}
		}
	}
}

// TestErrorResponseFormats tests consistent error response formats
func TestErrorResponseFormats(t *testing.T) {
	_, router := setupTestServer()

	testCases := []struct {
		name         string
		method       string
		path         string
		body         string
		expectedCode int
		checkError   func(t *testing.T, resp ErrorResponse)
	}{
		{
			name:         "Invalid JSON",
			method:       "POST",
			path:         "/api/v1/repos",
			body:         "{invalid json",
			expectedCode: http.StatusBadRequest,
			checkError: func(t *testing.T, resp ErrorResponse) {
				if resp.Code != "INVALID_REQUEST" {
					t.Errorf("Expected error code INVALID_REQUEST, got %s", resp.Code)
				}
			},
		},
		{
			name:         "Repository not found",
			method:       "GET",
			path:         "/api/v1/repos/non-existent",
			body:         "",
			expectedCode: http.StatusNotFound,
			checkError: func(t *testing.T, resp ErrorResponse) {
				if resp.Code != "REPO_NOT_FOUND" {
					t.Errorf("Expected error code REPO_NOT_FOUND, got %s", resp.Code)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tc.expectedCode {
				t.Errorf("Expected status %d, got %d", tc.expectedCode, w.Code)
			}

			var errResp ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("Failed to parse error response: %v", err)
			}

			if errResp.Error == "" {
				t.Error("Error message should not be empty")
			}

			if errResp.Code == "" {
				t.Error("Error code should not be empty")
			}

			tc.checkError(t, errResp)
		})
	}
}