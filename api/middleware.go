package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware provides simple token-based authentication
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: "missing authorization header",
				Code:  "UNAUTHORIZED",
			})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>" format
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: "invalid authorization header format",
				Code:  "INVALID_AUTH_FORMAT",
			})
			c.Abort()
			return
		}

		token := parts[1]

		// TODO: Implement proper token validation
		// For now, accept any non-empty token
		if token == "" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: "invalid token",
				Code:  "INVALID_TOKEN",
			})
			c.Abort()
			return
		}

		// Store token in context for use in handlers
		c.Set("token", token)
		c.Next()
	}
}

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
				Error:   "Request timeout",
				Code:    "REQUEST_TIMEOUT",
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
				Error:   "Request body too large",
				Code:    "REQUEST_TOO_LARGE",
				Details: map[string]interface{}{
					"max_size": maxSize,
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