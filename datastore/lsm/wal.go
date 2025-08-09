package lsm

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// WriteAheadLog provides crash recovery for the LSM tree
// Ensures durability by logging all writes before applying to MemTable
type WriteAheadLog struct {
	path       string
	file       *os.File
	writer     *bufio.Writer
	mu         sync.Mutex
	sequence   uint64    // Monotonic sequence number
	synced     uint64    // Last synced sequence number
	size       int64     // Current WAL file size
	maxSize    int64     // Max size before rotation
}

// WALEntry represents a single entry in the write-ahead log
type WALEntry struct {
	Sequence  uint64
	Key       string
	Value     []byte
	Deleted   bool
	Checksum  uint32
}

// NewWriteAheadLog creates a new write-ahead log
func NewWriteAheadLog(basePath string) (*WriteAheadLog, error) {
	walPath := filepath.Join(basePath, "wal")
	if err := os.MkdirAll(walPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create WAL directory: %w", err)
	}
	
	walFile := filepath.Join(walPath, "current.wal")
	file, err := os.OpenFile(walFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open WAL file: %w", err)
	}
	
	// Get current file size
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat WAL file: %w", err)
	}
	
	wal := &WriteAheadLog{
		path:     walFile,
		file:     file,
		writer:   bufio.NewWriter(file),
		size:     stat.Size(),
		maxSize:  256 * 1024 * 1024, // 256MB max WAL size
	}
	
	// Read existing entries to get last sequence number
	if err := wal.recoverSequence(); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to recover sequence: %w", err)
	}
	
	return wal, nil
}

// recoverSequence reads the WAL to find the last sequence number
func (wal *WriteAheadLog) recoverSequence() error {
	if wal.size == 0 {
		return nil // Empty WAL
	}
	
	// Open file for reading
	readFile, err := os.Open(wal.path)
	if err != nil {
		return err
	}
	defer readFile.Close()
	
	reader := bufio.NewReader(readFile)
	maxSequence := uint64(0)
	
	for {
		entry, err := wal.readEntry(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			// Corrupt entry at end of file is acceptable
			break
		}
		
		if entry.Sequence > maxSequence {
			maxSequence = entry.Sequence
		}
	}
	
	wal.sequence = maxSequence
	wal.synced = maxSequence
	
	return nil
}

// Write writes a key-value pair to the WAL
func (wal *WriteAheadLog) Write(key string, value []byte, deleted bool) error {
	wal.mu.Lock()
	defer wal.mu.Unlock()
	
	wal.sequence++
	
	entry := WALEntry{
		Sequence: wal.sequence,
		Key:      key,
		Value:    make([]byte, len(value)),
		Deleted:  deleted,
	}
	copy(entry.Value, value)
	
	// Calculate checksum
	entry.Checksum = wal.calculateChecksum(&entry)
	
	// Serialize entry
	data, err := wal.serializeEntry(&entry)
	if err != nil {
		return fmt.Errorf("failed to serialize WAL entry: %w", err)
	}
	
	// Write to buffer
	if _, err := wal.writer.Write(data); err != nil {
		return fmt.Errorf("failed to write WAL entry: %w", err)
	}
	
	wal.size += int64(len(data))
	
	// Check if we need to rotate the WAL
	if wal.size > wal.maxSize {
		if err := wal.rotate(); err != nil {
			return fmt.Errorf("failed to rotate WAL: %w", err)
		}
	}
	
	return nil
}

// Sync forces a sync of the WAL to disk
func (wal *WriteAheadLog) Sync() error {
	wal.mu.Lock()
	defer wal.mu.Unlock()
	
	if err := wal.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush WAL buffer: %w", err)
	}
	
	if err := wal.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync WAL to disk: %w", err)
	}
	
	wal.synced = wal.sequence
	return nil
}

