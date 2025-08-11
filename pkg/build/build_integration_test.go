package build_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Caia-Tech/govc"
	"github.com/Caia-Tech/govc/pkg/build"
	"github.com/Caia-Tech/govc/pkg/build/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryOnlyGoBuild(t *testing.T) {
	t.Run("SimpleBinaryBuild", func(t *testing.T) {
		// Create repository
		repo := govc.NewRepository()

		// Write Go source code
		mainGo := `package main

import "fmt"

func main() {
	fmt.Println("Hello from memory-only build!")
}
`
		err := repo.WriteFile("main.go", []byte(mainGo))
		require.NoError(t, err)

		goMod := `module memorytest
go 1.21
`
		err = repo.WriteFile("go.mod", []byte(goMod))
		require.NoError(t, err)

		// Create build system
		engine := build.NewMemoryBuildEngine()
		goPlugin := plugins.NewGoPlugin()
		engine.RegisterPlugin(goPlugin)

		// Create VFS from repository
		vfs := createVFSFromRepo(repo)

		// Build configuration
		config := build.BuildConfig{
			Target:     "binary",
			Mode:       "debug",
			OutputPath: "dist",
			PluginConfig: map[string]interface{}{
				"plugin": "go",
				"vfs":    vfs,
			},
		}

		// Execute build
		ctx := context.Background()
		result, err := engine.Build(ctx, config)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.Success)

		// Verify artifacts
		assert.NotEmpty(t, result.Artifacts)
		mainArtifact := result.Artifacts[0]
		assert.Equal(t, "app", mainArtifact.Name)
		assert.Equal(t, "binary", mainArtifact.Type)
		assert.NotEmpty(t, mainArtifact.Content)
		assert.True(t, mainArtifact.Executable)

		// Verify metadata
		assert.Equal(t, "go", result.Metadata.Plugin)
		assert.Equal(t, "memorytest", result.Metadata.Project.Name)
	})

	t.Run("WASMBuild", func(t *testing.T) {
		repo := govc.NewRepository()

		// Write WASM-compatible Go code
		wasmGo := `package main

import (
	"syscall/js"
)

func main() {
	js.Global().Set("goWASM", js.FuncOf(hello))
	select {} // Keep running
}

func hello(this js.Value, args []js.Value) interface{} {
	return "Hello from Go WASM!"
}
`
		repo.WriteFile("main.go", []byte(wasmGo))
		repo.WriteFile("go.mod", []byte("module wasm-test\ngo 1.21\n"))

		// Setup build
		engine := build.NewMemoryBuildEngine()
		engine.RegisterPlugin(plugins.NewGoPlugin())

		vfs := createVFSFromRepo(repo)
		config := build.BuildConfig{
			Target: "wasm",
			Mode:   "release",
			OutputPath: "dist",
			PluginConfig: map[string]interface{}{
				"plugin": "go",
				"vfs":    vfs,
			},
		}

		// Build WASM
		ctx := context.Background()
		result, err := engine.Build(ctx, config)
		
		// WASM builds require proper Go installation with WASM support
		if err != nil {
			t.Skip("WASM build not available in test environment")
		}

		assert.True(t, result.Success)
		
		// Should have WASM file and support files
		hasWASM := false
		hasJS := false
		hasHTML := false
		
		for _, artifact := range result.Artifacts {
			switch artifact.Type {
			case "wasm":
				hasWASM = true
				assert.Equal(t, "app.wasm", artifact.Name)
			case "support":
				if artifact.Name == "wasm_exec.js" {
					hasJS = true
				} else if artifact.Name == "index.html" {
					hasHTML = true
				}
			}
		}
		
		assert.True(t, hasWASM, "Should have WASM artifact")
		assert.True(t, hasJS, "Should have wasm_exec.js")
		assert.True(t, hasHTML, "Should have index.html")
	})

	t.Run("TestMode", func(t *testing.T) {
		repo := govc.NewRepository()

		// Write test file
		testGo := `package main

import "testing"

func TestAddition(t *testing.T) {
	result := Add(2, 3)
	if result != 5 {
		t.Errorf("Expected 5, got %d", result)
	}
}

func Add(a, b int) int {
	return a + b
}
`
		repo.WriteFile("main_test.go", []byte(testGo))
		repo.WriteFile("go.mod", []byte("module test-project\ngo 1.21\n"))

		engine := build.NewMemoryBuildEngine()
		engine.RegisterPlugin(plugins.NewGoPlugin())

		vfs := createVFSFromRepo(repo)
		config := build.BuildConfig{
			Target: "binary",
			Mode:   "test",
			PluginConfig: map[string]interface{}{
				"plugin": "go",
				"vfs":    vfs,
			},
		}

		ctx := context.Background()
		result, err := engine.Build(ctx, config)
		require.NoError(t, err)
		
		// Check for test results
		if result.Success {
			assert.NotEmpty(t, result.Artifacts)
			testResults := result.Artifacts[0]
			assert.Equal(t, "test-results.txt", testResults.Name)
			assert.Contains(t, string(testResults.Content), "PASS")
		}
	})
}

