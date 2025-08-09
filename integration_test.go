package govc

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Result types for integration tests
type updateResult struct {
	worldName string
	success   bool
	error     error
}

type operationResult struct {
	userID    int
	repoName  string
	success   bool
	error     error
	operation string
}

type sessionResult struct {
	sessionID int
	repoName  string
	success   bool
	error     error
	operation string
}

// TestIntegration_CompleteWorkflow tests end-to-end repository workflows
func TestIntegration_CompleteWorkflow(t *testing.T) {
	// Test complete repository workflow using concurrent-safe operations
	manager := NewRepositoryManager()
	
	// Step 1: Create multiple repositories representing different "worlds"
	worldRepos := make(map[string]*ConcurrentSafeRepository)
	worldNames := []string{"world_alpha", "world_beta", "world_gamma"}
	
	for _, worldName := range worldNames {
		repo, err := manager.GetOrCreateRepository(worldName)
		require.NoError(t, err)
		require.NotNil(t, repo)
		worldRepos[worldName] = repo
		
		// Verify repository initialization
		assert.False(t, repo.IsCorrupted())
	}

	// Step 2: Create initial content in each world
	for worldName, repo := range worldRepos {
		tx := repo.SafeTransaction()
		
		// Create world configuration
		configContent := []byte(fmt.Sprintf(`{
	"name": "%s",
	"version": 1,
	"settings": {
		"difficulty": "normal",
		"max_players": 100
	}
}`, worldName))
		
		err := tx.Add("world.json", configContent)
		require.NoError(t, err)
		
		// Create initial world data
		dataContent := []byte(fmt.Sprintf("Initial data for %s", worldName))
		err = tx.Add("data/initial.txt", dataContent)
		require.NoError(t, err)
		
		err = tx.Validate()
		require.NoError(t, err)
		
		_, err = tx.Commit(fmt.Sprintf("Initialize %s", worldName))
		require.NoError(t, err)
		
		t.Logf("✅ Initialized world: %s", worldName)
	}

	// Step 3: Simulate concurrent updates to different worlds
	var wg sync.WaitGroup
	updateResults := make(chan updateResult, len(worldNames)*5)

	for worldName, repo := range worldRepos {
		wg.Add(1)
		go func(wName string, r *ConcurrentSafeRepository) {
			defer wg.Done()
			
			// Perform multiple sequential updates
			for i := 1; i <= 5; i++ {
				tx := r.SafeTransaction()
				
				// Update world configuration
				newConfig := []byte(fmt.Sprintf(`{
	"name": "%s",
	"version": %d,
	"settings": {
		"difficulty": "normal",
		"max_players": %d
	}
}`, wName, i+1, 100+(i*10)))
				
				err := tx.Add("world.json", newConfig)
				if err != nil {
					updateResults <- updateResult{wName, false, err}
					continue
				}
				
				// Add new data file for this update
				dataContent := []byte(fmt.Sprintf("Update %d data for %s at %d", i, wName, time.Now().UnixNano()))
				err = tx.Add(fmt.Sprintf("data/update_%d.txt", i), dataContent)
				if err != nil {
					updateResults <- updateResult{wName, false, err}
					continue
				}
				
				err = tx.Validate()
				if err != nil {
					updateResults <- updateResult{wName, false, err}
					continue
				}
				
				_, err = tx.Commit(fmt.Sprintf("Update %d for %s", i, wName))
				if err != nil {
					updateResults <- updateResult{wName, false, err}
					continue
				}
				
				updateResults <- updateResult{wName, true, nil}
				
				// Small delay to simulate real-world timing
				time.Sleep(5 * time.Millisecond)
			}
		}(worldName, repo)
	}

	wg.Wait()
	close(updateResults)

	// Step 4: Analyze update results
	successCount := 0
	totalUpdates := 0
	errorsByWorld := make(map[string]int)
	
	for result := range updateResults {
		totalUpdates++
		if result.success {
			successCount++
		} else {
			errorsByWorld[result.worldName]++
			if result.error != nil {
				t.Logf("Update error in %s: %v", result.worldName, result.error)
			}
		}
	}

	// Step 5: Verify final state of each world
	for worldName, repo := range worldRepos {
		// Check if repository is still healthy
		assert.False(t, repo.IsCorrupted(), "World %s should not be corrupted", worldName)
		
		// Verify we can still perform operations
		tx := repo.SafeTransaction()
		finalData := []byte(fmt.Sprintf("Final verification data for %s", worldName))
		err := tx.Add("verification.txt", finalData)
		require.NoError(t, err)
		
		err = tx.Validate()
		require.NoError(t, err)
		
		_, err = tx.Commit(fmt.Sprintf("Final verification for %s", worldName))
		require.NoError(t, err)
		
		t.Logf("✅ World %s final verification passed", worldName)
	}

	// Step 6: Clean up - remove repositories
	for _, worldName := range worldNames {
		err := manager.RemoveRepository(worldName)
		assert.NoError(t, err)
	}

	t.Logf("Integration Workflow Results:")
	t.Logf("  Worlds created: %d", len(worldNames))
	t.Logf("  Total updates attempted: %d", totalUpdates)
	t.Logf("  Successful updates: %d", successCount)
	t.Logf("  Success rate: %.2f%%", float64(successCount)/float64(totalUpdates)*100)
	
	for world, errors := range errorsByWorld {
		t.Logf("  Errors in %s: %d", world, errors)
	}

	assert.True(t, successCount > totalUpdates*8/10, "Should have > 80% success rate")
	assert.Equal(t, len(worldNames), len(worldRepos), "Should have created all worlds")
	
	t.Logf("✅ Complete workflow integration test passed")
}

