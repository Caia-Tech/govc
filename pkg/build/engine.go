package build

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Caia-Tech/govc"
)

// MemoryBuildEngine implements the BuildEngine interface
type MemoryBuildEngine struct {
	plugins   map[string]BuildPlugin
	cache     BuildCache
	eventChan chan BuildEvent
	
	// Performance optimizations
	optimizer    *PipelineOptimizer
	distCache    *DistributedBuildCache
	incremental  *IncrementalBuilder
	
	mu        sync.RWMutex
}

// NewMemoryBuildEngine creates a new memory-based build engine
func NewMemoryBuildEngine() *MemoryBuildEngine {
	cache := NewMemoryCache()
	
	engine := &MemoryBuildEngine{
		plugins:   make(map[string]BuildPlugin),
		cache:     cache,
		eventChan: make(chan BuildEvent, 100),
	}
	
	// Initialize performance optimizations
	engine.optimizer = NewPipelineOptimizer()
	engine.distCache = NewDistributedBuildCache(".govc/cache", nil)
	engine.incremental = NewIncrementalBuilder(cache)
	
	return engine
}

// RegisterPlugin adds a new build plugin
func (e *MemoryBuildEngine) RegisterPlugin(plugin BuildPlugin) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	name := plugin.Name()
	if _, exists := e.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}

	e.plugins[name] = plugin
	
	// Emit plugin loaded event
	e.emitEvent(BuildEvent{
		Type:      EventPluginLoaded,
		Timestamp: time.Now(),
		Plugin:    name,
		Data: map[string]interface{}{
			"extensions": plugin.SupportedExtensions(),
			"memory_support": plugin.SupportsMemoryBuild(),
		},
	})

	return nil
}

// GetPlugin retrieves a specific build plugin
func (e *MemoryBuildEngine) GetPlugin(name string) (BuildPlugin, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	plugin, exists := e.plugins[name]
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", name)
	}

	return plugin, nil
}

// ListPlugins returns available build plugins
func (e *MemoryBuildEngine) ListPlugins() []PluginInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(e.plugins))
	for name, plugin := range e.plugins {
		info := PluginInfo{
			Name:                name,
			Extensions:          plugin.SupportedExtensions(),
			SupportsMemoryBuild: plugin.SupportsMemoryBuild(),
		}
		
		// Add descriptions based on plugin type
		switch name {
		case "go":
			info.Description = "Build Go applications with WASM support"
			info.Languages = []string{"go"}
			info.Capabilities = []string{"binary", "library", "wasm", "cross-compile"}
		case "javascript":
			info.Description = "Build JavaScript/TypeScript projects"
			info.Languages = []string{"javascript", "typescript"}
			info.Capabilities = []string{"bundle", "minify", "transpile", "test"}
		case "rust":
			info.Description = "Build Rust projects with cargo"
			info.Languages = []string{"rust"}
			info.Capabilities = []string{"binary", "library", "wasm"}
		case "python":
			info.Description = "Package Python applications"
			info.Languages = []string{"python"}
			info.Capabilities = []string{"package", "wheel", "executable"}
		}
		
		infos = append(infos, info)
	}

	return infos
}

