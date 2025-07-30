package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/caiatech/govc/config"
	"github.com/gin-gonic/gin"
)

// TestGracefulShutdown tests the graceful shutdown functionality
func TestGracefulShutdown(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Server closes cleanly with no active resources", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		
		server := NewServer(cfg)
		
		// Close should not return an error
		err := server.Close()
		if err != nil {
			t.Errorf("Expected no error from Close(), got %v", err)
		}
	})

	t.Run("Server closes cleanly with active transactions", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		
		server := NewServer(cfg)
		
		// Add some mock transactions to test cleanup
		server.mu.Lock()
		server.transactions["tx1"] = nil // Simulate active transaction
		server.transactions["tx2"] = nil // Simulate active transaction
		server.mu.Unlock()
		
		// Close should clean up transactions
		err := server.Close()
		if err != nil {
			t.Errorf("Expected no error from Close(), got %v", err)
		}
		
		// Verify transactions were cleaned up
		server.mu.RLock()
		txCount := len(server.transactions)
		server.mu.RUnlock()
		
		if txCount != 0 {
			t.Errorf("Expected 0 transactions after close, got %d", txCount)
		}
	})

	t.Run("Multiple close calls are safe", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		
		server := NewServer(cfg)
		
		// First close
		err1 := server.Close()
		if err1 != nil {
			t.Errorf("Expected no error from first Close(), got %v", err1)
		}
		
		// Second close should also be safe
		err2 := server.Close()
		if err2 != nil {
			t.Errorf("Expected no error from second Close(), got %v", err2)
		}
	})
}

// TestHTTPServerGracefulShutdown tests the HTTP server shutdown
func TestHTTPServerGracefulShutdown(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("HTTP server shuts down gracefully", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		cfg.Server.Port = 0 // Use random available port
		
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)
		
		// Create HTTP server
		srv := &http.Server{
			Addr:         ":0", // Use random port
			Handler:      router,
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
			IdleTimeout:  cfg.Server.IdleTimeout,
		}
		
		// Start server in goroutine
		go func() {
			srv.ListenAndServe()
		}()
		
		// Give server a moment to start
		time.Sleep(10 * time.Millisecond)
		
		// Create shutdown context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		// Shutdown server
		err := srv.Shutdown(ctx)
		if err != nil {
			t.Errorf("Expected no error from server shutdown, got %v", err)
		}
		
		// Close server resources
		err = server.Close()
		if err != nil {
			t.Errorf("Expected no error from server close, got %v", err)
		}
	})

	t.Run("Shutdown with immediate timeout", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		cfg.Server.Port = 0
		
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)
		
		srv := &http.Server{
			Addr:         ":0",
			Handler:      router,
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
			IdleTimeout:  cfg.Server.IdleTimeout,
		}
		
		go func() {
			srv.ListenAndServe() 
		}()
		
		time.Sleep(10 * time.Millisecond)
		
		// Already expired context
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()
		
		// Shutdown should respect the timeout
		start := time.Now()
		err := srv.Shutdown(ctx)
		duration := time.Since(start)
		
		// Should complete very quickly
		if duration > 50*time.Millisecond {
			t.Errorf("Shutdown took too long: %v", duration)
		}
		
		// Should get context deadline exceeded or canceled error
		if err == nil {
			t.Log("Shutdown completed without error (server had no active connections)")
		} else if err != context.DeadlineExceeded && err != context.Canceled {
			t.Errorf("Expected context deadline exceeded or canceled, got %v", err)
		}
		
		// Clean up
		server.Close()
	})
}

// TestShutdownIntegration tests integration with main server shutdown flow
func TestShutdownIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Complete shutdown flow with signal handling simulation", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)
		
		srv := &http.Server{
			Addr:         ":0",
			Handler:      router,
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
			IdleTimeout:  cfg.Server.IdleTimeout,
		}
		
		// Start server
		go func() {
			srv.ListenAndServe()
		}()
		
		time.Sleep(10 * time.Millisecond)
		
		// Simulate the shutdown flow from main.go
		shutdownComplete := make(chan error, 1)
		go func() {
			// Give outstanding requests time to complete (simulating graceful shutdown)
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			
			// Shutdown server
			err := srv.Shutdown(ctx)
			
			// Close server resources (this is what main.go does)
			server.Close()
			
			shutdownComplete <- err
		}()
		
		// Wait for shutdown to complete
		select {
		case err := <-shutdownComplete:
			if err != nil {
				t.Errorf("Expected no error from shutdown flow, got %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Shutdown flow did not complete in time") 
		}
	})

	t.Run("Server cleanup is idempotent", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		
		server := NewServer(cfg)
		
		// Multiple calls to Close should be safe
		err1 := server.Close()
		err2 := server.Close()
		err3 := server.Close()
		
		if err1 != nil || err2 != nil || err3 != nil {
			t.Errorf("Expected no errors from multiple close calls, got %v, %v, %v", err1, err2, err3)
		}
	})
}