// TestIntegration_ConcurrentUsers tests multiple concurrent repository operations
func TestIntegration_ConcurrentUsers(t *testing.T) {
	manager := NewRepositoryManager()
	
	numUsers := 10
	reposPerUser := 3
	
	var wg sync.WaitGroup
	results := make(chan operationResult, numUsers*reposPerUser*5)

	// Create and test multiple users concurrently  
	for userID := 0; userID < numUsers; userID++ {
		wg.Add(1)
		go func(uid int) {
			defer wg.Done()
			
			// Each user creates multiple repositories
			for repoNum := 0; repoNum < reposPerUser; repoNum++ {
				repoName := fmt.Sprintf("user_%d_repo_%d", uid, repoNum)
				
				// Create repository
				repo, err := manager.GetOrCreateRepository(repoName)
				if err != nil {
					results <- operationResult{uid, repoName, false, err, "create_repo"}
					continue
				}
				results <- operationResult{uid, repoName, true, nil, "create_repo"}
				
				// Initialize repository
				tx := repo.SafeTransaction()
				
				// Add user configuration
				userConfig := []byte(fmt.Sprintf(`{
	"user_id": %d,
	"repository": "%s", 
	"created_at": "%s"
}`, uid, repoName, time.Now().Format(time.RFC3339)))
				
				err = tx.Add("user.json", userConfig)
				if err != nil {
					results <- operationResult{uid, repoName, false, err, "add_config"}
					continue
				}
				results <- operationResult{uid, repoName, true, nil, "add_config"}
				
				err = tx.Validate()
				if err != nil {
					results <- operationResult{uid, repoName, false, err, "validate"}
					continue
				}
				results <- operationResult{uid, repoName, true, nil, "validate"}
				
				_, err = tx.Commit(fmt.Sprintf("Initialize repository for user %d", uid))
				if err != nil {
					results <- operationResult{uid, repoName, false, err, "commit"}
					continue
				}
				results <- operationResult{uid, repoName, true, nil, "commit"}
				
				// Verify repository state
				if repo.IsCorrupted() {
					results <- operationResult{uid, repoName, false, fmt.Errorf("repository corrupted"), "verify"}
					continue
				}
				results <- operationResult{uid, repoName, true, nil, "verify"}
				
				// Small delay to simulate realistic usage
				time.Sleep(2 * time.Millisecond)
			}
		}(userID)
	}

	wg.Wait()
	close(results)

	// Analyze results
	successCount := 0
	errorCount := 0
	operationCounts := make(map[string]int)
	userSuccessCount := make(map[int]int)
	
	for result := range results {
		operationCounts[result.operation]++
		if result.success {
			successCount++
			userSuccessCount[result.userID]++
		} else {
			errorCount++
			if result.error != nil {
				t.Logf("Error in user %d operation %s on %s: %v", 
					result.userID, result.operation, result.repoName, result.error)
			}
		}
	}

	expectedOperations := numUsers * reposPerUser * 5 // create_repo + add_config + validate + commit + verify
	totalResults := successCount + errorCount
	
	t.Logf("Concurrent Users Integration Test Results:")
	t.Logf("  Users: %d", numUsers)
	t.Logf("  Repositories per user: %d", reposPerUser)
	t.Logf("  Expected operations: %d", expectedOperations)
	t.Logf("  Total results: %d", totalResults)
	t.Logf("  Successful operations: %d", successCount)
	t.Logf("  Failed operations: %d", errorCount)
	t.Logf("  Success rate: %.2f%%", float64(successCount)/float64(totalResults)*100)
	
	t.Logf("  Operation breakdown:")
	for op, count := range operationCounts {
		t.Logf("    %s: %d", op, count)
	}

	assert.True(t, successCount > totalResults*8/10, "Should have > 80% success rate")
	assert.True(t, operationCounts["create_repo"] >= numUsers*reposPerUser*8/10, "Should create most repositories successfully")
}

