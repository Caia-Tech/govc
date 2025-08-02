package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResetEndpoint(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "test-reset-repo")
	createInitialCommit(t, router, "test-reset-repo")

	// Create additional commits
	// Second commit
	addReq := AddFileRequest{Path: "file1.txt", Content: "Modified content"}
	reqBody, _ := json.Marshal(addReq)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/repos/test-reset-repo/add", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to add file: %d", w.Code)
	}

	commitReq := CommitRequest{Message: "Second commit"}
	reqBody, _ = json.Marshal(commitReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-reset-repo/commit", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create commit: %d", w.Code)
	}

	var commit1 CommitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &commit1); err != nil {
		t.Fatalf("Failed to parse commit response: %v", err)
	}

	// Third commit
	addReq = AddFileRequest{Path: "file2.txt", Content: "Second file"}
	reqBody, _ = json.Marshal(addReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-reset-repo/add", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to add file: %d", w.Code)
	}

	commitReq = CommitRequest{Message: "Third commit"}
	reqBody, _ = json.Marshal(commitReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-reset-repo/commit", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create commit: %d", w.Code)
	}

	var commit2 CommitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &commit2); err != nil {
		t.Fatalf("Failed to parse commit response: %v", err)
	}

	// Fourth commit
	addReq = AddFileRequest{Path: "file3.txt", Content: "Third file"}
	reqBody, _ = json.Marshal(addReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-reset-repo/add", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to add file: %d", w.Code)
	}

	commitReq = CommitRequest{Message: "Fourth commit"}
	reqBody, _ = json.Marshal(commitReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-reset-repo/commit", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create commit: %d", w.Code)
	}

	var commit3 CommitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &commit3); err != nil {
		t.Fatalf("Failed to parse commit response: %v", err)
	}

	t.Run("Reset to previous commit (mixed mode)", func(t *testing.T) {
		resetReq := ResetRequest{
			Target: commit2.Hash,
			Mode:   "mixed",
		}
		reqBody, _ := json.Marshal(resetReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/test-reset-repo/reset", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
			return
		}

		var resetResp ResetResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resetResp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resetResp.OldHead != commit3.Hash {
			t.Errorf("Expected old head %s, got %s", commit3.Hash, resetResp.OldHead)
		}
		if resetResp.NewHead != commit2.Hash {
			t.Errorf("Expected new head %s, got %s", commit2.Hash, resetResp.NewHead)
		}
		if resetResp.Mode != "mixed" {
			t.Errorf("Expected mode 'mixed', got '%s'", resetResp.Mode)
		}

		// Verify HEAD is at commit2
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/v1/repos/test-reset-repo/log", nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var logResp LogResponse
		if err := json.Unmarshal(w.Body.Bytes(), &logResp); err != nil {
			t.Fatalf("Failed to parse log response: %v", err)
		}

		if len(logResp.Commits) < 2 {
			t.Errorf("Expected at least 2 commits in log, got %d", len(logResp.Commits))
		} else if logResp.Commits[0].Hash != commit2.Hash {
			t.Errorf("Expected HEAD at %s, got %s", commit2.Hash, logResp.Commits[0].Hash)
		}
	})

	t.Run("Reset with default mode", func(t *testing.T) {
		// Reset to commit1 without specifying mode (should default to mixed)
		resetReq := ResetRequest{
			Target: commit1.Hash,
		}
		reqBody, _ := json.Marshal(resetReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/test-reset-repo/reset", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resetResp ResetResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resetResp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resetResp.Mode != "mixed" {
			t.Errorf("Expected default mode 'mixed', got '%s'", resetResp.Mode)
		}
	})

	t.Run("Reset to invalid target", func(t *testing.T) {
		resetReq := ResetRequest{
			Target: "invalid-hash",
			Mode:   "mixed",
		}
		reqBody, _ := json.Marshal(resetReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/test-reset-repo/reset", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
		}

		var errResp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("Failed to parse error response: %v", err)
		}

		if errResp.Code != "RESET_FAILED" {
			t.Errorf("Expected error code 'RESET_FAILED', got '%s'", errResp.Code)
		}
	})

	t.Run("Reset without target", func(t *testing.T) {
		resetReq := ResetRequest{
			// Target is required
			Mode: "mixed",
		}
		reqBody, _ := json.Marshal(resetReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/test-reset-repo/reset", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}

		var errResp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("Failed to parse error response: %v", err)
		}

		if errResp.Code != "INVALID_REQUEST" {
			t.Errorf("Expected error code 'INVALID_REQUEST', got '%s'", errResp.Code)
		}
	})
}

