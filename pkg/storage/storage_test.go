package storage

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/caiatech/govc/pkg/object"
)

func TestMemoryBackend(t *testing.T) {
	backend := NewMemoryBackend()

	t.Run("basic operations", func(t *testing.T) {
		hash := "abc123"
		data := []byte("test data")

		// Test write
		err := backend.WriteObject(hash, data)
		if err != nil {
			t.Fatalf("WriteObject() error = %v", err)
		}

		// Test has
		if !backend.HasObject(hash) {
			t.Error("HasObject() = false, want true")
		}

		// Test read
		got, err := backend.ReadObject(hash)
		if err != nil {
			t.Fatalf("ReadObject() error = %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Errorf("ReadObject() = %v, want %v", got, data)
		}

		// Test non-existent object
		_, err = backend.ReadObject("nonexistent")
		if err == nil {
			t.Error("ReadObject(nonexistent) should return error")
		}
	})

	t.Run("list objects", func(t *testing.T) {
		// Add multiple objects
		for i := 0; i < 5; i++ {
			hash := fmt.Sprintf("hash%d", i)
			backend.WriteObject(hash, []byte(fmt.Sprintf("data%d", i)))
		}

		objects, err := backend.ListObjects()
		if err != nil {
			t.Fatalf("ListObjects() error = %v", err)
		}

		if len(objects) < 5 {
			t.Errorf("ListObjects() returned %d objects, want at least 5", len(objects))
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, 100)

		// Concurrent writes
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				hash := fmt.Sprintf("concurrent%d", id)
				data := []byte(fmt.Sprintf("data%d", id))
				if err := backend.WriteObject(hash, data); err != nil {
					errors <- err
				}
			}(i)
		}

		// Concurrent reads
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				hash := fmt.Sprintf("concurrent%d", id)
				if backend.HasObject(hash) {
					if _, err := backend.ReadObject(hash); err != nil {
						errors <- err
					}
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		for err := range errors {
			t.Errorf("Concurrent operation error: %v", err)
		}
	})
}

