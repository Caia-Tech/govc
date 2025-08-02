package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCherryPickOperations tests cherry-pick functionality
func TestCherryPickOperations(t *testing.T) {
	_, router := setupTestServer()

	// Create a repository with some history
	repoID := "cherrypick-test"
	createRepo(t, router, repoID)

	// Create initial commit on main branch
	addReq := AddFileRequest{
		Path:    "main.txt",
		Content: "Main branch content",
	}
	body, _ := json.Marshal(addReq)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	commitBody := bytes.NewBufferString(`{"message": "Initial commit on main", "author": "Alice", "email": "alice@example.com"}`)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), commitBody)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Create a feature branch
	branchBody := bytes.NewBufferString(`{"name": "feature"}`)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), branchBody)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Checkout feature branch
	checkoutBody := bytes.NewBufferString(`{"branch": "feature"}`)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), checkoutBody)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Add a commit on feature branch
	addReq = AddFileRequest{
		Path:    "feature.txt",
		Content: "Feature content",
	}
	body, _ = json.Marshal(addReq)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	commitBody = bytes.NewBufferString(`{"message": "Add feature file", "author": "Bob", "email": "bob@example.com"}`)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), commitBody)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var featureCommit CommitResponse
	json.Unmarshal(w.Body.Bytes(), &featureCommit)

	// Switch back to main
	checkoutBody = bytes.NewBufferString(`{"branch": "main"}`)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), checkoutBody)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	t.Run("Cherry-pick commit from feature branch", func(t *testing.T) {
		cherryPickReq := CherryPickRequest{
			Commit: featureCommit.Hash,
		}
		body, _ := json.Marshal(cherryPickReq)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/cherry-pick", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusCreated, w.Code, w.Body.String())
		}

		var resp CherryPickResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		// Verify response fields
		if resp.OriginalCommit != featureCommit.Hash {
			t.Errorf("Expected original commit %s, got %s", featureCommit.Hash, resp.OriginalCommit)
		}

		if !strings.Contains(resp.Message, "Cherry-pick") {
			t.Errorf("Expected cherry-pick message, got: %s", resp.Message)
		}

		if !strings.Contains(resp.Message, "cherry picked from commit") {
			t.Errorf("Expected cherry-pick footer in message, got: %s", resp.Message)
		}

		// Verify the file was cherry-picked
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/read/feature.txt", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected feature.txt to exist after cherry-pick")
		}
	})

	t.Run("Cherry-pick non-existent commit", func(t *testing.T) {
		cherryPickReq := CherryPickRequest{
			Commit: "nonexistent",
		}
		body, _ := json.Marshal(cherryPickReq)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/cherry-pick", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})

	t.Run("Cherry-pick with HEAD reference", func(t *testing.T) {
		// Switch to feature branch to get its HEAD
		checkoutBody := bytes.NewBufferString(`{"branch": "feature"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), checkoutBody)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Add another commit
		addReq := AddFileRequest{
			Path:    "feature2.txt",
			Content: "Feature 2 content",
		}
		body, _ := json.Marshal(addReq)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		commitBody := bytes.NewBufferString(`{"message": "Add feature2 file"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), commitBody)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Get the current commit hash before switching (this will be the feature2 commit)
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/log?limit=1", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		
		var logResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &logResp)
		commits := logResp["commits"].([]interface{})
		latestCommit := commits[0].(map[string]interface{})
		feature2CommitHash := latestCommit["hash"].(string)

		// Switch back to main
		checkoutBody = bytes.NewBufferString(`{"branch": "main"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), checkoutBody)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Cherry-pick using the specific feature2 commit hash
		cherryPickReq := CherryPickRequest{
			Commit: feature2CommitHash,
		}
		body, _ = json.Marshal(cherryPickReq)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/cherry-pick", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusCreated, w.Code, w.Body.String())
		}
	})
}

// TestRevertOperations tests revert functionality
func TestRevertOperations(t *testing.T) {
	_, router := setupTestServer()

	// Create a repository with some history
	repoID := "revert-test"
	createRepo(t, router, repoID)

	// Create initial commit
	addReq := AddFileRequest{
		Path:    "file1.txt",
		Content: "Original content",
	}
	body, _ := json.Marshal(addReq)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	commitBody := bytes.NewBufferString(`{"message": "Initial commit", "author": "Alice", "email": "alice@example.com"}`)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), commitBody)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Make a change to revert
	addReq = AddFileRequest{
		Path:    "file1.txt",
		Content: "Modified content",
	}
	body, _ = json.Marshal(addReq)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Add a new file
	addReq = AddFileRequest{
		Path:    "file2.txt",
		Content: "New file content",
	}
	body, _ = json.Marshal(addReq)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	commitBody = bytes.NewBufferString(`{"message": "Modify file1 and add file2", "author": "Bob", "email": "bob@example.com"}`)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), commitBody)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var modifyCommit CommitResponse
	json.Unmarshal(w.Body.Bytes(), &modifyCommit)

	t.Run("Revert commit", func(t *testing.T) {
		revertReq := RevertRequest{
			Commit: modifyCommit.Hash,
		}
		body, _ := json.Marshal(revertReq)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/revert", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusCreated, w.Code, w.Body.String())
		}

		var resp RevertResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		// Verify response fields
		if resp.RevertedCommit != modifyCommit.Hash {
			t.Errorf("Expected reverted commit %s, got %s", modifyCommit.Hash, resp.RevertedCommit)
		}

		if !strings.Contains(resp.Message, "Revert") {
			t.Errorf("Expected revert message, got: %s", resp.Message)
		}

		// Verify file1 was reverted to original content
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/read/file1.txt", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected file1.txt to exist after revert")
		}

		var readResp ReadFileResponse
		json.Unmarshal(w.Body.Bytes(), &readResp)
		if readResp.Content != "Original content" {
			t.Errorf("Expected file1.txt to have original content, got: %s", readResp.Content)
		}

		// Verify file2 was removed
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/read/file2.txt", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected file2.txt to be removed after revert, got status %d", w.Code)
		}
	})

	t.Run("Revert non-existent commit", func(t *testing.T) {
		revertReq := RevertRequest{
			Commit: "nonexistent",
		}
		body, _ := json.Marshal(revertReq)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/revert", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})

	t.Run("Revert HEAD commit", func(t *testing.T) {
		// Get current log to verify we can revert HEAD
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/log?limit=1", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Revert HEAD
		revertReq := RevertRequest{
			Commit: "HEAD",
		}
		body, _ := json.Marshal(revertReq)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/revert", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusCreated, w.Code, w.Body.String())
		}
	})
}

// TestCherryPickRevertEdgeCases tests edge cases for cherry-pick and revert
func TestCherryPickRevertEdgeCases(t *testing.T) {
	_, router := setupTestServer()

	repoID := "edge-case-test"
	createRepo(t, router, repoID)

	t.Run("Cherry-pick root commit should fail", func(t *testing.T) {
		// Create initial commit
		addReq := AddFileRequest{
			Path:    "root.txt",
			Content: "Root content",
		}
		body, _ := json.Marshal(addReq)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		commitBody := bytes.NewBufferString(`{"message": "Root commit"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), commitBody)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var rootCommit CommitResponse
		json.Unmarshal(w.Body.Bytes(), &rootCommit)

		// Try to cherry-pick root commit
		cherryPickReq := CherryPickRequest{
			Commit: rootCommit.Hash,
		}
		body, _ = json.Marshal(cherryPickReq)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/cherry-pick", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected status %d for cherry-picking root commit, got %d", http.StatusInternalServerError, w.Code)
		}

		var errResp ErrorResponse
		json.Unmarshal(w.Body.Bytes(), &errResp)
		if !strings.Contains(errResp.Error, "cannot cherry-pick root commit") {
			t.Errorf("Expected error about root commit, got: %s", errResp.Error)
		}
	})

	t.Run("Revert root commit should fail", func(t *testing.T) {
		// Get the root commit
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/log", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var logResp struct {
			Commits []CommitResponse `json:"commits"`
		}
		json.Unmarshal(w.Body.Bytes(), &logResp)
		rootCommit := logResp.Commits[len(logResp.Commits)-1]

		// Try to revert root commit
		revertReq := RevertRequest{
			Commit: rootCommit.Hash,
		}
		body, _ := json.Marshal(revertReq)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/revert", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected status %d for reverting root commit, got %d", http.StatusInternalServerError, w.Code)
		}

		var errResp ErrorResponse
		json.Unmarshal(w.Body.Bytes(), &errResp)
		if !strings.Contains(errResp.Error, "cannot revert root commit") {
			t.Errorf("Expected error about root commit, got: %s", errResp.Error)
		}
	})
}