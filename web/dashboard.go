package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/Caia-Tech/govc/auth"
	"github.com/Caia-Tech/govc/logging"
	"github.com/Caia-Tech/govc/pool"
)

// DashboardHandler handles dashboard-related requests
type DashboardHandler struct {
	repositoryPool *pool.RepositoryPool
	jwtAuth        *auth.JWTAuth
	logger         *logging.Logger
	upgrader       websocket.Upgrader
	wsClients      map[*websocket.Conn]bool
	broadcast      chan []byte
}

// NewDashboardHandler creates a new dashboard handler
func NewDashboardHandler(repositoryPool *pool.RepositoryPool, jwtAuth *auth.JWTAuth, logger *logging.Logger) *DashboardHandler {
	handler := &DashboardHandler{
		repositoryPool: repositoryPool,
		jwtAuth:        jwtAuth,
		logger:         logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // In production, implement proper origin checking
			},
		},
		wsClients: make(map[*websocket.Conn]bool),
		broadcast: make(chan []byte),
	}
	
	// Start metrics collection
	handler.StartMetricsCollection()
	
	return handler
}

// DashboardData represents the main dashboard data structure
type DashboardData struct {
	TotalRepositories    int                    `json:"totalRepositories"`
	ActiveRepositories   int                    `json:"activeRepositories"`
	AverageResponseTime  int                    `json:"averageResponseTime"`
	MemoryUsage          int                    `json:"memoryUsage"`
	PerformanceHistory   *PerformanceHistory    `json:"performanceHistory"`
	RepositoryStats      *RepositoryStatsData   `json:"repositoryStats"`
	AuthStats           *AuthStatsData         `json:"authStats"`
}

// PerformanceHistory represents historical performance data
type PerformanceHistory struct {
	Labels []string `json:"labels"`
	Values []int    `json:"values"`
}

// RepositoryStatsData represents repository statistics for charts
type RepositoryStatsData struct {
	Active int `json:"active"`
	Idle   int `json:"idle"`
}

// AuthStatsData represents authentication statistics
type AuthStatsData struct {
	JWTRate       int `json:"jwtRate"`
	CacheHitRate  int `json:"cacheHitRate"`
}

// PerformanceMetrics represents detailed performance metrics
type PerformanceMetrics struct {
	PoolStats  *PoolStatsData  `json:"poolStats"`
	AuthStats  *AuthStatsData  `json:"authStats"`
	Benchmarks []BenchmarkData `json:"benchmarks"`
}

// PoolStatsData represents pool statistics
type PoolStatsData struct {
	Total  int `json:"total"`
	Active int `json:"active"`
	Idle   int `json:"idle"`
}

// BenchmarkData represents benchmark results
type BenchmarkData struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

// RepositoryData represents repository information for the UI
type RepositoryData struct {
	ID           string    `json:"id"`
	Path         string    `json:"path"`
	Status       string    `json:"status"`
	LastAccessed time.Time `json:"lastAccessed"`
	AccessCount  int64     `json:"accessCount"`
	CreatedAt    time.Time `json:"createdAt"`
}

// CreateRepositoryRequest represents a request to create a new repository
type CreateRepositoryRequest struct {
	ID         string `json:"id" binding:"required"`
	Path       string `json:"path"`
	MemoryOnly bool   `json:"memoryOnly"`
}

// AuthInfo represents authentication information
type AuthInfo struct {
	User        *UserInfo `json:"user"`
	Permissions []string  `json:"permissions"`
}

// UserInfo represents user information
type UserInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// TokenGenerateRequest represents a token generation request
type TokenGenerateRequest struct {
	UserID      string   `json:"userID" binding:"required"`
	Username    string   `json:"username" binding:"required"`
	Email       string   `json:"email" binding:"required"`
	Permissions []string `json:"permissions"`
}

// TokenValidateRequest represents a token validation request
type TokenValidateRequest struct {
	Token string `json:"token" binding:"required"`
}

// LogEntry represents a log entry for the dashboard
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Component string    `json:"component,omitempty"`
}