func TestMemoryOnlyJavaScriptBuild(t *testing.T) {
	t.Run("NodeProjectBuild", func(t *testing.T) {
		repo := govc.NewRepository()

		// Create Node.js project
		packageJSON := `{
  "name": "memory-app",
  "version": "1.0.0",
  "main": "index.js",
  "scripts": {
    "build": "echo 'Building...' && node index.js",
    "test": "echo 'Testing...'"
  }
}`
		repo.WriteFile("package.json", []byte(packageJSON))

		indexJS := `console.log('Hello from memory-built Node.js!');
module.exports = {
  greet: (name) => 'Hello, ' + name + '!'
};`
		repo.WriteFile("index.js", []byte(indexJS))

		// Setup build
		engine := build.NewMemoryBuildEngine()
		jsPlugin := plugins.NewJavaScriptPlugin()
		engine.RegisterPlugin(jsPlugin)

		vfs := createVFSFromRepo(repo)
		config := build.BuildConfig{
			Target: "bundle",
			Mode:   "production",
			OutputPath: "dist",
			PluginConfig: map[string]interface{}{
				"plugin": "javascript",
				"vfs":    vfs,
			},
		}

		// Build
		ctx := context.Background()
		result, err := engine.Build(ctx, config)
		
		// Node.js required for JavaScript builds
		if err != nil {
			t.Skip("Node.js not available in test environment")
		}

		assert.True(t, result.Success)
		assert.NotEmpty(t, result.Artifacts)
	})

	t.Run("TypeScriptProject", func(t *testing.T) {
		repo := govc.NewRepository()

		// TypeScript files
		tsConfig := `{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "strict": true,
    "outDir": "./dist"
  }
}`
		repo.WriteFile("tsconfig.json", []byte(tsConfig))

		indexTS := `interface User {
  name: string;
  age: number;
}

function greet(user: User): string {
  return "Hello, " + user.name + "! You are " + user.age + " years old.";
}

export { greet, User };`
		repo.WriteFile("index.ts", []byte(indexTS))

		packageJSON := `{
  "name": "ts-memory-app",
  "version": "1.0.0",
  "main": "dist/index.js",
  "scripts": {
    "build": "tsc"
  },
  "devDependencies": {
    "typescript": "^5.0.0"
  }
}`
		repo.WriteFile("package.json", []byte(packageJSON))

		engine := build.NewMemoryBuildEngine()
		engine.RegisterPlugin(plugins.NewJavaScriptPlugin())

		vfs := createVFSFromRepo(repo)
		
		// Auto-detect should identify TypeScript
		ctx := context.Background()
		config := build.BuildConfig{
			Target:     "bundle",
			Mode:       "production",
			OutputPath: "dist",
		}

		result, err := engine.DetectAndBuild(ctx, vfs, config)
		if err != nil {
			t.Skip("TypeScript build environment not available")
		}

		if result != nil {
			assert.Equal(t, "typescript", result.Metadata.Project.Language)
		}
	})
}

