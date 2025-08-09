// Package datastore provides a unified interface for multiple database backends
// supporting Git object storage, metadata management, and transactional operations.
package datastore

import (
	"context"
	"errors"
	"time"
)

// Common errors
var (
	ErrNotFound         = errors.New("not found")
	ErrAlreadyExists    = errors.New("already exists")
	ErrInvalidData      = errors.New("invalid data")
	ErrTransactionFailed = errors.New("transaction failed")
	ErrNotImplemented   = errors.New("not implemented")
	ErrConnectionFailed = errors.New("connection failed")
)

// DataStore is the main interface that all database implementations must satisfy.
// It combines object storage, metadata management, and transaction support.
type DataStore interface {
	// Lifecycle management
	Initialize(config Config) error
	Close() error
	HealthCheck(ctx context.Context) error
	
	// Get specific stores
	ObjectStore() ObjectStore
	MetadataStore() MetadataStore
	
	// Transaction support
	BeginTx(ctx context.Context, opts *TxOptions) (Transaction, error)
	
	// Metrics and monitoring
	GetMetrics() Metrics
	
	// Backend information
	Type() string
	Info() map[string]interface{}
}

// ObjectStore handles Git object storage (blobs, trees, commits, tags)
type ObjectStore interface {
	// Basic operations
	GetObject(hash string) ([]byte, error)
	PutObject(hash string, data []byte) error
	DeleteObject(hash string) error
	HasObject(hash string) (bool, error)
	
	// Listing and iteration
	ListObjects(prefix string, limit int) ([]string, error)
	IterateObjects(prefix string, fn func(hash string, data []byte) error) error
	
	// Batch operations for performance
	GetObjects(hashes []string) (map[string][]byte, error)
	PutObjects(objects map[string][]byte) error
	DeleteObjects(hashes []string) error
	
	// Size and statistics
	GetObjectSize(hash string) (int64, error)
	CountObjects() (int64, error)
	GetStorageSize() (int64, error)
}

// MetadataStore handles repository metadata, users, configuration, etc.
type MetadataStore interface {
	// Repository management
	SaveRepository(repo *Repository) error
	GetRepository(id string) (*Repository, error)
	ListRepositories(filter RepositoryFilter) ([]*Repository, error)
	DeleteRepository(id string) error
	UpdateRepository(id string, updates map[string]interface{}) error
	
	// User management
	SaveUser(user *User) error
	GetUser(id string) (*User, error)
	GetUserByUsername(username string) (*User, error)
	ListUsers(filter UserFilter) ([]*User, error)
	DeleteUser(id string) error
	UpdateUser(id string, updates map[string]interface{}) error
	
	// Reference management (branches, tags)
	SaveRef(repoID string, ref *Reference) error
	GetRef(repoID string, name string) (*Reference, error)
	ListRefs(repoID string, refType RefType) ([]*Reference, error)
	DeleteRef(repoID string, name string) error
	UpdateRef(repoID string, name string, newHash string) error
	
	// Audit logging
	LogEvent(event *AuditEvent) error
	QueryEvents(filter EventFilter) ([]*AuditEvent, error)
	CountEvents(filter EventFilter) (int64, error)
	
	// Configuration
	GetConfig(key string) (string, error)
	SetConfig(key string, value string) error
	GetAllConfig() (map[string]string, error)
	DeleteConfig(key string) error
}

// Transaction represents a database transaction with all operations
type Transaction interface {
	// Transaction control
	Commit() error
	Rollback() error
	
	// Include all ObjectStore operations
	ObjectStore
	
	// Include all MetadataStore operations
	MetadataStore
}

// TxOptions configures transaction behavior
type TxOptions struct {
	Isolation IsolationLevel
	ReadOnly  bool
	Timeout   time.Duration
}

// IsolationLevel defines transaction isolation levels
type IsolationLevel int

const (
	IsolationDefault IsolationLevel = iota
	IsolationReadUncommitted
	IsolationReadCommitted
	IsolationRepeatableRead
	IsolationSerializable
)

