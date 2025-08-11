package build

import (
	"context"
	"io"
	"time"
)

// BuildEngine manages the overall build process
type BuildEngine interface {
	// Build executes a build using the specified plugin
	Build(ctx context.Context, config BuildConfig) (*BuildResult, error)

	// ListPlugins returns available build plugins
	ListPlugins() []PluginInfo

	// GetPlugin retrieves a specific build plugin
	GetPlugin(name string) (BuildPlugin, error)

	// RegisterPlugin adds a new build plugin
	RegisterPlugin(plugin BuildPlugin) error

	// DetectAndBuild auto-detects project type and builds
	DetectAndBuild(ctx context.Context, vfs VirtualFileSystem, config BuildConfig) (*BuildResult, error)
}

// BuildPlugin defines the interface for language-specific builders
type BuildPlugin interface {
	// Name returns the plugin name (e.g., "go", "nodejs", "rust")
	Name() string

	// SupportedExtensions returns file extensions this plugin handles
	SupportedExtensions() []string

	// DetectProject analyzes files to determine if this plugin can build
	DetectProject(vfs VirtualFileSystem) (bool, *ProjectMetadata, error)

	// Build executes the build process
	Build(ctx context.Context, config BuildConfig, vfs VirtualFileSystem) (*BuildResult, error)

	// GetDependencies resolves project dependencies
	GetDependencies(config BuildConfig, vfs VirtualFileSystem) ([]Dependency, error)

	// ValidateBuild checks if build configuration is valid
	ValidateBuild(config BuildConfig, vfs VirtualFileSystem) error

	// SupportsMemoryBuild returns true if plugin can build entirely in memory
	SupportsMemoryBuild() bool
}

// VirtualFileSystem abstracts file operations for memory-only builds
type VirtualFileSystem interface {
	// Read file content from memory
	Read(path string) ([]byte, error)

	// Write file content to memory
	Write(path string, data []byte) error

	// Delete file from memory
	Delete(path string) error

	// List files matching pattern
	Glob(pattern string) ([]string, error)

	// List all files
	List() ([]string, error)

	// Check if file exists
	Exists(path string) bool

	// Create temporary directory in memory
	TempDir(prefix string) (string, error)

	// CreateBridge creates a temporary bridge to real filesystem
	// This is used when external tools require real files
	CreateBridge() (FilesystemBridge, error)
}

// FilesystemBridge provides temporary access to real filesystem
type FilesystemBridge interface {
	// TempPath returns the temporary directory path
	TempPath() string

	// Sync writes memory files to temporary directory
	SyncToTemp() error

	// SyncFromTemp reads files from temp back to memory
	SyncFromTemp() error

	// Exec executes command with access to temp directory
	Exec(cmd string, args []string, env map[string]string) (*ExecResult, error)

	// Cleanup removes temporary directory
	Cleanup() error
}

// BuildConfig specifies how to build a project
type BuildConfig struct {
	// Build target (e.g., "binary", "library", "wasm", "container")
	Target string `json:"target"`

	// Build mode (e.g., "debug", "release", "test", "bench")
	Mode string `json:"mode"`

	// Output path in virtual filesystem
	OutputPath string `json:"output_path"`

	// Environment variables
	Environment map[string]string `json:"environment"`

	// Build arguments
	Args []string `json:"args"`

	// Plugin-specific configuration
	PluginConfig map[string]interface{} `json:"plugin_config"`

	// Cache settings
	Cache *CacheConfig `json:"cache,omitempty"`

	// Parallel build settings
	Parallel bool `json:"parallel"`

	// Memory limit for build process (bytes)
	MemoryLimit int64 `json:"memory_limit,omitempty"`

	// Timeout for build process
	Timeout time.Duration `json:"timeout,omitempty"`
}

// CacheConfig defines caching behavior
type CacheConfig struct {
	// Enable caching
	Enabled bool `json:"enabled"`

	// Cache key prefix
	Prefix string `json:"prefix"`

	// Cache TTL
	TTL time.Duration `json:"ttl"`

	// Force rebuild (ignore cache)
	Force bool `json:"force"`
}

// BuildResult contains the output of a build
type BuildResult struct {
	// Success indicates if build completed successfully
	Success bool `json:"success"`

	// Build artifacts (executables, libraries, etc.)
	Artifacts []BuildArtifact `json:"artifacts"`

	// Build metadata
	Metadata *BuildMetadata `json:"metadata"`

	// Build logs and output
	Output *BuildOutput `json:"output"`

	// Error information if build failed
	Error error `json:"error,omitempty"`

	// Build duration
	Duration time.Duration `json:"duration"`

	// Memory usage statistics
	MemoryStats *MemoryStats `json:"memory_stats,omitempty"`

	// Cache hit information
	CacheHit bool `json:"cache_hit"`
}

// BuildArtifact represents a build output
type BuildArtifact struct {
	// Name of the artifact
	Name string `json:"name"`

	// Type of artifact ("binary", "library", "documentation", "wasm", etc.)
	Type string `json:"type"`

	// Path in virtual filesystem
	Path string `json:"path"`

	// Content of the artifact (for memory storage)
	Content []byte `json:"-"`

	// Size of the artifact
	Size int64 `json:"size"`

	// Hash of the artifact for verification
	Hash string `json:"hash"`

	// Artifact metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Executable flag
	Executable bool `json:"executable"`
}

