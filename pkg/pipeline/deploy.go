package pipeline

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// NativeDeployEngine provides memory-first deployment with cross-tool intelligence
type NativeDeployEngine struct {
	// Shared memory with other pipeline tools
	sharedMemory *SharedPipelineMemory
	
	// Deployment targets
	targets map[string]DeploymentTarget
	
	// Deployment strategies
	strategies map[string]DeploymentStrategy
	
	// Active deployments
	activeDeployments map[string]*DeploymentStatus
	
	// Deployment history
	history []*DeploymentRecord
	
	mu sync.RWMutex
}

// DeploymentTarget represents where to deploy
type DeploymentTarget interface {
	Name() string
	Type() string // "kubernetes", "docker", "lambda", "vm", "static"
	Validate(artifacts map[string][]byte) error
	Deploy(ctx context.Context, artifacts map[string][]byte, config DeployConfig) (*DeploymentResult, error)
	Rollback(deploymentID string) error
	HealthCheck() error
}

// DeploymentStrategy defines how to deploy
type DeploymentStrategy interface {
	Name() string
	Execute(ctx context.Context, target DeploymentTarget, artifacts map[string][]byte) (*DeploymentResult, error)
	SupportsTarget(targetType string) bool
}

// DeployConfig contains deployment configuration
type DeployConfig struct {
	Target       string            `json:"target"`
	Strategy     string            `json:"strategy"`
	Environment  string            `json:"environment"`
	Version      string            `json:"version"`
	Tags         []string          `json:"tags"`
	Secrets      map[string]string `json:"-"` // Never logged
	Timeout      time.Duration     `json:"timeout"`
	AutoRollback bool              `json:"auto_rollback"`
	HealthCheck  *HealthCheckConfig `json:"health_check"`
}

// HealthCheckConfig defines health check parameters
type HealthCheckConfig struct {
	Enabled      bool          `json:"enabled"`
	Endpoint     string        `json:"endpoint"`
	Interval     time.Duration `json:"interval"`
	Timeout      time.Duration `json:"timeout"`
	Retries      int           `json:"retries"`
	SuccessCount int           `json:"success_count"`
}

// DeploymentResult contains deployment outcome
type DeploymentResult struct {
	Success      bool              `json:"success"`
	DeploymentID string            `json:"deployment_id"`
	Target       string            `json:"target"`
	Environment  string            `json:"environment"`
	Version      string            `json:"version"`
	URL          string            `json:"url,omitempty"`
	Endpoints    []string          `json:"endpoints,omitempty"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time"`
	Duration     time.Duration     `json:"duration"`
	HealthStatus string            `json:"health_status"`
	Metadata     map[string]string `json:"metadata"`
	Error        error             `json:"error,omitempty"`
}

// DeploymentStatus tracks active deployment
type DeploymentStatus struct {
	ID          string    `json:"id"`
	State       string    `json:"state"` // "pending", "deploying", "verifying", "completed", "failed", "rolled_back"
	Progress    int       `json:"progress"`
	StartTime   time.Time `json:"start_time"`
	UpdatedTime time.Time `json:"updated_time"`
	Message     string    `json:"message"`
}

// DeploymentRecord is historical record
type DeploymentRecord struct {
	Result    *DeploymentResult
	Config    DeployConfig
	Artifacts []string
	Timestamp time.Time
}

// NewNativeDeployEngine creates a new deployment engine
func NewNativeDeployEngine(sharedMem *SharedPipelineMemory) *NativeDeployEngine {
	return &NativeDeployEngine{
		sharedMemory:      sharedMem,
		targets:           make(map[string]DeploymentTarget),
		strategies:        make(map[string]DeploymentStrategy),
		activeDeployments: make(map[string]*DeploymentStatus),
		history:          make([]*DeploymentRecord, 0),
	}
}

