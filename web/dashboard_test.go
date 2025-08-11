package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Caia-Tech/govc/auth"
	"github.com/Caia-Tech/govc/logging"
	"github.com/Caia-Tech/govc/pool"
)

func setupTestDashboard() *DashboardHandler {
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

func setupTestRouter() (*gin.Engine, *DashboardHandler) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := setupTestDashboard()
	
	// Setup routes manually without starting metrics broadcaster for tests
	setupTestDashboardRoutes(router, handler)
	
	return router, handler
}

// setupTestDashboardRoutes sets up dashboard routes without starting the metrics broadcaster
func setupTestDashboardRoutes(router *gin.Engine, handler *DashboardHandler) {
	// Serve static files
	router.Static("/static", "./web/static")

	// Dashboard main page
	router.GET("/dashboard", handler.ServeHTTP)
	router.GET("/", func(c *gin.Context) {
		c.Redirect(302, "/dashboard")
	})

	// WebSocket endpoint
	router.GET("/ws", handler.HandleWebSocket)

	// Dashboard API routes
	dashboardAPI := router.Group("/api/v1/dashboard")
	{
		dashboardAPI.GET("/overview", handler.GetOverview)
		dashboardAPI.GET("/performance", handler.GetPerformanceMetrics)
		dashboardAPI.GET("/logs", handler.GetLogs)
	}

	// Repository management API (enhanced for dashboard)
	repoAPI := router.Group("/api/v1/repositories")
	{
		repoAPI.GET("", handler.GetRepositories)
		repoAPI.POST("", handler.CreateRepository)
		repoAPI.DELETE("/:id", handler.DeleteRepository)
	}

	// Authentication API (enhanced for dashboard)
	authAPI := router.Group("/api/v1/auth")
	{
		authAPI.GET("/info", handler.GetAuthInfo)
		authAPI.POST("/token/generate", handler.GenerateToken)
		authAPI.POST("/token/validate", handler.ValidateToken)
	}
}

func TestDashboardHandler_ServeHTTP(t *testing.T) {
	router, _ := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/dashboard", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, w.Body.String(), "govc Dashboard")
	assert.Contains(t, w.Body.String(), "dashboard-container")
}

func TestDashboardHandler_GetOverview(t *testing.T) {
	router, _ := setupTestRouter()

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
	assert.Greater(t, response.MemoryUsage, 0)
	assert.NotNil(t, response.PerformanceHistory)
	assert.NotNil(t, response.RepositoryStats)
	assert.NotNil(t, response.AuthStats)
}

func TestDashboardHandler_GetRepositories(t *testing.T) {
	router, handler := setupTestRouter()

	// Create a test repository
	_, err := handler.repositoryPool.Get("test-repo", ":memory:", true)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/repositories", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var repositories []RepositoryData
	err = json.Unmarshal(w.Body.Bytes(), &repositories)
	require.NoError(t, err)

	assert.Len(t, repositories, 1)
	assert.Equal(t, "test-repo", repositories[0].ID)
	assert.Equal(t, ":memory:", repositories[0].Path)
	assert.Equal(t, "active", repositories[0].Status)
}

func TestDashboardHandler_CreateRepository(t *testing.T) {
	router, _ := setupTestRouter()

	createReq := CreateRepositoryRequest{
		ID:         "new-repo",
		Path:       ":memory:",
		MemoryOnly: true,
	}

	body, _ := json.Marshal(createReq)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/repositories", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "Repository created successfully", response["message"])
	assert.Equal(t, "new-repo", response["id"])
}

func TestDashboardHandler_CreateRepository_InvalidRequest(t *testing.T) {
	router, _ := setupTestRouter()

	// Missing required ID field
	createReq := CreateRepositoryRequest{
		Path:       ":memory:",
		MemoryOnly: true,
	}

	body, _ := json.Marshal(createReq)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/repositories", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDashboardHandler_DeleteRepository(t *testing.T) {
	router, handler := setupTestRouter()

	// Create a test repository first
	_, err := handler.repositoryPool.Get("delete-repo", ":memory:", true)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/repositories/delete-repo", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "Repository deleted successfully", response["message"])
}