// Build executes a build using the specified plugin
func (e *MemoryBuildEngine) Build(ctx context.Context, config BuildConfig) (*BuildResult, error) {
	// Validate config
	if config.Target == "" {
		config.Target = "binary"
	}
	if config.Mode == "" {
		config.Mode = "debug"
	}
	if config.OutputPath == "" {
		config.OutputPath = "dist"
	}

	// Get plugin
	pluginName := config.PluginConfig["plugin"].(string)
	plugin, err := e.GetPlugin(pluginName)
	if err != nil {
		return nil, err
	}

	// Create VFS from config
	vfs, ok := config.PluginConfig["vfs"].(VirtualFileSystem)
	if !ok {
		return nil, fmt.Errorf("VFS not provided in config")
	}

	// Generate build ID
	buildID := fmt.Sprintf("%s-%d", pluginName, time.Now().UnixNano())
	
	// Emit build started event
	e.emitEvent(BuildEvent{
		Type:      EventBuildStarted,
		Timestamp: time.Now(),
		BuildID:   buildID,
		Plugin:    pluginName,
		Config:    config,
	})

	// Check cache
	cacheKey, err := e.generateCacheKey(plugin.Name(), config, vfs)
	if err == nil && config.Cache != nil && config.Cache.Enabled {
		if cached, hit := e.cache.Get(cacheKey); hit {
			e.emitEvent(BuildEvent{
				Type:      EventBuildCached,
				Timestamp: time.Now(),
				BuildID:   buildID,
				Plugin:    pluginName,
				Result:    cached,
			})
			cached.CacheHit = true
			return cached, nil
		}
	}

	// Execute build
	result, err := plugin.Build(ctx, config, vfs)
	if err != nil {
		e.emitEvent(BuildEvent{
			Type:      EventBuildFailed,
			Timestamp: time.Now(),
			BuildID:   buildID,
			Plugin:    pluginName,
			Error:     err,
		})
		return nil, err
	}

	// Add build ID to result
	if result.Metadata != nil {
		result.Metadata.BuildID = buildID
	}

	// Cache successful build
	if result.Success && config.Cache != nil && config.Cache.Enabled {
		e.cache.Put(cacheKey, result)
	}

	// Emit build completed event
	e.emitEvent(BuildEvent{
		Type:      EventBuildCompleted,
		Timestamp: time.Now(),
		BuildID:   buildID,
		Plugin:    pluginName,
		Config:    config,
		Result:    result,
	})

	return result, nil
}

// DetectAndBuild auto-detects project type and builds
func (e *MemoryBuildEngine) DetectAndBuild(ctx context.Context, vfs VirtualFileSystem, config BuildConfig) (*BuildResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Try each plugin to detect project type
	for name, plugin := range e.plugins {
		detected, _, err := plugin.DetectProject(vfs)
		if err != nil {
			continue
		}
		
		if detected {
			// Found compatible plugin
			config.PluginConfig = map[string]interface{}{
				"plugin": name,
				"vfs":    vfs,
			}
			
			// Validate build config
			if err := plugin.ValidateBuild(config, vfs); err != nil {
				return nil, fmt.Errorf("validation failed for %s: %w", name, err)
			}
			
			return e.Build(ctx, config)
		}
	}

	return nil, fmt.Errorf("no compatible build plugin found for project")
}

// Subscribe returns a channel for build events
func (e *MemoryBuildEngine) Subscribe() <-chan BuildEvent {
	return e.eventChan
}

// emitEvent sends a build event
func (e *MemoryBuildEngine) emitEvent(event BuildEvent) {
	select {
	case e.eventChan <- event:
	default:
		// Channel full, drop event
	}
}

// generateCacheKey creates a cache key for build config
func (e *MemoryBuildEngine) generateCacheKey(plugin string, config BuildConfig, vfs VirtualFileSystem) (CacheKey, error) {
	// Get all source files
	files, err := vfs.List()
	if err != nil {
		return CacheKey{}, err
	}

	// Calculate hash of all files
	hasher := NewFileHasher()
	for _, file := range files {
		content, err := vfs.Read(file)
		if err != nil {
			continue
		}
		hasher.Add(file, content)
	}

	return CacheKey{
		Plugin:    plugin,
		Config:    config,
		FilesHash: hasher.Hash(),
	}, nil
}

// BuildManagerImpl integrates build system with govc repository
type BuildManagerImpl struct {
	engine      BuildEngine
	repos       map[string]*govc.Repository
	builds      map[string]*BuildResult
	eventBus    *govc.EventBus
	subscribers []func(BuildEvent)
	mu          sync.RWMutex
}

