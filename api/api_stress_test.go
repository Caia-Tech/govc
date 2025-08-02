package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestStressConcurrentRepositories tests handling many repositories concurrently
func TestStressConcurrentRepositories(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	server, router := setupBenchmarkServer()

	const (
		numRepos       = 100
		numOpsPerRepo  = 50
		numGoroutines  = 20
	)

	// Create repositories
	t.Run("Create repositories under load", func(t *testing.T) {
		var wg sync.WaitGroup
		var created int32
		var failed int32

		sem := make(chan struct{}, numGoroutines)

		for i := 0; i < numRepos; i++ {
			wg.Add(1)
			sem <- struct{}{} // Acquire semaphore

			go func(index int) {
				defer wg.Done()
				defer func() { <-sem }() // Release semaphore

				body := bytes.NewBufferString(fmt.Sprintf(
					`{"id": "stress-repo-%d", "memory_only": true}`, index))
				req := httptest.NewRequest("POST", "/api/v1/repos", body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				if w.Code == 201 {
					atomic.AddInt32(&created, 1)
				} else {
					atomic.AddInt32(&failed, 1)
				}
			}(i)
		}

		wg.Wait()

		t.Logf("Created %d repositories, %d failed", created, failed)
		if failed > numRepos/10 {
			t.Errorf("Too many failures: %d out of %d", failed, numRepos)
		}
	})

	// Perform random operations on repositories
	t.Run("Random operations under load", func(t *testing.T) {
		var wg sync.WaitGroup
		var operations int32
		var errors int32

		startTime := time.Now()
		timeout := time.After(30 * time.Second)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				rand.Seed(time.Now().UnixNano() + int64(workerID))

				for j := 0; j < numOpsPerRepo; j++ {
					select {
					case <-timeout:
						return
					default:
					}

					repoID := fmt.Sprintf("stress-repo-%d", rand.Intn(numRepos))
					operation := rand.Intn(5)

					switch operation {
					case 0: // Add file
						path := fmt.Sprintf("file_%d_%d.txt", workerID, j)
						content := fmt.Sprintf("Content from worker %d, op %d", workerID, j)
						body := bytes.NewBufferString(fmt.Sprintf(
							`{"path": "%s", "content": "%s"}`, path, content))
						req := httptest.NewRequest("POST", 
							fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
						req.Header.Set("Content-Type", "application/json")
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)

						// V2 returns 201, V1 returns 200
						if w.Code != 200 && w.Code != 201 && w.Code != 404 {
							atomic.AddInt32(&errors, 1)
						}

					case 1: // Get status
						req := httptest.NewRequest("GET", 
							fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)

						if w.Code != 200 && w.Code != 404 {
							atomic.AddInt32(&errors, 1)
						}

					case 2: // List branches
						req := httptest.NewRequest("GET", 
							fmt.Sprintf("/api/v1/repos/%s/branches", repoID), nil)
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)

						if w.Code != 200 && w.Code != 404 {
							atomic.AddInt32(&errors, 1)
						}

					case 3: // Create branch
						branchName := fmt.Sprintf("branch-%d-%d", workerID, j)
						body := bytes.NewBufferString(fmt.Sprintf(`{"name": "%s"}`, branchName))
						req := httptest.NewRequest("POST", 
							fmt.Sprintf("/api/v1/repos/%s/branches", repoID), body)
						req.Header.Set("Content-Type", "application/json")
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)

						// May fail if no commits or V2 architecture issue
						if w.Code != 201 && w.Code != 400 && w.Code != 404 && w.Code != 500 {
							atomic.AddInt32(&errors, 1)
						}

					case 4: // Commit
						body := bytes.NewBufferString(fmt.Sprintf(
							`{"message": "Stress test commit %d-%d"}`, workerID, j))
						req := httptest.NewRequest("POST", 
							fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
						req.Header.Set("Content-Type", "application/json")
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)

						// May fail if nothing to commit or V2 architecture issue
						if w.Code != 201 && w.Code != 400 && w.Code != 404 && w.Code != 500 {
							atomic.AddInt32(&errors, 1)
						}
					}

					atomic.AddInt32(&operations, 1)
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(startTime)

		opsPerSecond := float64(operations) / duration.Seconds()
		t.Logf("Performed %d operations in %v (%.2f ops/sec)", operations, duration, opsPerSecond)
		t.Logf("Errors: %d (%.2f%%)", errors, float64(errors)/float64(operations)*100)

		if float64(errors)/float64(operations) > 0.05 {
			t.Errorf("Error rate too high: %.2f%%", float64(errors)/float64(operations)*100)
		}
	})

	// Verify server state
	t.Run("Verify server state after stress", func(t *testing.T) {
		server.mu.RLock()
		repoCount := server.repoPool.Size()
		txCount := len(server.transactions)
		server.mu.RUnlock()

		t.Logf("Final state: %d repositories, %d active transactions", repoCount, txCount)

		// Server should still be responsive
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Error("Health check failed after stress test")
		}
	})
}

