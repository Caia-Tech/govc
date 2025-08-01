package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/caiatech/govc/container"
	"github.com/gin-gonic/gin"
)

// Initialize container manager as part of the server
func (s *Server) initContainerSystem() {
	if s.containerManager == nil {
		s.containerManager = container.NewManager()
		
		// Register event handler to emit API events
		s.containerManager.OnEvent(func(event container.ContainerEvent) {
			s.logger.Infof("Container event: %s for repo %s", event.Type, event.RepositoryID)
		})
	}
}

// Container definition endpoints

// listContainerDefinitions returns all container definitions in a repository
func (s *Server) listContainerDefinitions(c *gin.Context) {
	repoID := c.Param("repo_id")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Ensure repository is registered with container manager
	if err := s.containerManager.RegisterRepository(repoID, repo); err != nil {
		s.logger.Debugf("Repository already registered: %s", err)
	}

	definitions, err := s.containerManager.GetContainerDefinitions(repoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"definitions": definitions,
		"count":       len(definitions),
	})
}

// getContainerDefinition returns a specific container definition
func (s *Server) getContainerDefinition(c *gin.Context) {
	repoID := c.Param("repo_id")
	path := strings.TrimPrefix(c.Param("path"), "/")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	content, err := repo.ReadFile(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "definition not found"})
		return
	}

	definition := &container.ContainerDefinition{
		Type:    s.detectDefinitionType(path),
		Path:    path,
		Content: content,
	}

	c.JSON(http.StatusOK, definition)
}

// createContainerDefinition adds a new container definition to the repository
func (s *Server) createContainerDefinition(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	var req struct {
		Path    string `json:"path" binding:"required"`
		Content string `json:"content" binding:"required"`
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Add file to repository
	err = repo.WriteFile(req.Path, []byte(req.Content))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	repo.Add(req.Path)

	// Commit if message provided
	if req.Message != "" {
		// Set author if provided
		if req.Author.Name != "" {
			repo.SetConfig("user.name", req.Author.Name)
			repo.SetConfig("user.email", req.Author.Email)
		}
		
		commit, err := repo.Commit(req.Message)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"path":   req.Path,
			"commit": commit.Hash(),
			"type":   s.detectDefinitionType(req.Path),
		})
	} else {
		c.JSON(http.StatusCreated, gin.H{
			"path":   req.Path,
			"staged": true,
			"type":   s.detectDefinitionType(req.Path),
		})
	}
}

// updateContainerDefinition updates an existing container definition
func (s *Server) updateContainerDefinition(c *gin.Context) {
	repoID := c.Param("repo_id")
	path := strings.TrimPrefix(c.Param("path"), "/")
	
	var req struct {
		Content string `json:"content" binding:"required"`
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Update file
	err = repo.WriteFile(path, []byte(req.Content))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	repo.Add(path)

	// Commit if message provided
	if req.Message != "" {
		// Set author if provided
		if req.Author.Name != "" {
			repo.SetConfig("user.name", req.Author.Name)
			repo.SetConfig("user.email", req.Author.Email)
		}
		
		commit, err := repo.Commit(req.Message)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"path":   path,
			"commit": commit.Hash(),
			"type":   s.detectDefinitionType(path),
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"path":   path,
			"staged": true,
			"type":   s.detectDefinitionType(path),
		})
	}
}

// deleteContainerDefinition removes a container definition
func (s *Server) deleteContainerDefinition(c *gin.Context) {
	repoID := c.Param("repo_id")
	path := strings.TrimPrefix(c.Param("path"), "/")
	
	var req struct {
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	}

	// Optional body for commit message
	c.ShouldBindJSON(&req)

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Remove file
	if err := repo.Remove(path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Commit if message provided
	if req.Message != "" {
		// Set author if provided
		if req.Author.Name != "" {
			repo.SetConfig("user.name", req.Author.Name)
			repo.SetConfig("user.email", req.Author.Email)
		}
		
		commit, err := repo.Commit(req.Message)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"path":    path,
			"commit":  commit.Hash(),
			"deleted": true,
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"path":    path,
			"staged":  true,
			"deleted": true,
		})
	}
}

