package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestNewAPIKeyManager(t *testing.T) {
	rbac := NewRBAC()
	apiKeyMgr := NewAPIKeyManager(rbac)
	
	if apiKeyMgr == nil {
		t.Fatal("NewAPIKeyManager returned nil")
	}
	
	if apiKeyMgr.rbac != rbac {
		t.Error("RBAC reference not set correctly")
	}
}

func TestGenerateAPIKey(t *testing.T) {
	rbac := NewRBAC()
	apiKeyMgr := NewAPIKeyManager(rbac)
	
	// Create a test user first
	err := rbac.CreateUser("testuser", "testname", "test@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	
	testCases := []struct {
		name        string
		userID      string
		keyName     string
		permissions []Permission
		repoPerms   map[string][]Permission
		expiresAt   *time.Time
		wantErr     bool
	}{
		{
			name:        "valid API key generation",
			userID:      "testuser",
			keyName:     "test-key",
			permissions: []Permission{PermissionRepoRead},
			repoPerms:   nil,
			expiresAt:   nil,
			wantErr:     false,
		},
		{
			name:        "API key with repository permissions",
			userID:      "testuser",
			keyName:     "repo-key",
			permissions: []Permission{PermissionRepoRead},
			repoPerms:   map[string][]Permission{"repo1": {PermissionRepoWrite}},
			expiresAt:   nil,
			wantErr:     false,
		},
		{
			name:        "API key with expiration",
			userID:      "testuser",
			keyName:     "expiring-key",
			permissions: []Permission{PermissionRepoRead},
			repoPerms:   nil,
			expiresAt:   timePtr(time.Now().Add(24 * time.Hour)),
			wantErr:     false,
		},
		{
			name:        "non-existent user",
			userID:      "nonexistent",
			keyName:     "invalid-key",
			permissions: []Permission{PermissionRepoRead},
			repoPerms:   nil,
			expiresAt:   nil,
			wantErr:     true,
		},
		{
			name:        "empty key name",
			userID:      "testuser",
			keyName:     "",
			permissions: []Permission{PermissionRepoRead},
			repoPerms:   nil,
			expiresAt:   nil,
			wantErr:     false, // Implementation allows empty names
		},
		{
			name:        "expired key",
			userID:      "testuser",
			keyName:     "expired-key",
			permissions: []Permission{PermissionRepoRead},
			repoPerms:   nil,
			expiresAt:   timePtr(time.Now().Add(-time.Hour)), // Already expired
			wantErr:     false, // Implementation allows expired keys
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			keyString, apiKey, err := apiKeyMgr.GenerateAPIKey(
				tc.userID, tc.keyName, tc.permissions, tc.repoPerms, tc.expiresAt)
			
			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			if !tc.wantErr {
				if keyString == "" {
					t.Error("Expected key string but got empty")
				}
				if apiKey == nil {
					t.Error("Expected API key but got nil")
				}
				if apiKey.UserID != tc.userID {
					t.Errorf("Expected UserID '%s', got '%s'", tc.userID, apiKey.UserID)
				}
				if apiKey.Name != tc.keyName {
					t.Errorf("Expected Name '%s', got '%s'", tc.keyName, apiKey.Name)
				}
				if len(apiKey.Permissions) != len(tc.permissions) {
					t.Errorf("Expected %d permissions, got %d", len(tc.permissions), len(apiKey.Permissions))
				}
			}
		})
	}
}

func TestValidateAPIKey(t *testing.T) {
	rbac := NewRBAC()
	apiKeyMgr := NewAPIKeyManager(rbac)
	
	// Create a test user and API key
	err := rbac.CreateUser("testuser", "testname", "test@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	
	validKeyString, validAPIKey, err := apiKeyMgr.GenerateAPIKey(
		"testuser", "test-key", []Permission{PermissionRepoRead}, nil, nil)
	if err != nil {
		t.Fatalf("Failed to generate API key: %v", err)
	}
	
	// Create an expired key (implementation allows this)
	_, _, err = apiKeyMgr.GenerateAPIKey(
		"testuser", "expired-key", []Permission{PermissionRepoRead}, nil, 
		timePtr(time.Now().Add(-time.Hour)))
	if err != nil {
		t.Fatalf("Unexpected error for expired key generation: %v", err)
	}
	
	testCases := []struct {
		name      string
		keyString string
		wantErr   bool
	}{
		{
			name:      "valid API key",
			keyString: validKeyString,
			wantErr:   false,
		},
		{
			name:      "empty key string",
			keyString: "",
			wantErr:   true,
		},
		{
			name:      "invalid key format",
			keyString: "invalid-key-format",
			wantErr:   true,
		},
		{
			name:      "non-existent key",
			keyString: "govc_1234567890abcdef1234567890abcdef12345678",
			wantErr:   true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			apiKey, err := apiKeyMgr.ValidateAPIKey(tc.keyString)
			
			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			if !tc.wantErr {
				if apiKey == nil {
					t.Error("Expected API key but got nil")
				}
				if apiKey.ID != validAPIKey.ID {
					t.Errorf("Expected key ID '%s', got '%s'", validAPIKey.ID, apiKey.ID)
				}
			}
		})
	}
}

