package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/caia-tech/govc/config"
	"github.com/gin-gonic/gin"
)

// TestTimeoutMiddleware tests the timeout middleware functionality
func TestTimeoutMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Request completes before timeout", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Server.RequestTimeout = 5 * time.Second
		cfg.Auth.Enabled = false
		
		server := NewServer(cfg)
		router := gin.New()
		router.Use(TimeoutMiddleware(cfg.Server.RequestTimeout))
		server.RegisterRoutes(router)
		
		// Create a test repository first
		body := bytes.NewBufferString(`{"id": "timeout-test", "memory_only": true}`)
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusCreated {
			t.Fatalf("Expected status %d, got %d", http.StatusCreated, w.Code)
		}
	})

	t.Run("Request times out", func(t *testing.T) {
		router := gin.New()
		router.Use(TimeoutMiddleware(100 * time.Millisecond))
		
		// Add a slow handler
		router.GET("/slow", func(c *gin.Context) {
			time.Sleep(200 * time.Millisecond)
			c.JSON(http.StatusOK, gin.H{"message": "slow response"})
		})
		
		req := httptest.NewRequest("GET", "/slow", nil)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusRequestTimeout {
			t.Errorf("Expected status %d, got %d", http.StatusRequestTimeout, w.Code)
		}
	})

	t.Run("WebSocket connection bypasses timeout", func(t *testing.T) {
		router := gin.New()
		router.Use(TimeoutMiddleware(100 * time.Millisecond))
		
		// Add a handler that would normally timeout
		router.GET("/websocket", func(c *gin.Context) {
			time.Sleep(200 * time.Millisecond)
			c.JSON(http.StatusOK, gin.H{"message": "websocket response"})
		})
		
		req := httptest.NewRequest("GET", "/websocket", nil)
		req.Header.Set("Upgrade", "websocket")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Event stream bypasses timeout", func(t *testing.T) {
		router := gin.New()
		router.Use(TimeoutMiddleware(100 * time.Millisecond))
		
		// Add a handler that would normally timeout
		router.GET("/api/v1/repos/test/events", func(c *gin.Context) {
			time.Sleep(200 * time.Millisecond)
			c.JSON(http.StatusOK, gin.H{"message": "events response"})
		})
		
		req := httptest.NewRequest("GET", "/api/v1/repos/test/events", nil)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})
}

// TestRequestSizeMiddleware tests the request size limiting middleware
func TestRequestSizeMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Request within size limit", func(t *testing.T) {
		router := gin.New()
		router.Use(RequestSizeMiddleware(1024)) // 1KB limit
		
		router.POST("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "ok"})
		})
		
		body := bytes.NewBufferString(`{"data": "small request"}`)
		req := httptest.NewRequest("POST", "/test", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Request exceeds size limit", func(t *testing.T) {
		router := gin.New()
		router.Use(RequestSizeMiddleware(100)) // 100 bytes limit
		
		router.POST("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "ok"})
		})
		
		// Create a request larger than 100 bytes
		largeData := make([]byte, 200)
		for i := range largeData {
			largeData[i] = 'A'
		}
		
		body := bytes.NewBuffer(largeData)
		req := httptest.NewRequest("POST", "/test", body)
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = int64(len(largeData))
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("Expected status %d, got %d", http.StatusRequestEntityTooLarge, w.Code)
		}
	})
}

// TestSecurityHeadersMiddleware tests security headers are added
func TestSecurityHeadersMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	router := gin.New()
	router.Use(SecurityHeadersMiddleware())
	
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})
	
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	// Check security headers
	expectedHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block", 
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"Content-Security-Policy": "default-src 'self'",
	}
	
	for header, expectedValue := range expectedHeaders {
		if w.Header().Get(header) != expectedValue {
			t.Errorf("Expected header %s to be %s, got %s", 
				header, expectedValue, w.Header().Get(header))
		}
	}
}

// TestCORSMiddleware tests CORS functionality
func TestCORSMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	t.Run("Allowed origin", func(t *testing.T) {
		router := gin.New()
		router.Use(CORSMiddleware([]string{"https://example.com"}))
		
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "ok"})
		})
		
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
			t.Errorf("Expected CORS origin header to be set")
		}
	})

	t.Run("Wildcard origin", func(t *testing.T) {
		router := gin.New()
		router.Use(CORSMiddleware([]string{"*"}))
		
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "ok"})
		})
		
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://any-origin.com")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Header().Get("Access-Control-Allow-Origin") != "https://any-origin.com" {
			t.Errorf("Expected CORS origin header to be set for wildcard")
		}
	})

	t.Run("OPTIONS preflight", func(t *testing.T) {
		router := gin.New()
		router.Use(CORSMiddleware([]string{"*"}))
		
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "ok"})
		})
		
		req := httptest.NewRequest("OPTIONS", "/test", nil)
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d for OPTIONS, got %d", http.StatusOK, w.Code)
		}
	})
}