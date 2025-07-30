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

// TestDiffOperations tests diff functionality
func TestDiffOperations(t *testing.T) {
	_, router := setupTestServer()

	// Create a repository and make some commits
	repoID := "diff-test"
	createRepo(t, router, repoID)

	// First commit
	files1 := map[string]string{
		"README.md": "# Test Project\\nThis is a test",
		"main.go":   "package main\\n\\nfunc main() {}",
	}
	
	for path, content := range files1 {
		body := bytes.NewBufferString(fmt.Sprintf(`{"path": "%s", "content": "%s"}`, path, content))
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	body := bytes.NewBufferString(`{"message": "Initial commit"}`)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	
	var commit1 CommitResponse
	json.Unmarshal(w.Body.Bytes(), &commit1)

	// Second commit with changes
	files2 := map[string]string{
		"README.md": "# Test Project\\nThis is a test\\n\\nNew section added",
		"config.yml": "version: 1.0",
	}
	
	for path, content := range files2 {
		body := bytes.NewBufferString(fmt.Sprintf(`{"path": "%s", "content": "%s"}`, path, content))
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	body = bytes.NewBufferString(`{"message": "Update README and add config"}`)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	
	var commit2 CommitResponse
	json.Unmarshal(w.Body.Bytes(), &commit2)

	t.Run("Diff between commits with path params", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/diff/HEAD~1/HEAD", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var resp DiffResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if resp.Format != "unified" {
			t.Errorf("Expected unified format, got %s", resp.Format)
		}

		// Check diff contains expected changes
		if !strings.Contains(resp.Diff, "README.md") {
			t.Error("Diff should contain README.md changes")
		}
		if !strings.Contains(resp.Diff, "config.yml") {
			t.Error("Diff should contain new config.yml file")
		}
	})

	t.Run("Diff between commits with query params", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/diff?from=HEAD~1&to=HEAD", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Diff with different formats", func(t *testing.T) {
		formats := []string{"unified", "raw", "name-only"}
		
		for _, format := range formats {
			req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/diff?from=HEAD~1&to=HEAD&format=%s", 
				repoID, format), nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d for format %s, got %d", http.StatusOK, format, w.Code)
			}

			var resp DiffResponse
			json.Unmarshal(w.Body.Bytes(), &resp)

			if resp.Format != format {
				t.Errorf("Expected format %s, got %s", format, resp.Format)
			}

			// Verify format-specific output
			switch format {
			case "raw":
				if !strings.Contains(resp.Diff, "M\tREADME.md") {
					t.Error("Raw format should show 'M\tREADME.md'")
				}
				if !strings.Contains(resp.Diff, "A\tconfig.yml") {
					t.Error("Raw format should show 'A\tconfig.yml'")
				}
			case "name-only":
				lines := strings.Split(strings.TrimSpace(resp.Diff), "\n")
				// Should contain README.md (modified) and config.yml (added)
				foundReadme := false
				foundConfig := false
				for _, line := range lines {
					if line == "README.md" {
						foundReadme = true
					} else if line == "config.yml" {
						foundConfig = true
					}
				}
				if !foundReadme {
					t.Error("Expected README.md in name-only diff")
				}
				if !foundConfig {
					t.Error("Expected config.yml in name-only diff")
				}
			}
		}
	})

	t.Run("Diff with non-existent commits", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/diff/invalid1/invalid2", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})

	t.Run("Working directory diff", func(t *testing.T) {
		// Add some changes without committing
		body := bytes.NewBufferString(`{"path": "new_file.txt", "content": "uncommitted content"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Get working diff
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/diff/working", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp WorkingDiffResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if len(resp.Staged) == 0 {
			t.Error("Expected staged changes in working diff")
		}

		found := false
		for _, diff := range resp.Staged {
			if diff.Path == "new_file.txt" && diff.Status == "added" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find new_file.txt in staged changes")
		}
	})

	t.Run("File-specific diff", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/diff/file/README.md?from=HEAD~1&to=HEAD", 
			repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp FileDiffResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if resp.Path != "README.md" {
			t.Errorf("Expected path 'README.md', got '%s'", resp.Path)
		}

		if !strings.Contains(resp.Diff, "New section added") {
			t.Error("File diff should contain the new content")
		}
	})

	t.Run("Diff for non-existent file", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/diff/file/missing.txt?from=HEAD~1&to=HEAD", 
			repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should return empty diff or 404
		if w.Code == http.StatusOK {
			var resp FileDiffResponse
			json.Unmarshal(w.Body.Bytes(), &resp)
			if resp.Diff != "" {
				t.Error("Expected empty diff for non-existent file")
			}
		}
	})
}

// TestDiffEdgeCases tests edge cases for diff operations
func TestDiffEdgeCases(t *testing.T) {
	_, router := setupTestServer()

	repoID := "diff-edge-test"
	createRepo(t, router, repoID)

	t.Run("Diff on empty repository", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/diff/HEAD/HEAD", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should fail gracefully
		if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
			t.Errorf("Expected error status, got %d", w.Code)
		}
	})

	// Make initial commit
	body := bytes.NewBufferString(`{"path": "file.txt", "content": "initial"}`)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body = bytes.NewBufferString(`{"message": "Initial"}`)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	t.Run("Diff with same commit", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/diff/HEAD/HEAD", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp DiffResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		// Should return empty diff
		if strings.TrimSpace(resp.Diff) != "" {
			t.Error("Expected empty diff when comparing same commits")
		}
	})

	t.Run("Diff with branch names", func(t *testing.T) {
		// Create a branch
		body := bytes.NewBufferString(`{"name": "feature"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Add change on feature branch
		body = bytes.NewBufferString(`{"branch": "feature"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		body = bytes.NewBufferString(`{"path": "feature.txt", "content": "feature content"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		body = bytes.NewBufferString(`{"message": "Add feature"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Diff between branches
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/diff/main/feature", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp DiffResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if !strings.Contains(resp.Diff, "feature.txt") {
			t.Error("Diff should show feature.txt added")
		}
	})
}