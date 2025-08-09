package sqlite

import (
	"database/sql"
	"fmt"
)

// Migration represents a database migration
type Migration struct {
	Version     int
	Description string
	Up          string
	Down        string
}

// migrations defines all database migrations
var migrations = []Migration{
	{
		Version:     1,
		Description: "Initial schema",
		Up: `
			-- Enable foreign keys
			PRAGMA foreign_keys = ON;
			
			-- Create schema version table
			CREATE TABLE IF NOT EXISTS schema_version (
				version INTEGER PRIMARY KEY,
				applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			
			-- Objects table for Git objects
			CREATE TABLE IF NOT EXISTS objects (
				hash TEXT PRIMARY KEY,
				type TEXT NOT NULL CHECK (type IN ('blob', 'tree', 'commit', 'tag')),
				size INTEGER NOT NULL,
				data BLOB NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_objects_type ON objects(type);
			CREATE INDEX IF NOT EXISTS idx_objects_created ON objects(created_at);
			
			-- Repositories table
			CREATE TABLE IF NOT EXISTS repositories (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				description TEXT,
				path TEXT,
				is_private BOOLEAN DEFAULT 0,
				metadata TEXT, -- JSON
				size INTEGER DEFAULT 0,
				commit_count INTEGER DEFAULT 0,
				branch_count INTEGER DEFAULT 0,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_repositories_name ON repositories(name);
			CREATE INDEX IF NOT EXISTS idx_repositories_created ON repositories(created_at);
			
			-- Users table
			CREATE TABLE IF NOT EXISTS users (
				id TEXT PRIMARY KEY,
				username TEXT UNIQUE NOT NULL,
				email TEXT UNIQUE NOT NULL,
				full_name TEXT,
				is_active BOOLEAN DEFAULT 1,
				is_admin BOOLEAN DEFAULT 0,
				metadata TEXT, -- JSON
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				last_login_at TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
			CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
			CREATE INDEX IF NOT EXISTS idx_users_active ON users(is_active);
			
			-- References table (branches and tags)
			CREATE TABLE IF NOT EXISTS refs (
				repository_id TEXT NOT NULL,
				name TEXT NOT NULL,
				hash TEXT NOT NULL,
				type TEXT NOT NULL CHECK (type IN ('branch', 'tag')),
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_by TEXT,
				PRIMARY KEY (repository_id, name),
				FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_refs_type ON refs(type);
			CREATE INDEX IF NOT EXISTS idx_refs_hash ON refs(hash);
			
			-- Audit events table
			CREATE TABLE IF NOT EXISTS audit_events (
				id TEXT PRIMARY KEY,
				timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				user_id TEXT,
				username TEXT,
				action TEXT NOT NULL,
				resource TEXT,
				resource_id TEXT,
				details TEXT, -- JSON
				ip_address TEXT,
				user_agent TEXT,
				success BOOLEAN DEFAULT 1,
				error_msg TEXT
			);
			CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_events(timestamp);
			CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_events(user_id);
			CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_events(action);
			CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit_events(resource);
			
			-- Configuration table
			CREATE TABLE IF NOT EXISTS config (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			
			-- Triggers for updated_at
			CREATE TRIGGER IF NOT EXISTS update_repositories_timestamp 
			AFTER UPDATE ON repositories
			BEGIN
				UPDATE repositories SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
			END;
			
			CREATE TRIGGER IF NOT EXISTS update_users_timestamp 
			AFTER UPDATE ON users
			BEGIN
				UPDATE users SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
			END;
			
			CREATE TRIGGER IF NOT EXISTS update_refs_timestamp 
			AFTER UPDATE ON refs
			BEGIN
				UPDATE refs SET updated_at = CURRENT_TIMESTAMP 
				WHERE repository_id = NEW.repository_id AND name = NEW.name;
			END;
			
			CREATE TRIGGER IF NOT EXISTS update_config_timestamp 
			AFTER UPDATE ON config
			BEGIN
				UPDATE config SET updated_at = CURRENT_TIMESTAMP WHERE key = NEW.key;
			END;
		`,
		Down: `
			DROP TABLE IF EXISTS config;
			DROP TABLE IF EXISTS audit_events;
			DROP TABLE IF EXISTS refs;
			DROP TABLE IF EXISTS users;
			DROP TABLE IF EXISTS repositories;
			DROP TABLE IF EXISTS objects;
			DROP TABLE IF EXISTS schema_version;
		`,
	},
	{
		Version:     2,
		Description: "Add indexes for performance",
		Up: `
			-- Additional performance indexes
			CREATE INDEX IF NOT EXISTS idx_objects_hash_prefix ON objects(substr(hash, 1, 2));
			CREATE INDEX IF NOT EXISTS idx_audit_events_composite ON audit_events(user_id, timestamp);
			CREATE INDEX IF NOT EXISTS idx_refs_composite ON refs(repository_id, type);
		`,
		Down: `
			DROP INDEX IF EXISTS idx_objects_hash_prefix;
			DROP INDEX IF EXISTS idx_audit_events_composite;
			DROP INDEX IF EXISTS idx_refs_composite;
		`,
	},
}

// Migrate runs all pending migrations
func Migrate(db *sql.DB) error {
	// Create schema_version table if it doesn't exist
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_version table: %w", err)
	}
	
	// Get current version
	var currentVersion int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}
	
	// Run pending migrations
	for _, migration := range migrations {
		if migration.Version <= currentVersion {
			continue
		}
		
		// Begin transaction
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", migration.Version, err)
		}
		
		// Run migration
		_, err = tx.Exec(migration.Up)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to run migration %d: %w", migration.Version, err)
		}
		
		// Record migration
		_, err = tx.Exec("INSERT INTO schema_version (version) VALUES (?)", migration.Version)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", migration.Version, err)
		}
		
		// Commit transaction
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", migration.Version, err)
		}
		
		fmt.Printf("Applied migration %d: %s\n", migration.Version, migration.Description)
	}
	
	return nil
}

// Rollback rolls back to a specific version
func Rollback(db *sql.DB, targetVersion int) error {
	// Get current version
	var currentVersion int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}
	
	if targetVersion >= currentVersion {
		return fmt.Errorf("target version %d is not less than current version %d", targetVersion, currentVersion)
	}
	
	// Run down migrations in reverse order
	for i := len(migrations) - 1; i >= 0; i-- {
		migration := migrations[i]
		
		if migration.Version <= targetVersion || migration.Version > currentVersion {
			continue
		}
		
		// Begin transaction
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for rollback %d: %w", migration.Version, err)
		}
		
		// Run down migration
		_, err = tx.Exec(migration.Down)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to rollback migration %d: %w", migration.Version, err)
		}
		
		// Remove migration record
		_, err = tx.Exec("DELETE FROM schema_version WHERE version = ?", migration.Version)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to remove migration record %d: %w", migration.Version, err)
		}
		
		// Commit transaction
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit rollback %d: %w", migration.Version, err)
		}
		
		fmt.Printf("Rolled back migration %d: %s\n", migration.Version, migration.Description)
	}
	
	return nil
}