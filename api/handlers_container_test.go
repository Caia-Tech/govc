package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/caiatech/govc"
	"github.com/caiatech/govc/config"
	"github.com/caiatech/govc/container"
	"github.com/caiatech/govc/logging"
	"github.com/caiatech/govc/pool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to write and stage a file in a repository
func writeAndStageFile(t *testing.T, repo *govc.Repository, path string, content []byte) {
	err := repo.WriteFile(path, content)
	require.NoError(t, err)
	err = repo.Add(path)
	require.NoError(t, err)
}

// Helper to setup a repository with a basic Govcfile
func setupRepoWithGovcfile(t *testing.T, server *Server, repoID string, govcfileContent string) *govc.Repository {
	pooledRepo, err := server.repoPool.Get(repoID, ":memory:", true)
	require.NoError(t, err)
	repo := pooledRepo.Repository
	
	writeAndStageFile(t, repo, "Govcfile", []byte(govcfileContent))
	
	repo.SetConfig("user.name", "Test")
	repo.SetConfig("user.email", "test@example.com")
	_, err = repo.Commit("Add Govcfile")
	require.NoError(t, err)
	
	// Register repository with container manager
	err = server.containerManager.RegisterRepository(repoID, repo)
	require.NoError(t, err)
	
	return repo
}

func setupContainerTestServer() (*gin.Engine, *Server) {
	gin.SetMode(gin.TestMode)
	
	// Create server with test dependencies
	poolCfg := pool.PoolConfig{
		MaxRepositories: 10,
		MaxIdleTime:     30 * time.Minute,
		CleanupInterval: 5 * time.Minute,
	}
	
	server := &Server{
		config:           &config.Config{},
		repoPool:         pool.NewRepositoryPool(poolCfg),
		containerManager: container.NewManager(),
		logger:           logging.NewLogger(logging.Config{
			Component: "test",
			Level:     logging.InfoLevel,
		}),
		repoMetadata:     make(map[string]*RepoMetadata),
	}
	
	// Register test repository metadata
	server.repoMetadata["test-repo"] = &RepoMetadata{
		ID:        "test-repo",
		CreatedAt: time.Now(),
		Path:      ":memory:", // Use memory repository for tests
	}
	
	// Setup routes
	router := gin.New()
	v1 := router.Group("/api/v1")
	
	// Use the actual setupContainerRoutes method
	server.setupContainerRoutes(v1)
	
	
	return router, server
}


func TestListContainerDefinitions(t *testing.T) {
	router, server := setupContainerTestServer()
	pooledRepo, err := server.repoPool.Get("test-repo", ":memory:", true)
	require.NoError(t, err)
	repo := pooledRepo.Repository
	
	// Add container definitions to repo
	govcfileContent := `BASE alpine:latest
RUN apk add --no-cache curl
CMD ["/app"]`
	
	composeContent := `version: '1'
services:
  app:
    build: .
    ports:
      - "8080:8080"`
	
	k8sContent := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app`
	
	writeAndStageFile(t, repo, "Govcfile", []byte(govcfileContent))
	writeAndStageFile(t, repo, "govc-compose.yml", []byte(composeContent))
	writeAndStageFile(t, repo, "k8s/deployment.yaml", []byte(k8sContent))
	
	repo.SetConfig("user.name", "Test")
	repo.SetConfig("user.email", "test@example.com")
	_, err = repo.Commit("Add container definitions")
	require.NoError(t, err)
	
	// List files in repo to debug
	files, err := repo.ListFiles()
	require.NoError(t, err)
	t.Logf("Files in repo: %v", files)
	
	// Test GET /api/v1/repos/:repo_id/containers/definitions
	req := httptest.NewRequest("GET", "/api/v1/repos/test-repo/containers/definitions", nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Logf("Response status: %d", w.Code)
		t.Logf("Response body: %s", w.Body.String())
	}
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response struct {
		Definitions []container.ContainerDefinition `json:"definitions"`
		Count       int                             `json:"count"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Len(t, response.Definitions, 3)
	assert.Equal(t, 3, response.Count)
	
	// Verify definition types
	typeCount := map[container.DefinitionType]int{}
	for _, def := range response.Definitions {
		typeCount[def.Type]++
	}
	
	assert.Equal(t, 1, typeCount[container.TypeGovcfile])
	assert.Equal(t, 1, typeCount[container.TypeGovcCompose])
	assert.Equal(t, 1, typeCount[container.TypeKubernetes])
}

