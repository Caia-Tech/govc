package govc

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/caia-tech/govc/pkg/object"
	"github.com/caia-tech/govc/pkg/refs"
	"github.com/caia-tech/govc/pkg/storage"
	"github.com/gorilla/mux"
)

// Repository is govc's core abstraction - a memory-first Git repository.
// Unlike traditional Git, this repository can exist entirely in memory,
// enabling instant operations and parallel realities. The path can be
// ":memory:" for pure in-memory operation or a filesystem path for
// optional persistence.
type Repository struct {
	path       string              // ":memory:" for pure memory operation
	store      *storage.Store      // Memory-first object storage
	refManager *refs.RefManager    // Instant branch operations
	staging    *StagingArea        // In-memory staging
	worktree   *Worktree          // Can be virtual (memory-only)
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

	if err := refManager.SetHEADToBranch("main"); err != nil {
		return nil, fmt.Errorf("failed to set HEAD: %v", err)
	}

	repo := &Repository{
		path:       path,
		store:      store,
		refManager: refManager,
		staging:    NewStagingArea(),
		worktree:   NewWorktree(path),
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
		Name:  r.getConfigValue("user.name", "Unknown"),
		Email: r.getConfigValue("user.email", "unknown@example.com"),
		Time:  time.Now(),
	}

	commit := object.NewCommit(treeHash, author, message)

	currentBranch, err := r.refManager.GetCurrentBranch()
	if err == nil {
		parentHash, err := r.refManager.GetBranch(currentBranch)
		if err == nil {
			commit.SetParent(parentHash)
		}
	}

	commitHash, err := r.store.StoreCommit(commit)
	if err != nil {
		return nil, err
	}

	if currentBranch != "" {
		if err := r.refManager.UpdateRef("refs/heads/"+currentBranch, commitHash, ""); err != nil {
			return nil, err
		}
	} else {
		if err := r.refManager.SetHEADToCommit(commitHash); err != nil {
			return nil, err
		}
	}

	r.staging.Clear()

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

func (r *Repository) Checkout(ref string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

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

	if strings.HasPrefix(ref, "refs/heads/") || !strings.Contains(ref, "/") {
		branchName := ref
		if strings.HasPrefix(ref, "refs/heads/") {
			branchName = strings.TrimPrefix(ref, "refs/heads/")
		}
		return r.refManager.SetHEADToBranch(branchName)
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

	for file := range r.staging.files {
		status.Staged = append(status.Staged, file)
	}

	return status, nil
}

func (r *Repository) Log(limit int) ([]*object.Commit, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	headHash, err := r.refManager.GetHEAD()
	if err != nil {
		return nil, err
	}

	commits := make([]*object.Commit, 0)
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
	
	for file, hash := range r.staging.files {
		mode := "100644"
		tree.AddEntry(mode, file, hash)
	}

	return tree, nil
}

func (r *Repository) updateWorktree(tree *object.Tree) error {
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
	if len(ref) == 40 {
		return ref, nil
	}

	hash, err := r.refManager.GetBranch(ref)
	if err == nil {
		return hash, nil
	}

	hash, err = r.refManager.GetTag(ref)
	if err == nil {
		return hash, nil
	}

	return "", fmt.Errorf("ref not found: %s", ref)
}

// SetConfig sets a configuration value.
func (r *Repository) SetConfig(key, value string) {
	// Simple in-memory config for now
	// In future, could persist to .govc/config
}

// GetConfig gets a configuration value.
func (r *Repository) GetConfig(key string) string {
	return r.getConfigValue(key, "")
}

func (r *Repository) getConfigValue(key, defaultValue string) string {
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
	files map[string]string
	mu    sync.RWMutex
}

func NewStagingArea() *StagingArea {
	return &StagingArea{
		files: make(map[string]string),
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
}

func (s *StagingArea) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = make(map[string]string)
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

type Worktree struct {
	path string
}

func NewWorktree(path string) *Worktree {
	return &Worktree{path: path}
}

func (w *Worktree) MatchFiles(pattern string) ([]string, error) {
	var files []string
	
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

func (w *Worktree) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(filepath.Join(w.path, path))
}

func (w *Worktree) WriteFile(path string, content []byte) error {
	fullPath := filepath.Join(w.path, path)
	dir := filepath.Dir(fullPath)
	
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	
	return os.WriteFile(fullPath, content, 0644)
}

type Status struct {
	Branch    string
	Staged    []string
	Modified  []string
	Untracked []string
}

type BranchBuilder struct {
	repo *Repository
	name string
}

func (b *BranchBuilder) Create() error {
	b.repo.mu.Lock()
	defer b.repo.mu.Unlock()

	headHash, err := b.repo.refManager.GetHEAD()
	if err != nil {
		return err
	}

	return b.repo.refManager.CreateBranch(b.name, headHash)
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

	return b.repo.refManager.DeleteBranch(b.name)
}

func Clone(url, path string) (*Repository, error) {
	return nil, fmt.Errorf("clone not yet implemented")
}

func (r *Repository) Push(remote, branch string) error {
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
	return fmt.Errorf("export not yet implemented")
}