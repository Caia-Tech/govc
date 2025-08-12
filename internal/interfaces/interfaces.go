package interfaces

import (
	"github.com/Caia-Tech/govc/pkg/object"
	"github.com/Caia-Tech/govc/pkg/storage"
	"github.com/Caia-Tech/govc/pkg/refs"
)

// Repository defines the interface for repository operations
// This breaks circular dependencies between internal packages
type Repository interface {
	GetObject(hash string) (object.Object, error)
	GetCommit(hash string) (*object.Commit, error)
	GetTree(hash string) (*object.Tree, error)
	GetBlob(hash string) ([]byte, error)
	GetBlobWithDelta(hash string) (*object.Blob, error)
	GetCurrentBranch() (string, error)
	ListBranches() ([]string, error)
	ListCommits(branch string, limit int) ([]*object.Commit, error)
	GetStore() *storage.Store
	FindFiles(pattern string) ([]string, error)
	GetRefManager() RefManager
	GetStagingArea() StagingArea
	Log(limit int) ([]*object.Commit, error)
}

// RefManager interface for reference operations
type RefManager interface {
	ListRefs() ([]refs.Ref, error)
	GetRef(name string) (*refs.Ref, error)
	GetHEAD() (string, error)
}

// StagingArea interface for staging operations  
type StagingArea interface {
	List() ([]string, error)
	IsStaged(path string) bool
	GetFileHash(path string) (string, error)
	Add(path string, content []byte) error
	Clear()
}