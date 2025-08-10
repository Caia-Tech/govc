package govc

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// AdvancedSearch provides sophisticated search capabilities beyond basic queries
type AdvancedSearch struct {
	repo             *Repository
	fullTextIndex    *FullTextIndex
	searchSubscriber *SearchSubscriber
	queryParser      *QueryParser
	aggregator       *SearchAggregator
	mu               sync.RWMutex
}

// FullTextIndex provides advanced text search with scoring
type FullTextIndex struct {
	documentIndex    map[string]*SearchDocument // file_path -> document
	tokenIndex       map[string][]string        // token -> file paths
	termFrequency    map[string]map[string]int  // token -> (file_path -> frequency)
	documentFreq     map[string]int             // token -> number of documents containing it
	totalDocuments   int
	indexLastUpdated time.Time
	mu               sync.RWMutex
}

// SearchDocument represents a document in the full-text index
type SearchDocument struct {
	Path         string            `json:"path"`
	Content      string            `json:"content"`
	TokenCount   int               `json:"token_count"`
	Tokens       map[string]int    `json:"tokens"`        // token -> frequency
	Metadata     map[string]string `json:"metadata"`
	LastModified time.Time         `json:"last_modified"`
	Size         int64             `json:"size"`
}

// SearchResult represents a scored search result
type SearchResult struct {
	Document    *SearchDocument `json:"document"`
	Score       float64         `json:"score"`
	Highlights  []string        `json:"highlights"`  // Text snippets with matches
	MatchReason string          `json:"match_reason"` // Why this matched
}

// FullTextSearchRequest represents a full-text search request
type FullTextSearchRequest struct {
	Query           string            `json:"query"`
	FileTypes       []string          `json:"file_types,omitempty"`       // e.g., ["go", "js", "md"]
	MaxSize         int64             `json:"max_size,omitempty"`         // Maximum file size
	MinScore        float64           `json:"min_score,omitempty"`        // Minimum relevance score
	IncludeContent  bool              `json:"include_content,omitempty"`  // Include full content in results
	HighlightLength int               `json:"highlight_length,omitempty"` // Length of highlight snippets
	Limit           int               `json:"limit,omitempty"`
	Offset          int               `json:"offset,omitempty"`
	SortBy          string            `json:"sort_by,omitempty"` // "score", "date", "size"
}

// FullTextSearchResponse represents search results with scoring
type FullTextSearchResponse struct {
	Results    []SearchResult    `json:"results"`
	Total      int               `json:"total"`
	QueryTime  time.Duration     `json:"query_time"`
	Statistics SearchStatistics  `json:"statistics"`
}

// SearchStatistics provides search performance metrics
type SearchStatistics struct {
	DocumentsScanned  int           `json:"documents_scanned"`
	TokensMatched     int           `json:"tokens_matched"`
	AvgScore          float64       `json:"avg_score"`
	MaxScore          float64       `json:"max_score"`
	IndexSize         int           `json:"index_size"`
	IndexLastUpdated  time.Time     `json:"index_last_updated"`
	ProcessingTime    time.Duration `json:"processing_time"`
}

// NewAdvancedSearch creates a new advanced search system
func NewAdvancedSearch(repo *Repository) *AdvancedSearch {
	as := &AdvancedSearch{
		repo:             repo,
		fullTextIndex:    NewFullTextIndex(),
		searchSubscriber: NewSearchSubscriber(),
		queryParser:      NewQueryParser(),
		aggregator:       NewSearchAggregator(),
	}
	
	// Build initial index from existing files
	go as.buildInitialIndex()
	
	return as
}

// NewFullTextIndex creates a new full-text search index
func NewFullTextIndex() *FullTextIndex {
	return &FullTextIndex{
		documentIndex:    make(map[string]*SearchDocument),
		tokenIndex:       make(map[string][]string),
		termFrequency:    make(map[string]map[string]int),
		documentFreq:     make(map[string]int),
		indexLastUpdated: time.Now(),
	}
}

