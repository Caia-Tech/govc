package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// getDiff generates diff between commits, branches, or working directory
func (s *Server) getDiff(c *gin.Context) {
	repoID := c.Param("repo_id")
	from := c.Param("from")
	to := c.Param("to")

	// Handle query parameters for more flexible diff operations
	if from == "" {
		from = c.Query("from")
	}
	if to == "" {
		to = c.Query("to")
	}

	// Default to HEAD if 'to' is not specified
	if to == "" {
		to = "HEAD"
	}

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get the diff format (unified, raw, name-only)
	format := c.DefaultQuery("format", "unified")

	// Generate the diff
	diff, err := repo.Diff(from, to, format)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "cannot go back") {
			// For cases where we can't resolve the reference, return an empty diff
			diff = ""
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: fmt.Sprintf("failed to generate diff: %v", err),
				Code:  "DIFF_FAILED",
			})
			return
		}
	}

	// Parse the diff to extract file information for backward compatibility
	var files []FileDiff
	if format == "unified" && diff != "" {
		// Simple parsing of unified diff format
		lines := strings.Split(diff, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "diff --git") {
				// Extract file path from diff header
				parts := strings.Fields(line)
				if len(parts) >= 4 {
					path := strings.TrimPrefix(parts[2], "a/")
					files = append(files, FileDiff{
						Path:   path,
						Status: "modified",
					})
				}
			}
		}
	}

	// Return both the raw diff and parsed files
	c.JSON(http.StatusOK, gin.H{
		"from":   from,
		"to":     to,
		"format": format,
		"diff":   diff,
		"files":  files,
	})
}

// getFileDiff generates diff for a specific file
func (s *Server) getFileDiff(c *gin.Context) {
	repoID := c.Param("repo_id")
	filePath := c.Param("path")

	// Clean the path
	filePath = strings.TrimPrefix(filePath, "/")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get from and to refs
	from := c.DefaultQuery("from", "HEAD~1")
	to := c.DefaultQuery("to", "HEAD")

	// Generate file-specific diff
	diff, err := repo.DiffFile(from, to, filePath)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: fmt.Sprintf("file not found in one or both commits: %v", err),
				Code:  "FILE_NOT_FOUND",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to generate file diff: %v", err),
			Code:  "DIFF_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, FileDiffResponse{
		Path: filePath,
		From: from,
		To:   to,
		Diff: diff,
	})
}

// getWorkingDiff shows diff of working directory changes
func (s *Server) getWorkingDiff(c *gin.Context) {
	repoID := c.Param("repo_id")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get diff between HEAD and working directory
	diff, err := repo.DiffWorking()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to generate working diff: %v", err),
			Code:  "DIFF_FAILED",
		})
		return
	}

	// Convert internal types to API types
	stagedDiffs := make([]FileDiff, len(diff.Staged))
	for i, d := range diff.Staged {
		stagedDiffs[i] = FileDiff{
			Path:      d.Path,
			OldPath:   d.OldPath,
			Status:    d.Status,
			Additions: d.Additions,
			Deletions: d.Deletions,
			Patch:     d.Patch,
		}
	}

	unstagedDiffs := make([]FileDiff, len(diff.Unstaged))
	for i, d := range diff.Unstaged {
		unstagedDiffs[i] = FileDiff{
			Path:      d.Path,
			OldPath:   d.OldPath,
			Status:    d.Status,
			Additions: d.Additions,
			Deletions: d.Deletions,
			Patch:     d.Patch,
		}
	}

	// Combine all files for backward compatibility
	allFiles := append(stagedDiffs, unstagedDiffs...)

	// Return both formats for backward compatibility
	c.JSON(http.StatusOK, gin.H{
		"staged":   stagedDiffs,
		"unstaged": unstagedDiffs,
		"total":    len(stagedDiffs) + len(unstagedDiffs),
		"files":    allFiles, // For backward compatibility
	})
}
