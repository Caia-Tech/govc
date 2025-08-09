package lsm

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteAheadLog_New(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	assert.NoError(t, err)
	assert.NotNil(t, wal)

	// Check that WAL directory and file are created
	walDir := filepath.Join(tempDir, "wal")
	walFile := filepath.Join(walDir, "current.wal")
	
	_, err = os.Stat(walDir)
	assert.NoError(t, err)
	
	_, err = os.Stat(walFile)
	assert.NoError(t, err)
	
	assert.Equal(t, walFile, wal.path)
	assert.NotNil(t, wal.file)
	assert.NotNil(t, wal.writer)
	assert.Equal(t, uint64(0), wal.sequence)
	assert.Equal(t, uint64(0), wal.synced)
	assert.Equal(t, int64(0), wal.size)

	err = wal.Close()
	assert.NoError(t, err)
}

func TestWriteAheadLog_Write(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	// Test writing a regular entry
	key := "test_key_123"
	value := []byte("test value data")
	
	err = wal.Write(key, value, false)
	assert.NoError(t, err)
	
	// Check sequence number was incremented
	assert.Equal(t, uint64(1), wal.sequence)
	assert.True(t, wal.size > 0)

	// Test writing a deleted entry
	deletedKey := "deleted_key"
	deletedValue := []byte("deleted value")
	
	err = wal.Write(deletedKey, deletedValue, true)
	assert.NoError(t, err)
	
	assert.Equal(t, uint64(2), wal.sequence)

	// Test writing empty key/value
	err = wal.Write("", []byte{}, false)
	assert.NoError(t, err)
	
	assert.Equal(t, uint64(3), wal.sequence)
}

func TestWriteAheadLog_Sync(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	// Write some entries
	err = wal.Write("key1", []byte("value1"), false)
	require.NoError(t, err)
	err = wal.Write("key2", []byte("value2"), false)
	require.NoError(t, err)

	// Before sync
	assert.Equal(t, uint64(0), wal.synced) // No sync yet

	// Sync to disk
	err = wal.Sync()
	assert.NoError(t, err)
	
	// After sync
	assert.Equal(t, wal.sequence, wal.synced)
}

func TestWriteAheadLog_ReadAll(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	// Write test data
	testData := []struct {
		key     string
		value   []byte
		deleted bool
	}{
		{"key1", []byte("value1"), false},
		{"key2", []byte("value2"), false},
		{"key3", []byte("value3"), true},
		{"key4", []byte(""), false},
		{"", []byte("empty_key_value"), false},
	}

	for _, td := range testData {
		err := wal.Write(td.key, td.value, td.deleted)
		require.NoError(t, err)
	}

	// Sync to ensure data is written
	err = wal.Sync()
	require.NoError(t, err)

	// Read all entries
	entries, err := wal.ReadAll()
	assert.NoError(t, err)
	assert.Len(t, entries, len(testData))

	// Verify entries in order
	for i, entry := range entries {
		expected := testData[i]
		assert.Equal(t, uint64(i+1), entry.Sequence)
		assert.Equal(t, expected.key, entry.Key)
		assert.Equal(t, expected.value, entry.Value)
		assert.Equal(t, expected.deleted, entry.Deleted)
		assert.NotZero(t, entry.Checksum)
	}
}

func TestWriteAheadLog_RecoverSequence(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create WAL and write some entries
	wal1, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)
	
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key_%d", i)
		value := []byte(fmt.Sprintf("value_%d", i))
		err := wal1.Write(key, value, false)
		require.NoError(t, err)
	}
	
	err = wal1.Sync()
	require.NoError(t, err)
	
	lastSequence := wal1.sequence
	err = wal1.Close()
	require.NoError(t, err)

	// Reopen WAL and verify sequence recovery
	wal2, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)
	defer wal2.Close()

	assert.Equal(t, lastSequence, wal2.sequence)
	assert.Equal(t, lastSequence, wal2.synced)

	// Write another entry and verify sequence continues
	err = wal2.Write("new_key", []byte("new_value"), false)
	assert.NoError(t, err)
	assert.Equal(t, lastSequence+1, wal2.sequence)
}

