package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// getBlame shows line-by-line authorship for a file
func (s *Server) getBlame(c *gin.Context) {
	repoID := c.Param("repo_id")
	filePath := c.Param("path")

	// Clean the path
	filePath = strings.TrimPrefix(filePath, "/")
	if filePath == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "file path cannot be empty",
			Code:  "INVALID_PATH",
		})
		return
	}

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get optional ref parameter (defaults to HEAD)
	ref := c.DefaultQuery("ref", "HEAD")

	// Get line range parameters
	startLine := 0
	endLine := -1 // -1 means all lines

	if startStr := c.Query("start"); startStr != "" {
		if start, err := strconv.Atoi(startStr); err == nil && start > 0 {
			startLine = start - 1 // Convert to 0-based
		}
	}

	if endStr := c.Query("end"); endStr != "" {
		if end, err := strconv.Atoi(endStr); err == nil && end > 0 {
			endLine = end
		}
	}

	// Get blame information
	blameInfo, err := repo.Blame(filePath, ref)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: fmt.Sprintf("file '%s' not found at ref '%s'", filePath, ref),
				Code:  "FILE_NOT_FOUND",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to generate blame: %v", err),
			Code:  "BLAME_FAILED",
		})
		return
	}

	// Convert internal types to API types
	blameLines := make([]BlameLine, 0, len(blameInfo.Lines))
	for _, line := range blameInfo.Lines {
		// Apply line range filter if specified
		if startLine > 0 && line.LineNumber < startLine+1 {
			continue
		}
		if endLine > 0 && line.LineNumber > endLine {
			continue
		}

		blameLines = append(blameLines, BlameLine{
			LineNumber: line.LineNumber,
			Content:    line.Content,
			CommitHash: line.CommitHash,
			Author:     line.Author,
			Email:      line.Email,
			Timestamp:  line.Timestamp,
			Message:    line.Message,
		})
	}

	c.JSON(http.StatusOK, BlameResponse{
		Path:  blameInfo.Path,
		Ref:   blameInfo.Ref,
		Lines: blameLines,
		Total: len(blameLines),
	})
}
