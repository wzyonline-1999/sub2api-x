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
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", quotePostgresIdentifier(schema))); err != nil {
		return fmt.Errorf("create schema %q: %w", schema, err)
	}
	return nil
}

func quotePostgresIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
