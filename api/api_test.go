package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)
	m.Run()
}

func setupTestServer() (*Server, *gin.Engine) {
	config := Config{
		Port:       "8080",
		MaxRepos:   100,
		EnableAuth: false,
	}
	server := NewServer(config)
	router := gin.New()
	server.RegisterRoutes(router)
	return server, router
}

func TestRepositoryManagement(t *testing.T) {
	_, router := setupTestServer()

	t.Run("Create repository", func(t *testing.T) {
		body := bytes.NewBufferString(`{"id": "test-repo", "memory_only": true}`)
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
		}

		var resp RepoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.ID != "test-repo" {
			t.Errorf("Expected repo ID 'test-repo', got '%s'", resp.ID)
		}
	})

	t.Run("Get repository", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/repos/test-repo", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("List repositories", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/repos", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if count, ok := resp["count"].(float64); !ok || count < 1 {
			t.Errorf("Expected at least 1 repository, got %v", resp["count"])
		}
	})

	t.Run("Delete repository", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/v1/repos/test-repo", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Get non-existent repository", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/repos/non-existent", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestBranchOperations(t *testing.T) {
	_, router := setupTestServer()

	// Create a repository first
	createRepo(t, router, "branch-test-repo")
	
	// Create an initial commit so branches can be created
	createInitialCommit(t, router, "branch-test-repo")

	t.Run("List branches", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/repos/branch-test-repo/branches", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Create branch", func(t *testing.T) {
		body := bytes.NewBufferString(`{"name": "feature-branch"}`)
		req := httptest.NewRequest("POST", "/api/v1/repos/branch-test-repo/branches", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
		}
	})

	t.Run("Checkout branch", func(t *testing.T) {
		body := bytes.NewBufferString(`{"branch": "feature-branch"}`)
		req := httptest.NewRequest("POST", "/api/v1/repos/branch-test-repo/checkout", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
		}
	})
}

func TestTransactions(t *testing.T) {
	_, router := setupTestServer()

	// Create a repository first
	createRepo(t, router, "tx-test-repo")

	var txID string

	t.Run("Begin transaction", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/repos/tx-test-repo/transaction", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
		}

		var resp TransactionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		txID = resp.ID
	})

	t.Run("Add to transaction", func(t *testing.T) {
		body := bytes.NewBufferString(`{"path": "test.txt", "content": "test content"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/tx-test-repo/transaction/%s/add", txID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Validate transaction", func(t *testing.T) {
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/tx-test-repo/transaction/%s/validate", txID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Commit transaction", func(t *testing.T) {
		body := bytes.NewBufferString(`{"message": "Test commit"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/tx-test-repo/transaction/%s/commit", txID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
		}
	})
}

func TestParallelRealities(t *testing.T) {
	_, router := setupTestServer()

	// Create a repository first
	createRepo(t, router, "reality-test-repo")

	t.Run("Create parallel realities", func(t *testing.T) {
		body := bytes.NewBufferString(`{"branches": ["reality-1", "reality-2", "reality-3"]}`)
		req := httptest.NewRequest("POST", "/api/v1/repos/reality-test-repo/parallel-realities", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if count, ok := resp["count"].(float64); !ok || count != 3 {
			t.Errorf("Expected 3 realities, got %v", resp["count"])
		}
	})

	t.Run("List parallel realities", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/repos/reality-test-repo/parallel-realities", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})
}

func TestHealthCheck(t *testing.T) {
	_, router := setupTestServer()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if status, ok := resp["status"].(string); !ok || status != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", resp["status"])
	}
}

// Helper function to create a repository
func createRepo(t *testing.T, router *gin.Engine, repoID string) {
	body := bytes.NewBufferString(fmt.Sprintf(`{"id": "%s", "memory_only": true}`, repoID))
	req := httptest.NewRequest("POST", "/api/v1/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create repository: status %d, body: %s", w.Code, w.Body.String())
	}
}

// Helper function to create an initial commit
func createInitialCommit(t *testing.T, router *gin.Engine, repoID string) {
	// Add a file
	body := bytes.NewBufferString(`{"path": "README.md", "content": "# Test Repo"}`)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to add file: status %d, body: %s", w.Code, w.Body.String())
	}
	
	// Commit the file
	body = bytes.NewBufferString(`{"message": "Initial commit"}`)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create commit: status %d, body: %s", w.Code, w.Body.String())
	}
}