package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Caia-Tech/govc/pkg/build"
)

// JavaScriptPlugin implements building JavaScript/Node.js projects in memory
type JavaScriptPlugin struct {
	nodePath string
	npmPath  string
	yarnPath string
	bunPath  string
	denoPath string
}

// NewJavaScriptPlugin creates a new JavaScript build plugin
func NewJavaScriptPlugin() *JavaScriptPlugin {
	return &JavaScriptPlugin{
		nodePath: findExecutable("node"),
		npmPath:  findExecutable("npm"),
		yarnPath: findExecutable("yarn"),
		bunPath:  findExecutable("bun"),
		denoPath: findExecutable("deno"),
	}
}

// Name returns the plugin name
func (p *JavaScriptPlugin) Name() string {
	return "javascript"
}

// SupportedExtensions returns JavaScript file extensions
func (p *JavaScriptPlugin) SupportedExtensions() []string {
	return []string{
		".js", ".mjs", ".cjs", ".jsx",
		".ts", ".tsx", ".mts", ".cts",
		"package.json", "package-lock.json",
		"yarn.lock", "pnpm-lock.yaml",
		"tsconfig.json", "webpack.config.js",
		"vite.config.js", "rollup.config.js",
	}
}

// DetectProject checks if this is a JavaScript project
func (p *JavaScriptPlugin) DetectProject(vfs build.VirtualFileSystem) (bool, *build.ProjectMetadata, error) {
	// Check for package.json
	if vfs.Exists("package.json") {
		content, err := vfs.Read("package.json")
		if err != nil {
			return false, nil, err
		}

		var packageJSON PackageJSON
		if err := json.Unmarshal(content, &packageJSON); err != nil {
			return false, nil, err
		}

		metadata := &build.ProjectMetadata{
			Name:        packageJSON.Name,
			Language:    "javascript",
			Version:     packageJSON.Version,
			Type:        determineProjectType(packageJSON),
			BuildSystem: detectBuildSystem(vfs, packageJSON),
			MainFile:    packageJSON.Main,
		}

		// Count dependencies
		metadata.DependencyCount = len(packageJSON.Dependencies) + len(packageJSON.DevDependencies)

		// Detect TypeScript
		if vfs.Exists("tsconfig.json") {
			metadata.Language = "typescript"
		}

		return true, metadata, nil
	}

	// Check for Deno project (deno.json or import_map.json)
	if vfs.Exists("deno.json") || vfs.Exists("deno.jsonc") {
		content, _ := vfs.Read("deno.json")
		if content == nil {
			content, _ = vfs.Read("deno.jsonc")
		}

		metadata := &build.ProjectMetadata{
			Name:        "deno-project",
			Language:    "typescript",
			Version:     "1.0.0",
			Type:        "application",
			BuildSystem: "deno",
		}

		// Parse deno.json for more info
		var denoConfig map[string]interface{}
		if json.Unmarshal(content, &denoConfig) == nil {
			if name, ok := denoConfig["name"].(string); ok {
				metadata.Name = name
			}
		}

		return true, metadata, nil
	}

	// Check for standalone JavaScript files
	jsFiles, _ := vfs.Glob("*.js")
	tsFiles, _ := vfs.Glob("*.ts")
	
	if len(jsFiles) > 0 || len(tsFiles) > 0 {
		metadata := &build.ProjectMetadata{
			Name:            "javascript-project",
			Language:        "javascript",
			Version:         "1.0.0",
			Type:            "script",
			BuildSystem:     "none",
			DependencyCount: 0,
		}

		if len(tsFiles) > 0 {
			metadata.Language = "typescript"
		}

		// Find entry point
		for _, file := range append(jsFiles, tsFiles...) {
			if strings.Contains(file, "index") || strings.Contains(file, "main") || strings.Contains(file, "app") {
				metadata.MainFile = file
				break
			}
		}

		return true, metadata, nil
	}

	return false, nil, nil
}

