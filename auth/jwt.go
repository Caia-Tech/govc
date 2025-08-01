package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTAuth handles JWT token generation and validation
type JWTAuth struct {
	Secret string
	Issuer string
	TTL    time.Duration
}

// Claims represents the JWT claims
type Claims struct {
	UserID      string   `json:"user_id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	Permissions []string `json:"permissions"`
	jwt.RegisteredClaims
}

// NewJWTAuth creates a new JWT authentication handler
func NewJWTAuth(secret, issuer string, ttl time.Duration) *JWTAuth {
	if secret == "" {
		// Generate a random secret if none provided (for development)
		secret = generateRandomSecret()
	}
	
	if ttl == 0 {
		ttl = 24 * time.Hour // Default 24 hours
	}

	return &JWTAuth{
		Secret: secret,
		Issuer: issuer,
		TTL:    ttl,
	}
}

// GenerateToken creates a new JWT token for a user
func (j *JWTAuth) GenerateToken(userID, username, email string, permissions []string) (string, error) {
	now := time.Now()
	
	claims := Claims{
		UserID:      userID,
		Username:    username,
		Email:       email,
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    j.Issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.TTL)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(j.Secret))
}

// ValidateToken validates a JWT token and returns the claims
func (j *JWTAuth) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(j.Secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// RefreshToken generates a new token with the same claims but extended expiration
func (j *JWTAuth) RefreshToken(tokenString string) (string, error) {
	claims, err := j.ValidateToken(tokenString)
	if err != nil {
		return "", fmt.Errorf("cannot refresh invalid token: %w", err)
	}

	// Generate new token with same claims but extended expiration
	return j.GenerateToken(claims.UserID, claims.Username, claims.Email, claims.Permissions)
}

// ExtractUserID extracts user ID from token without full validation (for logging/metrics)
func (j *JWTAuth) ExtractUserID(tokenString string) string {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(j.Secret), nil
	})

	if err != nil {
		return ""
	}

	if claims, ok := token.Claims.(*Claims); ok {
		return claims.UserID
	}

	return ""
}

// generateRandomSecret generates a cryptographically secure random secret
func generateRandomSecret() string {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a default secret (not recommended for production)
		return "default-development-secret-change-in-production"
	}
	return hex.EncodeToString(bytes)
}

// TokenInfo represents information about a token
type TokenInfo struct {
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	Permissions []string  `json:"permissions"`
	IssuedAt    time.Time `json:"issued_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Valid       bool      `json:"valid"`
}

// GetTokenInfo returns detailed information about a token
func (j *JWTAuth) GetTokenInfo(tokenString string) *TokenInfo {
	claims, err := j.ValidateToken(tokenString)
	if err != nil {
		return &TokenInfo{Valid: false}
	}

	return &TokenInfo{
		UserID:      claims.UserID,
		Username:    claims.Username,
		Email:       claims.Email,
		Permissions: claims.Permissions,
		IssuedAt:    claims.IssuedAt.Time,
		ExpiresAt:   claims.ExpiresAt.Time,
		Valid:       true,
	}
}