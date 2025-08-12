package search

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Caia-Tech/govc/pkg/object"
)

// startIndexing begins background indexing of repository data
func (qe *QueryEngine) startIndexing(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // Index every 30 seconds
	defer ticker.Stop()
	
	// Initial indexing
	qe.rebuildAllIndexes()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			qe.updateIndexes()
		}
	}
}

// rebuildAllIndexes rebuilds all indexes from scratch
func (qe *QueryEngine) rebuildAllIndexes() {
	qe.mu.Lock()
	defer qe.mu.Unlock()
	
	// Rebuild file index
	qe.rebuildFileIndex()
	
	// Rebuild commit index
	qe.rebuildCommitIndex()
	
	// Rebuild content index
	qe.rebuildContentIndex()
}

// updateIndexes incrementally updates indexes
func (qe *QueryEngine) updateIndexes() {
	qe.mu.RLock()
	lastFileUpdate := qe.fileIndex.lastUpdate
	lastCommitUpdate := qe.commitIndex.lastUpdate
	lastContentUpdate := qe.contentIndex.lastUpdate
	qe.mu.RUnlock()
	
	// Check if updates are needed
	now := time.Now()
	
	// Update file index if needed
	if now.Sub(lastFileUpdate) > time.Minute {
		qe.updateFileIndex()
	}
	
	// Update commit index if needed
	if now.Sub(lastCommitUpdate) > time.Minute {
		qe.updateCommitIndex()
	}
	
	// Update content index if needed  
	if now.Sub(lastContentUpdate) > 5*time.Minute {
		qe.updateContentIndex()
	}
}

// TriggerIndexing triggers immediate indexing (useful after commits)
func (qe *QueryEngine) TriggerIndexing() {
	qe.rebuildAllIndexes()
}

// rebuildFileIndex rebuilds the file index from current repository state
func (qe *QueryEngine) rebuildFileIndex() {
	newIndex := NewFileIndex()
	
	// Get current commit
	refManager := qe.repo.GetRefManager()
	store := qe.repo.GetStore()
	if headHash, err := refManager.GetHEAD(); err == nil && headHash != "" {
		if commit, err := store.GetCommit(headHash); err == nil {
			if tree, err := store.GetTree(commit.TreeHash); err == nil {
				qe.indexTreeFiles(tree, "", newIndex, commit.Hash())
			}
		}
	}
	
	// Also index staging area files
	stagingArea := qe.repo.GetStagingArea()
	if stagingArea != nil {
		stagedFiles, _ := stagingArea.List()
		for _, path := range stagedFiles {
			if _, exists := newIndex.pathIndex[path]; !exists {
				fileHash, err := stagingArea.GetFileHash(path)
				if err != nil {
					continue
				}
				
				entry := &FileEntry{
					Path:         path,
					Hash:         fileHash,
					LastModified: time.Now(),
					CommitHash:   "staging",
				}
				
				// Get file size and content if available
				store := qe.repo.GetStore()
				if blob, err := store.GetBlob(fileHash); err == nil {
					entry.Size = int64(len(blob.Content))
					if len(blob.Content) < 1024*1024 { // Cache small files (< 1MB)
						entry.Content = blob.Content
					}
				}
				
				newIndex.pathIndex[path] = entry
				newIndex.hashIndex[fileHash] = entry
			}
		}
	}
	
	// Replace index atomically
	qe.fileIndex.mu.Lock()
	qe.fileIndex.pathIndex = newIndex.pathIndex
	qe.fileIndex.hashIndex = newIndex.hashIndex
	qe.fileIndex.patternIndex = make(map[string][]*FileEntry) // Clear pattern cache
	qe.fileIndex.lastUpdate = time.Now()
	qe.fileIndex.mu.Unlock()
}

