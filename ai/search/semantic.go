package search

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/caiatech/govc"
	"github.com/caiatech/govc/ai/embeddings"
)

// SemanticSearchEngine provides intelligent code search using embeddings
type SemanticSearchEngine struct {
	provider   embeddings.EmbeddingProvider
	index      *SearchIndex
	config     SearchConfig
	mu         sync.RWMutex
	indexCache map[string]*FileEmbedding // filePath -> embedding
}

// SearchConfig holds configuration for semantic search
type SearchConfig struct {
	MinSimilarity    float64 `json:"min_similarity"`     // Minimum cosine similarity threshold (0.0-1.0)
	MaxResults       int     `json:"max_results"`        // Maximum number of results to return
	IndexChunkSize   int     `json:"index_chunk_size"`   // Size of code chunks for indexing
	SupportedExtensions []string `json:"supported_extensions"` // File extensions to index
	ExcludePatterns  []string `json:"exclude_patterns"`  // Patterns to exclude from indexing
	ReindexInterval  time.Duration `json:"reindex_interval"` // How often to rebuild the index
}

// DefaultSearchConfig returns sensible defaults for semantic search
func DefaultSearchConfig() SearchConfig {
	return SearchConfig{
		MinSimilarity:  0.3, // 30% similarity threshold
		MaxResults:     20,
		IndexChunkSize: 1000, // ~1000 characters per chunk
		SupportedExtensions: []string{
			".go", ".js", ".ts", ".py", ".java", ".cpp", ".c", ".h",
			".cs", ".php", ".rb", ".rs", ".kt", ".scala", ".swift",
			".md", ".txt", ".json", ".yaml", ".yml", ".xml", ".sql",
		},
		ExcludePatterns: []string{
			"node_modules/", "vendor/", ".git/", "target/",
			"*.min.js", "*.bundle.js", "*.map", "coverage/",
		},
		ReindexInterval: 10 * time.Minute,
	}
}

// SearchIndex holds the searchable index of code embeddings
type SearchIndex struct {
	Files       map[string]*FileEmbedding `json:"files"`        // filePath -> embedding info
	LastUpdated time.Time                 `json:"last_updated"` // When index was last updated
	Version     string                    `json:"version"`      // govc repository version/commit hash
	mu          sync.RWMutex
}

// FileEmbedding represents the embedding information for a file
type FileEmbedding struct {
	FilePath    string            `json:"file_path"`    // Relative path from repo root
	Language    string            `json:"language"`     // Detected programming language
	Size        int               `json:"size"`         // File size in bytes
	LastModified time.Time        `json:"last_modified"` // When file was last modified
	Chunks      []ChunkEmbedding  `json:"chunks"`       // File divided into chunks
	Metadata    map[string]string `json:"metadata"`     // Additional metadata (e.g., imports, functions)
}

// ChunkEmbedding represents a searchable chunk of code with its embedding
type ChunkEmbedding struct {
	Content     string    `json:"content"`     // The actual code/text content
	StartLine   int       `json:"start_line"`  // Starting line number in file
	EndLine     int       `json:"end_line"`    // Ending line number in file
	Embedding   []float32 `json:"embedding"`   // Vector embedding of the content
	Keywords    []string  `json:"keywords"`    // Extracted keywords/tokens
	Functions   []string  `json:"functions"`   // Function names in this chunk
	Complexity  float64   `json:"complexity"`  // Estimated code complexity (0-1)
}

// SearchResult represents a search result with similarity scoring
type SearchResult struct {
	FilePath    string            `json:"file_path"`    // Path to the file
	Chunk       ChunkEmbedding    `json:"chunk"`        // The matching chunk
	Similarity  float64           `json:"similarity"`   // Cosine similarity score (0-1)
	Rank        int               `json:"rank"`         // Result ranking
	Explanation string            `json:"explanation"`  // Why this result was matched
	Context     []ChunkEmbedding  `json:"context"`      // Surrounding chunks for context
	Metadata    map[string]string `json:"metadata"`     // Additional result metadata
}

// NewSemanticSearchEngine creates a new semantic search engine
func NewSemanticSearchEngine(provider embeddings.EmbeddingProvider, config SearchConfig) *SemanticSearchEngine {
	return &SemanticSearchEngine{
		provider:   provider,
		config:     config,
		index:      &SearchIndex{Files: make(map[string]*FileEmbedding)},
		indexCache: make(map[string]*FileEmbedding),
	}
}

