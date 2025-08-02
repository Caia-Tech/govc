package govc

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// Transaction represents a govc transaction
type Transaction struct {
	client    *Client
	repoID    string
	ID        string    `json:"id"`
	RepoID    string    `json:"repo_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Transaction creates a new transaction for atomic operations
func (r *Repository) Transaction(ctx context.Context) (*Transaction, error) {
	var tx Transaction
	err := r.client.post(ctx, fmt.Sprintf("/api/v1/repos/%s/transactions", url.PathEscape(r.ID)), nil, &tx)
	if err != nil {
		return nil, err
	}

	tx.client = r.client
	tx.repoID = r.ID
	return &tx, nil
}

// Transaction creates a new transaction for the specified repository
func (c *Client) Transaction(ctx context.Context, repoID string) (*Transaction, error) {
	var tx Transaction
	err := c.post(ctx, fmt.Sprintf("/api/v1/repos/%s/transactions", url.PathEscape(repoID)), nil, &tx)
	if err != nil {
		return nil, err
	}

	tx.client = c
	tx.repoID = repoID
	return &tx, nil
}

// Add adds a file to the transaction
func (tx *Transaction) Add(ctx context.Context, path, content string) error {
	req := struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}{
		Path:    path,
		Content: content,
	}

	var response struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	return tx.client.post(ctx,
		fmt.Sprintf("/api/v1/repos/%s/transactions/%s/add",
			url.PathEscape(tx.repoID), url.PathEscape(tx.ID)),
		req, &response)
}

// Remove removes a file from the transaction
func (tx *Transaction) Remove(ctx context.Context, path string) error {
	var response struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	return tx.client.delete(ctx,
		fmt.Sprintf("/api/v1/repos/%s/transactions/%s/remove/%s",
			url.PathEscape(tx.repoID), url.PathEscape(tx.ID), url.PathEscape(path)))
}

// Validate validates the transaction
func (tx *Transaction) Validate(ctx context.Context) error {
	var response struct {
		Valid   bool     `json:"valid"`
		Errors  []string `json:"errors,omitempty"`
		Message string   `json:"message"`
	}

	err := tx.client.post(ctx,
		fmt.Sprintf("/api/v1/repos/%s/transactions/%s/validate",
			url.PathEscape(tx.repoID), url.PathEscape(tx.ID)),
		nil, &response)

	if err != nil {
		return err
	}

	if !response.Valid && len(response.Errors) > 0 {
		return fmt.Errorf("validation failed: %v", response.Errors)
	}

	return nil
}

// Commit commits the transaction
func (tx *Transaction) Commit(ctx context.Context, message string) (*Commit, error) {
	req := struct {
		Message string `json:"message"`
	}{
		Message: message,
	}

	var commit Commit
	err := tx.client.post(ctx,
		fmt.Sprintf("/api/v1/repos/%s/transactions/%s/commit",
			url.PathEscape(tx.repoID), url.PathEscape(tx.ID)),
		req, &commit)

	return &commit, err
}

// Rollback rolls back the transaction
func (tx *Transaction) Rollback(ctx context.Context) error {
	var response struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	return tx.client.post(ctx,
		fmt.Sprintf("/api/v1/repos/%s/transactions/%s/rollback",
			url.PathEscape(tx.repoID), url.PathEscape(tx.ID)),
		nil, &response)
}

// Status returns the transaction status
func (tx *Transaction) Status(ctx context.Context) (*TransactionStatus, error) {
	var status TransactionStatus
	err := tx.client.get(ctx,
		fmt.Sprintf("/api/v1/repos/%s/transactions/%s",
			url.PathEscape(tx.repoID), url.PathEscape(tx.ID)),
		&status)

	return &status, err
}

// TransactionStatus represents the status of a transaction
type TransactionStatus struct {
	ID        string            `json:"id"`
	RepoID    string            `json:"repo_id"`
	State     string            `json:"state"`
	Files     map[string]string `json:"files"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}
