package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionresetcardgrant"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionresetcardusage"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionresetpreference"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	resetCardGrantStatusActive    = "active"
	resetCardGrantStatusExhausted = "exhausted"
	resetCardGrantStatusExpired   = "expired"
	resetCardUsageModeManual      = "manual"
	resetCardUsageModeAuto        = "auto"
	maxResetCardGrantCount        = 10000
	defaultResetCardListLimit     = 100
	maxResetCardListLimit         = 500
)

var (
	ErrResetCardServiceUnavailable = infraerrors.ServiceUnavailable("RESET_CARD_SERVICE_UNAVAILABLE", "subscription reset card service is unavailable")
	ErrResetCardInvalidCount       = infraerrors.BadRequest("RESET_CARD_INVALID_COUNT", "reset card count must be between 1 and 10000")
	ErrResetCardInvalidExpiry      = infraerrors.BadRequest("RESET_CARD_INVALID_EXPIRY", "reset card expiry must be in the future")
	ErrResetCardNotAvailable       = infraerrors.Conflict("RESET_CARD_NOT_AVAILABLE", "no available reset card for this subscription")
	ErrResetCardNoUsage            = infraerrors.Conflict("RESET_CARD_NO_USAGE", "subscription has no usage to reset")
	ErrResetCardNoQuota            = infraerrors.BadRequest("RESET_CARD_NO_QUOTA", "subscription group has no configured quota")
	ErrResetCardAutoUnavailable    = infraerrors.BadRequest("RESET_CARD_AUTO_UNAVAILABLE", "automatic reset is unavailable for this subscription")
)

// SubscriptionResetCardGrant is the API-safe view of one inventory batch.
type SubscriptionResetCardGrant struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	UserEmail      string     `json:"user_email,omitempty"`
	GroupID        int64      `json:"group_id"`
	GroupName      string     `json:"group_name"`
	IssuedCount    int        `json:"issued_count"`
	RemainingCount int        `json:"remaining_count"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Status         string     `json:"status"`
	Source         string     `json:"source"`
	IssuedBy       *int64     `json:"issued_by"`
	Notes          string     `json:"notes"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// SubscriptionResetCardUsage is an immutable card-consumption audit record.
type SubscriptionResetCardUsage struct {
	ID                      int64     `json:"id"`
	GrantID                 int64     `json:"grant_id"`
	SubscriptionID          int64     `json:"subscription_id"`
	UserID                  int64     `json:"user_id"`
	UserEmail               string    `json:"user_email,omitempty"`
	GroupID                 int64     `json:"group_id"`
	GroupName               string    `json:"group_name"`
	Mode                    string    `json:"mode"`
	PreviousDailyUsageUSD   float64   `json:"previous_daily_usage_usd"`
	PreviousWeeklyUsageUSD  float64   `json:"previous_weekly_usage_usd"`
	PreviousMonthlyUsageUSD float64   `json:"previous_monthly_usage_usd"`
	UsedAt                  time.Time `json:"used_at"`
}

// SubscriptionResetCardInventoryItem is the user-facing aggregate per group.
type SubscriptionResetCardInventoryItem struct {
	GroupID                int64      `json:"group_id"`
	GroupName              string     `json:"group_name"`
	Platform               string     `json:"platform"`
	RemainingCount         int        `json:"remaining_count"`
	NextExpiresAt          *time.Time `json:"next_expires_at"`
	AutoUseEnabled         bool       `json:"auto_use_enabled"`
	AutoUseAvailable       bool       `json:"auto_use_available"`
	EligibleSubscriptionID *int64     `json:"eligible_subscription_id"`
	CanUse                 bool       `json:"can_use"`
	UnavailableReason      string     `json:"unavailable_reason,omitempty"`
}

// GrantSubscriptionResetCardsInput describes one administrator grant.
type GrantSubscriptionResetCardsInput struct {
	UserID         int64
	GroupID        int64
	Count          int
	ExpiresAt      *time.Time
	IssuedBy       int64
	Notes          string
	IdempotencyKey string
}

// SubscriptionResetCardListFilter is shared by admin inventory and usage lists.
type SubscriptionResetCardListFilter struct {
	UserID  *int64
	GroupID *int64
	Limit   int
	Offset  int
}

func normalizeResetCardListLimit(limit int) int {
	if limit <= 0 {
		return defaultResetCardListLimit
	}
	if limit > maxResetCardListLimit {
		return maxResetCardListLimit
	}
	return limit
}

