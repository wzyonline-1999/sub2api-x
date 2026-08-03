package routes

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const readinessDatabaseTimeout = 2 * time.Second

// DatabasePinger is the minimal dependency needed by the readiness endpoint.
// *sql.DB implements it directly, while the narrow interface keeps route tests
// independent from a real database.
type DatabasePinger interface {
	PingContext(ctx context.Context) error
}

// RegisterCommonRoutes 注册通用路由（健康检查、状态等）
func RegisterCommonRoutes(r *gin.Engine, database DatabasePinger) {
	// Liveness only: this remains independent of external dependencies so an
	// orchestrator does not restart a healthy process during a database incident.
	r.GET("/health", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Readiness: a release must be able to acquire and use a database connection
	// before it can receive traffic. Keep the check lightweight and bounded.
	r.GET("/ready", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if database == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not_ready",
				"checks": gin.H{"database": "unavailable"},
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), readinessDatabaseTimeout)
		defer cancel()
		if err := database.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not_ready",
				"checks": gin.H{"database": "unavailable"},
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
			"checks": gin.H{"database": "ok"},
		})
	})

	// Claude Code 遥测日志（忽略，直接返回200）
	r.POST("/api/event_logging/batch", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Setup status endpoint (always returns needs_setup: false in normal mode)
	// This is used by the frontend to detect when the service has restarted after setup
	r.GET("/setup/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"needs_setup": false,
				"step":        "completed",
			},
		})
	})
}
