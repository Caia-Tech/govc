package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// readFileV2 reads a file from the repository using new architecture
// @Summary Read a file from repository (V2 Clean Architecture)
// @Description Reads a file from the working directory using V2 clean architecture with binary support
// @Tags V2 Architecture
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param path query string true "File path relative to repository root"
// @Success 200 {object} ReadFileResponse "File contents"
// @Failure 400 {object} ErrorResponse "Missing path parameter"
// @Failure 404 {object} ErrorResponse "Repository or file not found"
// @Router /v2/repositories/{repo_id}/files [get]
func (s *Server) readFileV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	path := c.Query("path")

	if path == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "path parameter is required",
			Code:  "MISSING_PATH",
		})
		return
	}

	// Clean the path
	path = strings.TrimPrefix(path, "/")

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Read file from workspace
	content, err := components.Workspace.ReadFile(path)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("file not found: %s", path),
			Code:  "FILE_NOT_FOUND",
		})
		return
	}

	// Check if binary
	isBinary := isBinaryContent(content)

	response := ReadFileResponse{
		Path:     path,
		Size:     int64(len(content)),
		Encoding: "utf-8",
	}

	if isBinary {
		response.Encoding = "base64"
		response.Content = base64.StdEncoding.EncodeToString(content)
	} else {
		response.Content = string(content)
	}

	c.JSON(http.StatusOK, response)
}

// writeFileV2 writes a file to the repository using new architecture
// @Summary Write a file to repository (V2 Clean Architecture)
// @Description Writes a file to the working directory using V2 clean architecture
// @Tags V2 Architecture
// @Accept json
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param request body WriteFileRequest true "File write request"
// @Success 200 {object} SuccessResponse "File written successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /v2/repositories/{repo_id}/files [put]
func (s *Server) writeFileV2(c *gin.Context) {
	repoID := c.Param("repo_id")

	var req WriteFileRequest
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

	// Convert content to bytes
	content := []byte(req.Content)

	// Write file
	if err := components.Workspace.WriteFile(req.Path, content); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to write file: %v", err),
			Code:  "WRITE_FAILED",
		})
		return
	}

	// No auto-add or auto-commit in basic write file

	c.JSON(http.StatusCreated, SuccessResponse{
		Message: fmt.Sprintf("file '%s' written", req.Path),
	})
}

// listTreeV2 lists files in a directory using new architecture
func (s *Server) listTreeV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	path := c.Param("path")

	// Clean the path
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		path = "."
	}

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// List all files
	files, err := components.Workspace.ListFiles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to list files: %v", err),
			Code:  "LIST_FAILED",
		})
		return
	}

	// Filter by path
	var entries []TreeEntry
	seen := make(map[string]bool)

	for _, file := range files {
		// Check if file is in the requested path
		if path != "." && !strings.HasPrefix(file, path+"/") {
			continue
		}

		// Get relative path
		relPath := file
		if path != "." {
			relPath = strings.TrimPrefix(file, path+"/")
		}

		// Check if it's a direct child
		parts := strings.Split(relPath, "/")
		if len(parts) == 0 {
			continue
		}

		// For directories, only show the first level
		name := parts[0]
		entryPath := filepath.Join(path, name)
		if path == "." {
			entryPath = name
		}

		// Skip if already seen
		if seen[name] {
			continue
		}
		seen[name] = true

		// Determine type
		entryType := "file"
		if len(parts) > 1 {
			entryType = "tree"
		}

		// Get size for files
		var size int64
		if entryType == "file" {
			if content, err := components.Workspace.ReadFile(file); err == nil {
				size = int64(len(content))
			}
		}

		entries = append(entries, TreeEntry{
			Name: name,
			Path: entryPath,
			Type: entryType,
			Size: size,
		})
	}

	c.JSON(http.StatusOK, TreeResponse{
		Path:    path,
		Entries: entries,
		Total:   len(entries),
	})
}

// removeFileV2 removes a file from the repository using new architecture
func (s *Server) removeFileV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	path := c.Param("path")

	// Clean the path
	path = strings.TrimPrefix(path, "/")

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Remove file
	if err := components.Workspace.Remove(path); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to remove file: %v", err),
			Code:  "REMOVE_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: fmt.Sprintf("file '%s' removed", path),
	})
}

// moveFileV2 moves/renames a file using new architecture
func (s *Server) moveFileV2(c *gin.Context) {
	repoID := c.Param("repo_id")

	var req MoveFileRequest
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

	// Read old file
	content, err := components.Workspace.ReadFile(req.From)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("source file not found: %s", req.From),
			Code:  "FILE_NOT_FOUND",
		})
		return
	}

	// Write to new location
	if err := components.Workspace.WriteFile(req.To, content); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to write new file: %v", err),
			Code:  "WRITE_FAILED",
		})
		return
	}

	// Remove old file
	if err := components.Workspace.Remove(req.From); err != nil {
		// Try to clean up
		components.Workspace.Remove(req.To)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to remove old file: %v", err),
			Code:  "REMOVE_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: fmt.Sprintf("moved '%s' to '%s'", req.From, req.To),
	})
}

// isBinaryContent checks if content appears to be binary
func isBinaryContent(content []byte) bool {
	if len(content) == 0 {
		return false
	}

	// Check for null bytes
	for _, b := range content {
		if b == 0 {
			return true
		}
	}

	// Check for high proportion of non-printable characters
	nonPrintable := 0
	for _, b := range content {
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			nonPrintable++
		}
	}

	return float64(nonPrintable)/float64(len(content)) > 0.3
}
