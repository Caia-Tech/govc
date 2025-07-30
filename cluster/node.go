package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/caiatech/govc"
)

// Node represents a single node in the govc cluster
type Node struct {
	ID       string    `json:"id"`
	Address  string    `json:"address"`
	Port     int       `json:"port"`
	State    NodeState `json:"state"`
	LastSeen time.Time `json:"last_seen"`
	Version  string    `json:"version"`
	
	// Raft-specific fields
	Term         uint64    `json:"term"`
	VotedFor     string    `json:"voted_for"`
	IsLeader     bool      `json:"is_leader"`
	LeaderID     string    `json:"leader_id"`
	CommitIndex  uint64    `json:"commit_index"`
	LastApplied  uint64    `json:"last_applied"`
	NextIndex    []uint64  `json:"next_index"`
	MatchIndex   []uint64  `json:"match_index"`
	
	// Node management
	cluster      *Cluster
	repositories map[string]*govc.Repository
	httpServer   *http.Server
	raftState    *RaftState
	Stats        NodeStats
	Metadata     map[string]string
	mu           sync.RWMutex
}

// NodeStats holds node statistics
type NodeStats struct {
	CPUUsage        float64   `json:"cpu_usage"`
	MemoryUsage     float64   `json:"memory_usage"`
	DiskUsage       float64   `json:"disk_usage"`
	RepositoryCount int64     `json:"repository_count"`
	LastUpdate      time.Time `json:"last_update"`
}

// NodeCapacity represents node capacity information
type NodeCapacity struct {
	RepositoryCount   int64   `json:"repository_count"`
	MaxRepositories   int64   `json:"max_repositories"`
	AvailableCapacity float64 `json:"available_capacity"`
}

// NodeState represents the current state of a cluster node
type NodeState string

const (
	NodeStateFollower  NodeState = "follower"
	NodeStateCandidate NodeState = "candidate"
	NodeStateLeader    NodeState = "leader"
	NodeStateOffline   NodeState = "offline"
	NodeStateJoining   NodeState = "joining"
	NodeStateLeaving   NodeState = "leaving"
)

// RaftState manages Raft consensus state for the node
type RaftState struct {
	// Persistent state
	CurrentTerm uint64   `json:"current_term"`
	VotedFor    string   `json:"voted_for"`
	Log         []LogEntry `json:"log"`
	
	// Volatile state
	CommitIndex uint64 `json:"commit_index"`
	LastApplied uint64 `json:"last_applied"`
	
	// Leader state (reinitialized after election)
	NextIndex  map[string]uint64 `json:"next_index"`
	MatchIndex map[string]uint64 `json:"match_index"`
	
	// Timing
	ElectionTimeout  time.Duration `json:"election_timeout"`
	HeartbeatTimeout time.Duration `json:"heartbeat_timeout"`
	LastHeartbeat    time.Time     `json:"last_heartbeat"`
	
	mu sync.RWMutex
}

// LogEntry represents a single entry in the Raft log  
type LogEntry struct {
	Term      uint64      `json:"term"`
	Index     uint64      `json:"index"`
	Command   Command     `json:"command"`
	Timestamp time.Time   `json:"timestamp"`
	ClientID  string      `json:"client_id"`
}

// Command represents a distributed command that can be replicated
type Command struct {
	Type       CommandType            `json:"type"`
	Repository string                 `json:"repository"`
	Data       map[string]interface{} `json:"data"`
	Metadata   map[string]string      `json:"metadata"`
}

// CommandType defines the types of commands that can be replicated
type CommandType string

const (
	CommandTypeCreateRepo    CommandType = "create_repo"
	CommandTypeDeleteRepo    CommandType = "delete_repo"
	CommandTypeCommit        CommandType = "commit"
	CommandTypeCreateBranch  CommandType = "create_branch"
	CommandTypeDeleteBranch  CommandType = "delete_branch"
	CommandTypeCreateTag     CommandType = "create_tag"
	CommandTypeDeleteTag     CommandType = "delete_tag"
	CommandTypeShardMove     CommandType = "shard_move"
	CommandTypeNodeJoin      CommandType = "node_join"
	CommandTypeNodeLeave     CommandType = "node_leave"
)

