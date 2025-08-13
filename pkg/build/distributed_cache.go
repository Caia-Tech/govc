package build

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Caia-Tech/govc/pkg/optimize"
)

// DistributedBuildCache implements a distributed build cache system
type DistributedBuildCache struct {
	local    BuildCache
	remote   RemoteCacheBackend
	storage  *CacheStorageManager
	hasher   *ContentHasher
	
	// Performance optimizations
	memPool       *optimize.MemoryPool
	compressor    *optimize.AdaptiveCompressor
	batcher       *optimize.BatchProcessor
	
	stats DistributedCacheStats
	mu    sync.RWMutex
}

// DistributedCacheStats tracks distributed cache performance
type DistributedCacheStats struct {
	LocalHits         int64     `json:"local_hits"`
	LocalMisses       int64     `json:"local_misses"`
	RemoteHits        int64     `json:"remote_hits"`
	RemoteMisses      int64     `json:"remote_misses"`
	BytesUploaded     int64     `json:"bytes_uploaded"`
	BytesDownloaded   int64     `json:"bytes_downloaded"`
	CompressionRatio  float64   `json:"compression_ratio"`
	LastSync          time.Time `json:"last_sync"`
}

// NewDistributedBuildCache creates a distributed build cache
func NewDistributedBuildCache(cacheDir string, remote RemoteCacheBackend) *DistributedBuildCache {
	storage := NewCacheStorageManager(cacheDir)
	
	cache := &DistributedBuildCache{
		local:      NewPersistentCache(cacheDir),
		remote:     remote,
		storage:    storage,
		hasher:     NewContentHasher(),
		memPool:    optimize.NewMemoryPool(),
		compressor: optimize.NewAdaptiveCompressor(),
		batcher:    optimize.NewBatchProcessor(50, 4),
	}
	
	// Start background sync
	go cache.syncLoop()
	
	return cache
}

// RemoteCacheBackend defines interface for remote cache storage
type RemoteCacheBackend interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, data []byte) error
	Exists(ctx context.Context, key string) (bool, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Get retrieves from distributed cache (local first, then remote)
func (dc *DistributedBuildCache) Get(key CacheKey) (*BuildResult, bool) {
	keyStr := key.String()
	
	// Try local cache first
	if result, found := dc.local.Get(key); found {
		dc.stats.LocalHits++
		return result, true
	}
	dc.stats.LocalMisses++
	
	// Try remote cache
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	if data, err := dc.remote.Get(ctx, keyStr); err == nil {
		// Decompress and deserialize
		if result, err := dc.deserializeCacheEntry(data); err == nil {
			// Store in local cache for future use
			dc.local.Put(key, result)
			dc.stats.RemoteHits++
			dc.stats.BytesDownloaded += int64(len(data))
			return result, true
		}
	}
	
	dc.stats.RemoteMisses++
	return nil, false
}

// Put stores in both local and remote cache
func (dc *DistributedBuildCache) Put(key CacheKey, result *BuildResult) error {
	// Store in local cache immediately
	if err := dc.local.Put(key, result); err != nil {
		return fmt.Errorf("failed to store in local cache: %w", err)
	}
	
	// Asynchronously store in remote cache
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		
		data, err := dc.serializeCacheEntry(result)
		if err != nil {
			return
		}
		
		keyStr := key.String()
		if err := dc.remote.Put(ctx, keyStr, data); err == nil {
			dc.stats.BytesUploaded += int64(len(data))
		}
	}()
	
	return nil
}

// IncrementalBuilder performs incremental builds based on file changes
type IncrementalBuilder struct {
	cache       BuildCache
	fileWatcher *FileWatcher
	depGraph    *DependencyGraph
	hasher      *ContentHasher
	
	lastBuild   map[string]time.Time
	buildQueue  chan BuildRequest
	results     chan BuildResponse
	
	mu sync.RWMutex
}

// BuildRequest represents a build request
type BuildRequest struct {
	ID         string
	Files      []string
	Config     BuildConfig
	Context    context.Context
	ResultChan chan BuildResponse
}

// BuildResponse represents a build response
type BuildResponse struct {
	Result *BuildResult
	Error  error
	FromCache bool
}

// NewIncrementalBuilder creates an incremental builder
func NewIncrementalBuilder(cache BuildCache) *IncrementalBuilder {
	builder := &IncrementalBuilder{
		cache:      cache,
		fileWatcher: NewFileWatcher(),
		depGraph:   NewDependencyGraph(),
		hasher:     NewContentHasher(),
		lastBuild:  make(map[string]time.Time),
		buildQueue: make(chan BuildRequest, 100),
		results:    make(chan BuildResponse, 100),
	}
	
	// Start build worker
	go builder.buildWorker()
	
	return builder
}

