package api

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Caia-Tech/govc/logging"
	"github.com/Caia-Tech/govc/validation"
	"github.com/gin-gonic/gin"
)

// SecurityConfig holds security configuration
type SecurityConfig struct {
	// Path sanitization
	EnablePathSanitization bool
	
	// Request validation
	MaxPathLength     int
	MaxHeaderSize     int
	MaxQueryLength    int
	
	// Rate limiting per IP
	RateLimitPerMinute int
	
	// Brute force protection
	MaxLoginAttempts      int
	LoginLockoutDuration  time.Duration
	
	// Content Security
	AllowedContentTypes []string
	MaxFileUploadSize   int64
}

// DefaultSecurityConfig returns secure defaults
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		EnablePathSanitization: true,
		MaxPathLength:         4096,
		MaxHeaderSize:         8192,
		MaxQueryLength:        2048,
		RateLimitPerMinute:    60,
		MaxLoginAttempts:      5,
		LoginLockoutDuration:  15 * time.Minute,
		AllowedContentTypes: []string{
			"application/json",
			"text/plain",
			"application/octet-stream",
		},
		MaxFileUploadSize: 50 * 1024 * 1024, // 50MB
	}
}

// loginAttempt tracks login attempts for brute force protection
type loginAttempt struct {
	count      int
	lastAttempt time.Time
	lockedUntil time.Time
}

