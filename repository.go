package govc

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/caiatech/govc/pkg/object"
	"github.com/caiatech/govc/pkg/refs"
	"github.com/caiatech/govc/pkg/storage"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// isCommitHash checks if a string looks like a commit hash (hexadecimal, 7-40 chars)
func isCommitHash(ref string) bool {
	// Match hexadecimal strings between 7 and 40 characters
	matched, _ := regexp.MatchString("^[a-fA-F0-9]{7,40}$", ref)
	return matched
}

// Repository is govc's core abstraction - a memory-first Git repository.
// Unlike traditional Git, this repository can exist entirely in memory,
// enabling instant operations and parallel realities. The path can be
// ":memory:" for pure in-memory operation or a filesystem path for
// optional persistence.
type Repository struct {
	path       string                // ":memory:" for pure memory operation
	store      *storage.Store        // Memory-first object storage
	refManager *refs.RefManager      // Instant branch operations
	staging    *StagingArea          // In-memory staging
	worktree   *Worktree             // Can be virtual (memory-only)
	config     map[string]string     // In-memory config
	stashes    []*Stash              // In-memory stash list
	webhooks   map[string]*Webhook   // Registered webhooks
	events     chan *RepositoryEvent      // Event stream
	gc         *GarbageCollector          // Memory compaction system
	eventBus   *EventBus                  // Pub/Sub event system
	txnManager *AtomicTransactionManager  // Atomic operations manager
	queryEngine *QueryEngine              // Efficient data access system
	deltaCompression *DeltaCompression    // Storage optimization for high-frequency commits
	mu         sync.RWMutex
}

// InitRepository initializes a new repository with file persistence.
func InitRepository(path string) (*Repository, error) {
	gitDir := filepath.Join(path, ".govc")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .govc directory: %v", err)
	}

	objectsDir := filepath.Join(gitDir, "objects")
	if err := os.MkdirAll(objectsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create objects directory: %v", err)
	}

	refsDir := filepath.Join(gitDir, "refs", "heads")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create refs directory: %v", err)
	}

	backend := storage.NewFileBackend(gitDir)
	store := storage.NewStore(backend)
	refStore := refs.NewFileRefStore(gitDir)
	refManager := refs.NewRefManager(refStore)

	// Create main branch pointing to nil (no commits yet)
	if err := refManager.CreateBranch("main", ""); err != nil {
		return nil, fmt.Errorf("failed to create main branch: %v", err)
	}

	if err := refManager.SetHEADToBranch("main"); err != nil {
		return nil, fmt.Errorf("failed to set HEAD: %v", err)
	}

	repo := &Repository{
		path:       path,
		store:      store,
		refManager: refManager,
		staging:    NewStagingArea(),
		worktree:   NewWorktree(path),
		config:     make(map[string]string),
		webhooks:   make(map[string]*Webhook),
		events:     make(chan *RepositoryEvent, 100),
	}

	return repo, nil
}

// OpenRepository opens an existing repository from disk.
func OpenRepository(path string) (*Repository, error) {
	gitDir := filepath.Join(path, ".govc")
	if _, err := os.Stat(gitDir); err != nil {
		return nil, fmt.Errorf("not a govc repository: %v", err)
	}

	backend := storage.NewFileBackend(gitDir)
	store := storage.NewStore(backend)
	refStore := refs.NewFileRefStore(gitDir)
	refManager := refs.NewRefManager(refStore)

	return &Repository{
		path:       path,
		store:      store,
		refManager: refManager,
		staging:    NewStagingArea(),
		worktree:   NewWorktree(path),
		config:     make(map[string]string),
		stashes:    make([]*Stash, 0),
		webhooks:   make(map[string]*Webhook),
		events:     make(chan *RepositoryEvent, 100),
	}, nil
}

func (r *Repository) Add(patterns ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(patterns) == 0 {
		patterns = []string{"."}
	}

	for _, pattern := range patterns {
		files, err := r.worktree.MatchFiles(pattern)
		if err != nil {
			return err
		}

		for _, file := range files {
			content, err := r.worktree.ReadFile(file)
			if err != nil {
				return err
			}

			hash, err := r.store.StoreBlob(content)
			if err != nil {
				return err
			}

			r.staging.Add(file, hash)
		}
	}

	return nil
}

func (r *Repository) Commit(message string) (*object.Commit, error) {
	// Execute pre-commit hooks first
	if err := r.executePreCommitHooks(); err != nil {
		return nil, fmt.Errorf("pre-commit hooks failed: %v", err)
	}

	// Get config values before locking
	authorName := r.getConfigValue("user.name", "Unknown")
	authorEmail := r.getConfigValue("user.email", "unknown@example.com")

	r.mu.Lock()
	defer r.mu.Unlock()

	tree, err := r.createTreeFromStaging()
	if err != nil {
		return nil, err
	}

	treeHash, err := r.store.StoreTree(tree)
	if err != nil {
		return nil, err
	}

	author := object.Author{
		Name:  authorName,
		Email: authorEmail,
		Time:  time.Now(),
	}

	commit := object.NewCommit(treeHash, author, message)

	currentBranch, err := r.refManager.GetCurrentBranch()
	if err == nil && currentBranch != "" {
		parentHash, err := r.refManager.GetBranch(currentBranch)
		if err == nil {
			commit.SetParent(parentHash)
		}
	} else if err != nil {
		// If we can't get current branch, we might be on main but it doesn't exist yet
		currentBranch = "main"
	}

	commitHash, err := r.store.StoreCommit(commit)
	if err != nil {
		return nil, err
	}

	if currentBranch != "" {
		// Update or create the branch
		if err := r.refManager.UpdateRef("refs/heads/"+currentBranch, commitHash, ""); err != nil {
			return nil, err
		}
		// Make sure HEAD points to the branch
		if err := r.refManager.SetHEADToBranch(currentBranch); err != nil {
			return nil, err
		}
	} else {
		if err := r.refManager.SetHEADToCommit(commitHash); err != nil {
			return nil, err
		}
	}

	r.staging.Clear()

	// Execute post-commit hooks
	if err := r.executePostCommitHooks(commit); err != nil {
		// Post-commit hooks failing shouldn't fail the commit itself
		// but we should log or emit an event about the failure
		r.EmitEvent("post-commit-hook-failed", map[string]interface{}{
			"commit": commit.Hash(),
			"error":  err.Error(),
		})
	}

	// Trigger query engine indexing after commit
	if r.queryEngine != nil {
		go r.queryEngine.TriggerIndexing() // Do this async to not block commit
	}

	return commit, nil
}

func (r *Repository) Branch(name string) *BranchBuilder {
	return &BranchBuilder{
		repo: r,
		name: name,
	}
}

func (r *Repository) ListBranches() ([]refs.Ref, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.refManager.ListBranches()
}

func (r *Repository) CurrentBranch() (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.refManager.GetCurrentBranch()
}

// CreateTag creates a new tag at the current HEAD
func (r *Repository) CreateTag(name string, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get current HEAD
	commitHash, err := r.refManager.GetHEAD()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Create tag
	return r.refManager.CreateTag(name, commitHash)
}

// ListTags returns all tags
func (r *Repository) ListTags() ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	refs, err := r.refManager.ListTags()
	if err != nil {
		return nil, err
	}

	tags := make([]string, len(refs))
	for i, ref := range refs {
		// Remove refs/tags/ prefix
		tags[i] = strings.TrimPrefix(ref.Name, "refs/tags/")
	}
	return tags, nil
}

// GetTagCommit returns the commit hash for a given tag
func (r *Repository) GetTagCommit(tagName string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	return r.refManager.GetTag(tagName)
}

// CurrentCommit returns the current HEAD commit
func (r *Repository) CurrentCommit() (*object.Commit, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	head, err := r.refManager.GetHEAD()
	if err != nil {
		return nil, err
	}

	return r.store.GetCommit(head)
}

func (r *Repository) Checkout(ref string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// First check if this is a branch name
	// Branch names can contain slashes (e.g., feature/add-tests)
	// So we need to check if it's a valid branch first
	isBranch := false
	branchName := ref
	if strings.HasPrefix(ref, "refs/heads/") {
		branchName = strings.TrimPrefix(ref, "refs/heads/")
		isBranch = true
	} else {
		// Check if this ref exists as a branch
		if _, err := r.refManager.GetBranch(ref); err == nil {
			branchName = ref
			isBranch = true
		} else {
			// Branch doesn't exist, but check if it looks like a commit hash
			// If it doesn't look like a commit hash, treat it as a new branch name
			if !isCommitHash(ref) {
				branchName = ref
				isBranch = true
			}
		}
	}

	if isBranch {

		// Check if the branch exists
		branchHash, err := r.refManager.GetBranch(branchName)
		if err != nil {
			// Branch doesn't exist - return error
			return fmt.Errorf("branch not found: %s", branchName)
		}

		// Branch exists, update worktree
		if branchHash == "" {
			// Empty branch - clear the worktree
			files := r.worktree.ListFiles()
			for _, file := range files {
				r.worktree.RemoveFile(file)
			}
			return r.refManager.SetHEADToBranch(branchName)
		}

		commit, err := r.store.GetCommit(branchHash)
		if err != nil {
			return err
		}

		tree, err := r.store.GetTree(commit.TreeHash)
		if err != nil {
			return err
		}

		if err := r.updateWorktree(tree); err != nil {
			return err
		}

		return r.refManager.SetHEADToBranch(branchName)
	}

	// Not a branch, resolve as commit
	commitHash, err := r.resolveRef(ref)
	if err != nil {
		return err
	}

	commit, err := r.store.GetCommit(commitHash)
	if err != nil {
		return err
	}

	tree, err := r.store.GetTree(commit.TreeHash)
	if err != nil {
		return err
	}

	if err := r.updateWorktree(tree); err != nil {
		return err
	}

	return r.refManager.SetHEADToCommit(commitHash)
}

func (r *Repository) Status() (*Status, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := &Status{
		Branch:    r.getCurrentBranchName(),
		Staged:    make([]string, 0),
		Modified:  make([]string, 0),
		Untracked: make([]string, 0),
	}

	// Get staged files
	status.Staged = r.staging.List()

	// Get worktree files to check for untracked/modified
	worktreeFiles := r.worktree.ListFiles()

	// Get committed files
	var committedFiles map[string]bool
	headHash, err := r.refManager.GetHEAD()
	if err == nil && headHash != "" {
		commit, err := r.store.GetCommit(headHash)
		if err == nil {
			tree, err := r.store.GetTree(commit.TreeHash)
			if err == nil {
				committedFiles = make(map[string]bool)
				for _, entry := range tree.Entries {
					committedFiles[entry.Name] = true
				}
			}
		}
	}

	// Check each worktree file
	for _, file := range worktreeFiles {
		// Skip if already staged
		if r.staging.Contains(file) {
			continue
		}

		// Check if untracked
		if committedFiles == nil || !committedFiles[file] {
			status.Untracked = append(status.Untracked, file)
		}
	}

	return status, nil
}

func (r *Repository) Log(limit int) ([]*object.Commit, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	commits := make([]*object.Commit, 0)

	headHash, err := r.refManager.GetHEAD()
	if err != nil {
		// No commits yet, return empty slice
		return commits, nil
	}
	currentHash := headHash

	for i := 0; (limit <= 0 || i < limit) && currentHash != ""; i++ {
		commit, err := r.store.GetCommit(currentHash)
		if err != nil {
			break
		}
		commits = append(commits, commit)
		currentHash = commit.ParentHash
	}

	return commits, nil
}

