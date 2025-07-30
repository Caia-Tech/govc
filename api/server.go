package api

import (
	"fmt"
	"sync"
	"time"

	"github.com/caia-tech/govc"
	"github.com/caia-tech/govc/auth"
	"github.com/caia-tech/govc/metrics"
	"github.com/caia-tech/govc/pool"
	"github.com/gin-gonic/gin"
)

type Config struct {
	Port           string
	MaxRepos       int
	EnableAuth     bool
	PersistenceDir string
	JWTSecret      string
	JWTIssuer      string
	JWTTTL         time.Duration
	// Pool configuration
	PoolMaxIdleTime     time.Duration
	PoolCleanupInterval time.Duration
	PoolEnableMetrics   bool
}

type Server struct {
	config            Config
	repoPool          *pool.RepositoryPool
	transactions      map[string]*govc.TransactionalCommit
	repoMetadata      map[string]*RepoMetadata
	jwtAuth           *auth.JWTAuth
	rbac              *auth.RBAC
	apiKeyMgr         *auth.APIKeyManager
	authMiddleware    *auth.AuthMiddleware
	prometheusMetrics *metrics.PrometheusMetrics
	mu                sync.RWMutex
}

type RepoMetadata struct {
	ID        string
	CreatedAt time.Time
	Path      string
}

func NewServer(config Config) *Server {
	// Initialize authentication components
	jwtAuth := auth.NewJWTAuth(config.JWTSecret, config.JWTIssuer, config.JWTTTL)
	rbac := auth.NewRBAC()
	apiKeyMgr := auth.NewAPIKeyManager(rbac)
	authMiddleware := auth.NewAuthMiddleware(jwtAuth, apiKeyMgr, rbac)

	// Initialize metrics
	prometheusMetrics := metrics.NewPrometheusMetrics()

	// Initialize repository pool
	poolConfig := pool.PoolConfig{
		MaxIdleTime:     config.PoolMaxIdleTime,
		CleanupInterval: config.PoolCleanupInterval,
		MaxRepositories: config.MaxRepos,
		EnableMetrics:   config.PoolEnableMetrics,
	}
	if poolConfig.MaxIdleTime == 0 {
		poolConfig.MaxIdleTime = 30 * time.Minute
	}
	if poolConfig.CleanupInterval == 0 {
		poolConfig.CleanupInterval = 5 * time.Minute
	}
	if poolConfig.MaxRepositories == 0 {
		poolConfig.MaxRepositories = 100
	}
	repoPool := pool.NewRepositoryPool(poolConfig)

	// Create default admin user if auth is enabled
	if config.EnableAuth {
		rbac.CreateUser("admin", "admin", "admin@govc.dev", []string{"admin"})
	}

	return &Server{
		config:            config,
		repoPool:          repoPool,
		transactions:      make(map[string]*govc.TransactionalCommit),
		repoMetadata:      make(map[string]*RepoMetadata),
		jwtAuth:           jwtAuth,
		rbac:              rbac,
		apiKeyMgr:         apiKeyMgr,
		authMiddleware:    authMiddleware,
		prometheusMetrics: prometheusMetrics,
	}
}

func (s *Server) RegisterRoutes(router *gin.Engine) {
	v1 := router.Group("/api/v1")

	// Authentication routes (no auth required)
	if s.config.EnableAuth {
		authRoutes := v1.Group("/auth")
		{
			authRoutes.POST("/login", s.login)
			authRoutes.POST("/refresh", s.refreshToken)
			authRoutes.GET("/whoami", s.authMiddleware.AuthRequired(), s.whoami)
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
	if s.config.EnableAuth {
		repoRoutes.Use(s.authMiddleware.OptionalAuth()) // Allow both authenticated and unauthenticated access
	}
	{
		repoRoutes.POST("", s.createRepo)
		repoRoutes.GET("/:repo_id", s.getRepo)
		repoRoutes.DELETE("/:repo_id", s.deleteRepo)
		repoRoutes.GET("", s.listRepos)
		// Basic Git operations
		repoRoutes.POST("/:repo_id/add", s.addFile)
		repoRoutes.POST("/:repo_id/commit", s.commit)
		repoRoutes.GET("/:repo_id/log", s.getLog)
		repoRoutes.GET("/:repo_id/status", s.getStatus)
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
		repoRoutes.GET("/:repo_id/branches", s.listBranches)
		repoRoutes.POST("/:repo_id/branches", s.createBranch)
		repoRoutes.DELETE("/:repo_id/branches/:branch", s.deleteBranch)
		repoRoutes.POST("/:repo_id/checkout", s.checkout)
		repoRoutes.POST("/:repo_id/merge", s.merge)

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

	// Health and monitoring endpoints
	router.GET("/health", s.healthCheck)
	router.GET("/health/live", s.liveness)
	router.GET("/health/ready", s.readiness)
	router.GET("/metrics", s.prometheusMetrics.PrometheusHandler())
	router.GET("/version", s.versionInfo)
	
	// Pool management endpoints (admin only if auth is enabled)
	poolRoutes := v1.Group("/pool")
	if s.config.EnableAuth {
		poolRoutes.Use(s.authMiddleware.AuthRequired())
		poolRoutes.Use(s.authMiddleware.RequirePermission(auth.PermissionSystemAdmin))
	}
	{
		poolRoutes.GET("/stats", s.getPoolStats)
		poolRoutes.POST("/cleanup", s.cleanupPool)
	}

	// Add Prometheus middleware for automatic metrics collection
	router.Use(s.prometheusMetrics.GinMiddleware())
}

func (s *Server) getRepository(id string) (*govc.Repository, error) {
	s.mu.RLock()
	metadata, ok := s.repoMetadata[id]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("repository not found: %s", id)
	}

	// Get repository from pool
	pooledRepo, err := s.repoPool.Get(id, metadata.Path, metadata.Path == ":memory:")
	if err != nil {
		return nil, fmt.Errorf("failed to get repository from pool: %v", err)
	}

	// Update access time
	pooledRepo.UpdateAccess()

	return pooledRepo.GetRepository(), nil
}

// updateMetrics updates the Prometheus metrics with current counts
func (s *Server) updateMetrics() {
	s.mu.RLock()
	repoCount := int64(len(s.repoMetadata))
	transactionCount := int64(len(s.transactions))
	s.mu.RUnlock()
	
	s.prometheusMetrics.SetRepositoryCount(repoCount)
	s.prometheusMetrics.SetTransactionCount(transactionCount)
}

// Close shuts down the server and cleans up resources
func (s *Server) Close() error {
	if s.repoPool != nil {
		s.repoPool.Close()
	}
	return nil
}

