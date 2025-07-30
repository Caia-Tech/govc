package api

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Status      string            `json:"status"`
	Timestamp   time.Time         `json:"timestamp"`
	Version     string            `json:"version"`
	Uptime      string            `json:"uptime"`
	Checks      map[string]Check  `json:"checks"`
	System      SystemInfo        `json:"system"`
}

// Check represents an individual health check
type Check struct {
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Duration  string    `json:"duration,omitempty"`
}

// SystemInfo represents system information
type SystemInfo struct {
	GoVersion    string `json:"go_version"`
	NumGoroutines int   `json:"num_goroutines"`
	MemoryUsage  MemoryInfo `json:"memory"`
}

// MemoryInfo represents memory usage information
type MemoryInfo struct {
	Allocated   uint64 `json:"allocated_bytes"`
	TotalAlloc  uint64 `json:"total_allocated_bytes"`
	System      uint64 `json:"system_bytes"`
	NumGC       uint32 `json:"num_gc"`
}

var (
	startTime = time.Now()
	version   = "1.0.0-dev" // Should be set via build flags
)

// healthCheck handles the main health check endpoint
func (s *Server) healthCheck(c *gin.Context) {
	checks := s.runHealthChecks()
	
	// Determine overall status
	status := "healthy"
	for _, check := range checks {
		if check.Status != "healthy" {
			status = "unhealthy"
			break
		}
	}

	// Get system info
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	response := HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Version:   version,
		Uptime:    time.Since(startTime).String(),
		Checks:    checks,
		System: SystemInfo{
			GoVersion:     runtime.Version(),
			NumGoroutines: runtime.NumGoroutine(),
			MemoryUsage: MemoryInfo{
				Allocated:  memStats.Alloc,
				TotalAlloc: memStats.TotalAlloc,
				System:     memStats.Sys,
				NumGC:      memStats.NumGC,
			},
		},
	}

	// Return appropriate HTTP status
	httpStatus := http.StatusOK
	if status != "healthy" {
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, response)
}

// liveness handles the liveness probe (Kubernetes)
func (s *Server) liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "alive",
		"timestamp": time.Now(),
	})
}

// readiness handles the readiness probe (Kubernetes)
func (s *Server) readiness(c *gin.Context) {
	checks := s.runHealthChecks()
	
	// Check if all critical systems are ready
	ready := true
	for _, check := range checks {
		if check.Status != "healthy" {
			ready = false
			break
		}
	}

	status := "ready"
	httpStatus := http.StatusOK
	
	if !ready {
		status = "not_ready"
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, gin.H{
		"status":    status,
		"timestamp": time.Now(),
		"checks":    checks,
	})
}

// runHealthChecks executes all health checks
func (s *Server) runHealthChecks() map[string]Check {
	checks := make(map[string]Check)

	// Database/Storage check (simulate for now)
	checks["storage"] = s.checkStorage()
	
	// Memory usage check
	checks["memory"] = s.checkMemory()
	
	// Repository count check
	checks["repositories"] = s.checkRepositories()

	// Authentication system check
	if s.config.Auth.Enabled {
		checks["auth"] = s.checkAuthSystem()
	}

	return checks
}

// checkStorage simulates a storage health check
func (s *Server) checkStorage() Check {
	start := time.Now()
	
	// In a real implementation, you would check:
	// - Disk space availability
	// - Write permissions
	// - Database connectivity
	
	// Simulate check
	time.Sleep(1 * time.Millisecond)
	
	return Check{
		Status:    "healthy",
		Message:   "Storage is accessible",
		Timestamp: time.Now(),
		Duration:  time.Since(start).String(),
	}
}

// checkMemory checks memory usage
func (s *Server) checkMemory() Check {
	start := time.Now()
	
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	// Check if memory usage is within acceptable limits
	// This is a simple check - in production you'd have more sophisticated thresholds
	const maxMemoryMB = 1024 // 1GB limit
	allocatedMB := memStats.Alloc / 1024 / 1024
	
	status := "healthy"
	message := "Memory usage is normal"
	
	if allocatedMB > maxMemoryMB {
		status = "unhealthy"
		message = "Memory usage is high"
	}
	
	return Check{
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
		Duration:  time.Since(start).String(),
	}
}

// checkRepositories checks the repository management system
func (s *Server) checkRepositories() Check {
	start := time.Now()
	
	s.mu.RLock()
	repoCount := len(s.repoMetadata)
	s.mu.RUnlock()
	
	status := "healthy"
	message := "Repository system is operational"
	
	// Check if we're approaching the max repo limit
	if s.config.Server.MaxRepos > 0 && repoCount > int(float64(s.config.Server.MaxRepos)*0.9) {
		status = "warning"
		message = "Repository count is approaching limit"
	}
	
	return Check{
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
		Duration:  time.Since(start).String(),
	}
}

// checkAuthSystem checks the authentication system
func (s *Server) checkAuthSystem() Check {
	start := time.Now()
	
	// Check if auth components are initialized
	if s.jwtAuth == nil || s.rbac == nil || s.apiKeyMgr == nil {
		return Check{
			Status:    "unhealthy",
			Message:   "Authentication system not properly initialized",
			Timestamp: time.Now(),
			Duration:  time.Since(start).String(),
		}
	}
	
	// Try to validate a simple operation (creating a token)
	_, err := s.jwtAuth.GenerateToken("health-check", "health", "health@test.com", []string{"reader"})
	if err != nil {
		return Check{
			Status:    "unhealthy",
			Message:   "JWT token generation failed",
			Timestamp: time.Now(),
			Duration:  time.Since(start).String(),
		}
	}
	
	return Check{
		Status:    "healthy",
		Message:   "Authentication system is operational",
		Timestamp: time.Now(),
		Duration:  time.Since(start).String(),
	}
}


// version returns version information
func (s *Server) versionInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":    version,
		"go_version": runtime.Version(),
		"build_time": "development", // Should be set via build flags
		"git_commit": "unknown",     // Should be set via build flags
	})
}