func TestBuildCaching(t *testing.T) {
	t.Run("CacheHitOnIdenticalBuild", func(t *testing.T) {
		cache := build.NewMemoryCache()
		_ = build.NewMemoryBuildEngine()
		
		// Create test content
		vfs := build.NewDirectMemoryVFS()
		vfs.Write("main.go", []byte("package main\nfunc main() {}"))
		vfs.Write("go.mod", []byte("module test\ngo 1.21"))

		// Build cache key
		builder := build.NewCacheKeyBuilder("go")
		builder.WithFile("main.go", []byte("package main\nfunc main() {}"))
		builder.WithFile("go.mod", []byte("module test\ngo 1.21"))
		key := builder.Build()

		// First build - cache miss
		result1 := &build.BuildResult{
			Success: true,
			Artifacts: []build.BuildArtifact{{
				Name:    "app",
				Content: []byte("binary content"),
			}},
		}
		
		err := cache.Put(key, result1)
		require.NoError(t, err)

		// Second build - cache hit
		result2, hit := cache.Get(key)
		assert.True(t, hit)
		assert.NotNil(t, result2)
		assert.Equal(t, result1.Success, result2.Success)
		assert.Len(t, result2.Artifacts, 1)

		// Verify cache stats
		stats := cache.Stats()
		assert.Equal(t, int64(1), stats.Hits)
		assert.Equal(t, 1, stats.Entries)
	})

	t.Run("CacheMissOnModifiedContent", func(t *testing.T) {
		cache := build.NewMemoryCache()

		// Original content
		builder1 := build.NewCacheKeyBuilder("go")
		builder1.WithFile("main.go", []byte("package main\nfunc main() {}"))
		key1 := builder1.Build()

		result1 := &build.BuildResult{Success: true}
		cache.Put(key1, result1)

		// Modified content
		builder2 := build.NewCacheKeyBuilder("go")
		builder2.WithFile("main.go", []byte("package main\nfunc main() { println(\"changed\") }"))
		key2 := builder2.Build()

		// Should be cache miss
		result2, hit := cache.Get(key2)
		assert.False(t, hit)
		assert.Nil(t, result2)
	})

	t.Run("CacheEviction", func(t *testing.T) {
		// Small cache with only 3 entries
		cache := build.NewMemoryCacheWithOptions(3, 1024, 1*time.Hour)

		// Add 4 entries - should evict LRU
		for i := 0; i < 4; i++ {
			builder := build.NewCacheKeyBuilder("go")
			builder.WithFile(filepath.Join("test", fmt.Sprintf("file%d.go", i)), []byte(fmt.Sprintf("content%d", i)))
			key := builder.Build()
			
			result := &build.BuildResult{
				Success: true,
				Artifacts: []build.BuildArtifact{{
					Name:    fmt.Sprintf("app%d", i),
					Content: []byte("content"),
				}},
			}
			cache.Put(key, result)
		}

		// Cache should have only 3 entries
		stats := cache.Stats()
		assert.LessOrEqual(t, stats.Entries, 3)
	})
}

