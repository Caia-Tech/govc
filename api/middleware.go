package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/caiatech/govc/logging"
	"github.com/gin-gonic/gin"
)

// Note: Simple AuthMiddleware was removed as it's replaced by the comprehensive
// auth package with JWT authentication, RBAC, and API key management.

// RateLimitMiddleware implements basic rate limiting
func RateLimitMiddleware(requestsPerMinute int) gin.HandlerFunc {
	// Simple in-memory rate limiter
	// In production, use Redis or similar
	type client struct {
		count     int
		resetTime int64
	}

	clients := make(map[string]*client)
	var mu sync.RWMutex

	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		now := time.Now().Unix()

		mu.Lock()
		defer mu.Unlock()

		cl, exists := clients[clientIP]
		if !exists || now > cl.resetTime {
			clients[clientIP] = &client{
				count:     1,
				resetTime: now + 60,
			}
			c.Next()
			return
		}

		if cl.count >= requestsPerMinute {
			c.JSON(http.StatusTooManyRequests, ErrorResponse{
				Error: "rate limit exceeded",
				Code:  "RATE_LIMIT_EXCEEDED",
			})
			c.Abort()
			return
		}

		cl.count++
		c.Next()
	}
}

// TimeoutMiddleware creates a middleware that enforces request timeouts
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip timeout for websocket connections and streaming endpoints
		if c.GetHeader("Upgrade") == "websocket" ||
			strings.Contains(c.Request.URL.Path, "/watch") ||
			strings.Contains(c.Request.URL.Path, "/events") {
			c.Next()
			return
		}

		// Create a context with timeout
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		// Replace the request context
		c.Request = c.Request.WithContext(ctx)

		// Channel to signal completion
		finished := make(chan struct{})

		go func() {
			defer close(finished)
			c.Next()
		}()

		select {
		case <-finished:
			// Request completed within timeout
			return
		case <-ctx.Done():
			// Request timed out
			c.JSON(http.StatusRequestTimeout, ErrorResponse{
				Error: "Request timeout",
				Code:  "REQUEST_TIMEOUT",
				Details: map[string]interface{}{
					"timeout": timeout.String(),
				},
			})
			c.Abort()
		}
	}
}

// RequestSizeMiddleware limits the size of request bodies
func RequestSizeMiddleware(maxSize int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxSize {
			c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
				Error: "Request body too large",
				Code:  "REQUEST_TOO_LARGE",
				Details: map[string]interface{}{
					"max_size":    maxSize,
					"actual_size": c.Request.ContentLength,
				},
			})
			c.Abort()
			return
		}

		// Limit the request body reader
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)
		c.Next()
	}
}

// CORSMiddleware handles Cross-Origin Resource Sharing
func CORSMiddleware(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range allowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-API-Key")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}

// SecurityHeadersMiddleware adds security headers to responses
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	}
}

// LoggerMiddleware provides request logging
func LoggerMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
			param.ClientIP,
			param.TimeStamp.Format("02/Jan/2006:15:04:05 -0700"),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
	})
}

// PerformanceLoggingMiddleware provides detailed performance logging
func PerformanceLoggingMiddleware(logger *logging.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		userAgent := c.Request.UserAgent()
		clientIP := c.ClientIP()
		
		// Process request
		c.Next()
		
		// Calculate metrics
		latency := time.Since(start)
		status := c.Writer.Status()
		bodySize := c.Writer.Size()
		
		// Determine log level based on status and latency
		logLevel := logging.InfoLevel
		if status >= 400 && status < 500 {
			logLevel = logging.WarnLevel
		} else if status >= 500 {
			logLevel = logging.ErrorLevel
		} else if latency > 1*time.Second {
			logLevel = logging.WarnLevel // Slow requests
		}

		// Log the request with structured data
		fields := map[string]interface{}{
			"method":          method,
			"path":            path,
			"status":          status,
			"latency_ms":      latency.Milliseconds(),
			"latency_human":   latency.String(),
			"client_ip":       clientIP,
			"user_agent":      userAgent,
			"response_size":   bodySize,
			"request_id":      logging.GetRequestID(c),
		}

		// Add query parameters if present
		if len(c.Request.URL.RawQuery) > 0 {
			fields["query"] = c.Request.URL.RawQuery
		}

		// Add user ID if available
		if userID := logging.GetUserID(c); userID != "" {
			fields["user_id"] = userID
		}

		// Add content type if present
		if contentType := c.GetHeader("Content-Type"); contentType != "" {
			fields["content_type"] = contentType
		}

		loggerWithFields := logger.WithFields(fields)
		switch logLevel {
		case logging.DebugLevel:
			loggerWithFields.Debug("HTTP request completed")
		case logging.InfoLevel:
			loggerWithFields.Info("HTTP request completed")
		case logging.WarnLevel:
			loggerWithFields.Warn("HTTP request completed")
		case logging.ErrorLevel:
			loggerWithFields.Error("HTTP request completed")
		default:
			loggerWithFields.Info("HTTP request completed")
		}
	}
}
