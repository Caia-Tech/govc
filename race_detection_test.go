package govc

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestRaceCondition_StagingAreaAccess tests for race conditions in StagingArea
func TestRaceCondition_StagingAreaAccess(t *testing.T) {
	repo := NewRepository()
	numGoroutines := 10
	operationsPerGoroutine := 50

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*operationsPerGoroutine)

	// Multiple goroutines accessing the same repository's staging area
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				// This should trigger race conditions in the original staging area
				path := fmt.Sprintf("file_%d_%d.txt", goroutineID, j)
				content := []byte(fmt.Sprintf("content %d %d", goroutineID, j))

				// Store blob (this might race)
				hash, err := repo.store.StoreBlob(content)
				if err != nil {
					errors <- fmt.Errorf("store blob failed: %w", err)
					continue
				}

				// Add to staging (this is where the race should be)
				repo.staging.Add(path, hash)

				// Try to read back immediately (potential race)
				files := repo.staging.GetFiles()
				if _, exists := files[path]; !exists {
					errors <- fmt.Errorf("file not found after adding: %s", path)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Report any errors found
	errorCount := 0
	for err := range errors {
		t.Logf("Race condition error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Logf("Found %d race condition errors", errorCount)
	} else {
		t.Logf("No race condition errors detected")
	}
}

// TestRaceCondition_RepositoryCommit tests for race conditions in commit operations
func TestRaceCondition_RepositoryCommit(t *testing.T) {
	repo := NewRepository()
	numGoroutines := 5
	commitsPerGoroutine := 10

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*commitsPerGoroutine)

	// Multiple goroutines trying to commit simultaneously
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < commitsPerGoroutine; j++ {
				// Add a file and commit
				path := fmt.Sprintf("commit_file_%d_%d.txt", goroutineID, j)
				content := []byte(fmt.Sprintf("commit content %d %d", goroutineID, j))

				// Write file first, then add to staging
				err := repo.WriteFile(path, content)
				if err != nil {
					errors <- fmt.Errorf("write file failed: %w", err)
					continue
				}
				
				// Add to repository (stages the file)
				err = repo.Add(path)
				if err != nil {
					errors <- fmt.Errorf("add failed: %w", err)
					continue
				}

				// This should trigger race conditions in createTreeFromStaging
				_, err = repo.Commit(fmt.Sprintf("Commit %d_%d", goroutineID, j))
				if err != nil {
					errors <- fmt.Errorf("commit failed: %w", err)
					continue
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Report any errors found
	errorCount := 0
	for err := range errors {
		t.Logf("Commit race condition error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Logf("Found %d commit race condition errors", errorCount)
	}
}

// TestRaceCondition_MixedOperations tests mixed read/write operations for races
func TestRaceCondition_MixedOperations(t *testing.T) {
	repo := NewRepository()
	duration := 2 * time.Second
	
	var wg sync.WaitGroup
	done := make(chan struct{})
	errors := make(chan error, 1000)

	// Start timer
	go func() {
		time.Sleep(duration)
		close(done)
	}()

	// Writer goroutines
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			count := 0
			for {
				select {
				case <-done:
					t.Logf("Writer %d completed %d operations", writerID, count)
					return
				default:
					path := fmt.Sprintf("writer_%d_file_%d.txt", writerID, count)
					content := []byte(fmt.Sprintf("writer %d content %d", writerID, count))
					
					// Write file first, then add to staging
					err := repo.WriteFile(path, content)
					if err != nil {
						errors <- fmt.Errorf("writer %d write failed: %w", writerID, err)
						continue
					}
					
					err = repo.Add(path)
					if err != nil {
						errors <- fmt.Errorf("writer %d add failed: %w", writerID, err)
						continue
					}
					count++
				}
			}
		}(i)
	}

	// Reader goroutines
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			count := 0
			for {
				select {
				case <-done:
					t.Logf("Reader %d completed %d operations", readerID, count)
					return
				default:
					// Try to read staging area
					files := repo.staging.GetFiles()
					if len(files) > 0 {
						// Try to get status (involves reading staging area)
						_, err := repo.Status()
						if err != nil {
							errors <- fmt.Errorf("reader %d status failed: %w", readerID, err)
						}
					}
					count++
				}
			}
		}(i)
	}

	// Committer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		count := 0
		for {
			select {
			case <-done:
				t.Logf("Committer completed %d operations", count)
				return
			default:
				// Try to commit if there are staged files
				files := repo.staging.GetFiles()
				if len(files) > 0 {
					_, err := repo.Commit(fmt.Sprintf("Commit %d", count))
					if err != nil {
						errors <- fmt.Errorf("committer commit failed: %w", err)
					} else {
						count++
					}
				}
				time.Sleep(10 * time.Millisecond) // Don't commit too aggressively
			}
		}
	}()

	wg.Wait()
	close(errors)

	// Report any errors found
	errorCount := 0
	for err := range errors {
		t.Logf("Mixed operations race error: %v", err)
		errorCount++
	}

	assert.Equal(t, 0, errorCount, "Should have no race condition errors")
}

// TestRaceCondition_RepositoryFields tests access to repository fields
func TestRaceCondition_RepositoryFields(t *testing.T) {
	repo := NewRepository()
	numGoroutines := 5
	operationsPerGoroutine := 20

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*operationsPerGoroutine)

	// Multiple goroutines accessing repository fields
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				// Access various repository fields that might race
				
				// Access staging through safe methods
				files := repo.staging.GetFiles()

				// This might race with commits clearing the staging area
				if len(files) > 0 {
					_, err := repo.Status()
					if err != nil {
						errors <- fmt.Errorf("status error: %w", err)
					}
				}

				// Add something to potentially cause a race
				path := fmt.Sprintf("race_test_%d_%d.txt", goroutineID, j)
				content := []byte("test content")
				
				err := repo.WriteFile(path, content)
				if err != nil {
					errors <- fmt.Errorf("write error: %w", err)
					continue
				}
				
				err = repo.Add(path)
				if err != nil {
					errors <- fmt.Errorf("add error: %w", err)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Report any errors found
	errorCount := 0
	for err := range errors {
		t.Logf("Repository field race error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Found %d repository field race condition errors", errorCount)
	}
}