func TestRebaseEndpoint(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "test-rebase-repo")
	createInitialCommit(t, router, "test-rebase-repo")

	// Get initial commit hash for later reference
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/repos/test-rebase-repo/log", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to get log: %d", w.Code)
	}

	var initialLog LogResponse
	if err := json.Unmarshal(w.Body.Bytes(), &initialLog); err != nil {
		t.Fatalf("Failed to parse log response: %v", err)
	}
	_ = initialLog.Commits[0] // Reference commit for later use

	// Create feature branch
	createBranchReq := CreateBranchRequest{Name: "feature"}
	reqBody, _ := json.Marshal(createBranchReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-rebase-repo/branches", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create branch: %d", w.Code)
	}

	// Checkout feature branch
	checkoutReq := CheckoutRequest{Branch: "feature"}
	reqBody, _ = json.Marshal(checkoutReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-rebase-repo/checkout", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to checkout branch: %d", w.Code)
	}

	// Make commits on feature branch
	addReq := AddFileRequest{Path: "feature1.txt", Content: "Feature 1"}
	reqBody, _ = json.Marshal(addReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-rebase-repo/add", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to add file: %d", w.Code)
	}

	commitReq := CommitRequest{Message: "Feature commit 1"}
	reqBody, _ = json.Marshal(commitReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-rebase-repo/commit", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create commit: %d", w.Code)
	}

	var featureCommit1 CommitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &featureCommit1); err != nil {
		t.Fatalf("Failed to parse commit response: %v", err)
	}

	addReq = AddFileRequest{Path: "feature2.txt", Content: "Feature 2"}
	reqBody, _ = json.Marshal(addReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-rebase-repo/add", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to add file: %d", w.Code)
	}

	commitReq = CommitRequest{Message: "Feature commit 2"}
	reqBody, _ = json.Marshal(commitReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-rebase-repo/commit", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create commit: %d", w.Code)
	}

	var featureCommit2 CommitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &featureCommit2); err != nil {
		t.Fatalf("Failed to parse commit response: %v", err)
	}

	// Checkout main and make another commit
	checkoutReq = CheckoutRequest{Branch: "main"}
	reqBody, _ = json.Marshal(checkoutReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-rebase-repo/checkout", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to checkout main: %d", w.Code)
	}

	addReq = AddFileRequest{Path: "main2.txt", Content: "Main content 2"}
	reqBody, _ = json.Marshal(addReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-rebase-repo/add", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to add file: %d", w.Code)
	}

	commitReq = CommitRequest{Message: "Second commit on main"}
	reqBody, _ = json.Marshal(commitReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-rebase-repo/commit", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create commit: %d", w.Code)
	}

	// Checkout feature branch again
	checkoutReq = CheckoutRequest{Branch: "feature"}
	reqBody, _ = json.Marshal(checkoutReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-rebase-repo/checkout", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to checkout feature: %d", w.Code)
	}

	t.Run("Rebase feature onto main", func(t *testing.T) {
		// Perform rebase
		rebaseReq := RebaseRequest{
			Onto: "main",
		}
		reqBody, _ := json.Marshal(rebaseReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/test-rebase-repo/rebase", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		var rebaseResp RebaseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &rebaseResp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if rebaseResp.OldHead != featureCommit2.Hash {
			t.Errorf("Expected old head %s, got %s", featureCommit2.Hash, rebaseResp.OldHead)
		}
		if rebaseResp.NewHead == featureCommit2.Hash {
			t.Errorf("Expected new head to be different from old head")
		}
		if rebaseResp.RebasedCount != 2 {
			t.Errorf("Expected 2 rebased commits, got %d", rebaseResp.RebasedCount)
		}
		if len(rebaseResp.Commits) != 2 {
			t.Errorf("Expected 2 commits in response, got %d", len(rebaseResp.Commits))
		}

		// Verify the rebased commits
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/v1/repos/test-rebase-repo/log", nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var logResp LogResponse
		if err := json.Unmarshal(w.Body.Bytes(), &logResp); err != nil {
			t.Fatalf("Failed to parse log response: %v", err)
		}

		if len(logResp.Commits) != 4 {
			t.Errorf("Expected 4 commits in log, got %d", len(logResp.Commits))
		} else {
			// Check commit order
			if logResp.Commits[0].Message != "Feature commit 2" {
				t.Errorf("Expected first commit message 'Feature commit 2', got '%s'", logResp.Commits[0].Message)
			}
			if logResp.Commits[1].Message != "Feature commit 1" {
				t.Errorf("Expected second commit message 'Feature commit 1', got '%s'", logResp.Commits[1].Message)
			}
			if logResp.Commits[2].Message != "Second commit on main" {
				t.Errorf("Expected third commit message 'Second commit on main', got '%s'", logResp.Commits[2].Message)
			}
		}
	})

	t.Run("Rebase onto non-existent branch", func(t *testing.T) {
		rebaseReq := RebaseRequest{
			Onto: "non-existent",
		}
		reqBody, _ := json.Marshal(rebaseReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/test-rebase-repo/rebase", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
		}

		var errResp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("Failed to parse error response: %v", err)
		}

		if errResp.Code != "REBASE_FAILED" {
			t.Errorf("Expected error code 'REBASE_FAILED', got '%s'", errResp.Code)
		}
	})

	t.Run("Rebase without onto field", func(t *testing.T) {
		rebaseReq := RebaseRequest{
			// Onto is required
		}
		reqBody, _ := json.Marshal(rebaseReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/test-rebase-repo/rebase", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}

		var errResp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("Failed to parse error response: %v", err)
		}

		if errResp.Code != "INVALID_REQUEST" {
			t.Errorf("Expected error code 'INVALID_REQUEST', got '%s'", errResp.Code)
		}
	})
}

