package builder

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// MemoryExecutor implements a memory-first container build executor
type MemoryExecutor struct {
	layers    map[string]*Layer
	images    map[string]*Image
	progress  map[string]chan BuildProgress
	mu        sync.RWMutex
}

// Layer represents a container image layer
type Layer struct {
	ID       string
	Parent   string
	Size     int64
	Created  time.Time
	Data     []byte
	Metadata map[string]string
}

// Image represents a built container image
type Image struct {
	ID         string
	Digest     string
	Layers     []string
	Config     ImageConfig
	Manifest   ImageManifest
	Created    time.Time
	Size       int64
	Tags       []string
}

// ImageConfig contains runtime configuration for an image
type ImageConfig struct {
	Env        []string
	Cmd        []string
	Entrypoint []string
	WorkingDir string
	User       string
	Volumes    map[string]struct{}
	ExposedPorts map[string]struct{}
	Labels     map[string]string
}

// ImageManifest represents an OCI image manifest
type ImageManifest struct {
	SchemaVersion int
	MediaType     string
	Config        ManifestConfig
	Layers        []ManifestLayer
}

// ManifestConfig represents config in a manifest
type ManifestConfig struct {
	MediaType string
	Size      int64
	Digest    string
}

// ManifestLayer represents a layer in a manifest
type ManifestLayer struct {
	MediaType string
	Size      int64
	Digest    string
}

// NewMemoryExecutor creates a new memory-based build executor
func NewMemoryExecutor() *MemoryExecutor {
	return &MemoryExecutor{
		layers:   make(map[string]*Layer),
		images:   make(map[string]*Image),
		progress: make(map[string]chan BuildProgress),
	}
}

// Build executes a container build in memory
func (e *MemoryExecutor) Build(ctx context.Context, options BuildOptions) (*BuildResult, error) {
	buildID := generateBuildID()
	progressChan := make(chan BuildProgress, 100)
	
	e.mu.Lock()
	e.progress[buildID] = progressChan
	e.mu.Unlock()
	
	defer func() {
		e.mu.Lock()
		delete(e.progress, buildID)
		e.mu.Unlock()
		close(progressChan)
	}()

	// Parse Govcfile (or legacy Dockerfile)
	govcfileContent := options.Govcfile
	if govcfileContent == "" {
		govcfileContent = options.Dockerfile // Legacy support
	}
	
	instructions, err := parseGovcfile(govcfileContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Govcfile: %w", err)
	}

	// Initialize build state
	state := &buildState{
		baseImage:  "scratch",
		env:        make(map[string]string),
		workingDir: "/",
		layers:     make([]string, 0),
		config:     ImageConfig{Labels: options.Labels},
		startTime:  time.Now(),
		ctx:        ctx,
	}

	// Execute each instruction
	totalSteps := len(instructions)
	for i, instruction := range instructions {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Send progress update
		progressChan <- BuildProgress{
			Step:        i + 1,
			TotalSteps:  totalSteps,
			Current:     instruction.String(),
			Status:      "executing",
			Progress:    float64(i) / float64(totalSteps) * 100,
			StartTime:   state.startTime,
			ElapsedTime: time.Since(state.startTime),
		}

		// Execute instruction
		if err := e.executeInstruction(state, instruction, options); err != nil {
			return nil, fmt.Errorf("failed at step %d: %w", i+1, err)
		}
	}

	// Create final image
	image, err := e.createImage(state, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create image: %w", err)
	}

	// Store image
	e.mu.Lock()
	e.images[image.ID] = image
	for _, tag := range options.Tags {
		e.images[tag] = image
	}
	e.mu.Unlock()

	return &BuildResult{
		ImageID:     image.ID,
		Digest:      image.Digest,
		Size:        image.Size,
		Tags:        options.Tags,
		BuildTime:   time.Since(state.startTime),
		LayerCount:  len(image.Layers),
		CacheHits:   state.cacheHits,
		CacheMisses: state.cacheMisses,
	}, nil
}

// executeInstruction executes a single Dockerfile instruction
func (e *MemoryExecutor) executeInstruction(state *buildState, inst instruction, options BuildOptions) error {
	switch inst.cmd {
	case "BASE", "FROM": // Support both BASE (Govcfile) and FROM (legacy)
		return e.handleFrom(state, inst)
	case "RUN":
		return e.handleRun(state, inst)
	case "COPY", "ADD":
		return e.handleCopy(state, inst)
	case "ENV":
		return e.handleEnv(state, inst)
	case "WORKDIR":
		return e.handleWorkdir(state, inst)
	case "CMD":
		return e.handleCmd(state, inst)
	case "ENTRYPOINT":
		return e.handleEntrypoint(state, inst)
	case "EXPOSE":
		return e.handleExpose(state, inst)
	case "VOLUME":
		return e.handleVolume(state, inst)
	case "USER":
		return e.handleUser(state, inst)
	case "LABEL":
		return e.handleLabel(state, inst)
	}
	return nil
}

