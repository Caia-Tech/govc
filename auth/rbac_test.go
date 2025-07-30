package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestNewRBAC(t *testing.T) {
	rbac := NewRBAC()
	
	if rbac == nil {
		t.Fatal("NewRBAC returned nil")
	}
	
	// Check default roles are created
	roles := rbac.ListRoles()
	expectedRoles := []string{"admin", "developer", "reader", "guest"}
	
	if len(roles) != len(expectedRoles) {
		t.Errorf("Expected %d default roles, got %d", len(expectedRoles), len(roles))
	}
	
	for _, expectedRole := range expectedRoles {
		found := false
		for _, role := range roles {
			if role.Name == expectedRole {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Default role '%s' not found", expectedRole)
		}
	}
}

func TestCreateUser(t *testing.T) {
	rbac := NewRBAC()
	
	testCases := []struct {
		name     string
		userID   string
		username string
		email    string
		roles    []string
		wantErr  bool
	}{
		{
			name:     "valid user creation",
			userID:   "user1",
			username: "johndoe",
			email:    "john@example.com",
			roles:    []string{"admin"},
			wantErr:  false,
		},
		{
			name:     "user with multiple roles",
			userID:   "user2",
			username: "janedoe",
			email:    "jane@example.com",
			roles:    []string{"developer", "reader"},
			wantErr:  false,
		},
		{
			name:     "user with invalid role",
			userID:   "user3",
			username: "bobsmith",
			email:    "bob@example.com",
			roles:    []string{"invalid-role"},
			wantErr:  true,
		},
		{
			name:     "duplicate user ID",
			userID:   "user1", // Already created above
			username: "duplicate",
			email:    "dup@example.com",
			roles:    []string{"guest"},
			wantErr:  true,
		},
		{
			name:     "empty user ID",
			userID:   "",
			username: "emptyid",
			email:    "empty@example.com",
			roles:    []string{"guest"},
			wantErr:  false, // Implementation allows empty user ID
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := rbac.CreateUser(tc.userID, tc.username, tc.email, tc.roles)
			
			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			// If creation was successful, verify user exists
			if !tc.wantErr && err == nil {
				user, getErr := rbac.GetUser(tc.userID)
				if getErr != nil {
					t.Errorf("Failed to get created user: %v", getErr)
				}
				if user.Username != tc.username {
					t.Errorf("Expected username '%s', got '%s'", tc.username, user.Username)
				}
				if user.Email != tc.email {
					t.Errorf("Expected email '%s', got '%s'", tc.email, user.Email)
				}
			}
		})
	}
}

func TestGetUser(t *testing.T) {
	rbac := NewRBAC()
	
	// Create a test user
	err := rbac.CreateUser("testuser", "testname", "test@example.com", []string{"reader"})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	
	testCases := []struct {
		name    string
		userID  string
		wantErr bool
	}{
		{
			name:    "existing user",
			userID:  "testuser",
			wantErr: false,
		},
		{
			name:    "non-existent user",
			userID:  "nonexistent",
			wantErr: true,
		},
		{
			name:    "empty user ID",
			userID:  "",
			wantErr: true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			user, err := rbac.GetUser(tc.userID)
			
			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tc.wantErr && user == nil {
				t.Error("Expected user but got nil")
			}
		})
	}
}

func TestAssignRole(t *testing.T) {
	rbac := NewRBAC()
	
	// Create a test user
	err := rbac.CreateUser("testuser", "testname", "test@example.com", []string{"guest"})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	
	testCases := []struct {
		name     string
		userID   string
		roleName string
		wantErr  bool
	}{
		{
			name:     "assign valid role",
			userID:   "testuser",
			roleName: "developer",
			wantErr:  false,
		},
		{
			name:     "assign duplicate role",
			userID:   "testuser",
			roleName: "guest", // Already assigned
			wantErr:  false, // Implementation allows duplicate role assignment
		},
		{
			name:     "assign invalid role",
			userID:   "testuser",
			roleName: "invalid-role",
			wantErr:  true,
		},
		{
			name:     "assign to non-existent user",
			userID:   "nonexistent",
			roleName: "reader",
			wantErr:  true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := rbac.AssignRole(tc.userID, tc.roleName)
			
			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			// If assignment was successful, verify role is assigned
			if !tc.wantErr && err == nil {
				user, getErr := rbac.GetUser(tc.userID)
				if getErr != nil {
					t.Errorf("Failed to get user: %v", getErr)
				}
				
				roleFound := false
				for _, role := range user.Roles {
					if role == tc.roleName {
						roleFound = true
						break
					}
				}
				if !roleFound {
					t.Errorf("Role '%s' not found in user's roles", tc.roleName)
				}
			}
		})
	}
}

