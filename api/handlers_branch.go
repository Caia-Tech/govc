package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// listBranches returns all branches
func (s *Server) listBranches(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	branches, err := repo.ListBranches()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to list branches: %v", err),
			Code:  "LIST_BRANCHES_FAILED",
		})
		return
	}

	currentBranch, _ := repo.CurrentBranch()

	// Convert to response format
	branchResponses := make([]BranchResponse, len(branches))
	for i, branch := range branches {
		name := strings.TrimPrefix(branch.Name, "refs/heads/")
		branchResponses[i] = BranchResponse{
			Name:      name,
			Commit:    branch.Hash,
			IsCurrent: name == currentBranch,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"branches": branchResponses,
		"current":  currentBranch,
		"count":    len(branchResponses),
	})
}

// createBranch creates a new branch
func (s *Server) createBranch(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req CreateBranchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// If 'from' is specified, checkout that branch first
	if req.From != "" {
		if err := repo.Checkout(req.From); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("failed to checkout source branch %s: %v", req.From, err),
				Code:  "CHECKOUT_FAILED",
			})
			return
		}
	}

	// Create the branch
	if err := repo.Branch(req.Name).Create(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to create branch: %v", err),
			Code:  "CREATE_BRANCH_FAILED",
		})
		return
	}

	c.JSON(http.StatusCreated, SuccessResponse{
		Status:  "created",
		Message: fmt.Sprintf("branch %s created", req.Name),
		Data: gin.H{
			"branch": req.Name,
			"from":   req.From,
		},
	})
}

// deleteBranch deletes a branch
func (s *Server) deleteBranch(c *gin.Context) {
	repoID := c.Param("repo_id")
	branchName := c.Param("branch")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Check if trying to delete current branch
	currentBranch, _ := repo.CurrentBranch()
	if currentBranch == branchName {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "cannot delete current branch",
			Code:  "DELETE_CURRENT_BRANCH",
		})
		return
	}

	// Delete the branch
	if err := repo.Branch(branchName).Delete(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to delete branch: %v", err),
			Code:  "DELETE_BRANCH_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "deleted",
		Message: fmt.Sprintf("branch %s deleted", branchName),
	})
}

// checkout switches to a different branch
func (s *Server) checkout(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req CheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Checkout the branch
	if err := repo.Checkout(req.Branch); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to checkout branch: %v", err),
			Code:  "CHECKOUT_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "checked out",
		Message: fmt.Sprintf("switched to branch %s", req.Branch),
		Data: gin.H{
			"branch": req.Branch,
		},
	})
}

// merge merges one branch into another
func (s *Server) merge(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req MergeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Checkout target branch if specified
	if req.To != "" {
		currentBranch, _ := repo.CurrentBranch()
		if currentBranch != req.To {
			if err := repo.Checkout(req.To); err != nil {
				c.JSON(http.StatusBadRequest, ErrorResponse{
					Error: fmt.Sprintf("failed to checkout target branch %s: %v", req.To, err),
					Code:  "CHECKOUT_FAILED",
				})
				return
			}
		}
	}

	// Perform the merge
	if err := repo.Merge(req.From, req.To); err != nil {
		if strings.Contains(err.Error(), "conflicts") {
			c.JSON(http.StatusConflict, ErrorResponse{
				Error: fmt.Sprintf("merge conflict: %v", err),
				Code:  "MERGE_CONFLICT",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to merge: %v", err),
			Code:  "MERGE_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "merged",
		Message: fmt.Sprintf("merged %s into %s", req.From, req.To),
		Data: gin.H{
			"from": req.From,
			"to":   req.To,
		},
	})
}