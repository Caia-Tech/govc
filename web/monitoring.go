package web

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Dashboard HTTP metrics
	dashboardRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "govc_dashboard_request_duration_seconds",
		Help:    "Duration of dashboard HTTP requests in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "endpoint", "status"})

	dashboardRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "govc_dashboard_requests_total",
		Help: "Total number of dashboard HTTP requests",
	}, []string{"method", "endpoint", "status"})

	// WebSocket metrics
	dashboardWebSocketConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "govc_dashboard_websocket_connections",
		Help: "Current number of WebSocket connections",
	})

	dashboardWebSocketMessagesSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "govc_dashboard_websocket_messages_sent_total",
		Help: "Total number of WebSocket messages sent",
	}, []string{"type"})

	dashboardWebSocketErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "govc_dashboard_websocket_errors_total",
		Help: "Total number of WebSocket errors",
	})

	// Dashboard performance metrics
	dashboardRenderTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "govc_dashboard_render_time_seconds",
		Help:    "Time taken to render dashboard pages",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
	})

	dashboardAPILatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "govc_dashboard_api_latency_seconds",
		Help:    "API endpoint latency in seconds",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
	}, []string{"endpoint"})

	// Resource usage metrics
	dashboardMemoryUsage = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "govc_dashboard_memory_usage_bytes",
		Help: "Current memory usage of dashboard components",
	})

	dashboardGoroutines = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "govc_dashboard_goroutines",
		Help: "Current number of goroutines used by dashboard",
	})

	// Business metrics
	dashboardActiveUsers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "govc_dashboard_active_users",
		Help: "Number of active dashboard users",
	})

	dashboardRepositoryViews = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "govc_dashboard_repository_views_total",
		Help: "Total number of repository views from dashboard",
	}, []string{"repository_id"})

	dashboardActionsPerformed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "govc_dashboard_actions_performed_total",
		Help: "Total number of actions performed via dashboard",
	}, []string{"action_type"})

	// Error metrics
	dashboardErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "govc_dashboard_errors_total",
		Help: "Total number of dashboard errors",
	}, []string{"error_type", "endpoint"})

	// Cache metrics
	dashboardCacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "govc_dashboard_cache_hits_total",
		Help: "Total number of dashboard cache hits",
	})

	dashboardCacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "govc_dashboard_cache_misses_total",
		Help: "Total number of dashboard cache misses",
	})
)

// MonitoringMiddleware wraps dashboard handlers with metrics collection
func (h *DashboardHandler) MonitoringMiddleware() func(handler func(*gin.Context)) func(*gin.Context) {
	return func(handler func(*gin.Context)) func(*gin.Context) {
		return func(c *gin.Context) {
			start := time.Now()
			
			// Call the handler
			handler(c)
			
			// Record metrics
			duration := time.Since(start).Seconds()
			status := fmt.Sprintf("%d", c.Writer.Status())
			endpoint := c.Request.URL.Path
			method := c.Request.Method
			
			dashboardRequestDuration.WithLabelValues(method, endpoint, status).Observe(duration)
			dashboardRequestsTotal.WithLabelValues(method, endpoint, status).Inc()
			
			// Record API latency for specific endpoints
			if strings.HasPrefix(endpoint, "/api/v1/dashboard/") {
				dashboardAPILatency.WithLabelValues(endpoint).Observe(duration)
			}
			
			// Track errors
			if c.Writer.Status() >= 400 {
				errorType := "client_error"
				if c.Writer.Status() >= 500 {
					errorType = "server_error"
				}
				dashboardErrors.WithLabelValues(errorType, endpoint).Inc()
			}
		}
	}
}

// RecordWebSocketConnection increments the WebSocket connection gauge
func (h *DashboardHandler) RecordWebSocketConnection() {
	dashboardWebSocketConnections.Inc()
}

// RecordWebSocketDisconnection decrements the WebSocket connection gauge
func (h *DashboardHandler) RecordWebSocketDisconnection() {
	dashboardWebSocketConnections.Dec()
}

