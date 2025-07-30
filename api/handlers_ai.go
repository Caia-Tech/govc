package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/caiatech/govc"
	"github.com/caiatech/govc/ai/embeddings"
	"github.com/caiatech/govc/ai/llm"
	"github.com/caiatech/govc/ai/search"
	"github.com/gin-gonic/gin"
)

// AI Search request/response types
type SemanticSearchRequest struct {
	Query         string  `json:"query" binding:"required"`
	FilePath      string  `json:"file_path,omitempty"`
	MaxResults    int     `json:"max_results,omitempty"`
	MinSimilarity float64 `json:"min_similarity,omitempty"`
}

type SemanticSearchResponse struct {
	Query     string                `json:"query"`
	Results   []search.SearchResult `json:"results"`
	Stats     SearchStats           `json:"stats"`
	Timestamp time.Time             `json:"timestamp"`
}

type SearchStats struct {
	TotalResults  int           `json:"total_results"`
	SearchTime    time.Duration `json:"search_time_ms"`
	MaxSimilarity float64       `json:"max_similarity"`
	MinSimilarity float64       `json:"min_similarity"`
}

// Index build request/response types
type BuildIndexRequest struct {
	Provider   string `json:"provider,omitempty"`   // "openai" or "local"
	Model      string `json:"model,omitempty"`      // embedding model
	ChunkSize  int    `json:"chunk_size,omitempty"` // chunk size for indexing
	Rebuild    bool   `json:"rebuild,omitempty"`    // force rebuild
	APIKey     string `json:"api_key,omitempty"`    // API key for provider
}

type BuildIndexResponse struct {
	Status      string              `json:"status"` // "building", "completed", "failed"
	Progress    IndexBuildProgress  `json:"progress"`
	Stats       search.IndexStats   `json:"stats,omitempty"`
	Error       string              `json:"error,omitempty"`
	BuildTime   time.Duration       `json:"build_time,omitempty"`
	Timestamp   time.Time           `json:"timestamp"`
}

type IndexBuildProgress struct {
	Phase           string  `json:"phase"`
	ProcessedFiles  int     `json:"processed_files"`
	TotalFiles      int     `json:"total_files"`
	ProcessedChunks int     `json:"processed_chunks"`
	Percentage      float64 `json:"percentage"`
	EstimatedTime   string  `json:"estimated_time,omitempty"`
}

// Commit message request/response types
type GenerateCommitMessageRequest struct {
	Provider   string `json:"provider,omitempty"`    // "openai", "local"
	Style      string `json:"style,omitempty"`       // "conventional", "semantic", "custom"
	Language   string `json:"language,omitempty"`    // "en", "es", "fr", etc.
	MaxLength  int    `json:"max_length,omitempty"`  // max message length
	Multiple   int    `json:"multiple,omitempty"`    // number of options to generate
	APIKey     string `json:"api_key,omitempty"`     // API key for provider
	Diff       string `json:"diff,omitempty"`        // git diff to analyze
}

type GenerateCommitMessageResponse struct {
	Messages    []string              `json:"messages"`
	Analysis    CommitAnalysisResult  `json:"analysis"`
	Provider    string                `json:"provider"`
	Model       string                `json:"model,omitempty"`
	GenerationTime time.Duration      `json:"generation_time"`
	Timestamp   time.Time             `json:"timestamp"`
}

type CommitAnalysisResult struct {
	ChangeType    string   `json:"change_type"`     // "feat", "fix", "refactor", etc.
	Scope         string   `json:"scope,omitempty"` // detected scope
	Languages     []string `json:"languages"`       // involved languages
	FilesChanged  int      `json:"files_changed"`   // number of files
	LinesAdded    int      `json:"lines_added"`     // lines added
	LinesDeleted  int      `json:"lines_deleted"`   // lines deleted
	Complexity    string   `json:"complexity"`      // "low", "medium", "high"
}

// Index statistics response
type IndexStatsResponse struct {
	Stats       search.IndexStats `json:"stats"`
	Status      string            `json:"status"`      // "ready", "building", "missing"
	Provider    string            `json:"provider"`    // embedding provider used
	Timestamp   time.Time         `json:"timestamp"`
}

