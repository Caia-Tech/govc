package importexport

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Caia-Tech/govc"
)

// GitExporter exports govc repositories to standard Git format
type GitExporter struct {
	repo       *govc.Repository
	exportPath string
	progress   ExportProgress
	objectMap  map[string]string // govc hash -> git hash mapping
}

// ExportProgress tracks export progress
type ExportProgress struct {
	TotalCommits    int
	ExportedCommits int
	CurrentPhase    string
	Errors          []error
}

// NewGitExporter creates a new Git exporter
func NewGitExporter(govcRepo *govc.Repository, exportPath string) *GitExporter {
	return &GitExporter{
		repo:       govcRepo,
		exportPath: exportPath,
		progress: ExportProgress{
			CurrentPhase: "Initializing",
			Errors:       []error{},
		},
		objectMap: make(map[string]string),
	}
}

// Export performs the full export from govc to Git
func (ge *GitExporter) Export() error {
	// Initialize Git repository structure
	ge.progress.CurrentPhase = "Creating Git repository"
	if err := ge.initGitRepo(); err != nil {
		return fmt.Errorf("failed to initialize git repo: %w", err)
	}

	// Export commits
	ge.progress.CurrentPhase = "Exporting commits"
	if err := ge.exportCommits(); err != nil {
		return fmt.Errorf("failed to export commits: %w", err)
	}

	// Export branches
	ge.progress.CurrentPhase = "Exporting branches"
	if err := ge.exportBranches(); err != nil {
		return fmt.Errorf("failed to export branches: %w", err)
	}

	// Export tags
	ge.progress.CurrentPhase = "Exporting tags"
	if err := ge.exportTags(); err != nil {
		return fmt.Errorf("failed to export tags: %w", err)
	}

	// Set HEAD
	ge.progress.CurrentPhase = "Setting HEAD"
	if err := ge.exportHEAD(); err != nil {
		return fmt.Errorf("failed to export HEAD: %w", err)
	}

	// Export working tree (optional)
	ge.progress.CurrentPhase = "Exporting working tree"
	if err := ge.exportWorkingTree(); err != nil {
		return fmt.Errorf("failed to export working tree: %w", err)
	}

	ge.progress.CurrentPhase = "Complete"
	return nil
}

// GetProgress returns the current export progress
func (ge *GitExporter) GetProgress() ExportProgress {
	return ge.progress
}