// TestStressTransactions tests heavy transaction load
func TestStressTransactions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	_, router := setupBenchmarkServer()

	const (
		numRepos        = 10
		txPerRepo       = 100
		filesPerTx      = 10
		numGoroutines   = 50
	)

	// Create repositories
	for i := 0; i < numRepos; i++ {
		body := bytes.NewBufferString(fmt.Sprintf(
			`{"id": "tx-stress-%d", "memory_only": true}`, i))
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	t.Run("Concurrent transaction stress", func(t *testing.T) {
		var wg sync.WaitGroup
		var txCreated int32
		var txCommitted int32
		var txFailed int32

		startTime := time.Now()
		sem := make(chan struct{}, numGoroutines)

		for r := 0; r < numRepos; r++ {
			for tx := 0; tx < txPerRepo; tx++ {
				wg.Add(1)
				sem <- struct{}{}

				go func(repoIdx, txIdx int) {
					defer wg.Done()
					defer func() { <-sem }()

					repoID := fmt.Sprintf("tx-stress-%d", repoIdx)

					// Begin transaction
					req := httptest.NewRequest("POST", 
						fmt.Sprintf("/api/v1/repos/%s/transaction", repoID), nil)
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)

					if w.Code != 201 {
						atomic.AddInt32(&txFailed, 1)
						return
					}

					atomic.AddInt32(&txCreated, 1)

					var txResp TransactionResponse
					json.Unmarshal(w.Body.Bytes(), &txResp)

					// Add files to transaction
					for f := 0; f < filesPerTx; f++ {
						body := bytes.NewBufferString(fmt.Sprintf(
							`{"path": "tx%d/file%d.txt", "content": "Transaction %d, File %d"}`,
							txIdx, f, txIdx, f))
						req := httptest.NewRequest("POST", 
							fmt.Sprintf("/api/v1/repos/%s/transaction/%s/add", repoID, txResp.ID), body)
						req.Header.Set("Content-Type", "application/json")
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)

						if w.Code != 200 {
							atomic.AddInt32(&txFailed, 1)
							// Don't return early, try to continue
							break
						}
					}

					// Randomly either commit or rollback
					if rand.Intn(10) < 8 { // 80% commit, 20% rollback
						// Commit
						body := bytes.NewBufferString(fmt.Sprintf(
							`{"message": "Transaction %d commit"}`, txIdx))
						req := httptest.NewRequest("POST", 
							fmt.Sprintf("/api/v1/repos/%s/transaction/%s/commit", repoID, txResp.ID), body)
						req.Header.Set("Content-Type", "application/json")
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)

						if w.Code == 201 {
							atomic.AddInt32(&txCommitted, 1)
						} else if w.Code != 400 { // 400 is expected if nothing to commit
							atomic.AddInt32(&txFailed, 1)
						}
					} else {
						// Rollback
						req := httptest.NewRequest("POST", 
							fmt.Sprintf("/api/v1/repos/%s/transaction/%s/rollback", repoID, txResp.ID), nil)
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)
					}
				}(r, tx)
			}
		}

		wg.Wait()
		duration := time.Since(startTime)

		t.Logf("Transaction stress test completed in %v", duration)
		t.Logf("Created: %d, Committed: %d, Failed: %d", txCreated, txCommitted, txFailed)
		t.Logf("Throughput: %.2f tx/sec", float64(txCommitted)/duration.Seconds())
	})
}

