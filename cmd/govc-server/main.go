package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/caia-tech/govc/api"
	"github.com/gin-gonic/gin"
)

var (
	port                = flag.String("port", "8080", "Server port")
	enableAuth          = flag.Bool("auth", false, "Enable authentication")
	maxRepos            = flag.Int("max-repos", 1000, "Maximum number of repositories")
	debug               = flag.Bool("debug", false, "Enable debug mode")
	poolMaxIdleTime     = flag.Duration("pool-max-idle", 30*time.Minute, "Maximum time a repository can stay idle in pool")
	poolCleanupInterval = flag.Duration("pool-cleanup-interval", 5*time.Minute, "Interval for pool cleanup")
	poolEnableMetrics   = flag.Bool("pool-metrics", true, "Enable pool metrics collection")
)

func main() {
	flag.Parse()

	// Set Gin mode
	if !*debug {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create router
	router := gin.Default()

	// Add CORS middleware
	router.Use(corsMiddleware())

	// Create server with config
	config := api.Config{
		Port:                *port,
		MaxRepos:            *maxRepos,
		EnableAuth:          *enableAuth,
		PoolMaxIdleTime:     *poolMaxIdleTime,
		PoolCleanupInterval: *poolCleanupInterval,
		PoolEnableMetrics:   *poolEnableMetrics,
	}

	server := api.NewServer(config)

	// Register routes
	server.RegisterRoutes(router)

	// Start server
	addr := fmt.Sprintf(":%s", *port)
	log.Printf("Starting govc REST API server on %s", addr)
	
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
		os.Exit(1)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}