// BuildMetadata contains build information
type BuildMetadata struct {
	// Builder plugin used
	Plugin string `json:"plugin"`

	// Project information
	Project *ProjectMetadata `json:"project"`

	// Build timestamp
	Timestamp time.Time `json:"timestamp"`

	// Build host information
	Host string `json:"host"`

	// Build configuration used
	Config BuildConfig `json:"config"`

	// Git commit hash (if applicable)
	CommitHash string `json:"commit_hash,omitempty"`

	// Build number/ID
	BuildID string `json:"build_id"`

	// Dependencies used
	Dependencies []Dependency `json:"dependencies,omitempty"`
}

// ProjectMetadata describes the project being built
type ProjectMetadata struct {
	// Project name
	Name string `json:"name"`

	// Programming language
	Language string `json:"language"`

	// Project version
	Version string `json:"version"`

	// Main entry point
	MainFile string `json:"main_file,omitempty"`

	// Project type (application, library, etc.)
	Type string `json:"type"`

	// Build system detected (make, npm, cargo, etc.)
	BuildSystem string `json:"build_system,omitempty"`

	// Project dependencies count
	DependencyCount int `json:"dependency_count"`
}

// BuildOutput contains build process output
type BuildOutput struct {
	// Standard output
	Stdout string `json:"stdout"`

	// Standard error
	Stderr string `json:"stderr"`

	// Build warnings
	Warnings []string `json:"warnings,omitempty"`

	// Build errors
	Errors []string `json:"errors,omitempty"`

	// Build info messages
	Info []string `json:"info,omitempty"`
}

// ExecResult contains command execution results
type ExecResult struct {
	// Command output
	Output string `json:"output"`

	// Exit code
	ExitCode int `json:"exit_code"`

	// Execution error
	Error error `json:"error,omitempty"`

	// Execution duration
	Duration time.Duration `json:"duration"`
}

// Dependency represents a project dependency
type Dependency struct {
	// Dependency name
	Name string `json:"name"`

	// Version specification
	Version string `json:"version"`

	// Dependency type (runtime, dev, build, etc.)
	Type string `json:"type"`

	// Source (npm, maven, cargo, etc.)
	Source string `json:"source"`

	// Resolved version
	ResolvedVersion string `json:"resolved_version,omitempty"`

	// Dependency hash/checksum
	Hash string `json:"hash,omitempty"`
}

// MemoryStats tracks memory usage during build
type MemoryStats struct {
	// Peak memory usage
	PeakUsage int64 `json:"peak_usage"`

	// Average memory usage
	AverageUsage int64 `json:"average_usage"`

	// Final memory usage
	FinalUsage int64 `json:"final_usage"`

	// Number of allocations
	Allocations int64 `json:"allocations"`

	// Number of frees
	Frees int64 `json:"frees"`
}

// PluginInfo describes an available build plugin
type PluginInfo struct {
	// Plugin name
	Name string `json:"name"`

	// Plugin version
	Version string `json:"version"`

	// Plugin description
	Description string `json:"description"`

	// Supported languages
	Languages []string `json:"languages"`

	// Supported file extensions
	Extensions []string `json:"extensions"`

	// Plugin capabilities
	Capabilities []string `json:"capabilities"`

	// Memory-only build support
	SupportsMemoryBuild bool `json:"supports_memory_build"`
}

// BuildEvent represents a build-related event
type BuildEvent struct {
	// Event type
	Type string `json:"type"`

	// Event timestamp
	Timestamp time.Time `json:"timestamp"`

	// Build ID
	BuildID string `json:"build_id"`

	// Plugin name
	Plugin string `json:"plugin"`

	// Build configuration
	Config BuildConfig `json:"config"`

	// Build result (for completion events)
	Result *BuildResult `json:"result,omitempty"`

	// Error (for failure events)
	Error error `json:"error,omitempty"`

	// Additional event data
	Data map[string]interface{} `json:"data,omitempty"`
}

// Build event types
const (
	EventBuildStarted   = "build.started"
	EventBuildCompleted = "build.completed"
	EventBuildFailed    = "build.failed"
	EventBuildCached    = "build.cached"
	EventBuildCancelled = "build.cancelled"
	EventPluginLoaded   = "build.plugin.loaded"
	EventArtifactCreated = "build.artifact.created"
)

// BuildManager coordinates builds with repository
type BuildManager interface {
	// Build executes a build for a repository
	Build(ctx context.Context, repoID string, config BuildConfig) (*BuildResult, error)

	// AutoBuild detects and builds project
	AutoBuild(ctx context.Context, repoID string) (*BuildResult, error)

	// BuildInReality builds in a parallel reality
	BuildInReality(ctx context.Context, repoID string, reality string, config BuildConfig) (*BuildResult, error)

	// GetBuildStatus returns current build status
	GetBuildStatus(buildID string) (*BuildStatus, error)

	// ListBuilds returns build history
	ListBuilds(repoID string, limit int) ([]*BuildResult, error)

	// Subscribe to build events
	Subscribe(handler func(BuildEvent)) func()
}

// BuildStatus represents current build status
type BuildStatus struct {
	// Build ID
	BuildID string `json:"build_id"`

	// Current state (pending, running, completed, failed)
	State string `json:"state"`

	// Progress percentage (0-100)
	Progress int `json:"progress"`

	// Current step
	CurrentStep string `json:"current_step"`

	// Start time
	StartTime time.Time `json:"start_time"`

	// Estimated completion time
	EstimatedCompletion *time.Time `json:"estimated_completion,omitempty"`

	// Build logs (streaming)
	Logs io.ReadCloser `json:"-"`
}