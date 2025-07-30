package cluster

import (
	"fmt"
	"testing"

	"github.com/caiatech/govc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConsistentHashRing tests consistent hash ring operations
func TestConsistentHashRing(t *testing.T) {
	ring := NewConsistentHashRing(160)

	// Test empty ring
	node := ring.GetNode("test-key")
	assert.Empty(t, node)

	// Add nodes
	ring.AddNode("node1")
	ring.AddNode("node2")
	ring.AddNode("node3")

	// Test getting nodes
	node1 := ring.GetNode("key1")
	assert.NotEmpty(t, node1)

	// Test consistency - same key should map to same node
	node1Again := ring.GetNode("key1")
	assert.Equal(t, node1, node1Again)

	// Test different keys map to potentially different nodes
	keys := []string{"key1", "key2", "key3", "key4", "key5"}
	nodeDistribution := make(map[string]int)
	for _, key := range keys {
		node := ring.GetNode(key)
		nodeDistribution[node]++
	}
	
	// Should have some distribution across nodes
	assert.Greater(t, len(nodeDistribution), 1)

	// Remove a node
	ring.RemoveNode("node2")
	
	// Keys should still map to remaining nodes
	node1AfterRemoval := ring.GetNode("key1")
	assert.NotEmpty(t, node1AfterRemoval)
}

// TestConsistentHashRingDistribution tests key distribution
func TestConsistentHashRingDistribution(t *testing.T) {
	ring := NewConsistentHashRing(160)

	// Add multiple nodes
	numNodes := 10
	for i := 1; i <= numNodes; i++ {
		ring.AddNode(fmt.Sprintf("node%d", i))
	}

	// Generate many keys and check distribution
	numKeys := 10000
	distribution := make(map[string]int)
	
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key-%d", i)
		node := ring.GetNode(key)
		distribution[node]++
	}

	// Check that all nodes got some keys
	assert.Len(t, distribution, numNodes)

	// Check distribution is reasonably balanced
	// Each node should get roughly numKeys/numNodes keys
	expectedPerNode := float64(numKeys) / float64(numNodes)
	tolerance := 0.3 // Allow 30% deviation

	for node, count := range distribution {
		deviation := float64(count) / expectedPerNode
		assert.InDelta(t, 1.0, deviation, tolerance, 
			"Node %s has %d keys, expected ~%.0f", node, count, expectedPerNode)
	}
}

// TestConsistentHashRingReplication tests getting multiple nodes for replication
func TestConsistentHashRingReplication(t *testing.T) {
	ring := NewConsistentHashRing(160)

	// Add nodes
	ring.AddNode("node1")
	ring.AddNode("node2")
	ring.AddNode("node3")
	ring.AddNode("node4")

	// Test getting multiple nodes
	nodes := ring.GetNodes("test-key", 3)
	assert.Len(t, nodes, 3)
	
	// All nodes should be unique
	seen := make(map[string]bool)
	for _, node := range nodes {
		assert.False(t, seen[node], "Duplicate node in replication set")
		seen[node] = true
	}

	// Test when requesting more nodes than available
	nodes = ring.GetNodes("test-key", 10)
	assert.Len(t, nodes, 4) // Should return all available nodes
}

// TestShardingManager tests sharding manager creation
func TestShardingManager(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 3,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	for i := 1; i <= 3; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	sm := NewShardingManager(cluster)
	assert.NotNil(t, sm)
	assert.NotNil(t, sm.hashRing)
	assert.NotNil(t, sm.shardMap)
	assert.NotNil(t, sm.rebalancer)
	assert.NotNil(t, sm.migrationQueue)
}

