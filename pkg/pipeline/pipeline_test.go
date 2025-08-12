package pipeline

import (
	"context"
	"testing"
	"time"
)

func TestMemoryWorkspace(t *testing.T) {
	t.Run("NewMemoryWorkspace", func(t *testing.T) {
		workspace := NewMemoryWorkspace()
		if workspace == nil {
			t.Fatal("NewMemoryWorkspace returned nil")
		}
		if workspace.files == nil {
			t.Error("files map not initialized")
		}
		if workspace.dirs == nil {
			t.Error("dirs map not initialized")
		}
	})

	t.Run("WriteFile and ReadFile", func(t *testing.T) {
		workspace := NewMemoryWorkspace()
		testData := []byte("test content")
		testPath := "test/file.txt"

		// Write file
		workspace.WriteFile(testPath, testData)

		// Read file
		data, exists := workspace.ReadFile(testPath)
		if !exists {
			t.Error("File should exist after writing")
		}
		if string(data) != string(testData) {
			t.Errorf("Read data = %s, want %s", string(data), string(testData))
		}
	})

	t.Run("ReadFile non-existent", func(t *testing.T) {
		workspace := NewMemoryWorkspace()
		_, exists := workspace.ReadFile("non-existent.txt")
		if exists {
			t.Error("Non-existent file should not exist")
		}
	})

	t.Run("ListFiles", func(t *testing.T) {
		workspace := NewMemoryWorkspace()
		
		// Add test files
		workspace.WriteFile("file1.txt", []byte("content1"))
		workspace.WriteFile("file2.txt", []byte("content2"))
		workspace.WriteFile("dir/file3.txt", []byte("content3"))

		files := workspace.ListFiles()
		if len(files) != 3 {
			t.Errorf("Expected 3 files, got %d", len(files))
		}

		expectedFiles := map[string]string{
			"file1.txt":     "content1",
			"file2.txt":     "content2",
			"dir/file3.txt": "content3",
		}

		for path, expectedContent := range expectedFiles {
			content, exists := files[path]
			if !exists {
				t.Errorf("File %s not found in list", path)
			}
			if string(content) != expectedContent {
				t.Errorf("File %s content = %s, want %s", path, string(content), expectedContent)
			}
		}
	})
}

// Mock implementations for testing
type mockSecurityScanner struct {
	scanResult *ScanResult
	scanError  error
}

func (m *mockSecurityScanner) ScanMemory(ctx context.Context, files map[string][]byte) (*ScanResult, error) {
	if m.scanError != nil {
		return nil, m.scanError
	}
	return m.scanResult, nil
}

func (m *mockSecurityScanner) ScanFile(ctx context.Context, path string, content []byte) (*FileScanResult, error) {
	return &FileScanResult{}, nil
}


type mockTestRunner struct {
	testResult *TestResult
	testError  error
}

func (m *mockTestRunner) RunMemory(ctx context.Context, sourceFiles, testFiles map[string][]byte) (*TestResult, error) {
	if m.testError != nil {
		return nil, m.testError
	}
	return m.testResult, nil
}

func (m *mockTestRunner) SupportedFrameworks() []string {
	return []string{"go", "jest", "pytest"}
}

type mockCoverageAnalyzer struct {
	coverageResult *CoverageResult
	coverageError  error
}

func (m *mockCoverageAnalyzer) AnalyzeMemory(ctx context.Context, sourceFiles map[string][]byte, testResults *TestResult) (*CoverageResult, error) {
	if m.coverageError != nil {
		return nil, m.coverageError
	}
	return m.coverageResult, nil
}

func (m *mockCoverageAnalyzer) GenerateReport(coverage *CoverageResult, format string) ([]byte, error) {
	return []byte("coverage report"), nil
}

type mockBuildEngine struct {
	buildResult *BuildResult
	buildError  error
}

func (m *mockBuildEngine) CompileMemory(ctx context.Context, sourceFiles map[string][]byte, config *BuildConfig) (*BuildResult, error) {
	if m.buildError != nil {
		return nil, m.buildError
	}
	return m.buildResult, nil
}

func (m *mockBuildEngine) SupportedLanguages() []string {
	return []string{"go", "javascript", "python"}
}

type mockDeployEngine struct {
	deployResult *DeployResult
	deployError  error
}

func (m *mockDeployEngine) Deploy(ctx context.Context, artifacts map[string][]byte, config *DeployConfig) (*DeployResult, error) {
	if m.deployError != nil {
		return nil, m.deployError
	}
	return m.deployResult, nil
}

func (m *mockDeployEngine) SupportedTargets() []string {
	return []string{"kubernetes", "docker", "aws"}
}

// Mock repository for testing
type mockRepository struct{}

