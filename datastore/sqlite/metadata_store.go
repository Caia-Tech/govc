package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Caia-Tech/govc/datastore"
	"github.com/google/uuid"
)

// MetadataStore implementation

// SaveRepository saves a repository
func (s *SQLiteStore) SaveRepository(repo *datastore.Repository) error {
	s.writes.Add(1)
	
	metadata, _ := json.Marshal(repo.Metadata)
	
	_, err := s.getStmt("saveRepo").Exec(
		repo.ID, repo.Name, repo.Description, repo.Path,
		repo.IsPrivate, string(metadata), repo.Size,
		repo.CommitCount, repo.BranchCount,
		repo.CreatedAt, repo.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save repository: %w", err)
	}
	
	return nil
}

// GetRepository retrieves a repository by ID
func (s *SQLiteStore) GetRepository(id string) (*datastore.Repository, error) {
	s.reads.Add(1)
	
	repo := &datastore.Repository{}
	var metadata string
	
	err := s.getStmt("getRepo").QueryRow(id).Scan(
		&repo.ID, &repo.Name, &repo.Description, &repo.Path,
		&repo.IsPrivate, &metadata, &repo.Size,
		&repo.CommitCount, &repo.BranchCount,
		&repo.CreatedAt, &repo.UpdatedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, datastore.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}
	
	if metadata != "" {
		json.Unmarshal([]byte(metadata), &repo.Metadata)
	}
	
	return repo, nil
}

// ListRepositories lists repositories with filtering
func (s *SQLiteStore) ListRepositories(filter datastore.RepositoryFilter) ([]*datastore.Repository, error) {
	s.reads.Add(1)
	
	query := "SELECT id, name, description, path, is_private, metadata, size, commit_count, branch_count, created_at, updated_at FROM repositories WHERE 1=1"
	args := []interface{}{}
	
	if filter.Name != "" {
		query += " AND name LIKE ?"
		args = append(args, "%"+filter.Name+"%")
	}
	
	if filter.IsPrivate != nil {
		query += " AND is_private = ?"
		args = append(args, *filter.IsPrivate)
	}
	
	if filter.CreatedAfter != nil {
		query += " AND created_at >= ?"
		args = append(args, *filter.CreatedAfter)
	}
	
	if filter.CreatedBefore != nil {
		query += " AND created_at <= ?"
		args = append(args, *filter.CreatedBefore)
	}
	
	// Add sorting
	switch filter.OrderBy {
	case "name":
		query += " ORDER BY name"
	case "updated_at":
		query += " ORDER BY updated_at DESC"
	case "size":
		query += " ORDER BY size DESC"
	default:
		query += " ORDER BY created_at DESC"
	}
	
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
		
		if filter.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filter.Offset)
		}
	}
	
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}
	defer rows.Close()
	
	var repos []*datastore.Repository
	for rows.Next() {
		repo := &datastore.Repository{}
		var metadata string
		
		err := rows.Scan(
			&repo.ID, &repo.Name, &repo.Description, &repo.Path,
			&repo.IsPrivate, &metadata, &repo.Size,
			&repo.CommitCount, &repo.BranchCount,
			&repo.CreatedAt, &repo.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan repository: %w", err)
		}
		
		if metadata != "" {
			json.Unmarshal([]byte(metadata), &repo.Metadata)
		}
		
		repos = append(repos, repo)
	}
	
	return repos, rows.Err()
}

