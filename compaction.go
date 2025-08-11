package govc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	
	"github.com/Caia-Tech/govc/pkg/storage"
)

// MemoryStats represents memory usage statistics
type MemoryStats struct {
	TotalObjects    int64     `json:"total_objects"`
	TotalBytes      int64     `json:"total_bytes"`
	CompactedBytes  int64     `json:"compacted_bytes"`
	LastCompaction  time.Time `json:"last_compaction"`
	FragmentRatio   float64   `json:"fragment_ratio"`
}

// CompactionPolicy defines the compaction behavior
type CompactionPolicy struct {
	Enabled            bool          `json:"enabled"`
	Interval           time.Duration `json:"interval"`
	MemoryThreshold    float64       `json:"memory_threshold"`    // 0.8 = 80%
	MinObjectAge       time.Duration `json:"min_object_age"`
	MaxObjectsPerCycle int           `json:"max_objects_per_cycle"`
}

// DefaultCompactionPolicy returns sensible defaults
func DefaultCompactionPolicy() CompactionPolicy {
	return CompactionPolicy{
		Enabled:            true,
		Interval:           5 * time.Minute,
		MemoryThreshold:    0.8,
		MinObjectAge:       1 * time.Hour,
		MaxObjectsPerCycle: 1000,
	}
}

// CompactOptions provides options for manual compaction
type CompactOptions struct {
	Force          bool  `json:"force"`
	TargetMemoryMB int64 `json:"target_memory_mb"`
}

// CompactionStats provides detailed compaction statistics
type CompactionStats struct {
	TotalRuns        int64         `json:"total_runs"`
	ObjectsCollected int64         `json:"objects_collected"`
	BytesFreed       int64         `json:"bytes_freed"`
	AverageRunTime   time.Duration `json:"average_run_time"`
	LastRunTime      time.Duration `json:"last_run_time"`
	ErrorCount       int64         `json:"error_count"`
	NextRun          time.Time     `json:"next_run"`
}

// Compactable defines the interface for objects that support compaction
type Compactable interface {
	Compact(ctx context.Context) error
	GetMemoryUsage() MemoryStats
	SetCompactionPolicy(policy CompactionPolicy)
}

// GarbageCollector manages memory compaction for a repository
type GarbageCollector struct {
	store           *storage.Store
	refCounter      map[string]int64 // Reference count per object
	lastCompaction  time.Time
	compactionMutex sync.RWMutex
	running         int32 // Atomic flag
	
	// Configuration
	policy CompactionPolicy
	
	// Statistics
	stats CompactionStats
	mu    sync.RWMutex // Protects stats and refCounter
	
	// Control channels
	stopCh   chan struct{}
	forceCh  chan CompactOptions
	resultCh chan error
}

// NewGarbageCollector creates a new garbage collector
func NewGarbageCollector(store *storage.Store) *GarbageCollector {
	return &GarbageCollector{
		store:      store,
		refCounter: make(map[string]int64),
		policy:     DefaultCompactionPolicy(),
		stopCh:     make(chan struct{}),
		forceCh:    make(chan CompactOptions, 1),
		resultCh:   make(chan error, 1),
	}
}

// Start begins the automatic compaction process
func (gc *GarbageCollector) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&gc.running, 0, 1) {
		return fmt.Errorf("garbage collector already running")
	}
	
	// Create new stop channel for this run
	gc.stopCh = make(chan struct{})
	gc.forceCh = make(chan CompactOptions, 1)
	gc.resultCh = make(chan error, 1)
	
	go gc.run(ctx)
	return nil
}

// Stop halts the automatic compaction process
func (gc *GarbageCollector) Stop() error {
	if !atomic.CompareAndSwapInt32(&gc.running, 1, 0) {
		return fmt.Errorf("garbage collector not running")
	}
	
	select {
	case <-gc.stopCh:
		// Already closed
	default:
		close(gc.stopCh)
	}
	return nil
}

// SetPolicy updates the compaction policy
func (gc *GarbageCollector) SetPolicy(policy CompactionPolicy) {
	gc.compactionMutex.Lock()
	defer gc.compactionMutex.Unlock()
	gc.policy = policy
}

