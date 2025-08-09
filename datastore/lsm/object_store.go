package lsm

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/caiatech/govc/datastore"
)

// ObjectStore implements the datastore.ObjectStore interface for LSM tree
type ObjectStore struct {
	lsm *LSMStore
}

// GetObject retrieves an object by hash using LSM tree read path
func (os *ObjectStore) GetObject(hash string) ([]byte, error) {
	if hash == "" {
		return nil, datastore.ErrInvalidData
	}
	
	atomic.AddInt64(&os.lsm.metrics.Reads, 1)
	
	// LSM Read Path: MemTable -> Immutable MemTable -> L0 -> L1 -> ... -> LN
	
	// 1. Check active MemTable first (newest data)
	os.lsm.mu.RLock()
	memTable := os.lsm.memTable
	os.lsm.mu.RUnlock()
	
	if memTable != nil {
		if value, found := memTable.Get(hash); found {
			return value, nil
		}
	}
	
	// 2. Check immutable MemTable
	os.lsm.mu.RLock()
	immutable := os.lsm.immutable
	os.lsm.mu.RUnlock()
	
	if immutable != nil {
		if value, found := immutable.Get(hash); found {
			return value, nil
		}
	}
	
	// 3. Check SSTables in all levels (L0 to LN)
	os.lsm.mu.RLock()
	levels := os.lsm.levels
	os.lsm.mu.RUnlock()
	
	for _, level := range levels {
		if level != nil {
			value, found, err := level.Get(hash)
			if err != nil {
				return nil, fmt.Errorf("error reading from level %d: %w", level.number, err)
			}
			if found {
				return value, nil
			}
		}
	}
	
	return nil, datastore.ErrNotFound
}

// PutObject stores an object using LSM tree write path
func (os *ObjectStore) PutObject(hash string, data []byte) error {
	if hash == "" {
		return datastore.ErrInvalidData
	}
	
	atomic.AddInt64(&os.lsm.metrics.Writes, 1)
	
	// LSM Write Path: WAL -> MemTable -> (flush when full)
	
	// 1. Write to WAL first for durability
	if os.lsm.wal != nil {
		if err := os.lsm.wal.Write(hash, data, false); err != nil {
			return fmt.Errorf("failed to write to WAL: %w", err)
		}
	}
	
	// 2. Write to MemTable
	os.lsm.mu.RLock()
	memTable := os.lsm.memTable
	os.lsm.mu.RUnlock()
	
	if memTable == nil {
		return fmt.Errorf("memtable not initialized")
	}
	
	if err := memTable.Put(hash, data); err != nil {
		return fmt.Errorf("failed to write to memtable: %w", err)
	}
	
	// 3. Check if MemTable needs flushing
	if memTable.IsFull() {
		// Trigger async flush
		select {
		case os.lsm.flushCh <- struct{}{}:
		default:
			// Channel full, flush will happen soon
		}
	}
	
	return nil
}

// DeleteObject marks an object as deleted (tombstone)
func (os *ObjectStore) DeleteObject(hash string) error {
	if hash == "" {
		return datastore.ErrInvalidData
	}
	
	atomic.AddInt64(&os.lsm.metrics.Deletes, 1)
	
	// Check if object exists first
	_, err := os.GetObject(hash)
	if err == datastore.ErrNotFound {
		return datastore.ErrNotFound
	} else if err != nil {
		return err
	}
	
	// Write tombstone to WAL
	if os.lsm.wal != nil {
		if err := os.lsm.wal.Write(hash, nil, true); err != nil {
			return fmt.Errorf("failed to write tombstone to WAL: %w", err)
		}
	}
	
	// Write tombstone to MemTable
	os.lsm.mu.RLock()
	memTable := os.lsm.memTable
	os.lsm.mu.RUnlock()
	
	if memTable == nil {
		return fmt.Errorf("memtable not initialized")
	}
	
	if err := memTable.Delete(hash); err != nil {
		return fmt.Errorf("failed to write tombstone to memtable: %w", err)
	}
	
	// Check if MemTable needs flushing
	if memTable.IsFull() {
		select {
		case os.lsm.flushCh <- struct{}{}:
		default:
		}
	}
	
	return nil
}

// HasObject checks if an object exists
func (os *ObjectStore) HasObject(hash string) (bool, error) {
	if hash == "" {
		return false, datastore.ErrInvalidData
	}
	
	_, err := os.GetObject(hash)
	if err == datastore.ErrNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}
	
	return true, nil
}

