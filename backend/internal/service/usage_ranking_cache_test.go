package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type usageRankingRepositoryStub struct {
	UsageLogRepository
	calls       int32
	legacyCalls int32
	snapshot    func(usagestats.UsageRankingQuery) *usagestats.UsageRankingSnapshot
	err         error
	wait        <-chan struct{}
	onCall      func(context.Context, usagestats.UsageRankingQuery)
}

func (s *usageRankingRepositoryStub) GetUsageRankingSnapshot(ctx context.Context, query usagestats.UsageRankingQuery) (*usagestats.UsageRankingSnapshot, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.onCall != nil {
		s.onCall(ctx, query)
	}
	if s.wait != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.wait:
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.snapshot != nil {
		return s.snapshot(query), nil
	}
	return rankingSnapshotForQuery(query), nil
}

func (s *usageRankingRepositoryStub) GetUsageRanking(ctx context.Context, query usagestats.UsageRankingQuery) (*usagestats.UsageRankingResponse, error) {
	atomic.AddInt32(&s.legacyCalls, 1)
	snapshot, err := s.GetUsageRankingSnapshot(ctx, query)
	if err != nil {
		return nil, err
	}
	return usagestats.PersonalizeUsageRanking(snapshot, query.CurrentUserID, query.Limit), nil
}

type usageRankingCacheStub struct {
	mu           sync.Mutex
	values       map[string]string
	getErr       error
	setErr       error
	deleteErr    error
	acquireErr   error
	releaseErr   error
	getCalls     int
	setCalls     int
	deleteCalls  int
	acquireCalls int
	releaseCalls int
	lastTTL      time.Duration
	locks        map[string]string
}

func newUsageRankingCacheStub() *usageRankingCacheStub {
	return &usageRankingCacheStub{
		values: make(map[string]string),
		locks:  make(map[string]string),
	}
}

func (c *usageRankingCacheStub) GetUsageRanking(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCalls++
	if c.getErr != nil {
		return "", c.getErr
	}
	value, ok := c.values[key]
	if !ok {
		return "", ErrUsageRankingCacheMiss
	}
	return value, nil
}

func (c *usageRankingCacheStub) SetUsageRanking(_ context.Context, key string, data string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setCalls++
	c.lastTTL = ttl
	if c.setErr != nil {
		return c.setErr
	}
	c.values[key] = data
	return nil
}

func (c *usageRankingCacheStub) DeleteUsageRanking(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleteCalls++
	if c.deleteErr != nil {
		return c.deleteErr
	}
	delete(c.values, key)
	return nil
}

func (c *usageRankingCacheStub) TryAcquireUsageRankingRefresh(
	_ context.Context,
	key string,
	token string,
	_ time.Duration,
) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acquireCalls++
	if c.acquireErr != nil {
		return false, c.acquireErr
	}
	if _, exists := c.locks[key]; exists {
		return false, nil
	}
	c.locks[key] = token
	return true, nil
}

func (c *usageRankingCacheStub) ReleaseUsageRankingRefresh(_ context.Context, key string, token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releaseCalls++
	if c.releaseErr != nil {
		return c.releaseErr
	}
	if c.locks[key] == token {
		delete(c.locks, key)
	}
	return nil
}

func rankingQueryForCacheTest() usagestats.UsageRankingQuery {
	return usagestats.UsageRankingQuery{
		StartTime:           time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		EndTime:             time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
		ComparisonStartTime: time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		ComparisonEndTime:   time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC),
		Metric:              usagestats.UsageRankingMetricTokens,
		Period:              usagestats.UsageRankingPeriodWeek,
		Limit:               10,
		CurrentUserID:       42,
	}
}

