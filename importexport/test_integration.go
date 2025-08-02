package importexport

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/caiatech/govc"
)

// TestImportExportIntegration provides integration testing for import/export functionality
type TestImportExportIntegration struct {
	testDir   string
	govcRepo  *govc.Repository
	gitRepo   string
	exportDir string
}

// NewTestIntegration creates a new integration test instance
func NewTestIntegration() (*TestImportExportIntegration, error) {
	testDir := filepath.Join(os.TempDir(), "govc-import-export-test")

	// Clean up any existing test directory
	os.RemoveAll(testDir)

	// Create test directories
	if err := os.MkdirAll(testDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create test directory: %w", err)
	}

	return &TestImportExportIntegration{
		testDir:   testDir,
		gitRepo:   filepath.Join(testDir, "git-repo"),
		exportDir: filepath.Join(testDir, "exported"),
	}, nil
}

// CreateTestGitRepo creates a test Git repository with sample content
func (t *TestImportExportIntegration) CreateTestGitRepo() error {
	// Create Git repository directory
	if err := os.MkdirAll(t.gitRepo, 0755); err != nil {
		return err
	}

	// Initialize bare Git repository structure
	gitDir := filepath.Join(t.gitRepo, ".git")
	dirs := []string{
		filepath.Join(gitDir, "objects"),
		filepath.Join(gitDir, "refs", "heads"),
		filepath.Join(gitDir, "refs", "tags"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// Create a simple blob object (Hello World content)
	blobContent := "blob 13\x00Hello, World!\n"
	blobHash := "af5626b4a114abcb82d63db7c8082c3c4756e51b" // SHA-1 of the content
	blobDir := filepath.Join(gitDir, "objects", blobHash[:2])
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		return err
	}

	// For simplicity, we'll create mock objects without proper compression
	// In a real test, these would be properly zlib-compressed Git objects
	blobPath := filepath.Join(blobDir, blobHash[2:])
	if err := os.WriteFile(blobPath, []byte(blobContent), 0644); err != nil {
		return err
	}

	// Create a mock tree object
	treeContent := "tree 36\x00100644 hello.txt\x00" + string([]byte{0xaf, 0x56, 0x26, 0xb4, 0xa1, 0x14, 0xab, 0xcb, 0x82, 0xd6, 0x3d, 0xb7, 0xc8, 0x08, 0x2c, 0x3c, 0x47, 0x56, 0xe5, 0x1b})
	treeHash := "b25c15b81f12526f4f4b2e3f5e9e9b5f8f8f8f8f"
	treeDir := filepath.Join(gitDir, "objects", treeHash[:2])
	if err := os.MkdirAll(treeDir, 0755); err != nil {
		return err
	}
	treePath := filepath.Join(treeDir, treeHash[2:])
	if err := os.WriteFile(treePath, []byte(treeContent), 0644); err != nil {
		return err
	}

	// Create a mock commit object
	commitContent := fmt.Sprintf("commit 200\x00tree %s\nauthor Test User <test@example.com> 1640995200 +0000\ncommitter Test User <test@example.com> 1640995200 +0000\n\nInitial commit\n", treeHash)
	commitHash := "c25c15b81f12526f4f4b2e3f5e9e9b5f8f8f8f8f"
	commitDir := filepath.Join(gitDir, "objects", commitHash[:2])
	if err := os.MkdirAll(commitDir, 0755); err != nil {
		return err
	}
	commitPath := filepath.Join(commitDir, commitHash[2:])
	if err := os.WriteFile(commitPath, []byte(commitContent), 0644); err != nil {
		return err
	}

	// Create main branch ref
	mainRefPath := filepath.Join(gitDir, "refs", "heads", "main")
	if err := os.WriteFile(mainRefPath, []byte(commitHash+"\n"), 0644); err != nil {
		return err
	}

	// Create HEAD
	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0644); err != nil {
		return err
	}

	// Create working directory file
	helloPath := filepath.Join(t.gitRepo, "hello.txt")
	if err := os.WriteFile(helloPath, []byte("Hello, World!\n"), 0644); err != nil {
		return err
	}

	return nil
}

// TestImport tests the Git import functionality
func (t *TestImportExportIntegration) TestImport() error {
	// Initialize govc repository
	govcRepoPath := filepath.Join(t.testDir, "govc-repo")
	var err error
	t.govcRepo, err = govc.InitRepository(govcRepoPath)
	if err != nil {
		return fmt.Errorf("failed to initialize govc repo: %w", err)
	}

	// Create Git importer
	importer := NewGitImporter(t.govcRepo, t.gitRepo)

	fmt.Printf("Testing Git import from %s to %s\n", t.gitRepo, govcRepoPath)

	// Perform import
	if err := importer.Import(); err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	// Get import progress
	progress := importer.GetProgress()
	fmt.Printf("Import completed: %s, Objects: %d/%d\n",
		progress.CurrentPhase, progress.ImportedObjects, progress.TotalObjects)

	if len(progress.Errors) > 0 {
		fmt.Printf("Import warnings/errors: %d\n", len(progress.Errors))
		for _, err := range progress.Errors {
			fmt.Printf("  - %v\n", err)
		}
	}

	// Verify import results
	files, err := t.govcRepo.ListFiles()
	if err != nil {
		return fmt.Errorf("failed to list files in imported repo: %w", err)
	}

	fmt.Printf("Imported files: %v\n", files)

	if len(files) == 0 {
		return fmt.Errorf("no files were imported")
	}

	return nil
}