// Build performs an incremental build
func (ib *IncrementalBuilder) Build(ctx context.Context, files []string, config BuildConfig) (*BuildResult, error) {
	// Check if incremental build is possible
	if ib.canSkipBuild(files, config) {
		// Try to get from cache
		key := ib.buildCacheKey(files, config)
		if result, found := ib.cache.Get(key); found {
			return result, nil
		}
	}
	
	// Submit build request
	resultChan := make(chan BuildResponse, 1)
	request := BuildRequest{
		Files:      files,
		Config:     config,
		Context:    ctx,
		ResultChan: resultChan,
	}
	
	select {
	case ib.buildQueue <- request:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	
	// Wait for result
	select {
	case response := <-resultChan:
		return response.Result, response.Error
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (ib *IncrementalBuilder) canSkipBuild(files []string, config BuildConfig) bool {
	ib.mu.RLock()
	defer ib.mu.RUnlock()
	
	for _, file := range files {
		if stat, err := os.Stat(file); err == nil {
			if lastBuild, exists := ib.lastBuild[file]; !exists || stat.ModTime().After(lastBuild) {
				return false
			}
		}
	}
	
	return true
}

func (ib *IncrementalBuilder) buildWorker() {
	for request := range ib.buildQueue {
		// Perform actual build
		result := ib.performBuild(request)
		
		// Send result
		select {
		case request.ResultChan <- result:
		case <-request.Context.Done():
		}
	}
}

func (ib *IncrementalBuilder) performBuild(request BuildRequest) BuildResponse {
	// Build cache key
	key := ib.buildCacheKey(request.Files, request.Config)
	
	// Check cache first
	if result, found := ib.cache.Get(key); found {
		return BuildResponse{
			Result: result,
			FromCache: true,
		}
	}
	
	// Perform actual build (simplified)
	result := &BuildResult{
		Success: true,
		Output: &BuildOutput{
			Stdout: "build output",
			Info:   []string{"Build completed successfully"},
		},
		Artifacts: []BuildArtifact{
			{
				Name: "output",
				Type: "binary",
				Path: "bin/output",
				Size: 1024,
			},
		},
		Metadata: &BuildMetadata{
			Timestamp: time.Now(),
			Plugin:    "go",
			BuildID:   fmt.Sprintf("build-%d", time.Now().Unix()),
		},
		Duration: time.Second,
	}
	
	// Cache result
	ib.cache.Put(key, result)
	
	// Update last build times
	ib.mu.Lock()
	now := time.Now()
	for _, file := range request.Files {
		ib.lastBuild[file] = now
	}
	ib.mu.Unlock()
	
	return BuildResponse{
		Result: result,
		FromCache: false,
	}
}

func (ib *IncrementalBuilder) buildCacheKey(files []string, config BuildConfig) CacheKey {
	// Sort files for consistent hashing
	sortedFiles := make([]string, len(files))
	copy(sortedFiles, files)
	sort.Strings(sortedFiles)
	
	// Hash file contents
	filesHash := ib.hasher.HashFiles(sortedFiles)
	
	return CacheKey{
		Plugin:    "go", // Default to go plugin, should be passed in config
		Config:    config,
		FilesHash: filesHash,
	}
}

// ContentHasher computes content-based hashes for caching
type ContentHasher struct {
	mu    sync.RWMutex
	cache map[string]string // file path -> hash cache
}

// NewContentHasher creates a content hasher
func NewContentHasher() *ContentHasher {
	return &ContentHasher{
		cache: make(map[string]string),
	}
}

// HashFiles computes hash of multiple files
func (ch *ContentHasher) HashFiles(files []string) string {
	hasher := sha256.New()
	
	for _, file := range files {
		hash := ch.HashFile(file)
		hasher.Write([]byte(hash))
	}
	
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

// HashFile computes hash of a single file with caching
func (ch *ContentHasher) HashFile(filepath string) string {
	// Check if we have cached hash
	ch.mu.RLock()
	if hash, exists := ch.cache[filepath]; exists {
		// Check if file has been modified
		if stat, err := os.Stat(filepath); err == nil {
			// Use mtime + size as quick check
			key := fmt.Sprintf("%s_%d_%d", filepath, stat.ModTime().Unix(), stat.Size())
			if strings.HasPrefix(hash, key) {
				ch.mu.RUnlock()
				return strings.TrimPrefix(hash, key+"_")
			}
		}
	}
	ch.mu.RUnlock()
	
	// Compute hash
	file, err := os.Open(filepath)
	if err != nil {
		return ""
	}
	defer file.Close()
	
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return ""
	}
	
	hash := fmt.Sprintf("%x", hasher.Sum(nil))
	
	// Cache result with metadata
	if stat, err := os.Stat(filepath); err == nil {
		key := fmt.Sprintf("%s_%d_%d", filepath, stat.ModTime().Unix(), stat.Size())
		cachedHash := fmt.Sprintf("%s_%s", key, hash)
		
		ch.mu.Lock()
		ch.cache[filepath] = cachedHash
		ch.mu.Unlock()
	}
	
	return hash
}

// DependencyGraph tracks build dependencies for incremental builds
type DependencyGraph struct {
	graph map[string][]string // file -> dependencies
	mu    sync.RWMutex
}

// NewDependencyGraph creates a dependency graph
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		graph: make(map[string][]string),
	}
}

// AddNode adds a node to the graph
func (dg *DependencyGraph) AddNode(node string) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	
	if _, exists := dg.graph[node]; !exists {
		dg.graph[node] = []string{}
	}
}

