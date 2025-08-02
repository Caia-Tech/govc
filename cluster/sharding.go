package cluster

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/caiatech/govc"
)

// ShardingManager manages repository sharding across cluster nodes
type ShardingManager struct {
	cluster        *Cluster
	hashRing       *ConsistentHashRing
	shardMap       map[string]*Shard
	rebalancer     *ShardRebalancer
	migrationQueue *MigrationQueue
	mu             sync.RWMutex
}

// ConsistentHashRing implements consistent hashing for shard distribution
type ConsistentHashRing struct {
	nodes       map[uint32]string
	sortedNodes []uint32
	replicas    int
	mu          sync.RWMutex
}

// ShardRebalancer handles automatic rebalancing of shards
type ShardRebalancer struct {
	enabled        bool
	threshold      float64 // Imbalance threshold (0.0-1.0)
	cooldownPeriod time.Duration
	lastRebalance  time.Time
	rebalanceQueue chan RebalanceTask
	mu             sync.RWMutex
}

// MigrationQueue manages shard migration operations
type MigrationQueue struct {
	pending   []MigrationTask
	active    map[string]*MigrationTask
	completed []MigrationTask
	maxActive int
	mu        sync.RWMutex
}

// RebalanceTask represents a rebalancing operation
type RebalanceTask struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	SourceNode string    `json:"source_node"`
	TargetNode string    `json:"target_node"`
	ShardID    string    `json:"shard_id"`
	Priority   int       `json:"priority"`
	CreatedAt  time.Time `json:"created_at"`
}

// MigrationTask represents a shard migration operation
type MigrationTask struct {
	ID           string            `json:"id"`
	ShardID      string            `json:"shard_id"`
	SourceNode   string            `json:"source_node"`
	TargetNode   string            `json:"target_node"`
	Repositories []string          `json:"repositories"`
	State        MigrationState    `json:"state"`
	Progress     int               `json:"progress"`
	Error        string            `json:"error"`
	StartedAt    time.Time         `json:"started_at"`
	CompletedAt  time.Time         `json:"completed_at"`
	Metadata     map[string]string `json:"metadata"`
}

// MigrationState represents the state of a migration task
type MigrationState string

const (
	MigrationStatePending   MigrationState = "pending"
	MigrationStateActive    MigrationState = "active"
	MigrationStateCompleted MigrationState = "completed"
	MigrationStateFailed    MigrationState = "failed"
	MigrationStateCancelled MigrationState = "cancelled"
)

// ShardMetrics contains metrics for shard distribution
type ShardMetrics struct {
	TotalShards          int            `json:"total_shards"`
	ShardsPerNode        map[string]int `json:"shards_per_node"`
	RepositoriesPerShard map[string]int `json:"repositories_per_shard"`
	LoadBalance          float64        `json:"load_balance"`
	Imbalance            float64        `json:"imbalance"`
	HotSpots             []string       `json:"hot_spots"`
}

// NewShardingManager creates a new sharding manager
func NewShardingManager(cluster *Cluster) *ShardingManager {
	hashRing := NewConsistentHashRing(160) // 160 virtual nodes per physical node

	rebalancer := &ShardRebalancer{
		enabled:        true,
		threshold:      0.3, // 30% imbalance threshold
		cooldownPeriod: 5 * time.Minute,
		rebalanceQueue: make(chan RebalanceTask, 100),
	}

	migrationQueue := &MigrationQueue{
		pending:   make([]MigrationTask, 0),
		active:    make(map[string]*MigrationTask),
		completed: make([]MigrationTask, 0),
		maxActive: 3, // Maximum concurrent migrations
	}

	sm := &ShardingManager{
		cluster:        cluster,
		hashRing:       hashRing,
		shardMap:       make(map[string]*Shard),
		rebalancer:     rebalancer,
		migrationQueue: migrationQueue,
	}

	// Initialize hash ring with existing nodes
	for _, node := range cluster.GetNodes() {
		hashRing.AddNode(node.ID)
	}

	// Start background processes
	go sm.runRebalancer()
	go sm.runMigrationProcessor()

	return sm
}

