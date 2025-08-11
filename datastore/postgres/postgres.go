// Package postgres implements a PostgreSQL-backed datastore for govc.
// This is designed for enterprise deployments with high availability,
// scalability, and advanced features like partitioning and replication.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Caia-Tech/govc/datastore"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgresStore implements datastore.DataStore using PostgreSQL
type PostgresStore struct {
	db       *sql.DB
	config   datastore.Config
	
	// Prepared statements cache
	stmts    map[string]*sql.Stmt
	stmtsMu  sync.RWMutex
	
	// Connection pool monitoring
	poolStats struct {
		activeConns   atomic.Int32
		idleConns     atomic.Int32
		waitCount     atomic.Int64
		waitDuration  atomic.Int64
	}
	
	// Metrics
	reads    atomic.Int64
	writes   atomic.Int64
	deletes  atomic.Int64
	
	// Lifecycle
	startTime time.Time
	closed    atomic.Bool
}

// init registers the PostgreSQL store factory
func init() {
	datastore.Register(datastore.TypePostgres, func(config datastore.Config) (datastore.DataStore, error) {
		return New(config)
	})
}

// New creates a new PostgreSQL store
func New(config datastore.Config) (*PostgresStore, error) {
	store := &PostgresStore{
		config:    config,
		stmts:     make(map[string]*sql.Stmt),
		startTime: time.Now(),
	}
	
	return store, nil
}

// Initialize initializes the PostgreSQL store
func (s *PostgresStore) Initialize(config datastore.Config) error {
	if s.db != nil {
		return nil // Already initialized
	}
	
	// Parse connection string and add options
	connStr := config.Connection
	if !strings.Contains(connStr, "sslmode=") {
		if strings.Contains(connStr, "?") {
			connStr += "&sslmode=prefer"
		} else {
			connStr += "?sslmode=prefer"
		}
	}
	
	// Add application name
	if !strings.Contains(connStr, "application_name=") {
		if strings.Contains(connStr, "?") {
			connStr += "&application_name=govc"
		} else {
			connStr += "?application_name=govc"
		}
	}
	
	// Open database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	
	// Configure connection pool
	if config.MaxConnections > 0 {
		db.SetMaxOpenConns(config.MaxConnections)
	} else {
		db.SetMaxOpenConns(25) // Default for PostgreSQL
	}
	
	if config.MaxIdleConnections > 0 {
		db.SetMaxIdleConns(config.MaxIdleConnections)
	} else {
		db.SetMaxIdleConns(5)
	}
	
	if config.ConnectionTimeout > 0 {
		db.SetConnMaxLifetime(config.ConnectionTimeout)
	} else {
		db.SetConnMaxLifetime(time.Hour)
	}
	
	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}
	
	s.db = db
	
	// Run migrations
	if err := s.runMigrations(); err != nil {
		db.Close()
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	
	// Prepare common statements
	if err := s.prepareStatements(); err != nil {
		db.Close()
		return fmt.Errorf("failed to prepare statements: %w", err)
	}
	
	// Start background tasks
	go s.monitorPool()
	go s.refreshStats()
	
	return nil
}