// NodeConfig contains configuration for a cluster node
type NodeConfig struct {
	ID                string        `yaml:"id"`
	Address           string        `yaml:"address"`
	Port              int           `yaml:"port"`
	DataDir           string        `yaml:"data_dir"`
	ClusterPeers      []string      `yaml:"cluster_peers"`
	ElectionTimeout   time.Duration `yaml:"election_timeout"`
	HeartbeatTimeout  time.Duration `yaml:"heartbeat_timeout"`
	MaxLogEntries     int           `yaml:"max_log_entries"`
	SnapshotThreshold int           `yaml:"snapshot_threshold"`
}

// NewNode creates a new cluster node
func NewNode(config NodeConfig) (*Node, error) {
	if config.ID == "" {
		config.ID = generateNodeID()
	}
	
	if config.ElectionTimeout == 0 {
		config.ElectionTimeout = 150 * time.Millisecond
	}
	
	if config.HeartbeatTimeout == 0 {
		config.HeartbeatTimeout = 50 * time.Millisecond
	}

	node := &Node{
		ID:           config.ID,
		Address:      config.Address,
		Port:         config.Port,
		State:        NodeStateFollower,
		LastSeen:     time.Now(),
		Version:      "1.0.0",
		repositories: make(map[string]*govc.Repository),
		Stats:        NodeStats{},
		Metadata:     make(map[string]string),
		raftState: &RaftState{
			CurrentTerm:      0,
			VotedFor:         "",
			Log:              make([]LogEntry, 0),
			CommitIndex:      0,
			LastApplied:      0,
			NextIndex:        make(map[string]uint64),
			MatchIndex:       make(map[string]uint64),
			ElectionTimeout:  config.ElectionTimeout,
			HeartbeatTimeout: config.HeartbeatTimeout,
		},
	}

	// Create data directory
	dataDir := filepath.Join(config.DataDir, "node-"+config.ID)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Load persistent state
	if err := node.loadState(dataDir); err != nil {
		return nil, fmt.Errorf("failed to load node state: %w", err)
	}

	return node, nil
}

// Start starts the node and joins the cluster
func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Start HTTP server for inter-node communication
	if err := n.startHTTPServer(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Start Raft consensus
	go n.runRaftConsensus(ctx)

	// Start background tasks
	go n.runMaintenanceTasks(ctx)

	return nil
}

// Stop gracefully stops the node
func (n *Node) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.State = NodeStateLeaving

	// Stop HTTP server
	if n.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		n.httpServer.Shutdown(ctx)
	}

	// Save state
	if err := n.saveState(); err != nil {
		return fmt.Errorf("failed to save node state: %w", err)
	}

	return nil
}

// startHTTPServer starts the HTTP server for inter-node communication
func (n *Node) startHTTPServer() error {
	mux := http.NewServeMux()
	
	// Raft endpoints
	mux.HandleFunc("/raft/vote", n.handleRequestVote)
	mux.HandleFunc("/raft/append", n.handleAppendEntries)
	mux.HandleFunc("/raft/snapshot", n.handleSnapshot)
	
	// Cluster management endpoints
	mux.HandleFunc("/cluster/join", n.handleJoin)
	mux.HandleFunc("/cluster/leave", n.handleLeave)
	mux.HandleFunc("/cluster/status", n.handleStatus)
	
	// Repository operations
	mux.HandleFunc("/repo/", n.handleRepositoryOperation)

	n.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", n.Port),
		Handler: mux,
	}

	listener, err := net.Listen("tcp", n.httpServer.Addr)
	if err != nil {
		return err
	}

	go func() {
		if err := n.httpServer.Serve(listener); err != http.ErrServerClosed {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	return nil
}

// runRaftConsensus runs the main Raft consensus loop
func (n *Node) runRaftConsensus(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.raftState.mu.Lock()
			switch n.State {
			case NodeStateFollower:
				n.runFollower()
			case NodeStateCandidate:
				n.runCandidate()
			case NodeStateLeader:
				n.runLeader()
			}
			n.raftState.mu.Unlock()
		}
	}
}