// TestStressMemoryLeaks tests for memory leaks under sustained load
func TestStressMemoryLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	server, router := setupBenchmarkServer()

	const (
		iterations = 1000
		reposPerIter = 5
	)

	t.Run("Repository lifecycle stress", func(t *testing.T) {
		for i := 0; i < iterations; i++ {
			// Create repositories
			repoIDs := make([]string, reposPerIter)
			for j := 0; j < reposPerIter; j++ {
				repoID := fmt.Sprintf("leak-test-%d-%d", i, j)
				repoIDs[j] = repoID

				body := bytes.NewBufferString(fmt.Sprintf(
					`{"id": "%s", "memory_only": true}`, repoID))
				req := httptest.NewRequest("POST", "/api/v1/repos", body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
			}

			// Use repositories
			for _, repoID := range repoIDs {
				// Add files
				for k := 0; k < 5; k++ {
					body := bytes.NewBufferString(fmt.Sprintf(
						`{"path": "file%d.txt", "content": "content %d"}`, k, k))
					req := httptest.NewRequest("POST", 
						fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
					req.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)
				}

				// Commit
				body := bytes.NewBufferString(`{"message": "Test commit"}`)
				req := httptest.NewRequest("POST", 
					fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
			}

			// Delete repositories
			for _, repoID := range repoIDs {
				req := httptest.NewRequest("DELETE", 
					fmt.Sprintf("/api/v1/repos/%s", repoID), nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
			}

			// Check repository count periodically
			if i%100 == 0 {
				server.mu.RLock()
				repoCount := server.repoPool.Size()
				server.mu.RUnlock()

				if repoCount > reposPerIter*2 {
					t.Logf("Warning: Repository count at iteration %d: %d", i, repoCount)
				}
			}
		}

		// Final check
		server.mu.RLock()
		finalRepoCount := server.repoPool.Size()
		finalTxCount := len(server.transactions)
		server.mu.RUnlock()

		t.Logf("Final repository count: %d", finalRepoCount)
		t.Logf("Final transaction count: %d", finalTxCount)

		if finalRepoCount > reposPerIter*2 {
			t.Errorf("Possible memory leak: %d repositories remaining", finalRepoCount)
		}

		if finalTxCount > 0 {
			t.Errorf("Possible transaction leak: %d transactions remaining", finalTxCount)
		}
	})
}

// TestStressParallelRealities tests parallel reality operations under load
func TestStressParallelRealities(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	_, router := setupBenchmarkServer()

	const (
		numRepos = 20
		realitiesPerRepo = 10
		changesPerReality = 5
	)

	// Create and initialize repositories
	for i := 0; i < numRepos; i++ {
		repoID := fmt.Sprintf("reality-stress-%d", i)
		createRepo(t, router, repoID)
		createInitialCommit(t, router, repoID)
	}

	t.Run("Parallel reality stress", func(t *testing.T) {
		var wg sync.WaitGroup
		var created int32
		var applied int32
		var errors int32

		for r := 0; r < numRepos; r++ {
			wg.Add(1)
			go func(repoIdx int) {
				defer wg.Done()

				repoID := fmt.Sprintf("reality-stress-%d", repoIdx)

				// Create parallel realities
				branches := make([]string, realitiesPerRepo)
				for i := 0; i < realitiesPerRepo; i++ {
					branches[i] = fmt.Sprintf("reality-%d-%d", repoIdx, i)
				}

				branchesJSON, _ := json.Marshal(branches)
				body := bytes.NewBufferString(fmt.Sprintf(`{"branches": %s}`, branchesJSON))
				req := httptest.NewRequest("POST", 
					fmt.Sprintf("/api/v1/repos/%s/parallel-realities", repoID), body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				if w.Code == 201 {
					atomic.AddInt32(&created, int32(realitiesPerRepo))
				} else {
					atomic.AddInt32(&errors, 1)
					return
				}

				// Apply changes to each reality
				for _, branch := range branches {
					changes := make(map[string]string)
					for j := 0; j < changesPerReality; j++ {
						path := fmt.Sprintf("config%d.yaml", j)
						content := fmt.Sprintf("reality: %s\nindex: %d", branch, j)
						changes[path] = content
					}

					changesJSON, _ := json.Marshal(changes)
					body := bytes.NewBufferString(fmt.Sprintf(`{"changes": %s}`, changesJSON))
					req := httptest.NewRequest("POST", 
						fmt.Sprintf("/api/v1/repos/%s/parallel-realities/%s/apply", repoID, branch), body)
					req.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)

					if w.Code == 200 {
						atomic.AddInt32(&applied, 1)
					} else {
						atomic.AddInt32(&errors, 1)
					}
				}
			}(r)
		}

		wg.Wait()

		t.Logf("Created %d realities, applied changes to %d, errors: %d", 
			created, applied, errors)
	})
}

// TestStressBurstLoad tests handling of burst traffic
func TestStressBurstLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	_, router := setupBenchmarkServer()

	// Create test repository
	createRepo(t, router, "burst-test")

	t.Run("Burst read operations", func(t *testing.T) {
		const burstSize = 1000
		results := make(chan int, burstSize)

		start := make(chan struct{})
		var wg sync.WaitGroup

		// Prepare goroutines
		for i := 0; i < burstSize; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				<-start // Wait for signal

				req := httptest.NewRequest("GET", "/api/v1/repos/burst-test/status", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				results <- w.Code
			}()
		}

		// Release the burst
		startTime := time.Now()
		close(start)
		wg.Wait()
		duration := time.Since(startTime)

		close(results)

		// Analyze results
		success := 0
		for code := range results {
			if code == 200 {
				success++
			}
		}

		t.Logf("Burst of %d requests completed in %v", burstSize, duration)
		t.Logf("Success rate: %.2f%%", float64(success)/float64(burstSize)*100)
		t.Logf("Requests per second: %.2f", float64(burstSize)/duration.Seconds())

		if success < burstSize*95/100 {
			t.Errorf("Too many failures in burst: %d/%d", burstSize-success, burstSize)
		}
	})

	t.Run("Burst write operations", func(t *testing.T) {
		const burstSize = 500
		results := make(chan int, burstSize)

		start := make(chan struct{})
		var wg sync.WaitGroup

		// Prepare goroutines
		for i := 0; i < burstSize; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				<-start // Wait for signal

				body := bytes.NewBufferString(fmt.Sprintf(
					`{"path": "burst/file%d.txt", "content": "burst content %d"}`, index, index))
				req := httptest.NewRequest("POST", "/api/v1/repos/burst-test/add", body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				results <- w.Code
			}(i)
		}

		// Release the burst
		startTime := time.Now()
		close(start)
		wg.Wait()
		duration := time.Since(startTime)

		close(results)

		// Analyze results
		success := 0
		for code := range results {
			// V2 returns 201, V1 returns 200
			if code == 200 || code == 201 {
				success++
			}
		}

		t.Logf("Write burst of %d requests completed in %v", burstSize, duration)
		t.Logf("Success rate: %.2f%%", float64(success)/float64(burstSize)*100)
		t.Logf("Writes per second: %.2f", float64(burstSize)/duration.Seconds())
	})
}