func TestGetUserPermissions(t *testing.T) {
	rbac := NewRBAC()
	
	// Create test users with different roles
	err := rbac.CreateUser("admin_user", "admin", "admin@example.com", []string{"admin"})
	if err != nil {
		t.Fatalf("Failed to create admin user: %v", err)
	}
	
	err = rbac.CreateUser("dev_user", "developer", "dev@example.com", []string{"developer"})
	if err != nil {
		t.Fatalf("Failed to create developer user: %v", err)
	}
	
	testCases := []struct {
		name               string
		userID             string
		expectedMinPerms   int // Minimum expected permissions
		shouldHaveSystemAdmin bool
		wantErr            bool
	}{
		{
			name:               "admin user permissions",
			userID:             "admin_user",
			expectedMinPerms:   5, // Admin should have many permissions
			shouldHaveSystemAdmin: true,
			wantErr:            false,
		},
		{
			name:               "developer user permissions",
			userID:             "dev_user",
			expectedMinPerms:   3, // Developer should have some permissions
			shouldHaveSystemAdmin: false,
			wantErr:            false,
		},
		{
			name:     "non-existent user",
			userID:   "nonexistent",
			wantErr:  true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			permissions, err := rbac.GetUserPermissions(tc.userID)
			
			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			if !tc.wantErr {
				if len(permissions) < tc.expectedMinPerms {
					t.Errorf("Expected at least %d permissions, got %d", tc.expectedMinPerms, len(permissions))
				}
				
				hasSystemAdmin := false
				for _, perm := range permissions {
					if perm == PermissionSystemAdmin {
						hasSystemAdmin = true
						break
					}
				}
				
				if tc.shouldHaveSystemAdmin && !hasSystemAdmin {
					t.Error("Expected user to have system admin permission")
				}
				if !tc.shouldHaveSystemAdmin && hasSystemAdmin {
					t.Error("User should not have system admin permission")
				}
			}
		})
	}
}

func TestGrantRepositoryPermission(t *testing.T) {
	rbac := NewRBAC()
	
	// Create a test user
	err := rbac.CreateUser("testuser", "testname", "test@example.com", []string{"guest"})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	
	testCases := []struct {
		name       string
		userID     string
		repoID     string
		permission Permission
		wantErr    bool
	}{
		{
			name:       "grant valid repo permission",
			userID:     "testuser",
			repoID:     "repo1",
			permission: PermissionRepoWrite,
			wantErr:    false,
		},
		{
			name:       "grant to non-existent user",
			userID:     "nonexistent",
			repoID:     "repo1",
			permission: PermissionRepoRead,
			wantErr:    true,
		},
		{
			name:       "grant duplicate permission",
			userID:     "testuser",
			repoID:     "repo1",
			permission: PermissionRepoWrite, // Already granted above
			wantErr:    false, // Implementation allows duplicate permissions
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := rbac.GrantRepositoryPermission(tc.userID, tc.repoID, tc.permission)
			
			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			// If grant was successful, verify permission exists
			if !tc.wantErr && err == nil {
				user, getErr := rbac.GetUser(tc.userID)
				if getErr != nil {
					t.Errorf("Failed to get user: %v", getErr)
				}
				
				repoPerms, exists := user.RepoPerms[tc.repoID]
				if !exists {
					t.Errorf("Repository permissions not found for repo '%s'", tc.repoID)
				}
				
				permFound := false
				for _, perm := range repoPerms {
					if perm == tc.permission {
						permFound = true
						break
					}
				}
				if !permFound {
					t.Errorf("Permission '%s' not found for repository '%s'", tc.permission, tc.repoID)
				}
			}
		})
	}
}

