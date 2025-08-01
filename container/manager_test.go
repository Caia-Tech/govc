package container

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/caiatech/govc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockRepository is a mock implementation of govc.Repository
type MockRepository struct {
	mock.Mock
	files map[string][]byte
	mu    sync.RWMutex
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		files: make(map[string][]byte),
	}
}

func (m *MockRepository) WriteFile(path string, content []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = content
	return nil
}

func (m *MockRepository) ReadFile(path string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	args := m.Called(path)
	if content, exists := m.files[path]; exists {
		return content, nil
	}
	return nil, args.Error(0)
}

func (m *MockRepository) ListFiles() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	args := m.Called()
	var files []string
	for path := range m.files {
		files = append(files, path)
	}
	return files, args.Error(0)
}

func (m *MockRepository) Watch(handler func(govc.CommitEvent)) {
	m.Called(handler)
}

func TestManagerCreation(t *testing.T) {
	manager := NewManager()
	
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.repos)
	assert.NotNil(t, manager.builds)
	assert.NotNil(t, manager.eventHandlers)
}

func TestRegisterRepository(t *testing.T) {
	manager := NewManager()
	repo := govc.New()
	
	t.Run("Register New Repository", func(t *testing.T) {
		err := manager.RegisterRepository("test-repo", repo)
		assert.NoError(t, err)
		
		// Verify repository is registered
		manager.mu.RLock()
		_, exists := manager.repos["test-repo"]
		manager.mu.RUnlock()
		assert.True(t, exists)
	})
	
	t.Run("Register Duplicate Repository", func(t *testing.T) {
		err := manager.RegisterRepository("test-repo", repo)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})
}

