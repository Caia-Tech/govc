package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/caiatech/govc"
	"github.com/caiatech/govc/importexport"
	"github.com/gin-gonic/gin"
)

// Import/Export API handlers

// ImportGitRequest represents a Git import request
type ImportGitRequest struct {
	GitRepoPath string `json:"git_repo_path" binding:"required"`
	RepoID      string `json:"repo_id" binding:"required"`
	MemoryOnly  bool   `json:"memory_only"`
}

// ImportGitResponse represents a Git import response
type ImportGitResponse struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	StartedAt time.Time `json:"started_at"`
}

// ExportGitRequest represents a Git export request
type ExportGitRequest struct {
	RepoID     string `json:"repo_id" binding:"required"`
	OutputPath string `json:"output_path" binding:"required"`
	Bare       bool   `json:"bare"`
	Branch     string `json:"branch"`
}

// ExportGitResponse represents a Git export response
type ExportGitResponse struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	StartedAt time.Time `json:"started_at"`
}

// MigrateRequest represents a migration request
type MigrateRequest struct {
	Source         string            `json:"source" binding:"required"`
	Organization   string            `json:"organization"`
	Token          string            `json:"token" binding:"required"`
	IncludeForks   bool              `json:"include_forks"`
	IncludePrivate bool              `json:"include_private"`
	DryRun         bool              `json:"dry_run"`
	Filters        map[string]string `json:"filters"`
}

// MigrateResponse represents a migration response
type MigrateResponse struct {
	JobID        string    `json:"job_id"`
	Status       string    `json:"status"`
	Message      string    `json:"message"`
	StartedAt    time.Time `json:"started_at"`
	Repositories int       `json:"repositories"`
}

// BackupRequest represents a backup request
type BackupRequest struct {
	RepoID      string            `json:"repo_id" binding:"required"`
	OutputPath  string            `json:"output_path" binding:"required"`
	Compression bool              `json:"compression"`
	Incremental bool              `json:"incremental"`
	Since       *time.Time        `json:"since"`
	Metadata    map[string]string `json:"metadata"`
}

// BackupResponse represents a backup response
type BackupResponse struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	StartedAt time.Time `json:"started_at"`
	Size      int64     `json:"size,omitempty"`
}

// RestoreRequest represents a restore request
type RestoreRequest struct {
	BackupPath string            `json:"backup_path" binding:"required"`
	TargetRepo string            `json:"target_repo" binding:"required"`
	Overwrite  bool              `json:"overwrite"`
	Branch     string            `json:"branch"`
	DryRun     bool              `json:"dry_run"`
	Metadata   map[string]string `json:"metadata"`
}

// RestoreResponse represents a restore response
type RestoreResponse struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	StartedAt time.Time `json:"started_at"`
}

// ProgressResponse represents progress information
type ProgressResponse struct {
	JobID         string        `json:"job_id"`
	Status        string        `json:"status"`
	CurrentPhase  string        `json:"current_phase"`
	Progress      float64       `json:"progress"`
	EstimatedTime time.Duration `json:"estimated_time,omitempty"`
	StartedAt     time.Time     `json:"started_at"`
	CompletedAt   *time.Time    `json:"completed_at,omitempty"`
	Errors        []string      `json:"errors,omitempty"`
	Details       interface{}   `json:"details,omitempty"`
}

// Job tracking for long-running operations
type Job struct {
	ID        string
	Type      string
	Status    string
	StartedAt time.Time
	Progress  interface{}
	Context   context.Context
	Cancel    context.CancelFunc
	Error     error
}

var jobs = make(map[string]*Job)

