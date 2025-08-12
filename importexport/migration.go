package importexport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Caia-Tech/govc"
)

// MigrationManager handles migrations from various Git hosting services
type MigrationManager struct {
	govcRepo *govc.Repository
	tempDir  string
	progress MigrationProgress
}

// MigrationProgress tracks migration progress
type MigrationProgress struct {
	CurrentPhase   string
	TotalSteps     int
	CompletedSteps int
	Repositories   int
	CurrentRepo    string
	Errors         []error
	StartTime      time.Time
	EstimatedTime  time.Duration
}

// GitHubRepo represents a GitHub repository
type GitHubRepo struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	CloneURL    string `json:"clone_url"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	Fork        bool   `json:"fork"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// GitLabProject represents a GitLab project
type GitLabProject struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	HTTPURLToRepo string `json:"http_url_to_repo"`
	Description   string `json:"description"`
	Visibility    string `json:"visibility"`
	CreatedAt     string `json:"created_at"`
	LastActivity  string `json:"last_activity_at"`
}

// MigrationOptions contains migration configuration
type MigrationOptions struct {
	Source         string            // github, gitlab, bitbucket
	Organization   string            // Organization/namespace to migrate
	Token          string            // API token for authentication
	IncludeForks   bool              // Include forked repositories
	IncludePrivate bool              // Include private repositories
	Filters        map[string]string // Additional filters
	DryRun         bool              // Don't actually migrate, just show what would be done
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(govcRepo *govc.Repository) *MigrationManager {
	tempDir := filepath.Join(os.TempDir(), "govc-migration")
	os.MkdirAll(tempDir, 0755)

	return &MigrationManager{
		govcRepo: govcRepo,
		tempDir:  tempDir,
		progress: MigrationProgress{
			StartTime: time.Now(),
		},
	}
}

// Migrate performs migration from specified source
func (mm *MigrationManager) Migrate(ctx context.Context, opts MigrationOptions) error {
	mm.progress.CurrentPhase = "Starting migration"

	switch strings.ToLower(opts.Source) {
	case "github":
		return mm.migrateFromGitHub(ctx, opts)
	case "gitlab":
		return mm.migrateFromGitLab(ctx, opts)
	case "bitbucket":
		return mm.migrateFromBitbucket(ctx, opts)
	default:
		return fmt.Errorf("unsupported source: %s", opts.Source)
	}
}

// GetProgress returns current migration progress
func (mm *MigrationManager) GetProgress() MigrationProgress {
	elapsed := time.Since(mm.progress.StartTime)
	if mm.progress.CompletedSteps > 0 && mm.progress.TotalSteps > 0 {
		rate := elapsed / time.Duration(mm.progress.CompletedSteps)
		remaining := time.Duration(mm.progress.TotalSteps-mm.progress.CompletedSteps) * rate
		mm.progress.EstimatedTime = remaining
	}
	return mm.progress
}

func (mm *MigrationManager) migrateFromGitHub(ctx context.Context, opts MigrationOptions) error {
	mm.progress.CurrentPhase = "Fetching GitHub repositories"

	// Get list of repositories
	repos, err := mm.getGitHubRepos(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to fetch GitHub repos: %w", err)
	}

	mm.progress.TotalSteps = len(repos)
	mm.progress.Repositories = len(repos)

	// Migrate each repository
	for i, repo := range repos {
		mm.progress.CurrentRepo = repo.FullName
		mm.progress.CurrentPhase = fmt.Sprintf("Migrating %s (%d/%d)", repo.FullName, i+1, len(repos))

		if opts.DryRun {
			fmt.Printf("Would migrate: %s\n", repo.FullName)
		} else {
			if err := mm.migrateRepository(ctx, repo.CloneURL, repo.Name, repo.Description); err != nil {
				mm.progress.Errors = append(mm.progress.Errors, fmt.Errorf("failed to migrate %s: %w", repo.FullName, err))
			}
		}

		mm.progress.CompletedSteps++

		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	mm.progress.CurrentPhase = "Migration complete"
	return nil
}

func (mm *MigrationManager) getGitHubRepos(ctx context.Context, opts MigrationOptions) ([]GitHubRepo, error) {
	var repos []GitHubRepo
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("https://api.github.com/orgs/%s/repos?page=%d&per_page=%d", opts.Organization, page, perPage)
		if opts.Organization == "" {
			url = fmt.Sprintf("https://api.github.com/user/repos?page=%d&per_page=%d", page, perPage)
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}

		if opts.Token != "" {
			req.Header.Set("Authorization", "token "+opts.Token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("GitHub API error: %s", string(body))
		}

		var pageRepos []GitHubRepo
		if err := json.NewDecoder(resp.Body).Decode(&pageRepos); err != nil {
			return nil, err
		}

		if len(pageRepos) == 0 {
			break
		}

		// Apply filters
		for _, repo := range pageRepos {
			if !opts.IncludeForks && repo.Fork {
				continue
			}
			if !opts.IncludePrivate && repo.Private {
				continue
			}
			repos = append(repos, repo)
		}

		if len(pageRepos) < perPage {
			break
		}
		page++
	}

	return repos, nil
}

func (mm *MigrationManager) migrateFromGitLab(ctx context.Context, opts MigrationOptions) error {
	mm.progress.CurrentPhase = "Fetching GitLab projects"

	// Similar implementation for GitLab
	projects, err := mm.getGitLabProjects(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to fetch GitLab projects: %w", err)
	}

	mm.progress.TotalSteps = len(projects)
	mm.progress.Repositories = len(projects)

	for i, project := range projects {
		mm.progress.CurrentRepo = project.Path
		mm.progress.CurrentPhase = fmt.Sprintf("Migrating %s (%d/%d)", project.Path, i+1, len(projects))

		if opts.DryRun {
			fmt.Printf("Would migrate: %s\n", project.Path)
		} else {
			if err := mm.migrateRepository(ctx, project.HTTPURLToRepo, project.Name, project.Description); err != nil {
				mm.progress.Errors = append(mm.progress.Errors, fmt.Errorf("failed to migrate %s: %w", project.Path, err))
			}
		}

		mm.progress.CompletedSteps++

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

func (mm *MigrationManager) getGitLabProjects(ctx context.Context, opts MigrationOptions) ([]GitLabProject, error) {
	var projects []GitLabProject
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("https://gitlab.com/api/v4/groups/%s/projects?page=%d&per_page=%d", opts.Organization, page, perPage)
		if opts.Organization == "" {
			url = fmt.Sprintf("https://gitlab.com/api/v4/projects?owned=true&page=%d&per_page=%d", page, perPage)
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}

		if opts.Token != "" {
			req.Header.Set("Private-Token", opts.Token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var pageProjects []GitLabProject
		if err := json.NewDecoder(resp.Body).Decode(&pageProjects); err != nil {
			return nil, err
		}

		if len(pageProjects) == 0 {
			break
		}

		// Apply filters
		for _, project := range pageProjects {
			if !opts.IncludePrivate && project.Visibility == "private" {
				continue
			}
			projects = append(projects, project)
		}

		if len(pageProjects) < perPage {
			break
		}
		page++
	}

	return projects, nil
}

func (mm *MigrationManager) migrateFromBitbucket(ctx context.Context, opts MigrationOptions) error {
	// Similar implementation for Bitbucket
	return fmt.Errorf("Bitbucket migration not yet implemented")
}

func (mm *MigrationManager) migrateRepository(ctx context.Context, cloneURL, name, description string) error {
	// Create temporary directory for this repo
	repoDir := filepath.Join(mm.tempDir, name)
	os.RemoveAll(repoDir) // Clean up any previous attempts

	// Clone the repository
	if err := mm.cloneRepo(ctx, cloneURL, repoDir); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}
	defer os.RemoveAll(repoDir) // Clean up

	// Create govc repository
	govcRepo := govc.NewRepository()
	// TODO: Set path to filepath.Join(mm.tempDir, name+"-govc")

	// Import from Git to govc
	importer := NewGitImporter(govcRepo, repoDir)
	if err := importer.Import(); err != nil {
		return fmt.Errorf("failed to import to govc: %w", err)
	}

	return nil
}

func (mm *MigrationManager) cloneRepo(ctx context.Context, cloneURL, destDir string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", "--mirror", cloneURL, destDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// BatchMigrationConfig contains configuration for batch migrations
type BatchMigrationConfig struct {
	Organizations []string          `yaml:"organizations"`
	Repositories  []string          `yaml:"repositories"`
	Mapping       map[string]string `yaml:"mapping"` // old name -> new name
	Parallel      int               `yaml:"parallel"`
	RetryCount    int               `yaml:"retry_count"`
}

// MigrateBatch performs batch migration with configuration
func (mm *MigrationManager) MigrateBatch(ctx context.Context, config BatchMigrationConfig, opts MigrationOptions) error {
	// Implementation for batch migration with parallel processing
	// This would use goroutines and channels for concurrent migration

	semaphore := make(chan struct{}, config.Parallel)
	errChan := make(chan error, len(config.Repositories))

	for _, repo := range config.Repositories {
		go func(repoName string) {
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			// Migrate individual repository
			if err := mm.migrateRepository(ctx, fmt.Sprintf("https://github.com/%s.git", repoName), repoName, ""); err != nil {
				errChan <- fmt.Errorf("failed to migrate %s: %w", repoName, err)
			} else {
				errChan <- nil
			}
		}(repo)
	}

	// Collect results
	var errors []error
	for i := 0; i < len(config.Repositories); i++ {
		if err := <-errChan; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("migration completed with %d errors", len(errors))
	}

	return nil
}

// Cleanup removes temporary files
func (mm *MigrationManager) Cleanup() error {
	return os.RemoveAll(mm.tempDir)
}
