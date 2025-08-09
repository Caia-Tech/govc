// Package sqlite implements a SQLite-backed datastore for govc.
// This provides persistent storage with good performance for small to medium deployments.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caiatech/govc/datastore"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// SQLiteStore implements datastore.DataStore using SQLite
type SQLiteStore struct {
	db       *sql.DB
	path     string
	config   datastore.Config
	
	// Prepared statements cache
	stmts    map[string]*sql.Stmt
	stmtsMu  sync.RWMutex
	
	// Metrics
	reads    atomic.Int64
	writes   atomic.Int64
	deletes  atomic.Int64
	
	// Lifecycle
	startTime time.Time
	closed    atomic.Bool
}

// init registers the SQLite store factory
func init() {
	datastore.Register(datastore.TypeSQLite, func(config datastore.Config) (datastore.DataStore, error) {
		return New(config)
	})
}

// New creates a new SQLite store
func New(config datastore.Config) (*SQLiteStore, error) {
	store := &SQLiteStore{
		path:      config.Connection,
		config:    config,
		stmts:     make(map[string]*sql.Stmt),
		startTime: time.Now(),
	}
	
	return store, nil
}

// Initialize initializes the SQLite store
func (s *SQLiteStore) Initialize(config datastore.Config) error {
	if s.db != nil {
		return nil // Already initialized
	}
	
	// Build connection string with pragmas
	connStr := s.path
	if !strings.Contains(connStr, "?") {
		connStr += "?"
	} else {
		connStr += "&"
	}
	
	// Add pragmas for performance
	pragmas := []string{
		"_journal_mode=" + config.GetStringOption("journal_mode", "WAL"),
		"_synchronous=" + config.GetStringOption("synchronous", "NORMAL"),
		"_cache_size=" + fmt.Sprintf("%d", config.GetIntOption("cache_size", 10000)),
		"_foreign_keys=" + fmt.Sprintf("%t", config.GetBoolOption("foreign_keys", true)),
		"_busy_timeout=" + fmt.Sprintf("%d", config.GetIntOption("busy_timeout", 5000)),
	}
	connStr += strings.Join(pragmas, "&")
	
	// Open database
	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	
	// Set connection pool settings
	db.SetMaxOpenConns(config.MaxConnections)
	if config.MaxConnections == 0 {
		db.SetMaxOpenConns(1) // SQLite works best with single connection in WAL mode
	}
	db.SetMaxIdleConns(config.MaxIdleConnections)
	db.SetConnMaxLifetime(config.ConnectionTimeout)
	
	s.db = db
	
	// Run migrations
	if err := Migrate(db); err != nil {
		db.Close()
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	
	// Prepare common statements
	if err := s.prepareStatements(); err != nil {
		db.Close()
		return fmt.Errorf("failed to prepare statements: %w", err)
	}
	
	return nil
}

// prepareStatements prepares commonly used SQL statements
func (s *SQLiteStore) prepareStatements() error {
	statements := map[string]string{
		"getObject":    "SELECT data FROM objects WHERE hash = ?",
		"putObject":    "INSERT OR REPLACE INTO objects (hash, type, size, data) VALUES (?, ?, ?, ?)",
		"deleteObject": "DELETE FROM objects WHERE hash = ?",
		"hasObject":    "SELECT 1 FROM objects WHERE hash = ? LIMIT 1",
		
		"getRepo":    "SELECT * FROM repositories WHERE id = ?",
		"saveRepo":   "INSERT OR REPLACE INTO repositories (id, name, description, path, is_private, metadata, size, commit_count, branch_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"deleteRepo": "DELETE FROM repositories WHERE id = ?",
		
		"getUser":       "SELECT * FROM users WHERE id = ?",
		"getUserByName": "SELECT * FROM users WHERE username = ?",
		"saveUser":      "INSERT OR REPLACE INTO users (id, username, email, full_name, is_active, is_admin, metadata, created_at, updated_at, last_login_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"deleteUser":    "DELETE FROM users WHERE id = ?",
		
		"getRef":    "SELECT * FROM refs WHERE repository_id = ? AND name = ?",
		"saveRef":   "INSERT OR REPLACE INTO refs (repository_id, name, hash, type, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?)",
		"deleteRef": "DELETE FROM refs WHERE repository_id = ? AND name = ?",
		
		"logEvent": "INSERT INTO audit_events (id, timestamp, user_id, username, action, resource, resource_id, details, ip_address, user_agent, success, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		
		"getConfig": "SELECT value FROM config WHERE key = ?",
		"setConfig": "INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)",
		"delConfig": "DELETE FROM config WHERE key = ?",
	}
	
	for name, query := range statements {
		stmt, err := s.db.Prepare(query)
		if err != nil {
			return fmt.Errorf("failed to prepare statement %s: %w", name, err)
		}
		s.stmts[name] = stmt
	}
	
	return nil
}

// getStmt retrieves a prepared statement
func (s *SQLiteStore) getStmt(name string) *sql.Stmt {
	s.stmtsMu.RLock()
	defer s.stmtsMu.RUnlock()
	return s.stmts[name]
}

// Close closes the SQLite store
func (s *SQLiteStore) Close() error {
	if s.closed.Swap(true) {
		return fmt.Errorf("already closed")
	}
	
	// Close all prepared statements
	s.stmtsMu.Lock()
	for _, stmt := range s.stmts {
		stmt.Close()
	}
	s.stmts = nil
	s.stmtsMu.Unlock()
	
	// Close database
	if s.db != nil {
		return s.db.Close()
	}
	
	return nil
}

// HealthCheck checks if the store is healthy
func (s *SQLiteStore) HealthCheck(ctx context.Context) error {
	if s.closed.Load() {
		return fmt.Errorf("store is closed")
	}
	
	// Ping database
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	
	// Check if we can query
	var version int
	err := s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_version").Scan(&version)
	if err != nil {
		return fmt.Errorf("schema check failed: %w", err)
	}
	
	return nil
}

// Type returns the store type
func (s *SQLiteStore) Type() string {
	return datastore.TypeSQLite
}

// Info returns store information
func (s *SQLiteStore) Info() map[string]interface{} {
	stats := s.db.Stats()
	
	var objectCount, repoCount, userCount int64
	s.db.QueryRow("SELECT COUNT(*) FROM objects").Scan(&objectCount)
	s.db.QueryRow("SELECT COUNT(*) FROM repositories").Scan(&repoCount)
	s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	
	// Get database file size
	var pageCount, pageSize int64
	s.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	s.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	dbSize := pageCount * pageSize
	
	return map[string]interface{}{
		"type":              datastore.TypeSQLite,
		"path":              s.path,
		"objects":           objectCount,
		"repositories":      repoCount,
		"users":             userCount,
		"database_size":     dbSize,
		"open_connections":  stats.OpenConnections,
		"in_use":           stats.InUse,
		"idle":             stats.Idle,
		"wait_count":       stats.WaitCount,
		"wait_duration":    stats.WaitDuration,
		"max_idle_closed":  stats.MaxIdleClosed,
		"max_lifetime_closed": stats.MaxLifetimeClosed,
		"uptime":           time.Since(s.startTime),
	}
}

// ObjectStore returns the object store interface
func (s *SQLiteStore) ObjectStore() datastore.ObjectStore {
	return s
}

// MetadataStore returns the metadata store interface
func (s *SQLiteStore) MetadataStore() datastore.MetadataStore {
	return s
}

// BeginTx begins a transaction
func (s *SQLiteStore) BeginTx(ctx context.Context, opts *datastore.TxOptions) (datastore.Transaction, error) {
	sqlOpts := &sql.TxOptions{
		ReadOnly: opts != nil && opts.ReadOnly,
	}
	
	// Map isolation levels
	if opts != nil {
		switch opts.Isolation {
		case datastore.IsolationReadUncommitted:
			sqlOpts.Isolation = sql.LevelReadUncommitted
		case datastore.IsolationReadCommitted:
			sqlOpts.Isolation = sql.LevelReadCommitted
		case datastore.IsolationRepeatableRead:
			sqlOpts.Isolation = sql.LevelRepeatableRead
		case datastore.IsolationSerializable:
			sqlOpts.Isolation = sql.LevelSerializable
		default:
			sqlOpts.Isolation = sql.LevelDefault
		}
	}
	
	tx, err := s.db.BeginTx(ctx, sqlOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	return &sqliteTransaction{
		store: s,
		tx:    tx,
		ctx:   ctx,
	}, nil
}

// GetMetrics returns store metrics
func (s *SQLiteStore) GetMetrics() datastore.Metrics {
	stats := s.db.Stats()
	uptime := time.Since(s.startTime)
	
	var objectCount int64
	s.db.QueryRow("SELECT COUNT(*) FROM objects").Scan(&objectCount)
	
	var dbSize int64
	var pageCount, pageSize int64
	s.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	s.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	dbSize = pageCount * pageSize
	
	return datastore.Metrics{
		Reads:             s.reads.Load(),
		Writes:            s.writes.Load(),
		Deletes:           s.deletes.Load(),
		ObjectCount:       objectCount,
		StorageSize:       dbSize,
		ActiveConnections: stats.InUse,
		TotalConnections:  int64(stats.OpenConnections),
		StartTime:         s.startTime,
		Uptime:           uptime,
	}
}

// ObjectStore implementation

// GetObject retrieves an object by hash
func (s *SQLiteStore) GetObject(hash string) ([]byte, error) {
	s.reads.Add(1)
	
	var data []byte
	err := s.getStmt("getObject").QueryRow(hash).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, datastore.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	
	return data, nil
}

// PutObject stores an object
func (s *SQLiteStore) PutObject(hash string, data []byte) error {
	s.writes.Add(1)
	
	// Determine object type (simplified - in production, parse the object)
	objType := "blob"
	
	_, err := s.getStmt("putObject").Exec(hash, objType, len(data), data)
	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}
	
	return nil
}

// DeleteObject deletes an object
func (s *SQLiteStore) DeleteObject(hash string) error {
	s.deletes.Add(1)
	
	result, err := s.getStmt("deleteObject").Exec(hash)
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	
	if rows == 0 {
		return datastore.ErrNotFound
	}
	
	return nil
}

// HasObject checks if an object exists
func (s *SQLiteStore) HasObject(hash string) (bool, error) {
	s.reads.Add(1)
	
	var exists int
	err := s.getStmt("hasObject").QueryRow(hash).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check object: %w", err)
	}
	
	return exists == 1, nil
}

// Additional ObjectStore methods continue...
// ListObjects, IterateObjects, GetObjects, PutObjects, DeleteObjects, etc.

// Helper function to marshal JSON
func marshalJSON(v interface{}) string {
	if v == nil {
		return "{}"
	}
	data, _ := json.Marshal(v)
	return string(data)
}

// Helper function to unmarshal JSON
func unmarshalJSON(data string, v interface{}) error {
	if data == "" {
		return nil
	}
	return json.Unmarshal([]byte(data), v)
}