// ImportGitHandler handles Git import requests
func (s *Server) ImportGitHandler(c *gin.Context) {
	var req ImportGitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify Git repository exists
	if _, err := os.Stat(req.GitRepoPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Git repository path does not exist"})
		return
	}

	// Create or get govc repository
	var govcRepo *govc.Repository
	var err error

	// Check if repository already exists
	s.mu.RLock()
	if _, exists := s.repoMetadata[req.RepoID]; exists {
		s.mu.RUnlock()
		c.JSON(http.StatusConflict, gin.H{"error": "Repository already exists"})
		return
	}
	s.mu.RUnlock()

	var repoPath string
	if req.MemoryOnly {
		repoPath = ":memory:"
		govcRepo = govc.New()
		err = nil
	} else {
		repoPath = filepath.Join("/tmp", req.RepoID) // This should be configurable
		govcRepo, err = govc.Init(repoPath)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create govc repository: %v", err)})
		return
	}

	// Create job
	jobID := generateJobID()
	ctx, cancel := context.WithCancel(context.Background())

	job := &Job{
		ID:        jobID,
		Type:      "import",
		Status:    "running",
		StartedAt: time.Now(),
		Context:   ctx,
		Cancel:    cancel,
	}
	jobs[jobID] = job

	// Start import in background
	go func() {
		defer func() {
			if r := recover(); r != nil {
				job.Status = "failed"
				job.Error = fmt.Errorf("panic: %v", r)
			}
		}()

		importer := importexport.NewGitImporter(govcRepo, req.GitRepoPath)
		if err := importer.Import(); err != nil {
			job.Status = "failed"
			job.Error = err
		} else {
			job.Status = "completed"
			// Register repository metadata
			s.mu.Lock()
			s.repoMetadata[req.RepoID] = &RepoMetadata{
				ID:        req.RepoID,
				CreatedAt: time.Now(),
				Path:      repoPath,
			}
			s.mu.Unlock()
		}
	}()

	response := ImportGitResponse{
		JobID:     jobID,
		Status:    "running",
		Message:   "Import started",
		StartedAt: job.StartedAt,
	}

	c.JSON(http.StatusAccepted, response)
}

// ExportGitHandler handles Git export requests
func (s *Server) ExportGitHandler(c *gin.Context) {
	var req ExportGitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get repository
	repo, err := s.getRepository(req.RepoID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Repository not found"})
		return
	}

	// Verify output directory exists
	outputDir := filepath.Dir(req.OutputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Cannot create output directory: %v", err)})
		return
	}

	// Create job
	jobID := generateJobID()
	ctx, cancel := context.WithCancel(context.Background())

	job := &Job{
		ID:        jobID,
		Type:      "export",
		Status:    "running",
		StartedAt: time.Now(),
		Context:   ctx,
		Cancel:    cancel,
	}
	jobs[jobID] = job

	// Start export in background
	go func() {
		defer func() {
			if r := recover(); r != nil {
				job.Status = "failed"
				job.Error = fmt.Errorf("panic: %v", r)
			}
		}()

		exporter := importexport.NewGitExporter(repo, req.OutputPath)
		if err := exporter.Export(); err != nil {
			job.Status = "failed"
			job.Error = err
		} else {
			job.Status = "completed"
		}
	}()

	response := ExportGitResponse{
		JobID:     jobID,
		Status:    "running",
		Message:   "Export started",
		StartedAt: job.StartedAt,
	}

	c.JSON(http.StatusAccepted, response)
}

// MigrateHandler handles migration requests
func (s *Server) MigrateHandler(c *gin.Context) {
	var req MigrateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create temporary govc repository for migration
	tempRepo := govc.New()

	// Create job
	jobID := generateJobID()
	ctx, cancel := context.WithCancel(context.Background())

	job := &Job{
		ID:        jobID,
		Type:      "migrate",
		Status:    "running",
		StartedAt: time.Now(),
		Context:   ctx,
		Cancel:    cancel,
	}
	jobs[jobID] = job

	// Start migration in background
	go func() {
		defer func() {
			if r := recover(); r != nil {
				job.Status = "failed"
				job.Error = fmt.Errorf("panic: %v", r)
			}
		}()

		migrationManager := importexport.NewMigrationManager(tempRepo)
		opts := importexport.MigrationOptions{
			Source:         req.Source,
			Organization:   req.Organization,
			Token:          req.Token,
			IncludeForks:   req.IncludeForks,
			IncludePrivate: req.IncludePrivate,
			DryRun:         req.DryRun,
			Filters:        req.Filters,
		}

		if err := migrationManager.Migrate(ctx, opts); err != nil {
			job.Status = "failed"
			job.Error = err
		} else {
			job.Status = "completed"
		}

		// Cleanup
		migrationManager.Cleanup()
	}()

	response := MigrateResponse{
		JobID:     jobID,
		Status:    "running",
		Message:   "Migration started",
		StartedAt: job.StartedAt,
	}

	c.JSON(http.StatusAccepted, response)
}

