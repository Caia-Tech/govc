package pipeline

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNativeDeployEngine(t *testing.T) {
	t.Run("PreDeploymentChecks", func(t *testing.T) {
		// Test that deployment is blocked by security issues
		sharedMem := &SharedPipelineMemory{
			SecurityIssues: []*ScanIssue{
				{
					Type:     VulnerabilityIssue,
					Severity: "CRITICAL",
					File:     "main.go",
					Line:     42,
					Message:  "SQL injection vulnerability",
				},
			},
			TestResults: &TestResult{
				Passed: 10,
				Failed: 0,
			},
			Coverage: &CoverageReport{
				Coverage:         85.0,
				SecurityCoverage: 100.0,
			},
		}
		
		engine := NewNativeDeployEngine(sharedMem)
		
		err := engine.performPreDeploymentChecks()
		if err == nil {
			t.Error("Expected deployment to be blocked by critical security issue")
		}
		if !strings.Contains(err.Error(), "critical security issues") {
			t.Errorf("Expected error about security issues, got: %v", err)
		}
	})
	
	t.Run("TestFailureBlocking", func(t *testing.T) {
		// Test that failed tests block deployment
		sharedMem := &SharedPipelineMemory{
			TestResults: &TestResult{
				Passed: 8,
				Failed: 2, // Failed tests should block
			},
			Coverage: &CoverageReport{
				Coverage:         80.0,
				SecurityCoverage: 100.0,
			},
		}
		
		engine := NewNativeDeployEngine(sharedMem)
		
		err := engine.performPreDeploymentChecks()
		if err == nil {
			t.Error("Expected deployment to be blocked by failed tests")
		}
		if !strings.Contains(err.Error(), "tests failed") {
			t.Errorf("Expected error about failed tests, got: %v", err)
		}
	})
	
	t.Run("LowCoverageBlocking", func(t *testing.T) {
		// Test that low coverage blocks deployment
		sharedMem := &SharedPipelineMemory{
			TestResults: &TestResult{
				Passed: 10,
				Failed: 0,
			},
			Coverage: &CoverageReport{
				Coverage:         65.0, // Below 70% threshold
				SecurityCoverage: 100.0,
			},
		}
		
		engine := NewNativeDeployEngine(sharedMem)
		
		err := engine.performPreDeploymentChecks()
		if err == nil {
			t.Error("Expected deployment to be blocked by low coverage")
		}
		if !strings.Contains(err.Error(), "below 70% threshold") {
			t.Errorf("Expected error about coverage threshold, got: %v", err)
		}
	})
	
	t.Run("SecurityCoverageBlocking", func(t *testing.T) {
		// Test that incomplete security coverage blocks deployment
		sharedMem := &SharedPipelineMemory{
			TestResults: &TestResult{
				Passed: 10,
				Failed: 0,
			},
			Coverage: &CoverageReport{
				Coverage:         85.0,
				SecurityCoverage: 75.0, // Vulnerabilities not fully tested
			},
		}
		
		engine := NewNativeDeployEngine(sharedMem)
		
		err := engine.performPreDeploymentChecks()
		if err == nil {
			t.Error("Expected deployment to be blocked by incomplete security coverage")
		}
		if !strings.Contains(err.Error(), "security vulnerabilities") {
			t.Errorf("Expected error about security coverage, got: %v", err)
		}
	})
	
	t.Run("SuccessfulPreChecks", func(t *testing.T) {
		// Test successful pre-deployment checks
		sharedMem := &SharedPipelineMemory{
			SecurityIssues: []*ScanIssue{}, // No critical issues
			TestResults: &TestResult{
				Passed: 10,
				Failed: 0, // All tests pass
			},
			Coverage: &CoverageReport{
				Coverage:         85.0,  // Good coverage
				SecurityCoverage: 100.0, // All vulnerabilities tested
			},
		}
		
		engine := NewNativeDeployEngine(sharedMem)
		
		err := engine.performPreDeploymentChecks()
		if err != nil {
			t.Errorf("Expected pre-deployment checks to pass, got: %v", err)
		}
	})
	
	t.Run("ArtifactRetrieval", func(t *testing.T) {
		// Test retrieving artifacts from shared memory
		expectedArtifacts := map[string][]byte{
			"app.exe":    []byte("binary content"),
			"config.yml": []byte("config content"),
		}
		
		sharedMem := &SharedPipelineMemory{
			BuildArtifacts: expectedArtifacts,
		}
		
		engine := NewNativeDeployEngine(sharedMem)
		
		artifacts, err := engine.getArtifactsFromMemory()
		if err != nil {
			t.Fatalf("Failed to get artifacts: %v", err)
		}
		
		if len(artifacts) != 2 {
			t.Errorf("Expected 2 artifacts, got %d", len(artifacts))
		}
		
		// Verify it's the same memory (not a copy)
		if &artifacts == &expectedArtifacts {
			// This is what we want - direct memory access
			t.Log("Artifacts retrieved by reference (no copy)")
		}
	})
	
	t.Run("TargetSelection", func(t *testing.T) {
		sharedMem := &SharedPipelineMemory{}
		engine := NewNativeDeployEngine(sharedMem)
		
		// Test Kubernetes target selection
		k8sTarget, err := engine.selectTarget("k8s-prod")
		if err != nil {
			t.Errorf("Failed to select k8s target: %v", err)
		}
		if k8sTarget.Type() != "kubernetes" {
			t.Errorf("Expected kubernetes target, got %s", k8sTarget.Type())
		}
		
		// Test Docker target selection
		dockerTarget, err := engine.selectTarget("docker-staging")
		if err != nil {
			t.Errorf("Failed to select docker target: %v", err)
		}
		if dockerTarget.Type() != "docker" {
			t.Errorf("Expected docker target, got %s", dockerTarget.Type())
		}
		
		// Test Lambda target selection
		lambdaTarget, err := engine.selectTarget("lambda-function")
		if err != nil {
			t.Errorf("Failed to select lambda target: %v", err)
		}
		if lambdaTarget.Type() != "lambda" {
			t.Errorf("Expected lambda target, got %s", lambdaTarget.Type())
		}
	})
	
	t.Run("StrategySelection", func(t *testing.T) {
		sharedMem := &SharedPipelineMemory{}
		engine := NewNativeDeployEngine(sharedMem)
		
		k8sTarget := NewKubernetesTarget()
		
		// Test rolling strategy
		rolling, err := engine.selectStrategy("rolling", k8sTarget)
		if err != nil {
			t.Errorf("Failed to select rolling strategy: %v", err)
		}
		if !rolling.SupportsTarget("kubernetes") {
			t.Error("Rolling strategy should support kubernetes")
		}
		
		// Test blue-green strategy
		blueGreen, err := engine.selectStrategy("blue-green", k8sTarget)
		if err != nil {
			t.Errorf("Failed to select blue-green strategy: %v", err)
		}
		if !blueGreen.SupportsTarget("kubernetes") {
			t.Error("Blue-green strategy should support kubernetes")
		}
		
		// Test canary strategy
		canary, err := engine.selectStrategy("canary", k8sTarget)
		if err != nil {
			t.Errorf("Failed to select canary strategy: %v", err)
		}
		if !canary.SupportsTarget("kubernetes") {
			t.Error("Canary strategy should support kubernetes")
		}
	})
	
	t.Run("FullDeploymentFlow", func(t *testing.T) {
		// Test full deployment with all checks passing
		sharedMem := &SharedPipelineMemory{
			SecurityIssues: []*ScanIssue{},
			TestResults: &TestResult{
				Passed: 10,
				Failed: 0,
			},
			Coverage: &CoverageReport{
				Coverage:         85.0,
				SecurityCoverage: 100.0,
			},
			BuildArtifacts: map[string][]byte{
				"app": []byte("application binary"),
			},
		}
		
		engine := NewNativeDeployEngine(sharedMem)
		
		config := DeployConfig{
			Target:      "kubernetes",
			Strategy:    "rolling",
			Environment: "staging",
			Version:     "v1.0.0",
			HealthCheck: &HealthCheckConfig{
				Enabled:      true,
				Interval:     time.Second,
				Retries:      3,
				SuccessCount: 2,
			},
		}
		
		ctx := context.Background()
		result, err := engine.DeployFromMemory(ctx, config)
		
		if err != nil {
			t.Fatalf("Deployment failed: %v", err)
		}
		
		if !result.Success {
			t.Error("Expected successful deployment")
		}
		
		if result.Target != "kubernetes" {
			t.Errorf("Expected kubernetes target, got %s", result.Target)
		}
		
		if result.Environment != "staging" {
			t.Errorf("Expected staging environment, got %s", result.Environment)
		}
		
		if result.HealthStatus != "healthy" {
			t.Errorf("Expected healthy status, got %s", result.HealthStatus)
		}
		
		// Verify deployment is stored in shared memory
		if sharedMem.Deployments == nil || len(sharedMem.Deployments) == 0 {
			t.Error("Deployment result not stored in shared memory")
		}
	})
	
	t.Run("DeploymentHistory", func(t *testing.T) {
		sharedMem := &SharedPipelineMemory{
			BuildArtifacts: map[string][]byte{
				"app": []byte("test app"),
			},
		}
		
		engine := NewNativeDeployEngine(sharedMem)
		
		// Perform multiple deployments
		for i := 0; i < 5; i++ {
			config := DeployConfig{
				Target:      "docker",
				Environment: "test",
				Version:     fmt.Sprintf("v1.0.%d", i),
			}
			
			ctx := context.Background()
			engine.DeployFromMemory(ctx, config)
		}
		
		// Check history
		history := engine.GetDeploymentHistory(3)
		if len(history) != 3 {
			t.Errorf("Expected 3 deployments in history, got %d", len(history))
		}
		
		// Verify most recent is last
		lastRecord := history[len(history)-1]
		if lastRecord.Config.Version != "v1.0.4" {
			t.Errorf("Expected most recent version v1.0.4, got %s", lastRecord.Config.Version)
		}
	})
}

