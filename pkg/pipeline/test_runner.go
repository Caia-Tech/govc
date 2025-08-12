package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// NativeTestRunner implements TestRunner with memory-first test execution
type NativeTestRunner struct {
	config       *TestConfig
	frameworks   map[string]TestFramework
	language     string
	tempDir      string
	workspace    *MemoryWorkspace
}

// TestFramework defines how to execute tests for a specific framework
type TestFramework interface {
	Name() string
	DetectTests(files map[string][]byte) []TestFile
	ExecuteTests(ctx context.Context, sourceFiles, testFiles map[string][]byte, config *TestConfig) (*TestResult, error)
	ParseOutput(output []byte) (*TestResult, error)
}

// TestFile represents a test file with its metadata
type TestFile struct {
	Path      string
	Framework string
	TestCases []TestCaseInfo
}

// TestCaseInfo contains metadata about a test case
type TestCaseInfo struct {
	Name        string
	Line        int
	Type        string // unit, integration, benchmark
	Timeout     time.Duration
	Skip        bool
	SkipReason  string
}

// NewNativeTestRunner creates a new memory-first test runner
func NewNativeTestRunner(language string, config *TestConfig) *NativeTestRunner {
	runner := &NativeTestRunner{
		config:     config,
		frameworks: make(map[string]TestFramework),
		language:   language,
		workspace:  NewMemoryWorkspace(),
	}
	
	// Initialize frameworks based on language
	runner.initializeFrameworks()
	
	return runner
}

// RunMemory executes tests on memory-resident files with shared data structures
func (ntr *NativeTestRunner) RunMemory(ctx context.Context, sourceFiles, testFiles map[string][]byte) (*TestResult, error) {
	startTime := time.Now()
	
	// Detect test frameworks and files
	detectedTests := ntr.detectTestFrameworks(testFiles)
	if len(detectedTests) == 0 {
		return &TestResult{
			Tests:    []TestCase{},
			Passed:   0,
			Failed:   0,
			Skipped:  0,
			Duration: time.Since(startTime),
		}, nil
	}
	
	// Create temporary workspace for test execution
	tempDir, err := ntr.createTempWorkspace(sourceFiles, testFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp workspace: %w", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Update config to use temporary directory
	if ntr.config.WorkDir == "" {
		ntr.config.WorkDir = tempDir
	}
	
	var allResults []*TestResult
	
	// Execute tests for each detected framework
	for frameworkName, framework := range ntr.frameworks {
		if ntr.hasFrameworkTests(detectedTests, frameworkName) {
			result, err := framework.ExecuteTests(ctx, sourceFiles, testFiles, ntr.config)
			if err != nil {
				return nil, fmt.Errorf("failed to execute %s tests: %w", frameworkName, err)
			}
			allResults = append(allResults, result)
		}
	}
	
	// Aggregate results from all frameworks
	aggregated := ntr.aggregateResults(allResults)
	aggregated.Duration = time.Since(startTime)
	
	return aggregated, nil
}

// SupportedFrameworks returns list of supported test frameworks
func (ntr *NativeTestRunner) SupportedFrameworks() []string {
	frameworks := make([]string, 0, len(ntr.frameworks))
	for name := range ntr.frameworks {
		frameworks = append(frameworks, name)
	}
	return frameworks
}

// initializeFrameworks sets up framework-specific test runners
func (ntr *NativeTestRunner) initializeFrameworks() {
	switch ntr.language {
	case "go":
		ntr.frameworks["go-test"] = &GoTestFramework{}
	case "javascript", "nodejs":
		ntr.frameworks["jest"] = &JestFramework{}
		ntr.frameworks["mocha"] = &MochaFramework{}
	case "python":
		ntr.frameworks["pytest"] = &PytestFramework{}
		ntr.frameworks["unittest"] = &UnittestFramework{}
	}
}

// detectTestFrameworks analyzes test files to determine frameworks in use
func (ntr *NativeTestRunner) detectTestFrameworks(testFiles map[string][]byte) map[string][]TestFile {
	detected := make(map[string][]TestFile)
	
	for frameworkName, framework := range ntr.frameworks {
		testFiles := framework.DetectTests(testFiles)
		if len(testFiles) > 0 {
			detected[frameworkName] = testFiles
		}
	}
	
	return detected
}

// hasFrameworkTests checks if any tests were detected for a framework
func (ntr *NativeTestRunner) hasFrameworkTests(detected map[string][]TestFile, framework string) bool {
	files, exists := detected[framework]
	return exists && len(files) > 0
}

// createTempWorkspace creates temporary files for test execution
func (ntr *NativeTestRunner) createTempWorkspace(sourceFiles, testFiles map[string][]byte) (string, error) {
	tempDir, err := os.MkdirTemp("", "govc-test-*")
	if err != nil {
		return "", err
	}
	
	// Write all files to temporary directory
	allFiles := make(map[string][]byte)
	for path, content := range sourceFiles {
		allFiles[path] = content
	}
	for path, content := range testFiles {
		allFiles[path] = content
	}
	
	for path, content := range allFiles {
		fullPath := filepath.Join(tempDir, path)
		dir := filepath.Dir(fullPath)
		
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
		
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			return "", err
		}
	}
	
	ntr.tempDir = tempDir
	return tempDir, nil
}

