package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Caia-Tech/govc/api"
	"github.com/Caia-Tech/govc/config"
	"github.com/Caia-Tech/govc/web"
)

func setupIntegrationTest() (*gin.Engine, *api.Server) {
	gin.SetMode(gin.TestMode)

	// Create test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:           "localhost",
			Port:           0, // Use random port for testing
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
			IdleTimeout:    60 * time.Second,
			RequestTimeout: 25 * time.Second,
			MaxRequestSize: 10 * 1024 * 1024,
		},
		Auth: config.AuthConfig{
			Enabled: true,
			JWT: config.JWTConfig{
				Secret: "test-integration-secret-key-32-chars-minimum",
				Issuer: "test-integration",
				TTL:    24 * time.Hour,
			},
		},
		Pool: config.PoolConfig{
			MaxRepositories: 50,
			MaxIdleTime:     15 * time.Minute,
			CleanupInterval: 2 * time.Minute,
			EnableMetrics:   true,
		},
		Logging: config.LoggingConfig{
			Level:     "INFO",
			Format:    "text",
			Component: "integration-test",
			Output:    "stdout",
		},
		Metrics: config.MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
		Development: config.DevelopmentConfig{
			Debug:          true,
			CORSEnabled:    true,
			AllowedOrigins: []string{"*"},
		},
	}

	// Create server
	server := api.NewServer(cfg)

	// Create router and register routes
	router := gin.New()
	router.Use(gin.Recovery())
	server.RegisterRoutes(router)

	return router, server
}

func TestIntegration_FullDashboardWorkflow(t *testing.T) {
	router, server := setupIntegrationTest()
	defer server.Close()

	// Test 1: Access dashboard homepage
	t.Run("Dashboard Homepage", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/dashboard", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "govc Dashboard")
	})

	// Test 2: Get initial dashboard overview
	t.Run("Initial Dashboard Overview", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/dashboard/overview", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var overview web.DashboardData
		err := json.Unmarshal(w.Body.Bytes(), &overview)
		require.NoError(t, err)

		assert.Equal(t, 0, overview.TotalRepositories)
		assert.Equal(t, 0, overview.ActiveRepositories)
	})

	// Test 3: Create repositories and verify dashboard updates
	t.Run("Create Repositories and Verify Updates", func(t *testing.T) {
		repositories := []web.CreateRepositoryRequest{
			{ID: "repo1", Path: ":memory:", MemoryOnly: true},
			{ID: "repo2", Path: ":memory:", MemoryOnly: true},
			{ID: "repo3", Path: ":memory:", MemoryOnly: true},
		}

		// Create repositories
		for _, repo := range repositories {
			body, _ := json.Marshal(repo)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/api/v1/repositories", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusCreated, w.Code)
		}

		// Verify dashboard overview reflects the changes
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/dashboard/overview", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var overview web.DashboardData
		err := json.Unmarshal(w.Body.Bytes(), &overview)
		require.NoError(t, err)

		assert.Equal(t, 3, overview.TotalRepositories)
		assert.Equal(t, 3, overview.ActiveRepositories)

		// Verify repositories list
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/v1/repositories", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var repoList []web.RepositoryData
		err = json.Unmarshal(w.Body.Bytes(), &repoList)
		require.NoError(t, err)

		assert.Len(t, repoList, 3)
		repoIDs := make(map[string]bool)
		for _, repo := range repoList {
			repoIDs[repo.ID] = true
			assert.Equal(t, ":memory:", repo.Path)
			assert.Equal(t, "active", repo.Status)
		}
		assert.True(t, repoIDs["repo1"])
		assert.True(t, repoIDs["repo2"])
		assert.True(t, repoIDs["repo3"])
	})

	// Test 4: Test authentication workflow
	t.Run("Authentication Workflow", func(t *testing.T) {
		// Generate token
		tokenReq := web.TokenGenerateRequest{
			UserID:      "integration-user",
			Username:    "integrationuser",
			Email:       "integration@test.com",
			Permissions: []string{"read", "write", "admin"},
		}

		body, _ := json.Marshal(tokenReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/auth/token/generate", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var tokenResponse map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &tokenResponse)
		require.NoError(t, err)

		token := tokenResponse["token"].(string)
		assert.NotEmpty(t, token)

		// Validate token
		validateReq := web.TokenValidateRequest{Token: token}
		body, _ = json.Marshal(validateReq)
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("POST", "/api/v1/auth/token/validate", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var validateResponse map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &validateResponse)
		require.NoError(t, err)

		assert.True(t, validateResponse["valid"].(bool))
		assert.Equal(t, "integration-user", validateResponse["userID"])
		assert.Equal(t, "integrationuser", validateResponse["username"])
	})

	// Test 5: Test performance metrics
	t.Run("Performance Metrics", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/dashboard/performance", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var metrics web.PerformanceMetrics
		err := json.Unmarshal(w.Body.Bytes(), &metrics)
		require.NoError(t, err)

		assert.NotNil(t, metrics.PoolStats)
		assert.Equal(t, 3, metrics.PoolStats.Total)
		assert.Equal(t, 3, metrics.PoolStats.Active)
		assert.Equal(t, 0, metrics.PoolStats.Idle)

		assert.NotNil(t, metrics.AuthStats)
		assert.Greater(t, len(metrics.Benchmarks), 0)
	})
}