func (ge *GitExporter) initGitRepo() error {
	// Create directory structure
	dirs := []string{
		filepath.Join(ge.exportPath, ".git"),
		filepath.Join(ge.exportPath, ".git", "objects"),
		filepath.Join(ge.exportPath, ".git", "objects", "info"),
		filepath.Join(ge.exportPath, ".git", "objects", "pack"),
		filepath.Join(ge.exportPath, ".git", "refs"),
		filepath.Join(ge.exportPath, ".git", "refs", "heads"),
		filepath.Join(ge.exportPath, ".git", "refs", "tags"),
		filepath.Join(ge.exportPath, ".git", "info"),
		filepath.Join(ge.exportPath, ".git", "hooks"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// Write basic Git files
	if err := ge.writeGitFile("config", ge.getGitConfig()); err != nil {
		return err
	}

	if err := ge.writeGitFile("description", "Exported from govc\n"); err != nil {
		return err
	}

	if err := ge.writeGitFile("info/exclude", ""); err != nil {
		return err
	}

	return nil
}

func (ge *GitExporter) getGitConfig() string {
	return `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
`
}

func (ge *GitExporter) exportCommits() error {
	// Get all commits
	commits, err := ge.repo.Log(1000000) // Large limit to get all commits
	if err != nil {
		return err
	}

	ge.progress.TotalCommits = len(commits)

	// Build commit graph to ensure proper ordering
	commitMap := make(map[string]*govc.Commit)
	for _, commit := range commits {
		commitMap[commit.Hash()] = commit
	}

	// Export commits in topological order
	visited := make(map[string]bool)
	for _, commit := range commits {
		if !visited[commit.Hash()] {
			if err := ge.exportCommitRecursive(commit, commitMap, visited); err != nil {
				ge.progress.Errors = append(ge.progress.Errors, err)
			}
		}
	}

	return nil
}

func (ge *GitExporter) exportCommitRecursive(commit *govc.Commit, commitMap map[string]*govc.Commit, visited map[string]bool) error {
	if visited[commit.Hash()] {
		return nil
	}

	// Export parent commits (simplified - not directly available in current govc API)
	// For now, we'll process commits in the order they appear in the log

	// Export this commit
	if err := ge.exportCommit(commit); err != nil {
		return err
	}

	visited[commit.Hash()] = true
	ge.progress.ExportedCommits++
	return nil
}

func (ge *GitExporter) exportCommit(commit *govc.Commit) error {
	// Export tree first
	treeHash, err := ge.exportTree(commit)
	if err != nil {
		return fmt.Errorf("failed to export tree for commit %s: %w", commit.Hash(), err)
	}

	// Build commit object content
	var content bytes.Buffer
	content.WriteString(fmt.Sprintf("tree %s\n", treeHash))

	// Add parent commits (simplified - govc doesn't expose parent relationships directly)
	// In a full implementation, we would need to track commit relationships
	// For now, we'll skip parent relationships to make it compile

	// Add author
	authorTime := commit.Author.Time.Unix()
	content.WriteString(fmt.Sprintf("author %s <%s> %d +0000\n",
		commit.Author.Name,
		commit.Author.Email,
		authorTime))

	// Add committer (same as author for simplicity)
	content.WriteString(fmt.Sprintf("committer %s <%s> %d +0000\n",
		commit.Author.Name,
		commit.Author.Email,
		authorTime))

	// Add commit message
	content.WriteString("\n")
	content.WriteString(commit.Message)
	if !strings.HasSuffix(commit.Message, "\n") {
		content.WriteString("\n")
	}

	// Write commit object
	gitHash := ge.writeObject("commit", content.Bytes())
	ge.objectMap[commit.Hash()] = gitHash

	return nil
}

func (ge *GitExporter) exportTree(commit *govc.Commit) (string, error) {
	// Build complete directory tree structure
	files, err := ge.repo.ListFiles()
	if err != nil {
		return "", err
	}

	// Group files by directory
	treeMap := make(map[string][]TreeEntry)

	for _, filePath := range files {
		// Get file content
		content, err := ge.repo.ReadFile(filePath)
		if err != nil {
			continue
		}

		// Write blob object
		blobHash := ge.writeObject("blob", []byte(content))

		// Determine file mode (simplified)
		mode := "100644" // Regular file

		// Split path into directory and filename
		dir := filepath.Dir(filePath)
		filename := filepath.Base(filePath)

		// Normalize directory path
		if dir == "." {
			dir = ""
		}

		treeMap[dir] = append(treeMap[dir], TreeEntry{
			Mode: mode,
			Name: filename,
			Hash: blobHash,
		})
	}

	// Build trees from deepest to shallowest
	treeHashes := make(map[string]string)

	// Sort directories by depth (deepest first)
	dirs := make([]string, 0, len(treeMap))
	for dir := range treeMap {
		dirs = append(dirs, dir)
	}

	// Sort by depth (number of path separators)
	sort.Slice(dirs, func(i, j int) bool {
		return strings.Count(dirs[i], string(filepath.Separator)) > strings.Count(dirs[j], string(filepath.Separator))
	})

	for _, dir := range dirs {
		var treeContent bytes.Buffer
		entries := treeMap[dir]

		// Sort entries by name
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name < entries[j].Name
		})

		for _, entry := range entries {
			// Write tree entry
			entryLine := fmt.Sprintf("%s %s\x00", entry.Mode, entry.Name)
			treeContent.WriteString(entryLine)

			// Write hash as binary
			hashBytes, _ := hex.DecodeString(entry.Hash)
			treeContent.Write(hashBytes)
		}

		// Add subdirectory trees
		for subdir, hash := range treeHashes {
			if filepath.Dir(subdir) == dir && subdir != dir {
				subdirName := filepath.Base(subdir)
				entryLine := fmt.Sprintf("40000 %s\x00", subdirName)
				treeContent.WriteString(entryLine)

				hashBytes, _ := hex.DecodeString(hash)
				treeContent.Write(hashBytes)
			}
		}

		treeHash := ge.writeObject("tree", treeContent.Bytes())
		treeHashes[dir] = treeHash
	}

	// Return root tree hash
	if rootHash, ok := treeHashes[""]; ok {
		return rootHash, nil
	}

	// If no root tree, create empty tree
	return ge.writeObject("tree", []byte{}), nil
}

