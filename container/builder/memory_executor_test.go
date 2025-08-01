package builder

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMemoryExecutor(t *testing.T) {
	executor := NewMemoryExecutor()
	
	assert.NotNil(t, executor)
	assert.NotNil(t, executor.layers)
	assert.NotNil(t, executor.images)
	assert.NotNil(t, executor.progress)
}

func TestBuildSimpleDockerfile(t *testing.T) {
	executor := NewMemoryExecutor()
	
	govcfile := `BASE alpine:latest
RUN apk add --no-cache curl
COPY app /app
CMD ["/app"]`
	
	options := BuildOptions{
		Context:    strings.NewReader("context"),
		Govcfile:   govcfile,
		Tags:       []string{"test:latest"},
		Labels:     map[string]string{"version": "1.0"},
	}
	
	ctx := context.Background()
	result, err := executor.Build(ctx, options)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.ImageID)
	assert.NotEmpty(t, result.Digest)
	assert.Greater(t, result.Size, int64(0))
	assert.Equal(t, options.Tags, result.Tags)
	assert.Equal(t, 3, result.LayerCount) // BASE + RUN + COPY (CMD doesn't create a layer)
	assert.GreaterOrEqual(t, result.BuildTime, time.Duration(0))
}

func TestBuildWithProgress(t *testing.T) {
	executor := NewMemoryExecutor()
	
	govcfile := `BASE alpine:latest
RUN apk add --no-cache curl
RUN apk add --no-cache git`
	
	options := BuildOptions{
		Govcfile: govcfile,
		Tags:     []string{"test:latest"},
	}
	
	// Start build in goroutine
	ctx := context.Background()
	var result *BuildResult
	var buildErr error
	var wg sync.WaitGroup
	
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, buildErr = executor.Build(ctx, options)
	}()
	
	// Give build time to start and register progress channel
	time.Sleep(100 * time.Millisecond)
	
	// Get progress updates
	buildID := "build-" // Prefix used in generateBuildID
	var progressChan <-chan BuildProgress
	
	executor.mu.RLock()
	for id, ch := range executor.progress {
		if strings.HasPrefix(id, buildID) {
			progressChan = ch
			break
		}
	}
	executor.mu.RUnlock()
	
	if progressChan != nil {
		var updates []BuildProgress
		for progress := range progressChan {
			updates = append(updates, progress)
		}
		
		// Should have progress for each instruction
		assert.GreaterOrEqual(t, len(updates), 3) // FROM, RUN, RUN
		
		// Check progress values
		for i, update := range updates {
			assert.Equal(t, i+1, update.Step)
			assert.Equal(t, 3, update.TotalSteps)
			assert.Equal(t, "executing", update.Status)
			assert.NotEmpty(t, update.Current)
		}
	}
	
	wg.Wait()
	require.NoError(t, buildErr)
	assert.NotNil(t, result)
}

func TestParseGovcfile(t *testing.T) {
	tests := []struct {
		name        string
		dockerfile  string
		expected    int
		shouldError bool
	}{
		{
			name: "Simple Dockerfile",
			dockerfile: `FROM alpine
RUN echo hello
COPY app /app
CMD ["/app"]`,
			expected: 4,
		},
		{
			name: "With Comments",
			dockerfile: `# This is a comment
FROM alpine
# Another comment
RUN echo hello
COPY app /app`,
			expected: 3,
		},
		{
			name: "Empty Lines",
			dockerfile: `FROM alpine

RUN echo hello

COPY app /app`,
			expected: 3,
		},
		{
			name: "Multi-stage",
			dockerfile: `FROM golang:1.20 AS builder
WORKDIR /app
COPY . .
RUN go build -o app

FROM alpine
COPY --from=builder /app/app /app
CMD ["/app"]`,
			expected: 7,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instructions, err := parseGovcfile(tt.dockerfile)
			
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, instructions, tt.expected)
			}
		})
	}
}

