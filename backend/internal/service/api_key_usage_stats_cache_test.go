//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type apiKeyUsageStatsRepositoryStub struct {
	UsageLogRepository
	calls     atomic.Int32
	active    atomic.Int32
	maxActive atomic.Int32
	wait      <-chan struct{}
	err       error
}

func (s *apiKeyUsageStatsRepositoryStub) GetBatchAPIKeyUsageStats(
	ctx context.Context,
	apiKeyIDs []int64,
	_ time.Time,
	_ time.Time,
) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	s.calls.Add(1)
	active := s.active.Add(1)
	defer s.active.Add(-1)
	for {
		maxActive := s.maxActive.Load()
		if active <= maxActive || s.maxActive.CompareAndSwap(maxActive, active) {
			break
		}
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
	stats := make(map[int64]*usagestats.BatchAPIKeyUsageStats, len(apiKeyIDs))
	for _, id := range apiKeyIDs {
		stats[id] = &usagestats.BatchAPIKeyUsageStats{
			APIKeyID:        id,
			TodayActualCost: float64(id),
			TotalActualCost: float64(id * 10),
		}
	}
	return stats, nil
}

type apiKeyUsageStatsCacheStub struct {
	*usageRankingCacheStub

	mu          sync.Mutex
	values      map[string]string
	getCalls    int
	setCalls    int
	deleteCalls int
	lastTTL     time.Duration
	getErr      error
	setErr      error
	deleteErr   error
}

func newAPIKeyUsageStatsCacheStub() *apiKeyUsageStatsCacheStub {
	return &apiKeyUsageStatsCacheStub{
		usageRankingCacheStub: newUsageRankingCacheStub(),
		values:                make(map[string]string),
	}
}

func (c *apiKeyUsageStatsCacheStub) GetAPIKeyUsageStats(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCalls++
	if c.getErr != nil {
		return "", c.getErr
	}
	value, ok := c.values[key]
	if !ok {
		return "", ErrAPIKeyUsageStatsCacheMiss
	}
	return value, nil
}

func (c *apiKeyUsageStatsCacheStub) SetAPIKeyUsageStats(
	_ context.Context,
	key string,
	data string,
	ttl time.Duration,
) error {
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

func (c *apiKeyUsageStatsCacheStub) DeleteAPIKeyUsageStats(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleteCalls++
	if c.deleteErr != nil {
		return c.deleteErr
	}
	delete(c.values, key)
	return nil
}

func TestBatchAPIKeyUsageStatsUsesCanonicalSharedCacheKey(t *testing.T) {
	repo := &apiKeyUsageStatsRepositoryStub{}
	cache := newAPIKeyUsageStatsCacheStub()
	svc := newUsageService(repo, nil, nil, nil, cache)

	first, err := svc.GetBatchAPIKeyUsageStats(context.Background(), []int64{2, 1, 2, -1}, time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Len(t, first, 2)

	second, err := svc.GetBatchAPIKeyUsageStats(context.Background(), []int64{1, 2}, time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, int32(1), repo.calls.Load())

	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Equal(t, 1, cache.setCalls)
	require.Equal(t, apiKeyUsageStatsCacheTTL, cache.lastTTL)
	require.Len(t, cache.values, 1)
}

func TestBatchAPIKeyUsageStatsCoalescesConcurrentCacheMisses(t *testing.T) {
	release := make(chan struct{})
	repo := &apiKeyUsageStatsRepositoryStub{wait: release}
	cache := newAPIKeyUsageStatsCacheStub()
	svc := newUsageService(repo, nil, nil, nil, cache)

	type result struct {
		stats map[int64]*usagestats.BatchAPIKeyUsageStats
		err   error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			stats, err := svc.GetBatchAPIKeyUsageStats(
				context.Background(),
				[]int64{9, 8},
				time.Time{},
				time.Time{},
			)
			results <- result{stats: stats, err: err}
		}()
	}

	require.Eventually(t, func() bool {
		return repo.calls.Load() == 1
	}, time.Second, 10*time.Millisecond)
	close(release)

	for range 2 {
		got := <-results
		require.NoError(t, got.err)
		require.Len(t, got.stats, 2)
	}
	require.Equal(t, int32(1), repo.calls.Load())
	require.Equal(t, int32(1), repo.maxActive.Load())
}

func TestBatchAPIKeyUsageStatsCancelsWhileWaitingForFetchSlot(t *testing.T) {
	repo := &apiKeyUsageStatsRepositoryStub{}
	cache := newAPIKeyUsageStatsCacheStub()
	svc := newUsageService(repo, nil, nil, nil, cache)
	for range apiKeyUsageStatsMaxConcurrentFetch {
		require.True(t, svc.apiKeyUsageStatsState.acquireFetchSlot(context.Background()))
		defer svc.apiKeyUsageStatsState.releaseFetchSlot()
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := svc.GetDashboardAPIKeyUsageStats(ctx, 7, []int64{1})
		result <- err
	}()
	cancel()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("request did not stop after cancellation while waiting for the fetch slot")
	}
	require.Zero(t, repo.calls.Load())
}

