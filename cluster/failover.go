package cluster

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// FailoverManager handles automatic failover and recovery
type FailoverManager struct {
	cluster           *Cluster
	healthChecker     *HealthChecker
	failoverPolicy    FailoverPolicy
	recoveryManager   *RecoveryManager
	alertManager      *AlertManager
	eventLog          []FailoverEvent
	mu                sync.RWMutex
}

// HealthChecker monitors node health
type HealthChecker struct {
	interval        time.Duration
	timeout         time.Duration
	unhealthyThreshold int
	nodeStatus      map[string]*NodeHealth
	callbacks       []HealthCallback
	mu              sync.RWMutex
}

// RecoveryManager handles node recovery
type RecoveryManager struct {
	cluster         *Cluster
	recoveryQueue   chan RecoveryTask
	activeRecoveries map[string]*RecoveryTask
	maxConcurrent   int
	mu              sync.RWMutex
}

// AlertManager handles alerting for failover events
type AlertManager struct {
	channels    []AlertChannel
	templates   map[AlertType]AlertTemplate
	history     []Alert
	mu          sync.RWMutex
}

// NodeHealth represents the health status of a node
type NodeHealth struct {
	NodeID           string        `json:"node_id"`
	Status           HealthStatus  `json:"status"`
	LastSeen         time.Time     `json:"last_seen"`
	ConsecutiveFails int           `json:"consecutive_fails"`
	ResponseTime     time.Duration `json:"response_time"`
	LoadAverage      float64       `json:"load_average"`
	MemoryUsage      float64       `json:"memory_usage"`
	DiskUsage        float64       `json:"disk_usage"`
	NetworkLatency   time.Duration `json:"network_latency"`
	Errors           []string      `json:"errors"`
}

// HealthStatus represents node health status
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// FailoverPolicy defines failover behavior
type FailoverPolicy struct {
	AutoFailoverEnabled     bool          `yaml:"auto_failover_enabled"`
	FailoverTimeout         time.Duration `yaml:"failover_timeout"`
	MinHealthyNodes         int           `yaml:"min_healthy_nodes"`
	RequireQuorum           bool          `yaml:"require_quorum"`
	PreventSplitBrain       bool          `yaml:"prevent_split_brain"`
	MaxFailoversPerMinute   int           `yaml:"max_failovers_per_minute"`
	CooldownPeriod          time.Duration `yaml:"cooldown_period"`
}

// FailoverEvent represents a failover event
type FailoverEvent struct {
	ID          string           `json:"id"`
	Type        FailoverType     `json:"type"`
	SourceNode  string           `json:"source_node"`
	TargetNode  string           `json:"target_node"`
	Reason      string           `json:"reason"`
	Status      FailoverStatus   `json:"status"`
	StartedAt   time.Time        `json:"started_at"`
	CompletedAt time.Time        `json:"completed_at"`
	Duration    time.Duration    `json:"duration"`
	Errors      []string         `json:"errors"`
	Metadata    map[string]string `json:"metadata"`
}

// FailoverType defines types of failover operations
type FailoverType string

const (
	FailoverTypeNodeFailure     FailoverType = "node_failure"
	FailoverTypeLeaderElection  FailoverType = "leader_election"
	FailoverTypeShardMigration  FailoverType = "shard_migration"
	FailoverTypeLoadBalancing   FailoverType = "load_balancing"
)

// FailoverStatus represents failover operation status
type FailoverStatus string

const (
	FailoverStatusPending   FailoverStatus = "pending"
	FailoverStatusActive    FailoverStatus = "active"
	FailoverStatusCompleted FailoverStatus = "completed"
	FailoverStatusFailed    FailoverStatus = "failed"
	FailoverStatusAborted   FailoverStatus = "aborted"
)

// RecoveryTask represents a node recovery operation
type RecoveryTask struct {
	ID          string           `json:"id"`
	NodeID      string           `json:"node_id"`
	Type        RecoveryType     `json:"type"`
	Status      RecoveryStatus   `json:"status"`
	StartedAt   time.Time        `json:"started_at"`
	CompletedAt time.Time        `json:"completed_at"`
	Progress    int              `json:"progress"`
	Errors      []string         `json:"errors"`
	Metadata    map[string]string `json:"metadata"`
}

