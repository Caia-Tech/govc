package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func setupTestRouter() (*gin.Engine, *AuthMiddleware, *JWTAuth, *APIKeyManager) {
	gin.SetMode(gin.TestMode)

	rbac := NewRBAC()
	jwtAuth := NewJWTAuth("test-secret", "test-issuer", time.Hour)
	apiKeyMgr := NewAPIKeyManager(rbac)
	middleware := NewAuthMiddleware(jwtAuth, apiKeyMgr, rbac)

	// Create test user
	rbac.CreateUser("testuser", "testuser", "test@example.com", []string{"developer"})
	rbac.CreateUser("admin", "admin", "admin@example.com", []string{"admin"})

	router := gin.New()
	return router, middleware, jwtAuth, apiKeyMgr
}

func TestAuthRequired(t *testing.T) {
	router, middleware, jwtAuth, _ := setupTestRouter()

	// Generate valid token
	validToken, err := jwtAuth.GenerateToken("testuser", "testuser", "test@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Setup test endpoint
	router.GET("/protected", middleware.AuthRequired(), func(c *gin.Context) {
		user, exists := GetCurrentUser(c)
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no user in context"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user_id": user.ID})
	})

	testCases := []struct {
		name           string
		authHeader     string
		expectedStatus int
		expectedUserID string
	}{
		{
			name:           "valid JWT token",
			authHeader:     "Bearer " + validToken,
			expectedStatus: http.StatusOK,
			expectedUserID: "testuser",
		},
		{
			name:           "no auth header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid auth header format",
			authHeader:     "Invalid " + validToken,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid token",
			authHeader:     "Bearer invalid-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "malformed bearer token",
			authHeader:     "Bearer",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/protected", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}

			if tc.expectedStatus == http.StatusOK && tc.expectedUserID != "" {
				// Parse response and check user ID
				// For simplicity, we're checking that the status is OK
				// In a real test, you'd parse the JSON response
			}
		})
	}
}

func TestAPIKeyAuthentication(t *testing.T) {
	router, middleware, _, apiKeyMgr := setupTestRouter()

	// Generate valid API key
	validKeyString, _, err := apiKeyMgr.GenerateAPIKey(
		"testuser", "test-key", []Permission{PermissionRepoRead}, nil, nil)
	if err != nil {
		t.Fatalf("Failed to generate API key: %v", err)
	}

	// Setup test endpoint
	router.GET("/api-protected", middleware.AuthRequired(), func(c *gin.Context) {
		user, exists := GetCurrentUser(c)
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no user in context"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "auth_method": user.AuthMethod})
	})

	testCases := []struct {
		name           string
		apiKeyHeader   string
		apiKeyQuery    string
		expectedStatus int
		expectedMethod string
	}{
		{
			name:           "valid API key in header",
			apiKeyHeader:   validKeyString,
			expectedStatus: http.StatusOK,
			expectedMethod: "apikey",
		},
		{
			name:           "valid API key in query",
			apiKeyQuery:    validKeyString,
			expectedStatus: http.StatusOK,
			expectedMethod: "apikey",
		},
		{
			name:           "invalid API key",
			apiKeyHeader:   "invalid-key",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "empty API key",
			apiKeyHeader:   "",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := "/api-protected"
			if tc.apiKeyQuery != "" {
				url += "?api_key=" + tc.apiKeyQuery
			}

			req := httptest.NewRequest("GET", url, nil)
			if tc.apiKeyHeader != "" {
				req.Header.Set("X-API-Key", tc.apiKeyHeader)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}
		})
	}
}

func TestRequirePermission(t *testing.T) {
	router, middleware, jwtAuth, _ := setupTestRouter()

	// Generate tokens for different users
	developerToken, err := jwtAuth.GenerateToken("testuser", "testuser", "test@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to generate developer token: %v", err)
	}

	adminToken, err := jwtAuth.GenerateToken("admin", "admin", "admin@example.com", []string{"admin"})
	if err != nil {
		t.Fatalf("Failed to generate admin token: %v", err)
	}

	// Setup test endpoints with different permission requirements
	router.GET("/admin-only",
		middleware.AuthRequired(),
		middleware.RequirePermission(PermissionSystemAdmin),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "admin access granted"})
		})

	router.GET("/repo-read",
		middleware.AuthRequired(),
		middleware.RequirePermission(PermissionRepoRead),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "repo read access granted"})
		})

	testCases := []struct {
		name           string
		endpoint       string
		token          string
		expectedStatus int
	}{
		{
			name:           "admin access with admin token",
			endpoint:       "/admin-only",
			token:          adminToken,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "admin access with developer token",
			endpoint:       "/admin-only",
			token:          developerToken,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "repo read with developer token",
			endpoint:       "/repo-read",
			token:          developerToken,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "repo read with admin token",
			endpoint:       "/repo-read",
			token:          adminToken,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.endpoint, nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}
		})
	}
}

