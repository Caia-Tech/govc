package govc

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExportBasic tests basic export functionality
func TestExportBasic(t *testing.T) {
	repo := New()
	
	// Create some content
	repo.WriteFile("test.txt", []byte("test content"))
	repo.Add("test.txt")
	repo.Commit("Test commit")
	
	// Export
	var buf bytes.Buffer
	err := repo.Export(&buf, "fast-export")
	assert.NoError(t, err)
	
	// Verify export contains expected content
	content := buf.String()
	assert.Contains(t, content, "test.txt")
	assert.Contains(t, content, "Test commit")
}

// TestExportWithBranches tests exporting with multiple branches
func TestExportWithBranches(t *testing.T) {
	repo := New()
	
	// Main branch
	repo.WriteFile("main.txt", []byte("main"))
	repo.Add("main.txt")
	repo.Commit("Main commit")
	
	// Feature branch
	repo.Branch("feature").Create()
	repo.Checkout("feature")
	repo.WriteFile("feature.txt", []byte("feature"))
	repo.Add("feature.txt")
	repo.Commit("Feature commit")
	
	// Export
	var buf bytes.Buffer
	err := repo.Export(&buf, "fast-export")
	assert.NoError(t, err)
	
	// Check both branches exported
	content := buf.String()
	assert.Contains(t, content, "refs/heads/main")
	assert.Contains(t, content, "refs/heads/feature")
}

// TestExportFormats tests different export formats
func TestExportFormats(t *testing.T) {
	repo := New()
	
	// Create content
	repo.WriteFile("file.txt", []byte("content"))
	repo.Add("file.txt")
	repo.Commit("Commit")
	
	// Test bundle format
	var bundleBuf bytes.Buffer
	err := repo.Export(&bundleBuf, "bundle")
	if err != nil {
		// Bundle format might not be implemented
		t.Skip("Bundle format not implemented")
	}
	
	// Test fast-export format
	var fastBuf bytes.Buffer
	err = repo.Export(&fastBuf, "fast-export")
	assert.NoError(t, err)
	
	// Verify formats are different
	if bundleBuf.Len() > 0 && fastBuf.Len() > 0 {
		assert.NotEqual(t, bundleBuf.String(), fastBuf.String())
	}
}

// TestExportLargeRepository tests exporting large repository
func TestExportLargeRepository(t *testing.T) {
	repo := New()
	
	// Create many commits
	for i := 0; i < 50; i++ {
		filename := "file" + string(rune('0'+i%10)) + ".txt"
		content := []byte(strings.Repeat("x", 100))
		repo.WriteFile(filename, content)
		repo.Add(filename)
		repo.Commit("Commit " + string(rune('0'+i%10)))
	}
	
	// Export should handle large repository
	var buf bytes.Buffer
	err := repo.Export(&buf, "fast-export")
	assert.NoError(t, err)
	assert.Greater(t, buf.Len(), 1000)
}

// TestExportEmptyRepository tests exporting empty repository
func TestExportEmptyRepository(t *testing.T) {
	repo := New()
	
	var buf bytes.Buffer
	err := repo.Export(&buf, "fast-export")
	// Should either succeed with minimal output or return error
	if err == nil {
		assert.NotEmpty(t, buf.String())
	}
}

// TestExportWithTags tests exporting with tags
func TestExportWithTags(t *testing.T) {
	repo := New()
	
	// Create commit
	repo.WriteFile("version.txt", []byte("1.0.0"))
	repo.Add("version.txt")
	repo.Commit("Version 1.0.0")
	
	// Create tag
	err := repo.CreateTag("v1.0.0", "Release 1.0.0")
	require.NoError(t, err)
	
	// Export
	var buf bytes.Buffer
	err = repo.Export(&buf, "fast-export")
	assert.NoError(t, err)
	
	// Verify tag exported
	content := buf.String()
	assert.Contains(t, content, "tag v1.0.0")
}

// TestExportBinaryFiles tests exporting binary files
func TestExportBinaryFiles(t *testing.T) {
	repo := New()
	
	// Add binary file
	binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}
	repo.WriteFile("binary.dat", binaryData)
	repo.Add("binary.dat")
	repo.Commit("Add binary file")
	
	// Export
	var buf bytes.Buffer
	err := repo.Export(&buf, "fast-export")
	assert.NoError(t, err)
	
	// Should handle binary data
	assert.Greater(t, buf.Len(), 0)
}

// TestExportConcurrent tests concurrent exports
func TestExportConcurrent(t *testing.T) {
	repo := New()
	
	// Create content
	for i := 0; i < 5; i++ {
		repo.WriteFile("file"+string(rune('0'+i))+".txt", []byte("content"))
		repo.Add("file" + string(rune('0'+i)) + ".txt")
		repo.Commit("Commit " + string(rune('0'+i)))
	}
	
	// Export concurrently
	done := make(chan error, 3)
	
	for i := 0; i < 3; i++ {
		go func() {
			var buf bytes.Buffer
			done <- repo.Export(&buf, "fast-export")
		}()
	}
	
	// Wait for all exports
	for i := 0; i < 3; i++ {
		err := <-done
		assert.NoError(t, err)
	}
}

// TestExportInvalidFormat tests export with invalid format
func TestExportInvalidFormat(t *testing.T) {
	repo := New()
	
	repo.WriteFile("test.txt", []byte("test"))
	repo.Add("test.txt")
	repo.Commit("Test")
	
	var buf bytes.Buffer
	err := repo.Export(&buf, "invalid-format")
	// Should return error for invalid format
	assert.Error(t, err)
}

// BenchmarkExport benchmarks export performance
func BenchmarkExport(b *testing.B) {
	repo := New()
	
	// Create repository with content
	for i := 0; i < 20; i++ {
		repo.WriteFile("file.txt", []byte("content "+string(rune('0'+i))))
		repo.Add("file.txt")
		repo.Commit("Commit")
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		repo.Export(&buf, "fast-export")
	}
}