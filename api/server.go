package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/caiatech/govc"
	"github.com/caiatech/govc/auth"
	"github.com/caiatech/govc/config"
	"github.com/caiatech/govc/logging"
	"github.com/caiatech/govc/metrics"
	"github.com/caiatech/govc/pool"
	"github.com/caiatech/govc/web"
	"github.com/gin-gonic/gin"
)

type Server struct {
	config            *config.Config
	repoPool          *pool.RepositoryPool
	repoFactory       *RepositoryFactory // New: factory for clean architecture
	transactions      map[string]*govc.TransactionalCommit
	repoMetadata      map[string]*RepoMetadata
	jwtAuth           *auth.JWTAuth
	rbac              *auth.RBAC
	apiKeyMgr         *auth.APIKeyManager
	authMiddleware    *auth.AuthMiddleware
	prometheusMetrics *metrics.PrometheusMetrics
	logger            *logging.Logger
	csrfStore         *CSRFStore
	repository        *govc.Repository // Add direct repository reference
	clusterManager    *ClusterManager  // High availability cluster manager
	monitoringManager *MonitoringManager // Comprehensive monitoring system
	mu                sync.RWMutex
}

type RepoMetadata struct {
	ID        string
	CreatedAt time.Time
	Path      string
}

func NewServer(cfg *config.Config) *Server {
	// Initialize logger
	logConfig := logging.Config{
		Level:     logging.LogLevel(cfg.GetLogLevel()),
		Component: cfg.Logging.Component,
		Output:    os.Stdout,
	}
	logger := logging.NewLogger(logConfig)

	// Initialize authentication components if enabled
	var jwtAuth *auth.JWTAuth
	var rbac *auth.RBAC
	var apiKeyMgr *auth.APIKeyManager
	var authMiddleware *auth.AuthMiddleware

	if cfg.Auth.Enabled {
		jwtAuth = auth.NewJWTAuth(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, cfg.Auth.JWT.TTL)
		rbac = auth.NewRBAC()
		apiKeyMgr = auth.NewAPIKeyManager(rbac)
		authMiddleware = auth.NewAuthMiddleware(jwtAuth, apiKeyMgr, rbac)

		// Create default admin user
		rbac.CreateUser("admin", "admin", "admin@govc.dev", []string{"admin"})
	}

	// Initialize metrics if enabled
	var prometheusMetrics *metrics.PrometheusMetrics
	if cfg.Metrics.Enabled {
		prometheusMetrics = metrics.NewPrometheusMetrics()
	}

	// Initialize repository pool
	poolConfig := pool.PoolConfig{
		MaxIdleTime:     cfg.Pool.MaxIdleTime,
		CleanupInterval: cfg.Pool.CleanupInterval,
		MaxRepositories: cfg.Pool.MaxRepositories,
		EnableMetrics:   cfg.Pool.EnableMetrics,
	}
	repoPool := pool.NewRepositoryPool(poolConfig)

	// Initialize CSRF store with 1 hour TTL
	csrfStore := NewCSRFStore(time.Hour)

	server := &Server{
		config:            cfg,
		repoPool:          repoPool,
		repoFactory:       NewRepositoryFactory(logger),
		transactions:      make(map[string]*govc.TransactionalCommit),
		repoMetadata:      make(map[string]*RepoMetadata),
		jwtAuth:           jwtAuth,
		rbac:              rbac,
		apiKeyMgr:         apiKeyMgr,
		authMiddleware:    authMiddleware,
		prometheusMetrics: prometheusMetrics,
		logger:            logger,
		csrfStore:         csrfStore,
		repository:        nil, // Will be set when repository is created
	}

	// Initialize monitoring manager
	server.monitoringManager = NewMonitoringManager(server)

	return server
}

// Start initializes and starts server components
func (s *Server) Start() error {
	// Start monitoring if initialized
	if s.monitoringManager != nil {
		go s.monitoringManager.Start(context.Background())
		s.logger.Info("Monitoring system started")
	}
	
	return nil
}

