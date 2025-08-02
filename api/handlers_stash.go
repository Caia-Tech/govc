package api

import (
	"fmt"
	"net/http"

	"github.com/caiatech/govc"
	"github.com/gin-gonic/gin"
)

// createStash creates a new stash
func (s *Server) createStash(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req StashRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Default message if not provided
	message := req.Message
	if message == "" {
		message = "WIP on " + getCurrentBranch(repo)
	}

	// Create stash
	stash, err := repo.Stash(message, req.IncludeUntracked)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to create stash: %v", err),
			Code:  "STASH_FAILED",
		})
		return
	}

	// Get list of affected files
	files := make([]string, 0)
	for path := range stash.StagedFiles {
		files = append(files, path)
	}
	for path := range stash.WorkingFiles {
		found := false
		for _, existing := range files {
			if existing == path {
				found = true
				break
			}
		}
		if !found {
			files = append(files, path)
		}
	}

	c.JSON(http.StatusCreated, StashResponse{
		ID:        stash.ID,
		Message:   stash.Message,
		Author:    stash.Author.Name,
		Email:     stash.Author.Email,
		Timestamp: stash.Timestamp,
		Files:     files,
	})
}

// listStashes lists all stashes
func (s *Server) listStashes(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	stashes := repo.ListStashes()
	stashResponses := make([]StashResponse, 0, len(stashes))

	for _, stash := range stashes {
		// Get list of affected files
		files := make([]string, 0)
		for path := range stash.StagedFiles {
			files = append(files, path)
		}
		for path := range stash.WorkingFiles {
			found := false
			for _, existing := range files {
				if existing == path {
					found = true
					break
				}
			}
			if !found {
				files = append(files, path)
			}
		}

		stashResponses = append(stashResponses, StashResponse{
			ID:        stash.ID,
			Message:   stash.Message,
			Author:    stash.Author.Name,
			Email:     stash.Author.Email,
			Timestamp: stash.Timestamp,
			Files:     files,
		})
	}

	c.JSON(http.StatusOK, StashListResponse{
		Stashes: stashResponses,
		Count:   len(stashResponses),
	})
}

// getStash returns details of a specific stash
func (s *Server) getStash(c *gin.Context) {
	repoID := c.Param("repo_id")
	stashID := c.Param("stash_id")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	stash, err := repo.GetStash(stashID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "STASH_NOT_FOUND",
		})
		return
	}

	// Get list of affected files
	files := make([]string, 0)
	for path := range stash.StagedFiles {
		files = append(files, path)
	}
	for path := range stash.WorkingFiles {
		found := false
		for _, existing := range files {
			if existing == path {
				found = true
				break
			}
		}
		if !found {
			files = append(files, path)
		}
	}

	c.JSON(http.StatusOK, StashResponse{
		ID:        stash.ID,
		Message:   stash.Message,
		Author:    stash.Author.Name,
		Email:     stash.Author.Email,
		Timestamp: stash.Timestamp,
		Files:     files,
	})
}

// applyStash applies a stash to the working directory
func (s *Server) applyStash(c *gin.Context) {
	repoID := c.Param("repo_id")
	stashID := c.Param("stash_id")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req StashApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// If no body provided, use default values
		req = StashApplyRequest{Drop: false}
	}

	if err := repo.ApplyStash(stashID, req.Drop); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to apply stash: %v", err),
			Code:  "APPLY_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "applied",
		Message: fmt.Sprintf("stash %s applied successfully", stashID),
	})
}

// dropStash removes a stash
func (s *Server) dropStash(c *gin.Context) {
	repoID := c.Param("repo_id")
	stashID := c.Param("stash_id")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	if err := repo.DropStash(stashID); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "STASH_NOT_FOUND",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "dropped",
		Message: fmt.Sprintf("stash %s dropped successfully", stashID),
	})
}

// getCurrentBranch is a helper to get current branch name
func getCurrentBranch(repo *govc.Repository) string {
	branch, err := repo.CurrentBranch()
	if err != nil {
		return "main"
	}
	return branch
}
