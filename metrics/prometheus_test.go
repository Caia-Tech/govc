package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestNewPrometheusMetrics(t *testing.T) {
	metrics := NewPrometheusMetrics()

	if metrics == nil {
		t.Fatal("NewPrometheusMetrics returned nil")
	}

	if metrics.httpRequests == nil {
		t.Error("httpRequests map not initialized")
	}

	if metrics.httpDuration == nil {
		t.Error("httpDuration map not initialized")
	}

	if metrics.startTime.IsZero() {
		t.Error("startTime should be set")
	}

	if metrics.lastMetricsUpdate.IsZero() {
		t.Error("lastMetricsUpdate should be set")
	}

	if metrics.httpInFlight != 0 {
		t.Errorf("Expected httpInFlight to be 0, got %d", metrics.httpInFlight)
	}

	if metrics.repositoryCount != 0 {
		t.Errorf("Expected repositoryCount to be 0, got %d", metrics.repositoryCount)
	}

	if metrics.transactionCount != 0 {
		t.Errorf("Expected transactionCount to be 0, got %d", metrics.transactionCount)
	}
}

func TestRecordHTTPRequest(t *testing.T) {
	metrics := NewPrometheusMetrics()

	testCases := []struct {
		name     string
		method   string
		path     string
		status   int
		duration time.Duration
	}{
		{
			name:     "GET request success",
			method:   "GET",
			path:     "/api/v1/repos",
			status:   200,
			duration: 100 * time.Millisecond,
		},
		{
			name:     "POST request created",
			method:   "POST",
			path:     "/api/v1/repos/test",
			status:   201,
			duration: 250 * time.Millisecond,
		},
		{
			name:     "GET request not found",
			method:   "GET",
			path:     "/api/v1/repos/nonexistent",
			status:   404,
			duration: 10 * time.Millisecond,
		},
		{
			name:     "same method and status",
			method:   "GET",
			path:     "/api/v1/users",
			status:   200,
			duration: 50 * time.Millisecond,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initialTime := metrics.lastMetricsUpdate
			time.Sleep(1 * time.Millisecond) // Ensure time difference

			metrics.RecordHTTPRequest(tc.method, tc.path, tc.status, tc.duration)

			// Check that request was recorded (we'll verify exact counts later)

			// Check HTTP duration
			pathKey := tc.method + ":" + tc.path
			metrics.mu.RLock()
			if _, exists := metrics.httpDuration[pathKey]; !exists {
				t.Errorf("Duration not recorded for path key %s", pathKey)
			}
			metrics.mu.RUnlock()

			// Check that lastMetricsUpdate was updated
			if !metrics.lastMetricsUpdate.After(initialTime) {
				t.Error("lastMetricsUpdate should have been updated")
			}
		})
	}

	// Test multiple requests to same endpoint
	metrics.RecordHTTPRequest("GET", "/api/test", 200, 100*time.Millisecond)
	metrics.RecordHTTPRequest("GET", "/api/test", 200, 150*time.Millisecond)

	metrics.mu.RLock()
	count := metrics.httpRequests["GET:200"]
	duration := metrics.httpDuration["GET:/api/test"]
	metrics.mu.RUnlock()

	if count < 2 { // Should be at least 2 from the test cases above
		t.Errorf("Expected at least 2 GET:200 requests, got %d", count)
	}

	if duration < 0.25 { // Should be at least 250ms from the two calls above
		t.Errorf("Expected duration >= 0.25 seconds, got %f", duration)
	}
}

func TestHTTPInFlightMetrics(t *testing.T) {
	metrics := NewPrometheusMetrics()

	// Test increment
	metrics.IncHTTPInFlight()
	if metrics.httpInFlight != 1 {
		t.Errorf("Expected httpInFlight to be 1, got %d", metrics.httpInFlight)
	}

	metrics.IncHTTPInFlight()
	if metrics.httpInFlight != 2 {
		t.Errorf("Expected httpInFlight to be 2, got %d", metrics.httpInFlight)
	}

	// Test decrement
	metrics.DecHTTPInFlight()
	if metrics.httpInFlight != 1 {
		t.Errorf("Expected httpInFlight to be 1 after decrement, got %d", metrics.httpInFlight)
	}

	metrics.DecHTTPInFlight()
	if metrics.httpInFlight != 0 {
		t.Errorf("Expected httpInFlight to be 0 after decrement, got %d", metrics.httpInFlight)
	}

	// Test decrement below zero
	metrics.DecHTTPInFlight()
	if metrics.httpInFlight != -1 {
		t.Errorf("Expected httpInFlight to be -1 after decrement below zero, got %d", metrics.httpInFlight)
	}
}

