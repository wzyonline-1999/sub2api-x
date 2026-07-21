//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type recordingSubscriptionCache struct {
	*billingCacheMissStub

	mu           sync.Mutex
	subscription *SubscriptionCacheData
	invalidated  []string
	published    []string
}

func newRecordingSubscriptionCache(data *SubscriptionCacheData) *recordingSubscriptionCache {
	return &recordingSubscriptionCache{
		billingCacheMissStub: &billingCacheMissStub{},
		subscription:         data,
	}
}

func (c *recordingSubscriptionCache) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.subscription == nil {
		return nil, errors.New("cache miss")
	}
	cp := *c.subscription
	return &cp, nil
}

func (c *recordingSubscriptionCache) InvalidateSubscriptionCache(_ context.Context, userID, groupID int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invalidated = append(c.invalidated, subCacheKey(userID, groupID))
	return nil
}

func (c *recordingSubscriptionCache) PublishSubscriptionCacheInvalidation(_ context.Context, cacheKey string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.published = append(c.published, cacheKey)
	return nil
}

func (c *recordingSubscriptionCache) SubscribeSubscriptionCacheInvalidation(context.Context, func(string)) error {
	return nil
}

func (c *recordingSubscriptionCache) cacheEvents() (invalidated, published []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.invalidated...), append([]string(nil), c.published...)
}

type fixedSubscriptionSnapshotRepo struct {
	userSubRepoNoop

	mu    sync.Mutex
	sub   UserSubscription
	calls int
}

func (r *fixedSubscriptionSnapshotRepo) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sub.UserID != userID || r.sub.GroupID != groupID {
		return nil, ErrSubscriptionNotFound
	}
	r.calls++
	cp := r.sub
	return &cp, nil
}

func (r *fixedSubscriptionSnapshotRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func newSubscriptionInvalidationTestService(t *testing.T, cache *recordingSubscriptionCache, repo UserSubscriptionRepository, cfg *config.Config) (*BillingCacheService, *SubscriptionService) {
	t.Helper()
	if cfg == nil {
		cfg = &config.Config{}
	}
	billing := NewBillingCacheService(cache, nil, repo, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(billing.Stop)
	subscriptions := NewSubscriptionService(groupRepoNoop{}, repo, billing, nil, cfg)
	t.Cleanup(subscriptions.Stop)
	return billing, subscriptions
}

func TestCheckSubscriptionEligibilitySynchronizesStaleL1WithFreshRedisSnapshot(t *testing.T) {
	now := time.Now()
	repo := &fixedSubscriptionSnapshotRepo{sub: UserSubscription{
		ID:                   1,
		UserID:               10,
		GroupID:              20,
		Status:               SubscriptionStatusSuspended,
		ExpiresAt:            now.Add(-time.Hour),
		DailyUsageUSD:        91,
		WeeklyUsageUSD:       92,
		MonthlyUsageUSD:      93,
		DailyWindowVersion:   1,
		WeeklyWindowVersion:  2,
		MonthlyWindowVersion: 3,
	}}
	freshExpiry := now.Add(2 * time.Hour)
	cache := newRecordingSubscriptionCache(&SubscriptionCacheData{
		Status:               SubscriptionStatusActive,
		ExpiresAt:            freshExpiry,
		DailyUsage:           1,
		WeeklyUsage:          2,
		MonthlyUsage:         3,
		DailyWindowVersion:   11,
		WeeklyWindowVersion:  12,
		MonthlyWindowVersion: 13,
	})
	billing, subscriptions := newSubscriptionInvalidationTestService(t, cache, repo, &config.Config{
		SubscriptionCache: config.SubscriptionCacheConfig{
			L1Size:       128,
			L1TTLSeconds: 60,
		},
	})

	// Prime L1 from the stale database-shaped snapshot and then prove the next
	// read is actually served from L1 rather than the repository.
	_, err := subscriptions.GetActiveSubscription(context.Background(), 10, 20)
	require.NoError(t, err)
	subscriptions.subCacheL1.Wait()
	stale, err := subscriptions.GetActiveSubscription(context.Background(), 10, 20)
	require.NoError(t, err)
	require.Equal(t, 1, repo.callCount())
	require.Equal(t, int64(1), stale.DailyWindowVersion)

	limit := 50.0
	err = billing.checkSubscriptionEligibility(context.Background(), 10, &Group{
		ID:              20,
		DailyLimitUSD:   &limit,
		WeeklyLimitUSD:  &limit,
		MonthlyLimitUSD: &limit,
	}, stale)
	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusActive, stale.Status)
	require.Equal(t, freshExpiry, stale.ExpiresAt)
	require.Equal(t, 1.0, stale.DailyUsageUSD)
	require.Equal(t, 2.0, stale.WeeklyUsageUSD)
	require.Equal(t, 3.0, stale.MonthlyUsageUSD)
	require.Equal(t, int64(11), stale.DailyWindowVersion)
	require.Equal(t, int64(12), stale.WeeklyWindowVersion)
	require.Equal(t, int64(13), stale.MonthlyWindowVersion)
}

func TestAdminResetQuotaPublishesCrossInstanceInvalidation(t *testing.T) {
	repo := &resetQuotaUserSubRepoStub{sub: &UserSubscription{ID: 1, UserID: 10, GroupID: 20}}
	cache := newRecordingSubscriptionCache(nil)
	_, subscriptions := newSubscriptionInvalidationTestService(t, cache, repo, nil)

	_, err := subscriptions.AdminResetQuota(context.Background(), 1, true, false, false)
	require.NoError(t, err)
	invalidated, published := cache.cacheEvents()
	require.Equal(t, []string{"sub:10:20"}, invalidated)
	require.Equal(t, []string{"sub:10:20"}, published)
}

func TestNaturalWindowResetPublishesCrossInstanceInvalidation(t *testing.T) {
	windowStart := time.Now().Add(-25 * time.Hour)
	repo := &resetQuotaUserSubRepoStub{sub: &UserSubscription{ID: 2, UserID: 11, GroupID: 21}}
	cache := newRecordingSubscriptionCache(nil)
	_, subscriptions := newSubscriptionInvalidationTestService(t, cache, repo, nil)
	sub := &UserSubscription{
		ID:               2,
		UserID:           11,
		GroupID:          21,
		StartsAt:         time.Now().Add(-48 * time.Hour),
		ExpiresAt:        time.Now().Add(48 * time.Hour),
		DailyWindowStart: &windowStart,
		DailyUsageUSD:    9,
	}

	err := subscriptions.CheckAndResetWindows(context.Background(), sub)
	require.NoError(t, err)
	invalidated, published := cache.cacheEvents()
	require.Equal(t, []string{"sub:11:21"}, invalidated)
	require.Equal(t, []string{"sub:11:21"}, published)
}

func TestExpiredSubscriptionRenewalPublishesCrossInstanceInvalidation(t *testing.T) {
	groupID := int64(22)
	userID := int64(12)
	repo := newSubscriptionUserSubRepoStub()
	repo.seed(&UserSubscription{
		ID:        3,
		UserID:    userID,
		GroupID:   groupID,
		StartsAt:  time.Now().AddDate(0, 0, -31),
		ExpiresAt: time.Now().Add(-time.Hour),
		Status:    SubscriptionStatusExpired,
		Notes:     "old",
	})
	cache := newRecordingSubscriptionCache(nil)
	billing := NewBillingCacheService(cache, nil, repo, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(billing.Stop)
	subscriptions := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{
		ID:               groupID,
		SubscriptionType: SubscriptionTypeSubscription,
	}}, repo, billing, nil, nil)
	t.Cleanup(subscriptions.Stop)

	_, err := subscriptions.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       userID,
		GroupID:      groupID,
		ValidityDays: 30,
		Notes:        "renewed",
	})
	require.NoError(t, err)
	invalidated, published := cache.cacheEvents()
	require.Equal(t, []string{"sub:12:22"}, invalidated)
	require.Equal(t, []string{"sub:12:22"}, published)
}

func TestRedeemedSubscriptionPostCommitInvalidationPublishesToOtherSlots(t *testing.T) {
	groupID := int64(23)
	repo := &resetQuotaUserSubRepoStub{}
	cache := newRecordingSubscriptionCache(nil)
	billing, subscriptions := newSubscriptionInvalidationTestService(t, cache, repo, nil)
	redeem := &RedeemService{
		subscriptionService: subscriptions,
		billingCacheService: billing,
	}

	redeem.invalidateRedeemCaches(context.Background(), 13, &RedeemCode{
		Type:    RedeemTypeSubscription,
		GroupID: &groupID,
	})
	invalidated, published := cache.cacheEvents()
	require.Equal(t, []string{"sub:13:23"}, invalidated)
	require.Equal(t, []string{"sub:13:23"}, published)
}