func (m *mockRepository) GetFiles(patterns []string) (map[string][]byte, error) {
	return map[string][]byte{
		"main.go":      []byte("package main\nfunc main() {}"),
		"main_test.go": []byte("package main\nfunc TestMain(t *testing.T) {}"),
	}, nil
}

func (m *mockRepository) WriteFiles(path string, files map[string][]byte) error {
	return nil
}

func (m *mockRepository) Commit(message string) error {
	return nil
}

func TestNewMemoryPipeline(t *testing.T) {
	repo := &mockRepository{}
	config := &PipelineConfig{
		Name:     "test-pipeline",
		Version:  "1.0.0",
		Language: "go",
		BuildConfig: &BuildConfig{
			Language:       "go",
			SourcePatterns: []string{"*.go"},
			OutputPath:     "bin/app",
		},
	}

	pipeline := NewMemoryPipeline(repo, config)

	if pipeline == nil {
		t.Fatal("NewMemoryPipeline returned nil")
	}
	if pipeline.workspace == nil {
		t.Error("workspace not initialized")
	}
	if pipeline.config != config {
		t.Error("config not set correctly")
	}
	if pipeline.stages == nil {
		t.Error("stages slice not initialized")
	}
}

func TestPipelineExecution(t *testing.T) {
	t.Run("ExecuteFullPipeline with mocks", func(t *testing.T) {
		// Setup
		repo := &mockRepository{}
		config := &PipelineConfig{
			Name:     "test-pipeline",
			Version:  "1.0.0",
			Language: "go",
			BuildConfig: &BuildConfig{
				Language:       "go",
				SourcePatterns: []string{"*.go"},
				OutputPath:     "bin/app",
			},
		}

		pipeline := NewMemoryPipeline(repo, config)

		// Setup mock tools
		pipeline.scanner = &mockSecurityScanner{
			scanResult: &ScanResult{
				Summary: ScanSummary{
					TotalFiles:      5,
					FilesScanned:    5,
					Vulnerabilities: 0,
					QualityIssues:   2,
					Score:           0.95,
				},
			},
		}

		pipeline.tester = &mockTestRunner{
			testResult: &TestResult{
				Passed:   10,
				Failed:   0,
				Skipped:  1,
				Duration: time.Second * 5,
			},
		}

		pipeline.coverage = &mockCoverageAnalyzer{
			coverageResult: &CoverageResult{
				Overall: &CoverageStats{
					Lines:      100,
					Covered:    85,
					Percentage: 85.0,
				},
				Passed: true,
			},
		}

		pipeline.builder = &mockBuildEngine{
			buildResult: &BuildResult{
				Success:   true,
				Duration:  time.Second * 10,
				Artifacts: map[string][]byte{"bin/app": []byte("binary content")},
			},
		}

		pipeline.deployer = &mockDeployEngine{
			deployResult: &DeployResult{
				DeploymentID: "deploy-123",
				Status:       "success",
				Success:      true,
			},
		}

		// Add some test files to workspace
		pipeline.workspace.WriteFile("main.go", []byte("package main\nfunc main() {}"))
		pipeline.workspace.WriteFile("main_test.go", []byte("package main\nfunc TestMain(t *testing.T) {}"))

		// Execute pipeline
		ctx := context.Background()
		result, err := pipeline.ExecuteFullPipeline(ctx)

		// Assertions
		if err != nil {
			t.Fatalf("ExecuteFullPipeline failed: %v", err)
		}
		if result == nil {
			t.Fatal("result is nil")
		}
		if !result.Success {
			t.Error("pipeline should succeed")
		}
		if result.Duration == 0 {
			t.Error("duration should be recorded")
		}

		// Check stage results
		if result.Security == nil {
			t.Error("security scan result missing")
		}
		if result.Tests == nil {
			t.Error("test result missing")
		}
		if result.Coverage == nil {
			t.Error("coverage result missing")
		}
		if result.Build == nil {
			t.Error("build result missing")
		}
		if result.Deploy == nil {
			t.Error("deploy result missing")
		}

		// Validate cross-tool correlation
		if result.Security.Summary.Score != 0.95 {
			t.Errorf("Security score = %f, want 0.95", result.Security.Summary.Score)
		}
		if result.Tests.Passed != 10 {
			t.Errorf("Test passed = %d, want 10", result.Tests.Passed)
		}
		if result.Coverage.Overall.Percentage != 85.0 {
			t.Errorf("Coverage = %f, want 85.0", result.Coverage.Overall.Percentage)
		}
		if !result.Build.Success {
			t.Error("Build should succeed")
		}
		if !result.Deploy.Success {
			t.Error("Deploy should succeed")
		}
	})

	t.Run("Pipeline failure handling", func(t *testing.T) {
		repo := &mockRepository{}
		config := &PipelineConfig{
			Name:     "failing-pipeline",
			Version:  "1.0.0",
			Language: "go",
			BuildConfig: &BuildConfig{
				Language:       "go",
				SourcePatterns: []string{"*.go"},
			},
		}

		pipeline := NewMemoryPipeline(repo, config)

		// Setup failing build
		pipeline.builder = &mockBuildEngine{
			buildResult: &BuildResult{
				Success:  false,
				Duration: time.Second * 5,
				Warnings: []BuildWarning{
					{
						File:     "main.go",
						Line:     10,
						Message:  "unused variable",
						Severity: "warning",
					},
				},
			},
		}

		// Deploy should not be attempted if build fails
		pipeline.deployer = &mockDeployEngine{
			deployResult: &DeployResult{
				Success: true,
			},
		}

		ctx := context.Background()
		result, err := pipeline.ExecuteFullPipeline(ctx)

		if err != nil {
			t.Fatalf("ExecuteFullPipeline failed: %v", err)
		}
		if result.Build == nil || result.Build.Success {
			t.Error("Build should fail")
		}
		if result.Deploy != nil {
			t.Error("Deploy should not run when build fails")
		}
	})
}

