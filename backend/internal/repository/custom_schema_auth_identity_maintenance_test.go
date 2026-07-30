package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestRepairCustomSchemaAuthIdentityHistoryRejectsUnsafeTargets(t *testing.T) {
	for _, schema := range []string{"", " ", "public", "Tenant-A"} {
		t.Run(schema, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			_, err = repairCustomSchemaAuthIdentityHistory(
				context.Background(),
				db,
				migrations.FS,
				schema,
			)
			require.Error(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRepairCustomSchemaAuthIdentityHistoryReplaysCanonicalDataWork(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1)")).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT current_schema()")).
		WillReturnRows(sqlmock.NewRows([]string{"current_schema"}).AddRow("tenant_history"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass(format('%I.%I', current_schema(), $1)) IS NOT NULL")).
		WithArgs("user_external_identities").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	expectCurrentSchemaRelations(t, mock, customSchemaAuthIdentityRequiredRelations...)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM schema_migrations WHERE filename IN ($1, $2, $3)")).
		WithArgs(
			customSchemaAuthIdentityRequiredMigrations[0],
			customSchemaAuthIdentityRequiredMigrations[1],
			customSchemaAuthIdentityRequiredMigrations[2],
		).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery(`(?s)WITH legacy AS .*WHERE canonical\.id IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SET LOCAL lock_timeout = '5s'")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)^CREATE OR REPLACE FUNCTION "tenant_history"\.__migration_115_safe_legacy_metadata_jsonb.*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)^CREATE OR REPLACE FUNCTION "tenant_history"\.__migration_116_safe_legacy_metadata_jsonb.*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(`(?s)WITH legacy AS .*WHERE canonical\.id IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := repairCustomSchemaAuthIdentityHistory(
		context.Background(),
		db,
		migrations.FS,
		"tenant_history",
	)
	require.NoError(t, err)
	require.Equal(t, int64(3), result.EligibleMissingBefore)
	require.Zero(t, result.EligibleMissingAfter)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepairCustomSchemaAuthIdentityHistoryReportsIncompleteRepair(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1)")).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT current_schema()")).
		WillReturnRows(sqlmock.NewRows([]string{"current_schema"}).AddRow("tenant_history"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass(format('%I.%I', current_schema(), $1)) IS NOT NULL")).
		WithArgs("user_external_identities").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	expectCurrentSchemaRelations(t, mock, customSchemaAuthIdentityRequiredRelations...)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM schema_migrations WHERE filename IN ($1, $2, $3)")).
		WithArgs(
			customSchemaAuthIdentityRequiredMigrations[0],
			customSchemaAuthIdentityRequiredMigrations[1],
			customSchemaAuthIdentityRequiredMigrations[2],
		).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery(`(?s)WITH legacy AS .*WHERE canonical\.id IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SET LOCAL lock_timeout = '5s'")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)^CREATE OR REPLACE FUNCTION "tenant_history"\.__migration_115_safe_legacy_metadata_jsonb.*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)^CREATE OR REPLACE FUNCTION "tenant_history"\.__migration_116_safe_legacy_metadata_jsonb.*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(`(?s)WITH legacy AS .*WHERE canonical\.id IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := repairCustomSchemaAuthIdentityHistory(
		context.Background(),
		db,
		migrations.FS,
		"tenant_history",
	)
	require.ErrorContains(t, err, "left 1 eligible legacy identities")
	require.Equal(t, int64(2), result.EligibleMissingBefore)
	require.Equal(t, int64(1), result.EligibleMissingAfter)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepairCustomSchemaAuthIdentityHistoryNoOpsWithoutTenantLegacyTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectMaintenanceLockAndSchema(mock, "tenant_history")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass(format('%I.%I', current_schema(), $1)) IS NOT NULL")).
		WithArgs("user_external_identities").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	expectMaintenanceUnlock(mock)

	result, err := repairCustomSchemaAuthIdentityHistory(
		context.Background(),
		db,
		migrations.FS,
		"tenant_history",
	)
	require.NoError(t, err)
	require.Zero(t, result.EligibleMissingBefore)
	require.Zero(t, result.EligibleMissingAfter)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepairCustomSchemaAuthIdentityHistoryRejectsMissingTenantDependency(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectMaintenanceLockAndSchema(mock, "tenant_history")
	expectCurrentSchemaRelation(mock, "user_external_identities", true)
	expectCurrentSchemaRelation(mock, "users", true)
	expectCurrentSchemaRelation(mock, "auth_identities", false)
	expectMaintenanceUnlock(mock)

	_, err = repairCustomSchemaAuthIdentityHistory(
		context.Background(),
		db,
		migrations.FS,
		"tenant_history",
	)
	require.ErrorContains(t, err, "required relation tenant_history.auth_identities is absent")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepairCustomSchemaAuthIdentityHistoryRequiresRecordedCanonicalMigrations(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectMaintenanceLockAndSchema(mock, "tenant_history")
	expectCurrentSchemaRelation(mock, "user_external_identities", true)
	expectCurrentSchemaRelations(t, mock, customSchemaAuthIdentityRequiredRelations...)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM schema_migrations WHERE filename IN ($1, $2, $3)")).
		WithArgs(
			customSchemaAuthIdentityRequiredMigrations[0],
			customSchemaAuthIdentityRequiredMigrations[1],
			customSchemaAuthIdentityRequiredMigrations[2],
		).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	expectMaintenanceUnlock(mock)

	_, err = repairCustomSchemaAuthIdentityHistory(
		context.Background(),
		db,
		migrations.FS,
		"tenant_history",
	)
	require.ErrorContains(t, err, "only 2/3 required auth identity migrations recorded")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReadSchemaAdaptedAuthIdentityMigrationsKeepsCanonicalFilesUntouched(t *testing.T) {
	for _, name := range customSchemaAuthIdentityMaintenanceMigrations {
		canonical, err := migrations.FS.ReadFile(name)
		require.NoError(t, err)
		require.Contains(t, string(canonical), "public.")

		adapted, err := readSchemaAdaptedMigration(migrations.FS, name, "tenant_history")
		require.NoError(t, err)
		require.NotContains(t, adapted, "public.")
		require.Contains(t, adapted, `"tenant_history".`)
		require.NotContains(t, adapted, "schema_migrations")
	}
}

func expectMaintenanceLockAndSchema(mock sqlmock.Sqlmock, schema string) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1)")).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT current_schema()")).
		WillReturnRows(sqlmock.NewRows([]string{"current_schema"}).AddRow(schema))
}

func expectCurrentSchemaRelations(t *testing.T, mock sqlmock.Sqlmock, relations ...string) {
	t.Helper()
	for _, relation := range relations {
		expectCurrentSchemaRelation(mock, relation, true)
	}
}

func expectCurrentSchemaRelation(mock sqlmock.Sqlmock, relation string, exists bool) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass(format('%I.%I', current_schema(), $1)) IS NOT NULL")).
		WithArgs(relation).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
}

func expectMaintenanceUnlock(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}
