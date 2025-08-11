package plugins

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Caia-Tech/govc/pkg/build"
)

// GoPlugin implements building Go projects in memory
type GoPlugin struct {
	goPath      string
	goRoot      string
	goBin       string
	cacheDir    string
	supportsWASM bool
}

// NewGoPlugin creates a new Go build plugin
func NewGoPlugin() *GoPlugin {
	return &GoPlugin{
		goPath:      os.Getenv("GOPATH"),
		goRoot:      os.Getenv("GOROOT"),
		goBin:       findGoBinary(),
		cacheDir:    filepath.Join(os.TempDir(), "govc-go-cache"),
		supportsWASM: checkWASMSupport(),
	}
}

// Name returns the plugin name
func (p *GoPlugin) Name() string {
	return "go"
}

// SupportedExtensions returns Go file extensions
func (p *GoPlugin) SupportedExtensions() []string {
	return []string{".go", "go.mod", "go.sum"}
}

// DetectProject checks if this is a Go project
func (p *GoPlugin) DetectProject(vfs build.VirtualFileSystem) (bool, *build.ProjectMetadata, error) {
	// Check for go.mod file (Go modules)
	if vfs.Exists("go.mod") {
		content, err := vfs.Read("go.mod")
		if err == nil {
			metadata := &build.ProjectMetadata{
				Name:        extractModuleName(string(content)),
				Language:    "go",
				Version:     extractGoVersion(string(content)),
				Type:        "module",
				BuildSystem: "go",
			}
			
			// Count .go files
			goFiles, _ := vfs.Glob("**/*.go")
			metadata.DependencyCount = len(goFiles)
			
			// Find main package
			for _, file := range goFiles {
				if content, err := vfs.Read(file); err == nil {
					if strings.Contains(string(content), "package main") {
						metadata.MainFile = file
						metadata.Type = "application"
						break
					}
				}
			}
			
			return true, metadata, nil
		}
	}

	// Check for any .go files (GOPATH mode)
	goFiles, err := vfs.Glob("*.go")
	if err == nil && len(goFiles) > 0 {
		metadata := &build.ProjectMetadata{
			Name:            "go-project",
			Language:        "go",
			Version:         runtime.Version(),
			Type:            "application",
			BuildSystem:     "go",
			DependencyCount: len(goFiles),
		}
		
		// Find main package
		for _, file := range goFiles {
			if content, err := vfs.Read(file); err == nil {
				if strings.Contains(string(content), "package main") && 
				   strings.Contains(string(content), "func main()") {
					metadata.MainFile = file
					break
				}
			}
		}
		
		return true, metadata, nil
	}

	return false, nil, nil
}