func TestSetRepositoryCount(t *testing.T) {
	metrics := NewPrometheusMetrics()

	testCases := []struct {
		name  string
		count int64
	}{
		{name: "zero repositories", count: 0},
		{name: "positive count", count: 42},
		{name: "large count", count: 999999},
		{name: "update count", count: 10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initialTime := metrics.lastMetricsUpdate
			time.Sleep(1 * time.Millisecond)

			metrics.SetRepositoryCount(tc.count)

			if metrics.repositoryCount != tc.count {
				t.Errorf("Expected repositoryCount to be %d, got %d", tc.count, metrics.repositoryCount)
			}

			if !metrics.lastMetricsUpdate.After(initialTime) {
				t.Error("lastMetricsUpdate should have been updated")
			}
		})
	}
}

func TestSetTransactionCount(t *testing.T) {
	metrics := NewPrometheusMetrics()

	testCases := []struct {
		name  string
		count int64
	}{
		{name: "zero transactions", count: 0},
		{name: "positive count", count: 123},
		{name: "large count", count: 888888},
		{name: "update count", count: 5},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initialTime := metrics.lastMetricsUpdate
			time.Sleep(1 * time.Millisecond)

			metrics.SetTransactionCount(tc.count)

			if metrics.transactionCount != tc.count {
				t.Errorf("Expected transactionCount to be %d, got %d", tc.count, metrics.transactionCount)
			}

			if !metrics.lastMetricsUpdate.After(initialTime) {
				t.Error("lastMetricsUpdate should have been updated")
			}
		})
	}
}

func TestGenerateMetrics(t *testing.T) {
	metrics := NewPrometheusMetrics()

	// Add some test data
	metrics.RecordHTTPRequest("GET", "/api/v1/repos", 200, 100*time.Millisecond)
	metrics.RecordHTTPRequest("POST", "/api/v1/repos/test", 201, 250*time.Millisecond)
	metrics.RecordHTTPRequest("GET", "/api/v1/repos/nonexistent", 404, 10*time.Millisecond)
	metrics.IncHTTPInFlight()
	metrics.IncHTTPInFlight()
	metrics.SetRepositoryCount(42)
	metrics.SetTransactionCount(7)

	output := metrics.generateMetrics()

	// Check that output contains expected metrics
	expectedMetrics := []string{
		"# HELP govc_http_requests_total",
		"# TYPE govc_http_requests_total counter",
		"govc_http_requests_total{method=\"GET\",status=\"200\"} 1",
		"govc_http_requests_total{method=\"POST\",status=\"201\"} 1",
		"govc_http_requests_total{method=\"GET\",status=\"404\"} 1",
		"# HELP govc_http_request_duration_seconds",
		"# TYPE govc_http_request_duration_seconds counter",
		"# HELP govc_http_requests_in_flight",
		"# TYPE govc_http_requests_in_flight gauge",
		"govc_http_requests_in_flight 2",
		"# HELP govc_repositories_total",
		"# TYPE govc_repositories_total gauge",
		"govc_repositories_total 42",
		"# HELP govc_transactions_active",
		"# TYPE govc_transactions_active gauge",
		"govc_transactions_active 7",
		"# HELP govc_uptime_seconds",
		"# TYPE govc_uptime_seconds counter",
		"# HELP go_memstats_alloc_bytes",
		"# HELP go_goroutines",
		"# HELP govc_last_metrics_update_timestamp_seconds",
	}

	for _, expected := range expectedMetrics {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain '%s'", expected)
		}
	}

	// Check that output is properly formatted (ends with newline)
	if !strings.HasSuffix(output, "\n") {
		t.Error("Metrics output should end with newline")
	}

	// Check that there are no empty metric names
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "govc_") || strings.HasPrefix(line, "go_") {
			parts := strings.Fields(line)
			if len(parts) < 2 {
				t.Errorf("Invalid metric line: %s", line)
			}
		}
	}
}

