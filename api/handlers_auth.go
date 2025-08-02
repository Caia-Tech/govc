package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/caiatech/govc/auth"
	"github.com/caiatech/govc/validation"
	"github.com/gin-gonic/gin"
)

// Authentication request/response types

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token     string                  `json:"token"`
	ExpiresAt time.Time               `json:"expires_at"`
	User      *auth.AuthenticatedUser `json:"user"`
}

type CreateAPIKeyRequest struct {
	Name        string                       `json:"name" binding:"required"`
	Permissions []auth.Permission            `json:"permissions"`
	RepoPerms   map[string][]auth.Permission `json:"repo_permissions"`
	ExpiresAt   *time.Time                   `json:"expires_at"`
}

type CreateAPIKeyResponse struct {
	Key     string           `json:"key"`
	KeyInfo *auth.APIKeyInfo `json:"key_info"`
}

type CreateUserRequest struct {
	Username string   `json:"username" binding:"required"`
	Email    string   `json:"email" binding:"required"`
	Roles    []string `json:"roles"`
}

type UpdateUserRequest struct {
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles"`
	Active   *bool    `json:"active"`
}

// login handles user authentication and returns JWT token
func (s *Server) login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// For development, we'll have a simple hardcoded admin user
	// In production, this would validate against a user database
	if req.Username == "admin" && req.Password == "admin" {
		// Generate JWT token
		token, err := s.jwtAuth.GenerateToken("admin", "admin", "admin@govc.dev", []string{"admin"})
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: fmt.Sprintf("failed to generate token: %v", err),
				Code:  "TOKEN_GENERATION_FAILED",
			})
			return
		}

		// Get user permissions
		permissions, _ := s.rbac.GetUserPermissions("admin")
		user, _ := s.rbac.GetUser("admin")

		authUser := &auth.AuthenticatedUser{
			ID:          "admin",
			Username:    "admin",
			Email:       "admin@govc.dev",
			Permissions: permissions,
			RepoPerms:   user.RepoPerms,
			AuthMethod:  "jwt",
		}

		c.JSON(http.StatusOK, LoginResponse{
			Token:     token,
			ExpiresAt: time.Now().Add(s.jwtAuth.TTL),
			User:      authUser,
		})
		return
	}

	c.JSON(http.StatusUnauthorized, ErrorResponse{
		Error: "Invalid credentials",
		Code:  "INVALID_CREDENTIALS",
	})
}

// refreshToken handles JWT token refresh
func (s *Server) refreshToken(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Authorization header required",
			Code:  "MISSING_AUTH_HEADER",
		})
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	newToken, err := s.jwtAuth.RefreshToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: fmt.Sprintf("failed to refresh token: %v", err),
			Code:  "TOKEN_REFRESH_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":      newToken,
		"expires_at": time.Now().Add(s.jwtAuth.TTL),
	})
}

// createAPIKey creates a new API key for the current user
func (s *Server) createAPIKey(c *gin.Context) {
	user, exists := auth.GetCurrentUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Authentication required",
			Code:  "UNAUTHORIZED",
		})
		return
	}

	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Generate API key
	keyString, apiKey, err := s.apiKeyMgr.GenerateAPIKey(
		user.ID,
		req.Name,
		req.Permissions,
		req.RepoPerms,
		req.ExpiresAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to create API key: %v", err),
			Code:  "API_KEY_CREATION_FAILED",
		})
		return
	}

	c.JSON(http.StatusCreated, CreateAPIKeyResponse{
		Key:     keyString,
		KeyInfo: apiKey.ToInfo(),
	})
}

// listAPIKeys lists all API keys for the current user
func (s *Server) listAPIKeys(c *gin.Context) {
	user, exists := auth.GetCurrentUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Authentication required",
			Code:  "UNAUTHORIZED",
		})
		return
	}

	apiKeys, err := s.apiKeyMgr.ListAPIKeys(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to list API keys: %v", err),
			Code:  "API_KEY_LIST_FAILED",
		})
		return
	}

	// Convert to public info
	var keyInfos []*auth.APIKeyInfo
	for _, key := range apiKeys {
		keyInfos = append(keyInfos, key.ToInfo())
	}

	c.JSON(http.StatusOK, gin.H{
		"api_keys": keyInfos,
		"count":    len(keyInfos),
	})
}