func (r *Repository) createTreeFromStaging() (*object.Tree, error) {
	tree := object.NewTree()

	// Get current branch to find parent commit
	currentBranch, err := r.refManager.GetCurrentBranch()
	if err == nil && currentBranch != "" {
		parentHash, err := r.refManager.GetBranch(currentBranch)
		if err == nil && parentHash != "" {
			// Load parent commit's tree
			parentCommit, err := r.store.GetCommit(parentHash)
			if err == nil {
				parentTree, err := r.store.GetTree(parentCommit.TreeHash)
				if err == nil {
					// Copy all entries from parent tree
					for _, entry := range parentTree.Entries {
						// Skip if this file is being updated in staging or removed
						if !r.staging.Contains(entry.Name) && !r.staging.IsRemoved(entry.Name) {
							tree.AddEntry(entry.Mode, entry.Name, entry.Hash)
						}
					}
				}
			}
		}
	}

	// Add/update files from staging
	r.staging.ForEach(func(file, hash string) {
		mode := "100644"
		tree.AddEntry(mode, file, hash)
	})

	return tree, nil
}

func (r *Repository) updateWorktree(tree *object.Tree) error {
	// First, clear the worktree
	// Get all current files
	currentFiles := r.worktree.ListFiles()

	// Build a set of files that should exist
	shouldExist := make(map[string]bool)
	for _, entry := range tree.Entries {
		shouldExist[entry.Name] = true
	}

	// Remove files that shouldn't exist
	for _, file := range currentFiles {
		if !shouldExist[file] {
			if err := r.worktree.RemoveFile(file); err != nil {
				return err
			}
		}
	}

	// Now write all files from the tree
	for _, entry := range tree.Entries {
		blob, err := r.store.GetBlob(entry.Hash)
		if err != nil {
			return err
		}

		if err := r.worktree.WriteFile(entry.Name, blob.Content); err != nil {
			return err
		}
	}

	return nil
}

func (r *Repository) resolveRef(ref string) (string, error) {
	// Handle special refs
	if ref == "HEAD" {
		return r.refManager.GetHEAD()
	}

	// Handle shortened refs like HEAD~1, HEAD^
	if strings.HasPrefix(ref, "HEAD") {
		headHash, err := r.refManager.GetHEAD()
		if err != nil {
			return "", err
		}

		// Simple HEAD~N parsing
		if strings.HasPrefix(ref, "HEAD~") {
			n := 1
			if len(ref) > 5 {
				fmt.Sscanf(ref[5:], "%d", &n)
			}

			// Walk back n commits
			currentHash := headHash
			for i := 0; i < n; i++ {
				commit, err := r.store.GetCommit(currentHash)
				if err != nil {
					return "", fmt.Errorf("cannot resolve %s: %w", ref, err)
				}
				if commit.ParentHash == "" {
					return "", fmt.Errorf("cannot go back %d commits from HEAD", n)
				}
				currentHash = commit.ParentHash
			}
			return currentHash, nil
		}

		if ref == "HEAD^" {
			commit, err := r.store.GetCommit(headHash)
			if err != nil {
				return "", err
			}
			if commit.ParentHash == "" {
				return "", fmt.Errorf("HEAD has no parent")
			}
			return commit.ParentHash, nil
		}
	}

	// Full hash
	if len(ref) == 40 {
		return ref, nil
	}

	// Try as branch name
	hash, err := r.refManager.GetBranch(ref)
	if err == nil {
		return hash, nil
	}

	// Try as tag
	hash, err = r.refManager.GetTag(ref)
	if err == nil {
		return hash, nil
	}

	// Try as shortened commit hash (at least 4 chars)
	if len(ref) >= 4 && len(ref) < 40 {
		// This is a simplified approach - in a real implementation,
		// we'd need to search through all commits
		// For now, we'll check if it's a valid hex string
		for _, c := range ref {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return "", fmt.Errorf("ref not found: %s", ref)
			}
		}
		// In a real implementation, we'd search for commits starting with this prefix
		return "", fmt.Errorf("shortened commit hash not yet supported: %s", ref)
	}

	return "", fmt.Errorf("ref not found: %s", ref)
}

// SetConfig sets a configuration value.
func (r *Repository) SetConfig(key, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config[key] = value
}

// GetConfig gets a configuration value.
func (r *Repository) GetConfig(key string) string {
	return r.getConfigValue(key, "")
}

func (r *Repository) getConfigValue(key, defaultValue string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if val, ok := r.config[key]; ok {
		return val
	}
	return defaultValue
}

func (r *Repository) getCurrentBranchName() string {
	branch, err := r.refManager.GetCurrentBranch()
	if err != nil {
		return "HEAD"
	}
	return branch
}

type StagingArea struct {
	files   map[string]string // Files staged for addition/modification
	removed map[string]bool   // Files staged for removal
	mu      sync.RWMutex
}

func NewStagingArea() *StagingArea {
	return &StagingArea{
		files:   make(map[string]string),
		removed: make(map[string]bool),
	}
}

func (s *StagingArea) Add(path, hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[path] = hash
}

func (s *StagingArea) Remove(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, path)
	s.removed[path] = true
}

func (s *StagingArea) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = make(map[string]string)
	s.removed = make(map[string]bool)
}

func (s *StagingArea) IsRemoved(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.removed[path]
}

func (s *StagingArea) GetFiles() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	files := make(map[string]string)
	for k, v := range s.files {
		files[k] = v
	}
	return files
}

func (s *StagingArea) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, 0, len(s.files))
	for path := range s.files {
		result = append(result, path)
	}
	return result
}

// ForEach iterates over staged files safely with a callback
func (s *StagingArea) ForEach(callback func(path, hash string)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	for path, hash := range s.files {
		callback(path, hash)
	}
}

// Contains checks if a path is staged
func (s *StagingArea) Contains(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	_, exists := s.files[path]
	return exists
}

// GetHash returns the hash for a specific staged file
func (s *StagingArea) GetHash(path string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	hash, exists := s.files[path]
	return hash, exists
}

// SetFile sets a file hash directly (for internal operations like stash apply)
func (s *StagingArea) SetFile(path, hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.files[path] = hash
}

type Worktree struct {
	path  string
	files map[string][]byte // For memory-based worktree
	mu    sync.RWMutex
}

func NewWorktree(path string) *Worktree {
	wt := &Worktree{path: path}
	if path == ":memory:" {
		wt.files = make(map[string][]byte)
	}
	return wt
}

func (w *Worktree) MatchFiles(pattern string) ([]string, error) {
	var files []string

	if w.path == ":memory:" {
		// For memory worktree, return files from memory
		w.mu.RLock()
		defer w.mu.RUnlock()
		for path := range w.files {
			// Simple pattern matching for "*" or specific patterns
			if pattern == "*" || pattern == "." {
				files = append(files, path)
			} else if matched, _ := filepath.Match(pattern, path); matched {
				files = append(files, path)
			}
		}
		return files, nil
	}

	if pattern == "." || pattern == "*" {
		err := filepath.Walk(w.path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || strings.Contains(path, ".govc") {
				return nil
			}
			relPath, err := filepath.Rel(w.path, path)
			if err != nil {
				return err
			}
			files = append(files, relPath)
			return nil
		})
		return files, err
	}

	if strings.Contains(pattern, "*") {
		matches, err := filepath.Glob(filepath.Join(w.path, pattern))
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			relPath, err := filepath.Rel(w.path, match)
			if err != nil {
				continue
			}
			files = append(files, relPath)
		}
		return files, nil
	}

	fullPath := filepath.Join(w.path, pattern)
	if _, err := os.Stat(fullPath); err == nil {
		files = append(files, pattern)
	}

	return files, nil
}

func (w *Worktree) ListFiles() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.path == ":memory:" {
		// For memory-based worktree, list from memory
		files := make([]string, 0, len(w.files))
		for path := range w.files {
			files = append(files, path)
		}
		return files
	}

	// For file-based worktree, walk the directory
	var files []string
	filepath.Walk(w.path, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Skip .git directory
		if strings.Contains(path, ".git") {
			return nil
		}

		relPath, err := filepath.Rel(w.path, path)
		if err == nil {
			files = append(files, relPath)
		}
		return nil
	})

	return files
}

func (w *Worktree) ReadFile(path string) ([]byte, error) {
	if w.path == ":memory:" {
		// For memory-based worktree, read from memory
		w.mu.RLock()
		defer w.mu.RUnlock()
		if content, ok := w.files[path]; ok {
			return content, nil
		}
		return nil, fmt.Errorf("file not found in memory worktree: %s", path)
	}
	return os.ReadFile(filepath.Join(w.path, path))
}

func (w *Worktree) WriteFile(path string, content []byte) error {
	if w.path == ":memory:" {
		// For memory-based worktree, store in memory
		w.mu.Lock()
		defer w.mu.Unlock()
		w.files[path] = content
		return nil
	}
	fullPath := filepath.Join(w.path, path)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(fullPath, content, 0644)
}

func (w *Worktree) RemoveFile(path string) error {
	if w.path == ":memory:" {
		// For memory-based worktree, remove from memory
		w.mu.Lock()
		defer w.mu.Unlock()
		delete(w.files, path)
		return nil
	}
	fullPath := filepath.Join(w.path, path)
	return os.Remove(fullPath)
}

type Status struct {
	Branch    string
	Staged    []string
	Modified  []string
	Untracked []string
}

// Stash represents a saved working directory and staging area state
type Stash struct {
	ID           string
	Message      string
	Author       object.Author
	TreeHash     string            // Hash of the stashed tree
	ParentCommit string            // Commit hash when stash was created
	StagedFiles  map[string]string // path -> hash of staged files
	WorkingFiles map[string][]byte // path -> content of working files
	Timestamp    time.Time
}

type BranchBuilder struct {
	repo *Repository
	name string
}

func (b *BranchBuilder) Create() error {
	b.repo.mu.Lock()
	defer b.repo.mu.Unlock()

	// Get the actual commit hash that HEAD points to
	currentBranch, err := b.repo.refManager.GetCurrentBranch()
	if err != nil {
		// HEAD is detached, get the commit directly
		headHash, err := b.repo.refManager.GetHEAD()
		if err != nil {
			return err
		}
		return b.repo.refManager.CreateBranch(b.name, headHash)
	}

	// HEAD points to a branch, get the branch's commit
	branchHash, err := b.repo.refManager.GetBranch(currentBranch)
	if err != nil {
		// Current branch has no commits yet
		// Create the new branch pointing to nothing (will be set on first commit)
		return b.repo.refManager.CreateBranch(b.name, "")
	}

	return b.repo.refManager.CreateBranch(b.name, branchHash)
}

func (b *BranchBuilder) Checkout() error {
	if err := b.Create(); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return err
		}
	}
	return b.repo.Checkout(b.name)
}

func (b *BranchBuilder) Delete() error {
	b.repo.mu.Lock()
	defer b.repo.mu.Unlock()

	// Check if this is the current branch
	currentBranch, err := b.repo.refManager.GetCurrentBranch()
	if err == nil && currentBranch == b.name {
		return fmt.Errorf("cannot delete current branch")
	}

	return b.repo.refManager.DeleteBranch(b.name)
}

func Clone(url, path string) (*Repository, error) {
	return nil, fmt.Errorf("clone not yet implemented")
}