func rankingSnapshotForQuery(query usagestats.UsageRankingQuery) *usagestats.UsageRankingSnapshot {
	items := make([]usagestats.UsageRankingItem, 0, 12)
	for rank := int64(1); rank <= 12; rank++ {
		userID := rank
		if rank == 2 {
			userID = 99
		}
		if rank == 11 {
			userID = 42
		}
		items = append(items, usagestats.UsageRankingItem{
			Rank:        rank,
			UserID:      userID,
			DisplayName: "ranked-user",
			TotalTokens: 1300 - rank*100,
			ActualCost:  float64(1300-rank*100) / 100,
		})
	}
	return &usagestats.UsageRankingSnapshot{
		Metric:      query.Metric,
		Period:      query.Period,
		GeneratedAt: "2026-07-30T08:00:00Z",
		StartDate:   query.StartTime.Format("2006-01-02"),
		EndDate:     query.EndTime.AddDate(0, 0, -1).Format("2006-01-02"),
		Summary:     usagestats.UsageRankingSummary{RankedUsers: int64(len(items))},
		Items:       items,
	}
}

func TestUsageRankingCacheKeySeparatesEverySnapshotDimension(t *testing.T) {
	base := rankingQueryForCacheTest()
	baseKey := usageRankingCacheKey(base)

	tests := map[string]func(usagestats.UsageRankingQuery) usagestats.UsageRankingQuery{
		"metric": func(query usagestats.UsageRankingQuery) usagestats.UsageRankingQuery {
			query.Metric = usagestats.UsageRankingMetricCost
			return query
		},
		"period": func(query usagestats.UsageRankingQuery) usagestats.UsageRankingQuery {
			query.Period = usagestats.UsageRankingPeriodMonth
			return query
		},
		"timezone": func(query usagestats.UsageRankingQuery) usagestats.UsageRankingQuery {
			tokyo := time.FixedZone("Asia/Tokyo", 9*60*60)
			query.StartTime = query.StartTime.In(tokyo)
			query.EndTime = query.EndTime.In(tokyo)
			query.ComparisonStartTime = query.ComparisonStartTime.In(tokyo)
			query.ComparisonEndTime = query.ComparisonEndTime.In(tokyo)
			return query
		},
		"start time": func(query usagestats.UsageRankingQuery) usagestats.UsageRankingQuery {
			query.StartTime = query.StartTime.Add(time.Hour)
			return query
		},
		"end time": func(query usagestats.UsageRankingQuery) usagestats.UsageRankingQuery {
			query.EndTime = query.EndTime.Add(time.Hour)
			return query
		},
		"comparison start": func(query usagestats.UsageRankingQuery) usagestats.UsageRankingQuery {
			query.ComparisonStartTime = query.ComparisonStartTime.Add(time.Hour)
			return query
		},
		"comparison end": func(query usagestats.UsageRankingQuery) usagestats.UsageRankingQuery {
			query.ComparisonEndTime = query.ComparisonEndTime.Add(time.Hour)
			return query
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			require.NotEqual(t, baseKey, usageRankingCacheKey(mutate(base)))
		})
	}
}

func TestUsageRankingCacheKeyIgnoresPersonalization(t *testing.T) {
	base := rankingQueryForCacheTest()
	personalized := base
	personalized.Limit = 5
	personalized.CurrentUserID = 99

	require.Equal(t, usageRankingCacheKey(base), usageRankingCacheKey(personalized))
}

func TestUsageRankingL1KeyGroupsAdjacentMinuteBuckets(t *testing.T) {
	base := rankingQueryForCacheTest()
	nextMinute := base
	nextMinute.ComparisonEndTime = nextMinute.ComparisonEndTime.Add(time.Minute)

	require.NotEqual(t, usageRankingCacheKey(base), usageRankingCacheKey(nextMinute))
	require.Equal(t, usageRankingL1Key(base), usageRankingL1Key(nextMinute))

	differentMetric := base
	differentMetric.Metric = usagestats.UsageRankingMetricCost
	require.NotEqual(t, usageRankingL1Key(base), usageRankingL1Key(differentMetric))
}

func TestUsageRankingCacheKeyUsesRepositoryNormalization(t *testing.T) {
	base := rankingQueryForCacheTest()
	normalizedEquivalent := base
	normalizedEquivalent.Metric = "invalid"
	normalizedEquivalent.Limit = 0
	normalizedEquivalent.ComparisonEndTime = normalizedEquivalent.ComparisonEndTime.Add(25 * time.Second)

	require.Equal(t, usageRankingCacheKey(base), usageRankingCacheKey(normalizedEquivalent))
}

