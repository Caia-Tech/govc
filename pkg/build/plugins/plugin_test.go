package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Caia-Tech/govc/pkg/build"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGoPlugin tests the Go build plugin
func TestGoPlugin(t *testing.T) {
	plugin := NewGoPlugin()

	t.Run("PluginMetadata", func(t *testing.T) {
		assert.Equal(t, "go", plugin.Name())
		assert.Contains(t, plugin.SupportedExtensions(), ".go")
		assert.Contains(t, plugin.SupportedExtensions(), "go.mod")
		assert.True(t, plugin.SupportsMemoryBuild())
	})

	t.Run("DetectGoModule", func(t *testing.T) {
		vfs := build.NewDirectMemoryVFS()
		
		// Create go.mod
		goMod := `module github.com/test/app
go 1.21

require (
	github.com/stretchr/testify v1.8.0
	github.com/gin-gonic/gin v1.9.0
)
`
		vfs.Write("go.mod", []byte(goMod))
		vfs.Write("main.go", []byte("package main\nfunc main() {}"))
		
		detected, metadata, err := plugin.DetectProject(vfs)
		require.NoError(t, err)
		assert.True(t, detected)
		assert.Equal(t, "github.com/test/app", metadata.Name)
		assert.Equal(t, "go", metadata.Language)
		assert.Equal(t, "1.21", metadata.Version)
		assert.Equal(t, "module", metadata.Type)
	})

	t.Run("DetectGoPath", func(t *testing.T) {
		vfs := build.NewDirectMemoryVFS()
		
		// No go.mod, just .go files
		vfs.Write("main.go", []byte("package main\nfunc main() { println(\"hello\") }"))
		vfs.Write("lib.go", []byte("package main\nfunc Lib() string { return \"lib\" }"))
		
		detected, metadata, err := plugin.DetectProject(vfs)
		require.NoError(t, err)
		assert.True(t, detected)
		assert.Equal(t, "go-project", metadata.Name)
		assert.Equal(t, "main.go", metadata.MainFile)
		assert.Equal(t, 2, metadata.DependencyCount) // Number of .go files
	})

	t.Run("DetectNonGoProject", func(t *testing.T) {
		vfs := build.NewDirectMemoryVFS()
		
		// Only non-Go files
		vfs.Write("main.js", []byte("console.log('hello')"))
		vfs.Write("package.json", []byte("{}"))
		
		detected, _, err := plugin.DetectProject(vfs)
		assert.NoError(t, err)
		assert.False(t, detected)
	})

	t.Run("ParseDependencies", func(t *testing.T) {
		vfs := build.NewDirectMemoryVFS()
		
		goMod := `module example
go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
	github.com/stretchr/testify v1.8.4
	golang.org/x/crypto v0.14.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
)
`
		vfs.Write("go.mod", []byte(goMod))
		
		config := build.BuildConfig{}
		deps, err := plugin.GetDependencies(config, vfs)
		require.NoError(t, err)
		
		assert.Len(t, deps, 5) // Direct + indirect
		
		// Check specific dependency
		var found bool
		for _, dep := range deps {
			if dep.Name == "github.com/gin-gonic/gin" {
				found = true
				assert.Equal(t, "v1.9.1", dep.Version)
				assert.Equal(t, "runtime", dep.Type)
				assert.Equal(t, "go.mod", dep.Source)
			}
		}
		assert.True(t, found, "Should find gin dependency")
	})

	t.Run("ValidateBuildValid", func(t *testing.T) {
		vfs := build.NewDirectMemoryVFS()
		
		// Valid Go project
		vfs.Write("main.go", []byte("package main\nfunc main() {}"))
		vfs.Write("go.mod", []byte("module test\ngo 1.21"))
		
		config := build.BuildConfig{
			Target: "binary",
			Mode:   "debug",
		}
		
		err := plugin.ValidateBuild(config, vfs)
		assert.NoError(t, err)
	})

	t.Run("ValidateBuildNoMainPackage", func(t *testing.T) {
		vfs := build.NewDirectMemoryVFS()
		
		// Library project (no main)
		vfs.Write("lib.go", []byte("package lib\nfunc Lib() {}"))
		
		config := build.BuildConfig{
			Target: "binary",
			Mode:   "debug",
		}
		
		err := plugin.ValidateBuild(config, vfs)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no main package")
	})

	t.Run("ValidateBuildLibrary", func(t *testing.T) {
		vfs := build.NewDirectMemoryVFS()
		
		// Library project
		vfs.Write("lib.go", []byte("package lib\nfunc Export() string { return \"exported\" }"))
		
		config := build.BuildConfig{
			Target: "library",
			Mode:   "release",
		}
		
		err := plugin.ValidateBuild(config, vfs)
		assert.NoError(t, err) // Libraries don't need main package
	})

	t.Run("BuildConfiguration", func(t *testing.T) {
		// Test build configuration without actual build
		vfs := build.NewDirectMemoryVFS()
		vfs.Write("main.go", []byte("package main\nfunc main() {}"))
		vfs.Write("go.mod", []byte("module test"))
		
		testCases := []struct {
			target      string
			mode        string
			expectError bool
		}{
			{"binary", "debug", false},
			{"binary", "release", false},
			{"wasm", "release", false},
			{"library", "debug", false},
			{"invalid", "debug", true},
			{"binary", "invalid", true},
		}
		
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s_%s", tc.target, tc.mode), func(t *testing.T) {
				config := build.BuildConfig{
					Target: tc.target,
					Mode:   tc.mode,
				}
				
				err := plugin.ValidateBuild(config, vfs)
				if tc.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
}

// TestJavaScriptPlugin tests the JavaScript build plugin
func TestJavaScriptPlugin(t *testing.T) {
	plugin := NewJavaScriptPlugin()

	t.Run("PluginMetadata", func(t *testing.T) {
		assert.Equal(t, "javascript", plugin.Name())
		assert.Contains(t, plugin.SupportedExtensions(), ".js")
		assert.Contains(t, plugin.SupportedExtensions(), ".ts")
		assert.Contains(t, plugin.SupportedExtensions(), "package.json")
		assert.True(t, plugin.SupportsMemoryBuild())
	})

	t.Run("DetectNodeProject", func(t *testing.T) {
		vfs := build.NewDirectMemoryVFS()
		
		packageJSON := `{
  "name": "test-app",
  "version": "1.2.3",
  "main": "index.js",
  "scripts": {
    "build": "webpack",
    "test": "jest"
  },
  "dependencies": {
    "express": "^4.18.0",
    "lodash": "^4.17.21"
  },
  "devDependencies": {
    "webpack": "^5.88.0",
    "jest": "^29.0.0"
  }
}`
		vfs.Write("package.json", []byte(packageJSON))
		vfs.Write("index.js", []byte("console.log('hello')"))
		
		detected, metadata, err := plugin.DetectProject(vfs)
		require.NoError(t, err)
		assert.True(t, detected)
		assert.Equal(t, "test-app", metadata.Name)
		assert.Equal(t, "javascript", metadata.Language)
		assert.Equal(t, "1.2.3", metadata.Version)
		assert.Equal(t, "index.js", metadata.MainFile)
		assert.Equal(t, 4, metadata.DependencyCount) // 2 deps + 2 devDeps
	})

	t.Run("DetectTypeScriptProject", func(t *testing.T) {
		vfs := build.NewDirectMemoryVFS()
		
		vfs.Write("package.json", []byte(`{"name": "ts-app", "version": "1.0.0"}`))
		vfs.Write("tsconfig.json", []byte(`{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs"
  }
}`))
		vfs.Write("index.ts", []byte("const greeting: string = 'hello'"))
		
		detected, metadata, err := plugin.DetectProject(vfs)
		require.NoError(t, err)
		assert.True(t, detected)
		assert.Equal(t, "typescript", metadata.Language)
	})

	t.Run("DetectDenoProject", func(t *testing.T) {
		vfs := build.NewDirectMemoryVFS()
		
		denoJSON := `{
  "name": "deno-app",
  "tasks": {
    "dev": "deno run --watch main.ts"
  }
}`
		vfs.Write("deno.json", []byte(denoJSON))
		vfs.Write("main.ts", []byte("console.log('Deno!')"))
		
		detected, metadata, err := plugin.DetectProject(vfs)
		require.NoError(t, err)
		assert.True(t, detected)
		assert.Equal(t, "deno-app", metadata.Name)
		assert.Equal(t, "typescript", metadata.Language)
		assert.Equal(t, "deno", metadata.BuildSystem)
	})

	t.Run("DetectBuildSystem", func(t *testing.T) {
		testCases := []struct {
			files    map[string]string
			expected string
		}{
			{
				files: map[string]string{
					"package.json":      `{"name": "app"}`,
					"webpack.config.js": "module.exports = {}",
				},
				expected: "webpack",
			},
			{
				files: map[string]string{
					"package.json":    `{"name": "app"}`,
					"vite.config.js":  "export default {}",
				},
				expected: "vite",
			},
			{
				files: map[string]string{
					"package.json":      `{"name": "app"}`,
					"rollup.config.js":  "export default {}",
				},
				expected: "rollup",
			},
			{
				files: map[string]string{
					"package.json": `{"name": "app"}`,
					"bun.lockb":    "",
				},
				expected: "bun",
			},
		}
		
		for _, tc := range testCases {
			t.Run(tc.expected, func(t *testing.T) {
				vfs := build.NewDirectMemoryVFS()
				for file, content := range tc.files {
					vfs.Write(file, []byte(content))
				}
				
				detected, metadata, err := plugin.DetectProject(vfs)
				require.NoError(t, err)
				assert.True(t, detected)
				assert.Equal(t, tc.expected, metadata.BuildSystem)
			})
		}
	})

	t.Run("ParseDependencies", func(t *testing.T) {
		vfs := build.NewDirectMemoryVFS()
		
		packageJSON := `{
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "axios": "^1.4.0"
  },
  "devDependencies": {
    "@types/react": "^18.2.0",
    "typescript": "^5.0.0",
    "vite": "^4.4.0"
  }
}`
		vfs.Write("package.json", []byte(packageJSON))
		
		config := build.BuildConfig{}
		deps, err := plugin.GetDependencies(config, vfs)
		require.NoError(t, err)
		
		assert.Len(t, deps, 6)
		
		// Check runtime vs dev dependencies
		var runtimeCount, devCount int
		for _, dep := range deps {
			if dep.Type == "runtime" {
				runtimeCount++
			} else if dep.Type == "dev" {
				devCount++
			}
		}
		assert.Equal(t, 3, runtimeCount)
		assert.Equal(t, 3, devCount)
	})

	t.Run("DetectFramework", func(t *testing.T) {
		testCases := []struct {
			deps     map[string]string
			expected string
		}{
			{
				deps:     map[string]string{"react": "^18.0.0"},
				expected: "react",
			},
			{
				deps:     map[string]string{"vue": "^3.0.0"},
				expected: "vue",
			},
			{
				deps:     map[string]string{"@angular/core": "^16.0.0"},
				expected: "angular",
			},
			{
				deps:     map[string]string{"svelte": "^4.0.0"},
				expected: "svelte",
			},
			{
				deps:     map[string]string{"next": "^13.0.0"},
				expected: "nextjs",
			},
			{
				deps:     map[string]string{"express": "^4.18.0"},
				expected: "express",
			},
		}
		
		for _, tc := range testCases {
			t.Run(tc.expected, func(t *testing.T) {
				vfs := build.NewDirectMemoryVFS()
				
				deps := ""
				for name, version := range tc.deps {
					if deps != "" {
						deps += ",\n    "
					}
					deps += fmt.Sprintf(`"%s": "%s"`, name, version)
				}
				
				packageJSON := fmt.Sprintf(`{
  "name": "app",
  "dependencies": {
    %s
  }
}`, deps)
				
				vfs.Write("package.json", []byte(packageJSON))
				
				detected, metadata, err := plugin.DetectProject(vfs)
				require.NoError(t, err)
				assert.True(t, detected)
				assert.Equal(t, tc.expected, metadata.Type)
			})
		}
	})
}

// TestPluginHelpers tests helper functions
func TestPluginHelpers(t *testing.T) {
	t.Run("FindExecutable", func(t *testing.T) {
		// Test finding go binary
		goBin := findExecutable("go")
		if runtime.GOOS != "windows" {
			// On Unix systems, should find go if installed
			if _, err := os.Stat("/usr/bin/go"); err == nil {
				assert.NotEmpty(t, goBin)
			}
		}
		
		// Test non-existent binary
		fakeBin := findExecutable("definitely-not-a-real-binary-12345")
		assert.Empty(t, fakeBin)
	})

	t.Run("ExtractModuleName", func(t *testing.T) {
		modContent := `module github.com/example/project

go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
)`
		
		name := extractModuleName(modContent)
		assert.Equal(t, "github.com/example/project", name)
		
		// Empty module
		name = extractModuleName("")
		assert.Equal(t, "unknown", name)
	})

	t.Run("ExtractGoVersion", func(t *testing.T) {
		modContent := `module example

go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
)`
		
		version := extractGoVersion(modContent)
		assert.Equal(t, "1.21", version)
		
		// No version specified
		version = extractGoVersion("module example")
		assert.Contains(t, version, "go")
	})

	t.Run("ExtractGoErrors", func(t *testing.T) {
		output := `# example
./main.go:10:5: undefined: fmt.Printn
./main.go:15:10: cannot use x (type int) as type string
./lib.go:5:1: syntax error: unexpected }
`
		
		errors := extractGoErrors(output)
		assert.Len(t, errors, 3)
		assert.Contains(t, errors[0], "undefined")
		assert.Contains(t, errors[1], "cannot use")
		assert.Contains(t, errors[2], "syntax error")
	})

	t.Run("CheckWASMSupport", func(t *testing.T) {
		supported := checkWASMSupport()
		
		// WASM is supported in Go 1.11+
		version := runtime.Version()
		if strings.HasPrefix(version, "go1.1") || strings.HasPrefix(version, "go1.2") {
			assert.True(t, supported)
		} else if strings.HasPrefix(version, "go1.10") || strings.HasPrefix(version, "go1.9") {
			assert.False(t, supported)
		}
	})

	t.Run("HashContent", func(t *testing.T) {
		content := []byte("test content")
		hash1 := hashContent(content)
		hash2 := hashContent(content)
		
		// Same content should produce same hash
		assert.Equal(t, hash1, hash2)
		assert.Len(t, hash1, 64) // SHA256 hex = 64 chars
		
		// Different content should produce different hash
		hash3 := hashContent([]byte("different"))
		assert.NotEqual(t, hash1, hash3)
	})
}

// TestPluginBuildScenarios tests various build scenarios
func TestPluginBuildScenarios(t *testing.T) {
	t.Run("GoTestMode", func(t *testing.T) {
		plugin := NewGoPlugin()
		vfs := build.NewDirectMemoryVFS()
		
		// Create test file
		testFile := `package main

import "testing"

func TestAdd(t *testing.T) {
	result := Add(2, 3)
	if result != 5 {
		t.Errorf("Expected 5, got %d", result)
	}
}

func Add(a, b int) int {
	return a + b
}
`
		vfs.Write("main_test.go", []byte(testFile))
		vfs.Write("go.mod", []byte("module test\ngo 1.21"))
		
		config := build.BuildConfig{
			Target: "binary",
			Mode:   "test",
			PluginConfig: map[string]interface{}{
				"coverage": true,
			},
		}
		
		err := plugin.ValidateBuild(config, vfs)
		assert.NoError(t, err)
	})

	t.Run("JavaScriptProductionBuild", func(t *testing.T) {
		plugin := NewJavaScriptPlugin()
		vfs := build.NewDirectMemoryVFS()
		
		// Create production-ready project
		packageJSON := `{
  "name": "prod-app",
  "version": "1.0.0",
  "scripts": {
    "build": "webpack --mode production",
    "build:prod": "NODE_ENV=production webpack"
  },
  "devDependencies": {
    "webpack": "^5.88.0",
    "webpack-cli": "^5.1.0"
  }
}`
		vfs.Write("package.json", []byte(packageJSON))
		vfs.Write("webpack.config.js", []byte("module.exports = {}"))
		vfs.Write("src/index.js", []byte("console.log('production')"))
		
		config := build.BuildConfig{
			Target: "production",
			Mode:   "release",
		}
		
		err := plugin.ValidateBuild(config, vfs)
		assert.NoError(t, err)
	})

	t.Run("MultipleLanguagesInVFS", func(t *testing.T) {
		vfs := build.NewDirectMemoryVFS()
		
		// Mixed language project
		vfs.Write("main.go", []byte("package main\nfunc main() {}"))
		vfs.Write("go.mod", []byte("module mixed"))
		vfs.Write("frontend/app.js", []byte("console.log('frontend')"))
		vfs.Write("frontend/package.json", []byte(`{"name": "frontend"}`))
		vfs.Write("scripts/build.sh", []byte("#!/bin/bash\ngo build"))
		
		// Go plugin should detect Go files
		goPlugin := NewGoPlugin()
		detected, metadata, _ := goPlugin.DetectProject(vfs)
		assert.True(t, detected)
		assert.Equal(t, "go", metadata.Language)
		
		// JS plugin should not detect (package.json not at root)
		jsPlugin := NewJavaScriptPlugin()
		detected, _, _ = jsPlugin.DetectProject(vfs)
		assert.False(t, detected)
	})
}

// BenchmarkPluginOperations benchmarks plugin performance
func BenchmarkGoPluginDetection(b *testing.B) {
	plugin := NewGoPlugin()
	vfs := build.NewDirectMemoryVFS()
	
	// Create typical Go project
	vfs.Write("go.mod", []byte("module bench\ngo 1.21"))
	for i := 0; i < 100; i++ {
		vfs.Write(fmt.Sprintf("pkg%d/file.go", i), []byte("package pkg\nfunc F() {}"))
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plugin.DetectProject(vfs)
	}
}

func BenchmarkJavaScriptPluginDetection(b *testing.B) {
	plugin := NewJavaScriptPlugin()
	vfs := build.NewDirectMemoryVFS()
	
	// Create typical Node project
	vfs.Write("package.json", []byte(`{"name": "bench", "dependencies": {}}`))
	for i := 0; i < 100; i++ {
		vfs.Write(fmt.Sprintf("src/module%d.js", i), []byte("module.exports = {}"))
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plugin.DetectProject(vfs)
	}
}

func BenchmarkDependencyParsing(b *testing.B) {
	plugin := NewGoPlugin()
	vfs := build.NewDirectMemoryVFS()
	
	// Large go.mod with many dependencies
	modContent := `module bench
go 1.21

require (
`
	for i := 0; i < 100; i++ {
		modContent += fmt.Sprintf("\tgithub.com/example/lib%d v1.0.%d\n", i, i)
	}
	modContent += ")"
	
	vfs.Write("go.mod", []byte(modContent))
	config := build.BuildConfig{}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plugin.GetDependencies(config, vfs)
	}
}

// TestPluginIntegration tests plugin integration scenarios
func TestPluginIntegration(t *testing.T) {
	t.Run("PluginTimeout", func(t *testing.T) {
		plugin := NewGoPlugin()
		vfs := build.NewDirectMemoryVFS()
		
		vfs.Write("main.go", []byte("package main\nfunc main() {}"))
		vfs.Write("go.mod", []byte("module test"))
		
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()
		
		config := build.BuildConfig{
			Target: "binary",
			Mode:   "debug",
		}
		
		// Build should respect context timeout
		_, err := plugin.Build(ctx, config, vfs)
		if err != nil {
			// Either timeout or build not available in test
			assert.True(t, 
				strings.Contains(err.Error(), "deadline") ||
				strings.Contains(err.Error(), "not found"))
		}
	})

	t.Run("InvalidConfiguration", func(t *testing.T) {
		plugin := NewGoPlugin()
		vfs := build.NewDirectMemoryVFS()
		
		vfs.Write("main.go", []byte("package main"))
		
		config := build.BuildConfig{
			Target: "invalid-target",
			Mode:   "invalid-mode",
		}
		
		err := plugin.ValidateBuild(config, vfs)
		assert.Error(t, err)
	})

	t.Run("EmptyProject", func(t *testing.T) {
		plugin := NewGoPlugin()
		vfs := build.NewDirectMemoryVFS()
		
		// Empty VFS
		detected, _, err := plugin.DetectProject(vfs)
		assert.NoError(t, err)
		assert.False(t, detected)
		
		config := build.BuildConfig{}
		err = plugin.ValidateBuild(config, vfs)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no Go source files")
	})
}