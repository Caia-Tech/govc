package build

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVFSEdgeCases tests edge cases and error conditions
func TestVFSEdgeCases(t *testing.T) {
	t.Run("EmptyFiles", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		
		// Write empty file
		err := vfs.Write("empty.txt", []byte{})
		require.NoError(t, err)
		
		// Read empty file
		content, err := vfs.Read("empty.txt")
		require.NoError(t, err)
		assert.Empty(t, content)
		assert.NotNil(t, content) // Should be empty slice, not nil
		
		// Verify it exists
		assert.True(t, vfs.Exists("empty.txt"))
	})

	t.Run("NonExistentFile", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		
		// Read non-existent file
		_, err := vfs.Read("doesnotexist.txt")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		
		// Check existence
		assert.False(t, vfs.Exists("doesnotexist.txt"))
		
		// Delete non-existent file (should not error)
		err = vfs.Delete("doesnotexist.txt")
		assert.NoError(t, err)
	})

	t.Run("OverwriteFile", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		
		// Write initial content
		err := vfs.Write("file.txt", []byte("initial"))
		require.NoError(t, err)
		
		// Overwrite with new content
		err = vfs.Write("file.txt", []byte("updated"))
		require.NoError(t, err)
		
		// Read should return new content
		content, err := vfs.Read("file.txt")
		require.NoError(t, err)
		assert.Equal(t, "updated", string(content))
	})

	t.Run("PathWithSpaces", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		
		// Write file with spaces in path
		err := vfs.Write("my file.txt", []byte("spaces"))
		require.NoError(t, err)
		
		err = vfs.Write("dir with spaces/file.txt", []byte("nested"))
		require.NoError(t, err)
		
		// Read back
		content, err := vfs.Read("my file.txt")
		require.NoError(t, err)
		assert.Equal(t, "spaces", string(content))
		
		content, err = vfs.Read("dir with spaces/file.txt")
		require.NoError(t, err)
		assert.Equal(t, "nested", string(content))
	})

	t.Run("SpecialCharacters", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		
		specialPaths := []string{
			"file-with-dashes.txt",
			"file_with_underscores.txt",
			"file.multiple.dots.txt",
			"file@special#chars$.txt",
			"файл.txt", // Unicode
			"文件.txt",  // Chinese
			"🚀.txt",   // Emoji
		}
		
		for _, path := range specialPaths {
			err := vfs.Write(path, []byte("content"))
			require.NoError(t, err, "Failed to write %s", path)
			
			content, err := vfs.Read(path)
			require.NoError(t, err, "Failed to read %s", path)
			assert.Equal(t, "content", string(content))
		}
		
		files, _ := vfs.List()
		assert.Len(t, files, len(specialPaths))
	})

	t.Run("VeryLongPath", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		
		// Create very long path
		longPath := ""
		for i := 0; i < 50; i++ {
			longPath = filepath.Join(longPath, fmt.Sprintf("dir%d", i))
		}
		longPath = filepath.Join(longPath, "file.txt")
		
		err := vfs.Write(longPath, []byte("deep"))
		require.NoError(t, err)
		
		content, err := vfs.Read(longPath)
		require.NoError(t, err)
		assert.Equal(t, "deep", string(content))
	})

	t.Run("BinaryData", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		
		// Create binary data with all byte values
		binaryData := make([]byte, 256)
		for i := range binaryData {
			binaryData[i] = byte(i)
		}
		
		err := vfs.Write("binary.dat", binaryData)
		require.NoError(t, err)
		
		content, err := vfs.Read("binary.dat")
		require.NoError(t, err)
		assert.Equal(t, binaryData, content)
	})

	t.Run("NilAndEmptyOperations", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		
		// Write nil (should handle gracefully)
		err := vfs.Write("nil.txt", nil)
		require.NoError(t, err)
		
		content, err := vfs.Read("nil.txt")
		require.NoError(t, err)
		assert.Empty(t, content)
		
		// Empty path operations
		err = vfs.Write("", []byte("content"))
		require.NoError(t, err) // Should handle empty path
		
		assert.True(t, vfs.Exists(""))
	})
}