// Build executes the JavaScript build process
func (p *JavaScriptPlugin) Build(ctx context.Context, config build.BuildConfig, vfs build.VirtualFileSystem) (*build.BuildResult, error) {
	start := time.Now()

	// Detect project
	detected, metadata, err := p.DetectProject(vfs)
	if !detected || err != nil {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("not a JavaScript project: %v", err),
		}, nil
	}

	// Choose build strategy based on build system
	switch metadata.BuildSystem {
	case "deno":
		return p.buildDeno(ctx, config, vfs, metadata, start)
	case "bun":
		return p.buildBun(ctx, config, vfs, metadata, start)
	case "webpack", "vite", "rollup", "parcel", "esbuild":
		return p.buildWithBundler(ctx, config, vfs, metadata, start)
	default:
		return p.buildNode(ctx, config, vfs, metadata, start)
	}
}

// buildNode builds with Node.js/npm
func (p *JavaScriptPlugin) buildNode(ctx context.Context, config build.BuildConfig, vfs build.VirtualFileSystem, metadata *build.ProjectMetadata, start time.Time) (*build.BuildResult, error) {
	// Validate Node.js installation
	if p.nodePath == "" {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("node binary not found in PATH"),
		}, nil
	}

	// Create filesystem bridge
	bridge, err := vfs.CreateBridge()
	if err != nil {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("failed to create filesystem bridge: %w", err),
		}, nil
	}
	defer bridge.Cleanup()

	// Read package.json
	packageContent, _ := vfs.Read("package.json")
	var packageJSON PackageJSON
	json.Unmarshal(packageContent, &packageJSON)

	artifacts := []build.BuildArtifact{}
	output := &build.BuildOutput{Info: []string{}}

	// Install dependencies if node_modules doesn't exist
	if !vfs.Exists("node_modules") && len(packageJSON.Dependencies) > 0 {
		installCmd := p.getPackageManager(vfs)
		installArgs := []string{"install"}
		
		if config.Mode == "release" {
			installArgs = append(installArgs, "--production")
		}

		result, err := bridge.Exec(installCmd, installArgs, nil)
		if err != nil && result.ExitCode != 0 {
			return &build.BuildResult{
				Success: false,
				Error:   fmt.Errorf("dependency installation failed: %s", result.Output),
				Output:  &build.BuildOutput{Stderr: result.Output},
			}, nil
		}
		output.Info = append(output.Info, "Dependencies installed successfully")
	}

	// Execute build based on target
	switch config.Target {
	case "bundle", "production":
		// Run build script if available
		if _, exists := packageJSON.Scripts["build"]; exists {
			result, err := p.runScript(bridge, "build", config.Environment)
			if err != nil {
				return &build.BuildResult{
					Success: false,
					Error:   fmt.Errorf("build script failed: %s", result.Output),
					Output:  &build.BuildOutput{Stderr: result.Output},
				}, nil
			}
			output.Stdout = result.Output
		}

		// Collect build artifacts from dist/build directory
		distDirs := []string{"dist", "build", "out", "public"}
		for _, dir := range distDirs {
			files, _ := vfs.Glob(fmt.Sprintf("%s/**/*", dir))
			for _, file := range files {
				content, err := vfs.Read(file)
				if err != nil {
					continue
				}

				artifacts = append(artifacts, build.BuildArtifact{
					Name:    filepath.Base(file),
					Type:    getJSArtifactType(file),
					Path:    file,
					Content: content,
					Size:    int64(len(content)),
					Hash:    hashContent(content),
				})
			}
		}

	case "test":
		// Run test script
		if _, exists := packageJSON.Scripts["test"]; exists {
			result, err := p.runScript(bridge, "test", config.Environment)
			testPassed := err == nil && result.ExitCode == 0
			
			artifacts = append(artifacts, build.BuildArtifact{
				Name:    "test-results.txt",
				Type:    "test-results",
				Path:    "test-results.txt",
				Content: []byte(result.Output),
				Size:    int64(len(result.Output)),
			})

			if !testPassed {
				return &build.BuildResult{
					Success: false,
					Error:   fmt.Errorf("tests failed"),
					Output:  &build.BuildOutput{Stderr: result.Output},
					Artifacts: artifacts,
				}, nil
			}
			output.Stdout = result.Output
		}

	case "lint":
		// Run lint script
		if _, exists := packageJSON.Scripts["lint"]; exists {
			result, _ := p.runScript(bridge, "lint", config.Environment)
			artifacts = append(artifacts, build.BuildArtifact{
				Name:    "lint-results.txt",
				Type:    "lint-results",
				Path:    "lint-results.txt",
				Content: []byte(result.Output),
				Size:    int64(len(result.Output)),
			})
			output.Stdout = result.Output
		}

	default:
		// Development mode or custom script
		if _, exists := packageJSON.Scripts[config.Target]; exists {
			result, err := p.runScript(bridge, config.Target, config.Environment)
			if err != nil {
				return &build.BuildResult{
					Success: false,
					Error:   fmt.Errorf("script %s failed: %s", config.Target, result.Output),
				}, nil
			}
			output.Stdout = result.Output
		} else {
			// Default: bundle the main file
			if metadata.MainFile != "" {
				content, _ := vfs.Read(metadata.MainFile)
				artifacts = append(artifacts, build.BuildArtifact{
					Name:    "bundle.js",
					Type:    "javascript",
					Path:    "bundle.js",
					Content: content,
					Size:    int64(len(content)),
				})
			}
		}
	}

	// Store artifacts in VFS
	for _, artifact := range artifacts {
		vfs.Write(filepath.Join(config.OutputPath, artifact.Name), artifact.Content)
	}

	return &build.BuildResult{
		Success:   true,
		Artifacts: artifacts,
		Metadata: &build.BuildMetadata{
			Plugin:    p.Name(),
			Project:   metadata,
			Timestamp: start,
			Config:    config,
			BuildID:   fmt.Sprintf("js-%d", time.Now().Unix()),
		},
		Output:   output,
		Duration: time.Since(start),
	}, nil
}

