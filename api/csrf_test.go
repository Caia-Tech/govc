package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCSRFProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	// Create CSRF store
	store := NewCSRFStore(time.Minute)
	
	t.Run("Generate and validate token", func(t *testing.T) {
		// Generate token
		token, err := store.GenerateToken("user123", "session456")
		assert.NoError(t, err)
		assert.NotEmpty(t, token)
		
		// Validate token
		valid := store.ValidateToken(token, "user123", "session456")
		assert.True(t, valid)
		
		// Token should be consumed (one-time use)
		valid = store.ValidateToken(token, "user123", "session456")
		assert.False(t, valid)
	})
	
	t.Run("Invalid user ID", func(t *testing.T) {
		token, err := store.GenerateToken("user123", "session456")
		assert.NoError(t, err)
		
		// Different user ID should fail
		valid := store.ValidateToken(token, "user999", "session456")
		assert.False(t, valid)
	})
	
	t.Run("Invalid session ID", func(t *testing.T) {
		token, err := store.GenerateToken("user123", "session456")
		assert.NoError(t, err)
		
		// Different session ID should fail
		valid := store.ValidateToken(token, "user123", "session999")
		assert.False(t, valid)
	})
	
	t.Run("Expired token", func(t *testing.T) {
		// Create store with very short TTL
		shortStore := NewCSRFStore(time.Millisecond)
		
		token, err := shortStore.GenerateToken("user123", "session456")
		assert.NoError(t, err)
		
		// Wait for expiration
		time.Sleep(time.Millisecond * 2)
		
		// Should be expired
		valid := shortStore.ValidateToken(token, "user123", "session456")
		assert.False(t, valid)
	})
	
	t.Run("CSRF middleware integration", func(t *testing.T) {
		// Setup test server with CSRF protection
		router := gin.New()
		router.Use(CSRFMiddleware(store, []string{"/skip"}))
		
		// Protected endpoint
		router.POST("/protected", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		
		// Skipped endpoint
		router.POST("/skip", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		
		// GET endpoint to get token
		router.GET("/token", func(c *gin.Context) {
			token := GetCSRFToken(c, store)
			c.JSON(http.StatusOK, gin.H{"token": token})
		})
		
		// Test: POST without token should fail
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/protected", bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
		
		// Test: Get token first
		w = httptest.NewRecorder()
		req = httptest.NewRequest("GET", "/token", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		
		var tokenResp map[string]string
		json.Unmarshal(w.Body.Bytes(), &tokenResp)
		token := tokenResp["token"]
		assert.NotEmpty(t, token)
		
		// Also get session ID from response headers
		sessionID := w.Header().Get("X-Session-ID")
		
		// Test: POST with valid token should succeed
		w = httptest.NewRecorder()
		req = httptest.NewRequest("POST", "/protected", bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", token)
		req.Header.Set("X-Session-ID", sessionID)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		
		// Test: Skipped path should work without token
		w = httptest.NewRecorder()
		req = httptest.NewRequest("POST", "/skip", bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
	
	t.Run("Double submit cookie pattern", func(t *testing.T) {
		router := gin.New()
		router.Use(DoubleSubmitCookieCSRF())
		
		router.POST("/protected", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		
		router.GET("/", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		
		// GET request should set cookie and header
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		
		// Get token from header
		token := w.Header().Get("X-CSRF-Token")
		assert.NotEmpty(t, token)
		
		// Get cookie
		cookies := w.Result().Cookies()
		var csrfCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "csrf_token" {
				csrfCookie = cookie
				break
			}
		}
		assert.NotNil(t, csrfCookie)
		// Cookie values are URL-encoded when set, so we need to decode for comparison
		decodedCookieValue, err := url.QueryUnescape(csrfCookie.Value)
		assert.NoError(t, err)
		assert.Equal(t, token, decodedCookieValue)
		
		// POST with matching token should work
		w = httptest.NewRecorder()
		req = httptest.NewRequest("POST", "/protected", bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", token)
		req.AddCookie(csrfCookie)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		
		// POST with mismatched token should fail
		w = httptest.NewRecorder()
		req = httptest.NewRequest("POST", "/protected", bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", "wrong-token")
		req.AddCookie(csrfCookie)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}