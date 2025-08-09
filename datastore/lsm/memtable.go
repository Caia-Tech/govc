package lsm

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemTable is an in-memory write buffer that maintains sorted key-value pairs
// Optimized for Git objects with content-addressable keys (SHA hashes)
type MemTable struct {
	data          map[string]*Entry
	maxSize       int64
	currentSize   int64
	mu            sync.RWMutex
	createdAt     time.Time
	sealed        bool  // When true, no more writes allowed
}

// Entry represents a key-value entry in the MemTable
type Entry struct {
	Key       string
	Value     []byte
	Timestamp time.Time
	Deleted   bool    // Tombstone marker
	Size      int64   // Memory footprint
}

// NewMemTable creates a new MemTable with the specified maximum size
func NewMemTable(maxSize int64) *MemTable {
	return &MemTable{
		data:        make(map[string]*Entry),
		maxSize:     maxSize,
		currentSize: 0,
		createdAt:   time.Now(),
		sealed:      false,
	}
}

// Put inserts or updates a key-value pair in the MemTable
func (mt *MemTable) Put(key string, value []byte) error {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	
	if mt.sealed {
		return fmt.Errorf("cannot write to sealed memtable")
	}
	
	// Calculate entry size
	entrySize := int64(len(key) + len(value) + 64) // Approximate overhead
	
	// Check if we have existing entry
	if existing, exists := mt.data[key]; exists {
		mt.currentSize -= existing.Size
	}
	
	// Create new entry
	entry := &Entry{
		Key:       key,
		Value:     make([]byte, len(value)),
		Timestamp: time.Now(),
		Deleted:   false,
		Size:      entrySize,
	}
	copy(entry.Value, value)
	
	// Update table
	mt.data[key] = entry
	mt.currentSize += entrySize
	
	return nil
}

// Get retrieves a value by key from the MemTable
func (mt *MemTable) Get(key string) ([]byte, bool) {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	
	entry, exists := mt.data[key]
	if !exists || entry.Deleted {
		return nil, false
	}
	
	// Return copy of value to prevent external modification
	result := make([]byte, len(entry.Value))
	copy(result, entry.Value)
	return result, true
}

// Delete marks a key as deleted (tombstone)
func (mt *MemTable) Delete(key string) error {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	
	if mt.sealed {
		return fmt.Errorf("cannot write to sealed memtable")
	}
	
	// Create tombstone entry
	tombstone := &Entry{
		Key:       key,
		Value:     nil,
		Timestamp: time.Now(),
		Deleted:   true,
		Size:      int64(len(key) + 64), // Approximate overhead
	}
	
	// Update size calculation
	if existing, exists := mt.data[key]; exists {
		mt.currentSize -= existing.Size
	}
	
	mt.data[key] = tombstone
	mt.currentSize += tombstone.Size
	
	return nil
}

// Has checks if a key exists (and is not deleted)
func (mt *MemTable) Has(key string) bool {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	
	entry, exists := mt.data[key]
	return exists && !entry.Deleted
}

// Size returns the number of entries in the MemTable
func (mt *MemTable) Size() int {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return len(mt.data)
}

// MemoryUsage returns the current memory usage in bytes
func (mt *MemTable) MemoryUsage() int64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.currentSize
}

// IsFull checks if the MemTable has reached its maximum size
func (mt *MemTable) IsFull() bool {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.currentSize >= mt.maxSize
}

// Seal marks the MemTable as read-only (no more writes allowed)
func (mt *MemTable) Seal() {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	mt.sealed = true
}

// IsSealed returns true if the MemTable is sealed
func (mt *MemTable) IsSealed() bool {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.sealed
}

// Iterator provides ordered iteration over the MemTable entries
type MemTableIterator struct {
	entries []*Entry
	index   int
}

// NewIterator creates a new iterator for the MemTable
func (mt *MemTable) NewIterator() *MemTableIterator {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	
	// Create sorted slice of entries
	entries := make([]*Entry, 0, len(mt.data))
	for _, entry := range mt.data {
		entries = append(entries, entry)
	}
	
	// Sort by key (Git hashes sort lexicographically well)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})
	
	return &MemTableIterator{
		entries: entries,
		index:   0,
	}
}

// Valid returns true if the iterator is positioned at a valid entry
func (it *MemTableIterator) Valid() bool {
	return it.index < len(it.entries)
}

// Next advances the iterator to the next entry
func (it *MemTableIterator) Next() {
	it.index++
}

