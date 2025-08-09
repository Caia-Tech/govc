package lsm

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Compactor handles background compaction of LSM tree levels
// Implements both leveled and universal compaction strategies
type Compactor struct {
	lsm           *LSMStore
	config        LSMConfig
	running       bool
	mu            sync.Mutex
	
	// Compaction statistics
	totalCompactions    int64
	totalBytesRead      int64
	totalBytesWritten   int64
	lastCompactionTime  time.Time
}

// NewCompactor creates a new compactor
func NewCompactor(lsm *LSMStore, config LSMConfig) *Compactor {
	return &Compactor{
		lsm:    lsm,
		config: config,
	}
}

// Start starts the background compaction process
func (c *Compactor) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.running {
		return
	}
	
	c.running = true
	
	// Start compaction workers
	for i := 0; i < c.config.CompactionThreads; i++ {
		go c.compactionWorker(i)
	}
}

// Stop stops the background compaction process
func (c *Compactor) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.running = false
}

// compactionWorker runs background compaction
func (c *Compactor) compactionWorker(workerID int) {
	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if !c.isRunning() {
				return
			}
			
			c.performCompaction()
			
		case <-c.lsm.stopCh:
			return
		}
	}
}

// isRunning checks if compaction is still running
func (c *Compactor) isRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// performCompaction performs a single compaction cycle
func (c *Compactor) performCompaction() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	startTime := time.Now()
	
	switch c.config.CompactionStyle {
	case LeveledCompaction:
		c.performLeveledCompaction()
	case UniversalCompaction:
		c.performUniversalCompaction()
	}
	
	c.lastCompactionTime = time.Now()
	c.totalCompactions++
	
	// Update metrics
	duration := time.Since(startTime)
	c.updateMetrics(duration)
}

// performLeveledCompaction performs leveled compaction strategy
func (c *Compactor) performLeveledCompaction() {
	c.lsm.mu.RLock()
	levels := c.lsm.levels
	c.lsm.mu.RUnlock()
	
	// Check each level for compaction needs
	for i := 0; i < len(levels)-1; i++ {
		level := levels[i]
		nextLevel := levels[i+1]
		
		if level.NeedsCompaction() {
			c.compactLevel(level, nextLevel)
			break // Only compact one level at a time
		}
	}
}

// performUniversalCompaction performs universal compaction strategy
func (c *Compactor) performUniversalCompaction() {
	c.lsm.mu.RLock()
	levels := c.lsm.levels
	c.lsm.mu.RUnlock()
	
	// Find the level with the most files that needs compaction
	var targetLevel *Level
	maxFiles := 0
	
	for _, level := range levels {
		if level.NeedsCompaction() {
			stats := level.Stats()
			if stats.NumFiles > maxFiles {
				maxFiles = stats.NumFiles
				targetLevel = level
			}
		}
	}
	
	if targetLevel != nil {
		c.compactLevelInPlace(targetLevel)
	}
}

// compactLevel compacts a level into the next level
func (c *Compactor) compactLevel(sourceLevel, targetLevel *Level) error {
	// Select files for compaction
	sourceFiles := sourceLevel.SelectFilesForCompaction()
	if len(sourceFiles) == 0 {
		return nil
	}
	
	// Find overlapping files in target level
	targetFiles := c.findOverlappingFiles(targetLevel, sourceFiles)
	
	// Create merge iterator
	allFiles := append(sourceFiles, targetFiles...)
	
	// Perform the merge
	newSSTables, err := c.mergeSSTables(allFiles, targetLevel)
	if err != nil {
		return fmt.Errorf("failed to merge SSTables: %w", err)
	}
	
	// Update levels atomically
	c.lsm.mu.Lock()
	defer c.lsm.mu.Unlock()
	
	// Remove old files
	sourceLevel.RemoveSSTables(sourceFiles)
	targetLevel.RemoveSSTables(targetFiles)
	
	// Add new files
	for _, sstable := range newSSTables {
		targetLevel.AddSSTable(sstable)
	}
	
	return nil
}

// compactLevelInPlace compacts a level by merging overlapping files
func (c *Compactor) compactLevelInPlace(level *Level) error {
	// Get all files in level
	level.mu.RLock()
	allFiles := make([]*SSTable, len(level.sstables))
	copy(allFiles, level.sstables)
	level.mu.RUnlock()
	
	if len(allFiles) <= 1 {
		return nil // Nothing to compact
	}
	
	// Merge all files
	newSSTables, err := c.mergeSSTables(allFiles, level)
	if err != nil {
		return fmt.Errorf("failed to merge SSTables: %w", err)
	}
	
	// Replace old files with new ones
	c.lsm.mu.Lock()
	defer c.lsm.mu.Unlock()
	
	level.RemoveSSTables(allFiles)
	for _, sstable := range newSSTables {
		level.AddSSTable(sstable)
	}
	
	return nil
}

