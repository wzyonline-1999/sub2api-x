package usagestats

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPersonalizeUsageRankingDoesNotMutateSharedSnapshot(t *testing.T) {
	snapshot := &UsageRankingSnapshot{
		Metric:      UsageRankingMetricTokens,
		Period:      UsageRankingPeriodWeek,
		GeneratedAt: "2026-07-30T08:00:00Z",
		StartDate:   "2026-07-27",
		EndDate:     "2026-08-02",
		Summary:     UsageRankingSummary{RankedUsers: 3},
		Items: []UsageRankingItem{
			{Rank: 1, UserID: 1, DisplayName: "first", TotalTokens: 300},
			{Rank: 2, UserID: 2, DisplayName: "second", TotalTokens: 200},
			{Rank: 3, UserID: 42, DisplayName: "current", TotalTokens: 100},
		},
	}
	originalItems := append([]UsageRankingItem(nil), snapshot.Items...)

	response := PersonalizeUsageRanking(snapshot, 42, 2)
	require.Len(t, response.Ranking, 2)
	require.Equal(t, int64(42), response.CurrentUser.UserID)
	require.Equal(t, snapshot.GeneratedAt, response.GeneratedAt)

	response.Ranking[0].DisplayName = "changed"
	response.CurrentUser.DisplayName = "changed"
	require.Equal(t, originalItems, snapshot.Items)

	again := PersonalizeUsageRanking(snapshot, 1, 1)
	require.Equal(t, "first", again.Ranking[0].DisplayName)
	require.Equal(t, "first", again.CurrentUser.DisplayName)
}

func TestPersonalizeUsageRankingUsesRequestedLimitFromOneSnapshot(t *testing.T) {
	snapshot := &UsageRankingSnapshot{
		Metric:      UsageRankingMetricTokens,
		Period:      UsageRankingPeriodWeek,
		GeneratedAt: "2026-07-30T08:00:00Z",
		StartDate:   "2026-07-27",
		EndDate:     "2026-08-02",
		Items: []UsageRankingItem{
			{Rank: 1, UserID: 1, TotalTokens: 400},
			{Rank: 2, UserID: 2, TotalTokens: 300},
			{Rank: 3, UserID: 3, TotalTokens: 200},
			{Rank: 4, UserID: 42, TotalTokens: 100},
		},
	}

	topTwo := PersonalizeUsageRanking(snapshot, 42, 2)
	topThree := PersonalizeUsageRanking(snapshot, 42, 3)

	require.Len(t, topTwo.Ranking, 2)
	require.Len(t, topThree.Ranking, 3)
	require.Equal(t, int64(2), *topTwo.CurrentUserTarget.TargetRank)
	require.Equal(t, int64(3), *topThree.CurrentUserTarget.TargetRank)
}

func TestPrepareUsageRankingSnapshotBuildsNonSerializedUserIndex(t *testing.T) {
	snapshot := &UsageRankingSnapshot{
		Metric:      UsageRankingMetricCost,
		Period:      UsageRankingPeriodMonth,
		GeneratedAt: "2026-07-30T08:00:00Z",
		StartDate:   "2026-07-01",
		EndDate:     "2026-07-30",
		Items: []UsageRankingItem{
			{Rank: 1, UserID: 7, DisplayName: "first"},
			{Rank: 2, UserID: 42, DisplayName: "current"},
		},
	}

	PrepareUsageRankingSnapshot(snapshot)

	require.Equal(t, map[int64]int{7: 0, 42: 1}, snapshot.itemIndexByUser)
	response := PersonalizeUsageRanking(snapshot, 42, 1)
	require.Equal(t, int64(42), response.CurrentUser.UserID)
	require.Equal(t, "current", response.CurrentUser.DisplayName)

	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "itemIndexByUser")
	require.NotContains(t, string(encoded), "item_index_by_user")
}
