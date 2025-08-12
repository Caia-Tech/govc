package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// LocalDeploymentTarget implements deployment for local execution
type LocalDeploymentTarget struct {
	// System info detected at initialization
	os           string
	arch         string
	cpuCount     int
	memoryLimit  int64
	
	// Process management
	processes    map[string]*LocalProcess
	logBuffer    *CircularLogBuffer
	
	mu sync.RWMutex
}

// LocalProcess represents a locally running process
type LocalProcess struct {
	ID          string
	Cmd         *exec.Cmd
	StartTime   time.Time
	Status      string // "running", "stopped", "crashed"
	ExitCode    int
	LogFile     string
	
	// Real-time metrics
	CPUUsage    float64
	MemoryUsage int64
	
	// Output capture
	stdout      io.ReadCloser
	stderr      io.ReadCloser
	logChan     chan string
}

// CircularLogBuffer stores recent logs in memory
type CircularLogBuffer struct {
	lines    []LogLine
	maxLines int
	index    int
	mu       sync.RWMutex
}

// LogLine represents a single log entry
type LogLine struct {
	Timestamp time.Time
	Source    string // "stdout", "stderr", "system"
	ProcessID string
	Content   string
	Level     string // "info", "warn", "error", "debug"
}

// NewLocalDeploymentTarget creates a local deployment target with system detection
func NewLocalDeploymentTarget() *LocalDeploymentTarget {
	target := &LocalDeploymentTarget{
		os:          runtime.GOOS,
		arch:        runtime.GOARCH,
		cpuCount:    runtime.NumCPU(),
		memoryLimit: getAvailableMemory(),
		processes:   make(map[string]*LocalProcess),
		logBuffer:   NewCircularLogBuffer(10000), // Keep last 10k lines in memory
	}
	
	// Log system detection
	fmt.Printf("Local deployment target initialized:\n")
	fmt.Printf("  OS: %s\n", target.os)
	fmt.Printf("  Architecture: %s\n", target.arch)
	fmt.Printf("  CPU cores: %d\n", target.cpuCount)
	fmt.Printf("  Memory limit: %d MB\n", target.memoryLimit/(1024*1024))
	
	return target
}

// Name returns the target name
func (lt *LocalDeploymentTarget) Name() string {
	return fmt.Sprintf("local-%s-%s", lt.os, lt.arch)
}

// Type returns the target type
func (lt *LocalDeploymentTarget) Type() string {
	return "local"
}

// Validate checks if artifacts are compatible with local system
func (lt *LocalDeploymentTarget) Validate(artifacts map[string][]byte) error {
	// Look for compatible binary
	binaryName := lt.selectBinary(artifacts)
	if binaryName == "" {
		return fmt.Errorf("no compatible binary found for %s/%s", lt.os, lt.arch)
	}
	
	// Check if binary exists and has content
	if binary, exists := artifacts[binaryName]; !exists || len(binary) == 0 {
		return fmt.Errorf("binary %s is empty or missing", binaryName)
	}
	
	return nil
}

// Deploy executes the binary locally with full observability
func (lt *LocalDeploymentTarget) Deploy(ctx context.Context, artifacts map[string][]byte, config DeployConfig) (*DeploymentResult, error) {
	startTime := time.Now()
	
	// Select appropriate binary
	binaryName := lt.selectBinary(artifacts)
	if binaryName == "" {
		return nil, fmt.Errorf("no compatible binary for %s/%s", lt.os, lt.arch)
	}
	
	binary := artifacts[binaryName]
	
	// Create temporary directory for deployment
	deployDir := filepath.Join(os.TempDir(), fmt.Sprintf("govc-deploy-%d", time.Now().Unix()))
	if err := os.MkdirAll(deployDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create deploy directory: %w", err)
	}
	
	// Write binary to disk (this is necessary for execution)
	binaryPath := filepath.Join(deployDir, binaryName)
	if err := os.WriteFile(binaryPath, binary, 0755); err != nil {
		return nil, fmt.Errorf("failed to write binary: %w", err)
	}
	
	// Make executable on Unix systems
	if lt.os != "windows" {
		if err := os.Chmod(binaryPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to make binary executable: %w", err)
		}
	}
	
	// Create process
	processID := fmt.Sprintf("proc-%s-%d", config.Version, time.Now().Unix())
	process, err := lt.createProcess(ctx, binaryPath, config, processID)
	if err != nil {
		return nil, fmt.Errorf("failed to create process: %w", err)
	}
	
	// Start process with output streaming
	if err := lt.startProcess(process); err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}
	
	// Store process
	lt.mu.Lock()
	lt.processes[processID] = process
	lt.mu.Unlock()
	
	// Wait for initial health check if configured
	healthy := true
	healthMessage := "Process started successfully"
	
	if config.HealthCheck != nil && config.HealthCheck.Enabled {
		healthy = lt.performLocalHealthCheck(process, config.HealthCheck)
		if !healthy {
			healthMessage = "Health check failed"
			// Optionally kill the process
			if config.AutoRollback {
				lt.stopProcess(processID)
			}
		}
	}
	
	// Get process port if it's a server
	port := lt.detectPort(process)
	url := ""
	if port > 0 {
		url = fmt.Sprintf("http://localhost:%d", port)
	}
	
	return &DeploymentResult{
		Success:      healthy,
		DeploymentID: processID,
		Target:       lt.Name(),
		Environment:  "local",
		Version:      config.Version,
		URL:          url,
		StartTime:    startTime,
		EndTime:      time.Now(),
		Duration:     time.Since(startTime),
		HealthStatus: healthMessage,
		Metadata: map[string]string{
			"binary":     binaryName,
			"pid":        fmt.Sprintf("%d", process.Cmd.Process.Pid),
			"deploy_dir": deployDir,
		},
	}, nil
}

