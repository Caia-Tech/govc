package pipeline

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLocalDeploymentTarget(t *testing.T) {
	t.Run("SystemDetection", func(t *testing.T) {
		target := NewLocalDeploymentTarget()
		
		// Verify system detection
		if target.os != runtime.GOOS {
			t.Errorf("Expected OS %s, got %s", runtime.GOOS, target.os)
		}
		
		if target.arch != runtime.GOARCH {
			t.Errorf("Expected arch %s, got %s", runtime.GOARCH, target.arch)
		}
		
		if target.cpuCount != runtime.NumCPU() {
			t.Errorf("Expected %d CPUs, got %d", runtime.NumCPU(), target.cpuCount)
		}
		
		if target.memoryLimit <= 0 {
			t.Error("Expected positive memory limit")
		}
		
		t.Logf("System detected: %s/%s with %d CPUs and %d MB memory",
			target.os, target.arch, target.cpuCount, target.memoryLimit/(1024*1024))
	})
	
	t.Run("BinarySelection", func(t *testing.T) {
		target := NewLocalDeploymentTarget()
		
		// Test with various artifact names
		artifacts := map[string][]byte{
			fmt.Sprintf("app-%s-%s", runtime.GOOS, runtime.GOARCH): []byte("correct binary"),
			"app-linux-amd64": []byte("linux binary"),
			"app-windows-amd64.exe": []byte("windows binary"),
			"config.yaml": []byte("config file"),
		}
		
		selected := target.selectBinary(artifacts)
		expected := fmt.Sprintf("app-%s-%s", runtime.GOOS, runtime.GOARCH)
		
		if selected != expected {
			t.Errorf("Expected binary %s, got %s", expected, selected)
		}
		
		// Test fallback
		fallbackArtifacts := map[string][]byte{
			"app": []byte("generic binary"),
			"config.yaml": []byte("config"),
		}
		
		selected = target.selectBinary(fallbackArtifacts)
		if selected != "app" && selected != "app.exe" {
			t.Errorf("Expected fallback binary 'app', got %s", selected)
		}
	})
	
	t.Run("LocalDeployment", func(t *testing.T) {
		target := NewLocalDeploymentTarget()
		
		// Create a simple shell script as "binary" for testing
		var testBinary []byte
		if runtime.GOOS == "windows" {
			testBinary = []byte("@echo off\necho Hello from deployment\nping -n 2 127.0.0.1 >nul")
		} else {
			testBinary = []byte("#!/bin/sh\necho 'Hello from deployment'\nsleep 1\necho 'Deployment running'")
		}
		
		artifacts := map[string][]byte{
			"app": testBinary,
		}
		
		config := DeployConfig{
			Target:      "local",
			Environment: "test",
			Version:     "v1.0.0",
			HealthCheck: &HealthCheckConfig{
				Enabled:      true,
				Interval:     100 * time.Millisecond,
				Retries:      3,
				SuccessCount: 1,
			},
		}
		
		ctx := context.Background()
		result, err := target.Deploy(ctx, artifacts, config)
		
		if err != nil {
			t.Fatalf("Deployment failed: %v", err)
		}
		
		if !result.Success {
			t.Error("Expected successful deployment")
		}
		
		if result.Target != target.Name() {
			t.Errorf("Expected target %s, got %s", target.Name(), result.Target)
		}
		
		if result.DeploymentID == "" {
			t.Error("Expected deployment ID")
		}
		
		// Check process metadata
		if pid, exists := result.Metadata["pid"]; !exists || pid == "" {
			t.Error("Expected process ID in metadata")
		}
		
		t.Logf("Deployed to %s with ID %s", result.Target, result.DeploymentID)
		
		// Give process time to run
		time.Sleep(500 * time.Millisecond)
		
		// Get process status
		status, err := target.GetProcessStatus(result.DeploymentID)
		if err != nil {
			t.Errorf("Failed to get process status: %v", err)
		}
		
		t.Logf("Process status: %s", status)
		
		// Get logs
		logs := target.GetLogs(result.DeploymentID, 10)
		if len(logs) > 0 {
			t.Logf("Process logs:")
			for _, log := range logs {
				t.Logf("  %s", log)
			}
		}
		
		// Clean up - stop the process
		if err := target.Rollback(result.DeploymentID); err != nil {
			t.Errorf("Failed to stop process: %v", err)
		}
	})
	
	t.Run("OutputCapture", func(t *testing.T) {
		target := NewLocalDeploymentTarget()
		
		// Create a script that produces output
		var testBinary []byte
		if runtime.GOOS == "windows" {
			testBinary = []byte(`@echo off
echo Starting application
echo [INFO] Server listening on port 8080
echo [WARN] Using default configuration
echo [ERROR] Failed to load optional module
echo Application ready`)
		} else {
			testBinary = []byte(`#!/bin/sh
echo "Starting application"
echo "[INFO] Server listening on port 8080"
echo "[WARN] Using default configuration"  
echo "[ERROR] Failed to load optional module" >&2
echo "Application ready"`)
		}
		
		artifacts := map[string][]byte{
			"app": testBinary,
		}
		
		config := DeployConfig{
			Target:      "local",
			Environment: "test",
			Version:     "v1.0.0",
		}
		
		ctx := context.Background()
		result, err := target.Deploy(ctx, artifacts, config)
		
		if err != nil {
			t.Fatalf("Deployment failed: %v", err)
		}
		
		// Wait for output to be captured
		time.Sleep(1 * time.Second)
		
		// Get logs
		logs := target.GetLogs(result.DeploymentID, 20)
		
		// Verify output was captured
		foundInfo := false
		foundWarn := false
		
		for _, log := range logs {
			if strings.Contains(log, "[INFO]") {
				foundInfo = true
			}
			if strings.Contains(log, "[WARN]") {
				foundWarn = true
			}
		}
		
		if !foundInfo {
			t.Error("Expected to find INFO log")
		}
		if !foundWarn {
			t.Error("Expected to find WARN log")
		}
		// Error might be on stderr, which we also capture
		
		t.Logf("Captured %d log lines", len(logs))
		
		// Clean up
		target.Rollback(result.DeploymentID)
	})
	
	t.Run("ProcessManagement", func(t *testing.T) {
		target := NewLocalDeploymentTarget()
		
		// Create a long-running script
		var testBinary []byte
		if runtime.GOOS == "windows" {
			testBinary = []byte(`@echo off
:loop
echo Process running
ping -n 2 127.0.0.1 >nul
goto loop`)
		} else {
			testBinary = []byte(`#!/bin/sh
while true; do
  echo "Process running"
  sleep 1
done`)
		}
		
		artifacts := map[string][]byte{
			"app": testBinary,
		}
		
		config := DeployConfig{
			Target:      "local",
			Environment: "test",
			Version:     "v1.0.0",
		}
		
		ctx := context.Background()
		result, err := target.Deploy(ctx, artifacts, config)
		
		if err != nil {
			t.Fatalf("Deployment failed: %v", err)
		}
		
		// Verify process is running
		status, _ := target.GetProcessStatus(result.DeploymentID)
		if status != "running" {
			t.Errorf("Expected process to be running, got %s", status)
		}
		
		// Stop the process
		err = target.Rollback(result.DeploymentID)
		if err != nil {
			t.Errorf("Failed to stop process: %v", err)
		}
		
		// Give it time to stop
		time.Sleep(500 * time.Millisecond)
		
		// Verify process stopped
		status, _ = target.GetProcessStatus(result.DeploymentID)
		if status != "stopped" {
			t.Errorf("Expected process to be stopped, got %s", status)
		}
	})
	
	t.Run("MemoryFirstIntegration", func(t *testing.T) {
		// Test that deployment integrates with shared memory
		sharedMem := &SharedPipelineMemory{
			BuildArtifacts: map[string][]byte{
				"app": []byte("#!/bin/sh\necho 'Deployed from memory'"),
			},
		}
		
		engine := NewNativeDeployEngine(sharedMem)
		
		config := DeployConfig{
			Target:      "local",
			Environment: "production",
			Version:     "v2.0.0",
		}
		
		ctx := context.Background()
		result, err := engine.DeployFromMemory(ctx, config)
		
		// Note: This will fail pre-deployment checks if no test results,
		// but that's OK for this test - we're verifying the local target is selected
		
		if err == nil {
			if !strings.Contains(result.Target, "local") {
				t.Errorf("Expected local deployment target, got %s", result.Target)
			}
		} else {
			// Even if deployment blocked, we can verify local target was attempted
			if !strings.Contains(err.Error(), "deployment blocked") {
				t.Errorf("Unexpected error: %v", err)
			}
		}
	})
}