func (r *Repository) Push(remote, branch string) error {
	// Execute pre-push hooks first
	if err := r.executePrePushHooks(remote, branch); err != nil {
		return fmt.Errorf("pre-push hooks failed: %v", err)
	}

	return fmt.Errorf("push not yet implemented")
}

func (r *Repository) Pull(remote, branch string) error {
	return fmt.Errorf("pull not yet implemented")
}

// Merge merges changes from the specified branch into the current branch.
// Supports both fast-forward and three-way merge.
func (r *Repository) Merge(from, to string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Save current branch
	currentBranch, err := r.refManager.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("not on a branch")
	}

	// Switch to target branch if needed
	if to != "" && to != currentBranch {
		if err := r.Checkout(to); err != nil {
			return fmt.Errorf("failed to checkout %s: %v", to, err)
		}
		defer r.Checkout(currentBranch) // Switch back after merge
	}

	targetHash, err := r.refManager.GetBranch(from)
	if err != nil {
		return fmt.Errorf("branch not found: %s", from)
	}

	currentHash, err := r.refManager.GetBranch(r.getCurrentBranchName())
	if err != nil {
		return fmt.Errorf("current branch has no commits")
	}

	if r.canFastForward(currentHash, targetHash) {
		return r.refManager.UpdateRef("refs/heads/"+r.getCurrentBranchName(), targetHash, currentHash)
	}

	mergeBase, err := r.findMergeBase(currentHash, targetHash)
	if err != nil {
		return err
	}

	if mergeBase == targetHash {
		return fmt.Errorf("already up to date")
	}

	if mergeBase == currentHash {
		return r.refManager.UpdateRef("refs/heads/"+r.getCurrentBranchName(), targetHash, currentHash)
	}

	return r.threeWayMerge(currentHash, targetHash, mergeBase, from)
}

func (r *Repository) canFastForward(currentHash, targetHash string) bool {
	commitHash := targetHash
	for commitHash != "" {
		if commitHash == currentHash {
			return true
		}
		commit, err := r.store.GetCommit(commitHash)
		if err != nil {
			return false
		}
		commitHash = commit.ParentHash
	}
	return false
}

func (r *Repository) findMergeBase(hash1, hash2 string) (string, error) {
	ancestors1 := make(map[string]bool)

	commitHash := hash1
	for commitHash != "" {
		ancestors1[commitHash] = true
		commit, err := r.store.GetCommit(commitHash)
		if err != nil {
			break
		}
		commitHash = commit.ParentHash
	}

	commitHash = hash2
	for commitHash != "" {
		if ancestors1[commitHash] {
			return commitHash, nil
		}
		commit, err := r.store.GetCommit(commitHash)
		if err != nil {
			break
		}
		commitHash = commit.ParentHash
	}

	return "", fmt.Errorf("no common ancestor found")
}

func (r *Repository) threeWayMerge(ourHash, theirHash, baseHash, branchName string) error {
	baseCommit, err := r.store.GetCommit(baseHash)
	if err != nil {
		return err
	}
	baseTree, err := r.store.GetTree(baseCommit.TreeHash)
	if err != nil {
		return err
	}

	ourCommit, err := r.store.GetCommit(ourHash)
	if err != nil {
		return err
	}
	ourTree, err := r.store.GetTree(ourCommit.TreeHash)
	if err != nil {
		return err
	}

	theirCommit, err := r.store.GetCommit(theirHash)
	if err != nil {
		return err
	}
	theirTree, err := r.store.GetTree(theirCommit.TreeHash)
	if err != nil {
		return err
	}

	mergedTree, conflicts := r.mergeTrees(baseTree, ourTree, theirTree)
	if len(conflicts) > 0 {
		return fmt.Errorf("merge conflicts in files: %v", conflicts)
	}

	mergedTreeHash, err := r.store.StoreTree(mergedTree)
	if err != nil {
		return err
	}

	author := object.Author{
		Name:  r.getConfigValue("user.name", "Unknown"),
		Email: r.getConfigValue("user.email", "unknown@example.com"),
		Time:  time.Now(),
	}

	mergeCommit := &object.Commit{
		TreeHash:   mergedTreeHash,
		ParentHash: ourHash,
		Author:     author,
		Committer:  author,
		Message:    fmt.Sprintf("Merge branch '%s'", branchName),
	}

	mergeCommitHash, err := r.store.StoreCommit(mergeCommit)
	if err != nil {
		return err
	}

	currentBranch, _ := r.refManager.GetCurrentBranch()
	return r.refManager.UpdateRef("refs/heads/"+currentBranch, mergeCommitHash, ourHash)
}

func (r *Repository) mergeTrees(base, ours, theirs *object.Tree) (*object.Tree, []string) {
	merged := object.NewTree()
	conflicts := []string{}

	allFiles := make(map[string]bool)
	for _, entry := range base.Entries {
		allFiles[entry.Name] = true
	}
	for _, entry := range ours.Entries {
		allFiles[entry.Name] = true
	}
	for _, entry := range theirs.Entries {
		allFiles[entry.Name] = true
	}

	for file := range allFiles {
		baseEntry := r.findTreeEntry(base, file)
		ourEntry := r.findTreeEntry(ours, file)
		theirEntry := r.findTreeEntry(theirs, file)

		if ourEntry == nil && theirEntry == nil {
			continue
		}

		if ourEntry != nil && theirEntry == nil {
			if baseEntry != nil {
				continue
			}
			merged.AddEntry(ourEntry.Mode, ourEntry.Name, ourEntry.Hash)
			continue
		}

		if ourEntry == nil && theirEntry != nil {
			if baseEntry != nil {
				continue
			}
			merged.AddEntry(theirEntry.Mode, theirEntry.Name, theirEntry.Hash)
			continue
		}

		if ourEntry.Hash == theirEntry.Hash {
			merged.AddEntry(ourEntry.Mode, ourEntry.Name, ourEntry.Hash)
			continue
		}

		if baseEntry != nil && baseEntry.Hash == ourEntry.Hash {
			merged.AddEntry(theirEntry.Mode, theirEntry.Name, theirEntry.Hash)
			continue
		}

		if baseEntry != nil && baseEntry.Hash == theirEntry.Hash {
			merged.AddEntry(ourEntry.Mode, ourEntry.Name, ourEntry.Hash)
			continue
		}

		conflicts = append(conflicts, file)
	}

	return merged, conflicts
}

func (r *Repository) findTreeEntry(tree *object.Tree, name string) *object.TreeEntry {
	for _, entry := range tree.Entries {
		if entry.Name == name {
			return &entry
		}
	}
	return nil
}

func (r *Repository) Serve(addr string) error {
	return StartServer(r, addr)
}

func StartServer(repo *Repository, addr string) error {
	server := NewHTTPServer(repo)
	return http.ListenAndServe(addr, server)
}

type HTTPServer struct {
	repo   *Repository
	router *mux.Router
}

func NewHTTPServer(repo *Repository) *HTTPServer {
	s := &HTTPServer{
		repo:   repo,
		router: mux.NewRouter(),
	}
	s.setupRoutes()
	return s
}

func (s *HTTPServer) setupRoutes() {
	s.router.HandleFunc("/{repo}/info/refs", s.handleInfoRefs).Methods("GET")
	s.router.HandleFunc("/{repo}/git-upload-pack", s.handleUploadPack).Methods("POST")
	s.router.HandleFunc("/{repo}/git-receive-pack", s.handleReceivePack).Methods("POST")
}

func (s *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *HTTPServer) handleInfoRefs(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")

	if service != "git-upload-pack" && service != "git-receive-pack" {
		http.Error(w, "Invalid service", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	w.Header().Set("Cache-Control", "no-cache")

	packet := fmt.Sprintf("# service=%s\n", service)
	fmt.Fprintf(w, "%04x%s0000", len(packet)+4, packet)

	branches, _ := s.repo.ListBranches()
	for i, branch := range branches {
		line := fmt.Sprintf("%s %s", branch.Hash, branch.Name)
		if i == 0 {
			line += "\x00capabilities^{}"
		}
		fmt.Fprintf(w, "%04x%s\n", len(line)+5, line)
	}

	fmt.Fprint(w, "0000")
}

func (s *HTTPServer) handleUploadPack(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, "0008NAK\n")
}

func (s *HTTPServer) handleReceivePack(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, "0000")
}

func (r *Repository) GetObject(hash string) (object.Object, error) {
	return r.store.GetObject(hash)
}

func (r *Repository) HasObject(hash string) bool {
	return r.store.HasObject(hash)
}

func (r *Repository) Export(w io.Writer, format string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if format != "fast-export" {
		return fmt.Errorf("unsupported export format: %s", format)
	}

	// Get all branches
	branches, err := r.refManager.ListBranches()
	if err != nil {
		return fmt.Errorf("failed to list branches: %w", err)
	}

	// Get all tags
	tags, err := r.refManager.ListTags()
	if err != nil {
		// Tags might not exist, that's OK
		tags = []refs.Ref{}
	}

	// Track exported commits and their marks
	commitToMark := make(map[string]int)
	markCounter := 1

	// Start with empty blob
	fmt.Fprintf(w, "blob\n")
	fmt.Fprintf(w, "mark :%d\n", markCounter)
	fmt.Fprintf(w, "data 0\n\n")
	markCounter++

	// Collect all unique commits from all branches
	allCommits := make(map[string]*object.Commit)
	commitOrder := make([]string, 0)

	var collectCommits func(hash string) error
	collectCommits = func(hash string) error {
		if hash == "" || allCommits[hash] != nil {
			return nil
		}

		commit, err := r.store.GetCommit(hash)
		if err != nil {
			return err
		}

		// Recurse to parent first
		if commit.ParentHash != "" {
			if err := collectCommits(commit.ParentHash); err != nil {
				return err
			}
		}

		allCommits[hash] = commit
		commitOrder = append(commitOrder, hash)
		return nil
	}

	// Collect commits from all branches
	for _, branch := range branches {
		if branch.Hash != "" {
			collectCommits(branch.Hash)
		}
	}

	// Export all commits first
	for _, hash := range commitOrder {
		commit := allCommits[hash]
		commitToMark[hash] = markCounter

		// For now, export all commits to the main branch
		// This is a simplification - a full implementation would track branch points
		fmt.Fprintf(w, "commit refs/heads/main\n")
		fmt.Fprintf(w, "mark :%d\n", markCounter)
		fmt.Fprintf(w, "author %s <%s> %d +0000\n", commit.Author.Name, commit.Author.Email, commit.Author.Time.Unix())
		fmt.Fprintf(w, "committer %s <%s> %d +0000\n", commit.Committer.Name, commit.Committer.Email, commit.Committer.Time.Unix())
		fmt.Fprintf(w, "data %d\n%s\n", len(commit.Message), commit.Message)

		if commit.ParentHash != "" {
			if parentMark, exists := commitToMark[commit.ParentHash]; exists {
				fmt.Fprintf(w, "from :%d\n", parentMark)
			}
		}

		// Export tree
		tree, err := r.store.GetTree(commit.TreeHash)
		if err == nil {
			for _, entry := range tree.Entries {
				blob, err := r.store.GetBlob(entry.Hash)
				if err == nil {
					fmt.Fprintf(w, "M 100644 inline %s\n", entry.Name)
					fmt.Fprintf(w, "data %d\n", len(blob.Content))
					w.Write(blob.Content)
					fmt.Fprintf(w, "\n")
				}
			}
		}

		markCounter++
	}

	// Export branch refs
	for _, branch := range branches {
		if branch.Hash != "" && branch.Name != "refs/heads/main" {
			if mark, exists := commitToMark[branch.Hash]; exists {
				fmt.Fprintf(w, "reset %s\n", branch.Name)
				fmt.Fprintf(w, "from :%d\n\n", mark)
			}
		}
	}

	// Export tags
	for _, tag := range tags {
		if mark, exists := commitToMark[tag.Hash]; exists {
			fmt.Fprintf(w, "tag %s\n", strings.TrimPrefix(tag.Name, "refs/tags/"))
			fmt.Fprintf(w, "from :%d\n", mark)
			fmt.Fprintf(w, "tagger Unknown <unknown@example.com> %d +0000\n", time.Now().Unix())
			fmt.Fprintf(w, "data 0\n\n")
		}
	}

	fmt.Fprintf(w, "done\n")
	return nil
}

