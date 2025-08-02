package cluster

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// RequestVoteRequest represents a Raft RequestVote RPC request
type RequestVoteRequest struct {
	Term         uint64 `json:"term"`
	CandidateID  string `json:"candidate_id"`
	LastLogIndex uint64 `json:"last_log_index"`
	LastLogTerm  uint64 `json:"last_log_term"`
}

// RequestVoteResponse represents a Raft RequestVote RPC response
type RequestVoteResponse struct {
	Term        uint64 `json:"term"`
	VoteGranted bool   `json:"vote_granted"`
}

// AppendEntriesRequest represents a Raft AppendEntries RPC request
type AppendEntriesRequest struct {
	Term         uint64     `json:"term"`
	LeaderID     string     `json:"leader_id"`
	PrevLogIndex uint64     `json:"prev_log_index"`
	PrevLogTerm  uint64     `json:"prev_log_term"`
	Entries      []LogEntry `json:"entries"`
	LeaderCommit uint64     `json:"leader_commit"`
}

// AppendEntriesResponse represents a Raft AppendEntries RPC response
type AppendEntriesResponse struct {
	Term    uint64 `json:"term"`
	Success bool   `json:"success"`
}

// SnapshotRequest represents a Raft InstallSnapshot RPC request
type SnapshotRequest struct {
	Term              uint64 `json:"term"`
	LeaderID          string `json:"leader_id"`
	LastIncludedIndex uint64 `json:"last_included_index"`
	LastIncludedTerm  uint64 `json:"last_included_term"`
	Offset            uint64 `json:"offset"`
	Data              []byte `json:"data"`
	Done              bool   `json:"done"`
}

// SnapshotResponse represents a Raft InstallSnapshot RPC response
type SnapshotResponse struct {
	Term uint64 `json:"term"`
}

// handleRequestVote handles Raft RequestVote RPC
func (n *Node) handleRequestVote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RequestVoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	n.raftState.mu.Lock()
	defer n.raftState.mu.Unlock()

	response := RequestVoteResponse{
		Term:        n.raftState.CurrentTerm,
		VoteGranted: false,
	}

	// Reply false if term < currentTerm
	if req.Term < n.raftState.CurrentTerm {
		n.sendJSONResponse(w, response)
		return
	}

	// If RPC request or response contains term T > currentTerm:
	// set currentTerm = T, convert to follower
	if req.Term > n.raftState.CurrentTerm {
		n.raftState.CurrentTerm = req.Term
		n.raftState.VotedFor = ""
		n.State = NodeStateFollower
		n.IsLeader = false
	}

	// Grant vote if:
	// - Haven't voted for anyone else in this term (or voted for this candidate)
	// - Candidate's log is at least as up-to-date as receiver's log
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
		response.VoteGranted = true
		n.raftState.LastHeartbeat = time.Now()
	}

	response.Term = n.raftState.CurrentTerm
	n.sendJSONResponse(w, response)
}

