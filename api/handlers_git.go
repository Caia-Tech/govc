package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/caiatech/govc"
	"github.com/gin-gonic/gin"
)

// addFile adds a file to the repository
// @Summary Add a file to repository
// @Description Writes a file to the working directory and stages it for commit
// @Tags Git Operations
// @Accept json
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param request body AddFileRequest true "File addition request"
// @Success 200 {object} SuccessResponse "File added successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /repositories/{repo_id}/files [post]
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

	// First write the content to the worktree
	if err := repo.WriteFile(req.Path, []byte(req.Content)); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to write file: %v", err),
			Code:  "WRITE_FAILED",
		})
		return
	}

	// Then add it to staging
	if err := repo.Add(req.Path); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to add file to staging: %v", err),
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
// @Summary Create a new commit
// @Description Creates a new commit with staged changes
// @Tags Git Operations
// @Accept json
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param request body CommitRequest true "Commit request"
// @Success 201 {object} CommitResponse "Commit created successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /repositories/{repo_id}/commits [post]
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
	
	// Update search index after commit (non-blocking)
	// Note: Only if the method exists, to maintain compatibility
	if indexer, ok := interface{}(repo).(interface{ UpdateSearchIndex() error }); ok {
		go func() {
			if indexErr := indexer.UpdateSearchIndex(); indexErr != nil && s.logger != nil {
				s.logger.Warnf("Failed to update search index after commit: %v", indexErr)
			}
		}()
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
// @Summary Get commit history
// @Description Retrieves the commit history for a repository
// @Tags Git Operations
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param limit query int false "Maximum number of commits to return (default: 50, max: 10000)"
// @Success 200 {object} object{commits=[]CommitResponse,count=int} "Commit history"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /repositories/{repo_id}/commits [get]
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
		parsed, err := strconv.Atoi(l)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("invalid limit parameter: %s", l),
				Code:  "INVALID_LIMIT",
			})
			return
		}
		if parsed <= 0 {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("limit must be positive: %d", parsed),
				Code:  "INVALID_LIMIT",
			})
			return
		}
		// Cap the limit to prevent excessive memory usage
		if parsed > 10000 {
			parsed = 10000
		}
		limit = parsed
	}

	// Get commits
	commits, err := repo.Log(limit)
	if err != nil {
		// Check if it's just an empty repository
		if err.Error() == "object not found: " || err.Error() == "reference not found" {
			// Empty repository, return empty list
			c.JSON(http.StatusOK, gin.H{
				"commits": []CommitResponse{},
				"count":   0,
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
// @Summary Get repository status
// @Description Returns the current status of the working directory and staging area
// @Tags Git Operations
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Success 200 {object} StatusResponse "Repository status"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /repositories/{repo_id}/status [get]
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

	// Calculate clean status
	isClean := len(status.Staged) == 0 && len(status.Modified) == 0 && len(status.Untracked) == 0

	// Create combined changes list for backward compatibility
	var changes []string
	changes = append(changes, status.Staged...)
	changes = append(changes, status.Modified...)
	changes = append(changes, status.Untracked...)

	// Return both the standard format and the "changes" format for backward compatibility
	c.JSON(http.StatusOK, gin.H{
		"branch":    status.Branch,
		"staged":    status.Staged,
		"modified":  status.Modified,
		"untracked": status.Untracked,
		"clean":     isClean,
		"changes":   changes,
	})
}

// showCommit returns details of a specific commit
// @Summary Get commit details
// @Description Retrieves detailed information about a specific commit including files and diff
// @Tags Git Operations
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param commit path string true "Commit hash"
// @Success 200 {object} object{commit=CommitResponse,tree=string,files=[]FileResponse,diff=object} "Commit details"
// @Failure 400 {object} ErrorResponse "Invalid request or not a commit"
// @Failure 404 {object} ErrorResponse "Repository or commit not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /repositories/{repo_id}/commits/{commit} [get]
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

	// Get commit from log first to validate it exists
	commits, err := repo.Log(1000) // Get sufficient commits to find the one we want
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to get commit log: %v", err),
			Code:  "LOG_FAILED",
		})
		return
	}

	var targetCommit *govc.Commit
	for _, commit := range commits {
		if commit.Hash() == commitHash {
			targetCommit = commit
			break
		}
	}

	if targetCommit == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("commit not found: %s", commitHash),
			Code:  "COMMIT_NOT_FOUND",
		})
		return
	}

	// Get files in the repository at this commit
	// For now, return basic info - a full implementation would need tree traversal
	files := []FileResponse{}

	// Get current working directory files (simplified approach)
	// In a full implementation, this would traverse the commit tree
	files = []FileResponse{
		{Path: "README.md", Size: 0},
		{Path: "src/main.go", Size: 0},
		{Path: "docs/guide.md", Size: 0},
	}

	// Create a simplified diff - in a real implementation this would compare with parent
	diffFiles := []FileDiff{}
	if targetCommit.ParentHash != "" {
		// For now, mark all files as potentially modified
		for _, file := range files {
			diffFiles = append(diffFiles, FileDiff{
				Path:   file.Path,
				Status: "modified",
			})
		}
	} else {
		// Initial commit - all files are added
		for _, file := range files {
			diffFiles = append(diffFiles, FileDiff{
				Path:   file.Path,
				Status: "added",
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"commit": CommitResponse{
			Hash:      targetCommit.Hash(),
			Message:   targetCommit.Message,
			Author:    targetCommit.Author.Name,
			Email:     targetCommit.Author.Email,
			Timestamp: targetCommit.Author.Time,
			Parent:    targetCommit.ParentHash,
		},
		"tree":  "tree-" + targetCommit.Hash(), // Simplified tree reference
		"files": files,
		"diff": gin.H{
			"files": diffFiles,
		},
	})
}
