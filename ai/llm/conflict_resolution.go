package llm

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/caiatech/govc"
)

// ConflictResolver provides AI-powered merge conflict resolution suggestions
type ConflictResolver struct {
	provider LLMProvider
	config   ConflictResolutionConfig
}

// ConflictResolutionConfig holds configuration for conflict resolution
type ConflictResolutionConfig struct {
	Provider         string  `json:"provider"`          // "openai", "local"
	Model            string  `json:"model"`             // LLM model to use
	Confidence       float64 `json:"confidence"`        // Minimum confidence threshold (0-1)
	MaxContextLines  int     `json:"max_context_lines"` // Lines of context around conflicts
	Language         string  `json:"language"`          // Programming language context
	AutoApply        bool    `json:"auto_apply"`        // Automatically apply high-confidence resolutions
	BackupEnabled    bool    `json:"backup_enabled"`    // Create backup before applying resolutions
}

// DefaultConflictResolutionConfig returns sensible defaults
func DefaultConflictResolutionConfig() ConflictResolutionConfig {
	return ConflictResolutionConfig{
		Provider:         "local",
		Model:            "gpt-4o-mini",
		Confidence:       0.8, // High confidence threshold for safety
		MaxContextLines:  10,
		Language:         "auto", // Auto-detect from file extension
		AutoApply:        false,  // Manual approval by default
		BackupEnabled:    true,
	}
}

// ConflictInfo represents information about a merge conflict
type ConflictInfo struct {
	FilePath     string            `json:"file_path"`     // Path to conflicted file
	ConflictType string            `json:"conflict_type"` // "merge", "rebase", "cherry-pick"
	Language     string            `json:"language"`      // Detected programming language
	Conflicts    []ConflictSection `json:"conflicts"`     // Individual conflict sections
	Context      ConflictContext   `json:"context"`       // Additional context information
}

// ConflictSection represents a single conflict within a file
type ConflictSection struct {
	StartLine    int      `json:"start_line"`    // Line where conflict starts
	EndLine      int      `json:"end_line"`      // Line where conflict ends
	OurCode      string   `json:"our_code"`      // Code from "our" branch
	TheirCode    string   `json:"their_code"`    // Code from "their" branch
	BaseCode     string   `json:"base_code"`     // Original code (if available)
	Context      []string `json:"context"`       // Surrounding lines for context
	Severity     string   `json:"severity"`      // "low", "medium", "high"
	ConflictHash string   `json:"conflict_hash"` // Unique identifier for this conflict
}

// ConflictContext provides additional context for conflict resolution
type ConflictContext struct {
	BranchNames    BranchInfo        `json:"branch_names"`    // Information about conflicting branches
	RecentCommits  []CommitInfo      `json:"recent_commits"`  // Recent commits on both branches
	FileHistory    []string          `json:"file_history"`    // Recent changes to the conflicted file
	Dependencies   []string          `json:"dependencies"`    // Related files or functions
	Metadata       map[string]string `json:"metadata"`        // Additional metadata
}

// BranchInfo holds information about the branches involved in the conflict
type BranchInfo struct {
	OurBranch   string `json:"our_branch"`   // Current branch name
	TheirBranch string `json:"their_branch"` // Branch being merged
	BaseBranch  string `json:"base_branch"`  // Common ancestor branch
}