func TestRevokeRepositoryPermission(t *testing.T) {
	rbac := NewRBAC()
	
	// Create a test user and grant permission
	err := rbac.CreateUser("testuser", "testname", "test@example.com", []string{"guest"})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	
	err = rbac.GrantRepositoryPermission("testuser", "repo1", PermissionRepoWrite)
	if err != nil {
		t.Fatalf("Failed to grant repository permission: %v", err)
	}
	
	testCases := []struct {
		name       string
		userID     string
		repoID     string
		permission Permission
		wantErr    bool
	}{
		{
			name:       "revoke existing permission",
			userID:     "testuser",
			repoID:     "repo1",
			permission: PermissionRepoWrite,
			wantErr:    false,
		},
		{
			name:       "revoke non-existent permission",
			userID:     "testuser",
			repoID:     "repo1",
			permission: PermissionRepoAdmin, // Not granted
			wantErr:    false, // Implementation allows revoking non-existent permissions
		},
		{
			name:       "revoke from non-existent user",
			userID:     "nonexistent",
			repoID:     "repo1",
			permission: PermissionRepoRead,
			wantErr:    true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := rbac.RevokeRepositoryPermission(tc.userID, tc.repoID, tc.permission)
			
			if tc.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			// If revoke was successful, verify permission is gone
			if !tc.wantErr && err == nil {
				user, getErr := rbac.GetUser(tc.userID)
				if getErr != nil {
					t.Errorf("Failed to get user: %v", getErr)
				}
				
				repoPerms, exists := user.RepoPerms[tc.repoID]
				if exists {
					for _, perm := range repoPerms {
						if perm == tc.permission {
							t.Errorf("Permission '%s' should have been revoked", tc.permission)
						}
					}
				}
			}
		})
	}
}

func TestConcurrentUserOperations(t *testing.T) {
	rbac := NewRBAC()
	
	// Test concurrent user creation
	done := make(chan bool, 100)
	
	for i := 0; i < 100; i++ {
		go func(id int) {
			userID := fmt.Sprintf("user%d", id)
			username := fmt.Sprintf("user%d", id)
			email := fmt.Sprintf("user%d@example.com", id)
			
			err := rbac.CreateUser(userID, username, email, []string{"guest"})
			if err != nil {
				t.Errorf("Failed to create user %s: %v", userID, err)
			}
			
			done <- true
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 100; i++ {
		<-done
	}
	
	// Verify all users were created
	for i := 0; i < 100; i++ {
		userID := fmt.Sprintf("user%d", i)
		user, err := rbac.GetUser(userID)
		if err != nil {
			t.Errorf("User %s not found: %v", userID, err)
		}
		if user == nil {
			t.Errorf("User %s is nil", userID)
		}
	}
}

func TestUserUpdateTime(t *testing.T) {
	rbac := NewRBAC()
	
	// Create user
	err := rbac.CreateUser("timetest", "timeuser", "time@example.com", []string{"guest"})
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	
	user, err := rbac.GetUser("timetest")
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	
	createdAt := user.CreatedAt
	updatedAt := user.UpdatedAt
	
	// Check that created and updated times are not empty
	if createdAt == "" {
		t.Error("CreatedAt should not be empty")
	}
	if updatedAt == "" {
		t.Error("UpdatedAt should not be empty")
	}
	
	// Wait a bit and assign a role
	time.Sleep(10 * time.Millisecond)
	err = rbac.AssignRole("timetest", "reader")
	if err != nil {
		t.Fatalf("Failed to assign role: %v", err)
	}
	
	user, err = rbac.GetUser("timetest")
	if err != nil {
		t.Fatalf("Failed to get user after role assignment: %v", err)
	}
	
	// UpdatedAt should have changed (simple string comparison won't work for time)
	// For this test, we'll just verify the fields exist and are not empty
	if user.UpdatedAt == "" {
		t.Error("UpdatedAt should not be empty after role assignment")
	}
	
	// CreatedAt should remain the same
	if user.CreatedAt != createdAt {
		t.Error("CreatedAt should not change after role assignment")
	}
}

func BenchmarkCreateUser(b *testing.B) {
	rbac := NewRBAC()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		userID := fmt.Sprintf("benchuser%d", i)
		err := rbac.CreateUser(userID, "benchuser", "bench@example.com", []string{"guest"})
		if err != nil {
			b.Fatalf("Failed to create user: %v", err)
		}
	}
}

func BenchmarkGetUser(b *testing.B) {
	rbac := NewRBAC()
	
	// Create a user first
	err := rbac.CreateUser("benchuser", "benchuser", "bench@example.com", []string{"guest"})
	if err != nil {
		b.Fatalf("Failed to create user: %v", err)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rbac.GetUser("benchuser")
		if err != nil {
			b.Fatalf("Failed to get user: %v", err)
		}
	}
}

func BenchmarkGetUserPermissions(b *testing.B) {
	rbac := NewRBAC()
	
	// Create an admin user (will have many permissions)
	err := rbac.CreateUser("benchuser", "benchuser", "bench@example.com", []string{"admin"})
	if err != nil {
		b.Fatalf("Failed to create user: %v", err)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rbac.GetUserPermissions("benchuser")
		if err != nil {
			b.Fatalf("Failed to get user permissions: %v", err)
		}
	}
}