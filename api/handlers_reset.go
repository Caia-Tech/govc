package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// reset moves the current branch pointer to a specific commit
func (s *Server) reset(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req ResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Get old HEAD before reset
	oldHead, _ := repo.CurrentCommit()
	oldHeadHash := ""
	if oldHead != nil {
		oldHeadHash = oldHead.Hash()
	}

	// Perform reset
	if err := repo.Reset(req.Target, req.Mode); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to reset: %v", err),
			Code:  "RESET_FAILED",
		})
		return
	}

	// Get new HEAD after reset
	newHead, _ := repo.CurrentCommit()
	newHeadHash := ""
	if newHead != nil {
		newHeadHash = newHead.Hash()
	}

	// Determine actual mode used
	mode := req.Mode
	if mode == "" {
		mode = "mixed"
	}

	c.JSON(http.StatusOK, ResetResponse{
		OldHead: oldHeadHash,
		NewHead: newHeadHash,
		Mode:    mode,
	})
}

// rebase replays commits from the current branch onto another branch
func (s *Server) rebase(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req RebaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Get old HEAD before rebase
	oldHead, _ := repo.CurrentCommit()
	oldHeadHash := ""
	if oldHead != nil {
		oldHeadHash = oldHead.Hash()
	}

	// Perform rebase
	rebasedCommits, err := repo.Rebase(req.Onto)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to rebase: %v", err),
			Code:  "REBASE_FAILED",
		})
		return
	}

	// Get new HEAD after rebase
	newHead, _ := repo.CurrentCommit()
	newHeadHash := ""
	if newHead != nil {
		newHeadHash = newHead.Hash()
	}

	c.JSON(http.StatusOK, RebaseResponse{
		OldHead:      oldHeadHash,
		NewHead:      newHeadHash,
		RebasedCount: len(rebasedCommits),
		Commits:      rebasedCommits,
	})
}