// handleFrom processes BASE/FROM instruction
func (e *MemoryExecutor) handleFrom(state *buildState, inst instruction) error {
	state.baseImage = inst.args[0]
	// In a real implementation, we would pull the base image
	// For now, we'll create a synthetic base layer
	baseLayer := &Layer{
		ID:      generateLayerID(),
		Size:    1024 * 1024, // 1MB synthetic size
		Created: time.Now(),
		Data:    []byte("base image content"),
	}
	
	e.mu.Lock()
	e.layers[baseLayer.ID] = baseLayer
	e.mu.Unlock()
	
	state.layers = append(state.layers, baseLayer.ID)
	return nil
}

// handleRun processes RUN instruction
func (e *MemoryExecutor) handleRun(state *buildState, inst instruction) error {
	// Simulate command execution
	command := strings.Join(inst.args, " ")
	
	// Simulate sleep commands for testing with context awareness
	if len(inst.args) > 0 && inst.args[0] == "sleep" {
		if len(inst.args) > 1 {
			// Parse sleep duration
			if duration, err := time.ParseDuration(inst.args[1] + "s"); err == nil {
				// Sleep with context cancellation support
				select {
				case <-time.After(duration):
					// Sleep completed
				case <-state.ctx.Done():
					return state.ctx.Err()
				}
			}
		}
	}
	
	// Create a new layer for this RUN command
	layer := &Layer{
		ID:      generateLayerID(),
		Parent:  state.currentLayer(),
		Size:    int64(len(command) * 100), // Synthetic size based on command
		Created: time.Now(),
		Data:    []byte(fmt.Sprintf("RUN %s", command)),
		Metadata: map[string]string{
			"command": command,
		},
	}
	
	e.mu.Lock()
	e.layers[layer.ID] = layer
	e.mu.Unlock()
	
	state.layers = append(state.layers, layer.ID)
	return nil
}

// handleCopy processes COPY/ADD instruction
func (e *MemoryExecutor) handleCopy(state *buildState, inst instruction) error {
	if len(inst.args) < 2 {
		return fmt.Errorf("COPY requires at least 2 arguments")
	}
	
	src := inst.args[0]
	dst := inst.args[1]
	
	// Create a layer for the copied content
	layer := &Layer{
		ID:      generateLayerID(),
		Parent:  state.currentLayer(),
		Size:    1024, // Synthetic size
		Created: time.Now(),
		Data:    []byte(fmt.Sprintf("COPY %s %s", src, dst)),
		Metadata: map[string]string{
			"source":      src,
			"destination": dst,
		},
	}
	
	e.mu.Lock()
	e.layers[layer.ID] = layer
	e.mu.Unlock()
	
	state.layers = append(state.layers, layer.ID)
	return nil
}

// Other handlers...
func (e *MemoryExecutor) handleEnv(state *buildState, inst instruction) error {
	if len(inst.args) >= 2 {
		state.env[inst.args[0]] = inst.args[1]
		state.config.Env = append(state.config.Env, fmt.Sprintf("%s=%s", inst.args[0], inst.args[1]))
	}
	return nil
}

func (e *MemoryExecutor) handleWorkdir(state *buildState, inst instruction) error {
	if len(inst.args) > 0 {
		state.workingDir = inst.args[0]
		state.config.WorkingDir = inst.args[0]
	}
	return nil
}

func (e *MemoryExecutor) handleCmd(state *buildState, inst instruction) error {
	state.config.Cmd = inst.args
	return nil
}

func (e *MemoryExecutor) handleEntrypoint(state *buildState, inst instruction) error {
	state.config.Entrypoint = inst.args
	return nil
}

func (e *MemoryExecutor) handleExpose(state *buildState, inst instruction) error {
	if state.config.ExposedPorts == nil {
		state.config.ExposedPorts = make(map[string]struct{})
	}
	for _, port := range inst.args {
		state.config.ExposedPorts[port] = struct{}{}
	}
	return nil
}

func (e *MemoryExecutor) handleVolume(state *buildState, inst instruction) error {
	if state.config.Volumes == nil {
		state.config.Volumes = make(map[string]struct{})
	}
	for _, vol := range inst.args {
		state.config.Volumes[vol] = struct{}{}
	}
	return nil
}