func TestBatchAPIKeyUsageStatsCancelsActiveRepositoryQuery(t *testing.T) {
	release := make(chan struct{})
	repo := &apiKeyUsageStatsRepositoryStub{wait: release}
	cache := newAPIKeyUsageStatsCacheStub()
	svc := newUsageService(repo, nil, nil, nil, cache)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := svc.GetDashboardAPIKeyUsageStats(ctx, 7, []int64{1})
		result <- err
	}()
	require.Eventually(t, func() bool {
		return repo.calls.Load() == 1 && repo.active.Load() == 1
	}, time.Second, 10*time.Millisecond)
	cancel()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("active repository query outlived the canceled request")
	}
	require.Eventually(t, func() bool {
		return repo.active.Load() == 0
	}, time.Second, 10*time.Millisecond)
	close(release)
}

func TestBatchAPIKeyUsageStatsKeepsSharedQueryForRemainingWaiter(t *testing.T) {
	release := make(chan struct{})
	repo := &apiKeyUsageStatsRepositoryStub{wait: release}
	cache := newAPIKeyUsageStatsCacheStub()
	svc := newUsageService(repo, nil, nil, nil, cache)

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, err := svc.GetDashboardAPIKeyUsageStats(firstCtx, 7, []int64{1})
		firstResult <- err
	}()
	require.Eventually(t, func() bool {
		return repo.calls.Load() == 1 && repo.active.Load() == 1
	}, time.Second, 10*time.Millisecond)

	secondResult := make(chan error, 1)
	go func() {
		_, err := svc.GetDashboardAPIKeyUsageStats(context.Background(), 7, []int64{1})
		secondResult <- err
	}()
	require.Eventually(t, func() bool {
		svc.apiKeyUsageStatsState.mu.Lock()
		defer svc.apiKeyUsageStatsState.mu.Unlock()
		for _, flight := range svc.apiKeyUsageStatsState.flights {
			if flight.waiters == 2 {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)

	cancelFirst()
	require.ErrorIs(t, <-firstResult, context.Canceled)
	require.Equal(t, int32(1), repo.active.Load(), "remaining waiter must keep the shared query alive")
	close(release)
	require.NoError(t, <-secondResult)
	require.Equal(t, int32(1), repo.calls.Load())
}

func TestAPIKeyUsageStatsAbandonedFlightDoesNotCaptureNewCaller(t *testing.T) {
	var state apiKeyUsageStatsCacheState
	oldStarted := make(chan struct{})
	oldCanceled := make(chan struct{})
	releaseOld := make(chan struct{})
	oldDone := make(chan struct{})

	oldCtx, cancelOld := context.WithCancel(context.Background())
	oldResult := make(chan error, 1)
	go func() {
		_, err := state.doFetch(oldCtx, "same-key", func(ctx context.Context) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
			close(oldStarted)
			<-ctx.Done()
			close(oldCanceled)
			<-releaseOld
			close(oldDone)
			return nil, ctx.Err()
		})
		oldResult <- err
	}()
	<-oldStarted
	cancelOld()
	require.ErrorIs(t, <-oldResult, context.Canceled)
	<-oldCanceled

	newStarted := make(chan struct{})
	releaseNew := make(chan struct{})
	newResult := make(chan error, 1)
	go func() {
		_, err := state.doFetch(context.Background(), "same-key", func(context.Context) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
			close(newStarted)
			<-releaseNew
			return map[int64]*usagestats.BatchAPIKeyUsageStats{
				1: {APIKeyID: 1},
			}, nil
		})
		newResult <- err
	}()
	select {
	case <-newStarted:
	case <-time.After(time.Second):
		t.Fatal("new caller joined the canceled flight instead of starting fresh work")
	}

	close(releaseOld)
	<-oldDone
	state.mu.Lock()
	newFlightStillRegistered := state.flights["same-key"] != nil
	state.mu.Unlock()
	require.True(t, newFlightStillRegistered, "old completion must not remove the replacement flight")

	close(releaseNew)
	require.NoError(t, <-newResult)
}