func TestUsageServiceUsageRankingCacheBucketsMovingComparisonEnd(t *testing.T) {
	cache := newUsageRankingCacheStub()
	repo := &usageRankingRepositoryStub{}
	service := NewUsageServiceWithRankingCache(repo, nil, nil, nil, cache)
	firstQuery := rankingQueryForCacheTest()
	firstQuery.ComparisonEndTime = firstQuery.ComparisonEndTime.Add(10 * time.Second)
	secondQuery := firstQuery
	secondQuery.ComparisonEndTime = secondQuery.ComparisonEndTime.Add(5 * time.Second)

	first, err := service.GetUsageRanking(context.Background(), firstQuery)
	require.NoError(t, err)
	cache.mu.Lock()
	getCallsAfterFirst := cache.getCalls
	cache.mu.Unlock()
	second, err := service.GetUsageRanking(context.Background(), secondQuery)
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.calls))
	cache.mu.Lock()
	require.Equal(t, getCallsAfterFirst, cache.getCalls, "fresh L1 should bypass Redis for the exact minute bucket")
	cache.mu.Unlock()
}

func TestUsageServiceUsageRankingCacheIsSharedAcrossInstances(t *testing.T) {
	cache := newUsageRankingCacheStub()
	firstQuery := rankingQueryForCacheTest()
	secondQuery := firstQuery
	secondQuery.CurrentUserID = 99
	secondQuery.Limit = 5
	firstRepo := &usageRankingRepositoryStub{}
	secondRepo := &usageRankingRepositoryStub{err: errors.New("second instance must not hit database")}
	firstService := NewUsageServiceWithRankingCache(firstRepo, nil, nil, nil, cache)
	secondService := NewUsageServiceWithRankingCache(secondRepo, nil, nil, nil, cache)

	first, err := firstService.GetUsageRanking(context.Background(), firstQuery)
	require.NoError(t, err)
	second, err := secondService.GetUsageRanking(context.Background(), secondQuery)
	require.NoError(t, err)

	require.Equal(t, int64(42), first.CurrentUser.UserID)
	require.Equal(t, int64(99), second.CurrentUser.UserID)
	require.Len(t, first.Ranking, 10)
	require.Len(t, second.Ranking, 5)
	require.Equal(t, int32(1), atomic.LoadInt32(&firstRepo.calls))
	require.Equal(t, int32(0), atomic.LoadInt32(&secondRepo.calls))
	require.Equal(t, usageRankingCacheTTL, cache.lastTTL)
}

func TestUsageServiceUsageRankingCachePersonalizesCurrentUsersFromOneSnapshot(t *testing.T) {
	cache := newUsageRankingCacheStub()
	repo := &usageRankingRepositoryStub{}
	service := NewUsageServiceWithRankingCache(repo, nil, nil, nil, cache)
	firstQuery := rankingQueryForCacheTest()
	secondQuery := firstQuery
	secondQuery.CurrentUserID = 99

	first, err := service.GetUsageRanking(context.Background(), firstQuery)
	require.NoError(t, err)
	second, err := service.GetUsageRanking(context.Background(), secondQuery)
	require.NoError(t, err)
	again, err := service.GetUsageRanking(context.Background(), firstQuery)
	require.NoError(t, err)

	require.Equal(t, int64(42), first.CurrentUser.UserID)
	require.Equal(t, int64(99), second.CurrentUser.UserID)
	require.Equal(t, first, again)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.calls))
}

func TestUsageServiceUsageRankingCachePersonalizesLimitsFromOneSnapshot(t *testing.T) {
	cache := newUsageRankingCacheStub()
	repo := &usageRankingRepositoryStub{}
	service := NewUsageServiceWithRankingCache(repo, nil, nil, nil, cache)
	topFiveQuery := rankingQueryForCacheTest()
	topFiveQuery.Limit = 5
	topTenQuery := topFiveQuery
	topTenQuery.Limit = 10

	topFive, err := service.GetUsageRanking(context.Background(), topFiveQuery)
	require.NoError(t, err)
	topTen, err := service.GetUsageRanking(context.Background(), topTenQuery)
	require.NoError(t, err)

	require.Len(t, topFive.Ranking, 5)
	require.Len(t, topTen.Ranking, 10)
	require.Equal(t, int64(5), *topFive.CurrentUserTarget.TargetRank)
	require.Equal(t, int64(10), *topTen.CurrentUserTarget.TargetRank)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.calls))
}