func (s *SubscriptionService) ensureResetCardClient() error {
	if s == nil || s.entClient == nil {
		return ErrResetCardServiceUnavailable
	}
	return nil
}

// refreshSubscriptionResetCaches deletes any pre-reset snapshot and then
// synchronously installs the committed generation. The Redis write is
// generation-aware, so an older concurrent cache fill cannot overwrite it.
func (s *SubscriptionService) refreshSubscriptionResetCaches(userID, groupID int64) error {
	if s.billingCacheService == nil {
		s.InvalidateSubCacheSync(userID, groupID)
		return nil
	}

	cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var cacheErrors []error
	if err := s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID); err != nil {
		cacheErrors = append(cacheErrors, fmt.Errorf("invalidate billing subscription cache: %w", err))
	}

	// Once the shared Redis snapshot has been deleted, immediately invalidate
	// this process and notify the other blue/green slot. A request arriving
	// before the synchronous refill safely performs its own revision-guarded DB
	// fill; concurrent usage advances the revision, so the refill cannot erase
	// that increment. Broadcasting before the refill avoids leaving another
	// instance on the exhausted L1 snapshot for the duration of a slow DB read.
	s.InvalidateSubCacheSync(userID, groupID)
	if err := s.billingCacheService.PublishSubscriptionCacheInvalidation(cacheCtx, subCacheKey(userID, groupID)); err != nil {
		cacheErrors = append(cacheErrors, fmt.Errorf("publish subscription cache invalidation: %w", err))
	}
	if err := s.billingCacheService.RefreshSubscription(cacheCtx, userID, groupID); err != nil {
		cacheErrors = append(cacheErrors, fmt.Errorf("refresh billing subscription cache: %w", err))
	}
	return errors.Join(cacheErrors...)
}

