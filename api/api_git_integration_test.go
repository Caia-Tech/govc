package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCompleteGitWorkflow tests a comprehensive Git workflow
func TestCompleteGitWorkflow(t *testing.T) {
	_, router := setupTestServer()

	repoID := "git-workflow-test"

	// Step 1: Create repository
	t.Run("Create repository", func(t *testing.T) {
		body := bytes.NewBufferString(fmt.Sprintf(`{"id": "%s", "memory_only": true}`, repoID))
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to create repository: %d - %s", w.Code, w.Body.String())
		}
	})

	// Step 2: Add initial files
	t.Run("Add initial files", func(t *testing.T) {
		files := map[string]string{
			"README.md":     "# Git Workflow Test\n\nThis is a comprehensive test.",
			"src/main.go":   "package main\n\nfunc main() {\n\tprintln(\"Hello World\")\n}",
			"go.mod":        "module git-workflow-test\n\ngo 1.21",
			"docs/api.md":   "# API Documentation\n\n## Endpoints",
			".gitignore":    "*.log\n*.tmp\n/build/",
		}

		for path, content := range files {
			requestData := map[string]string{
				"path":    path,
				"content": content,
			}
			jsonData, _ := json.Marshal(requestData)
			body := bytes.NewBuffer(jsonData)
			req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != 200 {
				t.Errorf("Failed to add file %s: %d - %s", path, w.Code, w.Body.String())
			}
		}
	})

	// Step 3: Check status before commit
	t.Run("Check status before commit", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to get status: %d - %s", w.Code, w.Body.String())
		}

		var status map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &status)

		if clean, ok := status["clean"].(bool); !ok || clean {
			t.Error("Repository should not be clean with staged changes")
		}

		changes, ok := status["changes"].([]interface{})
		if !ok || len(changes) != 5 {
			t.Errorf("Expected 5 staged changes, got %d", len(changes))
		}
	})

	// Step 4: Initial commit
	t.Run("Create initial commit", func(t *testing.T) {
		body := bytes.NewBufferString(`{
			"message": "Initial commit: Add project structure",
			"author": "Test User",
			"email": "test@example.com"
		}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to create initial commit: %d - %s", w.Code, w.Body.String())
		}

		var commitResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &commitResp)
		_ = commitResp["hash"].(string) // We don't need to use this variable
	})

	// Step 5: Create feature branch
	t.Run("Create feature branch", func(t *testing.T) {
		body := bytes.NewBufferString(`{"name": "feature/add-tests"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to create feature branch: %d - %s", w.Code, w.Body.String())
		}
	})

	// Step 6: Checkout feature branch
	t.Run("Checkout feature branch", func(t *testing.T) {
		body := bytes.NewBufferString(`{"branch": "feature/add-tests"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to checkout feature branch: %d - %s", w.Code, w.Body.String())
		}
	})

	// Step 7: Add test files on feature branch
	t.Run("Add test files on feature branch", func(t *testing.T) {
		testFiles := map[string]string{
			"src/main_test.go":     "package main\n\nimport \"testing\"\n\nfunc TestMain(t *testing.T) {\n\t// Test code here\n}",
			"tests/integration.go": "package tests\n\n// Integration tests",
			"Makefile":            "test:\n\tgo test ./...\n\nbuild:\n\tgo build -o app src/main.go",
		}

		for path, content := range testFiles {
			requestData := map[string]string{
				"path":    path,
				"content": content,
			}
			jsonData, _ := json.Marshal(requestData)
			body := bytes.NewBuffer(jsonData)
			req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != 200 {
				t.Errorf("Failed to add test file %s: %d", path, w.Code)
			}
		}
	})

	// Step 8: Modify existing file
	t.Run("Modify existing file", func(t *testing.T) {
		newContent := "# Git Workflow Test\n\nThis is a comprehensive test with testing infrastructure.\n\n## Testing\n\nRun `make test` to execute tests."
		requestData := map[string]string{
			"path":    "README.md",
			"content": newContent,
		}
		jsonData, _ := json.Marshal(requestData)
		body := bytes.NewBuffer(jsonData)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("Failed to modify README.md: %d", w.Code)
		}
	})

	// Step 9: Check diff before commit
	t.Run("Check working diff", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/diff/working", repoID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to get working diff: %d - %s", w.Code, w.Body.String())
		}

		var diffResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &diffResp)

		files, ok := diffResp["files"].([]interface{})
		if !ok || len(files) < 3 {
			t.Errorf("Expected at least 3 changed files in working diff, got %d", len(files))
		}
	})

	// Step 10: Commit feature changes
	var featureCommitHash string
	t.Run("Commit feature changes", func(t *testing.T) {
		body := bytes.NewBufferString(`{
			"message": "Add comprehensive test infrastructure\n\n- Add unit tests for main package\n- Add integration test structure\n- Add Makefile for build automation\n- Update README with testing instructions",
			"author": "Test Developer",
			"email": "dev@example.com"
		}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to commit feature changes: %d - %s", w.Code, w.Body.String())
		}

		var commitResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &commitResp)
		featureCommitHash = commitResp["hash"].(string)
	})

	// Step 11: View commit log on feature branch
	t.Run("View commit log on feature branch", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/log?limit=10", repoID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to get commit log: %d - %s", w.Code, w.Body.String())
		}

		var logResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &logResp)

		commits, ok := logResp["commits"].([]interface{})
		if !ok || len(commits) != 2 {
			t.Errorf("Expected 2 commits in log, got %d", len(commits))
		}

		// Check latest commit is our feature commit
		latestCommit := commits[0].(map[string]interface{})
		if message := latestCommit["message"].(string); !strings.Contains(message, "test infrastructure") {
			t.Errorf("Unexpected latest commit message: %s", message)
		}
	})

	// Step 12: Show specific commit
	t.Run("Show feature commit details", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/show/%s", repoID, featureCommitHash), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to show commit: %d - %s", w.Code, w.Body.String())
		}

		var commitDetails map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &commitDetails)

		// Check commit has diff information
		diff, ok := commitDetails["diff"].(map[string]interface{})
		if !ok {
			t.Error("Expected diff in commit details")
		}

		files, ok := diff["files"].([]interface{})
		if !ok || len(files) < 3 {
			t.Errorf("Expected at least 3 files in commit diff, got %d", len(files))
		}
	})

	// Step 13: Switch back to main branch
	t.Run("Checkout main branch", func(t *testing.T) {
		body := bytes.NewBufferString(`{"branch": "main"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to checkout main branch: %d - %s", w.Code, w.Body.String())
		}
	})

	// Step 14: Verify main branch doesn't have feature changes
	t.Run("Verify main branch state", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/read/Makefile", repoID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 404 {
			t.Error("Makefile should not exist on main branch yet")
		}
	})

	// Step 15: Compare branches
	t.Run("Compare main and feature branches", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/diff/main/feature/add-tests", repoID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to get branch diff: %d - %s", w.Code, w.Body.String())
		}

		var diffResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &diffResp)

		files, ok := diffResp["files"].([]interface{})
		if !ok || len(files) < 3 {
			t.Errorf("Expected at least 3 different files between branches, got %d", len(files))
		}
	})

	// Step 16: Merge feature branch
	t.Run("Merge feature branch", func(t *testing.T) {
		body := bytes.NewBufferString(`{"from": "feature/add-tests", "to": "main"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/merge", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to merge branches: %d - %s", w.Code, w.Body.String())
		}
	})

	// Step 17: Verify merge on main branch
	t.Run("Verify merge completed", func(t *testing.T) {
		// Check Makefile now exists on main
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/read/Makefile", repoID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Error("Makefile should exist on main branch after merge")
		}

		// Check log shows merge commit
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/log?limit=5", repoID), nil)
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		var logResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &logResp)

		commits, ok := logResp["commits"].([]interface{})
		if !ok || len(commits) < 3 {
			t.Errorf("Expected at least 3 commits after merge, got %d", len(commits))
		}
	})

	// Step 18: Create and test stash
	t.Run("Test stash functionality", func(t *testing.T) {
		// Make some uncommitted changes
		body := bytes.NewBufferString(`{"path": "temp-work.txt", "content": "Work in progress..."}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Stash the changes
		body = bytes.NewBufferString(`{"message": "WIP: temporary work"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/stash", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to create stash: %d - %s", w.Code, w.Body.String())
		}

		// List stashes
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/stash", repoID), nil)
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to list stashes: %d - %s", w.Code, w.Body.String())
		}

		var stashResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &stashResp)

		stashes, ok := stashResp["stashes"].([]interface{})
		if !ok || len(stashes) != 1 {
			t.Errorf("Expected 1 stash, got %d", len(stashes))
		}
	})

	// Step 19: Test file operations
	t.Run("Test file operations", func(t *testing.T) {
		// List directory tree
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/tree/src", repoID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to list directory tree: %d - %s", w.Code, w.Body.String())
		}

		var treeResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &treeResp)

		files, ok := treeResp["files"].([]interface{})
		if !ok || len(files) < 2 {
			t.Errorf("Expected at least 2 files in src/, got %d", len(files))
		}

		// Get blame information
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/blame/README.md", repoID), nil)
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to get blame: %d - %s", w.Code, w.Body.String())
		}

		var blameResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &blameResp)

		lines, ok := blameResp["lines"].([]interface{})
		if !ok || len(lines) == 0 {
			t.Error("Expected blame lines for README.md")
		}
	})

	// Step 20: Test search functionality
	t.Run("Test search functionality", func(t *testing.T) {
		// Search commits
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/search/commits?q=test", repoID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to search commits: %d - %s", w.Code, w.Body.String())
		}

		// Search file content
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/search/content?q=package", repoID), nil)
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to search content: %d - %s", w.Code, w.Body.String())
		}

		// Search file names
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/search/files?q=*.go", repoID), nil)
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to search files: %d - %s", w.Code, w.Body.String())
		}
	})

	// Step 21: Final verification
	t.Run("Final repository state verification", func(t *testing.T) {
		// Get final status
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		var status map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &status)

		if branch := status["branch"].(string); branch != "main" {
			t.Errorf("Expected to be on main branch, got %s", branch)
		}

		// List all branches
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), nil)
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		var branchResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &branchResp)

		branches, ok := branchResp["branches"].([]interface{})
		if !ok || len(branches) < 2 {
			t.Errorf("Expected at least 2 branches, got %d", len(branches))
		}
	})
}

