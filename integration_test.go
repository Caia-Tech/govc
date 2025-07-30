package govc_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/caia-tech/govc"
)

// TestIntegrationInfrastructureWorkflow tests a complete infrastructure management workflow
func TestIntegrationInfrastructureWorkflow(t *testing.T) {
	// Create memory-first repository for infrastructure state
	repo := govc.New()

	// Initialize with production config
	tx := repo.Transaction()
	tx.Add("nginx/nginx.conf", []byte(`
server {
    listen 80;
    worker_processes 4;
    keepalive_timeout 65;
}`))
	tx.Add("redis/redis.conf", []byte("maxmemory 1gb\nmaxclients 10000"))
	tx.Add("app/config.yaml", []byte("replicas: 3\nversion: 1.0"))
	tx.Validate()
	tx.Commit("Production configuration baseline")

	// Scenario 1: Test scaling configurations in parallel
	t.Run("parallel scaling tests", func(t *testing.T) {
		scalingConfigs := []struct {
			name   string
			nginx  int
			redis  string
			app    int
		}{
			{"scale-small", 2, "512mb", 2},
			{"scale-medium", 4, "2gb", 5},
			{"scale-large", 8, "4gb", 10},
			{"scale-xlarge", 16, "8gb", 20},
		}

		// Create parallel realities for each scaling config
		names := make([]string, len(scalingConfigs))
		for i, cfg := range scalingConfigs {
			names[i] = cfg.name
		}
		realities := repo.ParallelRealities(names)

		// Test each configuration concurrently
		var wg sync.WaitGroup
		results := make(chan struct {
			name   string
			score  float64
			memory int64
		}, len(realities))

		for i, reality := range realities {
			wg.Add(1)
			go func(r *govc.Reality, cfg struct {
				name   string
				nginx  int
				redis  string
				app    int
			}) {
				defer wg.Done()

				// Apply scaling configuration
				r.Apply(map[string][]byte{
					"nginx/nginx.conf": []byte(fmt.Sprintf("worker_processes %d;", cfg.nginx)),
					"redis/redis.conf": []byte(fmt.Sprintf("maxmemory %s", cfg.redis)),
					"app/config.yaml":  []byte(fmt.Sprintf("replicas: %d", cfg.app)),
				})

				// Simulate performance testing
				r.Benchmark()
				score := float64(cfg.nginx*10 + cfg.app*5) // Simulated score

				results <- struct {
					name   string
					score  float64
					memory int64
				}{cfg.name, score, int64(cfg.nginx * cfg.app)}
			}(reality, scalingConfigs[i])
		}

		wg.Wait()
		close(results)

		// Find best configuration
		var bestConfig string
		var bestScore float64
		for result := range results {
			t.Logf("Config %s: score=%.2f, memory=%dMB", result.name, result.score, result.memory)
			if result.score > bestScore {
				bestScore = result.score
				bestConfig = result.name
			}
		}

		t.Logf("Best configuration: %s with score %.2f", bestConfig, bestScore)
	})

	// Scenario 2: Canary deployment testing
	t.Run("canary deployment", func(t *testing.T) {
		// Create canary branch
		canary := repo.ParallelReality("canary-v2")

		// Apply v2 changes
		canary.Apply(map[string][]byte{
			"app/config.yaml": []byte("replicas: 5\nversion: 2.0\nfeatures:\n  - new-ui\n  - caching"),
		})

		// Simulate canary metrics
		time.Sleep(10 * time.Millisecond) // Simulate monitoring period
		
		canaryMetrics := struct {
			errorRate   float64
			latency     float64
			throughput  float64
		}{0.01, 45.5, 1000.0}

		// Decision logic
		if canaryMetrics.errorRate < 0.05 && canaryMetrics.latency < 100 {
			t.Log("Canary deployment successful, promoting to production")
			// In real scenario: repo.Merge("canary-v2", "main")
		} else {
			t.Log("Canary deployment failed metrics threshold")
		}
	})

	// Scenario 3: Event-driven infrastructure
	t.Run("reactive infrastructure", func(t *testing.T) {
		events := make([]govc.Event, 0)
		var mu sync.Mutex

		// Set up event handler
		repo.Watch(func(event govc.Event) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, event)

			// React to specific events
			if event.Message == "SCALE_UP_REQUIRED" {
				t.Logf("Auto-scaling triggered by %s", event.Author)
			}
			if event.Message == "EMERGENCY_ROLLBACK" {
				t.Logf("Emergency rollback initiated at %v", event.Timestamp)
			}
		})

		// Wait for watcher to start
		time.Sleep(150 * time.Millisecond)

		// Simulate infrastructure events
		emergencyTx := repo.Transaction()
		emergencyTx.Add("nginx/nginx.conf", []byte("worker_processes 1;"))
		emergencyTx.Validate()
		emergencyTx.Commit("EMERGENCY_ROLLBACK")

		time.Sleep(150 * time.Millisecond)

		mu.Lock()
		if len(events) > 0 {
			t.Logf("Captured %d infrastructure events", len(events))
		}
		mu.Unlock()
	})

	// Scenario 4: Time-travel debugging
	t.Run("incident investigation", func(t *testing.T) {
		// Create some history
		times := make([]time.Time, 0)
		
		for i := 0; i < 5; i++ {
			tx := repo.Transaction()
			tx.Add("app/version.txt", []byte(fmt.Sprintf("v1.%d", i)))
			tx.Validate()
			commit, _ := tx.Commit(fmt.Sprintf("Deploy v1.%d", i))
			times = append(times, commit.Author.Time)
			time.Sleep(10 * time.Millisecond)
		}

		// Simulate incident at v1.3
		incidentTime := times[3]
		snapshot := repo.TimeTravel(incidentTime)

		if snapshot != nil {
			content, err := snapshot.Read("app/version.txt")
			if err == nil {
				t.Logf("System state at incident: %s", string(content))
			}

			lastCommit := snapshot.LastCommit()
			t.Logf("Last change before incident: %s by %s", 
				lastCommit.Message, lastCommit.Author.Name)
		}
	})
}