// GrantSubscriptionResetCards grants a batch of group-bound cards to one user.
func (s *SubscriptionService) GrantSubscriptionResetCards(ctx context.Context, input GrantSubscriptionResetCardsInput) (*SubscriptionResetCardGrant, error) {
	if err := s.ensureResetCardClient(); err != nil {
		return nil, err
	}
	if input.Count <= 0 || input.Count > maxResetCardGrantCount {
		return nil, ErrResetCardInvalidCount
	}
	requestID, err := resetCardGrantRequestID(input.IssuedBy, input.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if requestID == "" {
		return nil, ErrIdempotencyKeyRequired
	}
	existing, recoverErr := s.recoverSubscriptionResetCardGrant(ctx, requestID, input)
	if recoverErr != nil || existing != nil {
		return existing, recoverErr
	}
	now := time.Now().UTC()
	if input.ExpiresAt != nil && !input.ExpiresAt.After(now) {
		return nil, ErrResetCardInvalidExpiry
	}
	grantUser, err := s.entClient.User.Query().Where(user.IDEQ(input.UserID)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	grp, err := s.groupRepo.GetByID(ctx, input.GroupID)
	if err != nil {
		return nil, err
	}
	if !grp.IsSubscriptionType() {
		return nil, ErrGroupNotSubscriptionType
	}

	builder := s.entClient.SubscriptionResetCardGrant.Create().
		SetUserID(input.UserID).
		SetGroupID(input.GroupID).
		SetIssuedCount(input.Count).
		SetRemainingCount(input.Count).
		SetStatus(resetCardGrantStatusActive).
		SetSource("admin_grant").
		SetNillableExpiresAt(input.ExpiresAt)
	builder.SetRequestID(requestID)
	if input.IssuedBy > 0 {
		builder.SetIssuedBy(input.IssuedBy)
	}
	if notes := strings.TrimSpace(input.Notes); notes != "" {
		builder.SetNotes(notes)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		// Another worker may have committed the same business operation after
		// our initial lookup. The unique request_id is the final concurrency
		// guard; recover that row instead of issuing a second batch.
		existing, recoverErr := s.recoverSubscriptionResetCardGrant(ctx, requestID, input)
		if recoverErr != nil || existing != nil {
			return existing, recoverErr
		}
		return nil, err
	}

	// User and group were already loaded before the insert. Constructing the
	// response from those values avoids a fallible post-commit hydration query
	// that could otherwise turn a successful grant into an ambiguous failure.
	result := resetCardGrantFromEnt(created)
	result.UserEmail = grantUser.Email
	result.GroupName = grp.Name
	return result, nil
}

// RecoverSubscriptionResetCardGrant performs a read-only lookup for a grant
// that was already committed for the same administrator idempotency key.
func (s *SubscriptionService) RecoverSubscriptionResetCardGrant(ctx context.Context, input GrantSubscriptionResetCardsInput) (*SubscriptionResetCardGrant, error) {
	if err := s.ensureResetCardClient(); err != nil {
		return nil, err
	}
	requestID, err := resetCardGrantRequestID(input.IssuedBy, input.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if requestID == "" {
		return nil, ErrIdempotencyKeyRequired
	}
	return s.recoverSubscriptionResetCardGrant(ctx, requestID, input)
}

func (s *SubscriptionService) recoverSubscriptionResetCardGrant(ctx context.Context, requestID string, input GrantSubscriptionResetCardsInput) (*SubscriptionResetCardGrant, error) {
	row, err := s.entClient.SubscriptionResetCardGrant.Query().
		Where(subscriptionresetcardgrant.RequestIDEQ(requestID)).
		WithUser().
		WithGroup().
		Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !resetCardGrantMatchesInput(row, input) {
		return nil, ErrIdempotencyKeyConflict
	}
	return resetCardGrantFromEnt(row), nil
}

// ListSubscriptionResetCardGrants returns recent grant batches for administrators.
func (s *SubscriptionService) ListSubscriptionResetCardGrants(ctx context.Context, filter SubscriptionResetCardListFilter) ([]SubscriptionResetCardGrant, error) {
	if err := s.ensureResetCardClient(); err != nil {
		return nil, err
	}
	query := s.entClient.SubscriptionResetCardGrant.Query()
	if filter.UserID != nil {
		query.Where(subscriptionresetcardgrant.UserIDEQ(*filter.UserID))
	}
	if filter.GroupID != nil {
		query.Where(subscriptionresetcardgrant.GroupIDEQ(*filter.GroupID))
	}
	rows, err := query.WithUser().WithGroup().
		Order(
			dbent.Desc(subscriptionresetcardgrant.FieldCreatedAt),
			dbent.Desc(subscriptionresetcardgrant.FieldID),
		).
		Limit(normalizeResetCardListLimit(filter.Limit)).
		Offset(max(filter.Offset, 0)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]SubscriptionResetCardGrant, 0, len(rows))
	for _, row := range rows {
		result = append(result, *resetCardGrantFromEnt(row))
	}
	return result, nil
}

// ListSubscriptionResetCardUsages returns immutable usage records.
func (s *SubscriptionService) ListSubscriptionResetCardUsages(ctx context.Context, filter SubscriptionResetCardListFilter) ([]SubscriptionResetCardUsage, error) {
	if err := s.ensureResetCardClient(); err != nil {
		return nil, err
	}
	query := s.entClient.SubscriptionResetCardUsage.Query()
	if filter.UserID != nil {
		query.Where(subscriptionresetcardusage.UserIDEQ(*filter.UserID))
	}
	if filter.GroupID != nil {
		query.Where(subscriptionresetcardusage.GroupIDEQ(*filter.GroupID))
	}
	rows, err := query.WithUser().WithGroup().
		Order(
			dbent.Desc(subscriptionresetcardusage.FieldUsedAt),
			dbent.Desc(subscriptionresetcardusage.FieldID),
		).
		Limit(normalizeResetCardListLimit(filter.Limit)).
		Offset(max(filter.Offset, 0)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]SubscriptionResetCardUsage, 0, len(rows))
	for _, row := range rows {
		result = append(result, *resetCardUsageFromEnt(row))
	}
	return result, nil
}

// ListUserSubscriptionResetCardInventory aggregates inventory, subscriptions and preferences by group.
func (s *SubscriptionService) ListUserSubscriptionResetCardInventory(ctx context.Context, userID int64) ([]SubscriptionResetCardInventoryItem, error) {
	if err := s.ensureResetCardClient(); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	grants, err := s.entClient.SubscriptionResetCardGrant.Query().
		Where(
			subscriptionresetcardgrant.UserIDEQ(userID),
			subscriptionresetcardgrant.StatusEQ(resetCardGrantStatusActive),
			subscriptionresetcardgrant.RemainingCountGT(0),
			subscriptionresetcardgrant.Or(
				subscriptionresetcardgrant.ExpiresAtIsNil(),
				subscriptionresetcardgrant.ExpiresAtGT(now),
			),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	subs, err := s.entClient.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.StatusEQ(SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(now),
		).
		WithGroup().
		All(ctx)
	if err != nil {
		return nil, err
	}
	prefs, err := s.entClient.SubscriptionResetPreference.Query().
		Where(subscriptionresetpreference.UserIDEQ(userID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	type inventoryAccumulator struct {
		item *SubscriptionResetCardInventoryItem
		sub  *dbent.UserSubscription
	}
	byGroup := make(map[int64]*inventoryAccumulator)
	ensure := func(groupID int64) *inventoryAccumulator {
		if current := byGroup[groupID]; current != nil {
			return current
		}
		current := &inventoryAccumulator{item: &SubscriptionResetCardInventoryItem{GroupID: groupID}}
		byGroup[groupID] = current
		return current
	}
	for _, grant := range grants {
		current := ensure(grant.GroupID)
		current.item.RemainingCount += grant.RemainingCount
		if grant.ExpiresAt != nil && (current.item.NextExpiresAt == nil || grant.ExpiresAt.Before(*current.item.NextExpiresAt)) {
			expiresAt := *grant.ExpiresAt
			current.item.NextExpiresAt = &expiresAt
		}
	}
	for _, sub := range subs {
		current := ensure(sub.GroupID)
		current.sub = sub
		subID := sub.ID
		current.item.EligibleSubscriptionID = &subID
		if sub.Edges.Group != nil {
			current.item.GroupName = sub.Edges.Group.Name
			current.item.Platform = sub.Edges.Group.Platform
		}
	}
	for _, pref := range prefs {
		ensure(pref.GroupID).item.AutoUseEnabled = pref.AutoUseEnabled
	}

	groupIDs := make([]int64, 0, len(byGroup))
	for groupID := range byGroup {
		groupIDs = append(groupIDs, groupID)
	}
	if len(groupIDs) > 0 {
		groups, queryErr := s.entClient.Group.Query().Where(group.IDIn(groupIDs...)).All(ctx)
		if queryErr != nil {
			return nil, queryErr
		}
		for _, grp := range groups {
			current := ensure(grp.ID)
			current.item.GroupName = grp.Name
			current.item.Platform = grp.Platform
			configureResetCardAvailability(current.item, current.sub, grp)
		}
	}

	result := make([]SubscriptionResetCardInventoryItem, 0, len(byGroup))
	for _, current := range byGroup {
		result = append(result, *current.item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CanUse != result[j].CanUse {
			return result[i].CanUse
		}
		return result[i].GroupName < result[j].GroupName
	})
	return result, nil
}

func configureResetCardAvailability(item *SubscriptionResetCardInventoryItem, sub *dbent.UserSubscription, grp *dbent.Group) {
	if item == nil || grp == nil {
		return
	}
	if sub == nil {
		item.UnavailableReason = "no_active_subscription"
		return
	}
	hasQuota := grp.DailyLimitUsd != nil || grp.WeeklyLimitUsd != nil || grp.MonthlyLimitUsd != nil
	item.AutoUseAvailable = hasQuota && !resetCardEntSubscriptionIsOneDay(sub)
	if !hasQuota {
		item.UnavailableReason = "no_configured_quota"
		return
	}
	if item.RemainingCount <= 0 {
		item.UnavailableReason = "no_cards"
		return
	}
	if !resetCardEntSubscriptionHasUsage(sub, grp) {
		item.UnavailableReason = "nothing_to_reset"
		return
	}
	item.CanUse = true
}

// SetSubscriptionResetAutoUse saves the automatic consumption preference for one group.
func (s *SubscriptionService) SetSubscriptionResetAutoUse(ctx context.Context, userID, groupID int64, enabled bool) (*SubscriptionResetCardInventoryItem, error) {
	if err := s.ensureResetCardClient(); err != nil {
		return nil, err
	}
	if enabled {
		sub, err := s.entClient.UserSubscription.Query().
			Where(
				usersubscription.UserIDEQ(userID),
				usersubscription.GroupIDEQ(groupID),
				usersubscription.StatusEQ(SubscriptionStatusActive),
				usersubscription.ExpiresAtGT(time.Now()),
			).
			WithGroup().
			Only(ctx)
		if err != nil {
			if dbent.IsNotFound(err) {
				return nil, ErrSubscriptionNotFound
			}
			return nil, err
		}
		if sub.Edges.Group == nil || (sub.Edges.Group.DailyLimitUsd == nil && sub.Edges.Group.WeeklyLimitUsd == nil && sub.Edges.Group.MonthlyLimitUsd == nil) || resetCardEntSubscriptionIsOneDay(sub) {
			return nil, ErrResetCardAutoUnavailable
		}
	}
	now := time.Now().UTC()
	err := s.entClient.SubscriptionResetPreference.Create().
		SetUserID(userID).
		SetGroupID(groupID).
		SetAutoUseEnabled(enabled).
		SetUpdatedAt(now).
		OnConflictColumns(subscriptionresetpreference.FieldUserID, subscriptionresetpreference.FieldGroupID).
		Update(func(update *dbent.SubscriptionResetPreferenceUpsert) {
			update.SetAutoUseEnabled(enabled)
			update.SetUpdatedAt(now)
		}).
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.ListUserSubscriptionResetCardInventory(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].GroupID == groupID {
			return &items[i], nil
		}
	}
	return &SubscriptionResetCardInventoryItem{GroupID: groupID, AutoUseEnabled: enabled}, nil
}

// UseSubscriptionResetCard manually consumes one card and resets all configured quota windows.
func (s *SubscriptionService) UseSubscriptionResetCard(ctx context.Context, userID, subscriptionID int64, idempotencyKey string) (*UserSubscription, *SubscriptionResetCardUsage, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return nil, nil, ErrIdempotencyKeyRequired
	}
	requestID := resetCardRequestID(resetCardUsageModeManual, userID, subscriptionID, idempotencyKey)
	return s.consumeSubscriptionResetCard(ctx, userID, subscriptionID, resetCardUsageModeManual, requestID, false)
}

// RecoverUseSubscriptionResetCard performs a strictly read-only lookup for a
// previously committed manual reset. It is used only when the outer
// idempotency record is ambiguous; a cache/store failure before execution must
// never cause this method to consume a card.
func (s *SubscriptionService) RecoverUseSubscriptionResetCard(ctx context.Context, userID, subscriptionID int64, idempotencyKey string) (*UserSubscription, *SubscriptionResetCardUsage, error) {
	if err := s.ensureResetCardClient(); err != nil {
		return nil, nil, err
	}
	if s.userSubRepo == nil {
		return nil, nil, ErrResetCardServiceUnavailable
	}
	key, err := NormalizeIdempotencyKey(idempotencyKey)
	if err != nil {
		return nil, nil, err
	}
	if key == "" {
		return nil, nil, ErrIdempotencyKeyRequired
	}
	requestID := resetCardRequestID(resetCardUsageModeManual, userID, subscriptionID, key)
	usageRow, err := s.entClient.SubscriptionResetCardUsage.Query().
		Where(
			subscriptionresetcardusage.RequestIDEQ(requestID),
			subscriptionresetcardusage.UserIDEQ(userID),
			subscriptionresetcardusage.SubscriptionIDEQ(subscriptionID),
			subscriptionresetcardusage.ModeEQ(resetCardUsageModeManual),
		).
		WithUser().
		WithGroup().
		Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	subscription, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, nil, err
	}
	if subscription == nil || subscription.ID != subscriptionID || subscription.UserID != userID {
		return nil, nil, ErrSubscriptionNotFound
	}
	return subscription, resetCardUsageFromEnt(usageRow), nil
}

// TryAutoUseSubscriptionResetCard consumes a card only when automatic use is enabled and the locked subscription is still exhausted.
func (s *SubscriptionService) TryAutoUseSubscriptionResetCard(ctx context.Context, sub *UserSubscription, limitErr error, requestKey string) (*UserSubscription, bool, error) {
	if sub == nil || !isSubscriptionLimitError(limitErr) {
		return sub, false, nil
	}
	if err := s.ensureResetCardClient(); err != nil {
		return sub, false, err
	}
	enabled, err := s.entClient.SubscriptionResetPreference.Query().
		Where(
			subscriptionresetpreference.UserIDEQ(sub.UserID),
			subscriptionresetpreference.GroupIDEQ(sub.GroupID),
			subscriptionresetpreference.AutoUseEnabledEQ(true),
		).
		Exist(ctx)
	if err != nil || !enabled {
		return sub, false, err
	}
	refreshed, _, err := s.consumeSubscriptionResetCard(ctx, sub.UserID, sub.ID, resetCardUsageModeAuto, requestKey, true)
	if errors.Is(err, ErrResetCardNotAvailable) || errors.Is(err, ErrResetCardAutoUnavailable) {
		return sub, false, nil
	}
	if err != nil {
		return sub, false, err
	}
	return refreshed, true, nil
}

func (s *SubscriptionService) consumeSubscriptionResetCard(ctx context.Context, userID, subscriptionID int64, mode, requestToken string, requireExhausted bool) (*UserSubscription, *SubscriptionResetCardUsage, error) {
	if err := s.ensureResetCardClient(); err != nil {
		return nil, nil, err
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin reset card transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	client := tx.Client()

	lockedSub, err := client.UserSubscription.Query().
		Where(usersubscription.IDEQ(subscriptionID), usersubscription.UserIDEQ(userID)).
		WithGroup().
		ForUpdate().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil, ErrSubscriptionNotFound
		}
		return nil, nil, err
	}
	requestID := requestToken
	if mode == resetCardUsageModeAuto {
		// Derive automatic idempotency from the quota generations read under the
		// subscription row lock. This remains correct even if the admission/L1
		// snapshot was stale: retries in one window share an ID, while the next
		// exhausted window gets a new ID despite a reused X-Request-ID.
		requestID = resetCardAutoRequestID(
			userID,
			subscriptionID,
			requestToken,
			lockedSub.DailyWindowVersion,
			lockedSub.WeeklyWindowVersion,
			lockedSub.MonthlyWindowVersion,
		)
	}
	if requestID != "" {
		existing, existingErr := client.SubscriptionResetCardUsage.Query().
			Where(subscriptionresetcardusage.RequestIDEQ(requestID)).
			WithUser().
			WithGroup().
			Only(ctx)
		if existingErr == nil {
			if err := tx.Commit(); err != nil {
				return nil, nil, err
			}
			if err := s.refreshSubscriptionResetCaches(userID, lockedSub.GroupID); err != nil {
				log.Printf("Warning: reset card replay found committed usage but cache refresh failed: user=%d group=%d error=%v", userID, lockedSub.GroupID, err)
			}
			refreshed, fetchErr := s.userSubRepo.GetByID(ctx, subscriptionID)
			return refreshed, resetCardUsageFromEnt(existing), fetchErr
		}
		if !dbent.IsNotFound(existingErr) {
			return nil, nil, existingErr
		}
	}
	now := time.Now().UTC()
	if lockedSub.Status != SubscriptionStatusActive || !lockedSub.ExpiresAt.After(now) {
		return nil, nil, ErrSubscriptionExpired
	}
	grp := lockedSub.Edges.Group
	if grp == nil || (grp.DailyLimitUsd == nil && grp.WeeklyLimitUsd == nil && grp.MonthlyLimitUsd == nil) {
		return nil, nil, ErrResetCardNoQuota
	}
	if mode == resetCardUsageModeAuto && resetCardEntSubscriptionIsOneDay(lockedSub) {
		return nil, nil, ErrResetCardAutoUnavailable
	}
	if mode == resetCardUsageModeAuto {
		_, preferenceErr := client.SubscriptionResetPreference.Query().
			Where(
				subscriptionresetpreference.UserIDEQ(userID),
				subscriptionresetpreference.GroupIDEQ(lockedSub.GroupID),
				subscriptionresetpreference.AutoUseEnabledEQ(true),
			).
			ForUpdate().
			Only(ctx)
		if preferenceErr != nil {
			if dbent.IsNotFound(preferenceErr) {
				return nil, nil, ErrResetCardAutoUnavailable
			}
			return nil, nil, preferenceErr
		}
	}

	if requireExhausted {
		if !resetCardEntSubscriptionIsExhausted(lockedSub, grp) {
			if err := tx.Commit(); err != nil {
				return nil, nil, err
			}
			if err := s.refreshSubscriptionResetCaches(userID, lockedSub.GroupID); err != nil {
				log.Printf("Warning: automatic reset found a fresh subscription but cache refresh failed: user=%d group=%d error=%v", userID, lockedSub.GroupID, err)
			}
			refreshed, fetchErr := s.userSubRepo.GetByID(ctx, subscriptionID)
			return refreshed, nil, fetchErr
		}
	} else if !resetCardEntSubscriptionHasUsage(lockedSub, grp) {
		return nil, nil, ErrResetCardNoUsage
	}

	grant, err := client.SubscriptionResetCardGrant.Query().
		Where(
			subscriptionresetcardgrant.UserIDEQ(userID),
			subscriptionresetcardgrant.GroupIDEQ(lockedSub.GroupID),
			subscriptionresetcardgrant.StatusEQ(resetCardGrantStatusActive),
			subscriptionresetcardgrant.RemainingCountGT(0),
			subscriptionresetcardgrant.Or(
				subscriptionresetcardgrant.ExpiresAtIsNil(),
				subscriptionresetcardgrant.ExpiresAtGT(now),
			),
		).
		Order(
			subscriptionresetcardgrant.ByExpiresAt(sql.OrderAsc(), sql.OrderNullsLast()),
			subscriptionresetcardgrant.ByCreatedAt(sql.OrderAsc()),
		).
		ForUpdate(sql.WithLockAction(sql.SkipLocked)).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil, ErrResetCardNotAvailable
		}
		return nil, nil, err
	}

	grantUpdate := client.SubscriptionResetCardGrant.UpdateOneID(grant.ID).
		AddRemainingCount(-1).
		SetUpdatedAt(now)
	if grant.RemainingCount == 1 {
		grantUpdate.SetStatus(resetCardGrantStatusExhausted)
	}
	if _, err := grantUpdate.Save(ctx); err != nil {
		return nil, nil, err
	}

	subUpdate := client.UserSubscription.UpdateOneID(lockedSub.ID)
	if grp.DailyLimitUsd != nil {
		subUpdate.SetDailyUsageUsd(0).SetDailyWindowStart(now).AddDailyWindowVersion(1)
	}
	if grp.WeeklyLimitUsd != nil {
		subUpdate.SetWeeklyUsageUsd(0).SetWeeklyWindowStart(now).AddWeeklyWindowVersion(1)
	}
	if grp.MonthlyLimitUsd != nil {
		subUpdate.SetMonthlyUsageUsd(0).SetMonthlyWindowStart(now).AddMonthlyWindowVersion(1)
	}
	if _, err := subUpdate.Save(ctx); err != nil {
		return nil, nil, err
	}

	usageBuilder := client.SubscriptionResetCardUsage.Create().
		SetGrantID(grant.ID).
		SetSubscriptionID(lockedSub.ID).
		SetUserID(userID).
		SetGroupID(lockedSub.GroupID).
		SetMode(mode).
		SetPreviousDailyUsageUsd(lockedSub.DailyUsageUsd).
		SetPreviousWeeklyUsageUsd(lockedSub.WeeklyUsageUsd).
		SetPreviousMonthlyUsageUsd(lockedSub.MonthlyUsageUsd).
		SetNillablePreviousDailyWindowStart(lockedSub.DailyWindowStart).
		SetNillablePreviousWeeklyWindowStart(lockedSub.WeeklyWindowStart).
		SetNillablePreviousMonthlyWindowStart(lockedSub.MonthlyWindowStart).
		SetUsedAt(now)
	if requestID != "" {
		usageBuilder.SetRequestID(requestID)
	}
	createdUsage, err := usageBuilder.Save(ctx)
	if err != nil {
		return nil, nil, err
	}
	createdUsage, err = client.SubscriptionResetCardUsage.Query().
		Where(subscriptionresetcardusage.IDEQ(createdUsage.ID)).
		WithUser().
		WithGroup().
		Only(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	if err := s.refreshSubscriptionResetCaches(userID, lockedSub.GroupID); err != nil {
		log.Printf("Warning: reset card committed but subscription cache invalidation failed: user=%d group=%d error=%v", userID, lockedSub.GroupID, err)
	}
	refreshed, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, nil, err
	}
	return refreshed, resetCardUsageFromEnt(createdUsage), nil
}

