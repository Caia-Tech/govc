package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestStashOperations tests stash functionality
func TestStashOperations(t *testing.T) {
	_, router := setupTestServer()

	// Create a repository with some content
	repoID := "stash-test"
	createRepo(t, router, repoID)

	// Add a file and commit it
	addReq := AddFileRequest{
		Path:    "file1.txt",
		Content: "Initial content",
	}
	body, _ := json.Marshal(addReq)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	commitBody := bytes.NewBufferString(`{"message": "Initial commit", "author": "Test User", "email": "test@example.com"}`)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), commitBody)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Modify the file and add a new file
	addReq = AddFileRequest{
		Path:    "file1.txt",
		Content: "Modified content",
	}
	body, _ = json.Marshal(addReq)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Add an untracked file
	writeReq := WriteFileRequest{
		Path:    "untracked.txt",
		Content: "Untracked content",
	}
	body, _ = json.Marshal(writeReq)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/write", repoID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	t.Run("Create stash without untracked files", func(t *testing.T) {
		stashReq := StashRequest{
			Message:          "Work in progress",
			IncludeUntracked: false,
		}
		body, _ := json.Marshal(stashReq)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/stash", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusCreated, w.Code, w.Body.String())
		}

		var resp StashResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.ID != "stash@{0}" {
			t.Errorf("Expected stash ID 'stash@{0}', got '%s'", resp.ID)
		}

		if resp.Message != "Work in progress" {
			t.Errorf("Expected message 'Work in progress', got '%s'", resp.Message)
		}

		if len(resp.Files) != 1 {
			t.Errorf("Expected 1 file in stash, got %d", len(resp.Files))
		}
	})

	t.Run("List stashes", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/stash", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp StashListResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if resp.Count != 1 {
			t.Errorf("Expected 1 stash, got %d", resp.Count)
		}
	})

	t.Run("Get specific stash", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/stash/stash@{0}", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp StashResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if resp.ID != "stash@{0}" {
			t.Errorf("Expected stash ID 'stash@{0}', got '%s'", resp.ID)
		}
	})

	t.Run("Apply stash without dropping", func(t *testing.T) {
		applyReq := StashApplyRequest{
			Drop: false,
		}
		body, _ := json.Marshal(applyReq)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/stash/stash@{0}/apply", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Verify file was restored
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var status StatusResponse
		json.Unmarshal(w.Body.Bytes(), &status)

		// Should have staged file restored
		if len(status.Staged) != 1 {
			t.Errorf("Expected 1 staged file after apply, got %d", len(status.Staged))
		}
	})

	t.Run("Stash list should still have one entry", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/stash", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var resp StashListResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if resp.Count != 1 {
			t.Errorf("Expected 1 stash after apply without drop, got %d", resp.Count)
		}
	})

	t.Run("Drop stash", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/repos/%s/stash/stash@{0}", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Verify stash was removed
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/stash", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var resp StashListResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if resp.Count != 0 {
			t.Errorf("Expected 0 stashes after drop, got %d", resp.Count)
		}
	})

	t.Run("Get non-existent stash", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/stash/stash@{99}", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

// TestStashWithUntrackedFiles tests stashing with untracked files
func TestStashWithUntrackedFiles(t *testing.T) {
	_, router := setupTestServer()

	repoID := "stash-untracked"
	createRepo(t, router, repoID)

	// Create untracked files
	for i := 1; i <= 3; i++ {
		writeReq := WriteFileRequest{
			Path:    fmt.Sprintf("untracked%d.txt", i),
			Content: fmt.Sprintf("Untracked file %d", i),
		}
		body, _ := json.Marshal(writeReq)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/write", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	t.Run("Create stash with untracked files", func(t *testing.T) {
		// First check status to see if files are detected
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var statusResp StatusResponse
		json.Unmarshal(w.Body.Bytes(), &statusResp)
		t.Logf("Status before stash: staged=%v, modified=%v, untracked=%v", statusResp.Staged, statusResp.Modified, statusResp.Untracked)
		stashReq := StashRequest{
			Message:          "Stash with untracked",
			IncludeUntracked: true,
		}
		body, _ := json.Marshal(stashReq)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/stash", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusCreated, w.Code, w.Body.String())
		}

		var resp StashResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if len(resp.Files) != 3 {
			t.Errorf("Expected 3 files in stash, got %d. Files: %v. Full response: %s", len(resp.Files), resp.Files, w.Body.String())
		}

		// Verify untracked files were removed
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var status StatusResponse
		json.Unmarshal(w.Body.Bytes(), &status)

		if len(status.Untracked) != 0 {
			t.Errorf("Expected 0 untracked files after stash, got %d", len(status.Untracked))
		}
	})

	t.Run("Apply stash with drop", func(t *testing.T) {
		applyReq := StashApplyRequest{
			Drop: true,
		}
		body, _ := json.Marshal(applyReq)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/stash/stash@{0}/apply", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Verify files were restored
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var status StatusResponse
		json.Unmarshal(w.Body.Bytes(), &status)

		if len(status.Untracked) != 3 {
			t.Errorf("Expected 3 untracked files after apply, got %d", len(status.Untracked))
		}

		// Verify stash was dropped
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/stash", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var listResp StashListResponse
		json.Unmarshal(w.Body.Bytes(), &listResp)

		if listResp.Count != 0 {
			t.Errorf("Expected 0 stashes after apply with drop, got %d", listResp.Count)
		}
	})
}

// TestMultipleStashes tests handling multiple stashes
func TestMultipleStashes(t *testing.T) {
	_, router := setupTestServer()

	repoID := "stash-multiple"
	createRepo(t, router, repoID)

	// Create multiple stashes
	for i := 0; i < 3; i++ {
		// Create a file for each stash
		writeReq := WriteFileRequest{
			Path:    fmt.Sprintf("file%d.txt", i),
			Content: fmt.Sprintf("Content %d", i),
		}
		body, _ := json.Marshal(writeReq)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/write", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Create stash
		stashReq := StashRequest{
			Message:          fmt.Sprintf("Stash %d", i),
			IncludeUntracked: true,
		}
		body, _ = json.Marshal(stashReq)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/stash", repoID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	t.Run("List multiple stashes", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/stash", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var resp StashListResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if resp.Count != 3 {
			t.Errorf("Expected 3 stashes, got %d", resp.Count)
		}

		// Verify stash IDs
		for i, stash := range resp.Stashes {
			expectedID := fmt.Sprintf("stash@{%d}", i)
			if stash.ID != expectedID {
				t.Errorf("Expected stash ID '%s', got '%s'", expectedID, stash.ID)
			}
		}
	})

	t.Run("Drop middle stash and verify renumbering", func(t *testing.T) {
		// Drop stash@{1}
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/repos/%s/stash/stash@{1}", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// List stashes again
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/stash", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var resp StashListResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if resp.Count != 2 {
			t.Errorf("Expected 2 stashes after drop, got %d", resp.Count)
		}

		// Verify renumbering
		if resp.Stashes[0].ID != "stash@{0}" {
			t.Errorf("Expected first stash ID 'stash@{0}', got '%s'", resp.Stashes[0].ID)
		}
		if resp.Stashes[1].ID != "stash@{1}" {
			t.Errorf("Expected second stash ID 'stash@{1}', got '%s'", resp.Stashes[1].ID)
		}
	})
}