func TestUsageServiceUsageRankingCacheCoalescesConcurrentUsers(t *testing.T) {
	cache := newUsageRankingCacheStub()
	release := make(chan struct{})
	repo := &usageRankingRepositoryStub{wait: release}
	service := NewUsageServiceWithRankingCache(repo, nil, nil, nil, cache)
	baseQuery := rankingQueryForCacheTest()

	const callers = 20
	start := make(chan struct{})
	errs := make(chan error, callers)
	for index := range callers {
		go func(index int) {
			<-start
			query := baseQuery
			if index%2 == 1 {
				query.CurrentUserID = 99
				query.Limit = 5
			}
			_, err := service.GetUsageRanking(context.Background(), query)
			errs <- err
		}(index)
	}
	close(start)
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&repo.calls) == 1
	}, time.Second, time.Millisecond)
	close(release)

	for range callers {
		require.NoError(t, <-errs)
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.calls))
}

func TestUsageServiceUsageRankingRefreshLeaseCoalescesAcrossInstances(t *testing.T) {
	cache := newUsageRankingCacheStub()
	release := make(chan struct{})
	firstRepo := &usageRankingRepositoryStub{wait: release}
	secondRepo := &usageRankingRepositoryStub{err: errors.New("lease waiter must not hit database")}
	firstService := NewUsageServiceWithRankingCache(firstRepo, nil, nil, nil, cache)
	secondService := NewUsageServiceWithRankingCache(secondRepo, nil, nil, nil, cache)
	query := rankingQueryForCacheTest()

	firstResult := make(chan error, 1)
	go func() {
		_, err := firstService.GetUsageRanking(context.Background(), query)
		firstResult <- err
	}()
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&firstRepo.calls) == 1
	}, time.Second, time.Millisecond)

	secondResult := make(chan error, 1)
	go func() {
		_, err := secondService.GetUsageRanking(context.Background(), query)
		secondResult <- err
	}()
	require.Eventually(t, func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return cache.acquireCalls >= 2
	}, time.Second, time.Millisecond)
	require.Equal(t, int32(0), atomic.LoadInt32(&secondRepo.calls))

	close(release)
	require.NoError(t, <-firstResult)
	require.NoError(t, <-secondResult)
	require.Equal(t, int32(1), atomic.LoadInt32(&firstRepo.calls))
	require.Equal(t, int32(0), atomic.LoadInt32(&secondRepo.calls))
}

func TestUsageServiceUsageRankingLeaseWaiterReturnsBoundedL1Stale(t *testing.T) {
	cache := newUsageRankingCacheStub()
	repo := &usageRankingRepositoryStub{}
	service := NewUsageServiceWithRankingCache(repo, nil, nil, nil, cache)
	query := rankingQueryForCacheTest()

	fresh, err := service.GetUsageRanking(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.calls))

	nextMinute := query
	nextMinute.ComparisonEndTime = nextMinute.ComparisonEndTime.Add(time.Minute)
	cache.mu.Lock()
	clear(cache.values)
	cache.locks[usageRankingGlobalRefreshLeaseKey] = "remote-owner"
	getCallsBeforeStale := cache.getCalls
	cache.mu.Unlock()

	stale, err := service.GetUsageRanking(context.Background(), nextMinute)
	require.NoError(t, err)
	require.Equal(t, fresh.GeneratedAt, stale.GeneratedAt)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.calls))
	cache.mu.Lock()
	require.Equal(t, getCallsBeforeStale+2, cache.getCalls, "stale fallback should not wait for a poll")
	cache.mu.Unlock()
}

