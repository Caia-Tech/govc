package build

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Caia-Tech/govc/pkg/storage"
)

// MemoryVFS implements VirtualFileSystem using govc's memory storage
type MemoryVFS struct {
	storage     storage.WorkingStorage
	tempCounter int64
	tempDirs    map[string]bool
	mu          sync.RWMutex
}

// NewMemoryVFS creates a VFS backed by govc's memory storage
func NewMemoryVFS(storage storage.WorkingStorage) *MemoryVFS {
	return &MemoryVFS{
		storage:  storage,
		tempDirs: make(map[string]bool),
	}
}

// Read reads file content from memory
func (vfs *MemoryVFS) Read(path string) ([]byte, error) {
	return vfs.storage.Read(path)
}

// Write writes file content to memory
func (vfs *MemoryVFS) Write(path string, data []byte) error {
	return vfs.storage.Write(path, data)
}

// Delete removes file from memory
func (vfs *MemoryVFS) Delete(path string) error {
	return vfs.storage.Delete(path)
}

// List returns all files in memory
func (vfs *MemoryVFS) List() ([]string, error) {
	return vfs.storage.List()
}

// Glob returns files matching pattern
func (vfs *MemoryVFS) Glob(pattern string) ([]string, error) {
	files, err := vfs.storage.List()
	if err != nil {
		return nil, err
	}

	var matches []string
	for _, file := range files {
		matched, err := filepath.Match(pattern, file)
		if err != nil {
			// Try with ** pattern support
			if strings.Contains(pattern, "**") {
				if matchesDoubleWildcard(pattern, file) {
					matches = append(matches, file)
				}
			}
			continue
		}
		if matched {
			matches = append(matches, file)
		}
	}
	return matches, nil
}

// Exists checks if file exists in memory
func (vfs *MemoryVFS) Exists(path string) bool {
	_, err := vfs.storage.Read(path)
	return err == nil
}

// TempDir creates a temporary directory path in memory
func (vfs *MemoryVFS) TempDir(prefix string) (string, error) {
	counter := atomic.AddInt64(&vfs.tempCounter, 1)
	
	// Generate random suffix
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	
	tempDir := fmt.Sprintf("/tmp/%s%d_%s", prefix, counter, hex.EncodeToString(b))
	
	vfs.mu.Lock()
	vfs.tempDirs[tempDir] = true
	vfs.mu.Unlock()
	
	return tempDir, nil
}

// CreateBridge creates a temporary bridge to real filesystem
func (vfs *MemoryVFS) CreateBridge() (FilesystemBridge, error) {
	// Create real temp directory
	tempDir, err := os.MkdirTemp("", "govc-build-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	bridge := &MemoryFSBridge{
		vfs:     vfs,
		tempDir: tempDir,
	}

	// Sync memory files to temp directory
	if err := bridge.SyncToTemp(); err != nil {
		bridge.Cleanup()
		return nil, err
	}

	return bridge, nil
}

// MemoryFSBridge bridges memory VFS to real filesystem
type MemoryFSBridge struct {
	vfs     *MemoryVFS
	tempDir string
	mu      sync.Mutex
}

// TempPath returns the temporary directory path
func (b *MemoryFSBridge) TempPath() string {
	return b.tempDir
}

// SyncToTemp writes all memory files to temporary directory
func (b *MemoryFSBridge) SyncToTemp() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	files, err := b.vfs.List()
	if err != nil {
		return fmt.Errorf("failed to list files: %w", err)
	}

	for _, file := range files {
		// Read from memory
		content, err := b.vfs.Read(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}

		// Create full path in temp directory
		fullPath := filepath.Join(b.tempDir, file)
		
		// Create directory structure
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// Write to temp file
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", fullPath, err)
		}
	}

	return nil
}

// SyncFromTemp reads files from temp directory back to memory
func (b *MemoryFSBridge) SyncFromTemp() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Walk temp directory and sync changes back
	return filepath.Walk(b.tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Calculate relative path
		relPath, err := filepath.Rel(b.tempDir, path)
		if err != nil {
			return err
		}

		// Skip hidden files and directories
		if strings.HasPrefix(relPath, ".") || strings.Contains(relPath, "/.") {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		// Write back to memory
		if err := b.vfs.Write(relPath, content); err != nil {
			return fmt.Errorf("failed to write %s to memory: %w", relPath, err)
		}

		return nil
	})
}

