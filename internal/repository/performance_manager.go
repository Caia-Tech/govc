package repository

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/Caia-Tech/govc/pkg/object"
	"github.com/Caia-Tech/govc/pkg/optimize"
)

// PerformanceManager manages system-wide performance optimizations
type PerformanceManager struct {
	memoryPool       *optimize.MemoryPool
	streamProcessor  *optimize.StreamProcessor
	concurrentLimiter *optimize.ConcurrentLimiter
	batchProcessor   *optimize.BatchProcessor
	
	mu      sync.RWMutex
	metrics PerformanceMetrics
	started time.Time
}

// PerformanceMetrics tracks performance metrics
type PerformanceMetrics struct {
	MemoryAllocated   uint64
	MemoryInUse       uint64
	GoroutinesActive  int
	PoolEfficiency    float64
	LastGC            time.Time
	GCPauseTotal      time.Duration
	NumGC             uint32
}

// GlobalPerformanceManager is the system-wide performance manager
var GlobalPerformanceManager *PerformanceManager

// InitPerformanceManager initializes the global performance manager
func InitPerformanceManager() {
	GlobalPerformanceManager = NewPerformanceManager()
}

// NewPerformanceManager creates a new performance manager
func NewPerformanceManager() *PerformanceManager {
	pm := &PerformanceManager{
		memoryPool:        optimize.NewMemoryPool(),
		concurrentLimiter: optimize.NewConcurrentLimiter(runtime.NumCPU() * 2),
		batchProcessor:    optimize.NewBatchProcessor(100, runtime.NumCPU()),
		started:           time.Now(),
	}
	
	// Create stream processor with memory pool
	pm.streamProcessor = optimize.NewStreamProcessor(pm.memoryPool, 64*1024, runtime.NumCPU())
	
	// Start metrics collector
	go pm.collectMetrics()
	
	return pm
}

// GetMemoryPool returns the memory pool
func (pm *PerformanceManager) GetMemoryPool() *optimize.MemoryPool {
	return pm.memoryPool
}

// GetStreamProcessor returns the stream processor
func (pm *PerformanceManager) GetStreamProcessor() *optimize.StreamProcessor {
	return pm.streamProcessor
}

// GetConcurrentLimiter returns the concurrent limiter
func (pm *PerformanceManager) GetConcurrentLimiter() *optimize.ConcurrentLimiter {
	return pm.concurrentLimiter
}

// GetBatchProcessor returns the batch processor
func (pm *PerformanceManager) GetBatchProcessor() *optimize.BatchProcessor {
	return pm.batchProcessor
}

// collectMetrics periodically collects performance metrics
func (pm *PerformanceManager) collectMetrics() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		
		pm.mu.Lock()
		pm.metrics.MemoryAllocated = m.Alloc
		pm.metrics.MemoryInUse = m.HeapInuse
		pm.metrics.GoroutinesActive = runtime.NumGoroutine()
		pm.metrics.LastGC = time.Unix(0, int64(m.LastGC))
		pm.metrics.GCPauseTotal = time.Duration(m.PauseTotalNs)
		pm.metrics.NumGC = m.NumGC
		
		// Calculate pool efficiency
		poolStats := pm.memoryPool.Stats()
		pm.metrics.PoolEfficiency = poolStats.EfficiencyRatio()
		
		pm.mu.Unlock()
	}
}

// GetMetrics returns current performance metrics
func (pm *PerformanceManager) GetMetrics() PerformanceMetrics {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.metrics
}

// OptimizeRepository adds performance optimizations to a repository
func (pm *PerformanceManager) OptimizeRepository(repo *Repository) {
	// Use memory pool for blob storage
	repo.store.SetMemoryPool(pm.memoryPool)
	
	// Use concurrent limiter for parallel operations
	repo.concurrentLimiter = pm.concurrentLimiter
	
	// Use batch processor for bulk operations
	repo.batchProcessor = pm.batchProcessor
}

// OptimizedAdd adds a file using optimized memory management
func (repo *Repository) OptimizedAdd(path string, content []byte) error {
	if GlobalPerformanceManager == nil {
		// Fallback to regular add
		return repo.Add(path)
	}
	
	pool := GlobalPerformanceManager.GetMemoryPool()
	
	// Use pooled buffer for content processing
	buf := pool.Get(len(content))
	defer pool.Put(buf)
	
	copy(buf, content)
	
	// Store blob
	hash, err := repo.store.StoreBlob(buf)
	if err != nil {
		return err
	}
	
	// Add to staging
	repo.staging.AddHash(path, hash)
	
	// Update worktree
	repo.worktree.files[path] = buf
	
	return nil
}