func TestStageResult(t *testing.T) {
	result := &StageResult{
		Success:   true,
		Output:    []byte("stage output"),
		Artifacts: map[string][]byte{"artifact1": []byte("content")},
		Metadata:  map[string]interface{}{"key": "value"},
		Duration:  time.Millisecond * 500,
	}

	if !result.Success {
		t.Error("Success should be true")
	}
	if string(result.Output) != "stage output" {
		t.Error("Output not set correctly")
	}
	if len(result.Artifacts) != 1 {
		t.Error("Artifacts not set correctly")
	}
	if result.Metadata["key"] != "value" {
		t.Error("Metadata not set correctly")
	}
	if result.Duration != time.Millisecond*500 {
		t.Error("Duration not set correctly")
	}
}

func TestPipelineConfig(t *testing.T) {
	config := &PipelineConfig{
		Name:     "test-config",
		Version:  "2.0.0",
		Language: "javascript",
		BuildConfig: &BuildConfig{
			Language:       "javascript",
			SourcePatterns: []string{"src/**/*.js"},
			OutputPath:     "dist/bundle.js",
			Targets:        []string{"node", "browser"},
		},
		TestConfig: &TestConfig{
			Framework:    "jest",
			TestPatterns: []string{"test/**/*.test.js"},
			Timeout:      time.Minute * 10,
			Parallel:     true,
		},
		ScanConfig: &ScanConfig{
			SecurityRules: []string{"security/*"},
			QualityRules:  []string{"quality/*"},
			IgnoreFiles:   []string{"**/node_modules/**"},
		},
		DeployConfig: &DeployConfig{
			Target:      "kubernetes",
			Environment: "staging",
			Variables:   map[string]string{"NODE_ENV": "production"},
		},
	}

	if config.Name != "test-config" {
		t.Error("Name not set correctly")
	}
	if config.Language != "javascript" {
		t.Error("Language not set correctly")
	}
	if config.BuildConfig.OutputPath != "dist/bundle.js" {
		t.Error("BuildConfig not set correctly")
	}
	if config.TestConfig.Framework != "jest" {
		t.Error("TestConfig not set correctly")
	}
	if len(config.ScanConfig.SecurityRules) != 1 {
		t.Error("ScanConfig not set correctly")
	}
	if config.DeployConfig.Target != "kubernetes" {
		t.Error("DeployConfig not set correctly")
	}
}

// Benchmark tests for memory performance
func BenchmarkMemoryWorkspace(b *testing.B) {
	workspace := NewMemoryWorkspace()
	testData := make([]byte, 1024) // 1KB test data

	b.Run("WriteFile", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			workspace.WriteFile("file", testData)
		}
	})

	b.Run("ReadFile", func(b *testing.B) {
		workspace.WriteFile("file", testData)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = workspace.ReadFile("file")
		}
	})

	b.Run("ListFiles", func(b *testing.B) {
		// Add 100 files
		for i := 0; i < 100; i++ {
			workspace.WriteFile(string(rune('a'+i)), testData)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = workspace.ListFiles()
		}
	})
}

// Test memory efficiency with large files
func TestMemoryEfficiency(t *testing.T) {
	workspace := NewMemoryWorkspace()
	
	// Test with 1MB file
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	workspace.WriteFile("large_file.bin", largeData)
	
	retrieved, exists := workspace.ReadFile("large_file.bin")
	if !exists {
		t.Error("Large file should exist")
	}
	if len(retrieved) != len(largeData) {
		t.Errorf("Large file size mismatch: got %d, want %d", len(retrieved), len(largeData))
	}

	// Verify data integrity
	for i := 0; i < 1000; i++ { // Check first 1000 bytes
		if retrieved[i] != largeData[i] {
			t.Errorf("Data corruption at byte %d: got %d, want %d", i, retrieved[i], largeData[i])
			break
		}
	}
}