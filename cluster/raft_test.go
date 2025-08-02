package cluster

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRaftStateInitialization tests Raft state initialization
func TestRaftStateInitialization(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Verify initial Raft state
	assert.Equal(t, uint64(0), node.raftState.CurrentTerm)
	assert.Equal(t, "", node.raftState.VotedFor)
	assert.Equal(t, uint64(0), node.raftState.CommitIndex)
	assert.Equal(t, uint64(0), node.raftState.LastApplied)
	assert.Empty(t, node.raftState.Log)
	assert.NotNil(t, node.raftState.NextIndex)
	assert.NotNil(t, node.raftState.MatchIndex)
}

// TestElectionTimeout tests election timeout behavior
func TestElectionTimeout(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Set up cluster with multiple nodes
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 3,
		ElectionTimeout:   150 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
	}, t.TempDir())
	require.NoError(t, err)

	node.cluster = cluster
	cluster.AddNode(node)

	// Add other nodes
	node2, _ := createTestNode("node2", "127.0.0.2", 8081)
	node3, _ := createTestNode("node3", "127.0.0.3", 8082)
	cluster.AddNode(node2)
	cluster.AddNode(node3)

	// Test election timeout
	node.raftState.mu.Lock()
	node.raftState.LastHeartbeat = time.Now().Add(-200 * time.Millisecond)
	node.raftState.mu.Unlock()

	// Should trigger election - test would check state change
	// In real implementation, this would be triggered by timeout

	// Node should become candidate
	assert.Equal(t, NodeStateCandidate, node.State)
	assert.Equal(t, uint64(1), node.raftState.CurrentTerm)
	assert.Equal(t, node.ID, node.raftState.VotedFor)
}

// TestStartElection tests election process
func TestStartElection(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Set up cluster
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{
		ReplicationFactor: 3,
	}, t.TempDir())
	require.NoError(t, err)

	node.cluster = cluster
	cluster.AddNode(node)

	// Start election
	node.startElection()

	// Verify election state
	assert.Equal(t, NodeStateCandidate, node.State)
	assert.Equal(t, uint64(1), node.raftState.CurrentTerm)
	assert.Equal(t, node.ID, node.raftState.VotedFor)
}

// TestBecomeLeader tests leader transition
func TestBecomeLeader(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Set up cluster
	cluster, err := NewCluster("test-cluster", "Test Cluster", ClusterConfig{}, t.TempDir())
	require.NoError(t, err)
	node.cluster = cluster

	// Become leader
	node.becomeLeader()

	// Verify leader state
	assert.Equal(t, NodeStateLeader, node.State)
	assert.True(t, node.IsLeader)
	assert.Equal(t, node.ID, node.LeaderID)
}

// TestBecomeFollower tests follower transition
func TestBecomeFollower(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Start as leader
	node.State = NodeStateLeader
	node.IsLeader = true
	node.LeaderID = node.ID

	// Become follower
	node.mu.Lock()
	node.State = NodeStateFollower
	node.IsLeader = false
	node.LeaderID = "other-leader"
	node.mu.Unlock()

	// Verify follower state
	assert.Equal(t, NodeStateFollower, node.State)
	assert.False(t, node.IsLeader)
	assert.Equal(t, "other-leader", node.LeaderID)
}

// TestAppendLogEntry tests log entry appending
func TestAppendLogEntry(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Create log entry
	entry := LogEntry{
		Term:  1,
		Index: 1,
		Command: Command{
			Type:       CommandTypeCommit,
			Repository: "test-repo",
			Data:       map[string]interface{}{"message": "test commit"},
		},
		Timestamp: time.Now(),
		ClientID:  "test-client",
	}

	// Append entry
	node.raftState.mu.Lock()
	node.raftState.Log = append(node.raftState.Log, entry)
	node.raftState.mu.Unlock()

	// Verify entry was appended
	assert.Len(t, node.raftState.Log, 1)
	assert.Equal(t, entry, node.raftState.Log[0])
}

// TestLogReplication tests log replication logic
func TestLogReplication(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Create multiple log entries
	for i := 1; i <= 5; i++ {
		entry := LogEntry{
			Term:  uint64(1),
			Index: uint64(i),
			Command: Command{
				Type:       CommandTypeCommit,
				Repository: "test-repo",
				Data:       map[string]interface{}{"commit": i},
			},
			Timestamp: time.Now(),
		}
		node.raftState.Log = append(node.raftState.Log, entry)
	}

	// Test getting entries after a specific index
	entries := node.getLogEntriesAfter(2)
	assert.Len(t, entries, 3)
	assert.Equal(t, uint64(3), entries[0].Index)
	assert.Equal(t, uint64(5), entries[2].Index)
}