// OptimizedCommit creates a commit with optimized processing
func (repo *Repository) OptimizedCommit(message string) (*object.Commit, error) {
	if GlobalPerformanceManager == nil {
		// Fallback to regular commit
		return repo.Commit(message)
	}
	
	ctx := context.Background()
	limiter := GlobalPerformanceManager.GetConcurrentLimiter()
	
	var commit *object.Commit
	err := limiter.Do(ctx, func() error {
		var err error
		commit, err = repo.Commit(message)
		return err
	})
	
	return commit, err
}

// BatchAdd adds multiple files in an optimized batch
func (repo *Repository) BatchAdd(files map[string][]byte) error {
	if GlobalPerformanceManager == nil {
		// Fallback to individual adds
		for path := range files {
			if err := repo.Add(path); err != nil {
				return err
			}
		}
		return nil
	}
	
	ctx := context.Background()
	processor := GlobalPerformanceManager.GetBatchProcessor()
	
	// Convert to items slice
	items := make([]interface{}, 0, len(files))
	for path, content := range files {
		items = append(items, struct {
			Path    string
			Content []byte
		}{path, content})
	}
	
	// Process in batches
	return processor.Process(ctx, items, func(batch []interface{}) error {
		for _, item := range batch {
			file := item.(struct {
				Path    string
				Content []byte
			})
			
			if err := repo.OptimizedAdd(file.Path, file.Content); err != nil {
				return err
			}
		}
		return nil
	})
}

// PerformanceReport generates a performance report
func (pm *PerformanceManager) PerformanceReport() string {
	metrics := pm.GetMetrics()
	poolStats := pm.memoryPool.Stats()
	concurrencyStats := pm.concurrentLimiter.Stats()
	
	report := "Performance Report\n"
	report += "==================\n\n"
	
	// Memory metrics
	report += "Memory Usage:\n"
	report += fmt.Sprintf("  Allocated: %.2f MB\n", float64(metrics.MemoryAllocated)/(1024*1024))
	report += fmt.Sprintf("  In Use: %.2f MB\n", float64(metrics.MemoryInUse)/(1024*1024))
	report += fmt.Sprintf("  Pool Efficiency: %.2f\n", metrics.PoolEfficiency)
	report += fmt.Sprintf("  Last GC: %s ago\n", time.Since(metrics.LastGC))
	report += fmt.Sprintf("  GC Runs: %d\n", metrics.NumGC)
	report += fmt.Sprintf("  GC Pause Total: %v\n\n", metrics.GCPauseTotal)
	
	// Pool statistics
	report += "Memory Pool:\n"
	report += fmt.Sprintf("  Total Gets: %d\n", poolStats.TotalGets)
	report += fmt.Sprintf("  Total Puts: %d\n", poolStats.TotalPuts)
	report += fmt.Sprintf("  Total News: %d\n", poolStats.TotalNews)
	report += fmt.Sprintf("  Currently In Use: %d\n\n", poolStats.CurrentInUse)
	
	// Concurrency metrics
	report += "Concurrency:\n"
	report += fmt.Sprintf("  Active Goroutines: %d\n", metrics.GoroutinesActive)
	report += fmt.Sprintf("  Total Started: %d\n", concurrencyStats.TotalStarted)
	report += fmt.Sprintf("  Total Completed: %d\n", concurrencyStats.TotalCompleted)
	report += fmt.Sprintf("  Total Blocked: %d\n", concurrencyStats.TotalBlocked)
	report += fmt.Sprintf("  Total Errors: %d\n\n", concurrencyStats.TotalErrors)
	
	// Uptime
	report += fmt.Sprintf("Uptime: %v\n", time.Since(pm.started))
	
	return report
}

// EnableProfiling enables CPU and memory profiling
func EnableProfiling(cpuProfile, memProfile string) {
	if cpuProfile != "" {
		f, err := os.Create(cpuProfile)
		if err == nil {
			pprof.StartCPUProfile(f)
			defer f.Close()
			defer pprof.StopCPUProfile()
		}
	}
	
	if memProfile != "" {
		f, err := os.Create(memProfile)
		if err == nil {
			defer f.Close()
			defer pprof.WriteHeapProfile(f)
		}
	}
}

// TuneGC tunes garbage collection for optimal performance
func TuneGC() {
	// Set GOGC to reduce GC frequency
	debug.SetGCPercent(200) // Default is 100
	
	// Set memory limit to prevent OOM
	debug.SetMemoryLimit(2 << 30) // 2GB limit
	
	// Force GC to establish baseline
	runtime.GC()
}