func TestBatchAPIKeyUsageStatsMapsInternalDeadlineToGatewayTimeout(t *testing.T) {
	repo := &apiKeyUsageStatsRepositoryStub{err: context.DeadlineExceeded}
	svc := newUsageService(repo, nil, nil, nil, nil)

	_, err := svc.GetDashboardAPIKeyUsageStats(context.Background(), 7, []int64{1})

	require.Error(t, err)
	require.True(t, infraerrors.IsGatewayTimeout(err))
	require.Equal(t, "API_KEY_USAGE_STATS_TIMEOUT", infraerrors.Reason(err))
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestBatchAPIKeyUsageStatsMapsCallerDeadlineWhileWaitingForFetchSlot(t *testing.T) {
	repo := &apiKeyUsageStatsRepositoryStub{}
	svc := newUsageService(repo, nil, nil, nil, nil)
	for range apiKeyUsageStatsMaxConcurrentFetch {
		require.True(t, svc.apiKeyUsageStatsState.acquireFetchSlot(context.Background()))
		defer svc.apiKeyUsageStatsState.releaseFetchSlot()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := svc.GetDashboardAPIKeyUsageStats(ctx, 7, []int64{1})

	require.True(t, infraerrors.IsGatewayTimeout(err))
	require.Equal(t, "API_KEY_USAGE_STATS_REQUEST_TIMEOUT", infraerrors.Reason(err))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Zero(t, repo.calls.Load())
}

func TestBatchAPIKeyUsageStatsPreservesCallerCancellationWhileWaitingForFetchSlot(t *testing.T) {
	repo := &apiKeyUsageStatsRepositoryStub{}
	svc := newUsageService(repo, nil, nil, nil, nil)
	for range apiKeyUsageStatsMaxConcurrentFetch {
		require.True(t, svc.apiKeyUsageStatsState.acquireFetchSlot(context.Background()))
		defer svc.apiKeyUsageStatsState.releaseFetchSlot()
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.GetDashboardAPIKeyUsageStats(ctx, 7, []int64{1})

	require.ErrorIs(t, err, context.Canceled)
	require.False(t, infraerrors.IsGatewayTimeout(err))
	require.Zero(t, repo.calls.Load())
}

func TestBatchAPIKeyUsageStatsBoundsDistinctCacheMissesAtTwo(t *testing.T) {
	release := make(chan struct{})
	repo := &apiKeyUsageStatsRepositoryStub{wait: release}
	cache := newAPIKeyUsageStatsCacheStub()
	svc := newUsageService(repo, nil, nil, nil, cache)

	const requests = 10
	results := make(chan error, requests)
	for id := int64(1); id <= requests; id++ {
		go func(apiKeyID int64) {
			_, err := svc.GetDashboardAPIKeyUsageStats(
				context.Background(),
				7,
				[]int64{apiKeyID},
			)
			results <- err
		}(id)
	}

	require.Eventually(t, func() bool {
		return repo.calls.Load() == int32(apiKeyUsageStatsMaxConcurrentFetch)
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, int32(apiKeyUsageStatsMaxConcurrentFetch), repo.maxActive.Load())
	close(release)

	for range requests {
		require.NoError(t, <-results)
	}
	require.Equal(t, int32(requests), repo.calls.Load())
	require.Equal(t, int32(apiKeyUsageStatsMaxConcurrentFetch), repo.maxActive.Load())
}

func TestDashboardAPIKeyUsageStatsCacheIsUserScoped(t *testing.T) {
	repo := &apiKeyUsageStatsRepositoryStub{}
	cache := newAPIKeyUsageStatsCacheStub()
	svc := newUsageService(repo, nil, nil, nil, cache)

	_, err := svc.GetDashboardAPIKeyUsageStats(context.Background(), 11, []int64{4})
	require.NoError(t, err)
	_, err = svc.GetDashboardAPIKeyUsageStats(context.Background(), 12, []int64{4})
	require.NoError(t, err)
	_, err = svc.GetDashboardAPIKeyUsageStats(context.Background(), 11, []int64{4})
	require.NoError(t, err)

	require.Equal(t, int32(2), repo.calls.Load())
	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Len(t, cache.values, 2)
}

func TestBatchAPIKeyUsageStatsCacheIsSharedAcrossServiceInstances(t *testing.T) {
	cache := newAPIKeyUsageStatsCacheStub()
	firstRepo := &apiKeyUsageStatsRepositoryStub{}
	secondRepo := &apiKeyUsageStatsRepositoryStub{}
	firstService := newUsageService(firstRepo, nil, nil, nil, cache)
	secondService := newUsageService(secondRepo, nil, nil, nil, cache)

	first, err := firstService.GetDashboardAPIKeyUsageStats(context.Background(), 7, []int64{5})
	require.NoError(t, err)
	second, err := secondService.GetDashboardAPIKeyUsageStats(context.Background(), 7, []int64{5})
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Equal(t, int32(1), firstRepo.calls.Load())
	require.Zero(t, secondRepo.calls.Load())
}

func TestBatchAPIKeyUsageStatsCacheFailureFallsBackWithoutSecondRead(t *testing.T) {
	repo := &apiKeyUsageStatsRepositoryStub{}
	cache := newAPIKeyUsageStatsCacheStub()
	cache.getErr = errors.New("redis unavailable")
	cache.setErr = errors.New("redis unavailable")
	svc := newUsageService(repo, nil, nil, nil, cache)

	stats, err := svc.GetDashboardAPIKeyUsageStats(context.Background(), 7, []int64{5})
	require.NoError(t, err)
	require.Len(t, stats, 1)
	stats[5].TotalActualCost = 999

	// The same request shape is served from a defensive L1 copy while Redis is
	// unavailable. A different shape still reaches PostgreSQL, but skips Redis
	// during the short failure backoff.
	cached, err := svc.GetDashboardAPIKeyUsageStats(context.Background(), 7, []int64{5})
	require.NoError(t, err)
	require.Equal(t, float64(50), cached[5].TotalActualCost)
	_, err = svc.GetDashboardAPIKeyUsageStats(context.Background(), 7, []int64{6})
	require.NoError(t, err)
	require.Equal(t, int32(2), repo.calls.Load())

	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Equal(t, 1, cache.getCalls)
	require.Equal(t, 1, cache.setCalls)
}

func TestBatchAPIKeyUsageStatsEvictsInvalidCacheEntry(t *testing.T) {
	repo := &apiKeyUsageStatsRepositoryStub{}
	cache := newAPIKeyUsageStatsCacheStub()
	svc := newUsageService(repo, nil, nil, nil, cache)

	normalizedIDs := []int64{3}
	key := apiKeyUsageStatsCacheKey(
		0,
		timezone.Location().String(),
		timezone.Today().UTC().Format(time.RFC3339),
		normalizedIDs,
	)
	cache.values[key] = `{"version":1,"day":"invalid","stats":{}}`

	stats, err := svc.GetBatchAPIKeyUsageStats(context.Background(), normalizedIDs, time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, int32(1), repo.calls.Load())

	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Equal(t, 1, cache.deleteCalls)
	require.Equal(t, 1, cache.setCalls)
}

func TestBatchAPIKeyUsageStatsExplicitRangeBypassesCache(t *testing.T) {
	repo := &apiKeyUsageStatsRepositoryStub{}
	cache := newAPIKeyUsageStatsCacheStub()
	svc := newUsageService(repo, nil, nil, nil, cache)
	start := time.Now().Add(-time.Hour)
	end := time.Now()

	_, err := svc.GetBatchAPIKeyUsageStats(context.Background(), []int64{1}, start, end)
	require.NoError(t, err)
	_, err = svc.GetBatchAPIKeyUsageStats(context.Background(), []int64{1}, start, end)
	require.NoError(t, err)
	require.Equal(t, int32(2), repo.calls.Load())

	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Zero(t, cache.getCalls)
	require.Zero(t, cache.setCalls)
}