// TestExport tests the Git export functionality
func (t *TestImportExportIntegration) TestExport() error {
	if t.govcRepo == nil {
		return fmt.Errorf("no govc repository available for export (run TestImport first)")
	}

	// Create Git exporter
	exporter := NewGitExporter(t.govcRepo, t.exportDir)

	fmt.Printf("Testing Git export from govc to %s\n", t.exportDir)

	// Perform export
	if err := exporter.Export(); err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	// Get export progress
	progress := exporter.GetProgress()
	fmt.Printf("Export completed: %s, Commits: %d/%d\n",
		progress.CurrentPhase, progress.ExportedCommits, progress.TotalCommits)

	if len(progress.Errors) > 0 {
		fmt.Printf("Export warnings/errors: %d\n", len(progress.Errors))
		for _, err := range progress.Errors {
			fmt.Printf("  - %v\n", err)
		}
	}

	// Verify export results
	gitDir := filepath.Join(t.exportDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Errorf(".git directory was not created")
	}

	// Check for basic Git structure
	requiredPaths := []string{
		filepath.Join(gitDir, "objects"),
		filepath.Join(gitDir, "refs"),
		filepath.Join(gitDir, "HEAD"),
		filepath.Join(gitDir, "config"),
	}

	for _, path := range requiredPaths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("required Git structure missing: %s", path)
		}
	}

	fmt.Printf("Git repository structure verified\n")

	return nil
}

// TestRoundTrip tests import followed by export to ensure consistency
func (t *TestImportExportIntegration) TestRoundTrip() error {
	fmt.Println("=== Testing Round Trip (Import -> Export) ===")

	// Test import
	if err := t.TestImport(); err != nil {
		return fmt.Errorf("round trip import failed: %w", err)
	}

	// Test export
	if err := t.TestExport(); err != nil {
		return fmt.Errorf("round trip export failed: %w", err)
	}

	// Compare original and exported files
	if err := t.compareDirectories(t.gitRepo, t.exportDir); err != nil {
		return fmt.Errorf("round trip comparison failed: %w", err)
	}

	fmt.Println("Round trip test completed successfully")
	return nil
}

// compareDirectories compares working directory files between original and exported repos
func (t *TestImportExportIntegration) compareDirectories(originalDir, exportedDir string) error {
	// Get list of files in original directory (excluding .git)
	originalFiles := make(map[string]string)
	err := filepath.WalkDir(originalDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || strings.Contains(path, ".git") {
			return nil
		}

		relPath, _ := filepath.Rel(originalDir, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		originalFiles[relPath] = string(content)
		return nil
	})
	if err != nil {
		return err
	}

	// Get list of files in exported directory (excluding .git)
	exportedFiles := make(map[string]string)
	err = filepath.WalkDir(exportedDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || strings.Contains(path, ".git") {
			return nil
		}

		relPath, _ := filepath.Rel(exportedDir, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		exportedFiles[relPath] = string(content)
		return nil
	})
	if err != nil {
		return err
	}

	// Compare file lists
	if len(originalFiles) != len(exportedFiles) {
		return fmt.Errorf("file count mismatch: original %d, exported %d",
			len(originalFiles), len(exportedFiles))
	}

	// Compare file contents
	for filename, originalContent := range originalFiles {
		exportedContent, exists := exportedFiles[filename]
		if !exists {
			return fmt.Errorf("file missing in export: %s", filename)
		}

		if originalContent != exportedContent {
			return fmt.Errorf("file content mismatch: %s", filename)
		}
	}

	fmt.Printf("Directory comparison successful: %d files match\n", len(originalFiles))
	return nil
}

// Cleanup removes all test files and directories
func (t *TestImportExportIntegration) Cleanup() error {
	return os.RemoveAll(t.testDir)
}

// RunIntegrationTests runs all integration tests
func RunIntegrationTests() error {
	fmt.Println("=== Running Import/Export Integration Tests ===")

	test, err := NewTestIntegration()
	if err != nil {
		return fmt.Errorf("failed to create test: %w", err)
	}
	defer test.Cleanup()

	// Create test Git repository
	fmt.Println("Creating test Git repository...")
	if err := test.CreateTestGitRepo(); err != nil {
		return fmt.Errorf("failed to create test Git repo: %w", err)
	}

	// Run tests
	if err := test.TestRoundTrip(); err != nil {
		return fmt.Errorf("integration test failed: %w", err)
	}

	fmt.Println("=== All Integration Tests Passed ===")
	return nil
}
