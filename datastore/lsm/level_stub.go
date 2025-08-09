package lsm

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Level represents a single level in the LSM tree - simplified stub
type Level struct {
	number   int
	path     string
	sstables []*SSTable
	config   LSMConfig
	mu       sync.RWMutex
	fileNum  int64
}

// LevelStats provides statistics about a level
type LevelStats struct {
	Level    int
	NumFiles int
	NumKeys  int64
	DiskSize int64
	MinKey   string
	MaxKey   string
}

// NewLevel creates a new level
func NewLevel(number int, basePath string, config LSMConfig) (*Level, error) {
	levelPath := filepath.Join(basePath, fmt.Sprintf("level-%d", number))
	if err := os.MkdirAll(levelPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create level directory: %w", err)
	}

	return &Level{
		number:   number,
		path:     levelPath,
		sstables: make([]*SSTable, 0),
		config:   config,
		fileNum:  0,
	}, nil
}

// Get retrieves a value from the level
func (l *Level) Get(key string) ([]byte, bool, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, sstable := range l.sstables {
		if sstable.bloomFilter != nil && !sstable.bloomFilter.MayContain(key) {
			continue
		}
		
		if key >= sstable.minKey && key <= sstable.maxKey {
			return sstable.Get(key)
		}
	}

	return nil, false, nil
}

// Stats returns statistics about the level
func (l *Level) Stats() LevelStats {
	l.mu.RLock()
	defer l.mu.RUnlock()

	stats := LevelStats{
		Level:    l.number,
		NumFiles: len(l.sstables),
	}

	for _, sstable := range l.sstables {
		stats.NumKeys += int64(sstable.numEntries)
		stats.DiskSize += sstable.size
		
		if stats.MinKey == "" || sstable.minKey < stats.MinKey {
			stats.MinKey = sstable.minKey
		}
		if stats.MaxKey == "" || sstable.maxKey > stats.MaxKey {
			stats.MaxKey = sstable.maxKey
		}
	}

	return stats
}

// NeedsCompaction returns true if the level needs compaction
func (l *Level) NeedsCompaction() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.number == 0 {
		return len(l.sstables) > l.config.Level0Files
	}
	
	// For other levels, check if size exceeds threshold
	totalSize := int64(0)
	for _, sstable := range l.sstables {
		totalSize += sstable.size
	}
	
	threshold := l.config.SSTableSize * int64(l.config.LevelMultiplier)
	return totalSize > threshold
}

// AddSSTable adds an SSTable to the level
func (l *Level) AddSSTable(sstable *SSTable) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	l.sstables = append(l.sstables, sstable)
	return nil
}

// RemoveSSTables removes SSTables from the level
func (l *Level) RemoveSSTables(sstables []*SSTable) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	// Simple removal - in production would be more efficient
	for _, toRemove := range sstables {
		for i, existing := range l.sstables {
			if existing == toRemove {
				l.sstables = append(l.sstables[:i], l.sstables[i+1:]...)
				break
			}
		}
	}
}

// SelectFilesForCompaction selects files for compaction
func (l *Level) SelectFilesForCompaction() []*SSTable {
	l.mu.RLock()
	defer l.mu.RUnlock()
	
	if len(l.sstables) == 0 {
		return nil
	}
	
	// Simple selection - return first few files
	maxFiles := 4
	if len(l.sstables) < maxFiles {
		maxFiles = len(l.sstables)
	}
	
	result := make([]*SSTable, maxFiles)
	copy(result, l.sstables[:maxFiles])
	return result
}

// CreateSSTable creates a new SSTable writer for this level
func (l *Level) CreateSSTable() (*SSTableWriter, error) {
	l.mu.Lock()
	l.fileNum++
	fileNum := l.fileNum
	l.mu.Unlock()
	
	return NewSSTableWriter(l.path, l.number, fileNum, l.config)
}

// Close closes the level and all its SSTables
func (l *Level) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	for _, sstable := range l.sstables {
		if err := sstable.Close(); err != nil {
			return err
		}
	}
	
	return nil
}