func isSubscriptionLimitError(err error) bool {
	return errors.Is(err, ErrDailyLimitExceeded) || errors.Is(err, ErrWeeklyLimitExceeded) || errors.Is(err, ErrMonthlyLimitExceeded)
}

func resetCardEntSubscriptionIsExhausted(sub *dbent.UserSubscription, grp *dbent.Group) bool {
	if sub == nil || grp == nil {
		return false
	}
	return (grp.DailyLimitUsd != nil && sub.DailyUsageUsd >= *grp.DailyLimitUsd) ||
		(grp.WeeklyLimitUsd != nil && sub.WeeklyUsageUsd >= *grp.WeeklyLimitUsd) ||
		(grp.MonthlyLimitUsd != nil && sub.MonthlyUsageUsd >= *grp.MonthlyLimitUsd)
}

func resetCardEntSubscriptionHasUsage(sub *dbent.UserSubscription, grp *dbent.Group) bool {
	if sub == nil || grp == nil {
		return false
	}
	const epsilon = 0.0000000001
	return (grp.DailyLimitUsd != nil && sub.DailyUsageUsd > epsilon) ||
		(grp.WeeklyLimitUsd != nil && sub.WeeklyUsageUsd > epsilon) ||
		(grp.MonthlyLimitUsd != nil && sub.MonthlyUsageUsd > epsilon)
}