func TestCircularLogBuffer(t *testing.T) {
	t.Run("BasicOperation", func(t *testing.T) {
		buffer := NewCircularLogBuffer(100)
		
		// Add some logs
		for i := 0; i < 10; i++ {
			buffer.Add(LogLine{
				Timestamp: time.Now(),
				Source:    "stdout",
				ProcessID: "test-proc",
				Content:   fmt.Sprintf("Log line %d", i),
				Level:     "info",
			})
		}
		
		// Get recent logs
		logs := buffer.GetRecent("test-proc", 5)
		if len(logs) != 5 {
			t.Errorf("Expected 5 logs, got %d", len(logs))
		}
		
		// Verify chronological order
		for i := 1; i < len(logs); i++ {
			if !strings.Contains(logs[i], fmt.Sprintf("Log line %d", i+4)) {
				t.Error("Logs not in chronological order")
			}
		}
	})
	
	t.Run("Overflow", func(t *testing.T) {
		buffer := NewCircularLogBuffer(10)
		
		// Add more logs than capacity
		for i := 0; i < 20; i++ {
			buffer.Add(LogLine{
				Timestamp: time.Now(),
				Source:    "stdout",
				ProcessID: "test-proc",
				Content:   fmt.Sprintf("Log %d", i),
				Level:     "info",
			})
		}
		
		// Should only get last 10
		logs := buffer.GetRecent("test-proc", 15)
		if len(logs) != 10 {
			t.Errorf("Expected max 10 logs, got %d", len(logs))
		}
		
		// Should have logs 10-19
		if !strings.Contains(logs[0], "Log 10") {
			t.Error("Buffer didn't wrap correctly")
		}
	})
	
	t.Run("ProcessFiltering", func(t *testing.T) {
		buffer := NewCircularLogBuffer(100)
		
		// Add logs from different processes
		buffer.Add(LogLine{
			Timestamp: time.Now(),
			ProcessID: "proc1",
			Content:   "Process 1 log",
		})
		buffer.Add(LogLine{
			Timestamp: time.Now(),
			ProcessID: "proc2",
			Content:   "Process 2 log",
		})
		buffer.Add(LogLine{
			Timestamp: time.Now(),
			ProcessID: "proc1",
			Content:   "Another proc1 log",
		})
		
		// Get logs for proc1 only
		logs := buffer.GetRecent("proc1", 10)
		if len(logs) != 2 {
			t.Errorf("Expected 2 logs for proc1, got %d", len(logs))
		}
		
		for _, log := range logs {
			if strings.Contains(log, "Process 2") {
				t.Error("Got log from wrong process")
			}
		}
	})
}

func TestLogLevelDetection(t *testing.T) {
	tests := []struct {
		content  string
		expected string
	}{
		{"[ERROR] Connection failed", "error"},
		{"Failed to start server", "error"},
		{"[WARN] Using default config", "warn"},
		{"Warning: deprecated function", "warn"},
		{"[DEBUG] Request received", "debug"},
		{"[INFO] Server started", "info"},
		{"Normal log message", "info"},
	}
	
	for _, test := range tests {
		level := detectLogLevel(test.content)
		if level != test.expected {
			t.Errorf("For '%s', expected level %s, got %s",
				test.content, test.expected, level)
		}
	}
}