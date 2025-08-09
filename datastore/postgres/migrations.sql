-- PostgreSQL Schema for govc datastore
-- Optimized for enterprise deployments with advanced features

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm"; -- For text search
CREATE EXTENSION IF NOT EXISTS "btree_gin"; -- For composite indexes

-- Create schema
CREATE SCHEMA IF NOT EXISTS govc;
SET search_path TO govc, public;

-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
    version INT PRIMARY KEY,
    description TEXT,
    applied_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Objects table with partitioning support for large deployments
CREATE TABLE IF NOT EXISTS objects (
    hash VARCHAR(64) PRIMARY KEY,
    type VARCHAR(10) NOT NULL CHECK (type IN ('blob', 'tree', 'commit', 'tag')),
    size BIGINT NOT NULL,
    data BYTEA NOT NULL,
    compressed BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    accessed_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
) PARTITION BY HASH (hash);

-- Create partitions for objects (16 partitions for better distribution)
DO $$
BEGIN
    FOR i IN 0..15 LOOP
        EXECUTE format('CREATE TABLE IF NOT EXISTS objects_%s PARTITION OF objects FOR VALUES WITH (modulus 16, remainder %s)', i, i);
    END LOOP;
END $$;

-- Repositories table with advanced features
CREATE TABLE IF NOT EXISTS repositories (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    path TEXT,
    is_private BOOLEAN DEFAULT false,
    is_archived BOOLEAN DEFAULT false,
    is_mirror BOOLEAN DEFAULT false,
    default_branch VARCHAR(255) DEFAULT 'main',
    metadata JSONB,
    size BIGINT DEFAULT 0,
    commit_count INT DEFAULT 0,
    branch_count INT DEFAULT 0,
    tag_count INT DEFAULT 0,
    contributor_count INT DEFAULT 0,
    star_count INT DEFAULT 0,
    fork_count INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    last_activity_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ
);

-- Users table with enterprise features
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    full_name VARCHAR(255),
    avatar_url TEXT,
    bio TEXT,
    location VARCHAR(255),
    company VARCHAR(255),
    is_active BOOLEAN DEFAULT true,
    is_admin BOOLEAN DEFAULT false,
    is_bot BOOLEAN DEFAULT false,
    metadata JSONB,
    preferences JSONB,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMPTZ,
    suspended_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ
);

-- References table with better indexing
CREATE TABLE IF NOT EXISTS refs (
    repository_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    hash VARCHAR(64) NOT NULL,
    type VARCHAR(10) NOT NULL CHECK (type IN ('branch', 'tag')),
    is_protected BOOLEAN DEFAULT false,
    metadata JSONB,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_by UUID,
    PRIMARY KEY (repository_id, name),
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE,
    FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

-- Audit events table with partitioning by time
CREATE TABLE IF NOT EXISTS audit_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    user_id UUID,
    username VARCHAR(255),
    action VARCHAR(100) NOT NULL,
    resource VARCHAR(100),
    resource_id TEXT,
    details JSONB,
    ip_address INET,
    user_agent TEXT,
    session_id VARCHAR(255),
    request_id VARCHAR(255),
    duration_ms INT,
    success BOOLEAN DEFAULT true,
    error_msg TEXT,
    stack_trace TEXT
) PARTITION BY RANGE (timestamp);

-- Create monthly partitions for audit events
DO $$
DECLARE
    start_date DATE := '2024-01-01';
    end_date DATE;
    partition_name TEXT;
