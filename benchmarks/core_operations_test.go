package benchmarks

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/caiatech/govc"
	"github.com/caiatech/govc/api"
	"github.com/caiatech/govc/auth"
	"github.com/caiatech/govc/config"
	"github.com/caiatech/govc/logging"
	"github.com/caiatech/govc/metrics"
	"github.com/caiatech/govc/pool"
	"github.com/gin-gonic/gin"
)

// BenchmarkCoreOperations tests the core govc operations
func BenchmarkCoreOperations(b *testing.B) {
	b.Run("Repository/New", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = govc.New()
		}
	})

	b.Run("Repository/Transaction", func(b *testing.B) {
		repo := govc.New()
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			tx := repo.Transaction()
			tx.Add(fmt.Sprintf("file%d.txt", i), []byte("test content"))
			tx.Commit(fmt.Sprintf("Commit %d", i))
		}
	})

	b.Run("Repository/ParallelRealities", func(b *testing.B) {
		repo := govc.New()
		// Add initial commit
		tx := repo.Transaction()
		tx.Add("base.txt", []byte("base"))
		tx.Commit("Initial")

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			branches := make([]string, 5)
			for j := 0; j < 5; j++ {
				branches[j] = fmt.Sprintf("reality-%d-%d", i, j)
			}
			repo.ParallelRealities(branches)
		}
	})
}

// BenchmarkAuthOperations benchmarks authentication operations
func BenchmarkAuthOperations(b *testing.B) {
	cfg := config.DefaultConfig()
	jwtAuth := auth.NewJWTAuth(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, cfg.Auth.JWT.TTL)
	rbac := auth.NewRBAC()

	// Create test user
	rbac.CreateUser("bench-user", "Bench User", "bench@test.com", []string{"developer"})

	b.Run("JWT/Generate", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = jwtAuth.GenerateToken("bench-user", "bench-user", "bench@test.com", []string{"repo:read"})
		}
	})

	b.Run("JWT/Validate", func(b *testing.B) {
		token, _ := jwtAuth.GenerateToken("bench-user", "bench-user", "bench@test.com", []string{"repo:read"})
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, _ = jwtAuth.ValidateToken(token)
		}
	})

	b.Run("RBAC/HasPermission", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			rbac.HasPermission("bench-user", auth.PermissionRepoRead)
		}
	})
}

// BenchmarkPoolOperations benchmarks repository pool operations
func BenchmarkPoolOperations(b *testing.B) {
	poolConfig := pool.PoolConfig{
		MaxRepositories: 1000,
		MaxIdleTime:     30 * time.Minute,
		CleanupInterval: 5 * time.Minute,
		EnableMetrics:   false,
	}
	p := pool.NewRepositoryPool(poolConfig)
	defer p.Close()

	b.Run("Pool/Get", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			repoID := fmt.Sprintf("pool-repo-%d", i%100)
			_, _ = p.Get(repoID, ":memory:", true)
		}
	})

	b.Run("Pool/GetStats", func(b *testing.B) {
		// Pre-populate pool
		for i := 0; i < 50; i++ {
			p.Get(fmt.Sprintf("stats-repo-%d", i), ":memory:", true)
		}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_ = p.Stats()
		}
	})
}

// BenchmarkMetricsOperations benchmarks metrics collection
func BenchmarkMetricsOperations(b *testing.B) {
	m := metrics.NewPrometheusMetrics()

	b.Run("Metrics/RecordHTTP", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			m.RecordHTTPRequest("GET", "/api/v1/repos", 200, 100*time.Microsecond)
		}
	})

	b.Run("Metrics/SetGauge", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			m.SetRepositoryCount(int64(i))
		}
	})
}

// BenchmarkLoggingOperations benchmarks logging operations
func BenchmarkLoggingOperations(b *testing.B) {
	logConfig := logging.Config{
		Level:     logging.InfoLevel,
		Component: "benchmark",
		Output:    bytes.NewBuffer(nil), // Discard output to a buffer
	}
	logger := logging.NewLogger(logConfig)

	b.Run("Logger/Info", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("Benchmark message")
		}
	})

	b.Run("Logger/WithField", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.WithField("index", i).Info("Benchmark with field")
		}
	})
}

// BenchmarkAPIEndToEnd benchmarks complete API workflows
func BenchmarkAPIEndToEnd(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false
	cfg.Metrics.Enabled = false

	server := api.NewServer(cfg)
	router := gin.New()
	server.RegisterRoutes(router)

	b.Run("API/CreateRepo", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			body := bytes.NewBufferString(fmt.Sprintf(`{"id": "bench-%d", "memory_only": true}`, i))
			req := httptest.NewRequest("POST", "/api/v1/repos", body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}
	})

	// Create a repo for other operations
	body := bytes.NewBufferString(`{"id": "bench-repo", "memory_only": true}`)
	req := httptest.NewRequest("POST", "/api/v1/repos", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	b.Run("API/GetRepo", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/api/v1/repos/bench-repo", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}
	})

	b.Run("API/AddFile", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			body := bytes.NewBufferString(fmt.Sprintf(`{"path": "file%d.txt", "content": "test"}`, i))
			req := httptest.NewRequest("POST", "/api/v1/repos/bench-repo/add", body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}
	})
}