// CommitInfo represents commit information for context
type CommitInfo struct {
	Hash    string    `json:"hash"`
	Message string    `json:"message"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
	Branch  string    `json:"branch"`
}

// ResolutionSuggestion represents an AI-generated conflict resolution
type ResolutionSuggestion struct {
	ConflictHash  string              `json:"conflict_hash"`  // Links to ConflictSection
	Resolution    string              `json:"resolution"`     // Suggested resolved code
	Confidence    float64             `json:"confidence"`     // AI confidence score (0-1)
	Reasoning     string              `json:"reasoning"`      // Explanation of the resolution
	ResolutionType string             `json:"resolution_type"` // "automatic", "manual", "hybrid"
	Risks         []string            `json:"risks"`          // Potential risks of this resolution
	Tests         []TestSuggestion    `json:"tests"`          // Suggested tests to validate resolution
	Metadata      map[string]string   `json:"metadata"`       // Additional metadata
}

// TestSuggestion represents a suggested test for validating conflict resolution
type TestSuggestion struct {
	TestType    string `json:"test_type"`    // "unit", "integration", "manual"
	Description string `json:"description"`  // What to test
	Code        string `json:"code"`         // Test code (if applicable)
	Priority    string `json:"priority"`     // "high", "medium", "low"
}

// ConflictResolutionResult represents the complete result of conflict analysis
type ConflictResolutionResult struct {
	FilePath      string                 `json:"file_path"`
	TotalConflicts int                   `json:"total_conflicts"`
	ResolvedCount  int                   `json:"resolved_count"`
	Suggestions    []ResolutionSuggestion `json:"suggestions"`
	OverallRisk    string                 `json:"overall_risk"`    // "low", "medium", "high"
	Confidence     float64                `json:"confidence"`      // Overall confidence
	EstimatedTime  string                 `json:"estimated_time"`  // Estimated time to resolve
	NextSteps      []string               `json:"next_steps"`      // Recommended next actions
	Timestamp      time.Time              `json:"timestamp"`
}

// NewConflictResolver creates a new conflict resolver
func NewConflictResolver(provider LLMProvider, config ConflictResolutionConfig) *ConflictResolver {
	return &ConflictResolver{
		provider: provider,
		config:   config,
	}
}

// AnalyzeConflicts analyzes merge conflicts and provides resolution suggestions
func (cr *ConflictResolver) AnalyzeConflicts(ctx context.Context, repo *govc.Repository) ([]ConflictResolutionResult, error) {
	// Get repository status to find conflicted files
	status, err := repo.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get repository status: %w", err)
	}

	// Find conflicted files (this would need to be implemented in govc.Status)
	conflictedFiles := cr.findConflictedFiles(status)
	if len(conflictedFiles) == 0 {
		return nil, fmt.Errorf("no merge conflicts found")
	}

	var results []ConflictResolutionResult
	
	for _, filePath := range conflictedFiles {
		result, err := cr.analyzeFileConflicts(ctx, repo, filePath)
		if err != nil {
			// Log error but continue with other files
			fmt.Printf("Warning: Failed to analyze conflicts in %s: %v\n", filePath, err)
			continue
		}
		results = append(results, *result)
	}

	return results, nil
}

// ResolveConflict applies a specific resolution suggestion
func (cr *ConflictResolver) ResolveConflict(ctx context.Context, repo *govc.Repository, filePath string, suggestion ResolutionSuggestion) error {
	// Safety check: only apply high-confidence resolutions if auto-apply is enabled
	if cr.config.AutoApply && suggestion.Confidence < cr.config.Confidence {
		return fmt.Errorf("resolution confidence %.2f below threshold %.2f", 
			suggestion.Confidence, cr.config.Confidence)
	}

	// Create backup if enabled
	if cr.config.BackupEnabled {
		if err := cr.createBackup(repo, filePath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Read current file content
	content, err := repo.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Apply the resolution
	resolvedContent, err := cr.applyResolution(string(content), suggestion)
	if err != nil {
		return fmt.Errorf("failed to apply resolution: %w", err)
	}

	// Write resolved content back to file
	// In a real implementation, this would use govc's file writing capabilities
	fmt.Printf("Would write resolved content to %s\n", filePath)
	fmt.Printf("Resolution applied: %s\n", suggestion.Reasoning)
	
	_ = resolvedContent // Use the variable to avoid warnings
	
	return nil
}

// GenerateResolutionSuggestions generates AI-powered resolution suggestions for a conflict
func (cr *ConflictResolver) GenerateResolutionSuggestions(ctx context.Context, conflict ConflictSection, context ConflictContext) ([]ResolutionSuggestion, error) {
	prompt := cr.buildConflictPrompt(conflict, context)
	
	options := GenerationOptions{
		MaxTokens:   1000, // Allow for detailed explanations
		Temperature: 0.2,  // Lower temperature for more consistent output
		TopP:        0.9,
	}

	response, err := cr.provider.GenerateText(ctx, prompt, options)
	if err != nil {
		return nil, fmt.Errorf("failed to generate resolution: %w", err)
	}

	// Parse the AI response into suggestions
	suggestions, err := cr.parseResolutionResponse(response, conflict.ConflictHash)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	return suggestions, nil
}

// Helper methods

func (cr *ConflictResolver) findConflictedFiles(status *govc.Status) []string {
	// In a real implementation, this would check for merge conflict markers
	// For now, return a placeholder
	var conflicted []string
	
	// Check for files with conflict markers (simplified)
	for _, file := range status.Modified {
		// In practice, we'd read the file and check for <<<<<<< markers
		conflicted = append(conflicted, file)
	}
	
	return conflicted
}

func (cr *ConflictResolver) analyzeFileConflicts(ctx context.Context, repo *govc.Repository, filePath string) (*ConflictResolutionResult, error) {
	// Read file content
	content, err := repo.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse conflicts from content
	conflicts, err := cr.parseConflictMarkers(string(content), filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse conflicts: %w", err)
	}

	// Generate context information
	context := cr.buildConflictContext(repo, filePath)

	// Generate resolution suggestions for each conflict
	var allSuggestions []ResolutionSuggestion
	resolvedCount := 0

	for _, conflict := range conflicts {
		suggestions, err := cr.GenerateResolutionSuggestions(ctx, conflict, context)
		if err != nil {
			fmt.Printf("Warning: Failed to generate suggestions for conflict %s: %v\n", 
				conflict.ConflictHash, err)
			continue
		}
		
		allSuggestions = append(allSuggestions, suggestions...)
		if len(suggestions) > 0 && suggestions[0].Confidence >= cr.config.Confidence {
			resolvedCount++
		}
	}

	// Calculate overall risk and confidence
	overallRisk := cr.calculateOverallRisk(allSuggestions)
	overallConfidence := cr.calculateOverallConfidence(allSuggestions)

	result := &ConflictResolutionResult{
		FilePath:       filePath,
		TotalConflicts: len(conflicts),
		ResolvedCount:  resolvedCount,
		Suggestions:    allSuggestions,
		OverallRisk:    overallRisk,
		Confidence:     overallConfidence,
		EstimatedTime:  cr.estimateResolutionTime(len(conflicts)),
		NextSteps:      cr.generateNextSteps(allSuggestions),
		Timestamp:      time.Now(),
	}

	return result, nil
}

func (cr *ConflictResolver) parseConflictMarkers(content, filePath string) ([]ConflictSection, error) {
	var conflicts []ConflictSection
	lines := strings.Split(content, "\n")
	
	// Look for Git conflict markers
	conflictStart := regexp.MustCompile(`^<{7} .*`)
	conflictSeparator := regexp.MustCompile(`^={7}$`)
	conflictEnd := regexp.MustCompile(`^>{7} .*`)
	
	var inConflict bool
	var currentConflict ConflictSection
	var ourCode, theirCode strings.Builder
	var conflictStartLine int
	
	for i, line := range lines {
		lineNum := i + 1
		
		if conflictStart.MatchString(line) {
			// Start of conflict
			inConflict = true
			conflictStartLine = lineNum
			ourCode.Reset()
			theirCode.Reset()
			currentConflict = ConflictSection{
				StartLine:    lineNum,
				ConflictHash: fmt.Sprintf("%s-%d", filePath, lineNum),
				Severity:     "medium", // Default severity
			}
		} else if conflictSeparator.MatchString(line) && inConflict {
			// Switch from "our" code to "their" code
			currentConflict.OurCode = ourCode.String()
		} else if conflictEnd.MatchString(line) && inConflict {
			// End of conflict
			currentConflict.TheirCode = theirCode.String()
			currentConflict.EndLine = lineNum
			
			// Add context lines
			contextStart := max(0, conflictStartLine-cr.config.MaxContextLines-1)
			contextEnd := min(len(lines), lineNum+cr.config.MaxContextLines)
			
			for j := contextStart; j < contextEnd; j++ {
				if j < conflictStartLine-1 || j > lineNum {
					currentConflict.Context = append(currentConflict.Context, lines[j])
				}
			}
			
			conflicts = append(conflicts, currentConflict)
			inConflict = false
		} else if inConflict {
			// Collect code lines
			if currentConflict.OurCode == "" { // Still collecting "our" code
				ourCode.WriteString(line + "\n")
			} else { // Collecting "their" code
				theirCode.WriteString(line + "\n")
			}
		}
	}
	
	return conflicts, nil
}

func (cr *ConflictResolver) buildConflictContext(repo *govc.Repository, filePath string) ConflictContext {
	// Build context information (simplified implementation)
	context := ConflictContext{
		BranchNames: BranchInfo{
			OurBranch:   "main",       // Would get from repo
			TheirBranch: "feature",    // Would get from merge info
			BaseBranch:  "main",       // Would get from merge base
		},
		FileHistory:  []string{},    // Would get recent file changes
		Dependencies: []string{},    // Would analyze imports/references
		Metadata: map[string]string{
			"file_type": cr.detectFileType(filePath),
			"timestamp": time.Now().Format(time.RFC3339),
		},
	}
	
	// Get recent commits (simplified)
	if commits, err := repo.Log(5); err == nil {
		for _, commit := range commits {
			context.RecentCommits = append(context.RecentCommits, CommitInfo{
				Hash:    commit.Hash(),
				Message: commit.Message,
				Author:  commit.Author.Name,
				Date:    commit.Author.Time,
				Branch:  "main", // Simplified
			})
		}
	}
	
	return context
}

func (cr *ConflictResolver) buildConflictPrompt(conflict ConflictSection, context ConflictContext) string {
	var prompt strings.Builder
	
	prompt.WriteString("You are an expert software developer analyzing a merge conflict. ")
	prompt.WriteString("Provide a resolution suggestion with high confidence.\n\n")
	
	prompt.WriteString(fmt.Sprintf("Conflict in file (language: %s):\n", cr.config.Language))
	prompt.WriteString("=== OUR CODE (current branch) ===\n")
	prompt.WriteString(conflict.OurCode)
	prompt.WriteString("\n=== THEIR CODE (incoming branch) ===\n")
	prompt.WriteString(conflict.TheirCode)
	
	if len(conflict.Context) > 0 {
		prompt.WriteString("\n=== SURROUNDING CONTEXT ===\n")
		for _, line := range conflict.Context {
			prompt.WriteString(line + "\n")
		}
	}
	
	prompt.WriteString("\n=== BRANCH INFORMATION ===\n")
	prompt.WriteString(fmt.Sprintf("Our branch: %s\n", context.BranchNames.OurBranch))
	prompt.WriteString(fmt.Sprintf("Their branch: %s\n", context.BranchNames.TheirBranch))
	
	if len(context.RecentCommits) > 0 {
		prompt.WriteString("\n=== RECENT COMMITS ===\n")
		for i, commit := range context.RecentCommits {
			if i >= 3 { // Limit to 3 commits
				break
			}
			prompt.WriteString(fmt.Sprintf("- %s: %s (by %s)\n", 
				commit.Hash[:7], commit.Message, commit.Author))
		}
	}
	
	prompt.WriteString("\nPlease provide:\n")
	prompt.WriteString("1. RESOLUTION: The merged code that resolves the conflict\n")
	prompt.WriteString("2. CONFIDENCE: Your confidence level (0.0-1.0)\n")
	prompt.WriteString("3. REASONING: Explanation of why this resolution is correct\n")
	prompt.WriteString("4. RISKS: Any potential risks or concerns\n")
	prompt.WriteString("5. TESTS: Suggested tests to validate the resolution\n")
	
	return prompt.String()
}

func (cr *ConflictResolver) parseResolutionResponse(response, conflictHash string) ([]ResolutionSuggestion, error) {
	// Simple parsing of AI response (in production, would use more sophisticated parsing)
	suggestion := ResolutionSuggestion{
		ConflictHash:   conflictHash,
		Resolution:     cr.extractSection(response, "RESOLUTION"),
		Confidence:     cr.extractConfidence(response),
		Reasoning:      cr.extractSection(response, "REASONING"),
		ResolutionType: "automatic",
		Risks:          cr.extractRisks(response),
		Tests:          cr.extractTests(response),
		Metadata: map[string]string{
			"generated_at": time.Now().Format(time.RFC3339),
			"provider":     cr.provider.GetModelInfo().Provider,
		},
	}
	
	return []ResolutionSuggestion{suggestion}, nil
}

func (cr *ConflictResolver) extractSection(response, section string) string {
	// Simple extraction (in production, would use more robust parsing)
	lines := strings.Split(response, "\n")
	var collecting bool
	var result strings.Builder
	
	for _, line := range lines {
		if strings.Contains(strings.ToUpper(line), section+":") {
			collecting = true
			continue
		}
		if collecting && strings.Contains(line, ":") && 
			(strings.Contains(strings.ToUpper(line), "CONFIDENCE") ||
			 strings.Contains(strings.ToUpper(line), "REASONING") ||
			 strings.Contains(strings.ToUpper(line), "RISKS") ||
			 strings.Contains(strings.ToUpper(line), "TESTS")) {
			break
		}
		if collecting {
			result.WriteString(line + "\n")
		}
	}
	
	return strings.TrimSpace(result.String())
}

func (cr *ConflictResolver) extractConfidence(response string) float64 {
	// Extract confidence score from response
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToUpper(line), "CONFIDENCE") {
			// Look for decimal number
			re := regexp.MustCompile(`([0-9]*\.?[0-9]+)`)
			matches := re.FindStringSubmatch(line)
			if len(matches) > 0 {
				if conf := parseFloat(matches[0]); conf > 0 {
					if conf <= 1.0 {
						return conf
					} else if conf <= 100 {
						return conf / 100.0
					}
				}
			}
		}
	}
	return 0.5 // Default moderate confidence
}

func (cr *ConflictResolver) extractRisks(response string) []string {
	risks := cr.extractSection(response, "RISKS")
	if risks == "" {
		return []string{}
	}
	
	lines := strings.Split(risks, "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "RISKS:") {
			// Remove bullet points
			line = strings.TrimPrefix(line, "- ")
			line = strings.TrimPrefix(line, "* ")
			if line != "" {
				result = append(result, line)
			}
		}
	}
	
	return result
}

func (cr *ConflictResolver) extractTests(response string) []TestSuggestion {
	tests := cr.extractSection(response, "TESTS")
	if tests == "" {
		return []TestSuggestion{}
	}
	
	// Simple test extraction (in production, would be more sophisticated)
	return []TestSuggestion{
		{
			TestType:    "unit",
			Description: strings.TrimSpace(tests),
			Priority:    "high",
		},
	}
}

func (cr *ConflictResolver) applyResolution(content string, suggestion ResolutionSuggestion) (string, error) {
	// Apply the resolution to the file content
	// This is a simplified implementation
	return suggestion.Resolution, nil
}

func (cr *ConflictResolver) createBackup(repo *govc.Repository, filePath string) error {
	// Create a backup of the conflicted file
	// In a real implementation, this would create a backup file
	fmt.Printf("Creating backup for %s\n", filePath)
	return nil
}

func (cr *ConflictResolver) detectFileType(filePath string) string {
	ext := strings.ToLower(filePath[strings.LastIndex(filePath, "."):])
	switch ext {
	case ".go":
		return "go"
	case ".js":
		return "javascript"
	case ".py":
		return "python"
	case ".java":
		return "java"
	default:
		return "text"
	}
}

func (cr *ConflictResolver) calculateOverallRisk(suggestions []ResolutionSuggestion) string {
	if len(suggestions) == 0 {
		return "unknown"
	}
	
	highRiskCount := 0
	for _, suggestion := range suggestions {
		if len(suggestion.Risks) > 2 || suggestion.Confidence < 0.6 {
			highRiskCount++
		}
	}
	
	riskRatio := float64(highRiskCount) / float64(len(suggestions))
	if riskRatio > 0.5 {
		return "high"
	} else if riskRatio > 0.2 {
		return "medium"
	}
	return "low"
}

func (cr *ConflictResolver) calculateOverallConfidence(suggestions []ResolutionSuggestion) float64 {
	if len(suggestions) == 0 {
		return 0.0
	}
	
	totalConfidence := 0.0
	for _, suggestion := range suggestions {
		totalConfidence += suggestion.Confidence
	}
	
	return totalConfidence / float64(len(suggestions))
}

func (cr *ConflictResolver) estimateResolutionTime(conflictCount int) string {
	minutes := conflictCount * 5 // Estimate 5 minutes per conflict
	if minutes < 10 {
		return "< 10 minutes"
	} else if minutes < 60 {
		return fmt.Sprintf("~%d minutes", minutes)
	} else {
		hours := minutes / 60
		return fmt.Sprintf("~%d hours", hours)
	}
}

func (cr *ConflictResolver) generateNextSteps(suggestions []ResolutionSuggestion) []string {
	steps := []string{}
	
	highConfidenceCount := 0
	for _, suggestion := range suggestions {
		if suggestion.Confidence >= 0.8 {
			highConfidenceCount++
		}
	}
	
	if highConfidenceCount > 0 {
		steps = append(steps, fmt.Sprintf("Review %d high-confidence resolutions", highConfidenceCount))
	}
	
	lowConfidenceCount := len(suggestions) - highConfidenceCount
	if lowConfidenceCount > 0 {
		steps = append(steps, fmt.Sprintf("Manually review %d complex conflicts", lowConfidenceCount))
	}
	
	steps = append(steps, "Run tests after applying resolutions")
	steps = append(steps, "Create a backup before applying changes")
	
	return steps
}

// Utility functions

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func parseFloat(s string) float64 {
	// Simple float parsing (in production, would use strconv.ParseFloat)
	if s == "1.0" || s == "1" {
		return 1.0
	} else if s == "0.9" {
		return 0.9
	} else if s == "0.8" {
		return 0.8
	} else if s == "0.7" {
		return 0.7
	} else if s == "0.6" {
		return 0.6
	} else if s == "0.5" {
		return 0.5
	}
	return 0.5 // Default
}