func TestUsageServiceUsageRankingCacheFailureFallsBackToRepository(t *testing.T) {
	cache := newUsageRankingCacheStub()
	cache.getErr = errors.New("redis unavailable")
	cache.setErr = errors.New("redis unavailable")
	cache.acquireErr = errors.New("redis unavailable")
	repo := &usageRankingRepositoryStub{}
	service := NewUsageServiceWithRankingCache(repo, nil, nil, nil, cache)

	first, err := service.GetUsageRanking(context.Background(), rankingQueryForCacheTest())
	require.NoError(t, err)
	require.NotNil(t, first)

	cache.mu.Lock()
	getCallsAfterFirst := cache.getCalls
	cache.mu.Unlock()
	second, err := service.GetUsageRanking(context.Background(), rankingQueryForCacheTest())

	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.calls))
	cache.mu.Lock()
	require.Equal(t, getCallsAfterFirst, cache.getCalls, "L1 should bypass Redis during the outage backoff")
	cache.mu.Unlock()
}

func TestUsageServiceUsageRankingCacheWriteFailureReusesL1(t *testing.T) {
	cache := newUsageRankingCacheStub()
	cache.setErr = errors.New("redis writes unavailable")
	repo := &usageRankingRepositoryStub{}
	service := NewUsageServiceWithRankingCache(repo, nil, nil, nil, cache)
	query := rankingQueryForCacheTest()

	first, err := service.GetUsageRanking(context.Background(), query)
	require.NoError(t, err)
	second, err := service.GetUsageRanking(context.Background(), query)

	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.calls))
	require.Equal(t, 1, cache.setCalls)

	nextMinute := query
	nextMinute.ComparisonEndTime = nextMinute.ComparisonEndTime.Add(time.Minute)
	cache.mu.Lock()
	getCallsBeforeStale := cache.getCalls
	cache.mu.Unlock()
	stale, err := service.GetUsageRanking(context.Background(), nextMinute)
	require.NoError(t, err)
	require.Equal(t, first.GeneratedAt, stale.GeneratedAt)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.calls))
	cache.mu.Lock()
	require.Equal(t, getCallsBeforeStale, cache.getCalls, "write backoff should reuse L1 without a read probe")
	cache.mu.Unlock()
}

func TestUsageServiceUsageRankingLeaseSuccessDoesNotClearSnapshotWriteBackoff(t *testing.T) {
	cache := newUsageRankingCacheStub()
	service := NewUsageServiceWithRankingCache(&usageRankingRepositoryStub{}, nil, nil, nil, cache)
	now := time.Now()
	service.rankingCacheState.markCacheWriteUnavailable(now)

	acquired, err := service.tryAcquireUsageRankingRefresh(context.Background(), "lease", "token")
	require.NoError(t, err)
	require.True(t, acquired)
	require.True(t, service.rankingCacheState.shouldBackOffCache(now))

	query := rankingQueryForCacheTest()
	service.storeUsageRankingCache(usageRankingCacheKey(query), rankingSnapshotForQuery(query))
	require.False(t, service.rankingCacheState.shouldBackOffCache(now))
}

func TestUsageServiceUsageRankingGlobalLeaseSerializesDifferentKeysAcrossInstances(t *testing.T) {
	cache := newUsageRankingCacheStub()
	firstRelease := make(chan struct{})
	firstRepo := &usageRankingRepositoryStub{wait: firstRelease}
	secondRepo := &usageRankingRepositoryStub{}
	firstService := NewUsageServiceWithRankingCache(firstRepo, nil, nil, nil, cache)
	secondService := NewUsageServiceWithRankingCache(secondRepo, nil, nil, nil, cache)
	firstQuery := rankingQueryForCacheTest()
	secondQuery := firstQuery
	secondQuery.Metric = usagestats.UsageRankingMetricCost

	firstResult := make(chan error, 1)
	go func() {
		_, err := firstService.GetUsageRanking(context.Background(), firstQuery)
		firstResult <- err
	}()
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&firstRepo.calls) == 1
	}, time.Second, time.Millisecond)

	secondResult := make(chan error, 1)
	go func() {
		_, err := secondService.GetUsageRanking(context.Background(), secondQuery)
		secondResult <- err
	}()
	require.Eventually(t, func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return cache.acquireCalls >= 2
	}, time.Second, time.Millisecond)
	require.Equal(t, int32(0), atomic.LoadInt32(&secondRepo.calls))

	close(firstRelease)
	require.NoError(t, <-firstResult)
	require.NoError(t, <-secondResult)
	require.Equal(t, int32(1), atomic.LoadInt32(&firstRepo.calls))
	require.Equal(t, int32(1), atomic.LoadInt32(&secondRepo.calls))
}

