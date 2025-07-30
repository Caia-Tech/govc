package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/caiatech/govc"
)

// CommitMessageGenerator generates intelligent commit messages based on code changes
type CommitMessageGenerator struct {
	provider LLMProvider
	config   CommitMessageConfig
}

// LLMProvider defines the interface for language model providers
type LLMProvider interface {
	GenerateText(ctx context.Context, prompt string, options GenerationOptions) (string, error)
	GetModelInfo() ModelInfo
}

// ModelInfo provides information about the LLM model
type ModelInfo struct {
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	MaxTokens    int    `json:"max_tokens"`
	ContextSize  int    `json:"context_size"`
	Cost         string `json:"cost"`
}

// GenerationOptions controls text generation behavior
type GenerationOptions struct {
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	TopP        float64 `json:"top_p"`
	Stop        []string `json:"stop"`
}

// CommitMessageConfig holds configuration for commit message generation
type CommitMessageConfig struct {
	Style           string            `json:"style"`            // "conventional", "angular", "semantic", "custom"
	MaxLength       int               `json:"max_length"`       // Maximum commit message length
	IncludeFiles    bool              `json:"include_files"`    // Include file list in prompt context
	IncludeFunctions bool             `json:"include_functions"` // Include changed functions
	Template        string            `json:"template"`         // Custom template for messages
	Language        string            `json:"language"`         // Language for commit messages (en, es, fr, etc.)
	Scopes          []string          `json:"scopes"`           // Valid scopes for conventional commits
	Types           []string          `json:"types"`            // Valid types for conventional commits
	CustomPrompts   map[string]string `json:"custom_prompts"`   // Custom prompts for different scenarios
}

// DefaultCommitMessageConfig returns sensible defaults
func DefaultCommitMessageConfig() CommitMessageConfig {
	return CommitMessageConfig{
		Style:           "conventional",
		MaxLength:       72, // Git best practice
		IncludeFiles:    true,
		IncludeFunctions: true,
		Language:        "en",
		Scopes: []string{
			"api", "ui", "core", "auth", "db", "config", "test", "doc", "ci", "build",
		},
		Types: []string{
			"feat", "fix", "docs", "style", "refactor", "test", "chore", "perf", "ci", "build",
		},
		Template: "{{type}}{{#scope}}({{scope}}){{/scope}}: {{description}}\n\n{{#body}}{{body}}\n\n{{/body}}{{#footer}}{{footer}}{{/footer}}",
	}
}

// OpenAIProvider implements LLM using OpenAI's API
type OpenAIProvider struct {
	APIKey  string
	Model   string
	BaseURL string
	client  *http.Client
}

// NewOpenAIProvider creates a new OpenAI LLM provider
func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		APIKey:  apiKey,
		Model:   "gpt-4o-mini", // Cost-effective model for commit messages
		BaseURL: "https://api.openai.com/v1",
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