func TestExecuteInstructions(t *testing.T) {
	executor := NewMemoryExecutor()
	state := &buildState{
		baseImage:  "scratch",
		env:        make(map[string]string),
		workingDir: "/",
		layers:     make([]string, 0),
		config:     ImageConfig{},
		startTime:  time.Now(),
	}
	
	tests := []struct {
		name string
		inst instruction
		test func()
	}{
		{
			name: "BASE",
			inst: instruction{cmd: "BASE", args: []string{"alpine:latest"}},
			test: func() {
				assert.Equal(t, "alpine:latest", state.baseImage)
				assert.Len(t, state.layers, 1)
			},
		},
		{
			name: "ENV",
			inst: instruction{cmd: "ENV", args: []string{"KEY", "value"}},
			test: func() {
				assert.Equal(t, "value", state.env["KEY"])
				assert.Contains(t, state.config.Env, "KEY=value")
			},
		},
		{
			name: "WORKDIR",
			inst: instruction{cmd: "WORKDIR", args: []string{"/app"}},
			test: func() {
				assert.Equal(t, "/app", state.workingDir)
				assert.Equal(t, "/app", state.config.WorkingDir)
			},
		},
		{
			name: "CMD",
			inst: instruction{cmd: "CMD", args: []string{"/bin/sh", "-c", "echo hello"}},
			test: func() {
				assert.Equal(t, []string{"/bin/sh", "-c", "echo hello"}, state.config.Cmd)
			},
		},
		{
			name: "EXPOSE",
			inst: instruction{cmd: "EXPOSE", args: []string{"8080", "9090"}},
			test: func() {
				assert.Contains(t, state.config.ExposedPorts, "8080")
				assert.Contains(t, state.config.ExposedPorts, "9090")
			},
		},
		{
			name: "VOLUME",
			inst: instruction{cmd: "VOLUME", args: []string{"/data", "/logs"}},
			test: func() {
				assert.Contains(t, state.config.Volumes, "/data")
				assert.Contains(t, state.config.Volumes, "/logs")
			},
		},
		{
			name: "USER",
			inst: instruction{cmd: "USER", args: []string{"nobody"}},
			test: func() {
				assert.Equal(t, "nobody", state.config.User)
			},
		},
		{
			name: "LABEL",
			inst: instruction{cmd: "LABEL", args: []string{"version", "1.0"}},
			test: func() {
				assert.Equal(t, "1.0", state.config.Labels["version"])
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state for some tests
			if tt.inst.cmd == "EXPOSE" || tt.inst.cmd == "VOLUME" || tt.inst.cmd == "LABEL" {
				state.config.ExposedPorts = make(map[string]struct{})
				state.config.Volumes = make(map[string]struct{})
				state.config.Labels = make(map[string]string)
			}
			
			err := executor.executeInstruction(state, tt.inst, BuildOptions{})
			require.NoError(t, err)
			tt.test()
		})
	}
}

func TestHandleRun(t *testing.T) {
	executor := NewMemoryExecutor()
	state := &buildState{
		layers: []string{"base-layer"},
	}
	
	inst := instruction{
		cmd:  "RUN",
		args: []string{"apk", "add", "--no-cache", "curl"},
	}
	
	err := executor.handleRun(state, inst)
	require.NoError(t, err)
	
	// Should create a new layer
	assert.Len(t, state.layers, 2)
	
	// Verify layer was stored
	layerID := state.layers[1]
	executor.mu.RLock()
	layer, exists := executor.layers[layerID]
	executor.mu.RUnlock()
	
	assert.True(t, exists)
	assert.Equal(t, "base-layer", layer.Parent)
	assert.Contains(t, string(layer.Data), "RUN apk add --no-cache curl")
}

func TestHandleCopy(t *testing.T) {
	executor := NewMemoryExecutor()
	state := &buildState{
		layers: []string{"base-layer"},
	}
	
	tests := []struct {
		name      string
		inst      instruction
		shouldErr bool
	}{
		{
			name: "Valid COPY",
			inst: instruction{
				cmd:  "COPY",
				args: []string{"app.go", "/app/app.go"},
			},
			shouldErr: false,
		},
		{
			name: "COPY with insufficient args",
			inst: instruction{
				cmd:  "COPY",
				args: []string{"app.go"},
			},
			shouldErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state
			state.layers = []string{"base-layer"}
			
			err := executor.handleCopy(state, tt.inst)
			
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, state.layers, 2)
				
				// Verify layer metadata
				layerID := state.layers[1]
				executor.mu.RLock()
				layer, exists := executor.layers[layerID]
				executor.mu.RUnlock()
				
				assert.True(t, exists)
				assert.Equal(t, tt.inst.args[0], layer.Metadata["source"])
				assert.Equal(t, tt.inst.args[1], layer.Metadata["destination"])
			}
		})
	}
}

