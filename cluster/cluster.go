package cluster

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/caiatech/govc"
)

// Cluster represents a govc distributed cluster
type Cluster struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Nodes     map[string]*Node  `json:"nodes"`
	Shards    map[string]*Shard `json:"shards"`
	Config    ClusterConfig     `json:"config"`
	State     ClusterState      `json:"state"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`

	// Internal state
	localNode *Node        `json:"-"`
	dataDir   string       `json:"-"`
	mu        sync.RWMutex `json:"-"`
}

// ClusterConfig contains cluster-wide configuration
type ClusterConfig struct {
	ReplicationFactor int           `yaml:"replication_factor"`
	ShardSize         int           `yaml:"shard_size"`
	ElectionTimeout   time.Duration `yaml:"election_timeout"`
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
	MaxLogEntries     int           `yaml:"max_log_entries"`
	SnapshotThreshold int           `yaml:"snapshot_threshold"`
	AutoRebalance     bool          `yaml:"auto_rebalance"`
	ConsistencyLevel  string        `yaml:"consistency_level"`
}

// ClusterState represents the overall state of the cluster
type ClusterState string

const (
	ClusterStateHealthy     ClusterState = "healthy"
	ClusterStateDegraded    ClusterState = "degraded"
	ClusterStateUnavailable ClusterState = "unavailable"
	ClusterStateRebalancing ClusterState = "rebalancing"
	ClusterStateRecovering  ClusterState = "recovering"
)

// clusterState represents the serializable cluster state
type clusterState struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Config    ClusterConfig          `json:"config"`
	State     ClusterState           `json:"state"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Nodes     map[string]*nodeState  `json:"nodes"`
	Shards    map[string]*shardState `json:"shards"`
}

// nodeState represents the serializable node state
type nodeState struct {
	ID       string    `json:"id"`
	Address  string    `json:"address"`
	State    NodeState `json:"state"`
	LastSeen time.Time `json:"last_seen"`
}

// shardState represents the serializable shard state
type shardState struct {
	ID           string        `json:"id"`
	KeyRange     ShardKeyRange `json:"key_range"`
	PrimaryNode  string        `json:"primary_node"`
	ReplicaNodes []string      `json:"replica_nodes"`
	State        ShardState    `json:"state"`
}

// Shard represents a data shard in the cluster
type Shard struct {
	ID           string          `json:"id"`
	KeyRange     ShardKeyRange   `json:"key_range"`
	PrimaryNode  string          `json:"primary_node"`
	ReplicaNodes []string        `json:"replica_nodes"`
	State        ShardState      `json:"state"`
	Repositories map[string]bool `json:"repositories"`
	Size         int64           `json:"size"`
	LastAccessed time.Time       `json:"last_accessed"`
	CreatedAt    time.Time       `json:"created_at"`
}

// ShardKeyRange defines the key range for a shard
type ShardKeyRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// ShardState represents the state of a shard
type ShardState string

const (
	ShardStateActive      ShardState = "active"
	ShardStateRebalancing ShardState = "rebalancing"
	ShardStateOffline     ShardState = "offline"
	ShardStateMigrating   ShardState = "migrating"
)

// ClusterEvent represents events that occur in the cluster
type ClusterEvent struct {
	Type      ClusterEventType       `json:"type"`
	NodeID    string                 `json:"node_id"`
	ShardID   string                 `json:"shard_id"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// ClusterEventType defines types of cluster events
type ClusterEventType string

const (
	EventNodeJoined        ClusterEventType = "node_joined"
	EventNodeLeft          ClusterEventType = "node_left"
	EventNodeFailed        ClusterEventType = "node_failed"
	EventShardMoved        ClusterEventType = "shard_moved"
	EventRebalanceStarted  ClusterEventType = "rebalance_started"
	EventRebalanceComplete ClusterEventType = "rebalance_complete"
	EventLeaderElected     ClusterEventType = "leader_elected"
)

