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

// TestNewNode tests node creation
func TestNewNode(t *testing.T) {
	tests := []struct {
		name     string
		config   NodeConfig
		wantErr  bool
	}{
		{
			name: "valid node creation",
			config: NodeConfig{
				ID:      "node1",
				Address: "127.0.0.1",
				Port:    8080,
				DataDir: "/tmp/test-node1",
			},
			wantErr: false,
		},
		{
			name: "empty id - should generate",
			config: NodeConfig{
				Address: "127.0.0.1",
				Port:    8080,
				DataDir: "/tmp/test-node2",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := NewNode(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, node)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, node)
				if tt.config.ID != "" {
					assert.Equal(t, tt.config.ID, node.ID)
				}
				assert.Equal(t, tt.config.Address, node.Address)
				assert.Equal(t, tt.config.Port, node.Port)
				assert.Equal(t, NodeStateFollower, node.State)
				assert.False(t, node.IsLeader)
				assert.NotNil(t, node.repositories)
				assert.NotNil(t, node.raftState)
			}
		})
	}
}

// TestNodeStart tests node startup
func TestNodeStart(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start node in background
	done := make(chan error, 1)
	go func() {
		done <- node.Start(ctx)
	}()

	// Give node time to start
	time.Sleep(100 * time.Millisecond)

	// Verify node is running
	assert.NotEqual(t, NodeStateOffline, node.GetState())

	// Stop node
	cancel()
	
	// Wait for shutdown
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Node failed to stop in time")
	}
}

// TestNodeStateTransitions tests node state transitions
func TestNodeStateTransitions(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Test initial state
	assert.Equal(t, NodeStateFollower, node.State)
	assert.False(t, node.IsLeader)

	// Test transition to candidate
	node.mu.Lock()
	node.State = NodeStateCandidate
	node.mu.Unlock()
	assert.Equal(t, NodeStateCandidate, node.GetState())

	// Test transition to leader
	node.mu.Lock()
	node.State = NodeStateLeader
	node.IsLeader = true
	node.LeaderID = node.ID
	node.mu.Unlock()
	assert.Equal(t, NodeStateLeader, node.GetState())
	assert.True(t, node.IsLeaderNode())

	// Test transition back to follower
	node.mu.Lock()
	node.State = NodeStateFollower
	node.IsLeader = false
	node.LeaderID = "other-node"
	node.mu.Unlock()
	assert.Equal(t, NodeStateFollower, node.GetState())
	assert.False(t, node.IsLeaderNode())
}

// TestNodeAddRepository tests adding repositories to a node
func TestNodeAddRepository(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Create mock repository
	repo := &govc.Repository{}
	repoID := "test-repo"

	// Add repository
	err = node.AddRepository(repoID, repo)
	assert.NoError(t, err)

	// Verify repository was added
	node.mu.RLock()
	storedRepo, exists := node.repositories[repoID]
	node.mu.RUnlock()
	
	assert.True(t, exists)
	assert.Equal(t, repo, storedRepo)

	// Test duplicate add
	err = node.AddRepository(repoID, repo)
	assert.Error(t, err)
}

// TestNodeRemoveRepository tests removing repositories from a node
func TestNodeRemoveRepository(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Add repository first
	repo := &govc.Repository{}
	repoID := "test-repo"
	err = node.AddRepository(repoID, repo)
	require.NoError(t, err)

	// Remove repository
	err = node.RemoveRepository(repoID)
	assert.NoError(t, err)

	// Verify repository was removed
	node.mu.RLock()
	_, exists := node.repositories[repoID]
	node.mu.RUnlock()
	assert.False(t, exists)

	// Test removing non-existent repository
	err = node.RemoveRepository("non-existent")
	assert.Error(t, err)
}

// TestNodeGetRepository tests retrieving repositories from a node
func TestNodeGetRepository(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Add repository
	repo := &govc.Repository{}
	repoID := "test-repo"
	err = node.AddRepository(repoID, repo)
	require.NoError(t, err)

	// Get repository
	retrievedRepo := node.GetRepository(repoID)
	assert.NotNil(t, retrievedRepo)
	assert.Equal(t, repo, retrievedRepo)

	// Get non-existent repository
	retrievedRepo = node.GetRepository("non-existent")
	assert.Nil(t, retrievedRepo)
}