// DeployFromMemory deploys artifacts directly from shared memory
func (de *NativeDeployEngine) DeployFromMemory(ctx context.Context, config DeployConfig) (*DeploymentResult, error) {
	startTime := time.Now()
	deploymentID := fmt.Sprintf("deploy-%d", startTime.Unix())
	
	// Create deployment status
	status := &DeploymentStatus{
		ID:        deploymentID,
		State:     "pending",
		Progress:  0,
		StartTime: startTime,
		Message:   "Initializing deployment",
	}
	de.trackDeployment(deploymentID, status)
	
	// Step 1: Pre-deployment checks using shared memory intelligence
	if err := de.performPreDeploymentChecks(); err != nil {
		return de.failDeployment(deploymentID, err)
	}
	de.updateDeploymentStatus(deploymentID, "deploying", 20, "Pre-deployment checks passed")
	
	// Step 2: Get artifacts from shared memory (instant, no I/O!)
	artifacts, err := de.getArtifactsFromMemory()
	if err != nil {
		return de.failDeployment(deploymentID, err)
	}
	de.updateDeploymentStatus(deploymentID, "deploying", 40, "Artifacts retrieved from memory")
	
	// Step 3: Select deployment target
	target, err := de.selectTarget(config.Target)
	if err != nil {
		return de.failDeployment(deploymentID, err)
	}
	
	// Step 4: Select deployment strategy
	strategy, err := de.selectStrategy(config.Strategy, target)
	if err != nil {
		return de.failDeployment(deploymentID, err)
	}
	de.updateDeploymentStatus(deploymentID, "deploying", 60, "Deploying to "+target.Name())
	
	// Step 5: Execute deployment
	result, err := strategy.Execute(ctx, target, artifacts)
	if err != nil {
		if config.AutoRollback {
			de.performRollback(deploymentID, target)
		}
		return de.failDeployment(deploymentID, err)
	}
	de.updateDeploymentStatus(deploymentID, "verifying", 80, "Deployment complete, verifying")
	
	// Step 6: Health check
	if config.HealthCheck != nil && config.HealthCheck.Enabled {
		if err := de.performHealthCheck(target, config.HealthCheck); err != nil {
			if config.AutoRollback {
				de.performRollback(deploymentID, target)
			}
			return de.failDeployment(deploymentID, fmt.Errorf("health check failed: %w", err))
		}
	}
	
	// Step 7: Mark successful and store in shared memory
	result.Success = true
	result.DeploymentID = deploymentID
	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()
	result.HealthStatus = "healthy"
	
	de.updateDeploymentStatus(deploymentID, "completed", 100, "Deployment successful")
	de.storeInSharedMemory(result)
	de.recordDeployment(result, config, artifacts)
	
	return result, nil
}

// performPreDeploymentChecks uses cross-tool intelligence
func (de *NativeDeployEngine) performPreDeploymentChecks() error {
	de.sharedMemory.mu.RLock()
	defer de.sharedMemory.mu.RUnlock()
	
	// Check security scan results
	if de.sharedMemory.SecurityIssues != nil {
		criticalCount := 0
		for _, issue := range de.sharedMemory.SecurityIssues {
			if issue.Severity == "CRITICAL" {
				criticalCount++
			}
		}
		if criticalCount > 0 {
			return fmt.Errorf("deployment blocked: %d critical security issues found", criticalCount)
		}
	}
	
	// Check test results
	if de.sharedMemory.TestResults != nil {
		if de.sharedMemory.TestResults.Failed > 0 {
			return fmt.Errorf("deployment blocked: %d tests failed", de.sharedMemory.TestResults.Failed)
		}
	}
	
	// Check coverage
	if de.sharedMemory.Coverage != nil {
		if de.sharedMemory.Coverage.SecurityCoverage < 100 {
			return fmt.Errorf("deployment blocked: security vulnerabilities have only %.1f%% test coverage",
				de.sharedMemory.Coverage.SecurityCoverage)
		}
		if de.sharedMemory.Coverage.Coverage < 70 {
			return fmt.Errorf("deployment blocked: overall coverage %.1f%% is below 70%% threshold",
				de.sharedMemory.Coverage.Coverage)
		}
	}
	
	return nil
}