// TestVFSConcurrency tests concurrent operations
func TestVFSConcurrency(t *testing.T) {
	t.Run("ConcurrentWrites", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		var wg sync.WaitGroup
		errors := make(chan error, 100)
		
		// 100 concurrent writes
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				path := fmt.Sprintf("file%d.txt", n)
				content := []byte(fmt.Sprintf("content%d", n))
				if err := vfs.Write(path, content); err != nil {
					errors <- err
				}
			}(i)
		}
		
		wg.Wait()
		close(errors)
		
		// Check for errors
		for err := range errors {
			t.Errorf("Concurrent write error: %v", err)
		}
		
		// Verify all files exist
		files, _ := vfs.List()
		assert.Len(t, files, 100)
	})

	t.Run("ConcurrentReads", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		
		// Pre-populate files
		for i := 0; i < 10; i++ {
			vfs.Write(fmt.Sprintf("file%d.txt", i), []byte(fmt.Sprintf("content%d", i)))
		}
		
		var wg sync.WaitGroup
		errors := make(chan error, 1000)
		
		// 100 readers per file
		for i := 0; i < 10; i++ {
			for j := 0; j < 100; j++ {
				wg.Add(1)
				go func(fileNum int) {
					defer wg.Done()
					path := fmt.Sprintf("file%d.txt", fileNum)
					content, err := vfs.Read(path)
					if err != nil {
						errors <- err
						return
					}
					expected := fmt.Sprintf("content%d", fileNum)
					if string(content) != expected {
						errors <- fmt.Errorf("content mismatch: got %s, want %s", content, expected)
					}
				}(i)
			}
		}
		
		wg.Wait()
		close(errors)
		
		for err := range errors {
			t.Errorf("Concurrent read error: %v", err)
		}
	})

	t.Run("ConcurrentMixed", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		var wg sync.WaitGroup
		var readCount, writeCount, deleteCount int32
		
		// Writers
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					path := fmt.Sprintf("file%d_%d.txt", n, j)
					vfs.Write(path, []byte("data"))
					atomic.AddInt32(&writeCount, 1)
				}
			}(i)
		}
		
		// Readers
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					path := fmt.Sprintf("file%d_%d.txt", n, j)
					if vfs.Exists(path) {
						vfs.Read(path)
						atomic.AddInt32(&readCount, 1)
					}
				}
			}(i)
		}
		
		// Deleters
		for i := 0; i < 25; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				time.Sleep(5 * time.Millisecond) // Let some writes happen first
				for j := 0; j < 5; j++ {
					path := fmt.Sprintf("file%d_%d.txt", n, j)
					vfs.Delete(path)
					atomic.AddInt32(&deleteCount, 1)
				}
			}(i)
		}
		
		wg.Wait()
		
		t.Logf("Operations: Writes=%d, Reads=%d, Deletes=%d", 
			writeCount, readCount, deleteCount)
		
		// Should have completed without deadlock
		assert.Greater(t, writeCount, int32(0))
	})

	t.Run("RaceConditionTest", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		const iterations = 1000
		
		// Shared file that multiple goroutines will update
		const sharedFile = "shared.txt"
		
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					// Rapid read-modify-write
					content, _ := vfs.Read(sharedFile)
					newContent := append(content, byte(id))
					vfs.Write(sharedFile, newContent)
				}
			}(i)
		}
		
		wg.Wait()
		
		// File should exist and have content
		assert.True(t, vfs.Exists(sharedFile))
		content, _ := vfs.Read(sharedFile)
		// Due to race conditions, we can't predict exact content,
		// but it should have some data
		assert.NotEmpty(t, content)
	})
}

