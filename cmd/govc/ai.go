package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/caiatech/govc/ai/embeddings"
	"github.com/caiatech/govc/ai/llm"
	"github.com/caiatech/govc/ai/search"
	"github.com/spf13/cobra"
)

var (
	// AI parent command
	aiCmd = &cobra.Command{
		Use:   "ai",
		Short: "AI and smart features for enhanced development",
		Long: `AI-powered features to improve your development workflow:
- Semantic code search using embeddings
- Automated commit message generation
- Intelligent code analysis and suggestions`,
	}

	// Semantic search command
	searchCmd = &cobra.Command{
		Use:   "search [query]",
		Short: "Perform semantic code search",
		Long: `Search your codebase using natural language queries.
This command uses AI embeddings to find semantically similar code,
not just exact text matches.

Examples:
  govc ai search "authentication logic"
  govc ai search "database connection handling"
  govc ai search "error handling patterns"`,
		Args: cobra.MinimumNArgs(1),
		Run:  runSemanticSearch,
	}

	// Index command for semantic search
	indexCmd = &cobra.Command{
		Use:   "index",
		Short: "Build semantic search index",
		Long: `Build or rebuild the semantic search index for the repository.
This process analyzes all code files and creates embeddings for
intelligent search capabilities.`,
		Run: runBuildIndex,
	}

	// Commit message generation command
	commitMsgCmd = &cobra.Command{
		Use:   "commit-msg",
		Short: "Generate AI-powered commit messages",
		Long: `Generate intelligent commit messages based on your changes.
The AI analyzes your staged changes and creates appropriate
commit messages following best practices.

Examples:
  govc ai commit-msg
  govc ai commit-msg --style conventional
  govc ai commit-msg --multiple 3`,
		Run: runGenerateCommitMessage,
	}

	// Conflict resolution command
	resolveCmd = &cobra.Command{
		Use:   "resolve",
		Short: "AI-powered conflict resolution",
		Long: `Analyze and suggest resolutions for merge conflicts.
The AI examines conflict markers and provides intelligent
suggestions for resolving merge conflicts.

Examples:
  govc ai resolve
  govc ai resolve --auto-apply
  govc ai resolve --file main.go`,
		Run: runConflictResolution,
	}

	// Code review command
	reviewCmd = &cobra.Command{
		Use:   "review [commit-hash]",
		Short: "AI-powered code review",
		Long: `Perform automated code review using AI.
Analyzes code changes and provides feedback on security,
performance, maintainability, and potential bugs.

Examples:
  govc ai review
  govc ai review HEAD~1
  govc ai review --depth comprehensive`,
		Run: runCodeReview,
	}

	// Search index stats command
	statsCmd = &cobra.Command{
		Use:   "stats",
		Short: "Show semantic search index statistics",
		Long:  `Display statistics about the current semantic search index.`,
		Run:   runIndexStats,
	}
)

// Command flags
var (
	// Search flags
	searchInFile      string
	searchMaxResults  int
	searchMinSimilarity float64
	
	// Index flags
	indexProvider     string
	indexOpenAIKey    string
	indexModel        string
	indexChunkSize    int
	
	// Commit message flags
	commitMsgStyle    string
	commitMsgProvider string
	commitMsgAPIKey   string
	commitMsgMultiple int
	commitMsgLanguage string
	commitMsgMaxLength int
	
	// Conflict resolution flags
	resolveAutoApply    bool
	resolveFile         string
	resolveConfidence   float64
	resolveBackup       bool
	
	// Code review flags
	reviewDepth         string
	reviewAutoComment   bool
	reviewFile          string
	reviewProvider      string
	reviewAPIKey        string
	
	// General AI flags
	aiVerbose bool
)

