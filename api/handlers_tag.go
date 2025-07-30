package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CreateTagRequest represents a request to create a tag
type CreateTagRequest struct {
	Name    string `json:"name" binding:"required"`
	Message string `json:"message"`
}

// TagResponse represents a tag in the repository
type TagResponse struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

// createTag creates a new tag
func (s *Server) createTag(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req CreateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Create the tag
	if err := repo.CreateTag(req.Name, req.Message); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to create tag: %v", err),
			Code:  "TAG_CREATE_FAILED",
		})
		return
	}

	c.JSON(http.StatusCreated, SuccessResponse{
		Status:  "created",
		Message: fmt.Sprintf("tag %s created", req.Name),
	})
}

// listTags lists all tags in the repository
func (s *Server) listTags(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	tags, err := repo.ListTags()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to list tags: %v", err),
			Code:  "TAG_LIST_FAILED",
		})
		return
	}

	// For now, return simple list of tag names
	// In a real implementation, we'd get the commit hash for each tag
	tagResponses := make([]TagResponse, len(tags))
	for i, tag := range tags {
		tagResponses[i] = TagResponse{
			Name:   tag,
			Commit: "", // TODO: Get actual commit hash
		}
	}

	c.JSON(http.StatusOK, tagResponses)
}