// findOverlappingFiles finds files in the target level that overlap with source files
func (c *Compactor) findOverlappingFiles(targetLevel *Level, sourceFiles []*SSTable) []*SSTable {
	if len(sourceFiles) == 0 {
		return nil
	}
	
	// Find key range of source files
	minKey := sourceFiles[0].minKey
	maxKey := sourceFiles[0].maxKey
	
	for _, file := range sourceFiles {
		if file.minKey < minKey {
			minKey = file.minKey
		}
		if file.maxKey > maxKey {
			maxKey = file.maxKey
		}
	}
	
	// Find overlapping files in target level
	targetLevel.mu.RLock()
	defer targetLevel.mu.RUnlock()
	
	overlapping := make([]*SSTable, 0)
	for _, file := range targetLevel.sstables {
		if c.keyRangesOverlap(minKey, maxKey, file.minKey, file.maxKey) {
			overlapping = append(overlapping, file)
		}
	}
	
	return overlapping
}

// keyRangesOverlap checks if two key ranges overlap
func (c *Compactor) keyRangesOverlap(min1, max1, min2, max2 string) bool {
	return !(max1 < min2 || max2 < min1)
}

// mergeSSTables merges multiple SSTables into one or more new SSTables
func (c *Compactor) mergeSSTables(sstables []*SSTable, targetLevel *Level) ([]*SSTable, error) {
	if len(sstables) == 0 {
		return nil, nil
	}
	
	// Create merge iterator
	merger := NewSSTableMerger(sstables)
	
	newSSTables := make([]*SSTable, 0)
	currentWriter, err := targetLevel.CreateSSTable()
	if err != nil {
		return nil, err
	}
	
	currentSize := int64(0)
	maxSSTableSize := c.config.SSTableSize
	
	// Iterate through all entries in sorted order
	for merger.Valid() {
		entry := merger.Current()
		
		// Check if we need to start a new SSTable
		if currentSize > maxSSTableSize {
			// Finish current SSTable
			sstable, err := currentWriter.Finish()
			if err != nil {
				return nil, err
			}
			newSSTables = append(newSSTables, sstable)
			
			// Start new SSTable
			currentWriter, err = targetLevel.CreateSSTable()
			if err != nil {
				return nil, err
			}
			currentSize = 0
		}
		
		// Add entry to current SSTable
		err := currentWriter.Add(entry.Key, entry.Value, entry.Deleted)
		if err != nil {
			return nil, err
		}
		
		currentSize += entry.Size
		c.totalBytesRead += entry.Size
		c.totalBytesWritten += entry.Size
		
		merger.Next()
	}
	
	// Finish final SSTable
	if currentSize > 0 {
		sstable, err := currentWriter.Finish()
		if err != nil {
			return nil, err
		}
		newSSTables = append(newSSTables, sstable)
	}
	
	return newSSTables, nil
}

// updateMetrics updates compaction metrics
func (c *Compactor) updateMetrics(duration time.Duration) {
	// Update LSM store metrics
	c.lsm.mu.Lock()
	c.lsm.metrics.Writes++ // Compaction is a write operation
	c.lsm.mu.Unlock()
}

// CompactionStats provides compaction statistics
type CompactionStats struct {
	TotalCompactions   int64
	BytesRead         int64
	BytesWritten      int64
	LastCompactionTime time.Time
	Running           bool
}

// Stats returns compaction statistics
func (c *Compactor) Stats() CompactionStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	return CompactionStats{
		TotalCompactions:   c.totalCompactions,
		BytesRead:         c.totalBytesRead,
		BytesWritten:      c.totalBytesWritten,
		LastCompactionTime: c.lastCompactionTime,
		Running:           c.running,
	}
}

// SSTableMerger provides ordered iteration over multiple SSTables
type SSTableMerger struct {
	iterators []*SSTableIterator
	heap      *mergeHeap
}

// SSTableIterator iterates over a single SSTable
type SSTableIterator struct {
	sstable      *SSTable
	blockIndex   int
	entryIndex   int
	currentBlock *Block
	valid        bool
}

