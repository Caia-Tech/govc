package api

import (
	"fmt"
	"sync"
	"time"

	"github.com/caia-tech/govc"
	"github.com/gin-gonic/gin"
)

type Config struct {
	Port           string
	MaxRepos       int
	EnableAuth     bool
	PersistenceDir string
}

type Server struct {
	config       Config
	repos        map[string]*govc.Repository
	transactions map[string]*govc.TransactionalCommit
	repoMetadata map[string]*RepoMetadata
	mu           sync.RWMutex
}

type RepoMetadata struct {
	ID        string
	CreatedAt time.Time
	Path      string
}

func NewServer(config Config) *Server {
	return &Server{
		config:       config,
		repos:        make(map[string]*govc.Repository),
		transactions: make(map[string]*govc.TransactionalCommit),
		repoMetadata: make(map[string]*RepoMetadata),
	}
}

func (s *Server) RegisterRoutes(router *gin.Engine) {
	v1 := router.Group("/api/v1")

	// Add auth middleware if enabled
	if s.config.EnableAuth {
		v1.Use(AuthMiddleware())
	}

	// Repository management
	v1.POST("/repos", s.createRepo)
	v1.GET("/repos/:repo_id", s.getRepo)
	v1.DELETE("/repos/:repo_id", s.deleteRepo)
	v1.GET("/repos", s.listRepos)

	// Basic Git operations
	v1.POST("/repos/:repo_id/add", s.addFile)
	v1.POST("/repos/:repo_id/commit", s.commit)
	v1.GET("/repos/:repo_id/log", s.getLog)
	v1.GET("/repos/:repo_id/status", s.getStatus)
	v1.GET("/repos/:repo_id/show/:commit", s.showCommit)
	v1.GET("/repos/:repo_id/diff/:from/:to", s.getDiff)

	// Branch operations
	v1.GET("/repos/:repo_id/branches", s.listBranches)
	v1.POST("/repos/:repo_id/branches", s.createBranch)
	v1.DELETE("/repos/:repo_id/branches/:branch", s.deleteBranch)
	v1.POST("/repos/:repo_id/checkout", s.checkout)
	v1.POST("/repos/:repo_id/merge", s.merge)

	// Memory-first special features
	v1.POST("/repos/:repo_id/transaction", s.beginTransaction)
	v1.POST("/repos/:repo_id/transaction/:tx_id/add", s.transactionAdd)
	v1.POST("/repos/:repo_id/transaction/:tx_id/validate", s.transactionValidate)
	v1.POST("/repos/:repo_id/transaction/:tx_id/commit", s.transactionCommit)
	v1.POST("/repos/:repo_id/transaction/:tx_id/rollback", s.transactionRollback)

	v1.POST("/repos/:repo_id/parallel-realities", s.createParallelRealities)
	v1.GET("/repos/:repo_id/parallel-realities", s.listParallelRealities)
	v1.POST("/repos/:repo_id/parallel-realities/:reality/apply", s.applyToReality)
	v1.GET("/repos/:repo_id/parallel-realities/:reality/benchmark", s.benchmarkReality)

	v1.GET("/repos/:repo_id/time-travel/:timestamp", s.timeTravel)
	v1.GET("/repos/:repo_id/time-travel/:timestamp/read/*path", s.timeTravelRead)

	// Watch events (WebSocket endpoint)
	v1.GET("/repos/:repo_id/watch", s.watchEvents)

	// Health check
	router.GET("/health", s.healthCheck)
}

func (s *Server) getRepository(id string) (*govc.Repository, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, ok := s.repos[id]
	if !ok {
		return nil, fmt.Errorf("repository not found: %s", id)
	}
	return repo, nil
}

func (s *Server) healthCheck(c *gin.Context) {
	s.mu.RLock()
	repoCount := len(s.repos)
	txCount := len(s.transactions)
	s.mu.RUnlock()

	c.JSON(200, gin.H{
		"status": "healthy",
		"repos":  repoCount,
		"transactions": txCount,
		"max_repos": s.config.MaxRepos,
	})
}