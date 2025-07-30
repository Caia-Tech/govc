package pool

import (
	"fmt"
	"sync"
	"time"

	"github.com/caiatech/govc"
)

// RepositoryPool manages a pool of repository connections
type RepositoryPool struct {
	mu            sync.RWMutex
	repositories  map[string]*PooledRepository
	config        PoolConfig
	lastCleanup   time.Time
	cleanupTicker *time.Ticker
	done          chan struct{}
	closed        bool
}

// PoolConfig defines configuration for the repository pool
type PoolConfig struct {
	MaxIdleTime     time.Duration // Maximum time a repository can stay idle
	CleanupInterval time.Duration // How often to run cleanup
	MaxRepositories int           // Maximum number of repositories in pool
	EnableMetrics   bool          // Whether to collect pool metrics
}

// PooledRepository wraps a repository with pooling metadata
type PooledRepository struct {
	Repository   *govc.Repository
	LastAccessed time.Time
	AccessCount  int64
	CreatedAt    time.Time
	Path         string
	ID           string
	mu           sync.RWMutex
}

// RepositoryStats provides statistics about the repository pool
type RepositoryStats struct {
	TotalRepositories int                            `json:"total_repositories"`
	ActiveRepositories int                           `json:"active_repositories"`
	IdleRepositories  int                            `json:"idle_repositories"`
	RepositoryDetails map[string]*RepositoryDetail   `json:"repository_details,omitempty"`
	LastCleanup       time.Time                      `json:"last_cleanup"`
	Config            PoolConfig                     `json:"config"`
}

// RepositoryDetail provides detailed information about a pooled repository
type RepositoryDetail struct {
	ID           string    `json:"id"`
	Path         string    `json:"path"`
	LastAccessed time.Time `json:"last_accessed"`
	AccessCount  int64     `json:"access_count"`
	CreatedAt    time.Time `json:"created_at"`
	IdleTime     string    `json:"idle_time"`
}

// DefaultPoolConfig returns a sensible default configuration
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxIdleTime:     30 * time.Minute,
		CleanupInterval: 5 * time.Minute,
		MaxRepositories: 100,
		EnableMetrics:   true,
	}
}

// NewRepositoryPool creates a new repository pool
func NewRepositoryPool(config PoolConfig) *RepositoryPool {
	if config.MaxIdleTime == 0 {
		config.MaxIdleTime = 30 * time.Minute
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 5 * time.Minute
	}
	if config.MaxRepositories == 0 {
		config.MaxRepositories = 100
	}

	pool := &RepositoryPool{
		repositories: make(map[string]*PooledRepository),
		config:       config,
		lastCleanup:  time.Now(),
		done:         make(chan struct{}),
	}

	// Start cleanup routine
	pool.startCleanupRoutine()

	return pool
}

// Get retrieves a repository from the pool or creates a new one
func (p *RepositoryPool) Get(id, path string, memoryOnly bool) (*PooledRepository, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if repository already exists in pool
	if pooledRepo, exists := p.repositories[id]; exists {
		pooledRepo.mu.Lock()
		pooledRepo.LastAccessed = time.Now()
		pooledRepo.AccessCount++
		pooledRepo.mu.Unlock()
		return pooledRepo, nil
	}

	// Check pool size limit
	if len(p.repositories) >= p.config.MaxRepositories {
		// Try to evict idle repositories
		evicted := p.evictIdleRepositories()
		if evicted == 0 && len(p.repositories) >= p.config.MaxRepositories {
			return nil, fmt.Errorf("repository pool is full (max: %d)", p.config.MaxRepositories)
		}
	}

	// Create new repository
	var repo *govc.Repository
	var err error

	if memoryOnly || path == ":memory:" {
		repo = govc.New()
		path = ":memory:"
	} else {
		repo, err = govc.Init(path)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize repository: %v", err)
		}
	}

	// Create pooled repository
	pooledRepo := &PooledRepository{
		Repository:   repo,
		LastAccessed: time.Now(),
		AccessCount:  1,
		CreatedAt:    time.Now(),
		Path:         path,
		ID:           id,
	}

	// Add to pool
	p.repositories[id] = pooledRepo

	return pooledRepo, nil
}

