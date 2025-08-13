package repository

import (
	"fmt"
)

// CreateBranch creates a new branch from the specified commit
func (r *Repository) CreateBranch(name string, fromCommit string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// If no commit specified, use current HEAD
	if fromCommit == "" {
		head, err := r.refManager.GetHEAD()
		if err != nil {
			return fmt.Errorf("failed to get HEAD: %w", err)
		}
		fromCommit = head
	}
	
	// Create the branch reference
	branchRef := "refs/heads/" + name
	err := r.refManager.UpdateRef(branchRef, fromCommit, "")
	if err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}
	
	return nil
}

// SwitchBranch switches to the specified branch
func (r *Repository) SwitchBranch(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Check if branch exists
	branchRef := "refs/heads/" + name
	_, err := r.refManager.GetRef(branchRef)
	if err != nil {
		return fmt.Errorf("branch %s does not exist: %w", name, err)
	}
	
	// Update HEAD to point to the branch
	err = r.refManager.SetHEADToBranch(name)
	if err != nil {
		return fmt.Errorf("failed to switch branch: %w", err)
	}
	
	return nil
}

// ListBranchesDetailed returns a list of all branches with detailed information
func (r *Repository) ListBranchesDetailed() ([]BranchInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	refs, err := r.refManager.ListBranches()
	if err != nil {
		return nil, err
	}
	
	var branchInfos []BranchInfo
	for _, ref := range refs {
		// refs already contains the branch information
		branchInfos = append(branchInfos, BranchInfo{
			Name: ref.Name,
			Hash: ref.Hash,
		})
	}
	
	return branchInfos, nil
}

// BranchInfo contains information about a branch
type BranchInfo struct {
	Name string
	Hash string
}

// DeleteBranch deletes the specified branch
func (r *Repository) DeleteBranch(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Don't allow deleting current branch
	currentBranch, err := r.refManager.GetCurrentBranch()
	if err == nil && currentBranch == name {
		return fmt.Errorf("cannot delete current branch")
	}
	
	// Delete the branch reference
	// TODO: RefManager doesn't have DeleteRef method yet
	// For now, we'll just return an error
	return fmt.Errorf("branch deletion not yet implemented")
}

// MergeBranch merges the specified branch into the current branch
func (r *Repository) MergeBranch(branchName string) error {
	// This is a simplified merge that just moves the current branch
	// to point to the same commit as the source branch
	// A real merge would handle conflicts and create merge commits
	
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Get the source branch commit
	sourceBranch := "refs/heads/" + branchName
	sourceRef, err := r.refManager.GetRef(sourceBranch)
	if err != nil {
		return fmt.Errorf("source branch %s not found: %w", branchName, err)
	}
	
	// Get current branch
	currentBranch, err := r.refManager.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	
	// Update current branch to point to source commit
	currentRef := "refs/heads/" + currentBranch
	err = r.refManager.UpdateRef(currentRef, sourceRef.Hash, "")
	if err != nil {
		return fmt.Errorf("failed to merge: %w", err)
	}
	
	return nil
}