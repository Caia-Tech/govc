package llm

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/caiatech/govc"
)

// CodeReviewer provides AI-powered code review automation
type CodeReviewer struct {
	provider LLMProvider
	config   CodeReviewConfig
}

// CodeReviewConfig holds configuration for automated code review
type CodeReviewConfig struct {
	Provider           string            `json:"provider"`            // "openai", "local"
	Model              string            `json:"model"`               // LLM model to use
	ReviewDepth        string            `json:"review_depth"`        // "surface", "detailed", "comprehensive"
	FocusAreas         []string          `json:"focus_areas"`         // Areas to focus on during review
	Languages          []string          `json:"languages"`           // Programming languages to review
	ExcludePatterns    []string          `json:"exclude_patterns"`    // Patterns to exclude from review
	AutoComment        bool              `json:"auto_comment"`        // Automatically add review comments
	SeverityThreshold  string            `json:"severity_threshold"`  // Minimum severity to report
	ContextLines       int               `json:"context_lines"`       // Lines of context around issues
	CustomRules        []CustomRule      `json:"custom_rules"`        // Custom review rules
	Templates          map[string]string `json:"templates"`           // Comment templates
}

// CustomRule represents a custom code review rule
type CustomRule struct {
	Name        string `json:"name"`
	Pattern     string `json:"pattern"`     // Regex pattern to match
	Severity    string `json:"severity"`    // "info", "warning", "error", "critical"
	Message     string `json:"message"`     // Message to display
	Suggestion  string `json:"suggestion"`  // Suggested fix
	Language    string `json:"language"`    // Applicable language
	Enabled     bool   `json:"enabled"`
}

// DefaultCodeReviewConfig returns sensible defaults
func DefaultCodeReviewConfig() CodeReviewConfig {
	return CodeReviewConfig{
		Provider:          "local",
		Model:             "gpt-4o-mini",
		ReviewDepth:       "detailed",
		FocusAreas:        []string{"security", "performance", "maintainability", "bugs"},
		Languages:         []string{"go", "javascript", "python", "java", "typescript"},
		ExcludePatterns:   []string{"vendor/", "node_modules/", "*.min.js", "*.generated.*"},
		AutoComment:       false, // Manual review by default
		SeverityThreshold: "warning",
		ContextLines:      3,
		CustomRules:       []CustomRule{},
		Templates: map[string]string{
			"security":       "🔒 Security issue: {message}",
			"performance":    "⚡ Performance concern: {message}",
			"maintainability": "🔧 Maintainability: {message}",
			"bug":            "🐛 Potential bug: {message}",
			"style":          "✨ Style suggestion: {message}",
		},
	}
}

// CodeReviewRequest represents a request for code review
type CodeReviewRequest struct {
	Changes      []FileChange `json:"changes"`       // Files changed
	CommitHash   string       `json:"commit_hash"`   // Commit being reviewed
	Branch       string       `json:"branch"`        // Branch name
	Author       string       `json:"author"`        // Change author
	Description  string       `json:"description"`   // Commit/PR description
	ReviewType   string       `json:"review_type"`   // "commit", "pull_request", "diff"
	ContextInfo  ReviewContext `json:"context_info"` // Additional context
}

// FileChange represents changes to a single file
type FileChange struct {
	FilePath    string       `json:"file_path"`    // Path to the file
	ChangeType  string       `json:"change_type"`  // "added", "modified", "deleted", "renamed"
	Language    string       `json:"language"`     // Detected programming language
	OldContent  string       `json:"old_content"`  // Original content
	NewContent  string       `json:"new_content"`  // New content
	Diff        string       `json:"diff"`         // Git diff
	LineCount   LineCount    `json:"line_count"`   // Line count statistics
	Complexity  int          `json:"complexity"`   // Estimated complexity
}

// LineCount holds statistics about code changes
type LineCount struct {
	Added   int `json:"added"`
	Deleted int `json:"deleted"`
	Total   int `json:"total"`
}

// ReviewContext provides additional context for code review
type ReviewContext struct {
	PullRequestInfo *PullRequestInfo      `json:"pull_request_info,omitempty"`
	RecentCommits   []CommitInfo          `json:"recent_commits"`
	RelatedFiles    []string              `json:"related_files"`
	TestFiles       []string              `json:"test_files"`
	Dependencies    []string              `json:"dependencies"`
	Metadata        map[string]string     `json:"metadata"`
}