func (s *Server) RegisterRoutes(router *gin.Engine) {
	// Check if we should use new architecture
	useNewArchitecture := s.config.Development.UseNewArchitecture

	// Add global middleware
	router.Use(gin.Recovery()) // Panic recovery
	router.Use(PerformanceLoggingMiddleware(s.logger)) // Enhanced request logging
	router.Use(SecurityHeadersMiddleware())
	router.Use(XSSProtectionMiddleware())
	router.Use(SecurityMiddleware(DefaultSecurityConfig(), s.logger))
	router.Use(RequestSizeMiddleware(s.config.Server.MaxRequestSize))
	router.Use(TimeoutMiddleware(s.config.Server.RequestTimeout))

	// Add CORS middleware if enabled
	if s.config.Development.CORSEnabled {
		router.Use(CORSMiddleware(s.config.Development.AllowedOrigins))
	}

	// Setup dashboard routes first (includes static files and WebSocket)
	web.SetupDashboardRoutes(router, s.repoPool, s.jwtAuth, s.logger)

	// Add CSRF protection for state-changing operations
	csrfSkipPaths := []string{
		"/api/v1/auth/login",
		"/api/v1/auth/csrf-token",
		"/health",
		"/metrics",
		"/swagger",
		"/dashboard",
		"/ws",
		"/static",
	}
	
	// Use double submit cookie pattern for simplicity in development
	// In production, use the stateful CSRF store
	if s.config.Auth.Enabled && !s.config.Development.Debug {
		router.Use(CSRFMiddleware(s.csrfStore, csrfSkipPaths))
	}

	v1 := router.Group("/api/v1")

	// Authentication routes (no auth required)
	if s.config.Auth.Enabled {
		authRoutes := v1.Group("/auth")
		{
			authRoutes.POST("/login", s.login)
			authRoutes.POST("/refresh", s.refreshToken)
			authRoutes.GET("/whoami", s.authMiddleware.AuthRequired(), s.whoami)
			authRoutes.GET("/csrf-token", s.getCSRFToken)
		}

		// API key management
		apikeys := v1.Group("/apikeys")
		apikeys.Use(s.authMiddleware.AuthRequired())
		{
			apikeys.POST("", s.createAPIKey)
			apikeys.GET("", s.listAPIKeys)
			apikeys.DELETE("/:key_id", s.revokeAPIKey)
		}

		// User management (admin only)
		users := v1.Group("/users")
		users.Use(s.authMiddleware.AuthRequired())
		users.Use(s.authMiddleware.RequirePermission(auth.PermissionSystemAdmin))
		{
			users.POST("", s.createUser)
			users.GET("/:user_id", s.getUser)
			users.POST("/:user_id/roles/:role_name", s.assignRole)
			users.POST("/:user_id/repos/:repo_id/permissions/:permission", s.grantRepositoryPermission)
		}

		// Roles
		v1.GET("/roles", s.authMiddleware.AuthRequired(), s.listRoles)
	}

	// Repository management
	repoRoutes := v1.Group("/repos")
	if s.config.Auth.Enabled {
		repoRoutes.Use(s.authMiddleware.OptionalAuth()) // Allow both authenticated and unauthenticated access
	}
	{
		// Choose handlers based on architecture
		if useNewArchitecture {
			repoRoutes.POST("", s.createRepoV2)
			repoRoutes.GET("/:repo_id", s.getRepoV2)
			repoRoutes.DELETE("/:repo_id", s.deleteRepoV2)
			repoRoutes.GET("", s.listReposV2)
		} else {
			repoRoutes.POST("", s.createRepo)
			repoRoutes.GET("/:repo_id", s.getRepo)
			repoRoutes.DELETE("/:repo_id", s.deleteRepo)
			repoRoutes.GET("", s.listRepos)
		}
		// Basic Git operations
		if useNewArchitecture {
			repoRoutes.POST("/:repo_id/add", s.addFileV2)
			repoRoutes.POST("/:repo_id/commit", s.commitV2)
			repoRoutes.GET("/:repo_id/log", s.getLogV2)
			repoRoutes.GET("/:repo_id/status", s.getStatusV2)
		} else {
			repoRoutes.POST("/:repo_id/add", s.addFile)
			repoRoutes.POST("/:repo_id/commit", s.commit)
			repoRoutes.GET("/:repo_id/log", s.getLog)
			repoRoutes.GET("/:repo_id/status", s.getStatus)
		}
		repoRoutes.GET("/:repo_id/show/:commit", s.showCommit)
		repoRoutes.GET("/:repo_id/diff/:from/:to", s.getDiff)
		repoRoutes.GET("/:repo_id/diff", s.getDiff) // Query params version
		repoRoutes.GET("/:repo_id/diff/working", s.getWorkingDiff)
		repoRoutes.GET("/:repo_id/diff/file/*path", s.getFileDiff)

		// File operations
		repoRoutes.GET("/:repo_id/read/*path", s.readFile)
		repoRoutes.POST("/:repo_id/write", s.writeFile)
		repoRoutes.GET("/:repo_id/tree/*path", s.listTree)
		repoRoutes.DELETE("/:repo_id/remove/*path", s.removeFile)
		repoRoutes.POST("/:repo_id/move", s.moveFile)
		repoRoutes.GET("/:repo_id/blame/*path", s.getBlame)

		// Branch operations
		if useNewArchitecture {
			repoRoutes.GET("/:repo_id/branches", s.listBranchesV2)
			repoRoutes.POST("/:repo_id/branches", s.createBranchV2)
			repoRoutes.DELETE("/:repo_id/branches/:branch", s.deleteBranchV2)
			repoRoutes.POST("/:repo_id/checkout", s.checkoutV2)
			repoRoutes.POST("/:repo_id/merge", s.mergeV2)
		} else {
			repoRoutes.GET("/:repo_id/branches", s.listBranches)
			repoRoutes.POST("/:repo_id/branches", s.createBranch)
			repoRoutes.DELETE("/:repo_id/branches/:branch", s.deleteBranch)
			repoRoutes.POST("/:repo_id/checkout", s.checkout)
			repoRoutes.POST("/:repo_id/merge", s.merge)
		}

		// Tag operations
		repoRoutes.GET("/:repo_id/tags", s.listTags)
		repoRoutes.POST("/:repo_id/tags", s.createTag)

		// Stash operations
		repoRoutes.POST("/:repo_id/stash", s.createStash)
		repoRoutes.GET("/:repo_id/stash", s.listStashes)
		repoRoutes.GET("/:repo_id/stash/:stash_id", s.getStash)
		repoRoutes.POST("/:repo_id/stash/:stash_id/apply", s.applyStash)
		repoRoutes.DELETE("/:repo_id/stash/:stash_id", s.dropStash)

		// Advanced Git operations
		repoRoutes.POST("/:repo_id/cherry-pick", s.cherryPick)
		repoRoutes.POST("/:repo_id/revert", s.revert)
		repoRoutes.POST("/:repo_id/reset", s.reset)
		repoRoutes.POST("/:repo_id/rebase", s.rebase)

		// Search & Query operations
		repoRoutes.GET("/:repo_id/search/commits", s.searchCommits)
		repoRoutes.GET("/:repo_id/search/content", s.searchContent)
		repoRoutes.GET("/:repo_id/search/files", s.searchFiles)
		repoRoutes.POST("/:repo_id/grep", s.grep)
		
		// Advanced Search Endpoints
		repoRoutes.POST("/:repo_id/search/fulltext", s.fullTextSearch)
		repoRoutes.POST("/:repo_id/search/sql", s.sqlQuery)
		repoRoutes.POST("/:repo_id/search/aggregate", s.searchWithAggregation)
		repoRoutes.GET("/:repo_id/search/statistics", s.getSearchIndexStatistics)
		repoRoutes.GET("/:repo_id/search/suggestions", s.getSearchSuggestions)
		repoRoutes.POST("/:repo_id/search/rebuild", s.rebuildSearchIndex)

		// Note: Streaming endpoints temporarily disabled for HA focus

		// Hooks & Events
		repoRoutes.POST("/:repo_id/hooks", s.registerHook)
		repoRoutes.GET("/:repo_id/hooks", s.listHooks)
		repoRoutes.GET("/:repo_id/hooks/:hook_id", s.getHook)
		repoRoutes.DELETE("/:repo_id/hooks/:hook_id", s.deleteHook)
		repoRoutes.GET("/:repo_id/events", s.eventStream)
		repoRoutes.POST("/:repo_id/hooks/execute/:hook_type", s.executeHook)

		// Memory-first special features
		repoRoutes.POST("/:repo_id/transaction", s.beginTransaction)
		repoRoutes.POST("/:repo_id/transaction/:tx_id/add", s.transactionAdd)
		repoRoutes.POST("/:repo_id/transaction/:tx_id/validate", s.transactionValidate)
		repoRoutes.POST("/:repo_id/transaction/:tx_id/commit", s.transactionCommit)
		repoRoutes.POST("/:repo_id/transaction/:tx_id/rollback", s.transactionRollback)

		repoRoutes.POST("/:repo_id/parallel-realities", s.createParallelRealities)
		repoRoutes.GET("/:repo_id/parallel-realities", s.listParallelRealities)
		repoRoutes.POST("/:repo_id/parallel-realities/:reality/apply", s.applyToReality)
		repoRoutes.GET("/:repo_id/parallel-realities/:reality/benchmark", s.benchmarkReality)

		repoRoutes.GET("/:repo_id/time-travel/:timestamp", s.timeTravel)
		repoRoutes.GET("/:repo_id/time-travel/:timestamp/read/*path", s.timeTravelRead)

		// Watch events (WebSocket endpoint)
		repoRoutes.GET("/:repo_id/watch", s.watchEvents)
	}

	// V2 API routes - always register to support migration
	v2 := router.Group("/api/v2")
	if s.config.Auth.Enabled {
		v2.Use(s.authMiddleware.OptionalAuth())
	}
	{
		// Repository management
		v2.POST("/repos", s.createRepoV2)
		v2.GET("/repos", s.listReposV2)
		v2.GET("/repos/:repo_id", s.getRepoV2)
		v2.DELETE("/repos/:repo_id", s.deleteRepoV2)

		// File operations
		v2.POST("/repos/:repo_id/files", s.addFileV2)
		v2.GET("/repos/:repo_id/files", s.readFileV2)
		v2.PUT("/repos/:repo_id/files", s.writeFileV2)
		v2.DELETE("/repos/:repo_id/files", s.removeFileV2)
		v2.POST("/repos/:repo_id/files/move", s.moveFileV2)
		v2.GET("/repos/:repo_id/tree", s.listTreeV2)

		// Git operations
		v2.POST("/repos/:repo_id/commits", s.commitV2)
		v2.GET("/repos/:repo_id/commits", s.getLogV2)
		v2.GET("/repos/:repo_id/status", s.getStatusV2)

		// Branch operations
		v2.POST("/repos/:repo_id/branches", s.createBranchV2)
		v2.POST("/repos/:repo_id/checkout", s.checkoutV2)
		v2.POST("/repos/:repo_id/merge", s.mergeV2)

		// Tag operations
		v2.POST("/repos/:repo_id/tags", s.createTagV2)

		// Stash operations
		v2.POST("/repos/:repo_id/stashes", s.createStashV2)
		v2.GET("/repos/:repo_id/stashes", s.listStashesV2)
		v2.GET("/repos/:repo_id/stashes/:stash_id", s.getStashV2)
		v2.POST("/repos/:repo_id/stashes/:stash_id/apply", s.applyStashV2)
		v2.DELETE("/repos/:repo_id/stashes/:stash_id", s.dropStashV2)

		// Hooks & Events
		v2.POST("/repos/:repo_id/hooks", s.registerHookV2)
		v2.GET("/repos/:repo_id/hooks", s.listHooksV2)
		v2.GET("/repos/:repo_id/hooks/:hook_id", s.getHookV2)
		v2.DELETE("/repos/:repo_id/hooks/:hook_id", s.deleteHookV2)
		v2.POST("/repos/:repo_id/hooks/execute/:hook_type", s.executeHookV2)
	}

	// Health and monitoring endpoints
	router.GET("/health", s.healthCheck)
	router.GET("/health/live", s.liveness)
	router.GET("/health/ready", s.readiness)
	router.GET("/metrics", s.prometheusMetrics.PrometheusHandler())
	router.GET("/version", s.versionInfo)
	
	// Enhanced monitoring endpoints
	monitoringRoutes := v1.Group("/monitoring")
	{
		monitoringRoutes.GET("/metrics", s.monitoringManager.GetSystemMetrics)
		monitoringRoutes.GET("/health/history", s.monitoringManager.GetHealthHistory)
		monitoringRoutes.GET("/alerts", s.monitoringManager.GetAlerts)
		monitoringRoutes.GET("/performance", s.monitoringManager.GetPerformanceProfile)
	}

	// Pool management endpoints (admin only if auth is enabled)
	poolRoutes := v1.Group("/pool")
	if s.config.Auth.Enabled {
		poolRoutes.Use(s.authMiddleware.AuthRequired())
		poolRoutes.Use(s.authMiddleware.RequirePermission(auth.PermissionSystemAdmin))
	}
	{
		poolRoutes.GET("/stats", s.getPoolStats)
		poolRoutes.POST("/cleanup", s.cleanupPool)
	}

	// Container system removed - focusing on core VCS functionality

	// Setup cluster management routes
	s.setupClusterRoutes(v1)

	// Add Prometheus middleware for automatic metrics collection
	if s.prometheusMetrics != nil {
		router.Use(s.prometheusMetrics.GinMiddleware())
	}

	// Add 404 handler for unmatched routes
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("endpoint not found: %s %s", c.Request.Method, c.Request.URL.Path),
			Code:  "ENDPOINT_NOT_FOUND",
		})
	})
}

