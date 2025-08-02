package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/caiatech/govc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReplicationManagerCreation tests replication manager creation
func TestReplicationManagerCreation(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	rm := NewReplicationManager(cluster)
	assert.NotNil(t, rm)
	assert.NotNil(t, rm.replicationQueue)
	assert.NotNil(t, rm.activeReplicas)
	assert.Equal(t, 5, rm.maxConcurrent)
	assert.Equal(t, 6, rm.compressionLevel)
}

// TestReplicationTaskCreation tests replication task creation
func TestReplicationTaskCreation(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.1", 8082)
	cluster.AddNode(node1)
	cluster.AddNode(node2)

	// Add repository to source node
	repo := &govc.Repository{}
	node1.AddRepository("repo1", repo)

	rm := NewReplicationManager(cluster)

	// Create replication strategy
	strategy := ReplicationStrategy{
		Type:             ReplicationTypeSync,
		ConsistencyLevel: "strong",
		SyncTimeout:      30 * time.Second,
		RetryPolicy: RetryPolicy{
			MaxRetries: 3,
		},
	}

	// Replicate repository
	err = rm.ReplicateRepository("repo1", "node1", []string{"node2"}, strategy)
	assert.NoError(t, err)

	// Verify task was queued
	select {
	case task := <-rm.replicationQueue:
		assert.Equal(t, "repo1", task.RepositoryID)
		assert.Equal(t, "node1", task.SourceNode)
		assert.Contains(t, task.TargetNodes, "node2")
		assert.Equal(t, ReplicationTypeSync, task.Type)
		assert.NotEmpty(t, task.Checksum)
		// Put it back for other tests
		rm.replicationQueue <- task
	default:
		t.Fatal("No task in replication queue")
	}
}

// TestReplicationStrategies tests different replication strategies
func TestReplicationStrategies(t *testing.T) {
	strategies := []struct {
		name     string
		strategy ReplicationStrategy
	}{
		{
			name: "sync replication",
			strategy: ReplicationStrategy{
				Type:             ReplicationTypeSync,
				ConsistencyLevel: "strong",
				SyncTimeout:      30 * time.Second,
			},
		},
		{
			name: "async replication",
			strategy: ReplicationStrategy{
				Type:             ReplicationTypeAsync,
				ConsistencyLevel: "eventual",
				AsyncBufferSize:  1000,
			},
		},
		{
			name: "incremental replication",
			strategy: ReplicationStrategy{
				Type:             ReplicationTypeIncremental,
				ConsistencyLevel: "eventual",
			},
		},
		{
			name: "full replication",
			strategy: ReplicationStrategy{
				Type:               ReplicationTypeFull,
				ConsistencyLevel:   "strong",
				CompressionEnabled: true,
				EncryptionEnabled:  true,
			},
		},
	}

	for _, tt := range strategies {
		t.Run(tt.name, func(t *testing.T) {
			cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
			node1, _ := createTestNode("node1", "127.0.0.1", 8081)
			node2, _ := createTestNode("node2", "127.0.0.1", 8082)
			cluster.AddNode(node1)
			cluster.AddNode(node2)

			repo := &govc.Repository{}
			node1.AddRepository("repo1", repo)

			rm := NewReplicationManager(cluster)

			err := rm.ReplicateRepository("repo1", "node1", []string{"node2"}, tt.strategy)
			assert.NoError(t, err)

			// Verify task was created with correct type
			select {
			case task := <-rm.replicationQueue:
				assert.Equal(t, tt.strategy.Type, task.Type)
				assert.Equal(t, tt.strategy.ConsistencyLevel, task.Metadata["consistency_level"])
			default:
				t.Fatal("No task in queue")
			}
		})
	}
}

// TestReplicationToNode tests replication to a specific node
func TestReplicationToNode(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.1", 8082)
	cluster.AddNode(node1)
	cluster.AddNode(node2)

	// Add repository to source
	repo := &govc.Repository{}
	node1.AddRepository("repo1", repo)

	rm := NewReplicationManager(cluster)

	// Create task
	data, _ := json.Marshal(repo)
	task := &ReplicationTask{
		ID:           "test-task",
		RepositoryID: "repo1",
		SourceNode:   "node1",
		TargetNodes:  []string{"node2"},
		Data:         data,
		Checksum:     rm.calculateChecksum(data),
		Metadata:     map[string]string{"compression": "false", "encryption": "false"},
	}

	// Replicate to node
	err = rm.replicateToNode(task, "node2")
	assert.NoError(t, err)

	// Verify repository was replicated
	assert.NotNil(t, node2.GetRepository("repo1"))
}

