package tests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Caia-Tech/govc"
	"github.com/Caia-Tech/govc/auth"
	"github.com/Caia-Tech/govc/config"
	"github.com/Caia-Tech/govc/logging"
	"github.com/Caia-Tech/govc/metrics"
	"github.com/Caia-Tech/govc/pool"
	"github.com/stretchr/testify/assert"
)

func TestEdgeCases(t *testing.T) {
	t.Run("Repository Edge Cases", func(t *testing.T) {
		t.Run("Empty Repository Operations", func(t *testing.T) {
			repo := govc.New()

			// Log on empty repo
			commits, err := repo.Log(10)
			assert.NoError(t, err)
			assert.Empty(t, commits)

			// Get branches on empty repo
			branches, err := repo.ListBranches()
			assert.NoError(t, err)
			assert.Contains(t, branches, "main") // Should have at least main

			// Current branch on empty repo
			current, err := repo.CurrentBranch()
			assert.NoError(t, err)
			assert.Equal(t, "main", current)
		})

		t.Run("Special Characters in Paths", func(t *testing.T) {
			repo := govc.New()

			testCases := []struct {
				name       string
				path       string
				content    string
				shouldWork bool
			}{
				{"Unicode path", "文件/测试.txt", "unicode content", true},
				{"Spaces in path", "my files/test file.txt", "space content", true},
				{"Special chars", "special-_chars.txt", "special content", true},
				{"Dots in path", "..hidden.txt", "hidden content", true},
				{"Deep nesting", "a/b/c/d/e/f/g/h/i/j/k/file.txt", "deep content", true},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					tx := repo.Transaction()
					tx.Add(tc.path, []byte(tc.content))

					if tc.shouldWork {
						err := tx.Validate()
						assert.NoError(t, err)
						_, err = tx.Commit(tc.name)
						assert.NoError(t, err)
					} else {
						err := tx.Validate()
						assert.Error(t, err)
					}
				})
			}
		})

		t.Run("Large Commit Messages", func(t *testing.T) {
			repo := govc.New()

			// Very long commit message
			longMessage := strings.Repeat("This is a very long commit message. ", 100)

			tx := repo.Transaction()
			tx.Add("test.txt", []byte("content"))
			tx.Validate()
			commit, err := tx.Commit(longMessage)

			assert.NoError(t, err)
			assert.Contains(t, commit.Message, "This is a very long commit message")
		})

		t.Run("Many Files in Single Commit", func(t *testing.T) {
			repo := govc.New()
			tx := repo.Transaction()

			// Add 1000 files
			for i := 0; i < 1000; i++ {
				path := fmt.Sprintf("files/file%04d.txt", i)
				content := fmt.Sprintf("Content of file %d", i)
				tx.Add(path, []byte(content))
			}

			err := tx.Validate()
			assert.NoError(t, err)

			commit, err := tx.Commit("Massive commit with 1000 files")
			assert.NoError(t, err)
			assert.NotEmpty(t, commit.Hash)
		})
	})

	t.Run("Auth Edge Cases", func(t *testing.T) {
		t.Run("Empty Credentials", func(t *testing.T) {
			cfg := config.DefaultConfig()
			jwtAuth := auth.NewJWTAuth(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, cfg.Auth.JWT.TTL)

			// Empty username
			token, err := jwtAuth.GenerateToken("", "user123", "email@test.com", []string{"read"})
			assert.Error(t, err)
			assert.Empty(t, token)

			// Empty email
			token, err = jwtAuth.GenerateToken("user", "user123", "", []string{"read"})
			assert.NoError(t, err) // Should still work
			assert.NotEmpty(t, token)

			// Empty permissions
			token, err = jwtAuth.GenerateToken("user", "user123", "email@test.com", []string{})
			assert.NoError(t, err)
			assert.NotEmpty(t, token)
		})

		t.Run("Invalid JWT Token", func(t *testing.T) {
			cfg := config.DefaultConfig()
			jwtAuth := auth.NewJWTAuth(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, cfg.Auth.JWT.TTL)

			// Malformed token
			claims, err := jwtAuth.ValidateToken("invalid.token.here")
			assert.Error(t, err)
			assert.Nil(t, claims)

			// Empty token
			claims, err = jwtAuth.ValidateToken("")
			assert.Error(t, err)
			assert.Nil(t, claims)

			// Token with wrong secret
			wrongAuth := auth.NewJWTAuth("wrong-secret", cfg.Auth.JWT.Issuer, cfg.Auth.JWT.TTL)
			wrongToken, _ := wrongAuth.GenerateToken("user", "user123", "email@test.com", []string{"read"})

			claims, err = jwtAuth.ValidateToken(wrongToken)
			assert.Error(t, err)
			assert.Nil(t, claims)
		})

		t.Run("RBAC Edge Cases", func(t *testing.T) {
			rbac := auth.NewRBAC()

			// Non-existent user
			hasPermission := rbac.HasPermission("non-existent-user", auth.PermissionRepoRead)
			assert.False(t, hasPermission)

			// Create user with no roles
			err := rbac.CreateUser("empty-user", "Empty User", "empty@test.com", []string{})
			assert.NoError(t, err)

			hasPermission = rbac.HasPermission("empty-user", auth.PermissionRepoRead)
			assert.False(t, hasPermission)

			// Try to create duplicate user
			err = rbac.CreateUser("empty-user", "Duplicate", "dup@test.com", []string{"admin"})
			assert.Error(t, err)

			// Invalid role assignment
			err = rbac.AssignRole("empty-user", "non-existent-role")
			assert.Error(t, err)
		})
	})

	t.Run("Pool Edge Cases", func(t *testing.T) {
		t.Run("Zero Config Values", func(t *testing.T) {
			p := pool.NewRepositoryPool(pool.PoolConfig{
				MaxRepositories: 0, // Zero max
				MaxIdleTime:     0, // No idle time
				CleanupInterval: 0, // No cleanup
			})
			defer p.Close()

			// Should still work with defaults
			repo, err := p.Get("test-repo", ":memory:", true)
			assert.NoError(t, err)
			assert.NotNil(t, repo)
		})

		t.Run("Rapid Get/Release", func(t *testing.T) {
			p := pool.NewRepositoryPool(pool.PoolConfig{
				MaxRepositories: 10,
				MaxIdleTime:     1 * time.Second,
				CleanupInterval: 100 * time.Millisecond,
			})
			defer p.Close()

			// Rapidly get and release same repo
			for i := 0; i < 100; i++ {
				repo, err := p.Get("rapid-repo", ":memory:", true)
				assert.NoError(t, err)
				assert.NotNil(t, repo)
				// Immediate next iteration reuses
			}

			stats := p.Stats()
			assert.Equal(t, 1, stats.TotalRepositories)
		})

		t.Run("Close While In Use", func(t *testing.T) {
			p := pool.NewRepositoryPool(pool.PoolConfig{
				MaxRepositories: 10,
				MaxIdleTime:     30 * time.Second,
				CleanupInterval: 5 * time.Second,
			})

			// Get repositories
			repos := make([]*pool.PooledRepository, 5)
			for i := 0; i < 5; i++ {
				repo, err := p.Get(fmt.Sprintf("close-test-%d", i), ":memory:", true)
				assert.NoError(t, err)
				repos[i] = repo
			}

			// Close pool while repos are "in use"
			p.Close()

			// Should not panic on second close
			p.Close()
		})
	})

	t.Run("Metrics Edge Cases", func(t *testing.T) {
		m := metrics.NewPrometheusMetrics()

		t.Run("Zero Duration", func(t *testing.T) {
			// Record with zero duration
			m.RecordHTTPRequest("GET", "/test", 200, 0)

			// Should not panic - record another request with zero duration
			m.RecordHTTPRequest("GET", "/test", 200, 0)
		})

		t.Run("Invalid Status Codes", func(t *testing.T) {
			// Negative status code
			m.RecordHTTPRequest("GET", "/test", -1, 100*time.Millisecond)

			// Very large status code
			m.RecordHTTPRequest("GET", "/test", 999, 100*time.Millisecond)

			// Should not panic
			m.RecordHTTPRequest("GET", "/test", 0, 100*time.Millisecond)
		})

		t.Run("Empty Labels", func(t *testing.T) {
			// Empty method
			m.RecordHTTPRequest("", "/test", 200, 100*time.Millisecond)

			// Empty path
			m.RecordHTTPRequest("GET", "", 200, 100*time.Millisecond)

			// Both empty
			m.RecordHTTPRequest("", "", 200, 100*time.Millisecond)
		})

		t.Run("Negative Values", func(t *testing.T) {
			// Negative counts
			m.SetRepositoryCount(-10)
			m.SetTransactionCount(-5)

			// Should handle gracefully - set to positive values
			m.SetRepositoryCount(1)
			m.SetTransactionCount(1)
		})
	})

	t.Run("Logging Edge Cases", func(t *testing.T) {
		t.Run("Nil Output", func(t *testing.T) {
			logger := logging.NewLogger(logging.Config{
				Level:     logging.InfoLevel,
				Component: "test",
				Output:    nil, // Nil output
			})

			// Should not panic
			logger.Info("Test message")
			logger.Error("Error message")
			logger.WithField("key", "value").Info("With field")
		})

		t.Run("Invalid Log Levels", func(t *testing.T) {
			// Very high log level
			logger := logging.NewLogger(logging.Config{
				Level:     logging.LogLevel(99),
				Component: "test",
			})

			// Should still work
			logger.Info("Should not appear")
			logger.Error("Should not appear")
		})

		t.Run("Large Log Messages", func(t *testing.T) {
			logger := logging.NewLogger(logging.Config{
				Level:     logging.InfoLevel,
				Component: "test",
			})

			// Very large message
			largeMessage := strings.Repeat("x", 10000)
			logger.Info(largeMessage)

			// Many fields
			l := logger
			for i := 0; i < 100; i++ {
				l = l.WithField(fmt.Sprintf("field%d", i), fmt.Sprintf("value%d", i))
			}
			l.Info("Many fields")
		})
	})

	t.Run("Config Edge Cases", func(t *testing.T) {
		t.Run("Empty Config", func(t *testing.T) {
			cfg := &config.Config{}

			// Validate should handle empty config
			err := cfg.Validate()
			assert.Error(t, err) // Should fail validation
		})

		t.Run("Invalid Durations", func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Server.ReadTimeout = -1 * time.Second
			cfg.Server.WriteTimeout = 0
			cfg.Auth.JWT.TTL = -24 * time.Hour

			err := cfg.Validate()
			assert.Error(t, err)
		})

		t.Run("Invalid Ports", func(t *testing.T) {
			cfg := config.DefaultConfig()

			// Negative port
			cfg.Server.Port = -1
			err := cfg.Validate()
			assert.Error(t, err)

			// Port too high
			cfg.Server.Port = 70000
			err = cfg.Validate()
			assert.Error(t, err)

			// Port 0 (should be valid - means random port)
			cfg.Server.Port = 0
			err = cfg.Validate()
			assert.NoError(t, err)
		})
	})
}
