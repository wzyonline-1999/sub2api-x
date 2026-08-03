//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dashboardRankingCacheRepo struct {
	service.UsageLogRepository
	calls       atomic.Int32
	started     chan struct{}
	startedOnce sync.Once
	release     <-chan struct{}
}

func (r *dashboardRankingCacheRepo) GetUserSpendingRanking(
	ctx context.Context,
	startTime, endTime time.Time,
	limit int,
) (*usagestats.UserSpendingRankingResponse, error) {
	r.calls.Add(1)
	if r.started != nil {
		r.startedOnce.Do(func() { close(r.started) })
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &usagestats.UserSpendingRankingResponse{
		Ranking: []usagestats.UserSpendingRankingItem{{
			UserID:     7,
			Email:      "rank@example.com",
			ActualCost: 10.5,
			Requests:   3,
			Tokens:     300,
		}},
		TotalActualCost: 88.8,
		TotalRequests:   44,
		TotalTokens:     1234,
	}, nil
}

func newDashboardRankingCacheTestRouter(repo *dashboardRankingCacheRepo) *gin.Engine {
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/users-ranking", handler.GetUserSpendingRanking)
	return router
}

func TestDashboardUsersRankingColdCacheCoalescesConcurrentRequests(t *testing.T) {
	previousCache := dashboardUsersRankingCache
	dashboardUsersRankingCache = newSnapshotCacheWithLoadTimeout(time.Minute, time.Second)
	t.Cleanup(func() { dashboardUsersRankingCache = previousCache })

	gin.SetMode(gin.TestMode)
	release := make(chan struct{})
	repo := &dashboardRankingCacheRepo{
		started: make(chan struct{}),
		release: release,
	}
	router := newDashboardRankingCacheTestRouter(repo)

	const callers = 8
	start := make(chan struct{})
	recorders := make(chan *httptest.ResponseRecorder, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			<-start
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(
				http.MethodGet,
				"/admin/dashboard/users-ranking?start_date=2026-07-01&end_date=2026-07-02&limit=12&timezone=Asia%2FShanghai",
				nil,
			)
			router.ServeHTTP(recorder, request)
			recorders <- recorder
		}()
	}
	close(start)
	<-repo.started
	require.Eventually(t, func() bool {
		dashboardUsersRankingCache.mu.RLock()
		defer dashboardUsersRankingCache.mu.RUnlock()
		for _, flight := range dashboardUsersRankingCache.flights {
			if flight.waiters == callers {
				return true
			}
		}
		return false
	}, time.Second, time.Millisecond)
	close(release)
	wg.Wait()
	close(recorders)

	for recorder := range recorders {
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "miss", recorder.Header().Get("X-Snapshot-Cache"))
		require.Contains(t, recorder.Body.String(), "rank@example.com")
	}
	require.Equal(t, int32(1), repo.calls.Load())
}

func TestDashboardUsersRankingLoaderDeadlineReturnsGatewayTimeout(t *testing.T) {
	previousCache := dashboardUsersRankingCache
	dashboardUsersRankingCache = newSnapshotCacheWithLoadTimeout(time.Minute, 20*time.Millisecond)
	t.Cleanup(func() { dashboardUsersRankingCache = previousCache })

	gin.SetMode(gin.TestMode)
	repo := &dashboardRankingCacheRepo{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	router := newDashboardRankingCacheTestRouter(repo)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/admin/dashboard/users-ranking?start_date=2026-07-01&end_date=2026-07-02",
		nil,
	)

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusGatewayTimeout, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Timed out while getting user spending ranking")
	require.Equal(t, int32(1), repo.calls.Load())
}