func TestStartBuild(t *testing.T) {
	router, server := setupContainerTestServer()
	pooledRepo, err := server.repoPool.Get("test-repo", ":memory:", true)
	require.NoError(t, err)
	repo := pooledRepo.Repository
	
	// Add Govcfile
	govcfileContent := `BASE alpine:latest
RUN echo "Hello World"
CMD ["/bin/sh"]`
	
	writeAndStageFile(t, repo, "Govcfile", []byte(govcfileContent))
	
	repo.SetConfig("user.name", "Test")
	repo.SetConfig("user.email", "test@example.com")
	_, err = repo.Commit("Add Govcfile")
	require.NoError(t, err)
	
	// Register repository with container manager
	err = server.containerManager.RegisterRepository("test-repo", repo)
	require.NoError(t, err)
	
	// Debug: Check if file exists
	content, err := repo.ReadFile("Govcfile")
	if err != nil {
		t.Logf("Failed to read Govcfile: %v", err)
		files, _ := repo.ListFiles()
		t.Logf("Files in repo: %v", files)
	} else {
		t.Logf("Govcfile content length: %d", len(content))
	}
	
	// Create build request
	buildReq := container.BuildRequest{
		Govcfile: "Govcfile",
		Context:  ".",
		Tags:     []string{"test:latest", "test:v1.0"},
		Args:     map[string]string{"VERSION": "1.0"},
		Labels:   map[string]string{"maintainer": "test@example.com"},
		NoCache:  false,
	}
	
	body, _ := json.Marshal(buildReq)
	req := httptest.NewRequest("POST", "/api/v1/repos/test-repo/containers/build", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	if w.Code != http.StatusAccepted {
		t.Logf("Response status: %d", w.Code)
		t.Logf("Response body: %s", w.Body.String())
	}
	assert.Equal(t, http.StatusAccepted, w.Code)
	
	var build container.Build
	err = json.Unmarshal(w.Body.Bytes(), &build)
	require.NoError(t, err)
	
	assert.NotEmpty(t, build.ID)
	assert.Equal(t, "test-repo", build.RepositoryID)
	// Build might be pending or already running by the time we check
	assert.Contains(t, []container.BuildStatus{container.BuildStatusPending, container.BuildStatusRunning}, build.Status)
	assert.Equal(t, buildReq.Tags, build.Request.Tags)
}

func TestListBuilds(t *testing.T) {
	router, server := setupContainerTestServer()
	_ = setupRepoWithGovcfile(t, server, "test-repo", "BASE alpine")
	
	// Start multiple builds
	for i := 0; i < 3; i++ {
		buildReq := container.BuildRequest{
			RepositoryID: "test-repo",
			Govcfile:     "Govcfile",
			Tags:         []string{fmt.Sprintf("test:v%d", i)},
		}
		_, err := server.containerManager.StartBuild(buildReq)
		require.NoError(t, err)
	}
	
	// Test GET /api/v1/repos/:repo_id/containers/builds
	req := httptest.NewRequest("GET", "/api/v1/repos/test-repo/containers/builds", nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response struct {
		Builds []container.Build `json:"builds"`
		Count  int               `json:"count"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Len(t, response.Builds, 3)
	assert.Equal(t, 3, response.Count)
}

func TestGetBuild(t *testing.T) {
	router, server := setupContainerTestServer()
	_ = setupRepoWithGovcfile(t, server, "test-repo", "BASE alpine")
	
	// Start a build
	buildReq := container.BuildRequest{
		RepositoryID: "test-repo",
		Govcfile:     "Govcfile",
		Tags:         []string{"test:latest"},
	}
	build, err := server.containerManager.StartBuild(buildReq)
	require.NoError(t, err)
	
	// Wait for build to complete
	time.Sleep(3 * time.Second)
	
	// Test GET /api/v1/repos/:repo_id/containers/builds/:build_id
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/test-repo/containers/builds/%s", build.ID), nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response container.Build
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Equal(t, build.ID, response.ID)
	assert.Equal(t, container.BuildStatusCompleted, response.Status)
	assert.NotEmpty(t, response.ImageID)
}

func TestGetBuildLogs(t *testing.T) {
	router, server := setupContainerTestServer()
	_ = setupRepoWithGovcfile(t, server, "test-repo", "BASE alpine\nRUN echo 'Building...'")
	
	// Start a build
	buildReq := container.BuildRequest{
		RepositoryID: "test-repo",
		Govcfile:     "Govcfile",
		Tags:         []string{"test:latest"},
	}
	build, err := server.containerManager.StartBuild(buildReq)
	require.NoError(t, err)
	
	// Test GET /api/v1/repos/:repo_id/containers/builds/:build_id/logs
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/test-repo/containers/builds/%s/logs", build.ID), nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response struct {
		BuildID string `json:"build_id"`
		Logs    string `json:"logs"`
		Status  string `json:"status"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Equal(t, build.ID, response.BuildID)
	assert.NotEmpty(t, response.Status)
}

func TestGetContainerDefinition(t *testing.T) {
	router, server := setupContainerTestServer()
	// Add a Govcfile
	govcfileContent := `BASE alpine:latest
RUN apk add --no-cache curl
CMD ["/app"]`
	_ = setupRepoWithGovcfile(t, server, "test-repo", govcfileContent)
	
	// Test GET /api/v1/repos/:repo_id/containers/definitions/*path
	req := httptest.NewRequest("GET", "/api/v1/repos/test-repo/containers/definitions/Govcfile", nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	var def container.ContainerDefinition
	err := json.Unmarshal(w.Body.Bytes(), &def)
	require.NoError(t, err)
	
	assert.Equal(t, "Govcfile", def.Path)
	assert.Equal(t, container.TypeGovcfile, def.Type)
	assert.Equal(t, govcfileContent, string(def.Content))
}

func TestCreateContainerDefinition(t *testing.T) {
	router, _ := setupContainerTestServer()
	
	// Create definition request
	createReq := struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	}{
		Path: "Govcfile.prod",
		Content: `BASE alpine:latest
RUN apk add --no-cache curl
CMD ["/app"]`,
		Message: "Add production Govcfile",
		Author: struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}{
			Name:  "Test User",
			Email: "test@example.com",
		},
	}
	
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest("POST", "/api/v1/repos/test-repo/containers/definitions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusCreated, w.Code)
	
	var response struct {
		Path   string                      `json:"path"`
		Commit string                      `json:"commit"`
		Type   container.DefinitionType    `json:"type"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Equal(t, createReq.Path, response.Path)
	assert.NotEmpty(t, response.Commit)
	assert.Equal(t, container.TypeGovcfile, response.Type)
}

func TestUpdateContainerDefinition(t *testing.T) {
	router, server := setupContainerTestServer()
	// Add initial Govcfile
	_ = setupRepoWithGovcfile(t, server, "test-repo", "BASE alpine:latest")
	
	// Update definition request
	updateReq := struct {
		Content string `json:"content"`
		Message string `json:"message"`
	}{
		Content: `BASE alpine:latest
RUN apk add --no-cache curl
CMD ["/app"]`,
		Message: "Update Govcfile with curl",
	}
	
	body, _ := json.Marshal(updateReq)
	req := httptest.NewRequest("PUT", "/api/v1/repos/test-repo/containers/definitions/Govcfile", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response struct {
		Path   string                      `json:"path"`
		Commit string                      `json:"commit"`
		Type   container.DefinitionType    `json:"type"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Equal(t, "Govcfile", response.Path)
	assert.NotEmpty(t, response.Commit)
}

func TestDeleteContainerDefinition(t *testing.T) {
	router, server := setupContainerTestServer()
	// We need to add the file manually since setupRepoWithGovcfile creates "Govcfile"
	repo := setupRepoWithGovcfile(t, server, "test-repo", "BASE alpine:latest")
	
	// Add Govcfile.old to delete
	writeAndStageFile(t, repo, "Govcfile.old", []byte("BASE alpine:latest"))
	repo.SetConfig("user.name", "Test")
	repo.SetConfig("user.email", "test@example.com")
	_, err := repo.Commit("Add old Govcfile")
	require.NoError(t, err)
	
	// Delete request
	deleteReq := struct {
		Message string `json:"message"`
	}{
		Message: "Remove old Govcfile",
	}
	
	body, _ := json.Marshal(deleteReq)
	req := httptest.NewRequest("DELETE", "/api/v1/repos/test-repo/containers/definitions/Govcfile.old", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response struct {
		Path    string `json:"path"`
		Commit  string `json:"commit"`
		Deleted bool   `json:"deleted"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Equal(t, "Govcfile.old", response.Path)
	assert.NotEmpty(t, response.Commit)
	assert.True(t, response.Deleted)
}

func TestInvalidRepository(t *testing.T) {
	router, _ := setupContainerTestServer()
	
	// Test with non-existent repository
	req := httptest.NewRequest("GET", "/api/v1/repos/invalid-repo/containers/definitions", nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusNotFound, w.Code)
	
	var response gin.H
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Contains(t, response["error"], "not found")
}

func TestInvalidBuildRequest(t *testing.T) {
	router, _ := setupContainerTestServer()
	
	// Test with invalid build request (missing required fields)
	req := httptest.NewRequest("POST", "/api/v1/repos/test-repo/containers/build", bytes.NewBuffer([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	// Should get 500 because Govcfile is not found
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestConcurrentBuilds(t *testing.T) {
	router, server := setupContainerTestServer()
	_ = setupRepoWithGovcfile(t, server, "test-repo", "BASE alpine")
	
	// Start multiple builds concurrently
	buildCount := 5
	results := make(chan *httptest.ResponseRecorder, buildCount)
	
	for i := 0; i < buildCount; i++ {
		go func(index int) {
			buildReq := container.BuildRequest{
				Govcfile: "Govcfile",
				Tags:     []string{fmt.Sprintf("test:v%d", index)},
			}
			
			body, _ := json.Marshal(buildReq)
			req := httptest.NewRequest("POST", "/api/v1/repos/test-repo/containers/build", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			
			router.ServeHTTP(w, req)
			results <- w
		}(i)
	}
	
	// Collect results
	for i := 0; i < buildCount; i++ {
		w := <-results
		assert.Equal(t, http.StatusAccepted, w.Code)
		
		var build container.Build
		err := json.Unmarshal(w.Body.Bytes(), &build)
		require.NoError(t, err)
		assert.NotEmpty(t, build.ID)
	}
}

func TestCancelBuild(t *testing.T) {
	router, _ := setupContainerTestServer()
	
	// Test DELETE /api/v1/repos/:repo_id/containers/builds/:build_id
	req := httptest.NewRequest("DELETE", "/api/v1/repos/test-repo/containers/builds/test-build-id", nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	// Currently returns 501 Not Implemented
	assert.Equal(t, http.StatusNotImplemented, w.Code)
	
	var response struct {
		Error   string `json:"error"`
		BuildID string `json:"build_id"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Contains(t, response.Error, "not yet implemented")
}

func TestDetectDefinitionType(t *testing.T) {
	router, server := setupContainerTestServer()
	// Setup repo with a basic Govcfile first
	repo := setupRepoWithGovcfile(t, server, "test-repo", "BASE alpine")
	
	// Add various container definition files
	files := map[string]container.DefinitionType{
		"Govcfile.prod":                container.TypeGovcfile,
		"govc-compose.yml":             container.TypeGovcCompose,
		"govc-compose.yaml":            container.TypeGovcCompose,
		"k8s/deployment.yaml":          container.TypeKubernetes,
		"kubernetes/service.yml":       container.TypeKubernetes,
		"charts/myapp/Chart.yaml":      container.TypeHelm,
	}
	
	// Add all files
	for path := range files {
		writeAndStageFile(t, repo, path, []byte("test content"))
	}
	repo.SetConfig("user.name", "Test")
	repo.SetConfig("user.email", "test@example.com")
	_, err := repo.Commit("Add various container definitions")
	require.NoError(t, err)
	
	// Add back Govcfile to expected files since it was created by setupRepoWithGovcfile
	files["Govcfile"] = container.TypeGovcfile
	
	// Get all definitions
	req := httptest.NewRequest("GET", "/api/v1/repos/test-repo/containers/definitions", nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response struct {
		Definitions []container.ContainerDefinition `json:"definitions"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	// Verify each file has correct type
	for _, def := range response.Definitions {
		expectedType, exists := files[def.Path]
		if exists {
			assert.Equal(t, expectedType, def.Type, "Wrong type for %s", def.Path)
		}
	}
}