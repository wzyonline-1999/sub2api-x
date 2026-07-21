//go:build unit

package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestIncrementUsageBillingSubscriptionCarriesWindowVersions(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	mock.ExpectExec(`(?s)UPDATE user_subscriptions us.*daily_window_version = \$3.*weekly_window_version = \$4.*monthly_window_version = \$5`).
		WithArgs(1.25, int64(42), int64(3), int64(5), int64(8)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, incrementUsageBillingSubscription(context.Background(), tx, 42, 1.25, 3, 5, 8))
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}