// TestAdvancedGitOperations tests more complex Git operations
func TestAdvancedGitOperations(t *testing.T) {
	_, router := setupTestServer()

	repoID := "advanced-git-test"
	createRepo(t, router, repoID)
	createInitialCommit(t, router, repoID)

	// Test cherry-pick
	t.Run("Test cherry-pick operation", func(t *testing.T) {
		// Create a feature branch with a specific commit
		body := bytes.NewBufferString(`{"name": "feature-branch"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/branches", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Checkout feature branch
		body = bytes.NewBufferString(`{"branch": "feature-branch"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Add a file and commit
		body = bytes.NewBufferString(`{"path": "feature.txt", "content": "Feature implementation"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		body = bytes.NewBufferString(`{"message": "Add feature implementation"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var commitResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &commitResp)
		featureCommitHash := commitResp["hash"].(string)

		// Switch back to main
		body = bytes.NewBufferString(`{"branch": "main"}`)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/checkout", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Cherry-pick the feature commit
		body = bytes.NewBufferString(fmt.Sprintf(`{"commit": "%s"}`, featureCommitHash))
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/cherry-pick", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Errorf("Cherry-pick failed: %d - %s", w.Code, w.Body.String())
		}
	})

	// Test revert
	t.Run("Test revert operation", func(t *testing.T) {
		// Get latest commit hash
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/log?limit=1", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var logResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &logResp)

		commits := logResp["commits"].([]interface{})
		latestCommit := commits[0].(map[string]interface{})
		commitHash := latestCommit["hash"].(string)

		// Revert the commit
		body := bytes.NewBufferString(fmt.Sprintf(`{"commit": "%s"}`, commitHash))
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/revert", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Errorf("Revert failed: %d - %s", w.Code, w.Body.String())
		}
	})

	// Test reset
	t.Run("Test reset operation", func(t *testing.T) {
		// Get commit log to find a commit to reset to
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/log?limit=3", repoID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var logResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &logResp)

		commits := logResp["commits"].([]interface{})
		if len(commits) >= 2 {
			targetCommit := commits[1].(map[string]interface{})
			commitHash := targetCommit["hash"].(string)

			// Reset to that commit (soft reset)
			body := bytes.NewBufferString(fmt.Sprintf(`{"target": "%s", "mode": "soft"}`, commitHash))
			req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/reset", repoID), body)
			req.Header.Set("Content-Type", "application/json")
			w = httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != 200 {
				t.Errorf("Reset failed: %d - %s", w.Code, w.Body.String())
			}
		}
	})
}

// TestFileOperations tests comprehensive file operations
func TestFileOperations(t *testing.T) {
	_, router := setupTestServer()

	repoID := "file-ops-test"
	createRepo(t, router, repoID)

	// Test file creation and reading
	t.Run("Create and read files", func(t *testing.T) {
		files := map[string]string{
			"text.txt":           "Simple text file",
			"json/data.json":     `{"key": "value", "number": 42}`,
			"code/main.py":       "#!/usr/bin/env python3\nprint('Hello World')",
			"docs/README.md":     "# Documentation\n\nThis is documentation.",
			"config/app.yaml":    "database:\n  host: localhost\n  port: 5432",
		}

		for path, content := range files {
			// Write file
			requestData := map[string]string{
				"path":    path,
				"content": content,
			}
			jsonData, _ := json.Marshal(requestData)
			body := bytes.NewBuffer(jsonData)
			req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != 200 {
				t.Errorf("Failed to add file %s: %d", path, w.Code)
				continue
			}

			// Read file back
			req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/read/%s", repoID, path), nil)
			w = httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != 200 {
				t.Errorf("Failed to read file %s: %d", path, w.Code)
				continue
			}

			var fileResp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &fileResp)

			if readContent := fileResp["content"].(string); readContent != content {
				t.Errorf("File content mismatch for %s: expected %q, got %q", path, content, readContent)
			}
		}
	})

	// Commit files
	t.Run("Commit all files", func(t *testing.T) {
		body := bytes.NewBufferString(`{"message": "Add all test files"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 201 {
			t.Fatalf("Failed to commit files: %d - %s", w.Code, w.Body.String())
		}
	})

	// Test file operations
	t.Run("File move operation", func(t *testing.T) {
		body := bytes.NewBufferString(`{"from": "text.txt", "to": "documents/text.txt"}`)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/repos/%s/move", repoID), body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("File move failed: %d - %s", w.Code, w.Body.String())
		}

		// Verify old location doesn't exist
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/read/text.txt", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 404 {
			t.Error("Old file location should not exist after move")
		}

		// Verify new location exists
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/read/documents/text.txt", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Error("New file location should exist after move")
		}
	})

	t.Run("File removal", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/repos/%s/remove/code/main.py", repoID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("File removal failed: %d - %s", w.Code, w.Body.String())
		}

		// Verify file is gone
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/read/code/main.py", repoID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 404 {
			t.Error("Removed file should not exist")
		}
	})

	// Test directory listing
	t.Run("Directory tree listing", func(t *testing.T) {
		// List root directory
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/repos/%s/tree/", repoID), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("Failed to list root directory: %d - %s", w.Code, w.Body.String())
		}

		var treeResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &treeResp)

		files, ok := treeResp["files"].([]interface{})
		if !ok {
			t.Fatal("Expected files array in tree response")
		}

		// Should have directories: documents, json, docs, config
		expectedDirs := []string{"documents", "json", "docs", "config"}
		foundDirs := make(map[string]bool)

		for _, file := range files {
			fileInfo := file.(map[string]interface{})
			if fileType := fileInfo["type"].(string); fileType == "directory" {
				foundDirs[fileInfo["name"].(string)] = true
			}
		}

		for _, expectedDir := range expectedDirs {
			if !foundDirs[expectedDir] {
				t.Errorf("Expected directory %s not found", expectedDir)
			}
		}
	})
}