package container

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/caiatech/govc"
	"github.com/google/uuid"
)

// Manager handles all container-related operations for govc repositories
type Manager struct {
	repos          map[string]*govc.Repository
	builds         map[string]*Build
	eventHandlers  []func(ContainerEvent)
	securityPolicy *SecurityPolicy
	mu             sync.RWMutex
}

// NewManager creates a new container manager
func NewManager() *Manager {
	return &Manager{
		repos:         make(map[string]*govc.Repository),
		builds:        make(map[string]*Build),
		eventHandlers: make([]func(ContainerEvent), 0),
	}
}

// RegisterRepository registers a govc repository for container operations
func (m *Manager) RegisterRepository(id string, repo *govc.Repository) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.repos[id]; exists {
		return fmt.Errorf("repository %s already registered", id)
	}

	m.repos[id] = repo

	// Set up commit watcher for container-related changes
	go m.watchRepository(id, repo)

	return nil
}

// watchRepository monitors a repository for container-related changes
func (m *Manager) watchRepository(repoID string, repo *govc.Repository) {
	// Watch for commits that affect container definitions
	repo.Watch(func(event govc.CommitEvent) {
		m.handleCommitEvent(repoID, event)
	})
}

// handleCommitEvent processes commits that might affect container definitions
func (m *Manager) handleCommitEvent(repoID string, event govc.CommitEvent) {
	// Check if any container-related files were changed
	for _, file := range event.Changes {
		if m.isContainerDefinition(file) {
			// Emit container event
			m.emitEvent(ContainerEvent{
				Type:         EventTypeBuildStarted,
				RepositoryID: repoID,
				Timestamp:    time.Now(),
				Actor:        event.Author,
				Data: map[string]interface{}{
					"file":   file,
					"commit": event.Hash,
				},
			})

			// Trigger build if auto-build is enabled
			if m.shouldAutoBuild(file) {
				go m.triggerBuild(repoID, file, event)
			}
		}
	}
}

// isContainerDefinition checks if a file is a container definition
func (m *Manager) isContainerDefinition(path string) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(path)

	// Check for Govcfiles
	if strings.HasPrefix(strings.ToLower(base), "govcfile") {
		return true
	}
	
	// Legacy support for Dockerfile (will be converted to Govcfile)
	if strings.HasPrefix(strings.ToLower(base), "dockerfile") {
		return true
	}

	// Check for govc-compose files
	if strings.Contains(base, "govc-compose") && (ext == ".yml" || ext == ".yaml") {
		return true
	}

	// Check for Kubernetes manifests
	if ext == ".yaml" || ext == ".yml" {
		// Simple heuristic: check if path contains k8s-related terms
		lower := strings.ToLower(path)
		if strings.Contains(lower, "k8s") || strings.Contains(lower, "kubernetes") ||
			strings.Contains(lower, "deployment") || strings.Contains(lower, "service") {
			return true
		}
	}

	// Check for Helm charts
	if base == "Chart.yaml" || base == "values.yaml" {
		return true
	}

	return false
}

// shouldAutoBuild determines if a file change should trigger an automatic build
func (m *Manager) shouldAutoBuild(path string) bool {
	// Auto-build Govcfiles and legacy Dockerfiles
	base := filepath.Base(path)
	return strings.HasPrefix(strings.ToLower(base), "govcfile") || 
		strings.HasPrefix(strings.ToLower(base), "dockerfile")
}

// triggerBuild initiates a container build
func (m *Manager) triggerBuild(repoID, containerfilePath string, event govc.CommitEvent) {
	buildID := uuid.New().String()
	
	build := &Build{
		ID:           buildID,
		RepositoryID: repoID,
		Request: BuildRequest{
			RepositoryID: repoID,
			Context:      filepath.Dir(containerfilePath),
			Govcfile:     containerfilePath,
			Tags:         []string{fmt.Sprintf("%s:latest", repoID)},
		},
		Status:    BuildStatusPending,
		StartTime: time.Now(),
	}

	m.mu.Lock()
	m.builds[buildID] = build
	m.mu.Unlock()

	// Emit build started event
	m.emitEvent(ContainerEvent{
		Type:         EventTypeBuildStarted,
		RepositoryID: repoID,
		Timestamp:    time.Now(),
		Resource:     build,
	})

	// TODO: Implement actual build logic
	// For now, we'll simulate a build
	go m.executeBuild(build)
}

