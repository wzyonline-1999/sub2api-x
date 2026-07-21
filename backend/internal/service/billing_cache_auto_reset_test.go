package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type autoResetEligibilityCache struct {
	BillingCache
}

func (c *autoResetEligibilityCache) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	return &SubscriptionCacheData{
		Status:     SubscriptionStatusActive,
		ExpiresAt:  time.Now().Add(time.Hour),
		DailyUsage: 10,
	}, nil
}

type autoResetEligibilityStub struct {
	called int
}

func (s *autoResetEligibilityStub) TryAutoUseSubscriptionResetCard(_ context.Context, sub *UserSubscription, _ error, _ string) (*UserSubscription, bool, error) {
	s.called++
	copy := *sub
	copy.DailyUsageUSD = 0
	return &copy, true, nil
}

func TestCheckBillingEligibilityRetriesAfterAutomaticReset(t *testing.T) {
	limit := 10.0
	cache := &autoResetEligibilityCache{}
	resetter := &autoResetEligibilityStub{}
	svc := &BillingCacheService{
		cache:                    cache,
		cfg:                      &config.Config{RunMode: config.RunModeStandard},
		subscriptionAutoResetter: resetter,
	}
	sub := &UserSubscription{
		ID:        9,
		UserID:    1,
		GroupID:   2,
		Status:    SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	group := &Group{ID: 2, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &limit}

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, group, sub, "")

	require.NoError(t, err)
	require.Equal(t, 1, resetter.called)
	require.Zero(t, sub.DailyUsageUSD)
}
