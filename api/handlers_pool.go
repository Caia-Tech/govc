package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// getPoolStats returns statistics about the repository pool
func (s *Server) getPoolStats(c *gin.Context) {
	stats := s.repoPool.Stats()
	c.JSON(http.StatusOK, stats)
}

// cleanupPool manually triggers pool cleanup
func (s *Server) cleanupPool(c *gin.Context) {
	evicted := s.repoPool.Cleanup()

	c.JSON(http.StatusOK, gin.H{
		"status":          "success",
		"evicted_count":   evicted,
		"remaining_count": s.repoPool.Size(),
		"message":         "Pool cleanup completed",
	})
}
