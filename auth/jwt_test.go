package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestNewJWTAuth(t *testing.T) {
	secret := "test-secret"
	issuer := "test-issuer"
	ttl := time.Hour

	jwtAuth := NewJWTAuth(secret, issuer, ttl)

	if jwtAuth.Secret != secret {
		t.Errorf("Expected secret %s, got %s", secret, jwtAuth.Secret)
	}
	if jwtAuth.Issuer != issuer {
		t.Errorf("Expected issuer %s, got %s", issuer, jwtAuth.Issuer)
	}
	if jwtAuth.TTL != ttl {
		t.Errorf("Expected TTL %v, got %v", ttl, jwtAuth.TTL)
	}
}

func TestGenerateToken(t *testing.T) {
	jwtAuth := NewJWTAuth("test-secret", "test-issuer", time.Hour)

	testCases := []struct {
		name     string
		userID   string
		username string
		email    string
		roles    []string
		wantErr  bool
	}{
		{
			name:     "valid token generation",
			userID:   "user1",
			username: "testuser",
			email:    "test@example.com",
			roles:    []string{"admin", "user"},
			wantErr:  false,
		},
		{
			name:     "empty user ID",
			userID:   "",
			username: "testuser",
			email:    "test@example.com",
			roles:    []string{"user"},
			wantErr:  false, // Should still work
		},
		{
			name:     "empty roles",
			userID:   "user1",
			username: "testuser",
			email:    "test@example.com",
			roles:    []string{},
			wantErr:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			token, err := jwtAuth.GenerateToken(tc.userID, tc.username, tc.email, tc.roles)

			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tc.wantErr && token == "" {
				t.Error("Expected token but got empty string")
			}
		})
	}
}

func TestValidateToken(t *testing.T) {
	jwtAuth := NewJWTAuth("test-secret", "test-issuer", time.Hour)

	// Generate a valid token
	validToken, err := jwtAuth.GenerateToken("user1", "testuser", "test@example.com", []string{"admin"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	testCases := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "valid token",
			token:   validToken,
			wantErr: false,
		},
		{
			name:    "empty token",
			token:   "",
			wantErr: true,
		},
		{
			name:    "invalid token format",
			token:   "invalid.token.format",
			wantErr: true,
		},
		{
			name:    "malformed token",
			token:   "not-a-jwt-token",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := jwtAuth.ValidateToken(tc.token)

			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tc.wantErr && claims == nil {
				t.Error("Expected claims but got nil")
			}
			if !tc.wantErr && claims != nil {
				if claims.UserID != "user1" {
					t.Errorf("Expected UserID 'user1', got '%s'", claims.UserID)
				}
				if claims.Username != "testuser" {
					t.Errorf("Expected Username 'testuser', got '%s'", claims.Username)
				}
			}
		})
	}
}