// IndexRepository builds a searchable index of the repository
func (s *SemanticSearchEngine) IndexRepository(ctx context.Context, repo *govc.Repository) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	fmt.Println("🔍 Building semantic search index...")
	startTime := time.Now()
	
	// Get all files - try repository first, fall back to working directory
	files, err := s.getIndexableFiles(repo)
	if err != nil {
		return fmt.Errorf("failed to get indexable files: %w", err)
	}
	
	// Filter files based on configuration
	supportedFiles := s.filterSupportedFiles(files)
	fmt.Printf("📁 Indexing %d files...\n", len(supportedFiles))
	
	// Process files in batches
	batchSize := 10 // Process 10 files at a time
	totalFiles := len(supportedFiles)
	processedFiles := 0
	
	for i := 0; i < len(supportedFiles); i += batchSize {
		end := i + batchSize
		if end > len(supportedFiles) {
			end = len(supportedFiles)
		}
		
		batch := supportedFiles[i:end]
		if err := s.processBatch(ctx, repo, batch); err != nil {
			fmt.Printf("⚠️  Error processing batch %d-%d: %v\n", i, end, err)
			continue // Continue with other batches
		}
		
		processedFiles += len(batch)
		progress := float64(processedFiles) / float64(totalFiles) * 100
		fmt.Printf("⚡ Progress: %.1f%% (%d/%d files)\n", progress, processedFiles, totalFiles)
	}
	
	// Update index metadata
	s.index.LastUpdated = time.Now()
	s.index.Version = s.getCurrentRepoVersion(repo)
	
	elapsed := time.Since(startTime)
	fmt.Printf("✅ Semantic index built successfully in %v\n", elapsed)
	fmt.Printf("📊 Indexed %d files with %d total chunks\n", len(s.index.Files), s.getTotalChunks())
	
	return nil
}

// Search performs semantic search across the indexed repository
func (s *SemanticSearchEngine) Search(ctx context.Context, query string) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if len(s.index.Files) == 0 {
		return nil, fmt.Errorf("no files indexed - run IndexRepository first")
	}
	
	// Get embedding for the search query
	queryEmbedding, err := s.provider.GetEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get query embedding: %w", err)
	}
	
	// Search across all chunks
	var results []SearchResult
	
	for filePath, fileEmbed := range s.index.Files {
		for i, chunk := range fileEmbed.Chunks {
			similarity := s.cosineSimilarity(queryEmbedding, chunk.Embedding)
			
			if similarity >= s.config.MinSimilarity {
				result := SearchResult{
					FilePath:    filePath,
					Chunk:       chunk,
					Similarity:  similarity,
					Explanation: s.generateExplanation(query, chunk, similarity),
					Context:     s.getContext(fileEmbed, i),
					Metadata: map[string]string{
						"language":   fileEmbed.Language,
						"file_size":  fmt.Sprintf("%d", fileEmbed.Size),
						"complexity": fmt.Sprintf("%.2f", chunk.Complexity),
					},
				}
				results = append(results, result)
			}
		}
	}
	
	// Sort by similarity (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})
	
	// Limit results and add ranking
	if len(results) > s.config.MaxResults {
		results = results[:s.config.MaxResults]
	}
	
	for i := range results {
		results[i].Rank = i + 1
	}
	
	return results, nil
}

// SearchByFile performs semantic search within a specific file
func (s *SemanticSearchEngine) SearchByFile(ctx context.Context, query, filePath string) ([]SearchResult, error) {
	s.mu.RLock()
	fileEmbed, exists := s.index.Files[filePath]
	s.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("file not found in index: %s", filePath)
	}
	
	queryEmbedding, err := s.provider.GetEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get query embedding: %w", err)
	}
	
	var results []SearchResult
	
	for i, chunk := range fileEmbed.Chunks {
		similarity := s.cosineSimilarity(queryEmbedding, chunk.Embedding)
		
		if similarity >= s.config.MinSimilarity {
			result := SearchResult{
				FilePath:    filePath,
				Chunk:       chunk,
				Similarity:  similarity,
				Explanation: s.generateExplanation(query, chunk, similarity),
				Context:     s.getContext(fileEmbed, i),
				Metadata: map[string]string{
					"language":   fileEmbed.Language,
					"complexity": fmt.Sprintf("%.2f", chunk.Complexity),
				},
			}
			results = append(results, result)
		}
	}
	
	// Sort and rank
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})
	
	for i := range results {
		results[i].Rank = i + 1
	}
	
	return results, nil
}