// GetMemoryUsage returns current memory statistics
func (gc *GarbageCollector) GetMemoryUsage() MemoryStats {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	
	var totalObjects, totalBytes int64
	
	// Count objects in store
	if gc.store != nil {
		if objects, err := gc.store.ListObjects(); err == nil {
			totalObjects = int64(len(objects))
			
			// Estimate total bytes (rough estimate)
			totalBytes = totalObjects * 1024 // Assume 1KB average per object
		}
	}
	
	return MemoryStats{
		TotalObjects:   totalObjects,
		TotalBytes:     totalBytes,
		CompactedBytes: gc.stats.BytesFreed,
		LastCompaction: gc.lastCompaction,
		FragmentRatio:  gc.calculateFragmentRatio(),
	}
}

// GetStats returns compaction statistics
func (gc *GarbageCollector) GetStats() CompactionStats {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	return gc.stats
}

// ForceCompaction triggers immediate compaction
func (gc *GarbageCollector) ForceCompaction(opts CompactOptions) error {
	if atomic.LoadInt32(&gc.running) == 0 {
		// If not running, do manual compaction
		return gc.performCompaction(context.Background(), opts)
	}
	
	// Send to running goroutine
	select {
	case gc.forceCh <- opts:
		// Wait for result
		select {
		case err := <-gc.resultCh:
			return err
		case <-time.After(5 * time.Second):
			return fmt.Errorf("compaction timeout")
		}
	case <-time.After(1 * time.Second):
		return fmt.Errorf("compaction request timeout")
	}
}

// AddReference increments the reference count for an object
func (gc *GarbageCollector) AddReference(objectHash string) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.refCounter[objectHash]++
}

// RemoveReference decrements the reference count for an object
func (gc *GarbageCollector) RemoveReference(objectHash string) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	if count, exists := gc.refCounter[objectHash]; exists {
		if count <= 1 {
			gc.refCounter[objectHash] = 0 // Keep entry with 0 count for garbage collection
		} else {
			gc.refCounter[objectHash]--
		}
	}
}

// GetReferenceCount returns the reference count for an object
func (gc *GarbageCollector) GetReferenceCount(objectHash string) int64 {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	return gc.refCounter[objectHash]
}

// run is the main compaction loop
func (gc *GarbageCollector) run(ctx context.Context) {
	ticker := time.NewTicker(gc.policy.Interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-gc.stopCh:
			return
		case <-ticker.C:
			if gc.shouldCompact() {
				err := gc.performCompaction(ctx, CompactOptions{})
				if err != nil {
					gc.incrementErrorCount()
				}
			}
		case opts := <-gc.forceCh:
			err := gc.performCompaction(ctx, opts)
			select {
			case gc.resultCh <- err:
			case <-time.After(1 * time.Second):
				// Don't block if nobody is listening
			}
		}
	}
}

// shouldCompact determines if compaction should run
func (gc *GarbageCollector) shouldCompact() bool {
	if !gc.policy.Enabled {
		return false
	}
	
	stats := gc.GetMemoryUsage()
	
	// Check memory threshold
	if stats.FragmentRatio > gc.policy.MemoryThreshold {
		return true
	}
	
	// Check minimum interval since last compaction
	if time.Since(gc.lastCompaction) < gc.policy.Interval {
		return false
	}
	
	return true
}

// performCompaction executes the actual compaction
func (gc *GarbageCollector) performCompaction(ctx context.Context, opts CompactOptions) error {
	startTime := time.Now()
	
	gc.compactionMutex.Lock()
	defer gc.compactionMutex.Unlock()
	
	var objectsCollected, bytesFreed int64
	
	// Mark phase: identify unreferenced objects
	candidates := gc.identifyGarbageObjects()
	
	// Sweep phase: remove unreferenced objects
	for _, objectHash := range candidates {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		
		// Only collect objects older than minimum age
		if !gc.isObjectOldEnough(objectHash) {
			continue
		}
		
		// Remove the object
		if err := gc.removeObject(objectHash); err != nil {
			continue // Log error but continue with other objects
		}
		
		objectsCollected++
		bytesFreed += gc.estimateObjectSize(objectHash)
		
		// Respect max objects per cycle
		if gc.policy.MaxObjectsPerCycle > 0 && objectsCollected >= int64(gc.policy.MaxObjectsPerCycle) {
			break
		}
	}
	
	duration := time.Since(startTime)
	gc.updateStats(objectsCollected, bytesFreed, duration)
	gc.lastCompaction = time.Now()
	
	return nil
}

