package routes

import (
	"shoukehuduan/internal/auth"
	"shoukehuduan/internal/handlers"
	"shoukehuduan/internal/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(h *handlers.Handler, authService *auth.Service) *gin.Engine {
	r := gin.Default()

	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.LoggerMiddleware())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	api := r.Group("/api/v1")
	{
		auth := api.Group("/auth")
		{
			auth.POST("/register", h.Register)
			auth.POST("/login", h.Login)
		}

		protected := api.Group("")
		protected.Use(middleware.JWTAuth(authService))
		{
			user := protected.Group("/user")
			{
				user.GET("/info", h.GetUserInfo)
			}

			vms := protected.Group("/vms")
			{
				vms.GET("", h.ListVMs)
				vms.GET("/:vmid", h.GetVM)
				vms.POST("/:vmid/start", h.StartVM)
				vms.POST("/:vmid/stop", h.StopVM)
				vms.POST("/:vmid/reboot", h.RebootVM)
				vms.POST("/:vmid/connect", h.GetConnection)
			}

			sessions := protected.Group("/sessions")
			{
				sessions.GET("", h.ListSessions)
				sessions.DELETE("/:session_id", h.CloseSession)
			}
		}
	}

	return r
}
