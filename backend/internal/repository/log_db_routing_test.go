package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryRoutesLogWritesToIsolatedPool(t *testing.T) {
	mainDB, mainMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mainDB.Close() })
	logSQLDB, logMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = logSQLDB.Close() })

	repo := ProvideOpsRepository(mainDB, &LogDB{db: logSQLDB}).(*opsRepository)
	createdAt := time.Date(2026, 7, 22, 8, 0, 0, 0, time.UTC)

	logMock.ExpectExec(`INSERT INTO ops_system_logs`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	inserted, err := repo.BatchInsertSystemLogs(context.Background(), []*service.OpsInsertSystemLogInput{
		{CreatedAt: createdAt, Level: "info", Message: "request completed"},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), inserted)

	mainMock.ExpectExec(`UPDATE ops_error_logs`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.UpdateErrorResolution(context.Background(), 7, true, nil, nil))

	require.NoError(t, logMock.ExpectationsWereMet())
	require.NoError(t, mainMock.ExpectationsWereMet())
}

func TestAuditRepositoryRoutesOnlyAsyncBatchToIsolatedPool(t *testing.T) {
	mainDB, mainMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mainDB.Close() })
	logSQLDB, logMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = logSQLDB.Close() })

	repo := ProvideAuditLogRepository(mainDB, &LogDB{db: logSQLDB}).(*auditLogRepository)
	entry := &service.AuditLog{
		CreatedAt:  time.Date(2026, 7, 22, 8, 0, 0, 0, time.UTC),
		Action:     "user.update",
		StatusCode: 200,
	}

	logMock.ExpectExec(`INSERT INTO audit_logs`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	inserted, err := repo.BatchInsert(context.Background(), []*service.AuditLog{entry})
	require.NoError(t, err)
	require.Equal(t, int64(1), inserted)

	// The synchronous clear-trace path must remain on the primary pool.
	mainMock.ExpectExec(`INSERT INTO audit_logs`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.Insert(context.Background(), entry))

	require.NoError(t, logMock.ExpectationsWereMet())
	require.NoError(t, mainMock.ExpectationsWereMet())
}