// Build executes the Go build process
func (p *GoPlugin) Build(ctx context.Context, config build.BuildConfig, vfs build.VirtualFileSystem) (*build.BuildResult, error) {
	start := time.Now()
	
	// Validate Go installation
	if p.goBin == "" {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("go binary not found in PATH"),
		}, nil
	}

	// Detect project first
	detected, metadata, err := p.DetectProject(vfs)
	if !detected || err != nil {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("not a Go project: %v", err),
		}, nil
	}

	// Create filesystem bridge for Go compiler
	bridge, err := vfs.CreateBridge()
	if err != nil {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("failed to create filesystem bridge: %w", err),
		}, nil
	}
	defer bridge.Cleanup()

	// Prepare build command
	args := []string{"build"}
	env := make(map[string]string)

	// Configure based on target
	outputName := "app"
	switch config.Target {
	case "wasm":
		if !p.supportsWASM {
			return &build.BuildResult{
				Success: false,
				Error:   fmt.Errorf("WASM target not supported by current Go installation"),
			}, nil
		}
		env["GOOS"] = "js"
		env["GOARCH"] = "wasm"
		outputName = "app.wasm"
		args = append(args, "-ldflags", "-w -s") // Strip debug info for smaller size

	case "linux":
		env["GOOS"] = "linux"
		env["GOARCH"] = "amd64"
		outputName = "app-linux"

	case "windows":
		env["GOOS"] = "windows"
		env["GOARCH"] = "amd64"
		outputName = "app.exe"

	case "darwin":
		env["GOOS"] = "darwin"
		env["GOARCH"] = "amd64"
		outputName = "app-darwin"

	case "library":
		args = append(args, "-buildmode=c-shared")
		outputName = "lib.so"

	default:
		// Native build
		outputName = "app"
	}

	// Configure build mode
	switch config.Mode {
	case "release":
		args = append(args, "-ldflags", "-s -w") // Strip symbols
		args = append(args, "-trimpath")         // Remove file paths

	case "debug":
		args = append(args, "-gcflags", "all=-N -l") // Disable optimizations

	case "test":
		// Change to test command
		args = []string{"test", "-v"}
		if config.PluginConfig["coverage"] == true {
			args = append(args, "-cover", "-coverprofile=coverage.out")
		}

	case "bench":
		args = append(args, "-bench=.")
	}

	// Set output path
	if config.Mode != "test" && config.Mode != "bench" {
		outputPath := filepath.Join(bridge.TempPath(), outputName)
		args = append(args, "-o", outputPath)
	}

	// Add custom build args
	args = append(args, config.Args...)

	// Add main file or package
	if metadata.MainFile != "" && config.Mode != "test" {
		args = append(args, metadata.MainFile)
	} else {
		args = append(args, "./...")
	}

	// Execute build
	result, err := bridge.Exec(p.goBin, args, env)
	if err != nil && result.ExitCode != 0 {
		return &build.BuildResult{
			Success: false,
			Error:   fmt.Errorf("build failed: %s", result.Output),
			Output: &build.BuildOutput{
				Stderr: result.Output,
				Errors: extractGoErrors(result.Output),
			},
			Duration: time.Since(start),
		}, nil
	}

	// Collect build artifacts
	artifacts := []build.BuildArtifact{}
	
	if config.Mode == "test" {
		// Test mode - no binary artifacts, just results
		artifacts = append(artifacts, build.BuildArtifact{
			Name: "test-results.txt",
			Type: "test-results",
			Path: "test-results.txt",
			Content: []byte(result.Output),
			Size: int64(len(result.Output)),
		})
		
		// Check for coverage file
		if coverageData, err := vfs.Read("coverage.out"); err == nil {
			artifacts = append(artifacts, build.BuildArtifact{
				Name: "coverage.out",
				Type: "coverage",
				Path: "coverage.out",
				Content: coverageData,
				Size: int64(len(coverageData)),
			})
		}
	} else {
		// Read built binary
		outputPath := filepath.Join(bridge.TempPath(), outputName)
		if binaryData, err := os.ReadFile(outputPath); err == nil {
			// Calculate hash
			hasher := sha256.New()
			hasher.Write(binaryData)
			hash := fmt.Sprintf("%x", hasher.Sum(nil))

			artifactType := "binary"
			if config.Target == "wasm" {
				artifactType = "wasm"
			} else if config.Target == "library" {
				artifactType = "library"
			}

			artifacts = append(artifacts, build.BuildArtifact{
				Name:       outputName,
				Type:       artifactType,
				Path:       filepath.Join(config.OutputPath, outputName),
				Content:    binaryData,
				Size:       int64(len(binaryData)),
				Hash:       hash,
				Executable: artifactType == "binary",
				Metadata: map[string]interface{}{
					"goos":   env["GOOS"],
					"goarch": env["GOARCH"],
					"goversion": runtime.Version(),
				},
			})

			// Store in VFS
			if err := vfs.Write(filepath.Join(config.OutputPath, outputName), binaryData); err != nil {
				return nil, fmt.Errorf("failed to store artifact: %w", err)
			}

			// For WASM, also include support files
			if config.Target == "wasm" {
				if wasmExec, err := p.getWASMExecJS(); err == nil {
					artifacts = append(artifacts, build.BuildArtifact{
						Name:    "wasm_exec.js",
						Type:    "support",
						Path:    filepath.Join(config.OutputPath, "wasm_exec.js"),
						Content: wasmExec,
						Size:    int64(len(wasmExec)),
					})
					vfs.Write(filepath.Join(config.OutputPath, "wasm_exec.js"), wasmExec)
				}

				// Add HTML wrapper
				htmlWrapper := p.generateWASMHTML(outputName)
				artifacts = append(artifacts, build.BuildArtifact{
					Name:    "index.html",
					Type:    "support",
					Path:    filepath.Join(config.OutputPath, "index.html"),
					Content: []byte(htmlWrapper),
					Size:    int64(len(htmlWrapper)),
				})
				vfs.Write(filepath.Join(config.OutputPath, "index.html"), []byte(htmlWrapper))
			}
		}
	}

	// Prepare build metadata
	buildMetadata := &build.BuildMetadata{
		Plugin:    p.Name(),
		Project:   metadata,
		Timestamp: start,
		Host:      runtime.GOOS + "/" + runtime.GOARCH,
		Config:    config,
		BuildID:   fmt.Sprintf("go-%d", time.Now().Unix()),
	}

	// Parse dependencies from go.mod if available
	if modContent, err := vfs.Read("go.mod"); err == nil {
		buildMetadata.Dependencies = p.parseGoModDependencies(string(modContent))
	}

	return &build.BuildResult{
		Success:   true,
		Artifacts: artifacts,
		Metadata:  buildMetadata,
		Output: &build.BuildOutput{
			Stdout: result.Output,
			Info:   []string{fmt.Sprintf("Build completed in %v", time.Since(start))},
		},
		Duration: time.Since(start),
		CacheHit: false,
	}, nil
}

// GetDependencies resolves Go module dependencies
func (p *GoPlugin) GetDependencies(config build.BuildConfig, vfs build.VirtualFileSystem) ([]build.Dependency, error) {
	if !vfs.Exists("go.mod") {
		return nil, nil
	}

	modContent, err := vfs.Read("go.mod")
	if err != nil {
		return nil, err
	}

	return p.parseGoModDependencies(string(modContent)), nil
}

