package cluster

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/caiatech/govc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCluster tests cluster creation
func TestNewCluster(t *testing.T) {
	dataDir := t.TempDir()

	tests := []struct {
		name   string
		id     string
		cname  string
		config ClusterConfig
	}{
		{
			name:  "default config",
			id:    "cluster1",
			cname: "Test Cluster",
			config: ClusterConfig{},
		},
		{
			name:  "custom config",
			id:    "cluster2",
			cname: "Custom Cluster",
			config: ClusterConfig{
				ReplicationFactor:  5,
				ShardSize:          2000,
				ElectionTimeout:    200 * time.Millisecond,
				HeartbeatInterval:  75 * time.Millisecond,
				MaxLogEntries:      10000,
				SnapshotThreshold:  5000,
				AutoRebalance:      true,
				ConsistencyLevel:   "strong",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster, err := NewCluster(tt.id, tt.cname, tt.config, filepath.Join(dataDir, tt.id))
			require.NoError(t, err)
			assert.NotNil(t, cluster)
			assert.Equal(t, tt.id, cluster.ID)
			assert.Equal(t, tt.cname, cluster.Name)
			assert.Equal(t, ClusterStateHealthy, cluster.State)
			assert.NotNil(t, cluster.Nodes)
			assert.NotNil(t, cluster.Shards)

			// Check default values
			if tt.config.ReplicationFactor == 0 {
				assert.Equal(t, 3, cluster.Config.ReplicationFactor)
			}
			if tt.config.ShardSize == 0 {
				assert.Equal(t, 1000, cluster.Config.ShardSize)
			}
			if tt.config.ConsistencyLevel == "" {
				assert.Equal(t, "eventual", cluster.Config.ConsistencyLevel)
			}
		})
	}
}

