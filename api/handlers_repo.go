package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/caia-tech/govc"
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

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if repo already exists
	if _, exists := s.repos[req.ID]; exists {
		c.JSON(http.StatusConflict, ErrorResponse{
			Error: fmt.Sprintf("repository %s already exists", req.ID),
			Code:  "REPO_EXISTS",
		})
		return
	}

	// Check max repos limit
	if len(s.repos) >= s.config.MaxRepos {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error: "maximum number of repositories reached",
			Code:  "MAX_REPOS_REACHED",
		})
		return
	}

	// Create repository
	var repo *govc.Repository
	var path string

	if req.MemoryOnly || s.config.PersistenceDir == "" {
		repo = govc.New()
		path = ":memory:"
	} else {
		path = fmt.Sprintf("%s/%s", s.config.PersistenceDir, req.ID)
		var err error
		repo, err = govc.Init(path)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: fmt.Sprintf("failed to create repository: %v", err),
				Code:  "REPO_CREATE_FAILED",
			})
			return
		}
	}

	// Store repository and metadata
	s.repos[req.ID] = repo
	s.repoMetadata[req.ID] = &RepoMetadata{
		ID:        req.ID,
		CreatedAt: time.Now(),
		Path:      path,
	}

	// Get current branch
	branch, _ := repo.CurrentBranch()

	c.JSON(http.StatusCreated, RepoResponse{
		ID:            req.ID,
		Path:          path,
		CurrentBranch: branch,
		CreatedAt:     s.repoMetadata[req.ID].CreatedAt,
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
	defer s.mu.Unlock()

	if _, exists := s.repos[repoID]; !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("repository %s not found", repoID),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Delete repository and its metadata
	delete(s.repos, repoID)
	delete(s.repoMetadata, repoID)

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

	repos := make([]RepoResponse, 0, len(s.repos))
	for id, repo := range s.repos {
		metadata := s.repoMetadata[id]
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