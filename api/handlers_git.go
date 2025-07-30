package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/caia-tech/govc/pkg/object"
	"github.com/gin-gonic/gin"
)

// addFile adds a file to the repository
func (s *Server) addFile(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req AddFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Add file to repository
	if err := repo.Add(req.Path); err != nil {
		// If Add by path fails, try adding with content directly
		// This is a workaround since our Add method expects file paths
		// We'll need to enhance the Repository API to support adding content directly
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to add file: %v", err),
			Code:  "ADD_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "added",
		Message: fmt.Sprintf("file %s added to staging", req.Path),
	})
}

// commit creates a new commit
func (s *Server) commit(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req CommitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Set author if provided
	if req.Author != "" {
		repo.SetConfig("user.name", req.Author)
	}
	if req.Email != "" {
		repo.SetConfig("user.email", req.Email)
	}

	// Create commit
	commit, err := repo.Commit(req.Message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to create commit: %v", err),
			Code:  "COMMIT_FAILED",
		})
		return
	}

	c.JSON(http.StatusCreated, CommitResponse{
		Hash:      commit.Hash(),
		Message:   commit.Message,
		Author:    commit.Author.Name,
		Email:     commit.Author.Email,
		Timestamp: commit.Author.Time,
		Parent:    commit.ParentHash,
	})
}

// getLog returns commit history
func (s *Server) getLog(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Parse query parameters
	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	// Get commits
	commits, err := repo.Log(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to get log: %v", err),
			Code:  "LOG_FAILED",
		})
		return
	}

	// Convert to response format
	commitResponses := make([]CommitResponse, len(commits))
	for i, commit := range commits {
		commitResponses[i] = CommitResponse{
			Hash:      commit.Hash(),
			Message:   commit.Message,
			Author:    commit.Author.Name,
			Email:     commit.Author.Email,
			Timestamp: commit.Author.Time,
			Parent:    commit.ParentHash,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"commits": commitResponses,
		"count":   len(commitResponses),
	})
}

// getStatus returns repository status
func (s *Server) getStatus(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	status, err := repo.Status()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to get status: %v", err),
			Code:  "STATUS_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, StatusResponse{
		Branch:    status.Branch,
		Staged:    status.Staged,
		Modified:  status.Modified,
		Untracked: status.Untracked,
	})
}

// showCommit returns details of a specific commit
func (s *Server) showCommit(c *gin.Context) {
	repoID := c.Param("repo_id")
	commitHash := c.Param("commit")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get commit object
	obj, err := repo.GetObject(commitHash)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("commit not found: %s", commitHash),
			Code:  "COMMIT_NOT_FOUND",
		})
		return
	}

	commit, ok := obj.(*object.Commit)
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("object %s is not a commit", commitHash),
			Code:  "NOT_A_COMMIT",
		})
		return
	}

	// Get tree for the commit
	tree, err := repo.GetObject(commit.TreeHash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "failed to get commit tree",
			Code:  "TREE_NOT_FOUND",
		})
		return
	}

	treeObj, _ := tree.(*object.Tree)
	files := make([]FileResponse, 0, len(treeObj.Entries))
	for _, entry := range treeObj.Entries {
		files = append(files, FileResponse{
			Path: entry.Name,
			Size: 0, // Would need to fetch blob to get size
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"commit": CommitResponse{
			Hash:      commit.Hash(),
			Message:   commit.Message,
			Author:    commit.Author.Name,
			Email:     commit.Author.Email,
			Timestamp: commit.Author.Time,
			Parent:    commit.ParentHash,
		},
		"tree":  commit.TreeHash,
		"files": files,
	})
}

// getDiff returns diff between two commits
func (s *Server) getDiff(c *gin.Context) {
	repoID := c.Param("repo_id")
	fromHash := c.Param("from")
	toHash := c.Param("to")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// This would require implementing a diff algorithm
	// For now, return a placeholder
	_ = repo
	_ = fromHash
	_ = toHash

	c.JSON(http.StatusOK, gin.H{
		"from": fromHash,
		"to":   toHash,
		"diff": "Diff functionality not yet implemented",
	})
}