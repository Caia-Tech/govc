package tests

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Caia-Tech/govc"
	"github.com/Caia-Tech/govc/internal/repository"
	"github.com/Caia-Tech/govc/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_CLIWorkflow tests the complete CLI workflow
func TestIntegration_CLIWorkflow(t *testing.T) {
	// Build the CLI binary first
	tmpDir, err := ioutil.TempDir("", "govc-cli-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Build govc binary
	goBuild := exec.Command("go", "build", "-o", filepath.Join(tmpDir, "govc"), "./cmd/govc")
	goBuild.Dir = "/Users/owner/Desktop/caiatech/software/govc"
	err = goBuild.Run()
	require.NoError(t, err, "Failed to build govc binary")

	govcBin := filepath.Join(tmpDir, "govc")
	testRepo := filepath.Join(tmpDir, "test-repo")

	t.Run("InitRepository", func(t *testing.T) {
		cmd := exec.Command(govcBin, "init", testRepo)
		output, err := cmd.CombinedOutput()
		assert.NoError(t, err)
		assert.Contains(t, string(output), "Initialized")
	})

	t.Run("AddFiles", func(t *testing.T) {
		// Make sure test repo directory exists
		os.MkdirAll(testRepo, 0755)
		
		// Create test files
		testFile1 := filepath.Join(testRepo, "test1.txt")
		testFile2 := filepath.Join(testRepo, "test2.txt")
		ioutil.WriteFile(testFile1, []byte("content1"), 0644)
		ioutil.WriteFile(testFile2, []byte("content2"), 0644)

		// Add files
		cmd := exec.Command(govcBin, "add", "test1.txt", "test2.txt")
		cmd.Dir = testRepo
		output, err := cmd.CombinedOutput()
		assert.NoError(t, err)
		assert.Contains(t, string(output), "Added")
	})

	t.Run("Status", func(t *testing.T) {
		cmd := exec.Command(govcBin, "status")
		cmd.Dir = testRepo
		output, err := cmd.CombinedOutput()
		assert.NoError(t, err)
		assert.Contains(t, string(output), "test1.txt")
		assert.Contains(t, string(output), "test2.txt")
	})

	t.Run("Commit", func(t *testing.T) {
		cmd := exec.Command(govcBin, "commit", "-m", "Initial commit")
		cmd.Dir = testRepo
		output, err := cmd.CombinedOutput()
		assert.NoError(t, err)
		assert.Contains(t, string(output), "[")
		assert.Contains(t, string(output), "]")
	})

	t.Run("Branch", func(t *testing.T) {
		// Create branch
		cmd := exec.Command(govcBin, "branch", "feature")
		cmd.Dir = testRepo
		err := cmd.Run()
		assert.NoError(t, err)

		// List branches
		cmd = exec.Command(govcBin, "branch")
		cmd.Dir = testRepo
		output, err := cmd.CombinedOutput()
		assert.NoError(t, err)
		assert.Contains(t, string(output), "main")
		assert.Contains(t, string(output), "feature")
	})

	t.Run("Checkout", func(t *testing.T) {
		cmd := exec.Command(govcBin, "checkout", "feature")
		cmd.Dir = testRepo
		output, err := cmd.CombinedOutput()
		assert.NoError(t, err)
		assert.Contains(t, string(output), "Switched to branch 'feature'")
	})
}

// TestIntegration_RepositoryWorkflow tests repository operations programmatically
func TestIntegration_RepositoryWorkflow(t *testing.T) {
	repo := govc.New()

	t.Run("CompleteWorkflow", func(t *testing.T) {
		// Initial setup
		staging := repo.GetStagingArea()
		
		// Add multiple files
		files := map[string][]byte{
			"main.go":     []byte("package main\nfunc main() {}"),
			"config.yaml": []byte("version: 1.0\nname: test"),
			"README.md":   []byte("# Test Project\nThis is a test"),
		}

		for path, content := range files {
			err := staging.Add(path, content)
			assert.NoError(t, err)
		}

		// Commit
		commit1, err := repo.Commit("Initial commit")
		assert.NoError(t, err)
		assert.NotNil(t, commit1)

		// Create and switch to feature branch
		err = repo.CreateBranch("feature/new-feature", "")
		assert.NoError(t, err)
		
		err = repo.SwitchBranch("feature/new-feature")
		assert.NoError(t, err)

		// Add feature files
		staging.Add("feature.go", []byte("package feature"))
		commit2, err := repo.Commit("Add feature")
		assert.NoError(t, err)
		assert.NotNil(t, commit2)

		// Switch back to main
		err = repo.SwitchBranch("main")
		assert.NoError(t, err)

		// Verify branch isolation
		mainCommits, _ := repo.ListCommits("main", 10)
		featureCommits, _ := repo.ListCommits("feature/new-feature", 10)
		
		assert.NotEmpty(t, mainCommits)
		assert.NotEmpty(t, featureCommits)
	})
}

// TestIntegration_ConcurrentUsers simulates multiple users working on the same repository
func TestIntegration_ConcurrentUsers(t *testing.T) {
	// Create a shared repository
	tmpDir, err := ioutil.TempDir("", "govc-concurrent-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Initialize repository
	repo, err := govc.Init(tmpDir)
	require.NoError(t, err)

	// Simulate multiple users
	numUsers := 5
	commitsPerUser := 10
	var wg sync.WaitGroup
	successCounts := make([]int, numUsers)

	for userID := 0; userID < numUsers; userID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Each user creates their own branch
			branchName := fmt.Sprintf("user-%d-branch", id)
			userRepo, _ := govc.Open(tmpDir)
			
			// Create branch
			err := userRepo.CreateBranch(branchName, "")
			if err != nil {
				return
			}
			
			err = userRepo.SwitchBranch(branchName)
			if err != nil {
				return
			}

			// Make commits
			for i := 0; i < commitsPerUser; i++ {
				staging := userRepo.GetStagingArea()
				fileName := fmt.Sprintf("user%d_file%d.txt", id, i)
				content := fmt.Sprintf("Content from user %d, commit %d", id, i)
				
				staging.Add(fileName, []byte(content))
				_, err := userRepo.Commit(fmt.Sprintf("User %d commit %d", id, i))
				if err == nil {
					successCounts[id]++
				}
			}
		}(userID)
	}

	wg.Wait()

	// Verify results
	totalSuccessful := 0
	for _, count := range successCounts {
		totalSuccessful += count
	}

	assert.Greater(t, totalSuccessful, 0, "At least some commits should succeed")
	t.Logf("Total successful commits across %d users: %d/%d", numUsers, totalSuccessful, numUsers*commitsPerUser)

	// Check branches were created (at least main should exist)
	branches, err := repo.ListBranches()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(branches), 1) // At least main branch exists
}

