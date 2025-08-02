package core

import (
	"testing"
)

func TestStashWithUntrackedFiles(t *testing.T) {
	t.Run("StashWithoutUntracked", func(t *testing.T) {
		// Create memory stores
		objects := NewMemoryObjectStore()
		refs := NewMemoryRefStore()
		working := NewMemoryWorkingStorage()
		config := NewMemoryConfigStore()

		// Create components
		repo := NewCleanRepository(objects, refs)
		workspace := NewCleanWorkspace(repo, working)
		cfg := NewConfig(config)
		ops := NewOperations(repo, workspace, cfg)
		stash := NewStashManager(repo, workspace)

		// Initialize repository
		if err := ops.Init(); err != nil {
			t.Fatalf("Failed to init: %v", err)
		}

		// Configure user
		cfg.Set("user.name", "Test User")
		cfg.Set("user.email", "test@example.com")

		// Create initial commit
		workspace.WriteFile("README.md", []byte("# Test Project"))
		ops.Add("README.md")
		ops.Commit("Initial commit")

		// Create some changes
		workspace.WriteFile("README.md", []byte("# Test Project\nModified"))
		workspace.WriteFile("untracked.txt", []byte("This is untracked"))

		// Create stash without untracked files
		stash1, err := stash.Create("WIP: without untracked", false)
		if err != nil {
			t.Fatalf("Failed to create stash: %v", err)
		}

		// Check that untracked file is still there
		if _, err := workspace.ReadFile("untracked.txt"); err != nil {
			t.Error("Untracked file should still exist after stash without includeUntracked")
		}

		// Check workspace is otherwise clean
		status, _ := ops.Status()
		if len(status.Modified) != 0 {
			t.Errorf("Expected no modified files after stash, got %d", len(status.Modified))
		}
		if len(status.Untracked) != 1 {
			t.Errorf("Expected 1 untracked file after stash without includeUntracked, got %d", len(status.Untracked))
		}

		// Verify stash doesn't contain untracked files
		if len(stash1.Untracked) != 0 {
			t.Errorf("Stash should not contain untracked files when includeUntracked=false")
		}
	})

	t.Run("StashWithUntracked", func(t *testing.T) {
		// Create memory stores
		objects := NewMemoryObjectStore()
		refs := NewMemoryRefStore()
		working := NewMemoryWorkingStorage()
		config := NewMemoryConfigStore()

		// Create components
		repo := NewCleanRepository(objects, refs)
		workspace := NewCleanWorkspace(repo, working)
		cfg := NewConfig(config)
		ops := NewOperations(repo, workspace, cfg)
		stash := NewStashManager(repo, workspace)

		// Initialize repository
		if err := ops.Init(); err != nil {
			t.Fatalf("Failed to init: %v", err)
		}

		// Configure user
		cfg.Set("user.name", "Test User")
		cfg.Set("user.email", "test@example.com")

		// Create initial commit
		workspace.WriteFile("README.md", []byte("# Test Project"))
		ops.Add("README.md")
		ops.Commit("Initial commit")

		// Create some changes
		workspace.WriteFile("README.md", []byte("# Test Project\nModified"))
		workspace.WriteFile("staged.txt", []byte("This is staged"))
		ops.Add("staged.txt")
		workspace.WriteFile("untracked.txt", []byte("This is untracked"))

		// Create stash with untracked files
		stash2, err := stash.Create("WIP: with untracked", true)
		if err != nil {
			t.Fatalf("Failed to create stash with untracked: %v", err)
		}

		// Check that untracked file is gone
		if _, err := workspace.ReadFile("untracked.txt"); err == nil {
			t.Error("Untracked file should be removed after stash with includeUntracked")
		}

		// Check workspace is completely clean
		status, _ := ops.Status()
		if !status.Clean() {
			t.Error("Workspace should be completely clean after stash with includeUntracked")
		}

		// Verify stash contains untracked files
		if len(stash2.Untracked) != 1 {
			t.Errorf("Stash should contain 1 untracked file, got %d", len(stash2.Untracked))
		}

		// Apply stash with untracked files
		if err := stash.Apply(stash2.ID); err != nil {
			t.Fatalf("Failed to apply stash with untracked: %v", err)
		}

		// Check everything is restored
		status, _ = ops.Status()
		if len(status.Modified) != 1 {
			t.Errorf("Expected 1 modified file after apply, got %d", len(status.Modified))
		}
		if len(status.Staged) != 1 {
			t.Errorf("Expected 1 staged file after apply, got %d", len(status.Staged))
		}
		if len(status.Untracked) != 1 {
			t.Errorf("Expected 1 untracked file after apply, got %d", len(status.Untracked))
		}

		// Verify untracked file content
		content, err := workspace.ReadFile("untracked.txt")
		if err != nil {
			t.Fatalf("Failed to read untracked file after apply: %v", err)
		}
		if string(content) != "This is untracked" {
			t.Errorf("Untracked file content mismatch: got %q", string(content))
		}
	})
}