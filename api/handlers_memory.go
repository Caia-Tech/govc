package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// beginTransaction starts a new transaction
func (s *Server) beginTransaction(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Create transaction
	tx := repo.Transaction()
	txID := fmt.Sprintf("tx_%s_%s", repoID, uuid.New().String())

	s.mu.Lock()
	s.transactions[txID] = tx
	s.mu.Unlock()

	// Update metrics
	s.updateMetrics()

	c.JSON(http.StatusCreated, TransactionResponse{
		ID:        txID,
		RepoID:    repoID,
		CreatedAt: time.Now(),
	})
}

// transactionAdd adds a file to a transaction
func (s *Server) transactionAdd(c *gin.Context) {
	txID := c.Param("tx_id")

	s.mu.RLock()
	tx, exists := s.transactions[txID]
	s.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "transaction not found",
			Code:  "TX_NOT_FOUND",
		})
		return
	}

	var req TransactionAddRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Add file to transaction
	tx.Add(req.Path, []byte(req.Content))

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "added",
		Message: fmt.Sprintf("file %s added to transaction", req.Path),
	})
}

// transactionValidate validates a transaction
func (s *Server) transactionValidate(c *gin.Context) {
	txID := c.Param("tx_id")

	s.mu.RLock()
	tx, exists := s.transactions[txID]
	s.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "transaction not found",
			Code:  "TX_NOT_FOUND",
		})
		return
	}

	// Validate transaction
	if err := tx.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("validation failed: %v", err),
			Code:  "VALIDATION_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "valid",
		Message: "transaction is valid",
	})
}

// transactionCommit commits a transaction
func (s *Server) transactionCommit(c *gin.Context) {
	txID := c.Param("tx_id")

	s.mu.Lock()
	tx, exists := s.transactions[txID]
	if exists {
		delete(s.transactions, txID) // Remove transaction after commit
	}
	s.mu.Unlock()

	// Update metrics if transaction was deleted
	if exists {
		s.updateMetrics()
	}

	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "transaction not found",
			Code:  "TX_NOT_FOUND",
		})
		return
	}

	var req TransactionCommitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Commit transaction
	commit, err := tx.Commit(req.Message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("commit failed: %v", err),
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

// transactionRollback rolls back a transaction
func (s *Server) transactionRollback(c *gin.Context) {
	txID := c.Param("tx_id")

	s.mu.Lock()
	tx, exists := s.transactions[txID]
	if exists {
		delete(s.transactions, txID)
	}
	s.mu.Unlock()

	// Update metrics if transaction was deleted
	if exists {
		s.updateMetrics()
	}

	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "transaction not found",
			Code:  "TX_NOT_FOUND",
		})
		return
	}

	// Rollback transaction
	tx.Rollback()

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "rolled back",
		Message: "transaction rolled back",
	})
}

// createParallelRealities creates multiple isolated branches
func (s *Server) createParallelRealities(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req ParallelRealitiesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Create parallel realities
	realities := repo.ParallelRealities(req.Branches)

	// Convert to response format
	realityResponses := make([]RealityResponse, len(realities))
	for i, reality := range realities {
		realityResponses[i] = RealityResponse{
			Name:      reality.Name(),
			Isolated:  true,
			Ephemeral: true,
			CreatedAt: time.Now(),
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"realities": realityResponses,
		"count":     len(realityResponses),
	})
}