// handleAppendEntries handles Raft AppendEntries RPC
func (n *Node) handleAppendEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AppendEntriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	n.raftState.mu.Lock()
	defer n.raftState.mu.Unlock()

	response := AppendEntriesResponse{
		Term:    n.raftState.CurrentTerm,
		Success: false,
	}

	// Reply false if term < currentTerm
	if req.Term < n.raftState.CurrentTerm {
		n.sendJSONResponse(w, response)
		return
	}

	// If RPC request or response contains term T > currentTerm:
	// set currentTerm = T, convert to follower
	if req.Term > n.raftState.CurrentTerm {
		n.raftState.CurrentTerm = req.Term
		n.raftState.VotedFor = ""
	}

	// Convert to follower state
	n.State = NodeStateFollower
	n.IsLeader = false
	n.LeaderID = req.LeaderID
	n.raftState.LastHeartbeat = time.Now()

	// Reply false if log doesn't contain an entry at prevLogIndex
	// whose term matches prevLogTerm
	if req.PrevLogIndex > 0 {
		if req.PrevLogIndex > uint64(len(n.raftState.Log)) {
			response.Term = n.raftState.CurrentTerm
			n.sendJSONResponse(w, response)
			return
		}

		if len(n.raftState.Log) > 0 && req.PrevLogIndex <= uint64(len(n.raftState.Log)) {
			prevEntry := n.raftState.Log[req.PrevLogIndex-1]
			if prevEntry.Term != req.PrevLogTerm {
				response.Term = n.raftState.CurrentTerm
				n.sendJSONResponse(w, response)
				return
			}
		}
	}

	// If an existing entry conflicts with a new one (same index
	// but different terms), delete the existing entry and all that follow it
	for i, entry := range req.Entries {
		logIndex := req.PrevLogIndex + uint64(i) + 1
		if logIndex <= uint64(len(n.raftState.Log)) {
			existingEntry := n.raftState.Log[logIndex-1]
			if existingEntry.Term != entry.Term {
				// Delete this entry and all that follow
				n.raftState.Log = n.raftState.Log[:logIndex-1]
				break
			}
		}
	}

	// Append any new entries not already in the log
	for i, entry := range req.Entries {
		logIndex := req.PrevLogIndex + uint64(i) + 1
		if logIndex > uint64(len(n.raftState.Log)) {
			n.raftState.Log = append(n.raftState.Log, entry)
		}
	}

	// If leaderCommit > commitIndex, set commitIndex = min(leaderCommit, index of last new entry)
	if req.LeaderCommit > n.raftState.CommitIndex {
		lastNewIndex := req.PrevLogIndex + uint64(len(req.Entries))
		if req.LeaderCommit < lastNewIndex {
			n.raftState.CommitIndex = req.LeaderCommit
		} else {
			n.raftState.CommitIndex = lastNewIndex
		}
	}

	response.Success = true
	response.Term = n.raftState.CurrentTerm
	n.sendJSONResponse(w, response)

	// Apply committed entries
	n.applyCommittedEntries()
}

// handleSnapshot handles Raft InstallSnapshot RPC
func (n *Node) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	n.raftState.mu.Lock()
	defer n.raftState.mu.Unlock()

	response := SnapshotResponse{
		Term: n.raftState.CurrentTerm,
	}

	// Reply immediately if term < currentTerm
	if req.Term < n.raftState.CurrentTerm {
		n.sendJSONResponse(w, response)
		return
	}

	// Convert to follower if necessary
	if req.Term > n.raftState.CurrentTerm {
		n.raftState.CurrentTerm = req.Term
		n.raftState.VotedFor = ""
		n.State = NodeStateFollower
		n.IsLeader = false
	}

	n.LeaderID = req.LeaderID
	n.raftState.LastHeartbeat = time.Now()

	// Create new snapshot file if first chunk (offset is 0)
	if req.Offset == 0 {
		// Initialize snapshot reception
		// In a full implementation, this would handle snapshot data
	}

	// Write data to snapshot file at given offset
	// In a full implementation, this would append snapshot data

	// If done, save snapshot and reset state machine
	if req.Done {
		// Discard entire log up through lastIncludedIndex
		newLog := make([]LogEntry, 0)
		for _, entry := range n.raftState.Log {
			if entry.Index > req.LastIncludedIndex {
				newLog = append(newLog, entry)
			}
		}
		n.raftState.Log = newLog

		// Reset state machine using snapshot contents
		n.raftState.LastApplied = req.LastIncludedIndex
		n.raftState.CommitIndex = req.LastIncludedIndex
	}

	response.Term = n.raftState.CurrentTerm
	n.sendJSONResponse(w, response)
}

// applyCommittedEntries applies committed log entries to the state machine
func (n *Node) applyCommittedEntries() {
	for n.raftState.LastApplied < n.raftState.CommitIndex {
		n.raftState.LastApplied++

		// Find the entry to apply
		var entryToApply *LogEntry
		for i := range n.raftState.Log {
			if n.raftState.Log[i].Index == n.raftState.LastApplied {
				entryToApply = &n.raftState.Log[i]
				break
			}
		}

		if entryToApply != nil {
			n.applyLogEntry(*entryToApply)
		}
	}
}

