package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Caia-Tech/govc/datastore"
)

// memoryTransaction implements datastore.Transaction for memory store
type memoryTransaction struct {
	store     *MemoryStore
	ctx       context.Context
	opts      *datastore.TxOptions
	
	// Transaction state
	mutations map[string]interface{} // Pending changes
	deleted   map[string]bool        // Deleted keys
	committed bool
	rolledback bool
	mu        sync.Mutex
}

// Commit commits the transaction
func (tx *memoryTransaction) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return fmt.Errorf("transaction already committed")
	}
	if tx.rolledback {
		return fmt.Errorf("transaction already rolled back")
	}

	// Apply all mutations to the store
	for key, value := range tx.mutations {
		switch v := value.(type) {
		case []byte:
			// Object mutation
			if err := tx.store.ObjectStore().PutObject(key, v); err != nil {
				return err
			}
		case *datastore.Repository:
			// Repository mutation
			if err := tx.store.SaveRepository(v); err != nil {
				return err
			}
		case *datastore.User:
			// User mutation
			if err := tx.store.SaveUser(v); err != nil {
				return err
			}
		case *datastore.Reference:
			// Reference mutation
			parts := splitRefKey(key)
			if len(parts) == 2 {
				if err := tx.store.SaveRef(parts[0], v); err != nil {
					return err
				}
			}
		case *datastore.AuditEvent:
			// Audit event mutation
			if err := tx.store.LogEvent(v); err != nil {
				return err
			}
		case configValue:
			// Config mutation
			if err := tx.store.SetConfig(v.key, v.value); err != nil {
				return err
			}
		}
	}

	// Apply deletions
	for key, isDeleted := range tx.deleted {
		if !isDeleted {
			continue
		}
		
		// Determine type by key prefix
		if isObjectKey(key) {
			_ = tx.store.ObjectStore().DeleteObject(key)
		} else if isRepoKey(key) {
			_ = tx.store.DeleteRepository(key)
		} else if isUserKey(key) {
			_ = tx.store.DeleteUser(key)
		} else if isRefKey(key) {
			parts := splitRefKey(key)
			if len(parts) == 2 {
				_ = tx.store.DeleteRef(parts[0], parts[1])
			}
		} else if isConfigKey(key) {
			_ = tx.store.DeleteConfig(key)
		}
	}

	tx.committed = true
	return nil
}

// Rollback rolls back the transaction
func (tx *memoryTransaction) Rollback() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return fmt.Errorf("transaction already committed")
	}
	if tx.rolledback {
		return fmt.Errorf("transaction already rolled back")
	}

	// Clear all pending changes
	tx.mutations = make(map[string]interface{})
	tx.deleted = make(map[string]bool)
	tx.rolledback = true

	return nil
}

// ObjectStore methods - these queue changes instead of applying immediately

func (tx *memoryTransaction) GetObject(hash string) ([]byte, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	// Check if deleted in this transaction
	if tx.deleted[hash] {
		return nil, datastore.ErrNotFound
	}

	// Check mutations first
	if value, exists := tx.mutations[hash]; exists {
		if data, ok := value.([]byte); ok {
			return data, nil
		}
	}

	// Fall back to store
	return tx.store.ObjectStore().GetObject(hash)
}

func (tx *memoryTransaction) PutObject(hash string, data []byte) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledback {
		return fmt.Errorf("transaction is closed")
	}

	// Store in mutations
	tx.mutations[hash] = data
	delete(tx.deleted, hash)
	return nil
}

func (tx *memoryTransaction) DeleteObject(hash string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledback {
		return fmt.Errorf("transaction is closed")
	}

	tx.deleted[hash] = true
	delete(tx.mutations, hash)
	return nil
}

func (tx *memoryTransaction) HasObject(hash string) (bool, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.deleted[hash] {
		return false, nil
	}

	if _, exists := tx.mutations[hash]; exists {
		return true, nil
	}

	return tx.store.ObjectStore().HasObject(hash)
}