func resetCardEntSubscriptionIsOneDay(sub *dbent.UserSubscription) bool {
	if sub == nil || sub.StartsAt.IsZero() || sub.ExpiresAt.IsZero() {
		return false
	}
	return !sub.ExpiresAt.After(sub.StartsAt.AddDate(0, 0, 1))
}

func resetCardRequestID(mode string, userID, subscriptionID int64, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	raw := fmt.Sprintf("%s|%d|%d|%s", mode, userID, subscriptionID, key)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func resetCardAutoRequestID(userID, subscriptionID int64, key string, dailyVersion, weeklyVersion, monthlyVersion int64) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	raw := fmt.Sprintf(
		"%s|%d|%d|%s|daily:%d|weekly:%d|monthly:%d",
		resetCardUsageModeAuto,
		userID,
		subscriptionID,
		key,
		dailyVersion,
		weeklyVersion,
		monthlyVersion,
	)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func resetCardGrantRequestID(issuedBy int64, key string) (string, error) {
	normalizedKey, err := NormalizeIdempotencyKey(key)
	if err != nil || normalizedKey == "" {
		return "", err
	}
	raw := fmt.Sprintf("admin.subscription_reset_cards.grant|%d|%s", issuedBy, normalizedKey)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:]), nil
}

func resetCardGrantMatchesInput(row *dbent.SubscriptionResetCardGrant, input GrantSubscriptionResetCardsInput) bool {
	if row == nil || row.UserID != input.UserID || row.GroupID != input.GroupID || row.IssuedCount != input.Count {
		return false
	}
	if strings.TrimSpace(stringValue(row.Notes)) != strings.TrimSpace(input.Notes) {
		return false
	}
	if (row.IssuedBy == nil) != (input.IssuedBy <= 0) {
		return false
	}
	if row.IssuedBy != nil && *row.IssuedBy != input.IssuedBy {
		return false
	}
	if row.ExpiresAt == nil || input.ExpiresAt == nil {
		return row.ExpiresAt == nil && input.ExpiresAt == nil
	}
	// PostgreSQL stores timestamptz with microsecond precision. Normalize both
	// sides so a valid retry with sub-microsecond JSON precision still matches.
	return row.ExpiresAt.UTC().Round(time.Microsecond).Equal(input.ExpiresAt.UTC().Round(time.Microsecond))
}

