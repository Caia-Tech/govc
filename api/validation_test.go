package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caiatech/govc/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestInputValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	// Create test server with minimal config
	cfg := &config.Config{
		Server: config.ServerConfig{
			MaxRepos:       10,
			MaxRequestSize: 10 * 1024 * 1024,
			RequestTimeout: 30 * time.Second,
		},
		Development: config.DevelopmentConfig{
			UseNewArchitecture: true, // Use V2 endpoints which have validation
		},
		Auth: config.AuthConfig{
			Enabled: true,
		},
		Metrics: config.MetricsConfig{
			Enabled: false, // Disable metrics for tests
		},
	}
	
	server := NewServer(cfg)
	server.RegisterRoutes(router)
	
	t.Run("Invalid Repository ID", func(t *testing.T) {
		testCases := []struct {
			name       string
			repoID     string
			wantStatus int
			wantError  string
		}{
			{
				name:       "Reserved ID",
				repoID:     "admin",
				wantStatus: http.StatusBadRequest,
				wantError:  "reserved",
			},
			{
				name:       "ID Too Long",
				repoID:     strings.Repeat("a", 65),
				wantStatus: http.StatusBadRequest,
				wantError:  "must not exceed 64 characters",
			},
			{
				name:       "ID With Spaces",
				repoID:     "my repo",
				wantStatus: http.StatusBadRequest,
				wantError:  "must be alphanumeric",
			},
			{
				name:       "Reserved ID",
				repoID:     "admin",
				wantStatus: http.StatusBadRequest,
				wantError:  "reserved",
			},
			{
				name:       "ID Starting With Hyphen",
				repoID:     "-repo",
				wantStatus: http.StatusBadRequest,
				wantError:  "starting with alphanumeric",
			},
		}
		
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := CreateRepoRequest{
					ID:         tc.repoID,
					MemoryOnly: true,
				}
				
				body, _ := json.Marshal(req)
				w := httptest.NewRecorder()
				r := httptest.NewRequest("POST", "/api/v2/repos", bytes.NewBuffer(body))
				r.Header.Set("Content-Type", "application/json")
				
				router.ServeHTTP(w, r)
				
				assert.Equal(t, tc.wantStatus, w.Code)
				
				var resp ErrorResponse
				err := json.Unmarshal(w.Body.Bytes(), &resp)
				assert.NoError(t, err)
				assert.Contains(t, resp.Error, tc.wantError)
			})
		}
	})
	
	t.Run("Invalid File Path", func(t *testing.T) {
		// First create a valid repository
		req := CreateRepoRequest{
			ID:         "test-repo",
			MemoryOnly: true,
		}
		body, _ := json.Marshal(req)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v2/repos", bytes.NewBuffer(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusCreated, w.Code)
		
		// Now test invalid file paths
		testCases := []struct {
			name       string
			path       string
			wantStatus int
			wantError  string
		}{
			{
				name:       "Path Traversal",
				path:       "../../../etc/passwd",
				wantStatus: http.StatusBadRequest,
				wantError:  "path traversal",
			},
			{
				name:       "Absolute Path",
				path:       "/etc/passwd",
				wantStatus: http.StatusBadRequest,
				wantError:  "absolute paths are not allowed",
			},
			{
				name:       "Null Byte",
				path:       "file\x00.txt",
				wantStatus: http.StatusBadRequest,
				wantError:  "null bytes",
			},
			{
				name:       "Hidden File at Root",
				path:       ".git/config",
				wantStatus: http.StatusBadRequest,
				wantError:  "hidden files at root level",
			},
		}
		
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				addReq := AddFileRequest{
					Path:    tc.path,
					Content: "test content",
				}
				
				body, _ := json.Marshal(addReq)
				w := httptest.NewRecorder()
				r := httptest.NewRequest("POST", "/api/v2/repos/test-repo/files", bytes.NewBuffer(body))
				r.Header.Set("Content-Type", "application/json")
				
				router.ServeHTTP(w, r)
				
				assert.Equal(t, tc.wantStatus, w.Code)
				
				var resp ErrorResponse
				err := json.Unmarshal(w.Body.Bytes(), &resp)
				assert.NoError(t, err)
				assert.Contains(t, resp.Error, tc.wantError)
			})
		}
	})
	
	t.Run("Invalid Branch Name", func(t *testing.T) {
		testCases := []struct {
			name       string
			branchName string
			wantStatus int
			wantError  string
		}{
			{
				name:       "Branch With Space",
				branchName: "my branch",
				wantStatus: http.StatusBadRequest,
				wantError:  "cannot contain ' '",
			},
			{
				name:       "Branch With @{",
				branchName: "branch@{",
				wantStatus: http.StatusBadRequest,
				wantError:  "cannot contain '@{'",
			},
			{
				name:       "Branch Starting With Hyphen",
				branchName: "-feature",
				wantStatus: http.StatusBadRequest,
				wantError:  "cannot start or end with hyphen",
			},
			{
				name:       "Branch With Double Dot",
				branchName: "feature..test",
				wantStatus: http.StatusBadRequest,
				wantError:  "consecutive periods",
			},
		}
		
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				branchReq := CreateBranchRequest{
					Name: tc.branchName,
				}
				
				body, _ := json.Marshal(branchReq)
				w := httptest.NewRecorder()
				r := httptest.NewRequest("POST", "/api/v2/repos/test-repo/branches", bytes.NewBuffer(body))
				r.Header.Set("Content-Type", "application/json")
				
				router.ServeHTTP(w, r)
				
				assert.Equal(t, tc.wantStatus, w.Code)
				
				var resp ErrorResponse
				err := json.Unmarshal(w.Body.Bytes(), &resp)
				assert.NoError(t, err)
				assert.Contains(t, resp.Error, tc.wantError)
			})
		}
	})
	
	t.Run("Invalid Commit Message", func(t *testing.T) {
		// Add a file first
		addReq := AddFileRequest{
			Path:    "test.txt",
			Content: "test content",
		}
		body, _ := json.Marshal(addReq)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v2/repos/test-repo/files", bytes.NewBuffer(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		
		// Test invalid commit messages
		testCases := []struct {
			name       string
			message    string
			email      string
			wantStatus int
			wantError  string
		}{
			{
				name:       "Empty Message",
				message:    "",
				email:      "test@example.com",
				wantStatus: http.StatusBadRequest,
				wantError:  "required", // JSON binding validation kicks in first
			},
			{
				name:       "Message Too Long",
				message:    strings.Repeat("a", 100001),
				email:      "test@example.com",
				wantStatus: http.StatusBadRequest,
				wantError:  "too long",
			},
			{
				name:       "Invalid Email",
				message:    "Valid commit message",
				email:      "not-an-email",
				wantStatus: http.StatusBadRequest,
				wantError:  "invalid email format",
			},
		}
		
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				commitReq := CommitRequest{
					Message: tc.message,
					Author:  "Test User",
					Email:   tc.email,
				}
				
				body, _ := json.Marshal(commitReq)
				w := httptest.NewRecorder()
				r := httptest.NewRequest("POST", "/api/v2/repos/test-repo/commits", bytes.NewBuffer(body))
				r.Header.Set("Content-Type", "application/json")
				
				router.ServeHTTP(w, r)
				
				assert.Equal(t, tc.wantStatus, w.Code)
				
				var resp ErrorResponse
				err := json.Unmarshal(w.Body.Bytes(), &resp)
				assert.NoError(t, err)
				assert.Contains(t, resp.Error, tc.wantError)
			})
		}
	})
	
	t.Run("File Content Validation", func(t *testing.T) {
		// Test file too large
		largeContent := strings.Repeat("a", 51*1024*1024) // 51MB
		addReq := AddFileRequest{
			Path:    "large.txt",
			Content: largeContent,
		}
		
		body, _ := json.Marshal(addReq)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v2/repos/test-repo/files", bytes.NewBuffer(body))
		r.Header.Set("Content-Type", "application/json")
		
		router.ServeHTTP(w, r)
		
		// Should fail at request size limit first
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	})
}

func TestSecurityMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	// Create test server
	cfg := &config.Config{
		Server: config.ServerConfig{
			MaxRepos:       10,
			MaxRequestSize: 10 * 1024 * 1024,
			RequestTimeout: 30 * time.Second,
		},
		Development: config.DevelopmentConfig{
			UseNewArchitecture: true, // Use V2 endpoints which have validation
		},
		Metrics: config.MetricsConfig{
			Enabled: false, // Disable metrics for tests
		},
	}
	
	server := NewServer(cfg)
	server.RegisterRoutes(router)
	
	t.Run("Path Too Long", func(t *testing.T) {
		longPath := "/api/v1/repos/" + strings.Repeat("a", 5000)
		
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", longPath, nil)
		
		router.ServeHTTP(w, r)
		
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		
		var resp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "PATH_TOO_LONG", resp.Code)
	})
	
	t.Run("Query Too Long", func(t *testing.T) {
		longQuery := "?param=" + strings.Repeat("a", 3000)
		
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v2/repos"+longQuery, nil)
		
		router.ServeHTTP(w, r)
		
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		
		var resp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "QUERY_TOO_LONG", resp.Code)
	})
	
	t.Run("Headers Too Large", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v2/repos", nil)
		
		// Add many large headers
		for i := 0; i < 100; i++ {
			r.Header.Set(fmt.Sprintf("X-Large-Header-%d", i), strings.Repeat("a", 100))
		}
		
		router.ServeHTTP(w, r)
		
		assert.Equal(t, http.StatusRequestHeaderFieldsTooLarge, w.Code)
		
		var resp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "HEADERS_TOO_LARGE", resp.Code)
	})
	
	t.Run("Invalid Content Type", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v2/repos", strings.NewReader("data"))
		r.Header.Set("Content-Type", "application/xml")
		
		router.ServeHTTP(w, r)
		
		assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
		
		var resp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "UNSUPPORTED_CONTENT_TYPE", resp.Code)
	})
}