// buildDeno builds with Deno
func (p *JavaScriptPlugin) buildDeno(ctx context.Context, config build.BuildConfig, vfs build.VirtualFileSystem, metadata *build.ProjectMetadata, start time.Time) (*build.BuildResult, error) {
	if p.denoPath == "" {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("deno binary not found"),
		}, nil
	}

	bridge, err := vfs.CreateBridge()
	if err != nil {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("failed to create bridge: %w", err),
		}, nil
	}
	defer bridge.Cleanup()

	artifacts := []build.BuildArtifact{}
	
	// Deno compile command
	args := []string{"compile"}
	
	// Add permissions
	args = append(args, "--allow-read", "--allow-write", "--allow-net")
	
	// Set target
	outputName := "app"
	switch config.Target {
	case "linux":
		args = append(args, "--target", "x86_64-unknown-linux-gnu")
		outputName = "app-linux"
	case "windows":
		args = append(args, "--target", "x86_64-pc-windows-msvc")
		outputName = "app.exe"
	case "darwin":
		args = append(args, "--target", "x86_64-apple-darwin")
		outputName = "app-darwin"
	}

	// Set output
	outputPath := filepath.Join(bridge.TempPath(), outputName)
	args = append(args, "--output", outputPath)

	// Add main file
	mainFile := metadata.MainFile
	if mainFile == "" {
		// Find main TypeScript file
		tsFiles, _ := vfs.Glob("*.ts")
		for _, file := range tsFiles {
			if strings.Contains(file, "main") || strings.Contains(file, "index") || strings.Contains(file, "app") {
				mainFile = file
				break
			}
		}
	}
	
	if mainFile == "" {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("no main file found"),
		}, nil
	}
	
	args = append(args, mainFile)

	// Execute build
	result, err := bridge.Exec(p.denoPath, args, nil)
	if err != nil && result.ExitCode != 0 {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("deno compile failed: %s", result.Output),
		}, nil
	}

	// Read compiled binary
	if binaryData, err := os.ReadFile(outputPath); err == nil {
		artifacts = append(artifacts, build.BuildArtifact{
			Name:       outputName,
			Type:       "binary",
			Path:       filepath.Join(config.OutputPath, outputName),
			Content:    binaryData,
			Size:       int64(len(binaryData)),
			Hash:       hashContent(binaryData),
			Executable: true,
		})
		
		vfs.Write(filepath.Join(config.OutputPath, outputName), binaryData)
	}

	return &build.BuildResult{
		Success:   true,
		Artifacts: artifacts,
		Metadata: &build.BuildMetadata{
			Plugin:    "deno",
			Project:   metadata,
			Timestamp: start,
			Config:    config,
		},
		Output:   &build.BuildOutput{Stdout: result.Output},
		Duration: time.Since(start),
	}, nil
}