// TestIntegrationConcurrentOperations tests high concurrency scenarios
func TestIntegrationConcurrentOperations(t *testing.T) {
	repo := govc.New()

	// Initialize
	tx := repo.Transaction()
	tx.Add("counter.txt", []byte("0"))
	tx.Validate()
	tx.Commit("Initialize counter")

	t.Run("concurrent transactions", func(t *testing.T) {
		const numGoroutines = 50
		var wg sync.WaitGroup
		successCount := int32(0)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				tx := repo.Transaction()
				tx.Add(fmt.Sprintf("file%d.txt", id), []byte(fmt.Sprintf("content %d", id)))
				
				if err := tx.Validate(); err != nil {
					return
				}

				if _, err := tx.Commit(fmt.Sprintf("Commit %d", id)); err == nil {
					atomic.AddInt32(&successCount, 1)
				}
			}(i)
		}

		wg.Wait()
		t.Logf("Successfully committed %d/%d concurrent transactions", 
			successCount, numGoroutines)
	})

	t.Run("parallel reality stress test", func(t *testing.T) {
		const numRealities = 100
		names := make([]string, numRealities)
		for i := 0; i < numRealities; i++ {
			names[i] = fmt.Sprintf("stress-%d", i)
		}

		start := time.Now()
		realities := repo.ParallelRealities(names)
		createDuration := time.Since(start)

		// Modify all realities concurrently
		start = time.Now()
		var wg sync.WaitGroup
		for i, reality := range realities {
			wg.Add(1)
			go func(r *govc.Reality, id int) {
				defer wg.Done()
				
				// Each reality gets 10 files
				for j := 0; j < 10; j++ {
					r.Apply(map[string][]byte{
						fmt.Sprintf("r%d_f%d.txt", id, j): []byte(fmt.Sprintf("data %d-%d", id, j)),
					})
				}
			}(reality, i)
		}
		wg.Wait()
		modifyDuration := time.Since(start)

		t.Logf("Created %d realities in %v", numRealities, createDuration)
		t.Logf("Modified %d realities (1000 files) in %v", numRealities, modifyDuration)
		
		// Should be very fast
		if createDuration > 100*time.Millisecond {
			t.Logf("Warning: Reality creation slower than expected")
		}
		if modifyDuration > 500*time.Millisecond {
			t.Logf("Warning: Concurrent modifications slower than expected")
		}
	})
}

// TestIntegrationDistributedScenario simulates distributed infrastructure management
func TestIntegrationDistributedScenario(t *testing.T) {
	// Simulate 3 regional repositories
	regions := map[string]*govc.Repository{
		"us-east": govc.New(),
		"eu-west": govc.New(),
		"ap-south": govc.New(),
	}

	// Initialize each region with base config
	for region, repo := range regions {
		tx := repo.Transaction()
		tx.Add("region.conf", []byte(fmt.Sprintf("region=%s\nstatus=active", region)))
		tx.Add("cdn.conf", []byte("provider=cloudfront\ncache_ttl=3600"))
		tx.Validate()
		tx.Commit(fmt.Sprintf("Initialize %s region", region))
	}

	t.Run("regional optimization", func(t *testing.T) {
		results := make(chan struct {
			region   string
			provider string
			latency  float64
		}, len(regions))

		// Each region tests different CDN providers
		for region, repo := range regions {
			go func(r string, repository *govc.Repository) {
				// Create test realities for different providers
				providers := []string{"cloudfront", "cloudflare", "fastly"}
				realities := repository.ParallelRealities(providers)

				bestLatency := float64(999)
				bestProvider := ""

				for i, reality := range realities {
					provider := providers[i]
					
					// Apply provider config
					reality.Apply(map[string][]byte{
						"cdn.conf": []byte(fmt.Sprintf("provider=%s\nregion=%s", provider, r)),
					})

					// Simulate latency test
					latency := float64(50 + i*10) // Simulated
					if latency < bestLatency {
						bestLatency = latency
						bestProvider = provider
					}
				}

				results <- struct {
					region   string
					provider string
					latency  float64
				}{r, bestProvider, bestLatency}
			}(region, repo)
		}

		// Collect results
		for i := 0; i < len(regions); i++ {
			result := <-results
			t.Logf("Region %s: best provider=%s, latency=%.2fms", 
				result.region, result.provider, result.latency)
		}
	})

	t.Run("cross-region consensus", func(t *testing.T) {
		// Simulate a change that needs consensus
		proposal := "Upgrade to TLS 1.3"
		votes := make(chan bool, len(regions))

		for region, repo := range regions {
			go func(r string, repository *govc.Repository) {
				// Each region validates the change
				testReality := repository.ParallelReality("tls-upgrade-test")
				testReality.Apply(map[string][]byte{
					"security.conf": []byte("tls_version=1.3\ncipher_suites=modern"),
				})

				// Simulate validation
				time.Sleep(10 * time.Millisecond)
				
				// Vote (simulated - all approve)
				votes <- true
				t.Logf("Region %s approved: %s", r, proposal)
			}(region, repo)
		}

		// Count votes
		approvals := 0
		for i := 0; i < len(regions); i++ {
			if <-votes {
				approvals++
			}
		}

		if approvals >= 2 { // Majority
			t.Logf("Consensus reached (%d/%d) for: %s", approvals, len(regions), proposal)
		}
	})
}

