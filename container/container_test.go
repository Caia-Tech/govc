package container

import (
	"fmt"
	"testing"
	"time"

	"github.com/caiatech/govc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerManager(t *testing.T) {
	// Create a new repository
	repo := govc.New()
	
	// Create container manager
	manager := NewManager()
	
	// Register repository
	err := manager.RegisterRepository("test-repo", repo)
	require.NoError(t, err)
	
	// Add a Govcfile to the repository
	govcfileContent := `BASE alpine:latest
RUN apk add --no-cache curl
COPY app /app
CMD ["/app"]`
	
	repo.WriteFile("Govcfile", []byte(govcfileContent))
	
	// Add govc-compose.yml
	composeContent := `version: '1'
services:
  app:
    build: .
    ports:
      - "8080:8080"`
	
	repo.WriteFile("govc-compose.yml", []byte(composeContent))
	
	// Add Kubernetes manifest
	k8sContent := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
      - name: app
        image: test-app:latest
        ports:
        - containerPort: 8080`
	
	repo.WriteFile("k8s/deployment.yaml", []byte(k8sContent))
	
	// Stage and commit the files
	repo.Add("Govcfile", "govc-compose.yml", "k8s/deployment.yaml")
	repo.SetConfig("user.name", "Test")
	repo.SetConfig("user.email", "test@example.com")
	commit, err := repo.Commit("Add container definitions")
	require.NoError(t, err)
	assert.NotEmpty(t, commit.Hash())
	
	// Test GetContainerDefinitions
	definitions, err := manager.GetContainerDefinitions("test-repo")
	require.NoError(t, err)
	assert.Len(t, definitions, 3)
	
	// Verify definitions
	var hasGovcfile, hasCompose, hasK8s bool
	for _, def := range definitions {
		switch def.Type {
		case TypeGovcfile:
			hasGovcfile = true
			assert.Equal(t, "Govcfile", def.Path)
		case TypeGovcCompose:
			hasCompose = true
			assert.Equal(t, "govc-compose.yml", def.Path)
		case TypeKubernetes:
			hasK8s = true
			assert.Equal(t, "k8s/deployment.yaml", def.Path)
		}
	}
	
	assert.True(t, hasGovcfile, "Should have Govcfile")
	assert.True(t, hasCompose, "Should have govc-compose.yml")
	assert.True(t, hasK8s, "Should have Kubernetes manifest")
}

func TestBuildTrigger(t *testing.T) {
	// Create a new repository
	repo := govc.New()
	
	// Create container manager
	manager := NewManager()
	
	// Track events
	var events []ContainerEvent
	manager.OnEvent(func(event ContainerEvent) {
		events = append(events, event)
	})
	
	// Register repository
	err := manager.RegisterRepository("build-test", repo)
	require.NoError(t, err)
	
	// Start a build manually
	request := BuildRequest{
		RepositoryID: "build-test",
		Govcfile:     "Govcfile",
		Context:      ".",
		Tags:         []string{"test:latest", "test:v1.0"},
	}
	
	// Add Govcfile first
	repo.WriteFile("Govcfile", []byte("BASE alpine:latest"))
	repo.Add("Govcfile")
	repo.SetConfig("user.name", "Test")
	repo.SetConfig("user.email", "test@example.com")
	repo.Commit("Add Govcfile")
	
	build, err := manager.StartBuild(request)
	require.NoError(t, err)
	assert.Equal(t, "build-test", build.RepositoryID)
	assert.Equal(t, BuildStatusPending, build.Status)
	assert.NotEmpty(t, build.ID)
	
	// Wait for build to complete (simulated)
	time.Sleep(3 * time.Second)
	
	// Get build status
	completedBuild, err := manager.GetBuild(build.ID)
	require.NoError(t, err)
	assert.Equal(t, BuildStatusCompleted, completedBuild.Status)
	assert.NotEmpty(t, completedBuild.ImageID)
	assert.NotEmpty(t, completedBuild.Digest)
	
	// Check events
	require.GreaterOrEqual(t, len(events), 1, "Should have at least 1 event")
	
	// Find build events
	var buildStarted, buildCompleted bool
	for _, event := range events {
		switch event.Type {
		case EventTypeBuildStarted:
			buildStarted = true
		case EventTypeBuildCompleted:
			buildCompleted = true
		}
	}
	
	assert.True(t, buildStarted || buildCompleted, "Should have build event")
}

func TestContainerDefinitionDetection(t *testing.T) {
	manager := NewManager()
	
	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		{"Govcfile", true, "Standard Govcfile"},
		{"Govcfile.prod", true, "Govcfile with suffix"},
		{"govc-compose.yml", true, "Govc Compose YAML"},
		{"govc-compose.yaml", true, "Govc Compose YAML alt"},
		{"k8s/deployment.yaml", true, "Kubernetes deployment"},
		{"kubernetes/service.yml", true, "Kubernetes service"},
		{"Chart.yaml", true, "Helm chart"},
		{"values.yaml", true, "Helm values"},
		{"README.md", false, "Non-container file"},
		{"app.go", false, "Go source file"},
		{"config.json", false, "JSON config"},
	}
	
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			result := manager.isContainerDefinition(test.path)
			assert.Equal(t, test.expected, result, "Path: %s", test.path)
		})
	}
}

func TestSecurityPolicy(t *testing.T) {
	manager := NewManager()
	
	policy := &SecurityPolicy{
		AllowedBaseImages: []string{
			"alpine:*",
			"ubuntu:22.04",
			"golang:1.21",
		},
		ProhibitedPackages: []string{
			"telnet",
			"ftp",
		},
		VulnerabilityPolicy: VulnerabilityPolicy{
			MaxCritical: 0,
			MaxHigh:     5,
			MaxMedium:   10,
			FailOnNew:   true,
		},
		SigningRequired: true,
		ScanRequired:    true,
		MaxImageSize:    1024 * 1024 * 1024, // 1GB
	}
	
	manager.SetSecurityPolicy(policy)
	
	// TODO: Test security policy enforcement
}

func TestListBuilds(t *testing.T) {
	repo := govc.New()
	manager := NewManager()
	
	err := manager.RegisterRepository("list-test", repo)
	require.NoError(t, err)
	
	// Add Govcfile
	repo.WriteFile("Govcfile", []byte("BASE alpine:latest"))
	repo.Add("Govcfile")
	repo.SetConfig("user.name", "Test")
	repo.SetConfig("user.email", "test@example.com")
	repo.Commit("Add Govcfile")
	
	// Start multiple builds
	for i := 0; i < 3; i++ {
		request := BuildRequest{
			RepositoryID: "list-test",
			Govcfile:     "Govcfile",
			Tags:         []string{fmt.Sprintf("test:v%d", i)},
		}
		
		_, err := manager.StartBuild(request)
		require.NoError(t, err)
	}
	
	// List builds
	builds, err := manager.ListBuilds("list-test")
	require.NoError(t, err)
	assert.Len(t, builds, 3)
	
	// List builds for non-existent repo
	builds, err = manager.ListBuilds("non-existent")
	require.NoError(t, err)
	assert.Empty(t, builds)
}