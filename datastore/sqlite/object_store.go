package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/Caia-Tech/govc/datastore"
)

// ListObjects lists objects with optional prefix filtering
func (s *SQLiteStore) ListObjects(prefix string, limit int) ([]string, error) {
	s.reads.Add(1)
	
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
	
	rows, err := s.db.Query(query, args...)
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

// IterateObjects iterates over all objects
func (s *SQLiteStore) IterateObjects(prefix string, fn func(hash string, data []byte) error) error {
	s.reads.Add(1)
	
	query := "SELECT hash, data FROM objects"
	args := []interface{}{}
	
	if prefix != "" {
		query += " WHERE hash LIKE ?"
		args = append(args, prefix+"%")
	}
	
	query += " ORDER BY hash"
	
	rows, err := s.db.Query(query, args...)
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

// GetObjects retrieves multiple objects at once
func (s *SQLiteStore) GetObjects(hashes []string) (map[string][]byte, error) {
	if len(hashes) == 0 {
		return map[string][]byte{}, nil
	}
	
	s.reads.Add(int64(len(hashes)))
	
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
	
	rows, err := s.db.Query(query, args...)
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

// PutObjects stores multiple objects at once
func (s *SQLiteStore) PutObjects(objects map[string][]byte) error {
	if len(objects) == 0 {
		return nil
	}
	
	s.writes.Add(int64(len(objects)))
	
	// Use transaction for batch insert
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	stmt, err := tx.Prepare("INSERT OR REPLACE INTO objects (hash, type, size, data) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()
	
	for hash, data := range objects {
		objType := "blob" // Simplified - should parse object type
		
		if _, err := stmt.Exec(hash, objType, len(data), data); err != nil {
			return fmt.Errorf("failed to insert object %s: %w", hash, err)
		}
	}
	
	return tx.Commit()
}

// DeleteObjects deletes multiple objects at once
func (s *SQLiteStore) DeleteObjects(hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	
	s.deletes.Add(int64(len(hashes)))
	
	// Use transaction for batch delete
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	stmt, err := tx.Prepare("DELETE FROM objects WHERE hash = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()
	
	for _, hash := range hashes {
		if _, err := stmt.Exec(hash); err != nil {
			return fmt.Errorf("failed to delete object %s: %w", hash, err)
		}
	}
	
	return tx.Commit()
}

// GetObjectSize returns the size of an object
func (s *SQLiteStore) GetObjectSize(hash string) (int64, error) {
	s.reads.Add(1)
	
	var size int64
	err := s.db.QueryRow("SELECT size FROM objects WHERE hash = ?", hash).Scan(&size)
	if err == sql.ErrNoRows {
		return 0, datastore.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get object size: %w", err)
	}
	
	return size, nil
}

// CountObjects returns the total number of objects
func (s *SQLiteStore) CountObjects() (int64, error) {
	var count int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM objects").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count objects: %w", err)
	}
	
	return count, nil
}

// GetStorageSize returns the total storage size used
func (s *SQLiteStore) GetStorageSize() (int64, error) {
	var size sql.NullInt64
	err := s.db.QueryRow("SELECT SUM(size) FROM objects").Scan(&size)
	if err != nil {
		return 0, fmt.Errorf("failed to get storage size: %w", err)
	}
	
	if !size.Valid {
		return 0, nil
	}
	
	return size.Int64, nil
}

// Vacuum optimizes the database
func (s *SQLiteStore) Vacuum() error {
	_, err := s.db.Exec("VACUUM")
	if err != nil {
		return fmt.Errorf("failed to vacuum database: %w", err)
	}
	return nil
}

// objectTransaction wraps SQLite transaction for ObjectStore operations
type objectTransaction struct {
	store *SQLiteStore
	tx    *sql.Tx
}

func (t *objectTransaction) GetObject(hash string) ([]byte, error) {
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

func (t *objectTransaction) PutObject(hash string, data []byte) error {
	t.store.writes.Add(1)
	
	objType := "blob" // Simplified
	_, err := t.tx.Exec("INSERT OR REPLACE INTO objects (hash, type, size, data) VALUES (?, ?, ?, ?)",
		hash, objType, len(data), data)
	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}
	
	return nil
}

func (t *objectTransaction) DeleteObject(hash string) error {
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

func (t *objectTransaction) HasObject(hash string) (bool, error) {
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