// getArtifactsFromMemory retrieves build artifacts from shared memory
func (de *NativeDeployEngine) getArtifactsFromMemory() (map[string][]byte, error) {
	de.sharedMemory.mu.RLock()
	defer de.sharedMemory.mu.RUnlock()
	
	if de.sharedMemory.BuildArtifacts == nil || len(de.sharedMemory.BuildArtifacts) == 0 {
		return nil, fmt.Errorf("no build artifacts found in shared memory")
	}
	
	// Direct memory access - no serialization!
	return de.sharedMemory.BuildArtifacts, nil
}

// selectTarget chooses deployment target
func (de *NativeDeployEngine) selectTarget(targetName string) (DeploymentTarget, error) {
	de.mu.RLock()
	defer de.mu.RUnlock()
	
	target, exists := de.targets[targetName]
	if !exists {
		// Default to local deployment
		if targetName == "" || targetName == "local" || strings.Contains(targetName, "local") {
			return NewLocalDeploymentTarget(), nil
		}
		
		// For govc, always use local deployment
		return NewLocalDeploymentTarget(), nil
	}
	
	return target, nil
}

// selectStrategy chooses deployment strategy
func (de *NativeDeployEngine) selectStrategy(strategyName string, target DeploymentTarget) (DeploymentStrategy, error) {
	de.mu.RLock()
	defer de.mu.RUnlock()
	
	// Default strategies
	if strategyName == "" {
		switch target.Type() {
		case "kubernetes":
			strategyName = "rolling"
		case "lambda":
			strategyName = "immediate"
		default:
			strategyName = "blue-green"
		}
	}
	
	strategy, exists := de.strategies[strategyName]
	if !exists {
		// Create default strategies
		switch strategyName {
		case "rolling":
			return NewRollingStrategy(), nil
		case "blue-green":
			return NewBlueGreenStrategy(), nil
		case "canary":
			return NewCanaryStrategy(), nil
		case "immediate":
			return NewImmediateStrategy(), nil
		default:
			return nil, fmt.Errorf("deployment strategy %s not found", strategyName)
		}
	}
	
	if !strategy.SupportsTarget(target.Type()) {
		return nil, fmt.Errorf("strategy %s does not support target type %s", strategyName, target.Type())
	}
	
	return strategy, nil
}

// performHealthCheck verifies deployment health
func (de *NativeDeployEngine) performHealthCheck(target DeploymentTarget, config *HealthCheckConfig) error {
	successCount := 0
	
	for i := 0; i < config.Retries; i++ {
		if err := target.HealthCheck(); err == nil {
			successCount++
			if successCount >= config.SuccessCount {
				return nil
			}
		}
		time.Sleep(config.Interval)
	}
	
	return fmt.Errorf("health check failed after %d retries", config.Retries)
}

// performRollback rolls back deployment
func (de *NativeDeployEngine) performRollback(deploymentID string, target DeploymentTarget) {
	de.updateDeploymentStatus(deploymentID, "rolling_back", 90, "Rolling back deployment")
	
	if err := target.Rollback(deploymentID); err != nil {
		de.updateDeploymentStatus(deploymentID, "rollback_failed", 95, 
			fmt.Sprintf("Rollback failed: %v", err))
	} else {
		de.updateDeploymentStatus(deploymentID, "rolled_back", 100, "Successfully rolled back")
	}
}

// storeInSharedMemory stores deployment result for other tools
func (de *NativeDeployEngine) storeInSharedMemory(result *DeploymentResult) {
	de.sharedMemory.mu.Lock()
	defer de.sharedMemory.mu.Unlock()
	
	// Store deployment info in shared memory
	// Other tools can instantly see deployment status!
	if de.sharedMemory.Deployments == nil {
		de.sharedMemory.Deployments = make([]*DeploymentResult, 0)
	}
	de.sharedMemory.Deployments = append(de.sharedMemory.Deployments, result)
}

// Helper methods

func (de *NativeDeployEngine) trackDeployment(id string, status *DeploymentStatus) {
	de.mu.Lock()
	defer de.mu.Unlock()
	de.activeDeployments[id] = status
}