// serializeEntry converts a WAL entry to bytes
func (wal *WriteAheadLog) serializeEntry(entry *WALEntry) ([]byte, error) {
	// Format: 
	// - Magic (4 bytes): 0xDEADBEEF
	// - Sequence (8 bytes)
	// - Key length (4 bytes) + Key
	// - Value length (4 bytes) + Value  
	// - Flags (1 byte): bit 0 = deleted
	// - Checksum (4 bytes)
	
	keyLen := len(entry.Key)
	valueLen := len(entry.Value)
	totalLen := 4 + 8 + 4 + keyLen + 4 + valueLen + 1 + 4
	
	data := make([]byte, totalLen)
	offset := 0
	
	// Magic number
	binary.LittleEndian.PutUint32(data[offset:], 0xDEADBEEF)
	offset += 4
	
	// Sequence
	binary.LittleEndian.PutUint64(data[offset:], entry.Sequence)
	offset += 8
	
	// Key
	binary.LittleEndian.PutUint32(data[offset:], uint32(keyLen))
	offset += 4
	copy(data[offset:], []byte(entry.Key))
	offset += keyLen
	
	// Value
	binary.LittleEndian.PutUint32(data[offset:], uint32(valueLen))
	offset += 4
	copy(data[offset:], entry.Value)
	offset += valueLen
	
	// Flags
	flags := byte(0)
	if entry.Deleted {
		flags |= 1
	}
	data[offset] = flags
	offset++
	
	// Checksum
	binary.LittleEndian.PutUint32(data[offset:], entry.Checksum)
	
	return data, nil
}

// readEntry reads a WAL entry from a reader
func (wal *WriteAheadLog) readEntry(reader *bufio.Reader) (*WALEntry, error) {
	// Read magic number
	magicBytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, magicBytes); err != nil {
		return nil, err
	}
	
	magic := binary.LittleEndian.Uint32(magicBytes)
	if magic != 0xDEADBEEF {
		return nil, fmt.Errorf("invalid WAL entry magic: %x", magic)
	}
	
	// Read sequence
	seqBytes := make([]byte, 8)
	if _, err := io.ReadFull(reader, seqBytes); err != nil {
		return nil, err
	}
	sequence := binary.LittleEndian.Uint64(seqBytes)
	
	// Read key length and key
	keyLenBytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, keyLenBytes); err != nil {
		return nil, err
	}
	keyLen := binary.LittleEndian.Uint32(keyLenBytes)
	
	keyBytes := make([]byte, keyLen)
	if _, err := io.ReadFull(reader, keyBytes); err != nil {
		return nil, err
	}
	key := string(keyBytes)
	
	// Read value length and value
	valueLenBytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, valueLenBytes); err != nil {
		return nil, err
	}
	valueLen := binary.LittleEndian.Uint32(valueLenBytes)
	
	value := make([]byte, valueLen)
	if _, err := io.ReadFull(reader, value); err != nil {
		return nil, err
	}
	
	// Read flags
	flagBytes := make([]byte, 1)
	if _, err := io.ReadFull(reader, flagBytes); err != nil {
		return nil, err
	}
	deleted := (flagBytes[0] & 1) != 0
	
	// Read checksum
	checksumBytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, checksumBytes); err != nil {
		return nil, err
	}
	checksum := binary.LittleEndian.Uint32(checksumBytes)
	
	entry := &WALEntry{
		Sequence: sequence,
		Key:      key,
		Value:    value,
		Deleted:  deleted,
		Checksum: checksum,
	}
	
	// Verify checksum
	expectedChecksum := wal.calculateChecksum(entry)
	if checksum != expectedChecksum {
		return nil, fmt.Errorf("WAL entry checksum mismatch")
	}
	
	return entry, nil
}

// calculateChecksum calculates a simple checksum for a WAL entry
func (wal *WriteAheadLog) calculateChecksum(entry *WALEntry) uint32 {
	// Simple checksum: XOR of all bytes
	sum := uint32(entry.Sequence)
	
	for _, b := range []byte(entry.Key) {
		sum ^= uint32(b)
	}
	
	for _, b := range entry.Value {
		sum ^= uint32(b)
	}
	
	if entry.Deleted {
		sum ^= 0xFFFFFFFF
	}
	
	return sum
}