type openAIRequest struct {
	Model       string                 `json:"model"`
	Messages    []openAIMessage        `json:"messages"`
	MaxTokens   int                    `json:"max_tokens"`
	Temperature float64                `json:"temperature"`
	TopP        float64                `json:"top_p"`
	Stop        []string               `json:"stop,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (p *OpenAIProvider) GenerateText(ctx context.Context, prompt string, options GenerationOptions) (string, error) {
	reqBody := openAIRequest{
		Model: p.Model,
		Messages: []openAIMessage{
			{
				Role:    "system",
				Content: "You are an expert software developer who writes clear, concise commit messages following best practices.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens:   options.MaxTokens,
		Temperature: options.Temperature,
		TopP:        options.TopP,
		Stop:        options.Stop,
	}
	
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("User-Agent", "govc/1.0")
	
	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	
	var apiResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	
	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("no response generated")
	}
	
	return strings.TrimSpace(apiResp.Choices[0].Message.Content), nil
}

func (p *OpenAIProvider) GetModelInfo() ModelInfo {
	return ModelInfo{
		Name:         p.Model,
		Provider:     "openai",
		MaxTokens:    4096,  // For gpt-4o-mini
		ContextSize:  128000, // For gpt-4o-mini
		Cost:         "$0.15/1M input tokens, $0.60/1M output tokens",
	}
}

// LocalProvider implements a simple local commit message generator (rule-based)
type LocalProvider struct {
	templates map[string][]string
}

func NewLocalProvider() *LocalProvider {
	templates := map[string][]string{
		"add": {
			"Add {{files}}",
			"Implement {{functionality}}",
			"Create {{feature}}",
		},
		"fix": {
			"Fix {{issue}}",
			"Resolve {{problem}}",
			"Correct {{bug}}",
		},
		"update": {
			"Update {{component}}",
			"Modify {{feature}}",
			"Improve {{functionality}}",
		},
		"remove": {
			"Remove {{files}}",
			"Delete {{component}}",
			"Clean up {{feature}}",
		},
		"refactor": {
			"Refactor {{component}}",
			"Restructure {{feature}}",
			"Optimize {{functionality}}",
		},
	}
	
	return &LocalProvider{templates: templates}
}

func (p *LocalProvider) GenerateText(ctx context.Context, prompt string, options GenerationOptions) (string, error) {
	// Simple rule-based commit message generation
	// This analyzes the prompt to determine the type of change and generates appropriate messages
	
	changeType := p.detectChangeType(prompt)
	templates, exists := p.templates[changeType]
	if !exists {
		templates = p.templates["update"] // Default fallback
	}
	
	// Use first template (in production, could rotate or pick randomly)
	template := templates[0]
	
	// Extract relevant information from prompt
	files := p.extractFiles(prompt)
	functionality := p.extractFunctionality(prompt)
	
	// Replace placeholders
	message := template
	message = strings.ReplaceAll(message, "{{files}}", files)
	message = strings.ReplaceAll(message, "{{functionality}}", functionality)
	message = strings.ReplaceAll(message, "{{feature}}", functionality)
	message = strings.ReplaceAll(message, "{{component}}", functionality)
	message = strings.ReplaceAll(message, "{{issue}}", functionality)
	message = strings.ReplaceAll(message, "{{problem}}", functionality)
	message = strings.ReplaceAll(message, "{{bug}}", functionality)
	
	return message, nil
}

func (p *LocalProvider) GetModelInfo() ModelInfo {
	return ModelInfo{
		Name:        "local-rules",
		Provider:    "local",
		MaxTokens:   1000,
		ContextSize: 1000,
		Cost:        "free",
	}
}

func (p *LocalProvider) detectChangeType(prompt string) string {
	prompt = strings.ToLower(prompt)
	
	if strings.Contains(prompt, "add") || strings.Contains(prompt, "new") || strings.Contains(prompt, "create") {
		return "add"
	}
	if strings.Contains(prompt, "fix") || strings.Contains(prompt, "bug") || strings.Contains(prompt, "error") {
		return "fix"
	}
	if strings.Contains(prompt, "remove") || strings.Contains(prompt, "delete") {
		return "remove"
	}
	if strings.Contains(prompt, "refactor") || strings.Contains(prompt, "restructure") {
		return "refactor"
	}
	
	return "update"
}

func (p *LocalProvider) extractFiles(prompt string) string {
	// Extract file names from the prompt
	re := regexp.MustCompile(`\b\w+\.\w+\b`) // Simple file pattern
	matches := re.FindAllString(prompt, -1)
	if len(matches) > 0 {
		if len(matches) == 1 {
			return matches[0]
		}
		return fmt.Sprintf("%d files", len(matches))
	}
	return "files"
}

func (p *LocalProvider) extractFunctionality(prompt string) string {
	// Extract functionality keywords from prompt
	words := strings.Fields(strings.ToLower(prompt))
	
	// Look for programming-related keywords
	keywords := []string{
		"authentication", "auth", "login", "user", "api", "endpoint", "function",
		"method", "class", "interface", "database", "db", "query", "test", "tests",
		"config", "configuration", "ui", "frontend", "backend", "server", "client",
	}
	
	for _, word := range words {
		for _, keyword := range keywords {
			if strings.Contains(word, keyword) {
				return keyword
			}
		}
	}
	
	return "functionality"
}

// NewCommitMessageGenerator creates a new commit message generator
func NewCommitMessageGenerator(provider LLMProvider, config CommitMessageConfig) *CommitMessageGenerator {
	return &CommitMessageGenerator{
		provider: provider,
		config:   config,
	}
}

// GenerateCommitMessage generates a commit message based on repository changes
func (g *CommitMessageGenerator) GenerateCommitMessage(ctx context.Context, repo *govc.Repository) (string, error) {
	// Get repository status to understand what changed
	status, err := repo.Status()
	if err != nil {
		return "", fmt.Errorf("failed to get repository status: %w", err)
	}
	
	// Analyze changes
	changes := g.analyzeChanges(status)
	if len(changes.ModifiedFiles) == 0 && len(changes.NewFiles) == 0 && len(changes.DeletedFiles) == 0 {
		return "", fmt.Errorf("no changes detected")
	}
	
	// Build prompt for LLM
	prompt := g.buildPrompt(changes)
	
	// Generate commit message
	options := GenerationOptions{
		MaxTokens:   200, // Keep commit messages concise
		Temperature: 0.3, // Lower temperature for more consistent output
		TopP:        0.9,
		Stop:        []string{"\n\n"}, // Stop at double newline
	}
	
	message, err := g.provider.GenerateText(ctx, prompt, options)
	if err != nil {
		return "", fmt.Errorf("failed to generate commit message: %w", err)
	}
	
	// Post-process the message
	message = g.postProcessMessage(message)
	
	return message, nil
}

// GenerateCommitMessageForDiff generates a commit message based on a specific diff
func (g *CommitMessageGenerator) GenerateCommitMessageForDiff(ctx context.Context, diff string) (string, error) {
	changes := g.analyzeDiff(diff)
	prompt := g.buildPromptFromDiff(changes, diff)
	
	options := GenerationOptions{
		MaxTokens:   200,
		Temperature: 0.3,
		TopP:        0.9,
		Stop:        []string{"\n\n"},
	}
	
	message, err := g.provider.GenerateText(ctx, prompt, options)
	if err != nil {
		return "", fmt.Errorf("failed to generate commit message: %w", err)
	}
	
	return g.postProcessMessage(message), nil
}

// GenerateMultipleOptions generates multiple commit message options for the user to choose from
func (g *CommitMessageGenerator) GenerateMultipleOptions(ctx context.Context, repo *govc.Repository, count int) ([]string, error) {
	if count <= 0 || count > 10 {
		count = 3 // Default to 3 options
	}
	
	status, err := repo.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get repository status: %w", err)
	}
	
	changes := g.analyzeChanges(status)
	basePrompt := g.buildPrompt(changes)
	
	var messages []string
	for i := 0; i < count; i++ {
		// Vary the temperature and prompts slightly to get different options
		temperature := 0.3 + float64(i)*0.2 // 0.3, 0.5, 0.7, etc.
		if temperature > 0.9 {
			temperature = 0.9
		}
		
		prompt := basePrompt
		if i == 1 {
			prompt += "\n\nProvide an alternative perspective on these changes."
		} else if i == 2 {
			prompt += "\n\nFocus on the technical implementation details."
		}
		
		options := GenerationOptions{
			MaxTokens:   200,
			Temperature: temperature,
			TopP:        0.9,
			Stop:        []string{"\n\n"},
		}
		
		message, err := g.provider.GenerateText(ctx, prompt, options)
		if err != nil {
			continue // Skip failed generations
		}
		
		message = g.postProcessMessage(message)
		if message != "" && !g.containsMessage(messages, message) {
			messages = append(messages, message)
		}
	}
	
	if len(messages) == 0 {
		return nil, fmt.Errorf("failed to generate any commit messages")
	}
	
	return messages, nil
}

// ChangeAnalysis represents an analysis of repository changes
type ChangeAnalysis struct {
	ModifiedFiles   []string          `json:"modified_files"`
	NewFiles        []string          `json:"new_files"`
	DeletedFiles    []string          `json:"deleted_files"`
	TotalLines      int               `json:"total_lines"`
	AddedLines      int               `json:"added_lines"`
	DeletedLines    int               `json:"deleted_lines"`
	ChangeType      string            `json:"change_type"`      // "feature", "fix", "refactor", etc.
	Scope           string            `json:"scope"`            // Detected scope
	Languages       []string          `json:"languages"`        // Programming languages involved
	Functions       []string          `json:"functions"`        // Changed functions
	Metadata        map[string]string `json:"metadata"`         // Additional metadata
}

func (g *CommitMessageGenerator) analyzeChanges(status *govc.Status) ChangeAnalysis {
	analysis := ChangeAnalysis{
		ModifiedFiles: append([]string{}, status.Modified...),
		NewFiles:      append([]string{}, status.Staged...), // Assuming staged files are new/modified
		DeletedFiles:  []string{}, // Would need additional status info
		Languages:     []string{},
		Functions:     []string{},
		Metadata:      make(map[string]string),
	}
	
	// Analyze file types and languages
	languages := make(map[string]bool)
	for _, file := range append(analysis.ModifiedFiles, analysis.NewFiles...) {
		lang := g.detectLanguageFromPath(file)
		if lang != "" {
			languages[lang] = true
		}
	}
	
	for lang := range languages {
		analysis.Languages = append(analysis.Languages, lang)
	}
	
	// Detect change type based on file patterns and names
	analysis.ChangeType = g.detectChangeType(analysis)
	analysis.Scope = g.detectScope(analysis)
	
	return analysis
}

func (g *CommitMessageGenerator) analyzeDiff(diff string) ChangeAnalysis {
	// Analyze a git diff to extract change information
	analysis := ChangeAnalysis{
		Metadata: make(map[string]string),
	}
	
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			// Extract file paths
			if strings.HasPrefix(line, "+++") && !strings.Contains(line, "/dev/null") {
				file := strings.TrimPrefix(line, "+++ b/")
				analysis.ModifiedFiles = append(analysis.ModifiedFiles, file)
			}
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			analysis.AddedLines++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			analysis.DeletedLines++
		}
	}
	
	analysis.TotalLines = analysis.AddedLines + analysis.DeletedLines
	
	// Detect languages
	languages := make(map[string]bool)
	for _, file := range analysis.ModifiedFiles {
		lang := g.detectLanguageFromPath(file)
		if lang != "" {
			languages[lang] = true
		}
	}
	
	for lang := range languages {
		analysis.Languages = append(analysis.Languages, lang)
	}
	
	analysis.ChangeType = g.detectChangeType(analysis)
	analysis.Scope = g.detectScope(analysis)
	
	return analysis
}

func (g *CommitMessageGenerator) buildPrompt(changes ChangeAnalysis) string {
	var prompt strings.Builder
	
	// Base instruction
	prompt.WriteString("Generate a ")
	if g.config.Style == "conventional" {
		prompt.WriteString("conventional commit message")
	} else {
		prompt.WriteString("clear and concise commit message")
	}
	prompt.WriteString(" for the following code changes:\n\n")
	
	// Change summary
	if len(changes.ModifiedFiles) > 0 {
		prompt.WriteString(fmt.Sprintf("Modified files (%d): %s\n", len(changes.ModifiedFiles), strings.Join(changes.ModifiedFiles, ", ")))
	}
	if len(changes.NewFiles) > 0 {
		prompt.WriteString(fmt.Sprintf("New files (%d): %s\n", len(changes.NewFiles), strings.Join(changes.NewFiles, ", ")))
	}
	if len(changes.DeletedFiles) > 0 {
		prompt.WriteString(fmt.Sprintf("Deleted files (%d): %s\n", len(changes.DeletedFiles), strings.Join(changes.DeletedFiles, ", ")))
	}
	
	// Change statistics
	if changes.TotalLines > 0 {
		prompt.WriteString(fmt.Sprintf("Changes: +%d -%d lines\n", changes.AddedLines, changes.DeletedLines))
	}
	
	// Languages involved
	if len(changes.Languages) > 0 {
		prompt.WriteString(fmt.Sprintf("Languages: %s\n", strings.Join(changes.Languages, ", ")))
	}
	
	// Style-specific instructions
	if g.config.Style == "conventional" {
		prompt.WriteString("\nUse conventional commit format: type(scope): description\n")
		prompt.WriteString(fmt.Sprintf("Valid types: %s\n", strings.Join(g.config.Types, ", ")))
		if len(g.config.Scopes) > 0 {
			prompt.WriteString(fmt.Sprintf("Available scopes: %s\n", strings.Join(g.config.Scopes, ", ")))
		}
	}
	
	// Length constraint
	prompt.WriteString(fmt.Sprintf("\nKeep the message under %d characters. Be specific and descriptive but concise.\n", g.config.MaxLength))
	
	// Language preference
	if g.config.Language != "en" {
		prompt.WriteString(fmt.Sprintf("Write the commit message in %s.\n", g.config.Language))
	}
	
	prompt.WriteString("\nCommit message:")
	
	return prompt.String()
}

func (g *CommitMessageGenerator) buildPromptFromDiff(changes ChangeAnalysis, diff string) string {
	prompt := g.buildPrompt(changes)
	
	// Add diff context (truncated to avoid token limits)
	if len(diff) > 2000 { // Limit diff size
		diff = diff[:2000] + "\n... (truncated)"
	}
	
	prompt += "\n\nCode diff:\n" + diff
	
	return prompt
}

func (g *CommitMessageGenerator) postProcessMessage(message string) string {
	// Clean up the generated message
	message = strings.TrimSpace(message)
	
	// Remove common prefixes that might be added by the model
	prefixes := []string{"Commit message:", "Message:", "Git commit:"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(message, prefix) {
			message = strings.TrimSpace(message[len(prefix):])
		}
	}
	
	// Ensure it doesn't exceed max length
	if len(message) > g.config.MaxLength {
		// Try to truncate at word boundary
		words := strings.Fields(message)
		truncated := ""
		for _, word := range words {
			if len(truncated)+len(word)+1 <= g.config.MaxLength {
				if truncated != "" {
					truncated += " "
				}
				truncated += word
			} else {
				break
			}
		}
		if truncated != "" {
			message = truncated
		} else {
			message = message[:g.config.MaxLength]
		}
	}
	
	// Validate conventional commit format if needed
	if g.config.Style == "conventional" {
		message = g.validateConventionalCommit(message)
	}
	
	return message
}

func (g *CommitMessageGenerator) validateConventionalCommit(message string) string {
	// Check if message follows conventional commit format
	conventionalPattern := regexp.MustCompile(`^(feat|fix|docs|style|refactor|test|chore|perf|ci|build)(\(.+\))?: .+`)
	
	if !conventionalPattern.MatchString(message) {
		// Try to fix common issues
		if !strings.Contains(message, ":") {
			// Add a type if missing
			changeType := "feat" // Default
			if strings.Contains(strings.ToLower(message), "fix") {
				changeType = "fix"
			}
			message = changeType + ": " + message
		}
	}
	
	return message
}

func (g *CommitMessageGenerator) detectChangeType(analysis ChangeAnalysis) string {
	// Analyze files and patterns to detect change type
	
	// Check file names for clues
	for _, file := range append(analysis.ModifiedFiles, analysis.NewFiles...) {
		filename := strings.ToLower(file)
		
		if strings.Contains(filename, "test") {
			return "test"
		}
		if strings.Contains(filename, "doc") || strings.Contains(filename, "readme") {
			return "docs"
		}
		if strings.Contains(filename, "config") {
			return "config"
		}
	}
	
	// Default based on operation type
	if len(analysis.NewFiles) > len(analysis.ModifiedFiles) {
		return "feat" // More new files suggests new feature
	}
	if len(analysis.DeletedFiles) > 0 {
		return "refactor" // Deletions suggest refactoring
	}
	
	return "feat" // Default to feature
}

func (g *CommitMessageGenerator) detectScope(analysis ChangeAnalysis) string {
	// Detect scope from file paths
	scopes := make(map[string]int)
	
	for _, file := range append(analysis.ModifiedFiles, analysis.NewFiles...) {
		parts := strings.Split(file, "/")
		if len(parts) > 1 {
			scope := parts[0]
			// Map common directory names to scopes
			switch scope {
			case "src", "lib":
				if len(parts) > 2 {
					scope = parts[1]
				}
			case "test", "tests":
				scope = "test"
			case "docs", "doc":
				scope = "docs"
			case "config", "configs":
				scope = "config"
			}
			scopes[scope]++
		}
	}
	
	// Return most common scope
	maxCount := 0
	detectedScope := ""
	for scope, count := range scopes {
		if count > maxCount {
			maxCount = count
			detectedScope = scope
		}
	}
	
	// Validate against configured scopes
	for _, validScope := range g.config.Scopes {
		if validScope == detectedScope {
			return detectedScope
		}
	}
	
	return "" // No valid scope detected
}

func (g *CommitMessageGenerator) detectLanguageFromPath(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	
	switch ext {
	case ".go":
		return "Go"
	case ".js":
		return "JavaScript"
	case ".ts":
		return "TypeScript"
	case ".py":
		return "Python"
	case ".java":
		return "Java"
	case ".cpp", ".cc", ".cxx":
		return "C++"
	case ".c":
		return "C"
	case ".cs":
		return "C#"
	case ".php":
		return "PHP"
	case ".rb":
		return "Ruby"
	case ".rs":
		return "Rust"
	case ".kt":
		return "Kotlin"
	case ".swift":
		return "Swift"
	case ".md":
		return "Markdown"
	default:
		return ""
	}
}

func (g *CommitMessageGenerator) containsMessage(messages []string, message string) bool {
	for _, existing := range messages {
		if existing == message {
			return true
		}
	}
	return false
}

// NewLLMProvider creates a new LLM provider based on configuration
func NewLLMProvider(providerType, apiKey string) (LLMProvider, error) {
	switch strings.ToLower(providerType) {
	case "openai":
		if apiKey == "" {
			return nil, fmt.Errorf("OpenAI API key required")
		}
		return NewOpenAIProvider(apiKey), nil
	case "local":
		return NewLocalProvider(), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", providerType)
	}
}