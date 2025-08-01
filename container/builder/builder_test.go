package builder

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/caiatech/govc"
	"github.com/caiatech/govc/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockExecutor is a mock implementation of the Executor interface
type MockExecutor struct {
	mock.Mock
}

func (m *MockExecutor) Build(ctx context.Context, options BuildOptions) (*BuildResult, error) {
	args := m.Called(ctx, options)
	
	// Handle function return type for dynamic results
	if fn, ok := args.Get(0).(func(context.Context, BuildOptions) *BuildResult); ok {
		return fn(ctx, options), args.Error(1)
	}
	
	// Handle direct BuildResult return
	if result := args.Get(0); result != nil {
		return result.(*BuildResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockExecutor) GetProgress(buildID string) <-chan BuildProgress {
	args := m.Called(buildID)
	return args.Get(0).(<-chan BuildProgress)
}

// MockCache is a mock implementation of the BuildCache interface
type MockCache struct {
	mock.Mock
	data map[string][]byte
}

func NewMockCache() *MockCache {
	return &MockCache{
		data: make(map[string][]byte),
	}
}

func (m *MockCache) Get(key string) ([]byte, bool) {
	args := m.Called(key)
	if data, exists := m.data[key]; exists {
		return data, true
	}
	return nil, args.Bool(1)
}

func (m *MockCache) Set(key string, data []byte) error {
	args := m.Called(key, data)
	m.data[key] = data
	return args.Error(0)
}

func (m *MockCache) Has(key string) bool {
	args := m.Called(key)
	_, exists := m.data[key]
	return exists || args.Bool(0)
}


func TestNewBuilder(t *testing.T) {
	repo := govc.New()
	builder := NewBuilder(repo)
	
	assert.NotNil(t, builder)
	assert.NotNil(t, builder.repo)
	assert.NotNil(t, builder.executor)
	assert.NotNil(t, builder.cache)
}

func TestBuildFromRepository(t *testing.T) {
	repo := govc.New()
	mockExecutor := &MockExecutor{}
	mockCache := NewMockCache()
	
	builder := NewBuilder(repo)
	builder.SetExecutor(mockExecutor)
	builder.SetCache(mockCache)
	
	// Set up test data
	govcfile := `BASE alpine:latest
RUN apk add --no-cache curl
COPY app /app
CMD ["/app"]`
	
	repo.WriteFile("Govcfile", []byte(govcfile))
	repo.Add("Govcfile")
	repo.SetConfig("user.name", "Test")
	repo.SetConfig("user.email", "test@example.com")
	repo.Commit("Add Govcfile")
	
	expectedResult := &BuildResult{
		ImageID:    "sha256:abc123",
		Digest:     "sha256:def456",
		Size:       1024 * 1024,
		Tags:       []string{"test:latest"},
		BuildTime:  5 * time.Second,
		CacheHits:  3,
		LayerCount: 4,
	}
	
	mockExecutor.On("Build", mock.Anything, mock.Anything).Return(expectedResult, nil)
	
	request := container.BuildRequest{
		RepositoryID: "test-repo",
		Govcfile:     "Govcfile",
		Context:      ".",
		Tags:         []string{"test:latest"},
		Args:         map[string]string{"VERSION": "1.0"},
		Labels:       map[string]string{"maintainer": "test@example.com"},
	}
	
	ctx := context.Background()
	build, err := builder.BuildFromRepository(ctx, request)
	
	require.NoError(t, err)
	assert.NotNil(t, build)
	assert.Equal(t, container.BuildStatusCompleted, build.Status)
	assert.Equal(t, expectedResult.ImageID, build.ImageID)
	assert.Equal(t, expectedResult.Digest, build.Digest)
	assert.Contains(t, build.Metadata["size"], "1048576")
	
	mockExecutor.AssertExpectations(t)
}

func TestBuildFromRepositoryErrors(t *testing.T) {
	repo := govc.New()
	mockExecutor := &MockExecutor{}
	
	builder := NewBuilder(repo)
	builder.SetExecutor(mockExecutor)
	
	t.Run("Govcfile Not Found", func(t *testing.T) {
		
		request := container.BuildRequest{
			RepositoryID: "test-repo",
			Govcfile:     "missing.govcfile",
		}
		
		ctx := context.Background()
		_, err := builder.BuildFromRepository(ctx, request)
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read Govcfile")
	})
	
	t.Run("Build Execution Error", func(t *testing.T) {
		repo.WriteFile("Govcfile", []byte("BASE alpine"))
		repo.Add("Govcfile")
		repo.Commit("Add Govcfile")
		mockExecutor.On("Build", mock.Anything, mock.Anything).Return(nil, assert.AnError)
		
		request := container.BuildRequest{
			RepositoryID: "test-repo",
			Govcfile:     "Govcfile",
		}
		
		ctx := context.Background()
		_, err := builder.BuildFromRepository(ctx, request)
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "build failed")
	})
}

func TestCreateBuildContext(t *testing.T) {
	repo := govc.New()
	builder := NewBuilder(repo)
	
	// Test creating build context
	ctx, err := builder.createBuildContext(".")
	require.NoError(t, err)
	assert.NotNil(t, ctx)
	
	// Read from context
	data, err := io.ReadAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, "build context", string(data))
}