// NewCluster creates a new cluster
func NewCluster(id, name string, config ClusterConfig, dataDir string) (*Cluster, error) {
	if config.ReplicationFactor == 0 {
		config.ReplicationFactor = 3
	}
	if config.ShardSize == 0 {
		config.ShardSize = 1000 // repositories per shard
	}
	if config.ElectionTimeout == 0 {
		config.ElectionTimeout = 150 * time.Millisecond
	}
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 50 * time.Millisecond
	}
	if config.ConsistencyLevel == "" {
		config.ConsistencyLevel = "eventual"
	}

	cluster := &Cluster{
		ID:        id,
		Name:      name,
		Nodes:     make(map[string]*Node),
		Shards:    make(map[string]*Shard),
		Config:    config,
		State:     ClusterStateHealthy,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		dataDir:   dataDir,
	}

	// Create data directory if specified
	if dataDir != "" {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create cluster data directory: %w", err)
		}
	}

	// Load existing cluster state if available
	if err := cluster.loadState(); err != nil {
		return nil, fmt.Errorf("failed to load cluster state: %w", err)
	}

	return cluster, nil
}

// AddNode adds a node to the cluster
func (c *Cluster) AddNode(node *Node) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.Nodes[node.ID]; exists {
		return fmt.Errorf("node %s already exists in cluster", node.ID)
	}

	c.Nodes[node.ID] = node
	node.cluster = c
	c.UpdatedAt = time.Now()

	// Trigger rebalancing if enabled
	if c.Config.AutoRebalance {
		go c.triggerRebalance()
	}

	return c.saveState()
}

// RemoveNode removes a node from the cluster
func (c *Cluster) RemoveNode(nodeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found in cluster", nodeID)
	}

	// Migrate shards from the departing node
	if err := c.migrateNodeShards(nodeID); err != nil {
		return fmt.Errorf("failed to migrate shards from node %s: %w", nodeID, err)
	}

	delete(c.Nodes, nodeID)
	node.cluster = nil
	c.UpdatedAt = time.Now()

	return c.saveState()
}