// indexTreeFiles recursively indexes files in a tree
func (qe *QueryEngine) indexTreeFiles(tree *object.Tree, prefix string, index *FileIndex, commitHash string) {
	for _, entry := range tree.Entries {
		fullPath := entry.Name
		if prefix != "" {
			fullPath = prefix + "/" + entry.Name
		}
		
		if entry.Mode == "040000" { // Directory
			store := qe.repo.GetStore()
			if subTree, err := store.GetTree(entry.Hash); err == nil {
				qe.indexTreeFiles(subTree, fullPath, index, commitHash)
			}
		} else { // File
			fileEntry := &FileEntry{
				Path:         fullPath,
				Hash:         entry.Hash,
				LastModified: time.Now(), // We'd need commit timestamp for accurate time
				CommitHash:   commitHash,
			}
			
			// Get file size and possibly cache content
			store := qe.repo.GetStore()
			if blob, err := store.GetBlob(entry.Hash); err == nil {
				fileEntry.Size = int64(len(blob.Content))
				if len(blob.Content) < 1024*1024 { // Cache small files (< 1MB)
					fileEntry.Content = blob.Content
				}
			}
			
			index.pathIndex[fullPath] = fileEntry
			index.hashIndex[entry.Hash] = fileEntry
		}
	}
}

// updateFileIndex incrementally updates the file index
func (qe *QueryEngine) updateFileIndex() {
	// For now, just rebuild - in production, this would be more sophisticated
	qe.rebuildFileIndex()
}

// rebuildCommitIndex rebuilds the commit index from repository history
func (qe *QueryEngine) rebuildCommitIndex() {
	newIndex := NewCommitIndex()
	
	// Get all commits
	commits, err := qe.repo.Log(0) // Get all commits
	if err != nil {
		return
	}
	
	for _, commit := range commits {
		entry := &CommitEntry{
			Hash:         commit.Hash(),
			Author:       commit.Author.Name,
			Email:        commit.Author.Email,
			Message:      commit.Message,
			Timestamp:    commit.Author.Time,
			ParentHashes: []string{commit.ParentHash}, // Convert single parent to slice
			TreeHash:     commit.TreeHash,
			Files:        qe.getCommitFiles(commit),
			Metadata:     make(map[string]string),
		}
		
		// Index by hash
		newIndex.hashIndex[entry.Hash] = entry
		
		// Index by author
		if _, exists := newIndex.authorIndex[entry.Author]; !exists {
			newIndex.authorIndex[entry.Author] = make([]*CommitEntry, 0)
		}
		newIndex.authorIndex[entry.Author] = append(newIndex.authorIndex[entry.Author], entry)
		
		// Index by message keywords
		words := strings.Fields(strings.ToLower(entry.Message))
		for _, word := range words {
			if len(word) >= 3 { // Skip short words
				if _, exists := newIndex.messageIndex[word]; !exists {
					newIndex.messageIndex[word] = make([]*CommitEntry, 0)
				}
				newIndex.messageIndex[word] = append(newIndex.messageIndex[word], entry)
			}
		}
		
		// Add to date index
		newIndex.dateIndex = append(newIndex.dateIndex, entry.Hash)
	}
	
	// Sort date index by timestamp (newest first)
	qe.sortDateIndex(newIndex)
	
	// Replace index atomically
	qe.commitIndex.mu.Lock()
	qe.commitIndex.hashIndex = newIndex.hashIndex
	qe.commitIndex.authorIndex = newIndex.authorIndex
	qe.commitIndex.dateIndex = newIndex.dateIndex
	qe.commitIndex.messageIndex = newIndex.messageIndex
	qe.commitIndex.lastUpdate = time.Now()
	qe.commitIndex.mu.Unlock()
}

// getCommitFiles gets the list of files in a commit
func (qe *QueryEngine) getCommitFiles(commit *object.Commit) []string {
	var files []string
	
	store := qe.repo.GetStore()
	if tree, err := store.GetTree(commit.TreeHash); err == nil {
		qe.collectTreeFiles(tree, "", &files)
	}
	
	return files
}

// collectTreeFiles recursively collects file paths from a tree
func (qe *QueryEngine) collectTreeFiles(tree *object.Tree, prefix string, files *[]string) {
	for _, entry := range tree.Entries {
		fullPath := entry.Name
		if prefix != "" {
			fullPath = prefix + "/" + entry.Name
		}
		
		if entry.Mode == "040000" { // Directory
			store := qe.repo.GetStore()
			if subTree, err := store.GetTree(entry.Hash); err == nil {
				qe.collectTreeFiles(subTree, fullPath, files)
			}
		} else { // File
			*files = append(*files, fullPath)
		}
	}
}

