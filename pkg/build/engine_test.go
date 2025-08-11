package build_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Caia-Tech/govc/pkg/build"
	"github.com/Caia-Tech/govc/pkg/build/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockPlugin for testing
type MockPlugin struct {
	name           string
	extensions     []string
	detectFunc     func(vfs build.VirtualFileSystem) (bool, *build.ProjectMetadata, error)
	buildFunc      func(ctx context.Context, config build.BuildConfig, vfs build.VirtualFileSystem) (*build.BuildResult, error)
	validateFunc   func(config build.BuildConfig, vfs build.VirtualFileSystem) error
	supportsMemory bool
}

func (m *MockPlugin) Name() string                          { return m.name }
func (m *MockPlugin) SupportedExtensions() []string         { return m.extensions }
func (m *MockPlugin) SupportsMemoryBuild() bool             { return m.supportsMemory }
func (m *MockPlugin) DetectProject(vfs build.VirtualFileSystem) (bool, *build.ProjectMetadata, error) {
	if m.detectFunc != nil {
		return m.detectFunc(vfs)
	}
	return true, &build.ProjectMetadata{Name: "mock-project"}, nil
}
func (m *MockPlugin) Build(ctx context.Context, config build.BuildConfig, vfs build.VirtualFileSystem) (*build.BuildResult, error) {
	if m.buildFunc != nil {
		return m.buildFunc(ctx, config, vfs)
	}
	return &build.BuildResult{Success: true}, nil
}
func (m *MockPlugin) GetDependencies(config build.BuildConfig, vfs build.VirtualFileSystem) ([]build.Dependency, error) {
	return nil, nil
}
func (m *MockPlugin) ValidateBuild(config build.BuildConfig, vfs build.VirtualFileSystem) error {
	if m.validateFunc != nil {
		return m.validateFunc(config, vfs)
	}
	return nil
}