// applyLogEntry applies a single log entry to the state machine
func (n *Node) applyLogEntry(entry LogEntry) {
	switch entry.Command.Type {
	case CommandTypeCreateRepo:
		n.handleCreateRepository(entry.Command)
	case CommandTypeDeleteRepo:
		n.handleDeleteRepository(entry.Command)
	case CommandTypeCommit:
		n.handleCommitCommand(entry.Command)
	case CommandTypeCreateBranch:
		n.handleCreateBranch(entry.Command)
	case CommandTypeDeleteBranch:
		n.handleDeleteBranch(entry.Command)
	case CommandTypeCreateTag:
		n.handleCreateTag(entry.Command)
	case CommandTypeDeleteTag:
		n.handleDeleteTag(entry.Command)
	case CommandTypeShardMove:
		n.handleShardMove(entry.Command)
	case CommandTypeNodeJoin:
		n.handleNodeJoin(entry.Command)
	case CommandTypeNodeLeave:
		n.handleNodeLeave(entry.Command)
	default:
		fmt.Printf("Unknown command type: %s\n", entry.Command.Type)
	}
}

// handleCreateRepository applies a create repository command
func (n *Node) handleCreateRepository(cmd Command) {
	repoID := cmd.Repository
	// In a full implementation, this would create the repository
	// For now, we'll just log the operation
	fmt.Printf("Creating repository: %s\n", repoID)
}

// handleDeleteRepository applies a delete repository command
func (n *Node) handleDeleteRepository(cmd Command) {
	repoID := cmd.Repository
	// In a full implementation, this would delete the repository
	fmt.Printf("Deleting repository: %s\n", repoID)
}

// handleCommitCommand applies a commit command
func (n *Node) handleCommitCommand(cmd Command) {
	repoID := cmd.Repository
	// In a full implementation, this would apply the commit
	fmt.Printf("Applying commit to repository: %s\n", repoID)
}

// handleCreateBranch applies a create branch command
func (n *Node) handleCreateBranch(cmd Command) {
	repoID := cmd.Repository
	branchName := cmd.Data["branch_name"]
	fmt.Printf("Creating branch %v in repository: %s\n", branchName, repoID)
}

// handleDeleteBranch applies a delete branch command
func (n *Node) handleDeleteBranch(cmd Command) {
	repoID := cmd.Repository
	branchName := cmd.Data["branch_name"]
	fmt.Printf("Deleting branch %v in repository: %s\n", branchName, repoID)
}

// handleCreateTag applies a create tag command
func (n *Node) handleCreateTag(cmd Command) {
	repoID := cmd.Repository
	tagName := cmd.Data["tag_name"]
	fmt.Printf("Creating tag %v in repository: %s\n", tagName, repoID)
}

// handleDeleteTag applies a delete tag command
func (n *Node) handleDeleteTag(cmd Command) {
	repoID := cmd.Repository
	tagName := cmd.Data["tag_name"]
	fmt.Printf("Deleting tag %v in repository: %s\n", tagName, repoID)
}

// handleShardMove applies a shard move command
func (n *Node) handleShardMove(cmd Command) {
	fromNode := cmd.Data["from_node"]
	toNode := cmd.Data["to_node"]
	fmt.Printf("Moving shard from %v to %v\n", fromNode, toNode)
}

// handleNodeJoin applies a node join command
func (n *Node) handleNodeJoin(cmd Command) {
	nodeID := cmd.Data["node_id"]
	fmt.Printf("Node %v joining cluster\n", nodeID)
}

// handleNodeLeave applies a node leave command
func (n *Node) handleNodeLeave(cmd Command) {
	nodeID := cmd.Data["node_id"]
	fmt.Printf("Node %v leaving cluster\n", nodeID)
}

// sendJSONResponse sends a JSON response
func (n *Node) sendJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