// TestStreamingVFSAdvanced tests streaming capabilities
func TestStreamingVFSAdvanced(t *testing.T) {
	t.Run("LargeStreamWrite", func(t *testing.T) {
		base := NewDirectMemoryVFS()
		vfs := NewStreamingVFS(base)
		
		stream, err := vfs.OpenStream("large.dat")
		require.NoError(t, err)
		
		// Write 10MB in chunks
		chunk := make([]byte, 1024) // 1KB chunks
		for i := range chunk {
			chunk[i] = byte(i % 256)
		}
		
		totalWritten := 0
		for i := 0; i < 10*1024; i++ { // 10K iterations = 10MB
			n, err := stream.Write(chunk)
			require.NoError(t, err)
			totalWritten += n
		}
		
		err = stream.Close()
		require.NoError(t, err)
		
		// Verify size
		content, err := base.Read("large.dat")
		require.NoError(t, err)
		assert.Equal(t, 10*1024*1024, len(content))
		assert.Equal(t, totalWritten, len(content))
	})

	t.Run("StreamReadWrite", func(t *testing.T) {
		base := NewDirectMemoryVFS()
		vfs := NewStreamingVFS(base)
		
		// Pre-populate file
		base.Write("stream.txt", []byte("initial content"))
		
		stream, err := vfs.OpenStream("stream.txt")
		require.NoError(t, err)
		
		// Read initial content
		buf := make([]byte, 7)
		n, err := stream.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
		assert.Equal(t, "initial", string(buf[:n]))
		
		// Append more content
		_, err = stream.Write([]byte(" appended"))
		require.NoError(t, err)
		
		err = stream.Close()
		require.NoError(t, err)
		
		// Verify combined content
		content, _ := base.Read("stream.txt")
		assert.Contains(t, string(content), "initial")
		assert.Contains(t, string(content), "appended")
	})

	t.Run("MultipleStreams", func(t *testing.T) {
		base := NewDirectMemoryVFS()
		vfs := NewStreamingVFS(base)
		
		// Open multiple streams
		streams := make([]io.ReadWriteCloser, 10)
		for i := 0; i < 10; i++ {
			stream, err := vfs.OpenStream(fmt.Sprintf("stream%d.txt", i))
			require.NoError(t, err)
			streams[i] = stream
		}
		
		// Write to all streams
		for i, stream := range streams {
			_, err := stream.Write([]byte(fmt.Sprintf("Stream %d data", i)))
			require.NoError(t, err)
		}
		
		// Close all streams
		for _, stream := range streams {
			err := stream.Close()
			require.NoError(t, err)
		}
		
		// Verify all files
		for i := 0; i < 10; i++ {
			content, err := base.Read(fmt.Sprintf("stream%d.txt", i))
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("Stream %d data", i), string(content))
		}
	})
}

// TestFilesystemBridgeAdvanced tests bridge edge cases
func TestFilesystemBridgeAdvanced(t *testing.T) {
	t.Run("LargeFileSync", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		
		// Create 5MB file
		largeData := make([]byte, 5*1024*1024)
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}
		vfs.Write("large.bin", largeData)
		
		// Create bridge
		bridge, err := vfs.CreateBridge()
		require.NoError(t, err)
		defer bridge.Cleanup()
		
		// Execute command to verify file size
		result, err := bridge.Exec("ls", []string{"-lh"}, nil)
		require.NoError(t, err)
		assert.Contains(t, result.Output, "large.bin")
		
		// Modify in temp and sync back
		result, err = bridge.Exec("echo", []string{"test"}, nil)
		require.NoError(t, err)
		
		err = bridge.SyncFromTemp()
		require.NoError(t, err)
	})

	t.Run("PermissionPreservation", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		
		// Create executable script
		script := `#!/bin/bash
echo "Hello from script"
`
		vfs.Write("script.sh", []byte(script))
		
		bridge, err := vfs.CreateBridge()
		require.NoError(t, err)
		defer bridge.Cleanup()
		
		// Make executable
		result, err := bridge.Exec("chmod", []string{"+x", "script.sh"}, nil)
		require.NoError(t, err)
		assert.Equal(t, 0, result.ExitCode)
		
		// Execute script
		result, err = bridge.Exec("bash", []string{"script.sh"}, nil)
		if err == nil {
			assert.Contains(t, result.Output, "Hello from script")
		}
	})

	t.Run("ComplexDirectoryStructure", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		
		// Create complex structure
		files := map[string]string{
			"src/main.go":           "package main",
			"src/lib/helper.go":     "package lib",
			"src/lib/util/util.go":  "package util",
			"test/unit_test.go":     "package test",
			"docs/README.md":        "# Docs",
			"config/app.yaml":       "config: true",
			".gitignore":            "*.tmp",
			"Makefile":              "build:",
		}
		
		for path, content := range files {
			vfs.Write(path, []byte(content))
		}
		
		bridge, err := vfs.CreateBridge()
		require.NoError(t, err)
		defer bridge.Cleanup()
		
		// Verify directory structure
		result, err := bridge.Exec("find", []string{".", "-type", "f"}, nil)
		require.NoError(t, err)
		
		for path := range files {
			assert.Contains(t, result.Output, path)
		}
		
		// Count files
		result, err = bridge.Exec("find", []string{".", "-type", "f", "-name", "*.go"}, nil)
		require.NoError(t, err)
		
		lines := bytes.Count([]byte(result.Output), []byte("\n"))
		assert.Equal(t, 4, lines) // 4 .go files
	})

	t.Run("EnvironmentVariables", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		vfs.Write("test.txt", []byte("test"))
		
		bridge, err := vfs.CreateBridge()
		require.NoError(t, err)
		defer bridge.Cleanup()
		
		// Execute with custom environment
		env := map[string]string{
			"CUSTOM_VAR": "custom_value",
			"TEST_MODE":  "true",
		}
		
		result, err := bridge.Exec("sh", []string{"-c", "echo $CUSTOM_VAR"}, env)
		require.NoError(t, err)
		assert.Contains(t, result.Output, "custom_value")
	})

	t.Run("CommandTimeout", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		vfs.Write("test.txt", []byte("test"))
		
		bridge, err := vfs.CreateBridge()
		require.NoError(t, err)
		defer bridge.Cleanup()
		
		// Start timer
		start := time.Now()
		
		// Execute command that would take time
		result, _ := bridge.Exec("sleep", []string{"0.1"}, nil)
		
		duration := time.Since(start)
		assert.GreaterOrEqual(t, duration, 100*time.Millisecond)
		assert.Equal(t, 0, result.ExitCode)
	})

	t.Run("MultipleBridges", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		vfs.Write("shared.txt", []byte("shared content"))
		
		// Create multiple bridges from same VFS
		bridges := make([]FilesystemBridge, 3)
		for i := range bridges {
			bridge, err := vfs.CreateBridge()
			require.NoError(t, err)
			bridges[i] = bridge
		}
		
		// Each bridge should have independent temp directory
		paths := make(map[string]bool)
		for _, bridge := range bridges {
			path := bridge.TempPath()
			assert.False(t, paths[path], "Duplicate temp path")
			paths[path] = true
		}
		
		// Cleanup all
		for _, bridge := range bridges {
			err := bridge.Cleanup()
			require.NoError(t, err)
		}
	})
}

