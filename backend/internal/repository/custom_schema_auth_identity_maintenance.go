package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/migrations"
)

var customSchemaAuthIdentityMaintenanceMigrations = []string{
	"115_auth_identity_legacy_external_backfill.sql",
	"116_auth_identity_legacy_external_safety_reports.sql",
}

var customSchemaAuthIdentityRequiredMigrations = []string{
	"115_auth_identity_legacy_external_backfill.sql",
	"116_auth_identity_legacy_external_safety_reports.sql",
	"191y_fork_repair_custom_schema_objects.sql",
}

var customSchemaAuthIdentityRequiredRelations = []string{
	"users",
	"auth_identities",
	"auth_identity_channels",
	"auth_identity_migration_reports",
	"schema_migrations",
}

const eligibleMissingLegacyAuthIdentitiesQuery = `
WITH legacy AS (
    SELECT
        legacy.user_id,
        LOWER(BTRIM(COALESCE(legacy.provider, ''))) AS provider_type,
        CASE
            WHEN LOWER(BTRIM(COALESCE(legacy.provider, ''))) = 'wechat'
                THEN 'wechat-main'
            ELSE 'linuxdo'
        END AS provider_key,
        CASE
            WHEN LOWER(BTRIM(COALESCE(legacy.provider, ''))) = 'wechat'
                THEN BTRIM(COALESCE(legacy.provider_union_id, ''))
            ELSE BTRIM(COALESCE(legacy.provider_user_id, ''))
        END AS provider_subject
    FROM user_external_identities AS legacy
    JOIN users AS app_user ON app_user.id = legacy.user_id
    WHERE app_user.deleted_at IS NULL
      AND LOWER(BTRIM(COALESCE(legacy.provider, ''))) IN ('linuxdo', 'wechat')
      AND CASE
            WHEN LOWER(BTRIM(COALESCE(legacy.provider, ''))) = 'wechat'
                THEN BTRIM(COALESCE(legacy.provider_union_id, '')) <> ''
            ELSE BTRIM(COALESCE(legacy.provider_user_id, '')) <> ''
          END
),
eligible AS (
    SELECT
        provider_type,
        provider_key,
        provider_subject
    FROM legacy
    GROUP BY provider_type, provider_key, provider_subject
    HAVING COUNT(DISTINCT user_id) = 1
)
SELECT COUNT(*)
FROM eligible
LEFT JOIN auth_identities AS canonical
  ON canonical.provider_type = eligible.provider_type
 AND canonical.provider_key = eligible.provider_key
 AND canonical.provider_subject = eligible.provider_subject
WHERE canonical.id IS NULL
`

// CustomSchemaAuthIdentityMaintenanceResult reports how many unambiguous
// legacy identities lacked a canonical auth_identities row around the repair.
type CustomSchemaAuthIdentityMaintenanceResult struct {
	EligibleMissingBefore int64
	EligibleMissingAfter  int64
}

// RepairCustomSchemaAuthIdentityHistory explicitly replays the idempotent data
// work from migrations 115 and 116 for an existing non-public schema.
//
// This is intentionally separate from application startup. It serializes with
// normal migrations, executes schema-adapted copies of the canonical SQL, and
// never modifies schema_migrations. The current release must complete normal
// startup migrations, including 191y, before this maintenance is allowed.
func RepairCustomSchemaAuthIdentityHistory(
	ctx context.Context,
	db *sql.DB,
	schema string,
) (CustomSchemaAuthIdentityMaintenanceResult, error) {
	return repairCustomSchemaAuthIdentityHistory(ctx, db, migrations.FS, schema)
}