// buildBun builds with Bun
func (p *JavaScriptPlugin) buildBun(ctx context.Context, config build.BuildConfig, vfs build.VirtualFileSystem, metadata *build.ProjectMetadata, start time.Time) (*build.BuildResult, error) {
	if p.bunPath == "" {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("bun binary not found"),
		}, nil
	}

	bridge, err := vfs.CreateBridge()
	if err != nil {
		return &build.BuildResult{
			Success: false,
			Error:   err,
		}, nil
	}
	defer bridge.Cleanup()

	// Bun build is super fast!
	args := []string{"build"}
	
	if config.Target == "bundle" {
		args = append(args, "--compile")
	}
	
	mainFile := metadata.MainFile
	if mainFile == "" {
		mainFile = "index.js"
	}
	
	args = append(args, mainFile)
	args = append(args, "--outdir", bridge.TempPath())

	result, err := bridge.Exec(p.bunPath, args, nil)
	if err != nil {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("bun build failed: %s", result.Output),
		}, nil
	}

	// Collect artifacts
	artifacts := []build.BuildArtifact{}
	files, _ := vfs.List()
	for _, file := range files {
		if strings.HasSuffix(file, ".js") || strings.HasSuffix(file, ".mjs") {
			content, _ := vfs.Read(file)
			artifacts = append(artifacts, build.BuildArtifact{
				Name:    filepath.Base(file),
				Type:    "javascript",
				Path:    file,
				Content: content,
				Size:    int64(len(content)),
			})
		}
	}

	return &build.BuildResult{
		Success:   true,
		Artifacts: artifacts,
		Duration:  time.Since(start),
	}, nil
}

// buildWithBundler builds using webpack/vite/rollup
func (p *JavaScriptPlugin) buildWithBundler(ctx context.Context, config build.BuildConfig, vfs build.VirtualFileSystem, metadata *build.ProjectMetadata, start time.Time) (*build.BuildResult, error) {
	bridge, err := vfs.CreateBridge()
	if err != nil {
		return &build.BuildResult{
			Success: false,
			Error:   err,
		}, nil
	}
	defer bridge.Cleanup()

	// Determine bundler command
	bundlerCmd := ""
	bundlerArgs := []string{}
	
	switch metadata.BuildSystem {
	case "webpack":
		bundlerCmd = "npx"
		bundlerArgs = []string{"webpack"}
		if config.Mode == "release" {
			bundlerArgs = append(bundlerArgs, "--mode", "production")
		} else {
			bundlerArgs = append(bundlerArgs, "--mode", "development")
		}
		
	case "vite":
		bundlerCmd = "npx"
		bundlerArgs = []string{"vite", "build"}
		if config.Mode == "debug" {
			bundlerArgs = append(bundlerArgs, "--mode", "development")
		}
		
	case "rollup":
		bundlerCmd = "npx"
		bundlerArgs = []string{"rollup", "-c"}
		
	case "parcel":
		bundlerCmd = "npx"
		bundlerArgs = []string{"parcel", "build", metadata.MainFile}
		
	case "esbuild":
		bundlerCmd = "npx"
		bundlerArgs = []string{"esbuild", metadata.MainFile, "--bundle"}
		if config.Target == "production" {
			bundlerArgs = append(bundlerArgs, "--minify")
		}
		bundlerArgs = append(bundlerArgs, "--outfile=dist/bundle.js")
	}

	result, err := bridge.Exec(bundlerCmd, bundlerArgs, config.Environment)
	if err != nil {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("%s build failed: %s", metadata.BuildSystem, result.Output),
		}, nil
	}

	// Collect bundled artifacts
	artifacts := []build.BuildArtifact{}
	distFiles, _ := vfs.Glob("dist/**/*")
	for _, file := range distFiles {
		content, _ := vfs.Read(file)
		artifacts = append(artifacts, build.BuildArtifact{
			Name:    filepath.Base(file),
			Type:    getJSArtifactType(file),
			Path:    file,
			Content: content,
			Size:    int64(len(content)),
			Hash:    hashContent(content),
		})
	}

	return &build.BuildResult{
		Success:   true,
		Artifacts: artifacts,
		Output:    &build.BuildOutput{Stdout: result.Output},
		Duration:  time.Since(start),
	}, nil
}

