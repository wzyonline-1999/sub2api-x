package handler

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SubscriptionSummaryItem represents a subscription item in summary
type SubscriptionSummaryItem struct {
	ID              int64   `json:"id"`
	GroupID         int64   `json:"group_id"`
	GroupName       string  `json:"group_name"`
	Status          string  `json:"status"`
	DailyUsedUSD    float64 `json:"daily_used_usd,omitempty"`
	DailyLimitUSD   float64 `json:"daily_limit_usd,omitempty"`
	WeeklyUsedUSD   float64 `json:"weekly_used_usd,omitempty"`
	WeeklyLimitUSD  float64 `json:"weekly_limit_usd,omitempty"`
	MonthlyUsedUSD  float64 `json:"monthly_used_usd,omitempty"`
	MonthlyLimitUSD float64 `json:"monthly_limit_usd,omitempty"`
	ExpiresAt       *string `json:"expires_at,omitempty"`
}

// ListResetCards lists reset-card inventory for the current user.
// GET /api/v1/subscription-reset-cards
func (h *SubscriptionHandler) ListResetCards(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	items, err := h.subscriptionService.ListUserSubscriptionResetCardInventory(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

// ListResetCardUsages lists recent reset-card consumption records for the current user.
// GET /api/v1/subscription-reset-cards/usages
func (h *SubscriptionHandler) ListResetCardUsages(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := h.subscriptionService.ListSubscriptionResetCardUsages(c.Request.Context(), service.SubscriptionResetCardListFilter{
		UserID: &subject.UserID,
		Limit:  limit,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

type useResetCardResponse struct {
	Subscription *dto.UserSubscription               `json:"subscription"`
	Usage        *service.SubscriptionResetCardUsage `json:"usage"`
}

func buildUseResetCardResponse(subscription *service.UserSubscription, usage *service.SubscriptionResetCardUsage) useResetCardResponse {
	return useResetCardResponse{
		Subscription: dto.UserSubscriptionFromService(subscription),
		Usage:        usage,
	}
}

// UseResetCard consumes one card and resets all configured quota windows.
// POST /api/v1/subscriptions/:id/reset-card/use
func (h *SubscriptionHandler) UseResetCard(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || subscriptionID <= 0 {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}
	payload := struct {
		SubscriptionID int64 `json:"subscription_id"`
	}{SubscriptionID: subscriptionID}
	const scope = "user.subscription_reset_cards.use"
	result, err := executeUserIdempotent(c, scope, payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		subscription, usage, useErr := h.subscriptionService.UseSubscriptionResetCard(ctx, subject.UserID, subscriptionID, c.GetHeader("Idempotency-Key"))
		if useErr != nil {
			return nil, useErr
		}
		return buildUseResetCardResponse(subscription, usage), nil
	})
	if err != nil {
		reason := infraerrors.Reason(err)
		if reason == infraerrors.Reason(service.ErrIdempotencyInProgress) || reason == infraerrors.Reason(service.ErrIdempotencyStoreUnavail) {
			subscription, usage, recoverErr := h.subscriptionService.RecoverUseSubscriptionResetCard(c.Request.Context(), subject.UserID, subscriptionID, c.GetHeader("Idempotency-Key"))
			if recoverErr != nil {
				logger.LegacyPrintf("handler.subscription", "[ResetCard] committed usage recovery failed: user=%d subscription=%d reason=%s error=%v", subject.UserID, subscriptionID, reason, recoverErr)
			} else if subscription != nil && usage != nil {
				c.Header("X-Idempotency-Recovered", "true")
				response.Success(c, buildUseResetCardResponse(subscription, usage))
				return
			}
		}
		respondUserIdempotentError(c, scope, err)
		return
	}
	if result != nil && result.Replayed {
		c.Header("X-Idempotency-Replayed", "true")
	}
	response.Success(c, result.Data)
}

type updateResetCardPreferenceRequest struct {
	AutoUseEnabled *bool `json:"auto_use_enabled" binding:"required"`
}

// UpdateResetCardPreference updates automatic card use for one subscription group.
// PUT /api/v1/subscription-reset-cards/preferences/:group_id
func (h *SubscriptionHandler) UpdateResetCardPreference(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.BadRequest(c, "Invalid group ID")
		return
	}
	var req updateResetCardPreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.subscriptionService.SetSubscriptionResetAutoUse(c.Request.Context(), subject.UserID, groupID, *req.AutoUseEnabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

// SubscriptionProgressInfo represents subscription with progress info
type SubscriptionProgressInfo struct {
	Subscription *dto.UserSubscription         `json:"subscription"`
	Progress     *service.SubscriptionProgress `json:"progress"`
}

// SubscriptionHandler handles user subscription operations
type SubscriptionHandler struct {
	subscriptionService *service.SubscriptionService
}

// NewSubscriptionHandler creates a new user subscription handler
func NewSubscriptionHandler(subscriptionService *service.SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{
		subscriptionService: subscriptionService,
	}
}

// List handles listing current user's subscriptions
// GET /api/v1/subscriptions
func (h *SubscriptionHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	subscriptions, err := h.subscriptionService.ListUserSubscriptions(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.UserSubscription, 0, len(subscriptions))
	for i := range subscriptions {
		out = append(out, *dto.UserSubscriptionFromService(&subscriptions[i]))
	}
	response.Success(c, out)
}

// GetActive handles getting current user's active subscriptions
// GET /api/v1/subscriptions/active
func (h *SubscriptionHandler) GetActive(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	subscriptions, err := h.subscriptionService.ListActiveUserSubscriptions(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.UserSubscription, 0, len(subscriptions))
	for i := range subscriptions {
		out = append(out, *dto.UserSubscriptionFromService(&subscriptions[i]))
	}
	response.Success(c, out)
}

// GetProgress handles getting subscription progress for current user
// GET /api/v1/subscriptions/progress
func (h *SubscriptionHandler) GetProgress(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	// Get all active subscriptions with progress
	subscriptions, err := h.subscriptionService.ListActiveUserSubscriptions(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result := make([]SubscriptionProgressInfo, 0, len(subscriptions))
	for i := range subscriptions {
		sub := &subscriptions[i]
		progress, err := h.subscriptionService.GetSubscriptionProgress(c.Request.Context(), sub.ID)
		if err != nil {
			// Skip subscriptions with errors
			continue
		}
		result = append(result, SubscriptionProgressInfo{
			Subscription: dto.UserSubscriptionFromService(sub),
			Progress:     progress,
		})
	}

	response.Success(c, result)
}

// GetSummary handles getting a summary of current user's subscription status
// GET /api/v1/subscriptions/summary
func (h *SubscriptionHandler) GetSummary(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	// Get all active subscriptions
	subscriptions, err := h.subscriptionService.ListActiveUserSubscriptions(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var totalUsed float64
	items := make([]SubscriptionSummaryItem, 0, len(subscriptions))

	for _, sub := range subscriptions {
		item := SubscriptionSummaryItem{
			ID:             sub.ID,
			GroupID:        sub.GroupID,
			Status:         sub.Status,
			DailyUsedUSD:   sub.DailyUsageUSD,
			WeeklyUsedUSD:  sub.WeeklyUsageUSD,
			MonthlyUsedUSD: sub.MonthlyUsageUSD,
		}

		// Add group info if preloaded
		if sub.Group != nil {
			item.GroupName = sub.Group.Name
			if sub.Group.DailyLimitUSD != nil {
				item.DailyLimitUSD = *sub.Group.DailyLimitUSD
			}
			if sub.Group.WeeklyLimitUSD != nil {
				item.WeeklyLimitUSD = *sub.Group.WeeklyLimitUSD
			}
			if sub.Group.MonthlyLimitUSD != nil {
				item.MonthlyLimitUSD = *sub.Group.MonthlyLimitUSD
			}
		}

		// Format expiration time
		if !sub.ExpiresAt.IsZero() {
			formatted := sub.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
			item.ExpiresAt = &formatted
		}

		// Track total usage (use monthly as the most comprehensive)
		totalUsed += sub.MonthlyUsageUSD

		items = append(items, item)
	}

	summary := struct {
		ActiveCount   int                       `json:"active_count"`
		TotalUsedUSD  float64                   `json:"total_used_usd"`
		Subscriptions []SubscriptionSummaryItem `json:"subscriptions"`
	}{
		ActiveCount:   len(subscriptions),
		TotalUsedUSD:  totalUsed,
		Subscriptions: items,
	}

	response.Success(c, summary)
}