// NewConsistentHashRing creates a new consistent hash ring
func NewConsistentHashRing(replicas int) *ConsistentHashRing {
	return &ConsistentHashRing{
		nodes:    make(map[uint32]string),
		replicas: replicas,
	}
}

// AddNode adds a node to the hash ring
func (chr *ConsistentHashRing) AddNode(nodeID string) {
	chr.mu.Lock()
	defer chr.mu.Unlock()

	for i := 0; i < chr.replicas; i++ {
		hash := chr.hash(fmt.Sprintf("%s:%d", nodeID, i))
		chr.nodes[hash] = nodeID
	}

	chr.updateSortedNodes()
}

// RemoveNode removes a node from the hash ring
func (chr *ConsistentHashRing) RemoveNode(nodeID string) {
	chr.mu.Lock()
	defer chr.mu.Unlock()

	for i := 0; i < chr.replicas; i++ {
		hash := chr.hash(fmt.Sprintf("%s:%d", nodeID, i))
		delete(chr.nodes, hash)
	}

	chr.updateSortedNodes()
}

// GetNode returns the node responsible for a given key
func (chr *ConsistentHashRing) GetNode(key string) string {
	chr.mu.RLock()
	defer chr.mu.RUnlock()

	if len(chr.sortedNodes) == 0 {
		return ""
	}

	hash := chr.hash(key)
	idx := chr.search(hash)
	return chr.nodes[chr.sortedNodes[idx]]
}

// GetNodes returns the top N nodes responsible for a key (for replication)
func (chr *ConsistentHashRing) GetNodes(key string, count int) []string {
	chr.mu.RLock()
	defer chr.mu.RUnlock()

	if len(chr.sortedNodes) == 0 || count <= 0 {
		return nil
	}

	hash := chr.hash(key)
	idx := chr.search(hash)

	nodes := make([]string, 0, count)
	seen := make(map[string]bool)

	for i := 0; i < len(chr.sortedNodes) && len(nodes) < count; i++ {
		nodeIdx := (idx + i) % len(chr.sortedNodes)
		nodeID := chr.nodes[chr.sortedNodes[nodeIdx]]

		if !seen[nodeID] {
			nodes = append(nodes, nodeID)
			seen[nodeID] = true
		}
	}

	return nodes
}

// hash generates a hash for a key
func (chr *ConsistentHashRing) hash(key string) uint32 {
	h := sha256.Sum256([]byte(key))
	return uint32(h[0])<<24 | uint32(h[1])<<16 | uint32(h[2])<<8 | uint32(h[3])
}

// search finds the appropriate node index for a hash
func (chr *ConsistentHashRing) search(hash uint32) int {
	idx := sort.Search(len(chr.sortedNodes), func(i int) bool {
		return chr.sortedNodes[i] >= hash
	})

	if idx == len(chr.sortedNodes) {
		idx = 0
	}

	return idx
}

// updateSortedNodes updates the sorted node list
func (chr *ConsistentHashRing) updateSortedNodes() {
	chr.sortedNodes = make([]uint32, 0, len(chr.nodes))
	for hash := range chr.nodes {
		chr.sortedNodes = append(chr.sortedNodes, hash)
	}
	sort.Slice(chr.sortedNodes, func(i, j int) bool {
		return chr.sortedNodes[i] < chr.sortedNodes[j]
	})
}

// GetShardForRepository determines which shard should contain a repository
func (sm *ShardingManager) GetShardForRepository(repoID string) *Shard {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Use consistent hashing to find the responsible node
	nodeID := sm.hashRing.GetNode(repoID)
	if nodeID == "" {
		return nil
	}

	// Find or create shard for this key range
	shardID := sm.generateShardID(repoID, nodeID)
	shard, exists := sm.shardMap[shardID]
	if !exists {
		// Create new shard
		keyRange := sm.calculateKeyRange(repoID)
		replicaNodes := sm.hashRing.GetNodes(repoID, sm.cluster.Config.ReplicationFactor)

		// Remove primary node from replicas
		replicas := make([]string, 0)
		for _, node := range replicaNodes {
			if node != nodeID {
				replicas = append(replicas, node)
			}
		}

		shard = &Shard{
			ID:           shardID,
			KeyRange:     keyRange,
			PrimaryNode:  nodeID,
			ReplicaNodes: replicas,
			State:        ShardStateActive,
			Repositories: make(map[string]bool),
			CreatedAt:    time.Now(),
		}

		sm.shardMap[shardID] = shard
		sm.cluster.Shards[shardID] = shard
	}

	return shard
}