// TestGetShardForRepositorySharding tests shard assignment
func TestGetShardForRepositorySharding(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 3,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	for i := 1; i <= 3; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	sm := NewShardingManager(cluster)

	// Get shard for repository
	shard := sm.GetShardForRepository("repo1")
	assert.NotNil(t, shard)
	assert.NotEmpty(t, shard.ID)
	assert.NotEmpty(t, shard.PrimaryNode)
	assert.Len(t, shard.ReplicaNodes, 2) // ReplicationFactor - 1

	// Same repository should map to same shard
	shard2 := sm.GetShardForRepository("repo1")
	assert.Equal(t, shard.ID, shard2.ID)

	// Different repository might map to different shard
	shard3 := sm.GetShardForRepository("repo2")
	assert.NotNil(t, shard3)
}

// TestDistributeRepositorySharding tests repository distribution
func TestDistributeRepositorySharding(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 2,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.1", 8082)
	cluster.AddNode(node1)
	cluster.AddNode(node2)

	sm := NewShardingManager(cluster)

	// Distribute repository
	repo := &govc.Repository{}
	err = sm.DistributeRepository("repo1", repo)
	assert.NoError(t, err)

	// Verify repository is on primary node
	shard := sm.GetShardForRepository("repo1")
	primaryNode := cluster.GetNodeByID(shard.PrimaryNode)
	assert.NotNil(t, primaryNode.GetRepository("repo1"))

	// Verify repository is on replica nodes
	for _, replicaID := range shard.ReplicaNodes {
		replicaNode := cluster.GetNodeByID(replicaID)
		assert.NotNil(t, replicaNode.GetRepository("repo1"))
	}
}

// TestShardMetrics tests shard metrics calculation
func TestShardMetrics(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 2,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	for i := 1; i <= 3; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	sm := NewShardingManager(cluster)

	// Distribute multiple repositories
	for i := 1; i <= 10; i++ {
		repoID := fmt.Sprintf("repo%d", i)
		sm.DistributeRepository(repoID, &govc.Repository{})
	}

	// Calculate metrics
	metrics := sm.CalculateShardMetrics()
	
	assert.Greater(t, metrics.TotalShards, 0)
	assert.NotEmpty(t, metrics.ShardsPerNode)
	assert.NotEmpty(t, metrics.RepositoriesPerShard)
	assert.GreaterOrEqual(t, metrics.LoadBalance, 0.0)
	assert.GreaterOrEqual(t, metrics.Imbalance, 0.0)
}

// TestShardMigrationSharding tests shard migration
func TestShardMigrationSharding(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 2,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.1", 8082)
	node3, _ := createTestNode("node3", "127.0.0.1", 8083)
	cluster.AddNode(node1)
	cluster.AddNode(node2)
	cluster.AddNode(node3)

	sm := NewShardingManager(cluster)

	// Create and distribute repository
	sm.DistributeRepository("repo1", &govc.Repository{})
	
	// Get shard
	shard := sm.GetShardForRepository("repo1")
	originalPrimary := shard.PrimaryNode

	// Migrate shard
	targetNode := "node3"
	if originalPrimary == "node3" {
		targetNode = "node2"
	}
	
	err = sm.MigrateShard(shard.ID, originalPrimary, targetNode)
	assert.NoError(t, err)

	// Verify migration task was created
	sm.migrationQueue.mu.RLock()
	assert.Greater(t, len(sm.migrationQueue.pending), 0)
	sm.migrationQueue.mu.RUnlock()
}

// TestRebalancer tests automatic rebalancing
func TestRebalancer(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 2,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	for i := 1; i <= 3; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	sm := NewShardingManager(cluster)

	// Create imbalanced distribution
	// Force all shards to node1
	for i := 1; i <= 10; i++ {
		shardID := fmt.Sprintf("shard%d", i)
		shard := &Shard{
			ID:           shardID,
			PrimaryNode:  "node1",
			ReplicaNodes: []string{"node2"},
			State:        ShardStateActive,
			Repositories: make(map[string]bool),
		}
		sm.shardMap[shardID] = shard
	}

	// Calculate metrics - should show imbalance
	metrics := sm.CalculateShardMetrics()
	assert.Greater(t, metrics.Imbalance, sm.rebalancer.threshold)

	// Generate rebalance tasks
	tasks := sm.generateRebalanceTasks(metrics)
	assert.Greater(t, len(tasks), 0)

	// Verify tasks move shards from overloaded to underloaded nodes
	for _, task := range tasks {
		assert.Equal(t, "node1", task.SourceNode) // All from overloaded node
		assert.NotEqual(t, "node1", task.TargetNode) // To other nodes
	}
}