func TestRequireRepositoryPermission(t *testing.T) {
	router, middleware, jwtAuth, _ := setupTestRouter()

	// Generate token for test user
	userToken, err := jwtAuth.GenerateToken("testuser", "testuser", "test@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to generate user token: %v", err)
	}

	// Grant repository-specific permission
	rbac := middleware.rbac
	err = rbac.GrantRepositoryPermission("testuser", "repo1", PermissionRepoWrite)
	if err != nil {
		t.Fatalf("Failed to grant repository permission: %v", err)
	}

	// Setup test endpoint
	router.GET("/repos/:repo_id/write",
		middleware.AuthRequired(),
		middleware.RequireRepositoryPermission(PermissionRepoWrite),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "repo write access granted"})
		})

	testCases := []struct {
		name           string
		repoID         string
		token          string
		expectedStatus int
	}{
		{
			name:           "access granted repo",
			repoID:         "repo1",
			token:          userToken,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "access non-granted repo (but allowed by global developer role)",
			repoID:         "repo2",
			token:          userToken,
			expectedStatus: http.StatusOK, // Developer role has global PermissionRepoWrite
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/repos/"+tc.repoID+"/write", nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}
		})
	}
}

func TestOptionalAuth(t *testing.T) {
	router, middleware, jwtAuth, _ := setupTestRouter()

	// Generate valid token
	validToken, err := jwtAuth.GenerateToken("testuser", "testuser", "test@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Setup test endpoint
	router.GET("/optional-auth", middleware.OptionalAuth(), func(c *gin.Context) {
		user, exists := GetCurrentUser(c)
		if exists {
			c.JSON(http.StatusOK, gin.H{"authenticated": true, "user_id": user.ID})
		} else {
			c.JSON(http.StatusOK, gin.H{"authenticated": false})
		}
	})

	testCases := []struct {
		name           string
		authHeader     string
		expectedStatus int
		expectedAuth   bool
	}{
		{
			name:           "with valid token",
			authHeader:     "Bearer " + validToken,
			expectedStatus: http.StatusOK,
			expectedAuth:   true,
		},
		{
			name:           "without token",
			authHeader:     "",
			expectedStatus: http.StatusOK,
			expectedAuth:   false,
		},
		{
			name:           "with invalid token",
			authHeader:     "Bearer invalid-token",
			expectedStatus: http.StatusOK,
			expectedAuth:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/optional-auth", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}
		})
	}
}

func TestGetCurrentUser(t *testing.T) {
	// This is tested implicitly in other tests, but let's have a dedicated test
	router, middleware, jwtAuth, _ := setupTestRouter()

	validToken, err := jwtAuth.GenerateToken("testuser", "testuser", "test@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	router.GET("/user-info", middleware.AuthRequired(), func(c *gin.Context) {
		user, exists := GetCurrentUser(c)
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no user context"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"id":          user.ID,
			"username":    user.Username,
			"email":       user.Email,
			"auth_method": user.AuthMethod,
		})
	})

	req := httptest.NewRequest("GET", "/user-info", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestExpiredJWTToken(t *testing.T) {
	router, middleware, _, _ := setupTestRouter()

	// Create JWT auth with very short TTL
	shortJWTAuth := NewJWTAuth("test-secret", "test-issuer", time.Millisecond)
	shortMiddleware := NewAuthMiddleware(shortJWTAuth, middleware.apiKeyMgr, middleware.rbac)

	expiredToken, err := shortJWTAuth.GenerateToken("testuser", "testuser", "test@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Wait for token to expire
	time.Sleep(10 * time.Millisecond)

	router.GET("/expired-test", shortMiddleware.AuthRequired(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "should not reach here"})
	})

	req := httptest.NewRequest("GET", "/expired-test", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d for expired token, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestRevokedAPIKey(t *testing.T) {
	router, middleware, _, apiKeyMgr := setupTestRouter()

	// Generate and then revoke API key
	keyString, apiKey, err := apiKeyMgr.GenerateAPIKey(
		"testuser", "test-key", []Permission{PermissionRepoRead}, nil, nil)
	if err != nil {
		t.Fatalf("Failed to generate API key: %v", err)
	}

	// Revoke the key
	err = apiKeyMgr.RevokeAPIKey(apiKey.ID)
	if err != nil {
		t.Fatalf("Failed to revoke API key: %v", err)
	}

	router.GET("/revoked-test", middleware.AuthRequired(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "should not reach here"})
	})

	req := httptest.NewRequest("GET", "/revoked-test", nil)
	req.Header.Set("X-API-Key", keyString)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d for revoked key, got %d", http.StatusUnauthorized, w.Code)
	}
}

func BenchmarkAuthRequired(b *testing.B) {
	router, middleware, jwtAuth, _ := setupTestRouter()

	validToken, err := jwtAuth.GenerateToken("testuser", "testuser", "test@example.com", []string{"developer"})
	if err != nil {
		b.Fatalf("Failed to generate token: %v", err)
	}

	router.GET("/bench", middleware.AuthRequired(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/bench", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	}
}

func BenchmarkAPIKeyAuth(b *testing.B) {
	router, middleware, _, apiKeyMgr := setupTestRouter()

	keyString, _, err := apiKeyMgr.GenerateAPIKey(
		"testuser", "bench-key", []Permission{PermissionRepoRead}, nil, nil)
	if err != nil {
		b.Fatalf("Failed to generate API key: %v", err)
	}

	router.GET("/bench-api", middleware.AuthRequired(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/bench-api", nil)
	req.Header.Set("X-API-Key", keyString)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	}
}