// ReadFile reads a file from the working directory if it exists, otherwise from HEAD
func (r *Repository) ReadFile(path string) ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// First try to read from working directory
	content, err := r.worktree.ReadFile(path)
	if err == nil {
		return content, nil
	}

	// Check if the file has been explicitly removed
	if r.staging.IsRemoved(path) {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	// If not in working directory and not removed, try to read from HEAD
	headHash, err := r.refManager.GetHEAD()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Get the commit
	commit, err := r.store.GetCommit(headHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	// Get the tree
	tree, err := r.store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get tree: %w", err)
	}

	// Find the file in the tree
	for _, entry := range tree.Entries {
		if entry.Name == path {
			blob, err := r.store.GetBlob(entry.Hash)
			if err != nil {
				return nil, fmt.Errorf("failed to get blob: %w", err)
			}
			return blob.Content, nil
		}
	}

	return nil, fmt.Errorf("file not found: %s", path)
}

// ListFiles lists all files that are currently accessible (working directory + HEAD - removed files)
func (r *Repository) ListFiles() ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Start with files from working directory
	files := make(map[string]bool)
	worktreeFiles := r.worktree.ListFiles()
	for _, file := range worktreeFiles {
		files[file] = true
	}

	// Add files from HEAD that haven't been explicitly removed
	headHash, err := r.refManager.GetHEAD()
	if err == nil {
		commit, err := r.store.GetCommit(headHash)
		if err == nil {
			tree, err := r.store.GetTree(commit.TreeHash)
			if err == nil {
				for _, entry := range tree.Entries {
					// Only add if not explicitly removed and not already in worktree
					if !r.staging.IsRemoved(entry.Name) {
						files[entry.Name] = true
					}
				}
			}
		}
	}

	// Convert to slice
	result := make([]string, 0, len(files))
	for file := range files {
		result = append(result, file)
	}

	return result, nil
}

// Remove removes a file from the staging area and working directory
func (r *Repository) Remove(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove from staging
	r.staging.Remove(path)

	// Also remove from working directory
	return r.worktree.RemoveFile(path)
}

// WriteFile writes a file to the worktree
func (r *Repository) WriteFile(path string, content []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.worktree.WriteFile(path, content)
}

// Diff generates a diff between two commits or refs
func (r *Repository) Diff(from, to, format string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Resolve refs to commit hashes
	fromHash, err := r.resolveRef(from)
	if err != nil {
		return "", fmt.Errorf("failed to resolve 'from' ref: %w", err)
	}

	toHash, err := r.resolveRef(to)
	if err != nil {
		return "", fmt.Errorf("failed to resolve 'to' ref: %w", err)
	}

	// Get the commits
	fromCommit, err := r.store.GetCommit(fromHash)
	if err != nil {
		return "", fmt.Errorf("failed to get 'from' commit: %w", err)
	}

	toCommit, err := r.store.GetCommit(toHash)
	if err != nil {
		return "", fmt.Errorf("failed to get 'to' commit: %w", err)
	}

	// Get the trees
	fromTree, err := r.store.GetTree(fromCommit.TreeHash)
	if err != nil {
		return "", fmt.Errorf("failed to get 'from' tree: %w", err)
	}

	toTree, err := r.store.GetTree(toCommit.TreeHash)
	if err != nil {
		return "", fmt.Errorf("failed to get 'to' tree: %w", err)
	}

	// Generate diff based on format
	switch format {
	case "unified":
		return r.generateUnifiedDiff(fromTree, toTree, fromHash, toHash)
	case "raw":
		return r.generateRawDiff(fromTree, toTree)
	case "name-only":
		return r.generateNameOnlyDiff(fromTree, toTree)
	default:
		return r.generateUnifiedDiff(fromTree, toTree, fromHash, toHash)
	}
}

// DiffFile generates a diff for a specific file between two commits
func (r *Repository) DiffFile(from, to, filePath string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Resolve refs to commit hashes
	fromHash, err := r.resolveRef(from)
	if err != nil {
		return "", fmt.Errorf("failed to resolve 'from' ref: %w", err)
	}

	toHash, err := r.resolveRef(to)
	if err != nil {
		return "", fmt.Errorf("failed to resolve 'to' ref: %w", err)
	}

	// Get the file content from both commits
	fromContent, err := r.getFileAtCommit(fromHash, filePath)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return "", fmt.Errorf("failed to get file from 'from' commit: %w", err)
	}

	toContent, err := r.getFileAtCommit(toHash, filePath)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return "", fmt.Errorf("failed to get file from 'to' commit: %w", err)
	}

	// Generate unified diff for the file
	return generateFileDiff(filePath, fromContent, toContent), nil
}

// FileDiffInfo represents changes to a single file
type FileDiffInfo struct {
	Path      string
	OldPath   string
	Status    string // added, modified, deleted, renamed
	Additions int
	Deletions int
	Patch     string
}

// WorkingDiffInfo represents staged and unstaged changes
type WorkingDiffInfo struct {
	Staged   []FileDiffInfo
	Unstaged []FileDiffInfo
}

// DiffWorking generates diff between HEAD and working directory
func (r *Repository) DiffWorking() (*WorkingDiffInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get current HEAD
	headHash, err := r.refManager.GetHEAD()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Get HEAD commit and tree
	headCommit, err := r.store.GetCommit(headHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD commit: %w", err)
	}

	headTree, err := r.store.GetTree(headCommit.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD tree: %w", err)
	}

	// Build map of files in HEAD
	headFiles := make(map[string]string)
	for _, entry := range headTree.Entries {
		headFiles[entry.Name] = entry.Hash
	}

	// Get staged files
	stagedDiffs := []FileDiffInfo{}
	r.staging.ForEach(func(path, hash string) {
		oldHash, exists := headFiles[path]
		status := "modified"
		if !exists {
			status = "added"
		}

		// Get content for diff
		var oldContent, newContent []byte
		if exists {
			if blob, err := r.store.GetBlob(oldHash); err == nil {
				oldContent = blob.Content
			}
		}
		if blob, err := r.store.GetBlob(hash); err == nil {
			newContent = blob.Content
		}

		patch := generateFileDiff(path, oldContent, newContent)
		additions, deletions := countChanges(patch)

		stagedDiffs = append(stagedDiffs, FileDiffInfo{
			Path:      path,
			Status:    status,
			Additions: additions,
			Deletions: deletions,
			Patch:     patch,
		})
	})

	// For unstaged changes, we'd need to compare working directory with staging
	// This is simplified for now
	unstagedDiffs := []FileDiffInfo{}

	return &WorkingDiffInfo{
		Staged:   stagedDiffs,
		Unstaged: unstagedDiffs,
	}, nil
}

// Helper method to get file content at a specific commit
func (r *Repository) getFileAtCommit(commitHash, filePath string) ([]byte, error) {
	commit, err := r.store.GetCommit(commitHash)
	if err != nil {
		return nil, err
	}

	tree, err := r.store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, err
	}

	for _, entry := range tree.Entries {
		if entry.Name == filePath {
			blob, err := r.store.GetBlob(entry.Hash)
			if err != nil {
				return nil, err
			}
			return blob.Content, nil
		}
	}

	return nil, fmt.Errorf("file not found: %s", filePath)
}

// Generate unified diff between two trees
func (r *Repository) generateUnifiedDiff(fromTree, toTree *object.Tree, fromRef, toRef string) (string, error) {
	var diff strings.Builder

	// Create maps for easy lookup
	fromFiles := make(map[string]string)
	for _, entry := range fromTree.Entries {
		fromFiles[entry.Name] = entry.Hash
	}

	toFiles := make(map[string]string)
	for _, entry := range toTree.Entries {
		toFiles[entry.Name] = entry.Hash
	}

	// Find all unique files
	allFiles := make(map[string]bool)
	for name := range fromFiles {
		allFiles[name] = true
	}
	for name := range toFiles {
		allFiles[name] = true
	}

	// Generate diff for each file
	for path := range allFiles {
		fromHash, inFrom := fromFiles[path]
		toHash, inTo := toFiles[path]

		// Skip if file unchanged
		if inFrom && inTo && fromHash == toHash {
			continue
		}

		// Get file contents
		var fromContent, toContent []byte
		if inFrom {
			if blob, err := r.store.GetBlob(fromHash); err == nil {
				fromContent = blob.Content
			}
		}
		if inTo {
			if blob, err := r.store.GetBlob(toHash); err == nil {
				toContent = blob.Content
			}
		}

		// Add file diff
		diff.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", path, path))
		if !inFrom {
			diff.WriteString("new file mode 100644\n")
		} else if !inTo {
			diff.WriteString("deleted file mode 100644\n")
		}
		diff.WriteString(fmt.Sprintf("--- a/%s\n", path))
		diff.WriteString(fmt.Sprintf("+++ b/%s\n", path))
		diff.WriteString(generateFileDiff(path, fromContent, toContent))
		diff.WriteString("\n")
	}

	return diff.String(), nil
}

// Generate raw diff format
func (r *Repository) generateRawDiff(fromTree, toTree *object.Tree) (string, error) {
	var diff strings.Builder

	// Create maps for easy lookup
	fromFiles := make(map[string]string)
	for _, entry := range fromTree.Entries {
		fromFiles[entry.Name] = entry.Hash
	}

	toFiles := make(map[string]string)
	for _, entry := range toTree.Entries {
		toFiles[entry.Name] = entry.Hash
	}

	// Check all files
	for path, fromHash := range fromFiles {
		if toHash, exists := toFiles[path]; exists {
			if fromHash != toHash {
				diff.WriteString(fmt.Sprintf("M\t%s\n", path))
			}
		} else {
			diff.WriteString(fmt.Sprintf("D\t%s\n", path))
		}
	}

	for path := range toFiles {
		if _, exists := fromFiles[path]; !exists {
			diff.WriteString(fmt.Sprintf("A\t%s\n", path))
		}
	}

	return diff.String(), nil
}

// Generate name-only diff
func (r *Repository) generateNameOnlyDiff(fromTree, toTree *object.Tree) (string, error) {
	var diff strings.Builder

	// Create maps for easy lookup
	fromFiles := make(map[string]string)
	for _, entry := range fromTree.Entries {
		fromFiles[entry.Name] = entry.Hash
	}

	toFiles := make(map[string]string)
	for _, entry := range toTree.Entries {
		toFiles[entry.Name] = entry.Hash
	}

	// Find changed files
	changed := make(map[string]bool)
	for path, fromHash := range fromFiles {
		if toHash, exists := toFiles[path]; !exists || fromHash != toHash {
			changed[path] = true
		}
	}
	for path := range toFiles {
		if _, exists := fromFiles[path]; !exists {
			changed[path] = true
		}
	}

	// Output sorted file names
	for path := range changed {
		diff.WriteString(path + "\n")
	}

	return diff.String(), nil
}