// DistributeRepository distributes a repository to the appropriate shard
func (sm *ShardingManager) DistributeRepository(repoID string, repo *govc.Repository) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	shard := sm.GetShardForRepository(repoID)
	if shard == nil {
		return fmt.Errorf("no available shard for repository %s", repoID)
	}

	// Add repository to shard
	shard.Repositories[repoID] = true
	shard.LastAccessed = time.Now()

	// Distribute to primary node
	primaryNode := sm.cluster.GetNodeByID(shard.PrimaryNode)
	if primaryNode != nil {
		primaryNode.mu.Lock()
		primaryNode.repositories[repoID] = repo
		primaryNode.mu.Unlock()
	}

	// Replicate to replica nodes
	for _, nodeID := range shard.ReplicaNodes {
		replicaNode := sm.cluster.GetNodeByID(nodeID)
		if replicaNode != nil {
			go sm.replicateToNode(replicaNode, repoID, repo)
		}
	}

	// Check if rebalancing is needed
	if sm.rebalancer.enabled {
		go sm.checkRebalanceNeeded()
	}

	return nil
}

// replicateToNode replicates a repository to a specific node
func (sm *ShardingManager) replicateToNode(node *Node, repoID string, repo *govc.Repository) {
	node.mu.Lock()
	defer node.mu.Unlock()

	// In a full implementation, this would handle async replication
	// For now, we'll just add it to the node's repository map
	node.repositories[repoID] = repo
}

// MigrateShard migrates a shard from one node to another
func (sm *ShardingManager) MigrateShard(shardID, sourceNode, targetNode string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	shard, exists := sm.shardMap[shardID]
	if !exists {
		return fmt.Errorf("shard %s not found", shardID)
	}

	// Create migration task
	task := MigrationTask{
		ID:         fmt.Sprintf("migration-%d", time.Now().UnixNano()),
		ShardID:    shardID,
		SourceNode: sourceNode,
		TargetNode: targetNode,
		State:      MigrationStatePending,
		StartedAt:  time.Now(),
		Metadata:   make(map[string]string),
	}

	// Add repositories to migration task
	for repoID := range shard.Repositories {
		task.Repositories = append(task.Repositories, repoID)
	}

	// Queue the migration
	sm.migrationQueue.mu.Lock()
	sm.migrationQueue.pending = append(sm.migrationQueue.pending, task)
	sm.migrationQueue.mu.Unlock()

	return nil
}

// runRebalancer runs the automatic rebalancing process
func (sm *ShardingManager) runRebalancer() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if sm.rebalancer.enabled {
				sm.checkRebalanceNeeded()
			}
		case task := <-sm.rebalancer.rebalanceQueue:
			sm.executeRebalanceTask(task)
		}
	}
}

// runMigrationProcessor processes migration tasks
func (sm *ShardingManager) runMigrationProcessor() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C
		sm.processPendingMigrations()
	}
}

// checkRebalanceNeeded checks if cluster rebalancing is needed
func (sm *ShardingManager) checkRebalanceNeeded() {
	sm.rebalancer.mu.Lock()
	defer sm.rebalancer.mu.Unlock()

	// Check cooldown period
	if time.Since(sm.rebalancer.lastRebalance) < sm.rebalancer.cooldownPeriod {
		return
	}

	metrics := sm.CalculateShardMetrics()

	// Check if imbalance exceeds threshold
	if metrics.Imbalance > sm.rebalancer.threshold {
		sm.rebalancer.lastRebalance = time.Now()

		// Generate rebalance tasks
		tasks := sm.generateRebalanceTasks(metrics)
		for _, task := range tasks {
			select {
			case sm.rebalancer.rebalanceQueue <- task:
			default:
				// Queue is full, skip for now
			}
		}
	}
}

