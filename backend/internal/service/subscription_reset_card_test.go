package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestResetCardRequestIDIsStableAndScoped(t *testing.T) {
	first := resetCardRequestID("manual", 10, 20, "same-key")
	require.NotEmpty(t, first)
	require.Equal(t, first, resetCardRequestID("manual", 10, 20, "same-key"))
	require.NotEqual(t, first, resetCardRequestID("manual", 11, 20, "same-key"))
	require.NotEqual(t, first, resetCardRequestID("auto", 10, 20, "same-key"))
	require.Empty(t, resetCardRequestID("manual", 10, 20, "  "))
}

func TestResetCardAutoRequestIDIsStableWithinWindowAndChangesAcrossWindows(t *testing.T) {
	current := resetCardAutoRequestID(10, 20, "fixed-request", 3, 4, 5)
	require.NotEmpty(t, current)
	require.Equal(t, current, resetCardAutoRequestID(10, 20, "fixed-request", 3, 4, 5), "same-window retries must remain idempotent")

	require.NotEqual(t, current, resetCardAutoRequestID(10, 20, "fixed-request", 4, 4, 5), "a new daily window must be able to consume another card")
	require.NotEqual(t, current, resetCardAutoRequestID(10, 20, "fixed-request", 3, 5, 5), "a new weekly window must be able to consume another card")
	require.NotEqual(t, current, resetCardAutoRequestID(10, 20, "fixed-request", 3, 4, 6), "a new monthly window must be able to consume another card")
	require.Empty(t, resetCardAutoRequestID(10, 20, "  ", 3, 4, 5))
}

func TestUseSubscriptionResetCardRequiresIdempotencyKey(t *testing.T) {
	svc := &SubscriptionService{}
	_, _, err := svc.UseSubscriptionResetCard(context.Background(), 1, 2, "")
	require.ErrorIs(t, err, ErrIdempotencyKeyRequired)
}

func TestResetCardSubscriptionEligibility(t *testing.T) {
	now := time.Now().UTC()
	daily := 10.0
	weekly := 50.0
	group := &dbent.Group{DailyLimitUsd: &daily, WeeklyLimitUsd: &weekly}
	sub := &dbent.UserSubscription{
		StartsAt:       now.Add(-12 * time.Hour),
		ExpiresAt:      now.Add(20 * time.Hour),
		DailyUsageUsd:  10,
		WeeklyUsageUsd: 12,
	}
	require.True(t, resetCardEntSubscriptionIsExhausted(sub, group))
	require.True(t, resetCardEntSubscriptionHasUsage(sub, group))
	require.False(t, resetCardEntSubscriptionIsOneDay(sub))

	sub.ExpiresAt = sub.StartsAt.Add(24 * time.Hour)
	require.True(t, resetCardEntSubscriptionIsOneDay(sub))
}

func TestRenewedSubscriptionTermAdvancesQuotaWindowVersions(t *testing.T) {
	now := time.Now().UTC()
	existing := &UserSubscription{
		DailyWindowVersion:   3,
		WeeklyWindowVersion:  4,
		MonthlyWindowVersion: 5,
		DailyUsageUSD:        8,
		WeeklyUsageUSD:       12,
		MonthlyUsageUSD:      20,
	}

	renewed := renewedSubscriptionTerm(existing, "renewed", now, now.AddDate(0, 0, 30))

	require.Equal(t, int64(4), renewed.DailyWindowVersion)
	require.Equal(t, int64(5), renewed.WeeklyWindowVersion)
	require.Equal(t, int64(6), renewed.MonthlyWindowVersion)
	require.Zero(t, renewed.DailyUsageUSD)
	require.Zero(t, renewed.WeeklyUsageUSD)
	require.Zero(t, renewed.MonthlyUsageUSD)
}

func TestResetCardGrantViewDerivesExpiredStatus(t *testing.T) {
	expiresAt := time.Now().UTC().Add(-time.Minute)
	grant := resetCardGrantFromEnt(&dbent.SubscriptionResetCardGrant{
		Status:    resetCardGrantStatusActive,
		ExpiresAt: &expiresAt,
	})

	require.Equal(t, resetCardGrantStatusExpired, grant.Status)
}