// FullTextSearch performs advanced full-text search with relevance scoring
func (as *AdvancedSearch) FullTextSearch(req *FullTextSearchRequest) (*FullTextSearchResponse, error) {
	start := time.Now()
	
	as.fullTextIndex.mu.RLock()
	defer as.fullTextIndex.mu.RUnlock()
	
	// Handle special case for "*" (match all documents)
	if req.Query == "*" {
		return as.matchAllDocuments(req, start)
	}
	
	// Tokenize and process query
	queryTokens := as.tokenizeQuery(req.Query)
	if len(queryTokens) == 0 {
		return &FullTextSearchResponse{
			Results:   []SearchResult{},
			Total:     0,
			QueryTime: time.Since(start),
		}, nil
	}
	
	// Find candidate documents
	candidates := as.findCandidateDocuments(queryTokens)
	
	// Score documents
	var results []SearchResult
	var totalScore float64
	var maxScore float64
	tokensMatched := 0
	
	for _, doc := range candidates {
		score := as.calculateTFIDF(doc, queryTokens)
		
		// Apply filters
		if req.MinScore > 0 && score < req.MinScore {
			continue
		}
		if req.MaxSize > 0 && doc.Size > req.MaxSize {
			continue
		}
		if len(req.FileTypes) > 0 && !as.matchesFileType(doc.Path, req.FileTypes) {
			continue
		}
		
		highlights := as.generateHighlights(doc.Content, queryTokens, req.HighlightLength)
		matchReason := as.generateMatchReason(doc, queryTokens, score)
		
		result := SearchResult{
			Document:    doc,
			Score:       score,
			Highlights:  highlights,
			MatchReason: matchReason,
		}
		
		// Remove content if not requested to save bandwidth
		if !req.IncludeContent {
			result.Document.Content = ""
		}
		
		results = append(results, result)
		totalScore += score
		if score > maxScore {
			maxScore = score
		}
		tokensMatched += len(as.getMatchingTokens(doc, queryTokens))
	}
	
	// Sort results by score (highest first) or other criteria
	as.sortResults(results, req.SortBy)
	
	// Apply pagination
	total := len(results)
	if req.Limit > 0 {
		start := req.Offset
		end := start + req.Limit
		if start >= len(results) {
			results = []SearchResult{}
		} else {
			if end > len(results) {
				end = len(results)
			}
			results = results[start:end]
		}
	}
	
	avgScore := 0.0
	if len(results) > 0 {
		avgScore = totalScore / float64(len(results))
	}
	
	return &FullTextSearchResponse{
		Results:   results,
		Total:     total,
		QueryTime: time.Since(start),
		Statistics: SearchStatistics{
			DocumentsScanned: len(candidates),
			TokensMatched:    tokensMatched,
			AvgScore:         avgScore,
			MaxScore:         maxScore,
			IndexSize:        as.fullTextIndex.totalDocuments,
			IndexLastUpdated: as.fullTextIndex.indexLastUpdated,
			ProcessingTime:   time.Since(start),
		},
	}, nil
}

// calculateTFIDF calculates the TF-IDF score for a document given query tokens
func (as *AdvancedSearch) calculateTFIDF(doc *SearchDocument, queryTokens []string) float64 {
	if doc.TokenCount == 0 {
		return 0.0
	}
	
	score := 0.0
	for _, token := range queryTokens {
		// Term Frequency (TF)
		tf := float64(doc.Tokens[token]) / float64(doc.TokenCount)
		
		// Inverse Document Frequency (IDF)
		df := as.fullTextIndex.documentFreq[token]
		if df == 0 {
			continue // Token not found
		}
		idf := math.Log(float64(as.fullTextIndex.totalDocuments) / float64(df))
		
		// TF-IDF Score
		tfidf := tf * idf
		score += tfidf
	}
	
	// Normalize by query length
	return score / float64(len(queryTokens))
}

// findCandidateDocuments finds documents that contain at least one query token
func (as *AdvancedSearch) findCandidateDocuments(queryTokens []string) []*SearchDocument {
	candidateSet := make(map[string]*SearchDocument)
	
	for _, token := range queryTokens {
		if filePaths, exists := as.fullTextIndex.tokenIndex[token]; exists {
			for _, path := range filePaths {
				if doc, exists := as.fullTextIndex.documentIndex[path]; exists {
					candidateSet[path] = doc
				}
			}
		}
	}
	
	// Convert to slice
	candidates := make([]*SearchDocument, 0, len(candidateSet))
	for _, doc := range candidateSet {
		candidates = append(candidates, doc)
	}
	
	return candidates
}

// tokenizeQuery tokenizes the search query
func (as *AdvancedSearch) tokenizeQuery(query string) []string {
	// Convert to lowercase and split on whitespace and punctuation
	query = strings.ToLower(query)
	words := regexp.MustCompile(`[^\w]+`).Split(query, -1)
	
	var tokens []string
	for _, word := range words {
		word = strings.TrimSpace(word)
		if len(word) >= 3 { // Ignore very short words
			tokens = append(tokens, word)
		}
	}
	
	return tokens
}

