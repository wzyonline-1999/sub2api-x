package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type visibleGroupListerStub struct {
	groups        []Group
	err           error
	requestedUser []int64
}

func (s *visibleGroupListerStub) GetAvailableGroups(_ context.Context, userID int64) ([]Group, error) {
	s.requestedUser = append(s.requestedUser, userID)
	if s.err != nil {
		return nil, s.err
	}
	return append([]Group(nil), s.groups...), nil
}

type visibleCapacityAccountRepoStub struct {
	AccountRepository
	rows           []GroupAccountCapacityRow
	err            error
	requested      []int64
	calls          int
	quotaRows      []GroupAccountQuotaLoadRow
	quotaErr       error
	quotaRequested []int64
	quotaCalls     int
}

func (s *visibleCapacityAccountRepoStub) ListSchedulableCapacityByGroupIDs(_ context.Context, groupIDs []int64) ([]GroupAccountCapacityRow, error) {
	s.calls++
	s.requested = append([]int64(nil), groupIDs...)
	if s.err != nil {
		return nil, s.err
	}
	return append([]GroupAccountCapacityRow(nil), s.rows...), nil
}

func (s *visibleCapacityAccountRepoStub) ListStableQuotaLoadByGroupIDs(_ context.Context, groupIDs []int64) ([]GroupAccountQuotaLoadRow, error) {
	s.quotaCalls++
	s.quotaRequested = append([]int64(nil), groupIDs...)
	if s.quotaErr != nil {
		return nil, s.quotaErr
	}
	return append([]GroupAccountQuotaLoadRow(nil), s.quotaRows...), nil
}

type visibleCapacityFallbackRepoStub struct {
	AccountRepository
	accounts     map[int64][]Account
	allAccounts  map[int64][]Account
	requested    []int64
	requestedAll []int64
}

func (s *visibleCapacityFallbackRepoStub) ListSchedulableByGroupID(_ context.Context, groupID int64) ([]Account, error) {
	s.requested = append(s.requested, groupID)
	return append([]Account(nil), s.accounts[groupID]...), nil
}

func (s *visibleCapacityFallbackRepoStub) ListByGroup(_ context.Context, groupID int64) ([]Account, error) {
	s.requestedAll = append(s.requestedAll, groupID)
	return append([]Account(nil), s.allAccounts[groupID]...), nil
}

type visibleCapacityLoadStub struct {
	loads     map[int64]*AccountLoadInfo
	err       error
	requested []AccountWithConcurrency
	calls     int
}

func (s *visibleCapacityLoadStub) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	s.calls++
	s.requested = append([]AccountWithConcurrency(nil), accounts...)
	if s.err != nil {
		return nil, s.err
	}
	return s.loads, nil
}

