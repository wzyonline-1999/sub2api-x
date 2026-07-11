package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type visibleCapacityProvider interface {
	GetVisibleGroupCapacity(ctx context.Context, userID int64) (*service.VisibleCapacitySnapshot, error)
}

// CapacityHandler exposes the authenticated user's read-only capacity view.
type CapacityHandler struct {
	capacityService visibleCapacityProvider
}

// NewCapacityHandler creates a CapacityHandler.
func NewCapacityHandler(capacityService *service.VisibleCapacityService) *CapacityHandler {
	return &CapacityHandler{capacityService: capacityService}
}

// GetVisible returns capacity aggregates for groups visible to the current user.
// GET /api/v1/capacity/visible
func (h *CapacityHandler) GetVisible(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	results, err := h.capacityService.GetVisibleGroupCapacity(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, results)
}
