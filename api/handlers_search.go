package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/caiatech/govc"
	"github.com/gin-gonic/gin"
)

// searchCommits searches through commit messages, authors, and emails
func (s *Server) searchCommits(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Parse query parameters
	query := c.Query("query")
	if query == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "query parameter is required",
			Code:  "MISSING_QUERY",
		})
		return
	}

	author := c.Query("author")
	since := c.Query("since")
	until := c.Query("until")

	limit := 50 // default
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Perform search
	commits, total, err := repo.SearchCommits(query, author, since, until, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("search failed: %v", err),
			Code:  "SEARCH_FAILED",
		})
		return
	}

	// Convert to API response format
	results := make([]CommitResponse, 0)
	matches := make([]SearchMatch, 0)

	for _, commit := range commits {
		// Create commit response
		commitResp := CommitResponse{
			Hash:      commit.Hash(),
			Message:   commit.Message,
			Author:    commit.Author.Name,
			Email:     commit.Author.Email,
			Timestamp: commit.Author.Time,
		}
		if commit.ParentHash != "" {
			commitResp.Parent = commit.ParentHash
		}
		results = append(results, commitResp)

		// Add search matches for highlighting
		queryLower := strings.ToLower(query)
		if strings.Contains(strings.ToLower(commit.Message), queryLower) {
			matches = append(matches, SearchMatch{
				Field:   "message",
				Preview: commit.Message,
			})
		}
		if strings.Contains(strings.ToLower(commit.Author.Name), queryLower) {
			matches = append(matches, SearchMatch{
				Field:   "author",
				Preview: commit.Author.Name,
			})
		}
		if strings.Contains(strings.ToLower(commit.Author.Email), queryLower) {
			matches = append(matches, SearchMatch{
				Field:   "email",
				Preview: commit.Author.Email,
			})
		}
	}

	c.JSON(http.StatusOK, SearchCommitsResponse{
		Query:   query,
		Results: results,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		Matches: matches,
	})
}

// searchContent searches for text within file contents
func (s *Server) searchContent(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Parse query parameters
	query := c.Query("query")
	if query == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "query parameter is required",
			Code:  "MISSING_QUERY",
		})
		return
	}

	path := c.Query("path")
	ref := c.Query("ref")
	caseSensitive := c.Query("case_sensitive") == "true"
	regex := c.Query("regex") == "true"

	limit := 50 // default
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Perform search
	matches, total, err := repo.SearchContent(query, path, ref, caseSensitive, regex, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("search failed: %v", err),
			Code:  "SEARCH_FAILED",
		})
		return
	}

	// Convert to API response format
	var results []ContentMatch
	for _, match := range matches {
		var matchRanges []MatchRange
		for _, mr := range match.Matches {
			matchRanges = append(matchRanges, MatchRange{
				Start: mr.Start,
				End:   mr.End,
			})
		}

		results = append(results, ContentMatch{
			Path:    match.Path,
			Ref:     match.Ref,
			Line:    match.Line,
			Column:  match.Column,
			Content: match.Content,
			Preview: match.Preview,
			Matches: matchRanges,
		})
	}

	c.JSON(http.StatusOK, SearchContentResponse{
		Query:   query,
		Results: results,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	})
}

// searchFiles searches for files by name
func (s *Server) searchFiles(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Parse query parameters
	query := c.Query("query")
	if query == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "query parameter is required",
			Code:  "MISSING_QUERY",
		})
		return
	}

	ref := c.Query("ref")
	caseSensitive := c.Query("case_sensitive") == "true"
	regex := c.Query("regex") == "true"

	limit := 50 // default
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Perform search
	matches, total, err := repo.SearchFiles(query, ref, caseSensitive, regex, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("search failed: %v", err),
			Code:  "SEARCH_FAILED",
		})
		return
	}

	// Convert to API response format
	results := make([]FileMatch, 0)
	for _, match := range matches {
		var matchRanges []MatchRange
		for _, mr := range match.Matches {
			matchRanges = append(matchRanges, MatchRange{
				Start: mr.Start,
				End:   mr.End,
			})
		}

		results = append(results, FileMatch{
			Path:    match.Path,
			Ref:     match.Ref,
			Size:    match.Size,
			Mode:    match.Mode,
			Matches: matchRanges,
		})
	}

	c.JSON(http.StatusOK, SearchFilesResponse{
		Query:   query,
		Results: results,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	})
}

