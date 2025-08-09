package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/caiatech/govc/datastore"
	"github.com/google/uuid"
)

// sqliteTransaction implements datastore.Transaction for SQLite
type sqliteTransaction struct {
	store *SQLiteStore
	tx    *sql.Tx
	ctx   context.Context
}

// ObjectStore methods

func (t *sqliteTransaction) GetObject(hash string) ([]byte, error) {
	t.store.reads.Add(1)
	
	var data []byte
	err := t.tx.QueryRow("SELECT data FROM objects WHERE hash = ?", hash).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, datastore.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	
	return data, nil
}

func (t *sqliteTransaction) PutObject(hash string, data []byte) error {
	t.store.writes.Add(1)
	
	objType := "blob" // Simplified
	_, err := t.tx.Exec("INSERT OR REPLACE INTO objects (hash, type, size, data) VALUES (?, ?, ?, ?)",
		hash, objType, len(data), data)
	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}
	
	return nil
}

func (t *sqliteTransaction) DeleteObject(hash string) error {
	t.store.deletes.Add(1)
	
	result, err := t.tx.Exec("DELETE FROM objects WHERE hash = ?", hash)
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

func (t *sqliteTransaction) HasObject(hash string) (bool, error) {
	t.store.reads.Add(1)
	
	var exists int
	err := t.tx.QueryRow("SELECT 1 FROM objects WHERE hash = ? LIMIT 1", hash).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check object: %w", err)
	}
	
	return exists == 1, nil
}

func (t *sqliteTransaction) ListObjects(prefix string, limit int) ([]string, error) {
	t.store.reads.Add(1)
	
	query := "SELECT hash FROM objects"
	args := []interface{}{}
	
	if prefix != "" {
		query += " WHERE hash LIKE ?"
		args = append(args, prefix+"%")
	}
	
	query += " ORDER BY hash"
	
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	
	rows, err := t.tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}
	defer rows.Close()
	
	var hashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("failed to scan hash: %w", err)
		}
		hashes = append(hashes, hash)
	}
	
	return hashes, rows.Err()
}

func (t *sqliteTransaction) IterateObjects(prefix string, fn func(hash string, data []byte) error) error {
	t.store.reads.Add(1)
	
	query := "SELECT hash, data FROM objects"
	args := []interface{}{}
	
	if prefix != "" {
		query += " WHERE hash LIKE ?"
		args = append(args, prefix+"%")
	}
	
	query += " ORDER BY hash"
	
	rows, err := t.tx.Query(query, args...)
	if err != nil {
		return fmt.Errorf("failed to query objects: %w", err)
	}
	defer rows.Close()
	
	for rows.Next() {
		var hash string
		var data []byte
		
		if err := rows.Scan(&hash, &data); err != nil {
			return fmt.Errorf("failed to scan object: %w", err)
		}
		
		if err := fn(hash, data); err != nil {
			return err
		}
	}
	
	return rows.Err()
}

func (t *sqliteTransaction) GetObjects(hashes []string) (map[string][]byte, error) {
	if len(hashes) == 0 {
		return map[string][]byte{}, nil
	}
	
	t.store.reads.Add(int64(len(hashes)))
	
	// Build IN clause
	query := "SELECT hash, data FROM objects WHERE hash IN ("
	args := make([]interface{}, len(hashes))
	for i, hash := range hashes {
		if i > 0 {
			query += ","
		}
		query += "?"
		args[i] = hash
	}
	query += ")"
	
	rows, err := t.tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get objects: %w", err)
	}
	defer rows.Close()
	
	result := make(map[string][]byte)
	for rows.Next() {
		var hash string
		var data []byte
		
		if err := rows.Scan(&hash, &data); err != nil {
			return nil, fmt.Errorf("failed to scan object: %w", err)
		}
		
		result[hash] = data
	}
	
	return result, rows.Err()
}