func TestIntegration_WebSocketRealTimeUpdates(t *testing.T) {
	router, server := setupIntegrationTest()
	defer server.Close()

	// Start test server
	testServer := httptest.NewServer(router)
	defer testServer.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/ws"

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Read welcome message
	var welcomeMsg web.WebSocketMessage
	err = conn.ReadJSON(&welcomeMsg)
	require.NoError(t, err)
	assert.Equal(t, "activity", welcomeMsg.Type)

	// Create a repository and expect WebSocket notification
	createReq := web.CreateRepositoryRequest{
		ID:         "websocket-test-repo",
		Path:       ":memory:",
		MemoryOnly: true,
	}

	body, _ := json.Marshal(createReq)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/repositories", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	// Read WebSocket notification for repository creation
	var repoUpdateMsg web.WebSocketMessage
	err = conn.ReadJSON(&repoUpdateMsg)
	require.NoError(t, err)

	assert.Equal(t, "repository_update", repoUpdateMsg.Type)
	assert.Contains(t, repoUpdateMsg.Message, "websocket-test-repo")
	assert.Contains(t, repoUpdateMsg.Message, "created")
}

func TestIntegration_ErrorHandling(t *testing.T) {
	router, server := setupIntegrationTest()
	defer server.Close()

	t.Run("Invalid JSON Request", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repositories", strings.NewReader("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Missing Required Fields", func(t *testing.T) {
		invalidReq := map[string]interface{}{
			"path":       ":memory:",
			"memoryOnly": true,
			// Missing "id" field
		}

		body, _ := json.Marshal(invalidReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repositories", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Nonexistent Repository Operations", func(t *testing.T) {
		// Try to delete nonexistent repository
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/v1/repositories/nonexistent", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestIntegration_ConcurrentOperations(t *testing.T) {
	router, server := setupIntegrationTest()
	defer server.Close()

	const numGoroutines = 10
	const opsPerGoroutine = 5

	t.Run("Concurrent Repository Creation", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines*opsPerGoroutine)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				for j := 0; j < opsPerGoroutine; j++ {
					createReq := web.CreateRepositoryRequest{
						ID:         fmt.Sprintf("concurrent-repo-%d-%d", goroutineID, j),
						Path:       ":memory:",
						MemoryOnly: true,
					}

					body, _ := json.Marshal(createReq)
					w := httptest.NewRecorder()
					req, _ := http.NewRequest("POST", "/api/v1/repositories", bytes.NewBuffer(body))
					req.Header.Set("Content-Type", "application/json")
					router.ServeHTTP(w, req)

					if w.Code != http.StatusCreated {
						errors <- fmt.Errorf("goroutine %d, op %d: expected 201, got %d", goroutineID, j, w.Code)
					}
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		var errorList []error
		for err := range errors {
			errorList = append(errorList, err)
		}

		assert.Empty(t, errorList, "Concurrent operations should not produce errors")

		// Verify all repositories were created
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repositories", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var repos []web.RepositoryData
		err := json.Unmarshal(w.Body.Bytes(), &repos)
		require.NoError(t, err)

		assert.Len(t, repos, numGoroutines*opsPerGoroutine)
	})
}