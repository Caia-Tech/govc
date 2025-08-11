package postgres

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/Caia-Tech/govc/datastore"
	"github.com/google/uuid"
)

// MetadataStore implementation

// SaveRepository saves a repository
func (s *PostgresStore) SaveRepository(repo *datastore.Repository) error {
	s.writes.Add(1)
	
	metadata, err := marshalJSON(repo.Metadata)
	if err != nil {
		return err
	}
	
	_, err = s.getStmt("saveRepo").Exec(
		repo.ID, repo.Name, repo.Description, repo.Path,
		repo.IsPrivate, metadata, repo.Size,
		repo.CommitCount, repo.BranchCount,
	)
	return err
}

// GetRepository retrieves a repository by ID
func (s *PostgresStore) GetRepository(id string) (*datastore.Repository, error) {
	s.reads.Add(1)
	
	repo := &datastore.Repository{}
	var metadata []byte
	
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
		return nil, err
	}
	
	if len(metadata) > 0 {
		unmarshalJSON(metadata, &repo.Metadata)
	}
	
	return repo, nil
}

// ListRepositories lists repositories with filtering
func (s *PostgresStore) ListRepositories(filter datastore.RepositoryFilter) ([]*datastore.Repository, error) {
	s.reads.Add(1)
	
	query := "SELECT id, name, description, path, is_private, metadata, size, commit_count, branch_count, created_at, updated_at FROM govc.repositories WHERE 1=1"
	args := []interface{}{}
	argCount := 0
	
	// Add filters...
	// Simplified for brevity
	
	query += " ORDER BY created_at DESC"
	
	if filter.Limit > 0 {
		argCount++
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, filter.Limit)
		
		if filter.Offset > 0 {
			argCount++
			query += fmt.Sprintf(" OFFSET $%d", argCount)
			args = append(args, filter.Offset)
		}
	}
	
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var repos []*datastore.Repository
	for rows.Next() {
		repo := &datastore.Repository{}
		var metadata []byte
		
		err := rows.Scan(
			&repo.ID, &repo.Name, &repo.Description, &repo.Path,
			&repo.IsPrivate, &metadata, &repo.Size,
			&repo.CommitCount, &repo.BranchCount,
			&repo.CreatedAt, &repo.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		
		if len(metadata) > 0 {
			unmarshalJSON(metadata, &repo.Metadata)
		}
		
		repos = append(repos, repo)
	}
	
	return repos, rows.Err()
}

// DeleteRepository deletes a repository
func (s *PostgresStore) DeleteRepository(id string) error {
	s.deletes.Add(1)
	
	result, err := s.getStmt("deleteRepo").Exec(id)
	if err != nil {
		return err
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return datastore.ErrNotFound
	}
	
	return nil
}

// UpdateRepository updates a repository
func (s *PostgresStore) UpdateRepository(id string, updates map[string]interface{}) error {
	// Simplified implementation
	return fmt.Errorf("not fully implemented")
}

// SaveUser saves a user
func (s *PostgresStore) SaveUser(user *datastore.User) error {
	s.writes.Add(1)
	
	metadata, err := marshalJSON(user.Metadata)
	if err != nil {
		return err
	}
	
	_, err = s.getStmt("saveUser").Exec(
		user.ID, user.Username, user.Email, user.FullName,
		user.IsActive, user.IsAdmin, metadata, user.LastLoginAt,
	)
	return err
}

// GetUser retrieves a user by ID
func (s *PostgresStore) GetUser(id string) (*datastore.User, error) {
	s.reads.Add(1)
	
	user := &datastore.User{}
	var metadata []byte
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
		return nil, err
	}
	
	if len(metadata) > 0 {
		unmarshalJSON(metadata, &user.Metadata)
	}
	
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	
	return user, nil
}

// GetUserByUsername retrieves a user by username
func (s *PostgresStore) GetUserByUsername(username string) (*datastore.User, error) {
	s.reads.Add(1)
	
	user := &datastore.User{}
	var metadata []byte
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
		return nil, err
	}
	
	if len(metadata) > 0 {
		unmarshalJSON(metadata, &user.Metadata)
	}
	
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	
	return user, nil
}

