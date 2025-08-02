package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// listBranches returns all branches
// @Summary List all branches
// @Description Retrieves a list of all branches in the repository
// @Tags Git Operations
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Success 200 {object} object{branches=[]BranchResponse,current=string,count=int} "List of branches"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /repositories/{repo_id}/branches [get]
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
// @Summary Create a new branch
// @Description Creates a new branch from the current or specified branch
// @Tags Git Operations
// @Accept json
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param request body CreateBranchRequest true "Branch creation request"
// @Success 201 {object} SuccessResponse "Branch created successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 404 {object} ErrorResponse "Repository or source branch not found"
// @Failure 409 {object} ErrorResponse "Branch already exists"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /repositories/{repo_id}/branches [post]
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

	// If 'from' is specified, verify it exists and checkout that branch first
	if req.From != "" {
		// Check if source branch exists
		branches, err := repo.ListBranches()
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: fmt.Sprintf("failed to list branches: %v", err),
				Code:  "LIST_BRANCHES_FAILED",
			})
			return
		}

		branchExists := false
		for _, branch := range branches {
			if branch.Name == req.From {
				branchExists = true
				break
			}
		}

		if !branchExists {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: fmt.Sprintf("source branch '%s' not found", req.From),
				Code:  "BRANCH_NOT_FOUND",
			})
			return
		}

		if err := repo.Checkout(req.From); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("failed to checkout source branch %s: %v", req.From, err),
				Code:  "CHECKOUT_FAILED",
			})
			return
		}
	}

	// Check if branch already exists
	branches, err := repo.ListBranches()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to list branches: %v", err),
			Code:  "LIST_BRANCHES_FAILED",
		})
		return
	}

	for _, branch := range branches {
		branchName := strings.TrimPrefix(branch.Name, "refs/heads/")
		if branchName == req.Name {
			c.JSON(http.StatusConflict, ErrorResponse{
				Error: fmt.Sprintf("branch '%s' already exists", req.Name),
				Code:  "BRANCH_EXISTS",
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
// @Summary Delete a branch
// @Description Permanently deletes a branch (cannot delete current branch)
// @Tags Git Operations
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param branch path string true "Branch name"
// @Success 200 {object} SuccessResponse "Branch deleted successfully"
// @Failure 400 {object} ErrorResponse "Cannot delete current branch"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /repositories/{repo_id}/branches/{branch} [delete]
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
// @Summary Checkout a branch
// @Description Switches the working directory to a different branch
// @Tags Git Operations
// @Accept json
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param request body CheckoutRequest true "Checkout request"
// @Success 200 {object} SuccessResponse "Branch checked out successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /repositories/{repo_id}/checkout [post]
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
// @Summary Merge branches
// @Description Merges one branch into another branch
// @Tags Git Operations
// @Accept json
// @Produce json
// @Param repo_id path string true "Repository ID"
// @Param request body MergeRequest true "Merge request"
// @Success 200 {object} SuccessResponse "Branches merged successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 404 {object} ErrorResponse "Repository not found"
// @Failure 409 {object} ErrorResponse "Merge conflict"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /repositories/{repo_id}/merge [post]
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