// runFollower implements follower behavior in Raft
func (n *Node) runFollower() {
	// Check for election timeout
	if time.Since(n.raftState.LastHeartbeat) > n.raftState.ElectionTimeout {
		n.startElection()
	}
}

// runCandidate implements candidate behavior in Raft
func (n *Node) runCandidate() {
	// Election timeout - start new election
	if time.Since(n.raftState.LastHeartbeat) > n.raftState.ElectionTimeout {
		n.startElection()
	}
}

// runLeader implements leader behavior in Raft
func (n *Node) runLeader() {
	// Send heartbeats to all followers
	if time.Since(n.raftState.LastHeartbeat) > n.raftState.HeartbeatTimeout {
		n.sendHeartbeats()
		n.raftState.LastHeartbeat = time.Now()
	}
}

// startElection starts a new leader election
func (n *Node) startElection() {
	n.State = NodeStateCandidate
	n.raftState.CurrentTerm++
	n.raftState.VotedFor = n.ID
	n.raftState.LastHeartbeat = time.Now()

	// Vote for self
	votes := 1
	
	// Request votes from other nodes (simplified)
	// In a full implementation, this would send RequestVote RPCs
	// to all other nodes in the cluster
	
	if n.cluster != nil {
		totalNodes := len(n.cluster.GetNodes())
		majority := totalNodes/2 + 1
		
		if votes >= majority {
			n.becomeLeader()
		}
	}
}

// becomeLeader transitions the node to leader state
func (n *Node) becomeLeader() {
	n.State = NodeStateLeader
	n.IsLeader = true
	n.LeaderID = n.ID
	
	// Initialize leader state
	if n.cluster != nil {
		for _, node := range n.cluster.GetNodes() {
			if node.ID != n.ID {
				n.raftState.NextIndex[node.ID] = uint64(len(n.raftState.Log)) + 1
				n.raftState.MatchIndex[node.ID] = 0
			}
		}
	}
	
	// Send initial heartbeats
	n.sendHeartbeats()
}

// sendHeartbeats sends heartbeat messages to all followers
func (n *Node) sendHeartbeats() {
	if n.cluster == nil {
		return
	}
	
	for _, node := range n.cluster.GetNodes() {
		if node.ID != n.ID {
			go n.sendHeartbeat(node)
		}
	}
}

// sendHeartbeat sends a heartbeat to a specific node
func (n *Node) sendHeartbeat(target *Node) {
	// Simplified heartbeat - in a full implementation this would be
	// an AppendEntries RPC with empty entries
	// For now, we'll just update the target's last heartbeat time
	target.LastSeen = time.Now()
}

// loadState loads the node's persistent state from disk
func (n *Node) loadState(dataDir string) error {
	statePath := filepath.Join(dataDir, "node-state.json")
	
	data, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		// No existing state, start fresh
		return nil
	}
	if err != nil {
		return err
	}

	var state struct {
		RaftState *RaftState `json:"raft_state"`
	}
	
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	if state.RaftState != nil {
		n.raftState = state.RaftState
	}

	return nil
}

// saveState saves the node's persistent state to disk
func (n *Node) saveState() error {
	dataDir := filepath.Join("data", "node-"+n.ID)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}

	statePath := filepath.Join(dataDir, "node-state.json")
	
	state := struct {
		RaftState *RaftState `json:"raft_state"`
	}{
		RaftState: n.raftState,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statePath, data, 0644)
}