func TestGetProgress(t *testing.T) {
	repo := govc.New()
	mockExecutor := &MockExecutor{}
	
	builder := NewBuilder(repo)
	builder.SetExecutor(mockExecutor)
	
	// Create progress channel
	progressChan := make(chan BuildProgress, 10)
	mockExecutor.On("GetProgress", "build-123").Return((<-chan BuildProgress)(progressChan))
	
	// Send progress updates
	go func() {
		progressChan <- BuildProgress{
			Step:       1,
			TotalSteps: 3,
			Current:    "BASE alpine",
			Status:     "pulling",
			Progress:   33.3,
		}
		progressChan <- BuildProgress{
			Step:       2,
			TotalSteps: 3,
			Current:    "RUN apk add",
			Status:     "running",
			Progress:   66.6,
		}
		close(progressChan)
	}()
	
	// Get progress channel
	progress := builder.GetProgress("build-123")
	
	// Collect progress updates
	var updates []BuildProgress
	for update := range progress {
		updates = append(updates, update)
	}
	
	assert.Len(t, updates, 2)
	assert.Equal(t, "pulling", updates[0].Status)
	assert.Equal(t, "running", updates[1].Status)
}

func TestSetExecutor(t *testing.T) {
	repo := govc.New()
	builder := NewBuilder(repo)
	
	// Create custom executor
	customExecutor := &MockExecutor{}
	builder.SetExecutor(customExecutor)
	
	// Verify executor was set
	assert.Equal(t, customExecutor, builder.executor)
}

func TestSetCache(t *testing.T) {
	repo := govc.New()
	builder := NewBuilder(repo)
	
	// Create custom cache
	customCache := NewMockCache()
	builder.SetCache(customCache)
	
	// Verify cache was set
	assert.Equal(t, customCache, builder.cache)
}

func TestBuildOptions(t *testing.T) {
	t.Run("Full Options", func(t *testing.T) {
		options := BuildOptions{
			Context:                      strings.NewReader("context"),
			Govcfile:                     "BASE alpine",
			Tags:                         []string{"app:latest", "app:v1.0"},
			BuildArgs:                    map[string]string{"VERSION": "1.0"},
			Labels:                       map[string]string{"maintainer": "test"},
			Target:                       "production",
			Platform:                     []string{"linux/amd64"},
			NoCache:                      true,
			Pull:                         true,
			ForceRemove:                  true,
			RemoveIntermediateContainers: true,
		}
		
		assert.NotNil(t, options.Context)
		assert.Len(t, options.Tags, 2)
		assert.Equal(t, "1.0", options.BuildArgs["VERSION"])
		assert.True(t, options.NoCache)
	})
	
	t.Run("Minimal Options", func(t *testing.T) {
		options := BuildOptions{
			Govcfile:   "BASE scratch",
			Tags:       []string{"minimal:latest"},
		}
		
		assert.Equal(t, "BASE scratch", options.Govcfile)
		assert.Len(t, options.Tags, 1)
		assert.False(t, options.NoCache)
		assert.Nil(t, options.BuildArgs)
	})
}