func init() {
	// Add AI commands to root
	rootCmd.AddCommand(aiCmd)
	aiCmd.AddCommand(searchCmd)
	aiCmd.AddCommand(indexCmd)
	aiCmd.AddCommand(commitMsgCmd)
	aiCmd.AddCommand(resolveCmd)
	aiCmd.AddCommand(reviewCmd)
	aiCmd.AddCommand(statsCmd)

	// Search command flags
	searchCmd.Flags().StringVarP(&searchInFile, "file", "f", "", "Search within specific file")
	searchCmd.Flags().IntVarP(&searchMaxResults, "max-results", "n", 10, "Maximum number of results")
	searchCmd.Flags().Float64VarP(&searchMinSimilarity, "min-similarity", "s", 0.3, "Minimum similarity threshold (0.0-1.0)")

	// Index command flags
	indexCmd.Flags().StringVarP(&indexProvider, "provider", "p", "local", "Embedding provider (openai, local)")
	indexCmd.Flags().StringVar(&indexOpenAIKey, "openai-key", "", "OpenAI API key (or set OPENAI_API_KEY env var)")
	indexCmd.Flags().StringVarP(&indexModel, "model", "m", "text-embedding-3-small", "Embedding model to use")
	indexCmd.Flags().IntVar(&indexChunkSize, "chunk-size", 1000, "Size of code chunks for indexing")

	// Commit message flags
	commitMsgCmd.Flags().StringVarP(&commitMsgStyle, "style", "s", "conventional", "Commit message style (conventional, semantic, custom)")
	commitMsgCmd.Flags().StringVarP(&commitMsgProvider, "provider", "p", "local", "LLM provider (openai, local)")
	commitMsgCmd.Flags().StringVar(&commitMsgAPIKey, "api-key", "", "API key for LLM provider (or set OPENAI_API_KEY env var)")
	commitMsgCmd.Flags().IntVarP(&commitMsgMultiple, "multiple", "m", 1, "Generate multiple options (1-10)")
	commitMsgCmd.Flags().StringVarP(&commitMsgLanguage, "language", "l", "en", "Language for commit messages")
	commitMsgCmd.Flags().IntVar(&commitMsgMaxLength, "max-length", 72, "Maximum commit message length")

	// Conflict resolution flags
	resolveCmd.Flags().BoolVar(&resolveAutoApply, "auto-apply", false, "Automatically apply high-confidence resolutions")
	resolveCmd.Flags().StringVarP(&resolveFile, "file", "f", "", "Resolve conflicts in specific file")
	resolveCmd.Flags().Float64Var(&resolveConfidence, "confidence", 0.8, "Minimum confidence threshold (0.0-1.0)")
	resolveCmd.Flags().BoolVar(&resolveBackup, "backup", true, "Create backup before applying resolutions")

	// Code review flags
	reviewCmd.Flags().StringVar(&reviewDepth, "depth", "detailed", "Review depth (surface, detailed, comprehensive)")
	reviewCmd.Flags().BoolVar(&reviewAutoComment, "auto-comment", false, "Automatically add review comments")
	reviewCmd.Flags().StringVarP(&reviewFile, "file", "f", "", "Review specific file")
	reviewCmd.Flags().StringVarP(&reviewProvider, "provider", "p", "local", "LLM provider (openai, local)")
	reviewCmd.Flags().StringVar(&reviewAPIKey, "api-key", "", "API key for LLM provider")

	// Global AI flags
	aiCmd.PersistentFlags().BoolVarP(&aiVerbose, "verbose", "v", false, "Verbose output")
}

func runSemanticSearch(cmd *cobra.Command, args []string) {
	query := strings.Join(args, " ")
	
	repo, err := openRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Create embedding provider
	config := embeddings.DefaultEmbeddingConfig()
	config.Provider = indexProvider
	if indexOpenAIKey != "" {
		config.OpenAIAPIKey = indexOpenAIKey
	} else if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		config.OpenAIAPIKey = key
	}

	provider, err := embeddings.NewEmbeddingProvider(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating embedding provider: %v\n", err)
		os.Exit(1)
	}

	// Create search engine
	searchConfig := search.DefaultSearchConfig()
	searchConfig.MaxResults = searchMaxResults
	searchConfig.MinSimilarity = searchMinSimilarity
	
	engine := search.NewSemanticSearchEngine(provider, searchConfig)

	// Try to load existing index (in production, this would be persisted)
	fmt.Println("🔍 Loading semantic search index...")
	if err := engine.IndexRepository(context.Background(), repo); err != nil {
		fmt.Fprintf(os.Stderr, "Error indexing repository: %v\n", err)
		fmt.Println("💡 Try running 'govc ai index' first to build the search index")
		os.Exit(1)
	}

	// Perform search
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var results []search.SearchResult
	if searchInFile != "" {
		results, err = engine.SearchByFile(ctx, query, searchInFile)
	} else {
		results, err = engine.Search(ctx, query)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error performing search: %v\n", err)
		os.Exit(1)
	}

	// Display results
	if len(results) == 0 {
		fmt.Printf("🔍 No results found for: %s\n", query)
		return
	}

	fmt.Printf("🔍 Found %d results for: %s\n\n", len(results), query)

	for _, result := range results {
		fmt.Printf("📄 %s:%d-%d (%.1f%% match)\n", 
			result.FilePath, 
			result.Chunk.StartLine, 
			result.Chunk.EndLine,
			result.Similarity*100)
		
		if aiVerbose {
			fmt.Printf("   %s\n", result.Explanation)
			if len(result.Chunk.Functions) > 0 {
				fmt.Printf("   Functions: %s\n", strings.Join(result.Chunk.Functions, ", "))
			}
		}
		
		// Show code snippet
		lines := strings.Split(result.Chunk.Content, "\n")
		maxLines := 5
		if len(lines) > maxLines {
			lines = lines[:maxLines]
		}
		
		for i, line := range lines {
			lineNum := result.Chunk.StartLine + i
			fmt.Printf("   %4d | %s\n", lineNum, line)
		}
		
		if len(strings.Split(result.Chunk.Content, "\n")) > maxLines {
			fmt.Printf("   ... (%d more lines)\n", len(strings.Split(result.Chunk.Content, "\n"))-maxLines)
		}
		
		fmt.Println()
	}
}