// revokeAPIKey revokes an API key
func (s *Server) revokeAPIKey(c *gin.Context) {
	user, exists := auth.GetCurrentUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Authentication required",
			Code:  "UNAUTHORIZED",
		})
		return
	}

	keyID := c.Param("key_id")
	if keyID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "API key ID required",
			Code:  "MISSING_KEY_ID",
		})
		return
	}

	// Verify the key belongs to the user
	apiKey, err := s.apiKeyMgr.GetAPIKey(keyID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "API key not found",
			Code:  "API_KEY_NOT_FOUND",
		})
		return
	}

	if apiKey.UserID != user.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error: "Cannot revoke API key belonging to another user",
			Code:  "FORBIDDEN",
		})
		return
	}

	// Revoke the key
	if err := s.apiKeyMgr.RevokeAPIKey(keyID); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to revoke API key: %v", err),
			Code:  "API_KEY_REVOKE_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "success",
		Message: "API key revoked successfully",
	})
}

// whoami returns information about the current authenticated user
func (s *Server) whoami(c *gin.Context) {
	user, exists := auth.GetCurrentUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Authentication required",
			Code:  "UNAUTHORIZED",
		})
		return
	}

	c.JSON(http.StatusOK, user)
}

// User management endpoints (admin only)

// createUser creates a new user (admin only)
func (s *Server) createUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Validate username
	if err := validation.ValidateUsername(req.Username); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_USERNAME",
		})
		return
	}
	
	// Validate email
	if err := validation.ValidateEmail(req.Email); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_EMAIL",
		})
		return
	}

	// Generate user ID
	userID := fmt.Sprintf("user_%d", time.Now().Unix())

	// Create user
	if err := s.rbac.CreateUser(userID, req.Username, req.Email, req.Roles); err != nil {
		c.JSON(http.StatusConflict, ErrorResponse{
			Error: fmt.Sprintf("failed to create user: %v", err),
			Code:  "USER_CREATION_FAILED",
		})
		return
	}

	// Get created user
	user, _ := s.rbac.GetUser(userID)

	c.JSON(http.StatusCreated, user)
}

// getUser retrieves a user by ID (admin only)
func (s *Server) getUser(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "User ID required",
			Code:  "MISSING_USER_ID",
		})
		return
	}

	user, err := s.rbac.GetUser(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "User not found",
			Code:  "USER_NOT_FOUND",
		})
		return
	}

	c.JSON(http.StatusOK, user)
}

// listRoles lists all available roles
func (s *Server) listRoles(c *gin.Context) {
	roles := s.rbac.ListRoles()
	c.JSON(http.StatusOK, gin.H{
		"roles": roles,
		"count": len(roles),
	})
}

// assignRole assigns a role to a user (admin only)
func (s *Server) assignRole(c *gin.Context) {
	userID := c.Param("user_id")
	roleName := c.Param("role_name")

	if userID == "" || roleName == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "User ID and role name required",
			Code:  "MISSING_PARAMETERS",
		})
		return
	}

	if err := s.rbac.AssignRole(userID, roleName); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("failed to assign role: %v", err),
			Code:  "ROLE_ASSIGNMENT_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "success",
		Message: fmt.Sprintf("Role '%s' assigned to user '%s'", roleName, userID),
	})
}

// grantRepositoryPermission grants repository-specific permission to a user
func (s *Server) grantRepositoryPermission(c *gin.Context) {
	userID := c.Param("user_id")
	repoID := c.Param("repo_id")
	permissionStr := c.Param("permission")

	if userID == "" || repoID == "" || permissionStr == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "User ID, repository ID, and permission required",
			Code:  "MISSING_PARAMETERS",
		})
		return
	}

	permission := auth.Permission(permissionStr)

	if err := s.rbac.GrantRepositoryPermission(userID, repoID, permission); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("failed to grant permission: %v", err),
			Code:  "PERMISSION_GRANT_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "success",
		Message: fmt.Sprintf("Permission '%s' granted to user '%s' for repository '%s'", permission, userID, repoID),
	})
}
