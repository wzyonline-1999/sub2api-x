package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

var trendResultColumns = []string{
	"date", "requests", "input_tokens", "output_tokens",
	"cache_creation_tokens", "cache_read_tokens", "total_tokens",
	"cost", "actual_cost",
}

func TestUsageTrendQueriesUseResolvedRequestTimezone(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(48 * time.Hour)
	requestTimezone := timezone.ResolveUserLocation("Asia/Tokyo")
	ctx := timezone.WithResolvedUserLocation(context.Background(), requestTimezone)

	// A non-server request timezone must use the exact raw query. Server-timezone
	// rollups cannot safely represent arbitrary local day/hour boundaries.
	mock.ExpectQuery("(?s)TO_CHAR\\(created_at AT TIME ZONE \\$3, 'YYYY-MM-DD'\\).*FROM usage_logs").
		WithArgs(start, end, "Asia/Tokyo").
		WillReturnRows(sqlmock.NewRows(trendResultColumns))
	trend, err := repo.GetUsageTrendWithFilters(ctx, start, end, "day", 0, 0, 0, 0, "", nil, nil, nil)
	require.NoError(t, err)
	require.Empty(t, trend)

	mock.ExpectQuery("(?s)TO_CHAR\\(u.created_at AT TIME ZONE \\$6, 'YYYY-MM-DD'\\).*GROUP BY date, u.api_key_id").
		WithArgs(start, end, 5, start, end, "Asia/Tokyo").
		WillReturnRows(sqlmock.NewRows([]string{"date", "api_key_id", "key_name", "requests", "tokens"}))
	apiKeyTrend, err := repo.GetAPIKeyUsageTrend(ctx, start, end, "day", 5)
	require.NoError(t, err)
	require.Empty(t, apiKeyTrend)

	mock.ExpectQuery("(?s)TO_CHAR\\(u.created_at AT TIME ZONE \\$6, 'YYYY-MM-DD'\\).*GROUP BY date, u.user_id").
		WithArgs(start, end, 7, start, end, "Asia/Tokyo").
		WillReturnRows(sqlmock.NewRows([]string{"date", "user_id", "email", "username", "requests", "tokens", "cost", "actual_cost"}))
	userTrend, err := repo.GetUserUsageTrend(ctx, start, end, "day", 7)
	require.NoError(t, err)
	require.Empty(t, userTrend)

	mock.ExpectQuery("(?s)TO_CHAR\\(created_at AT TIME ZONE \\$4, 'YYYY-MM-DD'\\).*WHERE user_id = \\$1").
		WithArgs(int64(42), start, end, "Asia/Tokyo").
		WillReturnRows(sqlmock.NewRows(trendResultColumns))
	personalTrend, err := repo.GetUserUsageTrendByUserID(ctx, 42, start, end, "day")
	require.NoError(t, err)
	require.Empty(t, personalTrend)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageTrendInvalidRequestTimezoneFallsBackToServerTimezone(t *testing.T) {
	resolved := timezone.ResolveUserLocation("not/a-timezone")
	ctx := timezone.WithResolvedUserLocation(context.Background(), resolved)
	require.Equal(t, resolveUsageStatsTimezone(), resolveUsageStatsTimezoneForContext(ctx))
}
