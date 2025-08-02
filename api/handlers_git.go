package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/caiatech/govc/pkg/object"
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

	// Get diff with parent commit
	var diffFiles []FileDiff
	if commit.ParentHash != "" {
		// Get parent and current trees to analyze changes
		parentObj, err := repo.GetObject(commit.ParentHash)
		if err == nil {
			parentCommit := parentObj.(*object.Commit)
			parentTree, _ := repo.GetObject(parentCommit.TreeHash)
			parentTreeObj := parentTree.(*object.Tree)
			
			// Compare trees to find changed files
			changedFiles := make(map[string]bool)
			
			// Check for removed/modified files from parent
			for _, entry := range parentTreeObj.Entries {
				changedFiles[entry.Name] = true
			}
			
			// Check for added/modified files in current
			for _, entry := range treeObj.Entries {
				changedFiles[entry.Name] = true
			}
			
			// Create file diff entries for changed files
			for path := range changedFiles {
				status := "modified"
				
				// Determine actual status
				inParent := false
				inCurrent := false
				
				for _, e := range parentTreeObj.Entries {
					if e.Name == path {
						inParent = true
						break
					}
				}
				
				for _, e := range treeObj.Entries {
					if e.Name == path {
						inCurrent = true
						break
					}
				}
				
				if !inParent && inCurrent {
					status = "added"
				} else if inParent && !inCurrent {
					status = "deleted"
				}
				
				diffFiles = append(diffFiles, FileDiff{
					Path:   path,
					Status: status,
				})
			}
		}
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
		"diff": gin.H{
			"files": diffFiles,
		},
	})
}

