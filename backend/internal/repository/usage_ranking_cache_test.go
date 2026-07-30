//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestUsageRankingCacheRoundTripTTLAndDelete(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cache := NewUsageRankingCache(rdb, &config.Config{
		Dashboard: config.DashboardCacheConfig{Enabled: true, KeyPrefix: "prod"},
	})
	impl, ok := cache.(*usageRankingCache)
	require.True(t, ok)
	require.Equal(t, "prod:"+usageRankingCacheKeyPrefix+"abc", impl.buildKey("abc"))

	_, err := cache.GetUsageRanking(ctx, "abc")
	require.ErrorIs(t, err, service.ErrUsageRankingCacheMiss)

	require.NoError(t, cache.SetUsageRanking(ctx, "abc", `{"metric":"tokens"}`, 90*time.Second))
	value, err := cache.GetUsageRanking(ctx, "abc")
	require.NoError(t, err)
	require.JSONEq(t, `{"metric":"tokens"}`, value)
	require.Equal(t, 90*time.Second, mr.TTL(impl.buildKey("abc")))

	require.NoError(t, cache.DeleteUsageRanking(ctx, "abc"))
	_, err = cache.GetUsageRanking(ctx, "abc")
	require.ErrorIs(t, err, service.ErrUsageRankingCacheMiss)
}

func TestAPIKeyUsageStatsCacheRoundTripTTLAndDelete(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	rankingCache := NewUsageRankingCache(rdb, &config.Config{
		Dashboard: config.DashboardCacheConfig{Enabled: true, KeyPrefix: "prod"},
	})
	cache, ok := rankingCache.(service.APIKeyUsageStatsCache)
	require.True(t, ok)
	impl, ok := rankingCache.(*usageRankingCache)
	require.True(t, ok)
	require.Equal(t, "prod:"+apiKeyUsageStatsCacheKeyPrefix+"abc", impl.buildAPIKeyUsageStatsKey("abc"))

	_, err := cache.GetAPIKeyUsageStats(ctx, "abc")
	require.ErrorIs(t, err, service.ErrAPIKeyUsageStatsCacheMiss)

	require.NoError(t, cache.SetAPIKeyUsageStats(ctx, "abc", `{"version":1}`, 30*time.Second))
	value, err := cache.GetAPIKeyUsageStats(ctx, "abc")
	require.NoError(t, err)
	require.JSONEq(t, `{"version":1}`, value)
	require.Equal(t, 30*time.Second, mr.TTL(impl.buildAPIKeyUsageStatsKey("abc")))

	require.NoError(t, cache.DeleteAPIKeyUsageStats(ctx, "abc"))
	_, err = cache.GetAPIKeyUsageStats(ctx, "abc")
	require.ErrorIs(t, err, service.ErrAPIKeyUsageStatsCacheMiss)
}

func TestUsageRankingCacheRefreshLeaseUsesTokenSafeRelease(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cache := NewUsageRankingCache(rdb, &config.Config{
		Dashboard: config.DashboardCacheConfig{Enabled: true, KeyPrefix: "prod"},
	})
	impl, ok := cache.(*usageRankingCache)
	require.True(t, ok)

	acquired, err := cache.TryAcquireUsageRankingRefresh(ctx, "abc", "owner-a", 30*time.Second)
	require.NoError(t, err)
	require.True(t, acquired)
	require.Equal(t, 30*time.Second, mr.TTL(impl.buildRefreshLockKey("abc")))

	acquired, err = cache.TryAcquireUsageRankingRefresh(ctx, "abc", "owner-b", 30*time.Second)
	require.NoError(t, err)
	require.False(t, acquired)

	// A late or timed-out non-owner must not delete the current owner's lease.
	require.NoError(t, cache.ReleaseUsageRankingRefresh(ctx, "abc", "owner-b"))
	acquired, err = cache.TryAcquireUsageRankingRefresh(ctx, "abc", "owner-c", 30*time.Second)
	require.NoError(t, err)
	require.False(t, acquired)

	require.NoError(t, cache.ReleaseUsageRankingRefresh(ctx, "abc", "owner-a"))
	acquired, err = cache.TryAcquireUsageRankingRefresh(ctx, "abc", "owner-c", 30*time.Second)
	require.NoError(t, err)
	require.True(t, acquired)

	// Simulate owner-c finishing after its lease expired and owner-d took over.
	// The late release must preserve owner-d's newer lease.
	mr.FastForward(31 * time.Second)
	acquired, err = cache.TryAcquireUsageRankingRefresh(ctx, "abc", "owner-d", 30*time.Second)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, cache.ReleaseUsageRankingRefresh(ctx, "abc", "owner-c"))
	acquired, err = cache.TryAcquireUsageRankingRefresh(ctx, "abc", "owner-e", 30*time.Second)
	require.NoError(t, err)
	require.False(t, acquired)
}

func TestNewUsageRankingCacheNormalizesPrefix(t *testing.T) {
	cache := NewUsageRankingCache(nil, &config.Config{
		Dashboard: config.DashboardCacheConfig{Enabled: true, KeyPrefix: "staging:"},
	})
	impl, ok := cache.(*usageRankingCache)
	require.True(t, ok)
	require.Equal(t, "staging:", impl.keyPrefix)

	cache = NewUsageRankingCache(nil, nil)
	impl, ok = cache.(*usageRankingCache)
	require.True(t, ok)
	require.Equal(t, "sub2api:", impl.keyPrefix)
}

func TestNewUsageRankingCacheDisabled(t *testing.T) {
	cache := NewUsageRankingCache(nil, &config.Config{
		Dashboard: config.DashboardCacheConfig{Enabled: false},
	})
	require.Nil(t, cache)
}
