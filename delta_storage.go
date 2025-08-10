package govc

import (
	"fmt"
	"time"

	"github.com/caiatech/govc/pkg/object"
)

// DeltaStorage provides delta compression integration with the repository

// StoreBlobWithDelta stores a blob using delta compression if beneficial
func (r *Repository) StoreBlobWithDelta(content []byte) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Generate hash for the content
	blob := object.NewBlob(content)
	data, err := blob.Serialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize blob: %w", err)
	}
	
	hash := object.HashObject(data)
	
	// Check if already exists in store
	if r.store.HasObject(hash) {
		return hash, nil
	}
	
	// Try delta compression first
	_, usedDelta, err := r.deltaCompression.StoreWithDelta(hash, data)
	if err != nil {
		return "", fmt.Errorf("delta compression failed: %w", err)
	}
	
	if usedDelta {
		// Successfully stored as delta, return hash
		return hash, nil
	}
	
	// Fall back to normal storage
	return r.store.StoreBlob(content)
}

// GetBlobWithDelta retrieves a blob, decompressing deltas if necessary
func (r *Repository) GetBlobWithDelta(hash string) (*object.Blob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// First try delta compression system
	data, usedDelta, err := r.deltaCompression.GetWithDelta(hash)
	if err == nil && usedDelta {
		// Successfully retrieved from delta system
		obj, err := object.ParseObject(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse delta object: %w", err)
		}
		
		blob, ok := obj.(*object.Blob)
		if !ok {
			return nil, fmt.Errorf("object %s is not a blob", hash)
		}
		
		return blob, nil
	}
	
	// Fall back to regular store
	return r.store.GetBlob(hash)
}

// GetDeltaCompressionStats returns delta compression statistics
func (r *Repository) GetDeltaCompressionStats() DeltaStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	return r.deltaCompression.GetStats()
}

// OptimizeDeltaChains optimizes delta chains for better performance
func (r *Repository) OptimizeDeltaChains() {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.deltaCompression.OptimizeChains()
}

// DeltaAwareBlobStore provides a delta-aware interface to blob storage
type DeltaAwareBlobStore struct {
	repo *Repository
}

// NewDeltaAwareBlobStore creates a new delta-aware blob store
func NewDeltaAwareBlobStore(repo *Repository) *DeltaAwareBlobStore {
	return &DeltaAwareBlobStore{
		repo: repo,
	}
}

// Store stores content using delta compression when beneficial
func (ds *DeltaAwareBlobStore) Store(content []byte) (string, error) {
	return ds.repo.StoreBlobWithDelta(content)
}

// Get retrieves content, handling delta decompression
func (ds *DeltaAwareBlobStore) Get(hash string) ([]byte, error) {
	blob, err := ds.repo.GetBlobWithDelta(hash)
	if err != nil {
		return nil, err
	}
	return blob.Content, nil
}

// Has checks if content exists (in delta or regular storage)
func (ds *DeltaAwareBlobStore) Has(hash string) bool {
	ds.repo.mu.RLock()
	defer ds.repo.mu.RUnlock()
	
	// Check delta system first
	_, usedDelta, err := ds.repo.deltaCompression.GetWithDelta(hash)
	if err == nil && usedDelta {
		return true
	}
	
	// Check regular store
	return ds.repo.store.HasObject(hash)
}

// Stats returns comprehensive storage statistics
func (ds *DeltaAwareBlobStore) Stats() DeltaBlobStats {
	ds.repo.mu.RLock()
	defer ds.repo.mu.RUnlock()
	
	deltaStats := ds.repo.deltaCompression.GetStats()
	
	// Get regular store statistics (would need to implement this)
	regularObjects := int64(0)
	regularSize := int64(0)
	
	// Approximate regular store size (this would need actual implementation)
	objects, _ := ds.repo.store.ListObjects()
	regularObjects = int64(len(objects))
	
	return DeltaBlobStats{
		TotalObjects:        deltaStats.TotalObjects + regularObjects,
		DeltaObjects:        deltaStats.DeltaObjects,
		BaseObjects:         deltaStats.BaseObjects,
		RegularObjects:      regularObjects,
		TotalOriginalSize:   deltaStats.TotalOriginalSize + regularSize,
		TotalCompressedSize: deltaStats.TotalCompressedSize + regularSize,
		OverallCompression:  float64(deltaStats.TotalCompressedSize + regularSize) / float64(deltaStats.TotalOriginalSize + regularSize),
		DeltaStats:          deltaStats,
	}
}

// DeltaBlobStats provides comprehensive blob storage statistics
type DeltaBlobStats struct {
	TotalObjects        int64      `json:"total_objects"`
	DeltaObjects        int64      `json:"delta_objects"`
	BaseObjects         int64      `json:"base_objects"`
	RegularObjects      int64      `json:"regular_objects"`
	TotalOriginalSize   int64      `json:"total_original_size"`
	TotalCompressedSize int64      `json:"total_compressed_size"`
	OverallCompression  float64    `json:"overall_compression"`
	DeltaStats          DeltaStats `json:"delta_stats"`
}

// Background delta optimization task
func (r *Repository) startDeltaOptimization() {
	ticker := time.NewTicker(10 * time.Minute) // Optimize every 10 minutes
	defer ticker.Stop()
	
	for range ticker.C {
		r.OptimizeDeltaChains()
	}
}

// Integration with existing Repository methods

// UpdatedStoreBlob method that uses delta compression
func (r *Repository) StoreBlob(content []byte) (string, error) {
	// Use delta compression for high-frequency operations
	return r.StoreBlobWithDelta(content)
}

// DeltaAwareCommit creates a commit using delta compression
func (r *Repository) DeltaAwareCommit(message string) (*object.Commit, error) {
	start := time.Now()
	
	// Use existing commit logic but with delta-aware storage
	commit, err := r.Commit(message)
	if err != nil {
		return nil, err
	}
	
	// Log compression statistics for monitoring
	if commit != nil {
		stats := r.GetDeltaCompressionStats()
		r.logDeltaStats(message, stats, time.Since(start))
	}
	
	return commit, nil
}

// logDeltaStats logs delta compression statistics for monitoring
func (r *Repository) logDeltaStats(operation string, stats DeltaStats, duration time.Duration) {
	// This would integrate with the logging system
	// For now, just track statistics internally
	
	// Could publish to event bus for monitoring
	if r.eventBus != nil {
		event := &RepositoryEvent{
			Event:     "delta_compression",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"operation":             operation,
				"compression_ratio":     stats.CompressionRatio,
				"delta_objects":         stats.DeltaObjects,
				"total_objects":         stats.TotalObjects,
				"average_chain_length":  stats.AverageChainLength,
				"duration":              duration.String(),
				"message":               fmt.Sprintf("Operation: %s, Compression: %.2f%%, Duration: %v", operation, (1.0-stats.CompressionRatio)*100, duration),
			},
		}
		
		// Publish async to not block operations
		go func() {
			select {
			case r.events <- event:
			default:
				// Events channel full, drop event
			}
		}()
	}
}