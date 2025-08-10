package govc

import (
	"fmt"
	"sync"
	"time"
)

// DeltaCompression provides efficient storage for high-frequency commits
// Critical for reducing storage overhead in MMO applications with 72K commits/hour
type DeltaCompression struct {
	// Delta storage
	deltas      map[string]*DeltaEntry    // hash -> delta entry
	baseObjects map[string][]byte         // hash -> full object data
	deltaChains map[string]*DeltaChain    // object_hash -> chain of deltas
	
	// Configuration
	maxDeltaChainLength int               // Maximum chain length before creating new base
	compressionRatio    float64           // Minimum compression ratio to use delta
	baseSelectionAlgo   BaseSelectionType // Algorithm for selecting base objects
	
	// Statistics
	stats DeltaStats
	mu    sync.RWMutex
}

// DeltaEntry represents a compressed delta
type DeltaEntry struct {
	Hash         string    `json:"hash"`          // Hash of the delta object
	BaseHash     string    `json:"base_hash"`     // Hash of the base object
	DeltaData    []byte    `json:"delta_data"`    // Compressed delta
	OriginalSize int       `json:"original_size"` // Size of original object
	DeltaSize    int       `json:"delta_size"`    // Size of delta
	CreatedAt    time.Time `json:"created_at"`    // When delta was created
	AccessCount  int64     `json:"access_count"`  // How often accessed
	ChainDepth   int       `json:"chain_depth"`   // Depth in delta chain
}

// DeltaChain manages a sequence of deltas
type DeltaChain struct {
	BaseHash    string       `json:"base_hash"`    // Hash of base object
	Deltas      []*DeltaEntry `json:"deltas"`       // Ordered list of deltas
	TotalSize   int          `json:"total_size"`   // Total size of all deltas
	ChainLength int          `json:"chain_length"` // Current chain length
	LastAccess  time.Time    `json:"last_access"`  // Last access time
}

// BaseSelectionType defines algorithms for selecting base objects
type BaseSelectionType int

const (
	BaseSelectionSimilarity BaseSelectionType = iota // Select most similar object
	BaseSelectionRecent                              // Select most recent object
	BaseSelectionFrequent                            // Select most frequently accessed
	BaseSelectionHybrid                              // Combination of factors
)

// DeltaStats tracks compression statistics
type DeltaStats struct {
	TotalObjects         int64   `json:"total_objects"`
	DeltaObjects         int64   `json:"delta_objects"`
	BaseObjects          int64   `json:"base_objects"`
	TotalOriginalSize    int64   `json:"total_original_size"`
	TotalCompressedSize  int64   `json:"total_compressed_size"`
	CompressionRatio     float64 `json:"compression_ratio"`
	AverageChainLength   float64 `json:"average_chain_length"`
	DeltaCreations       int64   `json:"delta_creations"`
	BasePromotions       int64   `json:"base_promotions"`
	ChainBreaks          int64   `json:"chain_breaks"`
	DecompressionTime    time.Duration `json:"decompression_time"`
	CompressionTime      time.Duration `json:"compression_time"`
}

// DeltaAlgorithm defines interface for delta algorithms
type DeltaAlgorithm interface {
	CreateDelta(baseData, targetData []byte) ([]byte, error)
	ApplyDelta(baseData, deltaData []byte) ([]byte, error)
	CompressionRatio(baseData, targetData, deltaData []byte) float64
}

// VCDIFFDelta implements VCDIFF-based delta compression
type VCDIFFDelta struct {
	windowSize   int    // Rolling window size for matching
	minMatchSize int    // Minimum match size
}

// BinaryDelta implements simple binary delta compression
type BinaryDelta struct {
	blockSize int // Block size for comparison
}

// NewDeltaCompression creates a new delta compression system
func NewDeltaCompression() *DeltaCompression {
	return &DeltaCompression{
		deltas:              make(map[string]*DeltaEntry),
		baseObjects:         make(map[string][]byte),
		deltaChains:         make(map[string]*DeltaChain),
		maxDeltaChainLength: 10,    // Limit chain depth to avoid deep recursion
		compressionRatio:    0.5,   // Must save at least 50% to use delta
		baseSelectionAlgo:   BaseSelectionHybrid,
		stats:              DeltaStats{},
	}
}

