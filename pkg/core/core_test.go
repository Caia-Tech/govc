package core_test

import (
	"testing"

	"github.com/caiatech/govc/pkg/core"
	"github.com/caiatech/govc/pkg/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanArchitecture(t *testing.T) {
	// Create in-memory stores
	objects := core.NewMemoryObjectStore()
	refs := core.NewMemoryRefStore()
	working := core.NewMemoryWorkingStorage()
	config := core.NewMemoryConfigStore()

	// Create components
	repo := core.NewCleanRepository(objects, refs)
	workspace := core.NewCleanWorkspace(repo, working)
	cfg := core.NewConfig(config)
	ops := core.NewOperations(repo, workspace, cfg)

	// Initialize repository
	err := ops.Init()
	require.NoError(t, err)

	// Test file operations
	t.Run("FileOperations", func(t *testing.T) {
		// Write a file
		content := []byte("Hello, world!")
		err := workspace.WriteFile("test.txt", content)
		require.NoError(t, err)

		// Read it back
		readContent, err := workspace.ReadFile("test.txt")
		require.NoError(t, err)
		assert.Equal(t, content, readContent)

		// Add to staging
		err = workspace.Add("test.txt")
		require.NoError(t, err)

		// Check status
		status, err := workspace.Status()
		require.NoError(t, err)
		assert.Equal(t, "main", status.Branch)
		assert.Len(t, status.Staged, 1)
		assert.Contains(t, status.Staged, "test.txt")
	})

	// Test commit operations
	t.Run("CommitOperations", func(t *testing.T) {
		// Set user config
		cfg.Set("user.name", "Test User")
		cfg.Set("user.email", "test@example.com")

		// Create commit
		hash, err := ops.Commit("Initial commit")
		require.NoError(t, err)
		assert.NotEmpty(t, hash)

		// Check log
		commits, err := ops.Log(10)
		require.NoError(t, err)
		assert.Len(t, commits, 1)
		assert.Equal(t, "Initial commit", commits[0].Message)
	})

	// Test branch operations
	t.Run("BranchOperations", func(t *testing.T) {
		// Create a new branch
		err := ops.Branch("feature")
		require.NoError(t, err)

		// List branches
		branches, err := repo.ListBranches()
		require.NoError(t, err)
		assert.Len(t, branches, 2)

		branchNames := make([]string, len(branches))
		for i, b := range branches {
			branchNames[i] = b.Name
		}
		assert.Contains(t, branchNames, "main")
		assert.Contains(t, branchNames, "feature")

		// Checkout new branch
		err = ops.Checkout("feature")
		require.NoError(t, err)

		// Check current branch
		status, err := ops.Status()
		require.NoError(t, err)
		assert.Equal(t, "feature", status.Branch)
	})

	// Test tag operations
	t.Run("TagOperations", func(t *testing.T) {
		// Create a tag
		err := ops.Tag("v1.0.0")
		require.NoError(t, err)

		// List tags
		tags, err := repo.ListTags()
		require.NoError(t, err)
		assert.Len(t, tags, 1)
		assert.Equal(t, "v1.0.0", tags[0].Name)
	})
}

