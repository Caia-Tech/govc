package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/caiatech/govc/logging"
	"github.com/caiatech/govc/validation"
	"github.com/gin-gonic/gin"
)

// addFileV2 stages files using new architecture
// @Summary Add a file to repository (V2 Clean Architecture)
// @Description Writes a file to the working directory and stages it using V2 clean architecture
// @Tags V2 Architecture
// @Accept json
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param request body AddFileRequest true "File addition request"
// @Success 201 {object} SuccessResponse "File added successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /v2/repositories/{repo_id}/files [post]
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

	// Validate file path
	if err := validation.ValidateFilePath(req.Path); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"repo_id":    repoID,
			"file_path":  req.Path,
			"error":      err.Error(),
			"request_id": logging.GetRequestID(c),
		}).Warn("Invalid file path")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_FILE_PATH",
		})
		return
	}
	
	// Validate file content
	if err := validation.ValidateFileContent([]byte(req.Content), req.Path); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"repo_id":    repoID,
			"file_path":  req.Path,
			"file_size":  len(req.Content),
			"error":      err.Error(),
			"request_id": logging.GetRequestID(c),
		}).Warn("Invalid file content")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_FILE_CONTENT",
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

	s.logger.WithFields(map[string]interface{}{
		"repo_id":     repoID,
		"file_path":   req.Path,
		"file_size":   len(req.Content),
		"request_id":  logging.GetRequestID(c),
	}).Info("Adding file to repository")

	// First write the file
	if err := components.Workspace.WriteFile(req.Path, []byte(req.Content)); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"repo_id":    repoID,
			"file_path":  req.Path,
			"error":      err.Error(),
			"request_id": logging.GetRequestID(c),
		}).Error("Failed to write file")
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to write file: %v", err),
			Code:  "WRITE_FAILED",
		})
		return
	}

	// Then add it to staging
	if err := components.Operations.Add(req.Path); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"repo_id":    repoID,
			"file_path":  req.Path,
			"error":      err.Error(),
			"request_id": logging.GetRequestID(c),
		}).Error("Failed to stage file")
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to add file: %v", err),
			Code:  "ADD_FAILED",
		})
		return
	}

	s.logger.WithFields(map[string]interface{}{
		"repo_id":     repoID,
		"file_path":   req.Path,
		"file_size":   len(req.Content),
		"request_id":  logging.GetRequestID(c),
	}).Info("File added successfully")

	c.JSON(http.StatusCreated, SuccessResponse{
		Status:  "added",
		Message: fmt.Sprintf("file '%s' added", req.Path),
	})
}

// commitV2 creates a commit using new architecture
// @Summary Create a new commit (V2 Clean Architecture)
// @Description Creates a new commit with staged changes using V2 clean architecture
// @Tags V2 Architecture
// @Accept json
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param request body CommitRequest true "Commit request"
// @Success 201 {object} CommitResponse "Commit created successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /v2/repositories/{repo_id}/commits [post]
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

	// Validate commit message
	if err := validation.ValidateCommitMessage(req.Message); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"repo_id":    repoID,
			"error":      err.Error(),
			"request_id": logging.GetRequestID(c),
		}).Warn("Invalid commit message")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_COMMIT_MESSAGE",
		})
		return
	}
	
	// Validate email if provided
	if req.Email != "" {
		if err := validation.ValidateEmail(req.Email); err != nil {
			s.logger.WithFields(map[string]interface{}{
				"repo_id":    repoID,
				"email":      req.Email,
				"error":      err.Error(),
				"request_id": logging.GetRequestID(c),
			}).Warn("Invalid email address")
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: err.Error(),
				Code:  "INVALID_EMAIL",
			})
			return
		}
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
	s.logger.WithFields(map[string]interface{}{
		"repo_id":     repoID,
		"message":     req.Message,
		"author":      req.Author,
		"email":       req.Email,
		"request_id":  logging.GetRequestID(c),
	}).Info("Creating commit")

	hash, err := components.Operations.Commit(req.Message)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"repo_id":    repoID,
			"message":    req.Message,
			"author":     req.Author,
			"error":      err.Error(),
			"request_id": logging.GetRequestID(c),
		}).Error("Failed to create commit")
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

	s.logger.WithFields(map[string]interface{}{
		"repo_id":       repoID,
		"commit_hash":   response.Hash,
		"message":       req.Message,
		"author":        response.Author,
		"request_id":    logging.GetRequestID(c),
	}).Info("Commit created successfully")

	c.JSON(http.StatusCreated, response)
}