func (s *Server) getRepository(id string) (*govc.Repository, error) {
	s.mu.RLock()
	metadata, ok := s.repoMetadata[id]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("repository not found: %s", id)
	}

	// For V2 architecture, we need to use a shared repository instance
	// to maintain state across operations
	if s.config.Development.UseNewArchitecture {
		// Try to get from factory first
		if s.repoFactory != nil {
			components, err := s.repoFactory.GetRepository(id)
			if err == nil && components != nil {
				// Check if we have a legacy wrapper, if not create one
				if components.LegacyRepo == nil {
					// Create and store a legacy wrapper
					var repo *govc.Repository
					if metadata.Path == ":memory:" {
						repo = govc.New()
					} else {
						repo, err = govc.Open(metadata.Path)
						if err != nil {
							// If can't open, initialize new one
							repo, err = govc.Init(metadata.Path)
							if err != nil {
								return nil, fmt.Errorf("failed to init repository: %v", err)
							}
						}
					}
					// Store for future use
					s.mu.Lock()
					if s.repository == nil {
						s.repository = repo
					}
					s.mu.Unlock()
					return repo, nil
				}
				// Return existing legacy repo (when we implement it)
			}
		}
		
		// Return cached repository if available
		s.mu.RLock()
		if s.repository != nil {
			defer s.mu.RUnlock()
			return s.repository, nil
		}
		s.mu.RUnlock()
		
		// Create new repository as fallback
		var repo *govc.Repository
		if metadata.Path == ":memory:" {
			repo = govc.New()
		} else {
			var err error
			repo, err = govc.Open(metadata.Path)
			if err != nil {
				repo, err = govc.Init(metadata.Path)
				if err != nil {
					return nil, fmt.Errorf("failed to init repository: %v", err)
				}
			}
		}
		
		// Cache it
		s.mu.Lock()
		s.repository = repo
		s.mu.Unlock()
		
		return repo, nil
	}

	// Get repository from pool (V1 architecture)
	pooledRepo, err := s.repoPool.Get(id, metadata.Path, metadata.Path == ":memory:")
	if err != nil {
		return nil, fmt.Errorf("failed to get repository from pool: %v", err)
	}

	// Update access time
	pooledRepo.UpdateAccess()

	return pooledRepo.GetRepository(), nil
}

