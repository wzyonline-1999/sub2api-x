package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration191xAlignsForkSessionMetadataWithOfficialSessionID(t *testing.T) {
	content, err := FS.ReadFile("191x_fork_align_usage_log_session_metadata.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS session_id_source VARCHAR(32)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS session_hash VARCHAR(64)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS session_explicit BOOLEAN")
	require.NotContains(t, sql, "ADD COLUMN IF NOT EXISTS session_id ")
	require.Contains(t, sql, "char_length(session_id) <= 255")
	require.Contains(t, sql, "chk_usage_logs_session_id_length")
	require.NotContains(t, sql, "ADD CONSTRAINT chk_usage_logs_prompt_cache_key_not_persisted")
	require.Contains(t, sql, "finalize-usage-log-session-metadata.sql")
	require.Contains(t, sql, "NOT VALID")
	require.Contains(t, sql, "SET LOCAL lock_timeout = '5s'")
	require.NotContains(t, sql, "UPDATE usage_logs")
	require.NotContains(t, sql, "ALTER COLUMN session_id TYPE")
	require.NotContains(t, sql, "VALIDATE CONSTRAINT")
	require.NotContains(t, sql, "FROM usage_logs")
}

func TestForkMigrationsStayBehindFutureOfficialNumbers(t *testing.T) {
	files, err := fs.Glob(FS, "*.sql")
	require.NoError(t, err)

	for _, name := range files {
		require.Falsef(t, strings.HasPrefix(name, "900_"), "fork migration %s must not become a permanent latest baseline", name)
	}
	require.Contains(t, files, "191x_fork_align_usage_log_session_metadata.sql")
	require.Contains(t, files, "191y_fork_repair_custom_schema_objects.sql")
	require.Contains(t, files, "191z_fork_rebind_long_context_triggers.sql")
	require.Less(t, "191z_fork_rebind_long_context_triggers.sql", "192_future_official.sql")
}

func TestMigration191yRepairsCustomSchemaObjectsByRelation(t *testing.T) {
	content, err := FS.ReadFile("191y_fork_repair_custom_schema_objects.sql")
	require.NoError(t, err)

	sql := string(content)
	for _, relation := range []string{
		"usage_logs",
		"users",
		"auth_identities",
		"auth_identity_channels",
		"auth_identity_migration_reports",
		"channel_monitors",
		"channel_monitor_request_templates",
	} {
		require.Contains(t, sql, "'"+relation+"'::regclass")
	}
	require.Contains(t, sql, "channel_monitors_template_id_fkey")
	require.Contains(t, sql, "channel_monitors_api_mode_check")
	require.Contains(t, sql, "channel_monitor_request_templates_api_mode_check")
	require.Contains(t, sql, "'grok'")
	require.Contains(t, sql, "NOT VALID")
	require.NotContains(t, sql, "public.")
	require.Contains(t, sql, "SET LOCAL lock_timeout = '5s'")
}

func TestMigration191zRebindsLongContextTriggersWithoutReplayingBackfill(t *testing.T) {
	content, err := FS.ReadFile("191z_fork_rebind_long_context_triggers.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION enforce_openai_long_context_billing_extra()")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION propagate_openai_long_context_billing_extra_to_shadows()")
	require.Contains(t, sql, "accounts_enforce_openai_long_context_billing_extra")
	require.Contains(t, sql, "accounts_propagate_openai_long_context_billing_extra")
	require.Equal(t, 2, strings.Count(sql, "SET search_path FROM CURRENT"))
	require.Contains(t, sql, "SET LOCAL lock_timeout = '5s'")
	require.NotContains(t, sql, "public.")
	require.NotContains(t, sql, "UPDATE accounts\nSET extra")
	require.NotContains(t, sql, "DROP FUNCTION")
}
