package cluster

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/caiatech/govc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFailoverManagerCreation tests failover manager creation
func TestFailoverManagerCreation(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	policy := FailoverPolicy{
		AutoFailoverEnabled:   true,
		FailoverTimeout:       30 * time.Second,
		MinHealthyNodes:       2,
		RequireQuorum:         true,
		PreventSplitBrain:     true,
		MaxFailoversPerMinute: 5,
		CooldownPeriod:        5 * time.Minute,
	}

	fm := NewFailoverManager(cluster, policy)
	assert.NotNil(t, fm)
	assert.NotNil(t, fm.healthChecker)
	assert.NotNil(t, fm.recoveryManager)
	assert.NotNil(t, fm.alertManager)
	assert.Equal(t, policy, fm.failoverPolicy)
}

// TestHealthChecker tests health checking functionality
func TestHealthChecker(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.1", 8082)
	cluster.AddNode(node1)
	cluster.AddNode(node2)

	fm := NewFailoverManager(cluster, FailoverPolicy{})
	hc := fm.healthChecker

	// Check all nodes
	hc.checkAllNodes(cluster)

	// Verify health status was recorded
	hc.mu.RLock()
	assert.Len(t, hc.nodeStatus, 2)
	assert.NotNil(t, hc.nodeStatus["node1"])
	assert.NotNil(t, hc.nodeStatus["node2"])
	hc.mu.RUnlock()
}

// TestNodeHealthStatus tests node health status determination
func TestNodeHealthStatus(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	fm := NewFailoverManager(cluster, FailoverPolicy{})
	hc := fm.healthChecker

	// Create test node
	node, _ := createTestNode("test-node", "127.0.0.1", 8080)

	// Test healthy node
	health := hc.checkNodeHealth(node)
	assert.Equal(t, HealthStatusHealthy, health.Status)
	assert.Equal(t, 0, health.ConsecutiveFails)

	// Test offline node
	node.mu.Lock()
	node.State = NodeStateOffline
	node.mu.Unlock()
	health = hc.checkNodeHealth(node)
	assert.Equal(t, HealthStatusDegraded, health.Status)

	// Test consecutive failures
	hc.mu.Lock()
	hc.nodeStatus["test-node"] = &NodeHealth{
		NodeID:           "test-node",
		ConsecutiveFails: hc.unhealthyThreshold - 1,
	}
	hc.mu.Unlock()

	health = hc.checkNodeHealth(node)
	assert.Equal(t, HealthStatusUnhealthy, health.Status)
}

// TestHealthChangeCallbacks tests health change callbacks
func TestHealthChangeCallbacks(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	fm := NewFailoverManager(cluster, FailoverPolicy{})
	hc := fm.healthChecker

	// Track callback invocations
	callbackInvoked := false
	var capturedNodeID string
	var capturedOldStatus, capturedNewStatus HealthStatus

	// Register callback
	hc.RegisterCallback(func(nodeID string, oldStatus, newStatus HealthStatus) {
		callbackInvoked = true
		capturedNodeID = nodeID
		capturedOldStatus = oldStatus
		capturedNewStatus = newStatus
	})

	// Create node and simulate health change
	node, _ := createTestNode("test-node", "127.0.0.1", 8080)
	cluster.AddNode(node)

	// First check - new node
	hc.checkAllNodes(cluster)
	assert.True(t, callbackInvoked)
	assert.Equal(t, "test-node", capturedNodeID)
	assert.Equal(t, HealthStatusUnknown, capturedOldStatus)
	assert.Equal(t, HealthStatusHealthy, capturedNewStatus)

	// Reset
	callbackInvoked = false

	// Make node unhealthy
	hc.mu.Lock()
	hc.nodeStatus["test-node"].ConsecutiveFails = hc.unhealthyThreshold
	hc.nodeStatus["test-node"].Status = HealthStatusUnhealthy
	hc.mu.Unlock()

	// Check again - should trigger callback for status change
	node.mu.Lock()
	node.State = NodeStateOffline
	node.mu.Unlock()

	hc.checkAllNodes(cluster)
	assert.True(t, callbackInvoked)
	assert.Equal(t, HealthStatusUnhealthy, capturedNewStatus)
}