// Repository represents a Git repository's metadata
type Repository struct {
	ID          string                 `json:"id" db:"id"`
	Name        string                 `json:"name" db:"name"`
	Description string                 `json:"description" db:"description"`
	Path        string                 `json:"path" db:"path"`
	IsPrivate   bool                   `json:"is_private" db:"is_private"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata" db:"metadata"`
	
	// Statistics
	Size        int64 `json:"size" db:"size"`
	CommitCount int64 `json:"commit_count" db:"commit_count"`
	BranchCount int   `json:"branch_count" db:"branch_count"`
}

// User represents a system user
type User struct {
	ID          string                 `json:"id" db:"id"`
	Username    string                 `json:"username" db:"username"`
	Email       string                 `json:"email" db:"email"`
	FullName    string                 `json:"full_name" db:"full_name"`
	IsActive    bool                   `json:"is_active" db:"is_active"`
	IsAdmin     bool                   `json:"is_admin" db:"is_admin"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
	LastLoginAt *time.Time             `json:"last_login_at" db:"last_login_at"`
	Metadata    map[string]interface{} `json:"metadata" db:"metadata"`
}

// Reference represents a Git reference (branch or tag)
type Reference struct {
	Name      string    `json:"name" db:"name"`
	Hash      string    `json:"hash" db:"hash"`
	Type      RefType   `json:"type" db:"type"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	UpdatedBy string    `json:"updated_by" db:"updated_by"`
}

// RefType defines the type of reference
type RefType string

const (
	RefTypeBranch RefType = "branch"
	RefTypeTag    RefType = "tag"
)

// AuditEvent represents an auditable action
type AuditEvent struct {
	ID         string                 `json:"id" db:"id"`
	Timestamp  time.Time              `json:"timestamp" db:"timestamp"`
	UserID     string                 `json:"user_id" db:"user_id"`
	Username   string                 `json:"username" db:"username"`
	Action     string                 `json:"action" db:"action"`
	Resource   string                 `json:"resource" db:"resource"`
	ResourceID string                 `json:"resource_id" db:"resource_id"`
	Details    map[string]interface{} `json:"details" db:"details"`
	IPAddress  string                 `json:"ip_address" db:"ip_address"`
	UserAgent  string                 `json:"user_agent" db:"user_agent"`
	Success    bool                   `json:"success" db:"success"`
	ErrorMsg   string                 `json:"error_msg,omitempty" db:"error_msg"`
}

// Filters for querying

// RepositoryFilter filters repository queries
type RepositoryFilter struct {
	IDs       []string
	Name      string
	IsPrivate *bool
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Limit     int
	Offset    int
	OrderBy   string
}

// UserFilter filters user queries
type UserFilter struct {
	IDs      []string
	Username string
	Email    string
	IsActive *bool
	IsAdmin  *bool
	Limit    int
	Offset   int
	OrderBy  string
}

// EventFilter filters audit event queries
type EventFilter struct {
	UserID     string
	Action     string
	Resource   string
	ResourceID string
	After      *time.Time
	Before     *time.Time
	Success    *bool
	Limit      int
	Offset     int
	OrderBy    string
}

// Metrics provides performance and usage metrics
type Metrics struct {
	// Operation counts
	Reads   int64 `json:"reads"`
	Writes  int64 `json:"writes"`
	Deletes int64 `json:"deletes"`
	
	// Performance metrics
	AvgReadLatency  time.Duration `json:"avg_read_latency"`
	AvgWriteLatency time.Duration `json:"avg_write_latency"`
	
	// Storage metrics
	ObjectCount  int64 `json:"object_count"`
	StorageSize  int64 `json:"storage_size"`
	
	// Connection metrics
	ActiveConnections int `json:"active_connections"`
	TotalConnections  int64 `json:"total_connections"`
	
	// Error metrics
	ErrorCount int64 `json:"error_count"`
	LastError  string `json:"last_error,omitempty"`
	
	// Uptime
	StartTime time.Time `json:"start_time"`
	Uptime    time.Duration `json:"uptime"`
}

// Iterator provides a way to iterate over results
type Iterator interface {
	Next() bool
	Key() string
	Value() []byte
	Error() error
	Close() error
}

// Batch represents a batch of operations
type Batch interface {
	Put(key string, value []byte)
	Delete(key string)
	Clear()
	Size() int
	Execute() error
}