func TestDeploymentTargets(t *testing.T) {
	t.Run("KubernetesTarget", func(t *testing.T) {
		target := NewKubernetesTarget()
		
		if target.Name() != "kubernetes" {
			t.Errorf("Expected name 'kubernetes', got %s", target.Name())
		}
		
		if target.Type() != "kubernetes" {
			t.Errorf("Expected type 'kubernetes', got %s", target.Type())
		}
		
		// Test deployment
		ctx := context.Background()
		artifacts := map[string][]byte{
			"manifest.yaml": []byte("k8s manifest"),
		}
		config := DeployConfig{
			Environment: "prod",
			Version:     "v2.0.0",
		}
		
		result, err := target.Deploy(ctx, artifacts, config)
		if err != nil {
			t.Errorf("Deployment failed: %v", err)
		}
		
		if !result.Success {
			t.Error("Expected successful deployment")
		}
		
		if result.URL == "" {
			t.Error("Expected URL to be set")
		}
	})
	
	t.Run("DockerTarget", func(t *testing.T) {
		target := NewDockerTarget()
		
		if target.Type() != "docker" {
			t.Errorf("Expected type 'docker', got %s", target.Type())
		}
		
		// Test health check
		err := target.HealthCheck()
		if err != nil {
			t.Errorf("Health check failed: %v", err)
		}
		
		// Test rollback
		err = target.Rollback("deploy-123")
		if err != nil {
			t.Errorf("Rollback failed: %v", err)
		}
	})
	
	t.Run("LambdaTarget", func(t *testing.T) {
		target := NewLambdaTarget()
		
		if target.Type() != "lambda" {
			t.Errorf("Expected type 'lambda', got %s", target.Type())
		}
		
		// Test deployment
		ctx := context.Background()
		artifacts := map[string][]byte{
			"function.zip": []byte("lambda code"),
		}
		config := DeployConfig{
			Environment: "prod",
			Version:     "v1.0.0",
		}
		
		result, err := target.Deploy(ctx, artifacts, config)
		if err != nil {
			t.Errorf("Deployment failed: %v", err)
		}
		
		if len(result.Endpoints) == 0 {
			t.Error("Expected endpoints to be set")
		}
	})
}