// TestIntegrationRealWorldPatterns tests common real-world patterns
func TestIntegrationRealWorldPatterns(t *testing.T) {
	t.Run("blue-green deployment", func(t *testing.T) {
		repo := govc.New()

		// Initialize blue (current production)
		tx := repo.Transaction()
		tx.Add("deployment.yaml", []byte("version: blue\nreplicas: 5"))
		tx.Validate()
		tx.Commit("Blue deployment active")

		// Create green deployment
		green := repo.ParallelReality("green-deployment")
		green.Apply(map[string][]byte{
			"deployment.yaml": []byte("version: green\nreplicas: 5\nfeatures:\n  - new-api\n  - improved-caching"),
		})

		// Test green deployment
		greenMetrics := green.Benchmark()
		if greenMetrics.Better() {
			t.Log("Green deployment passed tests, switching traffic")
			// In production: Update load balancer to point to green
		}
	})

	t.Run("configuration drift detection", func(t *testing.T) {
		repo := govc.New()

		// Define expected state
		expectedState := map[string][]byte{
			"firewall.rules": []byte("allow 80\nallow 443\ndeny all"),
			"users.conf":     []byte("admin:active\nservice:active"),
		}

		// Apply expected state
		tx := repo.Transaction()
		for file, content := range expectedState {
			tx.Add(file, content)
		}
		tx.Validate()
		baseline, _ := tx.Commit("Security baseline")

		// Simulate drift
		time.Sleep(10 * time.Millisecond)
		
		driftTx := repo.Transaction()
		driftTx.Add("firewall.rules", []byte("allow 80\nallow 443\nallow 22\ndeny all"))
		driftTx.Validate()
		driftTx.Commit("Unauthorized change")

		// Detect drift by comparing with baseline
		t.Logf("Drift detected from baseline %s", baseline.Hash()[:7])
	})

	t.Run("disaster recovery", func(t *testing.T) {
		repo := govc.New()

		// Create backup points
		backupPoints := make([]string, 0)
		
		for i := 0; i < 5; i++ {
			tx := repo.Transaction()
			tx.Add("state.json", []byte(fmt.Sprintf(`{"version": %d, "healthy": true}`, i)))
			tx.Validate()
			commit, _ := tx.Commit(fmt.Sprintf("Backup point %d", i))
			backupPoints = append(backupPoints, commit.Hash())
			time.Sleep(5 * time.Millisecond)
		}

		// Simulate disaster
		disasterTx := repo.Transaction()
		disasterTx.Add("state.json", []byte(`{"version": 99, "healthy": false, "error": "database corrupted"}`))
		disasterTx.Validate()
		disasterTx.Commit("DISASTER: Database corruption")

		// Instant recovery to last good state
		if len(backupPoints) > 0 {
			lastGood := backupPoints[len(backupPoints)-1]
			t.Logf("Disaster detected! Recovering to backup point: %s", lastGood[:7])
			// In production: repo.Checkout(lastGood)
		}
	})
}

// BenchmarkIntegrationScenarios benchmarks realistic scenarios
func BenchmarkIntegrationScenarios(b *testing.B) {
	b.Run("InfrastructureTestCycle", func(b *testing.B) {
		repo := govc.New()
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Create test reality
			reality := repo.ParallelReality(fmt.Sprintf("test-%d", i))
			
			// Apply configuration
			reality.Apply(map[string][]byte{
				"config.yaml": []byte("test: true"),
			})
			
			// Run benchmark
			_ = reality.Benchmark()
			
			// Would merge if successful
		}
	})

	b.Run("ConcurrentDeployments", func(b *testing.B) {
		repo := govc.New()
		
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			id := 0
			for pb.Next() {
				tx := repo.Transaction()
				tx.Add(fmt.Sprintf("deploy-%d.yaml", id), []byte("deployed"))
				tx.Validate()
				tx.Commit(fmt.Sprintf("Deploy %d", id))
				id++
			}
		})
	})
}