// AddEdge adds an edge from source to target
func (dg *DependencyGraph) AddEdge(source, target string) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	
	// Ensure both nodes exist
	if _, exists := dg.graph[source]; !exists {
		dg.graph[source] = []string{}
	}
	if _, exists := dg.graph[target]; !exists {
		dg.graph[target] = []string{}
	}
	
	// Add edge
	dg.graph[source] = append(dg.graph[source], target)
}

// AddDependency adds a dependency relationship
func (dg *DependencyGraph) AddDependency(file, dependency string) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	
	if deps, exists := dg.graph[file]; exists {
		// Check if dependency already exists
		for _, dep := range deps {
			if dep == dependency {
				return
			}
		}
		dg.graph[file] = append(deps, dependency)
	} else {
		dg.graph[file] = []string{dependency}
	}
}

// GetDependencies returns all dependencies for a file
func (dg *DependencyGraph) GetDependencies(file string) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	
	if deps, exists := dg.graph[file]; exists {
		// Return copy
		result := make([]string, len(deps))
		copy(result, deps)
		return result
	}
	
	return nil
}

// GetAffectedFiles returns files that depend on the given file
func (dg *DependencyGraph) GetAffectedFiles(file string) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	
	var affected []string
	for f, deps := range dg.graph {
		for _, dep := range deps {
			if dep == file {
				affected = append(affected, f)
				break
			}
		}
	}
	
	return affected
}

// FileWatcher watches for file system changes
type FileWatcher struct {
	watchers map[string]chan FileEvent
	mu       sync.RWMutex
}

// FileEvent represents a file system event
type FileEvent struct {
	Path      string
	Operation string // "create", "modify", "delete"
	Time      time.Time
}

// NewFileWatcher creates a file watcher
func NewFileWatcher() *FileWatcher {
	return &FileWatcher{
		watchers: make(map[string]chan FileEvent),
	}
}

// Watch starts watching a directory for changes
func (fw *FileWatcher) Watch(path string) (<-chan FileEvent, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	
	if ch, exists := fw.watchers[path]; exists {
		return ch, nil
	}
	
	ch := make(chan FileEvent, 100)
	fw.watchers[path] = ch
	
	// Start watching (simplified - would use fsnotify in real implementation)
	go fw.watchPath(path, ch)
	
	return ch, nil
}

func (fw *FileWatcher) watchPath(path string, ch chan FileEvent) {
	// Simplified polling-based watcher
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	lastSeen := make(map[string]time.Time)
	
	for range ticker.C {
		filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			
			if lastTime, exists := lastSeen[filePath]; !exists {
				// New file
				ch <- FileEvent{
					Path:      filePath,
					Operation: "create",
					Time:      time.Now(),
				}
				lastSeen[filePath] = info.ModTime()
			} else if info.ModTime().After(lastTime) {
				// Modified file
				ch <- FileEvent{
					Path:      filePath,
					Operation: "modify",
					Time:      time.Now(),
				}
				lastSeen[filePath] = info.ModTime()
			}
			
			return nil
		})
	}
}

// Utility functions for cache serialization

func (dc *DistributedBuildCache) serializeCacheEntry(result *BuildResult) ([]byte, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	
	// Compress if beneficial
	compressed, wasCompressed := dc.compressor.Compress(data)
	if wasCompressed {
		dc.stats.CompressionRatio = float64(len(data)) / float64(len(compressed))
	}
	
	return compressed, nil
}