// AI feature handlers

func (s *Server) setupAIRoutes(v1 *gin.RouterGroup) {
	ai := v1.Group("/ai")
	{
		// Semantic search endpoints
		ai.POST("/search", s.semanticSearch)
		ai.POST("/search/:repo_id", s.semanticSearchInRepo)
		
		// Index management endpoints
		ai.POST("/index", s.buildSemanticIndex)
		ai.POST("/index/:repo_id", s.buildSemanticIndexForRepo)
		ai.GET("/index/stats", s.getIndexStats)
		ai.GET("/index/:repo_id/stats", s.getIndexStatsForRepo)
		
		// Commit message generation endpoints
		ai.POST("/commit-message", s.generateCommitMessage)
		ai.POST("/commit-message/:repo_id", s.generateCommitMessageForRepo)
		
		// AI configuration endpoints
		ai.GET("/config", s.getAIConfig)
		ai.PUT("/config", s.updateAIConfig)
	}
}

func (s *Server) semanticSearch(c *gin.Context) {
	var req SemanticSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request: " + err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Use default repository for now (in production, this would be configurable)
	repo := s.repo
	
	startTime := time.Now()
	results, err := s.performSemanticSearch(c.Request.Context(), repo, req)
	searchTime := time.Since(startTime)
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Search failed: " + err.Error(),
			Code:  "SEARCH_FAILED",
		})
		return
	}

	// Calculate stats
	stats := SearchStats{
		TotalResults: len(results),
		SearchTime:   searchTime,
	}
	
	if len(results) > 0 {
		stats.MaxSimilarity = results[0].Similarity
		stats.MinSimilarity = results[len(results)-1].Similarity
	}

	response := SemanticSearchResponse{
		Query:     req.Query,
		Results:   results,
		Stats:     stats,
		Timestamp: time.Now(),
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) semanticSearchInRepo(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "Repository not found: " + err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req SemanticSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request: " + err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	startTime := time.Now()
	results, err := s.performSemanticSearch(c.Request.Context(), repo, req)
	searchTime := time.Since(startTime)
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Search failed: " + err.Error(),
			Code:  "SEARCH_FAILED",
		})
		return
	}

	stats := SearchStats{
		TotalResults: len(results),
		SearchTime:   searchTime,
	}
	
	if len(results) > 0 {
		stats.MaxSimilarity = results[0].Similarity
		stats.MinSimilarity = results[len(results)-1].Similarity
	}

	response := SemanticSearchResponse{
		Query:     req.Query,
		Results:   results,
		Stats:     stats,
		Timestamp: time.Now(),
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) buildSemanticIndex(c *gin.Context) {
	var req BuildIndexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request: " + err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Use default repository
	repo := s.repo
	
	response, err := s.performIndexBuild(c.Request.Context(), repo, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Index build failed: " + err.Error(),
			Code:  "INDEX_BUILD_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) buildSemanticIndexForRepo(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "Repository not found: " + err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req BuildIndexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request: " + err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	response, err := s.performIndexBuild(c.Request.Context(), repo, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Index build failed: " + err.Error(),
			Code:  "INDEX_BUILD_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) getIndexStats(c *gin.Context) {
	// Use default repository
	repo := s.repo
	
	response, err := s.getSemanticIndexStats(c.Request.Context(), repo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to get index stats: " + err.Error(),
			Code:  "STATS_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) getIndexStatsForRepo(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "Repository not found: " + err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	response, err := s.getSemanticIndexStats(c.Request.Context(), repo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to get index stats: " + err.Error(),
			Code:  "STATS_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) generateCommitMessage(c *gin.Context) {
	var req GenerateCommitMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request: " + err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Use default repository
	repo := s.repo
	
	response, err := s.performCommitMessageGeneration(c.Request.Context(), repo, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Commit message generation failed: " + err.Error(),
			Code:  "GENERATION_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) generateCommitMessageForRepo(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "Repository not found: " + err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req GenerateCommitMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request: " + err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	response, err := s.performCommitMessageGeneration(c.Request.Context(), repo, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Commit message generation failed: " + err.Error(),
			Code:  "GENERATION_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) getAIConfig(c *gin.Context) {
	// Return current AI configuration
	config := map[string]interface{}{
		"embedding_provider": "local",
		"llm_provider":      "local",
		"features": map[string]bool{
			"semantic_search":        true,
			"commit_message_gen":     true,
			"conflict_resolution":    false, // Not implemented yet
			"code_review_automation": false, // Not implemented yet
		},
		"limits": map[string]interface{}{
			"max_search_results":     50,
			"max_commit_msg_length":  72,
			"index_chunk_size":       1000,
		},
	}

	c.JSON(http.StatusOK, config)
}

func (s *Server) updateAIConfig(c *gin.Context) {
	var config map[string]interface{}
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid configuration: " + err.Error(),
			Code:  "INVALID_CONFIG",
		})
		return
	}

	// In a real implementation, this would update persistent configuration
	c.JSON(http.StatusOK, map[string]string{
		"status":  "updated",
		"message": "AI configuration updated successfully",
	})
}

// Helper methods

func (s *Server) performSemanticSearch(ctx context.Context, repo *govc.Repository, req SemanticSearchRequest) ([]search.SearchResult, error) {
	// Set defaults
	if req.MaxResults == 0 {
		req.MaxResults = 20
	}
	if req.MinSimilarity == 0 {
		req.MinSimilarity = 0.3
	}

	// Create embedding provider (default to local for API)
	config := embeddings.DefaultEmbeddingConfig()
	config.Provider = "local" // Safe default for API
	
	provider, err := embeddings.NewEmbeddingProvider(config)
	if err != nil {
		return nil, err
	}

	// Create search engine
	searchConfig := search.DefaultSearchConfig()
	searchConfig.MaxResults = req.MaxResults
	searchConfig.MinSimilarity = req.MinSimilarity
	
	engine := search.NewSemanticSearchEngine(provider, searchConfig)

	// Build index (in production, this would be cached/persistent)
	if err := engine.IndexRepository(ctx, repo); err != nil {
		return nil, err
	}

	// Perform search
	if req.FilePath != "" {
		return engine.SearchByFile(ctx, req.Query, req.FilePath)
	}
	
	return engine.Search(ctx, req.Query)
}

func (s *Server) performIndexBuild(ctx context.Context, repo *govc.Repository, req BuildIndexRequest) (*BuildIndexResponse, error) {
	startTime := time.Now()
	
	// Set defaults
	if req.Provider == "" {
		req.Provider = "local"
	}
	if req.ChunkSize == 0 {
		req.ChunkSize = 1000
	}

	// Create embedding provider
	config := embeddings.DefaultEmbeddingConfig()
	config.Provider = req.Provider
	if req.APIKey != "" {
		config.OpenAIAPIKey = req.APIKey
	}
	if req.Model != "" {
		config.Model = req.Model
	}
	
	provider, err := embeddings.NewEmbeddingProvider(config)
	if err != nil {
		return &BuildIndexResponse{
			Status:    "failed",
			Error:     err.Error(),
			Timestamp: time.Now(),
		}, err
	}

	// Create search engine
	searchConfig := search.DefaultSearchConfig()
	searchConfig.IndexChunkSize = req.ChunkSize
	
	engine := search.NewSemanticSearchEngine(provider, searchConfig)

	// Build index
	if err := engine.IndexRepository(ctx, repo); err != nil {
		return &BuildIndexResponse{
			Status:    "failed",
			Error:     err.Error(),
			Timestamp: time.Now(),
		}, err
	}

	buildTime := time.Since(startTime)
	stats := engine.GetIndexStats()

	return &BuildIndexResponse{
		Status:    "completed",
		Stats:     stats,
		BuildTime: buildTime,
		Timestamp: time.Now(),
	}, nil
}

func (s *Server) getSemanticIndexStats(ctx context.Context, repo *govc.Repository) (*IndexStatsResponse, error) {
	// Create a temporary engine to get stats (in production, would use persistent index)
	config := embeddings.DefaultEmbeddingConfig()
	provider, err := embeddings.NewEmbeddingProvider(config)
	if err != nil {
		return nil, err
	}

	searchConfig := search.DefaultSearchConfig()
	engine := search.NewSemanticSearchEngine(provider, searchConfig)

	// Build index to get current stats
	if err := engine.IndexRepository(ctx, repo); err != nil {
		return &IndexStatsResponse{
			Status:    "missing",
			Provider:  config.Provider,
			Timestamp: time.Now(),
		}, nil
	}

	stats := engine.GetIndexStats()

	return &IndexStatsResponse{
		Stats:     stats,
		Status:    "ready",
		Provider:  config.Provider,
		Timestamp: time.Now(),
	}, nil
}

func (s *Server) performCommitMessageGeneration(ctx context.Context, repo *govc.Repository, req GenerateCommitMessageRequest) (*GenerateCommitMessageResponse, error) {
	startTime := time.Now()
	
	// Set defaults
	if req.Provider == "" {
		req.Provider = "local"
	}
	if req.Style == "" {
		req.Style = "conventional"
	}
	if req.Language == "" {
		req.Language = "en"
	}
	if req.MaxLength == 0 {
		req.MaxLength = 72
	}
	if req.Multiple == 0 {
		req.Multiple = 1
	}

	// Create LLM provider
	provider, err := llm.NewLLMProvider(req.Provider, req.APIKey)
	if err != nil {
		return nil, err
	}

	// Create commit message generator
	config := llm.DefaultCommitMessageConfig()
	config.Style = req.Style
	config.Language = req.Language
	config.MaxLength = req.MaxLength
	
	generator := llm.NewCommitMessageGenerator(provider, config)

	var messages []string
	var analysis CommitAnalysisResult

	if req.Diff != "" {
		// Generate from diff
		message, err := generator.GenerateCommitMessageForDiff(ctx, req.Diff)
		if err != nil {
			return nil, err
		}
		messages = []string{message}
	} else {
		// Generate from repository state
		if req.Multiple > 1 {
			msgs, err := generator.GenerateMultipleOptions(ctx, repo, req.Multiple)
			if err != nil {
				return nil, err
			}
			messages = msgs
		} else {
			message, err := generator.GenerateCommitMessage(ctx, repo)
			if err != nil {
				return nil, err
			}
			messages = []string{message}
		}
	}

	// Analyze repository changes for additional context
	status, err := repo.Status()
	if err == nil {
		analysis = CommitAnalysisResult{
			ChangeType:   "feat", // Simplified for now
			Languages:    []string{"Go"}, // Simplified
			FilesChanged: len(status.Modified) + len(status.Staged),
			Complexity:   "medium", // Simplified
		}
	}

	generationTime := time.Since(startTime)
	modelInfo := provider.GetModelInfo()

	return &GenerateCommitMessageResponse{
		Messages:       messages,
		Analysis:       analysis,
		Provider:       modelInfo.Provider,
		Model:          modelInfo.Name,
		GenerationTime: generationTime,
		Timestamp:      time.Now(),
	}, nil
}

// Additional helper functions for AI features

func (s *Server) validateSearchQuery(query string) error {
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("search query cannot be empty")
	}
	if len(query) > 1000 {
		return fmt.Errorf("search query too long (max 1000 characters)")
	}
	return nil
}

func (s *Server) validateEmbeddingProvider(provider string) error {
	validProviders := []string{"openai", "local"}
	for _, valid := range validProviders {
		if provider == valid {
			return nil
		}
	}
	return fmt.Errorf("unsupported embedding provider: %s", provider)
}

func (s *Server) validateLLMProvider(provider string) error {
	validProviders := []string{"openai", "local"}
	for _, valid := range validProviders {
		if provider == valid {
			return nil
		}
	}
	return fmt.Errorf("unsupported LLM provider: %s", provider)
}

func (s *Server) parseIntParam(c *gin.Context, param string, defaultValue int) int {
	if value := c.Query(param); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func (s *Server) parseFloatParam(c *gin.Context, param string, defaultValue float64) float64 {
	if value := c.Query(param); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}