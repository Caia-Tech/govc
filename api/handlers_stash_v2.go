package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// createStashV2 creates a stash using new architecture
func (s *Server) createStashV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	var req StashRequest
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

	// Create stash
	stash, err := components.Stash.Create(req.Message, req.IncludeUntracked)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("failed to create stash: %v", err),
			Code:  "STASH_FAILED",
		})
		return
	}

	// Get author info from config
	author, _ := components.Config.Get("user.name")
	email, _ := components.Config.Get("user.email")
	
	// Get file list
	files := make([]string, 0)
	for path := range stash.Changes {
		files = append(files, path)
	}
	for path := range stash.Index {
		if _, exists := stash.Changes[path]; !exists {
			files = append(files, path)
		}
	}
	for path := range stash.Untracked {
		files = append(files, path)
	}
	
	c.JSON(http.StatusCreated, StashResponse{
		ID:        stash.ID,
		Message:   stash.Message,
		Author:    author,
		Email:     email,
		Timestamp: stash.Timestamp,
		Files:     files,
	})
}

// listStashesV2 lists all stashes using new architecture
func (s *Server) listStashesV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	
	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// List stashes
	stashes := components.Stash.List()
	
	// Get author info from config
	author, _ := components.Config.Get("user.name")
	email, _ := components.Config.Get("user.email")
	
	// Convert to response format
	stashList := make([]StashResponse, len(stashes))
	for i, stash := range stashes {
		// Get file list
		files := make([]string, 0)
		for path := range stash.Changes {
			files = append(files, path)
		}
		for path := range stash.Index {
			if _, exists := stash.Changes[path]; !exists {
				files = append(files, path)
			}
		}
		
		stashList[i] = StashResponse{
			ID:        stash.ID,
			Message:   stash.Message,
			Author:    author,
			Email:     email,
			Timestamp: stash.Timestamp,
			Files:     files,
		}
	}

	c.JSON(http.StatusOK, StashListResponse{
		Stashes: stashList,
		Count:   len(stashList),
	})
}

// getStashV2 returns details of a specific stash
func (s *Server) getStashV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	stashID := c.Param("stash_id")
	
	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get stash
	stash, err := components.Stash.Get(stashID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "STASH_NOT_FOUND",
		})
		return
	}

	// Get author info from config
	author, _ := components.Config.Get("user.name")
	email, _ := components.Config.Get("user.email")
	
	// Get file list
	files := make([]string, 0)
	for path := range stash.Changes {
		files = append(files, path)
	}
	for path := range stash.Index {
		if _, exists := stash.Changes[path]; !exists {
			files = append(files, path)
		}
	}
	for path := range stash.Untracked {
		files = append(files, path)
	}

	c.JSON(http.StatusOK, StashResponse{
		ID:        stash.ID,
		Message:   stash.Message,
		Author:    author,
		Email:     email,
		Timestamp: stash.Timestamp,
		Files:     files,
	})
}

// applyStashV2 applies a stash using new architecture
func (s *Server) applyStashV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	stashID := c.Param("stash_id")
	
	var req StashApplyRequest
	// Drop is optional, default to false
	c.ShouldBindJSON(&req)
	
	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Apply or pop stash
	if req.Drop {
		err = components.Stash.Pop(stashID)
	} else {
		err = components.Stash.Apply(stashID)
	}
	
	if err != nil {
		c.JSON(http.StatusConflict, ErrorResponse{
			Error: fmt.Sprintf("failed to apply stash: %v", err),
			Code:  "STASH_APPLY_FAILED",
		})
		return
	}

	message := "stash applied"
	if req.Drop {
		message = "stash popped"
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: message,
	})
}

// dropStashV2 removes a stash using new architecture
func (s *Server) dropStashV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	stashID := c.Param("stash_id")
	
	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Drop stash
	if err := components.Stash.Drop(stashID); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "STASH_NOT_FOUND",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: fmt.Sprintf("stash '%s' dropped", stashID[:7]),
	})
}