// executeRebalanceTask executes a rebalancing task
func (sm *ShardingManager) executeRebalanceTask(task RebalanceTask) {
	// Create migration for the shard
	err := sm.MigrateShard(task.ShardID, task.SourceNode, task.TargetNode)
	if err != nil {
		fmt.Printf("Failed to migrate shard %s: %v\n", task.ShardID, err)
	}
}

// generateRebalanceTasks generates tasks to rebalance the cluster
func (sm *ShardingManager) generateRebalanceTasks(metrics ShardMetrics) []RebalanceTask {
	tasks := make([]RebalanceTask, 0)

	// Find overloaded and underloaded nodes
	avgShards := float64(metrics.TotalShards) / float64(len(metrics.ShardsPerNode))

	overloaded := make([]string, 0)
	underloaded := make([]string, 0)

	for nodeID, shardCount := range metrics.ShardsPerNode {
		load := float64(shardCount) / avgShards
		if load > 1.0+sm.rebalancer.threshold {
			overloaded = append(overloaded, nodeID)
		} else if load < 1.0-sm.rebalancer.threshold {
			underloaded = append(underloaded, nodeID)
		}
	}

	// Generate migration tasks from overloaded to underloaded nodes
	for i := 0; i < len(overloaded) && i < len(underloaded); i++ {
		sourceNode := overloaded[i]
		targetNode := underloaded[i]

		// Find a shard to migrate
		for shardID, shard := range sm.shardMap {
			if shard.PrimaryNode == sourceNode {
				task := RebalanceTask{
					ID:         fmt.Sprintf("rebalance-%d", time.Now().UnixNano()),
					Type:       "migrate_shard",
					SourceNode: sourceNode,
					TargetNode: targetNode,
					ShardID:    shardID,
					Priority:   1,
					CreatedAt:  time.Now(),
				}
				tasks = append(tasks, task)
				break // Only migrate one shard at a time
			}
		}
	}

	return tasks
}

// processPendingMigrations processes pending migration tasks
func (sm *ShardingManager) processPendingMigrations() {
	sm.migrationQueue.mu.Lock()
	defer sm.migrationQueue.mu.Unlock()

	// Check if we can start new migrations
	if len(sm.migrationQueue.active) >= sm.migrationQueue.maxActive {
		return
	}

	// Start pending migrations
	for i := len(sm.migrationQueue.pending) - 1; i >= 0; i-- {
		if len(sm.migrationQueue.active) >= sm.migrationQueue.maxActive {
			break
		}

		task := sm.migrationQueue.pending[i]

		// Move from pending to active
		sm.migrationQueue.pending = append(sm.migrationQueue.pending[:i], sm.migrationQueue.pending[i+1:]...)
		task.State = MigrationStateActive
		sm.migrationQueue.active[task.ID] = &task

		// Start migration in background
		go sm.executeMigration(&task)
	}
}