func TestStashManager(t *testing.T) {
	// Setup
	objects := core.NewMemoryObjectStore()
	refs := core.NewMemoryRefStore()
	working := core.NewMemoryWorkingStorage()
	config := core.NewMemoryConfigStore()

	repo := core.NewCleanRepository(objects, refs)
	workspace := core.NewCleanWorkspace(repo, working)
	cfg := core.NewConfig(config)
	ops := core.NewOperations(repo, workspace, cfg)

	// Initialize and create initial commit
	err := ops.Init()
	require.NoError(t, err)

	cfg.Set("user.name", "Test User")
	cfg.Set("user.email", "test@example.com")

	// Create initial file and commit
	err = workspace.WriteFile("file1.txt", []byte("initial content"))
	require.NoError(t, err)
	err = workspace.Add("file1.txt")
	require.NoError(t, err)
	_, err = ops.Commit("Initial commit")
	require.NoError(t, err)

	// Create stash manager
	stashMgr := core.NewStashManager(repo, workspace)

	t.Run("CreateStash", func(t *testing.T) {
		// Make changes
		err := workspace.WriteFile("file1.txt", []byte("modified content"))
		require.NoError(t, err)
		err = workspace.WriteFile("file2.txt", []byte("new file"))
		require.NoError(t, err)
		err = workspace.Add("file2.txt")
		require.NoError(t, err)

		// Create stash
		stash, err := stashMgr.Create("WIP changes", false)
		require.NoError(t, err)
		assert.NotEmpty(t, stash.ID)
		assert.Equal(t, "WIP changes", stash.Message)

		// Check workspace is clean
		status, err := workspace.Status()
		require.NoError(t, err)
		assert.True(t, status.Clean())
	})

	t.Run("ApplyStash", func(t *testing.T) {
		// List stashes
		stashes := stashMgr.List()
		assert.Len(t, stashes, 1)

		// Apply stash
		err := stashMgr.Apply(stashes[0].ID)
		require.NoError(t, err)

		// Check changes are restored
		content, err := workspace.ReadFile("file1.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("modified content"), content)

		status, err := workspace.Status()
		require.NoError(t, err)
		assert.Len(t, status.Staged, 1)
		assert.Len(t, status.Modified, 1)
	})
}

func TestWebhookManager(t *testing.T) {
	mgr := core.NewWebhookManager()

	t.Run("RegisterWebhook", func(t *testing.T) {
		webhook, err := mgr.Register("https://example.com/hook", []string{"push", "commit"}, "secret123")
		require.NoError(t, err)
		assert.NotEmpty(t, webhook.ID)
		assert.Equal(t, "https://example.com/hook", webhook.URL)
		assert.Equal(t, []string{"push", "commit"}, webhook.Events)
	})

	t.Run("ListWebhooks", func(t *testing.T) {
		hooks := mgr.List()
		assert.Len(t, hooks, 1)
	})

	t.Run("GetWebhook", func(t *testing.T) {
		hooks := mgr.List()
		hook, err := mgr.Get(hooks[0].ID)
		require.NoError(t, err)
		assert.Equal(t, hooks[0].ID, hook.ID)
	})

	t.Run("UnregisterWebhook", func(t *testing.T) {
		hooks := mgr.List()
		err := mgr.Unregister(hooks[0].ID)
		require.NoError(t, err)

		hooks = mgr.List()
		assert.Len(t, hooks, 0)
	})
}

func TestMemoryObjectStore(t *testing.T) {
	store := core.NewMemoryObjectStore()

	t.Run("StoreAndRetrieveBlob", func(t *testing.T) {
		blob := object.NewBlob([]byte("test content"))
		hash, err := store.Put(blob)
		require.NoError(t, err)
		assert.NotEmpty(t, hash)

		// Retrieve
		obj, err := store.Get(hash)
		require.NoError(t, err)
		
		retrievedBlob, ok := obj.(*object.Blob)
		require.True(t, ok)
		assert.Equal(t, blob.Content, retrievedBlob.Content)
	})

	t.Run("StoreAndRetrieveTree", func(t *testing.T) {
		tree := object.NewTree()
		tree.Entries = []object.TreeEntry{
			{
				Mode: "100644",
				Name: "file.txt",
				Hash: "abc123",
			},
		}

		hash, err := store.Put(tree)
		require.NoError(t, err)

		obj, err := store.Get(hash)
		require.NoError(t, err)
		
		retrievedTree, ok := obj.(*object.Tree)
		require.True(t, ok)
		assert.Len(t, retrievedTree.Entries, 1)
		assert.Equal(t, "file.txt", retrievedTree.Entries[0].Name)
	})

	t.Run("ListObjects", func(t *testing.T) {
		hashes, err := store.List()
		require.NoError(t, err)
		assert.Len(t, hashes, 2) // blob and tree
	})

	t.Run("CheckSize", func(t *testing.T) {
		size, err := store.Size()
		require.NoError(t, err)
		assert.Greater(t, size, int64(0))
	})
}

func TestMemoryRefStore(t *testing.T) {
	store := core.NewMemoryRefStore()

	t.Run("UpdateAndGetRef", func(t *testing.T) {
		err := store.UpdateRef("refs/heads/main", "abc123")
		require.NoError(t, err)

		hash, err := store.GetRef("refs/heads/main")
		require.NoError(t, err)
		assert.Equal(t, "abc123", hash)
	})

	t.Run("ListRefs", func(t *testing.T) {
		err := store.UpdateRef("refs/heads/feature", "def456")
		require.NoError(t, err)

		refs, err := store.ListRefs()
		require.NoError(t, err)
		assert.Len(t, refs, 2)
		assert.Equal(t, "abc123", refs["refs/heads/main"])
		assert.Equal(t, "def456", refs["refs/heads/feature"])
	})

	t.Run("DeleteRef", func(t *testing.T) {
		err := store.DeleteRef("refs/heads/feature")
		require.NoError(t, err)

		_, err = store.GetRef("refs/heads/feature")
		assert.Error(t, err)
	})

	t.Run("HEAD", func(t *testing.T) {
		// Default HEAD
		head, err := store.GetHEAD()
		require.NoError(t, err)
		assert.Equal(t, "refs/heads/main", head)

		// Update HEAD
		err = store.SetHEAD("refs/heads/another")
		require.NoError(t, err)

		head, err = store.GetHEAD()
		require.NoError(t, err)
		assert.Equal(t, "refs/heads/another", head)
	})
}