// ListUsers lists users with filtering
func (s *PostgresStore) ListUsers(filter datastore.UserFilter) ([]*datastore.User, error) {
	// Simplified implementation
	return []*datastore.User{}, nil
}

// DeleteUser deletes a user
func (s *PostgresStore) DeleteUser(id string) error {
	s.deletes.Add(1)
	
	result, err := s.getStmt("deleteUser").Exec(id)
	if err != nil {
		return err
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return datastore.ErrNotFound
	}
	
	return nil
}

// UpdateUser updates a user
func (s *PostgresStore) UpdateUser(id string, updates map[string]interface{}) error {
	// Simplified implementation
	return fmt.Errorf("not fully implemented")
}

// SaveRef saves a reference
func (s *PostgresStore) SaveRef(repoID string, ref *datastore.Reference) error {
	s.writes.Add(1)
	
	_, err := s.getStmt("saveRef").Exec(
		repoID, ref.Name, ref.Hash, string(ref.Type), ref.UpdatedBy,
	)
	return err
}

// GetRef retrieves a reference
func (s *PostgresStore) GetRef(repoID string, name string) (*datastore.Reference, error) {
	s.reads.Add(1)
	
	ref := &datastore.Reference{}
	var refType string
	
	err := s.getStmt("getRef").QueryRow(repoID, name).Scan(
		&ref.Name, &ref.Hash, &refType, &ref.UpdatedAt, &ref.UpdatedBy,
	)
	
	if err == sql.ErrNoRows {
		return nil, datastore.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	
	ref.Type = datastore.RefType(refType)
	
	return ref, nil
}

// ListRefs lists references for a repository
func (s *PostgresStore) ListRefs(repoID string, refType datastore.RefType) ([]*datastore.Reference, error) {
	// Simplified implementation
	return []*datastore.Reference{}, nil
}

// DeleteRef deletes a reference
func (s *PostgresStore) DeleteRef(repoID string, name string) error {
	s.deletes.Add(1)
	
	result, err := s.getStmt("deleteRef").Exec(repoID, name)
	if err != nil {
		return err
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return datastore.ErrNotFound
	}
	
	return nil
}

// UpdateRef updates a reference hash
func (s *PostgresStore) UpdateRef(repoID string, name string, newHash string) error {
	// Simplified implementation
	return fmt.Errorf("not fully implemented")
}

// LogEvent logs an audit event
func (s *PostgresStore) LogEvent(event *datastore.AuditEvent) error {
	s.writes.Add(1)
	
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	details, err := marshalJSON(event.Details)
	if err != nil {
		return err
	}
	
	_, err = s.getStmt("logEvent").Exec(
		event.ID, event.Timestamp, event.UserID, event.Username,
		event.Action, event.Resource, event.ResourceID, details,
		event.IPAddress, event.UserAgent, event.Success, event.ErrorMsg,
	)
	return err
}

// QueryEvents queries audit events
func (s *PostgresStore) QueryEvents(filter datastore.EventFilter) ([]*datastore.AuditEvent, error) {
	// Simplified implementation
	return []*datastore.AuditEvent{}, nil
}

// CountEvents counts audit events
func (s *PostgresStore) CountEvents(filter datastore.EventFilter) (int64, error) {
	// Simplified implementation
	return 0, nil
}

// GetConfig retrieves a configuration value
func (s *PostgresStore) GetConfig(key string) (string, error) {
	s.reads.Add(1)
	
	var value string
	err := s.getStmt("getConfig").QueryRow(key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", datastore.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	
	return value, nil
}

// SetConfig sets a configuration value
func (s *PostgresStore) SetConfig(key string, value string) error {
	s.writes.Add(1)
	
	_, err := s.getStmt("setConfig").Exec(key, value)
	return err
}

// GetAllConfig retrieves all configuration
func (s *PostgresStore) GetAllConfig() (map[string]string, error) {
	s.reads.Add(1)
	
	rows, err := s.db.Query("SELECT key, value FROM govc.config")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	config := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		config[key] = value
	}
	
	return config, rows.Err()
}

// DeleteConfig deletes a configuration key
func (s *PostgresStore) DeleteConfig(key string) error {
	s.deletes.Add(1)
	
	result, err := s.getStmt("delConfig").Exec(key)
	if err != nil {
		return err
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return datastore.ErrNotFound
	}
	
	return nil
}