func runBuildIndex(cmd *cobra.Command, args []string) {
	repo, err := openRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Create embedding provider
	config := embeddings.DefaultEmbeddingConfig()
	config.Provider = indexProvider
	config.Dimensions = 512 // For local provider
	
	if indexOpenAIKey != "" {
		config.OpenAIAPIKey = indexOpenAIKey
	} else if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		config.OpenAIAPIKey = key
	}
	
	if indexModel != "" {
		config.Model = indexModel
	}

	provider, err := embeddings.NewEmbeddingProvider(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating embedding provider: %v\n", err)
		os.Exit(1)
	}

	// Create search engine
	searchConfig := search.DefaultSearchConfig()
	searchConfig.IndexChunkSize = indexChunkSize
	
	engine := search.NewSemanticSearchEngine(provider, searchConfig)

	// Build index
	fmt.Printf("🧠 Building semantic search index (provider: %s)\n", config.Provider)
	if config.Provider == "openai" {
		fmt.Println("💰 Note: Using OpenAI embeddings will incur API costs")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	startTime := time.Now()
	if err := engine.IndexRepository(ctx, repo); err != nil {
		fmt.Fprintf(os.Stderr, "Error building index: %v\n", err)
		os.Exit(1)
	}

	// Show completion stats
	stats := engine.GetIndexStats()
	elapsed := time.Since(startTime)
	
	fmt.Printf("✅ Index built successfully in %v\n", elapsed)
	fmt.Printf("📊 Indexed %d files, %d chunks\n", stats.TotalFiles, stats.TotalChunks)
	fmt.Printf("🌍 Languages: ")
	
	var langStats []string
	for lang, count := range stats.Languages {
		langStats = append(langStats, fmt.Sprintf("%s(%d)", lang, count))
	}
	fmt.Println(strings.Join(langStats, ", "))
	
	if config.Provider == "openai" {
		estimatedCost := float64(stats.TotalChunks) * 0.00001 // Rough estimate
		fmt.Printf("💰 Estimated cost: ~$%.4f\n", estimatedCost)
	}
}

