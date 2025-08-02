package core

import (
	"fmt"
	"sort"
	"strings"

	"github.com/caiatech/govc/pkg/object"
)

// CleanRepository represents the core immutable Git repository
// It only handles objects and references - no mutable state, no config, no webhooks
type CleanRepository struct {
	objects ObjectStore
	refs    RefStore
}

// NewCleanRepository creates a new clean repository
func NewCleanRepository(objects ObjectStore, refs RefStore) *CleanRepository {
	return &CleanRepository{
		objects: objects,
		refs:    refs,
	}
}

// GetObject retrieves any Git object by hash
func (r *CleanRepository) GetObject(hash string) (object.Object, error) {
	return r.objects.Get(hash)
}

// GetCommit retrieves a commit by hash
func (r *CleanRepository) GetCommit(hash string) (*object.Commit, error) {
	obj, err := r.objects.Get(hash)
	if err != nil {
		return nil, err
	}
	
	commit, ok := obj.(*object.Commit)
	if !ok {
		return nil, fmt.Errorf("object %s is not a commit", hash)
	}
	
	return commit, nil
}

// GetTree retrieves a tree by hash
func (r *CleanRepository) GetTree(hash string) (*object.Tree, error) {
	obj, err := r.objects.Get(hash)
	if err != nil {
		return nil, err
	}
	
	tree, ok := obj.(*object.Tree)
	if !ok {
		return nil, fmt.Errorf("object %s is not a tree", hash)
	}
	
	return tree, nil
}

// GetBlob retrieves a blob by hash
func (r *CleanRepository) GetBlob(hash string) (*object.Blob, error) {
	obj, err := r.objects.Get(hash)
	if err != nil {
		return nil, err
	}
	
	blob, ok := obj.(*object.Blob)
	if !ok {
		return nil, fmt.Errorf("object %s is not a blob", hash)
	}
	
	return blob, nil
}

// ListBranches returns all branches
func (r *CleanRepository) ListBranches() ([]Branch, error) {
	refs, err := r.refs.ListRefs()
	if err != nil {
		return nil, err
	}
	
	var branches []Branch
	for name, hash := range refs {
		if strings.HasPrefix(name, "refs/heads/") {
			branchName := strings.TrimPrefix(name, "refs/heads/")
			branches = append(branches, Branch{
				Name: branchName,
				Hash: hash,
			})
		}
	}
	
	// Sort for consistent output
	sort.Slice(branches, func(i, j int) bool {
		return branches[i].Name < branches[j].Name
	})
	
	return branches, nil
}

// GetBranch retrieves a specific branch
func (r *CleanRepository) GetBranch(name string) (string, error) {
	return r.refs.GetRef("refs/heads/" + name)
}

// ListTags returns all tags
func (r *CleanRepository) ListTags() ([]Tag, error) {
	refs, err := r.refs.ListRefs()
	if err != nil {
		return nil, err
	}
	
	var tags []Tag
	for name, hash := range refs {
		if strings.HasPrefix(name, "refs/tags/") {
			tagName := strings.TrimPrefix(name, "refs/tags/")
			tags = append(tags, Tag{
				Name: tagName,
				Hash: hash,
			})
		}
	}
	
	// Sort for consistent output
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Name < tags[j].Name
	})
	
	return tags, nil
}

// GetTag retrieves a specific tag
func (r *CleanRepository) GetTag(name string) (string, error) {
	return r.refs.GetRef("refs/tags/" + name)
}

// GetHEAD returns what HEAD points to
func (r *CleanRepository) GetHEAD() (string, error) {
	return r.refs.GetHEAD()
}

// ResolveRef resolves a reference to a commit hash
func (r *CleanRepository) ResolveRef(ref string) (string, error) {
	// Try as direct hash first
	if r.objects.Exists(ref) {
		return ref, nil
	}
	
	// Try as branch
	if hash, err := r.GetBranch(ref); err == nil {
		return hash, nil
	}
	
	// Try as tag
	if hash, err := r.GetTag(ref); err == nil {
		return hash, nil
	}
	
	// Try as full ref
	if hash, err := r.refs.GetRef(ref); err == nil {
		return hash, nil
	}
	
	// Try HEAD
	if ref == "HEAD" {
		return r.GetHEAD()
	}
	
	return "", fmt.Errorf("reference not found: %s", ref)
}

// Log returns commit history starting from a given commit
func (r *CleanRepository) Log(startHash string, limit int) ([]*object.Commit, error) {
	var commits []*object.Commit
	visited := make(map[string]bool)
	
	var walk func(hash string) error
	walk = func(hash string) error {
		if limit > 0 && len(commits) >= limit {
			return nil
		}
		
		if visited[hash] {
			return nil
		}
		visited[hash] = true
		
		commit, err := r.GetCommit(hash)
		if err != nil {
			return err
		}
		
		commits = append(commits, commit)
		
		// Walk parent
		if commit.ParentHash != "" {
			return walk(commit.ParentHash)
		}
		
		return nil
	}
	
	if err := walk(startHash); err != nil {
		return nil, err
	}
	
	return commits, nil
}

// Branch represents a Git branch
type Branch struct {
	Name string
	Hash string
}

// Tag represents a Git tag
type Tag struct {
	Name string
	Hash string
}