// ListObjects lists objects matching a prefix with optional limit
func (os *ObjectStore) ListObjects(prefix string, limit int) ([]string, error) {
	var hashes []string
	count := 0
	
	// Create a set to deduplicate keys across levels
	seen := make(map[string]bool)
	
	// Collect from MemTable
	os.lsm.mu.RLock()
	memTable := os.lsm.memTable
	os.lsm.mu.RUnlock()
	
	if memTable != nil {
		memTable.Range(prefix, "", func(key string, value []byte, deleted bool) bool {
			if !deleted && !seen[key] && (prefix == "" || strings.HasPrefix(key, prefix)) {
				seen[key] = true
				hashes = append(hashes, key)
				count++
				return limit == 0 || count < limit
			}
			return true
		})
	}
	
	// Collect from immutable MemTable
	os.lsm.mu.RLock()
	immutable := os.lsm.immutable
	os.lsm.mu.RUnlock()
	
	if immutable != nil && (limit == 0 || count < limit) {
		immutable.Range(prefix, "", func(key string, value []byte, deleted bool) bool {
			if !deleted && !seen[key] && (prefix == "" || strings.HasPrefix(key, prefix)) {
				seen[key] = true
				hashes = append(hashes, key)
				count++
				return limit == 0 || count < limit
			}
			return true
		})
	}
	
	// Collect from SSTables (would need proper implementation with iterators)
	// This is simplified - real implementation would use iterators
	os.lsm.mu.RLock()
	levels := os.lsm.levels
	os.lsm.mu.RUnlock()
	
	for _, level := range levels {
		if level != nil && (limit == 0 || count < limit) {
			// This would use level iterators in real implementation
			stats := level.Stats()
			// For now, just return what we have from memory
			if stats.NumKeys > 0 {
				break
			}
		}
	}
	
	// Sort results
	if len(hashes) > 1 {
		// Simple sort - real implementation would maintain sorted order
		for i := 0; i < len(hashes)-1; i++ {
			for j := i + 1; j < len(hashes); j++ {
				if hashes[i] > hashes[j] {
					hashes[i], hashes[j] = hashes[j], hashes[i]
				}
			}
		}
	}
	
	return hashes, nil
}

// IterateObjects iterates over objects matching a prefix
func (os *ObjectStore) IterateObjects(prefix string, fn func(hash string, data []byte) error) error {
	// Get list of keys first
	keys, err := os.ListObjects(prefix, 0)
	if err != nil {
		return err
	}
	
	// Iterate through keys and call function
	for _, key := range keys {
		data, err := os.GetObject(key)
		if err == datastore.ErrNotFound {
			continue // Key was deleted between list and get
		} else if err != nil {
			return err
		}
		
		if err := fn(key, data); err != nil {
			return err
		}
	}
	
	return nil
}

// GetObjects retrieves multiple objects efficiently
func (os *ObjectStore) GetObjects(hashes []string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	
	for _, hash := range hashes {
		if hash == "" {
			continue
		}
		
		data, err := os.GetObject(hash)
		if err == datastore.ErrNotFound {
			continue // Skip missing objects
		} else if err != nil {
			return nil, err
		}
		
		result[hash] = data
	}
	
	return result, nil
}

// PutObjects stores multiple objects efficiently
func (os *ObjectStore) PutObjects(objects map[string][]byte) error {
	for hash, data := range objects {
		if hash == "" {
			continue
		}
		
		if err := os.PutObject(hash, data); err != nil {
			return fmt.Errorf("failed to put object %s: %w", hash, err)
		}
	}
	
	// Sync WAL after batch
	if os.lsm.wal != nil {
		if err := os.lsm.wal.Sync(); err != nil {
			return fmt.Errorf("failed to sync WAL: %w", err)
		}
	}
	
	return nil
}

// DeleteObjects deletes multiple objects efficiently
func (os *ObjectStore) DeleteObjects(hashes []string) error {
	for _, hash := range hashes {
		if hash == "" {
			continue
		}
		
		if err := os.DeleteObject(hash); err != nil && err != datastore.ErrNotFound {
			return fmt.Errorf("failed to delete object %s: %w", hash, err)
		}
	}
	
	// Sync WAL after batch
	if os.lsm.wal != nil {
		if err := os.lsm.wal.Sync(); err != nil {
			return fmt.Errorf("failed to sync WAL: %w", err)
		}
	}
	
	return nil
}

// GetObjectSize returns the size of an object
func (os *ObjectStore) GetObjectSize(hash string) (int64, error) {
	data, err := os.GetObject(hash)
	if err != nil {
		return 0, err
	}
	
	return int64(len(data)), nil
}

// CountObjects returns the approximate number of objects
func (os *ObjectStore) CountObjects() (int64, error) {
	count := int64(0)
	
	// Count from MemTable
	os.lsm.mu.RLock()
	if os.lsm.memTable != nil {
		count += int64(os.lsm.memTable.Size())
	}
	if os.lsm.immutable != nil {
		count += int64(os.lsm.immutable.Size())
	}
	os.lsm.mu.RUnlock()
	
	// Count from levels
	os.lsm.mu.RLock()
	levels := os.lsm.levels
	os.lsm.mu.RUnlock()
	
	for _, level := range levels {
		if level != nil {
			stats := level.Stats()
			count += stats.NumKeys
		}
	}
	
	// Note: This might double-count due to duplicates across levels
	// Real implementation would use merge iterator to get exact count
	
	return count, nil
}