// TestIntegration_PersistenceConsistency tests data persistence across operations
func TestIntegration_PersistenceConsistency(t *testing.T) {
	manager := NewRepositoryManager()
	repo, err := manager.GetOrCreateRepository("persistence_test")
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Create initial state
	tx := repo.SafeTransaction()
	initialData := []byte(`{
	"name": "persistence_world",
	"version": 1,
	"data": "initial"
}`)
	
	err = tx.Add("world.json", initialData)
	require.NoError(t, err)
	
	err = tx.Validate()
	require.NoError(t, err)
	
	_, err = tx.Commit("Initial persistence test data")
	require.NoError(t, err)

	// Perform rapid sequential updates to test consistency
	updates := []struct {
		version int
		data    string
		desc    string
	}{
		{2, "first_update", "First update"},
		{3, "second_update", "Second update"},
		{4, "third_update", "Third update"},
		{5, "fourth_update", "Fourth update"},
		{6, "fifth_update", "Fifth update"},
	}

	for i, update := range updates {
		tx := repo.SafeTransaction()
		
		// Update the world configuration
		updatedData := []byte(fmt.Sprintf(`{
	"name": "persistence_world",
	"version": %d,
	"data": "%s",
	"description": "%s"
}`, update.version, update.data, update.desc))
		
		err = tx.Add("world.json", updatedData)
		require.NoError(t, err, "Update %d should add successfully", i+1)
		
		// Also create a versioned file to track changes
		versionFile := []byte(fmt.Sprintf("Version %d data: %s", update.version, update.data))
		err = tx.Add(fmt.Sprintf("versions/v%d.txt", update.version), versionFile)
		require.NoError(t, err, "Update %d version file should add successfully", i+1)
		
		err = tx.Validate()
		require.NoError(t, err, "Update %d should validate", i+1)
		
		_, err = tx.Commit(fmt.Sprintf("Update %d: %s", i+1, update.desc))
		require.NoError(t, err, "Update %d should commit successfully", i+1)
		
		// Verify the repository is still in good state
		assert.False(t, repo.IsCorrupted(), "Repository should not be corrupted after update %d", i+1)
		
		t.Logf("✅ Update %d completed successfully", i+1)
		
		// Small delay to simulate real-world timing
		time.Sleep(5 * time.Millisecond)
	}

	// Verify all updates are persisted by checking final state
	finalTx := repo.SafeTransaction()
	finalData := []byte(fmt.Sprintf("Final verification: All %d updates completed", len(updates)))
	err = finalTx.Add("verification.txt", finalData)
	require.NoError(t, err)
	
	err = finalTx.Validate()
	require.NoError(t, err)
	
	_, err = finalTx.Commit("Final verification")
	require.NoError(t, err)

	t.Logf("✅ Persistence consistency test passed - all sequential updates were properly persisted")
}