// GetNodes returns all nodes in the cluster
func (c *Cluster) GetNodes() []*Node {
	c.mu.RLock()
	defer c.mu.RUnlock()

	nodes := make([]*Node, 0, len(c.Nodes))
	for _, node := range c.Nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// GetLeaderNode returns the current leader node
func (c *Cluster) GetLeaderNode() *Node {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, node := range c.Nodes {
		if node.IsLeaderNode() {
			return node
		}
	}
	return nil
}

// GetNodeByID returns a node by its ID
func (c *Cluster) GetNodeByID(nodeID string) *Node {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Nodes[nodeID]
}

// CreateShard creates a new shard
func (c *Cluster) CreateShard(keyRange ShardKeyRange) (*Shard, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	shardID := generateShardID(keyRange)

	// Find nodes for the shard
	primaryNode, replicaNodes := c.selectNodesForShard()
	if primaryNode == "" {
		return nil, fmt.Errorf("no available nodes for shard")
	}

	shard := &Shard{
		ID:           shardID,
		KeyRange:     keyRange,
		PrimaryNode:  primaryNode,
		ReplicaNodes: replicaNodes,
		State:        ShardStateActive,
		Repositories: make(map[string]bool),
		CreatedAt:    time.Now(),
	}

	c.Shards[shardID] = shard
	c.UpdatedAt = time.Now()

	return shard, c.saveState()
}

// GetShardForRepository returns the shard that should contain a repository
func (c *Cluster) GetShardForRepository(repoID string) *Shard {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Use consistent hashing to determine shard
	for _, shard := range c.Shards {
		if c.keyInRange(repoID, shard.KeyRange) {
			return shard
		}
	}

	return nil
}

// DistributeRepository distributes a repository to the appropriate shard
func (c *Cluster) DistributeRepository(repoID string, repo *govc.Repository) error {
	shard := c.GetShardForRepository(repoID)
	if shard == nil {
		// Create a new shard for this repository
		keyRange := c.calculateKeyRange(repoID)
		var err error
		shard, err = c.CreateShard(keyRange)
		if err != nil {
			return fmt.Errorf("failed to create shard for repository %s: %w", repoID, err)
		}
	}

	// Add repository to the shard
	shard.Repositories[repoID] = true
	shard.LastAccessed = time.Now()

	// Add repository to the primary node
	primaryNode := c.GetNodeByID(shard.PrimaryNode)
	if primaryNode != nil {
		primaryNode.mu.Lock()
		primaryNode.repositories[repoID] = repo
		primaryNode.mu.Unlock()
	}

	// Replicate to replica nodes
	for _, nodeID := range shard.ReplicaNodes {
		replicaNode := c.GetNodeByID(nodeID)
		if replicaNode != nil {
			replicaNode.mu.Lock()
			replicaNode.repositories[repoID] = repo
			replicaNode.mu.Unlock()
		}
	}

	return c.saveState()
}

// GetClusterHealth returns the overall health of the cluster
func (c *Cluster) GetClusterHealth() ClusterHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()

	health := ClusterHealth{
		Status:       c.State,
		TotalNodes:   len(c.Nodes),
		HealthyNodes: 0,
		TotalShards:  len(c.Shards),
		ActiveShards: 0,
		Timestamp:    time.Now(),
	}

	// Count healthy nodes
	for _, node := range c.Nodes {
		if node.GetState() != NodeStateOffline {
			health.HealthyNodes++
		}
	}

	// Count active shards
	for _, shard := range c.Shards {
		if shard.State == ShardStateActive {
			health.ActiveShards++
		}
	}

	// Determine overall cluster state
	if health.TotalNodes == 0 {
		// Empty cluster is considered healthy
		health.Status = ClusterStateHealthy
	} else if health.HealthyNodes == 0 {
		health.Status = ClusterStateUnavailable
	} else if health.HealthyNodes <= len(c.Nodes)/2 {
		health.Status = ClusterStateDegraded
	} else {
		health.Status = ClusterStateHealthy
	}

	return health
}

// ClusterHealth represents the health status of the cluster
type ClusterHealth struct {
	Status       ClusterState `json:"status"`
	TotalNodes   int          `json:"total_nodes"`
	HealthyNodes int          `json:"healthy_nodes"`
	TotalShards  int          `json:"total_shards"`
	ActiveShards int          `json:"active_shards"`
	Timestamp    time.Time    `json:"timestamp"`
}

// triggerRebalance triggers cluster rebalancing
func (c *Cluster) triggerRebalance() {
	// Implement rebalancing logic
	// This would analyze the current distribution of shards
	// and move them to achieve better balance
	fmt.Println("Triggering cluster rebalance...")
}

// migrateNodeShards migrates all shards from a departing node
func (c *Cluster) migrateNodeShards(nodeID string) error {
	// Find all shards on this node and migrate them
	for _, shard := range c.Shards {
		if shard.PrimaryNode == nodeID {
			// Promote a replica to primary
			if len(shard.ReplicaNodes) > 0 {
				shard.PrimaryNode = shard.ReplicaNodes[0]
				shard.ReplicaNodes = shard.ReplicaNodes[1:]
			}
		}

		// Remove from replica nodes
		newReplicas := make([]string, 0)
		for _, replicaID := range shard.ReplicaNodes {
			if replicaID != nodeID {
				newReplicas = append(newReplicas, replicaID)
			}
		}
		shard.ReplicaNodes = newReplicas
	}

	return nil
}