// BackupHandler handles backup requests
func (s *Server) BackupHandler(c *gin.Context) {
	var req BackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get repository
	repo, err := s.getRepository(req.RepoID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Repository not found"})
		return
	}

	// Create job
	jobID := generateJobID()
	ctx, cancel := context.WithCancel(context.Background())

	job := &Job{
		ID:        jobID,
		Type:      "backup",
		Status:    "running",
		StartedAt: time.Now(),
		Context:   ctx,
		Cancel:    cancel,
	}
	jobs[jobID] = job

	// Start backup in background
	go func() {
		defer func() {
			if r := recover(); r != nil {
				job.Status = "failed"
				job.Error = fmt.Errorf("panic: %v", r)
			}
		}()

		backupManager := importexport.NewBackupManager(repo)
		opts := importexport.BackupOptions{
			Output:      req.OutputPath,
			Compression: req.Compression,
			Incremental: req.Incremental,
			Since:       req.Since,
			Metadata:    req.Metadata,
		}

		if err := backupManager.Backup(ctx, opts); err != nil {
			job.Status = "failed"
			job.Error = err
		} else {
			job.Status = "completed"
		}
	}()

	response := BackupResponse{
		JobID:     jobID,
		Status:    "running",
		Message:   "Backup started",
		StartedAt: job.StartedAt,
	}

	c.JSON(http.StatusAccepted, response)
}

// RestoreHandler handles restore requests
func (s *Server) RestoreHandler(c *gin.Context) {
	var req RestoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify backup file exists
	if _, err := os.Stat(req.BackupPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Backup file does not exist"})
		return
	}

	// Create temporary repository for restoration
	tempRepo := govc.New()

	// Create job
	jobID := generateJobID()
	ctx, cancel := context.WithCancel(context.Background())

	job := &Job{
		ID:        jobID,
		Type:      "restore",
		Status:    "running",
		StartedAt: time.Now(),
		Context:   ctx,
		Cancel:    cancel,
	}
	jobs[jobID] = job

	// Start restore in background
	go func() {
		defer func() {
			if r := recover(); r != nil {
				job.Status = "failed"
				job.Error = fmt.Errorf("panic: %v", r)
			}
		}()

		backupManager := importexport.NewBackupManager(tempRepo)
		opts := importexport.RestoreOptions{
			Target:    req.TargetRepo,
			Overwrite: req.Overwrite,
			Branch:    req.Branch,
			DryRun:    req.DryRun,
			Metadata:  req.Metadata,
		}

		if err := backupManager.Restore(ctx, req.BackupPath, opts); err != nil {
			job.Status = "failed"
			job.Error = err
		} else {
			job.Status = "completed"

			// If not dry run, load restored repository
			if !req.DryRun {
				// Register restored repository
				if _, err := govc.Open(req.TargetRepo); err == nil {
					s.mu.Lock()
					s.repoMetadata[req.TargetRepo] = &RepoMetadata{
						ID:        req.TargetRepo,
						CreatedAt: time.Now(),
						Path:      req.TargetRepo,
					}
					s.mu.Unlock()
				}
			}
		}
	}()

	response := RestoreResponse{
		JobID:     jobID,
		Status:    "running",
		Message:   "Restore started",
		StartedAt: job.StartedAt,
	}

	c.JSON(http.StatusAccepted, response)
}