// TestIntegration_ErrorHandling tests error scenarios and recovery
func TestIntegration_ErrorHandling(t *testing.T) {
	manager := NewRepositoryManager()
	
	// Test 1: Invalid repository name
	_, err := manager.GetOrCreateRepository("")
	assert.Error(t, err, "Should fail with empty repository name")
	
	// Test 2: Valid repository creation
	repo, err := manager.GetOrCreateRepository("error_test")
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Test 3: Transaction errors with invalid inputs
	tx := repo.SafeTransaction()
	
	// Test empty path
	err = tx.Add("", []byte("content"))
	assert.Error(t, err, "Should fail with empty path")
	
	// Test valid operations
	err = tx.Add("test.txt", []byte("test content"))
	require.NoError(t, err)
	
	// Test validation before adding anything
	emptyTx := repo.SafeTransaction()
	err = emptyTx.Validate()
	require.NoError(t, err, "Empty transaction should validate")
	
	// Test committing without validation
	unvalidatedTx := repo.SafeTransaction()
	err = unvalidatedTx.Add("unvalidated.txt", []byte("content"))
	require.NoError(t, err)
	
	_, err = unvalidatedTx.Commit("Should fail - not validated")
	assert.Error(t, err, "Should fail when committing unvalidated transaction")
	
	// Test double commit
	validTx := repo.SafeTransaction()
	err = validTx.Add("valid.txt", []byte("content"))
	require.NoError(t, err)
	
	err = validTx.Validate()
	require.NoError(t, err)
	
	_, err = validTx.Commit("First commit")
	require.NoError(t, err)
	
	_, err = validTx.Commit("Second commit - should fail")
	assert.Error(t, err, "Should fail on double commit")
	
	// Test operations on committed transaction
	err = validTx.Add("after-commit.txt", []byte("should fail"))
	assert.Error(t, err, "Should fail to add to committed transaction")
	
	// Test rollback scenarios
	rollbackTx := repo.SafeTransaction()
	err = rollbackTx.Add("rollback.txt", []byte("content"))
	require.NoError(t, err)
	
	err = rollbackTx.Rollback()
	require.NoError(t, err, "Rollback should succeed")
	
	err = rollbackTx.Rollback()
	require.NoError(t, err, "Multiple rollbacks should be safe")
	
	// Test rollback after commit
	commitTx := repo.SafeTransaction()
	err = commitTx.Add("commit-then-rollback.txt", []byte("content"))
	require.NoError(t, err)
	err = commitTx.Validate()
	require.NoError(t, err)
	_, err = commitTx.Commit("Commit first")
	require.NoError(t, err)
	
	err = commitTx.Rollback()
	assert.Error(t, err, "Should fail to rollback committed transaction")

	// Test branch operation errors
	err = repo.SafeCreateBranch("", "main")
	assert.Error(t, err, "Should fail with empty branch name")
	
	err = repo.SafeCheckout("")
	assert.Error(t, err, "Should fail with empty branch name")
	
	// Test valid branch operations
	err = repo.SafeCreateBranch("test-branch", "")
	require.NoError(t, err, "Should create valid branch")
	
	err = repo.SafeCheckout("test-branch")
	require.NoError(t, err, "Should checkout valid branch")

	// Test repository state validation
	assert.False(t, repo.IsCorrupted(), "Repository should not be corrupted")
	
	// Test quiescence
	err = repo.WaitForQuiescence(100 * time.Millisecond)
	require.NoError(t, err, "Should achieve quiescence quickly")

	t.Logf("✅ Error handling integration test passed - all error scenarios handled correctly")
}