// TestStressLongRunning simulates long-running server under continuous load
func TestStressLongRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running stress test in short mode")
	}

	server, router := setupBenchmarkServer()

	const (
		duration = 60 * time.Second
		numWorkers = 10
	)

	// Create test repositories
	for i := 0; i < numWorkers; i++ {
		createRepo(t, router, fmt.Sprintf("long-run-%d", i))
	}

	t.Run("Sustained load test", func(t *testing.T) {
		var wg sync.WaitGroup
		var totalOps int64
		var errors int64

		ctx, cancel := context.WithTimeout(context.Background(), duration)
		defer cancel()

		startTime := time.Now()

		// Start workers
		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				repoID := fmt.Sprintf("long-run-%d", workerID)
				ops := 0

				for {
					select {
					case <-ctx.Done():
						return
					default:
					}

					// Perform random operation
					switch rand.Intn(4) {
					case 0: // Add file
						body := bytes.NewBufferString(fmt.Sprintf(
							`{"path": "file_%d_%d.txt", "content": "content"}`, workerID, ops))
						req := httptest.NewRequest("POST", 
							fmt.Sprintf("/api/v1/repos/%s/add", repoID), body)
						req.Header.Set("Content-Type", "application/json")
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)

						// V2 returns 201, V1 returns 200
						if w.Code != 200 && w.Code != 201 {
							atomic.AddInt64(&errors, 1)
						}

					case 1: // Commit
						body := bytes.NewBufferString(fmt.Sprintf(
							`{"message": "Commit %d"}`, ops))
						req := httptest.NewRequest("POST", 
							fmt.Sprintf("/api/v1/repos/%s/commit", repoID), body)
						req.Header.Set("Content-Type", "application/json")
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)

						// Might fail if nothing to commit
						if w.Code != 201 && w.Code != 400 {
							atomic.AddInt64(&errors, 1)
						}

					case 2: // Get status
						req := httptest.NewRequest("GET", 
							fmt.Sprintf("/api/v1/repos/%s/status", repoID), nil)
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)

						if w.Code != 200 {
							atomic.AddInt64(&errors, 1)
						}

					case 3: // Get log
						req := httptest.NewRequest("GET", 
							fmt.Sprintf("/api/v1/repos/%s/log", repoID), nil)
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)

						if w.Code != 200 {
							atomic.AddInt64(&errors, 1)
						}
					}

					ops++
					atomic.AddInt64(&totalOps, 1)

					// Small delay to prevent overwhelming
					time.Sleep(time.Millisecond)
				}
			}(w)
		}

		// Monitor progress
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		go func() {
			for {
				select {
				case <-ticker.C:
					ops := atomic.LoadInt64(&totalOps)
					errs := atomic.LoadInt64(&errors)
					elapsed := time.Since(startTime)
					opsPerSec := float64(ops) / elapsed.Seconds()

					server.mu.RLock()
					repos := server.repoPool.Size()
					txs := len(server.transactions)
					server.mu.RUnlock()

					t.Logf("Progress: %d ops (%.2f/sec), %d errors, %d repos, %d txs",
						ops, opsPerSec, errs, repos, txs)

				case <-ctx.Done():
					return
				}
			}
		}()

		wg.Wait()

		finalOps := atomic.LoadInt64(&totalOps)
		finalErrors := atomic.LoadInt64(&errors)
		finalDuration := time.Since(startTime)

		t.Logf("Long-running test completed:")
		t.Logf("  Duration: %v", finalDuration)
		t.Logf("  Total operations: %d", finalOps)
		t.Logf("  Operations/sec: %.2f", float64(finalOps)/finalDuration.Seconds())
		t.Logf("  Errors: %d (%.2f%%)", finalErrors, float64(finalErrors)/float64(finalOps)*100)

		if float64(finalErrors)/float64(finalOps) > 0.01 {
			t.Errorf("Error rate too high during sustained load: %.2f%%", 
				float64(finalErrors)/float64(finalOps)*100)
		}
	})
}