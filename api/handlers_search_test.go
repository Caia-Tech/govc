package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchCommits(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "test-search-repo")
	createInitialCommit(t, router, "test-search-repo")

	// Create additional commits with different messages
	addReq := AddFileRequest{Path: "feature.txt", Content: "Feature implementation"}
	reqBody, _ := json.Marshal(addReq)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/repos/test-search-repo/add", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to add file: %d", w.Code)
	}

	commitReq := CommitRequest{Message: "Add feature implementation"}
	reqBody, _ = json.Marshal(commitReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-search-repo/commit", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create commit: %d", w.Code)
	}

	// Add a bug fix commit
	addReq = AddFileRequest{Path: "fix.txt", Content: "Bug fix"}
	reqBody, _ = json.Marshal(addReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-search-repo/add", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to add file: %d", w.Code)
	}

	commitReq = CommitRequest{Message: "Fix critical bug in feature"}
	reqBody, _ = json.Marshal(commitReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-search-repo/commit", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create commit: %d", w.Code)
	}

	t.Run("Search for commits with 'feature' in message", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/test-search-repo/search/commits?query=feature", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		var resp SearchCommitsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Query != "feature" {
			t.Errorf("Expected query 'feature', got '%s'", resp.Query)
		}

		if resp.Total < 1 {
			t.Errorf("Expected at least 1 result, got %d", resp.Total)
		}

		if len(resp.Results) < 1 {
			t.Errorf("Expected at least 1 result in results array, got %d", len(resp.Results))
		}
	})

	t.Run("Search for commits with 'bug' in message", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/test-search-repo/search/commits?query=bug", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
			return
		}

		var resp SearchCommitsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Total < 1 {
			t.Errorf("Expected at least 1 result, got %d", resp.Total)
		}
	})

	t.Run("Search with no results", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/test-search-repo/search/commits?query=nonexistent", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
			return
		}

		var resp SearchCommitsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Total != 0 {
			t.Errorf("Expected 0 results, got %d", resp.Total)
		}
	})

	t.Run("Search with missing query parameter", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/test-search-repo/search/commits", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}

		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse error response: %v", err)
		}

		if resp.Code != "MISSING_QUERY" {
			t.Errorf("Expected error code 'MISSING_QUERY', got '%s'", resp.Code)
		}
	})
}

func TestSearchContent(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "test-search-content")
	createInitialCommit(t, router, "test-search-content")

	// Add files with specific content
	addReq := AddFileRequest{Path: "src/main.go", Content: "package main\n\nfunc main() {\n\tprintln(\"Hello World\")\n}"}
	reqBody, _ := json.Marshal(addReq)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/repos/test-search-content/add", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to add file: %d", w.Code)
	}

	addReq = AddFileRequest{Path: "docs/guide.md", Content: "# User Guide\n\nThis is a comprehensive guide for users."}
	reqBody, _ = json.Marshal(addReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-search-content/add", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to add file: %d", w.Code)
	}

	// Commit the files
	commitReq := CommitRequest{Message: "Add source and docs"}
	reqBody, _ = json.Marshal(commitReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-search-content/commit", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create commit: %d", w.Code)
	}

	t.Run("Search for 'main' in content", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/test-search-content/search/content?query=main", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		var resp SearchContentResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Total < 1 {
			t.Errorf("Expected at least 1 result, got %d", resp.Total)
		}
	})

	t.Run("Search for 'guide' in content", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/test-search-content/search/content?query=guide", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
			return
		}

		var resp SearchContentResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Total < 1 {
			t.Errorf("Expected at least 1 result, got %d", resp.Total)
		}

		// Check that we have a match in docs/guide.md
		found := false
		for _, result := range resp.Results {
			if result.Path == "docs/guide.md" {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Expected to find match in docs/guide.md")
		}
	})
}

func TestSearchFiles(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "test-search-files")
	createInitialCommit(t, router, "test-search-files")

	// Add files with specific names
	filenames := []string{"main.go", "helper.go", "README.md", "config.yaml"}
	for _, filename := range filenames {
		addReq := AddFileRequest{Path: filename, Content: "test content"}
		reqBody, _ := json.Marshal(addReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/test-search-files/add", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("Failed to add file %s: %d", filename, w.Code)
		}
	}

	// Commit the files
	commitReq := CommitRequest{Message: "Add test files"}
	reqBody, _ := json.Marshal(commitReq)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/repos/test-search-files/commit", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create commit: %d", w.Code)
	}

	t.Run("Search for '.go' files", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/test-search-files/search/files?query=.go", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		var resp SearchFilesResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Total != 2 {
			t.Errorf("Expected 2 results, got %d", resp.Total)
		}
	})

	t.Run("Search for 'README' files", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/test-search-files/search/files?query=README", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
			return
		}

		var resp SearchFilesResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Total != 1 {
			t.Errorf("Expected 1 result, got %d", resp.Total)
		}

		if len(resp.Results) > 0 && resp.Results[0].Path != "README.md" {
			t.Errorf("Expected README.md, got %s", resp.Results[0].Path)
		}
	})
}

func TestGrepEndpoint(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "test-grep")
	createInitialCommit(t, router, "test-grep")

	// Add a file with content for grep testing
	addReq := AddFileRequest{
		Path:    "test.txt",
		Content: "Line 1: Hello World\nLine 2: Goodbye World\nLine 3: Hello Universe\nLine 4: Test content",
	}
	reqBody, _ := json.Marshal(addReq)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/repos/test-grep/add", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to add file: %d", w.Code)
	}

	// Commit the file
	commitReq := CommitRequest{Message: "Add test file for grep"}
	reqBody, _ = json.Marshal(commitReq)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/test-grep/commit", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create commit: %d", w.Code)
	}

	t.Run("Basic grep for 'Hello'", func(t *testing.T) {
		grepReq := GrepRequest{
			Pattern: "Hello",
		}
		reqBody, _ := json.Marshal(grepReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/test-grep/grep", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		var resp GrepResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Pattern != "Hello" {
			t.Errorf("Expected pattern 'Hello', got '%s'", resp.Pattern)
		}

		if resp.Total != 2 {
			t.Errorf("Expected 2 matches, got %d", resp.Total)
		}
	})

	t.Run("Grep with context", func(t *testing.T) {
		grepReq := GrepRequest{
			Pattern: "Goodbye",
			Context: 1,
		}
		reqBody, _ := json.Marshal(grepReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/test-grep/grep", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
			return
		}

		var resp GrepResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Total != 1 {
			t.Errorf("Expected 1 match, got %d", resp.Total)
		}

		if len(resp.Results) > 0 {
			result := resp.Results[0]
			if len(result.Before) == 0 || len(result.After) == 0 {
				t.Errorf("Expected context before and after, got before=%d, after=%d", len(result.Before), len(result.After))
			}
		}
	})
}