// StoreWithDelta stores an object using delta compression if beneficial
func (dc *DeltaCompression) StoreWithDelta(hash string, data []byte) (*DeltaEntry, bool, error) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	
	// Check if object already exists
	if entry, exists := dc.deltas[hash]; exists {
		entry.AccessCount++
		return entry, true, nil
	}
	
	// Find best base object for delta compression
	baseHash, baseData := dc.selectBaseObject(data)
	if baseHash == "" {
		// No suitable base found, store as base object
		dc.baseObjects[hash] = data
		dc.stats.BaseObjects++
		dc.stats.TotalObjects++
		dc.stats.TotalOriginalSize += int64(len(data))
		dc.stats.TotalCompressedSize += int64(len(data))
		return nil, false, nil
	}
	
	// Create delta
	deltaAlgo := &VCDIFFDelta{windowSize: 1024, minMatchSize: 4}
	deltaData, err := deltaAlgo.CreateDelta(baseData, data)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create delta: %w", err)
	}
	
	// Check if delta compression is beneficial
	compressionRatio := float64(len(deltaData)) / float64(len(data))
	if compressionRatio > dc.compressionRatio {
		// Delta not beneficial, store as base
		dc.baseObjects[hash] = data
		dc.stats.BaseObjects++
		dc.stats.TotalObjects++
		dc.stats.TotalOriginalSize += int64(len(data))
		dc.stats.TotalCompressedSize += int64(len(data))
		return nil, false, nil
	}
	
	// Create delta entry
	entry := &DeltaEntry{
		Hash:         hash,
		BaseHash:     baseHash,
		DeltaData:    deltaData,
		OriginalSize: len(data),
		DeltaSize:    len(deltaData),
		CreatedAt:    time.Now(),
		AccessCount:  1,
		ChainDepth:   dc.getChainDepth(baseHash) + 1,
	}
	
	// Check chain length limits
	if entry.ChainDepth > dc.maxDeltaChainLength {
		// Chain too long, promote to base object
		dc.baseObjects[hash] = data
		dc.stats.BasePromotions++
		dc.stats.BaseObjects++
		dc.stats.TotalObjects++
		dc.stats.TotalOriginalSize += int64(len(data))
		dc.stats.TotalCompressedSize += int64(len(data))
		return nil, false, nil
	}
	
	// Store delta
	dc.deltas[hash] = entry
	dc.addToChain(baseHash, entry)
	
	// Update statistics
	dc.stats.DeltaObjects++
	dc.stats.TotalObjects++
	dc.stats.DeltaCreations++
	dc.stats.TotalOriginalSize += int64(len(data))
	dc.stats.TotalCompressedSize += int64(len(deltaData))
	dc.updateCompressionStats()
	
	return entry, true, nil
}

// GetWithDelta retrieves an object, decompressing deltas if necessary
func (dc *DeltaCompression) GetWithDelta(hash string) ([]byte, bool, error) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	
	start := time.Now()
	defer func() {
		dc.stats.DecompressionTime += time.Since(start)
	}()
	
	// Check if it's a base object
	if baseData, exists := dc.baseObjects[hash]; exists {
		return baseData, false, nil
	}
	
	// Check if it's a delta object
	entry, exists := dc.deltas[hash]
	if !exists {
		return nil, false, fmt.Errorf("object not found: %s", hash)
	}
	
	// Reconstruct object from delta chain
	data, err := dc.reconstructFromDelta(entry)
	if err != nil {
		return nil, true, fmt.Errorf("failed to reconstruct delta: %w", err)
	}
	
	// Update access statistics
	entry.AccessCount++
	if chain, exists := dc.deltaChains[entry.BaseHash]; exists {
		chain.LastAccess = time.Now()
	}
	
	return data, true, nil
}

// selectBaseObject finds the best base object for delta compression
func (dc *DeltaCompression) selectBaseObject(data []byte) (string, []byte) {
	switch dc.baseSelectionAlgo {
	case BaseSelectionSimilarity:
		return dc.selectBySimilarity(data)
	case BaseSelectionRecent:
		return dc.selectByRecency(data)
	case BaseSelectionFrequent:
		return dc.selectByFrequency(data)
	case BaseSelectionHybrid:
		return dc.selectByHybrid(data)
	default:
		return dc.selectBySimilarity(data)
	}
}

// selectBySimilarity finds the most similar base object
func (dc *DeltaCompression) selectBySimilarity(data []byte) (string, []byte) {
	bestHash := ""
	bestData := []byte{}
	bestSimilarity := 0.0
	
	// Compare against base objects
	for hash, baseData := range dc.baseObjects {
		similarity := dc.calculateSimilarity(baseData, data)
		if similarity > bestSimilarity && similarity > 0.3 { // Minimum similarity threshold
			bestSimilarity = similarity
			bestHash = hash
			bestData = baseData
		}
	}
	
	// Also consider recent delta objects as potential bases
	for _, entry := range dc.deltas {
		if entry.ChainDepth < dc.maxDeltaChainLength-2 { // Leave room for new delta
			// Would need to reconstruct this object, skip for now
			continue
		}
	}
	
	return bestHash, bestData
}

// selectByRecency selects the most recently created base object
func (dc *DeltaCompression) selectByRecency(data []byte) (string, []byte) {
	// For simplicity, return first available base object
	// In a real implementation, would track creation times
	for hash, baseData := range dc.baseObjects {
		return hash, baseData
	}
	return "", nil
}

// selectByFrequency selects the most frequently accessed object
func (dc *DeltaCompression) selectByFrequency(data []byte) (string, []byte) {
	bestHash := ""
	bestData := []byte{}
	_ = int64(0) // bestAccessCount placeholder
	
	// Find most accessed base object
	for hash, baseData := range dc.baseObjects {
		// Would need to track access counts for base objects
		// For now, use first available
		if bestHash == "" {
			bestHash = hash
			bestData = baseData
			_ = 1 // bestAccessCount = 1
		}
	}
	
	return bestHash, bestData
}