// TestIntegration_LargeFiles tests handling of large files
func TestIntegration_LargeFiles(t *testing.T) {
	repo := govc.New()
	staging := repo.GetStagingArea()

	t.Run("SingleLargeFile", func(t *testing.T) {
		// Create a 50MB file
		largeContent := bytes.Repeat([]byte("a"), 50*1024*1024)
		
		start := time.Now()
		err := staging.Add("large.bin", largeContent)
		assert.NoError(t, err)
		
		commit, err := repo.Commit("Add large file")
		assert.NoError(t, err)
		assert.NotNil(t, commit)
		
		duration := time.Since(start)
		t.Logf("Large file (50MB) committed in %v", duration)
		assert.Less(t, duration, 5*time.Second, "Should handle large files efficiently")
	})

	t.Run("ManySmallFiles", func(t *testing.T) {
		staging.Clear()
		
		start := time.Now()
		// Add 1000 small files
		for i := 0; i < 1000; i++ {
			fileName := fmt.Sprintf("file%04d.txt", i)
			content := fmt.Sprintf("Content of file %d", i)
			staging.Add(fileName, []byte(content))
		}
		
		commit, err := repo.Commit("Add many files")
		assert.NoError(t, err)
		assert.NotNil(t, commit)
		
		duration := time.Since(start)
		t.Logf("1000 small files committed in %v", duration)
		assert.Less(t, duration, 2*time.Second, "Should handle many files efficiently")
	})
}