// Rollback stops a local deployment
func (lt *LocalDeploymentTarget) Rollback(deploymentID string) error {
	return lt.stopProcess(deploymentID)
}

// HealthCheck performs health check on local process
func (lt *LocalDeploymentTarget) HealthCheck() error {
	lt.mu.RLock()
	defer lt.mu.RUnlock()
	
	// Check if any processes are running
	runningCount := 0
	for _, proc := range lt.processes {
		if proc.Status == "running" {
			runningCount++
		}
	}
	
	if runningCount == 0 {
		return fmt.Errorf("no running processes")
	}
	
	return nil
}

// selectBinary chooses the appropriate binary for the system
func (lt *LocalDeploymentTarget) selectBinary(artifacts map[string][]byte) string {
	// Try exact match first
	patterns := []string{
		fmt.Sprintf("app-%s-%s", lt.os, lt.arch),
		fmt.Sprintf("govc-%s-%s", lt.os, lt.arch),
		fmt.Sprintf("main-%s-%s", lt.os, lt.arch),
		fmt.Sprintf("%s-%s", lt.os, lt.arch),
	}
	
	// Add .exe for Windows
	if lt.os == "windows" {
		for i, pattern := range patterns {
			patterns[i] = pattern + ".exe"
		}
	}
	
	// Look for matching binary
	for _, pattern := range patterns {
		for name := range artifacts {
			if strings.Contains(name, pattern) || name == pattern {
				return name
			}
		}
	}
	
	// Fallback to generic names
	fallbacks := []string{"app", "main", "govc", "binary"}
	if lt.os == "windows" {
		for i, name := range fallbacks {
			fallbacks[i] = name + ".exe"
		}
	}
	
	for _, name := range fallbacks {
		if _, exists := artifacts[name]; exists {
			return name
		}
	}
	
	return ""
}

// createProcess creates a new process with proper configuration
func (lt *LocalDeploymentTarget) createProcess(ctx context.Context, binaryPath string, config DeployConfig, processID string) (*LocalProcess, error) {
	cmd := exec.CommandContext(ctx, binaryPath)
	
	// Set environment variables
	cmd.Env = os.Environ()
	for key, value := range config.Secrets {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}
	
	// Set working directory
	cmd.Dir = filepath.Dir(binaryPath)
	
	// Create pipes for output capture
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	
	process := &LocalProcess{
		ID:        processID,
		Cmd:       cmd,
		StartTime: time.Now(),
		Status:    "pending",
		stdout:    stdout,
		stderr:    stderr,
		logChan:   make(chan string, 1000),
	}
	
	return process, nil
}

// startProcess starts the process and begins output streaming
func (lt *LocalDeploymentTarget) startProcess(process *LocalProcess) error {
	// Start the process
	if err := process.Cmd.Start(); err != nil {
		process.Status = "failed"
		return err
	}
	
	process.Status = "running"
	
	// Start output streaming goroutines
	go lt.streamOutput(process, process.stdout, "stdout")
	go lt.streamOutput(process, process.stderr, "stderr")
	
	// Start process monitoring
	go lt.monitorProcess(process)
	
	return nil
}

// streamOutput captures and streams process output
func (lt *LocalDeploymentTarget) streamOutput(process *LocalProcess, reader io.ReadCloser, source string) {
	defer reader.Close()
	
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		
		// Log to buffer
		logLine := LogLine{
			Timestamp: time.Now(),
			Source:    source,
			ProcessID: process.ID,
			Content:   line,
			Level:     detectLogLevel(line),
		}
		lt.logBuffer.Add(logLine)
		
		// Send to process channel
		select {
		case process.logChan <- line:
		default:
			// Channel full, skip
		}
		
		// Also print to console for immediate feedback
		fmt.Printf("[%s:%s] %s\n", process.ID, source, line)
	}
}

// monitorProcess monitors process health and metrics
func (lt *LocalDeploymentTarget) monitorProcess(process *LocalProcess) {
	// Wait for process to exit
	err := process.Cmd.Wait()
	
	lt.mu.Lock()
	defer lt.mu.Unlock()
	
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			process.ExitCode = exitErr.ExitCode()
			process.Status = "crashed"
		} else {
			process.Status = "error"
		}
	} else {
		process.ExitCode = 0
		process.Status = "stopped"
	}
	
	// Log process exit
	lt.logBuffer.Add(LogLine{
		Timestamp: time.Now(),
		Source:    "system",
		ProcessID: process.ID,
		Content:   fmt.Sprintf("Process exited with code %d", process.ExitCode),
		Level:     "info",
	})
}