// sortDateIndex sorts the date index by commit timestamp
func (qe *QueryEngine) sortDateIndex(index *CommitIndex) {
	// Sort commit hashes by timestamp (newest first)
	for i := 0; i < len(index.dateIndex)-1; i++ {
		for j := i + 1; j < len(index.dateIndex); j++ {
			commit1 := index.hashIndex[index.dateIndex[i]]
			commit2 := index.hashIndex[index.dateIndex[j]]
			
			if commit1.Timestamp.Before(commit2.Timestamp) {
				// Swap
				index.dateIndex[i], index.dateIndex[j] = index.dateIndex[j], index.dateIndex[i]
			}
		}
	}
}

// updateCommitIndex incrementally updates the commit index
func (qe *QueryEngine) updateCommitIndex() {
	// For now, just rebuild - in production, this would be more sophisticated
	qe.rebuildCommitIndex()
}

// rebuildContentIndex rebuilds the content search index
func (qe *QueryEngine) rebuildContentIndex() {
	newIndex := NewContentIndex()
	
	// Index content from current file index
	qe.fileIndex.mu.RLock()
	defer qe.fileIndex.mu.RUnlock()
	
	for path, entry := range qe.fileIndex.pathIndex {
		var content []byte
		
		// Use cached content if available
		if len(entry.Content) > 0 {
			content = entry.Content
		} else {
			// Load content from store (including delta-compressed objects)
			if blob, err := qe.repo.GetBlobWithDelta(entry.Hash); err == nil {
				content = blob.Content
			}
		}
		
		if len(content) > 0 && qe.isTextFile(path, content) {
			qe.indexFileContent(path, content, newIndex)
		}
	}
	
	// Replace index atomically
	qe.contentIndex.mu.Lock()
	qe.contentIndex.tokenIndex = newIndex.tokenIndex
	qe.contentIndex.invertedIndex = newIndex.invertedIndex
	qe.contentIndex.lastUpdate = time.Now()
	qe.contentIndex.mu.Unlock()
}

// indexFileContent indexes the content of a single file
func (qe *QueryEngine) indexFileContent(path string, content []byte, index *ContentIndex) {
	text := string(content)
	tokens := qe.tokenize(text)
	
	// Count token frequencies
	tokenCount := make(map[string]int)
	for _, token := range tokens {
		tokenCount[token]++
	}
	
	// Update inverted index
	for token, count := range tokenCount {
		if _, exists := index.invertedIndex[token]; !exists {
			index.invertedIndex[token] = make(map[string]int)
		}
		index.invertedIndex[token][path] = count
		
		// Update token index
		if _, exists := index.tokenIndex[token]; !exists {
			index.tokenIndex[token] = make([]string, 0)
		}
		index.tokenIndex[token] = append(index.tokenIndex[token], path)
	}
}

// isTextFile determines if a file is text-based (suitable for content indexing)
func (qe *QueryEngine) isTextFile(path string, content []byte) bool {
	// Check file extension
	textExtensions := []string{
		".txt", ".md", ".json", ".yaml", ".yml", ".xml", ".html", ".css", ".js", ".ts",
		".go", ".py", ".java", ".c", ".cpp", ".h", ".hpp", ".sh", ".sql", ".log",
		".config", ".conf", ".ini", ".properties", ".toml",
	}
	
	pathLower := strings.ToLower(path)
	for _, ext := range textExtensions {
		if strings.HasSuffix(pathLower, ext) {
			return true
		}
	}
	
	// Check content for binary data
	if len(content) == 0 {
		return false
	}
	
	// Simple heuristic: if more than 5% of bytes are null or high-ASCII, consider binary
	nullCount := 0
	for _, b := range content[:minInt(len(content), 1024)] { // Check first 1KB
		if b == 0 || b > 127 {
			nullCount++
		}
	}
	
	return float64(nullCount)/float64(minInt(len(content), 1024)) < 0.05
}

// updateContentIndex incrementally updates the content index
func (qe *QueryEngine) updateContentIndex() {
	// For now, just rebuild - in production, this would be more sophisticated
	qe.rebuildContentIndex()
}

// Cache management methods