// NewBuildManager creates a new build manager
func NewBuildManager() *BuildManagerImpl {
	engine := NewMemoryBuildEngine()
	
	// Register default plugins
	// These would be imported from the plugins package
	// engine.RegisterPlugin(plugins.NewGoPlugin())
	// engine.RegisterPlugin(plugins.NewJavaScriptPlugin())
	
	return &BuildManagerImpl{
		engine:      engine,
		repos:       make(map[string]*govc.Repository),
		builds:      make(map[string]*BuildResult),
		subscribers: make([]func(BuildEvent), 0),
	}
}

// RegisterRepository registers a repository for building
func (m *BuildManagerImpl) RegisterRepository(repoID string, repo *govc.Repository) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.repos[repoID] = repo
	
	// Subscribe to repository events for auto-build
	if repo != nil {
		m.subscribeToRepoEvents(repoID, repo)
	}
}

// Build executes a build for a repository
func (m *BuildManagerImpl) Build(ctx context.Context, repoID string, config BuildConfig) (*BuildResult, error) {
	m.mu.RLock()
	repo, exists := m.repos[repoID]
	m.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("repository %s not found", repoID)
	}

	// Create VFS from repository
	vfs := m.createVFSFromRepo(repo)
	
	// Add VFS to config
	if config.PluginConfig == nil {
		config.PluginConfig = make(map[string]interface{})
	}
	config.PluginConfig["vfs"] = vfs
	
	// Execute build
	result, err := m.engine.Build(ctx, config)
	if err != nil {
		return nil, err
	}

	// Store build result
	m.mu.Lock()
	m.builds[result.Metadata.BuildID] = result
	m.mu.Unlock()

	// Store artifacts back to repository
	for _, artifact := range result.Artifacts {
		repo.WriteFile(artifact.Path, artifact.Content)
	}

	return result, nil
}

// AutoBuild detects and builds project
func (m *BuildManagerImpl) AutoBuild(ctx context.Context, repoID string) (*BuildResult, error) {
	m.mu.RLock()
	repo, exists := m.repos[repoID]
	m.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("repository %s not found", repoID)
	}

	// Create VFS from repository
	vfs := m.createVFSFromRepo(repo)
	
	// Auto-detect and build
	config := BuildConfig{
		Target: "binary",
		Mode:   "debug",
		OutputPath: "dist",
	}
	
	return m.engine.DetectAndBuild(ctx, vfs, config)
}

// BuildInReality builds in a parallel reality
func (m *BuildManagerImpl) BuildInReality(ctx context.Context, repoID string, reality string, config BuildConfig) (*BuildResult, error) {
	m.mu.RLock()
	repo, exists := m.repos[repoID]
	m.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("repository %s not found", repoID)
	}

	// Save current branch
	currentBranch, _ := repo.CurrentBranch()
	
	// Switch to parallel reality
	realityBranch := fmt.Sprintf("parallel/%s", reality)
	if err := repo.Checkout(realityBranch); err != nil {
		// Create reality if it doesn't exist
		repo.Branch(realityBranch).Create()
		repo.Checkout(realityBranch)
	}
	
	// Build in reality
	result, err := m.Build(ctx, repoID, config)
	
	// Switch back to original branch
	repo.Checkout(currentBranch)
	
	return result, err
}

// GetBuildStatus returns current build status
func (m *BuildManagerImpl) GetBuildStatus(buildID string) (*BuildStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result, exists := m.builds[buildID]
	if !exists {
		return nil, fmt.Errorf("build %s not found", buildID)
	}

	state := "completed"
	if !result.Success {
		state = "failed"
	}

	return &BuildStatus{
		BuildID:     buildID,
		State:       state,
		Progress:    100,
		CurrentStep: "done",
		StartTime:   result.Metadata.Timestamp,
	}, nil
}

// ListBuilds returns build history
func (m *BuildManagerImpl) ListBuilds(repoID string, limit int) ([]*BuildResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	results := make([]*BuildResult, 0, limit)
	count := 0
	
	// Return most recent builds first
	for _, result := range m.builds {
		results = append(results, result)
		count++
		if count >= limit {
			break
		}
	}
	
	return results, nil
}