func (de *NativeDeployEngine) updateDeploymentStatus(id, state string, progress int, message string) {
	de.mu.Lock()
	defer de.mu.Unlock()
	
	if status, exists := de.activeDeployments[id]; exists {
		status.State = state
		status.Progress = progress
		status.Message = message
		status.UpdatedTime = time.Now()
	}
}

func (de *NativeDeployEngine) failDeployment(id string, err error) (*DeploymentResult, error) {
	de.updateDeploymentStatus(id, "failed", 100, err.Error())
	
	return &DeploymentResult{
		Success:      false,
		DeploymentID: id,
		Error:        err,
		EndTime:      time.Now(),
	}, err
}

func (de *NativeDeployEngine) recordDeployment(result *DeploymentResult, config DeployConfig, artifacts map[string][]byte) {
	de.mu.Lock()
	defer de.mu.Unlock()
	
	artifactNames := make([]string, 0, len(artifacts))
	for name := range artifacts {
		artifactNames = append(artifactNames, name)
	}
	
	record := &DeploymentRecord{
		Result:    result,
		Config:    config,
		Artifacts: artifactNames,
		Timestamp: time.Now(),
	}
	
	de.history = append(de.history, record)
	
	// Keep only last 100 deployments
	if len(de.history) > 100 {
		de.history = de.history[len(de.history)-100:]
	}
}

// RegisterTarget adds a deployment target
func (de *NativeDeployEngine) RegisterTarget(name string, target DeploymentTarget) {
	de.mu.Lock()
	defer de.mu.Unlock()
	de.targets[name] = target
}

// RegisterStrategy adds a deployment strategy
func (de *NativeDeployEngine) RegisterStrategy(name string, strategy DeploymentStrategy) {
	de.mu.Lock()
	defer de.mu.Unlock()
	de.strategies[name] = strategy
}

// GetDeploymentStatus returns current deployment status
func (de *NativeDeployEngine) GetDeploymentStatus(deploymentID string) (*DeploymentStatus, bool) {
	de.mu.RLock()
	defer de.mu.RUnlock()
	
	status, exists := de.activeDeployments[deploymentID]
	return status, exists
}

// GetDeploymentHistory returns deployment history
func (de *NativeDeployEngine) GetDeploymentHistory(limit int) []*DeploymentRecord {
	de.mu.RLock()
	defer de.mu.RUnlock()
	
	if limit <= 0 || limit > len(de.history) {
		limit = len(de.history)
	}
	
	// Return most recent deployments
	start := len(de.history) - limit
	if start < 0 {
		start = 0
	}
	
	return de.history[start:]
}

// Default deployment targets (simplified implementations)

// KubernetesTarget deploys to Kubernetes
type KubernetesTarget struct{}

func NewKubernetesTarget() *KubernetesTarget {
	return &KubernetesTarget{}
}

func (k *KubernetesTarget) Name() string { return "kubernetes" }
func (k *KubernetesTarget) Type() string { return "kubernetes" }

func (k *KubernetesTarget) Validate(artifacts map[string][]byte) error {
	// Validate Kubernetes manifests
	return nil
}

func (k *KubernetesTarget) Deploy(ctx context.Context, artifacts map[string][]byte, config DeployConfig) (*DeploymentResult, error) {
	// Simulate Kubernetes deployment
	return &DeploymentResult{
		Success:     true,
		Target:      "kubernetes",
		Environment: config.Environment,
		Version:     config.Version,
		URL:         fmt.Sprintf("https://app-%s.k8s.example.com", config.Environment),
		StartTime:   time.Now(),
	}, nil
}

func (k *KubernetesTarget) Rollback(deploymentID string) error {
	// Simulate rollback
	return nil
}

func (k *KubernetesTarget) HealthCheck() error {
	// Simulate health check
	return nil
}

// DockerTarget deploys Docker containers
type DockerTarget struct{}

func NewDockerTarget() *DockerTarget {
	return &DockerTarget{}
}

func (d *DockerTarget) Name() string { return "docker" }
func (d *DockerTarget) Type() string { return "docker" }

func (d *DockerTarget) Validate(artifacts map[string][]byte) error {
	return nil
}