// grep performs advanced pattern matching similar to git grep
func (s *Server) grep(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	var req GrepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Set defaults
	if req.Limit == 0 {
		req.Limit = 50
	}

	// Perform grep
	matches, total, err := repo.Grep(
		req.Pattern, req.Path, req.Ref,
		req.CaseSensitive, req.Regex, req.InvertMatch,
		req.WordRegexp, req.LineRegexp,
		req.ContextBefore, req.ContextAfter, req.Context,
		req.MaxCount, req.Limit, req.Offset,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("grep failed: %v", err),
			Code:  "GREP_FAILED",
		})
		return
	}

	// Convert to API response format
	var results []GrepMatch
	for _, match := range matches {
		var matchRanges []MatchRange
		for _, mr := range match.Matches {
			matchRanges = append(matchRanges, MatchRange{
				Start: mr.Start,
				End:   mr.End,
			})
		}

		results = append(results, GrepMatch{
			Path:    match.Path,
			Ref:     match.Ref,
			Line:    match.Line,
			Column:  match.Column,
			Content: match.Content,
			Before:  match.Before,
			After:   match.After,
			Matches: matchRanges,
		})
	}

	c.JSON(http.StatusOK, GrepResponse{
		Pattern: req.Pattern,
		Results: results,
		Total:   total,
		Limit:   req.Limit,
		Offset:  req.Offset,
	})
}

// Advanced Search API Handlers

// fullTextSearch performs advanced full-text search with TF-IDF scoring
func (s *Server) fullTextSearch(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Parse request body
	var req struct {
		Query           string   `json:"query" binding:"required"`
		FileTypes       []string `json:"file_types,omitempty"`
		MaxSize         int64    `json:"max_size,omitempty"`
		MinScore        float64  `json:"min_score,omitempty"`
		IncludeContent  bool     `json:"include_content,omitempty"`
		HighlightLength int      `json:"highlight_length,omitempty"`
		Limit           int      `json:"limit,omitempty"`
		Offset          int      `json:"offset,omitempty"`
		SortBy          string   `json:"sort_by,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("invalid request: %v", err),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Build search request
	searchReq := &govc.FullTextSearchRequest{
		Query:           req.Query,
		FileTypes:       req.FileTypes,
		MaxSize:         req.MaxSize,
		MinScore:        req.MinScore,
		IncludeContent:  req.IncludeContent,
		HighlightLength: req.HighlightLength,
		Limit:           req.Limit,
		Offset:          req.Offset,
		SortBy:          req.SortBy,
	}

	// Perform search
	response, err := repo.FullTextSearch(searchReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("search failed: %v", err),
			Code:  "SEARCH_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// sqlQuery executes SQL-like queries
func (s *Server) sqlQuery(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Parse request body
	var req struct {
		Query string `json:"query" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("invalid request: %v", err),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Execute SQL query
	result, err := repo.ExecuteSQLQuery(req.Query)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("query failed: %v", err),
			Code:  "QUERY_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// searchWithAggregation performs search with aggregation analytics
func (s *Server) searchWithAggregation(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Parse request body
	var req struct {
		Query        govc.FullTextSearchRequest `json:"query" binding:"required"`
		GroupBy      []string                   `json:"group_by"`
		Aggregations []string                   `json:"aggregations"`
		TimeRange    string                     `json:"time_range,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("invalid request: %v", err),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Build aggregation request
	aggReq := &govc.AggregationRequest{
		Query:        &req.Query,
		GroupBy:      req.GroupBy,
		Aggregations: req.Aggregations,
		TimeRange:    req.TimeRange,
	}

	// Perform aggregation
	response, err := repo.SearchWithAggregation(aggReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("aggregation failed: %v", err),
			Code:  "AGGREGATION_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// getSearchIndexStatistics returns search index statistics
func (s *Server) getSearchIndexStatistics(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get statistics
	stats, err := repo.GetSearchIndexStatistics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to get statistics: %v", err),
			Code:  "STATS_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// getSearchSuggestions provides query auto-completion suggestions
func (s *Server) getSearchSuggestions(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Parse query parameters
	partialQuery := c.Query("q")
	if partialQuery == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "query parameter 'q' is required",
			Code:  "MISSING_QUERY",
		})
		return
	}

	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid limit parameter",
			Code:  "INVALID_LIMIT",
		})
		return
	}

	// Get suggestions
	suggestions, err := repo.GetSearchSuggestions(partialQuery, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to get suggestions: %v", err),
			Code:  "SUGGESTIONS_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, map[string]interface{}{
		"query":       partialQuery,
		"suggestions": suggestions,
		"limit":       limit,
	})
}

// rebuildSearchIndex forces a rebuild of the search index
func (s *Server) rebuildSearchIndex(c *gin.Context) {
	repoID := c.Param("repo_id")
	repo, err := s.getRepository(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Rebuild index
	err = repo.RebuildSearchIndex()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to rebuild index: %v", err),
			Code:  "REBUILD_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Search index rebuild initiated",
		"status":  "success",
	})
}
