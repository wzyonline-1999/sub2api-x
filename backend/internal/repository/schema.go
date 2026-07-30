package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func EnsureDatabaseSchema(ctx context.Context, db *sql.DB, schema string) error {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return nil
	}
	if !config.IsValidPostgresIdentifier(schema) {
		return fmt.Errorf("invalid database schema %q; use lowercase letters, digits, and underscores", schema)
	}
	var schemaExists bool
	if err := db.QueryRowContext(ctx, "SELECT to_regnamespace($1) IS NOT NULL", schema).Scan(&schemaExists); err != nil {
		return fmt.Errorf("check schema %q: %w", schema, err)
	}
	if !schemaExists {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", quotePostgresIdentifier(schema))); err != nil {
			return fmt.Errorf("create schema %q: %w", schema, err)
		}
	}
	var currentSchema sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT current_schema()").Scan(&currentSchema); err != nil {
		return fmt.Errorf("check current database schema: %w", err)
	}
	if !currentSchema.Valid || currentSchema.String != schema {
		return databaseSchemaMismatchError(schema, currentSchema.String)
	}
	return nil
}

func databaseSchemaMismatchError(expected, current string) error {
	if expected == "public" {
		return fmt.Errorf(
			"database schema mismatch: DATABASE_SCHEMA is empty or set to public, but PostgreSQL current_schema() is %q; set DATABASE_SCHEMA=%s explicitly to keep the existing schema, or reset the role/database search_path to public",
			current,
			current,
		)
	}
	return fmt.Errorf(
		"database schema mismatch: configured=%q current=%q; ensure the connection search_path selects DATABASE_SCHEMA first",
		expected,
		current,
	)
}

func quotePostgresIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