BEGIN
    FOR i IN 0..23 LOOP
        end_date := start_date + INTERVAL '1 month';
        partition_name := 'audit_events_' || to_char(start_date, 'YYYY_MM');
        
        EXECUTE format('CREATE TABLE IF NOT EXISTS %I PARTITION OF audit_events FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date);
        
        start_date := end_date;
    END LOOP;
END $$;

-- Configuration table
CREATE TABLE IF NOT EXISTS config (
    key VARCHAR(255) PRIMARY KEY,
    value TEXT NOT NULL,
    description TEXT,
    is_secret BOOLEAN DEFAULT false,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_by UUID,
    FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

-- Access control tables for enterprise
CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS team_members (
    team_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role VARCHAR(50) DEFAULT 'member',
    added_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    added_by UUID,
    PRIMARY KEY (team_id, user_id),
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (added_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS repository_permissions (
    repository_id UUID NOT NULL,
    user_id UUID,
    team_id UUID,
    permission VARCHAR(50) NOT NULL CHECK (permission IN ('read', 'write', 'admin')),
    granted_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    granted_by UUID,
    expires_at TIMESTAMPTZ,
    PRIMARY KEY (repository_id, COALESCE(user_id, '00000000-0000-0000-0000-000000000000'), COALESCE(team_id, '00000000-0000-0000-0000-000000000000')),
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
    FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
    CHECK ((user_id IS NOT NULL AND team_id IS NULL) OR (user_id IS NULL AND team_id IS NOT NULL))
);

-- Sessions table for distributed deployments
CREATE TABLE IF NOT EXISTS sessions (
    id VARCHAR(255) PRIMARY KEY,
    user_id UUID NOT NULL,
    data JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_objects_type ON objects(type);
CREATE INDEX IF NOT EXISTS idx_objects_created ON objects(created_at);
CREATE INDEX IF NOT EXISTS idx_objects_accessed ON objects(accessed_at);

CREATE INDEX IF NOT EXISTS idx_repositories_name ON repositories(name);
CREATE INDEX IF NOT EXISTS idx_repositories_name_trgm ON repositories USING gin(name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_repositories_metadata ON repositories USING gin(metadata);
CREATE INDEX IF NOT EXISTS idx_repositories_created ON repositories(created_at);
CREATE INDEX IF NOT EXISTS idx_repositories_updated ON repositories(updated_at);
CREATE INDEX IF NOT EXISTS idx_repositories_deleted ON repositories(deleted_at) WHERE deleted_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_active ON users(is_active) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_users_metadata ON users USING gin(metadata);
CREATE INDEX IF NOT EXISTS idx_users_deleted ON users(deleted_at) WHERE deleted_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_refs_type ON refs(type);
CREATE INDEX IF NOT EXISTS idx_refs_hash ON refs(hash);
CREATE INDEX IF NOT EXISTS idx_refs_repository_type ON refs(repository_id, type);

CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_events(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_events(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_events(action);
CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit_events(resource, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_session ON audit_events(session_id);

CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

CREATE INDEX IF NOT EXISTS idx_permissions_user ON repository_permissions(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_permissions_team ON repository_permissions(team_id) WHERE team_id IS NOT NULL;

-- Create materialized views for statistics
CREATE MATERIALIZED VIEW IF NOT EXISTS repository_stats AS
SELECT 
    r.id,
    r.name,
    COUNT(DISTINCT c.hash) as commit_count,
    COUNT(DISTINCT CASE WHEN ref.type = 'branch' THEN ref.name END) as branch_count,
    COUNT(DISTINCT CASE WHEN ref.type = 'tag' THEN ref.name END) as tag_count,
    MAX(ae.timestamp) as last_activity
FROM repositories r
LEFT JOIN refs ref ON r.id = ref.repository_id
LEFT JOIN objects c ON c.type = 'commit'
LEFT JOIN audit_events ae ON ae.resource = 'repository' AND ae.resource_id = r.id::text
GROUP BY r.id, r.name;

CREATE UNIQUE INDEX ON repository_stats(id);

-- Functions for automatic timestamp updates
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply triggers for updated_at
CREATE TRIGGER update_repositories_timestamp BEFORE UPDATE ON repositories
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_users_timestamp BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_refs_timestamp BEFORE UPDATE ON refs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_teams_timestamp BEFORE UPDATE ON teams
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_config_timestamp BEFORE UPDATE ON config
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_sessions_timestamp BEFORE UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Function to clean up old audit events
CREATE OR REPLACE FUNCTION cleanup_old_audit_events(retention_days INT DEFAULT 90)
RETURNS INT AS $$
DECLARE
    deleted_count INT;
BEGIN
    DELETE FROM audit_events 
    WHERE timestamp < CURRENT_TIMESTAMP - (retention_days || ' days')::INTERVAL;
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Function to refresh statistics
CREATE OR REPLACE FUNCTION refresh_stats()
RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY repository_stats;
END;
$$ LANGUAGE plpgsql;

-- Insert initial schema version
INSERT INTO schema_version (version, description) VALUES (1, 'Initial PostgreSQL schema for govc')
ON CONFLICT (version) DO NOTHING;