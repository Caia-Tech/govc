package core

import (
	"fmt"
	"time"

	"github.com/Caia-Tech/govc/pkg/object"
)

// Operations provides high-level Git operations using clean architecture
type Operations struct {
	repo      *CleanRepository
	workspace *CleanWorkspace
	config    *Config
	diag      *DiagnosticLogger
}

// NewOperations creates a new operations handler
func NewOperations(repo *CleanRepository, workspace *CleanWorkspace, config *Config) *Operations {
	return &Operations{
		repo:      repo,
		workspace: workspace,
		config:    config,
		diag:      NewDiagnosticLogger("Operations"),
	}
}

// Init initializes a new repository
func (ops *Operations) Init() error {
	// Create initial branch reference
	return ops.repo.refs.UpdateRef("refs/heads/main", "")
}

// Add stages files
func (ops *Operations) Add(paths ...string) error {
	for _, path := range paths {
		if err := ops.workspace.Add(path); err != nil {
			return err
		}
	}
	return nil
}

// Commit creates a new commit
func (ops *Operations) Commit(message string) (string, error) {
	ops.diag.Log("Commit: Starting with message: %s", message)
	
	// Check if there are changes to commit
	staging := ops.workspace.GetStagingArea()
	ops.diag.LogData("Staging area", map[string]int{
		"entries": len(staging.entries),
		"removed": len(staging.removed),
	})
	
	if len(staging.entries) == 0 && len(staging.removed) == 0 {
		return "", fmt.Errorf("nothing to commit")
	}

	// Get author info from config
	authorName, _ := ops.config.store.Get("user.name")
	if authorName == "" {
		authorName = "Unknown"
	}

	authorEmail, _ := ops.config.store.Get("user.email")
	if authorEmail == "" {
		authorEmail = "unknown@example.com"
	}

	author := object.Author{
		Name:  authorName,
		Email: authorEmail,
		Time:  time.Now(),
	}

	// Build tree from staging area
	tree := ops.buildTreeFromStaging()
	treeHash, err := ops.repo.objects.Put(tree)
	if err != nil {
		ops.diag.LogError("Put tree", err)
		return "", err
	}
	ops.diag.Log("Created tree: %s", treeHash)

	// Get parent commit
	parentHash := ""
	if currentBranch := ops.workspace.branch; currentBranch != "" {
		ops.diag.Log("Current branch: %s", currentBranch)
		parentHash, _ = ops.repo.GetBranch(currentBranch)
		ops.diag.Log("Parent hash from branch %s: %s", currentBranch, parentHash)
	}

	// Create commit
	commit := object.NewCommit(treeHash, author, message)
	if parentHash != "" {
		commit.SetParent(parentHash)
		ops.diag.Log("Set parent: %s", parentHash)
	}
	commitHash, err := ops.repo.objects.Put(commit)
	if err != nil {
		ops.diag.LogError("Put commit", err)
		return "", err
	}
	ops.diag.Log("Created commit: %s", commitHash)

	// Update branch reference
	branchRef := fmt.Sprintf("refs/heads/%s", ops.workspace.branch)
	ops.diag.Log("Updating ref %s to %s", branchRef, commitHash)
	if err := ops.repo.refs.UpdateRef(branchRef, commitHash); err != nil {
		ops.diag.LogError("UpdateRef", err)
		return "", err
	}
	ops.diag.Log("Successfully updated branch ref")

	// Clear staging area
	ops.workspace.Reset()
	ops.diag.Log("Commit complete: %s", commitHash)

	return commitHash, nil
}

// Branch creates a new branch
func (ops *Operations) Branch(name string) error {
	ops.diag.Log("Branch: Creating branch %s from %s", name, ops.workspace.branch)
	
	// Get current HEAD
	currentHash, err := ops.repo.GetBranch(ops.workspace.branch)
	if err != nil {
		ops.diag.Log("No commits on branch %s yet", ops.workspace.branch)
		// No commits yet
		currentHash = ""
	} else {
		ops.diag.Log("Current HEAD: %s", currentHash)
	}

	// Create branch reference
	branchRef := fmt.Sprintf("refs/heads/%s", name)
	ops.diag.Log("Creating ref %s pointing to %s", branchRef, currentHash)
	err = ops.repo.refs.UpdateRef(branchRef, currentHash)
	if err != nil {
		ops.diag.LogError("UpdateRef for branch", err)
	} else {
		ops.diag.Log("Branch %s created successfully", name)
	}
	return err
}

// DeleteBranch deletes a branch
func (ops *Operations) DeleteBranch(name string) error {
	// Don't allow deleting current branch
	if name == ops.workspace.branch {
		return fmt.Errorf("cannot delete current branch")
	}

	// Check if branch exists
	if _, err := ops.repo.GetBranch(name); err != nil {
		return fmt.Errorf("branch not found: %s", name)
	}

	// Delete branch reference
	branchRef := fmt.Sprintf("refs/heads/%s", name)
	return ops.repo.refs.DeleteRef(branchRef)
}