func TestDashboardHandler_DeleteRepository_NotFound(t *testing.T) {
	router, _ := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/repositories/nonexistent", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDashboardHandler_GetPerformanceMetrics(t *testing.T) {
	router, _ := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/dashboard/performance", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response PerformanceMetrics
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.NotNil(t, response.PoolStats)
	assert.NotNil(t, response.AuthStats)
	assert.NotNil(t, response.Benchmarks)
	assert.Greater(t, len(response.Benchmarks), 0)
}

func TestDashboardHandler_GetAuthInfo(t *testing.T) {
	router, _ := setupTestRouter()

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
	assert.Contains(t, response.Permissions, "read")
	assert.Contains(t, response.Permissions, "write")
	assert.Contains(t, response.Permissions, "admin")
}

func TestDashboardHandler_GenerateToken(t *testing.T) {
	router, _ := setupTestRouter()

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
}

func TestDashboardHandler_ValidateToken(t *testing.T) {
	router, handler := setupTestRouter()

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
}

func TestDashboardHandler_ValidateToken_Invalid(t *testing.T) {
	router, _ := setupTestRouter()

	validateReq := TokenValidateRequest{
		Token: "invalid-token",
	}

	body, _ := json.Marshal(validateReq)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/token/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDashboardHandler_GetLogs(t *testing.T) {
	router, _ := setupTestRouter()

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
}

func TestDashboardHandler_GetLogs_WithFilter(t *testing.T) {
	router, _ := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/dashboard/logs?level=INFO&limit=5", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var logs []LogEntry
	err := json.Unmarshal(w.Body.Bytes(), &logs)
	require.NoError(t, err)

	assert.LessOrEqual(t, len(logs), 5)
	for _, log := range logs {
		assert.Equal(t, "INFO", log.Level)
	}
}

func TestDashboardHandler_WebSocket(t *testing.T) {
	router, handler := setupTestRouter()

	// Start test server
	server := httptest.NewServer(router)
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Read welcome message
	var welcomeMsg WebSocketMessage
	err = conn.ReadJSON(&welcomeMsg)
	require.NoError(t, err)

	assert.Equal(t, "activity", welcomeMsg.Type)
	assert.Equal(t, "info", welcomeMsg.Level)
	assert.Equal(t, "Connected to dashboard", welcomeMsg.Message)

	// Test broadcasting a message
	testMsg := &WebSocketMessage{
		Type:    "test",
		Message: "test message",
		Level:   "info",
	}

	handler.broadcastMessage(testMsg)

	// Read the broadcasted message
	var receivedMsg WebSocketMessage
	err = conn.ReadJSON(&receivedMsg)
	require.NoError(t, err)

	assert.Equal(t, "test", receivedMsg.Type)
	assert.Equal(t, "test message", receivedMsg.Message)
	assert.Equal(t, "info", receivedMsg.Level)
}

func TestDashboardHandler_BroadcastMetricsUpdate(t *testing.T) {
	router, handler := setupTestRouter()

	// Start test server
	server := httptest.NewServer(router)
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Read welcome message
	var welcomeMsg WebSocketMessage
	err = conn.ReadJSON(&welcomeMsg)
	require.NoError(t, err)

	// Broadcast metrics update
	handler.BroadcastMetricsUpdate()

	// Read the metrics message
	var metricsMsg WebSocketMessage
	err = conn.ReadJSON(&metricsMsg)
	require.NoError(t, err)

	assert.Equal(t, "metrics", metricsMsg.Type)
	assert.NotNil(t, metricsMsg.Data)

	// Verify the data structure (JSON unmarshaling creates map[string]interface{})
	data, ok := metricsMsg.Data.(map[string]interface{})
	assert.True(t, ok)
	assert.NotNil(t, data)
	
	// Verify key dashboard data fields are present
	assert.Contains(t, data, "totalRepositories")
	assert.Contains(t, data, "activeRepositories")
	assert.Contains(t, data, "averageResponseTime")
}

func TestDashboardHandler_BroadcastLogEntry(t *testing.T) {
	router, handler := setupTestRouter()

	// Start test server
	server := httptest.NewServer(router)
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Read welcome message
	var welcomeMsg WebSocketMessage
	err = conn.ReadJSON(&welcomeMsg)
	require.NoError(t, err)

	// Broadcast log entry
	handler.BroadcastLogEntry("ERROR", "Test error message", "test-component")

	// Read the log message
	var logMsg WebSocketMessage
	err = conn.ReadJSON(&logMsg)
	require.NoError(t, err)

	assert.Equal(t, "log", logMsg.Type)
	assert.Equal(t, "ERROR", logMsg.Level)
	assert.Equal(t, "Test error message", logMsg.Message)
	assert.NotZero(t, logMsg.Timestamp)
}

func TestCreateRepositoryRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request CreateRepositoryRequest
		wantErr bool
	}{
		{
			name: "valid memory repository",
			request: CreateRepositoryRequest{
				ID:         "test-repo",
				Path:       ":memory:",
				MemoryOnly: true,
			},
			wantErr: false,
		},
		{
			name: "valid file repository",
			request: CreateRepositoryRequest{
				ID:         "test-repo",
				Path:       "/tmp/test-repo",
				MemoryOnly: false,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			request: CreateRepositoryRequest{
				Path:       ":memory:",
				MemoryOnly: true,
			},
			wantErr: true,
		},
	}

	router, _ := setupTestRouter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/api/v1/repositories", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)

			if tt.wantErr {
				assert.Equal(t, http.StatusBadRequest, w.Code)
			} else {
				assert.Equal(t, http.StatusCreated, w.Code)
			}
		})
	}
}

