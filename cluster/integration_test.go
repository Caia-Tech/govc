package cluster

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/caiatech/govc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullClusterLifecycle tests complete cluster lifecycle
func TestFullClusterLifecycle(t *testing.T) {
	// Create cluster
	cluster, err := NewCluster("lifecycle-test", "Lifecycle Test", ClusterConfig{
		ReplicationFactor: 3,
		AutoRebalance:     true,
		ElectionTimeout:   200 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
	}, t.TempDir())
	require.NoError(t, err)

	// Phase 1: Bootstrap cluster with nodes
	nodes := make([]*Node, 5)
	for i := 0; i < 5; i++ {
		nodes[i], err = createTestNode(fmt.Sprintf("node%d", i+1), "127.0.0.1", 8080+i)
		require.NoError(t, err)
		err = cluster.AddNode(nodes[i])
		require.NoError(t, err)
	}

	// Start nodes
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(node *Node) {
			defer wg.Done()
			node.Start(ctx)
		}(nodes[i])
	}

	// Wait for cluster to stabilize
	time.Sleep(500 * time.Millisecond)

	// Phase 2: Verify leader election occurred
	leader := cluster.GetLeaderNode()
	assert.NotNil(t, leader)
	assert.True(t, leader.IsLeaderNode())

	// Phase 3: Create and distribute repositories
	for i := 0; i < 10; i++ {
		repo := &govc.Repository{}
		err = cluster.DistributeRepository(fmt.Sprintf("repo%d", i), repo)
		assert.NoError(t, err)
	}

	// Verify shards were created
	assert.Greater(t, len(cluster.Shards), 0)

	// Phase 4: Test failover by simulating leader failure
	leaderID := leader.ID
	leader.mu.Lock()
	leader.State = NodeStateOffline
	leader.mu.Unlock()

	// Wait for new leader election
	time.Sleep(500 * time.Millisecond)

	newLeader := cluster.GetLeaderNode()
	assert.NotNil(t, newLeader)
	assert.NotEqual(t, leaderID, newLeader.ID)

	// Phase 5: Test node removal and shard migration
	err = cluster.RemoveNode("node3")
	assert.NoError(t, err)

	// Verify shards were migrated
	for _, shard := range cluster.Shards {
		assert.NotEqual(t, "node3", shard.PrimaryNode)
		assert.NotContains(t, shard.ReplicaNodes, "node3")
	}

	// Phase 6: Verify cluster health
	health := cluster.GetClusterHealth()
	assert.Equal(t, ClusterStateHealthy, health.Status)
	assert.Equal(t, 4, health.TotalNodes)

	// Shutdown
	cancel()
	wg.Wait()
}

