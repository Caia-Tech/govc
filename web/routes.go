package web

import (
	"github.com/gin-gonic/gin"

	"github.com/Caia-Tech/govc/auth"
	"github.com/Caia-Tech/govc/logging"
	"github.com/Caia-Tech/govc/pool"
)

// SetupDashboardRoutes configures all dashboard-related routes
func SetupDashboardRoutes(router *gin.Engine, repositoryPool *pool.RepositoryPool, jwtAuth *auth.JWTAuth, logger *logging.Logger) {
	// Create dashboard handler
	dashboardHandler := NewDashboardHandler(repositoryPool, jwtAuth, logger)
	
	// Start metrics broadcaster
	dashboardHandler.StartMetricsBroadcaster()

	// Serve static files
	router.Static("/static", "./web/static")

	// Dashboard main page
	router.GET("/dashboard", dashboardHandler.ServeHTTP)
	router.GET("/", func(c *gin.Context) {
		c.Redirect(302, "/dashboard")
	})

	// WebSocket endpoint
	router.GET("/ws", dashboardHandler.HandleWebSocket)

	// Dashboard API routes
	dashboardAPI := router.Group("/api/v1/dashboard")
	{
		dashboardAPI.GET("/overview", dashboardHandler.GetOverview)
		dashboardAPI.GET("/performance", dashboardHandler.GetPerformanceMetrics)
		dashboardAPI.GET("/logs", dashboardHandler.GetLogs)
	}

	// Dashboard-specific auth info endpoint (the main API handles other auth)
	dashboardAuthAPI := router.Group("/api/v1/dashboard/auth")
	{
		dashboardAuthAPI.GET("/info", dashboardHandler.GetAuthInfo)
		dashboardAuthAPI.POST("/token/generate", dashboardHandler.GenerateToken)
		dashboardAuthAPI.POST("/token/validate", dashboardHandler.ValidateToken)
	}
}