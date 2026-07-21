//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionCacheRejectsOlderWindowSnapshot(t *testing.T) {
	cache, _ := newMiniRedisCache(t)
	ctx := context.Background()
	expiresAt := time.Now().Add(time.Hour)

	fresh := &service.SubscriptionCacheData{
		Status:               service.SubscriptionStatusActive,
		ExpiresAt:            expiresAt,
		DailyUsage:           0,
		Version:              200,
		DailyWindowVersion:   2,
		WeeklyWindowVersion:  2,
		MonthlyWindowVersion: 2,
	}
	require.NoError(t, cache.SetSubscriptionCache(ctx, 1, 2, fresh))

	stale := *fresh
	stale.DailyUsage = 10
	stale.Version = 300
	stale.DailyWindowVersion = 1
	require.NoError(t, cache.SetSubscriptionCache(ctx, 1, 2, &stale))

	got, err := cache.GetSubscriptionCache(ctx, 1, 2)
	require.NoError(t, err)
	require.Zero(t, got.DailyUsage)
	require.Equal(t, int64(2), got.DailyWindowVersion)
}

func TestSubscriptionCacheRejectsDelayedEqualSnapshotWithinSameWindows(t *testing.T) {
	cache, _ := newMiniRedisCache(t)
	ctx := context.Background()
	expiresAt := time.Now().Add(time.Hour)

	fresh := &service.SubscriptionCacheData{
		Status:               service.SubscriptionStatusActive,
		ExpiresAt:            expiresAt,
		DailyUsage:           7,
		Version:              200,
		DailyWindowVersion:   1,
		WeeklyWindowVersion:  1,
		MonthlyWindowVersion: 1,
	}
	require.NoError(t, cache.SetSubscriptionCache(ctx, 3, 4, fresh))

	stale := *fresh
	stale.DailyUsage = 2
	// An equal DB snapshot can arrive after an in-cache usage increment. It
	// must not overwrite the cache just because its window versions match.
	stale.Version = fresh.Version
	require.NoError(t, cache.SetSubscriptionCache(ctx, 3, 4, &stale))

	got, err := cache.GetSubscriptionCache(ctx, 3, 4)
	require.NoError(t, err)
	require.Equal(t, float64(7), got.DailyUsage)
	require.Equal(t, int64(200), got.Version)
}

func TestSubscriptionCacheWindowAwareIncrementIgnoresPreResetRequest(t *testing.T) {
	cache, _ := newMiniRedisCache(t)
	ctx := context.Background()
	data := &service.SubscriptionCacheData{
		Status:               service.SubscriptionStatusActive,
		ExpiresAt:            time.Now().Add(time.Hour),
		Version:              200,
		DailyWindowVersion:   2,
		WeeklyWindowVersion:  2,
		MonthlyWindowVersion: 2,
	}
	require.NoError(t, cache.SetSubscriptionCache(ctx, 5, 6, data))
	require.NoError(t, cache.UpdateSubscriptionUsageForWindows(ctx, 5, 6, 3.5, 1, 1, 1))

	got, err := cache.GetSubscriptionCache(ctx, 5, 6)
	require.NoError(t, err)
	require.Zero(t, got.DailyUsage)
	require.Zero(t, got.WeeklyUsage)
	require.Zero(t, got.MonthlyUsage)
}