func TestCreateImage(t *testing.T) {
	executor := NewMemoryExecutor()
	
	// Create test layers
	layer1 := &Layer{
		ID:   "layer1",
		Size: 1024,
	}
	layer2 := &Layer{
		ID:   "layer2",
		Size: 2048,
	}
	
	executor.mu.Lock()
	executor.layers["layer1"] = layer1
	executor.layers["layer2"] = layer2
	executor.mu.Unlock()
	
	state := &buildState{
		layers: []string{"layer1", "layer2"},
		config: ImageConfig{
			Cmd:        []string{"/app"},
			WorkingDir: "/app",
			Labels:     map[string]string{"version": "1.0"},
		},
	}
	
	options := BuildOptions{
		Tags: []string{"test:latest", "test:v1.0"},
	}
	
	image, err := executor.createImage(state, options)
	require.NoError(t, err)
	assert.NotNil(t, image)
	
	// Verify image properties
	assert.NotEmpty(t, image.ID)
	assert.NotEmpty(t, image.Digest)
	assert.Equal(t, int64(3072), image.Size) // 1024 + 2048
	assert.Equal(t, options.Tags, image.Tags)
	assert.Len(t, image.Layers, 2)
	assert.Equal(t, state.config, image.Config)
	
	// Verify manifest
	assert.Equal(t, 2, image.Manifest.SchemaVersion)
	assert.Len(t, image.Manifest.Layers, 2)
	assert.Equal(t, "sha256:layer1", image.Manifest.Layers[0].Digest)
	assert.Equal(t, "sha256:layer2", image.Manifest.Layers[1].Digest)
}

func TestGetProgressNonExistent(t *testing.T) {
	executor := NewMemoryExecutor()
	
	// Get progress for non-existent build
	progress := executor.GetProgress("non-existent")
	
	// Should return closed channel
	_, ok := <-progress
	assert.False(t, ok)
}

func TestCancelBuild(t *testing.T) {
	executor := NewMemoryExecutor()
	
	govcfile := `BASE alpine
RUN sleep 1
RUN echo done`
	
	options := BuildOptions{
		Govcfile: govcfile,
		Tags:     []string{"test:latest"},
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	// Start build
	done := make(chan error)
	go func() {
		_, err := executor.Build(ctx, options)
		done <- err
	}()
	
	// Cancel after short delay
	time.Sleep(100 * time.Millisecond)
	cancel()
	
	// Wait for build to finish
	err := <-done
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestConcurrentImageCreation(t *testing.T) {
	executor := NewMemoryExecutor()
	
	// Run multiple builds concurrently
	buildCount := 10
	var wg sync.WaitGroup
	results := make(chan *BuildResult, buildCount)
	
	for i := 0; i < buildCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			govcfile := fmt.Sprintf(`BASE alpine:%d
RUN echo "Build %d"`, index, index)
			
			options := BuildOptions{
				Govcfile: govcfile,
				Tags:     []string{fmt.Sprintf("test:v%d", index)},
			}
			
			ctx := context.Background()
			result, err := executor.Build(ctx, options)
			if err == nil {
				results <- result
			}
		}(i)
	}
	
	wg.Wait()
	close(results)
	
	// Verify all builds completed
	var images []*BuildResult
	for result := range results {
		images = append(images, result)
	}
	
	assert.Len(t, images, buildCount)
	
	// Verify each image has unique ID
	imageIDs := make(map[string]bool)
	for _, img := range images {
		assert.NotContains(t, imageIDs, img.ImageID)
		imageIDs[img.ImageID] = true
	}
}

func TestLayerManagement(t *testing.T) {
	executor := NewMemoryExecutor()
	
	// Create and store layers
	for i := 0; i < 5; i++ {
		layer := &Layer{
			ID:      fmt.Sprintf("layer%d", i),
			Size:    int64(1024 * (i + 1)),
			Created: time.Now(),
			Data:    []byte(fmt.Sprintf("Layer %d content", i)),
		}
		
		executor.mu.Lock()
		executor.layers[layer.ID] = layer
		executor.mu.Unlock()
	}
	
	// Verify layers are stored
	executor.mu.RLock()
	assert.Len(t, executor.layers, 5)
	executor.mu.RUnlock()
	
	// Verify layer properties
	executor.mu.RLock()
	layer0 := executor.layers["layer0"]
	executor.mu.RUnlock()
	
	assert.Equal(t, "layer0", layer0.ID)
	assert.Equal(t, int64(1024), layer0.Size)
	assert.Equal(t, "Layer 0 content", string(layer0.Data))
}