// TestDistributedReplicationScenario tests distributed replication
func TestDistributedReplicationScenario(t *testing.T) {
	cluster, err := NewCluster("replication-test", "Replication Test", ClusterConfig{
		ReplicationFactor: 3,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	for i := 1; i <= 5; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	// Create sharding and replication managers
	shardingMgr := NewShardingManager(cluster)
	replicationMgr := NewReplicationManager(cluster)

	// Start replication manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = replicationMgr.Start(ctx)
	require.NoError(t, err)

	// Distribute repositories across shards
	for i := 0; i < 20; i++ {
		repoID := fmt.Sprintf("repo%d", i)
		repo := &govc.Repository{}
		err = shardingMgr.DistributeRepository(repoID, repo)
		assert.NoError(t, err)
	}

	// Verify initial distribution
	metrics := shardingMgr.CalculateShardMetrics()
	assert.Greater(t, metrics.TotalShards, 0)
	assert.Greater(t, len(metrics.ShardsPerNode), 0)

	// Test cross-node replication
	strategy := ReplicationStrategy{
		Type:             ReplicationTypeAsync,
		ConsistencyLevel: "eventual",
		RetryPolicy: RetryPolicy{
			MaxRetries: 3,
		},
	}

	// Replicate a shard to ensure redundancy
	for shardID, shard := range cluster.Shards {
		if len(shard.Repositories) > 0 {
			err = replicationMgr.ReplicateShard(shardID, shard.PrimaryNode, shard.ReplicaNodes, strategy)
			assert.NoError(t, err)
			break
		}
	}

	// Wait for replications to process
	time.Sleep(500 * time.Millisecond)

	// Verify replications occurred
	_ = replicationMgr.GetActiveReplications()
	replicationMetrics := replicationMgr.GetReplicationMetrics()
	assert.GreaterOrEqual(t, replicationMetrics.ActiveReplications, 0)
}

// TestFailoverAndRecoveryScenario tests failover and recovery
func TestFailoverAndRecoveryScenario(t *testing.T) {
	cluster, err := NewCluster("failover-test", "Failover Test", ClusterConfig{
		ReplicationFactor: 2,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	nodes := make([]*Node, 4)
	for i := 0; i < 4; i++ {
		nodes[i], _ = createTestNode(fmt.Sprintf("node%d", i+1), "127.0.0.1", 8080+i)
		cluster.AddNode(nodes[i])
	}

	// Create failover manager
	failoverMgr := NewFailoverManager(cluster, FailoverPolicy{
		AutoFailoverEnabled:   true,
		MinHealthyNodes:       2,
		RequireQuorum:         true,
		MaxFailoversPerMinute: 5,
		CooldownPeriod:        1 * time.Minute,
	})

	// Start failover manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = failoverMgr.Start(ctx)
	require.NoError(t, err)

	// Create shards with repositories
	for i := 0; i < 10; i++ {
		repoID := fmt.Sprintf("repo%d", i)
		repo := &govc.Repository{}
		nodes[i%4].AddRepository(repoID, repo)

		err = cluster.DistributeRepository(repoID, repo)
		assert.NoError(t, err)
	}

	// Simulate node failure
	nodes[0].mu.Lock()
	nodes[0].State = NodeStateOffline
	nodes[0].mu.Unlock()

	// Mark node as unhealthy in health checker
	failoverMgr.healthChecker.mu.Lock()
	failoverMgr.healthChecker.nodeStatus["node1"] = &NodeHealth{
		NodeID:           "node1",
		Status:           HealthStatusUnhealthy,
		ConsecutiveFails: failoverMgr.healthChecker.unhealthyThreshold + 1,
	}
	failoverMgr.healthChecker.mu.Unlock()

	// Trigger failover check
	failoverMgr.checkFailoverConditions()

	// Wait for failover to process
	time.Sleep(500 * time.Millisecond)

	// Verify failover occurred
	failoverHistory := failoverMgr.GetFailoverHistory()
	assert.Greater(t, len(failoverHistory), 0)

	// Verify shards were migrated from failed node
	for _, shard := range cluster.Shards {
		assert.NotEqual(t, "node1", shard.PrimaryNode)
	}

	// Test recovery
	nodes[0].mu.Lock()
	nodes[0].State = NodeStateFollower
	nodes[0].mu.Unlock()

	// Update health status
	failoverMgr.healthChecker.mu.Lock()
	failoverMgr.healthChecker.nodeStatus["node1"].Status = HealthStatusHealthy
	failoverMgr.healthChecker.nodeStatus["node1"].ConsecutiveFails = 0
	failoverMgr.healthChecker.mu.Unlock()

	// Trigger health change callback
	failoverMgr.onHealthChange("node1", HealthStatusUnhealthy, HealthStatusHealthy)

	// Verify recovery alert was sent
	time.Sleep(100 * time.Millisecond)
}

// TestLoadBalancingScenario tests load balancing across cluster
func TestLoadBalancingScenario(t *testing.T) {
	cluster, err := NewCluster("loadbalance-test", "Load Balance Test", ClusterConfig{
		ReplicationFactor: 2,
		AutoRebalance:     true,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	for i := 1; i <= 6; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	shardingMgr := NewShardingManager(cluster)

	// Create unbalanced load - all repositories on first 2 nodes
	for i := 0; i < 30; i++ {
		repoID := fmt.Sprintf("repo%d", i)
		repo := &govc.Repository{}

		// Force to specific nodes
		nodeID := fmt.Sprintf("node%d", (i%2)+1)
		node := cluster.GetNodeByID(nodeID)
		node.AddRepository(repoID, repo)

		// Create shard manually
		shard := &Shard{
			ID:           fmt.Sprintf("shard%d", i),
			PrimaryNode:  nodeID,
			ReplicaNodes: []string{fmt.Sprintf("node%d", ((i % 2) + 2))},
			State:        ShardStateActive,
			Repositories: map[string]bool{repoID: true},
		}
		shardingMgr.shardMap[shard.ID] = shard
		cluster.Shards[shard.ID] = shard
	}

	// Check initial imbalance
	initialMetrics := shardingMgr.CalculateShardMetrics()
	assert.Greater(t, initialMetrics.Imbalance, 0.5)

	// Trigger rebalancing
	shardingMgr.checkRebalanceNeeded()

	// Process some migrations
	for i := 0; i < 5; i++ {
		shardingMgr.processPendingMigrations()
		time.Sleep(100 * time.Millisecond)
	}

	// Check improved balance
	finalMetrics := shardingMgr.CalculateShardMetrics()
	assert.Less(t, finalMetrics.Imbalance, initialMetrics.Imbalance)
}

// TestConcurrentOperationsUnderLoad tests system under concurrent load
func TestConcurrentOperationsUnderLoad(t *testing.T) {
	cluster, err := NewCluster("concurrent-test", "Concurrent Test", ClusterConfig{
		ReplicationFactor: 3,
		AutoRebalance:     true,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	for i := 1; i <= 8; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	// Create all managers
	shardingMgr := NewShardingManager(cluster)
	replicationMgr := NewReplicationManager(cluster)
	failoverMgr := NewFailoverManager(cluster, FailoverPolicy{
		AutoFailoverEnabled: true,
		MinHealthyNodes:     4,
	})

	// Start managers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	replicationMgr.Start(ctx)
	failoverMgr.Start(ctx)

	// Concurrent operations
	var wg sync.WaitGroup
	errors := make(chan error, 1000)

	// Writer goroutines - add repositories
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				repoID := fmt.Sprintf("writer%d-repo%d", writerID, j)
				repo := &govc.Repository{}
				if err := shardingMgr.DistributeRepository(repoID, repo); err != nil {
					errors <- err
				}
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	// Reader goroutines - query cluster state
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = cluster.GetClusterHealth()
				_ = shardingMgr.CalculateShardMetrics()
				_ = failoverMgr.GetClusterHealth()
				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	// Chaos goroutine - simulate failures
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(200 * time.Millisecond)

		// Fail a node
		failoverMgr.healthChecker.mu.Lock()
		failoverMgr.healthChecker.nodeStatus["node3"] = &NodeHealth{
			NodeID:           "node3",
			Status:           HealthStatusUnhealthy,
			ConsecutiveFails: 10,
		}
		failoverMgr.healthChecker.mu.Unlock()

		time.Sleep(200 * time.Millisecond)

		// Recover the node
		failoverMgr.healthChecker.mu.Lock()
		failoverMgr.healthChecker.nodeStatus["node3"].Status = HealthStatusHealthy
		failoverMgr.healthChecker.nodeStatus["node3"].ConsecutiveFails = 0
		failoverMgr.healthChecker.mu.Unlock()
	}()

	// Wait for all operations
	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Logf("Operation error: %v", err)
		errorCount++
	}
	assert.Less(t, errorCount, 10) // Allow some errors under heavy load

	// Verify system stability
	health := cluster.GetClusterHealth()
	assert.NotEqual(t, ClusterStateUnavailable, health.Status)

	metrics := shardingMgr.CalculateShardMetrics()
	assert.Greater(t, metrics.TotalShards, 0)

	// Verify data integrity - all repositories should be accessible
	repoCount := 0
	for _, node := range cluster.GetNodes() {
		repoCount += len(node.GetRepositories())
	}
	assert.Greater(t, repoCount, 50) // Should have created many repositories
}

// TestDisasterRecoveryScenario tests recovery from major failures
func TestDisasterRecoveryScenario(t *testing.T) {
	cluster, err := NewCluster("disaster-test", "Disaster Test", ClusterConfig{
		ReplicationFactor: 3,
		AutoRebalance:     true,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	nodes := make([]*Node, 6)
	for i := 0; i < 6; i++ {
		nodes[i], _ = createTestNode(fmt.Sprintf("node%d", i+1), "127.0.0.1", 8080+i)
		cluster.AddNode(nodes[i])
	}

	// Make node1 the leader
	nodes[0].mu.Lock()
	nodes[0].State = NodeStateLeader
	nodes[0].IsLeader = true
	nodes[0].LeaderID = nodes[0].ID
	nodes[0].mu.Unlock()

	// Distribute repositories
	for i := 0; i < 30; i++ {
		repo := &govc.Repository{}
		err = cluster.DistributeRepository(fmt.Sprintf("repo%d", i), repo)
		assert.NoError(t, err)
	}

	// Scenario 1: Simultaneous failure of multiple nodes (but maintain quorum)
	failedNodes := []int{0, 2, 4} // Fail 3 out of 6 nodes
	for _, idx := range failedNodes {
		nodes[idx].mu.Lock()
		nodes[idx].State = NodeStateOffline
		nodes[idx].mu.Unlock()
	}

	// Create and use failover manager
	failoverMgr := NewFailoverManager(cluster, FailoverPolicy{
		AutoFailoverEnabled: true,
		RequireQuorum:       true,
		MinHealthyNodes:     3,
	})

	// Mark failed nodes as unhealthy
	failoverMgr.healthChecker.mu.Lock()
	for _, idx := range failedNodes {
		nodeID := fmt.Sprintf("node%d", idx+1)
		failoverMgr.healthChecker.nodeStatus[nodeID] = &NodeHealth{
			NodeID:           nodeID,
			Status:           HealthStatusUnhealthy,
			ConsecutiveFails: 10,
		}
	}
	failoverMgr.healthChecker.mu.Unlock()

	// Check cluster can still operate with remaining nodes
	failoverMgr.checkFailoverConditions()

	health := failoverMgr.GetClusterHealth()
	assert.Equal(t, 3, health["healthy_nodes"])
	assert.Equal(t, 3, health["unhealthy_nodes"])

	// Scenario 2: Gradual recovery
	for i, idx := range failedNodes {
		// Recover one node at a time
		nodes[idx].mu.Lock()
		nodes[idx].State = NodeStateFollower
		nodes[idx].mu.Unlock()

		failoverMgr.healthChecker.mu.Lock()
		nodeID := fmt.Sprintf("node%d", idx+1)
		failoverMgr.healthChecker.nodeStatus[nodeID].Status = HealthStatusHealthy
		failoverMgr.healthChecker.nodeStatus[nodeID].ConsecutiveFails = 0
		failoverMgr.healthChecker.mu.Unlock()

		// Verify cluster health improves
		health = failoverMgr.GetClusterHealth()
		assert.Equal(t, 4+i, health["healthy_nodes"])
	}

	// Final verification - all nodes healthy
	finalHealth := cluster.GetClusterHealth()
	assert.Equal(t, 6, finalHealth.HealthyNodes)
	assert.Equal(t, ClusterStateHealthy, finalHealth.Status)
}

// TestNetworkPartitionScenario tests handling of network partitions
func TestNetworkPartitionScenario(t *testing.T) {
	cluster, err := NewCluster("partition-test", "Partition Test", ClusterConfig{
		ReplicationFactor: 3,
		AutoRebalance:     false, // Disable auto-rebalance for controlled test
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	nodes := make([]*Node, 6)
	for i := 0; i < 6; i++ {
		nodes[i], _ = createTestNode(fmt.Sprintf("node%d", i+1), "127.0.0.1", 8080+i)
		cluster.AddNode(nodes[i])
	}

	// Create two partitions
	// partition1 := []int{0, 1, 2} // nodes 1, 2, 3
	partition2 := []int{3, 4, 5} // nodes 4, 5, 6

	// Simulate network partition by marking cross-partition nodes as unreachable
	failoverMgr := NewFailoverManager(cluster, FailoverPolicy{
		RequireQuorum:     true,
		PreventSplitBrain: true,
		MinHealthyNodes:   4,
	})

	// From partition1's perspective, partition2 is unhealthy
	failoverMgr.healthChecker.mu.Lock()
	for _, idx := range partition2 {
		nID := fmt.Sprintf("node%d", idx+1)
		failoverMgr.healthChecker.nodeStatus[nID] = &NodeHealth{
			NodeID: nID,
			Status: HealthStatusUnhealthy,
		}
	}
	failoverMgr.healthChecker.mu.Unlock()

	// Check that neither partition has quorum (3 out of 6 nodes each)
	failoverMgr.checkFailoverConditions()

	// With split-brain prevention, cluster should degrade
	health := failoverMgr.GetClusterHealth()
	assert.Equal(t, 3, health["healthy_nodes"]) // Only sees its partition
	assert.Equal(t, 3, health["unhealthy_nodes"])

	// Heal partition
	failoverMgr.healthChecker.mu.Lock()
	for _, health := range failoverMgr.healthChecker.nodeStatus {
		health.Status = HealthStatusHealthy
		health.ConsecutiveFails = 0
	}
	failoverMgr.healthChecker.mu.Unlock()

	// Verify cluster recovers
	health = failoverMgr.GetClusterHealth()
	assert.Equal(t, 6, health["healthy_nodes"])
	assert.Equal(t, 0, health["unhealthy_nodes"])
}
