package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// createRepo creates a new repository
func (s *Server) createRepo(c *gin.Context) {
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

	// Check if repo already exists and get count
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

	// Determine path based on storage configuration
	var path string
	memoryOnly := req.MemoryOnly || s.config.Storage.Type == "memory"
	if memoryOnly {
		path = ":memory:"
	} else {
		path = fmt.Sprintf("%s/%s", s.config.Storage.DiskPath, req.ID)
	}

	// Create repository through pool (this will initialize it)
	pooledRepo, err := s.repoPool.Get(req.ID, path, memoryOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to create repository: %v", err),
			Code:  "REPO_CREATE_FAILED",
		})
		return
	}

	// Store metadata only (repository is now managed by pool)
	s.mu.Lock()
	s.repoMetadata[req.ID] = &RepoMetadata{
		ID:        req.ID,
		CreatedAt: time.Now(),
		Path:      path,
	}
	s.mu.Unlock()

	// Update metrics
	s.updateMetrics()

	// Get current branch from the pooled repository
	repo := pooledRepo.GetRepository()
	branch, _ := repo.CurrentBranch()

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

// getRepo returns repository information
func (s *Server) getRepo(c *gin.Context) {
	repoID := c.Param("repo_id")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	s.mu.RLock()
	metadata := s.repoMetadata[repoID]
	s.mu.RUnlock()

	branch, _ := repo.CurrentBranch()

	c.JSON(http.StatusOK, RepoResponse{
		ID:            repoID,
		Path:          metadata.Path,
		CurrentBranch: branch,
		CreatedAt:     metadata.CreatedAt,
	})
}

// deleteRepo deletes a repository
func (s *Server) deleteRepo(c *gin.Context) {
	repoID := c.Param("repo_id")

	s.mu.Lock()
	
	if _, exists := s.repoMetadata[repoID]; !exists {
		s.mu.Unlock()
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("repository %s not found", repoID),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Remove repository from pool and metadata
	s.repoPool.Remove(repoID)
	delete(s.repoMetadata, repoID)
	
	s.mu.Unlock()

	// Update metrics (outside the lock to avoid deadlock)
	s.updateMetrics()

	// Clean up any transactions for this repo
	for txID, tx := range s.transactions {
		// Check if transaction belongs to this repo
		// Note: We'd need to track repo ID in transactions for this
		// For now, we'll clean up orphaned transactions periodically
		_ = tx
		_ = txID
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "deleted",
		Message: fmt.Sprintf("repository %s deleted", repoID),
	})
}

// listRepos lists all repositories
func (s *Server) listRepos(c *gin.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repos := make([]RepoResponse, 0, len(s.repoMetadata))
	for id, metadata := range s.repoMetadata {
		// Get repository from pool to get current branch
		pooledRepo, err := s.repoPool.Get(id, metadata.Path, metadata.Path == ":memory:")
		if err != nil {
			// If we can't get the repo from pool, skip it or show basic info
			repos = append(repos, RepoResponse{
				ID:            id,
				Path:          metadata.Path,
				CurrentBranch: "unknown",
				CreatedAt:     metadata.CreatedAt,
			})
			continue
		}

		repo := pooledRepo.GetRepository()
		branch, _ := repo.CurrentBranch()

		repos = append(repos, RepoResponse{
			ID:            id,
			Path:          metadata.Path,
			CurrentBranch: branch,
			CreatedAt:     metadata.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"repositories": repos,
		"count":        len(repos),
	})
}