func TestVisibleCapacityAggregatesOnlyVisibleGroups(t *testing.T) {
	groups := &visibleGroupListerStub{groups: []Group{
		{ID: 10, Name: "Claude", Platform: PlatformAnthropic, AccountCount: 4, ActiveAccountCount: 3},
		{ID: 20, Name: "OpenAI", Platform: PlatformOpenAI, AccountCount: 2, ActiveAccountCount: 1},
	}}
	accounts := &visibleCapacityAccountRepoStub{rows: []GroupAccountCapacityRow{
		{GroupID: 10, AccountID: 1, Concurrency: 4},
		{GroupID: 10, AccountID: 2, Concurrency: 0},
		{GroupID: 10, AccountID: 1, Concurrency: 4}, // duplicate relation must not double count
		{GroupID: 20, AccountID: 3, Concurrency: 2},
		{GroupID: 99, AccountID: 9, Concurrency: 100}, // hidden group must be ignored defensively
	}}
	loads := &visibleCapacityLoadStub{loads: map[int64]*AccountLoadInfo{
		1: {AccountID: 1, CurrentConcurrency: 3, WaitingCount: 2},
		2: {AccountID: 2, CurrentConcurrency: 1, WaitingCount: 1},
		3: {AccountID: 3, CurrentConcurrency: 2, WaitingCount: 4},
		9: {AccountID: 9, CurrentConcurrency: 99, WaitingCount: 99},
	}}
	svc := newVisibleCapacityService(groups, accounts, loads)

	got, err := svc.GetVisibleGroupCapacity(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, []int64{42}, groups.requestedUser)
	require.Equal(t, []int64{10, 20}, accounts.requested, "capacity query must receive visible IDs only")
	require.Equal(t, 1, loads.calls, "runtime load must be fetched in one batch")
	require.Equal(t, []AccountWithConcurrency{
		{ID: 1, MaxConcurrency: 4},
		{ID: 2, MaxConcurrency: 0},
		{ID: 3, MaxConcurrency: 2},
	}, loads.requested)

	require.NotNil(t, got)
	require.False(t, got.CollectedAt.IsZero())
	require.Len(t, got.Groups, 2)
	require.Equal(t, int64(10), got.Groups[0].GroupID)
	require.Equal(t, "Claude", got.Groups[0].Name)
	require.Equal(t, PlatformAnthropic, got.Groups[0].Platform)
	require.Equal(t, VisibleGroupConcurrency{
		Current:        4,
		Max:            4,
		Remaining:      0,
		LoadPercentage: 100,
		Waiting:        3,
	}, got.Groups[0].Concurrency)
	require.Equal(t, &VisibleAccountConcurrency{
		Current:            3,
		Max:                4,
		LoadPercentage:     75,
		ConfiguredAccounts: 1,
	}, got.Groups[0].AccountConcurrency)
	require.Equal(t, VisibleLoadCapacity{Available: 3, Total: 4, Percentage: 75}, got.Groups[0].LoadCapacity)

	require.Equal(t, VisibleGroupConcurrency{
		Current:        2,
		Max:            2,
		Remaining:      0,
		LoadPercentage: 100,
		Waiting:        4,
	}, got.Groups[1].Concurrency)
	require.Equal(t, &VisibleAccountConcurrency{
		Current:            2,
		Max:                2,
		LoadPercentage:     100,
		ConfiguredAccounts: 1,
	}, got.Groups[1].AccountConcurrency)
	require.Equal(t, VisibleLoadCapacity{Available: 1, Total: 2, Percentage: 50}, got.Groups[1].LoadCapacity)
}

func TestVisibleCapacityAggregatesQuotaWindowsWithCoverage(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(2 * time.Hour)
	past := now.Add(-time.Hour)
	groups := &visibleGroupListerStub{groups: []Group{
		{ID: 10, Name: "Claude", Platform: PlatformAnthropic},
		{ID: 20, Name: "Codex", Platform: PlatformOpenAI},
	}}
	accounts := &visibleCapacityAccountRepoStub{quotaRows: []GroupAccountQuotaLoadRow{
		{
			GroupID: 10, AccountID: 1, Platform: PlatformAnthropic,
			Extra: map[string]any{
				"session_window_utilization":   0.2,
				"passive_usage_7d_utilization": 0.4,
				"passive_usage_7d_reset":       future.Unix(),
			},
			SessionWindowEnd: &future,
		},
		{
			GroupID: 10, AccountID: 2, Platform: PlatformAnthropic,
			Extra:            map[string]any{"session_window_utilization": 0.6},
			SessionWindowEnd: &past, // expired samples count as data, with zero load
		},
		{GroupID: 10, AccountID: 1, Platform: PlatformAnthropic}, // duplicate relation
		{
			GroupID: 20, AccountID: 3, Platform: PlatformOpenAI,
			Extra: map[string]any{
				"codex_5h_used_percent":        80,
				"codex_5h_reset_at":            future.Format(time.RFC3339),
				"codex_7d_used_percent":        120,
				"codex_7d_reset_after_seconds": 3600,
				"codex_usage_updated_at":       now.Format(time.RFC3339),
			},
		},
		{
			// This account can represent a dynamically rate-limited account: the
			// stable quota projection retains it even if the concurrency query does not.
			GroupID: 20, AccountID: 4, Platform: PlatformOpenAI,
			Extra: map[string]any{
				"codex_5h_used_percent": -10,
				"codex_7d_used_percent": 50,
				"codex_7d_reset_at":     past.Format(time.RFC3339),
			},
		},
		{GroupID: 99, AccountID: 9, Platform: PlatformOpenAI, Extra: map[string]any{"codex_5h_used_percent": 100}},
	}}
	svc := newVisibleCapacityService(groups, accounts, nil)

	got, err := svc.GetVisibleGroupCapacity(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, []int64{10, 20}, accounts.quotaRequested)
	require.Equal(t, 1, accounts.quotaCalls)
	require.Len(t, got.Groups, 2)
	require.Equal(t, VisibleQuotaLoad{
		FiveHour: &VisibleQuotaWindowLoad{
			LoadPercentage:   10,
			AccountsWithData: 2,
			TotalAccounts:    2,
		},
		SevenDay: &VisibleQuotaWindowLoad{
			LoadPercentage:   40,
			AccountsWithData: 1,
			TotalAccounts:    2,
		},
	}, got.Groups[0].QuotaLoad)
	require.Equal(t, VisibleQuotaLoad{
		FiveHour: &VisibleQuotaWindowLoad{
			LoadPercentage:   40,
			AccountsWithData: 2,
			TotalAccounts:    2,
		},
		SevenDay: &VisibleQuotaWindowLoad{
			LoadPercentage:   50,
			AccountsWithData: 2,
			TotalAccounts:    2,
		},
	}, got.Groups[1].QuotaLoad)
}