// TestAddNode tests adding nodes to cluster
func TestAddNode(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Create nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.2", 8082)

	// Add first node
	err = cluster.AddNode(node1)
	assert.NoError(t, err)
	assert.Len(t, cluster.Nodes, 1)
	assert.Equal(t, cluster, node1.cluster)

	// Add second node
	err = cluster.AddNode(node2)
	assert.NoError(t, err)
	assert.Len(t, cluster.Nodes, 2)

	// Try to add duplicate node
	err = cluster.AddNode(node1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// TestRemoveNode tests removing nodes from cluster
func TestRemoveNode(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.2", 8082)
	cluster.AddNode(node1)
	cluster.AddNode(node2)

	// Create shard on node1
	shard := &Shard{
		ID:           "shard1",
		PrimaryNode:  "node1",
		ReplicaNodes: []string{"node2"},
		Repositories: make(map[string]bool),
	}
	cluster.Shards["shard1"] = shard

	// Remove node
	err = cluster.RemoveNode("node1")
	assert.NoError(t, err)
	assert.Len(t, cluster.Nodes, 1)
	assert.Nil(t, node1.cluster)

	// Verify shard was migrated
	assert.NotEqual(t, "node1", shard.PrimaryNode)

	// Try to remove non-existent node
	err = cluster.RemoveNode("node3")
	assert.Error(t, err)
}

// TestGetNodes tests retrieving cluster nodes
func TestGetNodes(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Initially empty
	nodes := cluster.GetNodes()
	assert.Empty(t, nodes)

	// Add nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.2", 8082)
	cluster.AddNode(node1)
	cluster.AddNode(node2)

	// Get all nodes
	nodes = cluster.GetNodes()
	assert.Len(t, nodes, 2)
}

// TestGetLeaderNode tests leader node retrieval
func TestGetLeaderNode(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// No leader initially
	leader := cluster.GetLeaderNode()
	assert.Nil(t, leader)

	// Add nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.2", 8082)
	cluster.AddNode(node1)
	cluster.AddNode(node2)

	// Make node1 leader
	node1.mu.Lock()
	node1.State = NodeStateLeader
	node1.IsLeader = true
	node1.mu.Unlock()

	// Get leader
	leader = cluster.GetLeaderNode()
	assert.NotNil(t, leader)
	assert.Equal(t, "node1", leader.ID)
}

// TestCreateShard tests shard creation
func TestCreateShard(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 3,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	for i := 1; i <= 4; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	// Create shard
	keyRange := ShardKeyRange{Start: "a", End: "m"}
	shard, err := cluster.CreateShard(keyRange)
	assert.NoError(t, err)
	assert.NotNil(t, shard)
	assert.Equal(t, "shard-a-m", shard.ID)
	assert.NotEmpty(t, shard.PrimaryNode)
	assert.Len(t, shard.ReplicaNodes, 2) // ReplicationFactor - 1
	assert.Equal(t, ShardStateActive, shard.State)

	// Verify shard was added to cluster
	assert.NotNil(t, cluster.Shards[shard.ID])
}

// TestGetShardForRepository tests shard lookup for repositories
func TestGetShardForRepository(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Create shards with different key ranges
	shard1 := &Shard{
		ID:       "shard1",
		KeyRange: ShardKeyRange{Start: "a", End: "m"},
		State:    ShardStateActive,
	}
	shard2 := &Shard{
		ID:       "shard2",
		KeyRange: ShardKeyRange{Start: "n", End: "z"},
		State:    ShardStateActive,
	}

	cluster.Shards["shard1"] = shard1
	cluster.Shards["shard2"] = shard2

	// Test repository mapping
	tests := []struct {
		repoID       string
		expectedShard *Shard
	}{
		{"apple", shard1},
		{"banana", shard1},
		{"orange", shard2},
		{"zebra", shard2},
	}

	for _, tt := range tests {
		t.Run(tt.repoID, func(t *testing.T) {
			shard := cluster.GetShardForRepository(tt.repoID)
			assert.Equal(t, tt.expectedShard, shard)
		})
	}
}

// TestDistributeRepository tests repository distribution
func TestDistributeRepository(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 2,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.2", 8082)
	cluster.AddNode(node1)
	cluster.AddNode(node2)

	// Distribute repository
	repo := &govc.Repository{}
	err = cluster.DistributeRepository("repo1", repo)
	assert.NoError(t, err)

	// Find the shard
	shard := cluster.GetShardForRepository("repo1")
	assert.NotNil(t, shard)
	assert.Contains(t, shard.Repositories, "repo1")

	// Verify repository is on primary and replica nodes
	primaryNode := cluster.GetNodeByID(shard.PrimaryNode)
	assert.NotNil(t, primaryNode.GetRepository("repo1"))

	for _, replicaID := range shard.ReplicaNodes {
		replicaNode := cluster.GetNodeByID(replicaID)
		assert.NotNil(t, replicaNode.GetRepository("repo1"))
	}
}

// TestClusterHealth tests cluster health calculation
func TestClusterHealth(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Empty cluster
	health := cluster.GetClusterHealth()
	assert.Equal(t, ClusterStateHealthy, health.Status)
	assert.Equal(t, 0, health.TotalNodes)
	assert.Equal(t, 0, health.HealthyNodes)

	// Add healthy nodes
	for i := 1; i <= 3; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	health = cluster.GetClusterHealth()
	assert.Equal(t, ClusterStateHealthy, health.Status)
	assert.Equal(t, 3, health.TotalNodes)
	assert.Equal(t, 3, health.HealthyNodes)

	// Make nodes offline
	for _, node := range cluster.Nodes {
		node.mu.Lock()
		node.State = NodeStateOffline
		node.mu.Unlock()
	}

	health = cluster.GetClusterHealth()
	assert.Equal(t, ClusterStateUnavailable, health.Status)
	assert.Equal(t, 0, health.HealthyNodes)

	// Partial recovery
	node1 := cluster.GetNodeByID("node1")
	node1.mu.Lock()
	node1.State = NodeStateFollower
	node1.mu.Unlock()

	health = cluster.GetClusterHealth()
	assert.Equal(t, ClusterStateDegraded, health.Status)
	assert.Equal(t, 1, health.HealthyNodes)
}

// TestShardSelection tests node selection for shards
func TestShardSelection(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 3,
	}, t.TempDir())
	require.NoError(t, err)

	// Test with no nodes
	primary, replicas := cluster.selectNodesForShard()
	assert.Empty(t, primary)
	assert.Nil(t, replicas)

	// Add nodes
	for i := 1; i <= 5; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	// Test selection
	primary, replicas = cluster.selectNodesForShard()
	assert.NotEmpty(t, primary)
	assert.Len(t, replicas, 2) // ReplicationFactor - 1

	// Verify no duplicates
	nodes := append(replicas, primary)
	seen := make(map[string]bool)
	for _, nodeID := range nodes {
		assert.False(t, seen[nodeID])
		seen[nodeID] = true
	}
}

