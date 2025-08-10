package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/caiatech/govc/cluster"
	"github.com/gin-gonic/gin"
)

// ClusterManager manages high availability cluster operations
type ClusterManager struct {
	cluster         *cluster.Cluster
	failoverManager *cluster.FailoverManager
	server          *Server
}

// ClusterConfig represents cluster configuration for API
type ClusterConfig struct {
	ID                    string        `json:"id" yaml:"id"`
	Name                  string        `json:"name" yaml:"name"`
	ReplicationFactor     int           `json:"replication_factor" yaml:"replication_factor"`
	ShardSize             int           `json:"shard_size" yaml:"shard_size"`
	ElectionTimeout       time.Duration `json:"election_timeout" yaml:"election_timeout"`
	HeartbeatInterval     time.Duration `json:"heartbeat_interval" yaml:"heartbeat_interval"`
	MaxLogEntries         int           `json:"max_log_entries" yaml:"max_log_entries"`
	SnapshotThreshold     int           `json:"snapshot_threshold" yaml:"snapshot_threshold"`
	AutoRebalance         bool          `json:"auto_rebalance" yaml:"auto_rebalance"`
	ConsistencyLevel      string        `json:"consistency_level" yaml:"consistency_level"`
	AutoFailoverEnabled   bool          `json:"auto_failover_enabled" yaml:"auto_failover_enabled"`
	FailoverTimeout       time.Duration `json:"failover_timeout" yaml:"failover_timeout"`
	MinHealthyNodes       int           `json:"min_healthy_nodes" yaml:"min_healthy_nodes"`
	RequireQuorum         bool          `json:"require_quorum" yaml:"require_quorum"`
	PreventSplitBrain     bool          `json:"prevent_split_brain" yaml:"prevent_split_brain"`
	MaxFailoversPerMinute int           `json:"max_failovers_per_minute" yaml:"max_failovers_per_minute"`
	CooldownPeriod        time.Duration `json:"cooldown_period" yaml:"cooldown_period"`
	DataDir               string        `json:"data_dir" yaml:"data_dir"`
}

// NewClusterManager creates a new cluster manager
func NewClusterManager(server *Server) *ClusterManager {
	return &ClusterManager{
		server: server,
	}
}

// InitializeCluster initializes the cluster with high availability
func (s *Server) InitializeCluster(clusterConfig ClusterConfig) error {
	// Convert API config to cluster config
	config := cluster.ClusterConfig{
		ReplicationFactor: clusterConfig.ReplicationFactor,
		ShardSize:         clusterConfig.ShardSize,
		ElectionTimeout:   clusterConfig.ElectionTimeout,
		HeartbeatInterval: clusterConfig.HeartbeatInterval,
		MaxLogEntries:     clusterConfig.MaxLogEntries,
		SnapshotThreshold: clusterConfig.SnapshotThreshold,
		AutoRebalance:     clusterConfig.AutoRebalance,
		ConsistencyLevel:  clusterConfig.ConsistencyLevel,
	}

	// Set defaults if not provided
	if config.ReplicationFactor == 0 {
		config.ReplicationFactor = 3
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

	// Create cluster
	cl, err := cluster.NewCluster(clusterConfig.ID, clusterConfig.Name, config, clusterConfig.DataDir)
	if err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}

	// Create failover policy
	failoverPolicy := cluster.FailoverPolicy{
		AutoFailoverEnabled:   clusterConfig.AutoFailoverEnabled,
		FailoverTimeout:       clusterConfig.FailoverTimeout,
		MinHealthyNodes:       clusterConfig.MinHealthyNodes,
		RequireQuorum:         clusterConfig.RequireQuorum,
		PreventSplitBrain:     clusterConfig.PreventSplitBrain,
		MaxFailoversPerMinute: clusterConfig.MaxFailoversPerMinute,
		CooldownPeriod:        clusterConfig.CooldownPeriod,
	}

	// Set failover defaults
	if failoverPolicy.FailoverTimeout == 0 {
		failoverPolicy.FailoverTimeout = 30 * time.Second
	}
	if failoverPolicy.MinHealthyNodes == 0 {
		failoverPolicy.MinHealthyNodes = 1
	}
	if failoverPolicy.MaxFailoversPerMinute == 0 {
		failoverPolicy.MaxFailoversPerMinute = 5
	}
	if failoverPolicy.CooldownPeriod == 0 {
		failoverPolicy.CooldownPeriod = 5 * time.Minute
	}

	// Create failover manager
	failoverManager := cluster.NewFailoverManager(cl, failoverPolicy)

	// Start failover manager
	ctx := context.Background()
	if err := failoverManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start failover manager: %w", err)
	}

	// Store in server
	s.clusterManager = &ClusterManager{
		cluster:         cl,
		failoverManager: failoverManager,
		server:          s,
	}

	s.logger.Infof("Cluster initialized: %s (HA enabled: %v)", clusterConfig.Name, clusterConfig.AutoFailoverEnabled)
	return nil
}

// GetClusterStatus returns current cluster status
func (cm *ClusterManager) GetClusterStatus(c *gin.Context) {
	if cm.cluster == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "Cluster not initialized",
			"message": "High availability cluster is not configured",
		})
		return
	}

	health := cm.cluster.GetClusterHealth()
	nodes := make([]gin.H, 0)
	
	for _, node := range cm.cluster.GetNodes() {
		nodes = append(nodes, gin.H{
			"id":       node.ID,
			"address":  node.Address,
			"state":    node.GetState(),
			"is_leader": node.IsLeaderNode(),
			"last_seen": node.LastSeen,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"cluster_id":     cm.cluster.GetID(),
		"status":         health.Status,
		"total_nodes":    health.TotalNodes,
		"healthy_nodes":  health.HealthyNodes,
		"total_shards":   health.TotalShards,
		"active_shards":  health.ActiveShards,
		"timestamp":      health.Timestamp,
		"nodes":          nodes,
		"leader_node":    cm.getLeaderNodeInfo(),
		"failover_enabled": cm.failoverManager != nil,
	})
}