// TestNodeGetRepositories tests retrieving all repositories from a node
func TestNodeGetRepositories(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Add multiple repositories
	repo1 := &govc.Repository{}
	repo2 := &govc.Repository{}
	
	err = node.AddRepository("repo1", repo1)
	require.NoError(t, err)
	err = node.AddRepository("repo2", repo2)
	require.NoError(t, err)

	// Get all repositories
	repos := node.GetRepositories()
	assert.Len(t, repos, 2)
	assert.Contains(t, repos, "repo1")
	assert.Contains(t, repos, "repo2")
}

// TestNodeUpdateStats tests node statistics updates
func TestNodeUpdateStats(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Initial stats
	assert.Equal(t, 0.0, node.Stats.CPUUsage)
	assert.Equal(t, 0.0, node.Stats.MemoryUsage)
	assert.Equal(t, 0.0, node.Stats.DiskUsage)
	assert.Equal(t, int64(0), node.Stats.RepositoryCount)

	// Update stats
	node.UpdateStats()

	// Stats should be updated (exact values depend on system)
	assert.GreaterOrEqual(t, node.Stats.CPUUsage, 0.0)
	assert.GreaterOrEqual(t, node.Stats.MemoryUsage, 0.0)
	assert.GreaterOrEqual(t, node.Stats.DiskUsage, 0.0)
	assert.NotZero(t, node.Stats.LastUpdate)
}

// TestNodeIsHealthy tests node health check
func TestNodeIsHealthy(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Node should be healthy initially
	assert.True(t, node.IsHealthy())

	// Test unhealthy CPU
	node.mu.Lock()
	node.Stats.CPUUsage = 95.0
	node.mu.Unlock()
	assert.False(t, node.IsHealthy())

	// Test unhealthy memory
	node.mu.Lock()
	node.Stats.CPUUsage = 50.0
	node.Stats.MemoryUsage = 95.0
	node.mu.Unlock()
	assert.False(t, node.IsHealthy())

	// Test unhealthy disk
	node.mu.Lock()
	node.Stats.MemoryUsage = 50.0
	node.Stats.DiskUsage = 95.0
	node.mu.Unlock()
	assert.False(t, node.IsHealthy())

	// Test offline state
	node.mu.Lock()
	node.Stats.DiskUsage = 50.0
	node.State = NodeStateOffline
	node.mu.Unlock()
	assert.False(t, node.IsHealthy())
}

// TestNodeCapacity tests node capacity calculation
func TestNodeCapacity(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Add repositories
	err = node.AddRepository("repo1", &govc.Repository{})
	require.NoError(t, err)
	err = node.AddRepository("repo2", &govc.Repository{})
	require.NoError(t, err)

	// Update stats
	node.UpdateStats()

	// Get capacity
	capacity := node.GetCapacity()
	assert.Equal(t, int64(2), capacity.RepositoryCount)
	assert.Equal(t, int64(100), capacity.MaxRepositories)
	assert.Equal(t, 50.0, capacity.AvailableCapacity) // 100% - 50% (avg of CPU, mem, disk)
}

// TestNodeMetadata tests node metadata operations
func TestNodeMetadata(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Test initial metadata
	assert.Empty(t, node.Metadata)

	// Add metadata
	node.mu.Lock()
	node.Metadata["region"] = "us-west"
	node.Metadata["zone"] = "us-west-1a"
	node.mu.Unlock()

	// Verify metadata
	node.mu.RLock()
	assert.Equal(t, "us-west", node.Metadata["region"])
	assert.Equal(t, "us-west-1a", node.Metadata["zone"])
	node.mu.RUnlock()
}

// TestConcurrentNodeOperations tests concurrent access to node
func TestConcurrentNodeOperations(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Run concurrent operations
	done := make(chan bool)
	errors := make(chan error, 100)

	// Concurrent adds
	for i := 0; i < 10; i++ {
		go func(id int) {
			repoID := fmt.Sprintf("repo-%d", id)
			if err := node.AddRepository(repoID, &govc.Repository{}); err != nil {
				errors <- err
			}
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			_ = node.GetRepositories()
			done <- true
		}()
	}

	// Concurrent stats updates
	for i := 0; i < 5; i++ {
		go func() {
			node.UpdateStats()
			done <- true
		}()
	}

	// Wait for all operations to complete
	for i := 0; i < 25; i++ {
		<-done
	}

	// Check for errors
	close(errors)
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}

	// Verify all repositories were added
	repos := node.GetRepositories()
	assert.Len(t, repos, 10)
}