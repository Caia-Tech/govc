package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// createRepoV2 creates a new repository using clean architecture
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
	components, err := s.repoFactory.CreateRepository(req.ID, path, memoryOnly)
	if err != nil {
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

	c.JSON(http.StatusCreated, RepoResponse{
		ID:            req.ID,
		Path:          path,
		CurrentBranch: branch,
		CreatedAt:     metadata.CreatedAt,
	})
}

// getRepoV2 returns repository information using new architecture
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
func (s *Server) deleteRepoV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	// Check if exists
	s.mu.RLock()
	_, exists := s.repoMetadata[repoID]
	s.mu.RUnlock()
	
	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "repository not found",
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Remove from factory
	if err := s.repoFactory.DeleteRepository(repoID); err != nil {
		// Log error but continue
		s.logger.Errorf("Failed to delete from factory: %v", err)
	}

	// Remove from pool
	s.repoPool.Remove(repoID)

	// Remove metadata
	s.mu.Lock()
	delete(s.repoMetadata, repoID)
	s.mu.Unlock()

	// Update metrics
	s.updateMetrics()

	c.JSON(http.StatusOK, SuccessResponse{
		Message: fmt.Sprintf("repository %s deleted", repoID),
	})
}

// listReposV2 lists all repositories
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

	c.JSON(http.StatusOK, struct {
		Repositories []RepoResponse `json:"repositories"`
		Total        int            `json:"total"`
	}{
		Repositories: reposResponse,
		Total:        len(reposResponse),
	})
}