// Exec executes command with access to temp directory
func (b *MemoryFSBridge) Exec(cmd string, args []string, env map[string]string) (*ExecResult, error) {
	start := time.Now()
	
	command := exec.Command(cmd, args...)
	command.Dir = b.tempDir

	// Set environment variables
	command.Env = os.Environ()
	for k, v := range env {
		command.Env = append(command.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Capture output
	output, err := command.CombinedOutput()
	
	exitCode := 0
	if command.ProcessState != nil {
		exitCode = command.ProcessState.ExitCode()
	}

	// Sync changes back to memory after execution
	if syncErr := b.SyncFromTemp(); syncErr != nil {
		return nil, fmt.Errorf("failed to sync after exec: %w", syncErr)
	}

	return &ExecResult{
		Output:   string(output),
		ExitCode: exitCode,
		Error:    err,
		Duration: time.Since(start),
	}, nil
}

// Cleanup removes temporary directory
func (b *MemoryFSBridge) Cleanup() error {
	return os.RemoveAll(b.tempDir)
}

// DirectMemoryVFS is a pure in-memory VFS without any storage backend
type DirectMemoryVFS struct {
	files    map[string][]byte
	tempDirs map[string]bool
	mu       sync.RWMutex
	counter  int64
}

// NewDirectMemoryVFS creates a pure in-memory VFS
func NewDirectMemoryVFS() *DirectMemoryVFS {
	return &DirectMemoryVFS{
		files:    make(map[string][]byte),
		tempDirs: make(map[string]bool),
	}
}

// Read reads file from memory map
func (vfs *DirectMemoryVFS) Read(path string) ([]byte, error) {
	vfs.mu.RLock()
	defer vfs.mu.RUnlock()

	content, exists := vfs.files[path]
	if !exists {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	
	// Return a copy to prevent mutations
	result := make([]byte, len(content))
	copy(result, content)
	return result, nil
}

// Write writes file to memory map
func (vfs *DirectMemoryVFS) Write(path string, data []byte) error {
	vfs.mu.Lock()
	defer vfs.mu.Unlock()

	// Store a copy to prevent external mutations
	content := make([]byte, len(data))
	copy(content, data)
	vfs.files[path] = content
	return nil
}

// Delete removes file from memory map
func (vfs *DirectMemoryVFS) Delete(path string) error {
	vfs.mu.Lock()
	defer vfs.mu.Unlock()

	delete(vfs.files, path)
	return nil
}

// List returns all file paths
func (vfs *DirectMemoryVFS) List() ([]string, error) {
	vfs.mu.RLock()
	defer vfs.mu.RUnlock()

	paths := make([]string, 0, len(vfs.files))
	for path := range vfs.files {
		paths = append(paths, path)
	}
	return paths, nil
}

// Glob matches files against pattern
func (vfs *DirectMemoryVFS) Glob(pattern string) ([]string, error) {
	vfs.mu.RLock()
	defer vfs.mu.RUnlock()

	var matches []string
	for path := range vfs.files {
		matched, err := filepath.Match(pattern, path)
		if err != nil {
			if strings.Contains(pattern, "**") && matchesDoubleWildcard(pattern, path) {
				matches = append(matches, path)
			}
			continue
		}
		if matched {
			matches = append(matches, path)
		}
	}
	return matches, nil
}

// Exists checks if file exists
func (vfs *DirectMemoryVFS) Exists(path string) bool {
	vfs.mu.RLock()
	defer vfs.mu.RUnlock()
	_, exists := vfs.files[path]
	return exists
}

// TempDir creates a temporary directory path
func (vfs *DirectMemoryVFS) TempDir(prefix string) (string, error) {
	counter := atomic.AddInt64(&vfs.counter, 1)
	tempDir := fmt.Sprintf("/tmp/%s%d", prefix, counter)
	
	vfs.mu.Lock()
	vfs.tempDirs[tempDir] = true
	vfs.mu.Unlock()
	
	return tempDir, nil
}

// CreateBridge creates a filesystem bridge for DirectMemoryVFS
func (vfs *DirectMemoryVFS) CreateBridge() (FilesystemBridge, error) {
	tempDir, err := os.MkdirTemp("", "govc-build-")
	if err != nil {
		return nil, err
	}

	bridge := &DirectMemoryBridge{
		vfs:     vfs,
		tempDir: tempDir,
	}

	if err := bridge.SyncToTemp(); err != nil {
		bridge.Cleanup()
		return nil, err
	}

	return bridge, nil
}

// DirectMemoryBridge bridges DirectMemoryVFS to filesystem
type DirectMemoryBridge struct {
	vfs     *DirectMemoryVFS
	tempDir string
	mu      sync.Mutex
}

// TempPath returns temp directory path
func (b *DirectMemoryBridge) TempPath() string {
	return b.tempDir
}

// SyncToTemp syncs memory to filesystem
func (b *DirectMemoryBridge) SyncToTemp() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.vfs.mu.RLock()
	defer b.vfs.mu.RUnlock()

	for path, content := range b.vfs.files {
		fullPath := filepath.Join(b.tempDir, path)
		dir := filepath.Dir(fullPath)
		
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			return err
		}
	}
	
	return nil
}

// SyncFromTemp syncs filesystem back to memory
func (b *DirectMemoryBridge) SyncFromTemp() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return filepath.Walk(b.tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		relPath, err := filepath.Rel(b.tempDir, path)
		if err != nil {
			return err
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return b.vfs.Write(relPath, content)
	})
}