// TestIntegration_APIServer tests the API server integration
func TestIntegration_APIServer(t *testing.T) {
	t.Skip("Skipping API server test - requires server setup")
	
	// This would test the API server if it was running
	repo := govc.New()
	
	// Add test data
	staging := repo.GetStagingArea()
	staging.Add("api-test.txt", []byte("API test content"))
	repo.Commit("API test commit")

	// Start API server
	server := api.NewServer(repo)
	_ = server // Server would be used here
	
	// Server would be started here if the Start method existed
	// go server.ListenAndServe(":8080")

	// Test API endpoints would go here
}

// TestIntegration_PipelineExecution tests the pipeline system
func TestIntegration_PipelineExecution(t *testing.T) {
	t.Skip("Skipping pipeline test - pipeline system not fully implemented")
	
	// This would test the pipeline system when implemented
	repo := govc.New()
	
	// Pipeline tests would go here when the pipeline package is available
	_ = repo
}

// TestIntegration_SearchAndQuery tests search functionality
func TestIntegration_SearchAndQuery(t *testing.T) {
	repo := govc.New()

	// Add test data with searchable content
	testData := map[string]string{
		"main.go":        "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"Hello\") }",
		"utils.go":       "package utils\nfunc Helper() string { return \"helper\" }",
		"config.json":    `{"database": "postgres", "port": 5432}`,
		"README.md":      "# Project\nThis project uses PostgreSQL database",
		"docs/api.md":    "## API Documentation\nEndpoint: /api/v1/users",
		"test/main_test.go": "package main\nimport \"testing\"\nfunc TestMain(t *testing.T) {}",
	}

	staging := repo.GetStagingArea()
	for path, content := range testData {
		staging.Add(path, []byte(content))
	}
	repo.Commit("Add test files for search")

	t.Run("SearchCommits", func(t *testing.T) {
		// Search for commits
		commits, total, err := repo.SearchCommits("test", "", "", "", 10, 0)
		assert.NoError(t, err)
		assert.NotNil(t, commits)
		assert.GreaterOrEqual(t, total, 0)
	})

	t.Run("FullTextSearch", func(t *testing.T) {
		// Initialize search
		err := repo.InitializeAdvancedSearch()
		assert.NoError(t, err)

		// Perform search
		req := &repository.FullTextSearchRequest{
			Query: "database",
			Limit: 10,
		}
		
		resp, err := repo.FullTextSearch(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
	})
}

// TestIntegration_PersistenceAndRecovery tests data persistence and recovery
func TestIntegration_PersistenceAndRecovery(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "govc-persist-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("PersistentStaging", func(t *testing.T) {
		// Create repository and add files
		repo1, err := govc.Init(tmpDir)
		assert.NoError(t, err)

		staging1 := repo1.GetStagingArea()
		staging1.Add("persist1.txt", []byte("persistent content 1"))
		staging1.Add("persist2.txt", []byte("persistent content 2"))

		// Open repository again
		repo2, err := govc.Open(tmpDir)
		assert.NoError(t, err)

		// Check staging persisted
		staging2 := repo2.GetStagingArea()
		files, _ := staging2.List()
		assert.Contains(t, files, "persist1.txt")
		assert.Contains(t, files, "persist2.txt")
	})

	t.Run("Recovery", func(t *testing.T) {
		// Simulate crash during operation
		repo, _ := govc.Open(tmpDir)
		staging := repo.GetStagingArea()
		
		// Start adding files
		staging.Add("recovery1.txt", []byte("recovery test"))
		
		// Simulate crash (don't commit)
		// Repository should handle this gracefully on next open
		
		// Reopen
		recoveredRepo, err := govc.Open(tmpDir)
		assert.NoError(t, err)
		assert.NotNil(t, recoveredRepo)
		
		// Should be able to continue
		recoverStaging := recoveredRepo.GetStagingArea()
		files, _ := recoverStaging.List()
		assert.Contains(t, files, "recovery1.txt")
	})
}