// Key returns the current key
func (it *MemTableIterator) Key() string {
	if !it.Valid() {
		return ""
	}
	return it.entries[it.index].Key
}

// Value returns the current value
func (it *MemTableIterator) Value() []byte {
	if !it.Valid() {
		return nil
	}
	return it.entries[it.index].Value
}

// Entry returns the current entry
func (it *MemTableIterator) Entry() *Entry {
	if !it.Valid() {
		return nil
	}
	return it.entries[it.index]
}

// Seek positions the iterator at the first key >= target
func (it *MemTableIterator) Seek(target string) {
	// Binary search for the target key
	left, right := 0, len(it.entries)
	for left < right {
		mid := (left + right) / 2
		if it.entries[mid].Key < target {
			left = mid + 1
		} else {
			right = mid
		}
	}
	it.index = left
}

// SeekToFirst positions the iterator at the first key
func (it *MemTableIterator) SeekToFirst() {
	it.index = 0
}

// SeekToLast positions the iterator at the last key
func (it *MemTableIterator) SeekToLast() {
	it.index = len(it.entries) - 1
}

// MemTableStats provides statistics about the MemTable
type MemTableStats struct {
	NumEntries    int
	MemoryUsage   int64
	MaxSize       int64
	AverageKeySize float64
	AverageValueSize float64
	CreatedAt     time.Time
	Sealed        bool
}

// Stats returns statistics about the MemTable
func (mt *MemTable) Stats() MemTableStats {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	
	stats := MemTableStats{
		NumEntries:  len(mt.data),
		MemoryUsage: mt.currentSize,
		MaxSize:     mt.maxSize,
		CreatedAt:   mt.createdAt,
		Sealed:      mt.sealed,
	}
	
	if len(mt.data) > 0 {
		totalKeySize := int64(0)
		totalValueSize := int64(0)
		
		for _, entry := range mt.data {
			totalKeySize += int64(len(entry.Key))
			if !entry.Deleted {
				totalValueSize += int64(len(entry.Value))
			}
		}
		
		stats.AverageKeySize = float64(totalKeySize) / float64(len(mt.data))
		stats.AverageValueSize = float64(totalValueSize) / float64(len(mt.data))
	}
	
	return stats
}

// Flush prepares the MemTable for writing to disk
// Returns a sorted iterator for efficient SSTable creation
func (mt *MemTable) Flush() (*MemTableIterator, error) {
	mt.mu.RLock()
	sealed := mt.sealed
	mt.mu.RUnlock()
	
	if !sealed {
		return nil, fmt.Errorf("cannot flush unsealed memtable")
	}
	
	return mt.NewIterator(), nil
}

// Range iterates over keys in a specific range [start, end)
func (mt *MemTable) Range(start, end string, fn func(key string, value []byte, deleted bool) bool) {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	
	// Create sorted keys for range iteration
	keys := make([]string, 0, len(mt.data))
	for key := range mt.data {
		if (start == "" || key >= start) && (end == "" || key < end) {
			keys = append(keys, key)
		}
	}
	
	sort.Strings(keys)
	
	for _, key := range keys {
		entry := mt.data[key]
		if !fn(key, entry.Value, entry.Deleted) {
			break
		}
	}
}

// Merge merges multiple MemTables during compaction
// This is used when multiple memtables need to be combined
func MergeMemTables(tables []*MemTable) (*MemTable, error) {
	if len(tables) == 0 {
		return nil, fmt.Errorf("no memtables to merge")
	}
	
	// Calculate total size
	totalSize := int64(0)
	for _, table := range tables {
		totalSize += table.maxSize
	}
	
	merged := NewMemTable(totalSize)
	merged.Seal() // Result is read-only
	
	// Merge all entries, with later writes taking precedence
	allEntries := make(map[string]*Entry)
	
	for _, table := range tables {
		table.mu.RLock()
		for key, entry := range table.data {
			if existing, exists := allEntries[key]; !exists || entry.Timestamp.After(existing.Timestamp) {
				// Copy entry to avoid sharing references
				newEntry := &Entry{
					Key:       entry.Key,
					Value:     make([]byte, len(entry.Value)),
					Timestamp: entry.Timestamp,
					Deleted:   entry.Deleted,
					Size:      entry.Size,
				}
				copy(newEntry.Value, entry.Value)
				allEntries[key] = newEntry
			}
		}
		table.mu.RUnlock()
	}
	
	// Add merged entries to result
	for _, entry := range allEntries {
		merged.data[entry.Key] = entry
		merged.currentSize += entry.Size
	}
	
	return merged, nil
}