// GetDependencies returns JavaScript dependencies
func (p *JavaScriptPlugin) GetDependencies(config build.BuildConfig, vfs build.VirtualFileSystem) ([]build.Dependency, error) {
	if !vfs.Exists("package.json") {
		return nil, nil
	}

	content, err := vfs.Read("package.json")
	if err != nil {
		return nil, err
	}

	var packageJSON PackageJSON
	if err := json.Unmarshal(content, &packageJSON); err != nil {
		return nil, err
	}

	deps := []build.Dependency{}
	
	// Runtime dependencies
	for name, version := range packageJSON.Dependencies {
		deps = append(deps, build.Dependency{
			Name:    name,
			Version: version,
			Type:    "runtime",
			Source:  "npm",
		})
	}
	
	// Dev dependencies
	for name, version := range packageJSON.DevDependencies {
		deps = append(deps, build.Dependency{
			Name:    name,
			Version: version,
			Type:    "dev",
			Source:  "npm",
		})
	}

	return deps, nil
}

// ValidateBuild validates JavaScript build configuration
func (p *JavaScriptPlugin) ValidateBuild(config build.BuildConfig, vfs build.VirtualFileSystem) error {
	// Check for JavaScript/TypeScript files
	jsFiles, _ := vfs.Glob("**/*.js")
	tsFiles, _ := vfs.Glob("**/*.ts")
	
	if len(jsFiles) == 0 && len(tsFiles) == 0 {
		return fmt.Errorf("no JavaScript or TypeScript files found")
	}

	// Validate Node.js availability for non-Deno projects
	if !vfs.Exists("deno.json") && p.nodePath == "" {
		return fmt.Errorf("Node.js is required but not found")
	}

	return nil
}

// SupportsMemoryBuild returns true
func (p *JavaScriptPlugin) SupportsMemoryBuild() bool {
	return true
}

// Helper functions

func (p *JavaScriptPlugin) runScript(bridge build.FilesystemBridge, script string, env map[string]string) (*build.ExecResult, error) {
	pm := p.getPackageManagerFromBridge(bridge)
	args := []string{"run", script}
	return bridge.Exec(pm, args, env)
}

func (p *JavaScriptPlugin) getPackageManager(vfs build.VirtualFileSystem) string {
	if vfs.Exists("yarn.lock") && p.yarnPath != "" {
		return p.yarnPath
	}
	if vfs.Exists("pnpm-lock.yaml") {
		if pnpm := findExecutable("pnpm"); pnpm != "" {
			return pnpm
		}
	}
	if vfs.Exists("bun.lockb") && p.bunPath != "" {
		return p.bunPath
	}
	return p.npmPath
}

func (p *JavaScriptPlugin) getPackageManagerFromBridge(bridge build.FilesystemBridge) string {
	// Check temp path for lock files
	tempPath := bridge.TempPath()
	if _, err := os.Stat(filepath.Join(tempPath, "yarn.lock")); err == nil && p.yarnPath != "" {
		return p.yarnPath
	}
	if _, err := os.Stat(filepath.Join(tempPath, "pnpm-lock.yaml")); err == nil {
		if pnpm := findExecutable("pnpm"); pnpm != "" {
			return pnpm
		}
	}
	if _, err := os.Stat(filepath.Join(tempPath, "bun.lockb")); err == nil && p.bunPath != "" {
		return p.bunPath
	}
	return p.npmPath
}