func TestBuildResult(t *testing.T) {
	result := BuildResult{
		ImageID:     "sha256:abc123def456",
		Digest:      "sha256:fedcba654321",
		Size:        1024 * 1024 * 50, // 50MB
		Tags:        []string{"app:latest", "app:v1.0", "app:stable"},
		BuildTime:   30 * time.Second,
		CacheHits:   10,
		CacheMisses: 2,
		LayerCount:  5,
		Metadata: map[string]string{
			"builder":  "govc",
			"platform": "linux/amd64",
		},
	}
	
	assert.NotEmpty(t, result.ImageID)
	assert.NotEmpty(t, result.Digest)
	assert.Equal(t, int64(52428800), result.Size)
	assert.Len(t, result.Tags, 3)
	assert.Equal(t, 30*time.Second, result.BuildTime)
	assert.Equal(t, 10, result.CacheHits)
	assert.Equal(t, 2, result.CacheMisses)
	assert.Equal(t, 5, result.LayerCount)
	assert.Equal(t, "govc", result.Metadata["builder"])
}

func TestBuildProgress(t *testing.T) {
	startTime := time.Now()
	
	progress := BuildProgress{
		Step:        3,
		TotalSteps:  10,
		Current:     "RUN npm install",
		Status:      "running",
		Progress:    30.0,
		StartTime:   startTime,
		ElapsedTime: 5 * time.Second,
	}
	
	assert.Equal(t, 3, progress.Step)
	assert.Equal(t, 10, progress.TotalSteps)
	assert.Equal(t, "RUN npm install", progress.Current)
	assert.Equal(t, "running", progress.Status)
	assert.Equal(t, 30.0, progress.Progress)
	assert.Equal(t, 5*time.Second, progress.ElapsedTime)
}

func TestConcurrentBuilds(t *testing.T) {
	repo := govc.New()
	mockExecutor := &MockExecutor{}
	
	builder := NewBuilder(repo)
	builder.SetExecutor(mockExecutor)
	
	// Set up repository data
	govcfile := "BASE alpine"
	repo.WriteFile("Govcfile", []byte(govcfile))
	repo.Add("Govcfile")
	repo.Commit("Add Govcfile")
	
	// Mock executor to simulate concurrent builds
	mockExecutor.On("Build", mock.Anything, mock.Anything).Return(func(ctx context.Context, options BuildOptions) *BuildResult {
		// Simulate build time
		time.Sleep(100 * time.Millisecond)
		
		// Generate unique image ID based on tags
		imageID := "sha256:default"
		if len(options.Tags) > 0 {
			imageID = fmt.Sprintf("sha256:%s", strings.Replace(options.Tags[0], ":", "-", -1))
		}
		
		return &BuildResult{
			ImageID: imageID,
			Digest:  fmt.Sprintf("digest-%s", imageID),
			Size:    1024,
			Tags:    options.Tags,
		}
	}, nil)
	
	// Start multiple builds concurrently
	buildCount := 5
	results := make(chan *container.Build, buildCount)
	errors := make(chan error, buildCount)
	
	for i := 0; i < buildCount; i++ {
		go func(index int) {
			request := container.BuildRequest{
				RepositoryID: "test-repo",
				Govcfile:     "Govcfile",
				Tags:         []string{fmt.Sprintf("test:v%d", index)},
			}
			
			ctx := context.Background()
			build, err := builder.BuildFromRepository(ctx, request)
			if err != nil {
				errors <- err
			} else {
				results <- build
			}
		}(i)
	}
	
	// Collect results
	var builds []*container.Build
	for i := 0; i < buildCount; i++ {
		select {
		case build := <-results:
			builds = append(builds, build)
		case err := <-errors:
			t.Errorf("Build failed: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for builds")
		}
	}
	
	assert.Len(t, builds, buildCount)
	
	// Verify each build has unique image ID
	imageIDs := make(map[string]bool)
	for _, build := range builds {
		assert.NotContains(t, imageIDs, build.ImageID)
		imageIDs[build.ImageID] = true
	}
}