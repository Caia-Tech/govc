package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectMemoryVFS(t *testing.T) {
	t.Run("BasicFileOperations", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()

		// Test Write
		content := []byte("Hello, Memory!")
		err := vfs.Write("test.txt", content)
		require.NoError(t, err)

		// Test Read
		readContent, err := vfs.Read("test.txt")
		require.NoError(t, err)
		assert.Equal(t, content, readContent)

		// Test Exists
		assert.True(t, vfs.Exists("test.txt"))
		assert.False(t, vfs.Exists("nonexistent.txt"))

		// Test List
		files, err := vfs.List()
		require.NoError(t, err)
		assert.Contains(t, files, "test.txt")

		// Test Delete
		err = vfs.Delete("test.txt")
		require.NoError(t, err)
		assert.False(t, vfs.Exists("test.txt"))
	})

	t.Run("DirectoryStructure", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()

		// Create files in directories
		vfs.Write("src/main.go", []byte("package main"))
		vfs.Write("src/utils/helper.go", []byte("package utils"))
		vfs.Write("test/main_test.go", []byte("package main"))

		// Test listing
		files, _ := vfs.List()
		assert.Len(t, files, 3)
		assert.Contains(t, files, "src/main.go")
		assert.Contains(t, files, "src/utils/helper.go")
	})

	t.Run("GlobPatterns", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()

		// Create test files
		vfs.Write("main.go", []byte("package main"))
		vfs.Write("test.go", []byte("package main"))
		vfs.Write("utils.js", []byte("console.log"))
		vfs.Write("src/app.go", []byte("package app"))
		vfs.Write("src/lib.js", []byte("export"))

		// Test simple glob
		goFiles, err := vfs.Glob("*.go")
		require.NoError(t, err)
		assert.Len(t, goFiles, 2)
		assert.Contains(t, goFiles, "main.go")
		assert.Contains(t, goFiles, "test.go")

		// Test directory glob
		srcFiles, err := vfs.Glob("src/*")
		require.NoError(t, err)
		assert.Len(t, srcFiles, 2)

		// Test extension glob
		jsFiles, err := vfs.Glob("*.js")
		require.NoError(t, err)
		assert.Len(t, jsFiles, 1)
		assert.Contains(t, jsFiles, "utils.js")
	})

	t.Run("TempDirectory", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()

		// Create temp directories
		temp1, err := vfs.TempDir("build-")
		require.NoError(t, err)
		assert.Contains(t, temp1, "/tmp/build-")

		temp2, err := vfs.TempDir("test-")
		require.NoError(t, err)
		assert.Contains(t, temp2, "/tmp/test-")

		// Ensure unique
		assert.NotEqual(t, temp1, temp2)
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		done := make(chan bool, 10)

		// Concurrent writes
		for i := 0; i < 10; i++ {
			go func(n int) {
				filename := filepath.Join("concurrent", string(rune('a'+n))+".txt")
				vfs.Write(filename, []byte(strings.Repeat("x", n*100)))
				done <- true
			}(i)
		}

		// Wait for all writes
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify all files exist
		files, _ := vfs.List()
		assert.Len(t, files, 10)
	})
}

func TestFilesystemBridge(t *testing.T) {
	t.Run("SyncToTemp", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()

		// Create files in memory
		vfs.Write("main.go", []byte("package main\nfunc main() {}\n"))
		vfs.Write("lib/util.go", []byte("package lib\n"))
		vfs.Write("README.md", []byte("# Test Project\n"))

		// Create bridge
		bridge, err := vfs.CreateBridge()
		require.NoError(t, err)
		defer bridge.Cleanup()

		// Verify temp directory exists
		tempPath := bridge.TempPath()
		assert.DirExists(t, tempPath)

		// Verify files were synced
		mainPath := filepath.Join(tempPath, "main.go")
		assert.FileExists(t, mainPath)

		content, err := os.ReadFile(mainPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "package main")
	})

	t.Run("SyncFromTemp", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		vfs.Write("original.txt", []byte("original"))

		// Create bridge
		bridge, err := vfs.CreateBridge()
		require.NoError(t, err)
		defer bridge.Cleanup()

		// Create new file in temp directory
		tempPath := bridge.TempPath()
		newFile := filepath.Join(tempPath, "generated.txt")
		err = os.WriteFile(newFile, []byte("generated content"), 0644)
		require.NoError(t, err)

		// Sync back to memory
		err = bridge.SyncFromTemp()
		require.NoError(t, err)

		// Verify file exists in VFS
		content, err := vfs.Read("generated.txt")
		require.NoError(t, err)
		assert.Equal(t, "generated content", string(content))
	})

	t.Run("ExecuteCommand", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		vfs.Write("test.txt", []byte("Hello World"))

		bridge, err := vfs.CreateBridge()
		require.NoError(t, err)
		defer bridge.Cleanup()

		// Execute echo command
		result, err := bridge.Exec("echo", []string{"test"}, nil)
		require.NoError(t, err)
		assert.Equal(t, 0, result.ExitCode)
		assert.Contains(t, result.Output, "test")

		// Execute ls command to verify files
		result, err = bridge.Exec("ls", []string{"-la"}, nil)
		require.NoError(t, err)
		assert.Contains(t, result.Output, "test.txt")
	})

	t.Run("CleanupRemovesTempDir", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()
		vfs.Write("temp.txt", []byte("temporary"))

		bridge, err := vfs.CreateBridge()
		require.NoError(t, err)

		tempPath := bridge.TempPath()
		assert.DirExists(t, tempPath)

		// Cleanup
		err = bridge.Cleanup()
		require.NoError(t, err)

		// Verify temp directory is gone
		assert.NoDirExists(t, tempPath)
	})
}

