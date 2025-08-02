package refs

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestMemoryRefStore(t *testing.T) {
	store := NewMemoryRefStore()

	t.Run("basic operations", func(t *testing.T) {
		// Test set and get
		err := store.SetRef("refs/heads/main", "abc123")
		if err != nil {
			t.Fatalf("SetRef() error = %v", err)
		}

		hash, err := store.GetRef("refs/heads/main")
		if err != nil {
			t.Fatalf("GetRef() error = %v", err)
		}
		if hash != "abc123" {
			t.Errorf("GetRef() = %v, want abc123", hash)
		}

		// Test normalization - branch name without refs/heads/
		err = store.SetRef("feature", "def456")
		if err != nil {
			t.Fatalf("SetRef(feature) error = %v", err)
		}

		hash, err = store.GetRef("feature")
		if err != nil {
			t.Fatalf("GetRef(feature) error = %v", err)
		}
		if hash != "def456" {
			t.Errorf("GetRef(feature) = %v, want def456", hash)
		}

		// Verify it's stored with full path
		hash, err = store.GetRef("refs/heads/feature")
		if err != nil {
			t.Fatalf("GetRef(refs/heads/feature) error = %v", err)
		}
		if hash != "def456" {
			t.Errorf("GetRef(refs/heads/feature) = %v, want def456", hash)
		}
	})

	t.Run("HEAD operations", func(t *testing.T) {
		// Set HEAD to branch
		err := store.SetHEAD("refs/heads/main")
		if err != nil {
			t.Fatalf("SetHEAD() error = %v", err)
		}

		// Get HEAD should resolve to branch commit
		head, err := store.GetHEAD()
		if err != nil {
			t.Fatalf("GetHEAD() error = %v", err)
		}
		if head != "abc123" {
			t.Errorf("GetHEAD() = %v, want abc123", head)
		}

		// Set HEAD to direct commit (detached)
		err = store.SetHEAD("xyz789")
		if err != nil {
			t.Fatalf("SetHEAD(detached) error = %v", err)
		}

		head, err = store.GetHEAD()
		if err != nil {
			t.Fatalf("GetHEAD() error = %v", err)
		}
		if head != "xyz789" {
			t.Errorf("GetHEAD() = %v, want xyz789", head)
		}
	})

	t.Run("list refs", func(t *testing.T) {
		// Add various refs
		store.SetRef("refs/heads/develop", "aaa111")
		store.SetRef("refs/tags/v1.0", "bbb222")
		store.SetRef("refs/remotes/origin/main", "ccc333")

		// List all
		all, err := store.ListRefs("")
		if err != nil {
			t.Fatalf("ListRefs() error = %v", err)
		}
		if len(all) < 4 { // main, feature, develop, tag, remote
			t.Errorf("ListRefs() returned %d refs, want at least 4", len(all))
		}

		// List only heads
		heads, err := store.ListRefs("refs/heads/")
		if err != nil {
			t.Fatalf("ListRefs(refs/heads/) error = %v", err)
		}

		headCount := 0
		for _, ref := range heads {
			if ref.Type != RefTypeBranch {
				t.Errorf("Expected branch type, got %v for %s", ref.Type, ref.Name)
			}
			headCount++
		}
		if headCount < 3 {
			t.Errorf("Expected at least 3 branches, got %d", headCount)
		}

		// List only tags
		tags, err := store.ListRefs("refs/tags/")
		if err != nil {
			t.Fatalf("ListRefs(refs/tags/) error = %v", err)
		}

		tagCount := 0
		for _, ref := range tags {
			if ref.Type != RefTypeTag {
				t.Errorf("Expected tag type, got %v for %s", ref.Type, ref.Name)
			}
			tagCount++
		}
		if tagCount < 1 {
			t.Errorf("Expected at least 1 tag, got %d", tagCount)
		}
	})

	t.Run("delete ref", func(t *testing.T) {
		store.SetRef("refs/heads/temp", "temp123")

		// Verify it exists
		if _, err := store.GetRef("refs/heads/temp"); err != nil {
			t.Fatal("Ref should exist before deletion")
		}

		// Delete
		err := store.DeleteRef("refs/heads/temp")
		if err != nil {
			t.Fatalf("DeleteRef() error = %v", err)
		}

		// Verify gone
		_, err = store.GetRef("refs/heads/temp")
		if err == nil {
			t.Error("GetRef() should fail after deletion")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, 100)

		// Concurrent writes to different refs
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				ref := fmt.Sprintf("refs/heads/concurrent%d", id)
				hash := fmt.Sprintf("hash%d", id)
				if err := store.SetRef(ref, hash); err != nil {
					errors <- err
				}
			}(i)
		}

		// Concurrent reads
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				ref := fmt.Sprintf("refs/heads/concurrent%d", id)
				if _, err := store.GetRef(ref); err != nil {
					// OK if not found (race with write)
					if err.Error() != fmt.Sprintf("ref not found: %s", ref) {
						errors <- err
					}
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			t.Errorf("Concurrent operation error: %v", err)
		}
	})
}