// RecoveryType defines types of recovery operations
type RecoveryType string

const (
	RecoveryTypeNodeRestart     RecoveryType = "node_restart"
	RecoveryTypeDataSync        RecoveryType = "data_sync"
	RecoveryTypeShardRecovery   RecoveryType = "shard_recovery"
	RecoveryTypeLeaderPromotion RecoveryType = "leader_promotion"
)

// RecoveryStatus represents recovery operation status
type RecoveryStatus string

const (
	RecoveryStatusPending   RecoveryStatus = "pending"
	RecoveryStatusActive    RecoveryStatus = "active"
	RecoveryStatusCompleted RecoveryStatus = "completed"
	RecoveryStatusFailed    RecoveryStatus = "failed"
)

// HealthCallback is called when node health changes
type HealthCallback func(nodeID string, oldStatus, newStatus HealthStatus)

// AlertChannel defines an alerting channel
type AlertChannel interface {
	Send(alert Alert) error
	GetType() string
}

// AlertType defines types of alerts
type AlertType string

const (
	AlertTypeNodeDown        AlertType = "node_down"
	AlertTypeNodeRecovered   AlertType = "node_recovered"
	AlertTypeFailoverStarted AlertType = "failover_started"
	AlertTypeFailoverComplete AlertType = "failover_complete"
	AlertTypeClusterDegraded AlertType = "cluster_degraded"
)

// AlertTemplate defines alert message templates
type AlertTemplate struct {
	Subject string `yaml:"subject"`
	Body    string `yaml:"body"`
}