func runGenerateCommitMessage(cmd *cobra.Command, args []string) {
	repo, err := openRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Get API key
	apiKey := commitMsgAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	// Create LLM provider
	provider, err := llm.NewLLMProvider(commitMsgProvider, apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating LLM provider: %v\n", err)
		if commitMsgProvider == "openai" {
			fmt.Fprintf(os.Stderr, "💡 Set OPENAI_API_KEY environment variable or use --api-key flag\n")
		}
		os.Exit(1)
	}

	// Create commit message generator
	config := llm.DefaultCommitMessageConfig()
	config.Style = commitMsgStyle
	config.Language = commitMsgLanguage
	config.MaxLength = commitMsgMaxLength
	
	generator := llm.NewCommitMessageGenerator(provider, config)

	// Check for changes
	status, err := repo.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting repository status: %v\n", err)
		os.Exit(1)
	}

	if len(status.Staged) == 0 && len(status.Modified) == 0 {
		fmt.Println("💡 No changes detected. Stage some files first with 'govc add'")
		return
	}

	fmt.Printf("🤖 Generating commit message (provider: %s, style: %s)\n", commitMsgProvider, commitMsgStyle)
	if commitMsgProvider == "openai" {
		fmt.Println("💰 Note: Using OpenAI will incur small API costs")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if commitMsgMultiple > 1 {
		// Generate multiple options
		messages, err := generator.GenerateMultipleOptions(ctx, repo, commitMsgMultiple)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating commit messages: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("\n📝 Generated %d commit message options:\n\n", len(messages))
		for i, message := range messages {
			fmt.Printf("%d. %s\n", i+1, message)
		}
		
		fmt.Printf("\n💡 Use any of these messages with: govc commit -m \"<message>\"\n")
	} else {
		// Generate single message
		message, err := generator.GenerateCommitMessage(ctx, repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating commit message: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("\n📝 Generated commit message:\n")
		fmt.Printf("   %s\n", message)
		fmt.Printf("\n💡 Use this message with: govc commit -m \"%s\"\n", message)
	}
}

func runConflictResolution(cmd *cobra.Command, args []string) {
	repo, err := openRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Get API key
	apiKey := reviewAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	// Create LLM provider
	provider, err := llm.NewLLMProvider(reviewProvider, apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating LLM provider: %v\n", err)
		if reviewProvider == "openai" {
			fmt.Fprintf(os.Stderr, "💡 Set OPENAI_API_KEY environment variable or use --api-key flag\n")
		}
		os.Exit(1)
	}

	// Create conflict resolver
	config := llm.DefaultConflictResolutionConfig()
	config.Provider = reviewProvider
	config.Confidence = resolveConfidence
	config.AutoApply = resolveAutoApply
	config.BackupEnabled = resolveBackup
	
	resolver := llm.NewConflictResolver(provider, config)

	fmt.Printf("🔍 Analyzing merge conflicts (provider: %s, confidence: %.1f)\n", 
		config.Provider, config.Confidence)
	if config.Provider == "openai" {
		fmt.Println("💰 Note: Using OpenAI will incur API costs")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Analyze conflicts
	results, err := resolver.AnalyzeConflicts(ctx, repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error analyzing conflicts: %v\n", err)
		os.Exit(1)
	}

	if len(results) == 0 {
		fmt.Println("✅ No merge conflicts found")
		return
	}

	// Display results
	fmt.Printf("\n🔧 Found conflicts in %d files:\n\n", len(results))

	for _, result := range results {
		fmt.Printf("📄 %s\n", result.FilePath)
		fmt.Printf("   Conflicts: %d total, %d resolved\n", 
			result.TotalConflicts, result.ResolvedCount)
		fmt.Printf("   Risk level: %s, Confidence: %.1f%%\n", 
			result.OverallRisk, result.Confidence*100)
		fmt.Printf("   Est. time: %s\n", result.EstimatedTime)

		if aiVerbose && len(result.Suggestions) > 0 {
			fmt.Printf("   Suggestions:\n")
			for i, suggestion := range result.Suggestions {
				if i >= 3 { // Limit to 3 suggestions in summary
					fmt.Printf("   ... and %d more\n", len(result.Suggestions)-3)
					break
				}
				fmt.Printf("     - %s (%.1f%% confidence)\n", 
					suggestion.Reasoning, suggestion.Confidence*100)
			}
		}

		if len(result.NextSteps) > 0 {
			fmt.Printf("   Next steps:\n")
			for _, step := range result.NextSteps {
				fmt.Printf("     • %s\n", step)
			}
		}
		
		fmt.Println()
	}

	if resolveAutoApply {
		fmt.Printf("🤖 Auto-apply is enabled. High-confidence resolutions will be applied automatically.\n")
	} else {
		fmt.Printf("💡 Use --auto-apply flag to automatically apply high-confidence resolutions\n")
	}
}

func runCodeReview(cmd *cobra.Command, args []string) {
	repo, err := openRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Get API key
	apiKey := reviewAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	// Create LLM provider
	provider, err := llm.NewLLMProvider(reviewProvider, apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating LLM provider: %v\n", err)
		if reviewProvider == "openai" {
			fmt.Fprintf(os.Stderr, "💡 Set OPENAI_API_KEY environment variable or use --api-key flag\n")
		}
		os.Exit(1)
	}

	// Create code reviewer
	config := llm.DefaultCodeReviewConfig()
	config.Provider = reviewProvider
	config.ReviewDepth = reviewDepth
	config.AutoComment = reviewAutoComment
	
	reviewer := llm.NewCodeReviewer(provider, config)

	// Determine what to review
	var commitHash string
	if len(args) > 0 {
		commitHash = args[0]
	} else {
		commitHash = "HEAD" // Review latest commit
	}

	fmt.Printf("🔍 Starting AI code review (depth: %s, provider: %s)\n", 
		config.ReviewDepth, config.Provider)
	if config.Provider == "openai" {
		fmt.Println("💰 Note: Using OpenAI will incur API costs")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Perform review
	result, err := reviewer.ReviewCommit(ctx, repo, commitHash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error performing code review: %v\n", err)
		os.Exit(1)
	}

	// Display results
	fmt.Printf("\n📊 Code Review Results\n")
	fmt.Printf("======================\n")
	fmt.Printf("Overall Score:    %.1f/100\n", result.OverallScore)
	fmt.Printf("Files Reviewed:   %d\n", result.Summary.FilesReviewed)
	fmt.Printf("Issues Found:     %d\n", len(result.Issues))
	fmt.Printf("Suggestions:      %d\n", len(result.Suggestions))
	fmt.Printf("Risk Level:       %s\n", result.Summary.RiskLevel)

	if result.Summary.CriticalIssues > 0 {
		fmt.Printf("⚠️  Critical Issues: %d\n", result.Summary.CriticalIssues)
	}
	if result.Summary.SecurityIssues > 0 {
		fmt.Printf("🔒 Security Issues: %d\n", result.Summary.SecurityIssues)
	}
	if result.Summary.PerformanceIssues > 0 {
		fmt.Printf("⚡ Performance Issues: %d\n", result.Summary.PerformanceIssues)
	}

	// Show issues
	if len(result.Issues) > 0 {
		fmt.Printf("\n🐛 Issues Found:\n")
		for i, issue := range result.Issues {
			if i >= 10 { // Limit to 10 issues in summary
				fmt.Printf("... and %d more issues\n", len(result.Issues)-10)
				break
			}
			severityIcon := "ℹ️"
			switch issue.Severity {
			case "critical":
				severityIcon = "🚨"
			case "error":
				severityIcon = "❌"
			case "warning":
				severityIcon = "⚠️"
			}
			
			fmt.Printf("%s %s:%d - %s\n", severityIcon, issue.FilePath, issue.LineNumber, issue.Title)
			if aiVerbose {
				fmt.Printf("   %s\n", issue.Description)
			}
		}
	}

	// Show suggestions
	if len(result.Suggestions) > 0 && aiVerbose {
		fmt.Printf("\n💡 Suggestions:\n")
		for i, suggestion := range result.Suggestions {
			if i >= 5 { // Limit to 5 suggestions
				fmt.Printf("... and %d more suggestions\n", len(result.Suggestions)-5)
				break
			}
			fmt.Printf("• %s:%d - %s\n", suggestion.FilePath, suggestion.LineNumber, suggestion.Title)
			fmt.Printf("  %s\n", suggestion.Description)
		}
	}

	// Show recommendations
	if len(result.Recommendations) > 0 {
		fmt.Printf("\n📋 Recommendations:\n")
		for _, rec := range result.Recommendations {
			fmt.Printf("• %s\n", rec)
		}
	}

	fmt.Printf("\n%s\n", result.Summary.OverallAssessment)
}

func runIndexStats(cmd *cobra.Command, args []string) {
	repo, err := openRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Create a search engine to get stats (this would normally load existing index)
	config := embeddings.DefaultEmbeddingConfig()
	provider, err := embeddings.NewEmbeddingProvider(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating provider: %v\n", err)
		os.Exit(1)
	}

	searchConfig := search.DefaultSearchConfig()
	engine := search.NewSemanticSearchEngine(provider, searchConfig)

	// Build index to get current stats (in production, would load existing)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Println("📊 Calculating index statistics...")
	if err := engine.IndexRepository(ctx, repo); err != nil {
		fmt.Fprintf(os.Stderr, "No index found. Run 'govc ai index' first.\n")
		os.Exit(1)
	}

	stats := engine.GetIndexStats()

	fmt.Printf("\n📊 Semantic Search Index Statistics\n")
	fmt.Printf("=====================================\n")
	fmt.Printf("Total files:      %d\n", stats.TotalFiles)
	fmt.Printf("Total chunks:     %d\n", stats.TotalChunks)
	fmt.Printf("Avg chunk size:   %d characters\n", stats.AvgChunkSize)
	fmt.Printf("Last updated:     %s\n", stats.LastUpdated.Format("2006-01-02 15:04:05"))
	fmt.Printf("Repository ver:   %s\n", stats.Version)
	
	fmt.Printf("\n📈 Languages:\n")
	for lang, count := range stats.Languages {
		percentage := float64(count) / float64(stats.TotalFiles) * 100
		fmt.Printf("  %-12s %3d files (%.1f%%)\n", lang, count, percentage)
	}
	
	fmt.Printf("\n💡 Search tips:\n")
	fmt.Printf("  - Use natural language: 'authentication logic'\n")
	fmt.Printf("  - Be specific: 'error handling in HTTP requests'\n")
	fmt.Printf("  - Try different terms: 'database' vs 'DB' vs 'storage'\n")
}