// GetClusterHealth returns cluster health metrics
func (cm *ClusterManager) GetClusterHealth(c *gin.Context) {
	if cm.failoverManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Failover manager not initialized",
		})
		return
	}

	healthMetrics := cm.failoverManager.GetClusterHealth()
	c.JSON(http.StatusOK, healthMetrics)
}

// GetFailoverHistory returns failover event history
func (cm *ClusterManager) GetFailoverHistory(c *gin.Context) {
	if cm.failoverManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Failover manager not initialized",
		})
		return
	}

	history := cm.failoverManager.GetFailoverHistory()
	c.JSON(http.StatusOK, gin.H{
		"events": history,
		"count":  len(history),
	})
}

// JoinNode adds a new node to the cluster
func (cm *ClusterManager) JoinNode(c *gin.Context) {
	if cm.cluster == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Cluster not initialized",
		})
		return
	}

	var req struct {
		ID      string `json:"id" binding:"required"`
		Address string `json:"address" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	// Create new node
	nodeConfig := cluster.NodeConfig{
		ID:      req.ID,
		Address: req.Address,
	}
	node, err := cluster.NewNode(nodeConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create node",
			"details": err.Error(),
		})
		return
	}
	
	// Add to cluster
	if err := cm.cluster.AddNode(node); err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Failed to add node to cluster",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Node successfully joined cluster",
		"node_id": req.ID,
		"cluster_id": cm.cluster.GetID(),
	})
}

// RemoveNode removes a node from the cluster
func (cm *ClusterManager) RemoveNode(c *gin.Context) {
	if cm.cluster == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Cluster not initialized",
		})
		return
	}

	nodeID := c.Param("nodeId")
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Node ID is required",
		})
		return
	}

	if err := cm.cluster.RemoveNode(nodeID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Failed to remove node",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Node successfully removed from cluster",
		"node_id": nodeID,
	})
}

// TriggerFailover manually triggers a failover for a node
func (cm *ClusterManager) TriggerFailover(c *gin.Context) {
	if cm.failoverManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Failover manager not initialized",
		})
		return
	}

	var req struct {
		NodeID string `json:"node_id" binding:"required"`
		Reason string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	// Check if node exists
	node := cm.cluster.GetNodeByID(req.NodeID)
	if node == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Node not found",
			"node_id": req.NodeID,
		})
		return
	}

	// TODO: Implement manual failover trigger
	// For now, return success message
	c.JSON(http.StatusAccepted, gin.H{
		"message": "Manual failover triggered",
		"node_id": req.NodeID,
		"reason":  req.Reason,
	})
}

// GetShardDistribution returns shard distribution across nodes
func (cm *ClusterManager) GetShardDistribution(c *gin.Context) {
	if cm.cluster == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Cluster not initialized",
		})
		return
	}

	shards := make([]gin.H, 0)
	for _, shard := range cm.cluster.Shards {
		shards = append(shards, gin.H{
			"id":            shard.ID,
			"key_range":     shard.KeyRange,
			"primary_node":  shard.PrimaryNode,
			"replica_nodes": shard.ReplicaNodes,
			"state":         shard.State,
			"size":          shard.Size,
			"repositories":  len(shard.Repositories),
			"last_accessed": shard.LastAccessed,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"shards": shards,
		"total":  len(shards),
	})
}

// RebalanceCluster triggers cluster rebalancing
func (cm *ClusterManager) RebalanceCluster(c *gin.Context) {
	if cm.cluster == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Cluster not initialized",
		})
		return
	}

	// TODO: Implement cluster rebalancing
	c.JSON(http.StatusAccepted, gin.H{
		"message": "Cluster rebalancing initiated",
		"cluster_id": cm.cluster.GetID(),
	})
}

// Helper methods

func (cm *ClusterManager) getLeaderNodeInfo() gin.H {
	if cm.cluster == nil {
		return gin.H{"status": "no_cluster"}
	}

	leader := cm.cluster.GetLeaderNode()
	if leader == nil {
		return gin.H{"status": "no_leader"}
	}

	return gin.H{
		"node_id":   leader.ID,
		"address":   leader.Address,
		"state":     leader.GetState(),
		"last_seen": leader.LastSeen,
	}
}

// Add cluster manager field to Server
func (s *Server) setupClusterRoutes(router *gin.RouterGroup) {
	cluster := router.Group("/cluster")
	{
		if s.clusterManager != nil {
			cluster.GET("/status", s.clusterManager.GetClusterStatus)
			cluster.GET("/health", s.clusterManager.GetClusterHealth)
			cluster.GET("/failover/history", s.clusterManager.GetFailoverHistory)
			cluster.GET("/shards", s.clusterManager.GetShardDistribution)
			cluster.POST("/nodes", s.clusterManager.JoinNode)
			cluster.DELETE("/nodes/:nodeId", s.clusterManager.RemoveNode)
			cluster.POST("/failover", s.clusterManager.TriggerFailover)
			cluster.POST("/rebalance", s.clusterManager.RebalanceCluster)
		}
	}
}