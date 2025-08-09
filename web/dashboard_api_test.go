package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caiatech/govc/auth"
	"github.com/caiatech/govc/logging"
	"github.com/caiatech/govc/pool"
)

func setupTestDashboardAPI() *DashboardHandler {
	// Create test dependencies
	poolConfig := pool.DefaultPoolConfig()
	repositoryPool := pool.NewRepositoryPool(poolConfig)
	
	jwtAuth := auth.NewJWTAuth("test-secret-key-that-is-32-chars", "test-issuer", 24*time.Hour)
	
	logConfig := logging.Config{
		Level:     logging.InfoLevel,
		Component: "test",
	}
	logger := logging.NewLogger(logConfig)

	return NewDashboardHandler(repositoryPool, jwtAuth, logger)
}

func setupTestAPIRouter() (*gin.Engine, *DashboardHandler) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := setupTestDashboardAPI()
	
	// Setup only API routes (not the HTML template route)
	api := router.Group("/api/v1")
	{
		// Dashboard API routes
		dashboardAPI := api.Group("/dashboard")
		{
			dashboardAPI.GET("/overview", handler.GetOverview)
			dashboardAPI.GET("/performance", handler.GetPerformanceMetrics)
			dashboardAPI.GET("/logs", handler.GetLogs)
		}

		// Repository management API
		repoAPI := api.Group("/repositories")
		{
			repoAPI.GET("", handler.GetRepositories)
			repoAPI.POST("", handler.CreateRepository)
			repoAPI.DELETE("/:id", handler.DeleteRepository)
		}

		// Authentication API
		authAPI := api.Group("/auth")
		{
			authAPI.GET("/info", handler.GetAuthInfo)
			authAPI.POST("/token/generate", handler.GenerateToken)
			authAPI.POST("/token/validate", handler.ValidateToken)
		}
	}
	
	return router, handler
}

func TestDashboardAPI_GetOverview(t *testing.T) {
	router, _ := setupTestAPIRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/dashboard/overview", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var response DashboardData
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.TotalRepositories)
	assert.Equal(t, 0, response.ActiveRepositories)
	assert.Equal(t, 25, response.AverageResponseTime)
	assert.GreaterOrEqual(t, response.MemoryUsage, 0)
	assert.NotNil(t, response.PerformanceHistory)
	assert.NotNil(t, response.RepositoryStats)
	assert.NotNil(t, response.AuthStats)
	
	// Test performance history structure
	assert.Len(t, response.PerformanceHistory.Labels, 10)
	assert.Len(t, response.PerformanceHistory.Values, 10)
	
	// Test repository stats
	assert.Equal(t, 0, response.RepositoryStats.Active)
	assert.Equal(t, 0, response.RepositoryStats.Idle)
	
	// Test auth stats
	assert.Equal(t, 150, response.AuthStats.JWTRate)
	assert.Equal(t, 95, response.AuthStats.CacheHitRate)
}

func TestDashboardAPI_GetRepositories(t *testing.T) {
	router, handler := setupTestAPIRouter()

	// Create test repositories
	_, err := handler.repositoryPool.Get("test-repo-1", ":memory:", true)
	require.NoError(t, err)
	_, err = handler.repositoryPool.Get("test-repo-2", ":memory:", true)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/repositories", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var repositories []RepositoryData
	err = json.Unmarshal(w.Body.Bytes(), &repositories)
	require.NoError(t, err)

	assert.Len(t, repositories, 2)
	
	// Find repositories by ID
	repoMap := make(map[string]RepositoryData)
	for _, repo := range repositories {
		repoMap[repo.ID] = repo
	}
	
	assert.Contains(t, repoMap, "test-repo-1")
	assert.Contains(t, repoMap, "test-repo-2")
	
	for _, repo := range repositories {
		assert.Equal(t, ":memory:", repo.Path)
		assert.Equal(t, "active", repo.Status)
		assert.NotZero(t, repo.LastAccessed)
		assert.NotZero(t, repo.CreatedAt)
		assert.Greater(t, repo.AccessCount, int64(0))
	}
}