// TestKeyRangeCalculation tests key range calculation
func TestKeyRangeCalculation(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	sm := NewShardingManager(cluster)

	tests := []struct {
		repoID        string
		expectedStart string
		expectedEnd   string
	}{
		{"apple", "a", "b"},
		{"banana", "b", "c"},
		{"zebra", "z", "z"},
		{"123", "1", "2"},
	}

	for _, tt := range tests {
		t.Run(tt.repoID, func(t *testing.T) {
			keyRange := sm.calculateKeyRange(tt.repoID)
			assert.Equal(t, tt.expectedStart, keyRange.Start)
			assert.Equal(t, tt.expectedEnd, keyRange.End)
		})
	}
}

// TestConcurrentShardOperations tests concurrent shard operations
func TestConcurrentShardOperations(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 2,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	for i := 1; i <= 5; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	sm := NewShardingManager(cluster)

	// Run concurrent operations
	done := make(chan bool)
	errors := make(chan error, 100)

	// Concurrent distributions
	for i := 0; i < 20; i++ {
		go func(id int) {
			repoID := fmt.Sprintf("repo-%d", id)
			if err := sm.DistributeRepository(repoID, &govc.Repository{}); err != nil {
				errors <- err
			}
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			_ = sm.CalculateShardMetrics()
			done <- true
		}()
	}

	// Wait for completion
	for i := 0; i < 30; i++ {
		<-done
	}

	// Check for errors
	close(errors)
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}

	// Verify all repositories were distributed
	totalRepos := 0
	for _, shard := range sm.shardMap {
		totalRepos += len(shard.Repositories)
	}
	assert.GreaterOrEqual(t, totalRepos, 20)
}

// TestMigrationQueue tests migration queue operations
func TestMigrationQueue(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	sm := NewShardingManager(cluster)

	// Create migration tasks
	task1 := MigrationTask{
		ID:          "migration-1",
		ShardID:     "shard-1",
		SourceNode:  "node1",
		TargetNode:  "node2",
		State:       MigrationStatePending,
		Repositories: []string{"repo1", "repo2"},
	}

	task2 := MigrationTask{
		ID:          "migration-2",
		ShardID:     "shard-2",
		SourceNode:  "node2",
		TargetNode:  "node3",
		State:       MigrationStatePending,
		Repositories: []string{"repo3"},
	}

	// Add to queue
	sm.migrationQueue.mu.Lock()
	sm.migrationQueue.pending = append(sm.migrationQueue.pending, task1, task2)
	sm.migrationQueue.mu.Unlock()

	// Process migrations
	sm.processPendingMigrations()

	// Check that tasks moved to active
	sm.migrationQueue.mu.RLock()
	assert.Equal(t, 0, len(sm.migrationQueue.pending))
	assert.LessOrEqual(t, len(sm.migrationQueue.active), sm.migrationQueue.maxActive)
	sm.migrationQueue.mu.RUnlock()
}

// TestHashCollisions tests hash collision handling
func TestHashCollisions(t *testing.T) {
	ring := NewConsistentHashRing(1) // Low replicas to increase collision chance

	// Add nodes
	for i := 1; i <= 100; i++ {
		ring.AddNode(fmt.Sprintf("node%d", i))
	}

	// Verify all nodes were added despite potential collisions
	nodeCount := make(map[string]bool)
	for _, node := range ring.nodes {
		nodeCount[node] = true
	}
	assert.Len(t, nodeCount, 100)
}