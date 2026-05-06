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

	mock.ExpectExec(`CREATE SCHEMA IF NOT EXISTS "sub2api"`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = EnsureDatabaseSchema(context.Background(), db, " sub2api ")
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