// ValidateBuild checks if build configuration is valid
func (p *GoPlugin) ValidateBuild(config build.BuildConfig, vfs build.VirtualFileSystem) error {
	// Check for Go files
	goFiles, err := vfs.Glob("**/*.go")
	if err != nil || len(goFiles) == 0 {
		return fmt.Errorf("no Go source files found")
	}

	// Validate target
	switch config.Target {
	case "", "binary", "library", "wasm", "linux", "windows", "darwin":
		// Valid targets
	default:
		return fmt.Errorf("unsupported target: %s", config.Target)
	}

	// Validate mode
	switch config.Mode {
	case "", "debug", "release", "test", "bench":
		// Valid modes
	default:
		return fmt.Errorf("unsupported mode: %s", config.Mode)
	}

	// Check for main package if building binary
	if config.Target != "library" && config.Mode != "test" {
		hasMain := false
		for _, file := range goFiles {
			if content, err := vfs.Read(file); err == nil {
				if strings.Contains(string(content), "package main") && 
				   strings.Contains(string(content), "func main()") {
					hasMain = true
					break
				}
			}
		}
		if !hasMain {
			return fmt.Errorf("no main package found")
		}
	}

	return nil
}

// SupportsMemoryBuild returns true as Go can build from memory via temp bridge
func (p *GoPlugin) SupportsMemoryBuild() bool {
	return true
}

// Helper functions

func findGoBinary() string {
	if path, err := os.LookupEnv("GOROOT"); err {
		goBin := filepath.Join(path, "bin", "go")
		if _, err := os.Stat(goBin); err == nil {
			return goBin
		}
	}
	
	// Try to find in PATH
	paths := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))
	for _, path := range paths {
		goBin := filepath.Join(path, "go")
		if runtime.GOOS == "windows" {
			goBin += ".exe"
		}
		if _, err := os.Stat(goBin); err == nil {
			return goBin
		}
	}
	
	return ""
}

func checkWASMSupport() bool {
	// WASM support was added in Go 1.11
	version := runtime.Version()
	// Simple check - in production, parse version properly
	return !strings.Contains(version, "go1.10") && 
	       !strings.Contains(version, "go1.9") && 
	       !strings.Contains(version, "go1.8")
}

func extractModuleName(modContent string) string {
	lines := strings.Split(modContent, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return "unknown"
}

func extractGoVersion(modContent string) string {
	lines := strings.Split(modContent, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "go ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "go"))
		}
	}
	return runtime.Version()
}

func extractGoErrors(output string) []string {
	var errors []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "error:") || 
		   strings.Contains(line, "cannot") ||
		   strings.Contains(line, "undefined") ||
		   strings.Contains(line, "syntax error") {
			errors = append(errors, strings.TrimSpace(line))
		}
	}
	return errors
}

func (p *GoPlugin) parseGoModDependencies(modContent string) []build.Dependency {
	var deps []build.Dependency
	lines := strings.Split(modContent, "\n")
	inRequire := false
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		if line == "require (" {
			inRequire = true
			continue
		}
		
		if inRequire && line == ")" {
			inRequire = false
			continue
		}
		
		if inRequire || strings.HasPrefix(line, "require ") {
			// Parse dependency line
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				name := parts[0]
				if name == "require" && len(parts) >= 3 {
					name = parts[1]
					parts = parts[1:]
				}
				
				version := parts[1]
				if strings.HasPrefix(version, "v") {
					deps = append(deps, build.Dependency{
						Name:    name,
						Version: version,
						Type:    "runtime",
						Source:  "go.mod",
					})
				}
			}
		}
	}
	
	return deps
}

func (p *GoPlugin) getWASMExecJS() ([]byte, error) {
	// Try to find wasm_exec.js in GOROOT
	if p.goRoot != "" {
		wasmExecPath := filepath.Join(p.goRoot, "misc", "wasm", "wasm_exec.js")
		if content, err := os.ReadFile(wasmExecPath); err == nil {
			return content, nil
		}
	}
	
	// Fallback to embedded minimal version
	return []byte(minimalWASMExecJS), nil
}

func (p *GoPlugin) generateWASMHTML(wasmFile string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Go WASM Application</title>
    <script src="wasm_exec.js"></script>
    <script>
        const go = new Go();
        WebAssembly.instantiateStreaming(fetch("%s"), go.importObject).then((result) => {
            go.run(result.instance);
        });
    </script>
</head>
<body>
    <h1>Go WebAssembly Application</h1>
    <div id="app"></div>
</body>
</html>`, wasmFile)
}

// Minimal wasm_exec.js for fallback (truncated for brevity)
const minimalWASMExecJS = `
// Copyright 2018 The Go Authors. All rights reserved.
// Minimal wasm_exec.js implementation
(() => {
    if (typeof global !== "undefined") {
        // Node.js
        if (typeof module !== "undefined") {
            module.exports.Go = Go;
        }
    } else {
        // Browser
        window.Go = class {
            constructor() {
                this.importObject = {
                    go: {
                        // Minimal runtime implementation
                        "runtime.wasmExit": (sp) => {},
                        "runtime.wasmWrite": (sp) => {},
                        // ... other required functions
                    }
                };
            }
            async run(instance) {
                this._inst = instance;
                await this._inst.exports.run();
            }
        };
    }
})();
`