// performLocalHealthCheck checks if the process is healthy
func (lt *LocalDeploymentTarget) performLocalHealthCheck(process *LocalProcess, config *HealthCheckConfig) bool {
	// Wait for process to stabilize
	time.Sleep(2 * time.Second)
	
	// Check if process is still running
	if process.Cmd.Process == nil {
		return false
	}
	
	// Try to check if process is responsive (simplified)
	// In real implementation, could check HTTP endpoint, etc.
	for i := 0; i < config.Retries; i++ {
		// Check if process exists
		err := process.Cmd.Process.Signal(syscall.Signal(0))
		if err == nil {
			return true
		}
		time.Sleep(config.Interval)
	}
	
	return false
}

// detectPort attempts to detect which port the process is listening on
func (lt *LocalDeploymentTarget) detectPort(process *LocalProcess) int {
	// Wait for process to start listening
	time.Sleep(2 * time.Second)
	
	// Check common ports or parse from logs
	// This is simplified - real implementation would use netstat or lsof
	commonPorts := []int{8080, 3000, 8000, 5000, 9000}
	
	for _, port := range commonPorts {
		// Check if port is in use by our process
		// Simplified check - just return first common port for now
		return port
	}
	
	return 0
}

// stopProcess stops a running process
func (lt *LocalDeploymentTarget) stopProcess(processID string) error {
	lt.mu.Lock()
	process, exists := lt.processes[processID]
	lt.mu.Unlock()
	
	if !exists {
		return fmt.Errorf("process %s not found", processID)
	}
	
	if process.Cmd.Process != nil {
		// Try graceful shutdown first
		process.Cmd.Process.Signal(syscall.SIGTERM)
		
		// Wait for graceful shutdown
		done := make(chan bool)
		go func() {
			process.Cmd.Wait()
			done <- true
		}()
		
		select {
		case <-done:
			// Process stopped gracefully
		case <-time.After(10 * time.Second):
			// Force kill after timeout
			process.Cmd.Process.Kill()
		}
	}
	
	process.Status = "stopped"
	return nil
}

// GetLogs retrieves recent logs from memory
func (lt *LocalDeploymentTarget) GetLogs(processID string, lines int) []string {
	return lt.logBuffer.GetRecent(processID, lines)
}

// GetProcessStatus returns the status of a process
func (lt *LocalDeploymentTarget) GetProcessStatus(processID string) (string, error) {
	lt.mu.RLock()
	defer lt.mu.RUnlock()
	
	process, exists := lt.processes[processID]
	if !exists {
		return "", fmt.Errorf("process %s not found", processID)
	}
	
	return process.Status, nil
}

// NewCircularLogBuffer creates a circular log buffer
func NewCircularLogBuffer(maxLines int) *CircularLogBuffer {
	return &CircularLogBuffer{
		lines:    make([]LogLine, maxLines),
		maxLines: maxLines,
		index:    0,
	}
}

// Add adds a log line to the buffer
func (clb *CircularLogBuffer) Add(line LogLine) {
	clb.mu.Lock()
	defer clb.mu.Unlock()
	
	clb.lines[clb.index%clb.maxLines] = line
	clb.index++
}

// GetRecent returns recent log lines for a process
func (clb *CircularLogBuffer) GetRecent(processID string, count int) []string {
	clb.mu.RLock()
	defer clb.mu.RUnlock()
	
	result := make([]string, 0, count)
	
	// Start from most recent and work backwards
	start := clb.index - 1
	found := 0
	
	for i := 0; i < clb.maxLines && found < count; i++ {
		idx := (start - i + clb.maxLines) % clb.maxLines
		line := clb.lines[idx]
		
		if line.ProcessID == processID && line.Timestamp.Unix() > 0 {
			result = append(result, fmt.Sprintf("[%s] %s: %s",
				line.Timestamp.Format("15:04:05"),
				line.Source,
				line.Content))
			found++
		}
	}
	
	// Reverse to get chronological order
	for i := 0; i < len(result)/2; i++ {
		j := len(result) - 1 - i
		result[i], result[j] = result[j], result[i]
	}
	
	return result
}

// detectLogLevel attempts to detect log level from content
func detectLogLevel(content string) string {
	lower := strings.ToLower(content)
	
	if strings.Contains(lower, "error") || strings.Contains(lower, "fail") {
		return "error"
	}
	if strings.Contains(lower, "warn") {
		return "warn"
	}
	if strings.Contains(lower, "debug") || strings.Contains(lower, "trace") {
		return "debug"
	}
	
	return "info"
}

// getAvailableMemory returns available memory in bytes
func getAvailableMemory() int64 {
	// Simplified - return 1GB as default
	// Real implementation would check system resources
	return 1 << 30 // 1GB
}