// generateHighlights creates highlighted snippets showing matches
func (as *AdvancedSearch) generateHighlights(content string, queryTokens []string, maxLength int) []string {
	if maxLength == 0 {
		maxLength = 200 // Default highlight length
	}
	
	var highlights []string
	content = strings.ToLower(content)
	
	for _, token := range queryTokens {
		index := strings.Index(content, token)
		if index == -1 {
			continue
		}
		
		// Create snippet around the match
		start := index - 50
		if start < 0 {
			start = 0
		}
		end := index + len(token) + 50
		if end > len(content) {
			end = len(content)
		}
		
		snippet := content[start:end]
		
		// Highlight the matching term
		snippet = strings.ReplaceAll(snippet, token, fmt.Sprintf("**%s**", token))
		
		if len(snippet) > maxLength {
			snippet = snippet[:maxLength] + "..."
		}
		
		highlights = append(highlights, snippet)
		
		// Limit number of highlights per document
		if len(highlights) >= 3 {
			break
		}
	}
	
	return highlights
}

// generateMatchReason explains why a document matched
func (as *AdvancedSearch) generateMatchReason(doc *SearchDocument, queryTokens []string, score float64) string {
	matchingTokens := as.getMatchingTokens(doc, queryTokens)
	
	if len(matchingTokens) == 1 {
		return fmt.Sprintf("Contains '%s' (score: %.2f)", matchingTokens[0], score)
	}
	
	return fmt.Sprintf("Contains %d terms: %s (score: %.2f)", 
		len(matchingTokens), 
		strings.Join(matchingTokens, ", "), 
		score)
}

// getMatchingTokens returns tokens that match in the document
func (as *AdvancedSearch) getMatchingTokens(doc *SearchDocument, queryTokens []string) []string {
	var matching []string
	for _, token := range queryTokens {
		if doc.Tokens[token] > 0 {
			matching = append(matching, token)
		}
	}
	return matching
}

// matchesFileType checks if a file path matches the requested file types
func (as *AdvancedSearch) matchesFileType(path string, fileTypes []string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != "" && ext[0] == '.' {
		ext = ext[1:] // Remove the dot
	}
	
	for _, ft := range fileTypes {
		if ext == strings.ToLower(ft) {
			return true
		}
	}
	return false
}

// sortResults sorts search results by the specified criteria
func (as *AdvancedSearch) sortResults(results []SearchResult, sortBy string) {
	switch strings.ToLower(sortBy) {
	case "date":
		sort.Slice(results, func(i, j int) bool {
			return results[i].Document.LastModified.After(results[j].Document.LastModified)
		})
	case "size":
		sort.Slice(results, func(i, j int) bool {
			return results[i].Document.Size > results[j].Document.Size
		})
	case "path":
		sort.Slice(results, func(i, j int) bool {
			return results[i].Document.Path < results[j].Document.Path
		})
	default: // "score" or empty
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})
	}
}

// IndexDocument adds or updates a document in the full-text index
func (as *AdvancedSearch) IndexDocument(path string, content []byte) error {
	as.fullTextIndex.mu.Lock()
	defer as.fullTextIndex.mu.Unlock()
	
	// Remove old document if it exists
	if oldDoc, exists := as.fullTextIndex.documentIndex[path]; exists {
		as.removeDocumentFromIndex(path, oldDoc)
	}
	
	// Create new document
	doc := &SearchDocument{
		Path:         path,
		Content:      string(content),
		Tokens:       make(map[string]int),
		LastModified: time.Now(),
		Size:         int64(len(content)),
		Metadata:     make(map[string]string),
	}
	
	// Tokenize content
	tokens := as.tokenizeContent(string(content))
	doc.TokenCount = len(tokens)
	
	// Count token frequencies
	for _, token := range tokens {
		doc.Tokens[token]++
	}
	
	// Update indexes
	as.fullTextIndex.documentIndex[path] = doc
	
	for token := range doc.Tokens {
		// Update token index
		if _, exists := as.fullTextIndex.tokenIndex[token]; !exists {
			as.fullTextIndex.tokenIndex[token] = []string{}
		}
		as.fullTextIndex.tokenIndex[token] = append(as.fullTextIndex.tokenIndex[token], path)
		
		// Update term frequency
		if _, exists := as.fullTextIndex.termFrequency[token]; !exists {
			as.fullTextIndex.termFrequency[token] = make(map[string]int)
		}
		as.fullTextIndex.termFrequency[token][path] = doc.Tokens[token]
		
		// Update document frequency
		as.fullTextIndex.documentFreq[token]++
	}
	
	as.fullTextIndex.totalDocuments++
	as.fullTextIndex.indexLastUpdated = time.Now()
	
	return nil
}

