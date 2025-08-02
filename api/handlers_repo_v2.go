package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/caiatech/govc/logging"
	"github.com/caiatech/govc/validation"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// createRepoV2 creates a new repository using clean architecture
// @Summary Create a new repository (V2 Clean Architecture)
// @Description Creates a new Git repository using the V2 clean architecture with enhanced performance and modularity
// @Tags V2 Architecture
// @Accept json
// @Produce json
// @Param request body CreateRepoRequest true "Repository creation request"
// @Success 201 {object} RepoResponse "Repository created successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 409 {object} ErrorResponse "Repository already exists"
// @Failure 503 {object} ErrorResponse "Maximum repositories reached"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /v2/repositories [post]
func (s *Server) createRepoV2(c *gin.Context) {
	var req CreateRepoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Use UUID if no ID provided
	if req.ID == "" {
		req.ID = uuid.New().String()
	}
	
	// Validate repository ID
	if err := validation.ValidateRepositoryID(req.ID); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"repo_id":    req.ID,
			"error":      err.Error(),
			"request_id": logging.GetRequestID(c),
		}).Warn("Invalid repository ID")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REPO_ID",
		})
		return
	}

	// Check if repo already exists
	s.mu.RLock()
	_, exists := s.repoMetadata[req.ID]
	repoCount := len(s.repoMetadata)
	s.mu.RUnlock()

	if exists {
		c.JSON(http.StatusConflict, ErrorResponse{
			Error: fmt.Sprintf("repository %s already exists", req.ID),
			Code:  "REPO_EXISTS",
		})
		return
	}

	if repoCount >= s.config.Server.MaxRepos {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error: "maximum number of repositories reached",
			Code:  "MAX_REPOS_REACHED",
		})
		return
	}

	// Determine path and type
	var path string
	memoryOnly := req.MemoryOnly || s.config.Storage.Type == "memory"
	if memoryOnly {
		path = ":memory:"
	} else {
		path = fmt.Sprintf("%s/%s", s.config.Storage.DiskPath, req.ID)
	}

	// Create repository using new architecture
	s.logger.WithFields(map[string]interface{}{
		"repo_id":     req.ID,
		"path":        path,
		"memory_only": memoryOnly,
		"request_id":  logging.GetRequestID(c),
	}).Info("Creating new repository")

	components, err := s.repoFactory.CreateRepository(req.ID, path, memoryOnly)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"repo_id":    req.ID,
			"path":       path,
			"error":      err.Error(),
			"request_id": logging.GetRequestID(c),
		}).Error("Failed to create repository")
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to create repository: %v", err),
			Code:  "REPO_CREATE_FAILED",
		})
		return
	}

	// No default author configuration in basic request

	// Store metadata
	s.mu.Lock()
	s.repoMetadata[req.ID] = &RepoMetadata{
		ID:        req.ID,
		CreatedAt: time.Now(),
		Path:      path,
	}
	s.mu.Unlock()

	// Update metrics
	s.updateMetrics()

	// Get current branch
	status, _ := components.Operations.Status()
	branch := "main"
	if status != nil {
		branch = status.Branch
	}

	s.mu.RLock()
	metadata := s.repoMetadata[req.ID]
	s.mu.RUnlock()

	s.logger.WithFields(map[string]interface{}{
		"repo_id":        req.ID,
		"path":           path,
		"current_branch": branch,
		"memory_only":    memoryOnly,
		"request_id":     logging.GetRequestID(c),
	}).Info("Repository created successfully")

	c.JSON(http.StatusCreated, RepoResponse{
		ID:            req.ID,
		Path:          path,
		CurrentBranch: branch,
		CreatedAt:     metadata.CreatedAt,
	})
}

// getRepoV2 returns repository information using new architecture
// @Summary Get repository information (V2 Clean Architecture)
// @Description Retrieves detailed information about a specific repository using V2 clean architecture
// @Tags V2 Architecture
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Success 200 {object} RepoResponse "Repository information"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Router /v2/repositories/{repo_id} [get]
func (s *Server) getRepoV2(c *gin.Context) {
	repoID := c.Param("repo_id")

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get metadata
	s.mu.RLock()
	metadata, ok := s.repoMetadata[repoID]
	s.mu.RUnlock()

	if !ok {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "repository metadata not found",
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get current status
	status, _ := components.Operations.Status()
	branch := "main"
	if status != nil {
		branch = status.Branch
	}

	c.JSON(http.StatusOK, RepoResponse{
		ID:            repoID,
		Path:          metadata.Path,
		CurrentBranch: branch,
		CreatedAt:     metadata.CreatedAt,
	})
}

// deleteRepoV2 deletes a repository
// @Summary Delete a repository (V2 Clean Architecture)
// @Description Permanently deletes a repository using V2 clean architecture with proper cleanup
// @Tags V2 Architecture
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Success 200 {object} SuccessResponse "Repository deleted successfully"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Router /v2/repositories/{repo_id} [delete]
func (s *Server) deleteRepoV2(c *gin.Context) {
	repoID := c.Param("repo_id")

	s.logger.WithFields(map[string]interface{}{
		"repo_id":    repoID,
		"request_id": logging.GetRequestID(c),
	}).Info("Deleting repository")

	// Check if exists
	s.mu.RLock()
	metadata, exists := s.repoMetadata[repoID]
	s.mu.RUnlock()

	if !exists {
		s.logger.WithFields(map[string]interface{}{
			"repo_id":    repoID,
			"request_id": logging.GetRequestID(c),
		}).Warn("Attempted to delete non-existent repository")
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "repository not found",
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Remove from factory
	if err := s.repoFactory.DeleteRepository(repoID); err != nil {
		// Log error but continue
		s.logger.WithFields(map[string]interface{}{
			"repo_id":    repoID,
			"error":      err.Error(),
			"request_id": logging.GetRequestID(c),
		}).Error("Failed to delete repository from factory")
	}

	// Remove from pool
	s.repoPool.Remove(repoID)

	// Remove metadata
	s.mu.Lock()
	delete(s.repoMetadata, repoID)
	s.mu.Unlock()

	// Update metrics
	s.updateMetrics()

	s.logger.WithFields(map[string]interface{}{
		"repo_id":    repoID,
		"path":       metadata.Path,
		"request_id": logging.GetRequestID(c),
	}).Info("Repository deleted successfully")

	c.JSON(http.StatusOK, SuccessResponse{
		Message: fmt.Sprintf("repository %s deleted", repoID),
	})
}

// listReposV2 lists all repositories
// @Summary List all repositories (V2 Clean Architecture)
// @Description Retrieves a list of all repositories using V2 clean architecture with enhanced metadata
// @Tags V2 Architecture
// @Produce json
// @Success 200 {object} object{repositories=[]RepoResponse,count=int} "List of repositories"
// @Router /v2/repositories [get]
func (s *Server) listReposV2(c *gin.Context) {
	s.mu.RLock()
	reposResponse := make([]RepoResponse, 0, len(s.repoMetadata))

	for id, metadata := range s.repoMetadata {
		branch := "main"
		// Try to get current branch
		if components, err := s.repoFactory.GetRepository(id); err == nil {
			if status, err := components.Operations.Status(); err == nil {
				branch = status.Branch
			}
		}

		reposResponse = append(reposResponse, RepoResponse{
			ID:            id,
			Path:          metadata.Path,
			CurrentBranch: branch,
			CreatedAt:     metadata.CreatedAt,
		})
	}
	s.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"repositories": reposResponse,
		"count":        len(reposResponse),
	})
}