func TestVisibleCapacityQuotaWindowParsing(t *testing.T) {
	now := time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC)

	t.Run("codex reset-after expiration and numeric strings", func(t *testing.T) {
		load, ok := codexQuotaWindowLoad(map[string]any{
			"codex_5h_used_percent":        "82.5",
			"codex_5h_reset_after_seconds": 1800,
			"codex_usage_updated_at":       now.Add(-time.Hour).Format(time.RFC3339Nano),
		}, "5h", now)
		require.True(t, ok)
		require.Zero(t, load)
	})

	t.Run("codex invalid and non-finite values are missing", func(t *testing.T) {
		_, ok := codexQuotaWindowLoad(map[string]any{"codex_7d_used_percent": "not-a-number"}, "7d", now)
		require.False(t, ok)
		_, ok = codexQuotaWindowLoad(map[string]any{"codex_7d_used_percent": "NaN"}, "7d", now)
		require.False(t, ok)
	})

	t.Run("anthropic fractions are converted and clamped", func(t *testing.T) {
		future := now.Add(time.Hour)
		load, ok := anthropicQuotaWindowLoad(map[string]any{
			"session_window_utilization": 1.5,
		}, &future, "5h", now)
		require.True(t, ok)
		require.Equal(t, float64(100), load)

		load, ok = anthropicQuotaWindowLoad(map[string]any{
			"passive_usage_7d_utilization": -0.2,
			"passive_usage_7d_reset":       now.Add(time.Hour).Unix(),
		}, nil, "7d", now)
		require.True(t, ok)
		require.Zero(t, load)
	})
}

func TestVisibleCapacityAccountConcurrencyIsNilWithoutExplicitLimit(t *testing.T) {
	groups := &visibleGroupListerStub{groups: []Group{{ID: 10, Name: "Unlimited"}}}
	accounts := &visibleCapacityAccountRepoStub{rows: []GroupAccountCapacityRow{
		{GroupID: 10, AccountID: 1, Concurrency: 0},
	}}
	loads := &visibleCapacityLoadStub{loads: map[int64]*AccountLoadInfo{
		1: {AccountID: 1, CurrentConcurrency: 2, WaitingCount: 1},
	}}
	svc := newVisibleCapacityService(groups, accounts, loads)

	got, err := svc.GetVisibleGroupCapacity(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, got.Groups, 1)
	require.Nil(t, got.Groups[0].AccountConcurrency)
	require.Equal(t, VisibleGroupConcurrency{Current: 2, Waiting: 1}, got.Groups[0].Concurrency)
}

func TestVisibleCapacityKeepsEmptyGroupAndHandlesNoVisibleGroups(t *testing.T) {
	accounts := &visibleCapacityAccountRepoStub{}
	svc := newVisibleCapacityService(&visibleGroupListerStub{groups: []Group{
		{ID: 10, Name: "Empty", Platform: PlatformGemini},
	}}, accounts, &visibleCapacityLoadStub{})

	got, err := svc.GetVisibleGroupCapacity(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, got.Groups, 1)
	require.Equal(t, VisibleGroupConcurrency{}, got.Groups[0].Concurrency)
	require.Nil(t, got.Groups[0].AccountConcurrency)
	require.Equal(t, VisibleLoadCapacity{}, got.Groups[0].LoadCapacity)

	accounts = &visibleCapacityAccountRepoStub{}
	svc = newVisibleCapacityService(&visibleGroupListerStub{}, accounts, &visibleCapacityLoadStub{})
	got, err = svc.GetVisibleGroupCapacity(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got.Groups)
	require.Zero(t, accounts.calls, "empty visibility must not query account capacity")
}

