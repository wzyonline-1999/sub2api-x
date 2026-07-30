package repository

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestEnsureDatabaseSchema(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(`SELECT to_regnamespace\(\$1\) IS NOT NULL`).
		WithArgs("sub2api").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`CREATE SCHEMA IF NOT EXISTS "sub2api"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT current_schema\(\)`).
		WillReturnRows(sqlmock.NewRows([]string{"current_schema"}).AddRow("sub2api"))

	err = EnsureDatabaseSchema(context.Background(), db, " sub2api ")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureDatabaseSchemaRejectsInheritedNonPublicSchemaWhenPublicExpected(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(`SELECT to_regnamespace\(\$1\) IS NOT NULL`).
		WithArgs("public").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT current_schema\(\)`).
		WillReturnRows(sqlmock.NewRows([]string{"current_schema"}).AddRow("tenant_a"))

	err = EnsureDatabaseSchema(context.Background(), db, "public")
	require.Error(t, err)
	require.Contains(t, err.Error(), "DATABASE_SCHEMA=tenant_a")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureDatabaseSchemaDoesNotCreateExistingPublicSchema(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(`SELECT to_regnamespace\(\$1\) IS NOT NULL`).
		WithArgs("public").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT current_schema\(\)`).
		WillReturnRows(sqlmock.NewRows([]string{"current_schema"}).AddRow("public"))

	err = EnsureDatabaseSchema(context.Background(), db, "public")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureDatabaseSchemaSkipsEmptySchema(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	err = EnsureDatabaseSchema(context.Background(), db, "")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureDatabaseSchemaRejectsInvalidSchema(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	err = EnsureDatabaseSchema(context.Background(), db, "bad-schema")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid database schema")
	require.NoError(t, mock.ExpectationsWereMet())
}