// WebSocketMessage represents a WebSocket message
type WebSocketMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
	Message string   `json:"message,omitempty"`
	Level   string   `json:"level,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// ServeHTTP serves the dashboard HTML page
func (h *DashboardHandler) ServeHTTP(c *gin.Context) {
	tmpl, err := template.ParseFiles("web/templates/dashboard.html")
	if err != nil {
		h.logger.ErrorWithErr("Failed to parse dashboard template", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(c.Writer, nil); err != nil {
		h.logger.ErrorWithErr("Failed to execute dashboard template", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
}

// GetOverview returns dashboard overview data
func (h *DashboardHandler) GetOverview(c *gin.Context) {
	stats := h.repositoryPool.Stats()
	
	// Get memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memoryUsageMB := int(m.Alloc / 1024 / 1024)

	// Generate mock performance history (in production, you'd store this)
	now := time.Now()
	history := &PerformanceHistory{
		Labels: make([]string, 10),
		Values: make([]int, 10),
	}
	
	for i := 0; i < 10; i++ {
		timestamp := now.Add(time.Duration(-i*30) * time.Second)
		history.Labels[9-i] = timestamp.Format("15:04:05")
		// Mock response times between 10-50ms
		history.Values[9-i] = 10 + (i * 4)
	}

	data := &DashboardData{
		TotalRepositories:   stats.TotalRepositories,
		ActiveRepositories:  stats.ActiveRepositories,
		AverageResponseTime: 25, // Mock value
		MemoryUsage:        memoryUsageMB,
		PerformanceHistory: history,
		RepositoryStats: &RepositoryStatsData{
			Active: stats.ActiveRepositories,
			Idle:   stats.IdleRepositories,
		},
		AuthStats: &AuthStatsData{
			JWTRate:      150, // Mock value
			CacheHitRate: 95,  // Mock value
		},
	}

	c.JSON(http.StatusOK, data)
}

// GetRepositories returns all repositories
func (h *DashboardHandler) GetRepositories(c *gin.Context) {
	stats := h.repositoryPool.Stats()
	repositories := make([]RepositoryData, 0, len(stats.RepositoryDetails))

	for _, detail := range stats.RepositoryDetails {
		status := "active"
		if time.Since(detail.LastAccessed) > time.Hour {
			status = "idle"
		}

		repositories = append(repositories, RepositoryData{
			ID:           detail.ID,
			Path:         detail.Path,
			Status:       status,
			LastAccessed: detail.LastAccessed,
			AccessCount:  detail.AccessCount,
			CreatedAt:    detail.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, repositories)
}

// CreateRepository creates a new repository
func (h *DashboardHandler) CreateRepository(c *gin.Context) {
	var req CreateRepositoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	path := req.Path
	if req.MemoryOnly || path == "" {
		path = ":memory:"
	}

	_, err := h.repositoryPool.Get(req.ID, path, req.MemoryOnly)
	if err != nil {
		h.logger.ErrorWithErr("Failed to create repository", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create repository: %v", err)})
		return
	}

	h.logger.Infof("Repository created: %s", req.ID)
	
	// Broadcast update to WebSocket clients
	h.broadcastMessage(&WebSocketMessage{
		Type:    "repository_update",
		Message: fmt.Sprintf("Repository '%s' created", req.ID),
	})

	c.JSON(http.StatusCreated, gin.H{
		"message": "Repository created successfully",
		"id":      req.ID,
	})
}

// DeleteRepository deletes a repository
func (h *DashboardHandler) DeleteRepository(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Repository ID is required"})
		return
	}

	if !h.repositoryPool.Remove(id) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Repository not found"})
		return
	}

	h.logger.Infof("Repository deleted: %s", id)
	
	// Broadcast update to WebSocket clients
	h.broadcastMessage(&WebSocketMessage{
		Type:    "repository_update",
		Message: fmt.Sprintf("Repository '%s' deleted", id),
	})

	c.JSON(http.StatusOK, gin.H{"message": "Repository deleted successfully"})
}

// GetPerformanceMetrics returns detailed performance metrics
func (h *DashboardHandler) GetPerformanceMetrics(c *gin.Context) {
	stats := h.repositoryPool.Stats()

	poolStats := &PoolStatsData{
		Total:  stats.TotalRepositories,
		Active: stats.ActiveRepositories,
		Idle:   stats.IdleRepositories,
	}

	authStats := &AuthStatsData{
		JWTRate:      150, // Mock value
		CacheHitRate: 95,  // Mock value
	}

	// Mock benchmark data (in production, run actual benchmarks)
	benchmarks := []BenchmarkData{
		{Name: "Repository Creation", Value: "709", Unit: "ns/op"},
		{Name: "Pool Get Operations", Value: "107", Unit: "ns/op"},
		{Name: "JWT Validation", Value: "52", Unit: "ns/op"},
		{Name: "Pool Statistics", Value: "50", Unit: "ns/op"},
		{Name: "RBAC Check", Value: "123", Unit: "ns/op"},
	}

	metrics := &PerformanceMetrics{
		PoolStats:  poolStats,
		AuthStats:  authStats,
		Benchmarks: benchmarks,
	}

	c.JSON(http.StatusOK, metrics)
}

// GetAuthInfo returns authentication information
func (h *DashboardHandler) GetAuthInfo(c *gin.Context) {
	// In a real implementation, get this from the JWT token
	authInfo := &AuthInfo{
		User: &UserInfo{
			ID:       "dashboard-user",
			Username: "dashboard",
			Email:    "dashboard@localhost",
		},
		Permissions: []string{"read", "write", "admin"},
	}

	c.JSON(http.StatusOK, authInfo)
}

// GenerateToken generates a new JWT token
func (h *DashboardHandler) GenerateToken(c *gin.Context) {
	var req TokenGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := h.jwtAuth.GenerateToken(req.UserID, req.Username, req.Email, req.Permissions)
	if err != nil {
		h.logger.ErrorWithErr("Failed to generate JWT token", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

// ValidateToken validates a JWT token
func (h *DashboardHandler) ValidateToken(c *gin.Context) {
	var req TokenValidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	claims, err := h.jwtAuth.ValidateToken(req.Token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":       true,
		"userID":      claims.UserID,
		"username":    claims.Username,
		"email":       claims.Email,
		"permissions": claims.Permissions,
	})
}

// GetLogs returns recent log entries
func (h *DashboardHandler) GetLogs(c *gin.Context) {
	limitStr := c.Query("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	levelFilter := c.Query("level")

	// Mock log entries (in production, retrieve from actual log storage)
	logs := []LogEntry{
		{
			Timestamp: time.Now().Add(-time.Minute),
			Level:     "INFO",
			Message:   "Dashboard accessed",
			Component: "web",
		},
		{
			Timestamp: time.Now().Add(-2 * time.Minute),
			Level:     "INFO",
			Message:   "Repository pool initialized",
			Component: "pool",
		},
		{
			Timestamp: time.Now().Add(-3 * time.Minute),
			Level:     "DEBUG",
			Message:   "JWT token validated successfully",
			Component: "auth",
		},
	}

	// Filter by level if specified
	if levelFilter != "" {
		filtered := make([]LogEntry, 0)
		for _, log := range logs {
			if strings.EqualFold(log.Level, levelFilter) {
				filtered = append(filtered, log)
			}
		}
		logs = filtered
	}

	// Limit results
	if len(logs) > limit {
		logs = logs[:limit]
	}

	c.JSON(http.StatusOK, logs)
}

// HandleWebSocket handles WebSocket connections
func (h *DashboardHandler) HandleWebSocket(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.ErrorWithErr("Failed to upgrade WebSocket connection", err)
		return
	}
	defer conn.Close()

	// Register client
	h.wsClients[conn] = true
	h.logger.Info("WebSocket client connected")
	
	// Record WebSocket connection
	h.RecordWebSocketConnection()

	// Send welcome message
	welcomeMsg := &WebSocketMessage{
		Type:      "activity",
		Level:     "info",
		Message:   "Connected to dashboard",
		Timestamp: time.Now(),
	}
	
	if err := conn.WriteJSON(welcomeMsg); err != nil {
		h.logger.ErrorWithErr("Failed to send welcome message", err)
		h.RecordWebSocketError()
		delete(h.wsClients, conn)
		return
	}

	// Listen for messages (ping/pong, etc.)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			h.logger.ErrorWithErr("WebSocket read error", err)
			h.RecordWebSocketError()
			h.RecordWebSocketDisconnection()
			delete(h.wsClients, conn)
			break
		}
	}
}

// broadcastMessage sends a message to all connected WebSocket clients
func (h *DashboardHandler) broadcastMessage(message *WebSocketMessage) {
	if message.Timestamp.IsZero() {
		message.Timestamp = time.Now()
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		h.logger.ErrorWithErr("Failed to marshal WebSocket message", err)
		return
	}

	for client := range h.wsClients {
		if err := client.WriteMessage(websocket.TextMessage, messageBytes); err != nil {
			h.logger.ErrorWithErr("Failed to send WebSocket message", err)
			client.Close()
			delete(h.wsClients, client)
		}
	}
}

// BroadcastMetricsUpdate broadcasts metrics updates to connected clients
func (h *DashboardHandler) BroadcastMetricsUpdate() {
	stats := h.repositoryPool.Stats()
	
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memoryUsageMB := int(m.Alloc / 1024 / 1024)

	data := &DashboardData{
		TotalRepositories:  stats.TotalRepositories,
		ActiveRepositories: stats.ActiveRepositories,
		MemoryUsage:       memoryUsageMB,
		RepositoryStats: &RepositoryStatsData{
			Active: stats.ActiveRepositories,
			Idle:   stats.IdleRepositories,
		},
	}

	message := &WebSocketMessage{
		Type: "metrics",
		Data: data,
	}

	h.broadcastMessage(message)
	h.RecordWebSocketMessage("metrics")
}

// BroadcastLogEntry broadcasts log entries to connected clients
func (h *DashboardHandler) BroadcastLogEntry(level, message, component string) {
	logMessage := &WebSocketMessage{
		Type:      "log",
		Level:     level,
		Message:   message,
		Timestamp: time.Now(),
	}

	// Add component if provided
	if component != "" {
		logMessage.Data = map[string]string{"component": component}
	}

	h.broadcastMessage(logMessage)
	h.RecordWebSocketMessage("log")
}

// StartMetricsBroadcaster starts a goroutine that periodically broadcasts metrics
func (h *DashboardHandler) StartMetricsBroadcaster() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			if len(h.wsClients) > 0 {
				h.BroadcastMetricsUpdate()
			}
		}
	}()
}