// selectNodesForShard selects nodes for a new shard
func (c *Cluster) selectNodesForShard() (primary string, replicas []string) {
	// Simple selection: round-robin for now
	// In production, this would consider node load, capacity, etc.

	nodeIDs := make([]string, 0, len(c.Nodes))
	for nodeID := range c.Nodes {
		nodeIDs = append(nodeIDs, nodeID)
	}

	if len(nodeIDs) == 0 {
		return "", nil
	}

	primary = nodeIDs[0]

	replicaCount := c.Config.ReplicationFactor - 1
	if replicaCount > len(nodeIDs)-1 {
		replicaCount = len(nodeIDs) - 1
	}

	replicas = make([]string, 0, replicaCount)
	for i := 1; i <= replicaCount && i < len(nodeIDs); i++ {
		replicas = append(replicas, nodeIDs[i])
	}

	return primary, replicas
}

// keyInRange checks if a key is within a shard's range
func (c *Cluster) keyInRange(key string, keyRange ShardKeyRange) bool {
	// For single-character ranges, check only the first character
	if len(keyRange.Start) == 1 && len(keyRange.End) == 1 && len(key) > 0 {
		firstChar := string(key[0])
		return firstChar >= keyRange.Start && firstChar <= keyRange.End
	}
	// For full string ranges
	return key >= keyRange.Start && key <= keyRange.End
}

// calculateKeyRange calculates the key range for a new shard
func (c *Cluster) calculateKeyRange(key string) ShardKeyRange {
	// Simple implementation: use first character for range
	// In production, this would be more sophisticated
	start := string(key[0])
	end := string(key[0] + 1)

	return ShardKeyRange{
		Start: start,
		End:   end,
	}
}

// loadState loads the cluster state from disk
func (c *Cluster) loadState() error {
	if c.dataDir == "" {
		return nil
	}

	statePath := filepath.Join(c.dataDir, "cluster-state.json")

	data, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		return nil // No existing state
	}
	if err != nil {
		return err
	}

	// Unmarshal into temporary state
	var state clusterState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to unmarshal cluster state: %w", err)
	}

	// Restore cluster fields
	c.ID = state.ID
	c.Name = state.Name
	c.Config = state.Config
	c.State = state.State
	c.CreatedAt = state.CreatedAt
	c.UpdatedAt = state.UpdatedAt

	// Note: Nodes and shards need to be reconstructed when nodes join
	// This just stores the metadata for reference

	return nil
}

// saveState saves the cluster state to disk
func (c *Cluster) saveState() error {
	// Skip saving state if dataDir is empty (e.g., in tests)
	if c.dataDir == "" {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Create a serializable version of the cluster state
	state := &clusterState{
		ID:        c.ID,
		Name:      c.Name,
		Config:    c.Config,
		State:     c.State,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
		Nodes:     make(map[string]*nodeState),
		Shards:    make(map[string]*shardState),
	}

	// Convert nodes to serializable format
	for id, node := range c.Nodes {
		state.Nodes[id] = &nodeState{
			ID:       node.ID,
			Address:  node.Address,
			State:    node.State,
			LastSeen: node.LastSeen,
		}
	}

	// Convert shards to serializable format
	for id, shard := range c.Shards {
		state.Shards[id] = &shardState{
			ID:           shard.ID,
			KeyRange:     shard.KeyRange,
			PrimaryNode:  shard.PrimaryNode,
			ReplicaNodes: shard.ReplicaNodes,
			State:        shard.State,
		}
	}

	// Create the state file path
	statePath := filepath.Join(c.dataDir, "cluster-state.json")

	// Marshal to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cluster state: %w", err)
	}

	// Write atomically by writing to temp file first
	tempPath := statePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cluster state: %w", err)
	}

	// Rename to final location
	if err := os.Rename(tempPath, statePath); err != nil {
		return fmt.Errorf("failed to save cluster state: %w", err)
	}

	return nil
}

// GetID returns the cluster ID
func (c *Cluster) GetID() string {
	return c.ID
}

// generateShardID generates a unique shard ID
func generateShardID(keyRange ShardKeyRange) string {
	return fmt.Sprintf("shard-%s-%s", keyRange.Start, keyRange.End)
}
