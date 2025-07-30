package api

import (
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