func (d *DockerTarget) Deploy(ctx context.Context, artifacts map[string][]byte, config DeployConfig) (*DeploymentResult, error) {
	return &DeploymentResult{
		Success:     true,
		Target:      "docker",
		Environment: config.Environment,
		Version:     config.Version,
		StartTime:   time.Now(),
	}, nil
}

func (d *DockerTarget) Rollback(deploymentID string) error {
	return nil
}

func (d *DockerTarget) HealthCheck() error {
	return nil
}

// LambdaTarget deploys to AWS Lambda
type LambdaTarget struct{}

func NewLambdaTarget() *LambdaTarget {
	return &LambdaTarget{}
}

func (l *LambdaTarget) Name() string { return "lambda" }
func (l *LambdaTarget) Type() string { return "lambda" }

func (l *LambdaTarget) Validate(artifacts map[string][]byte) error {
	return nil
}

func (l *LambdaTarget) Deploy(ctx context.Context, artifacts map[string][]byte, config DeployConfig) (*DeploymentResult, error) {
	return &DeploymentResult{
		Success:     true,
		Target:      "lambda",
		Environment: config.Environment,
		Version:     config.Version,
		Endpoints: []string{
			fmt.Sprintf("https://api.amazonaws.com/lambda/%s", config.Version),
		},
		StartTime: time.Now(),
	}, nil
}

func (l *LambdaTarget) Rollback(deploymentID string) error {
	return nil
}

func (l *LambdaTarget) HealthCheck() error {
	return nil
}

// Default deployment strategies

// RollingStrategy performs rolling updates
type RollingStrategy struct{}

func NewRollingStrategy() *RollingStrategy {
	return &RollingStrategy{}
}

func (r *RollingStrategy) Name() string { return "rolling" }

func (r *RollingStrategy) Execute(ctx context.Context, target DeploymentTarget, artifacts map[string][]byte) (*DeploymentResult, error) {
	// Simulate rolling deployment
	return target.Deploy(ctx, artifacts, DeployConfig{Strategy: "rolling"})
}

func (r *RollingStrategy) SupportsTarget(targetType string) bool {
	return targetType == "kubernetes" || targetType == "docker"
}

// BlueGreenStrategy performs blue-green deployments
type BlueGreenStrategy struct{}

func NewBlueGreenStrategy() *BlueGreenStrategy {
	return &BlueGreenStrategy{}
}

func (b *BlueGreenStrategy) Name() string { return "blue-green" }

func (b *BlueGreenStrategy) Execute(ctx context.Context, target DeploymentTarget, artifacts map[string][]byte) (*DeploymentResult, error) {
	// Simulate blue-green deployment
	return target.Deploy(ctx, artifacts, DeployConfig{Strategy: "blue-green"})
}

func (b *BlueGreenStrategy) SupportsTarget(targetType string) bool {
	return true // Supports all targets
}

// CanaryStrategy performs canary deployments
type CanaryStrategy struct{}

func NewCanaryStrategy() *CanaryStrategy {
	return &CanaryStrategy{}
}

func (c *CanaryStrategy) Name() string { return "canary" }

func (c *CanaryStrategy) Execute(ctx context.Context, target DeploymentTarget, artifacts map[string][]byte) (*DeploymentResult, error) {
	// Simulate canary deployment
	return target.Deploy(ctx, artifacts, DeployConfig{Strategy: "canary"})
}

func (c *CanaryStrategy) SupportsTarget(targetType string) bool {
	return targetType == "kubernetes" || targetType == "lambda"
}

// ImmediateStrategy performs immediate deployments
type ImmediateStrategy struct{}

func NewImmediateStrategy() *ImmediateStrategy {
	return &ImmediateStrategy{}
}

func (i *ImmediateStrategy) Name() string { return "immediate" }

func (i *ImmediateStrategy) Execute(ctx context.Context, target DeploymentTarget, artifacts map[string][]byte) (*DeploymentResult, error) {
	// Immediate deployment without staging
	return target.Deploy(ctx, artifacts, DeployConfig{Strategy: "immediate"})
}

func (i *ImmediateStrategy) SupportsTarget(targetType string) bool {
	return true // Supports all targets
}