func TestUsageServiceUsageRankingLeaseWaitDoesNotConsumeQueryBudget(t *testing.T) {
	cache := newUsageRankingCacheStub()
	query := rankingQueryForCacheTest()
	queryStarted := make(chan context.Context, 1)
	releaseQuery := make(chan struct{})
	repo := &usageRankingRepositoryStub{
		wait: releaseQuery,
		onCall: func(ctx context.Context, _ usagestats.UsageRankingQuery) {
			queryStarted <- ctx
		},
	}
	service := NewUsageServiceWithRankingCache(repo, nil, nil, nil, cache)
	token := "lease-owner"
	cache.locks[usageRankingGlobalRefreshLeaseKey] = token

	waitCtx, cancelWait := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := service.fetchUsageRankingSnapshotWithLease(
			waitCtx,
			repo,
			query,
			usageRankingCacheKey(query),
			usageRankingL1Key(query),
			token,
		)
		result <- err
	}()

	queryCtx := <-queryStarted
	cancelWait()
	require.Error(t, waitCtx.Err())
	require.NoError(t, queryCtx.Err(), "database query must have a fresh budget after the lease wait")
	close(releaseQuery)
	require.NoError(t, <-result)
}

func TestUsageServiceUsageRankingDoesNotHoldGlobalLeaseWhileWaitingForLocalGate(t *testing.T) {
	cache := newUsageRankingCacheStub()
	query := rankingQueryForCacheTest()
	repo := &usageRankingRepositoryStub{}
	service := NewUsageServiceWithRankingCache(repo, nil, nil, nil, cache)
	require.True(t, service.rankingCacheState.acquireLocalRefreshSlot(context.Background()))

	token := "lease-owner"
	cache.locks[usageRankingGlobalRefreshLeaseKey] = token
	waitCtx, cancelWait := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := service.fetchUsageRankingSnapshotWithLease(
			waitCtx,
			repo,
			query,
			usageRankingCacheKey(query),
			usageRankingL1Key(query),
			token,
		)
		result <- err
	}()

	require.Eventually(t, func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		_, held := cache.locks[usageRankingGlobalRefreshLeaseKey]
		return !held
	}, time.Second, time.Millisecond)
	cancelWait()
	service.rankingCacheState.releaseLocalRefreshSlot()
	require.ErrorIs(t, <-result, context.Canceled)
	require.Equal(t, int32(0), atomic.LoadInt32(&repo.calls))
}

func TestUsageServiceUsageRankingLocalGateSerializesRedisOutageFallbacks(t *testing.T) {
	cache := newUsageRankingCacheStub()
	cache.getErr = errors.New("redis unavailable")
	cache.setErr = errors.New("redis unavailable")
	cache.acquireErr = errors.New("redis unavailable")
	release := make(chan struct{})
	repo := &usageRankingRepositoryStub{wait: release}
	service := NewUsageServiceWithRankingCache(repo, nil, nil, nil, cache)
	firstQuery := rankingQueryForCacheTest()
	secondQuery := firstQuery
	secondQuery.Metric = usagestats.UsageRankingMetricCost

	results := make(chan error, 2)
	go func() {
		_, err := service.GetUsageRanking(context.Background(), firstQuery)
		results <- err
	}()
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&repo.calls) == 1
	}, time.Second, time.Millisecond)

	go func() {
		_, err := service.GetUsageRanking(context.Background(), secondQuery)
		results <- err
	}()
	require.Never(t, func() bool {
		return atomic.LoadInt32(&repo.calls) > 1
	}, 100*time.Millisecond, 5*time.Millisecond)

	close(release)
	require.NoError(t, <-results)
	require.NoError(t, <-results)
	require.Equal(t, int32(2), atomic.LoadInt32(&repo.calls))
}

