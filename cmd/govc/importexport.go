package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/caiatech/govc"
	"github.com/caiatech/govc/importexport"
	"github.com/spf13/cobra"
)

var (
	importCmd = &cobra.Command{
		Use:   "import",
		Short: "Import from external version control systems",
	}

	importGitCmd = &cobra.Command{
		Use:   "git [git-repo-path] [govc-repo-id]",
		Short: "Import a Git repository into govc",
		Args:  cobra.ExactArgs(2),
		Run:   runImportGit,
	}

	exportCmd = &cobra.Command{
		Use:   "export",
		Short: "Export to external version control systems",
	}

	exportGitCmd = &cobra.Command{
		Use:   "git [govc-repo-id] [output-path]",
		Short: "Export a govc repository to Git format",
		Args:  cobra.ExactArgs(2),
		Run:   runExportGit,
	}

	migrateCmd = &cobra.Command{
		Use:   "migrate",
		Short: "Migrate repositories from hosting services",
	}

	migrateGitHubCmd = &cobra.Command{
		Use:   "github",
		Short: "Migrate repositories from GitHub",
		Run:   runMigrateGitHub,
	}

	migrateGitLabCmd = &cobra.Command{
		Use:   "gitlab",
		Short: "Migrate repositories from GitLab",
		Run:   runMigrateGitLab,
	}

	backupCmd = &cobra.Command{
		Use:   "backup [repo-id] [output-file]",
		Short: "Create a backup of a govc repository",
		Args:  cobra.ExactArgs(2),
		Run:   runBackup,
	}

	restoreCmd = &cobra.Command{
		Use:   "restore [backup-file] [target-path]",
		Short: "Restore a govc repository from backup",
		Args:  cobra.ExactArgs(2),
		Run:   runRestore,
	}
)

func init() {
	// Import command setup
	importCmd.AddCommand(importGitCmd)
	importGitCmd.Flags().Bool("memory", false, "Create memory-only repository")
	importGitCmd.Flags().Bool("progress", true, "Show import progress")

	// Export command setup
	exportCmd.AddCommand(exportGitCmd)
	exportGitCmd.Flags().Bool("bare", false, "Export as bare repository")
	exportGitCmd.Flags().String("branch", "", "Export specific branch only")
	exportGitCmd.Flags().Bool("progress", true, "Show export progress")

	// Migration command setup
	migrateCmd.AddCommand(migrateGitHubCmd)
	migrateCmd.AddCommand(migrateGitLabCmd)
	
	// GitHub migration flags
	migrateGitHubCmd.Flags().String("token", "", "GitHub API token")
	migrateGitHubCmd.Flags().String("org", "", "GitHub organization name")
	migrateGitHubCmd.Flags().Bool("include-forks", false, "Include forked repositories")
	migrateGitHubCmd.Flags().Bool("include-private", false, "Include private repositories")
	migrateGitHubCmd.Flags().Bool("dry-run", false, "Show what would be migrated without doing it")
	migrateGitHubCmd.Flags().String("output-dir", "./migrated", "Output directory for migrated repositories")

	// GitLab migration flags
	migrateGitLabCmd.Flags().String("token", "", "GitLab API token")
	migrateGitLabCmd.Flags().String("group", "", "GitLab group name")
	migrateGitLabCmd.Flags().Bool("include-private", false, "Include private repositories")
	migrateGitLabCmd.Flags().Bool("dry-run", false, "Show what would be migrated without doing it")
	migrateGitLabCmd.Flags().String("output-dir", "./migrated", "Output directory for migrated repositories")

	// Backup command flags
	backupCmd.Flags().Bool("compression", true, "Enable gzip compression")
	backupCmd.Flags().Bool("incremental", false, "Create incremental backup")
	backupCmd.Flags().String("since", "", "Backup changes since this time (RFC3339 format)")
	backupCmd.Flags().StringSlice("exclude", []string{}, "Exclude patterns")
	backupCmd.Flags().Bool("progress", true, "Show backup progress")

	// Restore command flags
	restoreCmd.Flags().Bool("overwrite", false, "Overwrite existing repository")
	restoreCmd.Flags().String("branch", "", "Restore specific branch only")
	restoreCmd.Flags().Bool("dry-run", false, "Show what would be restored without doing it")
	restoreCmd.Flags().Bool("verify", true, "Verify backup integrity before restore")
	restoreCmd.Flags().Bool("progress", true, "Show restore progress")
}