func (t *sqliteTransaction) PutObjects(objects map[string][]byte) error {
	if len(objects) == 0 {
		return nil
	}
	
	t.store.writes.Add(int64(len(objects)))
	
	stmt, err := t.tx.Prepare("INSERT OR REPLACE INTO objects (hash, type, size, data) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()
	
	for hash, data := range objects {
		objType := "blob" // Simplified
		
		if _, err := stmt.Exec(hash, objType, len(data), data); err != nil {
			return fmt.Errorf("failed to insert object %s: %w", hash, err)
		}
	}
	
	return nil
}

func (t *sqliteTransaction) DeleteObjects(hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	
	t.store.deletes.Add(int64(len(hashes)))
	
	stmt, err := t.tx.Prepare("DELETE FROM objects WHERE hash = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()
	
	for _, hash := range hashes {
		if _, err := stmt.Exec(hash); err != nil {
			return fmt.Errorf("failed to delete object %s: %w", hash, err)
		}
	}
	
	return nil
}

func (t *sqliteTransaction) GetObjectSize(hash string) (int64, error) {
	t.store.reads.Add(1)
	
	var size int64
	err := t.tx.QueryRow("SELECT size FROM objects WHERE hash = ?", hash).Scan(&size)
	if err == sql.ErrNoRows {
		return 0, datastore.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get object size: %w", err)
	}
	
	return size, nil
}

func (t *sqliteTransaction) CountObjects() (int64, error) {
	var count int64
	err := t.tx.QueryRow("SELECT COUNT(*) FROM objects").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count objects: %w", err)
	}
	
	return count, nil
}

func (t *sqliteTransaction) GetStorageSize() (int64, error) {
	var size sql.NullInt64
	err := t.tx.QueryRow("SELECT SUM(size) FROM objects").Scan(&size)
	if err != nil {
		return 0, fmt.Errorf("failed to get storage size: %w", err)
	}
	
	if !size.Valid {
		return 0, nil
	}
	
	return size.Int64, nil
}

// MetadataStore methods

func (t *sqliteTransaction) SaveRepository(repo *datastore.Repository) error {
	t.store.writes.Add(1)
	
	metadata, _ := json.Marshal(repo.Metadata)
	
	_, err := t.tx.Exec(
		"INSERT OR REPLACE INTO repositories (id, name, description, path, is_private, metadata, size, commit_count, branch_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
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

func (t *sqliteTransaction) GetRepository(id string) (*datastore.Repository, error) {
	t.store.reads.Add(1)
	
	repo := &datastore.Repository{}
	var metadata string
	
	err := t.tx.QueryRow(
		"SELECT id, name, description, path, is_private, metadata, size, commit_count, branch_count, created_at, updated_at FROM repositories WHERE id = ?",
		id,
	).Scan(
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

func (t *sqliteTransaction) ListRepositories(filter datastore.RepositoryFilter) ([]*datastore.Repository, error) {
	// Delegate to store for simplicity
	return t.store.ListRepositories(filter)
}

func (t *sqliteTransaction) DeleteRepository(id string) error {
	t.store.deletes.Add(1)
	
	result, err := t.tx.Exec("DELETE FROM repositories WHERE id = ?", id)
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

func (t *sqliteTransaction) UpdateRepository(id string, updates map[string]interface{}) error {
	repo, err := t.GetRepository(id)
	if err != nil {
		return err
	}
	
	// Apply updates
	for key, value := range updates {
		switch key {
		case "name":
			if v, ok := value.(string); ok {
				repo.Name = v
			}
		case "description":
			if v, ok := value.(string); ok {
				repo.Description = v
			}
		case "is_private":
			if v, ok := value.(bool); ok {
				repo.IsPrivate = v
			}
		case "metadata":
			if v, ok := value.(map[string]interface{}); ok {
				repo.Metadata = v
			}
		}
	}
	
	repo.UpdatedAt = time.Now()
	return t.SaveRepository(repo)
}

func (t *sqliteTransaction) SaveUser(user *datastore.User) error {
	t.store.writes.Add(1)
	
	metadata, _ := json.Marshal(user.Metadata)
	
	_, err := t.tx.Exec(
		"INSERT OR REPLACE INTO users (id, username, email, full_name, is_active, is_admin, metadata, created_at, updated_at, last_login_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		user.ID, user.Username, user.Email, user.FullName,
		user.IsActive, user.IsAdmin, string(metadata),
		user.CreatedAt, user.UpdatedAt, user.LastLoginAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}
	
	return nil
}

func (t *sqliteTransaction) GetUser(id string) (*datastore.User, error) {
	t.store.reads.Add(1)
	
	user := &datastore.User{}
	var metadata string
	var lastLogin sql.NullTime
	
	err := t.tx.QueryRow(
		"SELECT id, username, email, full_name, is_active, is_admin, metadata, created_at, updated_at, last_login_at FROM users WHERE id = ?",
		id,
	).Scan(
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

func (t *sqliteTransaction) GetUserByUsername(username string) (*datastore.User, error) {
	// Delegate to store for simplicity
	return t.store.GetUserByUsername(username)
}

func (t *sqliteTransaction) ListUsers(filter datastore.UserFilter) ([]*datastore.User, error) {
	// Delegate to store for simplicity
	return t.store.ListUsers(filter)
}

func (t *sqliteTransaction) DeleteUser(id string) error {
	t.store.deletes.Add(1)
	
	result, err := t.tx.Exec("DELETE FROM users WHERE id = ?", id)
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

func (t *sqliteTransaction) UpdateUser(id string, updates map[string]interface{}) error {
	user, err := t.GetUser(id)
	if err != nil {
		return err
	}
	
	// Apply updates
	for key, value := range updates {
		switch key {
		case "username":
			if v, ok := value.(string); ok {
				user.Username = v
			}
		case "email":
			if v, ok := value.(string); ok {
				user.Email = v
			}
		case "full_name":
			if v, ok := value.(string); ok {
				user.FullName = v
			}
		case "is_active":
			if v, ok := value.(bool); ok {
				user.IsActive = v
			}
		case "is_admin":
			if v, ok := value.(bool); ok {
				user.IsAdmin = v
			}
		case "metadata":
			if v, ok := value.(map[string]interface{}); ok {
				user.Metadata = v
			}
		}
	}
	
	user.UpdatedAt = time.Now()
	return t.SaveUser(user)
}

func (t *sqliteTransaction) SaveRef(repoID string, ref *datastore.Reference) error {
	t.store.writes.Add(1)
	
	_, err := t.tx.Exec(
		"INSERT OR REPLACE INTO refs (repository_id, name, hash, type, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?)",
		repoID, ref.Name, ref.Hash, ref.Type,
		ref.UpdatedAt, ref.UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to save ref: %w", err)
	}
	
	return nil
}

func (t *sqliteTransaction) GetRef(repoID string, name string) (*datastore.Reference, error) {
	t.store.reads.Add(1)
	
	ref := &datastore.Reference{}
	
	err := t.tx.QueryRow(
		"SELECT name, hash, type, updated_at, updated_by FROM refs WHERE repository_id = ? AND name = ?",
		repoID, name,
	).Scan(
		&ref.Name, &ref.Hash, &ref.Type,
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

func (t *sqliteTransaction) ListRefs(repoID string, refType datastore.RefType) ([]*datastore.Reference, error) {
	// Delegate to store for simplicity
	return t.store.ListRefs(repoID, refType)
}

func (t *sqliteTransaction) DeleteRef(repoID string, name string) error {
	t.store.deletes.Add(1)
	
	result, err := t.tx.Exec("DELETE FROM refs WHERE repository_id = ? AND name = ?", repoID, name)
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

func (t *sqliteTransaction) UpdateRef(repoID string, name string, newHash string) error {
	ref, err := t.GetRef(repoID, name)
	if err != nil {
		return err
	}
	
	ref.Hash = newHash
	ref.UpdatedAt = time.Now()
	return t.SaveRef(repoID, ref)
}

func (t *sqliteTransaction) LogEvent(event *datastore.AuditEvent) error {
	t.store.writes.Add(1)
	
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	details, _ := json.Marshal(event.Details)
	
	_, err := t.tx.Exec(
		"INSERT INTO audit_events (id, timestamp, user_id, username, action, resource, resource_id, details, ip_address, user_agent, success, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		event.ID, event.Timestamp, event.UserID, event.Username,
		event.Action, event.Resource, event.ResourceID, string(details),
		event.IPAddress, event.UserAgent, event.Success, event.ErrorMsg,
	)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}
	
	return nil
}

func (t *sqliteTransaction) QueryEvents(filter datastore.EventFilter) ([]*datastore.AuditEvent, error) {
	// Delegate to store for simplicity
	return t.store.QueryEvents(filter)
}

func (t *sqliteTransaction) CountEvents(filter datastore.EventFilter) (int64, error) {
	// Delegate to store for simplicity
	return t.store.CountEvents(filter)
}

func (t *sqliteTransaction) GetConfig(key string) (string, error) {
	t.store.reads.Add(1)
	
	var value string
	err := t.tx.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", datastore.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to get config: %w", err)
	}
	
	return value, nil
}

func (t *sqliteTransaction) SetConfig(key string, value string) error {
	t.store.writes.Add(1)
	
	_, err := t.tx.Exec("INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)", key, value)
	if err != nil {
		return fmt.Errorf("failed to set config: %w", err)
	}
	
	return nil
}

func (t *sqliteTransaction) GetAllConfig() (map[string]string, error) {
	// Delegate to store for simplicity
	return t.store.GetAllConfig()
}

func (t *sqliteTransaction) DeleteConfig(key string) error {
	t.store.deletes.Add(1)
	
	result, err := t.tx.Exec("DELETE FROM config WHERE key = ?", key)
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

// Transaction control

func (t *sqliteTransaction) Commit() error {
	return t.tx.Commit()
}

func (t *sqliteTransaction) Rollback() error {
	return t.tx.Rollback()
}

func (t *sqliteTransaction) ObjectStore() datastore.ObjectStore {
	return t
}

func (t *sqliteTransaction) MetadataStore() datastore.MetadataStore {
	return t
}