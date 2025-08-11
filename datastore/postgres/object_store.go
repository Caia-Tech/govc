package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/Caia-Tech/govc/datastore"
)

// ObjectStore implementation

// GetObject retrieves an object by hash
func (s *PostgresStore) GetObject(hash string) ([]byte, error) {
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
func (s *PostgresStore) PutObject(hash string, data []byte) error {
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
func (s *PostgresStore) DeleteObject(hash string) error {
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
func (s *PostgresStore) HasObject(hash string) (bool, error) {
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

// ListObjects lists objects with optional prefix filtering
func (s *PostgresStore) ListObjects(prefix string, limit int) ([]string, error) {
	s.reads.Add(1)
	
	query := "SELECT hash FROM govc.objects"
	args := []interface{}{}
	argCount := 0
	
	if prefix != "" {
		argCount++
		query += fmt.Sprintf(" WHERE hash LIKE $%d", argCount)
		args = append(args, prefix+"%")
	}
	
	query += " ORDER BY hash"
	
	if limit > 0 {
		argCount++
		query += fmt.Sprintf(" LIMIT $%d", argCount)
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
func (s *PostgresStore) IterateObjects(prefix string, fn func(hash string, data []byte) error) error {
	s.reads.Add(1)
	
	query := "SELECT hash, data FROM govc.objects"
	args := []interface{}{}
	
	if prefix != "" {
		query += " WHERE hash LIKE $1"
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
func (s *PostgresStore) GetObjects(hashes []string) (map[string][]byte, error) {
	if len(hashes) == 0 {
		return map[string][]byte{}, nil
	}
	
	s.reads.Add(int64(len(hashes)))
	
	// Build query with placeholders
	placeholders := make([]string, len(hashes))
	args := make([]interface{}, len(hashes))
	for i, hash := range hashes {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = hash
	}
	
	query := fmt.Sprintf("SELECT hash, data FROM govc.objects WHERE hash IN (%s)",
		strings.Join(placeholders, ","))
	
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
func (s *PostgresStore) PutObjects(objects map[string][]byte) error {
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
	
	stmt, err := tx.Prepare("INSERT INTO govc.objects (hash, type, size, data) VALUES ($1, $2, $3, $4) ON CONFLICT (hash) DO NOTHING")
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
	
	return tx.Commit()
}

// DeleteObjects deletes multiple objects at once
func (s *PostgresStore) DeleteObjects(hashes []string) error {
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
	
	stmt, err := tx.Prepare("DELETE FROM govc.objects WHERE hash = $1")
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
func (s *PostgresStore) GetObjectSize(hash string) (int64, error) {
	s.reads.Add(1)
	
	var size int64
	err := s.db.QueryRow("SELECT size FROM govc.objects WHERE hash = $1", hash).Scan(&size)
	if err == sql.ErrNoRows {
		return 0, datastore.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get object size: %w", err)
	}
	
	return size, nil
}

// CountObjects returns the total number of objects
func (s *PostgresStore) CountObjects() (int64, error) {
	var count int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM govc.objects").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count objects: %w", err)
	}
	
	return count, nil
}

// GetStorageSize returns the total storage size used
func (s *PostgresStore) GetStorageSize() (int64, error) {
	var size sql.NullInt64
	err := s.db.QueryRow("SELECT COALESCE(SUM(size), 0) FROM govc.objects").Scan(&size)
	if err != nil {
		return 0, fmt.Errorf("failed to get storage size: %w", err)
	}
	
	if !size.Valid {
		return 0, nil
	}
	
	return size.Int64, nil
}

// Vacuum optimizes the database
func (s *PostgresStore) Vacuum() error {
	// VACUUM ANALYZE for PostgreSQL
	_, err := s.db.Exec("VACUUM ANALYZE govc.objects")
	if err != nil {
		return fmt.Errorf("failed to vacuum: %w", err)
	}
	
	// Also update statistics
	_, err = s.db.Exec("ANALYZE govc.objects")
	if err != nil {
		return fmt.Errorf("failed to analyze: %w", err)
	}
	
	return nil
}