func (tx *memoryTransaction) ListObjects(prefix string, limit int) ([]string, error) {
	// For simplicity, we get from the store and filter based on transaction state
	results, err := tx.store.ObjectStore().ListObjects(prefix, limit)
	if err != nil {
		return nil, err
	}

	tx.mu.Lock()
	defer tx.mu.Unlock()

	// Filter out deleted items and add new ones
	filtered := make([]string, 0, len(results))
	for _, hash := range results {
		if !tx.deleted[hash] {
			filtered = append(filtered, hash)
		}
	}

	// Add items from mutations
	for hash := range tx.mutations {
		if isObjectKey(hash) && !contains(filtered, hash) {
			filtered = append(filtered, hash)
			if limit > 0 && len(filtered) >= limit {
				break
			}
		}
	}

	return filtered, nil
}

func (tx *memoryTransaction) IterateObjects(prefix string, fn func(hash string, data []byte) error) error {
	// This is simplified - a production implementation would be more sophisticated
	objects, err := tx.ListObjects(prefix, 0)
	if err != nil {
		return err
	}

	for _, hash := range objects {
		data, err := tx.GetObject(hash)
		if err != nil {
			continue // Skip missing objects
		}
		if err := fn(hash, data); err != nil {
			return err
		}
	}

	return nil
}

func (tx *memoryTransaction) GetObjects(hashes []string) (map[string][]byte, error) {
	results := make(map[string][]byte)
	for _, hash := range hashes {
		if data, err := tx.GetObject(hash); err == nil {
			results[hash] = data
		}
	}
	return results, nil
}

func (tx *memoryTransaction) PutObjects(objects map[string][]byte) error {
	for hash, data := range objects {
		if err := tx.PutObject(hash, data); err != nil {
			return err
		}
	}
	return nil
}

func (tx *memoryTransaction) DeleteObjects(hashes []string) error {
	for _, hash := range hashes {
		if err := tx.DeleteObject(hash); err != nil {
			return err
		}
	}
	return nil
}

func (tx *memoryTransaction) GetObjectSize(hash string) (int64, error) {
	data, err := tx.GetObject(hash)
	if err != nil {
		return 0, err
	}
	return int64(len(data)), nil
}

func (tx *memoryTransaction) CountObjects() (int64, error) {
	// This is a simplified implementation
	objects, err := tx.ListObjects("", 0)
	if err != nil {
		return 0, err
	}
	return int64(len(objects)), nil
}

func (tx *memoryTransaction) GetStorageSize() (int64, error) {
	// Calculate size including pending changes
	baseSize, err := tx.store.GetStorageSize()
	if err != nil {
		return 0, err
	}

	tx.mu.Lock()
	defer tx.mu.Unlock()

	// Add size of pending mutations
	for _, value := range tx.mutations {
		if data, ok := value.([]byte); ok {
			baseSize += int64(len(data))
		}
	}

	return baseSize, nil
}

// MetadataStore methods would follow the same pattern...
// Implementing SaveRepository, GetRepository, etc.

// The rest of the transaction methods follow the same pattern as above
// They queue changes in tx.mutations and tx.deleted instead of applying immediately

// Helper functions

type configValue struct {
	key   string
	value string
}

func isObjectKey(key string) bool {
	// Git object hashes are typically 40 chars (SHA-1) or 64 chars (SHA-256)
	return len(key) == 40 || len(key) == 64
}

func isRepoKey(key string) bool {
	// Repository IDs are UUIDs
	return len(key) == 36
}

func isUserKey(key string) bool {
	// User IDs are also UUIDs
	return len(key) == 36
}

func isRefKey(key string) bool {
	// Reference keys have format "repoID:refName"
	return len(splitRefKey(key)) == 2
}

func isConfigKey(key string) bool {
	// Config keys are arbitrary strings
	return true
}

func splitRefKey(key string) []string {
	parts := make([]string, 0, 2)
	if idx := strings.Index(key, ":"); idx > 0 {
		parts = append(parts, key[:idx], key[idx+1:])
	}
	return parts
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}