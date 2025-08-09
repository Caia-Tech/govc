package datastore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFactory_Registration(t *testing.T) {
	t.Run("RegisterFactory", func(t *testing.T) {
		// Create a test registry to avoid interfering with global state
		registry := &Registry{
			factories: make(map[string]Factory),
		}
		
		testFactory := func(config Config) (DataStore, error) {
			return nil, nil
		}
		
		// Register factory
		registry.Register("test-type", testFactory)
		
		// Verify it's registered
		assert.Contains(t, registry.factories, "test-type")
		
		// List types should include our type
		types := registry.ListTypes()
		assert.Contains(t, types, "test-type")
	})

	t.Run("RegisterNilFactory", func(t *testing.T) {
		registry := &Registry{
			factories: make(map[string]Factory),
		}
		
		// Should panic when registering nil factory
		assert.Panics(t, func() {
			registry.Register("nil-factory", nil)
		})
	})

	t.Run("RegisterDuplicateFactory", func(t *testing.T) {
		registry := &Registry{
			factories: make(map[string]Factory),
		}
		
		testFactory := func(config Config) (DataStore, error) {
			return nil, nil
		}
		
		// Register factory
		registry.Register("duplicate-test", testFactory)
		
		// Should panic when registering duplicate
		assert.Panics(t, func() {
			registry.Register("duplicate-test", testFactory)
		})
	})
}