func TestRevokeAPIKey(t *testing.T) {
	rbac := NewRBAC()
	apiKeyMgr := NewAPIKeyManager(rbac)
	
	// Create a test user and API key
	err := rbac.CreateUser("testuser", "testname", "test@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	
	keyString, apiKey, err := apiKeyMgr.GenerateAPIKey(
		"testuser", "test-key", []Permission{PermissionRepoRead}, nil, nil)
	if err != nil {
		t.Fatalf("Failed to generate API key: %v", err)
	}
	
	testCases := []struct {
		name    string
		keyID   string
		wantErr bool
	}{
		{
			name:    "revoke existing key",
			keyID:   apiKey.ID,
			wantErr: false,
		},
		{
			name:    "revoke non-existent key",
			keyID:   "nonexistent-key-id",
			wantErr: true,
		},
		{
			name:    "revoke already revoked key",
			keyID:   apiKey.ID, // Already revoked above
			wantErr: false, // Implementation allows revoking already revoked keys
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := apiKeyMgr.RevokeAPIKey(tc.keyID)
			
			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			// If revocation was successful, verify key can't be validated
			if !tc.wantErr && err == nil {
				_, validateErr := apiKeyMgr.ValidateAPIKey(keyString)
				if validateErr == nil {
					t.Error("Revoked key should not be valid")
				}
			}
		})
	}
}

func TestListAPIKeys(t *testing.T) {
	rbac := NewRBAC()
	apiKeyMgr := NewAPIKeyManager(rbac)
	
	// Create test users
	err := rbac.CreateUser("user1", "user1", "user1@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}
	
	err = rbac.CreateUser("user2", "user2", "user2@example.com", []string{"reader"})
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}
	
	// Create API keys for user1
	_, _, err = apiKeyMgr.GenerateAPIKey("user1", "key1", []Permission{PermissionRepoRead}, nil, nil)
	if err != nil {
		t.Fatalf("Failed to generate key1: %v", err)
	}
	
	_, _, err = apiKeyMgr.GenerateAPIKey("user1", "key2", []Permission{PermissionRepoWrite}, nil, nil)
	if err != nil {
		t.Fatalf("Failed to generate key2: %v", err)
	}
	
	// Create API key for user2
	_, _, err = apiKeyMgr.GenerateAPIKey("user2", "key3", []Permission{PermissionRepoRead}, nil, nil)
	if err != nil {
		t.Fatalf("Failed to generate key3: %v", err)
	}
	
	testCases := []struct {
		name         string
		userID       string
		expectedKeys int
		wantErr      bool
	}{
		{
			name:         "list keys for user1",
			userID:       "user1",
			expectedKeys: 2,
			wantErr:      false,
		},
		{
			name:         "list keys for user2",
			userID:       "user2",
			expectedKeys: 1,
			wantErr:      false,
		},
		{
			name:         "list keys for non-existent user",
			userID:       "nonexistent",
			expectedKeys: 0,
			wantErr:      false, // Implementation allows listing for non-existent users
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			keys, err := apiKeyMgr.ListAPIKeys(tc.userID)
			
			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			if !tc.wantErr {
				if len(keys) != tc.expectedKeys {
					t.Errorf("Expected %d keys, got %d", tc.expectedKeys, len(keys))
				}
				
				// Verify all keys belong to the user
				for _, key := range keys {
					if key.UserID != tc.userID {
						t.Errorf("Key belongs to wrong user: expected '%s', got '%s'", 
							tc.userID, key.UserID)
					}
				}
			}
		})
	}
}