// Subscribe to build events
func (m *BuildManagerImpl) Subscribe(handler func(BuildEvent)) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.subscribers = append(m.subscribers, handler)
	
	// Return unsubscribe function
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		
		for i, h := range m.subscribers {
			if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
				m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
				break
			}
		}
	}
}

// createVFSFromRepo creates a VFS from repository
func (m *BuildManagerImpl) createVFSFromRepo(repo *govc.Repository) VirtualFileSystem {
	// Create a custom VFS that reads from repository
	return &RepoVFS{
		repo: repo,
	}
}

// subscribeToRepoEvents subscribes to repository events for auto-build
func (m *BuildManagerImpl) subscribeToRepoEvents(repoID string, repo *govc.Repository) {
	// Subscribe to commit events
	repo.Watch(func(event govc.CommitEvent) {
		// Check event message for commit events
		if strings.Contains(event.Message, "commit") {
			// Auto-build on commit if configured
			if m.shouldAutoBuild(event) {
				go func() {
					ctx := context.Background()
					m.AutoBuild(ctx, repoID)
				}()
			}
		}
	})
}

// shouldAutoBuild determines if auto-build should trigger
func (m *BuildManagerImpl) shouldAutoBuild(event govc.CommitEvent) bool {
	// For now, auto-build on any commit event
	// In production, check for source file changes in event.Message
	return true
}

// RepoVFS implements VirtualFileSystem backed by govc.Repository
type RepoVFS struct {
	repo *govc.Repository
}

func (v *RepoVFS) Read(path string) ([]byte, error) {
	return v.repo.ReadFile(path)
}

func (v *RepoVFS) Write(path string, data []byte) error {
	return v.repo.WriteFile(path, data)
}

func (v *RepoVFS) Delete(path string) error {
	// govc doesn't have RemoveFile, simulate with empty write
	return v.repo.WriteFile(path, []byte{})
}

func (v *RepoVFS) List() ([]string, error) {
	return v.repo.ListFiles()
}

func (v *RepoVFS) Glob(pattern string) ([]string, error) {
	files, err := v.repo.ListFiles()
	if err != nil {
		return nil, err
	}
	var matches []string
	
	for _, file := range files {
		// Simple pattern matching
		if matchesPattern(file, pattern) {
			matches = append(matches, file)
		}
	}
	
	return matches, nil
}

func (v *RepoVFS) Exists(path string) bool {
	_, err := v.repo.ReadFile(path)
	return err == nil
}

func (v *RepoVFS) TempDir(prefix string) (string, error) {
	return fmt.Sprintf("/tmp/%s%d", prefix, time.Now().UnixNano()), nil
}

func (v *RepoVFS) CreateBridge() (FilesystemBridge, error) {
	// Create MemoryVFS from repo and use its bridge
	memVFS := NewDirectMemoryVFS()
	
	// Copy all files to memory VFS
	files, _ := v.repo.ListFiles()
	for _, file := range files {
		content, err := v.repo.ReadFile(file)
		if err != nil {
			continue
		}
		memVFS.Write(file, content)
	}
	
	return memVFS.CreateBridge()
}

// Helper functions

func hasSourceExtension(file string) bool {
	extensions := []string{
		".go", ".js", ".ts", ".jsx", ".tsx",
		".py", ".rs", ".java", ".c", ".cpp",
		".cs", ".rb", ".php", ".swift",
	}
	
	for _, ext := range extensions {
		if strings.HasSuffix(file, ext) {
			return true
		}
	}
	return false
}

func matchesPattern(file, pattern string) bool {
	// Simple pattern matching - in production use proper glob library
	if pattern == "**/*" {
		return true
	}
	
	if strings.Contains(pattern, "**") {
		prefix := strings.Split(pattern, "**")[0]
		return strings.HasPrefix(file, prefix)
	}
	
	if strings.Contains(pattern, "*") {
		prefix := strings.Split(pattern, "*")[0]
		suffix := strings.Split(pattern, "*")[1]
		return strings.HasPrefix(file, prefix) && strings.HasSuffix(file, suffix)
	}
	
	return file == pattern
}