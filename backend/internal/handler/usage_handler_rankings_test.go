package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type usageRankingRepoCapture struct {
	service.UsageLogRepository
	query usagestats.UsageRankingQuery
}

func (r *usageRankingRepoCapture) GetUsageRanking(ctx context.Context, query usagestats.UsageRankingQuery) (*usagestats.UsageRankingResponse, error) {
	r.query = query
	return &usagestats.UsageRankingResponse{
		Metric:    query.Metric,
		Period:    query.Period,
		StartDate: query.StartTime.Format("2006-01-02"),
		EndDate:   query.EndTime.AddDate(0, 0, -1).Format("2006-01-02"),
		Summary: usagestats.UsageRankingSummary{
			TotalTokens:     128600000,
			TotalActualCost: 1842.62,
			TotalRequests:   294108,
			RankedUsers:     128,
		},
		Ranking: []usagestats.UsageRankingItem{
			{Rank: 1, UserID: 1, DisplayName: "z***u@example.com", TotalTokens: 128600000, ActualCost: 1842.62, Requests: 12800},
		},
		CurrentUser: &usagestats.UsageRankingItem{Rank: 18, UserID: query.CurrentUserID, DisplayName: "me", TotalTokens: 12800000, ActualCost: 128.6, Requests: 1800},
	}, nil
}

func newUsageRankingTestRouter(repo *usageRankingRepoCapture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	usageSvc := service.NewUsageService(repo, nil, nil, nil)
	handler := NewUsageHandler(usageSvc, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		c.Next()
	})
	router.GET("/usage/rankings", handler.Rankings)
	return router
}

func TestUsageRankingsRejectsInvalidMetric(t *testing.T) {
	repo := &usageRankingRepoCapture{}
	router := newUsageRankingTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/usage/rankings?metric=model&period=month", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, repo.query.Metric)
}

func TestUsageRankingsDefaultsToDayPeriod(t *testing.T) {
	repo := &usageRankingRepoCapture{}
	router := newUsageRankingTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/usage/rankings?timezone=UTC", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, usagestats.UsageRankingMetricTokens, repo.query.Metric)
	require.Equal(t, usagestats.UsageRankingPeriodDay, repo.query.Period)
	require.Equal(t, 10, repo.query.Limit)
	require.Equal(t, int64(42), repo.query.CurrentUserID)
	require.False(t, repo.query.StartTime.IsZero())
	require.False(t, repo.query.EndTime.IsZero())
	require.Equal(t, 24*time.Hour, repo.query.EndTime.Sub(repo.query.StartTime))
	require.Equal(t, repo.query.StartTime.AddDate(0, 0, -1), repo.query.ComparisonStartTime)
	require.True(t, repo.query.ComparisonEndTime.After(repo.query.ComparisonStartTime))
	require.False(t, repo.query.ComparisonEndTime.After(repo.query.StartTime))
}

func TestUsageRankingsPassesMetricPeriodLimitAndCurrentUser(t *testing.T) {
	repo := &usageRankingRepoCapture{}
	router := newUsageRankingTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/usage/rankings?metric=cost&period=week&limit=100&timezone=UTC", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, usagestats.UsageRankingMetricCost, repo.query.Metric)
	require.Equal(t, usagestats.UsageRankingPeriodWeek, repo.query.Period)
	require.Equal(t, 10, repo.query.Limit)
	require.Equal(t, int64(42), repo.query.CurrentUserID)
	require.False(t, repo.query.StartTime.IsZero())
	require.False(t, repo.query.EndTime.IsZero())
	require.True(t, repo.query.EndTime.After(repo.query.StartTime))
	require.LessOrEqual(t, repo.query.EndTime.Sub(repo.query.StartTime), 8*24*time.Hour)
	require.Equal(t, repo.query.StartTime.AddDate(0, 0, -7), repo.query.ComparisonStartTime)
	require.True(t, repo.query.ComparisonEndTime.After(repo.query.ComparisonStartTime))
	require.False(t, repo.query.ComparisonEndTime.After(repo.query.StartTime))
}

func TestUsageRankingsIgnoresCallerTimezoneForSharedGlobalCache(t *testing.T) {
	repo := &usageRankingRepoCapture{}
	router := newUsageRankingTestRouter(repo)

	req := httptest.NewRequest(
		http.MethodGet,
		"/usage/rankings?metric=cost&period=month&timezone=Pacific%2FKiritimati",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, timezone.Location(), repo.query.StartTime.Location())
	require.Equal(t, timezone.Location(), repo.query.EndTime.Location())
	require.NotEqual(t, "Pacific/Kiritimati", repo.query.StartTime.Location().String())
}

func TestUsageRankingComparisonTimeRangeUsesEquivalentElapsedProgress(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	tests := []struct {
		name              string
		period            usagestats.UsageRankingPeriod
		currentStart      time.Time
		currentEnd        time.Time
		now               time.Time
		wantPreviousStart time.Time
		wantPreviousEnd   time.Time
	}{
		{
			name:              "day",
			period:            usagestats.UsageRankingPeriodDay,
			currentStart:      time.Date(2026, 7, 14, 0, 0, 0, 0, loc),
			currentEnd:        time.Date(2026, 7, 15, 0, 0, 0, 0, loc),
			now:               time.Date(2026, 7, 14, 15, 30, 0, 0, loc),
			wantPreviousStart: time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
			wantPreviousEnd:   time.Date(2026, 7, 13, 15, 30, 0, 0, loc),
		},
		{
			name:              "week",
			period:            usagestats.UsageRankingPeriodWeek,
			currentStart:      time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
			currentEnd:        time.Date(2026, 7, 20, 0, 0, 0, 0, loc),
			now:               time.Date(2026, 7, 14, 15, 30, 0, 0, loc),
			wantPreviousStart: time.Date(2026, 7, 6, 0, 0, 0, 0, loc),
			wantPreviousEnd:   time.Date(2026, 7, 7, 15, 30, 0, 0, loc),
		},
		{
			name:              "month",
			period:            usagestats.UsageRankingPeriodMonth,
			currentStart:      time.Date(2026, 7, 1, 0, 0, 0, 0, loc),
			currentEnd:        time.Date(2026, 8, 1, 0, 0, 0, 0, loc),
			now:               time.Date(2026, 7, 14, 15, 30, 0, 0, loc),
			wantPreviousStart: time.Date(2026, 6, 1, 0, 0, 0, 0, loc),
			wantPreviousEnd:   time.Date(2026, 6, 14, 15, 30, 0, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previousStart, previousEnd := usageRankingComparisonTimeRange(tt.period, tt.currentStart, tt.currentEnd, tt.now)
			require.Equal(t, tt.wantPreviousStart, previousStart)
			require.Equal(t, tt.wantPreviousEnd, previousEnd)
		})
	}
}