func TestFactory_Creation(t *testing.T) {
	t.Run("CreateUnknownType", func(t *testing.T) {
		registry := &Registry{
			factories: make(map[string]Factory),
		}
		
		config := Config{
			Type: "unknown-type",
		}
		
		_, err := registry.Create(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown datastore type")
	})

	t.Run("CreateInvalidConfig", func(t *testing.T) {
		registry := &Registry{
			factories: make(map[string]Factory),
		}
		
		config := Config{
			Type:       "", // Empty type should be invalid
			Connection: "test",
		}
		
		_, err := registry.Create(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid configuration")
	})

	t.Run("CreateWithFailingFactory", func(t *testing.T) {
		registry := &Registry{
			factories: make(map[string]Factory),
		}
		
		failingFactory := func(config Config) (DataStore, error) {
			return nil, assert.AnError
		}
		
		registry.Register("failing-type", failingFactory)
		
		config := Config{
			Type: "failing-type",
		}
		
		_, err := registry.Create(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create failing-type datastore")
	})
}

func TestConfig_Validation(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		config := Config{
			Type:               TypeMemory,
			Connection:         "/tmp/test.db",
			MaxConnections:     10,
			MaxIdleConnections: 2,
			ConnectionTimeout:  time.Second * 30,
			EnableCompression:  true,
		}
		
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("EmptyType", func(t *testing.T) {
		config := Config{
			Type:       "",
			Connection: "test",
		}
		
		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "type is required")
	})

	t.Run("InvalidConnections", func(t *testing.T) {
		config := Config{
			Type:           TypeMemory,
			MaxConnections: -1,
		}
		
		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max_connections cannot be negative")
		
		config = Config{
			Type:               TypeMemory,
			MaxConnections:     10,
			MaxIdleConnections: 20, // More than max connections
		}
		
		err = config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max_idle_connections cannot exceed max_connections")
	})

	t.Run("InvalidTimeout", func(t *testing.T) {
		config := Config{
			Type:              TypeMemory,
			ConnectionTimeout: -time.Second,
		}
		
		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection_timeout must be positive")
	})
}

func TestDataStoreTypes(t *testing.T) {
	t.Run("TypeConstants", func(t *testing.T) {
		// Verify type constants are defined
		assert.NotEmpty(t, TypeMemory)
		assert.NotEmpty(t, TypeSQLite)
		assert.NotEmpty(t, TypeBadger)
		assert.NotEmpty(t, TypePostgres)
		
		// Verify they are unique
		types := []string{TypeMemory, TypeSQLite, TypeBadger, TypePostgres}
		seen := make(map[string]bool)
		for _, typ := range types {
			assert.False(t, seen[typ], "Type %s is duplicated", typ)
			seen[typ] = true
		}
	})
}

func TestErrorTypes(t *testing.T) {
	t.Run("ErrorConstants", func(t *testing.T) {
		// Verify error constants are defined
		assert.NotNil(t, ErrNotFound)
		assert.NotNil(t, ErrAlreadyExists)
		
		// Verify error messages
		assert.Contains(t, ErrNotFound.Error(), "not found")
		assert.Contains(t, ErrAlreadyExists.Error(), "already exists")
	})

	t.Run("ErrorComparison", func(t *testing.T) {
		// Test error comparison
		assert.True(t, ErrNotFound == ErrNotFound)
		assert.False(t, ErrNotFound == ErrAlreadyExists)
		
		// Test with error wrapping
		wrappedErr := &WrappedError{
			Err:     ErrNotFound,
			Message: "additional context",
		}
		
		assert.ErrorIs(t, wrappedErr, ErrNotFound)
		assert.NotErrorIs(t, wrappedErr, ErrAlreadyExists)
	})
}

func TestIsolationLevels(t *testing.T) {
	t.Run("IsolationConstants", func(t *testing.T) {
		// Verify isolation level constants
		levels := []IsolationLevel{
			IsolationReadUncommitted,
			IsolationReadCommitted,
			IsolationRepeatableRead,
			IsolationSerializable,
		}
		
		// Should be unique values
		seen := make(map[IsolationLevel]bool)
		for _, level := range levels {
			assert.False(t, seen[level], "Isolation level %d is duplicated", level)
			seen[level] = true
		}
		
		// Should be ordered by strength
		assert.Less(t, int(IsolationReadUncommitted), int(IsolationReadCommitted))
		assert.Less(t, int(IsolationReadCommitted), int(IsolationRepeatableRead))
		assert.Less(t, int(IsolationRepeatableRead), int(IsolationSerializable))
	})
}

func TestRefTypes(t *testing.T) {
	t.Run("RefTypeConstants", func(t *testing.T) {
		// Verify reference type constants
		assert.NotEmpty(t, RefTypeBranch)
		assert.NotEmpty(t, RefTypeTag)
		
		// Should be different
		assert.NotEqual(t, RefTypeBranch, RefTypeTag)
	})

	t.Run("RefTypeValidation", func(t *testing.T) {
		// Test with valid reference
		ref := &Reference{
			Name: "refs/heads/main",
			Hash: "abc123",
			Type: RefTypeBranch,
		}
		
		// Should accept valid type
		assert.Equal(t, RefTypeBranch, ref.Type)
		
		// Test with tag
		ref.Type = RefTypeTag
		assert.Equal(t, RefTypeTag, ref.Type)
	})
}

func TestTxOptions(t *testing.T) {
	t.Run("DefaultTxOptions", func(t *testing.T) {
		opts := &TxOptions{}
		
		// Default values
		assert.False(t, opts.ReadOnly)
		assert.Equal(t, IsolationLevel(0), opts.Isolation)
		assert.Zero(t, opts.Timeout)
	})

	t.Run("CustomTxOptions", func(t *testing.T) {
		opts := &TxOptions{
			ReadOnly:  true,
			Isolation: IsolationSerializable,
			Timeout:   time.Second * 30,
		}
		
		assert.True(t, opts.ReadOnly)
		assert.Equal(t, IsolationSerializable, opts.Isolation)
		assert.Equal(t, time.Second*30, opts.Timeout)
	})
}

func TestFilters(t *testing.T) {
	t.Run("RepositoryFilter", func(t *testing.T) {
		filter := RepositoryFilter{
			Limit:  10,
			Offset: 20,
			OrderBy: "name",
		}
		
		assert.Equal(t, 10, filter.Limit)
		assert.Equal(t, 20, filter.Offset)
		assert.Equal(t, "name", filter.OrderBy)
	})

	t.Run("UserFilter", func(t *testing.T) {
		isActive := true
		filter := UserFilter{
			Limit:    50,
			Offset:   0,
			IsActive: &isActive,
		}
		
		assert.Equal(t, 50, filter.Limit)
		assert.Equal(t, 0, filter.Offset)
		assert.NotNil(t, filter.IsActive)
		assert.True(t, *filter.IsActive)
	})

	t.Run("EventFilter", func(t *testing.T) {
		now := time.Now()
		after := now.Add(-time.Hour)
		filter := EventFilter{
			UserID:  "user123",
			Action:  "create",
			After:   &after,
			Before:  &now,
			Limit:   100,
		}
		
		assert.Equal(t, "user123", filter.UserID)
		assert.Equal(t, "create", filter.Action)
		assert.NotNil(t, filter.After)
		assert.Equal(t, now.Add(-time.Hour), *filter.After)
		assert.NotNil(t, filter.Before)
		assert.Equal(t, now, *filter.Before)
		assert.Equal(t, 100, filter.Limit)
	})
}

func TestDataStructures(t *testing.T) {
	t.Run("Repository", func(t *testing.T) {
		now := time.Now()
		repo := &Repository{
			ID:          "repo123",
			Name:        "test-repo",
			Description: "A test repository",
			Path:        "/path/to/repo",
			IsPrivate:   true,
			Metadata: map[string]interface{}{
				"key": "value",
			},
			Size:        1024,
			CommitCount: 10,
			BranchCount: 3,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		
		// Verify all fields are set
		assert.Equal(t, "repo123", repo.ID)
		assert.Equal(t, "test-repo", repo.Name)
		assert.Equal(t, "A test repository", repo.Description)
		assert.Equal(t, "/path/to/repo", repo.Path)
		assert.True(t, repo.IsPrivate)
		assert.Equal(t, "value", repo.Metadata["key"])
		assert.Equal(t, int64(1024), repo.Size)
		assert.Equal(t, 10, repo.CommitCount)
		assert.Equal(t, 3, repo.BranchCount)
		assert.Equal(t, now, repo.CreatedAt)
		assert.Equal(t, now, repo.UpdatedAt)
	})

	t.Run("User", func(t *testing.T) {
		now := time.Now()
		user := &User{
			ID:          "user123",
			Username:    "testuser",
			Email:       "test@example.com",
			FullName:    "Test User",
			IsActive:    true,
			IsAdmin:     false,
			Metadata:    map[string]interface{}{"role": "developer"},
			CreatedAt:   now,
			UpdatedAt:   now,
			LastLoginAt: &now,
		}
		
		assert.Equal(t, "user123", user.ID)
		assert.Equal(t, "testuser", user.Username)
		assert.Equal(t, "test@example.com", user.Email)
		assert.Equal(t, "Test User", user.FullName)
		assert.True(t, user.IsActive)
		assert.False(t, user.IsAdmin)
		assert.Equal(t, "developer", user.Metadata["role"])
		assert.Equal(t, now, user.CreatedAt)
		assert.Equal(t, now, user.UpdatedAt)
		assert.NotNil(t, user.LastLoginAt)
		assert.Equal(t, now, *user.LastLoginAt)
	})

	t.Run("Reference", func(t *testing.T) {
		now := time.Now()
		ref := &Reference{
			Name:      "refs/heads/main",
			Hash:      "abc123def456",
			Type:      RefTypeBranch,
			UpdatedAt: now,
			UpdatedBy: "user123",
		}
		
		assert.Equal(t, "refs/heads/main", ref.Name)
		assert.Equal(t, "abc123def456", ref.Hash)
		assert.Equal(t, RefTypeBranch, ref.Type)
		assert.Equal(t, now, ref.UpdatedAt)
		assert.Equal(t, "user123", ref.UpdatedBy)
	})

	t.Run("AuditEvent", func(t *testing.T) {
		now := time.Now()
		event := &AuditEvent{
			ID:         "event123",
			Timestamp:  now,
			UserID:     "user123",
			Username:   "testuser",
			Action:     "create",
			Resource:   "repository",
			ResourceID: "repo123",
			Details:    map[string]interface{}{"name": "test-repo"},
			IPAddress:  "192.168.1.1",
			UserAgent:  "test-client/1.0",
			Success:    true,
			ErrorMsg:   "",
		}
		
		assert.Equal(t, "event123", event.ID)
		assert.Equal(t, now, event.Timestamp)
		assert.Equal(t, "user123", event.UserID)
		assert.Equal(t, "testuser", event.Username)
		assert.Equal(t, "create", event.Action)
		assert.Equal(t, "repository", event.Resource)
		assert.Equal(t, "repo123", event.ResourceID)
		assert.Equal(t, "test-repo", event.Details["name"])
		assert.Equal(t, "192.168.1.1", event.IPAddress)
		assert.Equal(t, "test-client/1.0", event.UserAgent)
		assert.True(t, event.Success)
		assert.Empty(t, event.ErrorMsg)
	})

	t.Run("Metrics", func(t *testing.T) {
		now := time.Now()
		metrics := Metrics{
			Reads:             100,
			Writes:            50,
			Deletes:           10,
			ObjectCount:       1000,
			StorageSize:       1024000,
			ActiveConnections: 5,
			TotalConnections:  100,
			StartTime:         now,
			Uptime:            time.Hour,
		}
		
		assert.Equal(t, int64(100), metrics.Reads)
		assert.Equal(t, int64(50), metrics.Writes)
		assert.Equal(t, int64(10), metrics.Deletes)
		assert.Equal(t, int64(1000), metrics.ObjectCount)
		assert.Equal(t, int64(1024000), metrics.StorageSize)
		assert.Equal(t, 5, metrics.ActiveConnections)
		assert.Equal(t, int64(100), metrics.TotalConnections)
		assert.Equal(t, now, metrics.StartTime)
		assert.Equal(t, time.Hour, metrics.Uptime)
	})
}

// WrappedError is a test error type for testing error wrapping
type WrappedError struct {
	Err     error
	Message string
}

func (e *WrappedError) Error() string {
	return e.Message + ": " + e.Err.Error()
}

func (e *WrappedError) Unwrap() error {
	return e.Err
}