// TestFailoverConditions tests failover condition checking
func TestFailoverConditions(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	for i := 1; i <= 5; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	policy := FailoverPolicy{
		AutoFailoverEnabled: true,
		MinHealthyNodes:     3,
		RequireQuorum:       true,
	}

	fm := NewFailoverManager(cluster, policy)

	// Set up health statuses
	fm.healthChecker.mu.Lock()
	fm.healthChecker.nodeStatus["node1"] = &NodeHealth{NodeID: "node1", Status: HealthStatusHealthy}
	fm.healthChecker.nodeStatus["node2"] = &NodeHealth{NodeID: "node2", Status: HealthStatusHealthy}
	fm.healthChecker.nodeStatus["node3"] = &NodeHealth{NodeID: "node3", Status: HealthStatusHealthy}
	fm.healthChecker.nodeStatus["node4"] = &NodeHealth{NodeID: "node4", Status: HealthStatusUnhealthy}
	fm.healthChecker.nodeStatus["node5"] = &NodeHealth{NodeID: "node5", Status: HealthStatusUnhealthy}
	fm.healthChecker.mu.Unlock()

	// Check conditions - should pass (3 healthy >= minHealthyNodes)
	fm.checkFailoverConditions()

	// Make another node unhealthy
	fm.healthChecker.mu.Lock()
	fm.healthChecker.nodeStatus["node3"].Status = HealthStatusUnhealthy
	fm.healthChecker.mu.Unlock()

	// Check conditions - should fail (2 healthy < minHealthyNodes)
	// This should trigger cluster degraded alert
	fm.checkFailoverConditions()
}

// TestNodeFailureHandling tests handling of node failures
func TestNodeFailureHandling(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 2,
	}, t.TempDir())
	require.NoError(t, err)

	// Add nodes and shards
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.1", 8082)
	cluster.AddNode(node1)
	cluster.AddNode(node2)

	// Create shard on node1
	shard := &Shard{
		ID:           "shard1",
		PrimaryNode:  "node1",
		ReplicaNodes: []string{"node2"},
		Repositories: map[string]bool{"repo1": true},
		State:        ShardStateActive,
	}
	cluster.Shards["shard1"] = shard

	fm := NewFailoverManager(cluster, FailoverPolicy{
		AutoFailoverEnabled: true,
	})

	// Handle node failure
	fm.handleNodeFailure("node1")

	// Verify failover event was created
	fm.mu.RLock()
	assert.Greater(t, len(fm.eventLog), 0)
	event := fm.eventLog[0]
	assert.Equal(t, FailoverTypeNodeFailure, event.Type)
	assert.Equal(t, "node1", event.SourceNode)
	fm.mu.RUnlock()
}

// TestLeaderElection tests leader election triggering
func TestLeaderElection(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.1", 8082)
	node1.IsLeader = true
	node1.State = NodeStateLeader
	cluster.AddNode(node1)
	cluster.AddNode(node2)

	fm := NewFailoverManager(cluster, FailoverPolicy{})

	// Set up health statuses
	fm.healthChecker.mu.Lock()
	fm.healthChecker.nodeStatus["node1"] = &NodeHealth{
		NodeID:       "node1",
		Status:       HealthStatusHealthy,
		ResponseTime: 200 * time.Millisecond,
	}
	fm.healthChecker.nodeStatus["node2"] = &NodeHealth{
		NodeID:       "node2",
		Status:       HealthStatusHealthy,
		ResponseTime: 100 * time.Millisecond,
	}
	fm.healthChecker.mu.Unlock()

	// Trigger leader election
	fm.triggerLeaderElection()

	// Verify election event was created
	fm.mu.RLock()
	var electionEvent *FailoverEvent
	for _, event := range fm.eventLog {
		if event.Type == FailoverTypeLeaderElection {
			electionEvent = &event
			break
		}
	}
	fm.mu.RUnlock()

	assert.NotNil(t, electionEvent)
	assert.Equal(t, "node2", electionEvent.TargetNode) // Node2 has better response time
}