func TestGetAPIKey(t *testing.T) {
	rbac := NewRBAC()
	apiKeyMgr := NewAPIKeyManager(rbac)
	
	// Create a test user and API key
	err := rbac.CreateUser("testuser", "testname", "test@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	
	_, apiKey, err := apiKeyMgr.GenerateAPIKey(
		"testuser", "test-key", []Permission{PermissionRepoRead}, nil, nil)
	if err != nil {
		t.Fatalf("Failed to generate API key: %v", err)
	}
	
	testCases := []struct {
		name    string
		keyID   string
		wantErr bool
	}{
		{
			name:    "get existing key",
			keyID:   apiKey.ID,
			wantErr: false,
		},
		{
			name:    "get non-existent key",
			keyID:   "nonexistent-key-id",
			wantErr: true,
		},
		{
			name:    "get with empty key ID",
			keyID:   "",
			wantErr: true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key, err := apiKeyMgr.GetAPIKey(tc.keyID)
			
			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			if !tc.wantErr {
				if key == nil {
					t.Error("Expected API key but got nil")
				}
				if key.ID != tc.keyID {
					t.Errorf("Expected key ID '%s', got '%s'", tc.keyID, key.ID)
				}
			}
		})
	}
}

func TestGetKeyStats(t *testing.T) {
	rbac := NewRBAC()
	apiKeyMgr := NewAPIKeyManager(rbac)
	
	// Create a test user
	err := rbac.CreateUser("testuser", "testname", "test@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	
	// Create various API keys
	_, _, err = apiKeyMgr.GenerateAPIKey("testuser", "active-key", []Permission{PermissionRepoRead}, nil, nil)
	if err != nil {
		t.Fatalf("Failed to generate active key: %v", err)
	}
	
	// Create expired key (implementation allows this)
	_, _, err = apiKeyMgr.GenerateAPIKey("testuser", "expired-key", []Permission{PermissionRepoRead}, nil, 
		timePtr(time.Now().Add(-time.Hour)))
	if err != nil {
		t.Fatalf("Unexpected error for expired key: %v", err)
	}
	
	// Create a key and then revoke it
	_, revokedKey, err := apiKeyMgr.GenerateAPIKey("testuser", "revoked-key", []Permission{PermissionRepoRead}, nil, nil)
	if err != nil {
		t.Fatalf("Failed to generate key to revoke: %v", err)
	}
	
	err = apiKeyMgr.RevokeAPIKey(revokedKey.ID)
	if err != nil {
		t.Fatalf("Failed to revoke key: %v", err)
	}
	
	stats := apiKeyMgr.GetKeyStats()
	
	// Check that stats contain expected keys
	if stats == nil {
		t.Fatal("GetKeyStats returned nil")
	}
	
	total, exists := stats["total"]
	if !exists {
		t.Error("Stats should contain 'total' key")
	}
	
	if total < 1 { // At least the active key
		t.Errorf("Expected at least 1 total key, got %v", total)
	}
	
	active, exists := stats["active"]
	if !exists {
		t.Error("Stats should contain 'active' key")
	}
	
	if active < 1 { // At least the active key
		t.Errorf("Expected at least 1 active key, got %v", active)
	}
}

func TestAPIKeyInfo(t *testing.T) {
	rbac := NewRBAC()
	apiKeyMgr := NewAPIKeyManager(rbac)
	
	// Create a test user and API key
	err := rbac.CreateUser("testuser", "testname", "test@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	
	expiresAt := time.Now().Add(24 * time.Hour)
	_, apiKey, err := apiKeyMgr.GenerateAPIKey(
		"testuser", "test-key", []Permission{PermissionRepoRead, PermissionRepoWrite}, 
		map[string][]Permission{"repo1": {PermissionRepoAdmin}}, &expiresAt)
	if err != nil {
		t.Fatalf("Failed to generate API key: %v", err)
	}
	
	info := apiKey.ToInfo()
	
	if info.ID != apiKey.ID {
		t.Errorf("Expected ID '%s', got '%s'", apiKey.ID, info.ID)
	}
	if info.Name != apiKey.Name {
		t.Errorf("Expected Name '%s', got '%s'", apiKey.Name, info.Name)
	}
	if info.UserID != apiKey.UserID {
		t.Errorf("Expected UserID '%s', got '%s'", apiKey.UserID, info.UserID)
	}
	if len(info.Permissions) != len(apiKey.Permissions) {
		t.Errorf("Expected %d permissions, got %d", len(apiKey.Permissions), len(info.Permissions))
	}
	if len(info.RepoPerms) != len(apiKey.RepoPerms) {
		t.Errorf("Expected %d repo permissions, got %d", len(apiKey.RepoPerms), len(info.RepoPerms))
	}
	if !info.ExpiresAt.Equal(*apiKey.ExpiresAt) {
		t.Errorf("Expected ExpiresAt '%v', got '%v'", *apiKey.ExpiresAt, info.ExpiresAt)
	}
	// KeyHash should not be in APIKeyInfo (it doesn't have this field)
	// This is good for security - sensitive data is not exposed
}

func TestConcurrentAPIKeyOperations(t *testing.T) {
	rbac := NewRBAC()
	apiKeyMgr := NewAPIKeyManager(rbac)
	
	// Create test users
	for i := 0; i < 10; i++ {
		userID := fmt.Sprintf("user%d", i)
		err := rbac.CreateUser(userID, userID, fmt.Sprintf("%s@example.com", userID), []string{"developer"})
		if err != nil {
			t.Fatalf("Failed to create user %s: %v", userID, err)
		}
	}
	
	// Concurrent key generation
	done := make(chan bool, 100)
	keyIDs := make(chan string, 100)
	
	for i := 0; i < 100; i++ {
		go func(id int) {
			userID := fmt.Sprintf("user%d", id%10) // 10 keys per user
			keyName := fmt.Sprintf("key%d", id)
			
			_, apiKey, err := apiKeyMgr.GenerateAPIKey(
				userID, keyName, []Permission{PermissionRepoRead}, nil, nil)
			if err != nil {
				t.Errorf("Failed to generate key %s: %v", keyName, err)
			} else {
				keyIDs <- apiKey.ID
			}
			
			done <- true
		}(i)
	}
	
	// Collect key IDs
	var generatedKeys []string
	for i := 0; i < 100; i++ {
		<-done
	}
	close(keyIDs)
	
	for keyID := range keyIDs {
		generatedKeys = append(generatedKeys, keyID)
	}
	
	// Verify all keys were created
	if len(generatedKeys) != 100 {
		t.Errorf("Expected 100 keys, got %d", len(generatedKeys))
	}
	
	// Test concurrent validation
	done = make(chan bool, len(generatedKeys))
	
	for _, keyID := range generatedKeys {
		go func(id string) {
			_, err := apiKeyMgr.GetAPIKey(id)
			if err != nil {
				t.Errorf("Failed to get key %s: %v", id, err)
			}
			done <- true
		}(keyID)
	}
	
	for i := 0; i < len(generatedKeys); i++ {
		<-done
	}
}

func BenchmarkGenerateAPIKey(b *testing.B) {
	rbac := NewRBAC()
	apiKeyMgr := NewAPIKeyManager(rbac)
	
	// Create a test user
	err := rbac.CreateUser("benchuser", "benchuser", "bench@example.com", []string{"developer"})
	if err != nil {
		b.Fatalf("Failed to create test user: %v", err)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		keyName := fmt.Sprintf("key%d", i)
		_, _, err := apiKeyMgr.GenerateAPIKey(
			"benchuser", keyName, []Permission{PermissionRepoRead}, nil, nil)
		if err != nil {
			b.Fatalf("Failed to generate API key: %v", err)
		}
	}
}

func BenchmarkValidateAPIKey(b *testing.B) {
	rbac := NewRBAC()
	apiKeyMgr := NewAPIKeyManager(rbac)
	
	// Create a test user and API key
	err := rbac.CreateUser("benchuser", "benchuser", "bench@example.com", []string{"developer"})
	if err != nil {
		b.Fatalf("Failed to create test user: %v", err)
	}
	
	keyString, _, err := apiKeyMgr.GenerateAPIKey(
		"benchuser", "bench-key", []Permission{PermissionRepoRead}, nil, nil)
	if err != nil {
		b.Fatalf("Failed to generate API key: %v", err)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := apiKeyMgr.ValidateAPIKey(keyString)
		if err != nil {
			b.Fatalf("Failed to validate API key: %v", err)
		}
	}
}

// Helper function to create time pointer
func timePtr(t time.Time) *time.Time {
	return &t
}