func TestPrometheusHandler(t *testing.T) {
	metrics := NewPrometheusMetrics()

	// Add some test data
	metrics.RecordHTTPRequest("GET", "/test", 200, 50*time.Millisecond)
	metrics.SetRepositoryCount(10)

	// Create test router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/metrics", metrics.PrometheusHandler())

	// Make request
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	expectedContentType := "text/plain; version=0.0.4; charset=utf-8"
	if contentType != expectedContentType {
		t.Errorf("Expected Content-Type '%s', got '%s'", expectedContentType, contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "govc_repositories_total 10") {
		t.Error("Response should contain repository count metric")
	}

	if !strings.Contains(body, "# HELP") {
		t.Error("Response should contain metric help text")
	}

	if !strings.Contains(body, "# TYPE") {
		t.Error("Response should contain metric type information")
	}
}

func TestGinMiddleware(t *testing.T) {
	metrics := NewPrometheusMetrics()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(metrics.GinMiddleware())

	// Add test routes
	router.GET("/test", func(c *gin.Context) {
		time.Sleep(10 * time.Millisecond) // Simulate some processing time
		c.JSON(http.StatusOK, gin.H{"message": "test"})
	})

	router.POST("/error", func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
	})

	// Test successful GET request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Test error POST request
	req = httptest.NewRequest("POST", "/error", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	// Check that metrics were recorded
	metrics.mu.RLock()
	getRequests := metrics.httpRequests["GET:200"]
	postRequests := metrics.httpRequests["POST:400"]
	getDuration := metrics.httpDuration["GET:/test"]
	postDuration := metrics.httpDuration["POST:/error"]
	inFlight := metrics.httpInFlight
	metrics.mu.RUnlock()

	if getRequests != 1 {
		t.Errorf("Expected 1 GET:200 request, got %d", getRequests)
	}

	if postRequests != 1 {
		t.Errorf("Expected 1 POST:400 request, got %d", postRequests)
	}

	if getDuration == 0 {
		t.Error("Expected non-zero duration for GET request")
	}

	if postDuration == 0 {
		t.Error("Expected non-zero duration for POST request")
	}

	if inFlight != 0 {
		t.Errorf("Expected 0 in-flight requests after completion, got %d", inFlight)
	}
}

func TestConcurrentMetrics(t *testing.T) {
	metrics := NewPrometheusMetrics()

	done := make(chan bool, 100)

	// Concurrent HTTP request recording
	for i := 0; i < 50; i++ {
		go func(id int) {
			metrics.RecordHTTPRequest("GET", "/test", 200, time.Duration(id)*time.Millisecond)
			done <- true
		}(i)
	}

	// Concurrent in-flight increment/decrement
	for i := 0; i < 50; i++ {
		go func() {
			metrics.IncHTTPInFlight()
			time.Sleep(1 * time.Millisecond)
			metrics.DecHTTPInFlight()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Check final state
	metrics.mu.RLock()
	requestCount := metrics.httpRequests["GET:200"]
	inFlight := metrics.httpInFlight
	metrics.mu.RUnlock()

	if requestCount != 50 {
		t.Errorf("Expected 50 requests, got %d", requestCount)
	}

	if inFlight != 0 {
		t.Errorf("Expected 0 in-flight requests, got %d", inFlight)
	}

	// Test concurrent metrics generation
	for i := 0; i < 10; i++ {
		go func() {
			output := metrics.generateMetrics()
			if output == "" {
				t.Error("Metrics output should not be empty")
			}
			done <- true
		}()
	}

	// Wait for metrics generation
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestHelperFunctions(t *testing.T) {
	// Test splitMethodStatus
	testCases := []struct {
		input    string
		expected []string
	}{
		{"GET:200", []string{"GET", "200"}},
		{"POST:404", []string{"POST", "404"}},
		{"PUT", []string{"PUT"}},
		{"", []string{""}},
		{"GET:200:extra", []string{"GET", "200:extra"}},
	}

	for _, tc := range testCases {
		result := splitMethodStatus(tc.input)
		if len(result) != len(tc.expected) {
			t.Errorf("splitMethodStatus(%s): expected %d parts, got %d", tc.input, len(tc.expected), len(result))
			continue
		}
		for i, expected := range tc.expected {
			if result[i] != expected {
				t.Errorf("splitMethodStatus(%s): expected part %d to be '%s', got '%s'", tc.input, i, expected, result[i])
			}
		}
	}

	// Test splitMethodPath
	pathTestCases := []struct {
		input    string
		expected []string
	}{
		{"GET:/api/v1/repos", []string{"GET", "/api/v1/repos"}},
		{"POST:/api/v1/repos/test/commits", []string{"POST", "/api/v1/repos/test/commits"}},
		{"DELETE", []string{"DELETE"}},
	}

	for _, tc := range pathTestCases {
		result := splitMethodPath(tc.input)
		if len(result) != len(tc.expected) {
			t.Errorf("splitMethodPath(%s): expected %d parts, got %d", tc.input, len(tc.expected), len(result))
			continue
		}
		for i, expected := range tc.expected {
			if result[i] != expected {
				t.Errorf("splitMethodPath(%s): expected part %d to be '%s', got '%s'", tc.input, i, expected, result[i])
			}
		}
	}

	// Test sanitizePrometheusLabel
	sanitizeTestCases := []struct {
		input    string
		expected string
	}{
		{"/api/v1/repos/test-repo", "/api/v1/repos/{repo_id}"},
		{"/api/v1/repos/test-repo/commits/abc123", "/api/v1/repos/{repo_id}/commits/{commit}"},
		{"/users/john.doe", "/users/{user_id}"},
		{"/hooks/webhook-1", "/hooks/{hook_id}"},
		{"/simple/path", "/simple/path"},
		{"/api/v1/repos", "/api/v1/repos"},
	}

	for _, tc := range sanitizeTestCases {
		result := sanitizePrometheusLabel(tc.input)
		if result != tc.expected {
			t.Errorf("sanitizePrometheusLabel(%s): expected '%s', got '%s'", tc.input, tc.expected, result)
		}
	}

	// Test formatFloat
	floatTests := []struct {
		input    float64
		expected string
	}{
		{123.456, "123.456"},
		{0.0, "0"},
		{42.0, "42"},
		{0.123456789, "0.123456789"},
	}

	for _, tc := range floatTests {
		result := formatFloat(tc.input)
		if result != tc.expected {
			t.Errorf("formatFloat(%f): expected '%s', got '%s'", tc.input, tc.expected, result)
		}
	}

	// Test joinMetrics
	metrics := []string{"metric1", "metric2", "metric3"}
	result := joinMetrics(metrics)
	expected := "metric1\nmetric2\nmetric3\n"
	if result != expected {
		t.Errorf("joinMetrics: expected '%s', got '%s'", expected, result)
	}

	// Test empty slice
	emptyResult := joinMetrics([]string{})
	if emptyResult != "\n" {
		t.Errorf("joinMetrics with empty slice: expected '\\n', got '%s'", emptyResult)
	}
}

func BenchmarkRecordHTTPRequest(b *testing.B) {
	metrics := NewPrometheusMetrics()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordHTTPRequest("GET", "/api/test", 200, 100*time.Millisecond)
	}
}

func BenchmarkGenerateMetrics(b *testing.B) {
	metrics := NewPrometheusMetrics()

	// Add some test data
	for i := 0; i < 100; i++ {
		metrics.RecordHTTPRequest("GET", "/test", 200, time.Duration(i)*time.Millisecond)
		metrics.RecordHTTPRequest("POST", "/create", 201, time.Duration(i*2)*time.Millisecond)
	}
	metrics.SetRepositoryCount(1000)
	metrics.SetTransactionCount(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = metrics.generateMetrics()
	}
}

func BenchmarkGinMiddleware(b *testing.B) {
	metrics := NewPrometheusMetrics()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(metrics.GinMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkConcurrentInFlight(b *testing.B) {
	metrics := NewPrometheusMetrics()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			metrics.IncHTTPInFlight()
			metrics.DecHTTPInFlight()
		}
	})
}
