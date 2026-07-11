package repository

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRepositoryListStableQuotaLoadByGroupIDsUsesStableFiltersOnly(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	future := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"group_id", "account_id", "platform", "extra", "session_window_end",
	}).AddRow(
		int64(20), int64(7), service.PlatformOpenAI,
		`{"codex_5h_used_percent":75}`, future,
	)
	var capturedSQL string
	mock.ExpectQuery("SELECT").
		WithArgs(sqlmock.AnyArg(), service.StatusActive, sqlmock.AnyArg()).
		WillReturnRows(rows)
	repo := newAccountRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQL}, nil)

	got, err := repo.ListStableQuotaLoadByGroupIDs(context.Background(), []int64{20, 10, 20, -1})
	require.NoError(t, err)
	require.Equal(t, []service.GroupAccountQuotaLoadRow{{
		GroupID:          20,
		AccountID:        7,
		Platform:         service.PlatformOpenAI,
		Extra:            map[string]any{"codex_5h_used_percent": float64(75)},
		SessionWindowEnd: &future,
	}}, got)

	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "a.deleted_at IS NULL")
	require.Contains(t, normalized, "a.status = $2")
	require.Contains(t, normalized, "a.schedulable = TRUE")
	require.Contains(t, normalized, "a.expires_at IS NULL OR a.expires_at > $3 OR a.auto_pause_on_expired = FALSE")
	require.NotContains(t, normalized, "temp_unschedulable_until",
		"temporary cooldown accounts must stay in quota load aggregation")
	require.NotContains(t, normalized, "overload_until",
		"overloaded accounts must stay in quota load aggregation")
	require.NotContains(t, normalized, "rate_limit_reset_at",
		"rate-limited accounts must stay in quota load aggregation")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountRepositoryListStableQuotaLoadByGroupIDsSkipsEmptyInput(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := newAccountRepositoryWithSQL(nil, db, nil)

	got, err := repo.ListStableQuotaLoadByGroupIDs(context.Background(), []int64{0, -1})
	require.NoError(t, err)
	require.Empty(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}