func TestIsContainerDefinition(t *testing.T) {
	manager := NewManager()
	
	tests := []struct {
		path     string
		expected bool
	}{
		{"Govcfile", true},
		{"Govcfile.prod", true},
		{"govcfile", true},
		{"Dockerfile", true}, // Legacy support
		{"govc-compose.yml", true},
		{"govc-compose.yaml", true},
		{"govc-compose.dev.yml", true},
		{"k8s/deployment.yaml", true},
		{"kubernetes/service.yml", true},
		{"k8s-deployment.yaml", true},
		{"Chart.yaml", true},
		{"values.yaml", true},
		{"README.md", false},
		{"main.go", false},
		{"package.json", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := manager.isContainerDefinition(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldAutoBuild(t *testing.T) {
	manager := NewManager()
	
	tests := []struct {
		path     string
		expected bool
	}{
		{"Govcfile", true},
		{"Govcfile.prod", true},
		{"Dockerfile", true}, // Legacy support
		{"govc-compose.yml", false},
		{"k8s/deployment.yaml", false},
		{"Chart.yaml", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := manager.shouldAutoBuild(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetContainerDefinitions(t *testing.T) {
	manager := NewManager()
	repo := govc.New()
	
	// Set up repository data
	repo.WriteFile("Govcfile", []byte("BASE alpine"))
	repo.WriteFile("govc-compose.yml", []byte("version: '1'"))
	repo.WriteFile("k8s/deployment.yaml", []byte("apiVersion: apps/v1"))
	repo.WriteFile("README.md", []byte("# README"))
	
	// Stage and commit files
	repo.Add("Govcfile", "govc-compose.yml", "k8s/deployment.yaml", "README.md")
	repo.SetConfig("user.name", "Test")
	repo.SetConfig("user.email", "test@example.com")
	repo.Commit("Add files")
	
	err := manager.RegisterRepository("test-repo", repo)
	require.NoError(t, err)
	
	definitions, err := manager.GetContainerDefinitions("test-repo")
	require.NoError(t, err)
	
	// Should have 3 container definitions (not README.md)
	assert.Len(t, definitions, 3)
	
	// Check definition types
	typeCount := map[DefinitionType]int{}
	for _, def := range definitions {
		typeCount[def.Type]++
	}
	
	assert.Equal(t, 1, typeCount[TypeGovcfile])
	assert.Equal(t, 1, typeCount[TypeGovcCompose])
	assert.Equal(t, 1, typeCount[TypeKubernetes])
}

func TestGetContainerDefinitionsErrors(t *testing.T) {
	manager := NewManager()
	
	t.Run("Repository Not Found", func(t *testing.T) {
		_, err := manager.GetContainerDefinitions("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestDetectDefinitionType(t *testing.T) {
	manager := NewManager()
	
	tests := []struct {
		path     string
		expected DefinitionType
	}{
		{"Govcfile", TypeGovcfile},
		{"Govcfile.prod", TypeGovcfile},
		{"Dockerfile", TypeGovcfile}, // Legacy support - converts to Govcfile
		{"govc-compose.yml", TypeGovcCompose},
		{"govc-compose.dev.yaml", TypeGovcCompose},
		{"Chart.yaml", TypeHelm},
		{"values.yaml", TypeHelm},
		{"deployment.yaml", TypeKubernetes},
		{"service.yml", TypeKubernetes},
	}
	
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := manager.detectDefinitionType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStartBuild(t *testing.T) {
	manager := NewManager()
	repo := govc.New()
	
	// Set up repository with Govcfile
	repo.WriteFile("Govcfile", []byte("BASE alpine"))
	repo.Add("Govcfile")
	repo.SetConfig("user.name", "Test")
	repo.SetConfig("user.email", "test@example.com")
	repo.Commit("Add Govcfile")
	
	err := manager.RegisterRepository("test-repo", repo)
	require.NoError(t, err)
	
	t.Run("Valid Build Request", func(t *testing.T) {
		request := BuildRequest{
			RepositoryID: "test-repo",
			Govcfile:     "Govcfile",
			Context:      ".",
			Tags:         []string{"test:latest"},
		}
		
		build, err := manager.StartBuild(request)
		require.NoError(t, err)
		assert.NotNil(t, build)
		assert.Equal(t, "test-repo", build.RepositoryID)
		assert.Equal(t, BuildStatusPending, build.Status)
		assert.NotEmpty(t, build.ID)
		
		// Wait for build to complete
		time.Sleep(3 * time.Second)
		
		// Get build status
		completedBuild, err := manager.GetBuild(build.ID)
		require.NoError(t, err)
		assert.Equal(t, BuildStatusCompleted, completedBuild.Status)
		assert.NotEmpty(t, completedBuild.ImageID)
	})
	
	t.Run("Invalid Repository", func(t *testing.T) {
		request := BuildRequest{
			RepositoryID: "non-existent",
			Govcfile:     "Govcfile",
		}
		
		_, err := manager.StartBuild(request)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
	
	t.Run("Missing Govcfile", func(t *testing.T) {
		request := BuildRequest{
			RepositoryID: "test-repo",
			Govcfile:     "missing.govcfile",
		}
		
		_, err := manager.StartBuild(request)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "govcfile not found")
	})
}

func TestGetBuild(t *testing.T) {
	manager := NewManager()
	
	t.Run("Build Not Found", func(t *testing.T) {
		_, err := manager.GetBuild("non-existent-build")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestManagerListBuilds(t *testing.T) {
	manager := NewManager()
	repo := govc.New()
	
	repo.WriteFile("Govcfile", []byte("BASE alpine"))
	repo.Add("Govcfile")
	repo.SetConfig("user.name", "Test")
	repo.SetConfig("user.email", "test@example.com")
	repo.Commit("Add Govcfile")
	
	err := manager.RegisterRepository("test-repo", repo)
	require.NoError(t, err)
	
	// Start multiple builds
	for i := 0; i < 3; i++ {
		request := BuildRequest{
			RepositoryID: "test-repo",
			Govcfile:     "Govcfile",
			Tags:         []string{fmt.Sprintf("test:v%d", i)},
		}
		_, err := manager.StartBuild(request)
		require.NoError(t, err)
	}
	
	// List builds
	builds, err := manager.ListBuilds("test-repo")
	require.NoError(t, err)
	assert.Len(t, builds, 3)
	
	// List builds for different repo
	otherBuilds, err := manager.ListBuilds("other-repo")
	require.NoError(t, err)
	assert.Empty(t, otherBuilds)
}

func TestEventHandling(t *testing.T) {
	manager := NewManager()
	
	// Track events
	var receivedEvents []ContainerEvent
	var mu sync.Mutex
	
	manager.OnEvent(func(event ContainerEvent) {
		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()
	})
	
	// Emit test event
	testEvent := ContainerEvent{
		Type:         EventTypeBuildStarted,
		RepositoryID: "test-repo",
		Timestamp:    time.Now(),
		Actor:        "test",
	}
	
	manager.emitEvent(testEvent)
	
	// Wait for async processing
	time.Sleep(100 * time.Millisecond)
	
	mu.Lock()
	assert.Len(t, receivedEvents, 1)
	assert.Equal(t, EventTypeBuildStarted, receivedEvents[0].Type)
	mu.Unlock()
}

func TestMultipleEventHandlers(t *testing.T) {
	manager := NewManager()
	
	// Register multiple handlers
	handler1Called := false
	handler2Called := false
	
	manager.OnEvent(func(event ContainerEvent) {
		handler1Called = true
	})
	
	manager.OnEvent(func(event ContainerEvent) {
		handler2Called = true
	})
	
	// Emit event
	manager.emitEvent(ContainerEvent{
		Type:         EventTypeBuildCompleted,
		RepositoryID: "test",
		Timestamp:    time.Now(),
	})
	
	// Wait for handlers
	time.Sleep(100 * time.Millisecond)
	
	assert.True(t, handler1Called)
	assert.True(t, handler2Called)
}

func TestManagerSecurityPolicy(t *testing.T) {
	manager := NewManager()
	
	policy := &SecurityPolicy{
		AllowedBaseImages:  []string{"alpine:*"},
		ProhibitedPackages: []string{"telnet"},
		SigningRequired:    true,
		ScanRequired:       true,
	}
	
	manager.SetSecurityPolicy(policy)
	
	// Verify policy is set
	manager.mu.RLock()
	assert.Equal(t, policy, manager.securityPolicy)
	manager.mu.RUnlock()
}

func TestConcurrentOperations(t *testing.T) {
	manager := NewManager()
	repo := govc.New()
	
	repo.WriteFile("Govcfile", []byte("BASE alpine"))
	repo.Add("Govcfile")
	repo.SetConfig("user.name", "Test")
	repo.SetConfig("user.email", "test@example.com")
	repo.Commit("Add Govcfile")
	
	err := manager.RegisterRepository("test-repo", repo)
	require.NoError(t, err)
	
	// Start multiple builds concurrently
	var wg sync.WaitGroup
	buildCount := 10
	
	for i := 0; i < buildCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			request := BuildRequest{
				RepositoryID: "test-repo",
				Govcfile:     "Govcfile",
				Tags:         []string{fmt.Sprintf("test:v%d", index)},
			}
			
			_, err := manager.StartBuild(request)
			assert.NoError(t, err)
		}(i)
	}
	
	wg.Wait()
	
	// Verify all builds were created
	builds, err := manager.ListBuilds("test-repo")
	require.NoError(t, err)
	assert.Len(t, builds, buildCount)
}

func TestBuildStatusUpdates(t *testing.T) {
	manager := NewManager()
	
	// Create a build directly
	build := &Build{
		ID:           "test-build",
		RepositoryID: "test",
		Status:       BuildStatusPending,
	}
	
	manager.mu.Lock()
	manager.builds[build.ID] = build
	manager.mu.Unlock()
	
	// Update status
	manager.updateBuildStatus(build.ID, BuildStatusRunning)
	
	// Verify status was updated
	manager.mu.RLock()
	assert.Equal(t, BuildStatusRunning, manager.builds[build.ID].Status)
	manager.mu.RUnlock()
	
	// Update non-existent build
	manager.updateBuildStatus("non-existent", BuildStatusCompleted)
	// Should not panic
}