// aggregateResults combines results from multiple test frameworks
func (ntr *NativeTestRunner) aggregateResults(results []*TestResult) *TestResult {
	if len(results) == 0 {
		return &TestResult{
			Tests:   []TestCase{},
			Passed:  0,
			Failed:  0,
			Skipped: 0,
		}
	}
	
	if len(results) == 1 {
		return results[0]
	}
	
	aggregated := &TestResult{
		Tests:   []TestCase{},
		Passed:  0,
		Failed:  0,
		Skipped: 0,
	}
	
	for _, result := range results {
		aggregated.Tests = append(aggregated.Tests, result.Tests...)
		aggregated.Passed += result.Passed
		aggregated.Failed += result.Failed
		aggregated.Skipped += result.Skipped
		if result.Duration > aggregated.Duration {
			aggregated.Duration = result.Duration
		}
	}
	
	return aggregated
}

// GetTestStats returns statistics about test execution capabilities
func (ntr *NativeTestRunner) GetTestStats() *TestStats {
	return &TestStats{
		SupportedFrameworks: len(ntr.frameworks),
		TempDirPath:        ntr.tempDir,
	}
}

// TestStats provides statistics about the test runner
type TestStats struct {
	SupportedFrameworks int
	TempDirPath        string
	LastRunDuration    time.Duration
	TotalTestsRun      int
}

// GoTestFramework implements Go's testing framework
type GoTestFramework struct{}

func (gtf *GoTestFramework) Name() string {
	return "go-test"
}

func (gtf *GoTestFramework) DetectTests(files map[string][]byte) []TestFile {
	var testFiles []TestFile
	
	for path, content := range files {
		if strings.HasSuffix(path, "_test.go") {
			testCases := gtf.parseGoTestCases(string(content))
			testFiles = append(testFiles, TestFile{
				Path:      path,
				Framework: "go-test",
				TestCases: testCases,
			})
		}
	}
	
	return testFiles
}