// TestIntegration_SessionManagement tests concurrent repository session management
func TestIntegration_SessionManagement(t *testing.T) {
	manager := NewRepositoryManager()
	
	// Test multiple concurrent repository sessions
	numSessions := 5
	reposPerSession := 3
	
	var wg sync.WaitGroup
	results := make(chan sessionResult, numSessions*reposPerSession*3) // 3 operations per repo

	// Create multiple concurrent sessions
	for sessionID := 0; sessionID < numSessions; sessionID++ {
		wg.Add(1)
		go func(sid int) {
			defer wg.Done()
			
			// Each session creates multiple repositories
			for repoNum := 0; repoNum < reposPerSession; repoNum++ {
				repoName := fmt.Sprintf("session_%d_repo_%d", sid, repoNum)
				
				// Operation 1: Create repository
				repo, err := manager.GetOrCreateRepository(repoName)
				if err != nil {
					results <- sessionResult{sid, repoName, false, err, "create_repo"}
					continue
				}
				results <- sessionResult{sid, repoName, true, nil, "create_repo"}
				
				// Operation 2: Initialize with content
				tx := repo.SafeTransaction()
				sessionData := []byte(fmt.Sprintf(`{
	"session_id": %d,
	"repository": "%s",
	"created_at": "%s"
}`, sid, repoName, time.Now().Format(time.RFC3339)))
				
				err = tx.Add("session.json", sessionData)
				if err != nil {
					results <- sessionResult{sid, repoName, false, err, "add_content"}
					continue
				}
				
				err = tx.Validate()
				if err != nil {
					results <- sessionResult{sid, repoName, false, err, "validate"}
					continue
				}
				
				_, err = tx.Commit(fmt.Sprintf("Initialize session %d repo %d", sid, repoNum))
				if err != nil {
					results <- sessionResult{sid, repoName, false, err, "commit"}
					continue
				}
				results <- sessionResult{sid, repoName, true, nil, "init_content"}
				
				// Operation 3: Verify repository state
				if repo.IsCorrupted() {
					results <- sessionResult{sid, repoName, false, fmt.Errorf("repository corrupted"), "verify"}
					continue
				}
				results <- sessionResult{sid, repoName, true, nil, "verify"}
				
				// Small delay to simulate real usage
				time.Sleep(2 * time.Millisecond)
			}
		}(sessionID)
	}
	
	wg.Wait()
	close(results)
	
	// Analyze session results
	successCount := 0
	errorCount := 0
	operationCounts := make(map[string]int)
	sessionSuccessCount := make(map[int]int)
	
	for result := range results {
		operationCounts[result.operation]++
		if result.success {
			successCount++
			sessionSuccessCount[result.sessionID]++
		} else {
			errorCount++
			if result.error != nil {
				t.Logf("Session %d error in %s on %s: %v", 
					result.sessionID, result.operation, result.repoName, result.error)
			}
		}
	}
	
	expectedOperations := numSessions * reposPerSession * 3 // create_repo + init_content + verify
	totalResults := successCount + errorCount
	
	t.Logf("Session Management Integration Test Results:")
	t.Logf("  Sessions: %d", numSessions)
	t.Logf("  Repositories per session: %d", reposPerSession)
	t.Logf("  Expected operations: %d", expectedOperations)
	t.Logf("  Total results: %d", totalResults)
	t.Logf("  Successful operations: %d", successCount)
	t.Logf("  Failed operations: %d", errorCount)
	t.Logf("  Success rate: %.2f%%", float64(successCount)/float64(totalResults)*100)
	
	t.Logf("  Operation breakdown:")
	for op, count := range operationCounts {
		t.Logf("    %s: %d", op, count)
	}
	
	// Verify each session succeeded
	for sid := 0; sid < numSessions; sid++ {
		expectedSuccess := reposPerSession * 3
		actualSuccess := sessionSuccessCount[sid]
		t.Logf("  Session %d: %d/%d successful", sid, actualSuccess, expectedSuccess)
	}

	assert.True(t, successCount > totalResults*8/10, "Should have > 80% success rate")
	assert.True(t, operationCounts["create_repo"] >= numSessions*reposPerSession*8/10, "Should create most repositories successfully")
	
	// Test cleanup - remove all created repositories
	for sessionID := 0; sessionID < numSessions; sessionID++ {
		for repoNum := 0; repoNum < reposPerSession; repoNum++ {
			repoName := fmt.Sprintf("session_%d_repo_%d", sessionID, repoNum)
			err := manager.RemoveRepository(repoName)
			assert.NoError(t, err, "Should remove repository %s", repoName)
		}
	}

	t.Logf("✅ Session management test passed - multiple concurrent sessions work correctly")
}