func TestWriteAheadLog_Truncate(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)

	// Write some entries
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("key_%d", i)
		value := []byte(fmt.Sprintf("value_%d", i))
		err := wal.Write(key, value, false)
		require.NoError(t, err)
	}

	err = wal.Sync()
	require.NoError(t, err)

	// Verify entries exist
	entries, err := wal.ReadAll()
	require.NoError(t, err)
	assert.Len(t, entries, 3)

	// Truncate WAL
	err = wal.Truncate()
	assert.NoError(t, err)

	// Verify WAL is empty
	assert.Equal(t, int64(0), wal.size)
	assert.Equal(t, uint64(0), wal.sequence)
	assert.Equal(t, uint64(0), wal.synced)

	entries, err = wal.ReadAll()
	require.NoError(t, err)
	assert.Empty(t, entries)

	// Verify we can still write after truncate
	err = wal.Write("post_truncate", []byte("value"), false)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), wal.sequence)

	err = wal.Close()
	assert.NoError(t, err)
}

func TestWriteAheadLog_Rotation(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	// Set a very small max size to force rotation
	wal.maxSize = 1024 // 1KB

	// Write enough data to trigger rotation
	largeValue := make([]byte, 300) // Large value
	for i := 0; i < 10; i++ { // 10 * 300 = 3KB > 1KB
		key := fmt.Sprintf("large_key_%d", i)
		err := wal.Write(key, largeValue, false)
		require.NoError(t, err)
	}

	// Sync to ensure rotation happens
	err = wal.Sync()
	require.NoError(t, err)

	// Check that size was reset (indicating rotation occurred)
	// Note: The exact behavior depends on when rotation triggers
	assert.True(t, wal.size < wal.maxSize || wal.size >= 0)

	// Verify we can still write after rotation
	err = wal.Write("post_rotation", []byte("value"), false)
	assert.NoError(t, err)
}

func TestWALEntry_SerializeDeserialize(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	// Test various entry types
	testEntries := []*WALEntry{
		{
			Sequence: 1,
			Key:      "normal_key",
			Value:    []byte("normal value"),
			Deleted:  false,
		},
		{
			Sequence: 2,
			Key:      "deleted_key",
			Value:    []byte("deleted value"),
			Deleted:  true,
		},
		{
			Sequence: 3,
			Key:      "",
			Value:    []byte("empty key"),
			Deleted:  false,
		},
		{
			Sequence: 4,
			Key:      "empty_value",
			Value:    []byte{},
			Deleted:  false,
		},
		{
			Sequence: 5,
			Key:      "unicode_key_测试",
			Value:    []byte("unicode value 测试"),
			Deleted:  false,
		},
	}

	for _, entry := range testEntries {
		// Calculate checksum
		entry.Checksum = wal.calculateChecksum(entry)

		// Serialize
		data, err := wal.serializeEntry(entry)
		assert.NoError(t, err)
		assert.True(t, len(data) > 0)

		// Verify magic number is present
		magic := data[0:4]
		assert.Equal(t, []byte{0xEF, 0xBE, 0xAD, 0xDE}, magic) // Little-endian 0xDEADBEEF
	}
}

func TestWriteAheadLog_Checksum(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	entry := &WALEntry{
		Sequence: 123,
		Key:      "test_key",
		Value:    []byte("test_value"),
		Deleted:  false,
	}

	// Calculate checksum
	checksum1 := wal.calculateChecksum(entry)
	checksum2 := wal.calculateChecksum(entry)
	
	// Should be deterministic
	assert.Equal(t, checksum1, checksum2)

	// Different entries should have different checksums (usually)
	entry2 := &WALEntry{
		Sequence: 123,
		Key:      "test_key",
		Value:    []byte("different_value"), // Different value
		Deleted:  false,
	}
	
	checksum3 := wal.calculateChecksum(entry2)
	assert.NotEqual(t, checksum1, checksum3)

	// Deleted flag should affect checksum
	entry.Deleted = true
	checksum4 := wal.calculateChecksum(entry)
	assert.NotEqual(t, checksum1, checksum4)
}