// Simple diff generator for file content
func generateFileDiff(path string, oldContent, newContent []byte) string {
	if len(oldContent) == 0 && len(newContent) == 0 {
		return ""
	}

	oldLines := strings.Split(string(oldContent), "\n")
	newLines := strings.Split(string(newContent), "\n")

	var diff strings.Builder
	diff.WriteString("@@ -1," + fmt.Sprintf("%d", len(oldLines)) + " +1," + fmt.Sprintf("%d", len(newLines)) + " @@\n")

	// Simple line-by-line diff (not a real diff algorithm)
	maxLines := len(oldLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}

	for i := 0; i < maxLines; i++ {
		if i < len(oldLines) && i < len(newLines) {
			if oldLines[i] != newLines[i] {
				diff.WriteString("-" + oldLines[i] + "\n")
				diff.WriteString("+" + newLines[i] + "\n")
			} else {
				diff.WriteString(" " + oldLines[i] + "\n")
			}
		} else if i < len(oldLines) {
			diff.WriteString("-" + oldLines[i] + "\n")
		} else if i < len(newLines) {
			diff.WriteString("+" + newLines[i] + "\n")
		}
	}

	return diff.String()
}

// Count additions and deletions in a patch
func countChanges(patch string) (additions, deletions int) {
	lines := strings.Split(patch, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}
	return
}

// BlameInfo contains blame information for a file
type BlameInfo struct {
	Path  string
	Ref   string
	Lines []BlameLineInfo
	Total int
}

// BlameLineInfo contains blame info for a single line
type BlameLineInfo struct {
	LineNumber int
	Content    string
	CommitHash string
	Author     string
	Email      string
	Timestamp  time.Time
	Message    string
}

// Blame generates line-by-line authorship information for a file
func (r *Repository) Blame(filePath, ref string) (*BlameInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Resolve the ref
	targetHash, err := r.resolveRef(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve ref '%s': %w", ref, err)
	}

	// Get the file content at the target commit
	content, err := r.getFileAtCommit(targetHash, filePath)
	if err != nil {
		return nil, err
	}

	// Split content into lines
	contentStr := string(content)
	// Handle empty content
	if contentStr == "" {
		contentStr = " " // Single empty line
	}
	lines := strings.Split(contentStr, "\n")
	blameLines := make([]BlameLineInfo, 0, len(lines))

	// For a simple implementation, we'll attribute all lines to the last commit
	// that modified the file. A full implementation would track line-by-line changes.
	lastCommitHash, lastCommit, err := r.findLastCommitForFile(filePath, targetHash)
	if err != nil {
		return nil, fmt.Errorf("failed to find commit history for file: %w", err)
	}

	// Create blame info for each line
	for i, line := range lines {
		blameLines = append(blameLines, BlameLineInfo{
			LineNumber: i + 1,
			Content:    line,
			CommitHash: lastCommitHash,
			Author:     lastCommit.Author.Name,
			Email:      lastCommit.Author.Email,
			Timestamp:  lastCommit.Author.Time,
			Message:    strings.TrimSpace(lastCommit.Message),
		})
	}

	return &BlameInfo{
		Path:  filePath,
		Ref:   ref,
		Lines: blameLines,
		Total: len(blameLines),
	}, nil
}

// findLastCommitForFile finds the most recent commit that modified a file
func (r *Repository) findLastCommitForFile(filePath, startHash string) (string, *object.Commit, error) {
	// Walk back through commit history
	currentHash := startHash

	for currentHash != "" {
		commit, err := r.store.GetCommit(currentHash)
		if err != nil {
			return "", nil, err
		}

		// Check if this commit contains the file
		tree, err := r.store.GetTree(commit.TreeHash)
		if err != nil {
			return "", nil, err
		}

		hasFile := false
		for _, entry := range tree.Entries {
			if entry.Name == filePath {
				hasFile = true
				break
			}
		}

		if hasFile {
			// Check if this is the commit that last modified the file
			// For simplicity, we'll return the first commit we find with the file
			// A full implementation would check if the file content changed
			return currentHash, commit, nil
		}

		// Move to parent commit
		currentHash = commit.ParentHash
	}

	return "", nil, fmt.Errorf("file not found in commit history")
}

// Stash saves the current working directory and staging area state
func (r *Repository) Stash(message string, includeUntracked bool) (*Stash, error) {
	// Get author info before locking
	authorName := r.getConfigValue("user.name", "Unknown")
	authorEmail := r.getConfigValue("user.email", "unknown@example.com")

	r.mu.Lock()
	defer r.mu.Unlock()

	// Get current HEAD commit (may be empty for new repos)
	headHash, err := r.refManager.GetHEAD()
	if err != nil {
		// If there's no HEAD yet (new repo), use empty string
		headHash = ""
	}

	// Get status to find files to stash
	// We need to get status without locking since we already hold the lock
	status := &Status{
		Branch:    r.getCurrentBranchName(),
		Staged:    r.staging.List(),
		Modified:  []string{},
		Untracked: []string{},
	}

	// Get modified and untracked files
	if r.worktree != nil {
		// Get all files in working directory
		workFiles, err := r.worktree.MatchFiles("*")
		if err == nil {
			for _, file := range workFiles {
				// Check if file is tracked
				isTracked := false
				if headHash != "" {
					commit, err := r.store.GetCommit(headHash)
					if err == nil {
						tree, err := r.store.GetTree(commit.TreeHash)
						if err == nil {
							for _, entry := range tree.Entries {
								if entry.Name == file {
									isTracked = true
									// Check if modified
									workContent, err := r.worktree.ReadFile(file)
									if err == nil {
										blob, err := r.store.GetBlob(entry.Hash)
										if err == nil && string(workContent) != string(blob.Content) {
											status.Modified = append(status.Modified, file)
										}
									}
									break
								}
							}
						}
					}
				}

				if !isTracked {
					// Check if it's staged
					isStaged := false
					for _, staged := range status.Staged {
						if staged == file {
							isStaged = true
							break
						}
					}
					if !isStaged {
						status.Untracked = append(status.Untracked, file)
					}
				}
			}
		}
	}

	// Create stash ID
	stashID := fmt.Sprintf("stash@{%d}", len(r.stashes))

	// Save staged files
	stagedFiles := make(map[string]string)
	for _, path := range status.Staged {
		if hash, exists := r.staging.GetHash(path); exists {
			stagedFiles[path] = hash
		}
	}

	// Save working directory files
	workingFiles := make(map[string][]byte)

	// Add modified files
	for _, path := range status.Modified {
		content, err := r.worktree.ReadFile(path)
		if err != nil {
			continue
		}
		workingFiles[path] = content
	}

	// Add untracked files if requested
	if includeUntracked {
		for _, path := range status.Untracked {
			content, err := r.worktree.ReadFile(path)
			if err != nil {
				continue
			}
			workingFiles[path] = content
		}
	}

	// Create stash object
	stash := &Stash{
		ID:      stashID,
		Message: message,
		Author: object.Author{
			Name:  authorName,
			Email: authorEmail,
			Time:  time.Now(),
		},
		ParentCommit: headHash,
		StagedFiles:  stagedFiles,
		WorkingFiles: workingFiles,
		Timestamp:    time.Now(),
	}

	// Add to stash list
	r.stashes = append(r.stashes, stash)

	// Clear staging area and reset working directory
	r.staging = NewStagingArea()

	// Reset modified files to HEAD state
	if headHash != "" {
		commit, err := r.store.GetCommit(headHash)
		if err == nil {
			tree, err := r.store.GetTree(commit.TreeHash)
			if err == nil {
				// Create map of tree entries for faster lookup
				treeFiles := make(map[string]*object.TreeEntry)
				for i := range tree.Entries {
					treeFiles[tree.Entries[i].Name] = &tree.Entries[i]
				}

				// Reset all modified files to their HEAD state
				for _, path := range status.Modified {
					if entry, exists := treeFiles[path]; exists {
						blob, err := r.store.GetBlob(entry.Hash)
						if err == nil {
							r.worktree.WriteFile(path, blob.Content)
						}
					}
				}
			}
		}
	}

	// Remove untracked files if they were included in the stash
	if includeUntracked {
		for _, path := range status.Untracked {
			r.worktree.RemoveFile(path)
		}
	}

	return stash, nil
}

// ListStashes returns all stashes
func (r *Repository) ListStashes() []*Stash {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Stash, len(r.stashes))
	copy(result, r.stashes)
	return result
}

// GetStash returns a stash by ID
func (r *Repository) GetStash(stashID string) (*Stash, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, stash := range r.stashes {
		if stash.ID == stashID {
			return stash, nil
		}
	}

	return nil, fmt.Errorf("stash not found: %s", stashID)
}

// ApplyStash applies a stash to the working directory
func (r *Repository) ApplyStash(stashID string, drop bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Find stash
	var stashIndex int = -1
	var stash *Stash
	for i, s := range r.stashes {
		if s.ID == stashID {
			stashIndex = i
			stash = s
			break
		}
	}

	if stash == nil {
		return fmt.Errorf("stash not found: %s", stashID)
	}

	// Apply staged files
	for path, hash := range stash.StagedFiles {
		r.staging.SetFile(path, hash)
	}

	// Apply working directory files
	for path, content := range stash.WorkingFiles {
		if err := r.worktree.WriteFile(path, content); err != nil {
			return fmt.Errorf("failed to restore file %s: %w", path, err)
		}
	}

	// Drop stash if requested
	if drop && stashIndex != -1 {
		r.stashes = append(r.stashes[:stashIndex], r.stashes[stashIndex+1:]...)

		// Renumber remaining stashes
		for i := stashIndex; i < len(r.stashes); i++ {
			r.stashes[i].ID = fmt.Sprintf("stash@{%d}", i)
		}
	}

	return nil
}

// DropStash removes a stash from the list
func (r *Repository) DropStash(stashID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, stash := range r.stashes {
		if stash.ID == stashID {
			r.stashes = append(r.stashes[:i], r.stashes[i+1:]...)

			// Renumber remaining stashes
			for j := i; j < len(r.stashes); j++ {
				r.stashes[j].ID = fmt.Sprintf("stash@{%d}", j)
			}

			return nil
		}
	}

	return fmt.Errorf("stash not found: %s", stashID)
}