func repairCustomSchemaAuthIdentityHistory(
	ctx context.Context,
	db *sql.DB,
	fsys fs.FS,
	schema string,
) (result CustomSchemaAuthIdentityMaintenanceResult, returnErr error) {
	if db == nil {
		return result, errors.New("nil sql db")
	}
	schema = strings.TrimSpace(schema)
	if schema == "" || schema == "public" {
		return result, errors.New("custom-schema auth identity maintenance requires an explicit non-public schema")
	}
	if !config.IsValidPostgresIdentifier(schema) {
		return result, fmt.Errorf(
			"invalid database schema %q; use lowercase letters, digits, and underscores",
			schema,
		)
	}

	lockConn, err := db.Conn(ctx)
	if err != nil {
		return result, fmt.Errorf("acquire maintenance lock connection: %w", err)
	}
	defer func() {
		if err := lockConn.Close(); returnErr == nil && err != nil {
			returnErr = fmt.Errorf("close maintenance connection: %w", err)
		}
	}()
	if err := pgAdvisoryLock(ctx, lockConn); err != nil {
		return result, err
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := pgAdvisoryUnlock(unlockCtx, lockConn); returnErr == nil && err != nil {
			returnErr = err
		}
	}()

	var currentSchema sql.NullString
	if err := lockConn.QueryRowContext(ctx, "SELECT current_schema()").Scan(&currentSchema); err != nil {
		return result, fmt.Errorf("check current database schema: %w", err)
	}
	if !currentSchema.Valid || currentSchema.String != schema {
		return result, databaseSchemaMismatchError(schema, currentSchema.String)
	}

	legacyTablePresent, err := relationExistsInCurrentSchema(ctx, lockConn, "user_external_identities")
	if err != nil {
		return result, err
	}
	if !legacyTablePresent {
		return result, nil
	}
	for _, relation := range customSchemaAuthIdentityRequiredRelations {
		exists, err := relationExistsInCurrentSchema(ctx, lockConn, relation)
		if err != nil {
			return result, err
		}
		if !exists {
			return result, fmt.Errorf(
				"required relation %s.%s is absent; run normal migrations before custom-schema maintenance",
				schema,
				relation,
			)
		}
	}
	if err := requireAuthIdentityMaintenanceMigrationsApplied(ctx, lockConn); err != nil {
		return result, err
	}
	result.EligibleMissingBefore, err = countEligibleMissingLegacyAuthIdentities(ctx, lockConn)
	if err != nil {
		return result, fmt.Errorf("count eligible missing identities before maintenance: %w", err)
	}

	tx, err := lockConn.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin custom-schema auth identity maintenance: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		_ = tx.Rollback()
		return result, fmt.Errorf("set maintenance lock timeout: %w", err)
	}
	for _, name := range customSchemaAuthIdentityMaintenanceMigrations {
		content, err := readSchemaAdaptedMigration(fsys, name, schema)
		if err != nil {
			_ = tx.Rollback()
			return result, err
		}
		if _, err := tx.ExecContext(ctx, content); err != nil {
			_ = tx.Rollback()
			return result, fmt.Errorf("replay migration data work %s: %w", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return result, fmt.Errorf("commit custom-schema auth identity maintenance: %w", err)
	}

	result.EligibleMissingAfter, err = countEligibleMissingLegacyAuthIdentities(ctx, lockConn)
	if err != nil {
		return result, fmt.Errorf("count eligible missing identities after maintenance: %w", err)
	}
	if result.EligibleMissingAfter != 0 {
		return result, fmt.Errorf(
			"custom-schema auth identity maintenance left %d eligible legacy identities without canonical rows",
			result.EligibleMissingAfter,
		)
	}
	return result, nil
}

type maintenanceQueryConnection interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func relationExistsInCurrentSchema(
	ctx context.Context,
	conn maintenanceQueryConnection,
	relation string,
) (bool, error) {
	var exists bool
	if err := conn.QueryRowContext(
		ctx,
		"SELECT to_regclass(format('%I.%I', current_schema(), $1)) IS NOT NULL",
		relation,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("check current-schema relation %s: %w", relation, err)
	}
	return exists, nil
}

func requireAuthIdentityMaintenanceMigrationsApplied(
	ctx context.Context,
	conn maintenanceQueryConnection,
) error {
	var applied int
	if err := conn.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE filename IN ($1, $2, $3)`,
		customSchemaAuthIdentityRequiredMigrations[0],
		customSchemaAuthIdentityRequiredMigrations[1],
		customSchemaAuthIdentityRequiredMigrations[2],
	).Scan(&applied); err != nil {
		return fmt.Errorf("check auth identity migration history: %w", err)
	}
	if applied != len(customSchemaAuthIdentityRequiredMigrations) {
		return fmt.Errorf(
			"current schema has only %d/%d required auth identity migrations recorded; run normal migrations before custom-schema maintenance",
			applied,
			len(customSchemaAuthIdentityRequiredMigrations),
		)
	}
	return nil
}

func countEligibleMissingLegacyAuthIdentities(
	ctx context.Context,
	conn maintenanceQueryConnection,
) (int64, error) {
	var count int64
	if err := conn.QueryRowContext(ctx, eligibleMissingLegacyAuthIdentitiesQuery).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func readSchemaAdaptedMigration(fsys fs.FS, name, schema string) (string, error) {
	contentBytes, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", fmt.Errorf("read maintenance migration %s: %w", name, err)
	}
	content := strings.TrimSpace(string(contentBytes))
	if content == "" {
		return "", fmt.Errorf("maintenance migration %s is empty", name)
	}
	executionContent, err := migrationSQLForSchema(name, content, schema)
	if err != nil {
		return "", fmt.Errorf("adapt maintenance migration %s for schema %q: %w", name, schema, err)
	}
	return executionContent, nil
}