func (gtf *GoTestFramework) ExecuteTests(ctx context.Context, sourceFiles, testFiles map[string][]byte, config *TestConfig) (*TestResult, error) {
	// Create a context with timeout
	testCtx := ctx
	if config.Timeout > 0 {
		var cancel context.CancelFunc
		testCtx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}
	
	// Create cache directory
	cacheDir := filepath.Join(config.WorkDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	
	// Execute go test command
	cmd := exec.CommandContext(testCtx, "go", "test", "-v", "./...")
	cmd.Dir = config.WorkDir
	
	// Set up proper environment for isolated testing
	cmd.Env = append(os.Environ(),
		"GOPROXY=direct", 
		"GOSUMDB=off",
		fmt.Sprintf("GOCACHE=%s", cacheDir),
	)
	
	output, err := cmd.CombinedOutput() // Use CombinedOutput to get both stdout and stderr
	if err != nil {
		// Log the error for debugging but continue parsing output
		fmt.Printf("Go test execution error (may be expected): %v\nOutput: %s\n", err, string(output))
	}
	
	return gtf.ParseOutput(output)
}

func (gtf *GoTestFramework) ParseOutput(output []byte) (*TestResult, error) {
	result := &TestResult{
		Tests:   []TestCase{},
		Passed:  0,
		Failed:  0,
		Skipped: 0,
	}
	
	lines := strings.Split(string(output), "\n")
	
	// Match both === RUN and --- PASS/FAIL/SKIP patterns
	runRegex := regexp.MustCompile(`^=== RUN\s+(.+)`)
	resultRegex := regexp.MustCompile(`^--- (PASS|FAIL|SKIP):\s+(.+)\s+\(`)
	
	runningTests := make(map[string]bool)
	
	for _, line := range lines {
		// Track running tests
		if matches := runRegex.FindStringSubmatch(line); matches != nil {
			testName := matches[1]
			runningTests[testName] = true
		}
		
		// Process test results
		if matches := resultRegex.FindStringSubmatch(line); matches != nil {
			status := matches[1]
			testName := matches[2]
			
			testCase := TestCase{
				Name:   testName,
			}
			
			switch status {
			case "PASS":
				result.Passed++
				testCase.Status = "passed"
			case "FAIL":
				result.Failed++
				testCase.Status = "failed"
			case "SKIP":
				result.Skipped++
				testCase.Status = "skipped"
			}
			
			result.Tests = append(result.Tests, testCase)
		}
	}
	
	return result, nil
}

func (gtf *GoTestFramework) parseGoTestCases(content string) []TestCaseInfo {
	var testCases []TestCaseInfo
	
	lines := strings.Split(content, "\n")
	testRegex := regexp.MustCompile(`^func\s+(Test\w+|Benchmark\w+|Example\w+)\s*\(`)
	
	for i, line := range lines {
		if matches := testRegex.FindStringSubmatch(line); matches != nil {
			testName := matches[1]
			testType := "unit"
			
			if strings.HasPrefix(testName, "Benchmark") {
				testType = "benchmark"
			} else if strings.HasPrefix(testName, "Example") {
				testType = "example"
			}
			
			testCases = append(testCases, TestCaseInfo{
				Name: testName,
				Line: i + 1,
				Type: testType,
			})
		}
	}
	
	return testCases
}

// JestFramework implements Jest testing framework for JavaScript
type JestFramework struct{}

func (jf *JestFramework) Name() string {
	return "jest"
}

func (jf *JestFramework) DetectTests(files map[string][]byte) []TestFile {
	var testFiles []TestFile
	
	for path, content := range files {
		if jf.isJestTestFile(path) {
			testCases := jf.parseJestTestCases(string(content))
			testFiles = append(testFiles, TestFile{
				Path:      path,
				Framework: "jest",
				TestCases: testCases,
			})
		}
	}
	
	return testFiles
}

func (jf *JestFramework) ExecuteTests(ctx context.Context, sourceFiles, testFiles map[string][]byte, config *TestConfig) (*TestResult, error) {
	// Jest execution would be implemented here
	// For now, return a placeholder result
	return &TestResult{
		Tests:   []TestCase{},
		Passed:  0,
		Failed:  0,
		Skipped: 0,
	}, nil
}

func (jf *JestFramework) ParseOutput(output []byte) (*TestResult, error) {
	// Jest output parsing would be implemented here
	return &TestResult{
		Tests:   []TestCase{},
		Passed:  0,
		Failed:  0,
		Skipped: 0,
	}, nil
}

func (jf *JestFramework) isJestTestFile(path string) bool {
	return strings.Contains(path, ".test.js") || 
		   strings.Contains(path, ".spec.js") ||
		   strings.Contains(path, "__tests__/")
}

func (jf *JestFramework) parseJestTestCases(content string) []TestCaseInfo {
	var testCases []TestCaseInfo
	
	lines := strings.Split(content, "\n")
	testRegex := regexp.MustCompile(`(test|it|describe)\s*\(\s*['"]([^'"]+)['"]`)
	
	for i, line := range lines {
		if matches := testRegex.FindStringSubmatch(line); matches != nil {
			testType := matches[1]
			testName := matches[2]
			
			caseType := "unit"
			if testType == "describe" {
				caseType = "suite"
			}
			
			testCases = append(testCases, TestCaseInfo{
				Name: testName,
				Line: i + 1,
				Type: caseType,
			})
		}
	}
	
	return testCases
}

// MochaFramework implements Mocha testing framework
type MochaFramework struct{}

func (mf *MochaFramework) Name() string {
	return "mocha"
}

func (mf *MochaFramework) DetectTests(files map[string][]byte) []TestFile {
	// Similar to Jest but with Mocha-specific detection
	return []TestFile{}
}

func (mf *MochaFramework) ExecuteTests(ctx context.Context, sourceFiles, testFiles map[string][]byte, config *TestConfig) (*TestResult, error) {
	return &TestResult{}, nil
}

func (mf *MochaFramework) ParseOutput(output []byte) (*TestResult, error) {
	return &TestResult{}, nil
}

// PytestFramework implements pytest for Python
type PytestFramework struct{}

func (pf *PytestFramework) Name() string {
	return "pytest"
}

func (pf *PytestFramework) DetectTests(files map[string][]byte) []TestFile {
	var testFiles []TestFile
	
	for path, content := range files {
		if strings.HasPrefix(filepath.Base(path), "test_") || 
		   strings.HasSuffix(path, "_test.py") {
			testCases := pf.parsePytestCases(string(content))
			testFiles = append(testFiles, TestFile{
				Path:      path,
				Framework: "pytest",
				TestCases: testCases,
			})
		}
	}
	
	return testFiles
}

func (pf *PytestFramework) ExecuteTests(ctx context.Context, sourceFiles, testFiles map[string][]byte, config *TestConfig) (*TestResult, error) {
	return &TestResult{}, nil
}

func (pf *PytestFramework) ParseOutput(output []byte) (*TestResult, error) {
	return &TestResult{}, nil
}

func (pf *PytestFramework) parsePytestCases(content string) []TestCaseInfo {
	var testCases []TestCaseInfo
	
	lines := strings.Split(content, "\n")
	testRegex := regexp.MustCompile(`^def\s+(test_\w+)\s*\(`)
	
	for i, line := range lines {
		if matches := testRegex.FindStringSubmatch(line); matches != nil {
			testName := matches[1]
			
			testCases = append(testCases, TestCaseInfo{
				Name: testName,
				Line: i + 1,
				Type: "unit",
			})
		}
	}
	
	return testCases
}

// UnittestFramework implements Python's unittest module
type UnittestFramework struct{}

func (uf *UnittestFramework) Name() string {
	return "unittest"
}

func (uf *UnittestFramework) DetectTests(files map[string][]byte) []TestFile {
	return []TestFile{}
}

func (uf *UnittestFramework) ExecuteTests(ctx context.Context, sourceFiles, testFiles map[string][]byte, config *TestConfig) (*TestResult, error) {
	return &TestResult{}, nil
}

func (uf *UnittestFramework) ParseOutput(output []byte) (*TestResult, error) {
	return &TestResult{}, nil
}

// Enhanced test execution with memory optimization
type MemoryOptimizedRunner struct {
	*NativeTestRunner
	memoryPool *MemoryPool
	stats      *ExecutionStats
}

// MemoryPool for efficient buffer reuse
type MemoryPool struct {
	buffers chan []byte
}

// ExecutionStats tracks test runner performance
type ExecutionStats struct {
	TotalRuns       int
	TotalDuration   time.Duration
	AverageRunTime  time.Duration
	MemoryUsage     int64
	BufferHits      int
	BufferMisses    int
}

// NewMemoryOptimizedRunner creates a test runner with memory optimizations
func NewMemoryOptimizedRunner(language string, config *TestConfig) *MemoryOptimizedRunner {
	return &MemoryOptimizedRunner{
		NativeTestRunner: NewNativeTestRunner(language, config),
		memoryPool:       newMemoryPool(10), // Pool of 10 buffers
		stats:           &ExecutionStats{},
	}
}

func newMemoryPool(size int) *MemoryPool {
	return &MemoryPool{
		buffers: make(chan []byte, size),
	}
}

func (mp *MemoryPool) Get(size int) []byte {
	select {
	case buf := <-mp.buffers:
		if cap(buf) >= size {
			return buf[:size]
		}
		// Buffer too small, return to pool and create new one
		mp.buffers <- buf
	default:
	}
	return make([]byte, size)
}

func (mp *MemoryPool) Put(buf []byte) {
	select {
	case mp.buffers <- buf:
	default:
		// Pool full, let GC handle it
	}
}