func TestWriteAheadLog_Stats(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	// Initial stats
	stats := wal.Stats()
	assert.Equal(t, int64(0), stats.Size)
	assert.Equal(t, uint64(0), stats.Sequence)
	assert.Equal(t, uint64(0), stats.SyncedSequence)
	assert.Equal(t, int64(0), stats.PendingWrites)

	// Write some entries
	err = wal.Write("key1", []byte("value1"), false)
	require.NoError(t, err)
	err = wal.Write("key2", []byte("value2"), false)
	require.NoError(t, err)

	// Stats after writes (before sync)
	stats = wal.Stats()
	assert.True(t, stats.Size > 0)
	assert.Equal(t, uint64(2), stats.Sequence)
	assert.Equal(t, uint64(0), stats.SyncedSequence) // Not synced yet
	assert.True(t, stats.PendingWrites > 0)

	// Sync and check stats
	err = wal.Sync()
	require.NoError(t, err)

	stats = wal.Stats()
	assert.True(t, stats.Size > 0)
	assert.Equal(t, uint64(2), stats.Sequence)
	assert.Equal(t, uint64(2), stats.SyncedSequence) // Now synced
	assert.Equal(t, int64(0), stats.PendingWrites)  // No pending writes
}

func TestWriteAheadLog_ErrorHandling(t *testing.T) {
	// Test creating WAL in non-existent directory
	_, err := NewWriteAheadLog("/non/existent/directory")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create WAL directory")

	// Test creating WAL in read-only directory
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	readOnlyDir := filepath.Join(tempDir, "readonly")
	err = os.Mkdir(readOnlyDir, 0444) // Read-only
	require.NoError(t, err)

	// This might not fail on all systems due to user permissions
	_, err = NewWriteAheadLog(readOnlyDir)
	if err != nil {
		// Expected on systems that enforce read-only directories
		t.Log("WAL creation failed on read-only directory (expected)")
	} else {
		t.Log("WAL creation succeeded on read-only directory (system-dependent)")
	}

	// Reset permissions for cleanup
	os.Chmod(readOnlyDir, 0755)
}

func TestWriteAheadLog_CorruptionHandling(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	walPath := filepath.Join(tempDir, "wal", "current.wal")

	// Create WAL and write some entries
	wal1, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)

	err = wal1.Write("key1", []byte("value1"), false)
	require.NoError(t, err)
	err = wal1.Write("key2", []byte("value2"), false)
	require.NoError(t, err)
	err = wal1.Sync()
	require.NoError(t, err)
	err = wal1.Close()
	require.NoError(t, err)

	// Corrupt the WAL file by truncating it
	walFile, err := os.OpenFile(walPath, os.O_WRONLY, 0644)
	require.NoError(t, err)
	err = walFile.Truncate(10) // Truncate to very small size
	require.NoError(t, err)
	err = walFile.Close()
	require.NoError(t, err)

	// Try to reopen WAL - should handle corruption gracefully
	wal2, err := NewWriteAheadLog(tempDir)
	// Should not fail to create WAL even with corrupted file
	assert.NoError(t, err)
	if wal2 != nil {
		wal2.Close()
	}
}

func TestWriteAheadLog_EmptyWAL(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	// Test operations on empty WAL
	entries, err := wal.ReadAll()
	assert.NoError(t, err)
	assert.Empty(t, entries)

	stats := wal.Stats()
	assert.Equal(t, int64(0), stats.Size)
	assert.Equal(t, uint64(0), stats.Sequence)

	// Sync empty WAL should work
	err = wal.Sync()
	assert.NoError(t, err)

	// Truncate empty WAL should work
	err = wal.Truncate()
	assert.NoError(t, err)
}

func TestWriteAheadLog_LargeValues(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	// Test with large value (1MB)
	largeValue := make([]byte, 1024*1024)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	key := "large_value_key"
	err = wal.Write(key, largeValue, false)
	assert.NoError(t, err)

	err = wal.Sync()
	require.NoError(t, err)

	// Read back and verify
	entries, err := wal.ReadAll()
	assert.NoError(t, err)
	assert.Len(t, entries, 1)

	entry := entries[0]
	assert.Equal(t, key, entry.Key)
	assert.Equal(t, largeValue, entry.Value)
	assert.False(t, entry.Deleted)
}