// Remove removes a repository from the pool
func (p *RepositoryPool) Remove(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.repositories[id]; exists {
		delete(p.repositories, id)
		return true
	}
	return false
}

// Size returns the current size of the pool
func (p *RepositoryPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.repositories)
}

// Stats returns statistics about the repository pool
func (p *RepositoryPool) Stats() RepositoryStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := RepositoryStats{
		TotalRepositories: len(p.repositories),
		ActiveRepositories: 0,
		IdleRepositories:  0,
		RepositoryDetails: make(map[string]*RepositoryDetail),
		LastCleanup:       p.lastCleanup,
		Config:            p.config,
	}

	now := time.Now()
	idleThreshold := now.Add(-p.config.MaxIdleTime)

	for id, pooledRepo := range p.repositories {
		pooledRepo.mu.RLock()
		
		detail := &RepositoryDetail{
			ID:           pooledRepo.ID,
			Path:         pooledRepo.Path,
			LastAccessed: pooledRepo.LastAccessed,
			AccessCount:  pooledRepo.AccessCount,
			CreatedAt:    pooledRepo.CreatedAt,
			IdleTime:     now.Sub(pooledRepo.LastAccessed).String(),
		}

		if pooledRepo.LastAccessed.After(idleThreshold) {
			stats.ActiveRepositories++
		} else {
			stats.IdleRepositories++
		}

		if p.config.EnableMetrics {
			stats.RepositoryDetails[id] = detail
		}

		pooledRepo.mu.RUnlock()
	}

	return stats
}

// Cleanup manually triggers cleanup of idle repositories
func (p *RepositoryPool) Cleanup() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.evictIdleRepositories()
}

// Close shuts down the repository pool
func (p *RepositoryPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Check if already closed
	if p.closed {
		return
	}
	
	p.closed = true
	close(p.done)
	
	if p.cleanupTicker != nil {
		p.cleanupTicker.Stop()
	}

	// Clear all repositories
	p.repositories = make(map[string]*PooledRepository)
}

// startCleanupRoutine starts the background cleanup routine
func (p *RepositoryPool) startCleanupRoutine() {
	p.cleanupTicker = time.NewTicker(p.config.CleanupInterval)
	
	go func() {
		for {
			select {
			case <-p.cleanupTicker.C:
				p.mu.Lock()
				p.evictIdleRepositories()
				p.lastCleanup = time.Now()
				p.mu.Unlock()
			case <-p.done:
				return
			}
		}
	}()
}

// evictIdleRepositories removes repositories that have been idle too long
// Must be called with write lock held
func (p *RepositoryPool) evictIdleRepositories() int {
	evicted := 0
	cutoff := time.Now().Add(-p.config.MaxIdleTime)

	for id, pooledRepo := range p.repositories {
		pooledRepo.mu.RLock()
		lastAccessed := pooledRepo.LastAccessed
		pooledRepo.mu.RUnlock()

		if lastAccessed.Before(cutoff) {
			delete(p.repositories, id)
			evicted++
		}
	}

	return evicted
}

// UpdateAccess updates the access time for a repository
func (pr *PooledRepository) UpdateAccess() {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.LastAccessed = time.Now()
	pr.AccessCount++
}

// GetRepository safely returns the underlying repository
func (pr *PooledRepository) GetRepository() *govc.Repository {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	return pr.Repository
}

// GetStats returns statistics for this pooled repository
func (pr *PooledRepository) GetStats() RepositoryDetail {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	
	return RepositoryDetail{
		ID:           pr.ID,
		Path:         pr.Path,
		LastAccessed: pr.LastAccessed,
		AccessCount:  pr.AccessCount,
		CreatedAt:    pr.CreatedAt,
		IdleTime:     time.Since(pr.LastAccessed).String(),
	}
}