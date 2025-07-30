package cluster

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// JoinRequest represents a request to join the cluster
type JoinRequest struct {
	NodeID   string `json:"node_id"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Version  string `json:"version"`
}

// JoinResponse represents a response to a join request
type JoinResponse struct {
	Success    bool     `json:"success"`
	Message    string   `json:"message"`
	LeaderID   string   `json:"leader_id"`
	ClusterID  string   `json:"cluster_id"`
	Nodes      []string `json:"nodes"`
}

// LeaveRequest represents a request to leave the cluster
type LeaveRequest struct {
	NodeID string `json:"node_id"`
	Reason string `json:"reason"`
}

// LeaveResponse represents a response to a leave request
type LeaveResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// StatusResponse represents cluster status information
type StatusResponse struct {
	NodeID        string    `json:"node_id"`
	State         NodeState `json:"state"`
	IsLeader      bool      `json:"is_leader"`
	LeaderID      string    `json:"leader_id"`
	Term          uint64    `json:"term"`
	CommitIndex   uint64    `json:"commit_index"`
	LastApplied   uint64    `json:"last_applied"`
	LogLength     int       `json:"log_length"`
	ClusterSize   int       `json:"cluster_size"`
	ConnectedNodes []string `json:"connected_nodes"`
}

// RepositoryOperation represents a repository operation
type RepositoryOperation struct {
	Operation string                 `json:"operation"`
	RepoID    string                 `json:"repo_id"`
	Data      map[string]interface{} `json:"data"`
}

// handleJoin handles requests to join the cluster
func (n *Node) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	response := JoinResponse{
		Success: false,
	}

	// Only the leader can handle join requests
	if !n.IsLeader {
		response.Message = "Not the leader"
		response.LeaderID = n.LeaderID
		w.WriteHeader(http.StatusServiceUnavailable)
		n.sendJSONResponse(w, response)
		return
	}

	// Validate the join request
	if req.NodeID == "" {
		response.Message = "Node ID is required"
		http.Error(w, response.Message, http.StatusBadRequest)
		return
	}

	if req.NodeID == n.ID {
		response.Message = "Cannot join self"
		http.Error(w, response.Message, http.StatusBadRequest)
		return
	}

	// Check if node is already in the cluster
	if n.cluster != nil {
		for _, node := range n.cluster.GetNodes() {
			if node.ID == req.NodeID {
				response.Message = "Node already in cluster"
				http.Error(w, response.Message, http.StatusConflict)
				return
			}
		}
	}

	// Create a command to add the node to the cluster
	cmd := Command{
		Type:       CommandTypeNodeJoin,
		Repository: "",
		Data: map[string]interface{}{
			"node_id": req.NodeID,
			"address": req.Address,
			"port":    req.Port,
			"version": req.Version,
		},
	}

	// Append to log and replicate
	if err := n.appendAndReplicate(cmd); err != nil {
		response.Message = fmt.Sprintf("Failed to replicate join command: %v", err)
		http.Error(w, response.Message, http.StatusInternalServerError)
		return
	}

	// Success response
	response.Success = true
	response.Message = "Node successfully joined cluster"
	response.LeaderID = n.ID
	response.ClusterID = n.cluster.GetID()
	
	// Get list of all nodes
	nodes := make([]string, 0)
	if n.cluster != nil {
		for _, node := range n.cluster.GetNodes() {
			nodes = append(nodes, node.GetAddress())
		}
	}
	response.Nodes = nodes

	n.sendJSONResponse(w, response)
}

// handleLeave handles requests to leave the cluster
func (n *Node) handleLeave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LeaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	response := LeaveResponse{
		Success: false,
	}

	// Only the leader can handle leave requests
	if !n.IsLeader {
		response.Message = "Not the leader"
		w.WriteHeader(http.StatusServiceUnavailable)
		n.sendJSONResponse(w, response)
		return
	}

	// Validate the leave request
	if req.NodeID == "" {
		response.Message = "Node ID is required"
		http.Error(w, response.Message, http.StatusBadRequest)
		return
	}

	// Create a command to remove the node from the cluster
	cmd := Command{
		Type:       CommandTypeNodeLeave,
		Repository: "",
		Data: map[string]interface{}{
			"node_id": req.NodeID,
			"reason":  req.Reason,
		},
	}

	// Append to log and replicate
	if err := n.appendAndReplicate(cmd); err != nil {
		response.Message = fmt.Sprintf("Failed to replicate leave command: %v", err)
		http.Error(w, response.Message, http.StatusInternalServerError)
		return
	}

	response.Success = true
	response.Message = "Node successfully left cluster"
	n.sendJSONResponse(w, response)
}

// handleStatus handles cluster status requests
func (n *Node) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	connectedNodes := make([]string, 0)
	clusterSize := 1 // At least this node

	if n.cluster != nil {
		clusterSize = len(n.cluster.GetNodes())
		for _, node := range n.cluster.GetNodes() {
			if node.ID != n.ID {
				connectedNodes = append(connectedNodes, node.GetAddress())
			}
		}
	}

	response := StatusResponse{
		NodeID:         n.ID,
		State:          n.State,
		IsLeader:       n.IsLeader,
		LeaderID:       n.LeaderID,
		Term:           n.raftState.CurrentTerm,
		CommitIndex:    n.raftState.CommitIndex,
		LastApplied:    n.raftState.LastApplied,
		LogLength:      len(n.raftState.Log),
		ClusterSize:    clusterSize,
		ConnectedNodes: connectedNodes,
	}

	n.sendJSONResponse(w, response)
}

// handleRepositoryOperation handles repository operations
func (n *Node) handleRepositoryOperation(w http.ResponseWriter, r *http.Request) {
	// Extract repository ID from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/repo/"), "/")
	if len(pathParts) == 0 {
		http.Error(w, "Repository ID required", http.StatusBadRequest)
		return
	}

	repoID := pathParts[0]
	operation := "read" // Default operation
	
	if len(pathParts) > 1 {
		operation = pathParts[1]
	}

	switch r.Method {
	case http.MethodGet:
		n.handleRepositoryRead(w, r, repoID)
	case http.MethodPost:
		n.handleRepositoryWrite(w, r, repoID, operation)
	case http.MethodPut:
		n.handleRepositoryCreate(w, r, repoID)
	case http.MethodDelete:
		n.handleRepositoryDelete(w, r, repoID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRepositoryRead handles repository read operations
func (n *Node) handleRepositoryRead(w http.ResponseWriter, r *http.Request, repoID string) {
	// Read operations can be handled by any node
	n.mu.RLock()
	defer n.mu.RUnlock()

	repo, exists := n.repositories[repoID]
	if !exists {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Extract the specific operation from query parameters
	operation := r.URL.Query().Get("operation")
	
	switch operation {
	case "status":
		status, err := repo.Status()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		n.sendJSONResponse(w, status)
	case "log":
		commits, err := repo.Log(100) // Limit to 100 commits
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		n.sendJSONResponse(w, commits)
	case "branches":
		branches, err := repo.ListBranches()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		n.sendJSONResponse(w, branches)
	default:
		// Default: return repository info
		info := map[string]interface{}{
			"id":     repoID,
			"exists": true,
			"node":   n.ID,
		}
		n.sendJSONResponse(w, info)
	}
}

// handleRepositoryWrite handles repository write operations
func (n *Node) handleRepositoryWrite(w http.ResponseWriter, r *http.Request, repoID, operation string) {
	// Write operations must go through the leader
	if !n.IsLeader {
		// Redirect to leader
		w.Header().Set("Location", fmt.Sprintf("http://%s/repo/%s/%s", n.getLeaderAddress(), repoID, operation))
		w.WriteHeader(http.StatusTemporaryRedirect)
		return
	}

	var req RepositoryOperation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Create command based on operation
	var cmd Command
	switch operation {
	case "commit":
		cmd = Command{
			Type:       CommandTypeCommit,
			Repository: repoID,
			Data:       req.Data,
		}
	case "branch":
		cmd = Command{
			Type:       CommandTypeCreateBranch,
			Repository: repoID,
			Data:       req.Data,
		}
	case "tag":
		cmd = Command{
			Type:       CommandTypeCreateTag,
			Repository: repoID,
			Data:       req.Data,
		}
	default:
		http.Error(w, "Unknown operation", http.StatusBadRequest)
		return
	}

	// Append to log and replicate
	if err := n.appendAndReplicate(cmd); err != nil {
		http.Error(w, fmt.Sprintf("Failed to replicate command: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success
	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Operation %s completed", operation),
	}
	n.sendJSONResponse(w, response)
}

// handleRepositoryCreate handles repository creation
func (n *Node) handleRepositoryCreate(w http.ResponseWriter, r *http.Request, repoID string) {
	// Repository creation must go through the leader
	if !n.IsLeader {
		// Redirect to leader
		w.Header().Set("Location", fmt.Sprintf("http://%s/repo/%s", n.getLeaderAddress(), repoID))
		w.WriteHeader(http.StatusTemporaryRedirect)
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Create command to create repository
	cmd := Command{
		Type:       CommandTypeCreateRepo,
		Repository: repoID,
		Data:       req,
	}

	// Append to log and replicate
	if err := n.appendAndReplicate(cmd); err != nil {
		http.Error(w, fmt.Sprintf("Failed to replicate command: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success
	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Repository %s created", repoID),
	}
	n.sendJSONResponse(w, response)
}

// handleRepositoryDelete handles repository deletion
func (n *Node) handleRepositoryDelete(w http.ResponseWriter, r *http.Request, repoID string) {
	// Repository deletion must go through the leader
	if !n.IsLeader {
		// Redirect to leader
		w.Header().Set("Location", fmt.Sprintf("http://%s/repo/%s", n.getLeaderAddress(), repoID))
		w.WriteHeader(http.StatusTemporaryRedirect)
		return
	}

	// Create command to delete repository
	cmd := Command{
		Type:       CommandTypeDeleteRepo,
		Repository: repoID,
		Data:       make(map[string]interface{}),
	}

	// Append to log and replicate
	if err := n.appendAndReplicate(cmd); err != nil {
		http.Error(w, fmt.Sprintf("Failed to replicate command: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success
	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Repository %s deleted", repoID),
	}
	n.sendJSONResponse(w, response)
}

// appendAndReplicate appends a command to the log and replicates it
func (n *Node) appendAndReplicate(cmd Command) error {
	n.raftState.mu.Lock()
	defer n.raftState.mu.Unlock()

	// Create new log entry
	entry := LogEntry{
		Term:      n.raftState.CurrentTerm,
		Index:     uint64(len(n.raftState.Log)) + 1,
		Command:   cmd,
		Timestamp: time.Now(),
		ClientID:  "cluster-internal", // For internal commands
	}

	// Append to local log
	n.raftState.Log = append(n.raftState.Log, entry)

	// In a full implementation, this would:
	// 1. Send AppendEntries RPCs to all followers
	// 2. Wait for majority acknowledgment
	// 3. Apply the command once committed
	// 4. Return result to client

	// For now, we'll simulate immediate success
	n.raftState.CommitIndex = entry.Index
	n.applyCommittedEntries()

	return nil
}

// getLeaderAddress returns the address of the current leader
func (n *Node) getLeaderAddress() string {
	if n.cluster == nil {
		return ""
	}

	for _, node := range n.cluster.GetNodes() {
		if node.ID == n.LeaderID {
			return node.GetAddress()
		}
	}

	return ""
}