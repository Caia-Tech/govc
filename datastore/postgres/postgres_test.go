package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/Caia-Tech/govc/datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestConfig() datastore.Config {
	// Get connection string from environment or use default
	connStr := os.Getenv("POSTGRES_TEST_URL")
	if connStr == "" {
		// Skip tests if no PostgreSQL is available
		return datastore.Config{}
	}
	
	return datastore.Config{
		Type:               datastore.TypePostgres,
		Connection:         connStr,
		MaxConnections:     10,
		MaxIdleConnections: 2,
	}
}

func TestPostgresStore(t *testing.T) {
	config := getTestConfig()
	if config.Connection == "" {
		t.Skip("PostgreSQL not available, set POSTGRES_TEST_URL to run tests")
	}
	
	store, err := New(config)
	require.NoError(t, err)
	
	err = store.Initialize(config)
	require.NoError(t, err)
	defer store.Close()
	
	t.Run("BasicOperations", func(t *testing.T) {
		// Test object storage
		hash := "test-hash-postgres"
		data := []byte("test data for postgres")
		
		// Put object
		err := store.ObjectStore().PutObject(hash, data)
		assert.NoError(t, err)
		
		// Get object
		retrieved, err := store.ObjectStore().GetObject(hash)
		assert.NoError(t, err)
		assert.Equal(t, data, retrieved)
		
		// Check existence
		exists, err := store.ObjectStore().HasObject(hash)
		assert.NoError(t, err)
		assert.True(t, exists)
		
		// Delete object
		err = store.ObjectStore().DeleteObject(hash)
		assert.NoError(t, err)
		
		// Verify deletion
		exists, err = store.ObjectStore().HasObject(hash)
		assert.NoError(t, err)
		assert.False(t, exists)
	})
	
	t.Run("Transaction", func(t *testing.T) {
		ctx := context.Background()
		
		// Begin transaction
		tx, err := store.BeginTx(ctx, nil)
		require.NoError(t, err)
		
		// This would need the transaction implementation
		// For now, just rollback
		err = tx.Rollback()
		assert.NoError(t, err)
	})
	
	t.Run("HealthCheck", func(t *testing.T) {
		ctx := context.Background()
		err := store.HealthCheck(ctx)
		assert.NoError(t, err)
	})
	
	t.Run("Info", func(t *testing.T) {
		info := store.Info()
		assert.Equal(t, datastore.TypePostgres, info["type"])
		assert.Contains(t, info, "postgres_version")
		assert.Contains(t, info, "database_size")
	})
}

func TestPostgresStoreWithoutDB(t *testing.T) {
	// Test that we handle connection errors gracefully
	config := datastore.Config{
		Type:       datastore.TypePostgres,
		Connection: "postgres://invalid:invalid@nonexistent:5432/test",
	}
	
	store, err := New(config)
	require.NoError(t, err)
	
	err = store.Initialize(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to ping database")
}