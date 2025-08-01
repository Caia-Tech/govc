package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestCompleteWorkflow tests a complete workflow from repo creation to merge
func TestCompleteWorkflow(t *testing.T) {
	_, router := setupTestServer()

	repoID := "workflow-test"

	// Step 1: Create repository
	t.Run("Create repository", func(t *testing.T) {
		body := bytes.NewBufferString(fmt.Sprintf(`{"id": "%s", "memory_only": true}`, repoID))
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to create repository: %d - %s", w.Code, w.Body.String())
		}
	})

	// Step 2: Initial commit
	t.Run("Initial commit", func(t *testing.T) {
		// Add README
		body := bytes.NewBufferString(`{"path": "README.md", "content": "# Project\nInitial version"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Add config
		body = bytes.NewBufferString(`{"path": "config.yaml", "content": "version: 1.0\nenv: development"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Commit
		body = bytes.NewBufferString(`{"message": "Initial commit with README and config"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to create initial commit: %d", w.Code)
		}
	})

	// Step 3: Create feature branch
	t.Run("Create feature branch", func(t *testing.T) {
		body := bytes.NewBufferString(`{"name": "feature/new-config"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to create branch: %d - %s", w.Code, w.Body.String())
		}
	})

	// Step 4: Work on feature branch
	t.Run("Work on feature branch", func(t *testing.T) {
		// Checkout feature branch
		body := bytes.NewBufferString(`{"branch": "feature/new-config"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Update config
		body = bytes.NewBufferString(`{"path": "config.yaml", "content": "version: 2.0\nenv: production\nfeatures:\n  - caching\n  - monitoring"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Add new feature file
		body = bytes.NewBufferString(`{"path": "features.md", "content": "# New Features\n- Caching layer\n- Monitoring dashboard"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Commit changes
		body = bytes.NewBufferString(`{"message": "Add new features and update config to v2.0"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to commit on feature branch: %d", w.Code)
		}
	})

	// Step 5: Create parallel branch for testing
	t.Run("Create test branch", func(t *testing.T) {
		// Checkout main first
		body := bytes.NewBufferString(`{"branch": "main"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Create test branch
		body = bytes.NewBufferString(`{"name": "test/performance"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Checkout test branch
		body = bytes.NewBufferString(`{"branch": "test/performance"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Add test results
		body = bytes.NewBufferString(`{"path": "tests/perf.txt", "content": "Performance test results:\n- Response time: 50ms\n- Throughput: 1000 req/s"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		body = bytes.NewBufferString(`{"message": "Add performance test results"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to commit on test branch: %d", w.Code)
		}
	})

	// Step 6: Verify branch states
	t.Run("Verify branch states", func(t *testing.T) {
		// Get log for each branch
		branches := []string{"main", "feature/new-config", "test/performance"}
		
		for _, branch := range branches {
			// Checkout branch
			body := bytes.NewBufferString(fmt.Sprintf(`{"branch": "%s"}`, branch))
			req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Get log
			req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/log?limit=10", repoID), nil)
			w = httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != 200 {
				t.Errorf("Failed to get log for branch %s: %d", branch, w.Code)
			}

			var logResp LogResponse
			json.Unmarshal(w.Body.Bytes(), &logResp)
			t.Logf("Branch %s has %d commits", branch, len(logResp.Commits))
		}
	})

	// Step 7: Merge feature branch
	t.Run("Merge feature branch", func(t *testing.T) {
		body := bytes.NewBufferString(`{"from": "feature/new-config", "to": "main"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/merge", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to merge branches: %d - %s", w.Code, w.Body.String())
		}
	})

	// Step 8: Final verification
	t.Run("Final verification", func(t *testing.T) {
		// Checkout main
		body := bytes.NewBufferString(`{"branch": "main"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Get status
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var status StatusResponse
		json.Unmarshal(w.Body.Bytes(), &status)

		if status.Branch != "main" {
			t.Errorf("Expected to be on main branch, but on %s", status.Branch)
		}

		// Verify files from feature branch are present
		// This would require implementing a file list endpoint
	})
}

// TestTransactionalWorkflow tests complex transactional operations
func TestTransactionalWorkflow(t *testing.T) {
	_, router := setupTestServer()

	repoID := "tx-workflow-test"
	createRepo(t, router, repoID)

	var mainTxID string

	// Step 1: Create main transaction
	t.Run("Create main transaction", func(t *testing.T) {
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/transaction", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to create transaction: %d", w.Code)
		}

		var txResp TransactionResponse
		json.Unmarshal(w.Body.Bytes(), &txResp)
		mainTxID = txResp.ID
	})

	// Step 2: Add multiple files in transaction
	t.Run("Add files to transaction", func(t *testing.T) {
		files := []struct {
			path    string
			content string
		}{
			{"src/main.go", "package main\n\nfunc main() {\n\tprintln(\"Hello\")\n}"},
			{"src/utils.go", "package main\n\nfunc utils() string {\n\treturn \"utils\"\n}"},
			{"tests/main_test.go", "package main\n\nimport \"testing\"\n\nfunc TestMain(t *testing.T) {}"},
			{"go.mod", "module example\n\ngo 1.21"},
			{"README.md", "# Example Project\n\nTransaction test"},
		}

		for _, file := range files {
			requestData := map[string]string{
				"path":    file.path,
				"content": file.content,
			}
			jsonData, _ := json.Marshal(requestData)
			body := bytes.NewBuffer(jsonData)
			req := httptest.NewRequest("POST", 
				fmt.Sprintf("/api/v1/repos/%s/transaction/%s/add", repoID, mainTxID), body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != 200 {
				t.Errorf("Failed to add file %s: %d", file.path, w.Code)
			}
		}
	})

	// Step 3: Validate transaction
	t.Run("Validate transaction", func(t *testing.T) {
		req := httptest.NewRequest("POST", 
			fmt.Sprintf("/api/v1/repos/%s/transaction/%s/validate", repoID, mainTxID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to validate transaction: %d", w.Code)
		}

		var validResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &validResp)
		
		// Check if the response has a "status" field set to "valid"
		if status, ok := validResp["status"].(string); !ok || status != "valid" {
			t.Errorf("Transaction validation failed: %v", validResp)
		}
	})

	// Step 4: Create another transaction (will fail)
	t.Run("Concurrent transaction handling", func(t *testing.T) {
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/transaction", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var txResp2 TransactionResponse
		json.Unmarshal(w.Body.Bytes(), &txResp2)

		// Try to add to second transaction
		body := bytes.NewBufferString(`{"path": "concurrent.txt", "content": "concurrent"}`)
		req = httptest.NewRequest("POST", 
			fmt.Sprintf("/api/v1/repos/%s/transaction/%s/add", repoID, txResp2.ID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should succeed - transactions are independent
		if w.Code != 200 {
			t.Logf("Concurrent transaction modification status: %d", w.Code)
		}
	})

	// Step 5: Commit main transaction
	t.Run("Commit transaction", func(t *testing.T) {
		body := bytes.NewBufferString(`{"message": "Add complete project structure via transaction"}`)
		req := httptest.NewRequest("POST", 
			fmt.Sprintf("/api/v1/repos/%s/transaction/%s/commit", repoID, mainTxID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to commit transaction: %d", w.Code)
		}

		var commitResp CommitResponse
		json.Unmarshal(w.Body.Bytes(), &commitResp)
		t.Logf("Transaction committed with hash: %s", commitResp.Hash)
	})

	// Step 6: Verify commit
	t.Run("Verify committed changes", func(t *testing.T) {
		// Get repository status
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var status StatusResponse
		json.Unmarshal(w.Body.Bytes(), &status)

		if status.Clean != true {
			t.Errorf("Repository should be clean after transaction commit. Status: %+v", status)
		}

		// Get log to verify commit
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/log?limit=1", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var logResp LogResponse
		json.Unmarshal(w.Body.Bytes(), &logResp)

		if len(logResp.Commits) == 0 {
			t.Fatal("No commits found after transaction")
		}

		if logResp.Commits[0].Message != "Add complete project structure via transaction" {
			t.Errorf("Unexpected commit message: %s", logResp.Commits[0].Message)
		}
	})
}

// TestParallelRealityWorkflow tests parallel reality A/B testing scenario
func TestParallelRealityWorkflow(t *testing.T) {
	_, router := setupTestServer()

	repoID := "ab-test-repo"
	createRepo(t, router, repoID)
	createInitialCommit(t, router, repoID)

	// Step 1: Set up base configuration
	t.Run("Setup base configuration", func(t *testing.T) {
		configs := map[string]string{
			"app.yaml":    "name: MyApp\nversion: 1.0",
			"cache.yaml":  "type: redis\nttl: 3600",
			"server.yaml": "port: 8080\nworkers: 4",
		}

		for path, content := range configs {
			body := bytes.NewBufferString(fmt.Sprintf(
				`{"path": "%s", "content": "%s"}`, path, content))
			req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}

		body := bytes.NewBufferString(`{"message": "Add base configuration files"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	})

	// Step 2: Create parallel realities for A/B testing
	t.Run("Create parallel realities", func(t *testing.T) {
		branches := []string{"config-aggressive", "config-balanced", "config-conservative"}
		branchesJSON, _ := json.Marshal(branches)
		body := bytes.NewBufferString(fmt.Sprintf(`{"branches": %s}`, branchesJSON))
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/parallel-realities", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to create parallel realities: %d", w.Code)
		}
	})

	// Step 3: Apply different configurations to each reality
	t.Run("Configure each reality", func(t *testing.T) {
		configs := []struct {
			reality string
			changes map[string]string
		}{
			{
				reality: "config-aggressive",
				changes: map[string]string{
					"cache.yaml":  "type: memory\nttl: 7200\nsize: 2GB",
					"server.yaml": "port: 8080\nworkers: 16\nqueue_size: 10000",
				},
			},
			{
				reality: "config-balanced",
				changes: map[string]string{
					"cache.yaml":  "type: redis\nttl: 3600\nsize: 1GB",
					"server.yaml": "port: 8080\nworkers: 8\nqueue_size: 5000",
				},
			},
			{
				reality: "config-conservative",
				changes: map[string]string{
					"cache.yaml":  "type: disk\nttl: 1800\nsize: 500MB",
					"server.yaml": "port: 8080\nworkers: 4\nqueue_size: 1000",
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

			if w.Code != 200 {
				t.Errorf("Failed to apply changes to %s: %d", cfg.reality, w.Code)
			}
		}
	})

	// Step 4: Benchmark each reality
	t.Run("Benchmark realities", func(t *testing.T) {
		realities := []string{"config-aggressive", "config-balanced", "config-conservative"}
		
		for _, reality := range realities {
			req := httptest.NewRequest("GET", 
				fmt.Sprintf("/api/v1/repos/%s/parallel-realities/%s/benchmark", repoID, reality), nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != 200 {
				t.Errorf("Failed to benchmark %s: %d", reality, w.Code)
			}

			var result map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &result)
			t.Logf("Benchmark result for %s: %v", reality, result)
		}
	})

	// Step 5: Select winner and merge
	t.Run("Merge winning configuration", func(t *testing.T) {
		// In a real scenario, you'd analyze benchmark results
		winner := "config-balanced"

		body := bytes.NewBufferString(fmt.Sprintf(
			`{"from": "parallel/%s", "to": "main"}`, winner))
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/merge", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to merge winning configuration: %d", w.Code)
		}
	})
}

// TestConcurrentOperations tests handling of concurrent API calls
func TestConcurrentOperations(t *testing.T) {
	server, router := setupTestServer()

	repoID := "concurrent-test"
	createRepo(t, router, repoID)
	createInitialCommit(t, router, repoID)

	t.Run("Concurrent file additions", func(t *testing.T) {
		const numGoroutines = 20
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		errors := make([]error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(index int) {
				defer wg.Done()

				// Each goroutine adds a different file
				body := bytes.NewBufferString(fmt.Sprintf(
					`{"path": "concurrent/file%d.txt", "content": "Content from goroutine %d"}`, 
					index, index))
				req := httptest.NewRequest("POST", 
					fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				if w.Code != 200 {
					errors[index] = fmt.Errorf("goroutine %d failed with status %d", index, w.Code)
				}
			}(i)
		}

		wg.Wait()

		// Check for errors
		errorCount := 0
		for _, err := range errors {
			if err != nil {
				t.Logf("Concurrent error: %v", err)
				errorCount++
			}
		}

		if errorCount > 0 {
			t.Errorf("%d out of %d concurrent operations failed", errorCount, numGoroutines)
		}

		// Commit all changes
		body := bytes.NewBufferString(`{"message": "Concurrent file additions"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Errorf("Failed to commit concurrent changes: %d", w.Code)
		}
	})

	t.Run("Concurrent branch operations", func(t *testing.T) {
		const numBranches = 10
		var wg sync.WaitGroup
		wg.Add(numBranches * 2) // Create and checkout

		// Create branches concurrently
		for i := 0; i < numBranches; i++ {
			go func(index int) {
				defer wg.Done()

				body := bytes.NewBufferString(fmt.Sprintf(`{"name": "concurrent-branch-%d"}`, index))
				req := httptest.NewRequest("POST", 
					fmt.Sprintf("/api/v1/repos/%s/branches", repoID), body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				if w.Code != 201 && w.Code != 409 { // 409 if branch exists
					t.Logf("Branch creation %d returned status %d", index, w.Code)
				}
			}(i)
		}

		// Checkout branches concurrently
		for i := 0; i < numBranches; i++ {
			go func(index int) {
				defer wg.Done()

				// Small delay to let branches be created
				time.Sleep(10 * time.Millisecond)

				body := bytes.NewBufferString(fmt.Sprintf(
					`{"branch": "concurrent-branch-%d"}`, index))
				req := httptest.NewRequest("POST", 
					fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				if w.Code != 200 {
					t.Logf("Branch checkout %d returned status %d", index, w.Code)
				}
			}(i)
		}

		wg.Wait()
	})

	t.Run("Repository state consistency", func(t *testing.T) {
		// Verify server state after concurrent operations
		server.mu.RLock()
		repoCount := server.repoPool.Size()
		txCount := len(server.transactions)
		server.mu.RUnlock()

		t.Logf("Final state: %d repositories, %d active transactions", repoCount, txCount)

		// Get final repository state
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var status StatusResponse
		json.Unmarshal(w.Body.Bytes(), &status)

		t.Logf("Repository on branch: %s", status.Branch)

		// List all branches
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var branchResp BranchListResponse
		json.Unmarshal(w.Body.Bytes(), &branchResp)

		t.Logf("Total branches created: %d", len(branchResp.Branches))
	})
}

// TestErrorRecovery tests error handling and recovery scenarios
func TestErrorRecovery(t *testing.T) {
	_, router := setupTestServer()

	t.Run("Transaction rollback", func(t *testing.T) {
		repoID := "rollback-test"
		createRepo(t, router, repoID)

		// Begin transaction
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/transaction", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var txResp TransactionResponse
		json.Unmarshal(w.Body.Bytes(), &txResp)

		// Add some files
		for i := 0; i < 3; i++ {
			body := bytes.NewBufferString(fmt.Sprintf(
				`{"path": "rollback%d.txt", "content": "to be rolled back"}`, i))
			req := httptest.NewRequest("POST", 
				fmt.Sprintf("/api/v1/repos/%s/transaction/%s/add", repoID, txResp.ID), body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}

		// Rollback transaction
		req = httptest.NewRequest("POST", 
			fmt.Sprintf("/api/v1/repos/%s/transaction/%s/rollback", repoID, txResp.ID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("Failed to rollback transaction: %d", w.Code)
		}

		// Verify transaction is gone
		req = httptest.NewRequest("POST", 
			fmt.Sprintf("/api/v1/repos/%s/transaction/%s/validate", repoID, txResp.ID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 404 {
			t.Errorf("Expected transaction to be gone after rollback, got status %d", w.Code)
		}
	})

	t.Run("Invalid operations recovery", func(t *testing.T) {
		repoID := "recovery-test"
		createRepo(t, router, repoID)

		// Try to checkout non-existent branch
		body := bytes.NewBufferString(`{"branch": "non-existent"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should fail gracefully
		if w.Code == 200 {
			t.Error("Checkout of non-existent branch should fail")
		}

		// Repository should still be functional
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Error("Repository should still be accessible after failed operation")
		}
	})
}