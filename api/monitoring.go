package api

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// MonitoringManager provides comprehensive system monitoring
type MonitoringManager struct {
	server        *Server
	metrics       *SystemMetrics
	alerts        *AlertManager
	healthHistory []HealthSnapshot
	mu            sync.RWMutex
}

// SystemMetrics tracks various system metrics
type SystemMetrics struct {
	// Performance metrics
	RequestCount      int64         `json:"request_count"`
	AverageLatency    time.Duration `json:"average_latency"`
	ErrorRate         float64       `json:"error_rate"`
	
	// Resource metrics
	MemoryUsage       uint64        `json:"memory_usage_bytes"`
	CPUUsage          float64       `json:"cpu_usage_percent"`
	GoroutineCount    int           `json:"goroutine_count"`
	
	// Repository metrics
	RepositoryCount   int           `json:"repository_count"`
	ActiveConnections int           `json:"active_connections"`
	PoolUtilization   float64       `json:"pool_utilization_percent"`
	
	// Storage metrics
	DiskUsage         uint64        `json:"disk_usage_bytes"`
	DiskFree          uint64        `json:"disk_free_bytes"`
	StorageHealth     string        `json:"storage_health"`
	
	// Cluster metrics (if enabled)
	ClusterNodes      int           `json:"cluster_nodes,omitempty"`
	HealthyNodes      int           `json:"healthy_nodes,omitempty"`
	ClusterHealth     string        `json:"cluster_health,omitempty"`
	
	Timestamp         time.Time     `json:"timestamp"`
}

// HealthSnapshot represents a point-in-time health check
type HealthSnapshot struct {
	Timestamp     time.Time              `json:"timestamp"`
	Status        string                 `json:"status"`
	ResponseTime  time.Duration          `json:"response_time"`
	Checks        map[string]Check       `json:"checks"`
	Metrics       SystemMetrics          `json:"metrics"`
}

// AlertRule defines conditions for triggering alerts
type AlertRule struct {
	Name        string        `json:"name"`
	Condition   string        `json:"condition"`   // e.g., "memory_usage > 80"
	Severity    AlertSeverity `json:"severity"`
	Threshold   float64       `json:"threshold"`
	Duration    time.Duration `json:"duration"`    // Alert after condition persists for this duration
	Enabled     bool          `json:"enabled"`
	LastFired   time.Time     `json:"last_fired"`
}

// AlertSeverity defines alert severity levels
type AlertSeverity string

const (
	SeverityInfo     AlertSeverity = "info"
	SeverityWarning  AlertSeverity = "warning"
	SeverityCritical AlertSeverity = "critical"
)

// Alert represents an active alert
type Alert struct {
	ID          string                 `json:"id"`
	Rule        AlertRule              `json:"rule"`
	Message     string                 `json:"message"`
	Data        map[string]interface{} `json:"data"`
	Timestamp   time.Time              `json:"timestamp"`
	Resolved    bool                   `json:"resolved"`
	ResolvedAt  time.Time              `json:"resolved_at,omitempty"`
}

// NewMonitoringManager creates a new monitoring manager
func NewMonitoringManager(server *Server) *MonitoringManager {
	return &MonitoringManager{
		server:        server,
		metrics:       &SystemMetrics{},
		alerts:        &AlertManager{},
		healthHistory: make([]HealthSnapshot, 0),
	}
}

// Start begins monitoring operations
func (mm *MonitoringManager) Start(ctx context.Context) {
	// Start metrics collection
	go mm.metricsCollector(ctx)
	
	// Start health check recording
	go mm.healthRecorder(ctx)
	
	// Start alert processor
	go mm.alertProcessor(ctx)
}

// GetSystemMetrics returns current system metrics
func (mm *MonitoringManager) GetSystemMetrics(c *gin.Context) {
	mm.mu.RLock()
	metrics := *mm.metrics
	mm.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"metrics": metrics,
	})
}

// GetHealthHistory returns health check history
func (mm *MonitoringManager) GetHealthHistory(c *gin.Context) {
	// Parse query parameters
	limitParam := c.DefaultQuery("limit", "100")
	hoursParam := c.DefaultQuery("hours", "24")
	
	limit := 100
	hours := 24
	
	// Convert parameters (simplified for demo)
	if limitParam != "100" {
		limit = 50 // Simple fallback
	}
	if hoursParam != "24" {
		hours = 12 // Simple fallback
	}

	mm.mu.RLock()
	defer mm.mu.RUnlock()

	// Filter history by time range
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	filtered := make([]HealthSnapshot, 0)
	
	for _, snapshot := range mm.healthHistory {
		if snapshot.Timestamp.After(cutoff) {
			filtered = append(filtered, snapshot)
		}
	}

	// Limit results
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"count":    len(filtered),
		"history":  filtered,
		"timespan": fmt.Sprintf("Last %d hours", hours),
	})
}