// runMaintenanceTasks runs background maintenance tasks
func (n *Node) runMaintenanceTasks(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.performMaintenance()
		}
	}
}

// performMaintenance performs routine maintenance tasks
func (n *Node) performMaintenance() {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Update last seen time
	n.LastSeen = time.Now()
	
	// Compact log if needed
	if len(n.raftState.Log) > 1000 { // Configurable threshold
		n.compactLog()
	}
	
	// Save state periodically
	if err := n.saveState(); err != nil {
		fmt.Printf("Warning: failed to save node state: %v\n", err)
	}
}

// compactLog compacts the Raft log by creating a snapshot
func (n *Node) compactLog() {
	// Simplified log compaction
	// In a full implementation, this would create a snapshot
	// and truncate the log up to the snapshot point
	
	if n.raftState.LastApplied > 100 {
		// Keep only the last 100 entries
		keepFrom := n.raftState.LastApplied - 100
		newLog := make([]LogEntry, 0)
		
		for _, entry := range n.raftState.Log {
			if entry.Index >= keepFrom {
				newLog = append(newLog, entry)
			}
		}
		
		n.raftState.Log = newLog
	}
}

// generateNodeID generates a unique node ID
func generateNodeID() string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", hostname, time.Now().Unix())
}

// GetState returns the current node state
func (n *Node) GetState() NodeState {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.State
}

// GetID returns the node ID
func (n *Node) GetID() string {
	return n.ID
}

// GetAddress returns the node address
func (n *Node) GetAddress() string {
	return fmt.Sprintf("%s:%d", n.Address, n.Port)
}

// IsLeaderNode returns true if this node is the current leader
func (n *Node) IsLeaderNode() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.IsLeader
}

// GetLeaderID returns the ID of the current leader
func (n *Node) GetLeaderID() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.LeaderID
}

// AddRepository adds a repository to the node
func (n *Node) AddRepository(repoID string, repo *govc.Repository) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	if _, exists := n.repositories[repoID]; exists {
		return fmt.Errorf("repository %s already exists", repoID)
	}
	
	n.repositories[repoID] = repo
	return nil
}

// RemoveRepository removes a repository from the node
func (n *Node) RemoveRepository(repoID string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	if _, exists := n.repositories[repoID]; !exists {
		return fmt.Errorf("repository %s not found", repoID)
	}
	
	delete(n.repositories, repoID)
	return nil
}

// GetRepository retrieves a repository from the node
func (n *Node) GetRepository(repoID string) *govc.Repository {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	return n.repositories[repoID]
}

// GetRepositories returns all repository IDs on this node
func (n *Node) GetRepositories() []string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	repos := make([]string, 0, len(n.repositories))
	for repoID := range n.repositories {
		repos = append(repos, repoID)
	}
	return repos
}

// UpdateStats updates the node's statistics
func (n *Node) UpdateStats() {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	// In a real implementation, these would be gathered from the system
	n.Stats.CPUUsage = 30.0 // Mock value
	n.Stats.MemoryUsage = 45.0 // Mock value
	n.Stats.DiskUsage = 60.0 // Mock value
	n.Stats.RepositoryCount = int64(len(n.repositories))
	n.Stats.LastUpdate = time.Now()
}

// IsHealthy returns true if the node is healthy
func (n *Node) IsHealthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	if n.State == NodeStateOffline {
		return false
	}
	
	// Check resource usage
	if n.Stats.CPUUsage > 90.0 || n.Stats.MemoryUsage > 90.0 || n.Stats.DiskUsage > 90.0 {
		return false
	}
	
	return true
}

// GetCapacity returns the node's capacity information
func (n *Node) GetCapacity() NodeCapacity {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	return NodeCapacity{
		RepositoryCount:   n.Stats.RepositoryCount,
		MaxRepositories:   100, // Mock value
		AvailableCapacity: 100.0 - (n.Stats.CPUUsage + n.Stats.MemoryUsage + n.Stats.DiskUsage) / 3.0,
	}
}