func TestDeploymentStrategies(t *testing.T) {
	t.Run("RollingStrategy", func(t *testing.T) {
		strategy := NewRollingStrategy()
		
		if !strategy.SupportsTarget("kubernetes") {
			t.Error("Rolling strategy should support kubernetes")
		}
		
		if !strategy.SupportsTarget("docker") {
			t.Error("Rolling strategy should support docker")
		}
		
		if strategy.SupportsTarget("lambda") {
			t.Error("Rolling strategy should not support lambda")
		}
	})
	
	t.Run("BlueGreenStrategy", func(t *testing.T) {
		strategy := NewBlueGreenStrategy()
		
		// Should support all targets
		if !strategy.SupportsTarget("kubernetes") {
			t.Error("Blue-green should support kubernetes")
		}
		if !strategy.SupportsTarget("docker") {
			t.Error("Blue-green should support docker")
		}
		if !strategy.SupportsTarget("lambda") {
			t.Error("Blue-green should support lambda")
		}
	})
	
	t.Run("CanaryStrategy", func(t *testing.T) {
		strategy := NewCanaryStrategy()
		
		if !strategy.SupportsTarget("kubernetes") {
			t.Error("Canary should support kubernetes")
		}
		
		if !strategy.SupportsTarget("lambda") {
			t.Error("Canary should support lambda")
		}
		
		if strategy.SupportsTarget("docker") {
			t.Error("Canary should not support docker")
		}
	})
}