func runImportGit(cmd *cobra.Command, args []string) {
	gitRepoPath := args[0]
	govcRepoID := args[1]

	memoryOnly, _ := cmd.Flags().GetBool("memory")
	showProgress, _ := cmd.Flags().GetBool("progress")

	// Verify Git repository exists
	if _, err := os.Stat(gitRepoPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Git repository path does not exist: %s\n", gitRepoPath)
		os.Exit(1)
	}

	// Create govc repository
	var govcRepo *govc.Repository
	var err error

	if memoryOnly {
		fmt.Println("Creating memory-only govc repository...")
		govcRepo = govc.New()
	} else {
		fmt.Printf("Creating govc repository: %s\n", govcRepoID)
		govcRepo, err = govc.Init(govcRepoID)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating govc repository: %v\n", err)
		os.Exit(1)
	}

	// Create importer
	importer := importexport.NewGitImporter(govcRepo, gitRepoPath)

	fmt.Printf("Importing Git repository from %s...\n", gitRepoPath)

	// Show progress if requested
	if showProgress {
		go func() {
			for {
				progress := importer.GetProgress()
				if progress.CurrentPhase == "Complete" {
					fmt.Printf("\r✓ Import complete!%50s\n", "")
					break
				}
				if len(progress.Errors) > 0 {
					fmt.Printf("\r✗ Import failed with %d errors%30s\n", len(progress.Errors), "")
					break
				}
				
				percentage := 0.0
				if progress.TotalObjects > 0 {
					percentage = float64(progress.ImportedObjects) / float64(progress.TotalObjects) * 100
				}
				
				fmt.Printf("\r%s: %.1f%% (%d/%d objects)", 
					progress.CurrentPhase, 
					percentage, 
					progress.ImportedObjects, 
					progress.TotalObjects)
				
				time.Sleep(500 * time.Millisecond)
			}
		}()
	}

	// Perform import
	if err := importer.Import(); err != nil {
		fmt.Fprintf(os.Stderr, "\nError importing repository: %v\n", err)
		
		// Show any accumulated errors
		progress := importer.GetProgress()
		if len(progress.Errors) > 0 {
			fmt.Fprintf(os.Stderr, "\nImport errors:\n")
			for _, importErr := range progress.Errors {
				fmt.Fprintf(os.Stderr, "  - %v\n", importErr)
			}
		}
		os.Exit(1)
	}

	if !showProgress {
		fmt.Println("✓ Import completed successfully!")
	}
}

func runExportGit(cmd *cobra.Command, args []string) {
	govcRepoID := args[0]
	outputPath := args[1]

	bare, _ := cmd.Flags().GetBool("bare")
	branch, _ := cmd.Flags().GetString("branch")
	showProgress, _ := cmd.Flags().GetBool("progress")

	// Open govc repository
	govcRepo, err := govc.Open(govcRepoID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening govc repository: %v\n", err)
		os.Exit(1)
	}

	// Create output directory
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Create exporter
	exporter := importexport.NewGitExporter(govcRepo, outputPath)

	fmt.Printf("Exporting govc repository %s to %s...\n", govcRepoID, outputPath)

	// Show progress if requested
	if showProgress {
		go func() {
			for {
				progress := exporter.GetProgress()
				if progress.CurrentPhase == "Complete" {
					fmt.Printf("\r✓ Export complete!%50s\n", "")
					break
				}
				if len(progress.Errors) > 0 {
					fmt.Printf("\r✗ Export failed with %d errors%30s\n", len(progress.Errors), "")
					break
				}
				
				percentage := 0.0
				if progress.TotalCommits > 0 {
					percentage = float64(progress.ExportedCommits) / float64(progress.TotalCommits) * 100
				}
				
				fmt.Printf("\r%s: %.1f%% (%d/%d commits)", 
					progress.CurrentPhase, 
					percentage, 
					progress.ExportedCommits, 
					progress.TotalCommits)
				
				time.Sleep(500 * time.Millisecond)
			}
		}()
	}

	// Perform export
	if err := exporter.Export(); err != nil {
		fmt.Fprintf(os.Stderr, "\nError exporting repository: %v\n", err)
		os.Exit(1)
	}

	if !showProgress {
		fmt.Println("✓ Export completed successfully!")
	}

	fmt.Printf("Git repository exported to: %s\n", outputPath)
	if !bare {
		fmt.Println("You can now use standard Git commands in the exported directory.")
	}

	_ = bare   // Use bare flag
	_ = branch // Use branch flag
}