func TestFileBackend(t *testing.T) {
	tmpDir := t.TempDir()
	backend := NewFileBackend(tmpDir)

	t.Run("basic operations", func(t *testing.T) {
		hash := "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391"
		data := []byte("test file data")

		// Test write
		err := backend.WriteObject(hash, data)
		if err != nil {
			t.Fatalf("WriteObject() error = %v", err)
		}

		// Verify file exists
		expectedPath := filepath.Join(tmpDir, "objects", "e6", "9de29bb2d1d6434b8b29ae775ad8c2e48c5391")
		if _, err := os.Stat(expectedPath); err != nil {
			t.Errorf("Object file not created at expected path: %v", err)
		}

		// Test has
		if !backend.HasObject(hash) {
			t.Error("HasObject() = false, want true")
		}

		// Test read (should decompress)
		got, err := backend.ReadObject(hash)
		if err != nil {
			t.Fatalf("ReadObject() error = %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Errorf("ReadObject() = %v, want %v", got, data)
		}
	})

	t.Run("compression", func(t *testing.T) {
		hash := "abc456"
		originalData := bytes.Repeat([]byte("compress me "), 100)

		err := backend.WriteObject(hash, originalData)
		if err != nil {
			t.Fatalf("WriteObject() error = %v", err)
		}

		// Read raw file to verify compression
		rawPath := filepath.Join(tmpDir, "objects", "ab", "c456")
		rawData, err := os.ReadFile(rawPath)
		if err != nil {
			t.Fatalf("Failed to read raw file: %v", err)
		}

		// Compressed data should be smaller
		if len(rawData) >= len(originalData) {
			t.Errorf("Compressed size %d >= original size %d", len(rawData), len(originalData))
		}

		// Verify decompression works
		got, err := backend.ReadObject(hash)
		if err != nil {
			t.Fatalf("ReadObject() error = %v", err)
		}
		if !bytes.Equal(got, originalData) {
			t.Error("Decompressed data doesn't match original")
		}
	})
}

func TestStore(t *testing.T) {
	backend := NewMemoryBackend()
	store := NewStore(backend)

	t.Run("store and retrieve blob", func(t *testing.T) {
		content := []byte("hello world")
		hash, err := store.StoreBlob(content)
		if err != nil {
			t.Fatalf("StoreBlob() error = %v", err)
		}

		// Retrieve as blob
		blob, err := store.GetBlob(hash)
		if err != nil {
			t.Fatalf("GetBlob() error = %v", err)
		}

		if !bytes.Equal(blob.Content, content) {
			t.Errorf("Blob content = %v, want %v", blob.Content, content)
		}

		// Try to retrieve as wrong type
		_, err = store.GetTree(hash)
		if err == nil {
			t.Error("GetTree() on blob should return error")
		}
	})

	t.Run("store and retrieve tree", func(t *testing.T) {
		tree := object.NewTree()
		tree.AddEntry("100644", "file.txt", "abc123")
		
		hash, err := store.StoreTree(tree)
		if err != nil {
			t.Fatalf("StoreTree() error = %v", err)
		}

		// Retrieve
		got, err := store.GetTree(hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}

		if len(got.Entries) != 1 {
			t.Errorf("Tree entries = %d, want 1", len(got.Entries))
		}
	})

	t.Run("store and retrieve commit", func(t *testing.T) {
		author := object.Author{
			Name:  "Test",
			Email: "test@example.com",
			Time:  time.Now(),
		}
		commit := object.NewCommit("tree123", author, "Test commit")
		
		hash, err := store.StoreCommit(commit)
		if err != nil {
			t.Fatalf("StoreCommit() error = %v", err)
		}

		// Retrieve
		got, err := store.GetCommit(hash)
		if err != nil {
			t.Fatalf("GetCommit() error = %v", err)
		}

		if got.Message != "Test commit" {
			t.Errorf("Commit message = %v, want Test commit", got.Message)
		}
	})

	t.Run("caching behavior", func(t *testing.T) {
		// Create a backend that tracks reads
		trackingBackend := &trackingBackend{
			MemoryBackend: NewMemoryBackend(),
			readCount:     make(map[string]int),
		}
		cachedStore := NewStore(trackingBackend)

		// Store object
		hash, _ := cachedStore.StoreBlob([]byte("cached"))

		// First read - should hit backend since cache doesn't have it
		_, err := cachedStore.GetObject(hash)
		if err != nil {
			t.Fatalf("First GetObject failed: %v", err)
		}
		if trackingBackend.readCount[hash] != 0 {
			t.Errorf("First read count = %d, want 0 (object was just stored, should be in cache)", trackingBackend.readCount[hash])
		}

		// Clear the cache to force backend read
		cachedStore.cache = NewMemoryBackend()
		
		// Now read - should hit backend
		_, err = cachedStore.GetObject(hash)
		if err != nil {
			t.Fatalf("Second GetObject failed: %v", err)
		}
		if trackingBackend.readCount[hash] != 1 {
			t.Errorf("Backend read count = %d, want 1", trackingBackend.readCount[hash])
		}
		
		// Third read - should hit cache
		_, err = cachedStore.GetObject(hash)
		if err != nil {
			t.Fatalf("Third GetObject failed: %v", err)
		}
		if trackingBackend.readCount[hash] != 1 {
			t.Errorf("Final read count = %d, want 1 (should use cache)", trackingBackend.readCount[hash])
		}
	})
}

// trackingBackend wraps MemoryBackend to count reads
type trackingBackend struct {
	*MemoryBackend
	readCount map[string]int
	mu        sync.Mutex
}

func (t *trackingBackend) ReadObject(hash string) ([]byte, error) {
	t.mu.Lock()
	t.readCount[hash]++
	t.mu.Unlock()
	return t.MemoryBackend.ReadObject(hash)
}

func TestStoreListObjects(t *testing.T) {
	backend := NewMemoryBackend()
	store := NewStore(backend)

	// Add various objects
	hashes := make([]string, 0)
	for i := 0; i < 5; i++ {
		hash, _ := store.StoreBlob([]byte(fmt.Sprintf("blob%d", i)))
		hashes = append(hashes, hash)
	}

	// List all
	got, err := store.ListObjects()
	if err != nil {
		t.Fatalf("ListObjects() error = %v", err)
	}

	if len(got) != len(hashes) {
		t.Errorf("ListObjects() returned %d, want %d", len(got), len(hashes))
	}

	// Verify all hashes present
	hashSet := make(map[string]bool)
	for _, h := range got {
		hashSet[h] = true
	}

	for _, want := range hashes {
		if !hashSet[want] {
			t.Errorf("Missing hash %s in ListObjects result", want)
		}
	}
}

func BenchmarkMemoryBackendWrite(b *testing.B) {
	backend := NewMemoryBackend()
	data := bytes.Repeat([]byte("benchmark"), 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hash := fmt.Sprintf("hash%d", i)
		backend.WriteObject(hash, data)
	}
}

func BenchmarkMemoryBackendRead(b *testing.B) {
	backend := NewMemoryBackend()
	data := bytes.Repeat([]byte("benchmark"), 100)
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		hash := fmt.Sprintf("hash%d", i)
		backend.WriteObject(hash, data)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hash := fmt.Sprintf("hash%d", i%1000)
		backend.ReadObject(hash)
	}
}

func BenchmarkStoreCaching(b *testing.B) {
	backend := NewMemoryBackend()
	store := NewStore(backend)
	
	// Store one object
	hash, _ := store.StoreBlob([]byte("benchmark data"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This should hit cache after first read
		store.GetBlob(hash)
	}
}

// Test that demonstrates memory-first benefits
func TestMemoryFirstPerformance(t *testing.T) {
	memBackend := NewMemoryBackend()
	memStore := NewStore(memBackend)

	tmpDir := t.TempDir()
	fileBackend := NewFileBackend(tmpDir)
	fileStore := NewStore(fileBackend)

	// Generate test data
	data := bytes.Repeat([]byte("performance test"), 100)

	// Time memory operations
	start := time.Now()
	for i := 0; i < 100; i++ {
		blob := object.NewBlob(append(data, byte(i)))
		memStore.StoreObject(blob)
	}
	memDuration := time.Since(start)

	// Time file operations
	start = time.Now()
	for i := 0; i < 100; i++ {
		blob := object.NewBlob(append(data, byte(i)))
		fileStore.StoreObject(blob)
	}
	fileDuration := time.Since(start)

	// Memory should be significantly faster
	t.Logf("Memory backend: %v", memDuration)
	t.Logf("File backend: %v", fileDuration)
	
	// Memory operations should be at least 10x faster
	if memDuration > fileDuration/10 {
		t.Logf("Memory backend not significantly faster than file backend")
	}
}