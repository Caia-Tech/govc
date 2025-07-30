package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caia-tech/govc/api"
	"github.com/caia-tech/govc/config"
	"github.com/caia-tech/govc/logging"
	"github.com/gin-gonic/gin"
)

var (
	configFile = flag.String("config", "", "Path to configuration file (optional)")
	debug      = flag.Bool("debug", false, "Enable debug mode (overrides config)")
	version    = flag.Bool("version", false, "Show version information")
)

const (
	AppVersion = "1.0.0-dev"
	BuildTime  = "unknown"
)

func main() {
	flag.Parse()

	// Show version and exit
	if *version {
		fmt.Printf("govc-server %s\n", AppVersion)
		fmt.Printf("Build time: %s\n", BuildTime)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Override debug mode from command line
	if *debug {
		cfg.Development.Debug = true
	}

	// Initialize logger early
	logConfig := logging.Config{
		Level:     logging.LogLevel(cfg.GetLogLevel()),
		Component: cfg.Logging.Component,
		Output:    os.Stdout,
	}
	logger := logging.NewLogger(logConfig)
	logger.Info("Starting govc server")

	// Set Gin mode based on configuration
	if cfg.Development.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create router with middleware
	router := gin.New()

	// Add recovery middleware (logging is handled by the logger directly)

	// Add recovery middleware
	router.Use(gin.Recovery())

	// Add CORS middleware if enabled
	if cfg.Development.CORSEnabled {
		router.Use(corsMiddleware(cfg.Development.AllowedOrigins))
	}

	logger.Info("Initializing server components")

	// Create server with config
	server := api.NewServer(cfg)

	// Register routes
	server.RegisterRoutes(router)

	// Create HTTP server with proper timeouts
	addr := cfg.Address()
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("Starting HTTP server on " + addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server")
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	// Give outstanding requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown server
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown")
		os.Exit(1)
	}

	// Close server resources
	server.Close()
	logger.Info("Server exited")
}

func corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		
		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range allowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}
		
		if allowed {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else if len(allowedOrigins) > 0 && allowedOrigins[0] == "*" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}
		
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-API-Key")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}