package main

import (
	"testing"
	"os"
	"path/filepath"
	"crypto/sha256"
	"fmt"
	
	"github.com/Caia-Tech/govc/internal/repository"
)

func TestRepositoryCreation(t *testing.T) {
	repo := repository.New()
	if repo == nil {
		t.Fatal("New() returned nil")
	}
	
	// Test that we can get the staging area (public method)
	staging := repo.GetStagingArea()
	if staging == nil {
		t.Error("New() did not initialize staging area properly")
	}
}

func TestStagingAreaOperations(t *testing.T) {
	staging := repository.NewStagingArea()
	
	testContent := []byte("test content")
	err := staging.Add("test.txt", testContent)
	if err != nil {
		t.Fatalf("Add() failed: %v", err)
	}
	
	// Check if file was added
	files, err := staging.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	
	found := false
	for _, file := range files {
		if file == "test.txt" {
			found = true
			break
		}
	}
	
	if !found {
		t.Error("File was not added to staging area")
	}
	
	// Check if file is staged
	if !staging.IsStaged("test.txt") {
		t.Error("IsStaged() returned false for added file")
	}
	
	// Check hash
	expectedHash := fmt.Sprintf("%x", sha256.Sum256(testContent))
	hash, err := staging.GetFileHash("test.txt")
	if err != nil {
		t.Fatalf("GetFileHash() failed: %v", err)
	}
	
	if hash != expectedHash {
		t.Errorf("Hash mismatch: got %s, want %s", hash, expectedHash)
	}
}

func TestStagingAreaList(t *testing.T) {
	staging := repository.NewStagingArea()
	
	// Empty staging area
	files, err := staging.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	
	if len(files) != 0 {
		t.Errorf("Expected empty list, got %d files", len(files))
	}
	
	// Add files
	staging.Add("file1.txt", []byte("content1"))
	staging.Add("file2.txt", []byte("content2"))
	
	files, err = staging.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	
	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}
}

func TestStagingAreaPersistence(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()
	
	// Create staging area with path
	staging := repository.NewStagingAreaWithPath(tempDir)
	
	// Add files
	err := staging.Add("persistent.txt", []byte("persistent content"))
	if err != nil {
		t.Fatalf("Add() failed: %v", err)
	}
	
	// Save to disk
	err = staging.Save(tempDir)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}
	
	// Check if staging.json exists
	stagingFile := filepath.Join(tempDir, ".govc", "staging.json")
	if _, err := os.Stat(stagingFile); os.IsNotExist(err) {
		t.Error("staging.json was not created")
	}
	
	// Load new staging area
	newStaging := repository.NewStagingAreaWithPath(tempDir)
	err = newStaging.Load(tempDir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	
	// Check if file is still staged
	if !newStaging.IsStaged("persistent.txt") {
		t.Error("File was not loaded from persistent storage")
	}
}

func TestLoadRepository(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()
	
	// Load repository
	repo, err := repository.LoadRepository(tempDir)
	if err != nil {
		t.Fatalf("LoadRepository() failed: %v", err)
	}
	
	if repo == nil {
		t.Fatal("LoadRepository() returned nil")
	}
	
	// Check .govc directory was created
	govcDir := filepath.Join(tempDir, ".govc")
	if _, err := os.Stat(govcDir); os.IsNotExist(err) {
		t.Error(".govc directory was not created")
	}
	
	// Test adding a file and persistence
	staging := repo.GetStagingArea()
	err = staging.Add("test.txt", []byte("test content"))
	if err != nil {
		t.Fatalf("Add() failed: %v", err)
	}
	
	// Load same repository again
	repo2, err := repository.LoadRepository(tempDir)
	if err != nil {
		t.Fatalf("Second LoadRepository() failed: %v", err)
	}
	
	// Check if file is still staged
	staging2 := repo2.GetStagingArea()
	if !staging2.IsStaged("test.txt") {
		t.Error("File was not persisted across repository loads")
	}
}

func TestRepositoryStatus(t *testing.T) {
	repo := repository.New()
	
	// Add files to staging
	staging := repo.GetStagingArea()
	staging.Add("staged1.txt", []byte("content1"))
	staging.Add("staged2.txt", []byte("content2"))
	
	status, err := repo.Status()
	if err != nil {
		t.Fatalf("Status() failed: %v", err)
	}
	
	if status == nil {
		t.Fatal("Status() returned nil")
	}
	
	if status.Branch != "main" {
		t.Errorf("Expected branch 'main', got '%s'", status.Branch)
	}
	
	if len(status.Staged) != 2 {
		t.Errorf("Expected 2 staged files, got %d", len(status.Staged))
	}
}

func TestStagingAreaEdgeCases(t *testing.T) {
	staging := repository.NewStagingArea()
	
	// Test non-existent file
	if staging.IsStaged("nonexistent.txt") {
		t.Error("IsStaged() returned true for non-existent file")
	}
	
	// Test getting hash of non-existent file
	_, err := staging.GetFileHash("nonexistent.txt")
	if err == nil {
		t.Error("GetFileHash() should have failed for non-existent file")
	}
}