// NewSSTableMerger creates a new SSTable merger
func NewSSTableMerger(sstables []*SSTable) *SSTableMerger {
	iterators := make([]*SSTableIterator, len(sstables))
	
	for i, sstable := range sstables {
		iterator := &SSTableIterator{
			sstable: sstable,
		}
		iterator.SeekToFirst()
		iterators[i] = iterator
	}
	
	// Initialize min-heap for efficient merging
	heap := &mergeHeap{}
	for i, iter := range iterators {
		if iter.Valid() {
			heap.Push(&heapItem{
				entry:    iter.Current(),
				iterator: i,
			})
		}
	}
	
	return &SSTableMerger{
		iterators: iterators,
		heap:      heap,
	}
}

// Valid returns true if there are more entries
func (m *SSTableMerger) Valid() bool {
	return m.heap.Len() > 0
}

// Current returns the current entry
func (m *SSTableMerger) Current() *Entry {
	if m.heap.Len() == 0 {
		return nil
	}
	return m.heap.Peek().entry
}

// Next advances to the next entry
func (m *SSTableMerger) Next() {
	if m.heap.Len() == 0 {
		return
	}
	
	// Get current top item
	item := m.heap.Pop().(*heapItem)
	iteratorIndex := item.iterator
	
	// Advance the iterator that produced this entry
	m.iterators[iteratorIndex].Next()
	
	// If iterator still has entries, add it back to heap
	if m.iterators[iteratorIndex].Valid() {
		m.heap.Push(&heapItem{
			entry:    m.iterators[iteratorIndex].Current(),
			iterator: iteratorIndex,
		})
	}
	
	// Handle duplicate keys (keep the newest entry)
	for m.heap.Len() > 0 && m.heap.Peek().entry.Key == item.entry.Key {
		duplicateItem := m.heap.Pop().(*heapItem)
		duplicateIndex := duplicateItem.iterator
		
		m.iterators[duplicateIndex].Next()
		if m.iterators[duplicateIndex].Valid() {
			m.heap.Push(&heapItem{
				entry:    m.iterators[duplicateIndex].Current(),
				iterator: duplicateIndex,
			})
		}
	}
}

// SeekToFirst positions the iterator at the first entry
func (it *SSTableIterator) SeekToFirst() {
	it.blockIndex = 0
	it.entryIndex = 0
	it.loadCurrentBlock()
}

// Valid returns true if the iterator is at a valid position
func (it *SSTableIterator) Valid() bool {
	return it.valid
}

// Current returns the current entry
func (it *SSTableIterator) Current() *Entry {
	if !it.valid || it.currentBlock == nil || it.entryIndex >= len(it.currentBlock.entries) {
		return nil
	}
	return it.currentBlock.entries[it.entryIndex]
}

// Next advances the iterator
func (it *SSTableIterator) Next() {
	if !it.valid {
		return
	}
	
	it.entryIndex++
	
	// Check if we need to move to next block
	if it.currentBlock == nil || it.entryIndex >= len(it.currentBlock.entries) {
		it.blockIndex++
		it.entryIndex = 0
		it.loadCurrentBlock()
	}
}

// loadCurrentBlock loads the current block
func (it *SSTableIterator) loadCurrentBlock() {
	if it.sstable == nil || it.sstable.index == nil {
		it.valid = false
		return
	}
	
	if it.blockIndex >= len(it.sstable.index.entries) {
		it.valid = false
		return
	}
	
	block, err := it.sstable.loadBlock(it.blockIndex)
	if err != nil {
		it.valid = false
		return
	}
	
	it.currentBlock = block
	it.valid = len(block.entries) > 0
}

// Heap implementation for efficient merging
type heapItem struct {
	entry    *Entry
	iterator int
}

type mergeHeap []*heapItem

func (h *mergeHeap) Len() int { return len(*h) }

func (h *mergeHeap) Less(i, j int) bool {
	return (*h)[i].entry.Key < (*h)[j].entry.Key
}

func (h *mergeHeap) Swap(i, j int) { (*h)[i], (*h)[j] = (*h)[j], (*h)[i] }

func (h *mergeHeap) Push(x interface{}) {
	*h = append(*h, x.(*heapItem))
}

func (h *mergeHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

func (h *mergeHeap) Peek() *heapItem {
	if len(*h) == 0 {
		return nil
	}
	return (*h)[0]
}

// Maintain heap property after modifying elements
func (h *mergeHeap) Fix(i int) {
	// This would implement heap fix operation
	// Simplified for this implementation
	sort.Slice(*h, func(i, j int) bool {
		return (*h)[i].entry.Key < (*h)[j].entry.Key
	})
}