func TestVisibleCapacityLoadFailureDegradesRuntimeValuesToZero(t *testing.T) {
	groups := &visibleGroupListerStub{groups: []Group{{ID: 10, AccountCount: 1, ActiveAccountCount: 1}}}
	accounts := &visibleCapacityAccountRepoStub{rows: []GroupAccountCapacityRow{
		{GroupID: 10, AccountID: 1, Concurrency: 5},
	}}
	loads := &visibleCapacityLoadStub{err: errors.New("cache unavailable")}
	svc := newVisibleCapacityService(groups, accounts, loads)

	got, err := svc.GetVisibleGroupCapacity(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, got.Groups, 1)
	require.Equal(t, VisibleGroupConcurrency{Max: 5, Remaining: 5}, got.Groups[0].Concurrency)
	require.Equal(t, &VisibleAccountConcurrency{Max: 5, ConfiguredAccounts: 1}, got.Groups[0].AccountConcurrency)
	require.Equal(t, 1, loads.calls)
}

func TestVisibleCapacityVisibilityFailureShortCircuits(t *testing.T) {
	sentinel := errors.New("visibility unavailable")
	accounts := &visibleCapacityAccountRepoStub{}
	svc := newVisibleCapacityService(&visibleGroupListerStub{err: sentinel}, accounts, &visibleCapacityLoadStub{})

	got, err := svc.GetVisibleGroupCapacity(context.Background(), 1)
	require.Nil(t, got)
	require.ErrorIs(t, err, sentinel)
	require.Zero(t, accounts.calls)
}

func TestVisibleCapacitySequentialFallbackQueriesVisibleIDsOnly(t *testing.T) {
	repo := &visibleCapacityFallbackRepoStub{accounts: map[int64][]Account{
		10: {{ID: 1, Concurrency: 3}},
		20: {{ID: 2, Concurrency: 4}},
		99: {{ID: 9, Concurrency: 100}},
	}}
	svc := newVisibleCapacityService(&visibleGroupListerStub{groups: []Group{{ID: 10}, {ID: 20}}}, repo, nil)

	got, err := svc.GetVisibleGroupCapacity(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, got.Groups, 2)
	require.Equal(t, []int64{10, 20}, repo.requested)
	require.Equal(t, 3, got.Groups[0].Concurrency.Max)
	require.Equal(t, 4, got.Groups[1].Concurrency.Max)
}

func TestVisibleCapacityQuotaFallbackKeepsDynamicCooldownAccounts(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	repo := &visibleCapacityFallbackRepoStub{
		accounts: map[int64][]Account{10: {}},
		allAccounts: map[int64][]Account{10: {
			{
				ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true,
				RateLimitResetAt: &future, OverloadUntil: &future, TempUnschedulableUntil: &future,
				Extra: map[string]any{"codex_5h_used_percent": 20},
			},
			{
				ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true,
				ExpiresAt: &past, AutoPauseOnExpired: false,
				Extra: map[string]any{"codex_5h_used_percent": 60},
			},
			{
				ID: 3, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: false,
				Extra: map[string]any{"codex_5h_used_percent": 100},
			},
			{
				ID: 4, Platform: PlatformOpenAI, Status: StatusDisabled, Schedulable: true,
				Extra: map[string]any{"codex_5h_used_percent": 100},
			},
			{
				ID: 5, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true,
				ExpiresAt: &past, AutoPauseOnExpired: true,
				Extra: map[string]any{"codex_5h_used_percent": 100},
			},
		}},
	}
	svc := newVisibleCapacityService(&visibleGroupListerStub{groups: []Group{{ID: 10}}}, repo, nil)

	got, err := svc.GetVisibleGroupCapacity(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, []int64{10}, repo.requestedAll)
	require.Len(t, got.Groups, 1)
	require.Equal(t, &VisibleQuotaWindowLoad{
		LoadPercentage:   40,
		AccountsWithData: 2,
		TotalAccounts:    2,
	}, got.Groups[0].QuotaLoad.FiveHour)
	require.Nil(t, got.Groups[0].QuotaLoad.SevenDay)
}
