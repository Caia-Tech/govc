package metrics

import (
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// PrometheusMetrics provides Prometheus-compatible metrics
type PrometheusMetrics struct {
	mu sync.RWMutex
	
	// HTTP metrics
	httpRequests      map[string]int64 // method:status -> count
	httpDuration      map[string]float64 // method:path -> total duration in seconds
	httpInFlight      int64
	
	// Repository metrics
	repositoryCount   int64
	transactionCount  int64
	
	// System metrics
	startTime         time.Time
	lastMetricsUpdate time.Time
}

// NewPrometheusMetrics creates a new Prometheus metrics instance
func NewPrometheusMetrics() *PrometheusMetrics {
	return &PrometheusMetrics{
		httpRequests:      make(map[string]int64),
		httpDuration:      make(map[string]float64),
		startTime:         time.Now(),
		lastMetricsUpdate: time.Now(),
	}
}

// RecordHTTPRequest records an HTTP request
func (p *PrometheusMetrics) RecordHTTPRequest(method, path string, status int, duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	statusStr := strconv.Itoa(status)
	key := method + ":" + statusStr
	p.httpRequests[key]++
	
	pathKey := method + ":" + path
	p.httpDuration[pathKey] += duration.Seconds()
	
	p.lastMetricsUpdate = time.Now()
}

// IncHTTPInFlight increments in-flight HTTP requests
func (p *PrometheusMetrics) IncHTTPInFlight() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.httpInFlight++
}

// DecHTTPInFlight decrements in-flight HTTP requests
func (p *PrometheusMetrics) DecHTTPInFlight() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.httpInFlight--
}

// SetRepositoryCount sets the current repository count
func (p *PrometheusMetrics) SetRepositoryCount(count int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.repositoryCount = count
	p.lastMetricsUpdate = time.Now()
}

// SetTransactionCount sets the current transaction count
func (p *PrometheusMetrics) SetTransactionCount(count int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.transactionCount = count
	p.lastMetricsUpdate = time.Now()
}

// PrometheusHandler returns a Gin handler for Prometheus metrics
func (p *PrometheusMetrics) PrometheusHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		c.String(http.StatusOK, p.generateMetrics())
	}
}