// TestChecksumValidation tests checksum validation
func TestChecksumValidation(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.1", 8082)
	cluster.AddNode(node1)
	cluster.AddNode(node2)

	rm := NewReplicationManager(cluster)

	// Create task with invalid checksum
	task := &ReplicationTask{
		ID:           "test-task",
		RepositoryID: "repo1",
		Data:         []byte("test data"),
		Checksum:     "invalid-checksum",
		Metadata:     map[string]string{"compression": "false", "encryption": "false"},
	}

	// Should fail checksum validation
	err := rm.replicateToNode(task, "node2")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}

// TestConflictResolution tests conflict resolution strategies
func TestConflictResolution(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	rm := NewReplicationManager(cluster)

	existing := &govc.Repository{}
	incoming := &govc.Repository{}
	task := &ReplicationTask{SourceNode: "node1"}

	tests := []struct {
		name     string
		strategy ConflictStrategy
	}{
		{"source wins", ConflictStrategySourceWins},
		{"target wins", ConflictStrategyTargetWins},
		{"last write wins", ConflictStrategyLastWrite},
		{"merge", ConflictStrategyMerge},
		{"manual", ConflictStrategyManual},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolution, err := rm.resolveConflict(existing, incoming, task)
			assert.NoError(t, err)
			assert.NotNil(t, resolution)
			assert.NotZero(t, resolution.ResolvedAt)
		})
	}
}

// TestRetryMechanism tests retry mechanism
func TestRetryMechanism(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	rm := NewReplicationManager(cluster)

	task := &ReplicationTask{
		ID:         "retry-test",
		MaxRetries: 3,
		RetryCount: 0,
	}

	// Schedule retry
	rm.scheduleRetry(task)

	// Verify retry count increased
	assert.Equal(t, 1, task.RetryCount)

	// Test max retries
	task.RetryCount = 3
	rm.scheduleRetry(task)
	assert.Equal(t, ReplicationStateFailed, task.State)
}

// TestReplicationWorker tests replication worker processing
func TestReplicationWorker(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.1", 8082)
	cluster.AddNode(node1)
	cluster.AddNode(node2)

	// Add repository
	repo := &govc.Repository{}
	node1.AddRepository("repo1", repo)

	rm := NewReplicationManager(cluster)

	// Start worker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go rm.replicationWorker(ctx, 0)

	// Create and queue task
	data, _ := json.Marshal(repo)
	task := ReplicationTask{
		ID:           "worker-test",
		Type:         ReplicationTypeSync,
		SourceNode:   "node1",
		TargetNodes:  []string{"node2"},
		RepositoryID: "repo1",
		Data:         data,
		Checksum:     rm.calculateChecksum(data),
		State:        ReplicationStatePending,
		Metadata:     map[string]string{"compression": "false", "encryption": "false"},
	}

	rm.replicationQueue <- task

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify replication completed
	assert.NotNil(t, node2.GetRepository("repo1"))
}

// TestShardReplication tests shard-level replication
func TestShardReplication(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Add nodes
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	node2, _ := createTestNode("node2", "127.0.0.1", 8082)
	cluster.AddNode(node1)
	cluster.AddNode(node2)

	// Create shard with repositories
	shard := &Shard{
		ID:           "shard1",
		PrimaryNode:  "node1",
		Repositories: map[string]bool{"repo1": true, "repo2": true},
	}
	cluster.Shards["shard1"] = shard

	// Add repositories to source node
	node1.AddRepository("repo1", &govc.Repository{})
	node1.AddRepository("repo2", &govc.Repository{})

	rm := NewReplicationManager(cluster)

	strategy := ReplicationStrategy{
		Type: ReplicationTypeFull,
		RetryPolicy: RetryPolicy{
			MaxRetries: 3,
		},
	}

	// Replicate shard
	err = rm.ReplicateShard("shard1", "node1", []string{"node2"}, strategy)
	assert.NoError(t, err)

	// Should have queued tasks for both repositories
	taskCount := 0
	timeout := time.After(100 * time.Millisecond)

Loop:
	for {
		select {
		case <-rm.replicationQueue:
			taskCount++
			if taskCount == 2 {
				break Loop
			}
		case <-timeout:
			break Loop
		}
	}

	assert.Equal(t, 2, taskCount)
}

// TestReplicationMetrics tests metrics collection
func TestReplicationMetrics(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	rm := NewReplicationManager(cluster)

	// Add active replications
	rm.mu.Lock()
	rm.activeReplicas["task1"] = &ReplicationTask{ID: "task1"}
	rm.activeReplicas["task2"] = &ReplicationTask{ID: "task2"}
	rm.mu.Unlock()

	// Get metrics
	metrics := rm.GetReplicationMetrics()
	assert.Equal(t, 2, metrics.ActiveReplications)
	assert.NotNil(t, metrics.ReplicationsByNode)
	assert.NotNil(t, metrics.ErrorsByType)
}