// identifyGarbageObjects finds objects with zero references
func (gc *GarbageCollector) identifyGarbageObjects() []string {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	
	var candidates []string
	for objectHash, refCount := range gc.refCounter {
		if refCount <= 0 { // Include zero and negative (corrupted) references
			candidates = append(candidates, objectHash)
		}
	}
	
	return candidates
}

// isObjectOldEnough checks if an object is old enough to be collected
func (gc *GarbageCollector) isObjectOldEnough(objectHash string) bool {
	// This would need implementation to track object creation times
	// For now, assume all objects are old enough
	return true
}

// removeObject removes an object from storage
func (gc *GarbageCollector) removeObject(objectHash string) error {
	if gc.store == nil {
		return fmt.Errorf("store is nil")
	}
	
	// Remove from cache and backend if possible
	if gc.store != nil {
		// For now, we can't remove from store as it doesn't have a remove method
		// This would be implemented when we extend the Store interface
		// err := gc.store.RemoveObject(objectHash)
	}
	
	// Remove from reference counter
	gc.mu.Lock()
	delete(gc.refCounter, objectHash)
	gc.mu.Unlock()
	
	return nil
}

// estimateObjectSize estimates the size of an object
func (gc *GarbageCollector) estimateObjectSize(objectHash string) int64 {
	// This would need implementation to get actual object sizes
	// For now, return a reasonable estimate
	return 1024 // 1KB average
}

// calculateFragmentRatio calculates memory fragmentation ratio
func (gc *GarbageCollector) calculateFragmentRatio() float64 {
	totalObjects := int64(len(gc.refCounter))
	if totalObjects == 0 {
		return 0.0
	}
	
	unreferenced := int64(0)
	for _, refCount := range gc.refCounter {
		if refCount == 0 {
			unreferenced++
		}
	}
	
	return float64(unreferenced) / float64(totalObjects)
}

// updateStats updates compaction statistics
func (gc *GarbageCollector) updateStats(objectsCollected, bytesFreed int64, duration time.Duration) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	
	gc.stats.TotalRuns++
	gc.stats.ObjectsCollected += objectsCollected
	gc.stats.BytesFreed += bytesFreed
	gc.stats.LastRunTime = duration
	
	// Calculate average run time
	if gc.stats.TotalRuns > 0 {
		totalTime := time.Duration(gc.stats.TotalRuns) * gc.stats.AverageRunTime + duration
		gc.stats.AverageRunTime = totalTime / time.Duration(gc.stats.TotalRuns)
	}
	
	gc.stats.NextRun = time.Now().Add(gc.policy.Interval)
}

// incrementErrorCount increments the error count
func (gc *GarbageCollector) incrementErrorCount() {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.stats.ErrorCount++
}

// Compact implements the Compactable interface for Repository
func (r *Repository) Compact(ctx context.Context) error {
	if r.gc == nil {
		return fmt.Errorf("garbage collector not initialized")
	}
	
	return r.gc.ForceCompaction(CompactOptions{})
}

// CompactWithOptions performs compaction with specific options
func (r *Repository) CompactWithOptions(opts CompactOptions) error {
	if r.gc == nil {
		return fmt.Errorf("garbage collector not initialized")
	}
	
	return r.gc.ForceCompaction(opts)
}

// GetMemoryUsage returns memory usage statistics
func (r *Repository) GetMemoryUsage() MemoryStats {
	if r.gc == nil {
		return MemoryStats{}
	}
	
	return r.gc.GetMemoryUsage()
}

// SetCompactionPolicy sets the compaction policy
func (r *Repository) SetCompactionPolicy(policy CompactionPolicy) {
	if r.gc != nil {
		r.gc.SetPolicy(policy)
	}
}

// EnableAutoCompaction enables automatic compaction with the given policy
func (r *Repository) EnableAutoCompaction(policy CompactionPolicy) error {
	if r.gc == nil {
		r.gc = NewGarbageCollector(r.store)
	}
	
	r.gc.SetPolicy(policy)
	return r.gc.Start(context.Background())
}

// GetCompactionStats returns compaction statistics
func (r *Repository) GetCompactionStats() CompactionStats {
	if r.gc == nil {
		return CompactionStats{}
	}
	
	return r.gc.GetStats()
}