// GetIndexStats returns statistics about the search index
func (s *SemanticSearchEngine) GetIndexStats() IndexStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	stats := IndexStats{
		TotalFiles:   len(s.index.Files),
		TotalChunks:  s.getTotalChunks(),
		LastUpdated:  s.index.LastUpdated,
		Version:      s.index.Version,
		Languages:    make(map[string]int),
		AvgChunkSize: 0,
	}
	
	totalChunkContent := 0
	for _, fileEmbed := range s.index.Files {
		stats.Languages[fileEmbed.Language]++
		for _, chunk := range fileEmbed.Chunks {
			totalChunkContent += len(chunk.Content)
		}
	}
	
	if stats.TotalChunks > 0 {
		stats.AvgChunkSize = totalChunkContent / stats.TotalChunks
	}
	
	return stats
}

// IndexStats provides statistics about the search index
type IndexStats struct {
	TotalFiles   int            `json:"total_files"`
	TotalChunks  int            `json:"total_chunks"`
	LastUpdated  time.Time      `json:"last_updated"`
	Version      string         `json:"version"`
	Languages    map[string]int `json:"languages"`
	AvgChunkSize int            `json:"avg_chunk_size"`
}

// Helper methods

func (s *SemanticSearchEngine) filterSupportedFiles(files []string) []string {
	var supported []string
	
	for _, file := range files {
		// Check if file extension is supported
		ext := strings.ToLower(filepath.Ext(file))
		isSupported := false
		for _, supportedExt := range s.config.SupportedExtensions {
			if ext == supportedExt {
				isSupported = true
				break
			}
		}
		
		if !isSupported {
			continue
		}
		
		// Check exclude patterns
		excluded := false
		for _, pattern := range s.config.ExcludePatterns {
			if strings.Contains(file, pattern) {
				excluded = true
				break
			}
		}
		
		if !excluded {
			supported = append(supported, file)
		}
	}
	
	return supported
}

func (s *SemanticSearchEngine) processBatch(ctx context.Context, repo *govc.Repository, files []string) error {
	for _, filePath := range files {
		if err := s.processFile(ctx, repo, filePath); err != nil {
			fmt.Printf("⚠️  Error processing file %s: %v\n", filePath, err)
			continue // Continue with other files
		}
	}
	return nil
}

func (s *SemanticSearchEngine) processFile(ctx context.Context, repo *govc.Repository, filePath string) error {
	// Read file content - try repository first, then working directory
	content, err := s.readFileContent(repo, filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	
	// Detect language
	language := s.detectLanguage(filePath, string(content))
	
	// Split content into chunks
	chunks := s.chunkContent(string(content), filePath)
	if len(chunks) == 0 {
		return nil // Skip empty files
	}
	
	// Generate embeddings for chunks
	chunkTexts := make([]string, len(chunks))
	for i, chunk := range chunks {
		chunkTexts[i] = chunk.Content
	}
	
	embeddings, err := s.provider.GetBatchEmbeddings(ctx, chunkTexts)
	if err != nil {
		return fmt.Errorf("failed to get embeddings for file %s: %w", filePath, err)
	}
	
	// Update chunks with embeddings
	for i, embedding := range embeddings {
		if i < len(chunks) {
			chunks[i].Embedding = embedding
		}
	}
	
	// Create file embedding
	fileEmbed := &FileEmbedding{
		FilePath:     filePath,
		Language:     language,
		Size:         len(content),
		LastModified: time.Now(), // In a real implementation, get actual file mod time
		Chunks:       chunks,
		Metadata:     s.extractMetadata(string(content), language),
	}
	
	// Store in index
	s.index.Files[filePath] = fileEmbed
	
	return nil
}

func (s *SemanticSearchEngine) chunkContent(content, filePath string) []ChunkEmbedding {
	lines := strings.Split(content, "\n")
	var chunks []ChunkEmbedding
	
	chunkSize := s.config.IndexChunkSize
	currentChunk := strings.Builder{}
	startLine := 1
	currentLine := 1
	
	for _, line := range lines {
		if currentChunk.Len()+len(line)+1 > chunkSize && currentChunk.Len() > 0 {
			// Create chunk
			chunkContent := currentChunk.String()
			if strings.TrimSpace(chunkContent) != "" {
				chunk := ChunkEmbedding{
					Content:    chunkContent,
					StartLine:  startLine,
					EndLine:    currentLine - 1,
					Keywords:   s.extractKeywords(chunkContent),
					Functions:  s.extractFunctions(chunkContent, s.detectLanguage(filePath, content)),
					Complexity: s.estimateComplexity(chunkContent),
				}
				chunks = append(chunks, chunk)
			}
			
			// Start new chunk
			currentChunk.Reset()
			startLine = currentLine
		}
		
		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n")
		}
		currentChunk.WriteString(line)
		currentLine++
	}
	
	// Add final chunk
	if currentChunk.Len() > 0 {
		chunkContent := currentChunk.String()
		if strings.TrimSpace(chunkContent) != "" {
			chunk := ChunkEmbedding{
				Content:    chunkContent,
				StartLine:  startLine,
				EndLine:    currentLine - 1,
				Keywords:   s.extractKeywords(chunkContent),
				Functions:  s.extractFunctions(chunkContent, s.detectLanguage(filePath, content)),
				Complexity: s.estimateComplexity(chunkContent),
			}
			chunks = append(chunks, chunk)
		}
	}
	
	return chunks
}

