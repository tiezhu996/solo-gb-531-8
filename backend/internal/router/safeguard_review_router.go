package router

import (
	"hazop-safeguard-coverage/backend/internal/constants"
	"hazop-safeguard-coverage/backend/internal/handler"
	"hazop-safeguard-coverage/backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterSafeguardReviewRoutes(api *gin.RouterGroup, h *handler.SafeguardReviewHandler) {
	group := api.Group("/safeguards")
	group.GET("/:id/reviews", middleware.RequirePermission(constants.PermissionRead), h.ListBySafeguard)
	group.GET("/:id/reviews/:review_id", middleware.RequirePermission(constants.PermissionRead), h.Get)
	write := middleware.RequirePermission(constants.PermissionSafeguard)
	group.POST("/:id/reviews", write, h.Open)
	group.POST("/:id/reviews/:review_id/complete", write, h.Complete)

	// 待复评/逾期任务总览（只读）。
	api.GET("/safeguard-reviews", middleware.RequirePermission(constants.PermissionRead), h.ListOpen)
}