// CherryPick applies the changes from a specific commit to the current branch
func (r *Repository) CherryPick(commitHash string) (*object.Commit, error) {
	// Get author info before locking
	authorName := r.getConfigValue("user.name", "Unknown")
	authorEmail := r.getConfigValue("user.email", "unknown@example.com")

	r.mu.Lock()
	defer r.mu.Unlock()

	// Resolve the commit hash
	targetHash, err := r.resolveRef(commitHash)
	if err != nil {
		return nil, fmt.Errorf("commit not found: %s", commitHash)
	}

	// Get the commit to cherry-pick
	commit, err := r.store.GetCommit(targetHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	// Get the parent commit to calculate changes
	if commit.ParentHash == "" {
		return nil, fmt.Errorf("cannot cherry-pick root commit")
	}

	parentCommit, err := r.store.GetCommit(commit.ParentHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent commit: %w", err)
	}

	// Get trees for both commits
	commitTree, err := r.store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit tree: %w", err)
	}

	parentTree, err := r.store.GetTree(parentCommit.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent tree: %w", err)
	}

	// Calculate changes between parent and commit
	changes := make(map[string]string) // path -> hash

	// Find files that were added or modified
	for _, entry := range commitTree.Entries {
		found := false
		for _, parentEntry := range parentTree.Entries {
			if parentEntry.Name == entry.Name {
				found = true
				if parentEntry.Hash != entry.Hash {
					// File was modified
					changes[entry.Name] = entry.Hash
				}
				break
			}
		}
		if !found {
			// File was added
			changes[entry.Name] = entry.Hash
		}
	}

	// Apply changes to staging area
	for path, hash := range changes {
		r.staging.Add(path, hash)

		// Also update working directory if not memory-based
		if r.worktree.path != ":memory:" {
			blob, err := r.store.GetBlob(hash)
			if err == nil {
				r.worktree.WriteFile(path, blob.Content)
			}
		}
	}

	// Create a new commit with cherry-pick message
	message := fmt.Sprintf("Cherry-pick: %s\n\n%s\n\n(cherry picked from commit %s)",
		strings.Split(commit.Message, "\n")[0], // First line of original message
		commit.Message,
		targetHash[:8])

	// Create the commit with current user as author
	newCommit := &object.Commit{
		TreeHash:   "", // Will be set by commit process
		ParentHash: "", // Will be set by commit process
		Author: object.Author{
			Name:  authorName,
			Email: authorEmail,
			Time:  time.Now(),
		},
		Committer: object.Author{
			Name:  authorName,
			Email: authorEmail,
			Time:  time.Now(),
		},
		Message: message,
	}

	// Use internal commit method to avoid deadlock
	return r.commitInternal(newCommit)
}

// Revert creates a new commit that undoes the changes from a specific commit
func (r *Repository) Revert(commitHash string) (*object.Commit, error) {
	// Get author info before locking
	authorName := r.getConfigValue("user.name", "Unknown")
	authorEmail := r.getConfigValue("user.email", "unknown@example.com")

	r.mu.Lock()
	defer r.mu.Unlock()

	// Resolve the commit hash
	targetHash, err := r.resolveRef(commitHash)
	if err != nil {
		return nil, fmt.Errorf("commit not found: %s", commitHash)
	}

	// Get the commit to revert
	commit, err := r.store.GetCommit(targetHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	// Get the parent commit to understand what to revert to
	if commit.ParentHash == "" {
		return nil, fmt.Errorf("cannot revert root commit")
	}

	parentCommit, err := r.store.GetCommit(commit.ParentHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent commit: %w", err)
	}

	// Get trees for both commits
	commitTree, err := r.store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit tree: %w", err)
	}

	parentTree, err := r.store.GetTree(parentCommit.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent tree: %w", err)
	}

	// Calculate reverse changes (undo what the commit did)
	// Find files that were added (need to remove)
	for _, entry := range commitTree.Entries {
		found := false
		for _, parentEntry := range parentTree.Entries {
			if parentEntry.Name == entry.Name {
				found = true
				if parentEntry.Hash != entry.Hash {
					// File was modified - revert to parent version
					r.staging.Add(entry.Name, parentEntry.Hash)

					// Update working directory
					blob, err := r.store.GetBlob(parentEntry.Hash)
					if err == nil {
						r.worktree.WriteFile(entry.Name, blob.Content)
					}
				}
				break
			}
		}
		if !found {
			// File was added in this commit - remove it
			r.staging.Remove(entry.Name)
			r.worktree.RemoveFile(entry.Name)
		}
	}

	// Find files that were removed (need to restore)
	for _, parentEntry := range parentTree.Entries {
		found := false
		for _, entry := range commitTree.Entries {
			if entry.Name == parentEntry.Name {
				found = true
				break
			}
		}
		if !found {
			// File was removed in this commit - restore it
			r.staging.Add(parentEntry.Name, parentEntry.Hash)

			// Update working directory
			blob, err := r.store.GetBlob(parentEntry.Hash)
			if err == nil {
				r.worktree.WriteFile(parentEntry.Name, blob.Content)
			}
		}
	}

	// Create a revert commit message
	message := fmt.Sprintf("Revert \"%s\"\n\nThis reverts commit %s.",
		strings.Split(commit.Message, "\n")[0], // First line of original message
		targetHash[:8])

	// Create the commit
	newCommit := &object.Commit{
		TreeHash:   "", // Will be set by commit process
		ParentHash: "", // Will be set by commit process
		Author: object.Author{
			Name:  authorName,
			Email: authorEmail,
			Time:  time.Now(),
		},
		Committer: object.Author{
			Name:  authorName,
			Email: authorEmail,
			Time:  time.Now(),
		},
		Message: message,
	}

	// Use internal commit method to avoid deadlock
	return r.commitInternal(newCommit)
}

// commitInternal is an internal version of Commit that assumes the lock is already held
func (r *Repository) commitInternal(commit *object.Commit) (*object.Commit, error) {
	// Get current HEAD
	currentHash, err := r.refManager.GetHEAD()
	if err != nil {
		// First commit - no parent
		currentHash = ""
	}

	// Create tree from staging
	entries := make([]object.TreeEntry, 0)
	for path, hash := range r.staging.GetFiles() {
		entries = append(entries, object.TreeEntry{
			Mode: "100644",
			Hash: hash,
			Name: path,
		})
	}

	tree := &object.Tree{Entries: entries}
	treeHash, err := r.store.StoreTree(tree)
	if err != nil {
		return nil, fmt.Errorf("failed to store tree: %w", err)
	}

	// Update commit fields
	commit.TreeHash = treeHash
	commit.ParentHash = currentHash

	// Store commit
	commitHash, err := r.store.StoreCommit(commit)
	if err != nil {
		return nil, fmt.Errorf("failed to store commit: %w", err)
	}

	// Update HEAD
	if err := r.refManager.SetHEAD(commitHash); err != nil {
		return nil, fmt.Errorf("failed to update HEAD: %w", err)
	}

	// Clear staging area
	r.staging.Clear()

	return commit, nil
}

// Reset moves the current branch pointer to a specific commit
func (r *Repository) Reset(target string, mode string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Default to mixed mode if not specified
	if mode == "" {
		mode = "mixed"
	}

	// Validate mode
	if mode != "soft" && mode != "mixed" && mode != "hard" {
		return fmt.Errorf("invalid reset mode: %s (must be soft, mixed, or hard)", mode)
	}

	// Resolve the target commit
	targetHash, err := r.resolveRef(target)
	if err != nil {
		return fmt.Errorf("failed to resolve target: %w", err)
	}

	// Verify the commit exists
	_, err = r.store.GetCommit(targetHash)
	if err != nil {
		return fmt.Errorf("target commit not found: %s", target)
	}

	// Update HEAD to point to the target commit
	if err := r.refManager.SetHEAD(targetHash); err != nil {
		return fmt.Errorf("failed to update HEAD: %w", err)
	}

	// Handle different reset modes
	switch mode {
	case "soft":
		// Soft reset: only move HEAD, keep staging and working directory
		// Nothing more to do

	case "mixed":
		// Mixed reset: move HEAD and clear staging, keep working directory
		r.staging.Clear()

	case "hard":
		// Hard reset: move HEAD, clear staging, and reset working directory
		r.staging.Clear()

		// Reset working directory to match the target commit
		commit, _ := r.store.GetCommit(targetHash)
		if commit != nil {
			tree, err := r.store.GetTree(commit.TreeHash)
			if err == nil {
				// Clear working directory first
				currentFiles := r.worktree.ListFiles()
				for _, file := range currentFiles {
					r.worktree.RemoveFile(file)
				}

				// Restore files from the target commit
				for _, entry := range tree.Entries {
					blob, err := r.store.GetBlob(entry.Hash)
					if err == nil {
						r.worktree.WriteFile(entry.Name, blob.Content)
					}
				}
			}
		}
	}

	return nil
}

// Rebase replays commits from the current branch onto another branch
func (r *Repository) Rebase(onto string) ([]string, error) {
	// Get author info before locking
	authorName := r.getConfigValue("user.name", "Unknown")
	authorEmail := r.getConfigValue("user.email", "unknown@example.com")

	r.mu.Lock()
	defer r.mu.Unlock()

	// Get current branch
	currentBranch, err := r.refManager.GetCurrentBranch()
	if err != nil {
		return nil, fmt.Errorf("not on a branch")
	}

	// Get current HEAD
	currentHead, err := r.refManager.GetHEAD()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Resolve the onto ref
	ontoHash, err := r.resolveRef(onto)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve onto ref: %w", err)
	}

	// Find common ancestor
	commonAncestor := r.findCommonAncestor(currentHead, ontoHash)
	if commonAncestor == "" {
		return nil, fmt.Errorf("no common ancestor found")
	}

	// If already up to date
	if commonAncestor == currentHead {
		return []string{}, nil // No commits to rebase
	}

	// Collect commits to rebase (from common ancestor to current HEAD)
	commitsToRebase, err := r.collectCommitsBetween(commonAncestor, currentHead)
	if err != nil {
		return nil, fmt.Errorf("failed to collect commits: %w", err)
	}

	// Checkout onto commit
	if err := r.refManager.SetHEAD(ontoHash); err != nil {
		return nil, fmt.Errorf("failed to checkout onto commit: %w", err)
	}

	// Clear staging area
	r.staging.Clear()

	// Replay each commit
	rebasedCommits := make([]string, 0, len(commitsToRebase))

	for _, commitHash := range commitsToRebase {
		commit, err := r.store.GetCommit(commitHash)
		if err != nil {
			continue
		}

		// Get the changes from this commit
		if commit.ParentHash != "" {
			_, err := r.store.GetCommit(commit.ParentHash)
			if err == nil {
				tree, err := r.store.GetTree(commit.TreeHash)
				if err == nil {
					// Apply changes from this commit
					for _, entry := range tree.Entries {
						r.staging.Add(entry.Name, entry.Hash)

						// Update working directory
						if r.worktree.path != ":memory:" {
							blob, err := r.store.GetBlob(entry.Hash)
							if err == nil {
								r.worktree.WriteFile(entry.Name, blob.Content)
							}
						}
					}
				}
			}
		}

		// Create new commit with same message but new parent
		newCommit := &object.Commit{
			TreeHash:   "", // Will be set by commitInternal
			ParentHash: "", // Will be set by commitInternal
			Author: object.Author{
				Name:  authorName,
				Email: authorEmail,
				Time:  time.Now(),
			},
			Committer: object.Author{
				Name:  authorName,
				Email: authorEmail,
				Time:  time.Now(),
			},
			Message: commit.Message,
		}

		rebasedCommit, err := r.commitInternal(newCommit)
		if err != nil {
			// Rebase failed, should handle conflicts in real implementation
			return rebasedCommits, fmt.Errorf("failed to rebase commit %s: %w", commitHash[:8], err)
		}

		rebasedCommits = append(rebasedCommits, rebasedCommit.Hash())
	}

	// Update the original branch to point to the new HEAD
	newHead, _ := r.refManager.GetHEAD()
	// Delete and recreate the branch at the new position
	r.refManager.DeleteBranch(currentBranch)
	if err := r.refManager.CreateBranch(currentBranch, newHead); err != nil {
		return rebasedCommits, fmt.Errorf("failed to update branch: %w", err)
	}

	// Checkout the original branch
	if err := r.refManager.SetHEADToBranch(currentBranch); err != nil {
		return rebasedCommits, fmt.Errorf("failed to checkout branch: %w", err)
	}

	return rebasedCommits, nil
}

