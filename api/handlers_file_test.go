package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestReadFileOperations tests file reading functionality
func TestReadFileOperations(t *testing.T) {
	_, router := setupTestServer()

	// Create a repository and add some files
	repoID := "read-file-test"
	createRepo(t, router, repoID)

	// Add files
	files := map[string]string{
		"README.md":       "# Test Repository",
		"src/main.go":     "package main\\n\\nfunc main() {}\\n",
		"docs/guide.md":   "# User Guide",
		"config/app.yaml": "version: 1.0\\nname: test",
	}

	for path, content := range files {
		body := bytes.NewBufferString(fmt.Sprintf(`{"path": "%s", "content": "%s"}`, path, content))
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Failed to add file %s: %d, Response: %s", path, w.Code, w.Body.String())
		}
	}

	// Commit the files
	body := bytes.NewBufferString(`{"message": "Add test files"}`)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to commit: %d", w.Code)
	}

	t.Run("Read existing file", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/read/README.md", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp ReadFileResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Path != "README.md" {
			t.Errorf("Expected path 'README.md', got '%s'", resp.Path)
		}

		if resp.Content != "# Test Repository" {
			t.Errorf("Expected content '# Test Repository', got '%s'", resp.Content)
		}

		if resp.Encoding != "utf-8" {
			t.Errorf("Expected encoding 'utf-8', got '%s'", resp.Encoding)
		}
	})

	t.Run("Read non-existent file", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/read/missing.txt", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})

	t.Run("Read file in subdirectory", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/read/src/main.go", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp ReadFileResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if resp.Path != "src/main.go" {
			t.Errorf("Expected path 'src/main.go', got '%s'", resp.Path)
		}
	})

	t.Run("Read binary file", func(t *testing.T) {
		// For now, skip binary file test as it requires different handling
		// We would need to either:
		// 1. Accept base64 encoded content in the API
		// 2. Use a different endpoint for binary files
		// 3. Use multipart form data
		t.Skip("Binary file handling needs proper implementation")
	})
}

// TestTreeOperations tests directory listing functionality
func TestTreeOperations(t *testing.T) {
	_, router := setupTestServer()

	repoID := "tree-test"
	createRepo(t, router, repoID)

	// Create a file structure
	files := map[string]string{
		"README.md":         "Root readme",
		"LICENSE":           "MIT License",
		"src/main.go":       "package main",
		"src/utils.go":      "package main",
		"src/lib/helper.go": "package lib",
		"docs/guide.md":     "Guide",
		"docs/api.md":       "API docs",
		"test/unit_test.go": "package test",
	}

	for path, content := range files {
		body := bytes.NewBufferString(fmt.Sprintf(`{"path": "%s", "content": "%s"}`, path, content))
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// Commit
	body := bytes.NewBufferString(`{"message": "Initial structure"}`)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	t.Run("List root directory", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/tree/", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp TreeResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		// Should have 5 entries: README.md, LICENSE, src/, docs/, test/
		if resp.Total < 5 {
			t.Errorf("Expected at least 5 entries in root, got %d", resp.Total)
		}
	})

	t.Run("List subdirectory", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/tree/src", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp TreeResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		// Should have at least 2 files and 1 directory
		if resp.Total < 3 {
			t.Errorf("Expected at least 3 entries in src/, got %d", resp.Total)
		}
	})

	t.Run("List with recursive flag", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/tree/?recursive=true", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp TreeResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		// Should have all 8 files
		expectedFiles := 8
		if resp.Total != expectedFiles {
			t.Errorf("Expected %d files with recursive listing, got %d", expectedFiles, resp.Total)
		}
	})

	t.Run("List non-existent directory", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/tree/missing", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			var resp TreeResponse
			json.Unmarshal(w.Body.Bytes(), &resp)
			if resp.Total != 0 {
				t.Errorf("Expected 0 entries for non-existent directory, got %d", resp.Total)
			}
		}
	})
}

// TestRemoveFileOperations tests file removal functionality
func TestRemoveFileOperations(t *testing.T) {
	_, router := setupTestServer()

	repoID := "remove-test"
	createRepo(t, router, repoID)

	// Add files
	files := []string{"file1.txt", "file2.txt", "dir/file3.txt"}
	for _, path := range files {
		body := bytes.NewBufferString(fmt.Sprintf(`{"path": "%s", "content": "test content"}`, path))
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	t.Run("Remove staged file", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/repos/%s/remove/file1.txt", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Check status to verify file was removed from staging
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var status StatusResponse
		json.Unmarshal(w.Body.Bytes(), &status)

		// file1.txt should not be in staged files
		for _, file := range status.Staged {
			if file == "file1.txt" {
				t.Error("file1.txt should not be in staged files after removal")
			}
		}
	})

	t.Run("Remove non-existent file", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/repos/%s/remove/missing.txt", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should succeed even for non-existent files (like git rm)
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d or %d, got %d", http.StatusOK, http.StatusNotFound, w.Code)
		}
	})

	t.Run("Remove with empty path", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/repos/%s/remove/", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d or %d, got %d", http.StatusBadRequest, http.StatusNotFound, w.Code)
		}
	})
}

// TestMoveFileOperations tests file move/rename functionality
func TestMoveFileOperations(t *testing.T) {
	_, router := setupTestServer()

	repoID := "move-test"
	createRepo(t, router, repoID)

	// Add and commit a file
	body := bytes.NewBufferString(`{"path": "old.txt", "content": "original content"}`)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body = bytes.NewBufferString(`{"message": "Add old.txt"}`)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	t.Run("Move existing file", func(t *testing.T) {
		body := bytes.NewBufferString(`{"from": "old.txt", "to": "new.txt"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/move", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
		}

		// Verify the file was moved
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var status StatusResponse
		json.Unmarshal(w.Body.Bytes(), &status)

		// new.txt should be in staged files
		found := false
		for _, file := range status.Staged {
			if file == "new.txt" {
				found = true
				break
			}
		}
		if !found {
			t.Error("new.txt should be in staged files after move")
		}
	})

	t.Run("Move non-existent file", func(t *testing.T) {
		body := bytes.NewBufferString(`{"from": "missing.txt", "to": "new.txt"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/move", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})

	t.Run("Move to different directory", func(t *testing.T) {
		// First add a file
		body := bytes.NewBufferString(`{"path": "file.txt", "content": "test"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		body = bytes.NewBufferString(`{"message": "Add file.txt"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Move to subdirectory
		body = bytes.NewBufferString(`{"from": "file.txt", "to": "subdir/file.txt"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/move", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Move with invalid request", func(t *testing.T) {
		body := bytes.NewBufferString(`{"from": ""}`) // Missing 'to' field
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/move", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})
}
