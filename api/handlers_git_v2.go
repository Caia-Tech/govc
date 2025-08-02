package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// addFileV2 stages files using new architecture
func (s *Server) addFileV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	var req AddFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// First write the file
	if err := components.Workspace.WriteFile(req.Path, []byte(req.Content)); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to write file: %v", err),
			Code:  "WRITE_FAILED",
		})
		return
	}
	
	// Then add it to staging
	if err := components.Operations.Add(req.Path); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to add file: %v", err),
			Code:  "ADD_FAILED",
		})
		return
	}

	c.JSON(http.StatusCreated, SuccessResponse{
		Status:  "added",
		Message: fmt.Sprintf("file '%s' added", req.Path),
	})
}

// commitV2 creates a commit using new architecture
func (s *Server) commitV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	var req CommitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Set author information
	if req.Author != "" {
		components.Config.Set("user.name", req.Author)
	}
	if req.Email != "" {
		components.Config.Set("user.email", req.Email)
	}

	// Create commit
	hash, err := components.Operations.Commit(req.Message)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("failed to commit: %v", err),
			Code:  "COMMIT_FAILED",
		})
		return
	}

	// Get commit info
	commits, _ := components.Operations.Log(1)
	var response CommitResponse
	if len(commits) > 0 {
		commit := commits[0]
		response = CommitResponse{
			Hash:      commit.Hash(),
			Author:    commit.Author.Name,
			Email:     commit.Author.Email,
			Message:   commit.Message,
			Timestamp: commit.Author.Time,
			Parent:    commit.ParentHash,
		}
	} else {
		// Fallback if log fails
		response = CommitResponse{
			Hash:    hash,
			Message: req.Message,
			Author:  req.Author,
			Email:   req.Email,
		}
	}

	c.JSON(http.StatusCreated, response)
}

// getLogV2 returns commit history using new architecture
func (s *Server) getLogV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	// Get limit from query
	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("invalid limit parameter: %s", limitStr),
				Code:  "INVALID_LIMIT",
			})
			return
		}
		if l <= 0 {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("limit must be positive: %d", l),
				Code:  "INVALID_LIMIT",
			})
			return
		}
		// Cap the limit to prevent excessive memory usage
		if l > 10000 {
			l = 10000
		}
		limit = l
	}

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get commits
	commits, err := components.Operations.Log(limit)
	if err != nil {
		// Check if it's just an empty repository
		if err.Error() == "object not found: " || err.Error() == "reference not found" {
			// Empty repository, return empty list
			c.JSON(http.StatusOK, LogResponse{
				Commits: []CommitResponse{},
				Total:   0,
			})
			return
		}
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
			Author:    commit.Author.Name,
			Email:     commit.Author.Email,
			Message:   commit.Message,
			Timestamp: commit.Author.Time,
			Parent:    commit.ParentHash,
		}
	}

	c.JSON(http.StatusOK, LogResponse{
		Commits: commitResponses,
		Total:   len(commitResponses),
	})
}

// getStatusV2 returns repository status using new architecture
func (s *Server) getStatusV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get status
	status, err := components.Operations.Status()
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
		Clean:     status.Clean(),
	})
}

// createBranchV2 creates a new branch using new architecture
func (s *Server) createBranchV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	var req CreateBranchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Create branch
	if err := components.Operations.Branch(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("failed to create branch: %v", err),
			Code:  "BRANCH_CREATE_FAILED",
		})
		return
	}

	c.JSON(http.StatusCreated, SuccessResponse{
		Message: fmt.Sprintf("branch '%s' created", req.Name),
	})
}

// checkoutV2 switches branches using new architecture
func (s *Server) checkoutV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	var req CheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Checkout
	if err := components.Operations.Checkout(req.Branch); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("failed to checkout: %v", err),
			Code:  "CHECKOUT_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: fmt.Sprintf("switched to branch '%s'", req.Branch),
	})
}

// mergeV2 merges branches using new architecture
func (s *Server) mergeV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	var req MergeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Merge branches (MergeRequest has From and To fields)
	// First checkout the target branch
	if err := components.Operations.Checkout(req.To); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("failed to checkout target branch: %v", err),
			Code:  "CHECKOUT_FAILED",
		})
		return
	}
	
	// Then merge from source
	hash, err := components.Operations.Merge(req.From, fmt.Sprintf("Merge branch '%s' into '%s'", req.From, req.To))
	if err != nil {
		c.JSON(http.StatusConflict, ErrorResponse{
			Error: fmt.Sprintf("failed to merge: %v", err),
			Code:  "MERGE_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: fmt.Sprintf("merged '%s' into '%s' (commit: %s)", req.From, req.To, hash[:7]),
	})
}

// createTagV2 creates a tag using new architecture
func (s *Server) createTagV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Create tag
	if err := components.Operations.Tag(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("failed to create tag: %v", err),
			Code:  "TAG_CREATE_FAILED",
		})
		return
	}

	c.JSON(http.StatusCreated, SuccessResponse{
		Message: fmt.Sprintf("tag '%s' created", req.Name),
	})
}