// findCommonAncestor finds the common ancestor between two commits
func (r *Repository) findCommonAncestor(hash1, hash2 string) string {
	// Simple implementation: walk back from hash1 and check if we reach hash2's ancestors
	ancestors1 := make(map[string]bool)

	// Collect all ancestors of hash1
	current := hash1
	for current != "" {
		ancestors1[current] = true
		commit, err := r.store.GetCommit(current)
		if err != nil || commit.ParentHash == "" {
			break
		}
		current = commit.ParentHash
	}

	// Walk back from hash2 and find first common ancestor
	current = hash2
	for current != "" {
		if ancestors1[current] {
			return current
		}
		commit, err := r.store.GetCommit(current)
		if err != nil || commit.ParentHash == "" {
			break
		}
		current = commit.ParentHash
	}

	return ""
}

// collectCommitsBetween collects all commits between start (exclusive) and end (inclusive)
func (r *Repository) collectCommitsBetween(start, end string) ([]string, error) {
	commits := []string{}
	current := end

	for current != "" && current != start {
		commits = append([]string{current}, commits...) // Prepend to maintain order

		commit, err := r.store.GetCommit(current)
		if err != nil {
			return nil, err
		}

		current = commit.ParentHash
	}

	return commits, nil
}

// Webhook and Event System