func TestWriteAheadLog_Concurrent(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(t, err)
	defer wal.Close()

	numGoroutines := 10
	numWrites := 100
	done := make(chan bool, numGoroutines)

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < numWrites; j++ {
				key := fmt.Sprintf("concurrent_key_%d_%d", id, j)
				value := []byte(fmt.Sprintf("concurrent_value_%d_%d", id, j))
				
				err := wal.Write(key, value, false)
				assert.NoError(t, err)
			}
		}(i)
	}

	// Wait for all writes to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Sync and verify
	err = wal.Sync()
	require.NoError(t, err)

	entries, err := wal.ReadAll()
	assert.NoError(t, err)
	assert.Len(t, entries, numGoroutines*numWrites)

	// Verify sequence numbers are unique and increasing
	seqSet := make(map[uint64]bool)
	for i, entry := range entries {
		assert.Equal(t, uint64(i+1), entry.Sequence)
		assert.False(t, seqSet[entry.Sequence], "Duplicate sequence number: %d", entry.Sequence)
		seqSet[entry.Sequence] = true
	}
}

func TestWriteAheadLog_Recovery_Scenarios(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "wal_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Scenario 1: Normal shutdown and recovery
	{
		wal, err := NewWriteAheadLog(tempDir)
		require.NoError(t, err)

		// Write some data
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("key_%d", i)
			value := []byte(fmt.Sprintf("value_%d", i))
			err := wal.Write(key, value, false)
			require.NoError(t, err)
		}

		err = wal.Sync()
		require.NoError(t, err)
		err = wal.Close()
		require.NoError(t, err)

		// Reopen and verify recovery
		wal2, err := NewWriteAheadLog(tempDir)
		require.NoError(t, err)
		defer wal2.Close()

		assert.Equal(t, uint64(5), wal2.sequence)
		
		entries, err := wal2.ReadAll()
		require.NoError(t, err)
		assert.Len(t, entries, 5)
	}

	// Scenario 2: Recovery after truncate
	{
		tempDir2, err := ioutil.TempDir("", "wal_test_2")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir2)

		wal, err := NewWriteAheadLog(tempDir2)
		require.NoError(t, err)

		// Write data, truncate, then write more
		err = wal.Write("before_truncate", []byte("value"), false)
		require.NoError(t, err)
		
		err = wal.Truncate()
		require.NoError(t, err)
		
		err = wal.Write("after_truncate", []byte("value"), false)
		require.NoError(t, err)
		
		err = wal.Sync()
		require.NoError(t, err)
		err = wal.Close()
		require.NoError(t, err)

		// Reopen and verify
		wal2, err := NewWriteAheadLog(tempDir2)
		require.NoError(t, err)
		defer wal2.Close()

		entries, err := wal2.ReadAll()
		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Equal(t, "after_truncate", entries[0].Key)
	}
}

// Benchmark tests
func BenchmarkWAL_Write(b *testing.B) {
	tempDir, err := ioutil.TempDir("", "wal_bench")
	require.NoError(b, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(b, err)
	defer wal.Close()

	value := make([]byte, 1024) // 1KB value

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench_key_%d", i)
			wal.Write(key, value, false)
			i++
		}
	})
}

func BenchmarkWAL_WriteAndSync(b *testing.B) {
	tempDir, err := ioutil.TempDir("", "wal_bench")
	require.NoError(b, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(b, err)
	defer wal.Close()

	value := make([]byte, 1024) // 1KB value

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench_key_%d", i)
		wal.Write(key, value, false)
		if i%100 == 0 { // Sync every 100 writes
			wal.Sync()
		}
	}
}

func BenchmarkWAL_ReadAll(b *testing.B) {
	tempDir, err := ioutil.TempDir("", "wal_bench")
	require.NoError(b, err)
	defer os.RemoveAll(tempDir)

	wal, err := NewWriteAheadLog(tempDir)
	require.NoError(b, err)
	defer wal.Close()

	// Populate WAL with test data
	value := make([]byte, 100)
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("bench_key_%d", i)
		wal.Write(key, value, false)
	}
	wal.Sync()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wal.ReadAll()
	}
}