// Exec executes command with filesystem access
func (b *DirectMemoryBridge) Exec(cmd string, args []string, env map[string]string) (*ExecResult, error) {
	start := time.Now()
	
	command := exec.Command(cmd, args...)
	command.Dir = b.tempDir

	command.Env = os.Environ()
	for k, v := range env {
		command.Env = append(command.Env, k+"="+v)
	}

	output, err := command.CombinedOutput()
	
	exitCode := 0
	if command.ProcessState != nil {
		exitCode = command.ProcessState.ExitCode()
	}

	// Sync back to memory
	if syncErr := b.SyncFromTemp(); syncErr != nil {
		return nil, syncErr
	}

	return &ExecResult{
		Output:   string(output),
		ExitCode: exitCode,
		Error:    err,
		Duration: time.Since(start),
	}, nil
}

// Cleanup removes temp directory
func (b *DirectMemoryBridge) Cleanup() error {
	return os.RemoveAll(b.tempDir)
}

// StreamingVFS wraps a VFS to provide streaming capabilities
type StreamingVFS struct {
	base VirtualFileSystem
	streams map[string]io.ReadWriteCloser
	mu sync.RWMutex
}

// NewStreamingVFS creates a streaming-capable VFS
func NewStreamingVFS(base VirtualFileSystem) *StreamingVFS {
	return &StreamingVFS{
		base: base,
		streams: make(map[string]io.ReadWriteCloser),
	}
}

// OpenStream opens a file for streaming
func (s *StreamingVFS) OpenStream(path string) (io.ReadWriteCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if stream, exists := s.streams[path]; exists {
		return stream, nil
	}

	// Create a new stream backed by memory
	content, err := s.base.Read(path)
	if err != nil {
		content = []byte{} // New file
	}

	stream := &MemoryStream{
		path: path,
		vfs: s.base,
		buffer: content,
	}

	s.streams[path] = stream
	return stream, nil
}

// MemoryStream provides streaming access to memory files
type MemoryStream struct {
	path   string
	vfs    VirtualFileSystem
	buffer []byte
	pos    int
	mu     sync.Mutex
}

// Read implements io.Reader
func (m *MemoryStream) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pos >= len(m.buffer) {
		return 0, io.EOF
	}

	n = copy(p, m.buffer[m.pos:])
	m.pos += n
	return n, nil
}

// Write implements io.Writer
func (m *MemoryStream) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.buffer = append(m.buffer, p...)
	return len(p), nil
}

// Close implements io.Closer
func (m *MemoryStream) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Write buffer back to VFS
	return m.vfs.Write(m.path, m.buffer)
}

// Helper function for ** glob patterns
func matchesDoubleWildcard(pattern, path string) bool {
	// Simple implementation of ** matching
	// Convert ** to a regex-like pattern
	pattern = strings.ReplaceAll(pattern, "**", ".*")
	pattern = strings.ReplaceAll(pattern, "*", "[^/]*")
	pattern = "^" + pattern + "$"
	
	// This is simplified - in production use a proper glob library
	return strings.Contains(path, strings.ReplaceAll(pattern, ".*", ""))
}