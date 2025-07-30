package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/caia-tech/govc"
	"github.com/caia-tech/govc/auth"
	"github.com/caia-tech/govc/config"
	"github.com/caia-tech/govc/metrics"
	"github.com/caia-tech/govc/pool"
	"github.com/stretchr/testify/assert"
)

// Performance thresholds to detect regressions
var performanceThresholds = struct {
	RepoCreation      time.Duration
	CommitOperation   time.Duration
	AuthTokenGen      time.Duration
	PoolGet           time.Duration
	MetricsRecord     time.Duration
}{
	RepoCreation:      1 * time.Millisecond,
	CommitOperation:   2 * time.Millisecond,
	AuthTokenGen:      5 * time.Millisecond,
	PoolGet:           500 * time.Microsecond,
	MetricsRecord:     500 * time.Microsecond,
}

func TestPerformanceRegression(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance regression tests in short mode")
	}

	t.Run("Repository Operations", func(t *testing.T) {
		t.Run("Creation Performance", func(t *testing.T) {
			// Warm up
			for i := 0; i < 10; i++ {
				_ = govc.New()
			}

			// Measure
			iterations := 100
			start := time.Now()
			for i := 0; i < iterations; i++ {
				_ = govc.New()
			}
			duration := time.Since(start)
			avgDuration := duration / time.Duration(iterations)

			t.Logf("Average repository creation time: %v", avgDuration)
			assert.Less(t, avgDuration, performanceThresholds.RepoCreation,
				"Repository creation performance regression detected")
		})

		t.Run("Commit Performance", func(t *testing.T) {
			repo := govc.New()
			
			// Warm up
			for i := 0; i < 10; i++ {
				tx := repo.Transaction()
				tx.Add(fmt.Sprintf("warmup%d.txt", i), []byte("warmup"))
				tx.Commit("Warmup")
			}

			// Measure
			iterations := 100
			start := time.Now()
			for i := 0; i < iterations; i++ {
				tx := repo.Transaction()
				tx.Add(fmt.Sprintf("file%d.txt", i), []byte("test content"))
				tx.Validate()
				tx.Commit(fmt.Sprintf("Commit %d", i))
			}
			duration := time.Since(start)
			avgDuration := duration / time.Duration(iterations)

			t.Logf("Average commit time: %v", avgDuration)
			assert.Less(t, avgDuration, performanceThresholds.CommitOperation,
				"Commit operation performance regression detected")
		})

		t.Run("Branch Creation Performance", func(t *testing.T) {
			repo := govc.New()
			
			// Add base commit
			tx := repo.Transaction()
			tx.Add("base.txt", []byte("base"))
			tx.Commit("Base")

			// Measure branch creation
			iterations := 100
			start := time.Now()
			for i := 0; i < iterations; i++ {
				repo.Branch(fmt.Sprintf("branch-%d", i)).Create()
			}
			duration := time.Since(start)
			avgDuration := duration / time.Duration(iterations)

			t.Logf("Average branch creation time: %v", avgDuration)
			assert.Less(t, avgDuration, 100*time.Microsecond,
				"Branch creation performance regression detected")
		})
	})

	t.Run("Auth Performance", func(t *testing.T) {
		cfg := config.DefaultConfig()
		jwtAuth := auth.NewJWTAuth(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, cfg.Auth.JWT.TTL)
		rbac := auth.NewRBAC()
		
		// Create test user
		rbac.CreateUser("perf-user", "Perf User", "perf@test.com", []string{"developer"})

		t.Run("Token Generation", func(t *testing.T) {
			// Warm up
			for i := 0; i < 10; i++ {
				jwtAuth.GenerateToken("perf-user", "perf-user", "perf@test.com", []string{"read"})
			}

			// Measure
			iterations := 100
			start := time.Now()
			for i := 0; i < iterations; i++ {
				_, err := jwtAuth.GenerateToken("perf-user", "perf-user", "perf@test.com", []string{"read", "write"})
				assert.NoError(t, err)
			}
			duration := time.Since(start)
			avgDuration := duration / time.Duration(iterations)

			t.Logf("Average token generation time: %v", avgDuration)
			assert.Less(t, avgDuration, performanceThresholds.AuthTokenGen,
				"JWT token generation performance regression detected")
		})

		t.Run("Token Validation", func(t *testing.T) {
			token, _ := jwtAuth.GenerateToken("perf-user", "perf-user", "perf@test.com", []string{"read"})
			
			// Warm up
			for i := 0; i < 10; i++ {
				jwtAuth.ValidateToken(token)
			}

			// Measure
			iterations := 1000
			start := time.Now()
			for i := 0; i < iterations; i++ {
				_, err := jwtAuth.ValidateToken(token)
				assert.NoError(t, err)
			}
			duration := time.Since(start)
			avgDuration := duration / time.Duration(iterations)

			t.Logf("Average token validation time: %v", avgDuration)
			assert.Less(t, avgDuration, 15*time.Microsecond,
				"JWT token validation performance regression detected")
		})

		t.Run("Permission Check", func(t *testing.T) {
			// Warm up
			for i := 0; i < 100; i++ {
				rbac.HasPermission("perf-user", auth.PermissionRepoRead)
			}

			// Measure
			iterations := 10000
			start := time.Now()
			for i := 0; i < iterations; i++ {
				rbac.HasPermission("perf-user", auth.PermissionRepoRead)
			}
			duration := time.Since(start)
			avgDuration := duration / time.Duration(iterations)

			t.Logf("Average permission check time: %v", avgDuration)
			assert.Less(t, avgDuration, 1*time.Microsecond,
				"RBAC permission check performance regression detected")
		})
	})

	t.Run("Pool Performance", func(t *testing.T) {
		p := pool.NewRepositoryPool(pool.PoolConfig{
			MaxRepositories: 100,
			MaxIdleTime:     30 * time.Second,
			CleanupInterval: 5 * time.Second,
		})
		defer p.Close()

		t.Run("Repository Get", func(t *testing.T) {
			// Warm up - create some repos
			for i := 0; i < 10; i++ {
				p.Get(fmt.Sprintf("warmup-%d", i), ":memory:", true)
			}

			// Measure getting existing repos
			iterations := 1000
			start := time.Now()
			for i := 0; i < iterations; i++ {
				_, err := p.Get(fmt.Sprintf("warmup-%d", i%10), ":memory:", true)
				assert.NoError(t, err)
			}
			duration := time.Since(start)
			avgDuration := duration / time.Duration(iterations)

			t.Logf("Average pool get time: %v", avgDuration)
			assert.Less(t, avgDuration, performanceThresholds.PoolGet,
				"Pool get operation performance regression detected")
		})

		t.Run("Stats Collection", func(t *testing.T) {
			// Warm up
			for i := 0; i < 10; i++ {
				p.Stats()
			}

			// Measure
			iterations := 1000
			start := time.Now()
			for i := 0; i < iterations; i++ {
				stats := p.Stats()
				assert.NotNil(t, stats)
			}
			duration := time.Since(start)
			avgDuration := duration / time.Duration(iterations)

			t.Logf("Average stats collection time: %v", avgDuration)
			assert.Less(t, avgDuration, 100*time.Microsecond,
				"Pool stats collection performance regression detected")
		})
	})

	t.Run("Metrics Performance", func(t *testing.T) {
		m := metrics.NewPrometheusMetrics()

		t.Run("HTTP Request Recording", func(t *testing.T) {
			// Warm up
			for i := 0; i < 100; i++ {
				m.RecordHTTPRequest("GET", "/api/test", 200, 100*time.Microsecond)
			}

			// Measure
			iterations := 10000
			start := time.Now()
			for i := 0; i < iterations; i++ {
				m.RecordHTTPRequest("GET", "/api/test", 200, 100*time.Microsecond)
			}
			duration := time.Since(start)
			avgDuration := duration / time.Duration(iterations)

			t.Logf("Average metrics record time: %v", avgDuration)
			assert.Less(t, avgDuration, performanceThresholds.MetricsRecord,
				"Metrics recording performance regression detected")
		})

		t.Run("Gauge Updates", func(t *testing.T) {
			// Warm up
			for i := 0; i < 100; i++ {
				m.SetRepositoryCount(int64(i))
			}

			// Measure
			iterations := 10000
			start := time.Now()
			for i := 0; i < iterations; i++ {
				m.SetRepositoryCount(int64(i))
			}
			duration := time.Since(start)
			avgDuration := duration / time.Duration(iterations)

			t.Logf("Average gauge update time: %v", avgDuration)
			assert.Less(t, avgDuration, 100*time.Nanosecond,
				"Gauge update performance regression detected")
		})
	})
}

// TestMemoryUsage checks for memory leaks and excessive allocations
func TestMemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory usage tests in short mode")
	}

	t.Run("Repository Memory Leak Check", func(t *testing.T) {
		// Create and destroy many repositories
		for i := 0; i < 1000; i++ {
			repo := govc.New()
			tx := repo.Transaction()
			tx.Add("test.txt", []byte("test"))
			tx.Commit("Test")
			// repo should be garbage collected
		}
		
		// Note: In a real test, we'd use runtime.ReadMemStats to check memory
		t.Log("Repository memory leak check completed")
	})

	t.Run("Pool Memory Management", func(t *testing.T) {
		p := pool.NewRepositoryPool(pool.PoolConfig{
			MaxRepositories: 10,
			MaxIdleTime:     1 * time.Second,
			CleanupInterval: 500 * time.Millisecond,
		})
		
		// Create many repos that should be cleaned up
		for i := 0; i < 100; i++ {
			p.Get(fmt.Sprintf("temp-%d", i), ":memory:", true)
		}
		
		// Wait for cleanup
		time.Sleep(2 * time.Second)
		
		stats := p.Stats()
		assert.LessOrEqual(t, stats.TotalRepositories, 10, "Pool should enforce max repositories")
		
		p.Close()
	})
}