// TestShardMigration tests shard migration during failover
func TestShardMigration(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.1", 8082)
	node3, _ := createTestNode("node3", "127.0.0.1", 8083)
	cluster.AddNode(node1)
	cluster.AddNode(node2)
	cluster.AddNode(node3)

	// Add repositories to node1
	node1.AddRepository("repo1", &govc.Repository{})
	node1.AddRepository("repo2", &govc.Repository{})

	// Create shards
	shard1 := &Shard{
		ID:           "shard1",
		PrimaryNode:  "node1",
		ReplicaNodes: []string{"node2"},
		Repositories: map[string]bool{"repo1": true, "repo2": true},
	}
	cluster.Shards["shard1"] = shard1

	fm := NewFailoverManager(cluster, FailoverPolicy{})

	// Create failover event
	event := &FailoverEvent{
		ID:         "test-failover",
		Type:       FailoverTypeNodeFailure,
		SourceNode: "node1",
		TargetNode: "node3",
		Status:     FailoverStatusActive,
		StartedAt:  time.Now(),
	}

	// Migrate shards
	fm.migrateNodeShards("node1", "node3", event)

	// Verify shard primary was updated
	assert.NotEqual(t, "node1", shard1.PrimaryNode)

	// Verify repositories were migrated
	assert.NotNil(t, node3.GetRepository("repo1"))
	assert.NotNil(t, node3.GetRepository("repo2"))
}

// TestFailoverCooldown tests failover cooldown period
func TestFailoverCooldown(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	
	fm := NewFailoverManager(cluster, FailoverPolicy{
		MaxFailoversPerMinute: 2,
		CooldownPeriod:        1 * time.Minute,
	})

	// Add recent failover events
	now := time.Now()
	fm.eventLog = []FailoverEvent{
		{SourceNode: "node1", StartedAt: now.Add(-30 * time.Second)},
		{SourceNode: "node1", StartedAt: now.Add(-20 * time.Second)},
	}

	// Should be in cooldown
	assert.True(t, fm.isInCooldown("node1"))

	// Different node should not be in cooldown
	assert.False(t, fm.isInCooldown("node2"))

	// Old events should not count
	fm.eventLog = []FailoverEvent{
		{SourceNode: "node1", StartedAt: now.Add(-2 * time.Minute)},
	}
	assert.False(t, fm.isInCooldown("node1"))
}

// TestRecoveryManager tests recovery manager operations
func TestRecoveryManager(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	fm := NewFailoverManager(cluster, FailoverPolicy{})
	rm := fm.recoveryManager

	// Create recovery task
	task := RecoveryTask{
		ID:        "recovery-1",
		NodeID:    "node1",
		Type:      RecoveryTypeNodeRestart,
		Status:    RecoveryStatusPending,
		StartedAt: time.Now(),
	}

	// Queue recovery task
	select {
	case rm.recoveryQueue <- task:
		// Success
	default:
		t.Fatal("Failed to queue recovery task")
	}

	// Execute recovery
	rm.executeRecovery(&task)

	// Verify task completed
	assert.Equal(t, RecoveryStatusCompleted, task.Status)
	assert.Equal(t, 100, task.Progress)
	assert.NotZero(t, task.CompletedAt)
}

// TestAlertManager tests alert management
func TestAlertManager(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	fm := NewFailoverManager(cluster, FailoverPolicy{})
	am := fm.alertManager

	// Create test alert channel
	testChannel := &MockAlertChannel{
		sentAlerts: make([]Alert, 0),
	}
	am.channels = append(am.channels, testChannel)

	// Send alert
	alert := Alert{
		ID:       "alert-1",
		Type:     AlertTypeNodeDown,
		Severity: AlertSeverityCritical,
		Message:  "Node node1 is down",
		Data: map[string]interface{}{
			"node_id": "node1",
		},
		Timestamp: time.Now(),
	}

	am.SendAlert(alert)

	// Give time for async send
	time.Sleep(100 * time.Millisecond)

	// Verify alert was sent
	assert.Len(t, testChannel.sentAlerts, 1)
	assert.Equal(t, alert.ID, testChannel.sentAlerts[0].ID)

	// Verify alert history
	am.mu.RLock()
	assert.Len(t, am.history, 1)
	am.mu.RUnlock()
}

