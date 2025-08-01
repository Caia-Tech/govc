package govc_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caiatech/govc/api"
	"github.com/caiatech/govc/config"
	"github.com/caiatech/govc/container"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegrationFullContainerWorkflow tests the complete container workflow
// from repository creation to building and managing containers
func TestIntegrationFullContainerWorkflow(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:           8080,
			MaxRequestSize: 10 * 1024 * 1024,
			RequestTimeout: 30 * time.Second,
			MaxRepos:       100,
		},
		Pool: config.PoolConfig{
			MaxRepositories: 100,
			MaxIdleTime:     30 * time.Minute,
			CleanupInterval: 5 * time.Minute,
		},
		Metrics: config.MetricsConfig{
			Enabled: true,
		},
	}
	
	server := api.NewServer(cfg)
	router := gin.New()
	server.RegisterRoutes(router)
	
	// Step 1: Create a new repository
	t.Run("CreateRepository", func(t *testing.T) {
		createReq := map[string]interface{}{
			"id":          "test-app",
			"memory_only": true,
		}
		body, _ := json.Marshal(createReq)
		
		req := httptest.NewRequest("POST", "/api/v1/repos", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusCreated {
			t.Logf("Response body: %s", w.Body.String())
		}
		assert.Equal(t, http.StatusCreated, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response["id"])
		assert.Equal(t, "test-app", response["id"])
	})
	
	repoID := "test-app"
	
	// Step 2: Add a Govcfile to the repository
	t.Run("AddGovcfile", func(t *testing.T) {
		govcfileContent := `BASE alpine:latest
WORKDIR /app
COPY . .
RUN apk add --no-cache nodejs npm
RUN npm install
EXPOSE 3000
CMD ["node", "server.js"]`
		
		createReq := map[string]interface{}{
			"path":    "Govcfile",
			"content": govcfileContent,
			"message": "Add Govcfile for Node.js app",
			"author": map[string]string{
				"name":  "Test User",
				"email": "test@example.com",
			},
		}
		body, _ := json.Marshal(createReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/containers/definitions", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusCreated {
			t.Logf("Create container definition response: %s", w.Body.String())
		}
		assert.Equal(t, http.StatusCreated, w.Code)
	})
	
	// Step 3: Add application files (skip for now - would need proper git workflow)
	/*t.Run("AddApplicationFiles", func(t *testing.T) {
		// Add package.json
		packageJSON := `{
  "name": "test-app",
  "version": "1.0.0",
  "main": "server.js",
  "dependencies": {
    "express": "^4.18.0"
  }
}`
		
		writeReq := map[string]interface{}{
			"path":    "package.json",
			"content": packageJSON,
			"message": "Add package.json",
		}
		body, _ := json.Marshal(writeReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/write", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
		
		// Add server.js
		serverJS := `const express = require('express');
const app = express();
const port = process.env.PORT || 3000;

app.get('/', (req, res) => {
  res.json({ message: 'Hello from govc container!' });
});

app.listen(port, () => {
  console.log('Server running on port ' + port);
});`
		
		writeReq = map[string]interface{}{
			"path":    "server.js",
			"content": serverJS,
			"message": "Add server.js",
		}
		body, _ = json.Marshal(writeReq)
		
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/write", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
		
		// Commit all changes
		commitReq := map[string]interface{}{
			"message": "Add application files",
			"author": map[string]string{
				"name":  "Test User",
				"email": "test@example.com",
			},
		}
		body, _ = json.Marshal(commitReq)
		
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})*/
	
	// Step 4: List container definitions
	t.Run("ListContainerDefinitions", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/containers/definitions", repoID), nil)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var response struct {
			Definitions []container.ContainerDefinition `json:"definitions"`
			Count       int                             `json:"count"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, 1, response.Count)
		assert.Equal(t, "Govcfile", response.Definitions[0].Path)
		assert.Equal(t, container.TypeGovcfile, response.Definitions[0].Type)
	})
	
	// Step 5: Start a container build
	var buildID string
	t.Run("StartContainerBuild", func(t *testing.T) {
		buildReq := container.BuildRequest{
			Govcfile: "Govcfile",
			Context:  ".",
			Tags:     []string{"test-app:latest", "test-app:v1.0"},
			Args: map[string]string{
				"NODE_ENV": "production",
			},
			Labels: map[string]string{
				"version":    "1.0.0",
				"maintainer": "test@example.com",
			},
		}
		body, _ := json.Marshal(buildReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/containers/build", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusAccepted, w.Code)
		
		var build container.Build
		err := json.Unmarshal(w.Body.Bytes(), &build)
		require.NoError(t, err)
		
		assert.NotEmpty(t, build.ID)
		buildID = build.ID
		assert.Equal(t, repoID, build.RepositoryID)
		assert.Contains(t, []container.BuildStatus{
			container.BuildStatusPending,
			container.BuildStatusRunning,
		}, build.Status)
	})
	
	// Step 6: Monitor build progress
	t.Run("MonitorBuildProgress", func(t *testing.T) {
		// Wait for build to complete
		time.Sleep(3 * time.Second)
		
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/containers/builds/%s", repoID, buildID), nil)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var build container.Build
		err := json.Unmarshal(w.Body.Bytes(), &build)
		require.NoError(t, err)
		
		assert.Equal(t, buildID, build.ID)
		assert.Equal(t, container.BuildStatusCompleted, build.Status)
		assert.NotEmpty(t, build.ImageID)
		assert.NotNil(t, build.EndTime)
	})
	
	// Step 7: Get build logs
	t.Run("GetBuildLogs", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/containers/builds/%s/logs", repoID, buildID), nil)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var response struct {
			BuildID string `json:"build_id"`
			Logs    string `json:"logs"`
			Status  string `json:"status"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, buildID, response.BuildID)
		assert.Equal(t, "completed", response.Status)
	})
	
	// Step 8: List all builds
	t.Run("ListAllBuilds", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/containers/builds", repoID), nil)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var response struct {
			Builds []container.Build `json:"builds"`
			Count  int               `json:"count"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.GreaterOrEqual(t, response.Count, 1)
		
		// Find our build
		found := false
		for _, b := range response.Builds {
			if b.ID == buildID {
				found = true
				assert.Equal(t, container.BuildStatusCompleted, b.Status)
				break
			}
		}
		assert.True(t, found, "Build should be in the list")
	})
	
	// Step 9: Add govc-compose.yml
	t.Run("AddGovcCompose", func(t *testing.T) {
		composeContent := `version: '1'
services:
  web:
    build: .
    ports:
      - "3000:3000"
    environment:
      - NODE_ENV=production
  redis:
    image: redis:alpine
    ports:
      - "6379:6379"`
		
		createReq := map[string]interface{}{
			"path":    "govc-compose.yml",
			"content": composeContent,
			"message": "Add govc-compose configuration",
			"author": map[string]string{
				"name":  "Test User",
				"email": "test@example.com",
			},
		}
		body, _ := json.Marshal(createReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/containers/definitions", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusCreated, w.Code)
		
		var response struct {
			Path   string                   `json:"path"`
			Commit string                   `json:"commit"`
			Type   container.DefinitionType `json:"type"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, "govc-compose.yml", response.Path)
		assert.Equal(t, container.TypeGovcCompose, response.Type)
		assert.NotEmpty(t, response.Commit)
	})
	
	// Step 10: Update Govcfile
	t.Run("UpdateGovcfile", func(t *testing.T) {
		updatedContent := `BASE alpine:latest
WORKDIR /app
COPY package*.json ./
RUN apk add --no-cache nodejs npm
RUN npm ci --only=production
COPY . .
EXPOSE 3000
CMD ["node", "server.js"]`
		
		updateReq := map[string]interface{}{
			"content": updatedContent,
			"message": "Optimize Govcfile for production",
		}
		body, _ := json.Marshal(updateReq)
		
		req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/repos/%s/containers/definitions/Govcfile", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var response struct {
			Path   string `json:"path"`
			Commit string `json:"commit"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, "Govcfile", response.Path)
		assert.NotEmpty(t, response.Commit)
	})
	
	// Step 11: Create another build with the updated Govcfile
	t.Run("BuildWithUpdatedGovcfile", func(t *testing.T) {
		buildReq := container.BuildRequest{
			Govcfile: "Govcfile",
			Context:  ".",
			Tags:     []string{"test-app:latest", "test-app:v1.1"},
			NoCache:  true, // Force rebuild without cache
		}
		body, _ := json.Marshal(buildReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/containers/build", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusAccepted, w.Code)
		
		var build container.Build
		err := json.Unmarshal(w.Body.Bytes(), &build)
		require.NoError(t, err)
		
		assert.NotEmpty(t, build.ID)
		assert.NotEqual(t, buildID, build.ID) // Different build
		
		// Wait for completion
		time.Sleep(3 * time.Second)
	})
	
	// Cleanup
	defer server.Close()
}

// TestIntegrationMultiStageContainer tests multi-stage container builds
func TestIntegrationMultiStageContainer(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:           8080,
			MaxRequestSize: 10 * 1024 * 1024,
			RequestTimeout: 30 * time.Second,
			MaxRepos:       100,
		},
		Pool: config.PoolConfig{
			MaxRepositories: 100,
			MaxIdleTime:     30 * time.Minute,
			CleanupInterval: 5 * time.Minute,
		},
		Metrics: config.MetricsConfig{
			Enabled: true,
		},
	}
	
	server := api.NewServer(cfg)
	router := gin.New()
	server.RegisterRoutes(router)
	
	repoID := "multi-stage-app"
	
	// Create repository
	t.Run("CreateRepository", func(t *testing.T) {
		createReq := map[string]interface{}{
			"id":          repoID,
			"memory_only": true,
		}
		body, _ := json.Marshal(createReq)
		
		req := httptest.NewRequest("POST", "/api/v1/repos", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
	})
	
	// Add multi-stage Govcfile
	t.Run("AddMultiStageGovcfile", func(t *testing.T) {
		govcfileContent := `# Build stage
BASE golang:1.19-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o app main.go

# Runtime stage
BASE alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /build/app .
EXPOSE 8080
CMD ["./app"]`
		
		writeReq := map[string]interface{}{
			"path":    "Govcfile",
			"content": govcfileContent,
			"message": "Add multi-stage Govcfile",
		}
		body, _ := json.Marshal(writeReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/write", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
		
		// Add Go files
		goMod := `module github.com/example/app
go 1.19`
		
		writeReq = map[string]interface{}{
			"path":    "go.mod",
			"content": goMod,
		}
		body, _ = json.Marshal(writeReq)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/write", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		
		mainGo := `package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello from multi-stage build!")
	})
	http.ListenAndServe(":8080", nil)
}`
		
		writeReq = map[string]interface{}{
			"path":    "main.go",
			"content": mainGo,
		}
		body, _ = json.Marshal(writeReq)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/write", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		
		// Commit all
		commitReq := map[string]interface{}{
			"message": "Add Go application files",
			"author": map[string]string{
				"name":  "Test User",
				"email": "test@example.com",
			},
		}
		body, _ = json.Marshal(commitReq)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
	
	// Build multi-stage container
	t.Run("BuildMultiStageContainer", func(t *testing.T) {
		buildReq := container.BuildRequest{
			Govcfile: "Govcfile",
			Context:  ".",
			Tags:     []string{"multi-stage:latest"},
			Target:   "runtime", // Target the runtime stage
		}
		body, _ := json.Marshal(buildReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/containers/build", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusAccepted, w.Code)
		
		var build container.Build
		err := json.Unmarshal(w.Body.Bytes(), &build)
		require.NoError(t, err)
		
		// Wait and verify
		time.Sleep(3 * time.Second)
		
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/containers/builds/%s", repoID, build.ID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		err = json.Unmarshal(w.Body.Bytes(), &build)
		require.NoError(t, err)
		assert.Equal(t, container.BuildStatusCompleted, build.Status)
	})
	
	defer server.Close()
}

// TestIntegrationKubernetesDeployment tests Kubernetes deployment workflow
func TestIntegrationKubernetesDeployment(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:           8080,
			MaxRequestSize: 10 * 1024 * 1024,
			RequestTimeout: 30 * time.Second,
			MaxRepos:       100,
		},
		Pool: config.PoolConfig{
			MaxRepositories: 100,
			MaxIdleTime:     30 * time.Minute,
			CleanupInterval: 5 * time.Minute,
		},
		Metrics: config.MetricsConfig{
			Enabled: true,
		},
	}
	
	server := api.NewServer(cfg)
	router := gin.New()
	server.RegisterRoutes(router)
	
	repoID := "k8s-app"
	
	// Create repository
	t.Run("CreateRepository", func(t *testing.T) {
		createReq := map[string]interface{}{
			"id":          repoID,
			"memory_only": true,
		}
		body, _ := json.Marshal(createReq)
		
		req := httptest.NewRequest("POST", "/api/v1/repos", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
	})
	
	// Add Kubernetes manifests
	t.Run("AddKubernetesManifests", func(t *testing.T) {
		// Add deployment.yaml
		deploymentYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  labels:
    app: test-app
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
        - containerPort: 8080
        env:
        - name: ENV
          value: production`
		
		createReq := map[string]interface{}{
			"path":    "k8s/deployment.yaml",
			"content": deploymentYAML,
			"message": "Add Kubernetes deployment",
			"author": map[string]string{
				"name":  "Test User",
				"email": "test@example.com",
			},
		}
		body, _ := json.Marshal(createReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/containers/definitions", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
		
		// Add service.yaml
		serviceYAML := `apiVersion: v1
kind: Service
metadata:
  name: test-app-service
spec:
  selector:
    app: test-app
  ports:
    - protocol: TCP
      port: 80
      targetPort: 8080
  type: LoadBalancer`
		
		createReq = map[string]interface{}{
			"path":    "k8s/service.yaml",
			"content": serviceYAML,
			"message": "Add Kubernetes service",
			"author": map[string]string{
				"name":  "Test User",
				"email": "test@example.com",
			},
		}
		body, _ = json.Marshal(createReq)
		
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/containers/definitions", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
		
		// Add Govcfile for the application
		govcfileContent := `BASE nginx:alpine
COPY index.html /usr/share/nginx/html/
EXPOSE 80`
		
		createReq = map[string]interface{}{
			"path":    "Govcfile",
			"content": govcfileContent,
			"message": "Add Govcfile for nginx app",
			"author": map[string]string{
				"name":  "Test User",
				"email": "test@example.com",
			},
		}
		body, _ = json.Marshal(createReq)
		
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/containers/definitions", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
	})
	
	// List all container definitions
	t.Run("ListAllDefinitions", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/containers/definitions", repoID), nil)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		
		var response struct {
			Definitions []container.ContainerDefinition `json:"definitions"`
			Count       int                             `json:"count"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, 3, response.Count)
		
		// Verify types
		typeMap := make(map[container.DefinitionType]int)
		for _, def := range response.Definitions {
			typeMap[def.Type]++
		}
		
		assert.Equal(t, 1, typeMap[container.TypeGovcfile])
		assert.Equal(t, 2, typeMap[container.TypeKubernetes])
	})
	
	// Build the container
	t.Run("BuildContainer", func(t *testing.T) {
		// First add the index.html file
		indexHTML := `<!DOCTYPE html>
<html>
<head><title>Test App</title></head>
<body><h1>Hello from Kubernetes!</h1></body>
</html>`
		
		writeReq := map[string]interface{}{
			"path":    "index.html",
			"content": indexHTML,
			"message": "Add index.html",
		}
		body, _ := json.Marshal(writeReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/write", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		
		// Commit
		commitReq := map[string]interface{}{
			"message": "Add index.html",
			"author": map[string]string{
				"name":  "Test User",
				"email": "test@example.com",
			},
		}
		body, _ = json.Marshal(commitReq)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		
		// Now build
		buildReq := container.BuildRequest{
			Govcfile: "Govcfile",
			Context:  ".",
			Tags:     []string{"k8s-app:latest", "k8s-app:v1.0"},
		}
		body, _ = json.Marshal(buildReq)
		
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/containers/build", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusAccepted, w.Code)
	})
	
	defer server.Close()
}

// TestIntegrationBranchWorkflow tests container builds across different branches
func TestIntegrationBranchWorkflow(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:           8080,
			MaxRequestSize: 10 * 1024 * 1024,
			RequestTimeout: 30 * time.Second,
			MaxRepos:       100,
		},
		Pool: config.PoolConfig{
			MaxRepositories: 100,
			MaxIdleTime:     30 * time.Minute,
			CleanupInterval: 5 * time.Minute,
		},
		Metrics: config.MetricsConfig{
			Enabled: true,
		},
	}
	
	server := api.NewServer(cfg)
	router := gin.New()
	server.RegisterRoutes(router)
	
	repoID := "branch-test"
	
	// Create repository
	t.Run("CreateRepository", func(t *testing.T) {
		createReq := map[string]interface{}{
			"id":          repoID,
			"memory_only": true,
		}
		body, _ := json.Marshal(createReq)
		
		req := httptest.NewRequest("POST", "/api/v1/repos", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
	})
	
	// Add initial Govcfile on main branch
	t.Run("AddGovcfileOnMain", func(t *testing.T) {
		govcfileContent := `BASE alpine:latest
RUN echo "Main branch build"`
		
		addReq := map[string]interface{}{
			"path":    "Govcfile",
			"content": govcfileContent,
		}
		body, _ := json.Marshal(addReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		
		// Commit
		commitReq := map[string]interface{}{
			"message": "Initial commit",
			"author":  "Test User",
			"email":   "test@example.com",
		}
		body, _ = json.Marshal(commitReq)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code) // Commit should return 201
		
		// Explicitly create main branch from current HEAD
		branchReq := map[string]interface{}{
			"name": "main",
		}
		body, _ = json.Marshal(branchReq)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		// Don't assert status here as it might fail if main already exists
	})
	
	// Create feature branch from main
	t.Run("CreateFeatureBranch", func(t *testing.T) {
		branchReq := map[string]interface{}{
			"name": "feature/new-base",
		}
		body, _ := json.Marshal(branchReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
	})
	
	// Checkout feature branch
	t.Run("CheckoutFeatureBranch", func(t *testing.T) {
		checkoutReq := map[string]interface{}{
			"branch": "feature/new-base",
		}
		body, _ := json.Marshal(checkoutReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
	
	// Update Govcfile on feature branch
	t.Run("UpdateGovcfileOnFeature", func(t *testing.T) {
		updatedContent := `BASE ubuntu:22.04
RUN apt-get update && apt-get install -y curl
RUN echo "Feature branch build"`
		
		updateReq := map[string]interface{}{
			"content": updatedContent,
			"message": "Switch to Ubuntu base",
		}
		body, _ := json.Marshal(updateReq)
		
		req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/repos/%s/containers/definitions/Govcfile", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
	
	// Build on feature branch
	t.Run("BuildOnFeatureBranch", func(t *testing.T) {
		buildReq := container.BuildRequest{
			Govcfile: "Govcfile",
			Context:  ".",
			Tags:     []string{"branch-test:feature"},
		}
		body, _ := json.Marshal(buildReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/containers/build", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusAccepted, w.Code)
		
		var build container.Build
		err := json.Unmarshal(w.Body.Bytes(), &build)
		require.NoError(t, err)
		
		// Wait for build
		time.Sleep(3 * time.Second)
		
		// Verify build completed
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/containers/builds/%s", repoID, build.ID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		err = json.Unmarshal(w.Body.Bytes(), &build)
		require.NoError(t, err)
		assert.Equal(t, container.BuildStatusCompleted, build.Status)
	})
	
	// Switch back to main and verify original Govcfile
	t.Run("SwitchBackToMain", func(t *testing.T) {
		checkoutReq := map[string]interface{}{
			"branch": "main",
		}
		body, _ := json.Marshal(checkoutReq)
		
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		
		// Get Govcfile content
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/containers/definitions/Govcfile", repoID), nil)
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		
		var def container.ContainerDefinition
		err := json.Unmarshal(w.Body.Bytes(), &def)
		require.NoError(t, err)
		
		assert.Contains(t, string(def.Content), "BASE alpine:latest")
		assert.Contains(t, string(def.Content), "Main branch build")
	})
	
	defer server.Close()
}

// TestIntegrationErrorHandling tests error scenarios
func TestIntegrationErrorHandling(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:           8080,
			MaxRequestSize: 10 * 1024 * 1024,
			RequestTimeout: 30 * time.Second,
			MaxRepos:       100,
		},
		Pool: config.PoolConfig{
			MaxRepositories: 100,
			MaxIdleTime:     30 * time.Minute,
			CleanupInterval: 5 * time.Minute,
		},
		Metrics: config.MetricsConfig{
			Enabled: true,
		},
	}
	
	server := api.NewServer(cfg)
	router := gin.New()
	server.RegisterRoutes(router)
	
	// Test building without Govcfile
	t.Run("BuildWithoutGovcfile", func(t *testing.T) {
		// Create repo
		createReq := map[string]interface{}{
			"id":          "no-govcfile",
			"memory_only": true,
		}
		body, _ := json.Marshal(createReq)
		
		req := httptest.NewRequest("POST", "/api/v1/repos", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
		
		// Try to build
		buildReq := container.BuildRequest{
			Govcfile: "Govcfile",
			Context:  ".",
			Tags:     []string{"test:latest"},
		}
		body, _ = json.Marshal(buildReq)
		
		req = httptest.NewRequest("POST", "/api/v1/repos/no-govcfile/containers/build", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, strings.ToLower(response["error"].(string)), "not found")
	})
	
	// Test invalid repository
	t.Run("InvalidRepository", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/repos/non-existent/containers/definitions", nil)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	
	// Test invalid build ID
	t.Run("InvalidBuildID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/repos/test-app/containers/builds/invalid-id", nil)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	
	defer server.Close()
}