func TestSubscriptionCacheRevisionCASRejectsSnapshotAfterUsageOnMiss(t *testing.T) {
	cache, _ := newMiniRedisCache(t)
	ctx := context.Background()

	revision, err := cache.GetSubscriptionCacheRevision(ctx, 7, 8)
	require.NoError(t, err)
	require.Zero(t, revision)

	// Usage updates must advance the mutation revision even when there is no
	// snapshot hash to increment. Otherwise an older DB loader could fill the
	// missing key afterward and hide the concurrent usage.
	require.NoError(t, cache.UpdateSubscriptionUsage(ctx, 7, 8, 4.25))
	newRevision, err := cache.GetSubscriptionCacheRevision(ctx, 7, 8)
	require.NoError(t, err)
	require.Equal(t, int64(1), newRevision)

	stale := &service.SubscriptionCacheData{
		Status:               service.SubscriptionStatusActive,
		ExpiresAt:            time.Now().Add(time.Hour),
		DailyUsage:           0,
		WeeklyUsage:          0,
		MonthlyUsage:         0,
		Version:              100,
		DailyWindowVersion:   1,
		WeeklyWindowVersion:  1,
		MonthlyWindowVersion: 1,
	}
	written, err := cache.SetSubscriptionCacheIfRevision(ctx, 7, 8, revision, stale)
	require.NoError(t, err)
	require.False(t, written, "a snapshot loaded before the usage mutation must be rejected")

	fresh := *stale
	fresh.DailyUsage = 4.25
	fresh.WeeklyUsage = 4.25
	fresh.MonthlyUsage = 4.25
	fresh.Version = 101
	written, err = cache.SetSubscriptionCacheIfRevision(ctx, 7, 8, newRevision, &fresh)
	require.NoError(t, err)
	require.True(t, written)

	got, err := cache.GetSubscriptionCache(ctx, 7, 8)
	require.NoError(t, err)
	require.Equal(t, 4.25, got.DailyUsage)
}

func TestSubscriptionCacheRevisionCASRejectsNewerDBVersionAfterCachedIncrement(t *testing.T) {
	cache, _ := newMiniRedisCache(t)
	ctx := context.Background()

	initial := &service.SubscriptionCacheData{
		Status:               service.SubscriptionStatusActive,
		ExpiresAt:            time.Now().Add(time.Hour),
		DailyUsage:           1,
		WeeklyUsage:          1,
		MonthlyUsage:         1,
		Version:              100,
		DailyWindowVersion:   1,
		WeeklyWindowVersion:  1,
		MonthlyWindowVersion: 1,
	}
	require.NoError(t, cache.SetSubscriptionCache(ctx, 9, 10, initial))
	revision, err := cache.GetSubscriptionCacheRevision(ctx, 9, 10)
	require.NoError(t, err)

	require.NoError(t, cache.UpdateSubscriptionUsage(ctx, 9, 10, 2))
	delayed := *initial
	delayed.Version = 200
	written, err := cache.SetSubscriptionCacheIfRevision(ctx, 9, 10, revision, &delayed)
	require.NoError(t, err)
	require.False(t, written, "a larger snapshot version does not make a pre-mutation DB read safe")

	got, err := cache.GetSubscriptionCache(ctx, 9, 10)
	require.NoError(t, err)
	require.Equal(t, float64(3), got.DailyUsage)
	require.Equal(t, float64(3), got.WeeklyUsage)
	require.Equal(t, float64(3), got.MonthlyUsage)
}

func TestInvalidateSubscriptionCacheAdvancesRevisionAndDeletesLegacyKey(t *testing.T) {
	cache, _ := newMiniRedisCache(t)
	ctx := context.Background()
	data := &service.SubscriptionCacheData{
		Status:    service.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(time.Hour),
		Version:   1,
	}
	require.NoError(t, cache.SetSubscriptionCache(ctx, 11, 12, data))
	require.NoError(t, cache.rdb.HSet(ctx, billingSubLegacyKey(11, 12), subFieldStatus, service.SubscriptionStatusActive).Err())

	require.NoError(t, cache.InvalidateSubscriptionCache(ctx, 11, 12))
	exists, err := cache.rdb.Exists(ctx, billingSubKey(11, 12), billingSubLegacyKey(11, 12)).Result()
	require.NoError(t, err)
	require.Zero(t, exists, "both current and rollback-compatible legacy snapshots must be removed")

	revision, err := cache.GetSubscriptionCacheRevision(ctx, 11, 12)
	require.NoError(t, err)
	require.Equal(t, int64(1), revision)
}