// GetStorageSize returns the approximate storage size
func (os *ObjectStore) GetStorageSize() (int64, error) {
	size := int64(0)
	
	// Size from MemTables
	os.lsm.mu.RLock()
	if os.lsm.memTable != nil {
		size += os.lsm.memTable.MemoryUsage()
	}
	if os.lsm.immutable != nil {
		size += os.lsm.immutable.MemoryUsage()
	}
	os.lsm.mu.RUnlock()
	
	// Size from SSTables
	os.lsm.mu.RLock()
	levels := os.lsm.levels
	os.lsm.mu.RUnlock()
	
	for _, level := range levels {
		if level != nil {
			stats := level.Stats()
			size += stats.DiskSize
		}
	}
	
	return size, nil
}

// flushMemTable handles memtable flushing to L0
func (os *ObjectStore) flushMemTable() error {
	os.lsm.flushMu.Lock()
	defer os.lsm.flushMu.Unlock()
	
	// Get current memtable
	os.lsm.mu.Lock()
	currentMemTable := os.lsm.memTable
	if currentMemTable == nil {
		os.lsm.mu.Unlock()
		return nil
	}
	
	// Create new memtable and make current one immutable
	newMemTable := NewMemTable(currentMemTable.maxSize)
	os.lsm.memTable = newMemTable
	os.lsm.immutable = currentMemTable
	currentMemTable.Seal()
	os.lsm.mu.Unlock()
	
	// Flush immutable memtable to L0
	level0 := os.lsm.levels[0]
	writer, err := level0.CreateSSTable()
	if err != nil {
		return fmt.Errorf("failed to create SSTable writer: %w", err)
	}
	
	// Write all entries from memtable
	iterator := currentMemTable.NewIterator()
	for iterator.Valid() {
		entry := iterator.Entry()
		err := writer.Add(entry.Key, entry.Value, entry.Deleted)
		if err != nil {
			return fmt.Errorf("failed to add entry to SSTable: %w", err)
		}
		iterator.Next()
	}
	
	// Finish SSTable
	sstable, err := writer.Finish()
	if err != nil {
		return fmt.Errorf("failed to finish SSTable: %w", err)
	}
	
	// Add to L0
	err = level0.AddSSTable(sstable)
	if err != nil {
		return fmt.Errorf("failed to add SSTable to L0: %w", err)
	}
	
	// Clear immutable memtable and WAL
	os.lsm.mu.Lock()
	os.lsm.immutable = nil
	os.lsm.mu.Unlock()
	
	if os.lsm.wal != nil {
		if err := os.lsm.wal.Truncate(); err != nil {
			return fmt.Errorf("failed to truncate WAL: %w", err)
		}
	}
	
	// Trigger compaction if needed
	if level0.NeedsCompaction() {
		select {
		case os.lsm.compactCh <- 0:
		default:
		}
	}
	
	return nil
}

// compactLevel handles level compaction
func (os *ObjectStore) compactLevel(level int) error {
	os.lsm.compactionMu.Lock()
	defer os.lsm.compactionMu.Unlock()
	
	if level >= len(os.lsm.levels)-1 {
		return nil // No next level to compact into
	}
	
	sourceLevel := os.lsm.levels[level]
	targetLevel := os.lsm.levels[level+1]
	
	if sourceLevel == nil || targetLevel == nil {
		return nil
	}
	
	// Use compactor to perform the actual compaction
	return os.lsm.compactor.compactLevel(sourceLevel, targetLevel)
}

// Helper functions for the main LSM store

// flushMemTable is called by the LSM store
func (lsm *LSMStore) flushMemTable() error {
	return lsm.ObjectStore().(*ObjectStore).flushMemTable()
}

// compactLevel is called by the LSM store
func (lsm *LSMStore) compactLevel(level int) error {
	return lsm.ObjectStore().(*ObjectStore).compactLevel(level)
}

// GitObjectMetrics provides Git-specific metrics
type GitObjectMetrics struct {
	BlobObjects   int64
	TreeObjects   int64
	CommitObjects int64
	TagObjects    int64
}

// GetGitObjectMetrics returns Git object type breakdown
func (os *ObjectStore) GetGitObjectMetrics() GitObjectMetrics {
	// This would analyze object types in a real implementation
	// For now, return empty metrics
	return GitObjectMetrics{}
}

// OptimizeForGit performs Git-specific optimizations
func (os *ObjectStore) OptimizeForGit() error {
	// This would implement Git-specific optimizations like:
	// - Delta compression for similar objects
	// - Pack file generation
	// - Reference counting for garbage collection
	// - Commit chain optimization
	
	return nil
}

// GarbageCollect removes unreferenced objects
func (os *ObjectStore) GarbageCollect() error {
	// This would implement garbage collection:
	// - Mark phase: find reachable objects from refs
	// - Sweep phase: remove unreachable objects
	// - Compact: reorganize storage
	
	return nil
}