// Alert represents an alert message
type Alert struct {
	ID        string                 `json:"id"`
	Type      AlertType              `json:"type"`
	Severity  AlertSeverity          `json:"severity"`
	Message   string                 `json:"message"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
	Resolved  bool                   `json:"resolved"`
}

// AlertSeverity defines alert severity levels
type AlertSeverity string

const (
	AlertSeverityLow      AlertSeverity = "low"
	AlertSeverityMedium   AlertSeverity = "medium"
	AlertSeverityHigh     AlertSeverity = "high"
	AlertSeverityCritical AlertSeverity = "critical"
)

// NewFailoverManager creates a new failover manager
func NewFailoverManager(cluster *Cluster, policy FailoverPolicy) *FailoverManager {
	healthChecker := &HealthChecker{
		interval:           30 * time.Second,
		timeout:            10 * time.Second,
		unhealthyThreshold: 3,
		nodeStatus:         make(map[string]*NodeHealth),
		callbacks:          make([]HealthCallback, 0),
	}

	recoveryManager := &RecoveryManager{
		cluster:          cluster,
		recoveryQueue:    make(chan RecoveryTask, 100),
		activeRecoveries: make(map[string]*RecoveryTask),
		maxConcurrent:    2,
	}

	alertManager := &AlertManager{
		channels:  make([]AlertChannel, 0),
		templates: make(map[AlertType]AlertTemplate),
		history:   make([]Alert, 0),
	}

	fm := &FailoverManager{
		cluster:         cluster,
		healthChecker:   healthChecker,
		failoverPolicy:  policy,
		recoveryManager: recoveryManager,
		alertManager:    alertManager,
		eventLog:        make([]FailoverEvent, 0),
	}

	// Set up health check callback
	healthChecker.RegisterCallback(fm.onHealthChange)

	return fm
}

// Start starts the failover manager
func (fm *FailoverManager) Start(ctx context.Context) error {
	// Start health checking
	go fm.healthChecker.Start(ctx, fm.cluster)
	
	// Start recovery manager
	go fm.recoveryManager.Start(ctx)
	
	// Start failover monitoring
	go fm.runFailoverMonitor(ctx)

	return nil
}

// runFailoverMonitor monitors for failover conditions
func (fm *FailoverManager) runFailoverMonitor(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fm.checkFailoverConditions()
		}
	}
}

// checkFailoverConditions checks if failover is needed
func (fm *FailoverManager) checkFailoverConditions() {
	if !fm.failoverPolicy.AutoFailoverEnabled {
		return
	}

	fm.healthChecker.mu.RLock()
	unhealthyNodes := make([]*NodeHealth, 0)
	healthyNodes := make([]*NodeHealth, 0)

	for _, health := range fm.healthChecker.nodeStatus {
		if health.Status == HealthStatusUnhealthy {
			unhealthyNodes = append(unhealthyNodes, health)
		} else if health.Status == HealthStatusHealthy {
			healthyNodes = append(healthyNodes, health)
		}
	}
	fm.healthChecker.mu.RUnlock()

	// Check minimum healthy nodes requirement
	if len(healthyNodes) < fm.failoverPolicy.MinHealthyNodes {
		fm.triggerClusterDegradedAlert(len(healthyNodes), fm.failoverPolicy.MinHealthyNodes)
		return
	}

	// Check quorum requirement
	if fm.failoverPolicy.RequireQuorum {
		totalNodes := len(fm.cluster.GetNodes())
		if len(healthyNodes) <= totalNodes/2 {
			log.Printf("Insufficient nodes for quorum: %d healthy out of %d total", len(healthyNodes), totalNodes)
			return
		}
	}

	// Process unhealthy nodes
	for _, health := range unhealthyNodes {
		fm.handleNodeFailure(health.NodeID)
	}

	// Check if leader election is needed
	leaderNode := fm.cluster.GetLeaderNode()
	if leaderNode == nil || fm.isNodeUnhealthy(leaderNode.ID) {
		fm.triggerLeaderElection()
	}
}

// handleNodeFailure handles a node failure
func (fm *FailoverManager) handleNodeFailure(nodeID string) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	// Check cooldown period
	if fm.isInCooldown(nodeID) {
		return
	}

	// Create failover event
	event := FailoverEvent{
		ID:         fmt.Sprintf("failover-%d", time.Now().UnixNano()),
		Type:       FailoverTypeNodeFailure,
		SourceNode: nodeID,
		Reason:     "Node health check failed",
		Status:     FailoverStatusPending,
		StartedAt:  time.Now(),
		Metadata:   make(map[string]string),
	}

	fm.eventLog = append(fm.eventLog, event)

	// Find replacement node
	replacementNode := fm.findReplacementNode(nodeID)
	if replacementNode == "" {
		event.Status = FailoverStatusFailed
		event.Errors = append(event.Errors, "No suitable replacement node found")
		return
	}

	event.TargetNode = replacementNode
	event.Status = FailoverStatusActive

	// Migrate shards from failed node
	go fm.migrateNodeShards(nodeID, replacementNode, &event)

	// Send alert
	fm.alertManager.SendAlert(Alert{
		ID:       fmt.Sprintf("alert-%d", time.Now().UnixNano()),
		Type:     AlertTypeNodeDown,
		Severity: AlertSeverityCritical,
		Message:  fmt.Sprintf("Node %s has failed and failover is in progress", nodeID),
		Data: map[string]interface{}{
			"failed_node":      nodeID,
			"replacement_node": replacementNode,
		},
		Timestamp: time.Now(),
	})
}

// triggerLeaderElection triggers a leader election
func (fm *FailoverManager) triggerLeaderElection() {
	// Find the best candidate for leadership
	candidate := fm.findLeaderCandidate()
	if candidate == "" {
		log.Printf("No suitable leader candidate found")
		return
	}

	// Create failover event
	event := FailoverEvent{
		ID:         fmt.Sprintf("leader-election-%d", time.Now().UnixNano()),
		Type:       FailoverTypeLeaderElection,
		TargetNode: candidate,
		Reason:     "Leader election triggered",
		Status:     FailoverStatusActive,
		StartedAt:  time.Now(),
	}

	fm.mu.Lock()
	fm.eventLog = append(fm.eventLog, event)
	fm.mu.Unlock()

	// Promote candidate to leader
	candidateNode := fm.cluster.GetNodeByID(candidate)
	if candidateNode != nil {
		candidateNode.mu.Lock()
		candidateNode.State = NodeStateLeader
		candidateNode.IsLeader = true
		candidateNode.LeaderID = candidate
		candidateNode.mu.Unlock()

		event.Status = FailoverStatusCompleted
		event.CompletedAt = time.Now()
		event.Duration = event.CompletedAt.Sub(event.StartedAt)
	}
}

// migrateNodeShards migrates shards from a failed node
func (fm *FailoverManager) migrateNodeShards(sourceNode, targetNode string, event *FailoverEvent) {
	shardsToMigrate := make([]*Shard, 0)

	// Find shards on the failed node
	for _, shard := range fm.cluster.Shards {
		if shard.PrimaryNode == sourceNode {
			shardsToMigrate = append(shardsToMigrate, shard)
		}
	}

	// Migrate each shard
	for _, shard := range shardsToMigrate {
		// Promote a replica to primary or assign to target node
		if len(shard.ReplicaNodes) > 0 {
			shard.PrimaryNode = shard.ReplicaNodes[0]
			shard.ReplicaNodes = shard.ReplicaNodes[1:]
		} else {
			shard.PrimaryNode = targetNode
		}

		// Move repositories
		sourceNodeObj := fm.cluster.GetNodeByID(sourceNode)
		targetNodeObj := fm.cluster.GetNodeByID(targetNode)

		if sourceNodeObj != nil && targetNodeObj != nil {
			sourceNodeObj.mu.RLock()
			for repoID, repo := range sourceNodeObj.repositories {
				if shard.Repositories[repoID] {
					targetNodeObj.mu.Lock()
					targetNodeObj.repositories[repoID] = repo
					targetNodeObj.mu.Unlock()
				}
			}
			sourceNodeObj.mu.RUnlock()
		}
	}

	// Update event status
	fm.mu.Lock()
	event.Status = FailoverStatusCompleted
	event.CompletedAt = time.Now()
	event.Duration = event.CompletedAt.Sub(event.StartedAt)
	fm.mu.Unlock()

	// Queue node for recovery
	recoveryTask := RecoveryTask{
		ID:        fmt.Sprintf("recovery-%d", time.Now().UnixNano()),
		NodeID:    sourceNode,
		Type:      RecoveryTypeNodeRestart,
		Status:    RecoveryStatusPending,
		StartedAt: time.Now(),
		Metadata:  make(map[string]string),
	}

	select {
	case fm.recoveryManager.recoveryQueue <- recoveryTask:
	default:
		log.Printf("Recovery queue is full, cannot queue recovery for node %s", sourceNode)
	}
}

// Start starts the health checker
func (hc *HealthChecker) Start(ctx context.Context, cluster *Cluster) {
	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.checkAllNodes(cluster)
		}
	}
}

// checkAllNodes checks the health of all nodes
func (hc *HealthChecker) checkAllNodes(cluster *Cluster) {
	nodes := cluster.GetNodes()
	
	for _, node := range nodes {
		health := hc.checkNodeHealth(node)
		
		hc.mu.Lock()
		oldHealth := hc.nodeStatus[node.ID]
		hc.nodeStatus[node.ID] = health
		hc.mu.Unlock()

		// Trigger callbacks if status changed
		if oldHealth == nil || oldHealth.Status != health.Status {
			oldStatus := HealthStatusUnknown
			if oldHealth != nil {
				oldStatus = oldHealth.Status
			}
			
			for _, callback := range hc.callbacks {
				callback(node.ID, oldStatus, health.Status)
			}
		}
	}
}

// checkNodeHealth checks the health of a single node
func (hc *HealthChecker) checkNodeHealth(node *Node) *NodeHealth {
	health := &NodeHealth{
		NodeID:   node.ID,
		LastSeen: time.Now(),
		Status:   HealthStatusHealthy,
		Errors:   make([]string, 0),
	}

	// Check if node is responsive
	ctx, cancel := context.WithTimeout(context.Background(), hc.timeout)
	defer cancel()

	start := time.Now()
	
	// Simulate health check - in production this would be an HTTP request
	// to the node's health endpoint
	isResponsive := hc.pingNode(ctx, node)
	health.ResponseTime = time.Since(start)

	if !isResponsive {
		hc.mu.Lock()
		if oldHealth, exists := hc.nodeStatus[node.ID]; exists {
			health.ConsecutiveFails = oldHealth.ConsecutiveFails + 1
		} else {
			health.ConsecutiveFails = 1
		}
		hc.mu.Unlock()

		if health.ConsecutiveFails >= hc.unhealthyThreshold {
			health.Status = HealthStatusUnhealthy
			health.Errors = append(health.Errors, "Node is not responsive")
		} else {
			health.Status = HealthStatusDegraded
		}
	} else {
		health.ConsecutiveFails = 0
		health.Status = HealthStatusHealthy
	}

	return health
}

// pingNode checks if a node is responsive
func (hc *HealthChecker) pingNode(ctx context.Context, node *Node) bool {
	// Simplified ping - in production this would make an HTTP request
	// to the node's health endpoint
	select {
	case <-ctx.Done():
		return false
	default:
		// Check if node state indicates it's online
		return node.GetState() != NodeStateOffline
	}
}

// RegisterCallback registers a health change callback
func (hc *HealthChecker) RegisterCallback(callback HealthCallback) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.callbacks = append(hc.callbacks, callback)
}

// onHealthChange handles health status changes
func (fm *FailoverManager) onHealthChange(nodeID string, oldStatus, newStatus HealthStatus) {
	log.Printf("Node %s health changed from %s to %s", nodeID, oldStatus, newStatus)

	if newStatus == HealthStatusUnhealthy && oldStatus != HealthStatusUnhealthy {
		// Node became unhealthy
		if fm.failoverPolicy.AutoFailoverEnabled {
			go fm.handleNodeFailure(nodeID)
		}
	} else if newStatus == HealthStatusHealthy && oldStatus == HealthStatusUnhealthy {
		// Node recovered
		fm.alertManager.SendAlert(Alert{
			ID:       fmt.Sprintf("alert-%d", time.Now().UnixNano()),
			Type:     AlertTypeNodeRecovered,
			Severity: AlertSeverityMedium,
			Message:  fmt.Sprintf("Node %s has recovered", nodeID),
			Data: map[string]interface{}{
				"node_id": nodeID,
			},
			Timestamp: time.Now(),
		})
	}
}

// Helper methods
func (fm *FailoverManager) isNodeUnhealthy(nodeID string) bool {
	fm.healthChecker.mu.RLock()
	defer fm.healthChecker.mu.RUnlock()
	
	health, exists := fm.healthChecker.nodeStatus[nodeID]
	return exists && health.Status == HealthStatusUnhealthy
}

func (fm *FailoverManager) isInCooldown(nodeID string) bool {
	// Check if we've done too many failovers recently
	cutoff := time.Now().Add(-fm.failoverPolicy.CooldownPeriod)
	
	count := 0
	for _, event := range fm.eventLog {
		if event.SourceNode == nodeID && event.StartedAt.After(cutoff) {
			count++
		}
	}
	
	return count >= fm.failoverPolicy.MaxFailoversPerMinute
}

func (fm *FailoverManager) findReplacementNode(failedNodeID string) string {
	for _, node := range fm.cluster.GetNodes() {
		if node.ID != failedNodeID && !fm.isNodeUnhealthy(node.ID) {
			return node.ID
		}
	}
	return ""
}

func (fm *FailoverManager) findLeaderCandidate() string {
	// Find the healthiest node to become leader
	var bestCandidate string
	var bestHealth *NodeHealth

	fm.healthChecker.mu.RLock()
	defer fm.healthChecker.mu.RUnlock()

	for nodeID, health := range fm.healthChecker.nodeStatus {
		if health.Status == HealthStatusHealthy {
			if bestHealth == nil || health.ResponseTime < bestHealth.ResponseTime {
				bestCandidate = nodeID
				bestHealth = health
			}
		}
	}

	return bestCandidate
}

func (fm *FailoverManager) triggerClusterDegradedAlert(healthy, required int) {
	fm.alertManager.SendAlert(Alert{
		ID:       fmt.Sprintf("alert-%d", time.Now().UnixNano()),
		Type:     AlertTypeClusterDegraded,
		Severity: AlertSeverityCritical,
		Message:  fmt.Sprintf("Cluster is degraded: only %d healthy nodes out of %d required", healthy, required),
		Data: map[string]interface{}{
			"healthy_nodes":  healthy,
			"required_nodes": required,
		},
		Timestamp: time.Now(),
	})
}

// Start starts the recovery manager
func (rm *RecoveryManager) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-rm.recoveryQueue:
			if len(rm.activeRecoveries) < rm.maxConcurrent {
				rm.mu.Lock()
				rm.activeRecoveries[task.ID] = &task
				rm.mu.Unlock()
				
				go rm.executeRecovery(&task)
			} else {
				// Queue is full, put task back
				select {
				case rm.recoveryQueue <- task:
				default:
					log.Printf("Recovery queue overflow, dropping task %s", task.ID)
				}
			}
		}
	}
}

// executeRecovery executes a recovery task
func (rm *RecoveryManager) executeRecovery(task *RecoveryTask) {
	defer func() {
		rm.mu.Lock()
		delete(rm.activeRecoveries, task.ID)
		rm.mu.Unlock()
	}()

	task.Status = RecoveryStatusActive
	
	switch task.Type {
	case RecoveryTypeNodeRestart:
		rm.executeNodeRestart(task)
	case RecoveryTypeDataSync:
		rm.executeDataSync(task)
	case RecoveryTypeShardRecovery:
		rm.executeShardRecovery(task)
	default:
		task.Status = RecoveryStatusFailed
		task.Errors = append(task.Errors, "Unknown recovery type")
	}

	task.CompletedAt = time.Now()
}

func (rm *RecoveryManager) executeNodeRestart(task *RecoveryTask) {
	// Simulate node restart recovery
	task.Progress = 50
	time.Sleep(2 * time.Second) // Simulate recovery time
	task.Progress = 100
	task.Status = RecoveryStatusCompleted
}

func (rm *RecoveryManager) executeDataSync(task *RecoveryTask) {
	// Simulate data synchronization
	task.Progress = 25
	time.Sleep(1 * time.Second)
	task.Progress = 75
	time.Sleep(1 * time.Second)
	task.Progress = 100
	task.Status = RecoveryStatusCompleted
}

func (rm *RecoveryManager) executeShardRecovery(task *RecoveryTask) {
	// Simulate shard recovery
	task.Progress = 33
	time.Sleep(1 * time.Second)
	task.Progress = 66
	time.Sleep(1 * time.Second)
	task.Progress = 100
	task.Status = RecoveryStatusCompleted
}

// SendAlert sends an alert through all configured channels
func (am *AlertManager) SendAlert(alert Alert) {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.history = append(am.history, alert)

	for _, channel := range am.channels {
		go func(ch AlertChannel, a Alert) {
			if err := ch.Send(a); err != nil {
				log.Printf("Failed to send alert via %s: %v", ch.GetType(), err)
			}
		}(channel, alert)
	}
}

// GetFailoverHistory returns the failover event history
func (fm *FailoverManager) GetFailoverHistory() []FailoverEvent {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	
	// Return a copy to prevent race conditions
	history := make([]FailoverEvent, len(fm.eventLog))
	copy(history, fm.eventLog)
	return history
}

// GetClusterHealth returns overall cluster health
func (fm *FailoverManager) GetClusterHealth() map[string]interface{} {
	fm.healthChecker.mu.RLock()
	defer fm.healthChecker.mu.RUnlock()

	healthyCount := 0
	degradedCount := 0
	unhealthyCount := 0

	for _, health := range fm.healthChecker.nodeStatus {
		switch health.Status {
		case HealthStatusHealthy:
			healthyCount++
		case HealthStatusDegraded:
			degradedCount++
		case HealthStatusUnhealthy:
			unhealthyCount++
		}
	}

	return map[string]interface{}{
		"total_nodes":    len(fm.healthChecker.nodeStatus),
		"healthy_nodes":  healthyCount,
		"degraded_nodes": degradedCount,
		"unhealthy_nodes": unhealthyCount,
		"failover_enabled": fm.failoverPolicy.AutoFailoverEnabled,
	}
}