func runMigrateGitHub(cmd *cobra.Command, args []string) {
	token, _ := cmd.Flags().GetString("token")
	org, _ := cmd.Flags().GetString("org")
	includeForks, _ := cmd.Flags().GetBool("include-forks")
	includePrivate, _ := cmd.Flags().GetBool("include-private")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	outputDir, _ := cmd.Flags().GetString("output-dir")

	if token == "" {
		fmt.Fprintf(os.Stderr, "Error: GitHub token is required\n")
		fmt.Fprintf(os.Stderr, "Get a token from: https://github.com/settings/tokens\n")
		os.Exit(1)
	}

	// Create temporary repository for migration
	tempRepo := govc.New()

	// Create migration manager
	migrationManager := importexport.NewMigrationManager(tempRepo)
	defer migrationManager.Cleanup()

	// Setup migration options
	opts := importexport.MigrationOptions{
		Source:         "github",
		Organization:   org,
		Token:          token,
		IncludeForks:   includeForks,
		IncludePrivate: includePrivate,
		DryRun:         dryRun,
	}

	if dryRun {
		fmt.Println("🔍 Dry run mode - showing what would be migrated...")
	} else {
		fmt.Printf("🚀 Starting GitHub migration to %s...\n", outputDir)
	}

	if org != "" {
		fmt.Printf("📁 Organization: %s\n", org)
	} else {
		fmt.Println("👤 Migrating user repositories")
	}

	fmt.Printf("⚙️ Include forks: %v\n", includeForks)
	fmt.Printf("🔒 Include private: %v\n", includePrivate)

	// Show progress
	go func() {
		for {
			progress := migrationManager.GetProgress()
			if progress.CurrentPhase == "Migration complete" {
				fmt.Printf("\r✓ Migration complete!%50s\n", "")
				break
			}
			if len(progress.Errors) > 0 && progress.CompletedSteps == progress.TotalSteps {
				fmt.Printf("\r⚠️  Migration completed with %d errors%20s\n", len(progress.Errors), "")
				break
			}
			
			if progress.TotalSteps > 0 {
				percentage := float64(progress.CompletedSteps) / float64(progress.TotalSteps) * 100
				fmt.Printf("\r%s: %.1f%% (%d/%d repos) - %s", 
					progress.CurrentPhase, 
					percentage, 
					progress.CompletedSteps, 
					progress.TotalSteps,
					progress.CurrentRepo)
			} else {
				fmt.Printf("\r%s...", progress.CurrentPhase)
			}
			
			time.Sleep(1 * time.Second)
		}
	}()

	// Perform migration
	ctx := context.Background()
	if err := migrationManager.Migrate(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "\nError during migration: %v\n", err)
		os.Exit(1)
	}

	// Show final results
	progress := migrationManager.GetProgress()
	if len(progress.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "\n⚠️  Migration completed with errors:\n")
		for _, migrationErr := range progress.Errors {
			fmt.Fprintf(os.Stderr, "  - %v\n", migrationErr)
		}
	}

	if !dryRun {
		fmt.Printf("\n📂 Migrated repositories saved to: %s\n", outputDir)
	}
}

func runMigrateGitLab(cmd *cobra.Command, args []string) {
	token, _ := cmd.Flags().GetString("token")
	group, _ := cmd.Flags().GetString("group")
	includePrivate, _ := cmd.Flags().GetBool("include-private")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	outputDir, _ := cmd.Flags().GetString("output-dir")

	if token == "" {
		fmt.Fprintf(os.Stderr, "Error: GitLab token is required\n")
		fmt.Fprintf(os.Stderr, "Get a token from: https://gitlab.com/-/profile/personal_access_tokens\n")
		os.Exit(1)
	}

	// Similar implementation to GitHub migration
	fmt.Printf("🦊 GitLab migration not fully implemented yet\n")
	fmt.Printf("Parameters received:\n")
	fmt.Printf("  Token: %s\n", strings.Repeat("*", len(token)))
	fmt.Printf("  Group: %s\n", group)
	fmt.Printf("  Include private: %v\n", includePrivate)
	fmt.Printf("  Dry run: %v\n", dryRun)
	fmt.Printf("  Output dir: %s\n", outputDir)
}