// ProgressHandler returns job progress
func (s *Server) ProgressHandler(c *gin.Context) {
	jobID := c.Param("job_id")

	job, exists := jobs[jobID]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	progress := ProgressResponse{
		JobID:     job.ID,
		Status:    job.Status,
		StartedAt: job.StartedAt,
	}

	if job.Status == "completed" || job.Status == "failed" {
		now := time.Now()
		progress.CompletedAt = &now
	}

	if job.Error != nil {
		progress.Errors = []string{job.Error.Error()}
	}

	// Add type-specific progress information
	switch job.Type {
	case "import":
		// Get import progress if available
		progress.CurrentPhase = "Importing Git repository"
	case "export":
		// Get export progress if available
		progress.CurrentPhase = "Exporting to Git format"
	case "migrate":
		// Get migration progress if available
		progress.CurrentPhase = "Migrating repositories"
	case "backup":
		// Get backup progress if available
		progress.CurrentPhase = "Creating backup"
	case "restore":
		// Get restore progress if available
		progress.CurrentPhase = "Restoring from backup"
	}

	c.JSON(http.StatusOK, progress)
}

// CancelJobHandler cancels a running job
func (s *Server) CancelJobHandler(c *gin.Context) {
	jobID := c.Param("job_id")

	job, exists := jobs[jobID]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	if job.Status == "running" {
		job.Cancel()
		job.Status = "cancelled"
	}

	c.JSON(http.StatusOK, gin.H{
		"job_id":  jobID,
		"status":  job.Status,
		"message": "Job cancellation requested",
	})
}

// ListJobsHandler lists all jobs
func (s *Server) ListJobsHandler(c *gin.Context) {
	var jobList []ProgressResponse

	for _, job := range jobs {
		progress := ProgressResponse{
			JobID:     job.ID,
			Status:    job.Status,
			StartedAt: job.StartedAt,
		}

		if job.Status == "completed" || job.Status == "failed" {
			now := time.Now()
			progress.CompletedAt = &now
		}

		if job.Error != nil {
			progress.Errors = []string{job.Error.Error()}
		}

		jobList = append(jobList, progress)
	}

	c.JSON(http.StatusOK, gin.H{
		"jobs":  jobList,
		"total": len(jobList),
	})
}

// ListBackupsHandler lists available backups
func (s *Server) ListBackupsHandler(c *gin.Context) {
	backupDir := c.Query("backup_dir")
	if backupDir == "" {
		backupDir = "/tmp/govc-backups" // Default backup directory
	}

	backups, err := importexport.ListBackups(backupDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list backups: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"backups":    backups,
		"total":      len(backups),
		"backup_dir": backupDir,
	})
}

// generateJobID generates a unique job ID
func generateJobID() string {
	return fmt.Sprintf("job_%d", time.Now().UnixNano())
}

// SetupImportExportRoutes sets up import/export API routes
// NOTE: This function needs to be updated to match the new routing pattern
// where routes are passed in as a parameter instead of using s.router
/*
func (s *Server) SetupImportExportRoutes() {
	// Import/Export routes
	v1 := s.router.Group("/api/v1")

	// Import
	v1.POST("/import/git", s.ImportGitHandler)

	// Export
	v1.POST("/export/git", s.ExportGitHandler)

	// Migration
	v1.POST("/migrate", s.MigrateHandler)

	// Backup/Restore
	v1.POST("/backup", s.BackupHandler)
	v1.POST("/restore", s.RestoreHandler)
	v1.GET("/backups", s.ListBackupsHandler)

	// Job management
	v1.GET("/jobs", s.ListJobsHandler)
	v1.GET("/jobs/:job_id/progress", s.ProgressHandler)
	v1.POST("/jobs/:job_id/cancel", s.CancelJobHandler)
}
*/
