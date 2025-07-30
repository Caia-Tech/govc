package auth

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Permission represents a specific permission
type Permission string

const (
	// Repository permissions
	PermissionRepoRead   Permission = "repo:read"
	PermissionRepoWrite  Permission = "repo:write"
	PermissionRepoDelete Permission = "repo:delete"
	PermissionRepoAdmin  Permission = "repo:admin"

	// System permissions
	PermissionSystemRead  Permission = "system:read"
	PermissionSystemWrite Permission = "system:write"
	PermissionSystemAdmin Permission = "system:admin"

	// User management permissions
	PermissionUserRead   Permission = "user:read"
	PermissionUserWrite  Permission = "user:write"
	PermissionUserDelete Permission = "user:delete"

	// Webhook permissions
	PermissionWebhookRead   Permission = "webhook:read"
	PermissionWebhookWrite  Permission = "webhook:write"
	PermissionWebhookDelete Permission = "webhook:delete"
)

// Role represents a collection of permissions
type Role struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Permissions []Permission `json:"permissions"`
}

// User represents a user in the system
type User struct {
	ID          string            `json:"id"`
	Username    string            `json:"username"`
	Email       string            `json:"email"`
	Roles       []string          `json:"roles"`
	Permissions []Permission      `json:"permissions"` // Direct permissions
	RepoPerms   map[string][]Permission `json:"repo_permissions"` // Repository-specific permissions
	Active      bool              `json:"active"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

// RBAC manages role-based access control
type RBAC struct {
	roles map[string]*Role
	users map[string]*User
	mu    sync.RWMutex
}

// NewRBAC creates a new RBAC manager with default roles
func NewRBAC() *RBAC {
	rbac := &RBAC{
		roles: make(map[string]*Role),
		users: make(map[string]*User),
	}

	// Define default roles
	rbac.defineDefaultRoles()
	
	return rbac
}

// defineDefaultRoles sets up the default role hierarchy
func (r *RBAC) defineDefaultRoles() {
	// Admin role - full system access
	r.roles["admin"] = &Role{
		Name:        "admin",
		Description: "Full system administrator",
		Permissions: []Permission{
			PermissionSystemAdmin,
			PermissionRepoAdmin,
			PermissionUserRead,
			PermissionUserWrite,
			PermissionUserDelete,
			PermissionWebhookRead,
			PermissionWebhookWrite,
			PermissionWebhookDelete,
		},
	}

	// Developer role - can read/write repositories
	r.roles["developer"] = &Role{
		Name:        "developer",
		Description: "Can read and write repositories",
		Permissions: []Permission{
			PermissionRepoRead,
			PermissionRepoWrite,
			PermissionWebhookRead,
			PermissionWebhookWrite,
		},
	}

	// Reader role - read-only access
	r.roles["reader"] = &Role{
		Name:        "reader",
		Description: "Read-only access to repositories",
		Permissions: []Permission{
			PermissionRepoRead,
		},
	}

	// Guest role - minimal access
	r.roles["guest"] = &Role{
		Name:        "guest",
		Description: "Minimal system access",
		Permissions: []Permission{
			PermissionSystemRead,
		},
	}
}

// CreateRole creates a new role
func (r *RBAC) CreateRole(name, description string, permissions []Permission) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.roles[name]; exists {
		return fmt.Errorf("role '%s' already exists", name)
	}

	r.roles[name] = &Role{
		Name:        name,
		Description: description,
		Permissions: permissions,
	}

	return nil
}

// GetRole retrieves a role by name
func (r *RBAC) GetRole(name string) (*Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	role, exists := r.roles[name]
	if !exists {
		return nil, fmt.Errorf("role '%s' not found", name)
	}

	// Return a copy to prevent external modification
	roleCopy := *role
	roleCopy.Permissions = make([]Permission, len(role.Permissions))
	copy(roleCopy.Permissions, role.Permissions)

	return &roleCopy, nil
}

// ListRoles returns all available roles
func (r *RBAC) ListRoles() []*Role {
	r.mu.RLock()
	defer r.mu.RUnlock()

	roles := make([]*Role, 0, len(r.roles))
	for _, role := range r.roles {
		roleCopy := *role
		roleCopy.Permissions = make([]Permission, len(role.Permissions))
		copy(roleCopy.Permissions, role.Permissions)
		roles = append(roles, &roleCopy)
	}

	return roles
}

// CreateUser creates a new user
func (r *RBAC) CreateUser(id, username, email string, roles []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.users[id]; exists {
		return fmt.Errorf("user '%s' already exists", id)
	}

	// Validate roles exist
	for _, roleName := range roles {
		if _, exists := r.roles[roleName]; !exists {
			return fmt.Errorf("role '%s' does not exist", roleName)
		}
	}

	r.users[id] = &User{
		ID:        id,
		Username:  username,
		Email:     email,
		Roles:     roles,
		RepoPerms: make(map[string][]Permission),
		Active:    true,
		CreatedAt: fmt.Sprintf("%d", time.Now().Unix()),
		UpdatedAt: fmt.Sprintf("%d", time.Now().Unix()),
	}

	return nil
}

// GetUser retrieves a user by ID
func (r *RBAC) GetUser(userID string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, exists := r.users[userID]
	if !exists {
		return nil, fmt.Errorf("user '%s' not found", userID)
	}

	// Return a copy
	userCopy := *user
	userCopy.Roles = make([]string, len(user.Roles))
	copy(userCopy.Roles, user.Roles)
	userCopy.Permissions = make([]Permission, len(user.Permissions))
	copy(userCopy.Permissions, user.Permissions)
	userCopy.RepoPerms = make(map[string][]Permission)
	for k, v := range user.RepoPerms {
		userCopy.RepoPerms[k] = make([]Permission, len(v))
		copy(userCopy.RepoPerms[k], v)
	}

	return &userCopy, nil
}

// GetUserPermissions returns all permissions for a user (from roles + direct permissions)
func (r *RBAC) GetUserPermissions(userID string) ([]Permission, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, exists := r.users[userID]
	if !exists {
		return nil, fmt.Errorf("user '%s' not found", userID)
	}

	if !user.Active {
		return nil, fmt.Errorf("user '%s' is inactive", userID)
	}

	permissionSet := make(map[Permission]bool)

	// Add permissions from roles
	for _, roleName := range user.Roles {
		if role, exists := r.roles[roleName]; exists {
			for _, perm := range role.Permissions {
				permissionSet[perm] = true
			}
		}
	}

	// Add direct permissions
	for _, perm := range user.Permissions {
		permissionSet[perm] = true
	}

	// Convert to slice
	permissions := make([]Permission, 0, len(permissionSet))
	for perm := range permissionSet {
		permissions = append(permissions, perm)
	}

	return permissions, nil
}

// HasPermission checks if a user has a specific permission
func (r *RBAC) HasPermission(userID string, permission Permission) bool {
	permissions, err := r.GetUserPermissions(userID)
	if err != nil {
		return false
	}

	for _, perm := range permissions {
		if perm == permission {
			return true
		}
		// Check for admin permissions that grant everything
		if perm == PermissionSystemAdmin || 
		   (strings.HasPrefix(string(permission), "repo:") && perm == PermissionRepoAdmin) {
			return true
		}
	}

	return false
}

// HasRepositoryPermission checks if a user has permission for a specific repository
func (r *RBAC) HasRepositoryPermission(userID, repoID string, permission Permission) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, exists := r.users[userID]
	if !exists || !user.Active {
		return false
	}

	// Check global permissions first
	if r.HasPermission(userID, permission) {
		return true
	}

	// Check repository-specific permissions
	if repoPerms, exists := user.RepoPerms[repoID]; exists {
		for _, perm := range repoPerms {
			if perm == permission || perm == PermissionRepoAdmin {
				return true
			}
		}
	}

	return false
}

// GrantRepositoryPermission grants a user permission to a specific repository
func (r *RBAC) GrantRepositoryPermission(userID, repoID string, permission Permission) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, exists := r.users[userID]
	if !exists {
		return fmt.Errorf("user '%s' not found", userID)
	}

	if user.RepoPerms == nil {
		user.RepoPerms = make(map[string][]Permission)
	}

	// Check if permission already exists
	for _, perm := range user.RepoPerms[repoID] {
		if perm == permission {
			return nil // Already has permission
		}
	}

	user.RepoPerms[repoID] = append(user.RepoPerms[repoID], permission)
	return nil
}

// RevokeRepositoryPermission removes a user's permission from a specific repository
func (r *RBAC) RevokeRepositoryPermission(userID, repoID string, permission Permission) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, exists := r.users[userID]
	if !exists {
		return fmt.Errorf("user '%s' not found", userID)
	}

	if user.RepoPerms == nil {
		return nil // No permissions to revoke
	}

	perms := user.RepoPerms[repoID]
	for i, perm := range perms {
		if perm == permission {
			// Remove the permission
			user.RepoPerms[repoID] = append(perms[:i], perms[i+1:]...)
			break
		}
	}

	// Clean up empty permission slice
	if len(user.RepoPerms[repoID]) == 0 {
		delete(user.RepoPerms, repoID)
	}

	return nil
}

// AssignRole assigns a role to a user
func (r *RBAC) AssignRole(userID, roleName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, exists := r.users[userID]
	if !exists {
		return fmt.Errorf("user '%s' not found", userID)
	}

	if _, exists := r.roles[roleName]; !exists {
		return fmt.Errorf("role '%s' not found", roleName)
	}

	// Check if role already assigned
	for _, role := range user.Roles {
		if role == roleName {
			return nil // Already has role
		}
	}

	user.Roles = append(user.Roles, roleName)
	return nil
}

// RemoveRole removes a role from a user
func (r *RBAC) RemoveRole(userID, roleName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, exists := r.users[userID]
	if !exists {
		return fmt.Errorf("user '%s' not found", userID)
	}

	for i, role := range user.Roles {
		if role == roleName {
			user.Roles = append(user.Roles[:i], user.Roles[i+1:]...)
			break
		}
	}

	return nil
}

// DeactivateUser deactivates a user account
func (r *RBAC) DeactivateUser(userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, exists := r.users[userID]
	if !exists {
		return fmt.Errorf("user '%s' not found", userID)
	}

	user.Active = false
	return nil
}

// ActivateUser activates a user account
func (r *RBAC) ActivateUser(userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, exists := r.users[userID]
	if !exists {
		return fmt.Errorf("user '%s' not found", userID)
	}

	user.Active = true
	return nil
}