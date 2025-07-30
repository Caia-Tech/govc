package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// APIKey represents an API key for authentication
type APIKey struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	HashedKey   string            `json:"-"` // Never expose the actual key
	UserID      string            `json:"user_id"`
	Permissions []Permission      `json:"permissions"`
	RepoPerms   map[string][]Permission `json:"repo_permissions"`
	Active      bool              `json:"active"`
	LastUsed    *time.Time        `json:"last_used,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

// APIKeyManager manages API keys
type APIKeyManager struct {
	keys map[string]*APIKey // keyed by hashed key
	rbac *RBAC
	mu   sync.RWMutex
}

// NewAPIKeyManager creates a new API key manager
func NewAPIKeyManager(rbac *RBAC) *APIKeyManager {
	return &APIKeyManager{
		keys: make(map[string]*APIKey),
		rbac: rbac,
	}
}

// GenerateAPIKey creates a new API key for a user
func (m *APIKeyManager) GenerateAPIKey(userID, name string, permissions []Permission, repoPerms map[string][]Permission, expiresAt *time.Time) (string, *APIKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Verify user exists
	if _, err := m.rbac.GetUser(userID); err != nil {
		return "", nil, fmt.Errorf("user not found: %w", err)
	}

	// Generate a secure random key
	keyBytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(keyBytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	// Create the key string with a prefix for identification
	keyString := "govc_" + base64.URLEncoding.EncodeToString(keyBytes)
	
	// Hash the key for storage
	hash := sha256.Sum256([]byte(keyString))
	hashedKey := hex.EncodeToString(hash[:])

	// Generate unique ID
	id := generateAPIKeyID()

	// Create API key record
	apiKey := &APIKey{
		ID:          id,
		Name:        name,
		HashedKey:   hashedKey,
		UserID:      userID,
		Permissions: permissions,
		RepoPerms:   repoPerms,
		Active:      true,
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
	}

	// Store the API key
	m.keys[hashedKey] = apiKey

	return keyString, apiKey, nil
}

// ValidateAPIKey validates an API key and returns the associated API key record
func (m *APIKeyManager) ValidateAPIKey(keyString string) (*APIKey, error) {
	// Validate key format
	if !strings.HasPrefix(keyString, "govc_") {
		return nil, fmt.Errorf("invalid API key format")
	}

	// Hash the provided key
	hash := sha256.Sum256([]byte(keyString))
	hashedKey := hex.EncodeToString(hash[:])

	m.mu.RLock()
	defer m.mu.RUnlock()

	apiKey, exists := m.keys[hashedKey]
	if !exists {
		return nil, fmt.Errorf("API key not found")
	}

	// Check if key is active
	if !apiKey.Active {
		return nil, fmt.Errorf("API key is deactivated")
	}

	// Check if key has expired
	if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
		return nil, fmt.Errorf("API key has expired")
	}

	// Update last used timestamp (in a separate goroutine to avoid holding the lock)
	go func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		now := time.Now()
		apiKey.LastUsed = &now
	}()

	// Return a copy to prevent external modification
	apiKeyCopy := *apiKey
	apiKeyCopy.Permissions = make([]Permission, len(apiKey.Permissions))
	copy(apiKeyCopy.Permissions, apiKey.Permissions)
	
	if apiKey.RepoPerms != nil {
		apiKeyCopy.RepoPerms = make(map[string][]Permission)
		for repo, perms := range apiKey.RepoPerms {
			apiKeyCopy.RepoPerms[repo] = make([]Permission, len(perms))
			copy(apiKeyCopy.RepoPerms[repo], perms)
		}
	}

	return &apiKeyCopy, nil
}

// GetAPIKey retrieves an API key by ID
func (m *APIKeyManager) GetAPIKey(keyID string) (*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, apiKey := range m.keys {
		if apiKey.ID == keyID {
			// Return a copy
			apiKeyCopy := *apiKey
			apiKeyCopy.Permissions = make([]Permission, len(apiKey.Permissions))
			copy(apiKeyCopy.Permissions, apiKey.Permissions)
			
			if apiKey.RepoPerms != nil {
				apiKeyCopy.RepoPerms = make(map[string][]Permission)
				for repo, perms := range apiKey.RepoPerms {
					apiKeyCopy.RepoPerms[repo] = make([]Permission, len(perms))
					copy(apiKeyCopy.RepoPerms[repo], perms)
				}
			}
			
			return &apiKeyCopy, nil
		}
	}

	return nil, fmt.Errorf("API key not found")
}

// ListAPIKeys returns all API keys for a user
func (m *APIKeyManager) ListAPIKeys(userID string) ([]*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var userKeys []*APIKey
	for _, apiKey := range m.keys {
		if apiKey.UserID == userID {
			// Return a copy
			apiKeyCopy := *apiKey
			apiKeyCopy.Permissions = make([]Permission, len(apiKey.Permissions))
			copy(apiKeyCopy.Permissions, apiKey.Permissions)
			
			if apiKey.RepoPerms != nil {
				apiKeyCopy.RepoPerms = make(map[string][]Permission)
				for repo, perms := range apiKey.RepoPerms {
					apiKeyCopy.RepoPerms[repo] = make([]Permission, len(perms))
					copy(apiKeyCopy.RepoPerms[repo], perms)
				}
			}
			
			userKeys = append(userKeys, &apiKeyCopy)
		}
	}

	return userKeys, nil
}

// RevokeAPIKey deactivates an API key
func (m *APIKeyManager) RevokeAPIKey(keyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, apiKey := range m.keys {
		if apiKey.ID == keyID {
			apiKey.Active = false
			return nil
		}
	}

	return fmt.Errorf("API key not found")
}

// DeleteAPIKey permanently removes an API key
func (m *APIKeyManager) DeleteAPIKey(keyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for hashedKey, apiKey := range m.keys {
		if apiKey.ID == keyID {
			delete(m.keys, hashedKey)
			return nil
		}
	}

	return fmt.Errorf("API key not found")
}

// HasPermission checks if an API key has a specific permission
func (m *APIKeyManager) HasPermission(apiKey *APIKey, permission Permission) bool {
	// Check direct permissions
	for _, perm := range apiKey.Permissions {
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

// HasRepositoryPermission checks if an API key has permission for a specific repository
func (m *APIKeyManager) HasRepositoryPermission(apiKey *APIKey, repoID string, permission Permission) bool {
	// Check global permissions first
	if m.HasPermission(apiKey, permission) {
		return true
	}

	// Check repository-specific permissions
	if apiKey.RepoPerms != nil {
		if repoPerms, exists := apiKey.RepoPerms[repoID]; exists {
			for _, perm := range repoPerms {
				if perm == permission || perm == PermissionRepoAdmin {
					return true
				}
			}
		}
	}

	return false
}

// UpdateAPIKeyPermissions updates the permissions for an API key
func (m *APIKeyManager) UpdateAPIKeyPermissions(keyID string, permissions []Permission, repoPerms map[string][]Permission) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, apiKey := range m.keys {
		if apiKey.ID == keyID {
			apiKey.Permissions = permissions
			apiKey.RepoPerms = repoPerms
			return nil
		}
	}

	return fmt.Errorf("API key not found")
}

// CleanupExpiredKeys removes expired API keys
func (m *APIKeyManager) CleanupExpiredKeys() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	var removed int

	for hashedKey, apiKey := range m.keys {
		if apiKey.ExpiresAt != nil && now.After(*apiKey.ExpiresAt) {
			delete(m.keys, hashedKey)
			removed++
		}
	}

	return removed
}

// GetKeyStats returns statistics about API keys
func (m *APIKeyManager) GetKeyStats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := map[string]int{
		"total":    0,
		"active":   0,
		"inactive": 0,
		"expired":  0,
	}

	now := time.Now()
	for _, apiKey := range m.keys {
		stats["total"]++
		
		if apiKey.ExpiresAt != nil && now.After(*apiKey.ExpiresAt) {
			stats["expired"]++
		} else if apiKey.Active {
			stats["active"]++
		} else {
			stats["inactive"]++
		}
	}

	return stats
}

// generateAPIKeyID generates a unique ID for an API key
func generateAPIKeyID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// APIKeyInfo represents public information about an API key
type APIKeyInfo struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	UserID      string                  `json:"user_id"`
	Permissions []Permission            `json:"permissions"`
	RepoPerms   map[string][]Permission `json:"repo_permissions"`
	Active      bool                    `json:"active"`
	LastUsed    *time.Time              `json:"last_used,omitempty"`
	CreatedAt   time.Time               `json:"created_at"`
	ExpiresAt   *time.Time              `json:"expires_at,omitempty"`
}

// ToInfo converts an APIKey to APIKeyInfo (removes sensitive data)
func (k *APIKey) ToInfo() *APIKeyInfo {
	info := &APIKeyInfo{
		ID:          k.ID,
		Name:        k.Name,
		UserID:      k.UserID,
		Permissions: make([]Permission, len(k.Permissions)),
		Active:      k.Active,
		LastUsed:    k.LastUsed,
		CreatedAt:   k.CreatedAt,
		ExpiresAt:   k.ExpiresAt,
	}

	copy(info.Permissions, k.Permissions)

	if k.RepoPerms != nil {
		info.RepoPerms = make(map[string][]Permission)
		for repo, perms := range k.RepoPerms {
			info.RepoPerms[repo] = make([]Permission, len(perms))
			copy(info.RepoPerms[repo], perms)
		}
	}

	return info
}