// Checkout switches to a branch
func (ops *Operations) Checkout(branch string) error {
	ops.diag.Log("Checkout: Switching to branch %s", branch)
	err := ops.workspace.Checkout(branch)
	if err != nil {
		ops.diag.LogError("Checkout", err)
	} else {
		ops.diag.Log("Successfully checked out %s", branch)
	}
	return err
}

// Status returns repository status
func (ops *Operations) Status() (*Status, error) {
	return ops.workspace.Status()
}

// Log returns commit history
func (ops *Operations) Log(limit int) ([]*object.Commit, error) {
	ops.diag.Log("Log: Getting history for branch %s (limit %d)", ops.workspace.branch, limit)
	
	// Get current branch
	currentHash, err := ops.repo.GetBranch(ops.workspace.branch)
	if err != nil {
		ops.diag.LogError("GetBranch", err)
		return nil, fmt.Errorf("no commits on branch %s", ops.workspace.branch)
	}
	ops.diag.Log("Current branch HEAD: %s", currentHash)

	commits, err := ops.repo.Log(currentHash, limit)
	if err != nil {
		ops.diag.LogError("Log", err)
	} else {
		ops.diag.Log("Retrieved %d commits", len(commits))
	}
	return commits, err
}

// Merge merges another branch into current branch
func (ops *Operations) Merge(branch string, message string) (string, error) {
	// Get both branch commits
	currentHash, err := ops.repo.GetBranch(ops.workspace.branch)
	if err != nil {
		return "", fmt.Errorf("current branch has no commits")
	}

	otherHash, err := ops.repo.GetBranch(branch)
	if err != nil {
		return "", fmt.Errorf("branch %s not found", branch)
	}

	// For now, implement fast-forward only
	// Full merge would require three-way merge algorithm

	// Check if fast-forward is possible
	commits, err := ops.repo.Log(otherHash, 0)
	if err != nil {
		return "", err
	}

	// Check if current is ancestor of other
	isAncestor := false
	for _, commit := range commits {
		if commit.Hash() == currentHash {
			isAncestor = true
			break
		}
	}

	if isAncestor {
		// Fast-forward merge
		branchRef := fmt.Sprintf("refs/heads/%s", ops.workspace.branch)
		if err := ops.repo.refs.UpdateRef(branchRef, otherHash); err != nil {
			return "", err
		}

		// Update working directory
		if err := ops.workspace.Checkout(ops.workspace.branch); err != nil {
			return "", err
		}

		return otherHash, nil
	}

	// Would need three-way merge here
	return "", fmt.Errorf("three-way merge not yet implemented")
}

// Tag creates a new tag
func (ops *Operations) Tag(name string) error {
	// Get current HEAD
	currentHash, err := ops.repo.GetBranch(ops.workspace.branch)
	if err != nil {
		return fmt.Errorf("no commits to tag")
	}

	// Create tag reference
	tagRef := fmt.Sprintf("refs/tags/%s", name)
	return ops.repo.refs.UpdateRef(tagRef, currentHash)
}

// buildTreeFromStaging creates a tree from staging area
func (ops *Operations) buildTreeFromStaging() *object.Tree {
	staging := ops.workspace.GetStagingArea()
	entries := make([]object.TreeEntry, 0, len(staging.entries))

	for path, staged := range staging.entries {
		entries = append(entries, object.TreeEntry{
			Mode: staged.Mode,
			Name: path,
			Hash: staged.Hash,
		})
	}

	// Sort entries for consistent trees
	tree := object.NewTree()
	tree.Entries = entries
	return tree
}

// Clone clones a repository (simplified version)
func Clone(sourceRepo *CleanRepository, targetObjects ObjectStore, targetRefs RefStore) (*CleanRepository, error) {
	// Copy all objects
	hashes, err := sourceRepo.objects.List()
	if err != nil {
		return nil, err
	}

	for _, hash := range hashes {
		obj, err := sourceRepo.objects.Get(hash)
		if err != nil {
			return nil, err
		}

		if _, err := targetObjects.Put(obj); err != nil {
			return nil, err
		}
	}

	// Copy all refs
	refs, err := sourceRepo.refs.ListRefs()
	if err != nil {
		return nil, err
	}

	for name, hash := range refs {
		if err := targetRefs.UpdateRef(name, hash); err != nil {
			return nil, err
		}
	}

	// Copy HEAD
	head, err := sourceRepo.refs.GetHEAD()
	if err == nil {
		targetRefs.SetHEAD(head)
	}

	return NewCleanRepository(targetObjects, targetRefs), nil
}
