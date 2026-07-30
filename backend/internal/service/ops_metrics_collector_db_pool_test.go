package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestOpsMetricsCollectorDBPoolStatsReportsWaitEventDeltaSincePreviousSample(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()
	db.SetMaxOpenConns(1)

	collector := &OpsMetricsCollector{db: db}

	_, _, firstWaitEventDelta := collector.dbPoolStats()
	require.Equal(t, 0, firstWaitEventDelta)

	mock.ExpectQuery("SELECT 1").
		WillDelayFor(80 * time.Millisecond).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectQuery("SELECT 2").
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(2))

	firstStarted := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		close(firstStarted)
		firstDone <- runDBPoolStatsTestQuery(db, "SELECT 1")
	}()
	<-firstStarted
	time.Sleep(20 * time.Millisecond)

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- runDBPoolStatsTestQuery(db, "SELECT 2")
	}()

	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)
	require.NoError(t, mock.ExpectationsWereMet())

	_, _, waitEventDelta := collector.dbPoolStats()
	require.GreaterOrEqual(t, waitEventDelta, 1)

	_, _, nextWaitEventDelta := collector.dbPoolStats()
	require.Equal(t, 0, nextWaitEventDelta)
}

func runDBPoolStatsTestQuery(db *sql.DB, query string) error {
	rows, err := db.QueryContext(context.Background(), query)
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	if err := rows.Err(); err != nil {
		closeErr := rows.Close()
		if closeErr != nil {
			return closeErr
		}
		return err
	}
	return rows.Close()
}