// getLogV2 returns commit history using new architecture
// @Summary Get commit history (V2 Clean Architecture)
// @Description Retrieves the commit history using V2 clean architecture with enhanced performance
// @Tags V2 Architecture
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param limit query int false "Maximum number of commits to return (default: 20, max: 10000)"
// @Success 200 {object} LogResponse "Commit history"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /v2/repositories/{repo_id}/commits [get]
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
// @Summary Get repository status (V2 Clean Architecture)
// @Description Returns the current status using V2 clean architecture with improved accuracy
// @Tags V2 Architecture
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Success 200 {object} StatusResponse "Repository status"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /v2/repositories/{repo_id}/status [get]
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
// @Summary Create a new branch (V2 Clean Architecture)
// @Description Creates a new branch using V2 clean architecture with enhanced branch management
// @Tags V2 Architecture
// @Accept json
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param request body CreateBranchRequest true "Branch creation request"
// @Success 201 {object} SuccessResponse "Branch created successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Router /v2/repositories/{repo_id}/branches [post]
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

	// Validate branch name
	if err := validation.ValidateBranchName(req.Name); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"repo_id":     repoID,
			"branch_name": req.Name,
			"error":       err.Error(),
			"request_id":  logging.GetRequestID(c),
		}).Warn("Invalid branch name")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_BRANCH_NAME",
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

// listBranchesV2 lists all branches using new architecture
// @Summary List all branches (V2 Clean Architecture)
// @Description Retrieves all branches using V2 clean architecture with enhanced metadata
// @Tags V2 Architecture
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Success 200 {object} object{branches=[]BranchResponse,current=string,count=int} "List of branches"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /v2/repositories/{repo_id}/branches [get]
func (s *Server) listBranchesV2(c *gin.Context) {
	repoID := c.Param("repo_id")

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get branches
	branches, err := components.Repository.ListBranches()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to list branches: %v", err),
			Code:  "LIST_BRANCHES_FAILED",
		})
		return
	}

	// Get current branch from status
	status, _ := components.Workspace.Status()
	currentBranch := "main"
	if status != nil {
		currentBranch = status.Branch
	}

	// Convert to response format
	branchResponses := make([]BranchResponse, len(branches))
	for i, branch := range branches {
		branchResponses[i] = BranchResponse{
			Name:      branch.Name,
			Commit:    branch.Hash,
			IsCurrent: branch.Name == currentBranch,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"branches": branchResponses,
		"current":  currentBranch,
		"count":    len(branchResponses),
	})
}

// deleteBranchV2 deletes a branch using new architecture
func (s *Server) deleteBranchV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	branchName := c.Param("branch")
	
	// URL decode the branch name to handle slashes properly
	if decodedName, err := url.QueryUnescape(branchName); err == nil {
		branchName = decodedName
	}

	if branchName == "main" || branchName == "master" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "cannot delete main/master branch",
			Code:  "PROTECTED_BRANCH",
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

	// Delete branch
	if err := components.Operations.DeleteBranch(branchName); err != nil {
		if err.Error() == fmt.Sprintf("branch not found: %s", branchName) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: err.Error(),
				Code:  "BRANCH_NOT_FOUND",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to delete branch: %v", err),
			Code:  "DELETE_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: fmt.Sprintf("branch '%s' deleted", branchName),
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