func (s *SemanticSearchEngine) detectLanguage(filePath, content string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	
	switch ext {
	case ".go":
		return "go"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".c":
		return "c"
	case ".h", ".hpp":
		return "c_header"
	case ".cs":
		return "csharp"
	case ".php":
		return "php"
	case ".rb":
		return "ruby"
	case ".rs":
		return "rust"
	case ".kt":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".swift":
		return "swift"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".xml":
		return "xml"
	case ".sql":
		return "sql"
	default:
		return "text"
	}
}

func (s *SemanticSearchEngine) extractKeywords(content string) []string {
	// Simple keyword extraction - in production, use more sophisticated NLP
	words := strings.Fields(strings.ToLower(content))
	keywords := make(map[string]bool)
	
	for _, word := range words {
		// Clean word
		word = strings.Trim(word, "(){}[];,.:!?\"'")
		if len(word) >= 3 && !s.isStopWord(word) {
			keywords[word] = true
		}
	}
	
	result := make([]string, 0, len(keywords))
	for keyword := range keywords {
		result = append(result, keyword)
	}
	
	return result
}

func (s *SemanticSearchEngine) extractFunctions(content, language string) []string {
	var functions []string
	
	switch language {
	case "go":
		// Simple Go function extraction
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "func ") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					funcName := parts[1]
					if idx := strings.Index(funcName, "("); idx > 0 {
						funcName = funcName[:idx]
					}
					functions = append(functions, funcName)
				}
			}
		}
	case "javascript", "typescript":
		// Simple JS/TS function extraction
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "function ") || strings.Contains(line, " => ") {
				// Basic function name extraction
				if strings.Contains(line, "function ") {
					start := strings.Index(line, "function ") + 9
					if start < len(line) {
						remaining := line[start:]
						if idx := strings.Index(remaining, "("); idx > 0 {
							funcName := strings.TrimSpace(remaining[:idx])
							if funcName != "" {
								functions = append(functions, funcName)
							}
						}
					}
				}
			}
		}
	}
	
	return functions
}

func (s *SemanticSearchEngine) estimateComplexity(content string) float64 {
	// Simple complexity estimation based on code characteristics
	complexity := 0.0
	
	// Count control structures
	controlStructures := []string{"if", "for", "while", "switch", "case", "try", "catch"}
	for _, structure := range controlStructures {
		complexity += float64(strings.Count(strings.ToLower(content), structure)) * 0.1
	}
	
	// Count nesting levels (approximate)
	braces := strings.Count(content, "{") - strings.Count(content, "}")
	if braces < 0 {
		braces = -braces
	}
	complexity += float64(braces) * 0.05
	
	// Normalize to 0-1 range
	if complexity > 1.0 {
		complexity = 1.0
	}
	
	return complexity
}

func (s *SemanticSearchEngine) extractMetadata(content, language string) map[string]string {
	metadata := make(map[string]string)
	metadata["language"] = language
	metadata["lines"] = fmt.Sprintf("%d", len(strings.Split(content, "\n")))
	
	// Language-specific metadata extraction
	switch language {
	case "go":
		if strings.Contains(content, "package ") {
			lines := strings.Split(content, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "package ") {
					metadata["package"] = strings.TrimSpace(line[8:])
					break
				}
			}
		}
	case "javascript", "typescript":
		if strings.Contains(content, "import ") {
			metadata["has_imports"] = "true"
		}
		if strings.Contains(content, "export ") {
			metadata["has_exports"] = "true"
		}
	}
	
	return metadata
}