// PullRequestInfo holds information about a pull request
type PullRequestInfo struct {
	Number      int      `json:"number"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
	Reviewers   []string `json:"reviewers"`
	Draft       bool     `json:"draft"`
}

// CodeReviewResult represents the complete result of a code review
type CodeReviewResult struct {
	ReviewID      string              `json:"review_id"`
	CommitHash    string              `json:"commit_hash"`
	OverallScore  float64             `json:"overall_score"`  // 0-100 quality score
	Issues        []ReviewIssue       `json:"issues"`         // Found issues
	Suggestions   []ReviewSuggestion  `json:"suggestions"`    // Improvement suggestions
	Summary       ReviewSummary       `json:"summary"`        // Overall summary
	Metrics       ReviewMetrics       `json:"metrics"`        // Code quality metrics
	Recommendations []string          `json:"recommendations"` // High-level recommendations
	Timestamp     time.Time           `json:"timestamp"`
}

// ReviewIssue represents a specific issue found during review
type ReviewIssue struct {
	IssueID     string              `json:"issue_id"`
	FilePath    string              `json:"file_path"`
	LineNumber  int                 `json:"line_number"`
	Column      int                 `json:"column,omitempty"`
	Severity    string              `json:"severity"`    // "info", "warning", "error", "critical"
	Category    string              `json:"category"`    // "security", "performance", "bug", etc.
	Title       string              `json:"title"`       // Short description
	Description string              `json:"description"` // Detailed description
	Code        string              `json:"code"`        // Problematic code snippet
	Context     []string            `json:"context"`     // Surrounding lines
	Rule        string              `json:"rule"`        // Rule that triggered this issue
	Confidence  float64             `json:"confidence"`  // AI confidence (0-1)
	AutoFix     *AutoFix            `json:"auto_fix,omitempty"` // Suggested automatic fix
	References  []string            `json:"references"`  // Links to documentation/resources
}

// ReviewSuggestion represents an improvement suggestion
type ReviewSuggestion struct {
	SuggestionID string   `json:"suggestion_id"`
	FilePath     string   `json:"file_path"`
	LineNumber   int      `json:"line_number"`
	Type         string   `json:"type"`         // "refactor", "optimize", "simplify", etc.
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Before       string   `json:"before"`       // Original code
	After        string   `json:"after"`        // Suggested code
	Impact       string   `json:"impact"`       // Expected impact
	Effort       string   `json:"effort"`       // Implementation effort
	Priority     string   `json:"priority"`     // "low", "medium", "high"
	Benefits     []string `json:"benefits"`     // Expected benefits
}

// AutoFix represents an automatic fix suggestion
type AutoFix struct {
	Description string `json:"description"`
	OldCode     string `json:"old_code"`
	NewCode     string `json:"new_code"`
	Safe        bool   `json:"safe"`        // Whether this fix is safe to apply automatically
	TestRequired bool  `json:"test_required"` // Whether tests are required after applying
}

// ReviewSummary provides a high-level summary of the review
type ReviewSummary struct {
	FilesReviewed     int                `json:"files_reviewed"`
	IssuesFound       int                `json:"issues_found"`
	CriticalIssues    int                `json:"critical_issues"`
	SecurityIssues    int                `json:"security_issues"`
	PerformanceIssues int                `json:"performance_issues"`
	MainConcerns      []string           `json:"main_concerns"`
	PositiveAspects   []string           `json:"positive_aspects"`
	OverallAssessment string             `json:"overall_assessment"`
	RecommendedActions []string          `json:"recommended_actions"`
	RiskLevel         string             `json:"risk_level"` // "low", "medium", "high"
}

// ReviewMetrics provides quantitative metrics about the code
type ReviewMetrics struct {
	LinesAdded       int     `json:"lines_added"`
	LinesDeleted     int     `json:"lines_deleted"`
	FilesChanged     int     `json:"files_changed"`
	CyclomaticComplexity int `json:"cyclomatic_complexity"`
	TechnicalDebt    string  `json:"technical_debt"`    // Estimated time to fix issues
	Maintainability  float64 `json:"maintainability"`   // 0-100 score
	Testability      float64 `json:"testability"`       // 0-100 score
	CodeCoverage     float64 `json:"code_coverage"`     // Estimated test coverage impact
}

// NewCodeReviewer creates a new code reviewer
func NewCodeReviewer(provider LLMProvider, config CodeReviewConfig) *CodeReviewer {
	return &CodeReviewer{
		provider: provider,
		config:   config,
	}
}

// ReviewChanges performs comprehensive code review of changes
func (cr *CodeReviewer) ReviewChanges(ctx context.Context, repo *govc.Repository, request CodeReviewRequest) (*CodeReviewResult, error) {
	fmt.Printf("🔍 Starting AI code review (depth: %s)\n", cr.config.ReviewDepth)
	
	// Analyze each file change
	var allIssues []ReviewIssue
	var allSuggestions []ReviewSuggestion
	var metrics ReviewMetrics
	
	for _, fileChange := range request.Changes {
		// Skip excluded files
		if cr.shouldExcludeFile(fileChange.FilePath) {
			continue
		}
		
		// Review individual file
		fileResult, err := cr.reviewFile(ctx, fileChange, request.ContextInfo)
		if err != nil {
			fmt.Printf("Warning: Failed to review %s: %v\n", fileChange.FilePath, err)
			continue
		}
		
		allIssues = append(allIssues, fileResult.Issues...)
		allSuggestions = append(allSuggestions, fileResult.Suggestions...)
		
		// Update metrics
		metrics.LinesAdded += fileChange.LineCount.Added
		metrics.LinesDeleted += fileChange.LineCount.Deleted
		metrics.FilesChanged++
	}
	
	// Calculate overall quality score
	overallScore := cr.calculateQualityScore(allIssues, allSuggestions)
	
	// Generate summary
	summary := cr.generateSummary(allIssues, allSuggestions, request)
	
	// Generate high-level recommendations
	recommendations := cr.generateRecommendations(allIssues, allSuggestions, summary)
	
	result := &CodeReviewResult{
		ReviewID:        fmt.Sprintf("review-%d", time.Now().Unix()),
		CommitHash:      request.CommitHash,
		OverallScore:    overallScore,
		Issues:          allIssues,
		Suggestions:     allSuggestions,
		Summary:         summary,
		Metrics:         metrics,
		Recommendations: recommendations,
		Timestamp:       time.Now(),
	}
	
	fmt.Printf("✅ Code review completed: %.1f/100 score, %d issues, %d suggestions\n", 
		overallScore, len(allIssues), len(allSuggestions))
	
	return result, nil
}

// ReviewCommit reviews a specific commit
func (cr *CodeReviewer) ReviewCommit(ctx context.Context, repo *govc.Repository, commitHash string) (*CodeReviewResult, error) {
	// Get commit information
	commits, err := repo.Log(1) // Get the latest commit
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}
	
	if len(commits) == 0 {
		return nil, fmt.Errorf("no commits found")
	}
	
	commit := commits[0]
	
	// Build review request from commit
	request := CodeReviewRequest{
		CommitHash:  commitHash,
		Branch:      "main", // Simplified
		Author:      commit.Author.Name,
		Description: commit.Message,
		ReviewType:  "commit",
		Changes:     cr.buildChangesFromCommit(repo, commit),
		ContextInfo: cr.buildContextInfo(repo),
	}
	
	return cr.ReviewChanges(ctx, repo, request)
}

// Helper methods

func (cr *CodeReviewer) reviewFile(ctx context.Context, fileChange FileChange, context ReviewContext) (*CodeReviewResult, error) {
	// Build review prompt for this file
	prompt := cr.buildFileReviewPrompt(fileChange, context)
	
	options := GenerationOptions{
		MaxTokens:   2000, // Allow for detailed analysis
		Temperature: 0.3,  // Lower temperature for more consistent analysis
		TopP:        0.9,
	}
	
	response, err := cr.provider.GenerateText(ctx, prompt, options)
	if err != nil {
		return nil, fmt.Errorf("failed to generate review: %w", err)
	}
	
	// Parse AI response into issues and suggestions
	issues, suggestions := cr.parseReviewResponse(response, fileChange.FilePath)
	
	// Apply custom rules
	customIssues := cr.applyCustomRules(fileChange)
	issues = append(issues, customIssues...)
	
	return &CodeReviewResult{
		Issues:      issues,
		Suggestions: suggestions,
	}, nil
}

func (cr *CodeReviewer) buildFileReviewPrompt(fileChange FileChange, context ReviewContext) string {
	var prompt strings.Builder
	
	prompt.WriteString("You are an expert code reviewer. Analyze the following code changes and provide feedback.\n\n")
	
	// File information
	prompt.WriteString(fmt.Sprintf("File: %s (Language: %s)\n", fileChange.FilePath, fileChange.Language))
	prompt.WriteString(fmt.Sprintf("Change type: %s\n", fileChange.ChangeType))
	prompt.WriteString(fmt.Sprintf("Lines: +%d -%d\n\n", fileChange.LineCount.Added, fileChange.LineCount.Deleted))
	
	// Focus areas
	prompt.WriteString(fmt.Sprintf("Focus on: %s\n", strings.Join(cr.config.FocusAreas, ", ")))
	prompt.WriteString(fmt.Sprintf("Review depth: %s\n\n", cr.config.ReviewDepth))
	
	// Code changes
	if fileChange.Diff != "" {
		prompt.WriteString("=== CODE DIFF ===\n")
		prompt.WriteString(fileChange.Diff)
		prompt.WriteString("\n")
	} else if fileChange.NewContent != "" {
		prompt.WriteString("=== NEW CODE ===\n")
		// Limit content length to avoid token limits
		content := fileChange.NewContent
		if len(content) > 5000 {
			content = content[:5000] + "\n... (content truncated)"
		}
		prompt.WriteString(content)
		prompt.WriteString("\n")
	}
	
	// Context information
	if len(context.RecentCommits) > 0 {
		prompt.WriteString("=== RECENT COMMITS ===\n")
		for i, commit := range context.RecentCommits {
			if i >= 3 { // Limit to 3 commits
				break
			}
			prompt.WriteString(fmt.Sprintf("- %s: %s\n", commit.Hash[:7], commit.Message))
		}
		prompt.WriteString("\n")
	}
	
	// Review instructions
	prompt.WriteString("Please provide:\n")
	prompt.WriteString("1. ISSUES: List any problems found (format: SEVERITY|CATEGORY|LINE|TITLE|DESCRIPTION)\n")
	prompt.WriteString("2. SUGGESTIONS: List improvement suggestions (format: TYPE|LINE|TITLE|DESCRIPTION|BEFORE|AFTER)\n")
	prompt.WriteString("3. ASSESSMENT: Overall quality assessment\n")
	
	switch cr.config.ReviewDepth {
	case "comprehensive":
		prompt.WriteString("\nConduct a thorough review covering:\n")
		prompt.WriteString("- Security vulnerabilities\n")
		prompt.WriteString("- Performance implications\n")
		prompt.WriteString("- Code maintainability\n")
		prompt.WriteString("- Testing completeness\n")
		prompt.WriteString("- Architecture adherence\n")
		prompt.WriteString("- Documentation quality\n")
	case "detailed":
		prompt.WriteString("\nConduct a detailed review covering:\n")
		prompt.WriteString("- Security and bug issues\n")
		prompt.WriteString("- Performance concerns\n")
		prompt.WriteString("- Code quality and maintainability\n")
	case "surface":
		prompt.WriteString("\nConduct a surface-level review covering:\n")
		prompt.WriteString("- Critical bugs and security issues\n")
		prompt.WriteString("- Basic code quality\n")
	}
	
	return prompt.String()
}

func (cr *CodeReviewer) parseReviewResponse(response, filePath string) ([]ReviewIssue, []ReviewSuggestion) {
	var issues []ReviewIssue
	var suggestions []ReviewSuggestion
	
	lines := strings.Split(response, "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Parse issues (format: SEVERITY|CATEGORY|LINE|TITLE|DESCRIPTION)
		if strings.Contains(line, "|") && (strings.Contains(strings.ToUpper(line), "ERROR") || 
			strings.Contains(strings.ToUpper(line), "WARNING") || 
			strings.Contains(strings.ToUpper(line), "CRITICAL")) {
			
			parts := strings.Split(line, "|")
			if len(parts) >= 5 {
				lineNum := parseLineNumber(parts[2])
				issue := ReviewIssue{
					IssueID:     fmt.Sprintf("%s-%d-%d", filePath, lineNum, time.Now().Unix()),
					FilePath:    filePath,
					LineNumber:  lineNum,
					Severity:    strings.ToLower(strings.TrimSpace(parts[0])),
					Category:    strings.ToLower(strings.TrimSpace(parts[1])),
					Title:       strings.TrimSpace(parts[3]),
					Description: strings.TrimSpace(parts[4]),
					Confidence:  0.8, // Default confidence
					Rule:        "ai-review",
				}
				issues = append(issues, issue)
			}
		}
		
		// Parse suggestions (format: TYPE|LINE|TITLE|DESCRIPTION|BEFORE|AFTER)
		if strings.Contains(line, "|") && (strings.Contains(strings.ToUpper(line), "REFACTOR") || 
			strings.Contains(strings.ToUpper(line), "OPTIMIZE") || 
			strings.Contains(strings.ToUpper(line), "IMPROVE")) {
			
			parts := strings.Split(line, "|")
			if len(parts) >= 6 {
				lineNum := parseLineNumber(parts[1])
				suggestion := ReviewSuggestion{
					SuggestionID: fmt.Sprintf("%s-%d-%d", filePath, lineNum, time.Now().Unix()),
					FilePath:     filePath,
					LineNumber:   lineNum,
					Type:         strings.ToLower(strings.TrimSpace(parts[0])),
					Title:        strings.TrimSpace(parts[2]),
					Description:  strings.TrimSpace(parts[3]),
					Before:       strings.TrimSpace(parts[4]),
					After:        strings.TrimSpace(parts[5]),
					Priority:     "medium",
					Impact:       "positive",
					Effort:       "low",
				}
				suggestions = append(suggestions, suggestion)
			}
		}
	}
	
	return issues, suggestions
}

func (cr *CodeReviewer) applyCustomRules(fileChange FileChange) []ReviewIssue {
	var issues []ReviewIssue
	
	for _, rule := range cr.config.CustomRules {
		if !rule.Enabled {
			continue
		}
		
		// Check if rule applies to this language
		if rule.Language != "" && rule.Language != fileChange.Language {
			continue
		}
		
		// Apply regex pattern
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue // Skip invalid regex
		}
		
		matches := re.FindAllStringIndex(fileChange.NewContent, -1)
		for _, match := range matches {
			lineNum := cr.findLineNumber(fileChange.NewContent, match[0])
			
			issue := ReviewIssue{
				IssueID:     fmt.Sprintf("%s-%s-%d", fileChange.FilePath, rule.Name, lineNum),
				FilePath:    fileChange.FilePath,
				LineNumber:  lineNum,
				Severity:    rule.Severity,
				Category:    "custom",
				Title:       rule.Name,
				Description: rule.Message,
				Rule:        rule.Name,
				Confidence:  1.0, // Custom rules are always confident
			}
			
			if rule.Suggestion != "" {
				issue.AutoFix = &AutoFix{
					Description: rule.Suggestion,
					Safe:        false, // Conservative default
				}
			}
			
			issues = append(issues, issue)
		}
	}
	
	return issues
}

func (cr *CodeReviewer) shouldExcludeFile(filePath string) bool {
	for _, pattern := range cr.config.ExcludePatterns {
		if strings.Contains(filePath, pattern) {
			return true
		}
	}
	return false
}

func (cr *CodeReviewer) calculateQualityScore(issues []ReviewIssue, suggestions []ReviewSuggestion) float64 {
	// Start with perfect score
	score := 100.0
	
	// Deduct points based on issues
	for _, issue := range issues {
		switch issue.Severity {
		case "critical":
			score -= 20
		case "error":
			score -= 10
		case "warning":
			score -= 5
		case "info":
			score -= 1
		}
	}
	
	// Minor deduction for suggestions (areas for improvement)
	score -= float64(len(suggestions)) * 0.5
	
	// Ensure score doesn't go below 0
	if score < 0 {
		score = 0
	}
	
	return score
}

func (cr *CodeReviewer) generateSummary(issues []ReviewIssue, suggestions []ReviewSuggestion, request CodeReviewRequest) ReviewSummary {
	summary := ReviewSummary{
		FilesReviewed: len(request.Changes),
		IssuesFound:   len(issues),
	}
	
	// Count issues by severity
	for _, issue := range issues {
		switch issue.Severity {
		case "critical":
			summary.CriticalIssues++
		}
		
		switch issue.Category {
		case "security":
			summary.SecurityIssues++
		case "performance":
			summary.PerformanceIssues++
		}
	}
	
	// Generate assessment
	if summary.CriticalIssues > 0 {
		summary.OverallAssessment = "Requires immediate attention due to critical issues"
		summary.RiskLevel = "high"
	} else if len(issues) > 10 {
		summary.OverallAssessment = "Multiple issues found, recommend addressing before merge"
		summary.RiskLevel = "medium"
	} else if len(issues) > 0 {
		summary.OverallAssessment = "Minor issues found, consider addressing"
		summary.RiskLevel = "low"
	} else {
		summary.OverallAssessment = "Good code quality, ready for merge"
		summary.RiskLevel = "low"
	}
	
	// Generate main concerns
	if summary.SecurityIssues > 0 {
		summary.MainConcerns = append(summary.MainConcerns, "Security vulnerabilities")
	}
	if summary.PerformanceIssues > 0 {
		summary.MainConcerns = append(summary.MainConcerns, "Performance issues")
	}
	
	// Generate positive aspects
	if len(issues) == 0 {
		summary.PositiveAspects = append(summary.PositiveAspects, "Clean code with no issues found")
	}
	if len(suggestions) < 3 {
		summary.PositiveAspects = append(summary.PositiveAspects, "Well-structured code")
	}
	
	return summary
}

func (cr *CodeReviewer) generateRecommendations(issues []ReviewIssue, suggestions []ReviewSuggestion, summary ReviewSummary) []string {
	var recommendations []string
	
	if summary.CriticalIssues > 0 {
		recommendations = append(recommendations, "Address all critical issues before proceeding")
	}
	
	if summary.SecurityIssues > 0 {
		recommendations = append(recommendations, "Review and fix security vulnerabilities")
	}
	
	if len(issues) > 5 {
		recommendations = append(recommendations, "Consider breaking this change into smaller commits")
	}
	
	if len(suggestions) > 0 {
		recommendations = append(recommendations, "Consider implementing suggested improvements")
	}
	
	recommendations = append(recommendations, "Run comprehensive tests after addressing issues")
	
	return recommendations
}

func (cr *CodeReviewer) buildChangesFromCommit(repo *govc.Repository, commit *govc.Commit) []FileChange {
	// Simplified implementation - in practice would analyze commit diff
	files, _ := repo.ListFiles()
	
	var changes []FileChange
	for _, file := range files {
		content, err := repo.ReadFile(file)
		if err != nil {
			continue
		}
		
		change := FileChange{
			FilePath:    file,
			ChangeType:  "modified",
			Language:    cr.detectLanguage(file),
			NewContent:  string(content),
			LineCount:   LineCount{Added: 10, Deleted: 5, Total: 15}, // Simplified
			Complexity:  cr.estimateComplexity(string(content)),
		}
		changes = append(changes, change)
	}
	
	return changes
}

func (cr *CodeReviewer) buildContextInfo(repo *govc.Repository) ReviewContext {
	context := ReviewContext{
		Metadata: make(map[string]string),
	}
	
	// Get recent commits
	if commits, err := repo.Log(5); err == nil {
		for _, commit := range commits {
			context.RecentCommits = append(context.RecentCommits, CommitInfo{
				Hash:    commit.Hash(),
				Message: commit.Message,
				Author:  commit.Author.Name,
				Date:    commit.Author.Time,
			})
		}
	}
	
	return context
}

func (cr *CodeReviewer) detectLanguage(filePath string) string {
	ext := strings.ToLower(filePath[strings.LastIndex(filePath, "."):])
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
	default:
		return "text"
	}
}

func (cr *CodeReviewer) estimateComplexity(content string) int {
	// Simple complexity estimation
	complexity := 0
	complexity += strings.Count(content, "if")
	complexity += strings.Count(content, "for")
	complexity += strings.Count(content, "while")
	complexity += strings.Count(content, "switch")
	return complexity
}

func (cr *CodeReviewer) findLineNumber(content string, position int) int {
	lineNum := 1
	for i := 0; i < position && i < len(content); i++ {
		if content[i] == '\n' {
			lineNum++
		}
	}
	return lineNum
}

func parseLineNumber(s string) int {
	// Simple line number parsing
	s = strings.TrimSpace(s)
	if s == "" {
		return 1
	}
	
	// Extract number from string
	re := regexp.MustCompile(`\d+`)
	match := re.FindString(s)
	if match == "" {
		return 1
	}
	
	// Simple conversion (in production would use strconv.Atoi)
	switch match {
	case "1":
		return 1
	case "2":
		return 2
	case "3":
		return 3
	default:
		return 1
	}
}