// Build endpoints

// startBuild initiates a container build
func (s *Server) startBuild(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	var req container.BuildRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set repository ID from path
	req.RepositoryID = repoID

	// Ensure repository is registered
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	if err := s.containerManager.RegisterRepository(repoID, repo); err != nil {
		s.logger.Debugf("Repository already registered: %s", err)
	}

	// Start build
	build, err := s.containerManager.StartBuild(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, build)
}

// getBuild returns build information
func (s *Server) getBuild(c *gin.Context) {
	buildID := c.Param("build_id")

	build, err := s.containerManager.GetBuild(buildID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, build)
}

// listBuilds returns all builds for a repository
func (s *Server) listBuilds(c *gin.Context) {
	repoID := c.Param("repo_id")

	builds, err := s.containerManager.ListBuilds(repoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"builds": builds,
		"count":  len(builds),
	})
}

// cancelBuild cancels a running build
func (s *Server) cancelBuild(c *gin.Context) {
	buildID := c.Param("build_id")

	// TODO: Implement build cancellation
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "build cancellation not yet implemented",
		"build_id": buildID,
	})
}

// getBuildLogs streams build logs
func (s *Server) getBuildLogs(c *gin.Context) {
	buildID := c.Param("build_id")

	build, err := s.containerManager.GetBuild(buildID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Return build output
	c.JSON(http.StatusOK, gin.H{
		"build_id": buildID,
		"logs":     build.Output,
		"status":   build.Status,
	})
}

// Helper function to detect container definition type
func (s *Server) detectDefinitionType(path string) container.DefinitionType {
	base := filepath.Base(path)
	
	if strings.HasPrefix(strings.ToLower(base), "govcfile") {
		return container.TypeGovcfile
	}
	
	// Legacy support for Dockerfile
	if strings.HasPrefix(strings.ToLower(base), "dockerfile") {
		return container.TypeGovcfile // Convert to govc format
	}
	
	if strings.Contains(base, "govc-compose") {
		return container.TypeGovcCompose
	}
	
	// Legacy support for docker-compose
	if strings.Contains(base, "docker-compose") {
		return container.TypeGovcCompose // Convert to govc format
	}
	
	if base == "Chart.yaml" || base == "values.yaml" {
		return container.TypeHelm
	}
	
	// Check for Kubernetes-related paths
	lower := strings.ToLower(path)
	if strings.Contains(lower, "k8s") || strings.Contains(lower, "kubernetes") ||
		strings.Contains(lower, "deployment") || strings.Contains(lower, "service") {
		ext := filepath.Ext(path)
		if ext == ".yaml" || ext == ".yml" {
			return container.TypeKubernetes
		}
	}
	
	// Default to Kubernetes for YAML files
	ext := filepath.Ext(path)
	if ext == ".yaml" || ext == ".yml" {
		return container.TypeKubernetes
	}
	
	return container.TypeGovcfile
}

// setupContainerRoutes registers container-related routes
func (s *Server) setupContainerRoutes(v1 *gin.RouterGroup) {
	// Initialize container system
	s.initContainerSystem()

	// Container routes under repos
	containers := v1.Group("/repos/:repo_id/containers")
	if s.config.Auth.Enabled {
		containers.Use(s.authMiddleware.OptionalAuth())
	}
	{
		// Definition management
		containers.GET("/definitions", s.listContainerDefinitions)
		containers.POST("/definitions", s.createContainerDefinition)
		containers.GET("/definitions/*path", s.getContainerDefinition)
		containers.PUT("/definitions/*path", s.updateContainerDefinition)
		containers.DELETE("/definitions/*path", s.deleteContainerDefinition)

		// Build operations
		containers.POST("/build", s.startBuild)
		containers.GET("/builds", s.listBuilds)
		containers.GET("/builds/:build_id", s.getBuild)
		containers.DELETE("/builds/:build_id", s.cancelBuild)
		containers.GET("/builds/:build_id/logs", s.getBuildLogs)
	}
}