// DeleteRepository deletes a repository
func (s *SQLiteStore) DeleteRepository(id string) error {
	s.deletes.Add(1)
	
	result, err := s.getStmt("deleteRepo").Exec(id)
	if err != nil {
		return fmt.Errorf("failed to delete repository: %w", err)
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

// UpdateRepository updates a repository
func (s *SQLiteStore) UpdateRepository(id string, updates map[string]interface{}) error {
	s.writes.Add(1)
	
	// Build dynamic UPDATE query
	query := "UPDATE repositories SET updated_at = CURRENT_TIMESTAMP"
	args := []interface{}{}
	
	for key, value := range updates {
		switch key {
		case "name", "description", "path":
			query += fmt.Sprintf(", %s = ?", key)
			args = append(args, value)
		case "is_private":
			query += ", is_private = ?"
			args = append(args, value)
		case "size", "commit_count", "branch_count":
			query += fmt.Sprintf(", %s = ?", key)
			args = append(args, value)
		case "metadata":
			data, _ := json.Marshal(value)
			query += ", metadata = ?"
			args = append(args, string(data))
		}
	}
	
	query += " WHERE id = ?"
	args = append(args, id)
	
	result, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update repository: %w", err)
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

// SaveUser saves a user
func (s *SQLiteStore) SaveUser(user *datastore.User) error {
	s.writes.Add(1)
	
	metadata, _ := json.Marshal(user.Metadata)
	
	_, err := s.getStmt("saveUser").Exec(
		user.ID, user.Username, user.Email, user.FullName,
		user.IsActive, user.IsAdmin, string(metadata),
		user.CreatedAt, user.UpdatedAt, user.LastLoginAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}
	
	return nil
}

// GetUser retrieves a user by ID
func (s *SQLiteStore) GetUser(id string) (*datastore.User, error) {
	s.reads.Add(1)
	
	user := &datastore.User{}
	var metadata string
	var lastLogin sql.NullTime
	
	err := s.getStmt("getUser").QueryRow(id).Scan(
		&user.ID, &user.Username, &user.Email, &user.FullName,
		&user.IsActive, &user.IsAdmin, &metadata,
		&user.CreatedAt, &user.UpdatedAt, &lastLogin,
	)
	
	if err == sql.ErrNoRows {
		return nil, datastore.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	
	if metadata != "" {
		json.Unmarshal([]byte(metadata), &user.Metadata)
	}
	
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	
	return user, nil
}

// GetUserByUsername retrieves a user by username
func (s *SQLiteStore) GetUserByUsername(username string) (*datastore.User, error) {
	s.reads.Add(1)
	
	user := &datastore.User{}
	var metadata string
	var lastLogin sql.NullTime
	
	err := s.getStmt("getUserByName").QueryRow(username).Scan(
		&user.ID, &user.Username, &user.Email, &user.FullName,
		&user.IsActive, &user.IsAdmin, &metadata,
		&user.CreatedAt, &user.UpdatedAt, &lastLogin,
	)
	
	if err == sql.ErrNoRows {
		return nil, datastore.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}
	
	if metadata != "" {
		json.Unmarshal([]byte(metadata), &user.Metadata)
	}
	
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	
	return user, nil
}

// ListUsers lists users with filtering
func (s *SQLiteStore) ListUsers(filter datastore.UserFilter) ([]*datastore.User, error) {
	s.reads.Add(1)
	
	query := "SELECT id, username, email, full_name, is_active, is_admin, metadata, created_at, updated_at, last_login_at FROM users WHERE 1=1"
	args := []interface{}{}
	
	if filter.IsActive != nil {
		query += " AND is_active = ?"
		args = append(args, *filter.IsActive)
	}
	
	if filter.IsAdmin != nil {
		query += " AND is_admin = ?"
		args = append(args, *filter.IsAdmin)
	}
	
	// Add sorting
	switch filter.OrderBy {
	case "username":
		query += " ORDER BY username"
	case "email":
		query += " ORDER BY email"
	case "last_login_at":
		query += " ORDER BY last_login_at DESC"
	default:
		query += " ORDER BY created_at DESC"
	}
	
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
		
		if filter.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filter.Offset)
		}
	}
	
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()
	
	var users []*datastore.User
	for rows.Next() {
		user := &datastore.User{}
		var metadata string
		var lastLogin sql.NullTime
		
		err := rows.Scan(
			&user.ID, &user.Username, &user.Email, &user.FullName,
			&user.IsActive, &user.IsAdmin, &metadata,
			&user.CreatedAt, &user.UpdatedAt, &lastLogin,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		
		if metadata != "" {
			json.Unmarshal([]byte(metadata), &user.Metadata)
		}
		
		if lastLogin.Valid {
			user.LastLoginAt = &lastLogin.Time
		}
		
		users = append(users, user)
	}
	
	return users, rows.Err()
}

// DeleteUser deletes a user
func (s *SQLiteStore) DeleteUser(id string) error {
	s.deletes.Add(1)
	
	result, err := s.getStmt("deleteUser").Exec(id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
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

// UpdateUser updates a user
func (s *SQLiteStore) UpdateUser(id string, updates map[string]interface{}) error {
	s.writes.Add(1)
	
	// Build dynamic UPDATE query
	query := "UPDATE users SET updated_at = CURRENT_TIMESTAMP"
	args := []interface{}{}
	
	for key, value := range updates {
		switch key {
		case "username", "email", "full_name":
			query += fmt.Sprintf(", %s = ?", key)
			args = append(args, value)
		case "is_active", "is_admin":
			query += fmt.Sprintf(", %s = ?", key)
			args = append(args, value)
		case "last_login_at":
			query += ", last_login_at = ?"
			args = append(args, value)
		case "metadata":
			data, _ := json.Marshal(value)
			query += ", metadata = ?"
			args = append(args, string(data))
		}
	}
	
	query += " WHERE id = ?"
	args = append(args, id)
	
	result, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
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

// SaveRef saves a reference
func (s *SQLiteStore) SaveRef(repoID string, ref *datastore.Reference) error {
	s.writes.Add(1)
	
	_, err := s.getStmt("saveRef").Exec(
		repoID, ref.Name, ref.Hash, ref.Type,
		ref.UpdatedAt, ref.UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to save ref: %w", err)
	}
	
	return nil
}

// GetRef retrieves a reference
func (s *SQLiteStore) GetRef(repoID string, name string) (*datastore.Reference, error) {
	s.reads.Add(1)
	
	ref := &datastore.Reference{}
	
	err := s.getStmt("getRef").QueryRow(repoID, name).Scan(
		&repoID, &ref.Name, &ref.Hash, &ref.Type,
		&ref.UpdatedAt, &ref.UpdatedBy,
	)
	
	if err == sql.ErrNoRows {
		return nil, datastore.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get ref: %w", err)
	}
	
	return ref, nil
}

// ListRefs lists references for a repository
func (s *SQLiteStore) ListRefs(repoID string, refType datastore.RefType) ([]*datastore.Reference, error) {
	s.reads.Add(1)
	
	query := "SELECT name, hash, type, updated_at, updated_by FROM refs WHERE repository_id = ?"
	args := []interface{}{repoID}
	
	if refType != "" {
		query += " AND type = ?"
		args = append(args, string(refType))
	}
	
	query += " ORDER BY name"
	
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list refs: %w", err)
	}
	defer rows.Close()
	
	var refs []*datastore.Reference
	for rows.Next() {
		ref := &datastore.Reference{}
		
		err := rows.Scan(
			&ref.Name, &ref.Hash, &ref.Type,
			&ref.UpdatedAt, &ref.UpdatedBy,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ref: %w", err)
		}
		
		refs = append(refs, ref)
	}
	
	return refs, rows.Err()
}

// DeleteRef deletes a reference
func (s *SQLiteStore) DeleteRef(repoID string, name string) error {
	s.deletes.Add(1)
	
	result, err := s.getStmt("deleteRef").Exec(repoID, name)
	if err != nil {
		return fmt.Errorf("failed to delete ref: %w", err)
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

// UpdateRef updates a reference hash
func (s *SQLiteStore) UpdateRef(repoID string, name string, newHash string) error {
	s.writes.Add(1)
	
	result, err := s.db.Exec(
		"UPDATE refs SET hash = ?, updated_at = CURRENT_TIMESTAMP WHERE repository_id = ? AND name = ?",
		newHash, repoID, name,
	)
	if err != nil {
		return fmt.Errorf("failed to update ref: %w", err)
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

// LogEvent logs an audit event
func (s *SQLiteStore) LogEvent(event *datastore.AuditEvent) error {
	s.writes.Add(1)
	
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	details, _ := json.Marshal(event.Details)
	
	_, err := s.getStmt("logEvent").Exec(
		event.ID, event.Timestamp, event.UserID, event.Username,
		event.Action, event.Resource, event.ResourceID, string(details),
		event.IPAddress, event.UserAgent, event.Success, event.ErrorMsg,
	)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}
	
	return nil
}

// QueryEvents queries audit events
func (s *SQLiteStore) QueryEvents(filter datastore.EventFilter) ([]*datastore.AuditEvent, error) {
	s.reads.Add(1)
	
	query := "SELECT id, timestamp, user_id, username, action, resource, resource_id, details, ip_address, user_agent, success, error_msg FROM audit_events WHERE 1=1"
	args := []interface{}{}
	
	if filter.UserID != "" {
		query += " AND user_id = ?"
		args = append(args, filter.UserID)
	}
	
	if filter.Action != "" {
		query += " AND action = ?"
		args = append(args, filter.Action)
	}
	
	if filter.Resource != "" {
		query += " AND resource = ?"
		args = append(args, filter.Resource)
	}
	
	if filter.Success != nil {
		query += " AND success = ?"
		args = append(args, *filter.Success)
	}
	
	if filter.After != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filter.After)
	}
	
	if filter.Before != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filter.Before)
	}
	
	query += " ORDER BY timestamp DESC"
	
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
		
		if filter.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filter.Offset)
		}
	}
	
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()
	
	var events []*datastore.AuditEvent
	for rows.Next() {
		event := &datastore.AuditEvent{}
		var details string
		var errorMsg sql.NullString
		
		err := rows.Scan(
			&event.ID, &event.Timestamp, &event.UserID, &event.Username,
			&event.Action, &event.Resource, &event.ResourceID, &details,
			&event.IPAddress, &event.UserAgent, &event.Success, &errorMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		
		if details != "" {
			json.Unmarshal([]byte(details), &event.Details)
		}
		
		if errorMsg.Valid {
			event.ErrorMsg = errorMsg.String
		}
		
		events = append(events, event)
	}
	
	return events, rows.Err()
}

// CountEvents counts audit events
func (s *SQLiteStore) CountEvents(filter datastore.EventFilter) (int64, error) {
	s.reads.Add(1)
	
	query := "SELECT COUNT(*) FROM audit_events WHERE 1=1"
	args := []interface{}{}
	
	if filter.UserID != "" {
		query += " AND user_id = ?"
		args = append(args, filter.UserID)
	}
	
	if filter.Action != "" {
		query += " AND action = ?"
		args = append(args, filter.Action)
	}
	
	if filter.Resource != "" {
		query += " AND resource = ?"
		args = append(args, filter.Resource)
	}
	
	if filter.Success != nil {
		query += " AND success = ?"
		args = append(args, *filter.Success)
	}
	
	if filter.After != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filter.After)
	}
	
	if filter.Before != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filter.Before)
	}
	
	var count int64
	err := s.db.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count events: %w", err)
	}
	
	return count, nil
}