func (s *SemanticSearchEngine) isStopWord(word string) bool {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "is": true,
		"are": true, "was": true, "were": true, "be": true, "been": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
	}
	return stopWords[word]
}

func (s *SemanticSearchEngine) cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0.0
	}
	
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	
	if normA == 0 || normB == 0 {
		return 0.0
	}
	
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// getIndexableFiles gets files from repository or falls back to working directory
func (s *SemanticSearchEngine) getIndexableFiles(repo *govc.Repository) ([]string, error) {
	// First try to get files from the repository (from commits)
	files, err := repo.ListFiles()
	if err == nil && len(files) > 0 {
		return files, nil
	}
	
	// If repository has no files (no commits), scan working directory
	return s.scanWorkingDirectory(".")
}

// scanWorkingDirectory scans the current working directory for supported files
func (s *SemanticSearchEngine) scanWorkingDirectory(dir string) ([]string, error) {
	var files []string
	
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip directories
		if info.IsDir() {
			// Skip excluded directories
			for _, pattern := range s.config.ExcludePatterns {
				if strings.Contains(path, pattern) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		
		// Check if file has supported extension
		ext := filepath.Ext(path)
		for _, supportedExt := range s.config.SupportedExtensions {
			if ext == supportedExt {
				// Make path relative to working directory
				relPath, err := filepath.Rel(".", path)
				if err != nil {
					relPath = path
				}
				files = append(files, relPath)
				break
			}
		}
		
		return nil
	})
	
	return files, err
}

// readFileContent reads file content from repository or working directory
func (s *SemanticSearchEngine) readFileContent(repo *govc.Repository, filePath string) ([]byte, error) {
	// First try to read from repository
	content, err := repo.ReadFile(filePath)
	if err == nil {
		return content, nil
	}
	
	// If repository read fails, try reading from working directory
	return os.ReadFile(filePath)
}

func (s *SemanticSearchEngine) generateExplanation(query string, chunk ChunkEmbedding, similarity float64) string {
	// Generate a human-readable explanation of why this result matched
	explanation := fmt.Sprintf("%.1f%% semantic similarity", similarity*100)
	
	if len(chunk.Keywords) > 0 {
		commonKeywords := s.findCommonKeywords(query, chunk.Keywords)
		if len(commonKeywords) > 0 {
			explanation += fmt.Sprintf(" (keywords: %s)", strings.Join(commonKeywords, ", "))
		}
	}
	
	if len(chunk.Functions) > 0 {
		explanation += fmt.Sprintf(" [%d functions]", len(chunk.Functions))
	}
	
	return explanation
}

func (s *SemanticSearchEngine) findCommonKeywords(query string, keywords []string) []string {
	queryWords := strings.Fields(strings.ToLower(query))
	var common []string
	
	for _, keyword := range keywords {
		for _, queryWord := range queryWords {
			if strings.Contains(keyword, queryWord) || strings.Contains(queryWord, keyword) {
				common = append(common, keyword)
				break
			}
		}
	}
	
	return common
}

func (s *SemanticSearchEngine) getContext(fileEmbed *FileEmbedding, chunkIndex int) []ChunkEmbedding {
	// Return surrounding chunks for context
	var context []ChunkEmbedding
	
	// Add previous chunk
	if chunkIndex > 0 {
		context = append(context, fileEmbed.Chunks[chunkIndex-1])
	}
	
	// Add next chunk
	if chunkIndex < len(fileEmbed.Chunks)-1 {
		context = append(context, fileEmbed.Chunks[chunkIndex+1])
	}
	
	return context
}

func (s *SemanticSearchEngine) getTotalChunks() int {
	total := 0
	for _, fileEmbed := range s.index.Files {
		total += len(fileEmbed.Chunks)
	}
	return total
}

func (s *SemanticSearchEngine) getCurrentRepoVersion(repo *govc.Repository) string {
	// In a real implementation, get the current commit hash
	// For now, return a timestamp
	return fmt.Sprintf("snapshot-%d", time.Now().Unix())
}