// TestFailoverHistory tests failover event history
func TestFailoverHistory(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	fm := NewFailoverManager(cluster, FailoverPolicy{})

	// Add events
	events := []FailoverEvent{
		{ID: "event-1", Type: FailoverTypeNodeFailure},
		{ID: "event-2", Type: FailoverTypeLeaderElection},
		{ID: "event-3", Type: FailoverTypeShardMigration},
	}

	fm.mu.Lock()
	fm.eventLog = append(fm.eventLog, events...)
	fm.mu.Unlock()

	// Get history
	history := fm.GetFailoverHistory()
	assert.Len(t, history, 3)
	
	// Verify events are copied (not references)
	history[0].ID = "modified"
	assert.Equal(t, "event-1", fm.eventLog[0].ID)
}

// TestClusterHealthMetrics tests cluster health metrics
func TestClusterHealthMetrics(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	fm := NewFailoverManager(cluster, FailoverPolicy{
		AutoFailoverEnabled: true,
	})

	// Set up health statuses
	fm.healthChecker.mu.Lock()
	fm.healthChecker.nodeStatus["node1"] = &NodeHealth{Status: HealthStatusHealthy}
	fm.healthChecker.nodeStatus["node2"] = &NodeHealth{Status: HealthStatusHealthy}
	fm.healthChecker.nodeStatus["node3"] = &NodeHealth{Status: HealthStatusDegraded}
	fm.healthChecker.nodeStatus["node4"] = &NodeHealth{Status: HealthStatusUnhealthy}
	fm.healthChecker.mu.Unlock()

	// Get cluster health
	health := fm.GetClusterHealth()

	assert.Equal(t, 4, health["total_nodes"])
	assert.Equal(t, 2, health["healthy_nodes"])
	assert.Equal(t, 1, health["degraded_nodes"])
	assert.Equal(t, 1, health["unhealthy_nodes"])
	assert.Equal(t, true, health["failover_enabled"])
}

// TestConcurrentFailoverOperations tests concurrent failover operations
func TestConcurrentFailoverOperations(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Add many nodes
	for i := 1; i <= 10; i++ {
		node, _ := createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(node)
	}

	fm := NewFailoverManager(cluster, FailoverPolicy{
		AutoFailoverEnabled: true,
	})

	// Start failover manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	go fm.Start(ctx)

	// Run concurrent operations
	done := make(chan bool)

	// Concurrent health checks
	for i := 0; i < 5; i++ {
		go func() {
			fm.healthChecker.checkAllNodes(cluster)
			done <- true
		}()
	}

	// Concurrent failovers
	for i := 1; i <= 3; i++ {
		go func(nodeID string) {
			fm.handleNodeFailure(nodeID)
			done <- true
		}(fmt.Sprintf("node%d", i))
	}

	// Concurrent metric collection
	for i := 0; i < 3; i++ {
		go func() {
			_ = fm.GetClusterHealth()
			done <- true
		}()
	}

	// Wait for completion
	for i := 0; i < 11; i++ {
		<-done
	}

	// Verify system is still consistent
	history := fm.GetFailoverHistory()
	assert.GreaterOrEqual(t, len(history), 3)
}

// MockAlertChannel is a mock alert channel for testing
type MockAlertChannel struct {
	sentAlerts []Alert
}

func (m *MockAlertChannel) Send(alert Alert) error {
	m.sentAlerts = append(m.sentAlerts, alert)
	return nil
}

func (m *MockAlertChannel) GetType() string {
	return "mock"
}