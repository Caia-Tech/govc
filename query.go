package govc

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// QueryEngine provides efficient data access without full repository scans
// This is critical for high-frequency applications that need instant lookups
type QueryEngine struct {
	repo          *Repository
	fileIndex     *FileIndex
	commitIndex   *CommitIndex
	contentIndex  *ContentIndex
	queryCache    *QueryCache
	mu            sync.RWMutex
	
	// Performance metrics
	totalQueries    int64
	cacheHits       int64
	indexHits       int64
	avgQueryTime    time.Duration
}

// FileIndex maintains efficient lookups for file operations
type FileIndex struct {
	pathIndex     map[string]*FileEntry       // path -> file entry
	patternIndex  map[string][]*FileEntry     // pattern -> matching files
	hashIndex     map[string]*FileEntry       // hash -> file entry
	mu            sync.RWMutex
	lastUpdate    time.Time
}

// FileEntry represents a file in the index
type FileEntry struct {
	Path         string    `json:"path"`
	Hash         string    `json:"hash"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	CommitHash   string    `json:"commit_hash"`
	Content      []byte    `json:"-"` // Cached content
}

// CommitIndex maintains efficient lookups for commit operations
type CommitIndex struct {
	hashIndex     map[string]*CommitEntry     // hash -> commit entry
	authorIndex   map[string][]*CommitEntry   // author -> commits
	dateIndex     []string                    // sorted commit hashes by date
	messageIndex  map[string][]*CommitEntry   // message keywords -> commits
	mu            sync.RWMutex
	lastUpdate    time.Time
}

// CommitEntry represents a commit in the index
type CommitEntry struct {
	Hash         string            `json:"hash"`
	Author       string            `json:"author"`
	Email        string            `json:"email"`
	Message      string            `json:"message"`
	Timestamp    time.Time         `json:"timestamp"`
	ParentHashes []string          `json:"parent_hashes"`
	TreeHash     string            `json:"tree_hash"`
	Files        []string          `json:"files"`
	Metadata     map[string]string `json:"metadata"`
}

// ContentIndex provides content-based search capabilities
type ContentIndex struct {
	tokenIndex    map[string][]string         // token -> file paths containing it
	invertedIndex map[string]map[string]int   // token -> (file_path -> occurrence_count)
	stemmer       *PorterStemmer
	mu            sync.RWMutex
	lastUpdate    time.Time
}

// QueryCache provides caching layer for query optimization
type QueryCache struct {
	cache         map[string]*CacheEntry
	maxSize       int
	ttl           time.Duration
	hits          int64
	misses        int64
	mu            sync.RWMutex
}

// CacheEntry represents a cached query result
type CacheEntry struct {
	Result    interface{} `json:"result"`
	CreatedAt time.Time   `json:"created_at"`
	AccessCount int64     `json:"access_count"`
	LastAccess  time.Time `json:"last_access"`
}

// QueryType represents different types of queries
type QueryType int

const (
	QueryByPath QueryType = iota
	QueryByPattern
	QueryByContent
	QueryByCommit
	QueryByAuthor
	QueryByDate
	QueryByHash
)

// QueryRequest represents a query request
type QueryRequest struct {
	Type      QueryType                `json:"type"`
	Query     string                   `json:"query"`
	Filters   map[string]interface{}   `json:"filters"`
	Limit     int                      `json:"limit"`
	Offset    int                      `json:"offset"`
	SortBy    string                   `json:"sort_by"`
	SortOrder string                   `json:"sort_order"` // "asc" or "desc"
}

// QueryResult represents query results
type QueryResult struct {
	Files         []*FileEntry   `json:"files,omitempty"`
	Commits       []*CommitEntry `json:"commits,omitempty"`
	Total         int            `json:"total"`
	QueryTime     time.Duration  `json:"query_time"`
	CacheHit      bool           `json:"cache_hit"`
	IndexUsed     bool           `json:"index_used"`
}

// PorterStemmer provides basic word stemming for content search
type PorterStemmer struct {
	// Simple implementation for basic stemming
}

// NewQueryEngine creates a new query engine
func NewQueryEngine(repo *Repository) *QueryEngine {
	qe := &QueryEngine{
		repo:         repo,
		fileIndex:    NewFileIndex(),
		commitIndex:  NewCommitIndex(),
		contentIndex: NewContentIndex(),
		queryCache:   NewQueryCache(1000, 10*time.Minute), // 1000 entries, 10min TTL
	}
	
	// Do initial indexing if repository already has content
	if hasContent, _ := qe.repositoryHasContent(); hasContent {
		qe.rebuildAllIndexes()
	}
	
	// Start background indexing for updates
	go qe.startIndexing(context.Background())
	
	return qe
}

// NewFileIndex creates a new file index
func NewFileIndex() *FileIndex {
	return &FileIndex{
		pathIndex:    make(map[string]*FileEntry),
		patternIndex: make(map[string][]*FileEntry),
		hashIndex:    make(map[string]*FileEntry),
		lastUpdate:   time.Now(),
	}
}

// NewCommitIndex creates a new commit index
func NewCommitIndex() *CommitIndex {
	return &CommitIndex{
		hashIndex:    make(map[string]*CommitEntry),
		authorIndex:  make(map[string][]*CommitEntry),
		dateIndex:    make([]string, 0),
		messageIndex: make(map[string][]*CommitEntry),
		lastUpdate:   time.Now(),
	}
}

// NewContentIndex creates a new content index
func NewContentIndex() *ContentIndex {
	return &ContentIndex{
		tokenIndex:    make(map[string][]string),
		invertedIndex: make(map[string]map[string]int),
		stemmer:       &PorterStemmer{},
		lastUpdate:    time.Now(),
	}
}

// NewQueryCache creates a new query cache
func NewQueryCache(maxSize int, ttl time.Duration) *QueryCache {
	cache := &QueryCache{
		cache:   make(map[string]*CacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
	
	// Start cleanup goroutine
	go cache.startCleanup()
	
	return cache
}

// Query executes a query and returns results
func (qe *QueryEngine) Query(req *QueryRequest) (*QueryResult, error) {
	start := time.Now()
	
	// Generate cache key
	cacheKey := qe.generateCacheKey(req)
	
	// Always increment total queries counter
	qe.totalQueries++
	
	// Check cache first
	if cached, hit := qe.queryCache.Get(cacheKey); hit {
		result := cached.(*QueryResult)
		result.QueryTime = time.Since(start)
		result.CacheHit = true
		qe.cacheHits++
		qe.updateAvgQueryTime(result.QueryTime)
		return result, nil
	}
	
	// Execute query
	var result *QueryResult
	var err error
	
	switch req.Type {
	case QueryByPath:
		result, err = qe.queryByPath(req)
	case QueryByPattern:
		result, err = qe.queryByPattern(req)
	case QueryByContent:
		result, err = qe.queryByContent(req)
	case QueryByCommit:
		result, err = qe.queryByCommit(req)
	case QueryByAuthor:
		result, err = qe.queryByAuthor(req)
	case QueryByDate:
		result, err = qe.queryByDate(req)
	case QueryByHash:
		result, err = qe.queryByHash(req)
	default:
		return nil, fmt.Errorf("unsupported query type: %d", req.Type)
	}
	
	if err != nil {
		return nil, err
	}
	
	// Apply sorting and pagination
	qe.applySortingAndPagination(result, req)
	
	result.QueryTime = time.Since(start)
	result.CacheHit = false
	
	// Cache the result
	qe.queryCache.Set(cacheKey, result)
	
	qe.updateAvgQueryTime(result.QueryTime)
	
	return result, nil
}

// queryByPath queries files by exact path
func (qe *QueryEngine) queryByPath(req *QueryRequest) (*QueryResult, error) {
	qe.fileIndex.mu.RLock()
	defer qe.fileIndex.mu.RUnlock()
	
	result := &QueryResult{
		Files:     make([]*FileEntry, 0),
		IndexUsed: true,
	}
	
	if entry, exists := qe.fileIndex.pathIndex[req.Query]; exists {
		result.Files = append(result.Files, entry)
		result.Total = 1
	}
	
	return result, nil
}

// queryByPattern queries files by glob pattern
func (qe *QueryEngine) queryByPattern(req *QueryRequest) (*QueryResult, error) {
	qe.fileIndex.mu.RLock()
	defer qe.fileIndex.mu.RUnlock()
	
	result := &QueryResult{
		Files:     make([]*FileEntry, 0),
		IndexUsed: true,
	}
	
	// Check if pattern is cached
	if entries, cached := qe.fileIndex.patternIndex[req.Query]; cached {
		result.Files = entries
		result.Total = len(entries)
		return result, nil
	}
	
	// Try different pattern matching strategies
	for path, entry := range qe.fileIndex.pathIndex {
		matched := false
		
		// Strategy 1: Full path match
		if m, _ := filepath.Match(req.Query, path); m {
			matched = true
		}
		
		// Strategy 2: Basename match for simple patterns (e.g., "*.js")
		if !matched && !strings.Contains(req.Query, "/") {
			if m, _ := filepath.Match(req.Query, filepath.Base(path)); m {
				matched = true
			}
		}
		
		// Strategy 3: Directory pattern match (e.g., "backend/*", "src/**")
		if !matched && strings.Contains(req.Query, "/") {
			// Handle "dir/*" patterns
			if strings.HasSuffix(req.Query, "/*") {
				prefix := strings.TrimSuffix(req.Query, "/*")
				if strings.HasPrefix(path, prefix+"/") {
					matched = true
				}
			} else {
				// Handle other directory patterns
				if m, _ := filepath.Match(req.Query, path); m {
					matched = true
				}
			}
		}
		
		if matched {
			result.Files = append(result.Files, entry)
		}
	}
	
	result.Total = len(result.Files)
	
	// Cache frequently used patterns (need write lock for this)
	if result.Total > 0 && result.Total < 100 { // Don't cache huge result sets
		qe.fileIndex.mu.RUnlock()
		qe.fileIndex.mu.Lock()
		qe.fileIndex.patternIndex[req.Query] = result.Files
		qe.fileIndex.mu.Unlock()
		qe.fileIndex.mu.RLock()
	}
	
	return result, nil
}

// queryByContent searches files by content
func (qe *QueryEngine) queryByContent(req *QueryRequest) (*QueryResult, error) {
	qe.contentIndex.mu.RLock()
	defer qe.contentIndex.mu.RUnlock()
	
	result := &QueryResult{
		Files:     make([]*FileEntry, 0),
		IndexUsed: true,
	}
	
	// Tokenize search query
	tokens := qe.tokenize(req.Query)
	if len(tokens) == 0 {
		return result, nil
	}
	
	// Find files containing all tokens (AND operation)
	candidateFiles := make(map[string]int)
	
	for i, token := range tokens {
		if fileCounts, exists := qe.contentIndex.invertedIndex[token]; exists {
			if i == 0 {
				// First token - initialize candidates
				for filePath, count := range fileCounts {
					candidateFiles[filePath] = count
				}
			} else {
				// Subsequent tokens - intersection
				newCandidates := make(map[string]int)
				for filePath, count := range fileCounts {
					if _, exists := candidateFiles[filePath]; exists {
						newCandidates[filePath] = candidateFiles[filePath] + count
					}
				}
				candidateFiles = newCandidates
			}
		} else {
			// Token not found - no results
			return result, nil
		}
	}
	
	// Convert to file entries and sort by relevance (token count)
	type scoredFile struct {
		entry *FileEntry
		score int
	}
	
	scoredFiles := make([]scoredFile, 0, len(candidateFiles))
	qe.fileIndex.mu.RLock()
	for filePath, score := range candidateFiles {
		if entry, exists := qe.fileIndex.pathIndex[filePath]; exists {
			scoredFiles = append(scoredFiles, scoredFile{entry, score})
		}
	}
	qe.fileIndex.mu.RUnlock()
	
	// Sort by relevance score
	sort.Slice(scoredFiles, func(i, j int) bool {
		return scoredFiles[i].score > scoredFiles[j].score
	})
	
	// Extract file entries
	for _, sf := range scoredFiles {
		result.Files = append(result.Files, sf.entry)
	}
	
	result.Total = len(result.Files)
	return result, nil
}

// queryByCommit queries commits by criteria
func (qe *QueryEngine) queryByCommit(req *QueryRequest) (*QueryResult, error) {
	qe.commitIndex.mu.RLock()
	defer qe.commitIndex.mu.RUnlock()
	
	result := &QueryResult{
		Commits:   make([]*CommitEntry, 0),
		IndexUsed: true,
	}
	
	// Search in commit messages with deduplication
	query := strings.ToLower(req.Query)
	seen := make(map[string]bool) // Track seen commit hashes to avoid duplicates
	
	// First, try to find exact word matches in the message index
	if entries, exists := qe.commitIndex.messageIndex[query]; exists {
		for _, entry := range entries {
			if !seen[entry.Hash] {
				result.Commits = append(result.Commits, entry)
				seen[entry.Hash] = true
			}
		}
	}
	
	// If no exact matches, fall back to substring search
	if len(result.Commits) == 0 {
		for _, entry := range qe.commitIndex.hashIndex {
			if strings.Contains(strings.ToLower(entry.Message), query) && !seen[entry.Hash] {
				result.Commits = append(result.Commits, entry)
				seen[entry.Hash] = true
			}
		}
	}
	
	result.Total = len(result.Commits)
	return result, nil
}

// queryByAuthor queries commits by author
func (qe *QueryEngine) queryByAuthor(req *QueryRequest) (*QueryResult, error) {
	qe.commitIndex.mu.RLock()
	defer qe.commitIndex.mu.RUnlock()
	
	result := &QueryResult{
		Commits:   make([]*CommitEntry, 0),
		IndexUsed: true,
	}
	
	// Exact match or pattern match for author
	for author, entries := range qe.commitIndex.authorIndex {
		if strings.Contains(strings.ToLower(author), strings.ToLower(req.Query)) {
			result.Commits = append(result.Commits, entries...)
		}
	}
	
	result.Total = len(result.Commits)
	return result, nil
}

// queryByDate queries commits by date range
func (qe *QueryEngine) queryByDate(req *QueryRequest) (*QueryResult, error) {
	qe.commitIndex.mu.RLock()
	defer qe.commitIndex.mu.RUnlock()
	
	result := &QueryResult{
		Commits:   make([]*CommitEntry, 0),
		IndexUsed: true,
	}
	
	// Parse date criteria from filters
	var startDate, endDate time.Time
	var err error
	
	if start, ok := req.Filters["start_date"]; ok {
		startDate, err = time.Parse(time.RFC3339, start.(string))
		if err != nil {
			return nil, fmt.Errorf("invalid start_date format: %w", err)
		}
	}
	
	if end, ok := req.Filters["end_date"]; ok {
		endDate, err = time.Parse(time.RFC3339, end.(string))
		if err != nil {
			return nil, fmt.Errorf("invalid end_date format: %w", err)
		}
	}
	
	// Use sorted date index for efficient range queries
	for _, commitHash := range qe.commitIndex.dateIndex {
		if entry, exists := qe.commitIndex.hashIndex[commitHash]; exists {
			if (!startDate.IsZero() && entry.Timestamp.Before(startDate)) ||
			   (!endDate.IsZero() && entry.Timestamp.After(endDate)) {
				continue
			}
			result.Commits = append(result.Commits, entry)
		}
	}
	
	result.Total = len(result.Commits)
	return result, nil
}

// queryByHash queries by commit or file hash
func (qe *QueryEngine) queryByHash(req *QueryRequest) (*QueryResult, error) {
	result := &QueryResult{
		Files:     make([]*FileEntry, 0),
		Commits:   make([]*CommitEntry, 0),
		IndexUsed: true,
	}
	
	// Check file hash index
	qe.fileIndex.mu.RLock()
	if entry, exists := qe.fileIndex.hashIndex[req.Query]; exists {
		result.Files = append(result.Files, entry)
	}
	qe.fileIndex.mu.RUnlock()
	
	// Check commit hash index
	qe.commitIndex.mu.RLock()
	if entry, exists := qe.commitIndex.hashIndex[req.Query]; exists {
		result.Commits = append(result.Commits, entry)
	}
	qe.commitIndex.mu.RUnlock()
	
	result.Total = len(result.Files) + len(result.Commits)
	return result, nil
}

// applySortingAndPagination applies sorting and pagination to query results
func (qe *QueryEngine) applySortingAndPagination(result *QueryResult, req *QueryRequest) {
	// Apply sorting
	if req.SortBy != "" {
		qe.sortResults(result, req.SortBy, req.SortOrder)
	}
	
	// Apply pagination
	if req.Limit > 0 {
		start := req.Offset
		end := start + req.Limit
		
		if len(result.Files) > 0 {
			if start >= len(result.Files) {
				result.Files = []*FileEntry{}
			} else {
				if end > len(result.Files) {
					end = len(result.Files)
				}
				result.Files = result.Files[start:end]
			}
		}
		
		if len(result.Commits) > 0 {
			if start >= len(result.Commits) {
				result.Commits = []*CommitEntry{}
			} else {
				if end > len(result.Commits) {
					end = len(result.Commits)
				}
				result.Commits = result.Commits[start:end]
			}
		}
	}
}

// sortResults sorts query results
func (qe *QueryEngine) sortResults(result *QueryResult, sortBy, sortOrder string) {
	ascending := sortOrder != "desc"
	
	// Sort files
	if len(result.Files) > 0 {
		switch sortBy {
		case "path":
			sort.Slice(result.Files, func(i, j int) bool {
				if ascending {
					return result.Files[i].Path < result.Files[j].Path
				}
				return result.Files[i].Path > result.Files[j].Path
			})
		case "size":
			sort.Slice(result.Files, func(i, j int) bool {
				if ascending {
					return result.Files[i].Size < result.Files[j].Size
				}
				return result.Files[i].Size > result.Files[j].Size
			})
		case "modified":
			sort.Slice(result.Files, func(i, j int) bool {
				if ascending {
					return result.Files[i].LastModified.Before(result.Files[j].LastModified)
				}
				return result.Files[i].LastModified.After(result.Files[j].LastModified)
			})
		}
	}
	
	// Sort commits
	if len(result.Commits) > 0 {
		switch sortBy {
		case "date", "timestamp":
			sort.Slice(result.Commits, func(i, j int) bool {
				if ascending {
					return result.Commits[i].Timestamp.Before(result.Commits[j].Timestamp)
				}
				return result.Commits[i].Timestamp.After(result.Commits[j].Timestamp)
			})
		case "author":
			sort.Slice(result.Commits, func(i, j int) bool {
				if ascending {
					return result.Commits[i].Author < result.Commits[j].Author
				}
				return result.Commits[i].Author > result.Commits[j].Author
			})
		case "message":
			sort.Slice(result.Commits, func(i, j int) bool {
				if ascending {
					return result.Commits[i].Message < result.Commits[j].Message
				}
				return result.Commits[i].Message > result.Commits[j].Message
			})
		}
	}
}

// tokenize breaks text into searchable tokens
func (qe *QueryEngine) tokenize(text string) []string {
	// Simple tokenization - split by whitespace and punctuation
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	
	// Remove short words and stem
	tokens := make([]string, 0, len(words))
	for _, word := range words {
		if len(word) >= 3 { // Ignore very short words
			tokens = append(tokens, qe.contentIndex.stemmer.stem(word))
		}
	}
	
	return tokens
}

// repositoryHasContent checks if the repository has any commits
func (qe *QueryEngine) repositoryHasContent() (bool, error) {
	if qe.repo == nil {
		return false, nil
	}
	
	// Check if there's a HEAD commit
	headHash, err := qe.repo.refManager.GetHEAD()
	if err != nil || headHash == "" {
		return false, nil
	}
	
	// Check if the commit exists
	_, err = qe.repo.store.GetCommit(headHash)
	return err == nil, nil
}

// stem provides basic word stemming
func (ps *PorterStemmer) stem(word string) string {
	// Very basic stemming - just remove common suffixes
	word = strings.TrimSuffix(word, "ing")
	word = strings.TrimSuffix(word, "ed")
	word = strings.TrimSuffix(word, "er")
	word = strings.TrimSuffix(word, "est")
	word = strings.TrimSuffix(word, "ly")
	word = strings.TrimSuffix(word, "s")
	return word
}

// generateCacheKey generates a cache key for a query request
func (qe *QueryEngine) generateCacheKey(req *QueryRequest) string {
	// Simple cache key generation
	key := fmt.Sprintf("%d:%s:%d:%d:%s:%s", 
		req.Type, req.Query, req.Limit, req.Offset, req.SortBy, req.SortOrder)
	
	// Add filters to key
	for k, v := range req.Filters {
		key += fmt.Sprintf(":%s=%v", k, v)
	}
	
	return key
}

// updateAvgQueryTime updates the average query time
func (qe *QueryEngine) updateAvgQueryTime(queryTime time.Duration) {
	if qe.totalQueries == 1 {
		qe.avgQueryTime = queryTime
	} else {
		// Exponential moving average
		alpha := 0.1
		qe.avgQueryTime = time.Duration(float64(qe.avgQueryTime)*(1-alpha) + float64(queryTime)*alpha)
	}
}