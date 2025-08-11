package build_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Caia-Tech/govc"
	"github.com/Caia-Tech/govc/pkg/build"
	"github.com/Caia-Tech/govc/pkg/build/plugins"
)

func Example_memoryBuild() {
	// Create a pure in-memory repository
	repo := govc.NewRepository()
	
	// Write Go source code directly to memory
	repo.WriteFile("main.go", []byte(`
package main

import "fmt"

func main() {
    fmt.Println("Hello from memory-only build!")
}
`))
	
	repo.WriteFile("go.mod", []byte(`
module example
go 1.21
`))
	
	// Create build engine with Go plugin
	engine := build.NewMemoryBuildEngine()
	goPlugin := plugins.NewGoPlugin()
	engine.RegisterPlugin(goPlugin)
	
	// Create VFS from repository
	vfs := build.NewDirectMemoryVFS()
	files, _ := repo.ListFiles()
	for _, file := range files {
		content, _ := repo.ReadFile(file)
		vfs.Write(file, content)
	}
	
	// Configure build
	config := build.BuildConfig{
		Target: "binary",
		Mode:   "debug",
		OutputPath: "dist",
		PluginConfig: map[string]interface{}{
			"plugin": "go",
			"vfs":    vfs,
		},
	}
	
	// Build entirely in memory!
	ctx := context.Background()
	result, err := engine.Build(ctx, config)
	
	if err != nil {
		fmt.Printf("Build failed: %v\n", err)
	} else if result.Success {
		fmt.Printf("Build successful! Created %d artifacts\n", len(result.Artifacts))
	}
	
	// Output: Build successful! Created 1 artifacts
}

func TestMemoryVFSBasic(t *testing.T) {
	// Create memory VFS
	vfs := build.NewDirectMemoryVFS()
	
	// Write file to memory
	err := vfs.Write("test.txt", []byte("Hello, Memory!"))
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	
	// Read file from memory
	content, err := vfs.Read("test.txt")
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}
	
	if string(content) != "Hello, Memory!" {
		t.Errorf("Expected 'Hello, Memory!', got %s", string(content))
	}
	
	// Test file exists
	if !vfs.Exists("test.txt") {
		t.Error("File should exist")
	}
	
	// List files
	files, err := vfs.List()
	if err != nil {
		t.Fatalf("Failed to list: %v", err)
	}
	
	if len(files) != 1 || files[0] != "test.txt" {
		t.Errorf("Expected ['test.txt'], got %v", files)
	}
}

func TestFilesystemBridge(t *testing.T) {
	// Create memory VFS with content
	vfs := build.NewDirectMemoryVFS()
	vfs.Write("hello.txt", []byte("Hello from memory"))
	
	// Create bridge to real filesystem
	bridge, err := vfs.CreateBridge()
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}
	defer bridge.Cleanup()
	
	// Execute command in temp directory
	result, err := bridge.Exec("ls", []string{"-la"}, nil)
	if err != nil && result.ExitCode != 0 {
		t.Fatalf("Command failed: %v", err)
	}
	
	// Verify file was synced
	if !contains(result.Output, "hello.txt") {
		t.Errorf("Expected file not found in output: %s", result.Output)
	}
}

func TestCacheSystem(t *testing.T) {
	cache := build.NewMemoryCache()
	
	// Create cache key
	builder := build.NewCacheKeyBuilder("go")
	builder.WithFile("main.go", []byte("package main"))
	key := builder.Build()
	
	// Store result
	result := &build.BuildResult{
		Success: true,
		Artifacts: []build.BuildArtifact{
			{
				Name: "app",
				Content: []byte("binary content"),
			},
		},
	}
	
	err := cache.Put(key, result)
	if err != nil {
		t.Fatalf("Failed to cache: %v", err)
	}
	
	// Retrieve from cache
	cached, hit := cache.Get(key)
	if !hit {
		t.Fatal("Expected cache hit")
	}
	
	if !cached.Success {
		t.Error("Cached result should be successful")
	}
	
	if len(cached.Artifacts) != 1 {
		t.Errorf("Expected 1 artifact, got %d", len(cached.Artifacts))
	}
	
	// Check stats
	stats := cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && 
		(s == substr || len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
		 findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}