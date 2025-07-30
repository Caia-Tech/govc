package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthenticatedUser represents an authenticated user context
type AuthenticatedUser struct {
	ID          string            `json:"id"`
	Username    string            `json:"username"`
	Email       string            `json:"email"`
	Permissions []Permission      `json:"permissions"`
	RepoPerms   map[string][]Permission `json:"repo_permissions"`
	AuthMethod  string            `json:"auth_method"` // "jwt" or "apikey"
	TokenInfo   interface{}       `json:"token_info,omitempty"`
}

// AuthMiddleware provides authentication middleware
type AuthMiddleware struct {
	jwtAuth   *JWTAuth
	apiKeyMgr *APIKeyManager
	rbac      *RBAC
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(jwtAuth *JWTAuth, apiKeyMgr *APIKeyManager, rbac *RBAC) *AuthMiddleware {
	return &AuthMiddleware{
		jwtAuth:   jwtAuth,
		apiKeyMgr: apiKeyMgr,
		rbac:      rbac,
	}
}

// AuthRequired middleware that requires authentication
func (m *AuthMiddleware) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := m.authenticateRequest(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
				"code":  "UNAUTHORIZED",
			})
			c.Abort()
			return
		}

		// Store user in context
		c.Set("user", user)
		c.Next()
	}
}

// RequirePermission middleware that requires a specific permission
func (m *AuthMiddleware) RequirePermission(permission Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
				"code":  "UNAUTHORIZED",
			})
			c.Abort()
			return
		}

		authUser, ok := user.(*AuthenticatedUser)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Invalid user context",
				"code":  "INTERNAL_ERROR",
			})
			c.Abort()
			return
		}

		if !m.hasPermission(authUser, permission) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Insufficient permissions",
				"code":  "FORBIDDEN", 
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireRepositoryPermission middleware that requires permission for a specific repository
func (m *AuthMiddleware) RequireRepositoryPermission(permission Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
				"code":  "UNAUTHORIZED",
			})
			c.Abort()
			return
		}

		authUser, ok := user.(*AuthenticatedUser)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Invalid user context",
				"code":  "INTERNAL_ERROR",
			})
			c.Abort()
			return
		}

		repoID := c.Param("repo_id")
		if repoID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Repository ID required",
				"code":  "MISSING_REPO_ID",
			})
			c.Abort()
			return
		}

		if !m.hasRepositoryPermission(authUser, repoID, permission) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Insufficient repository permissions",
				"code":  "FORBIDDEN",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// OptionalAuth middleware that attempts authentication but doesn't require it
func (m *AuthMiddleware) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := m.authenticateRequest(c)
		if user != nil {
			c.Set("user", user)
		}
		c.Next()
	}
}

// authenticateRequest attempts to authenticate a request using JWT or API key
func (m *AuthMiddleware) authenticateRequest(c *gin.Context) (*AuthenticatedUser, error) {
	// Try JWT authentication first
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			return m.authenticateJWT(token)
		}
	}

	// Try API key authentication
	if apiKey := c.GetHeader("X-API-Key"); apiKey != "" {
		return m.authenticateAPIKey(apiKey)
	}

	// Try API key from query parameter (less secure, for webhooks etc.)
	if apiKey := c.Query("api_key"); apiKey != "" {
		return m.authenticateAPIKey(apiKey)
	}

	return nil, ErrNoAuthProvided
}

// authenticateJWT authenticates using JWT token
func (m *AuthMiddleware) authenticateJWT(token string) (*AuthenticatedUser, error) {
	claims, err := m.jwtAuth.ValidateToken(token)
	if err != nil {
		return nil, err
	}

	// Get user permissions from RBAC
	permissions, err := m.rbac.GetUserPermissions(claims.UserID)
	if err != nil {
		return nil, err
	}

	// Get user for repository permissions
	user, err := m.rbac.GetUser(claims.UserID)
	if err != nil {
		return nil, err
	}

	return &AuthenticatedUser{
		ID:          claims.UserID,
		Username:    claims.Username,
		Email:       claims.Email,
		Permissions: permissions,
		RepoPerms:   user.RepoPerms,
		AuthMethod:  "jwt",
		TokenInfo:   claims,
	}, nil
}

// authenticateAPIKey authenticates using API key
func (m *AuthMiddleware) authenticateAPIKey(keyString string) (*AuthenticatedUser, error) {
	apiKey, err := m.apiKeyMgr.ValidateAPIKey(keyString)
	if err != nil {
		return nil, err
	}

	// Get user info
	user, err := m.rbac.GetUser(apiKey.UserID)
	if err != nil {
		return nil, err
	}

	// Combine user permissions with API key permissions
	userPermissions, _ := m.rbac.GetUserPermissions(apiKey.UserID)
	
	// API key permissions are a subset of user permissions
	effectivePermissions := m.intersectPermissions(userPermissions, apiKey.Permissions)

	return &AuthenticatedUser{
		ID:          apiKey.UserID,
		Username:    user.Username,
		Email:       user.Email,
		Permissions: effectivePermissions,
		RepoPerms:   apiKey.RepoPerms,
		AuthMethod:  "apikey",
		TokenInfo:   apiKey,
	}, nil
}

// hasPermission checks if a user has a specific permission
func (m *AuthMiddleware) hasPermission(user *AuthenticatedUser, permission Permission) bool {
	for _, perm := range user.Permissions {
		if perm == permission {
			return true
		}
		// Check for admin permissions
		if perm == PermissionSystemAdmin || 
		   (strings.HasPrefix(string(permission), "repo:") && perm == PermissionRepoAdmin) {
			return true
		}
	}
	return false
}

// hasRepositoryPermission checks if a user has permission for a specific repository
func (m *AuthMiddleware) hasRepositoryPermission(user *AuthenticatedUser, repoID string, permission Permission) bool {
	// Check global permissions first
	if m.hasPermission(user, permission) {
		return true
	}

	// Check repository-specific permissions
	if user.RepoPerms != nil {
		if repoPerms, exists := user.RepoPerms[repoID]; exists {
			for _, perm := range repoPerms {
				if perm == permission || perm == PermissionRepoAdmin {
					return true
				}
			}
		}
	}

	return false
}

// intersectPermissions returns permissions that exist in both slices
func (m *AuthMiddleware) intersectPermissions(userPerms, keyPerms []Permission) []Permission {
	permMap := make(map[Permission]bool)
	for _, perm := range userPerms {
		permMap[perm] = true
	}

	var result []Permission
	for _, perm := range keyPerms {
		if permMap[perm] {
			result = append(result, perm)
		}
	}

	return result
}

// GetCurrentUser returns the current authenticated user from gin context
func GetCurrentUser(c *gin.Context) (*AuthenticatedUser, bool) {
	user, exists := c.Get("user")
	if !exists {
		return nil, false
	}

	authUser, ok := user.(*AuthenticatedUser)
	return authUser, ok
}

// Custom errors
var (
	ErrNoAuthProvided = fmt.Errorf("no authentication provided")
	ErrInvalidToken   = fmt.Errorf("invalid token")
	ErrExpiredToken   = fmt.Errorf("token expired")
	ErrInvalidAPIKey  = fmt.Errorf("invalid API key")
)