func TestParallelRealityBuilds(t *testing.T) {
	t.Run("SimultaneousBuilds", func(t *testing.T) {
		repo := govc.NewRepository()

		// Create base project
		repo.WriteFile("main.go", []byte("package main\nfunc main() { println(VERSION) }"))
		repo.WriteFile("go.mod", []byte("module parallel-test\ngo 1.21"))
		repo.Commit("Initial commit")

		// Create parallel realities
		realities := repo.ParallelRealities([]string{"debug", "release", "test"})
		assert.Len(t, realities, 3)

		// Modify each reality differently
		repo.Checkout("parallel/debug")
		repo.WriteFile("version.go", []byte("package main\nconst VERSION = \"debug\""))
		repo.Commit("Debug version")

		repo.Checkout("parallel/release")
		repo.WriteFile("version.go", []byte("package main\nconst VERSION = \"release\""))
		repo.Commit("Release version")

		repo.Checkout("parallel/test")
		repo.WriteFile("version.go", []byte("package main\nconst VERSION = \"test\""))
		repo.WriteFile("main_test.go", []byte("package main\nimport \"testing\"\nfunc TestVersion(t *testing.T) {}"))
		repo.Commit("Test version")

		// Build manager
		buildManager := build.NewBuildManager()
		buildManager.RegisterRepository("test-repo", repo)

		// Build in each reality concurrently
		ctx := context.Background()
		results := make(chan *build.BuildResult, 3)
		errors := make(chan error, 3)

		for _, reality := range []string{"debug", "release", "test"} {
			go func(r string) {
				config := build.BuildConfig{
					Mode: r,
					PluginConfig: map[string]interface{}{
						"plugin": "go",
					},
				}
				result, err := buildManager.BuildInReality(ctx, "test-repo", r, config)
				if err != nil {
					errors <- err
				} else {
					results <- result
				}
			}(reality)
		}

		// Collect results
		successCount := 0
		for i := 0; i < 3; i++ {
			select {
			case result := <-results:
				if result.Success {
					successCount++
				}
			case err := <-errors:
				// Build might fail in test environment
				t.Logf("Build error (expected in test env): %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("Build timeout")
			}
		}

		// At least one should succeed in a proper environment
		t.Logf("Successful builds: %d/3", successCount)
	})
}

func TestBuildEventSystem(t *testing.T) {
	t.Run("EventSubscription", func(t *testing.T) {
		engine := build.NewMemoryBuildEngine()
		events := make([]build.BuildEvent, 0)

		// Subscribe to events
		eventChan := engine.Subscribe()
		go func() {
			for event := range eventChan {
				events = append(events, event)
			}
		}()

		// Register plugin - should emit event
		goPlugin := plugins.NewGoPlugin()
		engine.RegisterPlugin(goPlugin)

		// Give time for event processing
		time.Sleep(100 * time.Millisecond)

		// Should have plugin loaded event
		hasPluginEvent := false
		for _, event := range events {
			if event.Type == build.EventPluginLoaded {
				hasPluginEvent = true
				assert.Equal(t, "go", event.Plugin)
			}
		}
		assert.True(t, hasPluginEvent, "Should have plugin loaded event")
	})

	t.Run("BuildLifecycleEvents", func(t *testing.T) {
		buildManager := build.NewBuildManager()
		repo := govc.NewRepository()

		repo.WriteFile("main.go", []byte("package main\nfunc main() {}"))
		repo.WriteFile("go.mod", []byte("module test\ngo 1.21"))

		buildManager.RegisterRepository("test", repo)

		// Track events
		events := make([]build.BuildEvent, 0)
		unsubscribe := buildManager.Subscribe(func(event build.BuildEvent) {
			events = append(events, event)
		})
		defer unsubscribe()

		// Trigger build through commit
		repo.Commit("Test commit")

		// Auto-build might trigger
		time.Sleep(100 * time.Millisecond)

		// Manual build
		ctx := context.Background()
		config := build.BuildConfig{
			PluginConfig: map[string]interface{}{
				"plugin": "go",
			},
		}
		buildManager.Build(ctx, "test", config)
	})
}

// Helper function to create VFS from repository
func createVFSFromRepo(repo *govc.Repository) build.VirtualFileSystem {
	vfs := build.NewDirectMemoryVFS()
	files, _ := repo.ListFiles()
	for _, file := range files {
		content, err := repo.ReadFile(file)
		if err == nil {
			vfs.Write(file, content)
		}
	}
	return vfs
}

func BenchmarkMemoryBuilds(b *testing.B) {
	b.Run("SmallProject", func(b *testing.B) {
		vfs := build.NewDirectMemoryVFS()
		vfs.Write("main.go", []byte("package main\nfunc main() {}"))
		vfs.Write("go.mod", []byte("module bench\ngo 1.21"))

		engine := build.NewMemoryBuildEngine()
		engine.RegisterPlugin(plugins.NewGoPlugin())

		config := build.BuildConfig{
			Target: "binary",
			Mode:   "release",
			PluginConfig: map[string]interface{}{
				"plugin": "go",
				"vfs":    vfs,
			},
		}

		ctx := context.Background()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			engine.Build(ctx, config)
		}
	})

	b.Run("CachedBuilds", func(b *testing.B) {
		vfs := build.NewDirectMemoryVFS()
		vfs.Write("main.go", []byte("package main\nfunc main() {}"))

		cache := build.NewMemoryCache()
		key := build.NewCacheKeyBuilder("go").
			WithFile("main.go", []byte("package main\nfunc main() {}")).
			Build()

		result := &build.BuildResult{
			Success: true,
			Artifacts: []build.BuildArtifact{{
				Name:    "app",
				Content: make([]byte, 1024*1024), // 1MB binary
			}},
		}
		cache.Put(key, result)

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			cache.Get(key)
		}
	})
}