// selectByHybrid uses multiple factors to select best base
func (dc *DeltaCompression) selectByHybrid(data []byte) (string, []byte) {
	bestHash := ""
	bestData := []byte{}
	bestScore := 0.0
	
	for hash, baseData := range dc.baseObjects {
		// Calculate composite score
		similarity := dc.calculateSimilarity(baseData, data)
		
		// Weight similarity heavily, with small bonuses for other factors
		score := similarity * 0.8
		
		// Bonus for shorter delta chains
		if chain, exists := dc.deltaChains[hash]; exists {
			chainPenalty := float64(chain.ChainLength) / float64(dc.maxDeltaChainLength)
			score *= (1.0 - chainPenalty*0.2)
		}
		
		if score > bestScore && score > 0.2 { // Minimum threshold
			bestScore = score
			bestHash = hash
			bestData = baseData
		}
	}
	
	return bestHash, bestData
}

// calculateSimilarity estimates similarity between two byte arrays
func (dc *DeltaCompression) calculateSimilarity(a, b []byte) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	
	// Simple byte-level similarity using longest common subsequence approximation
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	
	commonBytes := 0
	for i := 0; i < minLen; i++ {
		if a[i] == b[i] {
			commonBytes++
		}
	}
	
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	
	return float64(commonBytes) / float64(maxLen)
}

// reconstructFromDelta reconstructs original data from delta entry
func (dc *DeltaCompression) reconstructFromDelta(entry *DeltaEntry) ([]byte, error) {
	// Get base object
	var baseData []byte
	var err error
	
	if baseEntry, exists := dc.deltas[entry.BaseHash]; exists {
		// Base is also a delta, recurse
		baseData, err = dc.reconstructFromDelta(baseEntry)
		if err != nil {
			return nil, err
		}
	} else if base, exists := dc.baseObjects[entry.BaseHash]; exists {
		baseData = base
	} else {
		return nil, fmt.Errorf("base object not found: %s", entry.BaseHash)
	}
	
	// Apply delta
	deltaAlgo := &VCDIFFDelta{windowSize: 1024, minMatchSize: 4}
	return deltaAlgo.ApplyDelta(baseData, entry.DeltaData)
}

// getChainDepth returns the depth of delta chain for an object
func (dc *DeltaCompression) getChainDepth(hash string) int {
	if entry, exists := dc.deltas[hash]; exists {
		return entry.ChainDepth
	}
	return 0 // Base objects have depth 0
}

// addToChain adds a delta entry to the appropriate chain
func (dc *DeltaCompression) addToChain(baseHash string, entry *DeltaEntry) {
	chain, exists := dc.deltaChains[baseHash]
	if !exists {
		chain = &DeltaChain{
			BaseHash:    baseHash,
			Deltas:      make([]*DeltaEntry, 0),
			ChainLength: 0,
			LastAccess:  time.Now(),
		}
		dc.deltaChains[baseHash] = chain
	}
	
	chain.Deltas = append(chain.Deltas, entry)
	chain.ChainLength++
	chain.TotalSize += entry.DeltaSize
	chain.LastAccess = time.Now()
}

// updateCompressionStats recalculates compression statistics
func (dc *DeltaCompression) updateCompressionStats() {
	if dc.stats.TotalOriginalSize == 0 {
		dc.stats.CompressionRatio = 0
		return
	}
	
	dc.stats.CompressionRatio = float64(dc.stats.TotalCompressedSize) / float64(dc.stats.TotalOriginalSize)
	
	// Calculate average chain length
	if len(dc.deltaChains) > 0 {
		totalChainLength := 0
		for _, chain := range dc.deltaChains {
			totalChainLength += chain.ChainLength
		}
		dc.stats.AverageChainLength = float64(totalChainLength) / float64(len(dc.deltaChains))
	}
}

// GetStats returns current delta compression statistics
func (dc *DeltaCompression) GetStats() DeltaStats {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	
	// Update stats before returning
	stats := dc.stats
	return stats
}

// OptimizeChains optimizes delta chains for better performance
func (dc *DeltaCompression) OptimizeChains() {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	
	for _, chain := range dc.deltaChains {
		// If chain is too long or hasn't been accessed recently, consider breaking it
		if chain.ChainLength > dc.maxDeltaChainLength || 
		   time.Since(chain.LastAccess) > 24*time.Hour {
			
			// Promote frequently accessed deltas to base objects
			for _, entry := range chain.Deltas {
				if entry.AccessCount > 10 { // Threshold for promotion
					// Reconstruct and store as base
					if data, err := dc.reconstructFromDelta(entry); err == nil {
						dc.baseObjects[entry.Hash] = data
						delete(dc.deltas, entry.Hash)
						dc.stats.BasePromotions++
						dc.stats.ChainBreaks++
					}
				}
			}
		}
	}
}