func (dc *DistributedBuildCache) deserializeCacheEntry(data []byte) (*BuildResult, error) {
	// Try to decompress
	if decompressed, err := dc.compressor.Decompress(data); err == nil {
		data = decompressed
	}
	
	var result BuildResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	
	return &result, nil
}

func (dc *DistributedBuildCache) syncLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		dc.stats.LastSync = time.Now()
		// Implement periodic sync logic here
	}
}

// CacheStorageManager manages cache storage and cleanup
type CacheStorageManager struct {
	cacheDir   string
	maxSize    int64
	maxAge     time.Duration
	
	mu sync.Mutex
}

// NewCacheStorageManager creates a cache storage manager
func NewCacheStorageManager(cacheDir string) *CacheStorageManager {
	return &CacheStorageManager{
		cacheDir: cacheDir,
		maxSize:  10 << 30, // 10GB
		maxAge:   7 * 24 * time.Hour, // 1 week
	}
}

// Cleanup performs cache cleanup based on size and age
func (csm *CacheStorageManager) Cleanup() error {
	csm.mu.Lock()
	defer csm.mu.Unlock()
	
	// Get all cache files
	var files []os.FileInfo
	filepath.Walk(csm.cacheDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files = append(files, info)
		}
		return nil
	})
	
	// Sort by access time (oldest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().Before(files[j].ModTime())
	})
	
	// Calculate total size
	var totalSize int64
	for _, file := range files {
		totalSize += file.Size()
	}
	
	// Remove old files if over size limit
	if totalSize > csm.maxSize {
		for _, file := range files {
			if totalSize <= csm.maxSize {
				break
			}
			
			filePath := filepath.Join(csm.cacheDir, file.Name())
			if err := os.Remove(filePath); err == nil {
				totalSize -= file.Size()
			}
		}
	}
	
	// Remove files older than maxAge
	cutoff := time.Now().Add(-csm.maxAge)
	for _, file := range files {
		if file.ModTime().Before(cutoff) {
			filePath := filepath.Join(csm.cacheDir, file.Name())
			os.Remove(filePath)
		}
	}
	
	return nil
}

// PersistentCache implements BuildCache with disk persistence
type PersistentCache struct {
	dir     string
	memory  *MemoryCache
	hasher  *ContentHasher
	
	mu sync.RWMutex
}

// NewPersistentCache creates a persistent cache
func NewPersistentCache(dir string) *PersistentCache {
	os.MkdirAll(dir, 0755)
	
	return &PersistentCache{
		dir:     dir,
		memory:  NewMemoryCache(),
		hasher:  NewContentHasher(),
	}
}

// Get retrieves from persistent cache
func (pc *PersistentCache) Get(key CacheKey) (*BuildResult, bool) {
	// Try memory cache first
	if result, found := pc.memory.Get(key); found {
		return result, true
	}
	
	// Try disk cache
	keyStr := key.String()
	filePath := filepath.Join(pc.dir, keyStr+".cache")
	
	if data, err := os.ReadFile(filePath); err == nil {
		var result BuildResult
		if err := json.Unmarshal(data, &result); err == nil {
			// Store in memory for next time
			pc.memory.Put(key, &result)
			return &result, true
		}
	}
	
	return nil, false
}

// Put stores in persistent cache
func (pc *PersistentCache) Put(key CacheKey, result *BuildResult) error {
	// Store in memory
	if err := pc.memory.Put(key, result); err != nil {
		return err
	}
	
	// Store on disk
	keyStr := key.String()
	filePath := filepath.Join(pc.dir, keyStr+".cache")
	
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	
	return os.WriteFile(filePath, data, 0644)
}

// Stats returns cache statistics
func (pc *PersistentCache) Stats() CacheStats {
	return pc.memory.Stats()
}

// Invalidate removes from cache
func (pc *PersistentCache) Invalidate(key CacheKey) error {
	// Remove from memory
	pc.memory.Invalidate(key)
	
	// Remove from disk
	keyStr := key.String()
	filePath := filepath.Join(pc.dir, keyStr+".cache")
	os.Remove(filePath)
	
	return nil
}

// Clear removes all cached results
func (pc *PersistentCache) Clear() error {
	pc.memory.Clear()
	
	// Remove all files
	return filepath.Walk(pc.dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".cache") {
			return os.Remove(path)
		}
		return nil
	})
}