// RecordWebSocketMessage records a sent WebSocket message
func (h *DashboardHandler) RecordWebSocketMessage(messageType string) {
	dashboardWebSocketMessagesSent.WithLabelValues(messageType).Inc()
}

// RecordWebSocketError records a WebSocket error
func (h *DashboardHandler) RecordWebSocketError() {
	dashboardWebSocketErrors.Inc()
}

// RecordAction records a user action
func (h *DashboardHandler) RecordAction(actionType string) {
	dashboardActionsPerformed.WithLabelValues(actionType).Inc()
}

// RecordRepositoryView records a repository view
func (h *DashboardHandler) RecordRepositoryView(repoID string) {
	dashboardRepositoryViews.WithLabelValues(repoID).Inc()
}

// UpdateResourceMetrics updates resource usage metrics
func (h *DashboardHandler) UpdateResourceMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	dashboardMemoryUsage.Set(float64(m.Alloc))
	dashboardGoroutines.Set(float64(runtime.NumGoroutine()))
}

// AlertRule represents a monitoring alert rule
type AlertRule struct {
	Name        string
	Expression  string
	Duration    time.Duration
	Severity    string
	Description string
	Actions     []AlertAction
}

// AlertAction represents an action to take when an alert fires
type AlertAction struct {
	Type   string // email, webhook, pagerduty
	Target string
	Config map[string]interface{}
}

// GetDefaultAlertRules returns default alert rules for dashboard monitoring
func GetDefaultAlertRules() []AlertRule {
	return []AlertRule{
		{
			Name:       "DashboardHighLatency",
			Expression: `rate(govc_dashboard_request_duration_seconds_sum[5m]) / rate(govc_dashboard_request_duration_seconds_count[5m]) > 0.5`,
			Duration:   5 * time.Minute,
			Severity:   "warning",
			Description: "Dashboard API latency is above 500ms",
		},
		{
			Name:       "DashboardErrorRate",
			Expression: `rate(govc_dashboard_errors_total[5m]) > 0.05`,
			Duration:   5 * time.Minute,
			Severity:   "critical",
			Description: "Dashboard error rate is above 5%",
		},
		{
			Name:       "WebSocketConnectionSpike",
			Expression: `govc_dashboard_websocket_connections > 1000`,
			Duration:   1 * time.Minute,
			Severity:   "warning",
			Description: "Too many WebSocket connections",
		},
		{
			Name:       "DashboardMemoryHigh",
			Expression: `govc_dashboard_memory_usage_bytes > 500000000`, // 500MB
			Duration:   10 * time.Minute,
			Severity:   "warning",
			Description: "Dashboard memory usage is high",
		},
		{
			Name:       "DashboardDown",
			Expression: `up{job="govc-dashboard"} == 0`,
			Duration:   1 * time.Minute,
			Severity:   "critical",
			Description: "Dashboard is down",
		},
		{
			Name:       "DashboardSlowQueries",
			Expression: `histogram_quantile(0.95, rate(govc_dashboard_api_latency_seconds_bucket[5m])) > 1`,
			Duration:   5 * time.Minute,
			Severity:   "warning",
			Description: "95th percentile API latency is above 1 second",
		},
		{
			Name:       "WebSocketErrorsHigh",
			Expression: `rate(govc_dashboard_websocket_errors_total[5m]) > 1`,
			Duration:   5 * time.Minute,
			Severity:   "warning",
			Description: "WebSocket errors are occurring frequently",
		},
		{
			Name:       "DashboardCacheMissRate",
			Expression: `rate(govc_dashboard_cache_misses_total[5m]) / (rate(govc_dashboard_cache_hits_total[5m]) + rate(govc_dashboard_cache_misses_total[5m])) > 0.3`,
			Duration:   10 * time.Minute,
			Severity:   "info",
			Description: "Dashboard cache miss rate is above 30%",
		},
	}
}

// StartMetricsCollection starts periodic collection of metrics
func (h *DashboardHandler) StartMetricsCollection() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		
		for range ticker.C {
			h.UpdateResourceMetrics()
			
			// Update active users (simplified - in production, track actual sessions)
			activeUsers := float64(len(h.wsClients))
			dashboardActiveUsers.Set(activeUsers)
		}
	}()
}