func (e *MemoryExecutor) handleUser(state *buildState, inst instruction) error {
	if len(inst.args) > 0 {
		state.config.User = inst.args[0]
	}
	return nil
}

func (e *MemoryExecutor) handleLabel(state *buildState, inst instruction) error {
	if state.config.Labels == nil {
		state.config.Labels = make(map[string]string)
	}
	if len(inst.args) >= 2 {
		state.config.Labels[inst.args[0]] = inst.args[1]
	}
	return nil
}

// createImage creates the final image from build state
func (e *MemoryExecutor) createImage(state *buildState, options BuildOptions) (*Image, error) {
	// Calculate total size
	var totalSize int64
	e.mu.RLock()
	for _, layerID := range state.layers {
		if layer, exists := e.layers[layerID]; exists {
			totalSize += layer.Size
		}
	}
	e.mu.RUnlock()

	// Generate image ID and digest
	imageID := generateImageID()
	digest := generateDigest(imageID)

	image := &Image{
		ID:      imageID,
		Digest:  digest,
		Layers:  state.layers,
		Config:  state.config,
		Created: time.Now(),
		Size:    totalSize,
		Tags:    options.Tags,
		Manifest: ImageManifest{
			SchemaVersion: 2,
			MediaType:     "application/vnd.docker.distribution.manifest.v2+json",
			Config: ManifestConfig{
				MediaType: "application/vnd.docker.container.image.v1+json",
				Size:      256, // Synthetic config size
				Digest:    digest,
			},
		},
	}

	// Add layers to manifest
	e.mu.RLock()
	for _, layerID := range state.layers {
		if layer, exists := e.layers[layerID]; exists {
			image.Manifest.Layers = append(image.Manifest.Layers, ManifestLayer{
				MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
				Size:      layer.Size,
				Digest:    "sha256:" + layerID,
			})
		}
	}
	e.mu.RUnlock()

	return image, nil
}

// GetProgress returns the progress channel for a build
func (e *MemoryExecutor) GetProgress(buildID string) <-chan BuildProgress {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	if ch, exists := e.progress[buildID]; exists {
		return ch
	}
	
	// Return closed channel if build doesn't exist
	ch := make(chan BuildProgress)
	close(ch)
	return ch
}

// Helper types and functions

type buildState struct {
	baseImage    string
	env          map[string]string
	workingDir   string
	layers       []string
	config       ImageConfig
	startTime    time.Time
	cacheHits    int
	cacheMisses  int
	ctx          context.Context
}

func (s *buildState) currentLayer() string {
	if len(s.layers) == 0 {
		return ""
	}
	return s.layers[len(s.layers)-1]
}

type instruction struct {
	cmd  string
	args []string
}

func (i instruction) String() string {
	return fmt.Sprintf("%s %s", i.cmd, strings.Join(i.args, " "))
}

func parseGovcfile(content string) ([]instruction, error) {
	var instructions []instruction
	scanner := bufio.NewScanner(strings.NewReader(content))
	
	// Check if this looks like a Govcfile or legacy Dockerfile
	isGovcfile := strings.Contains(content, "BASE ") || strings.Contains(content, "@version=")
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Skip metadata lines in Govcfile (lines starting with @)
		if isGovcfile && strings.HasPrefix(line, "@") {
			continue
		}
		
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		
		cmd := strings.ToUpper(parts[0])
		
		// Convert legacy Dockerfile commands to Govcfile commands
		if !isGovcfile && cmd == "FROM" {
			cmd = "BASE"
		}
		
		inst := instruction{
			cmd:  cmd,
			args: parts[1:],
		}
		instructions = append(instructions, inst)
	}
	
	return instructions, scanner.Err()
}

// Legacy support - keep for backward compatibility
func parseDockerfile(content string) ([]instruction, error) {
	return parseGovcfile(content)
}

func generateBuildID() string {
	return fmt.Sprintf("build-%d", time.Now().UnixNano())
}

func generateLayerID() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("layer-%d", time.Now().UnixNano()))))[:12]
}

func generateImageID() string {
	// Use more entropy to avoid collisions
	data := fmt.Sprintf("image-%d-%d", time.Now().UnixNano(), rand.Int63())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("sha256:%x", hash)[:64] // Use more characters for uniqueness
}

func generateDigest(data string) string {
	hash := sha256.Sum256([]byte(data))
	return "sha256:" + hex.EncodeToString(hash[:])
}