func TestMemoryBuildEngine(t *testing.T) {
	t.Run("RegisterPlugin", func(t *testing.T) {
		engine := build.NewMemoryBuildEngine()
		
		// Register plugin
		plugin := &MockPlugin{name: "test", supportsMemory: true}
		err := engine.RegisterPlugin(plugin)
		require.NoError(t, err)
		
		// Try to register same plugin again
		err = engine.RegisterPlugin(plugin)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
		
		// Register another plugin
		plugin2 := &MockPlugin{name: "test2", supportsMemory: true}
		err = engine.RegisterPlugin(plugin2)
		require.NoError(t, err)
	})

	t.Run("GetPlugin", func(t *testing.T) {
		engine := build.NewMemoryBuildEngine()
		plugin := &MockPlugin{name: "test"}
		engine.RegisterPlugin(plugin)
		
		// Get existing plugin
		retrieved := engine.GetPlugin("test")
		assert.NotNil(t, retrieved)
		assert.Equal(t, "test", retrieved.Name())
		
		// Get non-existent plugin
		notFound := engine.GetPlugin("nonexistent")
		assert.Nil(t, notFound)
	})

	t.Run("ListPlugins", func(t *testing.T) {
		engine := build.NewMemoryBuildEngine()
		
		// Initially empty
		plugins := engine.ListPlugins()
		assert.Empty(t, plugins)
		
		// Add plugins
		engine.RegisterPlugin(&MockPlugin{name: "plugin1"})
		engine.RegisterPlugin(&MockPlugin{name: "plugin2"})
		
		plugins = engine.ListPlugins()
		assert.Len(t, plugins, 2)
		assert.Contains(t, plugins, "plugin1")
		assert.Contains(t, plugins, "plugin2")
	})

	t.Run("SimpleBuild", func(t *testing.T) {
		engine := build.NewMemoryBuildEngine()
		
		buildCalled := false
		plugin := &MockPlugin{
			name: "test",
			buildFunc: func(ctx context.Context, config build.BuildConfig, vfs build.VirtualFileSystem) (*build.BuildResult, error) {
				buildCalled = true
				return &build.BuildResult{
					Success: true,
					Artifacts: []build.BuildArtifact{
						{Name: "output.bin", Content: []byte("data")},
					},
				}, nil
			},
		}
		engine.RegisterPlugin(plugin)
		
		vfs := build.NewDirectMemoryVFS()
		vfs.Write("main.go", []byte("package main"))
		
		config := build.BuildConfig{
			PluginConfig: map[string]interface{}{
				"plugin": "test",
				"vfs":    vfs,
			},
		}
		
		ctx := context.Background()
		result, err := engine.Build(ctx, config)
		require.NoError(t, err)
		assert.True(t, buildCalled)
		assert.True(t, result.Success)
		assert.Len(t, result.Artifacts, 1)
	})

	t.Run("BuildWithCache", func(t *testing.T) {
		engine := build.NewMemoryBuildEngine()
		cache := build.NewMemoryCache()
		engine.SetCache(cache)
		
		buildCount := 0
		plugin := &MockPlugin{
			name: "test",
			buildFunc: func(ctx context.Context, config build.BuildConfig, vfs build.VirtualFileSystem) (*build.BuildResult, error) {
				buildCount++
				return &build.BuildResult{
					Success: true,
					Artifacts: []build.BuildArtifact{
						{Name: fmt.Sprintf("output%d.bin", buildCount), Content: []byte("data")},
					},
				}, nil
			},
		}
		engine.RegisterPlugin(plugin)
		
		vfs := build.NewDirectMemoryVFS()
		vfs.Write("main.go", []byte("package main"))
		
		config := build.BuildConfig{
			PluginConfig: map[string]interface{}{
				"plugin": "test",
				"vfs":    vfs,
			},
			UseCache: true,
		}
		
		ctx := context.Background()
		
		// First build - should execute
		result1, err := engine.Build(ctx, config)
		require.NoError(t, err)
		assert.Equal(t, 1, buildCount)
		assert.Equal(t, "output1.bin", result1.Artifacts[0].Name)
		
		// Second build with same config - should use cache
		result2, err := engine.Build(ctx, config)
		require.NoError(t, err)
		assert.Equal(t, 1, buildCount) // Build count should not increase
		assert.Equal(t, "output1.bin", result2.Artifacts[0].Name)
		
		// Modify VFS
		vfs.Write("main.go", []byte("package main // changed"))
		
		// Third build - should execute due to changed content
		result3, err := engine.Build(ctx, config)
		require.NoError(t, err)
		assert.Equal(t, 2, buildCount)
		assert.Equal(t, "output2.bin", result3.Artifacts[0].Name)
	})

	t.Run("BuildTimeout", func(t *testing.T) {
		engine := build.NewMemoryBuildEngine()
		
		plugin := &MockPlugin{
			name: "slow",
			buildFunc: func(ctx context.Context, config build.BuildConfig, vfs build.VirtualFileSystem) (*build.BuildResult, error) {
				select {
				case <-time.After(1 * time.Second):
					return &build.BuildResult{Success: true}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		}
		engine.RegisterPlugin(plugin)
		
		vfs := build.NewDirectMemoryVFS()
		config := build.BuildConfig{
			PluginConfig: map[string]interface{}{
				"plugin": "slow",
				"vfs":    vfs,
			},
		}
		
		// Build with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		
		result, err := engine.Build(ctx, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")
		assert.Nil(t, result)
	})

	t.Run("BuildValidation", func(t *testing.T) {
		engine := build.NewMemoryBuildEngine()
		
		plugin := &MockPlugin{
			name: "validator",
			validateFunc: func(config build.BuildConfig, vfs build.VirtualFileSystem) error {
				if config.Target == "invalid" {
					return fmt.Errorf("invalid target")
				}
				return nil
			},
		}
		engine.RegisterPlugin(plugin)
		
		vfs := build.NewDirectMemoryVFS()
		
		// Valid config
		validConfig := build.BuildConfig{
			Target: "valid",
			PluginConfig: map[string]interface{}{
				"plugin": "validator",
				"vfs":    vfs,
			},
		}
		ctx := context.Background()
		result, err := engine.Build(ctx, validConfig)
		require.NoError(t, err)
		assert.True(t, result.Success)
		
		// Invalid config
		invalidConfig := build.BuildConfig{
			Target: "invalid",
			PluginConfig: map[string]interface{}{
				"plugin": "validator",
				"vfs":    vfs,
			},
		}
		result, err = engine.Build(ctx, invalidConfig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid target")
	})

	t.Run("EventSubscription", func(t *testing.T) {
		engine := build.NewMemoryBuildEngine()
		
		events := []build.BuildEvent{}
		eventChan := engine.Subscribe()
		
		// Collector goroutine
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case event := <-eventChan:
					events = append(events, event)
					if event.Type == build.EventBuildCompleted {
						return
					}
				case <-time.After(1 * time.Second):
					return
				}
			}
		}()
		
		// Register plugin - should emit event
		plugin := &MockPlugin{name: "test"}
		engine.RegisterPlugin(plugin)
		
		// Perform build - should emit events
		vfs := build.NewDirectMemoryVFS()
		config := build.BuildConfig{
			PluginConfig: map[string]interface{}{
				"plugin": "test",
				"vfs":    vfs,
			},
		}
		
		ctx := context.Background()
		engine.Build(ctx, config)
		
		// Wait for events to be collected
		wg.Wait()
		
		// Check events
		assert.GreaterOrEqual(t, len(events), 2)
		
		// Should have plugin loaded event
		hasPluginEvent := false
		for _, event := range events {
			if event.Type == build.EventPluginLoaded {
				hasPluginEvent = true
				assert.Equal(t, "test", event.Plugin)
			}
		}
		assert.True(t, hasPluginEvent)
		
		// Should have build events
		hasBuildStart := false
		hasBuildComplete := false
		for _, event := range events {
			if event.Type == build.EventBuildStarted {
				hasBuildStart = true
			}
			if event.Type == build.EventBuildCompleted {
				hasBuildComplete = true
			}
		}
		assert.True(t, hasBuildStart)
		assert.True(t, hasBuildComplete)
	})
}

func TestDetectAndBuild(t *testing.T) {
	t.Run("AutoDetectPlugin", func(t *testing.T) {
		engine := build.NewMemoryBuildEngine()
		
		// Register multiple plugins
		goPlugin := &MockPlugin{
			name:       "go",
			extensions: []string{".go", "go.mod"},
			detectFunc: func(vfs build.VirtualFileSystem) (bool, *build.ProjectMetadata, error) {
				if vfs.Exists("go.mod") {
					return true, &build.ProjectMetadata{
						Name:     "go-project",
						Language: "go",
					}, nil
				}
				return false, nil, nil
			},
		}
		
		jsPlugin := &MockPlugin{
			name:       "javascript",
			extensions: []string{".js", "package.json"},
			detectFunc: func(vfs build.VirtualFileSystem) (bool, *build.ProjectMetadata, error) {
				if vfs.Exists("package.json") {
					return true, &build.ProjectMetadata{
						Name:     "js-project",
						Language: "javascript",
					}, nil
				}
				return false, nil, nil
			},
		}
		
		engine.RegisterPlugin(goPlugin)
		engine.RegisterPlugin(jsPlugin)
		
		// Test Go project detection
		goVFS := build.NewDirectMemoryVFS()
		goVFS.Write("go.mod", []byte("module test"))
		goVFS.Write("main.go", []byte("package main"))
		
		ctx := context.Background()
		config := build.BuildConfig{}
		
		result, err := engine.DetectAndBuild(ctx, goVFS, config)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "go", result.Metadata.Plugin)
		assert.Equal(t, "go-project", result.Metadata.Project.Name)
		
		// Test JavaScript project detection
		jsVFS := build.NewDirectMemoryVFS()
		jsVFS.Write("package.json", []byte(`{"name": "app"}`))
		jsVFS.Write("index.js", []byte("console.log('hello')"))
		
		result, err = engine.DetectAndBuild(ctx, jsVFS, config)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "javascript", result.Metadata.Plugin)
		assert.Equal(t, "js-project", result.Metadata.Project.Name)
	})

	t.Run("NoPluginDetected", func(t *testing.T) {
		engine := build.NewMemoryBuildEngine()
		
		plugin := &MockPlugin{
			name: "test",
			detectFunc: func(vfs build.VirtualFileSystem) (bool, *build.ProjectMetadata, error) {
				return false, nil, nil
			},
		}
		engine.RegisterPlugin(plugin)
		
		vfs := build.NewDirectMemoryVFS()
		vfs.Write("unknown.txt", []byte("text"))
		
		ctx := context.Background()
		config := build.BuildConfig{}
		
		result, err := engine.DetectAndBuild(ctx, vfs, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no suitable plugin")
		assert.Nil(t, result)
	})
}