func resetCardGrantFromEnt(row *dbent.SubscriptionResetCardGrant) *SubscriptionResetCardGrant {
	if row == nil {
		return nil
	}
	result := &SubscriptionResetCardGrant{
		ID:             row.ID,
		UserID:         row.UserID,
		GroupID:        row.GroupID,
		IssuedCount:    row.IssuedCount,
		RemainingCount: row.RemainingCount,
		ExpiresAt:      row.ExpiresAt,
		Status:         row.Status,
		Source:         row.Source,
		IssuedBy:       row.IssuedBy,
		Notes:          stringValue(row.Notes),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if result.Status == resetCardGrantStatusActive && result.ExpiresAt != nil && !result.ExpiresAt.After(time.Now().UTC()) {
		result.Status = resetCardGrantStatusExpired
	}
	if row.Edges.User != nil {
		result.UserEmail = row.Edges.User.Email
	}
	if row.Edges.Group != nil {
		result.GroupName = row.Edges.Group.Name
	}
	return result
}

func resetCardUsageFromEnt(row *dbent.SubscriptionResetCardUsage) *SubscriptionResetCardUsage {
	if row == nil {
		return nil
	}
	result := &SubscriptionResetCardUsage{
		ID:                      row.ID,
		GrantID:                 row.GrantID,
		SubscriptionID:          row.SubscriptionID,
		UserID:                  row.UserID,
		GroupID:                 row.GroupID,
		Mode:                    row.Mode,
		PreviousDailyUsageUSD:   row.PreviousDailyUsageUsd,
		PreviousWeeklyUsageUSD:  row.PreviousWeeklyUsageUsd,
		PreviousMonthlyUsageUSD: row.PreviousMonthlyUsageUsd,
		UsedAt:                  row.UsedAt,
	}
	if row.Edges.User != nil {
		result.UserEmail = row.Edges.User.Email
	}
	if row.Edges.Group != nil {
		result.GroupName = row.Edges.Group.Name
	}
	return result
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
