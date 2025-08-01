package builder

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/caiatech/govc"
	"github.com/caiatech/govc/container"
)

// Builder handles container image building operations
type Builder struct {
	repo     *govc.Repository
	executor Executor
	cache    BuildCache
}

// Executor defines the interface for container build execution
type Executor interface {
	Build(ctx context.Context, options BuildOptions) (*BuildResult, error)
	GetProgress(buildID string) <-chan BuildProgress
}

// BuildCache provides caching for build layers
type BuildCache interface {
	Get(key string) ([]byte, bool)
	Set(key string, data []byte) error
	Has(key string) bool
}

// BuildOptions contains options for building a container
type BuildOptions struct {
	Context      io.Reader
	Govcfile     string            // Govcfile content (replaces Dockerfile)
	Dockerfile   string            // Deprecated: use Govcfile
	Tags         []string
	BuildArgs    map[string]string
	Labels       map[string]string
	Target       string
	Platform     []string
	NoCache      bool
	Pull         bool
	ForceRemove  bool
	RemoveIntermediateContainers bool
}

// BuildResult contains the result of a build operation
type BuildResult struct {
	ImageID      string
	Digest       string
	Size         int64
	Tags         []string
	BuildTime    time.Duration
	CacheHits    int
	CacheMisses  int
	LayerCount   int
	Metadata     map[string]string
}

// BuildProgress represents progress during a build
type BuildProgress struct {
	Step        int
	TotalSteps  int
	Current     string
	Status      string
	Progress    float64
	StartTime   time.Time
	ElapsedTime time.Duration
}

// NewBuilder creates a new container builder
func NewBuilder(repo *govc.Repository) *Builder {
	return &Builder{
		repo:     repo,
		executor: NewMemoryExecutor(), // Default to memory-first executor
		cache:    NewMemoryCache(),
	}
}

// BuildFromRepository builds a container from repository content
func (b *Builder) BuildFromRepository(ctx context.Context, request container.BuildRequest) (*container.Build, error) {
	// Determine which file to read
	containerfilePath := request.Govcfile
	if containerfilePath == "" {
		containerfilePath = request.Containerfile // Legacy support
	}
	
	// Read Govcfile/Containerfile from repository
	containerfileContent, err := b.repo.ReadFile(containerfilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Govcfile: %w", err)
	}

	// Create build context from repository
	buildContext, err := b.createBuildContext(request.Context)
	if err != nil {
		return nil, fmt.Errorf("failed to create build context: %w", err)
	}

	// Prepare build options
	options := BuildOptions{
		Context:    buildContext,
		Govcfile:   string(containerfileContent),
		Tags:       request.Tags,
		BuildArgs:  request.Args,
		Labels:     request.Labels,
		Target:     request.Target,
		Platform:   request.Platform,
		NoCache:    request.NoCache,
	}

	// Execute build
	startTime := time.Now()
	result, err := b.executor.Build(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("build failed: %w", err)
	}

	// Create build record
	build := &container.Build{
		ID:           request.RepositoryID + "-" + fmt.Sprintf("%d", time.Now().Unix()),
		RepositoryID: request.RepositoryID,
		Request:      request,
		Status:       container.BuildStatusCompleted,
		StartTime:    startTime,
		EndTime:      &[]time.Time{time.Now()}[0],
		ImageID:      result.ImageID,
		Digest:       result.Digest,
		Metadata: map[string]string{
			"size":        fmt.Sprintf("%d", result.Size),
			"layers":      fmt.Sprintf("%d", result.LayerCount),
			"cache_hits":  fmt.Sprintf("%d", result.CacheHits),
			"build_time":  result.BuildTime.String(),
		},
	}

	return build, nil
}

// createBuildContext creates a build context from repository files
func (b *Builder) createBuildContext(contextPath string) (io.Reader, error) {
	// TODO: Implement actual build context creation
	// For now, return a simple reader
	return strings.NewReader("build context"), nil
}

// GetProgress returns a channel for build progress updates
func (b *Builder) GetProgress(buildID string) <-chan BuildProgress {
	return b.executor.GetProgress(buildID)
}

// SetExecutor sets a custom build executor
func (b *Builder) SetExecutor(executor Executor) {
	b.executor = executor
}

// SetCache sets a custom build cache
func (b *Builder) SetCache(cache BuildCache) {
	b.cache = cache
}