func TestFileRefStore(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileRefStore(tmpDir)

	t.Run("basic operations", func(t *testing.T) {
		// Set ref
		err := store.SetRef("refs/heads/main", "abc123")
		if err != nil {
			t.Fatalf("SetRef() error = %v", err)
		}

		// Verify file created
		refPath := filepath.Join(tmpDir, "refs/heads/main")
		if _, err := os.Stat(refPath); err != nil {
			t.Errorf("Ref file not created: %v", err)
		}

		// Read ref
		hash, err := store.GetRef("refs/heads/main")
		if err != nil {
			t.Fatalf("GetRef() error = %v", err)
		}
		if hash != "abc123" {
			t.Errorf("GetRef() = %v, want abc123", hash)
		}
	})

	t.Run("HEAD file", func(t *testing.T) {
		// Set HEAD to branch
		err := store.SetHEAD("refs/heads/main")
		if err != nil {
			t.Fatalf("SetHEAD() error = %v", err)
		}

		// Read HEAD file directly
		headPath := filepath.Join(tmpDir, "HEAD")
		content, err := os.ReadFile(headPath)
		if err != nil {
			t.Fatalf("Failed to read HEAD file: %v", err)
		}

		expected := "ref: refs/heads/main\n"
		if string(content) != expected {
			t.Errorf("HEAD content = %q, want %q", string(content), expected)
		}

		// GetHEAD should resolve
		head, err := store.GetHEAD()
		if err != nil {
			t.Fatalf("GetHEAD() error = %v", err)
		}
		if head != "abc123" {
			t.Errorf("GetHEAD() = %v, want abc123", head)
		}
	})

	t.Run("packed refs", func(t *testing.T) {
		// Create packed-refs file
		packedContent := `# pack-refs with: peeled fully-peeled
abc123 refs/heads/packed-branch
def456 refs/tags/v1.0
`
		packedPath := filepath.Join(tmpDir, "packed-refs")
		err := os.WriteFile(packedPath, []byte(packedContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create packed-refs: %v", err)
		}

		// Should read from packed refs
		hash, err := store.GetRef("refs/heads/packed-branch")
		if err != nil {
			t.Fatalf("GetRef(packed) error = %v", err)
		}
		if hash != "abc123" {
			t.Errorf("GetRef(packed) = %v, want abc123", hash)
		}
	})
}

func TestRefManager(t *testing.T) {
	store := NewMemoryRefStore()
	manager := NewRefManager(store)

	t.Run("branch operations", func(t *testing.T) {
		// Create branch
		err := manager.CreateBranch("feature", "commit123")
		if err != nil {
			t.Fatalf("CreateBranch() error = %v", err)
		}

		// Get branch
		hash, err := manager.GetBranch("feature")
		if err != nil {
			t.Fatalf("GetBranch() error = %v", err)
		}
		if hash != "commit123" {
			t.Errorf("GetBranch() = %v, want commit123", hash)
		}

		// List branches
		branches, err := manager.ListBranches()
		if err != nil {
			t.Fatalf("ListBranches() error = %v", err)
		}

		found := false
		for _, branch := range branches {
			if branch.Name == "refs/heads/feature" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Created branch not found in list")
		}

		// Delete branch
		err = manager.DeleteBranch("feature")
		if err != nil {
			t.Fatalf("DeleteBranch() error = %v", err)
		}

		_, err = manager.GetBranch("feature")
		if err == nil {
			t.Error("GetBranch() should fail after deletion")
		}
	})

	t.Run("tag operations", func(t *testing.T) {
		// Create tag
		err := manager.CreateTag("v1.0", "tagobj123")
		if err != nil {
			t.Fatalf("CreateTag() error = %v", err)
		}

		// Get tag
		hash, err := manager.GetTag("v1.0")
		if err != nil {
			t.Fatalf("GetTag() error = %v", err)
		}
		if hash != "tagobj123" {
			t.Errorf("GetTag() = %v, want tagobj123", hash)
		}

		// List tags
		tags, err := manager.ListTags()
		if err != nil {
			t.Fatalf("ListTags() error = %v", err)
		}

		if len(tags) < 1 {
			t.Error("Expected at least one tag")
		}
	})

	t.Run("HEAD operations", func(t *testing.T) {
		// Set HEAD to branch
		manager.CreateBranch("main", "maincommit")
		err := manager.SetHEADToBranch("main")
		if err != nil {
			t.Fatalf("SetHEADToBranch() error = %v", err)
		}

		head, err := manager.GetHEAD()
		if err != nil {
			t.Fatalf("GetHEAD() error = %v", err)
		}
		if head != "maincommit" {
			t.Errorf("GetHEAD() = %v, want maincommit", head)
		}

		// Set HEAD to commit (detached)
		err = manager.SetHEADToCommit("detached123")
		if err != nil {
			t.Fatalf("SetHEADToCommit() error = %v", err)
		}

		head, err = manager.GetHEAD()
		if err != nil {
			t.Fatalf("GetHEAD() error = %v", err)
		}
		if head != "detached123" {
			t.Errorf("GetHEAD() = %v, want detached123", head)
		}
	})

	t.Run("update ref with old value check", func(t *testing.T) {
		manager.CreateBranch("atomic", "old123")

		// Update with correct old value
		err := manager.UpdateRef("refs/heads/atomic", "new456", "old123")
		if err != nil {
			t.Fatalf("UpdateRef() error = %v", err)
		}

		// Update with wrong old value should fail
		err = manager.UpdateRef("refs/heads/atomic", "newer789", "wrong")
		if err == nil {
			t.Error("UpdateRef() should fail with wrong old value")
		}

		// Verify ref didn't change
		hash, _ := manager.GetBranch("atomic")
		if hash != "new456" {
			t.Errorf("Branch should still be new456, got %v", hash)
		}
	})
}

func BenchmarkMemoryRefStore(b *testing.B) {
	store := NewMemoryRefStore()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		ref := fmt.Sprintf("refs/heads/branch%d", i)
		hash := fmt.Sprintf("hash%d", i)
		store.SetRef(ref, hash)
	}

	b.ResetTimer()
	b.Run("SetRef", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ref := fmt.Sprintf("refs/heads/bench%d", i)
			store.SetRef(ref, "benchhash")
		}
	})

	b.Run("GetRef", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ref := fmt.Sprintf("refs/heads/branch%d", i%1000)
			store.GetRef(ref)
		}
	})

	b.Run("ListRefs", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			store.ListRefs("refs/heads/")
		}
	})
}

// Test that demonstrates instant branch operations
func TestInstantBranchOperations(t *testing.T) {
	store := NewMemoryRefStore()
	manager := NewRefManager(store)

	// Create 1000 branches and measure time
	start := time.Now()
	for i := 0; i < 1000; i++ {
		branch := fmt.Sprintf("branch%d", i)
		hash := fmt.Sprintf("commit%d", i)
		err := manager.CreateBranch(branch, hash)
		if err != nil {
			t.Fatalf("Failed to create branch %d: %v", i, err)
		}
	}
	duration := time.Since(start)

	t.Logf("Created 1000 branches in %v", duration)

	// Should be very fast (< 10ms)
	if duration > 10*time.Millisecond {
		t.Logf("Warning: Creating 1000 branches took %v (expected < 10ms)", duration)
	}

	// List all branches should also be instant
	start = time.Now()
	branches, err := manager.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches() error = %v", err)
	}
	listDuration := time.Since(start)

	if len(branches) < 1000 {
		t.Errorf("Expected at least 1000 branches, got %d", len(branches))
	}

	t.Logf("Listed %d branches in %v", len(branches), listDuration)
}