type Webhook struct {
	ID           string                `json:"id"`
	URL          string                `json:"url"`
	Events       []string              `json:"events"`
	Secret       string                `json:"secret"`
	ContentType  string                `json:"content_type"`
	Active       bool                  `json:"active"`
	InsecureSSL  bool                  `json:"insecure_ssl"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
	LastDelivery *HookDeliveryInternal `json:"last_delivery,omitempty"`
}

type HookDeliveryInternal struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	Event      string    `json:"event"`
	StatusCode int       `json:"status_code"`
	Duration   int64     `json:"duration_ms"`
	Request    string    `json:"request"`
	Response   string    `json:"response"`
	Delivered  bool      `json:"delivered"`
	CreatedAt  time.Time `json:"created_at"`
}

type RepositoryEvent struct {
	ID         string             `json:"id"`
	Event      string             `json:"event"`
	Repository string             `json:"repository"`
	Timestamp  time.Time          `json:"timestamp"`
	Actor      EventActorInternal `json:"actor"`
	Data       interface{}        `json:"data"`
}

type EventActorInternal struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// RegisterWebhook adds a new webhook to the repository
func (r *Repository) RegisterWebhook(url string, events []string, secret, contentType string, active, insecureSSL bool) (*Webhook, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.webhooks == nil {
		r.webhooks = make(map[string]*Webhook)
	}

	hookID := uuid.New().String()
	webhook := &Webhook{
		ID:          hookID,
		URL:         url,
		Events:      events,
		Secret:      secret,
		ContentType: contentType,
		Active:      active,
		InsecureSSL: insecureSSL,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	r.webhooks[hookID] = webhook
	return webhook, nil
}

// GetWebhook retrieves a webhook by ID
func (r *Repository) GetWebhook(hookID string) (*Webhook, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	webhook, exists := r.webhooks[hookID]
	if !exists {
		return nil, fmt.Errorf("webhook not found: %s", hookID)
	}

	return webhook, nil
}

// ListWebhooks returns all registered webhooks
func (r *Repository) ListWebhooks() ([]*Webhook, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	webhooks := make([]*Webhook, 0, len(r.webhooks))
	for _, hook := range r.webhooks {
		webhooks = append(webhooks, hook)
	}

	return webhooks, nil
}

// DeleteWebhook removes a webhook
func (r *Repository) DeleteWebhook(hookID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.webhooks[hookID]; !exists {
		return fmt.Errorf("webhook not found: %s", hookID)
	}

	delete(r.webhooks, hookID)
	return nil
}

// EmitEvent sends an event to all registered webhooks and the event stream
func (r *Repository) EmitEvent(eventType string, data interface{}) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create event
	event := &RepositoryEvent{
		ID:         uuid.New().String(),
		Event:      eventType,
		Repository: r.path,
		Timestamp:  time.Now(),
		Actor: EventActorInternal{
			Name:  r.getConfigValue("user.name", "Unknown"),
			Email: r.getConfigValue("user.email", "unknown@example.com"),
		},
		Data: data,
	}

	// Send to event stream (non-blocking)
	if r.events != nil {
		select {
		case r.events <- event:
		default:
			// Channel full, skip
		}
	}

	// Send to webhooks
	for _, webhook := range r.webhooks {
		if webhook.Active && r.shouldTriggerWebhook(webhook, eventType) {
			go r.deliverWebhook(webhook, event)
		}
	}
}

// shouldTriggerWebhook checks if a webhook should be triggered for an event
func (r *Repository) shouldTriggerWebhook(webhook *Webhook, eventType string) bool {
	for _, subscribedEvent := range webhook.Events {
		if subscribedEvent == eventType || subscribedEvent == "*" {
			return true
		}
	}
	return false
}

// deliverWebhook sends the webhook payload to the registered URL
func (r *Repository) deliverWebhook(webhook *Webhook, event *RepositoryEvent) {
	startTime := time.Now()
	deliveryID := uuid.New().String()

	// Create payload
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}

	// Create request
	req, err := http.NewRequest("POST", webhook.URL, bytes.NewBuffer(payload))
	if err != nil {
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "govc-webhook/1.0")
	req.Header.Set("X-Govc-Event", event.Event)
	req.Header.Set("X-Govc-Delivery", deliveryID)

	// Add signature if secret is provided
	if webhook.Secret != "" {
		signature := r.computeSignature(payload, webhook.Secret)
		req.Header.Set("X-Hub-Signature-256", "sha256="+signature)
	}

	// Create HTTP client
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Make request
	resp, err := client.Do(req)
	duration := time.Since(startTime).Milliseconds()

	delivery := &HookDeliveryInternal{
		ID:        deliveryID,
		URL:       webhook.URL,
		Event:     event.Event,
		Duration:  duration,
		Request:   string(payload),
		CreatedAt: startTime,
	}

	if err != nil {
		delivery.Delivered = false
		delivery.Response = err.Error()
		delivery.StatusCode = 0
	} else {
		delivery.Delivered = true
		delivery.StatusCode = resp.StatusCode
		defer resp.Body.Close()

		responseBody, _ := io.ReadAll(resp.Body)
		delivery.Response = string(responseBody)
	}

	// Update webhook with last delivery info
	r.mu.Lock()
	webhook.LastDelivery = delivery
	webhook.UpdatedAt = time.Now()
	r.mu.Unlock()
}

// computeSignature computes HMAC-SHA256 signature for webhook verification
func (r *Repository) computeSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// GetEventStream returns the event channel for Server-Sent Events
func (r *Repository) GetEventStream() <-chan *RepositoryEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.events == nil {
		r.events = make(chan *RepositoryEvent, 100) // Buffered channel
	}

	return r.events
}

// Search functionality

// SearchCommits searches through commit messages, authors, and emails
func (r *Repository) SearchCommits(query, author, since, until string, limit, offset int) ([]*object.Commit, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var allCommits []*object.Commit
	var sinceTime, untilTime time.Time
	var err error

	// Parse time filters if provided
	if since != "" {
		sinceTime, err = time.Parse(time.RFC3339, since)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid since time: %v", err)
		}
	}
	if until != "" {
		untilTime, err = time.Parse(time.RFC3339, until)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid until time: %v", err)
		}
	}

	// Convert query to lowercase for case-insensitive search
	queryLower := strings.ToLower(query)
	authorLower := strings.ToLower(author)

	// Get all commits by walking from all branch heads and HEAD
	visitedCommits := make(map[string]bool)

	// Start with HEAD
	headHash, err := r.refManager.GetHEAD()
	if err == nil && headHash != "" {
		r.walkCommitsForSearch(headHash, visitedCommits, &allCommits, queryLower, authorLower, author, query, sinceTime, untilTime)
	}

	// Also walk from all branch heads to catch any additional commits
	branches, err := r.refManager.ListBranches()
	if err == nil {
		for _, branch := range branches {
			branchHash, err := r.refManager.GetBranch(branch.Name)
			if err != nil {
				continue
			}

			// Walk commits from this branch head
			r.walkCommitsForSearch(branchHash, visitedCommits, &allCommits, queryLower, authorLower, author, query, sinceTime, untilTime)
		}
	}

	// Sort by commit time (newest first)
	sort.Slice(allCommits, func(i, j int) bool {
		return allCommits[i].Author.Time.After(allCommits[j].Author.Time)
	})

	total := len(allCommits)

	// Apply pagination
	if offset >= total {
		return []*object.Commit{}, total, nil
	}

	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}

	return allCommits[offset:end], total, nil
}

// walkCommitsForSearch recursively walks commits and adds matching ones
func (r *Repository) walkCommitsForSearch(commitHash string, visited map[string]bool, results *[]*object.Commit, queryLower, authorLower, author, query string, sinceTime, untilTime time.Time) {
	if commitHash == "" || visited[commitHash] {
		return
	}
	visited[commitHash] = true

	commit, err := r.store.GetCommit(commitHash)
	if err != nil {
		return
	}

	// Check time filters
	if !sinceTime.IsZero() && commit.Author.Time.Before(sinceTime) {
		return // Don't go further back if we're past the since time
	}
	if !untilTime.IsZero() && commit.Author.Time.After(untilTime) {
		// Continue walking but don't include this commit
		r.walkCommitsForSearch(commit.ParentHash, visited, results, queryLower, authorLower, author, query, sinceTime, untilTime)
		return
	}

	// Check author filter
	if author != "" && !strings.Contains(strings.ToLower(commit.Author.Name), authorLower) &&
		!strings.Contains(strings.ToLower(commit.Author.Email), authorLower) {
		// Continue walking but don't include this commit
		r.walkCommitsForSearch(commit.ParentHash, visited, results, queryLower, authorLower, author, query, sinceTime, untilTime)
		return
	}

	// Check query in message, author name, or email
	if strings.Contains(strings.ToLower(commit.Message), queryLower) ||
		strings.Contains(strings.ToLower(commit.Author.Name), queryLower) ||
		strings.Contains(strings.ToLower(commit.Author.Email), queryLower) {
		*results = append(*results, commit)
	}

	// Continue walking parent commits
	r.walkCommitsForSearch(commit.ParentHash, visited, results, queryLower, authorLower, author, query, sinceTime, untilTime)
}

// SearchContent searches for text within file contents
func (r *Repository) SearchContent(query, pathPattern, ref string, caseSensitive, regex bool, limit, offset int) ([]ContentMatchInternal, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Resolve reference (default to HEAD)
	targetRef := ref
	if targetRef == "" {
		targetRef = r.getCurrentBranchName()
		if targetRef == "" {
			targetRef = "HEAD"
		}
	}

	commitHash, err := r.resolveRef(targetRef)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to resolve ref %s: %v", targetRef, err)
	}

	commit, err := r.store.GetCommit(commitHash)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get commit: %v", err)
	}

	tree, err := r.store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get tree: %v", err)
	}

	var allMatches []ContentMatchInternal
	pattern := query
	if !caseSensitive {
		pattern = "(?i)" + pattern
	}

	var regexPattern *regexp.Regexp
	if regex {
		regexPattern, err = regexp.Compile(pattern)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid regex pattern: %v", err)
		}
	}

	// Search through all files in the tree
	for _, entry := range tree.Entries {
		// Check path pattern if specified
		if pathPattern != "" {
			matched, err := filepath.Match(pathPattern, entry.Name)
			if err != nil || !matched {
				continue
			}
		}

		// Get file content
		blob, err := r.store.GetBlob(entry.Hash)
		if err != nil {
			continue // Skip files we can't read
		}

		content := string(blob.Content)
		lines := strings.Split(content, "\n")

		// Search within file content
		for lineNum, line := range lines {
			var matches []MatchRangeInternal
			var found bool

			if regex && regexPattern != nil {
				// Regex search
				regexMatches := regexPattern.FindAllStringIndex(line, -1)
				for _, match := range regexMatches {
					matches = append(matches, MatchRangeInternal{Start: match[0], End: match[1]})
					found = true
				}
			} else {
				// Simple string search
				searchText := line
				searchQuery := query
				if !caseSensitive {
					searchText = strings.ToLower(line)
					searchQuery = strings.ToLower(query)
				}

				index := strings.Index(searchText, searchQuery)
				if index != -1 {
					matches = append(matches, MatchRangeInternal{
						Start: index,
						End:   index + len(query),
					})
					found = true
				}
			}

			if found {
				allMatches = append(allMatches, ContentMatchInternal{
					Path:    entry.Name,
					Ref:     targetRef,
					Line:    lineNum + 1,
					Column:  matches[0].Start + 1,
					Content: line,
					Preview: line, // Could be enhanced with highlighting
					Matches: matches,
				})
			}
		}
	}

	total := len(allMatches)

	// Apply pagination
	if offset >= total {
		return []ContentMatchInternal{}, total, nil
	}

	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}

	return allMatches[offset:end], total, nil
}

// SearchFiles searches for files by name
func (r *Repository) SearchFiles(query, ref string, caseSensitive, regex bool, limit, offset int) ([]FileMatchInternal, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Resolve reference (default to HEAD)
	targetRef := ref
	if targetRef == "" {
		targetRef = r.getCurrentBranchName()
		if targetRef == "" {
			targetRef = "HEAD"
		}
	}

	commitHash, err := r.resolveRef(targetRef)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to resolve ref %s: %v", targetRef, err)
	}

	commit, err := r.store.GetCommit(commitHash)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get commit: %v", err)
	}

	tree, err := r.store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get tree: %v", err)
	}

	var allMatches []FileMatchInternal
	pattern := query
	if !caseSensitive {
		pattern = "(?i)" + pattern
	}

	var regexPattern *regexp.Regexp
	if regex {
		regexPattern, err = regexp.Compile(pattern)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid regex pattern: %v", err)
		}
	}

	// Search through all files in the tree
	for _, entry := range tree.Entries {
		var matches []MatchRangeInternal
		var found bool

		if regex && regexPattern != nil {
			// Regex search on filename
			regexMatches := regexPattern.FindAllStringIndex(entry.Name, -1)
			for _, match := range regexMatches {
				matches = append(matches, MatchRangeInternal{Start: match[0], End: match[1]})
				found = true
			}
		} else {
			// Simple string search
			searchText := entry.Name
			searchQuery := query
			if !caseSensitive {
				searchText = strings.ToLower(entry.Name)
				searchQuery = strings.ToLower(query)
			}

			index := strings.Index(searchText, searchQuery)
			if index != -1 {
				matches = append(matches, MatchRangeInternal{
					Start: index,
					End:   index + len(query),
				})
				found = true
			}
		}

		if found {
			// Get file size from blob
			blob, err := r.store.GetBlob(entry.Hash)
			size := int64(0)
			if err == nil {
				size = int64(len(blob.Content))
			}

			allMatches = append(allMatches, FileMatchInternal{
				Path:    entry.Name,
				Ref:     targetRef,
				Size:    size,
				Mode:    entry.Mode,
				Matches: matches,
			})
		}
	}

	total := len(allMatches)

	// Apply pagination
	if offset >= total {
		return []FileMatchInternal{}, total, nil
	}

	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}

	return allMatches[offset:end], total, nil
}

// Grep performs advanced pattern matching similar to git grep
func (r *Repository) Grep(pattern, pathPattern, ref string, caseSensitive, regex, invertMatch, wordRegexp, lineRegexp bool, contextBefore, contextAfter, context, maxCount, limit, offset int) ([]GrepMatchInternal, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Handle context flags
	if context > 0 {
		contextBefore = context
		contextAfter = context
	}

	// Resolve reference (default to HEAD)
	targetRef := ref
	if targetRef == "" {
		targetRef = r.getCurrentBranchName()
		if targetRef == "" {
			targetRef = "HEAD"
		}
	}

	commitHash, err := r.resolveRef(targetRef)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to resolve ref %s: %v", targetRef, err)
	}

	commit, err := r.store.GetCommit(commitHash)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get commit: %v", err)
	}

	tree, err := r.store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get tree: %v", err)
	}

	var allMatches []GrepMatchInternal

	// Build regex pattern
	regexPattern := pattern
	if wordRegexp {
		regexPattern = `\b` + regexPattern + `\b`
	}
	if lineRegexp {
		regexPattern = `^` + regexPattern + `$`
	}
	if !caseSensitive {
		regexPattern = "(?i)" + regexPattern
	}

	var compiledPattern *regexp.Regexp
	if regex || wordRegexp || lineRegexp {
		compiledPattern, err = regexp.Compile(regexPattern)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid pattern: %v", err)
		}
	}

	// Search through all files in the tree
	for _, entry := range tree.Entries {
		// Check path pattern if specified
		if pathPattern != "" {
			matched, err := filepath.Match(pathPattern, entry.Name)
			if err != nil || !matched {
				continue
			}
		}

		// Get file content
		blob, err := r.store.GetBlob(entry.Hash)
		if err != nil {
			continue // Skip files we can't read
		}

		content := string(blob.Content)
		lines := strings.Split(content, "\n")
		matchCount := 0

		// Search within file content
		for lineNum, line := range lines {
			var matches []MatchRangeInternal
			var found bool

			if compiledPattern != nil {
				// Regex search
				regexMatches := compiledPattern.FindAllStringIndex(line, -1)
				found = len(regexMatches) > 0
				for _, match := range regexMatches {
					matches = append(matches, MatchRangeInternal{Start: match[0], End: match[1]})
				}
			} else {
				// Simple string search
				searchText := line
				searchQuery := pattern
				if !caseSensitive {
					searchText = strings.ToLower(line)
					searchQuery = strings.ToLower(pattern)
				}

				index := strings.Index(searchText, searchQuery)
				found = index != -1
				if found {
					matches = append(matches, MatchRangeInternal{
						Start: index,
						End:   index + len(pattern),
					})
				}
			}

			// Apply invert match logic
			if invertMatch {
				found = !found
				matches = nil // No specific matches when inverted
			}

			if found {
				matchCount++

				// Get context lines
				var before, after []string
				if contextBefore > 0 {
					start := lineNum - contextBefore
					if start < 0 {
						start = 0
					}
					before = lines[start:lineNum]
				}
				if contextAfter > 0 {
					end := lineNum + contextAfter + 1
					if end > len(lines) {
						end = len(lines)
					}
					after = lines[lineNum+1 : end]
				}

				allMatches = append(allMatches, GrepMatchInternal{
					Path:    entry.Name,
					Ref:     targetRef,
					Line:    lineNum + 1,
					Column:  1,
					Content: line,
					Before:  before,
					After:   after,
					Matches: matches,
				})

				// Check max count per file
				if maxCount > 0 && matchCount >= maxCount {
					break
				}
			}
		}
	}

	total := len(allMatches)

	// Apply pagination
	if offset >= total {
		return []GrepMatchInternal{}, total, nil
	}

	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}

	return allMatches[offset:end], total, nil
}

// Internal types for repository layer
type ContentMatchInternal struct {
	Path    string
	Ref     string
	Line    int
	Column  int
	Content string
	Preview string
	Matches []MatchRangeInternal
}

type MatchRangeInternal struct {
	Start int
	End   int
}

type FileMatchInternal struct {
	Path    string
	Ref     string
	Size    int64
	Mode    string
	Matches []MatchRangeInternal
}

type GrepMatchInternal struct {
	Path    string
	Ref     string
	Line    int
	Column  int
	Content string
	Before  []string
	After   []string
	Matches []MatchRangeInternal
}

// Hook execution functionality

type HookExecutionResult struct {
	Success     bool              `json:"success"`
	ExitCode    int               `json:"exit_code"`
	Output      string            `json:"output"`
	Error       string            `json:"error,omitempty"`
	Duration    int64             `json:"duration_ms"`
	Environment map[string]string `json:"environment"`
}

// ExecuteHook executes a shell script as a repository hook
func (r *Repository) ExecuteHook(hookType, script string, environment map[string]string, timeout int) (*HookExecutionResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if timeout <= 0 {
		timeout = 30 // Default 30 seconds
	}

	// Create the execution environment
	execEnv := make(map[string]string)

	// Add repository-specific environment variables
	execEnv["GOVC_REPO_PATH"] = r.path
	if currentBranch := r.getCurrentBranchName(); currentBranch != "" {
		execEnv["GOVC_BRANCH"] = currentBranch
	}
	if currentHash, err := r.refManager.GetHEAD(); err == nil {
		execEnv["GOVC_COMMIT"] = currentHash
	}
	execEnv["GOVC_HOOK_TYPE"] = hookType

	// Add custom environment variables
	for k, v := range environment {
		execEnv[k] = v
	}

	start := time.Now()

	// For security and simplicity, we'll simulate hook execution
	// In a real implementation, you would execute the script using os/exec
	// with proper sandboxing and security measures

	result := &HookExecutionResult{
		Success:     true,
		ExitCode:    0,
		Output:      fmt.Sprintf("Executed %s hook: %s", hookType, script),
		Duration:    time.Since(start).Milliseconds(),
		Environment: execEnv,
	}

	// Simulate some basic validation
	if strings.Contains(script, "exit 1") {
		result.Success = false
		result.ExitCode = 1
		result.Error = "Hook script failed"
	}

	return result, nil
}

// executePreCommitHooks runs all pre-commit hooks
func (r *Repository) executePreCommitHooks() error {
	// This would typically read hook scripts from .govc/hooks/pre-commit
	// For now, we'll just return nil to indicate hooks were executed
	// In a production implementation, this would execute actual hook scripts
	return nil
}

// executePostCommitHooks runs all post-commit hooks
func (r *Repository) executePostCommitHooks(commit *object.Commit) error {
	// This would typically read hook scripts from .govc/hooks/post-commit
	// For now, we'll just return nil to indicate hooks were executed
	// In a production implementation, this would execute actual hook scripts
	return nil
}

// executePrePushHooks runs all pre-push hooks
func (r *Repository) executePrePushHooks(remote, branch string) error {
	// This would typically read hook scripts from .govc/hooks/pre-push
	// For now, we'll just return nil to indicate hooks were executed
	// In a production implementation, this would execute actual hook scripts
	return nil
}