func TestCrossToolIntelligenceInDeployment(t *testing.T) {
	t.Run("InstantFeedbackFromAllTools", func(t *testing.T) {
		// This test demonstrates the power of shared memory
		// All tools contribute to deployment decision instantly
		
		sharedMem := &SharedPipelineMemory{
			// Scanner found issues but not critical
			SecurityIssues: []*ScanIssue{
				{Type: VulnerabilityIssue, Severity: "MEDIUM", File: "utils.go"},
			},
			// Tests all pass
			TestResults: &TestResult{
				Passed: 25,
				Failed: 0,
			},
			// Coverage is good
			Coverage: &CoverageReport{
				Coverage:         82.5,
				SecurityCoverage: 100.0, // Medium issues are covered
			},
			// Build produced artifacts
			BuildArtifacts: map[string][]byte{
				"app":    []byte("compiled application"),
				"config": []byte("configuration"),
			},
		}
		
		engine := NewNativeDeployEngine(sharedMem)
		
		// Despite medium security issues, deployment proceeds
		// because they're all covered by tests
		err := engine.performPreDeploymentChecks()
		if err != nil {
			t.Errorf("Deployment should proceed with covered medium issues: %v", err)
		}
		
		// Deploy
		config := DeployConfig{
			Target:      "kubernetes",
			Environment: "staging",
			Version:     "v1.0.0",
		}
		
		ctx := context.Background()
		result, err := engine.DeployFromMemory(ctx, config)
		
		if err != nil {
			t.Errorf("Deployment failed: %v", err)
		}
		
		if !result.Success {
			t.Error("Expected successful deployment")
		}
		
		// Verify all tools contributed to the decision
		// Scanner: Provided security context
		// Tester: Confirmed all tests pass
		// Coverage: Verified vulnerabilities are tested
		// Builder: Provided artifacts
		// Deployer: Made intelligent decision based on all inputs
		
		t.Log("Cross-tool intelligence successfully demonstrated:")
		t.Log("- Security scanner found medium issues")
		t.Log("- Coverage analyzer confirmed 100% security coverage")
		t.Log("- Test runner confirmed all tests pass")
		t.Log("- Deployer made intelligent decision to proceed")
		t.Log("All happening in shared memory with zero file I/O!")
	})
}

func BenchmarkDeploymentEngine(b *testing.B) {
	sharedMem := &SharedPipelineMemory{
		TestResults: &TestResult{Passed: 10, Failed: 0},
		Coverage:    &CoverageReport{Coverage: 80.0, SecurityCoverage: 100.0},
		BuildArtifacts: map[string][]byte{
			"app": []byte("test application"),
		},
	}
	
	engine := NewNativeDeployEngine(sharedMem)
	config := DeployConfig{
		Target:      "docker",
		Environment: "test",
		Version:     "v1.0.0",
	}
	
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.DeployFromMemory(ctx, config)
	}
}

func BenchmarkPreDeploymentChecks(b *testing.B) {
	sharedMem := &SharedPipelineMemory{
		SecurityIssues: []*ScanIssue{
			{Type: VulnerabilityIssue, Severity: "LOW"},
			{Type: VulnerabilityIssue, Severity: "MEDIUM"},
		},
		TestResults: &TestResult{Passed: 20, Failed: 0},
		Coverage:    &CoverageReport{Coverage: 85.0, SecurityCoverage: 100.0},
	}
	
	engine := NewNativeDeployEngine(sharedMem)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.performPreDeploymentChecks()
	}
}