// getRepositoryComponents gets repository using new architecture
func (s *Server) getRepositoryComponents(id string) (*RepositoryComponents, error) {
	components, err := s.repoFactory.GetRepository(id)
	if err != nil {
		// Try to load from metadata
		s.mu.RLock()
		metadata, ok := s.repoMetadata[id]
		s.mu.RUnlock()

		if !ok {
			return nil, fmt.Errorf("repository not found: %s", id)
		}

		// Open existing repository
		if metadata.Path == ":memory:" {
			return nil, fmt.Errorf("cannot reload memory repository: %s", id)
		}

		components, err = s.repoFactory.OpenRepository(id, metadata.Path)
		if err != nil {
			return nil, err
		}
	}

	return components, nil
}

// updateMetrics updates the Prometheus metrics with current counts
func (s *Server) updateMetrics() {
	if s.prometheusMetrics == nil {
		return
	}

	s.mu.RLock()
	repoCount := int64(len(s.repoMetadata))
	transactionCount := int64(len(s.transactions))
	s.mu.RUnlock()

	s.prometheusMetrics.SetRepositoryCount(repoCount)
	s.prometheusMetrics.SetTransactionCount(transactionCount)
}

// Close shuts down the server and cleans up resources
func (s *Server) Close() error {
	s.logger.Info("Starting server shutdown cleanup")

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clean up active transactions
	if len(s.transactions) > 0 {
		s.logger.Infof("Cleaning up %d active transactions", len(s.transactions))
		for txID, tx := range s.transactions {
			if tx != nil {
				s.logger.Debugf("Rolling back transaction: %s", txID)
			}
		}
		// Clear the transactions map
		s.transactions = make(map[string]*govc.TransactionalCommit)
	}

	// Close repository pool
	if s.repoPool != nil {
		s.logger.Info("Closing repository pool")
		s.repoPool.Close()
	}

	// Stop any background goroutines
	// Note: In a more complex implementation, you might have background workers
	// that need to be signaled to stop here

	s.logger.Info("Server shutdown cleanup completed")
	return nil
}