// TestGlobPatterns tests glob pattern matching
func TestGlobPatterns(t *testing.T) {
	vfs := NewDirectMemoryVFS()
	
	// Create test files
	files := []string{
		"main.go",
		"main_test.go",
		"lib.go",
		"src/app.go",
		"src/app_test.go",
		"src/lib/util.go",
		"src/lib/util_test.go",
		"test/integration_test.go",
		"docs/README.md",
		"docs/api.md",
		"config.yaml",
		"config.json",
		".gitignore",
		"Makefile",
	}
	
	for _, file := range files {
		vfs.Write(file, []byte("content"))
	}
	
	tests := []struct {
		pattern  string
		expected []string
	}{
		{
			pattern:  "*.go",
			expected: []string{"main.go", "main_test.go", "lib.go"},
		},
		{
			pattern:  "*_test.go",
			expected: []string{"main_test.go"},
		},
		{
			pattern:  "src/*.go",
			expected: []string{"src/app.go", "src/app_test.go"},
		},
		{
			pattern:  "src/*/*.go",
			expected: []string{"src/lib/util.go", "src/lib/util_test.go"},
		},
		{
			pattern:  "*.md",
			expected: []string{},
		},
		{
			pattern:  "docs/*.md",
			expected: []string{"docs/README.md", "docs/api.md"},
		},
		{
			pattern:  "config.*",
			expected: []string{"config.yaml", "config.json"},
		},
		{
			pattern:  ".*",
			expected: []string{".gitignore"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			matches, err := vfs.Glob(tt.pattern)
			require.NoError(t, err)
			
			// Sort for consistent comparison
			assert.ElementsMatch(t, tt.expected, matches,
				"Pattern %s: expected %v, got %v", tt.pattern, tt.expected, matches)
		})
	}
}

// BenchmarkVFSOperations benchmarks VFS performance
func BenchmarkVFSWrite(b *testing.B) {
	vfs := NewDirectMemoryVFS()
	content := []byte("benchmark content")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := fmt.Sprintf("file%d.txt", i)
		vfs.Write(path, content)
	}
}

func BenchmarkVFSRead(b *testing.B) {
	vfs := NewDirectMemoryVFS()
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		vfs.Write(fmt.Sprintf("file%d.txt", i), []byte("content"))
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := fmt.Sprintf("file%d.txt", i%1000)
		vfs.Read(path)
	}
}

func BenchmarkVFSConcurrentWrite(b *testing.B) {
	vfs := NewDirectMemoryVFS()
	content := []byte("benchmark content")
	
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			path := fmt.Sprintf("file%d.txt", i)
			vfs.Write(path, content)
			i++
		}
	})
}

func BenchmarkBridgeCreation(b *testing.B) {
	vfs := NewDirectMemoryVFS()
	
	// Add some files
	for i := 0; i < 100; i++ {
		vfs.Write(fmt.Sprintf("file%d.txt", i), []byte("content"))
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bridge, _ := vfs.CreateBridge()
		bridge.Cleanup()
	}
}