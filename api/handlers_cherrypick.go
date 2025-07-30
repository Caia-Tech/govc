package api

import (
	"fmt"
	"net/http"

	"github.com/caiatech/govc/pkg/object"
	"github.com/gin-gonic/gin"
)

// cherryPick applies changes from a specific commit to the current branch
func (s *Server) cherryPick(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req CherryPickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Perform cherry-pick
	commit, err := repo.CherryPick(req.Commit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to cherry-pick: %v", err),
			Code:  "CHERRY_PICK_FAILED",
		})
		return
	}

	// Get list of changed files
	filesChanged := make([]string, 0)
	tree, err := repo.GetObject(commit.TreeHash)
	if err == nil {
		if treeObj, ok := tree.(*object.Tree); ok {
			for _, entry := range treeObj.Entries {
				filesChanged = append(filesChanged, entry.Name)
			}
		}
	}

	c.JSON(http.StatusCreated, CherryPickResponse{
		OriginalCommit: req.Commit,
		NewCommit:      commit.Hash(),
		Message:        commit.Message,
		Author:         commit.Author.Name,
		Email:          commit.Author.Email,
		Timestamp:      commit.Author.Time,
		FilesChanged:   filesChanged,
	})
}

// revert creates a new commit that undoes changes from a specific commit
func (s *Server) revert(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req RevertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Perform revert
	commit, err := repo.Revert(req.Commit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to revert: %v", err),
			Code:  "REVERT_FAILED",
		})
		return
	}

	// Get list of changed files
	filesChanged := make([]string, 0)
	tree, err := repo.GetObject(commit.TreeHash)
	if err == nil {
		if treeObj, ok := tree.(*object.Tree); ok {
			for _, entry := range treeObj.Entries {
				filesChanged = append(filesChanged, entry.Name)
			}
		}
	}

	c.JSON(http.StatusCreated, RevertResponse{
		RevertedCommit: req.Commit,
		NewCommit:      commit.Hash(),
		Message:        commit.Message,
		Author:         commit.Author.Name,
		Email:          commit.Author.Email,
		Timestamp:      commit.Author.Time,
		FilesChanged:   filesChanged,
	})
}