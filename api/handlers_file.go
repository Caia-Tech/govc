package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// readFile reads file content from repository
func (s *Server) readFile(c *gin.Context) {
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

	// Check for ref query parameter (commit hash or branch name)
	ref := c.Query("ref")
	
	// If ref is specified, we need to read from that specific commit/branch
	if ref != "" {
		// Try to checkout the ref temporarily
		currentBranch, _ := repo.CurrentBranch()
		defer func() {
			// Restore original branch
			repo.Checkout(currentBranch)
		}()
		
		if err := repo.Checkout(ref); err != nil {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: fmt.Sprintf("ref '%s' not found", ref),
				Code:  "REF_NOT_FOUND",
			})
			return
		}
	}

	// Read the file content
	content, err := repo.ReadFile(filePath)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("file '%s' not found", filePath),
			Code:  "FILE_NOT_FOUND",
		})
		return
	}

	// Determine encoding based on content
	encoding := "utf-8"
	contentStr := string(content)
	if !utf8.ValidString(contentStr) {
		// Binary file, encode as base64
		encoding = "base64"
		contentStr = base64.StdEncoding.EncodeToString(content)
	}

	c.JSON(http.StatusOK, ReadFileResponse{
		Path:     filePath,
		Content:  contentStr,
		Encoding: encoding,
		Size:     int64(len(content)),
	})
}

// writeFile writes file content to working directory
func (s *Server) writeFile(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req WriteFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Write the file to the working directory
	if err := repo.WriteFile(req.Path, []byte(req.Content)); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to write file: %v", err),
			Code:  "WRITE_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "written",
		Message: fmt.Sprintf("file %s written to working directory", req.Path),
	})
}

// listTree lists directory contents
func (s *Server) listTree(c *gin.Context) {
	repoID := c.Param("repo_id")
	dirPath := c.Param("path")
	
	// Clean the path
	dirPath = strings.TrimPrefix(dirPath, "/")
	if dirPath == "" {
		dirPath = "."
	}

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Check if recursive listing is requested
	recursive := c.Query("recursive") == "true"
	
	// Get all files in the repository
	allFiles, err := repo.ListFiles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to list files: %v", err),
			Code:  "LIST_FILES_FAILED",
		})
		return
	}

	// Filter files based on directory path
	entries := make([]TreeEntry, 0)
	seenDirs := make(map[string]bool)
	
	for _, file := range allFiles {
		// Skip if not in the requested directory
		if dirPath != "." {
			if !strings.HasPrefix(file, dirPath+"/") {
				continue
			}
		}
		
		// Get relative path from the directory
		relPath := file
		if dirPath != "." {
			relPath = strings.TrimPrefix(file, dirPath+"/")
		}
		
		// Split the path
		parts := strings.Split(relPath, "/")
		
		if !recursive && len(parts) > 1 {
			// Only show immediate children
			dirName := parts[0]
			dirFullPath := path.Join(dirPath, dirName)
			if dirPath == "." {
				dirFullPath = dirName
			}
			
			if !seenDirs[dirFullPath] {
				seenDirs[dirFullPath] = true
				entries = append(entries, TreeEntry{
					Name: dirName,
					Path: dirFullPath,
					Type: "dir",
					Mode: "040755",
				})
			}
		} else {
			// Show the file
			content, _ := repo.ReadFile(file)
			entries = append(entries, TreeEntry{
				Name: filepath.Base(file),
				Path: file,
				Type: "file",
				Size: int64(len(content)),
				Mode: "100644",
			})
		}
	}

	// Return both "entries" and "files" for backward compatibility
	c.JSON(http.StatusOK, gin.H{
		"path":    dirPath,
		"entries": entries,
		"files":   entries, // Alias for backward compatibility
		"total":   len(entries),
	})
}

// removeFile removes a file from the repository
func (s *Server) removeFile(c *gin.Context) {
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

	// Remove the file from staging
	if err := repo.Remove(filePath); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("failed to remove file '%s': %v", filePath, err),
			Code:  "REMOVE_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "removed",
		Message: fmt.Sprintf("file '%s' removed from staging", filePath),
		Data: gin.H{
			"path": filePath,
		},
	})
}

// moveFile moves or renames a file
func (s *Server) moveFile(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req MoveFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Read the content from the source file
	content, err := repo.ReadFile(req.From)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("source file '%s' not found", req.From),
			Code:  "FILE_NOT_FOUND",
		})
		return
	}

	// Write the file at the new location
	// First we need to write it to the worktree, then add it to staging
	if err := repo.WriteFile(req.To, content); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to write file at new location: %v", err),
			Code:  "WRITE_FAILED",
		})
		return
	}
	
	// Add the new file to staging
	if err := repo.Add(req.To); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to add file to staging: %v", err),
			Code:  "ADD_FAILED",
		})
		return
	}

	// Remove the file from the old location
	if err := repo.Remove(req.From); err != nil {
		// Try to rollback
		repo.Remove(req.To)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to remove file from old location: %v", err),
			Code:  "REMOVE_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "moved",
		Message: fmt.Sprintf("file moved from '%s' to '%s'", req.From, req.To),
		Data: gin.H{
			"from": req.From,
			"to":   req.To,
		},
	})
}