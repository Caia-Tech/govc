package govc

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// Repository represents a govc repository
type Repository struct {
	client        *Client
	ID            string    `json:"id"`
	Path          string    `json:"path"`
	CurrentBranch string    `json:"current_branch,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// CreateRepoOptions options for creating a repository
type CreateRepoOptions struct {
	MemoryOnly bool `json:"memory_only"`
}

// CreateRepo creates a new repository
func (c *Client) CreateRepo(ctx context.Context, id string, opts *CreateRepoOptions) (*Repository, error) {
	req := struct {
		ID         string `json:"id"`
		MemoryOnly bool   `json:"memory_only"`
	}{
		ID: id,
	}

	if opts != nil {
		req.MemoryOnly = opts.MemoryOnly
	}

	var repo Repository
	err := c.post(ctx, "/api/v1/repos", req, &repo)
	if err != nil {
		return nil, err
	}

	repo.client = c
	return &repo, nil
}

// GetRepo retrieves a repository by ID
func (c *Client) GetRepo(ctx context.Context, id string) (*Repository, error) {
	var repo Repository
	err := c.get(ctx, fmt.Sprintf("/api/v1/repos/%s", url.PathEscape(id)), &repo)
	if err != nil {
		return nil, err
	}

	repo.client = c
	repo.ID = id
	return &repo, nil
}

// ListRepos lists all repositories
func (c *Client) ListRepos(ctx context.Context) ([]*Repository, error) {
	var response struct {
		Repositories []*Repository `json:"repositories"`
		Count        int           `json:"count"`
	}

	err := c.get(ctx, "/api/v1/repos", &response)
	if err != nil {
		return nil, err
	}

	// Set client reference for each repo
	for _, repo := range response.Repositories {
		repo.client = c
	}

	return response.Repositories, nil
}

// DeleteRepo deletes a repository
func (c *Client) DeleteRepo(ctx context.Context, id string) error {
	return c.delete(ctx, fmt.Sprintf("/api/v1/repos/%s", url.PathEscape(id)))
}

// AddFile adds a file to the repository's staging area
func (r *Repository) AddFile(ctx context.Context, path, content string) error {
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

	err := r.client.post(ctx, fmt.Sprintf("/api/v1/repos/%s/add", url.PathEscape(r.ID)), req, &response)
	return err
}

// Commit creates a new commit
func (r *Repository) Commit(ctx context.Context, message string, author *Author) (*Commit, error) {
	req := struct {
		Message string `json:"message"`
		Author  string `json:"author,omitempty"`
		Email   string `json:"email,omitempty"`
	}{
		Message: message,
	}

	if author != nil {
		req.Author = author.Name
		req.Email = author.Email
	}

	var commit Commit
	err := r.client.post(ctx, fmt.Sprintf("/api/v1/repos/%s/commit", url.PathEscape(r.ID)), req, &commit)
	return &commit, err
}

// Author represents a commit author
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Commit represents a git commit
type Commit struct {
	Hash      string    `json:"hash"`
	Message   string    `json:"message"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Timestamp time.Time `json:"timestamp"`
	Parent    string    `json:"parent,omitempty"`
}

// Status returns the repository status
func (r *Repository) Status(ctx context.Context) (*Status, error) {
	var status Status
	err := r.client.get(ctx, fmt.Sprintf("/api/v1/repos/%s/status", url.PathEscape(r.ID)), &status)
	return &status, err
}

// Status represents repository status
type Status struct {
	Branch    string   `json:"branch"`
	Staged    []string `json:"staged"`
	Modified  []string `json:"modified"`
	Untracked []string `json:"untracked"`
	Clean     bool     `json:"clean"`
}

// Log returns the commit log
func (r *Repository) Log(ctx context.Context, limit int) ([]*Commit, error) {
	path := fmt.Sprintf("/api/v1/repos/%s/log", url.PathEscape(r.ID))
	if limit > 0 {
		path = fmt.Sprintf("%s?limit=%d", path, limit)
	}

	var response struct {
		Commits []*Commit `json:"commits"`
		Count   int       `json:"count"`
	}

	err := r.client.get(ctx, path, &response)
	return response.Commits, err
}

// ReadFile reads a file from the repository
func (r *Repository) ReadFile(ctx context.Context, path string) (string, error) {
	var response struct {
		Path     string `json:"path"`
		Content  string `json:"content"`
		Size     int    `json:"size"`
		Encoding string `json:"encoding,omitempty"`
	}

	err := r.client.get(ctx, fmt.Sprintf("/api/v1/repos/%s/read/%s",
		url.PathEscape(r.ID), url.PathEscape(path)), &response)

	return response.Content, err
}

// WriteFile writes a file to the repository
func (r *Repository) WriteFile(ctx context.Context, path, content string) error {
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

	return r.client.post(ctx, fmt.Sprintf("/api/v1/repos/%s/write", url.PathEscape(r.ID)), req, &response)
}

// Branch operations

// Branch represents a git branch
type Branch struct {
	Name      string `json:"name"`
	Commit    string `json:"commit"`
	IsCurrent bool   `json:"is_current"`
}

// ListBranches lists all branches
func (r *Repository) ListBranches(ctx context.Context) ([]*Branch, error) {
	var response struct {
		Branches []*Branch `json:"branches"`
		Current  string    `json:"current"`
		Count    int       `json:"count"`
	}

	err := r.client.get(ctx, fmt.Sprintf("/api/v1/repos/%s/branches", url.PathEscape(r.ID)), &response)
	return response.Branches, err
}

// CreateBranch creates a new branch
func (r *Repository) CreateBranch(ctx context.Context, name string, from string) error {
	req := struct {
		Name string `json:"name"`
		From string `json:"from,omitempty"`
	}{
		Name: name,
		From: from,
	}

	var response struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	return r.client.post(ctx, fmt.Sprintf("/api/v1/repos/%s/branches", url.PathEscape(r.ID)), req, &response)
}

// Checkout switches to a branch
func (r *Repository) Checkout(ctx context.Context, branch string) error {
	req := struct {
		Branch string `json:"branch"`
	}{
		Branch: branch,
	}

	var response struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	err := r.client.post(ctx, fmt.Sprintf("/api/v1/repos/%s/checkout", url.PathEscape(r.ID)), req, &response)
	if err == nil {
		r.CurrentBranch = branch
	}
	return err
}

// Merge merges branches
func (r *Repository) Merge(ctx context.Context, from, to string) error {
	req := struct {
		From string `json:"from"`
		To   string `json:"to"`
	}{
		From: from,
		To:   to,
	}

	var response struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	return r.client.post(ctx, fmt.Sprintf("/api/v1/repos/%s/merge", url.PathEscape(r.ID)), req, &response)
}

// Tag operations

// Tag represents a git tag
type Tag struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

// ListTags lists all tags
func (r *Repository) ListTags(ctx context.Context) ([]*Tag, error) {
	var tags []*Tag
	err := r.client.get(ctx, fmt.Sprintf("/api/v1/repos/%s/tags", url.PathEscape(r.ID)), &tags)
	return tags, err
}

// CreateTag creates a new tag
func (r *Repository) CreateTag(ctx context.Context, name, message string) error {
	req := struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	}{
		Name:    name,
		Message: message,
	}

	var response struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	return r.client.post(ctx, fmt.Sprintf("/api/v1/repos/%s/tags", url.PathEscape(r.ID)), req, &response)
}
