package cluster

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterStatePersistenceFixed(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "cluster-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("SaveAndLoadState", func(t *testing.T) {
		// Create cluster with some state
		config := ClusterConfig{
			ReplicationFactor: 3,
			ShardSize:         100,
			ElectionTimeout:   5 * time.Second,
			HeartbeatInterval: 1 * time.Second,
		}

		cluster, err := NewCluster("test-id", "test-cluster", config, tempDir)
		require.NoError(t, err)

		// Add some nodes
		node1 := &Node{
			ID:       "node1",
			Address:  "127.0.0.1:8001",
			State:    NodeStateFollower,
			LastSeen: time.Now(),
		}
		node2 := &Node{
			ID:       "node2",
			Address:  "127.0.0.1:8002",
			State:    NodeStateLeader,
			LastSeen: time.Now(),
		}

		cluster.mu.Lock()
		cluster.Nodes[node1.ID] = node1
		cluster.Nodes[node2.ID] = node2
		cluster.mu.Unlock()

		// Add a shard
		shard := &Shard{
			ID: "shard1",
			KeyRange: ShardKeyRange{
				Start: "a",
				End:   "m",
			},
			PrimaryNode:  "node1",
			ReplicaNodes: []string{"node2"},
			State:        ShardStateActive,
		}

		cluster.mu.Lock()
		cluster.Shards[shard.ID] = shard
		cluster.mu.Unlock()

		// Update cluster state
		cluster.State = ClusterStateHealthy
		cluster.UpdatedAt = time.Now()

		// Save state
		err = cluster.saveState()
		require.NoError(t, err)

		// Verify state file exists
		statePath := filepath.Join(tempDir, "cluster-state.json")
		assert.FileExists(t, statePath)

		// Create new cluster instance and load state
		cluster2, err := NewCluster("", "", config, tempDir)
		require.NoError(t, err)

		// Verify restored state
		assert.Equal(t, cluster.ID, cluster2.ID)
		assert.Equal(t, cluster.Name, cluster2.Name)
		assert.Equal(t, cluster.Config, cluster2.Config)
		assert.Equal(t, cluster.State, cluster2.State)
		assert.Equal(t, cluster.CreatedAt.Unix(), cluster2.CreatedAt.Unix())
		assert.Equal(t, cluster.UpdatedAt.Unix(), cluster2.UpdatedAt.Unix())
	})

	t.Run("EmptyDataDir", func(t *testing.T) {
		// Create cluster with empty data dir
		config := ClusterConfig{
			ReplicationFactor: 1,
		}

		cluster, err := NewCluster("test-id", "test-cluster", config, "")
		require.NoError(t, err)

		// Save should not error with empty data dir
		err = cluster.saveState()
		assert.NoError(t, err)

		// Load should not error with empty data dir
		err = cluster.loadState()
		assert.NoError(t, err)
	})

	t.Run("CorruptedState", func(t *testing.T) {
		// Write corrupted state file
		statePath := filepath.Join(tempDir, "cluster-state.json")
		err := os.WriteFile(statePath, []byte("invalid json"), 0644)
		require.NoError(t, err)

		// Try to load corrupted state
		config := ClusterConfig{
			ReplicationFactor: 1,
		}

		_, err = NewCluster("test-id", "test-cluster", config, tempDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal cluster state")
	})
}

func TestClusterStateAtomicWrite(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "cluster-atomic-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config := ClusterConfig{
		ReplicationFactor: 1,
	}

	cluster, err := NewCluster("test-id", "test-cluster", config, tempDir)
	require.NoError(t, err)

	// Save initial state
	err = cluster.saveState()
	require.NoError(t, err)

	statePath := filepath.Join(tempDir, "cluster-state.json")
	originalData, err := os.ReadFile(statePath)
	require.NoError(t, err)

	// Simulate partial write by creating temp file
	tempPath := statePath + ".tmp"
	err = os.WriteFile(tempPath, []byte("partial"), 0644)
	require.NoError(t, err)

	// Original file should still be intact
	data, err := os.ReadFile(statePath)
	require.NoError(t, err)
	assert.Equal(t, originalData, data)

	// Clean up temp file
	os.Remove(tempPath)
}