func TestRebaseNoOp(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "test-rebase-noop")
	createInitialCommit(t, router, "test-rebase-noop")

	// Try to rebase main onto main (no-op)
	rebaseReq := RebaseRequest{
		Onto: "main",
	}
	reqBody, _ := json.Marshal(rebaseReq)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/repos/test-rebase-noop/rebase", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var rebaseResp RebaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &rebaseResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if rebaseResp.RebasedCount != 0 {
		t.Errorf("Expected 0 rebased commits for no-op, got %d", rebaseResp.RebasedCount)
	}
	if len(rebaseResp.Commits) != 0 {
		t.Errorf("Expected 0 commits in response for no-op, got %d", len(rebaseResp.Commits))
	}
}

func TestResetModes(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository with multiple commits
	createRepo(t, router, "test-reset-modes")
	createInitialCommit(t, router, "test-reset-modes")

	// Create second commit
	addReq := AddFileRequest{Path: "test.txt", Content: "test content"}
	reqBody, _ := json.Marshal(addReq)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/repos/test-reset-modes/add", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to add file: %d", w.Code)
	}

	commitReq := CommitRequest{Message: "Second commit"}
	reqBody, _ = json.Marshal(commitReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-reset-modes/commit", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create commit: %d", w.Code)
	}

	var commit2 CommitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &commit2); err != nil {
		t.Fatalf("Failed to parse commit response: %v", err)
	}

	// Make some working directory changes
	writeReq := WriteFileRequest{Path: "working.txt", Content: "working changes"}
	reqBody, _ = json.Marshal(writeReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-reset-modes/write", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to write file: %d", w.Code)
	}

	t.Run("Hard reset discards working changes", func(t *testing.T) {
		resetReq := ResetRequest{
			Target: "HEAD~1",
			Mode:   "hard",
		}
		reqBody, _ := json.Marshal(resetReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/test-reset-modes/reset", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var resetResp ResetResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resetResp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resetResp.Mode != "hard" {
			t.Errorf("Expected mode 'hard', got '%s'", resetResp.Mode)
		}

		// Check status should be clean after hard reset
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/v1/repos/test-reset-modes/status", nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var statusResp StatusResponse
		if err := json.Unmarshal(w.Body.Bytes(), &statusResp); err != nil {
			t.Fatalf("Failed to parse status response: %v", err)
		}

		if !statusResp.Clean {
			t.Errorf("Expected clean status after hard reset, got clean=%v", statusResp.Clean)
		}
	})

	t.Run("Invalid mode", func(t *testing.T) {
		resetReq := ResetRequest{
			Target: "HEAD",
			Mode:   "invalid-mode",
		}
		reqBody, _ := json.Marshal(resetReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/test-reset-modes/reset", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
		}

		var errResp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("Failed to parse error response: %v", err)
		}

		if errResp.Code != "RESET_FAILED" {
			t.Errorf("Expected error code 'RESET_FAILED', got '%s'", errResp.Code)
		}
	})
}