// TestActiveReplications tests getting active replications
func TestActiveReplications(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	rm := NewReplicationManager(cluster)

	// Add active tasks
	tasks := []ReplicationTask{
		{ID: "task1", RepositoryID: "repo1", State: ReplicationStateActive},
		{ID: "task2", RepositoryID: "repo2", State: ReplicationStateActive},
	}

	rm.mu.Lock()
	for i := range tasks {
		rm.activeReplicas[tasks[i].ID] = &tasks[i]
	}
	rm.mu.Unlock()

	// Get active replications
	active := rm.GetActiveReplications()
	assert.Len(t, active, 2)
}

// TestSerializationDeserialization tests repository serialization
func TestSerializationDeserialization(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	rm := NewReplicationManager(cluster)

	// Create test repository
	repo := &govc.Repository{}

	// Serialize
	data, err := rm.serializeRepository(repo)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	// Deserialize
	deserialized, err := rm.deserializeRepository(data)
	assert.NoError(t, err)
	assert.NotNil(t, deserialized)
}

// TestConcurrentReplication tests concurrent replication operations
func TestConcurrentReplication(t *testing.T) {
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)

	// Add many nodes
	nodes := make([]*Node, 10)
	for i := 0; i < 10; i++ {
		nodes[i], _ = createTestNode(fmt.Sprintf("node%d", i), "127.0.0.1", 8080+i)
		cluster.AddNode(nodes[i])
	}

	rm := NewReplicationManager(cluster)

	// Start replication manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = rm.Start(ctx)
	require.NoError(t, err)

	// Add repositories to source nodes
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			repoID := fmt.Sprintf("repo-%d-%d", i, j)
			nodes[i].AddRepository(repoID, &govc.Repository{})
		}
	}

	// Concurrent replication requests
	done := make(chan bool)
	errors := make(chan error, 100)

	strategy := ReplicationStrategy{
		Type: ReplicationTypeAsync,
		RetryPolicy: RetryPolicy{
			MaxRetries: 3,
		},
	}

	for i := 0; i < 5; i++ {
		go func(nodeIdx int) {
			for j := 0; j < 5; j++ {
				repoID := fmt.Sprintf("repo-%d-%d", nodeIdx, j)
				targetNodes := []string{
					fmt.Sprintf("node%d", (nodeIdx+1)%10),
					fmt.Sprintf("node%d", (nodeIdx+2)%10),
				}

				if err := rm.ReplicateRepository(repoID, fmt.Sprintf("node%d", nodeIdx), targetNodes, strategy); err != nil {
					errors <- err
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	// Check for errors
	close(errors)
	for err := range errors {
		t.Errorf("Replication error: %v", err)
	}

	// Give time for some replications to complete
	time.Sleep(500 * time.Millisecond)

	// Verify some replications completed
	completed := 0
	for i := 5; i < 10; i++ {
		repos := nodes[i].GetRepositories()
		completed += len(repos)
	}
	assert.Greater(t, completed, 0)
}

// TestReplicationQueueOverflow tests queue overflow handling
func TestReplicationQueueOverflow(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	rm := NewReplicationManager(cluster)

	// Fill the queue
	for i := 0; i < 500; i++ {
		task := ReplicationTask{ID: fmt.Sprintf("task-%d", i)}
		select {
		case rm.replicationQueue <- task:
			// Success
		default:
			// Queue full - expected
		}
	}

	// Try to add one more - should fail
	node1, _ := createTestNode("node1", "127.0.0.1", 8081)
	cluster.AddNode(node1)

	err := rm.ReplicateRepository("repo1", "node1", []string{"node2"}, ReplicationStrategy{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue is full")
}

// TestReplicationHealthMonitoring tests health monitoring
func TestReplicationHealthMonitoring(t *testing.T) {
	cluster, _ := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	rm := NewReplicationManager(cluster)

	// Add stuck replication
	oldTask := &ReplicationTask{
		ID:        "stuck-task",
		StartedAt: time.Now().Add(-15 * time.Minute),
	}

	rm.mu.Lock()
	rm.activeReplicas["stuck-task"] = oldTask
	rm.mu.Unlock()

	// Check health - should log warning
	rm.checkReplicationHealth()

	// Fill queue near capacity
	for i := 0; i < 450; i++ {
		task := ReplicationTask{ID: fmt.Sprintf("task-%d", i)}
		rm.replicationQueue <- task
	}

	// Check health - should log queue warning
	rm.checkReplicationHealth()
}