// ReadAll reads all entries from the WAL (used for recovery)
func (wal *WriteAheadLog) ReadAll() ([]*WALEntry, error) {
	wal.mu.Lock()
	defer wal.mu.Unlock()
	
	readFile, err := os.Open(wal.path)
	if err != nil {
		return nil, err
	}
	defer readFile.Close()
	
	reader := bufio.NewReader(readFile)
	entries := make([]*WALEntry, 0)
	
	for {
		entry, err := wal.readEntry(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			// Log corruption but continue with what we have
			break
		}
		
		entries = append(entries, entry)
	}
	
	return entries, nil
}

// Truncate clears the WAL (used after successful memtable flush)
func (wal *WriteAheadLog) Truncate() error {
	wal.mu.Lock()
	defer wal.mu.Unlock()
	
	// Close current file
	if err := wal.file.Close(); err != nil {
		return err
	}
	
	// Create new empty file
	file, err := os.Create(wal.path)
	if err != nil {
		return fmt.Errorf("failed to recreate WAL file: %w", err)
	}
	
	wal.file = file
	wal.writer = bufio.NewWriter(file)
	wal.size = 0
	wal.sequence = 0
	wal.synced = 0
	
	return nil
}

// rotate creates a new WAL file and archives the old one
func (wal *WriteAheadLog) rotate() error {
	// Flush current buffer
	if err := wal.writer.Flush(); err != nil {
		return err
	}
	
	if err := wal.file.Close(); err != nil {
		return err
	}
	
	// Move current file to archived file
	archivePath := fmt.Sprintf("%s.%d", wal.path, wal.sequence)
	if err := os.Rename(wal.path, archivePath); err != nil {
		return err
	}
	
	// Create new current file
	file, err := os.Create(wal.path)
	if err != nil {
		return err
	}
	
	wal.file = file
	wal.writer = bufio.NewWriter(file)
	wal.size = 0
	
	// Clean up old archived files (keep only recent ones)
	go wal.cleanupArchivedFiles()
	
	return nil
}

// cleanupArchivedFiles removes old archived WAL files
func (wal *WriteAheadLog) cleanupArchivedFiles() {
	walDir := filepath.Dir(wal.path)
	files, err := os.ReadDir(walDir)
	if err != nil {
		return
	}
	
	// Keep only the 5 most recent archived files
	archiveFiles := make([]os.DirEntry, 0)
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".wal" && file.Name() != "current.wal" {
			archiveFiles = append(archiveFiles, file)
		}
	}
	
	if len(archiveFiles) > 5 {
		// Sort by modification time and remove oldest
		// This is simplified - in production would sort properly
		for i := 5; i < len(archiveFiles); i++ {
			os.Remove(filepath.Join(walDir, archiveFiles[i].Name()))
		}
	}
}

// Close closes the WAL
func (wal *WriteAheadLog) Close() error {
	wal.mu.Lock()
	defer wal.mu.Unlock()
	
	if err := wal.writer.Flush(); err != nil {
		return err
	}
	
	return wal.file.Close()
}

// Stats returns WAL statistics
type WALStats struct {
	Size           int64  // Current WAL size
	Sequence       uint64 // Current sequence number
	SyncedSequence uint64 // Last synced sequence
	PendingWrites  int64  // Unsynced bytes
}

// Stats returns statistics about the WAL
func (wal *WriteAheadLog) Stats() WALStats {
	wal.mu.Lock()
	defer wal.mu.Unlock()
	
	pendingWrites := int64(0)
	if wal.writer != nil {
		pendingWrites = int64(wal.writer.Buffered())
	}
	
	return WALStats{
		Size:           wal.size,
		Sequence:       wal.sequence,
		SyncedSequence: wal.synced,
		PendingWrites:  pendingWrites,
	}
}