// executeBuild performs the actual container build
func (m *Manager) executeBuild(build *Build) {
	// Update status to running
	m.updateBuildStatus(build.ID, BuildStatusRunning)

	// Simulate build process
	// TODO: Integrate with actual container build engine
	time.Sleep(2 * time.Second)

	// Mark as completed
	now := time.Now()
	build.EndTime = &now
	build.Status = BuildStatusCompleted
	build.ImageID = fmt.Sprintf("sha256:%s", uuid.New().String()[:12])
	build.Digest = build.ImageID

	m.updateBuildStatus(build.ID, BuildStatusCompleted)

	// Emit build completed event
	m.emitEvent(ContainerEvent{
		Type:         EventTypeBuildCompleted,
		RepositoryID: build.RepositoryID,
		Timestamp:    time.Now(),
		Resource:     build,
	})
}

// updateBuildStatus updates the status of a build
func (m *Manager) updateBuildStatus(buildID string, status BuildStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if build, exists := m.builds[buildID]; exists {
		build.Status = status
	}
}

// GetContainerDefinitions retrieves all container definitions from a repository
func (m *Manager) GetContainerDefinitions(repoID string) ([]*ContainerDefinition, error) {
	m.mu.RLock()
	repo, exists := m.repos[repoID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("repository %s not found", repoID)
	}

	definitions := make([]*ContainerDefinition, 0)

	// Walk through repository files
	files, err := repo.ListFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}
	for _, file := range files {
		if m.isContainerDefinition(file) {
			content, err := repo.ReadFile(file)
			if err != nil {
				continue
			}

			def := &ContainerDefinition{
				Type:         m.detectDefinitionType(file),
				Path:         file,
				Content:      content,
				LastModified: time.Now(), // TODO: Get actual modification time
			}

			definitions = append(definitions, def)
		}
	}

	return definitions, nil
}

// detectDefinitionType determines the type of container definition
func (m *Manager) detectDefinitionType(path string) DefinitionType {
	base := filepath.Base(path)
	
	if strings.HasPrefix(strings.ToLower(base), "govcfile") {
		return TypeGovcfile
	}
	
	// Legacy support
	if strings.HasPrefix(strings.ToLower(base), "dockerfile") {
		return TypeGovcfile // Convert to govc format
	}
	
	if strings.Contains(base, "govc-compose") {
		return TypeGovcCompose
	}
	
	if base == "Chart.yaml" || base == "values.yaml" {
		return TypeHelm
	}
	
	// Default to Kubernetes for YAML files
	return TypeKubernetes
}

// StartBuild manually starts a container build
func (m *Manager) StartBuild(request BuildRequest) (*Build, error) {
	m.mu.RLock()
	repo, exists := m.repos[request.RepositoryID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("repository %s not found", request.RepositoryID)
	}

	// Check for Govcfile first, then legacy Dockerfile
	govcfilePath := request.Govcfile
	if govcfilePath == "" {
		govcfilePath = request.Containerfile // Legacy field
	}
	
	_, err := repo.ReadFile(govcfilePath)
	if err != nil {
		return nil, fmt.Errorf("govcfile not found: %s", govcfilePath)
	}

	buildID := uuid.New().String()
	build := &Build{
		ID:           buildID,
		RepositoryID: request.RepositoryID,
		Request:      request,
		Status:       BuildStatusPending,
		StartTime:    time.Now(),
	}

	m.mu.Lock()
	m.builds[buildID] = build
	m.mu.Unlock()

	// Execute build asynchronously
	go m.executeBuild(build)

	return build, nil
}

// GetBuild retrieves information about a specific build
func (m *Manager) GetBuild(buildID string) (*Build, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	build, exists := m.builds[buildID]
	if !exists {
		return nil, fmt.Errorf("build %s not found", buildID)
	}

	return build, nil
}

// ListBuilds returns all builds for a repository
func (m *Manager) ListBuilds(repoID string) ([]*Build, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	builds := make([]*Build, 0)
	for _, build := range m.builds {
		if build.RepositoryID == repoID {
			builds = append(builds, build)
		}
	}

	return builds, nil
}

// OnEvent registers an event handler
func (m *Manager) OnEvent(handler func(ContainerEvent)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventHandlers = append(m.eventHandlers, handler)
}

// emitEvent sends an event to all registered handlers
func (m *Manager) emitEvent(event ContainerEvent) {
	m.mu.RLock()
	handlers := make([]func(ContainerEvent), len(m.eventHandlers))
	copy(handlers, m.eventHandlers)
	m.mu.RUnlock()

	for _, handler := range handlers {
		go handler(event)
	}
}

// SetSecurityPolicy sets the security policy for container operations
func (m *Manager) SetSecurityPolicy(policy *SecurityPolicy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.securityPolicy = policy
}