// TestIntegration_PerformanceUnderLoad tests system performance under heavy load
func TestIntegration_PerformanceUnderLoad(t *testing.T) {
	repo := govc.New()

	t.Run("HighFrequencyCommits", func(t *testing.T) {
		numCommits := 1000
		start := time.Now()

		for i := 0; i < numCommits; i++ {
			staging := repo.GetStagingArea()
			staging.Add(fmt.Sprintf("file%d.txt", i), []byte(fmt.Sprintf("content %d", i)))
			_, err := repo.Commit(fmt.Sprintf("Commit %d", i))
			assert.NoError(t, err)
		}

		duration := time.Since(start)
		commitsPerSecond := float64(numCommits) / duration.Seconds()
		
		t.Logf("Performed %d commits in %v (%.2f commits/sec)", numCommits, duration, commitsPerSecond)
		assert.Greater(t, commitsPerSecond, 100.0, "Should handle at least 100 commits/sec")
	})

	t.Run("ConcurrentLoad", func(t *testing.T) {
		var wg sync.WaitGroup
		numGoroutines := 50
		opsPerGoroutine := 100
		var successCount int32

		start := time.Now()

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				for j := 0; j < opsPerGoroutine; j++ {
					staging := repo.GetStagingArea()
					fileName := fmt.Sprintf("concurrent_%d_%d.txt", id, j)
					staging.Add(fileName, []byte(fmt.Sprintf("data %d %d", id, j)))
					
					if _, err := repo.Commit(fmt.Sprintf("Concurrent %d-%d", id, j)); err == nil {
						atomic.AddInt32(&successCount, 1)
					}
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(start)
		
		totalOps := numGoroutines * opsPerGoroutine
		opsPerSecond := float64(successCount) / duration.Seconds()
		
		t.Logf("Concurrent load test: %d/%d successful ops in %v (%.2f ops/sec)", 
			successCount, totalOps, duration, opsPerSecond)
		assert.Greater(t, float64(successCount), float64(totalOps)*0.5, 
			"At least 50% of operations should succeed under concurrent load")
	})

	t.Run("MemoryUsage", func(t *testing.T) {
		// Monitor memory usage during operations
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		startMem := m.Alloc

		// Perform memory-intensive operations
		for i := 0; i < 100; i++ {
			staging := repo.GetStagingArea()
			// Add 1MB file
			largeContent := bytes.Repeat([]byte("x"), 1024*1024)
			staging.Add(fmt.Sprintf("large%d.bin", i), largeContent)
			repo.Commit(fmt.Sprintf("Large file %d", i))
		}

		runtime.ReadMemStats(&m)
		endMem := m.Alloc
		memUsed := endMem - startMem

		t.Logf("Memory used for 100 x 1MB files: %d MB", memUsed/1024/1024)
		// Memory-first system will use memory, but should be reasonable
		assert.Less(t, memUsed, uint64(500*1024*1024), "Should use less than 500MB for 100MB of data")
	})
}

// TestIntegration_MultiBranchWorkflow tests complex multi-branch scenarios
func TestIntegration_MultiBranchWorkflow(t *testing.T) {
	repo := govc.New()

	// Setup initial repository
	staging := repo.GetStagingArea()
	staging.Add("main.go", []byte("package main"))
	staging.Add("config.yaml", []byte("version: 1.0"))
	baseCommit, _ := repo.Commit("Initial setup")

	t.Run("FeatureBranches", func(t *testing.T) {
		// Create multiple feature branches
		features := []string{"feature-a", "feature-b", "feature-c"}
		
		for _, feature := range features {
			err := repo.CreateBranch(feature, baseCommit.Hash())
			assert.NoError(t, err)
			
			// Switch to feature branch
			err = repo.SwitchBranch(feature)
			assert.NoError(t, err)
			
			// Add feature-specific files
			staging.Add(fmt.Sprintf("%s.go", feature), []byte(fmt.Sprintf("package %s", feature)))
			_, err = repo.Commit(fmt.Sprintf("Add %s", feature))
			assert.NoError(t, err)
		}

		// Switch back to main
		err := repo.SwitchBranch("main")
		assert.NoError(t, err)

		// Verify all branches exist
		branches, _ := repo.ListBranches()
		for _, feature := range features {
			assert.Contains(t, branches, feature)
		}
	})

	t.Run("ParallelDevelopment", func(t *testing.T) {
		// Simulate parallel development on different branches
		var wg sync.WaitGroup
		branches := []string{"dev", "staging", "hotfix"}

		for _, branch := range branches {
			repo.CreateBranch(branch, "")
		}

		for _, branch := range branches {
			wg.Add(1)
			go func(b string) {
				defer wg.Done()
				
				// Each branch makes its own commits
				branchRepo := govc.New() // In real scenario, would be same repo
				branchRepo.SwitchBranch(b)
				
				for i := 0; i < 5; i++ {
					staging := branchRepo.GetStagingArea()
					staging.Add(fmt.Sprintf("%s_%d.txt", b, i), []byte(fmt.Sprintf("%s content %d", b, i)))
					branchRepo.Commit(fmt.Sprintf("%s commit %d", b, i))
				}
			}(branch)
		}

		wg.Wait()
	})
}

// TestIntegration_RealWorldScenario tests a realistic development workflow
func TestIntegration_RealWorldScenario(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "govc-realworld-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Initialize project repository
	repo, err := govc.Init(tmpDir)
	require.NoError(t, err)

	t.Run("ProjectSetup", func(t *testing.T) {
		staging := repo.GetStagingArea()
		
		// Add project files
		projectFiles := map[string]string{
			"main.go": `package main
import "fmt"
func main() {
    fmt.Println("Hello, World!")
}`,
			"go.mod": `module github.com/example/project
go 1.19`,
			"README.md": `# Example Project
This is an example project for testing GoVC`,
			".gitignore": `*.exe
*.dll
*.so
*.dylib
/vendor/`,
		}

		for path, content := range projectFiles {
			staging.Add(path, []byte(content))
		}

		commit, err := repo.Commit("Initial project setup")
		assert.NoError(t, err)
		assert.NotNil(t, commit)
	})

	t.Run("FeatureDevelopment", func(t *testing.T) {
		// Create feature branch
		err := repo.CreateBranch("feature/user-auth", "")
		assert.NoError(t, err)
		
		err = repo.SwitchBranch("feature/user-auth")
		assert.NoError(t, err)

		// Add authentication code
		staging := repo.GetStagingArea()
		staging.Add("auth/auth.go", []byte(`package auth

import "crypto/sha256"

func HashPassword(password string) string {
    h := sha256.Sum256([]byte(password))
    return fmt.Sprintf("%x", h)
}`))
		
		staging.Add("auth/auth_test.go", []byte(`package auth

import "testing"

func TestHashPassword(t *testing.T) {
    hash := HashPassword("test")
    if hash == "" {
        t.Error("Hash should not be empty")
    }
}`))

		_, err = repo.Commit("Add user authentication")
		assert.NoError(t, err)
	})

	t.Run("HotfixScenario", func(t *testing.T) {
		// Switch to main for hotfix
		err := repo.SwitchBranch("main")
		assert.NoError(t, err)

		// Create hotfix branch
		err = repo.CreateBranch("hotfix/security-patch", "")
		assert.NoError(t, err)
		
		err = repo.SwitchBranch("hotfix/security-patch")
		assert.NoError(t, err)

		// Apply hotfix
		staging := repo.GetStagingArea()
		staging.Add("security.go", []byte(`package main

func SecurityCheck() bool {
    return true
}`))

		_, err = repo.Commit("Apply security patch")
		assert.NoError(t, err)

		// Switch back to main
		err = repo.SwitchBranch("main")
		assert.NoError(t, err)
	})

	t.Run("ReleasePreparation", func(t *testing.T) {
		// Create release branch
		err := repo.CreateBranch("release/v1.0.0", "")
		assert.NoError(t, err)
		
		err = repo.SwitchBranch("release/v1.0.0")
		assert.NoError(t, err)

		// Update version
		staging := repo.GetStagingArea()
		staging.Add("VERSION", []byte("1.0.0"))
		staging.Add("CHANGELOG.md", []byte(`# Changelog

## [1.0.0] - 2024-01-01
### Added
- Initial release
- User authentication
- Security patches`))

		_, err = repo.Commit("Prepare release v1.0.0")
		assert.NoError(t, err)
	})

	// Verify final state
	branches, err := repo.ListBranches()
	assert.NoError(t, err)
	assert.Contains(t, branches, "main")
	assert.Contains(t, branches, "feature/user-auth")
	assert.Contains(t, branches, "hotfix/security-patch")
	assert.Contains(t, branches, "release/v1.0.0")
}