// removeDocumentFromIndex removes a document from the index
func (as *AdvancedSearch) removeDocumentFromIndex(path string, doc *SearchDocument) {
	for token := range doc.Tokens {
		// Remove from token index
		if paths, exists := as.fullTextIndex.tokenIndex[token]; exists {
			newPaths := make([]string, 0, len(paths)-1)
			for _, p := range paths {
				if p != path {
					newPaths = append(newPaths, p)
				}
			}
			if len(newPaths) == 0 {
				delete(as.fullTextIndex.tokenIndex, token)
			} else {
				as.fullTextIndex.tokenIndex[token] = newPaths
			}
		}
		
		// Remove from term frequency
		if tf, exists := as.fullTextIndex.termFrequency[token]; exists {
			delete(tf, path)
			if len(tf) == 0 {
				delete(as.fullTextIndex.termFrequency, token)
			}
		}
		
		// Update document frequency
		as.fullTextIndex.documentFreq[token]--
		if as.fullTextIndex.documentFreq[token] <= 0 {
			delete(as.fullTextIndex.documentFreq, token)
		}
	}
	
	delete(as.fullTextIndex.documentIndex, path)
	as.fullTextIndex.totalDocuments--
}

// tokenizeContent tokenizes document content for indexing
func (as *AdvancedSearch) tokenizeContent(content string) []string {
	// More sophisticated tokenization than query tokenization
	content = strings.ToLower(content)
	
	// Split on whitespace, punctuation, but preserve some structure
	words := regexp.MustCompile(`[^\w\-\.]+`).Split(content, -1)
	
	var tokens []string
	for _, word := range words {
		word = strings.TrimSpace(word)
		if len(word) >= 2 { // More inclusive for content indexing
			tokens = append(tokens, word)
		}
	}
	
	return tokens
}

// buildInitialIndex builds the search index from existing repository content
func (as *AdvancedSearch) buildInitialIndex() {
	if as.repo.queryEngine == nil {
		return
	}
	
	// Get all files from the repository
	files, err := as.repo.FindFiles("*")
	if err != nil {
		return
	}
	
	for _, file := range files {
		// Get file content
		blob, err := as.repo.GetBlobWithDelta(file.Hash)
		if err != nil {
			continue
		}
		
		// Only index text files (basic heuristic)
		if as.isTextFile(file.Path, blob.Content) {
			as.IndexDocument(file.Path, blob.Content)
		}
	}
}

// isTextFile determines if a file is likely to contain text
func (as *AdvancedSearch) isTextFile(path string, content []byte) bool {
	// Check file extension
	ext := strings.ToLower(filepath.Ext(path))
	textExts := []string{
		".txt", ".md", ".go", ".js", ".ts", ".py", ".java", ".cpp", ".c", ".h",
		".json", ".yaml", ".yml", ".xml", ".html", ".css", ".scss", ".sass",
		".sh", ".bash", ".sql", ".log", ".conf", ".config", ".ini", ".toml",
		".rs", ".php", ".rb", ".swift", ".kt", ".scala", ".clj", ".fs",
	}
	
	for _, textExt := range textExts {
		if ext == textExt {
			return true
		}
	}
	
	// Heuristic: check if content is mostly printable ASCII
	if len(content) == 0 {
		return false
	}
	
	printableCount := 0
	for i := 0; i < len(content) && i < 1000; i++ { // Check first 1000 bytes
		b := content[i]
		if (b >= 32 && b <= 126) || b == 9 || b == 10 || b == 13 { // Printable + tab/newline/CR
			printableCount++
		}
	}
	
	sampleSize := len(content)
	if sampleSize > 1000 {
		sampleSize = 1000
	}
	
	return float64(printableCount)/float64(sampleSize) > 0.7 // 70% printable
}

// GetIndexStatistics returns statistics about the search index
func (as *AdvancedSearch) GetIndexStatistics() map[string]interface{} {
	as.fullTextIndex.mu.RLock()
	defer as.fullTextIndex.mu.RUnlock()
	
	return map[string]interface{}{
		"total_documents":    as.fullTextIndex.totalDocuments,
		"total_tokens":       len(as.fullTextIndex.tokenIndex),
		"index_last_updated": as.fullTextIndex.indexLastUpdated,
		"average_doc_size":   as.calculateAverageDocSize(),
		"top_tokens":         as.getTopTokens(10),
	}
}