func findExecutable(name string) string {
	paths := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))
	for _, path := range paths {
		exe := filepath.Join(path, name)
		if _, err := os.Stat(exe); err == nil {
			return exe
		}
		// Try with .exe on Windows
		if _, err := os.Stat(exe + ".exe"); err == nil {
			return exe + ".exe"
		}
		// Try with .cmd on Windows (npm, yarn)
		if _, err := os.Stat(exe + ".cmd"); err == nil {
			return exe + ".cmd"
		}
	}
	return ""
}

func detectBuildSystem(vfs build.VirtualFileSystem, pkg PackageJSON) string {
	// Check for specific config files
	if vfs.Exists("webpack.config.js") || vfs.Exists("webpack.config.ts") {
		return "webpack"
	}
	if vfs.Exists("vite.config.js") || vfs.Exists("vite.config.ts") {
		return "vite"
	}
	if vfs.Exists("rollup.config.js") || vfs.Exists("rollup.config.ts") {
		return "rollup"
	}
	if vfs.Exists(".parcelrc") {
		return "parcel"
	}
	if vfs.Exists("esbuild.config.js") {
		return "esbuild"
	}
	if vfs.Exists("deno.json") || vfs.Exists("deno.jsonc") {
		return "deno"
	}
	if vfs.Exists("bun.lockb") {
		return "bun"
	}
	
	// Check package.json scripts
	for script := range pkg.Scripts {
		if strings.Contains(script, "webpack") {
			return "webpack"
		}
		if strings.Contains(script, "vite") {
			return "vite"
		}
		if strings.Contains(script, "rollup") {
			return "rollup"
		}
	}
	
	// Check dev dependencies
	for dep := range pkg.DevDependencies {
		if strings.Contains(dep, "webpack") {
			return "webpack"
		}
		if strings.Contains(dep, "vite") {
			return "vite"
		}
		if strings.Contains(dep, "rollup") {
			return "rollup"
		}
		if strings.Contains(dep, "parcel") {
			return "parcel"
		}
		if strings.Contains(dep, "esbuild") {
			return "esbuild"
		}
	}
	
	return "npm"
}

func determineProjectType(pkg PackageJSON) string {
	// Check for known frameworks
	deps := mergeStringMaps(pkg.Dependencies, pkg.DevDependencies)
	
	if _, ok := deps["react"]; ok {
		return "react"
	}
	if _, ok := deps["vue"]; ok {
		return "vue"
	}
	if _, ok := deps["@angular/core"]; ok {
		return "angular"
	}
	if _, ok := deps["svelte"]; ok {
		return "svelte"
	}
	if _, ok := deps["next"]; ok {
		return "nextjs"
	}
	if _, ok := deps["nuxt"]; ok {
		return "nuxt"
	}
	if _, ok := deps["express"]; ok {
		return "express"
	}
	if _, ok := deps["fastify"]; ok {
		return "fastify"
	}
	if _, ok := deps["electron"]; ok {
		return "electron"
	}
	
	// Check main field
	if pkg.Main != "" {
		if pkg.Bin != nil {
			return "cli"
		}
		return "library"
	}
	
	return "application"
}

func getJSArtifactType(filename string) string {
	ext := filepath.Ext(filename)
	switch ext {
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	case ".css":
		return "stylesheet"
	case ".html":
		return "html"
	case ".map":
		return "sourcemap"
	case ".json":
		return "config"
	case ".wasm":
		return "wasm"
	default:
		if strings.Contains(filename, "bundle") {
			return "bundle"
		}
		return "asset"
	}
}

func hashContent(content []byte) string {
	hasher := sha256.New()
	hasher.Write(content)
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func mergeStringMaps(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// PackageJSON represents package.json structure
type PackageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Main            string            `json:"main,omitempty"`
	Scripts         map[string]string `json:"scripts,omitempty"`
	Dependencies    map[string]string `json:"dependencies,omitempty"`
	DevDependencies map[string]string `json:"devDependencies,omitempty"`
	Bin             interface{}       `json:"bin,omitempty"`
	Type            string            `json:"type,omitempty"`
	Module          string            `json:"module,omitempty"`
	Browser         interface{}       `json:"browser,omitempty"`
	Exports         interface{}       `json:"exports,omitempty"`
}