func TestDashboardHandler_ConcurrentWebSocketConnections(t *testing.T) {
	router, handler := setupTestRouter()

	// Start test server
	server := httptest.NewServer(router)
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Connect multiple WebSocket clients
	const numClients = 5
	conns := make([]*websocket.Conn, numClients)

	for i := 0; i < numClients; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err)
		conns[i] = conn

		// Read welcome message
		var welcomeMsg WebSocketMessage
		err = conn.ReadJSON(&welcomeMsg)
		require.NoError(t, err)
	}

	// Broadcast a message to all clients
	testMsg := &WebSocketMessage{
		Type:    "broadcast-test",
		Message: "message to all clients",
		Level:   "info",
	}

	handler.broadcastMessage(testMsg)

	// Verify all clients receive the message
	for i, conn := range conns {
		var receivedMsg WebSocketMessage
		err := conn.ReadJSON(&receivedMsg)
		require.NoError(t, err, "Client %d should receive the message", i)

		assert.Equal(t, "broadcast-test", receivedMsg.Type)
		assert.Equal(t, "message to all clients", receivedMsg.Message)

		conn.Close()
	}
}

func BenchmarkDashboardHandler_GetOverview(b *testing.B) {
	router, _ := setupTestRouter()

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

func BenchmarkDashboardHandler_GetRepositories(b *testing.B) {
	router, handler := setupTestRouter()

	// Create some test repositories
	for i := 0; i < 10; i++ {
		_, err := handler.repositoryPool.Get(
			fmt.Sprintf("repo-%d", i),
			":memory:",
			true,
		)
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

func BenchmarkDashboardHandler_WebSocketBroadcast(b *testing.B) {
	_, handler := setupTestRouter()

	// Create a test message
	testMsg := &WebSocketMessage{
		Type:    "benchmark-test",
		Message: "benchmark message",
		Level:   "info",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.broadcastMessage(testMsg)
	}
}