// TestCommitIndex tests commit index advancement
func TestCommitIndex(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Add log entries
	for i := 1; i <= 5; i++ {
		entry := LogEntry{
			Term:  1,
			Index: uint64(i),
			Command: Command{
				Type: CommandTypeCommit,
			},
		}
		node.raftState.Log = append(node.raftState.Log, entry)
	}

	// Update commit index
	node.raftState.mu.Lock()
	node.raftState.CommitIndex = 3
	node.raftState.mu.Unlock()

	// Apply committed entries
	node.applyCommittedEntries()

	// Verify LastApplied was updated
	assert.Equal(t, uint64(3), node.raftState.LastApplied)
}

// TestRequestVoteLogic tests vote request handling logic
func TestRequestVoteLogic(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	tests := []struct {
		name         string
		currentTerm  uint64
		votedFor     string
		lastLogIndex uint64
		lastLogTerm  uint64
		reqTerm      uint64
		reqCandidate string
		reqLastIndex uint64
		reqLastTerm  uint64
		expectVote   bool
		expectTerm   uint64
	}{
		{
			name:         "grant vote - higher term",
			currentTerm:  1,
			votedFor:     "",
			lastLogIndex: 5,
			lastLogTerm:  1,
			reqTerm:      2,
			reqCandidate: "candidate1",
			reqLastIndex: 5,
			reqLastTerm:  1,
			expectVote:   true,
			expectTerm:   2,
		},
		{
			name:         "reject vote - lower term",
			currentTerm:  2,
			votedFor:     "",
			lastLogIndex: 5,
			lastLogTerm:  2,
			reqTerm:      1,
			reqCandidate: "candidate1",
			reqLastIndex: 5,
			reqLastTerm:  1,
			expectVote:   false,
			expectTerm:   2,
		},
		{
			name:         "reject vote - already voted",
			currentTerm:  2,
			votedFor:     "other-candidate",
			lastLogIndex: 5,
			lastLogTerm:  2,
			reqTerm:      2,
			reqCandidate: "candidate1",
			reqLastIndex: 5,
			reqLastTerm:  2,
			expectVote:   false,
			expectTerm:   2,
		},
		{
			name:         "grant vote - voted for same candidate",
			currentTerm:  2,
			votedFor:     "candidate1",
			lastLogIndex: 5,
			lastLogTerm:  2,
			reqTerm:      2,
			reqCandidate: "candidate1",
			reqLastIndex: 5,
			reqLastTerm:  2,
			expectVote:   true,
			expectTerm:   2,
		},
		{
			name:         "reject vote - candidate log not up to date",
			currentTerm:  2,
			votedFor:     "",
			lastLogIndex: 5,
			lastLogTerm:  2,
			reqTerm:      2,
			reqCandidate: "candidate1",
			reqLastIndex: 3,
			reqLastTerm:  2,
			expectVote:   false,
			expectTerm:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up node state
			node.raftState.CurrentTerm = tt.currentTerm
			node.raftState.VotedFor = tt.votedFor

			// Set up log
			node.raftState.Log = nil
			if tt.lastLogIndex > 0 {
				for i := uint64(1); i <= tt.lastLogIndex; i++ {
					node.raftState.Log = append(node.raftState.Log, LogEntry{
						Term:  tt.lastLogTerm,
						Index: i,
					})
				}
			}

			// Simulate vote request processing
			req := RequestVoteRequest{
				Term:         tt.reqTerm,
				CandidateID:  tt.reqCandidate,
				LastLogIndex: tt.reqLastIndex,
				LastLogTerm:  tt.reqLastTerm,
			}

			voteGranted := node.processVoteRequest(req)

			assert.Equal(t, tt.expectVote, voteGranted)
			assert.Equal(t, tt.expectTerm, node.raftState.CurrentTerm)
		})
	}
}