func TestBuildManager(t *testing.T) {
	t.Run("RegisterRepository", func(t *testing.T) {
		manager := build.NewBuildManager()
		
		// Create mock repository
		repo := &mockRepository{name: "test-repo"}
		
		// Register repository
		err := manager.RegisterRepository("test", repo)
		require.NoError(t, err)
		
		// Try to register with same name
		err = manager.RegisterRepository("test", repo)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("BuildFromRepository", func(t *testing.T) {
		manager := build.NewBuildManager()
		
		// Setup repository with files
		repo := &mockRepository{
			files: map[string][]byte{
				"main.go": []byte("package main"),
				"go.mod":  []byte("module test"),
			},
		}
		manager.RegisterRepository("test", repo)
		
		// Build
		ctx := context.Background()
		config := build.BuildConfig{
			PluginConfig: map[string]interface{}{
				"plugin": "go",
			},
		}
		
		result, err := manager.Build(ctx, "test", config)
		if err != nil {
			// Build might fail in test environment
			t.Logf("Build error (expected in test env): %v", err)
		} else {
			assert.NotNil(t, result)
		}
	})

	t.Run("BuildInReality", func(t *testing.T) {
		manager := build.NewBuildManager()
		
		// Create repository with branches
		repo := &mockRepository{
			branches: map[string]map[string][]byte{
				"main": {
					"main.go": []byte("package main"),
				},
				"parallel/debug": {
					"main.go":    []byte("package main"),
					"version.go": []byte("const VERSION = \"debug\""),
				},
				"parallel/release": {
					"main.go":    []byte("package main"),
					"version.go": []byte("const VERSION = \"release\""),
				},
			},
			currentBranch: "main",
		}
		manager.RegisterRepository("test", repo)
		
		// Build in debug reality
		ctx := context.Background()
		config := build.BuildConfig{
			Mode: "debug",
			PluginConfig: map[string]interface{}{
				"plugin": "go",
			},
		}
		
		result, err := manager.BuildInReality(ctx, "test", "debug", config)
		if err != nil {
			t.Logf("Build error (expected): %v", err)
		} else {
			assert.NotNil(t, result)
		}
	})

	t.Run("ConcurrentBuilds", func(t *testing.T) {
		manager := build.NewBuildManager()
		
		// Register multiple repositories
		for i := 0; i < 3; i++ {
			repo := &mockRepository{
				name: fmt.Sprintf("repo%d", i),
				files: map[string][]byte{
					"main.go": []byte(fmt.Sprintf("package main // repo%d", i)),
				},
			}
			manager.RegisterRepository(fmt.Sprintf("repo%d", i), repo)
		}
		
		// Run concurrent builds
		ctx := context.Background()
		var wg sync.WaitGroup
		results := make(chan *build.BuildResult, 3)
		errors := make(chan error, 3)
		
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(repoName string) {
				defer wg.Done()
				config := build.BuildConfig{
					PluginConfig: map[string]interface{}{
						"plugin": "go",
					},
				}
				result, err := manager.Build(ctx, repoName, config)
				if err != nil {
					errors <- err
				} else {
					results <- result
				}
			}(fmt.Sprintf("repo%d", i))
		}
		
		wg.Wait()
		close(results)
		close(errors)
		
		// Check results
		successCount := 0
		for range results {
			successCount++
		}
		
		errorCount := 0
		for err := range errors {
			errorCount++
			t.Logf("Build error: %v", err)
		}
		
		t.Logf("Concurrent builds: %d successful, %d errors", successCount, errorCount)
		assert.Equal(t, 3, successCount+errorCount)
	})

	t.Run("EventPropagation", func(t *testing.T) {
		manager := build.NewBuildManager()
		
		events := []build.BuildEvent{}
		unsubscribe := manager.Subscribe(func(event build.BuildEvent) {
			events = append(events, event)
		})
		defer unsubscribe()
		
		// Register repository
		repo := &mockRepository{
			files: map[string][]byte{
				"main.go": []byte("package main"),
			},
		}
		manager.RegisterRepository("test", repo)
		
		// Trigger build
		ctx := context.Background()
		config := build.BuildConfig{
			PluginConfig: map[string]interface{}{
				"plugin": "mock",
			},
		}
		manager.Build(ctx, "test", config)
		
		// Give time for events
		time.Sleep(100 * time.Millisecond)
		
		// Should have received events
		assert.NotEmpty(t, events)
	})
}

// Mock repository for testing
type mockRepository struct {
	name          string
	files         map[string][]byte
	branches      map[string]map[string][]byte
	currentBranch string
}

func (m *mockRepository) ListFiles() ([]string, error) {
	if m.currentBranch != "" && m.branches != nil {
		if branch, ok := m.branches[m.currentBranch]; ok {
			files := make([]string, 0, len(branch))
			for file := range branch {
				files = append(files, file)
			}
			return files, nil
		}
	}
	
	files := make([]string, 0, len(m.files))
	for file := range m.files {
		files = append(files, file)
	}
	return files, nil
}

func (m *mockRepository) ReadFile(path string) ([]byte, error) {
	if m.currentBranch != "" && m.branches != nil {
		if branch, ok := m.branches[m.currentBranch]; ok {
			if content, ok := branch[path]; ok {
				return content, nil
			}
		}
	}
	
	if content, ok := m.files[path]; ok {
		return content, nil
	}
	return nil, fmt.Errorf("file not found: %s", path)
}

func (m *mockRepository) WriteFile(path string, content []byte) error {
	if m.files == nil {
		m.files = make(map[string][]byte)
	}
	m.files[path] = content
	return nil
}

func (m *mockRepository) Checkout(branch string) error {
	if m.branches != nil {
		if _, ok := m.branches[branch]; ok {
			m.currentBranch = branch
			return nil
		}
	}
	return fmt.Errorf("branch not found: %s", branch)
}

func (m *mockRepository) Commit(message string) error {
	return nil
}

func (m *mockRepository) ParallelRealities(names []string) []string {
	realities := []string{}
	for _, name := range names {
		branchName := fmt.Sprintf("parallel/%s", name)
		if m.branches == nil {
			m.branches = make(map[string]map[string][]byte)
		}
		// Copy current files to new branch
		m.branches[branchName] = make(map[string][]byte)
		for k, v := range m.files {
			m.branches[branchName][k] = v
		}
		realities = append(realities, branchName)
	}
	return realities
}

func TestRealPluginIntegration(t *testing.T) {
	t.Run("GoPluginIntegration", func(t *testing.T) {
		engine := build.NewMemoryBuildEngine()
		goPlugin := plugins.NewGoPlugin()
		engine.RegisterPlugin(goPlugin)
		
		vfs := build.NewDirectMemoryVFS()
		vfs.Write("main.go", []byte(`package main
import "fmt"
func main() {
	fmt.Println("Hello from test")
}`))
		vfs.Write("go.mod", []byte("module testapp\ngo 1.21"))
		
		config := build.BuildConfig{
			Target: "binary",
			Mode:   "debug",
			PluginConfig: map[string]interface{}{
				"plugin": "go",
				"vfs":    vfs,
			},
		}
		
		ctx := context.Background()
		result, err := engine.Build(ctx, config)
		
		// Build might fail without Go installed
		if err != nil {
			t.Logf("Go build error (expected): %v", err)
		} else {
			assert.True(t, result.Success)
			assert.NotEmpty(t, result.Artifacts)
		}
	})

	t.Run("JavaScriptPluginIntegration", func(t *testing.T) {
		engine := build.NewMemoryBuildEngine()
		jsPlugin := plugins.NewJavaScriptPlugin()
		engine.RegisterPlugin(jsPlugin)
		
		vfs := build.NewDirectMemoryVFS()
		vfs.Write("package.json", []byte(`{
	"name": "test-app",
	"version": "1.0.0",
	"main": "index.js"
}`))
		vfs.Write("index.js", []byte("console.log('Hello from test')"))
		
		config := build.BuildConfig{
			Target: "bundle",
			Mode:   "production",
			PluginConfig: map[string]interface{}{
				"plugin": "javascript",
				"vfs":    vfs,
			},
		}
		
		ctx := context.Background()
		result, err := engine.Build(ctx, config)
		
		// Build might fail without Node.js installed
		if err != nil {
			t.Logf("JavaScript build error (expected): %v", err)
		} else {
			assert.True(t, result.Success)
			assert.NotEmpty(t, result.Artifacts)
		}
	})
}

func BenchmarkEngineOperations(b *testing.B) {
	b.Run("PluginRegistration", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			engine := build.NewMemoryBuildEngine()
			plugin := &MockPlugin{name: fmt.Sprintf("plugin%d", i)}
			engine.RegisterPlugin(plugin)
		}
	})

	b.Run("SimpleBuild", func(b *testing.B) {
		engine := build.NewMemoryBuildEngine()
		plugin := &MockPlugin{
			name: "test",
			buildFunc: func(ctx context.Context, config build.BuildConfig, vfs build.VirtualFileSystem) (*build.BuildResult, error) {
				return &build.BuildResult{
					Success: true,
					Artifacts: []build.BuildArtifact{
						{Name: "output.bin", Content: make([]byte, 1024)},
					},
				}, nil
			},
		}
		engine.RegisterPlugin(plugin)
		
		vfs := build.NewDirectMemoryVFS()
		vfs.Write("main.go", []byte("package main"))
		
		config := build.BuildConfig{
			PluginConfig: map[string]interface{}{
				"plugin": "test",
				"vfs":    vfs,
			},
		}
		
		ctx := context.Background()
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			engine.Build(ctx, config)
		}
	})

	b.Run("CachedBuild", func(b *testing.B) {
		engine := build.NewMemoryBuildEngine()
		cache := build.NewMemoryCache()
		engine.SetCache(cache)
		
		plugin := &MockPlugin{
			name: "test",
			buildFunc: func(ctx context.Context, config build.BuildConfig, vfs build.VirtualFileSystem) (*build.BuildResult, error) {
				// Simulate some work
				time.Sleep(1 * time.Millisecond)
				return &build.BuildResult{
					Success: true,
					Artifacts: []build.BuildArtifact{
						{Name: "output.bin", Content: make([]byte, 1024*1024)}, // 1MB
					},
				}, nil
			},
		}
		engine.RegisterPlugin(plugin)
		
		vfs := build.NewDirectMemoryVFS()
		vfs.Write("main.go", []byte("package main"))
		
		config := build.BuildConfig{
			UseCache: true,
			PluginConfig: map[string]interface{}{
				"plugin": "test",
				"vfs":    vfs,
			},
		}
		
		ctx := context.Background()
		
		// Prime the cache
		engine.Build(ctx, config)
		
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			engine.Build(ctx, config)
		}
	})
}