// GetAlerts returns active alerts
func (mm *MonitoringManager) GetAlerts(c *gin.Context) {
	activeOnly := c.DefaultQuery("active", "true") == "true"
	
	mm.alerts.mu.RLock()
	defer mm.alerts.mu.RUnlock()
	
	alerts := make([]Alert, 0)
	for _, alert := range mm.alerts.ActiveAlerts {
		if !activeOnly || !alert.Resolved {
			alerts = append(alerts, alert)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"count":  len(alerts),
		"alerts": alerts,
	})
}

// GetPerformanceProfile returns detailed performance metrics
func (mm *MonitoringManager) GetPerformanceProfile(c *gin.Context) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	profile := gin.H{
		"timestamp": time.Now(),
		"runtime": gin.H{
			"go_version":     runtime.Version(),
			"goroutines":     runtime.NumGoroutine(),
			"cpu_cores":      runtime.NumCPU(),
			"gc_cycles":      memStats.NumGC,
			"gc_pause_ns":    memStats.PauseTotalNs,
		},
		"memory": gin.H{
			"allocated_mb":    memStats.Alloc / 1024 / 1024,
			"total_alloc_mb":  memStats.TotalAlloc / 1024 / 1024,
			"system_mb":       memStats.Sys / 1024 / 1024,
			"heap_mb":         memStats.HeapAlloc / 1024 / 1024,
			"stack_mb":        memStats.StackInuse / 1024 / 1024,
		},
		"repositories": gin.H{
			"total_count":    mm.getActualRepositoryCount(),
			"metadata_count": len(mm.server.repoMetadata),
			"pool_count":     mm.getPoolRepositoryCount(),
			"factory_count":  mm.getFactoryRepositoryCount(),
			"pool_stats":     mm.getPoolStats(),
		},
	}

	// Add cluster metrics if available
	if mm.server.clusterManager != nil {
		clusterHealth := mm.server.clusterManager.failoverManager.GetClusterHealth()
		profile["cluster"] = clusterHealth
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"profile": profile,
	})
}

// metricsCollector runs continuously to collect system metrics
func (mm *MonitoringManager) metricsCollector(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // Collect every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mm.collectMetrics()
		}
	}
}

// collectMetrics gathers current system metrics
func (mm *MonitoringManager) collectMetrics() {
	// Log that metrics collection is running
	if mm.server.logger != nil {
		mm.server.logger.Debug("Collecting system metrics")
	}
	
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	metrics := SystemMetrics{
		RequestCount:      mm.getRequestCount(),
		AverageLatency:    mm.getAverageLatency(),
		ErrorRate:         mm.getErrorRate(),
		MemoryUsage:       memStats.Alloc,
		CPUUsage:          mm.getCPUUsage(),
		GoroutineCount:    runtime.NumGoroutine(),
		RepositoryCount:   mm.getActualRepositoryCount(),
		ActiveConnections: mm.getActiveConnections(),
		PoolUtilization:   mm.getPoolUtilization(),
		DiskUsage:         mm.getDiskUsage(),
		DiskFree:          mm.getDiskFree(),
		StorageHealth:     "healthy", // Simplified
		Timestamp:         time.Now(),
	}

	// Add cluster metrics if available
	if mm.server.clusterManager != nil {
		clusterHealth := mm.server.clusterManager.cluster.GetClusterHealth()
		metrics.ClusterNodes = clusterHealth.TotalNodes
		metrics.HealthyNodes = clusterHealth.HealthyNodes
		metrics.ClusterHealth = string(clusterHealth.Status)
	}

	mm.mu.Lock()
	mm.metrics = &metrics
	mm.mu.Unlock()
}

// healthRecorder records health check snapshots
func (mm *MonitoringManager) healthRecorder(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute) // Record every minute
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mm.recordHealthSnapshot()
		}
	}
}

// recordHealthSnapshot captures current health state
func (mm *MonitoringManager) recordHealthSnapshot() {
	start := time.Now()
	checks := mm.server.runHealthChecks()
	
	status := "healthy"
	for _, check := range checks {
		if check.Status != "healthy" {
			status = "unhealthy"
			break
		}
	}

	snapshot := HealthSnapshot{
		Timestamp:    time.Now(),
		Status:       status,
		ResponseTime: time.Since(start),
		Checks:       checks,
	}

	mm.mu.RLock()
	if mm.metrics != nil {
		snapshot.Metrics = *mm.metrics // Copy current metrics
	}
	mm.mu.RUnlock()

	mm.mu.Lock()
	mm.healthHistory = append(mm.healthHistory, snapshot)
	
	// Keep only last 1000 entries to prevent memory growth
	if len(mm.healthHistory) > 1000 {
		mm.healthHistory = mm.healthHistory[1:]
	}
	mm.mu.Unlock()
}

// alertProcessor monitors for alert conditions
func (mm *MonitoringManager) alertProcessor(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mm.processAlerts()
		}
	}
}