// Get retrieves a cached query result
func (qc *QueryCache) Get(key string) (interface{}, bool) {
	qc.mu.RLock()
	defer qc.mu.RUnlock()
	
	entry, exists := qc.cache[key]
	if !exists {
		atomic.AddInt64(&qc.misses, 1)
		return nil, false
	}
	
	// Check if expired
	if time.Since(entry.CreatedAt) > qc.ttl {
		// Don't remove here - let cleanup handle it
		atomic.AddInt64(&qc.misses, 1)
		return nil, false
	}
	
	// Update access statistics
	entry.LastAccess = time.Now()
	atomic.AddInt64(&entry.AccessCount, 1)
	atomic.AddInt64(&qc.hits, 1)
	
	return entry.Result, true
}

// Set stores a query result in the cache
func (qc *QueryCache) Set(key string, result interface{}) {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	
	// Check if we need to evict entries
	if len(qc.cache) >= qc.maxSize {
		qc.evictLRU()
	}
	
	now := time.Now()
	qc.cache[key] = &CacheEntry{
		Result:      result,
		CreatedAt:   now,
		LastAccess:  now,
		AccessCount: 0,
	}
}

// evictLRU evicts the least recently used cache entry
func (qc *QueryCache) evictLRU() {
	var oldestKey string
	var oldestTime time.Time
	
	for key, entry := range qc.cache {
		if oldestKey == "" || entry.LastAccess.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.LastAccess
		}
	}
	
	if oldestKey != "" {
		delete(qc.cache, oldestKey)
	}
}

// startCleanup starts the cache cleanup goroutine
func (qc *QueryCache) startCleanup() {
	ticker := time.NewTicker(qc.ttl / 2) // Cleanup twice per TTL period
	defer ticker.Stop()
	
	for range ticker.C {
		qc.cleanup()
	}
}

// cleanup removes expired cache entries
func (qc *QueryCache) cleanup() {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	
	now := time.Now()
	for key, entry := range qc.cache {
		if now.Sub(entry.CreatedAt) > qc.ttl {
			delete(qc.cache, key)
		}
	}
}

// GetStats returns cache statistics
func (qc *QueryCache) GetStats() CacheStats {
	qc.mu.RLock()
	defer qc.mu.RUnlock()
	
	hits := atomic.LoadInt64(&qc.hits)
	misses := atomic.LoadInt64(&qc.misses)
	total := hits + misses
	
	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}
	
	return CacheStats{
		Size:    len(qc.cache),
		MaxSize: qc.maxSize,
		Hits:    hits,
		Misses:  misses,
		HitRate: hitRate,
	}
}

// CacheStats represents cache statistics
type CacheStats struct {
	Size    int     `json:"size"`
	MaxSize int     `json:"max_size"`
	Hits    int64   `json:"hits"`
	Misses  int64   `json:"misses"`
	HitRate float64 `json:"hit_rate"`
}

// GetQueryStats returns query engine statistics
func (qe *QueryEngine) GetQueryStats() QueryEngineStats {
	qe.mu.RLock()
	defer qe.mu.RUnlock()
	
	cacheStats := qe.queryCache.GetStats()
	
	return QueryEngineStats{
		TotalQueries:     qe.totalQueries,
		CacheHits:        qe.cacheHits,
		IndexHits:        qe.indexHits,
		AverageQueryTime: qe.avgQueryTime,
		CacheStats:       cacheStats,
		FileIndexSize:    len(qe.fileIndex.pathIndex),
		CommitIndexSize:  len(qe.commitIndex.hashIndex),
		ContentIndexSize: len(qe.contentIndex.tokenIndex),
	}
}

// QueryEngineStats represents query engine statistics
type QueryEngineStats struct {
	TotalQueries     int64         `json:"total_queries"`
	CacheHits        int64         `json:"cache_hits"`
	IndexHits        int64         `json:"index_hits"`
	AverageQueryTime time.Duration `json:"average_query_time"`
	CacheStats       CacheStats    `json:"cache_stats"`
	FileIndexSize    int           `json:"file_index_size"`
	CommitIndexSize  int           `json:"commit_index_size"`
	ContentIndexSize int           `json:"content_index_size"`
}

// Helper function
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}