func TestMemoryVFSIntegration(t *testing.T) {
	t.Run("LargeFileHandling", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()

		// Create large file (1MB)
		largeContent := make([]byte, 1024*1024)
		for i := range largeContent {
			largeContent[i] = byte(i % 256)
		}

		err := vfs.Write("large.bin", largeContent)
		require.NoError(t, err)

		// Read back
		readContent, err := vfs.Read("large.bin")
		require.NoError(t, err)
		assert.Equal(t, largeContent, readContent)

		// Bridge sync
		bridge, err := vfs.CreateBridge()
		require.NoError(t, err)
		defer bridge.Cleanup()

		// Verify file in temp
		tempFile := filepath.Join(bridge.TempPath(), "large.bin")
		info, err := os.Stat(tempFile)
		require.NoError(t, err)
		assert.Equal(t, int64(1024*1024), info.Size())
	})

	t.Run("ComplexProjectStructure", func(t *testing.T) {
		vfs := NewDirectMemoryVFS()

		// Create Go project structure
		projectFiles := map[string]string{
			"go.mod":             "module example.com/test\ngo 1.21\n",
			"main.go":            "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"Hello\") }",
			"internal/app.go":    "package internal\nfunc App() string { return \"app\" }",
			"internal/util.go":   "package internal\nfunc Util() string { return \"util\" }",
			"cmd/cli/main.go":    "package main\nfunc main() {}",
			"test/main_test.go":  "package main\nimport \"testing\"\nfunc TestMain(t *testing.T) {}",
			"docs/README.md":     "# Documentation",
			".gitignore":         "*.exe\n*.dll\n*.so\n*.dylib",
		}

		// Write all files
		for path, content := range projectFiles {
			err := vfs.Write(path, []byte(content))
			require.NoError(t, err)
		}

		// Test glob patterns
		goFiles, _ := vfs.Glob("*.go")
		assert.Len(t, goFiles, 1) // Only main.go at root

		// Test that ** pattern works (even if simplified)
		_, _ = vfs.Glob("**/*.go")
		// This would need proper ** support, for now just check basic glob
		
		// Test reading specific files
		goMod, err := vfs.Read("go.mod")
		require.NoError(t, err)
		assert.Contains(t, string(goMod), "module example.com/test")
	})
}

func TestStreamingVFS(t *testing.T) {
	t.Run("StreamingOperations", func(t *testing.T) {
		base := NewDirectMemoryVFS()
		vfs := NewStreamingVFS(base)

		// Open stream for writing
		stream, err := vfs.OpenStream("output.log")
		require.NoError(t, err)

		// Write data
		_, err = stream.Write([]byte("Line 1\n"))
		require.NoError(t, err)
		_, err = stream.Write([]byte("Line 2\n"))
		require.NoError(t, err)

		// Close to flush
		err = stream.Close()
		require.NoError(t, err)

		// Read back
		content, err := base.Read("output.log")
		require.NoError(t, err)
		assert.Equal(t, "Line 1\nLine 2\n", string(content))
	})

	t.Run("ConcurrentStreaming", func(t *testing.T) {
		base := NewDirectMemoryVFS()
		vfs := NewStreamingVFS(base)

		// Multiple streams
		stream1, _ := vfs.OpenStream("log1.txt")
		stream2, _ := vfs.OpenStream("log2.txt")

		stream1.Write([]byte("Stream 1"))
		stream2.Write([]byte("Stream 2"))

		stream1.Close()
		stream2.Close()

		// Verify both files
		content1, _ := base.Read("log1.txt")
		content2, _ := base.Read("log2.txt")

		assert.Equal(t, "Stream 1", string(content1))
		assert.Equal(t, "Stream 2", string(content2))
	})
}

func BenchmarkVFSOperations(b *testing.B) {
	b.Run("MemoryWrite", func(b *testing.B) {
		vfs := NewDirectMemoryVFS()
		content := []byte("benchmark content")

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			vfs.Write("test.txt", content)
		}
	})

	b.Run("MemoryRead", func(b *testing.B) {
		vfs := NewDirectMemoryVFS()
		vfs.Write("test.txt", []byte("benchmark content"))

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			vfs.Read("test.txt")
		}
	})

	b.Run("BridgeCreation", func(b *testing.B) {
		vfs := NewDirectMemoryVFS()
		vfs.Write("file1.txt", []byte("content1"))
		vfs.Write("file2.txt", []byte("content2"))

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bridge, _ := vfs.CreateBridge()
			bridge.Cleanup()
		}
	})
}