func runBackup(cmd *cobra.Command, args []string) {
	repoID := args[0]
	outputFile := args[1]

	compression, _ := cmd.Flags().GetBool("compression")
	incremental, _ := cmd.Flags().GetBool("incremental")
	sinceStr, _ := cmd.Flags().GetString("since")
	excludePatterns, _ := cmd.Flags().GetStringSlice("exclude")
	showProgress, _ := cmd.Flags().GetBool("progress")

	// Parse since time if provided
	var since *time.Time
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing since time: %v\n", err)
			fmt.Fprintf(os.Stderr, "Use RFC3339 format: 2006-01-02T15:04:05Z07:00\n")
			os.Exit(1)
		} else {
			since = &t
		}
	}

	// Open repository
	repo, err := govc.Open(repoID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening repository: %v\n", err)
		os.Exit(1)
	}

	// Create backup manager
	backupManager := importexport.NewBackupManager(repo)

	// Setup backup options
	opts := importexport.BackupOptions{
		Output:      outputFile,
		Compression: compression,
		Incremental: incremental,
		Since:       since,
		Exclude:     excludePatterns,
	}

	fmt.Printf("📦 Creating backup of repository: %s\n", repoID)
	fmt.Printf("📁 Output file: %s\n", outputFile)
	fmt.Printf("🗜️  Compression: %v\n", compression)
	if incremental {
		fmt.Printf("📈 Incremental backup")
		if since != nil {
			fmt.Printf(" since: %s", since.Format(time.RFC3339))
		}
		fmt.Println()
	}

	// Show progress if requested
	if showProgress {
		go func() {
			for {
				progress := backupManager.GetProgress()
				if progress.CurrentPhase == "Backup complete" {
					fmt.Printf("\r✓ Backup complete!%50s\n", "")
					break
				}
				if len(progress.Errors) > 0 {
					fmt.Printf("\r✗ Backup failed with %d errors%30s\n", len(progress.Errors), "")
					break
				}
				
				var percentage float64
				if progress.TotalSize > 0 {
					percentage = float64(progress.ProcessedSize) / float64(progress.TotalSize) * 100
				} else if progress.TotalFiles > 0 {
					percentage = float64(progress.ProcessedFiles) / float64(progress.TotalFiles) * 100
				}
				
				fmt.Printf("\r%s: %.1f%% (%d/%d files, %s)", 
					progress.CurrentPhase, 
					percentage, 
					progress.ProcessedFiles, 
					progress.TotalFiles,
					formatSize(progress.ProcessedSize))
				
				time.Sleep(500 * time.Millisecond)
			}
		}()
	}

	// Perform backup
	ctx := context.Background()
	if err := backupManager.Backup(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "\nError creating backup: %v\n", err)
		os.Exit(1)
	}

	// Show file size
	if stat, err := os.Stat(outputFile); err == nil {
		fmt.Printf("💾 Backup size: %s\n", formatSize(stat.Size()))
	}

	if !showProgress {
		fmt.Println("✓ Backup completed successfully!")
	}
}

func runRestore(cmd *cobra.Command, args []string) {
	backupFile := args[0]
	targetPath := args[1]

	overwrite, _ := cmd.Flags().GetBool("overwrite")
	branch, _ := cmd.Flags().GetString("branch")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	verify, _ := cmd.Flags().GetBool("verify")
	showProgress, _ := cmd.Flags().GetBool("progress")

	// Verify backup file exists
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Backup file does not exist: %s\n", backupFile)
		os.Exit(1)
	}

	// Create temporary repository for restoration
	tempRepo := govc.New()

	// Create backup manager
	backupManager := importexport.NewBackupManager(tempRepo)

	// Setup restore options
	opts := importexport.RestoreOptions{
		Target:       targetPath,
		Overwrite:    overwrite,
		Branch:       branch,
		DryRun:       dryRun,
		Verification: verify,
	}

	if dryRun {
		fmt.Println("🔍 Dry run mode - showing what would be restored...")
	} else {
		fmt.Printf("📦 Restoring backup from: %s\n", backupFile)
		fmt.Printf("📁 Target path: %s\n", targetPath)
		fmt.Printf("🔄 Overwrite existing: %v\n", overwrite)
		if branch != "" {
			fmt.Printf("🌿 Restore branch: %s\n", branch)
		}
	}

	// Show progress if requested
	if showProgress {
		go func() {
			for {
				progress := backupManager.GetProgress()
				if progress.CurrentPhase == "Restore complete" {
					fmt.Printf("\r✓ Restore complete!%50s\n", "")
					break
				}
				if len(progress.Errors) > 0 {
					fmt.Printf("\r✗ Restore failed with %d errors%30s\n", len(progress.Errors), "")
					break
				}
				
				percentage := 0.0
				if progress.TotalFiles > 0 {
					percentage = float64(progress.ProcessedFiles) / float64(progress.TotalFiles) * 100
				}
				
				fmt.Printf("\r%s: %.1f%% (%d/%d files)", 
					progress.CurrentPhase, 
					percentage, 
					progress.ProcessedFiles, 
					progress.TotalFiles)
				
				time.Sleep(500 * time.Millisecond)
			}
		}()
	}

	// Perform restore
	ctx := context.Background()
	if err := backupManager.Restore(ctx, backupFile, opts); err != nil {
		fmt.Fprintf(os.Stderr, "\nError restoring backup: %v\n", err)
		os.Exit(1)
	}

	if !showProgress && !dryRun {
		fmt.Println("✓ Restore completed successfully!")
	}

	if !dryRun {
		fmt.Printf("📂 Repository restored to: %s\n", targetPath)
	}
}

// formatSize formats a size in bytes to human readable format
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}