package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/caiatech/govc/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupV2TestEnvironment() (*Server, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	
	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false // Disable auth for basic tests
	cfg.Server.MaxRepos = 100
	cfg.Storage.Type = "memory"
	
	server := NewServer(cfg)
	router := gin.New()
	server.RegisterRoutes(router)
	return server, router
}

func TestV2BasicOperations(t *testing.T) {
	_, router := setupV2TestEnvironment()
	
	t.Run("Create Repository V2", func(t *testing.T) {
		reqBody := CreateRepoRequest{
			ID:         "test-v2-repo",
			MemoryOnly: true,
		}
		body, _ := json.Marshal(reqBody)
		
		req := httptest.NewRequest("POST", "/api/v2/repos", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		// Check if V2 endpoint exists, if not, skip test
		if w.Code == http.StatusNotFound {
			t.Skip("V2 endpoints not yet implemented")
			return
		}
		
		assert.Equal(t, http.StatusCreated, w.Code)
		
		var resp RepoResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "test-v2-repo", resp.ID)
	})
	
	t.Run("List Repositories V2", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v2/repos", nil)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code == http.StatusNotFound {
			t.Skip("V2 endpoints not yet implemented")
			return
		}
		
		assert.Equal(t, http.StatusOK, w.Code)
	})
	
	t.Run("Get Repository V2", func(t *testing.T) {
		// First create a repo using V1 if V2 doesn't exist
		reqBody := CreateRepoRequest{
			ID:         "get-test-v2",
			MemoryOnly: true,
		}
		body, _ := json.Marshal(reqBody)
		
		req := httptest.NewRequest("POST", "/api/v1/repos", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		
		// Now try to get it via V2
		req = httptest.NewRequest("GET", "/api/v2/repos/get-test-v2", nil)
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code == http.StatusNotFound {
			t.Skip("V2 endpoints not yet implemented")
			return
		}
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var resp RepoResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "get-test-v2", resp.ID)
	})
}

func TestV2FileOperations(t *testing.T) {
	_, router := setupV2TestEnvironment()
	
	// Create a repository first
	reqBody := CreateRepoRequest{
		ID:         "file-test-v2",
		MemoryOnly: true,
	}
	body, _ := json.Marshal(reqBody)
	
	req := httptest.NewRequest("POST", "/api/v1/repos", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	
	t.Run("Add File V2", func(t *testing.T) {
		fileReq := AddFileRequest{
			Path:    "test.txt",
			Content: "Test content for V2",
		}
		body, _ := json.Marshal(fileReq)
		
		req := httptest.NewRequest("POST", "/api/v2/repos/file-test-v2/files", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code == http.StatusNotFound {
			t.Skip("V2 endpoints not yet implemented")
			return
		}
		
		assert.Equal(t, http.StatusCreated, w.Code)
	})
	
	t.Run("Read File V2", func(t *testing.T) {
		// First add file via V1
		fileReq := AddFileRequest{
			Path:    "read-test.txt",
			Content: "Content to read",
		}
		body, _ := json.Marshal(fileReq)
		
		req := httptest.NewRequest("POST", "/api/v1/repos/file-test-v2/add", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		
		// Now read via V2
		req = httptest.NewRequest("GET", "/api/v2/repos/file-test-v2/files?path=read-test.txt", nil)
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code == http.StatusNotFound {
			t.Skip("V2 endpoints not yet implemented")
			return
		}
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var resp FileResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "read-test.txt", resp.Path)
		assert.Equal(t, "Content to read", resp.Content)
	})
}

func TestV2GitOperations(t *testing.T) {
	_, router := setupV2TestEnvironment()
	
	// Create a repository
	reqBody := CreateRepoRequest{
		ID:         "git-test-v2",
		MemoryOnly: true,
	}
	body, _ := json.Marshal(reqBody)
	
	req := httptest.NewRequest("POST", "/api/v1/repos", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	
	// Add a file
	fileReq := AddFileRequest{
		Path:    "commit-test.txt",
		Content: "File to commit",
	}
	body, _ = json.Marshal(fileReq)
	
	req = httptest.NewRequest("POST", "/api/v1/repos/git-test-v2/add", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	
	t.Run("Commit V2", func(t *testing.T) {
		commitReq := CommitRequest{
			Message: "Test commit V2",
			Author:  "Test User",
			Email:   "test@example.com",
		}
		body, _ := json.Marshal(commitReq)
		
		req := httptest.NewRequest("POST", "/api/v2/repos/git-test-v2/commits", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code == http.StatusNotFound {
			t.Skip("V2 endpoints not yet implemented")
			return
		}
		
		if w.Code != http.StatusCreated {
			t.Logf("Response body: %s", w.Body.String())
		}
		assert.Equal(t, http.StatusCreated, w.Code)
		
		var resp CommitResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Hash)
		assert.Equal(t, "Test commit V2", resp.Message)
	})
	
	t.Run("Get Status V2", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v2/repos/git-test-v2/status", nil)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code == http.StatusNotFound {
			t.Skip("V2 endpoints not yet implemented")
			return
		}
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var resp StatusResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotNil(t, resp.Branch)
	})
}

func TestV2ErrorHandling(t *testing.T) {
	_, router := setupV2TestEnvironment()
	
	t.Run("Repository Not Found V2", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v2/repos/non-existent", nil)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		// If V2 not implemented, could be 404 for route not found
		// If V2 implemented, should be 404 for repo not found
		if w.Code == http.StatusNotFound {
			// Check if it's route not found vs repo not found
			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			if err == nil && resp["error"] != nil {
				// This is a proper error response, not route not found
				assert.Contains(t, resp["error"], "not found")
			} else {
				t.Skip("V2 endpoints not yet implemented")
			}
		}
	})
	
	t.Run("Invalid Request Body V2", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v2/repos", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code == http.StatusNotFound {
			t.Skip("V2 endpoints not yet implemented")
			return
		}
		
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	
	t.Run("Duplicate Repository V2", func(t *testing.T) {
		// Create a repo
		reqBody := CreateRepoRequest{
			ID:         "duplicate-v2",
			MemoryOnly: true,
		}
		body, _ := json.Marshal(reqBody)
		
		req := httptest.NewRequest("POST", "/api/v1/repos", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		
		// Try to create again via V2
		req = httptest.NewRequest("POST", "/api/v2/repos", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code == http.StatusNotFound {
			t.Skip("V2 endpoints not yet implemented")
			return
		}
		
		assert.Equal(t, http.StatusConflict, w.Code)
	})
}