// runMigrations ensures the database schema is up to date
func (s *PostgresStore) runMigrations() error {
	// Check if govc schema exists, create if not
	var schemaExists bool
	err := s.db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.schemata 
			WHERE schema_name = 'govc'
		)
	`).Scan(&schemaExists)
	if err != nil {
		return fmt.Errorf("failed to check schema: %w", err)
	}
	
	if !schemaExists {
		// Read and execute migrations.sql
		// In production, you would read from embedded file or migration tool
		// For now, we'll create basic schema inline
		if err := s.createBasicSchema(); err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}
	}
	
	return nil
}

// createBasicSchema creates the essential tables
func (s *PostgresStore) createBasicSchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	statements := []string{
		`CREATE SCHEMA IF NOT EXISTS govc`,
		`SET search_path TO govc, public`,
		
		// Schema version
		`CREATE TABLE IF NOT EXISTS govc.schema_version (
			version INT PRIMARY KEY,
			description TEXT,
			applied_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		)`,
		
		// Objects table
		`CREATE TABLE IF NOT EXISTS govc.objects (
			hash VARCHAR(64) PRIMARY KEY,
			type VARCHAR(10) NOT NULL CHECK (type IN ('blob', 'tree', 'commit', 'tag')),
			size BIGINT NOT NULL,
			data BYTEA NOT NULL,
			compressed BOOLEAN DEFAULT false,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		)`,
		
		// Repositories table
		`CREATE TABLE IF NOT EXISTS govc.repositories (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name VARCHAR(255) NOT NULL,
			description TEXT,
			path TEXT,
			is_private BOOLEAN DEFAULT false,
			metadata JSONB,
			size BIGINT DEFAULT 0,
			commit_count INT DEFAULT 0,
			branch_count INT DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		)`,
		
		// Users table
		`CREATE TABLE IF NOT EXISTS govc.users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			username VARCHAR(255) UNIQUE NOT NULL,
			email VARCHAR(255) UNIQUE NOT NULL,
			full_name VARCHAR(255),
			is_active BOOLEAN DEFAULT true,
			is_admin BOOLEAN DEFAULT false,
			metadata JSONB,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
			last_login_at TIMESTAMPTZ
		)`,
		
		// References table
		`CREATE TABLE IF NOT EXISTS govc.refs (
			repository_id UUID NOT NULL,
			name VARCHAR(255) NOT NULL,
			hash VARCHAR(64) NOT NULL,
			type VARCHAR(10) NOT NULL CHECK (type IN ('branch', 'tag')),
			updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
			updated_by UUID,
			PRIMARY KEY (repository_id, name),
			FOREIGN KEY (repository_id) REFERENCES govc.repositories(id) ON DELETE CASCADE
		)`,
		
		// Audit events table
		`CREATE TABLE IF NOT EXISTS govc.audit_events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
			user_id UUID,
			username VARCHAR(255),
			action VARCHAR(100) NOT NULL,
			resource VARCHAR(100),
			resource_id TEXT,
			details JSONB,
			ip_address INET,
			user_agent TEXT,
			success BOOLEAN DEFAULT true,
			error_msg TEXT
		)`,
		
		// Configuration table
		`CREATE TABLE IF NOT EXISTS govc.config (
			key VARCHAR(255) PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		)`,
		
		// Create indexes
		`CREATE INDEX IF NOT EXISTS idx_objects_type ON govc.objects(type)`,
		`CREATE INDEX IF NOT EXISTS idx_objects_created ON govc.objects(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_repositories_name ON govc.repositories(name)`,
		`CREATE INDEX IF NOT EXISTS idx_users_username ON govc.users(username)`,
		`CREATE INDEX IF NOT EXISTS idx_refs_type ON govc.refs(type)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON govc.audit_events(timestamp)`,
		
		// Record schema version
		`INSERT INTO govc.schema_version (version, description) 
		 VALUES (1, 'Initial schema') 
		 ON CONFLICT (version) DO NOTHING`,
	}
	
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute: %s: %w", stmt, err)
		}
	}
	
	return tx.Commit()
}

// prepareStatements prepares commonly used SQL statements
func (s *PostgresStore) prepareStatements() error {
	statements := map[string]string{
		"getObject":    "SELECT data FROM govc.objects WHERE hash = $1",
		"putObject":    "INSERT INTO govc.objects (hash, type, size, data) VALUES ($1, $2, $3, $4) ON CONFLICT (hash) DO NOTHING",
		"deleteObject": "DELETE FROM govc.objects WHERE hash = $1",
		"hasObject":    "SELECT 1 FROM govc.objects WHERE hash = $1 LIMIT 1",
		
		"getRepo":    "SELECT id, name, description, path, is_private, metadata, size, commit_count, branch_count, created_at, updated_at FROM govc.repositories WHERE id = $1",
		"saveRepo":   "INSERT INTO govc.repositories (id, name, description, path, is_private, metadata, size, commit_count, branch_count) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (id) DO UPDATE SET name = $2, description = $3, path = $4, is_private = $5, metadata = $6, size = $7, commit_count = $8, branch_count = $9, updated_at = CURRENT_TIMESTAMP",
		"deleteRepo": "DELETE FROM govc.repositories WHERE id = $1",
		
		"getUser":       "SELECT id, username, email, full_name, is_active, is_admin, metadata, created_at, updated_at, last_login_at FROM govc.users WHERE id = $1",
		"getUserByName": "SELECT id, username, email, full_name, is_active, is_admin, metadata, created_at, updated_at, last_login_at FROM govc.users WHERE username = $1",
		"saveUser":      "INSERT INTO govc.users (id, username, email, full_name, is_active, is_admin, metadata, last_login_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (id) DO UPDATE SET username = $2, email = $3, full_name = $4, is_active = $5, is_admin = $6, metadata = $7, last_login_at = $8, updated_at = CURRENT_TIMESTAMP",
		"deleteUser":    "DELETE FROM govc.users WHERE id = $1",
		
		"getRef":    "SELECT name, hash, type, updated_at, updated_by FROM govc.refs WHERE repository_id = $1 AND name = $2",
		"saveRef":   "INSERT INTO govc.refs (repository_id, name, hash, type, updated_by) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (repository_id, name) DO UPDATE SET hash = $3, type = $4, updated_by = $5, updated_at = CURRENT_TIMESTAMP",
		"deleteRef": "DELETE FROM govc.refs WHERE repository_id = $1 AND name = $2",
		
		"logEvent": "INSERT INTO govc.audit_events (id, timestamp, user_id, username, action, resource, resource_id, details, ip_address, user_agent, success, error_msg) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)",
		
		"getConfig": "SELECT value FROM govc.config WHERE key = $1",
		"setConfig": "INSERT INTO govc.config (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = CURRENT_TIMESTAMP",
		"delConfig": "DELETE FROM govc.config WHERE key = $1",
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
func (s *PostgresStore) getStmt(name string) *sql.Stmt {
	s.stmtsMu.RLock()
	defer s.stmtsMu.RUnlock()
	return s.stmts[name]
}

// monitorPool monitors connection pool statistics
func (s *PostgresStore) monitorPool() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		if s.closed.Load() {
			return
		}
		
		stats := s.db.Stats()
		s.poolStats.activeConns.Store(int32(stats.InUse))
		s.poolStats.idleConns.Store(int32(stats.Idle))
		s.poolStats.waitCount.Store(stats.WaitCount)
		s.poolStats.waitDuration.Store(int64(stats.WaitDuration))
	}
}

// refreshStats refreshes materialized views periodically
func (s *PostgresStore) refreshStats() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		if s.closed.Load() {
			return
		}
		
		// Refresh materialized views if they exist
		s.db.Exec("REFRESH MATERIALIZED VIEW CONCURRENTLY IF EXISTS govc.repository_stats")
	}
}

// Close closes the PostgreSQL store
func (s *PostgresStore) Close() error {
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
	
	// Close database connection
	if s.db != nil {
		return s.db.Close()
	}
	
	return nil
}

// HealthCheck checks if the store is healthy
func (s *PostgresStore) HealthCheck(ctx context.Context) error {
	if s.closed.Load() {
		return fmt.Errorf("store is closed")
	}
	
	// Check database connection
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	
	// Check schema version
	var version int
	err := s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM govc.schema_version").Scan(&version)
	if err != nil {
		return fmt.Errorf("schema check failed: %w", err)
	}
	
	return nil
}

// Type returns the store type
func (s *PostgresStore) Type() string {
	return datastore.TypePostgres
}

// Info returns store information
func (s *PostgresStore) Info() map[string]interface{} {
	stats := s.db.Stats()
	
	var objectCount, repoCount, userCount int64
	var dbSize string
	
	s.db.QueryRow("SELECT COUNT(*) FROM govc.objects").Scan(&objectCount)
	s.db.QueryRow("SELECT COUNT(*) FROM govc.repositories").Scan(&repoCount)
	s.db.QueryRow("SELECT COUNT(*) FROM govc.users").Scan(&userCount)
	s.db.QueryRow("SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&dbSize)
	
	// Get PostgreSQL version
	var pgVersion string
	s.db.QueryRow("SELECT version()").Scan(&pgVersion)
	
	return map[string]interface{}{
		"type":               datastore.TypePostgres,
		"connection":         s.config.Connection,
		"objects":            objectCount,
		"repositories":       repoCount,
		"users":              userCount,
		"database_size":      dbSize,
		"postgres_version":   pgVersion,
		"open_connections":   stats.OpenConnections,
		"in_use":            stats.InUse,
		"idle":              stats.Idle,
		"wait_count":        stats.WaitCount,
		"wait_duration":     stats.WaitDuration,
		"max_idle_closed":   stats.MaxIdleClosed,
		"max_lifetime_closed": stats.MaxLifetimeClosed,
		"uptime":            time.Since(s.startTime),
	}
}

// ObjectStore returns the object store interface
func (s *PostgresStore) ObjectStore() datastore.ObjectStore {
	return s
}

// MetadataStore returns the metadata store interface
func (s *PostgresStore) MetadataStore() datastore.MetadataStore {
	return s
}

// BeginTx begins a transaction
func (s *PostgresStore) BeginTx(ctx context.Context, opts *datastore.TxOptions) (datastore.Transaction, error) {
	sqlOpts := &sql.TxOptions{}
	
	if opts != nil {
		sqlOpts.ReadOnly = opts.ReadOnly
		
		// Map isolation levels
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
	
	// Set search path for transaction
	if _, err := tx.Exec("SET search_path TO govc, public"); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to set search path: %w", err)
	}
	
	return &postgresTransaction{
		store: s,
		tx:    tx,
		ctx:   ctx,
	}, nil
}

// GetMetrics returns store metrics
func (s *PostgresStore) GetMetrics() datastore.Metrics {
	stats := s.db.Stats()
	
	var objectCount int64
	var storageSize int64
	
	s.db.QueryRow("SELECT COUNT(*) FROM govc.objects").Scan(&objectCount)
	s.db.QueryRow("SELECT COALESCE(SUM(size), 0) FROM govc.objects").Scan(&storageSize)
	
	return datastore.Metrics{
		Reads:             s.reads.Load(),
		Writes:            s.writes.Load(),
		Deletes:           s.deletes.Load(),
		ObjectCount:       objectCount,
		StorageSize:       storageSize,
		ActiveConnections: stats.InUse,
		TotalConnections:  int64(stats.OpenConnections),
		StartTime:         s.startTime,
		Uptime:            time.Since(s.startTime),
	}
}

// Helper functions

func marshalJSON(v interface{}) ([]byte, error) {
	if v == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(v)
}

func unmarshalJSON(data []byte, v interface{}) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}