package govc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCreateRepo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/repos" {
			t.Errorf("Expected path /api/v1/repos, got %s", r.URL.Path)
		}

		var req struct {
			ID         string `json:"id"`
			MemoryOnly bool   `json:"memory_only"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.ID != "test-repo" {
			t.Errorf("Expected repo ID test-repo, got %s", req.ID)
		}

		repo := Repository{
			ID:            req.ID,
			Path:          "/tmp/test-repo",
			CurrentBranch: "main",
			CreatedAt:     time.Now(),
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(repo)
	}))
	defer server.Close()

	client, _ := NewClient(server.URL)
	repo, err := client.CreateRepo(context.Background(), "test-repo", &CreateRepoOptions{
		MemoryOnly: true,
	})

	if err != nil {
		t.Fatalf("CreateRepo() error = %v", err)
	}

	if repo.ID != "test-repo" {
		t.Errorf("Expected repo ID test-repo, got %s", repo.ID)
	}
}

func TestRepositoryOperations(t *testing.T) {
	// Create a mock server with multiple endpoints
	mux := http.NewServeMux()

	// Add file endpoint
	mux.HandleFunc("/api/v1/repos/test-repo/add", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		response := struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		}{
			Status:  "added",
			Message: "file added to staging",
		}
		json.NewEncoder(w).Encode(response)
	})

	// Commit endpoint
	mux.HandleFunc("/api/v1/repos/test-repo/commit", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Message string `json:"message"`
			Author  string `json:"author"`
			Email   string `json:"email"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		commit := Commit{
			Hash:      "abc123",
			Message:   req.Message,
			Author:    req.Author,
			Email:     req.Email,
			Timestamp: time.Now(),
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(commit)
	})

	// Status endpoint
	mux.HandleFunc("/api/v1/repos/test-repo/status", func(w http.ResponseWriter, r *http.Request) {
		status := Status{
			Branch:    "main",
			Staged:    []string{"test.txt"},
			Modified:  []string{},
			Untracked: []string{},
			Clean:     false,
		}
		json.NewEncoder(w).Encode(status)
	})

	// Log endpoint
	mux.HandleFunc("/api/v1/repos/test-repo/log", func(w http.ResponseWriter, r *http.Request) {
		commits := []*Commit{
			{
				Hash:      "abc123",
				Message:   "Initial commit",
				Author:    "Test User",
				Email:     "test@example.com",
				Timestamp: time.Now(),
			},
		}
		response := struct {
			Commits []*Commit `json:"commits"`
			Count   int       `json:"count"`
		}{
			Commits: commits,
			Count:   len(commits),
		}
		json.NewEncoder(w).Encode(response)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client, _ := NewClient(server.URL)
	repo := &Repository{
		client: client,
		ID:     "test-repo",
	}

	t.Run("AddFile", func(t *testing.T) {
		err := repo.AddFile(context.Background(), "test.txt", "Hello, World!")
		if err != nil {
			t.Errorf("AddFile() error = %v", err)
		}
	})

	t.Run("Commit", func(t *testing.T) {
		commit, err := repo.Commit(context.Background(), "Initial commit", &Author{
			Name:  "Test User",
			Email: "test@example.com",
		})
		if err != nil {
			t.Fatalf("Commit() error = %v", err)
		}
		if commit.Hash != "abc123" {
			t.Errorf("Expected commit hash abc123, got %s", commit.Hash)
		}
	})

	t.Run("Status", func(t *testing.T) {
		status, err := repo.Status(context.Background())
		if err != nil {
			t.Fatalf("Status() error = %v", err)
		}
		if status.Branch != "main" {
			t.Errorf("Expected branch main, got %s", status.Branch)
		}
		if len(status.Staged) != 1 {
			t.Errorf("Expected 1 staged file, got %d", len(status.Staged))
		}
	})

	t.Run("Log", func(t *testing.T) {
		commits, err := repo.Log(context.Background(), 10)
		if err != nil {
			t.Fatalf("Log() error = %v", err)
		}
		if len(commits) != 1 {
			t.Errorf("Expected 1 commit, got %d", len(commits))
		}
	})
}

func TestBranchOperations(t *testing.T) {
	mux := http.NewServeMux()

	// List branches
	mux.HandleFunc("/api/v1/repos/test-repo/branches", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			branches := []*Branch{
				{Name: "main", Commit: "abc123", IsCurrent: true},
				{Name: "feature", Commit: "def456", IsCurrent: false},
			}
			response := struct {
				Branches []*Branch `json:"branches"`
				Current  string    `json:"current"`
				Count    int       `json:"count"`
			}{
				Branches: branches,
				Current:  "main",
				Count:    len(branches),
			}
			json.NewEncoder(w).Encode(response)
		} else if r.Method == http.MethodPost {
			// Create branch
			var req struct {
				Name string `json:"name"`
				From string `json:"from"`
			}
			json.NewDecoder(r.Body).Decode(&req)

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			}{
				Status:  "created",
				Message: "branch created",
			})
		}
	})

	// Checkout
	mux.HandleFunc("/api/v1/repos/test-repo/checkout", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Branch string `json:"branch"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		json.NewEncoder(w).Encode(struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		}{
			Status:  "success",
			Message: "switched to branch " + req.Branch,
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client, _ := NewClient(server.URL)
	repo := &Repository{
		client: client,
		ID:     "test-repo",
	}

	t.Run("ListBranches", func(t *testing.T) {
		branches, err := repo.ListBranches(context.Background())
		if err != nil {
			t.Fatalf("ListBranches() error = %v", err)
		}
		if len(branches) != 2 {
			t.Errorf("Expected 2 branches, got %d", len(branches))
		}
	})

	t.Run("CreateBranch", func(t *testing.T) {
		err := repo.CreateBranch(context.Background(), "new-feature", "main")
		if err != nil {
			t.Errorf("CreateBranch() error = %v", err)
		}
	})

	t.Run("Checkout", func(t *testing.T) {
		err := repo.Checkout(context.Background(), "feature")
		if err != nil {
			t.Errorf("Checkout() error = %v", err)
		}
		if repo.CurrentBranch != "feature" {
			t.Errorf("Expected current branch to be feature, got %s", repo.CurrentBranch)
		}
	})
}
