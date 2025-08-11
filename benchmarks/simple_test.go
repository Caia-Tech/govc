package benchmarks

import (
	"fmt"
	"testing"
	"time"

	"github.com/Caia-Tech/govc"
	"github.com/Caia-Tech/govc/auth"
	"github.com/Caia-Tech/govc/config"
	"github.com/Caia-Tech/govc/logging"
	"github.com/Caia-Tech/govc/metrics"
	"github.com/Caia-Tech/govc/pool"
)

// BenchmarkSimpleOperations tests basic operations
func BenchmarkSimpleOperations(b *testing.B) {
	b.Run("Repository/Create", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = govc.New()
		}
	})

	b.Run("Repository/Commit", func(b *testing.B) {
		repo := govc.New()
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			tx := repo.Transaction()
			tx.Add(fmt.Sprintf("file%d.txt", i), []byte("test"))
			tx.Commit("Test commit")
		}
	})

	b.Run("Auth/GenerateToken", func(b *testing.B) {
		cfg := config.DefaultConfig()
		jwtAuth := auth.NewJWTAuth(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, cfg.Auth.JWT.TTL)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, _ = jwtAuth.GenerateToken("user", "user", "user@test.com", []string{"read"})
		}
	})

	b.Run("Pool/GetRepository", func(b *testing.B) {
		p := pool.NewRepositoryPool(pool.PoolConfig{
			MaxRepositories: 100,
			MaxIdleTime:     5 * time.Minute,
			CleanupInterval: 1 * time.Minute,
		})
		defer p.Close()

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, _ = p.Get(fmt.Sprintf("repo-%d", i%10), ":memory:", true)
		}
	})

	b.Run("Metrics/Record", func(b *testing.B) {
		m := metrics.NewPrometheusMetrics()

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			m.RecordHTTPRequest("GET", "/api/v1/repos", 200, 100*time.Microsecond)
		}
	})

	b.Run("Logging/Info", func(b *testing.B) {
		logger := logging.NewLogger(logging.Config{
			Level:     logging.InfoLevel,
			Component: "bench",
			Output:    nil, // Will go to stderr but not captured in benchmark
		})

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			logger.Info("Benchmark log")
		}
	})
}

// BenchmarkConcurrentAccess tests concurrent operations
func BenchmarkConcurrentAccess(b *testing.B) {
	b.Run("Repository/ConcurrentCommits", func(b *testing.B) {
		repo := govc.New()

		b.ReportAllocs()
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				tx := repo.Transaction()
				tx.Add(fmt.Sprintf("file%d.txt", i), []byte("concurrent"))
				tx.Commit("Concurrent commit")
				i++
			}
		})
	})

	b.Run("Pool/ConcurrentGet", func(b *testing.B) {
		p := pool.NewRepositoryPool(pool.PoolConfig{
			MaxRepositories: 100,
			MaxIdleTime:     5 * time.Minute,
			CleanupInterval: 1 * time.Minute,
		})
		defer p.Close()

		b.ReportAllocs()
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				_, _ = p.Get(fmt.Sprintf("repo-%d", i%20), ":memory:", true)
				i++
			}
		})
	})
}

// BenchmarkMemoryVsDisk compares memory and disk performance
func BenchmarkMemoryVsDisk(b *testing.B) {
	b.Run("Memory/Commits", func(b *testing.B) {
		repo := govc.New()

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			tx := repo.Transaction()
			tx.Add("file.txt", []byte("memory test"))
			tx.Commit("Memory commit")
		}
	})

	b.Run("Disk/Commits", func(b *testing.B) {
		tmpDir := b.TempDir()
		repo, err := govc.Init(tmpDir)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			tx := repo.Transaction()
			tx.Add("file.txt", []byte("disk test"))
			tx.Commit("Disk commit")
		}
	})
}
