package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// CSRFToken represents a CSRF token with metadata
type CSRFToken struct {
	Token     string
	CreatedAt time.Time
	UserID    string
	SessionID string
}

// CSRFStore manages CSRF tokens
type CSRFStore struct {
	mu     sync.RWMutex
	tokens map[string]*CSRFToken
	ttl    time.Duration
}

// NewCSRFStore creates a new CSRF token store
func NewCSRFStore(ttl time.Duration) *CSRFStore {
	store := &CSRFStore{
		tokens: make(map[string]*CSRFToken),
		ttl:    ttl,
	}
	
	// Start cleanup goroutine
	go store.cleanupExpired()
	
	return store
}

// GenerateToken creates a new CSRF token
func (s *CSRFStore) GenerateToken(userID, sessionID string) (string, error) {
	// Generate 32 bytes of random data
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	
	// Encode to base64
	token := base64.URLEncoding.EncodeToString(tokenBytes)
	
	// Store token
	s.mu.Lock()
	s.tokens[token] = &CSRFToken{
		Token:     token,
		CreatedAt: time.Now(),
		UserID:    userID,
		SessionID: sessionID,
	}
	s.mu.Unlock()
	
	return token, nil
}

// ValidateToken checks if a token is valid
func (s *CSRFStore) ValidateToken(token, userID, sessionID string) bool {
	s.mu.RLock()
	csrfToken, exists := s.tokens[token]
	s.mu.RUnlock()
	
	if !exists {
		return false
	}
	
	// Check if expired
	if time.Since(csrfToken.CreatedAt) > s.ttl {
		s.removeToken(token)
		return false
	}
	
	// Validate user and session
	if userID != "" && csrfToken.UserID != userID {
		return false
	}
	
	if sessionID != "" && csrfToken.SessionID != sessionID {
		return false
	}
	
	// Remove token after use (one-time use)
	s.removeToken(token)
	
	return true
}

// removeToken removes a token from the store
func (s *CSRFStore) removeToken(token string) {
	s.mu.Lock()
	delete(s.tokens, token)
	s.mu.Unlock()
}

// cleanupExpired periodically removes expired tokens
func (s *CSRFStore) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for token, csrfToken := range s.tokens {
			if now.Sub(csrfToken.CreatedAt) > s.ttl {
				delete(s.tokens, token)
			}
		}
		s.mu.Unlock()
	}
}

// GetCSRFToken generates and returns a CSRF token
func GetCSRFToken(c *gin.Context, store *CSRFStore) string {
	// Get user ID from context (if authenticated)
	userID := ""
	if user, exists := c.Get("user_id"); exists {
		userID = user.(string)
	}
	
	// Get session ID
	sessionID := c.GetHeader("X-Session-ID")
	if sessionID == "" {
		// Generate session ID if not present
		sessionBytes := make([]byte, 16)
		rand.Read(sessionBytes)
		sessionID = base64.URLEncoding.EncodeToString(sessionBytes)
		c.Header("X-Session-ID", sessionID)
	}
	
	// Generate token
	token, err := store.GenerateToken(userID, sessionID)
	if err != nil {
		return ""
	}
	
	// Set token in response header
	c.Header("X-CSRF-Token", token)
	
	return token
}

// ValidateCSRFToken validates the CSRF token from the request
func ValidateCSRFToken(c *gin.Context, store *CSRFStore) bool {
	// Get token from request
	token := c.GetHeader("X-CSRF-Token")
	if token == "" {
		token = c.PostForm("csrf_token")
	}
	
	if token == "" {
		return false
	}
	
	// Get user ID from context
	userID := ""
	if user, exists := c.Get("user_id"); exists {
		userID = user.(string)
	}
	
	// Get session ID
	sessionID := c.GetHeader("X-Session-ID")
	
	// Validate token
	return store.ValidateToken(token, userID, sessionID)
}

// CSRFMiddleware provides enhanced CSRF protection
func CSRFMiddleware(store *CSRFStore, skipPaths []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip CSRF for certain paths
		for _, path := range skipPaths {
			if strings.HasPrefix(c.Request.URL.Path, path) {
				c.Next()
				return
			}
		}
		
		// Skip CSRF for safe methods
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
			// Generate new token for GET requests
			GetCSRFToken(c, store)
			c.Next()
			return
		}
		
		// Validate CSRF token for state-changing operations
		if !ValidateCSRFToken(c, store) {
			c.JSON(http.StatusForbidden, ErrorResponse{
				Error: "Invalid or missing CSRF token",
				Code:  "CSRF_TOKEN_INVALID",
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// DoubleSubmitCookieCSRF implements the double submit cookie pattern
func DoubleSubmitCookieCSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip for safe methods
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
			// Set CSRF cookie on GET requests
			token := generateRandomToken(32)
			c.SetCookie("csrf_token", token, 3600, "/", "", false, true)
			c.Header("X-CSRF-Token", token)
			c.Next()
			return
		}
		
		// Get token from cookie
		cookieToken, err := c.Cookie("csrf_token")
		if err != nil {
			c.JSON(http.StatusForbidden, ErrorResponse{
				Error: "CSRF cookie not found",
				Code:  "CSRF_COOKIE_MISSING",
			})
			c.Abort()
			return
		}
		
		// Get token from header or form
		headerToken := c.GetHeader("X-CSRF-Token")
		if headerToken == "" {
			headerToken = c.PostForm("csrf_token")
		}
		
		// Compare tokens using constant time comparison
		if headerToken == "" || subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) != 1 {
			c.JSON(http.StatusForbidden, ErrorResponse{
				Error: "CSRF token mismatch",
				Code:  "CSRF_TOKEN_MISMATCH",
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// generateRandomToken generates a random token
func generateRandomToken(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}