// TestClusterStatePersistence tests saving and loading cluster state
func TestClusterStatePersistence(t *testing.T) {
	dataDir := t.TempDir()

	// Create cluster
	cluster1, err := NewCluster("persist-test", "Persistence Test", ClusterConfig{
		ReplicationFactor: 3,
		AutoRebalance:     true,
	}, dataDir)
	require.NoError(t, err)

	// Add nodes and shards
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	cluster1.AddNode(node1)

	shard := &Shard{
		ID:       "shard1",
		KeyRange: ShardKeyRange{Start: "a", End: "z"},
	}
	cluster1.Shards["shard1"] = shard

	// Save state
	err = cluster1.saveState()
	assert.NoError(t, err)

	// Verify state file exists
	statePath := filepath.Join(dataDir, "cluster-state.json")
	_, err = os.Stat(statePath)
	assert.NoError(t, err)

	// Create new cluster instance and load state
	cluster2, err := NewCluster("persist-test", "Persistence Test", ClusterConfig{}, dataDir)
	require.NoError(t, err)

	// Verify state was loaded
	assert.Equal(t, cluster1.ID, cluster2.ID)
	assert.Equal(t, cluster1.Name, cluster2.Name)
	assert.Equal(t, cluster1.Config.ReplicationFactor, cluster2.Config.ReplicationFactor)
	assert.Len(t, cluster2.Shards, 1)
	assert.NotNil(t, cluster2.Shards["shard1"])
}

// TestMigrateNodeShards tests shard migration when removing nodes
func TestMigrateNodeShards(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	for i := 1; i <= 4; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	// Create shards
	shard1 := &Shard{
		ID:           "shard1",
		PrimaryNode:  "node1",
		ReplicaNodes: []string{"node2", "node3"},
	}
	shard2 := &Shard{
		ID:           "shard2",
		PrimaryNode:  "node2",
		ReplicaNodes: []string{"node1", "node3"},
	}

	cluster.Shards["shard1"] = shard1
	cluster.Shards["shard2"] = shard2

	// Migrate shards from node1
	err = cluster.migrateNodeShards("node1")
	assert.NoError(t, err)

	// Verify shard1 primary was changed
	assert.NotEqual(t, "node1", shard1.PrimaryNode)
	assert.Equal(t, "node2", shard1.PrimaryNode) // First replica promoted

	// Verify node1 removed from replicas
	assert.NotContains(t, shard1.ReplicaNodes, "node1")
	assert.NotContains(t, shard2.ReplicaNodes, "node1")
}

// TestKeyRangeValidation tests key range validation
func TestKeyRangeValidation(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())

	tests := []struct {
		key      string
		keyRange ShardKeyRange
		expected bool
	}{
		{"apple", ShardKeyRange{Start: "a", End: "m"}, true},
		{"banana", ShardKeyRange{Start: "a", End: "m"}, true},
		{"orange", ShardKeyRange{Start: "a", End: "m"}, false},
		{"zebra", ShardKeyRange{Start: "n", End: "z"}, true},
		{"", ShardKeyRange{Start: "a", End: "z"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := cluster.keyInRange(tt.key, tt.keyRange)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConcurrentClusterOperations tests concurrent operations
func TestConcurrentClusterOperations(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 2,
	}, t.TempDir())
	require.NoError(t, err)

	// Add initial nodes
	for i := 1; i <= 10; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	done := make(chan bool)
	errors := make(chan error, 100)

	// Concurrent node additions
	for i := 11; i <= 20; i++ {
		go func(id int) {
			node, _ := createTestNode(fmt.Sprintf("node%d", id), "127.0.0.1", 8080+id)
			if err := cluster.AddNode(node); err != nil {
				errors <- err
			}
			done <- true
		}(i)
	}

	// Concurrent repository distributions
	for i := 0; i < 20; i++ {
		go func(id int) {
			repo := &govc.Repository{}
			if err := cluster.DistributeRepository(fmt.Sprintf("repo%d", id), repo); err != nil {
				errors <- err
			}
			done <- true
		}(i)
	}

	// Concurrent health checks
	for i := 0; i < 5; i++ {
		go func() {
			_ = cluster.GetClusterHealth()
			done <- true
		}()
	}

	// Wait for completion
	for i := 0; i < 45; i++ {
		<-done
	}

	// Check for errors
	close(errors)
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}

	// Verify final state
	assert.Equal(t, 20, len(cluster.Nodes))
	assert.Greater(t, len(cluster.Shards), 0)
}

// TestClusterEventTypes tests cluster event type constants
func TestClusterEventTypes(t *testing.T) {
	// Verify event types are defined correctly
	assert.Equal(t, ClusterEventType("node_joined"), EventNodeJoined)
	assert.Equal(t, ClusterEventType("node_left"), EventNodeLeft)
	assert.Equal(t, ClusterEventType("node_failed"), EventNodeFailed)
	assert.Equal(t, ClusterEventType("shard_moved"), EventShardMoved)
	assert.Equal(t, ClusterEventType("rebalance_started"), EventRebalanceStarted)
	assert.Equal(t, ClusterEventType("rebalance_complete"), EventRebalanceComplete)
	assert.Equal(t, ClusterEventType("leader_elected"), EventLeaderElected)
}