// GetConfig retrieves a configuration value
func (s *SQLiteStore) GetConfig(key string) (string, error) {
	s.reads.Add(1)
	
	var value string
	err := s.getStmt("getConfig").QueryRow(key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", datastore.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to get config: %w", err)
	}
	
	return value, nil
}

// SetConfig sets a configuration value
func (s *SQLiteStore) SetConfig(key string, value string) error {
	s.writes.Add(1)
	
	_, err := s.getStmt("setConfig").Exec(key, value)
	if err != nil {
		return fmt.Errorf("failed to set config: %w", err)
	}
	
	return nil
}

// GetAllConfig retrieves all configuration
func (s *SQLiteStore) GetAllConfig() (map[string]string, error) {
	s.reads.Add(1)
	
	rows, err := s.db.Query("SELECT key, value FROM config")
	if err != nil {
		return nil, fmt.Errorf("failed to get all config: %w", err)
	}
	defer rows.Close()
	
	config := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("failed to scan config: %w", err)
		}
		config[key] = value
	}
	
	return config, rows.Err()
}

// DeleteConfig deletes a configuration key
func (s *SQLiteStore) DeleteConfig(key string) error {
	s.deletes.Add(1)
	
	result, err := s.getStmt("delConfig").Exec(key)
	if err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
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