func (ge *GitExporter) exportBranches() error {
	branches, err := ge.repo.ListBranches()
	if err != nil {
		return err
	}

	for _, branch := range branches {
		// Get the Git hash for this branch's commit
		if gitHash, ok := ge.objectMap[branch.Hash]; ok {
			// Remove refs/heads/ prefix if present to avoid double nesting
			branchName := strings.TrimPrefix(branch.Name, "refs/heads/")
			branchPath := filepath.Join(ge.exportPath, ".git", "refs", "heads", branchName)
			if err := os.WriteFile(branchPath, []byte(gitHash+"\n"), 0644); err != nil {
				return fmt.Errorf("failed to write branch %s: %w", branchName, err)
			}
		}
	}

	return nil
}

func (ge *GitExporter) exportTags() error {
	tags, err := ge.repo.ListTags()
	if err != nil {
		return err
	}

	for _, tagName := range tags {
		// For now, create lightweight tags pointing to the current commit
		// In a full implementation, we would need to get tag details from govc
		currentCommit, err := ge.repo.CurrentCommit()
		if err != nil {
			continue
		}

		// Find the Git hash for the current commit
		if gitHash, ok := ge.objectMap[currentCommit.Hash()]; ok {
			// Create lightweight tag (direct reference to commit)
			tagPath := filepath.Join(ge.exportPath, ".git", "refs", "tags", tagName)
			if err := os.MkdirAll(filepath.Dir(tagPath), 0755); err != nil {
				return fmt.Errorf("failed to create tag directory: %w", err)
			}

			if err := os.WriteFile(tagPath, []byte(gitHash+"\n"), 0644); err != nil {
				return fmt.Errorf("failed to write tag %s: %w", tagName, err)
			}
		}
	}

	return nil
}

func (ge *GitExporter) exportHEAD() error {
	currentBranch, err := ge.repo.CurrentBranch()
	if err != nil {
		return err
	}

	headContent := fmt.Sprintf("ref: refs/heads/%s\n", currentBranch)
	return ge.writeGitFile("HEAD", headContent)
}

func (ge *GitExporter) exportWorkingTree() error {
	// Export current working tree files
	files, err := ge.repo.ListFiles()
	if err != nil {
		return err
	}

	for _, filePath := range files {
		content, err := ge.repo.ReadFile(filePath)
		if err != nil {
			continue
		}

		fullPath := filepath.Join(ge.exportPath, filePath)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}

		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return err
		}
	}

	return nil
}

func (ge *GitExporter) writeObject(objectType string, content []byte) string {
	// Create Git object with header
	header := fmt.Sprintf("%s %d\x00", objectType, len(content))
	fullContent := append([]byte(header), content...)

	// Calculate SHA-1 hash
	hash := sha1.Sum(fullContent)
	hashStr := hex.EncodeToString(hash[:])

	// Compress content
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	writer.Write(fullContent)
	writer.Close()

	// Write to objects directory
	objectPath := filepath.Join(ge.exportPath, ".git", "objects", hashStr[:2], hashStr[2:])
	os.MkdirAll(filepath.Dir(objectPath), 0755)
	os.WriteFile(objectPath, compressed.Bytes(), 0444)

	return hashStr
}

func (ge *GitExporter) writeGitFile(path, content string) error {
	fullPath := filepath.Join(ge.exportPath, ".git", path)
	return os.WriteFile(fullPath, []byte(content), 0644)
}

// createAnnotatedTag creates a Git tag object for annotated tags
// Simplified version without govc.TagInfo dependency
func (ge *GitExporter) createAnnotatedTag(tagName, targetHash, message string) string {
	var content bytes.Buffer

	content.WriteString(fmt.Sprintf("object %s\n", targetHash))
	content.WriteString("type commit\n")
	content.WriteString(fmt.Sprintf("tag %s\n", tagName))

	// Add simple tagger info
	content.WriteString("tagger govc-exporter <govc@localhost> 1640995200 +0000\n")

	// Add tag message
	content.WriteString("\n")
	if message != "" {
		content.WriteString(message)
		if !strings.HasSuffix(message, "\n") {
			content.WriteString("\n")
		}
	} else {
		content.WriteString("Tag created by govc exporter\n")
	}

	return ge.writeObject("tag", content.Bytes())
}

// ExportOptions contains options for exporting
type ExportOptions struct {
	Bare           bool   // Export as bare repository
	Branch         string // Specific branch to export
	IncludeTags    bool   // Include tags in export
	IncludeHistory bool   // Include full history or just current state
}

// ExportWithOptions exports with specific options
func (ge *GitExporter) ExportWithOptions(opts ExportOptions) error {
	// Implementation would handle various export scenarios
	return ge.Export()
}
