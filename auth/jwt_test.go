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