// processAlerts checks alert conditions and triggers alerts
func (mm *MonitoringManager) processAlerts() {
	mm.mu.RLock()
	metrics := *mm.metrics
	mm.mu.RUnlock()

	// Check memory usage alert
	memoryUsageMB := float64(metrics.MemoryUsage) / 1024 / 1024
	if memoryUsageMB > 512 { // 512MB threshold
		mm.triggerAlert("high_memory_usage", "Memory usage is high", map[string]interface{}{
			"current_mb": memoryUsageMB,
			"threshold":  512,
		})
	}

	// Check repository count
	if metrics.RepositoryCount > 800 { // 80% of default 1000 limit
		mm.triggerAlert("high_repository_count", "Repository count approaching limit", map[string]interface{}{
			"current_count": metrics.RepositoryCount,
			"limit":        1000,
		})
	}

	// Check goroutine count
	if metrics.GoroutineCount > 1000 {
		mm.triggerAlert("high_goroutine_count", "High number of goroutines detected", map[string]interface{}{
			"current_count": metrics.GoroutineCount,
			"threshold":    1000,
		})
	}
}

// Helper methods (simplified implementations)

func (mm *MonitoringManager) getRequestCount() int64 {
	// In a real implementation, this would track HTTP requests
	return 0
}

func (mm *MonitoringManager) getAverageLatency() time.Duration {
	// Would calculate from request metrics
	return 5 * time.Millisecond
}

func (mm *MonitoringManager) getErrorRate() float64 {
	// Would calculate error rate from request metrics
	return 0.01 // 1%
}

func (mm *MonitoringManager) getCPUUsage() float64 {
	// Would use system calls to get actual CPU usage
	return 25.5 // 25.5%
}

func (mm *MonitoringManager) getActiveConnections() int {
	// Would track active HTTP connections
	return 10
}

func (mm *MonitoringManager) getPoolUtilization() float64 {
	if mm.server.repoPool == nil {
		return 0
	}
	stats := mm.server.repoPool.Stats()
	maxRepos := stats.Config.MaxRepositories
	if maxRepos == 0 {
		return 0
	}
	return float64(stats.ActiveRepositories) / float64(maxRepos) * 100
}

func (mm *MonitoringManager) getDiskUsage() uint64 {
	// Would check actual disk usage
	return 1024 * 1024 * 1024 // 1GB
}

func (mm *MonitoringManager) getDiskFree() uint64 {
	// Would check actual free disk space
	return 10 * 1024 * 1024 * 1024 // 10GB
}

func (mm *MonitoringManager) getActualRepositoryCount() int {
	// Count from both pool and factory depending on configuration
	poolCount := 0
	factoryCount := 0
	
	// Get count from repository pool
	if mm.server.repoPool != nil {
		stats := mm.server.repoPool.Stats()
		poolCount = stats.TotalRepositories
	}
	
	// Get count from repository factory
	if mm.server.repoFactory != nil {
		factoryRepos := mm.server.repoFactory.ListRepositories()
		factoryCount = len(factoryRepos)
	}
	
	// Return the higher count (for transition period where both might be used)
	if factoryCount > poolCount {
		return factoryCount
	}
	return poolCount
}

func (mm *MonitoringManager) getPoolRepositoryCount() int {
	if mm.server.repoPool == nil {
		return 0
	}
	stats := mm.server.repoPool.Stats()
	return stats.TotalRepositories
}

func (mm *MonitoringManager) getFactoryRepositoryCount() int {
	if mm.server.repoFactory == nil {
		return 0
	}
	factoryRepos := mm.server.repoFactory.ListRepositories()
	return len(factoryRepos)
}

func (mm *MonitoringManager) getPoolStats() map[string]interface{} {
	if mm.server.repoPool == nil {
		return map[string]interface{}{"status": "not_initialized"}
	}
	
	stats := mm.server.repoPool.Stats()
	return map[string]interface{}{
		"active_repositories":  stats.ActiveRepositories,
		"idle_repositories":    stats.IdleRepositories,
		"total_repositories":   stats.TotalRepositories,
		"max_repositories":     stats.Config.MaxRepositories,
		"last_cleanup":        stats.LastCleanup,
		"config":              stats.Config,
	}
}

func (mm *MonitoringManager) triggerAlert(alertType, message string, data map[string]interface{}) {
	alert := Alert{
		ID:        fmt.Sprintf("%s-%d", alertType, time.Now().UnixNano()),
		Message:   message,
		Data:      data,
		Timestamp: time.Now(),
		Resolved:  false,
	}

	mm.alerts.mu.Lock()
	if mm.alerts.ActiveAlerts == nil {
		mm.alerts.ActiveAlerts = make(map[string]Alert)
	}
	mm.alerts.ActiveAlerts[alert.ID] = alert
	mm.alerts.mu.Unlock()

	// In a real implementation, you would send this to external alerting systems
	mm.server.logger.Warnf("Alert triggered: %s - %s", alertType, message)
}

// AlertManager manages system alerts
type AlertManager struct {
	ActiveAlerts map[string]Alert `json:"active_alerts"`
	mu           sync.RWMutex
}