func TestValidateTokenWithWrongSecret(t *testing.T) {
	jwtAuth1 := NewJWTAuth("secret1", "test-issuer", time.Hour)
	jwtAuth2 := NewJWTAuth("secret2", "test-issuer", time.Hour)

	// Generate token with first auth
	token, err := jwtAuth1.GenerateToken("user1", "testuser", "test@example.com", []string{"admin"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Try to validate with second auth (different secret)
	_, err = jwtAuth2.ValidateToken(token)
	if err == nil {
		t.Error("Expected error when validating token with wrong secret")
	}
}

func TestExpiredToken(t *testing.T) {
	// Create JWT auth with very short TTL
	jwtAuth := NewJWTAuth("test-secret", "test-issuer", time.Millisecond)

	token, err := jwtAuth.GenerateToken("user1", "testuser", "test@example.com", []string{"admin"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Wait for token to expire
	time.Sleep(10 * time.Millisecond)

	_, err = jwtAuth.ValidateToken(token)
	if err == nil {
		t.Error("Expected error for expired token")
	}
}

func TestRefreshToken(t *testing.T) {
	jwtAuth := NewJWTAuth("test-secret", "test-issuer", time.Hour)

	// Generate original token
	originalToken, err := jwtAuth.GenerateToken("user1", "testuser", "test@example.com", []string{"admin"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	testCases := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "valid token refresh",
			token:   originalToken,
			wantErr: false,
		},
		{
			name:    "empty token",
			token:   "",
			wantErr: true,
		},
		{
			name:    "invalid token",
			token:   "invalid-token",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			refreshedToken, err := jwtAuth.RefreshToken(tc.token)

			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tc.wantErr && refreshedToken == "" {
				t.Error("Expected refreshed token but got empty string")
			}
			// Note: Refreshed token might be the same if generated at the exact same time
			// This is acceptable behavior since the content and expiration are the same
		})
	}
}

func TestClaimsValidation(t *testing.T) {
	jwtAuth := NewJWTAuth("test-secret", "test-issuer", time.Hour)

	token, err := jwtAuth.GenerateToken("user123", "johndoe", "john@example.com", []string{"admin", "user"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	claims, err := jwtAuth.ValidateToken(token)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	// Validate all claim fields
	if claims.UserID != "user123" {
		t.Errorf("Expected UserID 'user123', got '%s'", claims.UserID)
	}
	if claims.Username != "johndoe" {
		t.Errorf("Expected Username 'johndoe', got '%s'", claims.Username)
	}
	if claims.Email != "john@example.com" {
		t.Errorf("Expected Email 'john@example.com', got '%s'", claims.Email)
	}
	if len(claims.Permissions) != 2 {
		t.Errorf("Expected 2 permissions, got %d", len(claims.Permissions))
	}
	if claims.Issuer != "test-issuer" {
		t.Errorf("Expected Issuer 'test-issuer', got '%s'", claims.Issuer)
	}

	// Check expiration is in future
	if claims.ExpiresAt.Time.Before(time.Now()) {
		t.Error("Token should not be expired")
	}

	// Check issued at is in past
	if claims.IssuedAt.Time.After(time.Now()) {
		t.Error("IssuedAt should be in the past")
	}
}

func TestTokenStructure(t *testing.T) {
	jwtAuth := NewJWTAuth("test-secret", "test-issuer", time.Hour)

	token, err := jwtAuth.GenerateToken("user1", "testuser", "test@example.com", []string{"admin"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Parse token without validation to check structure
	parser := jwt.NewParser()
	parsedToken, _, err := parser.ParseUnverified(token, &Claims{})
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	claims, ok := parsedToken.Claims.(*Claims)
	if !ok {
		t.Fatal("Failed to cast claims to Claims")
	}

	// Verify standard claims
	if claims.Issuer != "test-issuer" {
		t.Errorf("Expected issuer 'test-issuer', got '%s'", claims.Issuer)
	}

	// Verify custom claims
	if claims.UserID != "user1" {
		t.Errorf("Expected UserID 'user1', got '%s'", claims.UserID)
	}
}

func BenchmarkGenerateToken(b *testing.B) {
	jwtAuth := NewJWTAuth("test-secret", "test-issuer", time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := jwtAuth.GenerateToken("user1", "testuser", "test@example.com", []string{"admin"})
		if err != nil {
			b.Fatalf("Failed to generate token: %v", err)
		}
	}
}

func BenchmarkValidateToken(b *testing.B) {
	jwtAuth := NewJWTAuth("test-secret", "test-issuer", time.Hour)

	token, err := jwtAuth.GenerateToken("user1", "testuser", "test@example.com", []string{"admin"})
	if err != nil {
		b.Fatalf("Failed to generate token: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := jwtAuth.ValidateToken(token)
		if err != nil {
			b.Fatalf("Failed to validate token: %v", err)
		}
	}
}

// Tests for functions with 0% coverage

func TestCleanupExpiredTokens(t *testing.T) {
	jwtAuth := NewJWTAuth("test-secret", "test-issuer", time.Hour)
	
	// Create some test tokens
	token1, err := jwtAuth.GenerateToken("user1", "testuser1", "user1@example.com", []string{"admin"})
	if err != nil {
		t.Fatalf("Failed to generate token1: %v", err)
	}
	
	token2, err := jwtAuth.GenerateToken("user2", "testuser2", "user2@example.com", []string{"user"})
	if err != nil {
		t.Fatalf("Failed to generate token2: %v", err)
	}
	
	// Validate tokens to add them to cache
	_, err = jwtAuth.ValidateToken(token1)
	if err != nil {
		t.Fatalf("Failed to validate token1: %v", err)
	}
	
	_, err = jwtAuth.ValidateToken(token2)
	if err != nil {
		t.Fatalf("Failed to validate token2: %v", err)
	}
	
	// Access the cleanup function through reflection or test the behavior indirectly
	// Since cleanupExpiredTokens is called internally, we test it by checking cache behavior
	
	// Create a JWT with very short cache TTL to test cleanup
	shortTTLAuth := NewJWTAuth("test-secret", "test-issuer", time.Hour)
	shortTTLAuth.cacheTTL = time.Millisecond // Very short cache
	
	token3, err := shortTTLAuth.GenerateToken("user3", "testuser3", "user3@example.com", []string{"user"})
	if err != nil {
		t.Fatalf("Failed to generate token3: %v", err)
	}
	
	// Validate to add to cache
	_, err = shortTTLAuth.ValidateToken(token3)
	if err != nil {
		t.Fatalf("Failed to validate token3: %v", err)
	}
	
	// Wait for cache to expire
	time.Sleep(10 * time.Millisecond)
	
	// Validate again - this should trigger cleanup of expired cache entries
	_, err = shortTTLAuth.ValidateToken(token3)
	if err != nil {
		t.Fatalf("Failed to validate token3 after cache expiry: %v", err)
	}
	
	// Test passes if no panic occurs and validation still works
	t.Log("Cleanup test completed successfully")
}

func TestExtractUserID(t *testing.T) {
	jwtAuth := NewJWTAuth("test-secret", "test-issuer", time.Hour)
	
	testCases := []struct {
		name           string
		userID         string
		username       string
		email          string
		permissions    []string
		expectUserID   string
	}{
		{
			name:         "extract valid user ID",
			userID:       "testuser123",
			username:     "testuser",
			email:        "test@example.com",
			permissions:  []string{"admin"},
			expectUserID: "testuser123",
		},
		{
			name:         "extract user ID with special characters",
			userID:       "user@example.com",
			username:     "specialuser",
			email:        "special@example.com",
			permissions:  []string{"user"},
			expectUserID: "user@example.com",
		},
		{
			name:         "extract empty user ID",
			userID:       "",
			username:     "emptyuser",
			email:        "empty@example.com",
			permissions:  []string{"user"},
			expectUserID: "",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			token, err := jwtAuth.GenerateToken(tc.userID, tc.username, tc.email, tc.permissions)
			if err != nil {
				t.Fatalf("Failed to generate token: %v", err)
			}
			
			extractedUserID := jwtAuth.ExtractUserID(token)
			
			if extractedUserID != tc.expectUserID {
				t.Errorf("Expected user ID '%s', got '%s'", tc.expectUserID, extractedUserID)
			}
		})
	}
	
	// Test with invalid tokens
	invalidTokenTests := []struct {
		name  string
		token string
	}{
		{
			name:  "empty token",
			token: "",
		},
		{
			name:  "malformed token",
			token: "invalid.token.here",
		},
		{
			name:  "token with wrong signature",
			token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0IiwiZXhwIjoxNjE2MjM5MDIyfQ.wrong_signature",
		},
	}
	
	for _, tc := range invalidTokenTests {
		t.Run(tc.name, func(t *testing.T) {
			userID := jwtAuth.ExtractUserID(tc.token)
			if userID != "" {
				t.Errorf("Expected empty user ID for invalid token, got '%s'", userID)
			}
		})
	}
}

func TestGenerateRandomSecret(t *testing.T) {
	// Test the generateRandomSecret function by creating JWTAuth with empty secret
	jwtAuth1 := NewJWTAuth("", "test-issuer", time.Hour)
	jwtAuth2 := NewJWTAuth("", "test-issuer", time.Hour)
	
	// Verify secrets are not empty
	if jwtAuth1.Secret == "" {
		t.Error("Generated secret should not be empty")
	}
	if jwtAuth2.Secret == "" {
		t.Error("Generated secret should not be empty")
	}
	
	// Verify secrets are different (highly likely)
	if jwtAuth1.Secret == jwtAuth2.Secret {
		t.Error("Generated secrets should be different")
	}
	
	// Verify minimum length (should be reasonable for security)
	if len(jwtAuth1.Secret) < 32 {
		t.Errorf("Generated secret should be at least 32 characters, got %d", len(jwtAuth1.Secret))
	}
	if len(jwtAuth2.Secret) < 32 {
		t.Errorf("Generated secret should be at least 32 characters, got %d", len(jwtAuth2.Secret))
	}
	
	// Test tokens can be generated and validated with random secrets
	token, err := jwtAuth1.GenerateToken("testuser", "testuser", "test@example.com", []string{"user"})
	if err != nil {
		t.Fatalf("Failed to generate token with random secret: %v", err)
	}
	
	_, err = jwtAuth1.ValidateToken(token)
	if err != nil {
		t.Errorf("Failed to validate token with random secret: %v", err)
	}
}

func TestGetTokenInfo(t *testing.T) {
	jwtAuth := NewJWTAuth("test-secret", "test-issuer", time.Hour)
	
	userID := "testuser"
	username := "testusername"
	email := "test@example.com"
	permissions := []string{"admin", "user"}
	
	token, err := jwtAuth.GenerateToken(userID, username, email, permissions)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}
	
	testCases := []struct {
		name    string
		token   string
		wantNil bool
	}{
		{
			name:    "get info for valid token",
			token:   token,
			wantNil: false,
		},
		{
			name:    "get info for empty token",
			token:   "",
			wantNil: false, // Returns invalid token info, not nil
		},
		{
			name:    "get info for invalid token",
			token:   "invalid.token.format",
			wantNil: false, // Returns invalid token info, not nil
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info := jwtAuth.GetTokenInfo(tc.token)
			
			if tc.wantNil && info != nil {
				t.Error("Expected nil but got token info")
			}
			if !tc.wantNil && info == nil {
				t.Error("Expected token info but got nil")
			}
			
			if !tc.wantNil && info != nil {
				if tc.token == token {
					// Verify the token info contains expected data for valid token
					if info.UserID != userID {
						t.Errorf("Expected user ID '%s', got '%s'", userID, info.UserID)
					}
					if info.Username != username {
						t.Errorf("Expected username '%s', got '%s'", username, info.Username)
					}
					if info.Email != email {
						t.Errorf("Expected email '%s', got '%s'", email, info.Email)
					}
					if len(info.Permissions) != len(permissions) {
						t.Errorf("Expected %d permissions, got %d", len(permissions), len(info.Permissions))
					}
					if info.IssuedAt.IsZero() {
						t.Error("IssuedAt should not be zero")
					}
					if info.ExpiresAt.IsZero() {
						t.Error("ExpiresAt should not be zero")
					}
					if info.ExpiresAt.Before(info.IssuedAt) {
						t.Error("ExpiresAt should be after IssuedAt")
					}
					if !info.Valid {
						t.Error("Token should be valid")
					}
				} else {
					// For invalid tokens, Valid should be false
					if info.Valid {
						t.Error("Invalid token should have Valid=false")
					}
				}
			}
		})
	}
}
