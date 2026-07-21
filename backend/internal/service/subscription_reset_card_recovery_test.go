package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecoverUseSubscriptionResetCardIsReadOnlyAndScoped(t *testing.T) {
	fixture := newResetCardConsumeFixture(t, true)
	grant := fixture.createGrant(t, 2, nil)
	const key = "manual-recovery-read-only"

	_, committedUsage, err := fixture.service.UseSubscriptionResetCard(
		fixture.ctx,
		fixture.userID,
		fixture.subscription.ID,
		key,
	)
	require.NoError(t, err)

	recoveredSubscription, recoveredUsage, err := fixture.service.RecoverUseSubscriptionResetCard(
		fixture.ctx,
		fixture.userID,
		fixture.subscription.ID,
		key,
	)
	require.NoError(t, err)
	require.NotNil(t, recoveredSubscription)
	require.NotNil(t, recoveredUsage)
	require.Equal(t, committedUsage.ID, recoveredUsage.ID)

	otherUserSubscription, otherUserUsage, err := fixture.service.RecoverUseSubscriptionResetCard(
		fixture.ctx,
		fixture.userID+1000,
		fixture.subscription.ID,
		key,
	)
	require.NoError(t, err)
	require.Nil(t, otherUserSubscription)
	require.Nil(t, otherUserUsage)

	otherSubscription, otherSubscriptionUsage, err := fixture.service.RecoverUseSubscriptionResetCard(
		fixture.ctx,
		fixture.userID,
		fixture.subscription.ID+1000,
		key,
	)
	require.NoError(t, err)
	require.Nil(t, otherSubscription)
	require.Nil(t, otherSubscriptionUsage)

	require.Equal(t, 1, fixture.client.SubscriptionResetCardUsage.Query().CountX(fixture.ctx))
	require.Equal(t, 1, fixture.client.SubscriptionResetCardGrant.GetX(fixture.ctx, grant.ID).RemainingCount)
	loadedSubscription := fixture.client.UserSubscription.GetX(fixture.ctx, fixture.subscription.ID)
	require.Equal(t, int64(5), loadedSubscription.DailyWindowVersion)
	require.Equal(t, int64(6), loadedSubscription.WeeklyWindowVersion)
	require.Equal(t, int64(7), loadedSubscription.MonthlyWindowVersion)
}
