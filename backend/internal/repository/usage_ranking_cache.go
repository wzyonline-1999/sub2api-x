package repository

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	usageRankingCacheKeyPrefix       = "usage:rankings:v3:"
	usageRankingRefreshLockKeyPrefix = "usage:rankings:refresh:v1:"
	apiKeyUsageStatsCacheKeyPrefix   = "usage:api-keys:v1:"
)

var releaseUsageRankingRefreshScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
end
return 0
`)

type usageRankingCache struct {
	rdb       *redis.Client
	keyPrefix string
}

// NewUsageRankingCache stores leaderboard snapshots in Redis so blue/green
// instances share the same short-lived aggregation. The existing dashboard prefix
// is reused for environment isolation without introducing another setting.
func NewUsageRankingCache(rdb *redis.Client, cfg *config.Config) service.UsageRankingCache {
	if cfg != nil && !cfg.Dashboard.Enabled {
		return nil
	}
	prefix := "sub2api:"
	if cfg != nil {
		prefix = strings.TrimSpace(cfg.Dashboard.KeyPrefix)
	}
	if prefix != "" && !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	return &usageRankingCache{
		rdb:       rdb,
		keyPrefix: prefix,
	}
}

func (c *usageRankingCache) GetUsageRanking(ctx context.Context, key string) (string, error) {
	value, err := c.rdb.Get(ctx, c.buildKey(key)).Result()
	if err == redis.Nil {
		return "", service.ErrUsageRankingCacheMiss
	}
	return value, err
}

func (c *usageRankingCache) SetUsageRanking(ctx context.Context, key string, data string, ttl time.Duration) error {
	return c.rdb.Set(ctx, c.buildKey(key), data, ttl).Err()
}

func (c *usageRankingCache) DeleteUsageRanking(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, c.buildKey(key)).Err()
}

func (c *usageRankingCache) GetAPIKeyUsageStats(ctx context.Context, key string) (string, error) {
	value, err := c.rdb.Get(ctx, c.buildAPIKeyUsageStatsKey(key)).Result()
	if err == redis.Nil {
		return "", service.ErrAPIKeyUsageStatsCacheMiss
	}
	return value, err
}

func (c *usageRankingCache) SetAPIKeyUsageStats(ctx context.Context, key string, data string, ttl time.Duration) error {
	return c.rdb.Set(ctx, c.buildAPIKeyUsageStatsKey(key), data, ttl).Err()
}

func (c *usageRankingCache) DeleteAPIKeyUsageStats(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, c.buildAPIKeyUsageStatsKey(key)).Err()
}

func (c *usageRankingCache) TryAcquireUsageRankingRefresh(
	ctx context.Context,
	key string,
	token string,
	ttl time.Duration,
) (bool, error) {
	return c.rdb.SetNX(ctx, c.buildRefreshLockKey(key), token, ttl).Result()
}

func (c *usageRankingCache) ReleaseUsageRankingRefresh(ctx context.Context, key string, token string) error {
	return releaseUsageRankingRefreshScript.Run(
		ctx,
		c.rdb,
		[]string{c.buildRefreshLockKey(key)},
		token,
	).Err()
}

func (c *usageRankingCache) buildKey(key string) string {
	return c.keyPrefix + usageRankingCacheKeyPrefix + key
}

func (c *usageRankingCache) buildRefreshLockKey(key string) string {
	return c.keyPrefix + usageRankingRefreshLockKeyPrefix + key
}

func (c *usageRankingCache) buildAPIKeyUsageStatsKey(key string) string {
	return c.keyPrefix + apiKeyUsageStatsCacheKeyPrefix + key
}