// calculateAverageDocSize calculates average document size
func (as *AdvancedSearch) calculateAverageDocSize() float64 {
	if as.fullTextIndex.totalDocuments == 0 {
		return 0
	}
	
	totalSize := int64(0)
	for _, doc := range as.fullTextIndex.documentIndex {
		totalSize += doc.Size
	}
	
	return float64(totalSize) / float64(as.fullTextIndex.totalDocuments)
}

// getTopTokens returns the most frequent tokens
func (as *AdvancedSearch) getTopTokens(limit int) []map[string]interface{} {
	type tokenFreq struct {
		Token string
		Freq  int
	}
	
	var tokens []tokenFreq
	for token, freq := range as.fullTextIndex.documentFreq {
		tokens = append(tokens, tokenFreq{Token: token, Freq: freq})
	}
	
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Freq > tokens[j].Freq
	})
	
	if limit > len(tokens) {
		limit = len(tokens)
	}
	
	result := make([]map[string]interface{}, limit)
	for i := 0; i < limit; i++ {
		result[i] = map[string]interface{}{
			"token":     tokens[i].Token,
			"frequency": tokens[i].Freq,
		}
	}
	
	return result
}

// SearchSuggestions provides query suggestions based on the index
func (as *AdvancedSearch) SearchSuggestions(partialQuery string, limit int) []string {
	if limit == 0 {
		limit = 10
	}
	
	as.fullTextIndex.mu.RLock()
	defer as.fullTextIndex.mu.RUnlock()
	
	partialQuery = strings.ToLower(strings.TrimSpace(partialQuery))
	if len(partialQuery) < 2 {
		return []string{}
	}
	
	type suggestion struct {
		Token string
		Freq  int
	}
	
	var suggestions []suggestion
	for token, freq := range as.fullTextIndex.documentFreq {
		if strings.HasPrefix(token, partialQuery) {
			suggestions = append(suggestions, suggestion{Token: token, Freq: freq})
		}
	}
	
	// Sort by frequency
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Freq > suggestions[j].Freq
	})
	
	if limit > len(suggestions) {
		limit = len(suggestions)
	}
	
	result := make([]string, limit)
	for i := 0; i < limit; i++ {
		result[i] = suggestions[i].Token
	}
	
	return result
}

// matchAllDocuments returns all documents in the index (for "*" query)
func (as *AdvancedSearch) matchAllDocuments(req *FullTextSearchRequest, start time.Time) (*FullTextSearchResponse, error) {
	var results []SearchResult
	
	// Get all documents from the index
	for _, doc := range as.fullTextIndex.documentIndex {
		// Apply filters
		if req.MaxSize > 0 && doc.Size > req.MaxSize {
			continue
		}
		if len(req.FileTypes) > 0 && !as.matchesFileType(doc.Path, req.FileTypes) {
			continue
		}
		
		result := SearchResult{
			Document:    doc,
			Score:       1.0, // Default score for match-all
			Highlights:  []string{},
			MatchReason: "Match all query (*)",
		}
		
		// Remove content if not requested to save bandwidth
		if !req.IncludeContent {
			result.Document.Content = ""
		}
		
		results = append(results, result)
	}
	
	// Sort results by the specified criteria
	as.sortResults(results, req.SortBy)
	
	// Apply pagination
	total := len(results)
	if req.Limit > 0 {
		startIdx := req.Offset
		endIdx := startIdx + req.Limit
		if startIdx >= len(results) {
			results = []SearchResult{}
		} else {
			if endIdx > len(results) {
				endIdx = len(results)
			}
			results = results[startIdx:endIdx]
		}
	}
	
	avgScore := 1.0
	maxScore := 1.0
	
	return &FullTextSearchResponse{
		Results:   results,
		Total:     total,
		QueryTime: time.Since(start),
		Statistics: SearchStatistics{
			DocumentsScanned: len(as.fullTextIndex.documentIndex),
			TokensMatched:    0, // No specific tokens for match-all
			AvgScore:         avgScore,
			MaxScore:         maxScore,
			IndexSize:        as.fullTextIndex.totalDocuments,
			IndexLastUpdated: as.fullTextIndex.indexLastUpdated,
			ProcessingTime:   time.Since(start),
		},
	}, nil
}