func TestUsageServiceUsageRankingLocalGateSerializesWhenCacheIsDisabled(t *testing.T) {
	release := make(chan struct{})
	repo := &usageRankingRepositoryStub{wait: release}
	service := NewUsageService(repo, nil, nil, nil)
	firstQuery := rankingQueryForCacheTest()
	secondQuery := firstQuery
	secondQuery.Metric = usagestats.UsageRankingMetricCost

	results := make(chan error, 2)
	go func() {
		_, err := service.GetUsageRanking(context.Background(), firstQuery)
		results <- err
	}()
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&repo.calls) == 1
	}, time.Second, time.Millisecond)

	go func() {
		_, err := service.GetUsageRanking(context.Background(), secondQuery)
		results <- err
	}()
	require.Never(t, func() bool {
		return atomic.LoadInt32(&repo.calls) > 1
	}, 100*time.Millisecond, 5*time.Millisecond)

	close(release)
	require.NoError(t, <-results)
	require.NoError(t, <-results)
	require.Equal(t, int32(2), atomic.LoadInt32(&repo.calls))
}

func TestUsageServiceUsageRankingMapsInternalDeadlineToGatewayTimeout(t *testing.T) {
	repo := &usageRankingRepositoryStub{err: context.DeadlineExceeded}
	service := NewUsageService(repo, nil, nil, nil)

	_, err := service.GetUsageRanking(context.Background(), rankingQueryForCacheTest())

	require.Error(t, err)
	require.True(t, infraerrors.IsGatewayTimeout(err))
	require.Equal(t, "USAGE_RANKING_TIMEOUT", infraerrors.Reason(err))
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestUsageServiceUsageRankingPreservesCallerCancellation(t *testing.T) {
	repo := &usageRankingRepositoryStub{wait: make(chan struct{})}
	service := NewUsageService(repo, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.GetUsageRanking(ctx, rankingQueryForCacheTest())

	require.ErrorIs(t, err, context.Canceled)
	require.False(t, infraerrors.IsGatewayTimeout(err))
}

func TestUsageServiceUsageRankingCorruptCacheIsEvictedAndRebuilt(t *testing.T) {
	cache := newUsageRankingCacheStub()
	query := rankingQueryForCacheTest()
	cache.values[usageRankingCacheKey(query)] = "{"
	repo := &usageRankingRepositoryStub{}
	service := NewUsageServiceWithRankingCache(repo, nil, nil, nil, cache)

	got, err := service.GetUsageRanking(context.Background(), query)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.calls))
	require.GreaterOrEqual(t, cache.deleteCalls, 1)
	require.Equal(t, 1, cache.setCalls)
}

func TestUsageRankingL1IsCapacityAndTimeBounded(t *testing.T) {
	var state usageRankingCacheState
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	snapshot := rankingSnapshotForQuery(rankingQueryForCacheTest())

	for index := 0; index < usageRankingL1Capacity+10; index++ {
		key := fmt.Sprintf("key-%03d", index)
		state.storeL1(key, "exact-"+key, snapshot, now)
	}

	state.mu.Lock()
	require.Len(t, state.l1, usageRankingL1Capacity)
	state.mu.Unlock()
	_, oldestStillPresent := state.loadL1("key-000", now)
	require.False(t, oldestStillPresent)
	_, newestPresent := state.loadL1(fmt.Sprintf("key-%03d", usageRankingL1Capacity+9), now)
	require.True(t, newestPresent)

	_, stillFresh := state.loadL1(fmt.Sprintf("key-%03d", usageRankingL1Capacity+9), now.Add(usageRankingL1StaleTTL-time.Nanosecond))
	require.True(t, stillFresh)
	_, expired := state.loadL1(fmt.Sprintf("key-%03d", usageRankingL1Capacity+9), now.Add(usageRankingL1StaleTTL))
	require.False(t, expired)
	state.mu.Lock()
	require.Empty(t, state.l1)
	state.mu.Unlock()
}