// listParallelRealities lists all parallel realities for a repository
func (s *Server) listParallelRealities(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// List branches that start with "parallel/"
	branches, err := repo.ListBranches()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to list branches: %v", err),
			Code:  "LIST_FAILED",
		})
		return
	}

	realities := make([]RealityResponse, 0)
	for _, branch := range branches {
		if len(branch.Name) > 17 && branch.Name[:17] == "refs/heads/parallel/" {
			name := branch.Name[17:] // Remove "refs/heads/parallel/" prefix
			realities = append(realities, RealityResponse{
				Name:      name,
				Isolated:  true,
				Ephemeral: true,
				CreatedAt: time.Now(), // Would need to track this properly
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"realities": realities,
		"count":     len(realities),
	})
}

// applyToReality applies changes to a specific reality
func (s *Server) applyToReality(c *gin.Context) {
	repoID := c.Param("repo_id")
	realityName := c.Param("reality")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get the reality (it's just a branch)
	reality := repo.ParallelReality(realityName)

	var req struct {
		Changes map[string]string `json:"changes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Convert changes to byte map
	changes := make(map[string][]byte)
	for path, content := range req.Changes {
		changes[path] = []byte(content)
	}

	// Apply changes
	if err := reality.Apply(changes); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to apply changes: %v", err),
			Code:  "APPLY_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Status:  "applied",
		Message: fmt.Sprintf("changes applied to reality %s", realityName),
	})
}

// benchmarkReality runs benchmark on a reality
func (s *Server) benchmarkReality(c *gin.Context) {
	repoID := c.Param("repo_id")
	realityName := c.Param("reality")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Check if the reality exists
	branchName := fmt.Sprintf("parallel/%s", realityName)
	branches, err := repo.ListBranches()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "failed to list branches",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	found := false
	for _, branch := range branches {
		if branch.Name == fmt.Sprintf("refs/heads/%s", branchName) {
			found = true
			break
		}
	}

	if !found {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("reality '%s' not found", realityName),
			Code:  "REALITY_NOT_FOUND",
		})
		return
	}

	// Get the reality
	reality := repo.ParallelReality(realityName)

	// Run benchmark
	result := reality.Benchmark()

	c.JSON(http.StatusOK, gin.H{
		"reality":   realityName,
		"benchmark": result,
		"better":    result.Better(),
	})
}

// timeTravel returns repository state at a specific time
func (s *Server) timeTravel(c *gin.Context) {
	repoID := c.Param("repo_id")
	timestampStr := c.Param("timestamp")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Parse timestamp
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid timestamp format",
			Code:  "INVALID_TIMESTAMP",
		})
		return
	}

	targetTime := time.Unix(timestamp, 0)

	// Time travel
	snapshot := repo.TimeTravel(targetTime)
	if snapshot == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "no commits found before specified time",
			Code:  "NO_COMMITS",
		})
		return
	}

	commit := snapshot.LastCommit()

	c.JSON(http.StatusOK, gin.H{
		"timestamp": targetTime.Unix(),
		"commit": CommitResponse{
			Hash:      commit.Hash(),
			Message:   commit.Message,
			Author:    commit.Author.Name,
			Email:     commit.Author.Email,
			Timestamp: commit.Author.Time,
			Parent:    commit.ParentHash,
		},
	})
}

// timeTravelRead reads a file at a specific time
func (s *Server) timeTravelRead(c *gin.Context) {
	repoID := c.Param("repo_id")
	timestampStr := c.Param("timestamp")
	path := c.Param("path")

	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Parse timestamp
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid timestamp format",
			Code:  "INVALID_TIMESTAMP",
		})
		return
	}

	targetTime := time.Unix(timestamp, 0)

	// Time travel
	snapshot := repo.TimeTravel(targetTime)
	if snapshot == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "no commits found before specified time",
			Code:  "NO_COMMITS",
		})
		return
	}

	// Read file at that time
	content, err := snapshot.Read(path)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("file not found at specified time: %s", path),
			Code:  "FILE_NOT_FOUND",
		})
		return
	}

	c.JSON(http.StatusOK, FileResponse{
		Path:    path,
		Content: string(content),
		Size:    len(content),
	})
}

// watchEvents sets up WebSocket for watching events
func (s *Server) watchEvents(c *gin.Context) {
	// This would require WebSocket implementation
	// For now, return a placeholder
	c.JSON(http.StatusNotImplemented, ErrorResponse{
		Error: "WebSocket support not yet implemented",
		Code:  "NOT_IMPLEMENTED",
	})
}