func TestDashboardAPI_CreateRepository(t *testing.T) {
	tests := []struct {
		name     string
		request  CreateRepositoryRequest
		wantCode int
		wantErr  bool
	}{
		{
			name: "valid memory repository",
			request: CreateRepositoryRequest{
				ID:         "new-repo",
				Path:       ":memory:",
				MemoryOnly: true,
			},
			wantCode: http.StatusCreated,
			wantErr:  false,
		},
		{
			name: "valid file repository",
			request: CreateRepositoryRequest{
				ID:         "file-repo",
				Path:       "/tmp/test-repo",
				MemoryOnly: false,
			},
			wantCode: http.StatusCreated,
			wantErr:  false,
		},
		{
			name: "missing ID",
			request: CreateRepositoryRequest{
				Path:       ":memory:",
				MemoryOnly: true,
			},
			wantCode: http.StatusBadRequest,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := setupTestAPIRouter()

			body, _ := json.Marshal(tt.request)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/api/v1/repositories", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)

			if !tt.wantErr {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.Equal(t, "Repository created successfully", response["message"])
				assert.Equal(t, tt.request.ID, response["id"])
			}
		})
	}
}

func TestDashboardAPI_DeleteRepository(t *testing.T) {
	router, handler := setupTestAPIRouter()

	// Create a test repository first
	_, err := handler.repositoryPool.Get("delete-repo", ":memory:", true)
	require.NoError(t, err)

	// Test successful deletion
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/repositories/delete-repo", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "Repository deleted successfully", response["message"])

	// Test deletion of non-existent repository
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/api/v1/repositories/nonexistent", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDashboardAPI_GetPerformanceMetrics(t *testing.T) {
	router, handler := setupTestAPIRouter()

	// Create some repositories for metrics
	for i := 0; i < 5; i++ {
		_, err := handler.repositoryPool.Get(fmt.Sprintf("metrics-repo-%d", i), ":memory:", true)
		require.NoError(t, err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/dashboard/performance", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response PerformanceMetrics
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Test pool stats
	assert.NotNil(t, response.PoolStats)
	assert.Equal(t, 5, response.PoolStats.Total)
	assert.Equal(t, 5, response.PoolStats.Active)
	assert.Equal(t, 0, response.PoolStats.Idle)

	// Test auth stats
	assert.NotNil(t, response.AuthStats)
	assert.Equal(t, 150, response.AuthStats.JWTRate)
	assert.Equal(t, 95, response.AuthStats.CacheHitRate)

	// Test benchmarks
	assert.NotNil(t, response.Benchmarks)
	assert.Greater(t, len(response.Benchmarks), 0)
	
	// Verify benchmark structure
	for _, benchmark := range response.Benchmarks {
		assert.NotEmpty(t, benchmark.Name)
		assert.NotEmpty(t, benchmark.Value)
		assert.NotEmpty(t, benchmark.Unit)
	}
}

func TestDashboardAPI_Authentication(t *testing.T) {
	router, handler := setupTestAPIRouter()

	t.Run("GetAuthInfo", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/auth/info", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response AuthInfo
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.NotNil(t, response.User)
		assert.Equal(t, "dashboard-user", response.User.ID)
		assert.Equal(t, "dashboard", response.User.Username)
		assert.Equal(t, "dashboard@localhost", response.User.Email)
		assert.Contains(t, response.Permissions, "read")
		assert.Contains(t, response.Permissions, "write")
		assert.Contains(t, response.Permissions, "admin")
	})

	t.Run("GenerateToken", func(t *testing.T) {
		tokenReq := TokenGenerateRequest{
			UserID:      "test-user",
			Username:    "testuser",
			Email:       "test@example.com",
			Permissions: []string{"read", "write"},
		}

		body, _ := json.Marshal(tokenReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/auth/token/generate", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		token, exists := response["token"]
		assert.True(t, exists)
		assert.NotEmpty(t, token)
		assert.IsType(t, "", token)
	})

	t.Run("ValidateToken", func(t *testing.T) {
		// First generate a token
		token, err := handler.jwtAuth.GenerateToken("test-user", "testuser", "test@example.com", []string{"read", "write"})
		require.NoError(t, err)

		validateReq := TokenValidateRequest{
			Token: token,
		}

		body, _ := json.Marshal(validateReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/auth/token/validate", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.True(t, response["valid"].(bool))
		assert.Equal(t, "test-user", response["userID"])
		assert.Equal(t, "testuser", response["username"])
		assert.Equal(t, "test@example.com", response["email"])
	})

	t.Run("ValidateInvalidToken", func(t *testing.T) {
		validateReq := TokenValidateRequest{
			Token: "invalid-token",
		}

		body, _ := json.Marshal(validateReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/auth/token/validate", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestDashboardAPI_GetLogs(t *testing.T) {
	router, _ := setupTestAPIRouter()

	t.Run("GetBasicLogs", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/dashboard/logs", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var logs []LogEntry
		err := json.Unmarshal(w.Body.Bytes(), &logs)
		require.NoError(t, err)

		assert.Greater(t, len(logs), 0)
		for _, log := range logs {
			assert.NotEmpty(t, log.Level)
			assert.NotEmpty(t, log.Message)
			assert.NotZero(t, log.Timestamp)
		}
	})

	t.Run("GetLogsWithLimit", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/dashboard/logs?limit=2", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var logs []LogEntry
		err := json.Unmarshal(w.Body.Bytes(), &logs)
		require.NoError(t, err)

		assert.LessOrEqual(t, len(logs), 2)
	})

	t.Run("GetLogsWithFilter", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/dashboard/logs?level=INFO", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var logs []LogEntry
		err := json.Unmarshal(w.Body.Bytes(), &logs)
		require.NoError(t, err)

		for _, log := range logs {
			assert.Equal(t, "INFO", log.Level)
		}
	})
}

func TestDashboardAPI_ErrorHandling(t *testing.T) {
	router, _ := setupTestAPIRouter()

	t.Run("InvalidJSON", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repositories", bytes.NewBuffer([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("MissingContentType", func(t *testing.T) {
		validReq := CreateRepositoryRequest{
			ID:         "test-repo-no-content-type",
			Path:       ":memory:",
			MemoryOnly: true,
		}

		body, _ := json.Marshal(validReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repositories", bytes.NewBuffer(body))
		// Don't set Content-Type header - Gin will still accept it as JSON if it can parse it
		router.ServeHTTP(w, req)

		// Note: Gin is permissive and will accept JSON even without Content-Type header
		// so we expect success here. For stricter validation, middleware would be needed.
		assert.Equal(t, http.StatusCreated, w.Code)
	})
}

func TestDashboardAPI_ConcurrentOperations(t *testing.T) {
	router, handler := setupTestAPIRouter()

	t.Run("ConcurrentRepositoryCreation", func(t *testing.T) {
		const numRequests = 10
		results := make(chan int, numRequests)

		for i := 0; i < numRequests; i++ {
			go func(id int) {
				createReq := CreateRepositoryRequest{
					ID:         fmt.Sprintf("concurrent-repo-%d", id),
					Path:       ":memory:",
					MemoryOnly: true,
				}

				body, _ := json.Marshal(createReq)
				w := httptest.NewRecorder()
				req, _ := http.NewRequest("POST", "/api/v1/repositories", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(w, req)

				results <- w.Code
			}(i)
		}

		// Collect results
		successCount := 0
		for i := 0; i < numRequests; i++ {
			code := <-results
			if code == http.StatusCreated {
				successCount++
			}
		}

		assert.Equal(t, numRequests, successCount, "All concurrent requests should succeed")

		// Verify all repositories were created
		assert.Equal(t, numRequests, handler.repositoryPool.Size())
	})

	t.Run("ConcurrentOverviewRequests", func(t *testing.T) {
		const numRequests = 20
		results := make(chan int, numRequests)

		for i := 0; i < numRequests; i++ {
			go func() {
				w := httptest.NewRecorder()
				req, _ := http.NewRequest("GET", "/api/v1/dashboard/overview", nil)
				router.ServeHTTP(w, req)

				results <- w.Code
			}()
		}

		// Collect results
		successCount := 0
		for i := 0; i < numRequests; i++ {
			code := <-results
			if code == http.StatusOK {
				successCount++
			}
		}

		assert.Equal(t, numRequests, successCount, "All concurrent overview requests should succeed")
	})
}

func BenchmarkDashboardAPI_GetOverview(b *testing.B) {
	router, _ := setupTestAPIRouter()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/dashboard/overview", nil)
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusOK {
			b.Fatalf("Expected status 200, got %d", w.Code)
		}
	}
}

func BenchmarkDashboardAPI_GetRepositories(b *testing.B) {
	router, handler := setupTestAPIRouter()

	// Create test repositories
	for i := 0; i < 10; i++ {
		_, err := handler.repositoryPool.Get(fmt.Sprintf("bench-repo-%d", i), ":memory:", true)
		require.NoError(b, err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repositories", nil)
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusOK {
			b.Fatalf("Expected status 200, got %d", w.Code)
		}
	}
}

func BenchmarkDashboardAPI_CreateRepository(b *testing.B) {
	router, _ := setupTestAPIRouter()

	createReq := CreateRepositoryRequest{
		Path:       ":memory:",
		MemoryOnly: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		createReq.ID = fmt.Sprintf("bench-repo-%d", i)
		body, _ := json.Marshal(createReq)
		
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repositories", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusCreated {
			b.Fatalf("Expected status 201, got %d", w.Code)
		}
	}
}