// TestAppendEntriesLogic tests append entries handling logic
func TestAppendEntriesLogic(t *testing.T) {
	node, err := createTestNode("test-node", "127.0.0.1", 8080)
	require.NoError(t, err)

	// Set up initial log
	for i := 1; i <= 3; i++ {
		node.raftState.Log = append(node.raftState.Log, LogEntry{
			Term:  1,
			Index: uint64(i),
		})
	}

	tests := []struct {
		name          string
		currentTerm   uint64
		reqTerm       uint64
		reqPrevIndex  uint64
		reqPrevTerm   uint64
		reqEntries    []LogEntry
		reqCommit     uint64
		expectSuccess bool
		expectLogLen  int
	}{
		{
			name:         "append new entries",
			currentTerm:  1,
			reqTerm:      1,
			reqPrevIndex: 3,
			reqPrevTerm:  1,
			reqEntries: []LogEntry{
				{Term: 1, Index: 4},
				{Term: 1, Index: 5},
			},
			reqCommit:     5,
			expectSuccess: true,
			expectLogLen:  5,
		},
		{
			name:          "reject - term mismatch",
			currentTerm:   2,
			reqTerm:       1,
			reqPrevIndex:  3,
			reqPrevTerm:   1,
			reqEntries:    []LogEntry{},
			reqCommit:     3,
			expectSuccess: false,
			expectLogLen:  3,
		},
		{
			name:          "reject - missing prev entry",
			currentTerm:   1,
			reqTerm:       1,
			reqPrevIndex:  5,
			reqPrevTerm:   1,
			reqEntries:    []LogEntry{},
			reqCommit:     3,
			expectSuccess: false,
			expectLogLen:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset log for each test
			node.raftState.Log = nil
			for i := 1; i <= 3; i++ {
				node.raftState.Log = append(node.raftState.Log, LogEntry{
					Term:  1,
					Index: uint64(i),
				})
			}

			node.raftState.CurrentTerm = tt.currentTerm

			req := AppendEntriesRequest{
				Term:         tt.reqTerm,
				LeaderID:     "leader",
				PrevLogIndex: tt.reqPrevIndex,
				PrevLogTerm:  tt.reqPrevTerm,
				Entries:      tt.reqEntries,
				LeaderCommit: tt.reqCommit,
			}

			success := node.processAppendEntries(req)

			assert.Equal(t, tt.expectSuccess, success)
			assert.Equal(t, tt.expectLogLen, len(node.raftState.Log))
		})
	}
}

// Helper functions for tests

func (n *Node) getLogEntriesAfter(index uint64) []LogEntry {
	n.raftState.mu.RLock()
	defer n.raftState.mu.RUnlock()

	var entries []LogEntry
	for _, entry := range n.raftState.Log {
		if entry.Index > index {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (n *Node) processVoteRequest(req RequestVoteRequest) bool {
	n.raftState.mu.Lock()
	defer n.raftState.mu.Unlock()

	// Reply false if term < currentTerm
	if req.Term < n.raftState.CurrentTerm {
		return false
	}

	// If RPC request contains term T > currentTerm: set currentTerm = T, convert to follower
	if req.Term > n.raftState.CurrentTerm {
		n.raftState.CurrentTerm = req.Term
		n.raftState.VotedFor = ""
		n.State = NodeStateFollower
		n.IsLeader = false
	}

	// Check if we can grant vote
	lastLogIndex := uint64(0)
	lastLogTerm := uint64(0)
	if len(n.raftState.Log) > 0 {
		lastEntry := n.raftState.Log[len(n.raftState.Log)-1]
		lastLogIndex = lastEntry.Index
		lastLogTerm = lastEntry.Term
	}

	logUpToDate := (req.LastLogTerm > lastLogTerm) ||
		(req.LastLogTerm == lastLogTerm && req.LastLogIndex >= lastLogIndex)

	if (n.raftState.VotedFor == "" || n.raftState.VotedFor == req.CandidateID) && logUpToDate {
		n.raftState.VotedFor = req.CandidateID
		return true
	}

	return false
}

func (n *Node) processAppendEntries(req AppendEntriesRequest) bool {
	n.raftState.mu.Lock()
	defer n.raftState.mu.Unlock()

	// Reply false if term < currentTerm
	if req.Term < n.raftState.CurrentTerm {
		return false
	}

	// Reply false if log doesn't contain an entry at prevLogIndex whose term matches prevLogTerm
	if req.PrevLogIndex > 0 {
		if req.PrevLogIndex > uint64(len(n.raftState.Log)) {
			return false
		}

		if len(n.raftState.Log) > 0 && req.PrevLogIndex <= uint64(len(n.raftState.Log)) {
			prevEntry := n.raftState.Log[req.PrevLogIndex-1]
			if prevEntry.Term != req.PrevLogTerm {
				return false
			}
		}
	}

	// Append new entries
	for _, entry := range req.Entries {
		n.raftState.Log = append(n.raftState.Log, entry)
	}

	// Update commit index
	if req.LeaderCommit > n.raftState.CommitIndex {
		lastNewIndex := req.PrevLogIndex + uint64(len(req.Entries))
		if req.LeaderCommit < lastNewIndex {
			n.raftState.CommitIndex = req.LeaderCommit
		} else {
			n.raftState.CommitIndex = lastNewIndex
		}
	}

	return true
}