// generateMetrics generates Prometheus-formatted metrics
func (p *PrometheusMetrics) generateMetrics() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	var metrics []string
	
	// HTTP request metrics
	metrics = append(metrics, "# HELP govc_http_requests_total The total number of HTTP requests")
	metrics = append(metrics, "# TYPE govc_http_requests_total counter")
	for key, count := range p.httpRequests {
		parts := splitMethodStatus(key)
		if len(parts) == 2 {
			method, status := parts[0], parts[1]
			metrics = append(metrics, 
				"govc_http_requests_total{method=\""+method+"\",status=\""+status+"\"} "+strconv.FormatInt(count, 10))
		}
	}
	
	// HTTP duration metrics
	metrics = append(metrics, "")
	metrics = append(metrics, "# HELP govc_http_request_duration_seconds The total time spent on HTTP requests")
	metrics = append(metrics, "# TYPE govc_http_request_duration_seconds counter")
	for key, duration := range p.httpDuration {
		parts := splitMethodPath(key)
		if len(parts) == 2 {
			method, path := parts[0], parts[1]
			// Sanitize path for Prometheus labels
			path = sanitizePrometheusLabel(path)
			metrics = append(metrics,
				"govc_http_request_duration_seconds{method=\""+method+"\",path=\""+path+"\"} "+formatFloat(duration))
		}
	}
	
	// In-flight requests
	metrics = append(metrics, "")
	metrics = append(metrics, "# HELP govc_http_requests_in_flight The current number of in-flight HTTP requests")
	metrics = append(metrics, "# TYPE govc_http_requests_in_flight gauge")
	metrics = append(metrics, "govc_http_requests_in_flight "+strconv.FormatInt(p.httpInFlight, 10))
	
	// Application metrics
	metrics = append(metrics, "")
	metrics = append(metrics, "# HELP govc_repositories_total The total number of repositories")
	metrics = append(metrics, "# TYPE govc_repositories_total gauge")
	metrics = append(metrics, "govc_repositories_total "+strconv.FormatInt(p.repositoryCount, 10))
	
	metrics = append(metrics, "")
	metrics = append(metrics, "# HELP govc_transactions_active The number of active transactions")
	metrics = append(metrics, "# TYPE govc_transactions_active gauge")
	metrics = append(metrics, "govc_transactions_active "+strconv.FormatInt(p.transactionCount, 10))
	
	// System uptime
	uptime := time.Since(p.startTime).Seconds()
	metrics = append(metrics, "")
	metrics = append(metrics, "# HELP govc_uptime_seconds The uptime of the govc server in seconds")
	metrics = append(metrics, "# TYPE govc_uptime_seconds counter")
	metrics = append(metrics, "govc_uptime_seconds "+formatFloat(uptime))
	
	// Go runtime metrics
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	metrics = append(metrics, "")
	metrics = append(metrics, "# HELP go_memstats_alloc_bytes Number of bytes allocated and still in use")
	metrics = append(metrics, "# TYPE go_memstats_alloc_bytes gauge")
	metrics = append(metrics, "go_memstats_alloc_bytes "+strconv.FormatUint(memStats.Alloc, 10))
	
	metrics = append(metrics, "")
	metrics = append(metrics, "# HELP go_memstats_total_alloc_bytes Total number of bytes allocated, even if freed")
	metrics = append(metrics, "# TYPE go_memstats_total_alloc_bytes counter")
	metrics = append(metrics, "go_memstats_total_alloc_bytes "+strconv.FormatUint(memStats.TotalAlloc, 10))
	
	metrics = append(metrics, "")
	metrics = append(metrics, "# HELP go_memstats_sys_bytes Number of bytes obtained from system")
	metrics = append(metrics, "# TYPE go_memstats_sys_bytes gauge")
	metrics = append(metrics, "go_memstats_sys_bytes "+strconv.FormatUint(memStats.Sys, 10))
	
	metrics = append(metrics, "")
	metrics = append(metrics, "# HELP go_goroutines Number of goroutines that currently exist")
	metrics = append(metrics, "# TYPE go_goroutines gauge")
	metrics = append(metrics, "go_goroutines "+strconv.Itoa(runtime.NumGoroutine()))
	
	metrics = append(metrics, "")
	metrics = append(metrics, "# HELP go_memstats_gc_total Number of completed GC cycles")
	metrics = append(metrics, "# TYPE go_memstats_gc_total counter")
	metrics = append(metrics, "go_memstats_gc_total "+strconv.FormatUint(uint64(memStats.NumGC), 10))
	
	// Last metrics update
	lastUpdate := p.lastMetricsUpdate.Unix()
	metrics = append(metrics, "")
	metrics = append(metrics, "# HELP govc_last_metrics_update_timestamp_seconds Timestamp of the last metrics update")
	metrics = append(metrics, "# TYPE govc_last_metrics_update_timestamp_seconds gauge")
	metrics = append(metrics, "govc_last_metrics_update_timestamp_seconds "+strconv.FormatInt(lastUpdate, 10))
	
	return joinMetrics(metrics)
}

// Helper functions

func splitMethodStatus(key string) []string {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return []string{key[:i], key[i+1:]}
		}
	}
	return []string{key}
}

func splitMethodPath(key string) []string {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return []string{key[:i], key[i+1:]}
		}
	}
	return []string{key}
}

func sanitizePrometheusLabel(path string) string {
	// Replace path parameters with placeholders for better grouping
	result := path
	
	// Common replacements for API paths
	replacements := map[string]string{
		"/api/v1/repos/": "/api/v1/repos/{repo_id}",
		"/commits/":      "/commits/{commit}",
		"/branches/":     "/branches/{branch}",
		"/stash/":        "/stash/{stash_id}",
		"/users/":        "/users/{user_id}",
		"/hooks/":        "/hooks/{hook_id}",
	}
	
	for pattern, replacement := range replacements {
		if len(result) > len(pattern) {
			// Simple pattern matching for common API structures
			for i := 0; i <= len(result)-len(pattern); i++ {
				if result[i:i+len(pattern)] == pattern {
					// Check if there's more path after the pattern
					remaining := result[i+len(pattern):]
					if remaining != "" {
						// Find the next slash or end
						nextSlash := 0
						for j, c := range remaining {
							if c == '/' {
								nextSlash = j
								break
							}
						}
						if nextSlash == 0 {
							nextSlash = len(remaining)
						}
						
						// Replace the ID with placeholder
						result = result[:i] + replacement + remaining[nextSlash:]
						break
					}
				}
			}
		}
	}
	
	return result
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func joinMetrics(metrics []string) string {
	result := ""
	for i, metric := range metrics {
		result += metric
		if i < len(metrics)-1 {
			result += "\n"
		}
	}
	return result + "\n"
}

// Middleware for automatic metrics collection
func (p *PrometheusMetrics) GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		
		// Increment in-flight requests
		p.IncHTTPInFlight()
		defer p.DecHTTPInFlight()
		
		// Process request
		c.Next()
		
		// Record metrics
		duration := time.Since(start)
		status := c.Writer.Status()
		
		p.RecordHTTPRequest(method, path, status, duration)
	}
}