// SecurityMiddleware provides comprehensive security checks
func SecurityMiddleware(config SecurityConfig, logger *logging.Logger) gin.HandlerFunc {
	// Track login attempts
	loginAttempts := make(map[string]*loginAttempt)
	var mu sync.RWMutex
	
	return func(c *gin.Context) {
		// 1. Check path length
		if len(c.Request.URL.Path) > config.MaxPathLength {
			logger.WithFields(map[string]interface{}{
				"path":       c.Request.URL.Path,
				"path_length": len(c.Request.URL.Path),
				"max_length": config.MaxPathLength,
				"client_ip":  c.ClientIP(),
			}).Warn("Path too long")
			
			c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
				Error: "Request path too long",
				Code:  "PATH_TOO_LONG",
			})
			c.Abort()
			return
		}
		
		// 2. Check query string length
		if len(c.Request.URL.RawQuery) > config.MaxQueryLength {
			logger.WithFields(map[string]interface{}{
				"query_length": len(c.Request.URL.RawQuery),
				"max_length":   config.MaxQueryLength,
				"client_ip":    c.ClientIP(),
			}).Warn("Query string too long")
			
			c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
				Error: "Query string too long",
				Code:  "QUERY_TOO_LONG",
			})
			c.Abort()
			return
		}
		
		// 3. Check header size
		headerSize := 0
		for k, v := range c.Request.Header {
			headerSize += len(k)
			for _, val := range v {
				headerSize += len(val)
			}
		}
		if headerSize > config.MaxHeaderSize {
			logger.WithFields(map[string]interface{}{
				"header_size": headerSize,
				"max_size":    config.MaxHeaderSize,
				"client_ip":   c.ClientIP(),
			}).Warn("Headers too large")
			
			c.JSON(http.StatusRequestHeaderFieldsTooLarge, ErrorResponse{
				Error: "Request headers too large",
				Code:  "HEADERS_TOO_LARGE",
			})
			c.Abort()
			return
		}
		
		// 4. Path sanitization for file operations
		if config.EnablePathSanitization && strings.Contains(c.Request.URL.Path, "files") {
			// Extract path parameter or query
			filePath := c.Query("path")
			if filePath == "" {
				filePath = c.Param("path")
			}
			
			if filePath != "" {
				// Validate the file path
				if err := validation.ValidateFilePath(filePath); err != nil {
					logger.WithFields(map[string]interface{}{
						"file_path": filePath,
						"error":     err.Error(),
						"client_ip": c.ClientIP(),
					}).Warn("Invalid file path detected")
					
					c.JSON(http.StatusBadRequest, ErrorResponse{
						Error: err.Error(),
						Code:  "INVALID_FILE_PATH",
					})
					c.Abort()
					return
				}
				
				// Sanitize the path
				sanitized := validation.SanitizeFilePath(filePath)
				if filePath != sanitized {
					logger.WithFields(map[string]interface{}{
						"original_path":  filePath,
						"sanitized_path": sanitized,
						"client_ip":      c.ClientIP(),
					}).Info("Path sanitized")
				}
				
				// Update the path in the context
				if c.Query("path") != "" {
					c.Request.URL.RawQuery = strings.Replace(c.Request.URL.RawQuery, 
						"path="+filePath, "path="+sanitized, 1)
				}
			}
		}
		
		// 5. Brute force protection for login endpoints
		if strings.Contains(c.Request.URL.Path, "/login") && c.Request.Method == "POST" {
			clientIP := c.ClientIP()
			
			mu.Lock()
			attempt, exists := loginAttempts[clientIP]
			if !exists {
				attempt = &loginAttempt{}
				loginAttempts[clientIP] = attempt
			}
			
			// Check if locked out
			if time.Now().Before(attempt.lockedUntil) {
				mu.Unlock()
				
				logger.WithFields(map[string]interface{}{
					"client_ip":     clientIP,
					"locked_until":  attempt.lockedUntil,
					"attempt_count": attempt.count,
				}).Warn("Login attempt during lockout period")
				
				c.JSON(http.StatusTooManyRequests, ErrorResponse{
					Error: fmt.Sprintf("Too many login attempts. Try again after %s", 
						attempt.lockedUntil.Format(time.RFC3339)),
					Code: "LOGIN_LOCKOUT",
				})
				c.Abort()
				return
			}
			
			// Reset count if last attempt was long ago
			if time.Since(attempt.lastAttempt) > config.LoginLockoutDuration {
				attempt.count = 0
			}
			
			attempt.count++
			attempt.lastAttempt = time.Now()
			
			// Lock out if too many attempts
			if attempt.count >= config.MaxLoginAttempts {
				attempt.lockedUntil = time.Now().Add(config.LoginLockoutDuration)
				
				logger.WithFields(map[string]interface{}{
					"client_ip":     clientIP,
					"attempt_count": attempt.count,
					"lockout_duration": config.LoginLockoutDuration,
				}).Warn("Login lockout triggered")
			}
			
			mu.Unlock()
		}
		
		// 6. Content-Type validation for uploads
		if c.Request.Method == "POST" || c.Request.Method == "PUT" {
			contentType := c.GetHeader("Content-Type")
			if contentType != "" {
				// Extract base content type (without charset, etc.)
				baseType := strings.Split(contentType, ";")[0]
				baseType = strings.TrimSpace(baseType)
				
				allowed := false
				for _, allowedType := range config.AllowedContentTypes {
					if baseType == allowedType {
						allowed = true
						break
					}
				}
				
				if !allowed {
					logger.WithFields(map[string]interface{}{
						"content_type": contentType,
						"client_ip":    c.ClientIP(),
					}).Warn("Disallowed content type")
					
					c.JSON(http.StatusUnsupportedMediaType, ErrorResponse{
						Error: fmt.Sprintf("Content type '%s' not allowed", contentType),
						Code:  "UNSUPPORTED_CONTENT_TYPE",
					})
					c.Abort()
					return
				}
			}
		}
		
		// Continue to next handler
		c.Next()
		
		// After request: Clear successful login attempts
		if strings.Contains(c.Request.URL.Path, "/login") && 
		   c.Request.Method == "POST" && 
		   c.Writer.Status() == http.StatusOK {
			clientIP := c.ClientIP()
			mu.Lock()
			delete(loginAttempts, clientIP)
			mu.Unlock()
		}
	}
}

// XSSProtectionMiddleware adds XSS protection headers and sanitizes output
func XSSProtectionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Set XSS protection headers
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("X-Content-Type-Options", "nosniff")
		
		// For JSON responses, Gin already escapes properly
		// Additional output encoding would be done here if needed
		
		c.Next()
	}
}

// CSRFProtectionMiddleware provides CSRF protection for state-changing operations
func CSRFProtectionMiddleware(skipPaths []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip CSRF for certain paths (e.g., login, public endpoints)
		for _, path := range skipPaths {
			if strings.HasPrefix(c.Request.URL.Path, path) {
				c.Next()
				return
			}
		}
		
		// Skip CSRF for read operations
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}
		
		// Check for CSRF token in header or form
		csrfToken := c.GetHeader("X-CSRF-Token")
		if csrfToken == "" {
			csrfToken = c.PostForm("csrf_token")
		}
		
		// For now, we're using a simplified check
		// In production, this would validate against a server-generated token
		if csrfToken == "" {
			c.JSON(http.StatusForbidden, ErrorResponse{
				Error: "CSRF token required",
				Code:  "CSRF_TOKEN_MISSING",
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}