// executeMigration executes a migration task
func (sm *ShardingManager) executeMigration(task *MigrationTask) {
	defer func() {
		sm.migrationQueue.mu.Lock()
		delete(sm.migrationQueue.active, task.ID)
		task.CompletedAt = time.Now()
		sm.migrationQueue.completed = append(sm.migrationQueue.completed, *task)
		sm.migrationQueue.mu.Unlock()
	}()

	sourceNode := sm.cluster.GetNodeByID(task.SourceNode)
	targetNode := sm.cluster.GetNodeByID(task.TargetNode)

	if sourceNode == nil || targetNode == nil {
		task.State = MigrationStateFailed
		task.Error = "Source or target node not found"
		return
	}

	// Migrate each repository
	totalRepos := len(task.Repositories)
	for i, repoID := range task.Repositories {
		// Get repository from source node
		sourceNode.mu.RLock()
		repo, exists := sourceNode.repositories[repoID]
		sourceNode.mu.RUnlock()

		if !exists {
			continue
		}

		// Add to target node
		targetNode.mu.Lock()
		targetNode.repositories[repoID] = repo
		targetNode.mu.Unlock()

		// Remove from source node
		sourceNode.mu.Lock()
		delete(sourceNode.repositories, repoID)
		sourceNode.mu.Unlock()

		// Update progress
		task.Progress = int(float64(i+1) / float64(totalRepos) * 100)
	}

	// Update shard assignment
	sm.mu.Lock()
	shard := sm.shardMap[task.ShardID]
	if shard != nil {
		if shard.PrimaryNode == task.SourceNode {
			shard.PrimaryNode = task.TargetNode
		}

		// Update replica nodes
		newReplicas := make([]string, 0)
		for _, nodeID := range shard.ReplicaNodes {
			if nodeID == task.SourceNode {
				newReplicas = append(newReplicas, task.TargetNode)
			} else {
				newReplicas = append(newReplicas, nodeID)
			}
		}
		shard.ReplicaNodes = newReplicas
	}
	sm.mu.Unlock()

	task.State = MigrationStateCompleted
	task.Progress = 100
}

// CalculateShardMetrics calculates sharding metrics for the cluster
func (sm *ShardingManager) CalculateShardMetrics() ShardMetrics {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	metrics := ShardMetrics{
		TotalShards:          len(sm.shardMap),
		ShardsPerNode:        make(map[string]int),
		RepositoriesPerShard: make(map[string]int),
		HotSpots:             make([]string, 0),
	}

	// Count shards per node
	for _, shard := range sm.shardMap {
		metrics.ShardsPerNode[shard.PrimaryNode]++
		metrics.RepositoriesPerShard[shard.ID] = len(shard.Repositories)
	}

	// Calculate load balance and imbalance
	if len(metrics.ShardsPerNode) > 0 {
		total := 0
		max := 0
		min := math.MaxInt32

		for _, count := range metrics.ShardsPerNode {
			total += count
			if count > max {
				max = count
			}
			if count < min {
				min = count
			}
		}

		avg := float64(total) / float64(len(metrics.ShardsPerNode))
		metrics.LoadBalance = avg

		if min > 0 {
			metrics.Imbalance = float64(max-min) / float64(min)
		}
	}

	// Identify hot spots (shards with high repository count)
	avgReposPerShard := 0
	if len(sm.shardMap) > 0 {
		totalRepos := 0
		for _, count := range metrics.RepositoriesPerShard {
			totalRepos += count
		}
		avgReposPerShard = totalRepos / len(sm.shardMap)
	}

	for shardID, count := range metrics.RepositoriesPerShard {
		if count > avgReposPerShard*2 { // 2x average is considered a hot spot
			metrics.HotSpots = append(metrics.HotSpots, shardID)
		}
	}

	return metrics
}

// generateShardID generates a unique shard ID
func (sm *ShardingManager) generateShardID(repoID, nodeID string) string {
	hash := sha256.Sum256([]byte(repoID + nodeID))
	return hex.EncodeToString(hash[:8])
}

// calculateKeyRange calculates the key range for a shard based on repository ID
func (sm *ShardingManager) calculateKeyRange(repoID string) ShardKeyRange {
	// Use the first few characters to determine range
	if len(repoID) == 0 {
		return ShardKeyRange{Start: "0", End: "z"}
	}

	start := string(repoID[0])
	end := start
	if repoID[0] < 'z' {
		end = string(repoID[0] + 1)
	} else {
		end = "z"
	}

	return ShardKeyRange{
		Start: start,
		End:   end,
	}
}

// GetMigrationStatus returns the status of all migrations
func (sm *ShardingManager) GetMigrationStatus() map[string]interface{} {
	sm.migrationQueue.mu.RLock()
	defer sm.migrationQueue.mu.RUnlock()

	return map[string]interface{}{
		"pending":    len(sm.migrationQueue.pending),
		"active":     len(sm.migrationQueue.active),
		"completed":  len(sm.migrationQueue.completed),
		"max_active": sm.migrationQueue.maxActive,
	}
}
