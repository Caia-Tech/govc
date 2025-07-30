package api

import "time"

// Request types

type CreateRepoRequest struct {
	ID         string `json:"id" binding:"required"`
	MemoryOnly bool   `json:"memory_only"`
}

type AddFileRequest struct {
	Path    string `json:"path" binding:"required"`
	Content string `json:"content" binding:"required"`
}

type CommitRequest struct {
	Message string `json:"message" binding:"required"`
	Author  string `json:"author"`
	Email   string `json:"email"`
}

type CreateBranchRequest struct {
	Name string `json:"name" binding:"required"`
	From string `json:"from"`
}

type CheckoutRequest struct {
	Branch string `json:"branch" binding:"required"`
}

type MergeRequest struct {
	From string `json:"from" binding:"required"`
	To   string `json:"to" binding:"required"`
}

type ParallelRealitiesRequest struct {
	Branches []string `json:"branches" binding:"required"`
}

type TransactionAddRequest struct {
	Path    string `json:"path" binding:"required"`
	Content string `json:"content" binding:"required"`
}

type TransactionCommitRequest struct {
	Message string `json:"message" binding:"required"`
}

// Response types

type RepoResponse struct {
	ID           string    `json:"id"`
	Path         string    `json:"path"`
	CurrentBranch string   `json:"current_branch,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type CommitResponse struct {
	Hash      string    `json:"hash"`
	Message   string    `json:"message"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Timestamp time.Time `json:"timestamp"`
	Parent    string    `json:"parent,omitempty"`
}

type BranchResponse struct {
	Name      string `json:"name"`
	Commit    string `json:"commit"`
	IsCurrent bool   `json:"is_current"`
}

type FileResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int    `json:"size"`
}

type StatusResponse struct {
	Branch    string   `json:"branch"`
	Staged    []string `json:"staged"`
	Modified  []string `json:"modified"`
	Untracked []string `json:"untracked"`
	Clean     bool     `json:"clean"`
}

type TransactionResponse struct {
	ID        string    `json:"id"`
	RepoID    string    `json:"repo_id"`
	CreatedAt time.Time `json:"created_at"`
}

type RealityResponse struct {
	Name      string    `json:"name"`
	Isolated  bool      `json:"isolated"`
	Ephemeral bool      `json:"ephemeral"`
	CreatedAt time.Time `json:"created_at"`
}

type ErrorResponse struct {
	Error   string      `json:"error"`
	Code    string      `json:"code,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

type SuccessResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// Additional response types for tests

type LogResponse struct {
	Commits []CommitResponse `json:"commits"`
	Total   int              `json:"total"`
}

type BranchListResponse struct {
	Branches []BranchResponse `json:"branches"`
	Current  string           `json:"current"`
}