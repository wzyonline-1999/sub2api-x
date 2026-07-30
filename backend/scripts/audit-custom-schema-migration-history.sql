-- Read-only audit for installations that used a non-public PostgreSQL schema
-- before the schema-aware migration runner was introduced.
--
-- This script does not repair or delete persistent data. It identifies the
-- historical cases that cannot safely be replayed during application startup.
--
-- Usage:
--   psql "$DATABASE_URL" -v schema=sub2api \
--     -f scripts/audit-custom-schema-migration-history.sql

\set ON_ERROR_STOP on
\if :{?schema}
\else
\echo 'ERROR: pass the existing non-public schema with -v schema=<name>'
\quit 1
\endif

SELECT format('SET search_path TO %I, public', :'schema')
\gexec

CREATE TEMP TABLE migration_history_audit (
    check_name TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    detail TEXT NOT NULL
) ON COMMIT PRESERVE ROWS;

DO $$
DECLARE
    issue_count BIGINT;
    trigger_count INTEGER;
    local_function_count INTEGER;
    pinned_function_count INTEGER;
BEGIN
    IF current_schema() IS NULL OR current_schema() = 'public' THEN
        RAISE EXCEPTION
            'this audit requires an existing non-public current_schema()';
    END IF;

    IF to_regclass(format('%I.user_subscriptions', current_schema())) IS NULL THEN
        INSERT INTO migration_history_audit
        VALUES ('006_subscription_expiry', 'not_applicable', 'user_subscriptions table is absent');
    ELSE
        EXECUTE $query$
            SELECT COUNT(*)
            FROM user_subscriptions
            WHERE expires_at > TIMESTAMPTZ '2099-12-31 23:59:59+00'
        $query$ INTO issue_count;
        INSERT INTO migration_history_audit
        VALUES (
            '006_subscription_expiry',
            CASE WHEN issue_count = 0 THEN 'ok' ELSE 'needs_repair' END,
            format('%s rows still exceed the supported expiration bound', issue_count)
        );
    END IF;

    IF to_regclass(format('%I.usage_logs', current_schema())) IS NULL THEN
        INSERT INTO migration_history_audit
        VALUES ('009_cache_token_backfill', 'not_applicable', 'usage_logs table is absent');
    ELSIF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'usage_logs'
          AND column_name = 'cache_creation5m_tokens'
    ) OR EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'usage_logs'
          AND column_name = 'cache_creation1h_tokens'
    ) THEN
        INSERT INTO migration_history_audit
        VALUES (
            '009_cache_token_backfill',
            'needs_review',
            'legacy cache-token columns still exist; compare them with the underscored columns before dropping'
        );
    ELSE
        INSERT INTO migration_history_audit
        VALUES (
            '009_cache_token_backfill',
            'history_only',
            'legacy columns were already dropped; a missed historical backfill can only be verified from a backup'
        );
    END IF;

    IF to_regclass(format('%I.user_external_identities', current_schema())) IS NULL
       OR to_regclass(format('%I.auth_identities', current_schema())) IS NULL THEN
        INSERT INTO migration_history_audit
        VALUES (
            '115_116_legacy_identity_backfill',
            'not_applicable',
            'legacy or canonical identity table is absent'
        );
    ELSE
        EXECUTE $query$
            SELECT COUNT(*)
            FROM user_external_identities AS legacy
            JOIN users AS u ON u.id = legacy.user_id
            LEFT JOIN auth_identities AS canonical
              ON canonical.provider_type = LOWER(BTRIM(COALESCE(legacy.provider, '')))
             AND canonical.provider_key = CASE
                    WHEN LOWER(BTRIM(COALESCE(legacy.provider, ''))) = 'wechat'
                        THEN 'wechat-main'
                    ELSE 'linuxdo'
                 END
             AND canonical.provider_subject = CASE
                    WHEN LOWER(BTRIM(COALESCE(legacy.provider, ''))) = 'wechat'
                        THEN BTRIM(COALESCE(legacy.provider_union_id, ''))
                    ELSE BTRIM(COALESCE(legacy.provider_user_id, ''))
                 END
            WHERE u.deleted_at IS NULL
              AND LOWER(BTRIM(COALESCE(legacy.provider, ''))) IN ('linuxdo', 'wechat')
              AND (
                    (
                        LOWER(BTRIM(COALESCE(legacy.provider, ''))) = 'linuxdo'
                        AND BTRIM(COALESCE(legacy.provider_user_id, '')) <> ''
                    )
                    OR (
                        LOWER(BTRIM(COALESCE(legacy.provider, ''))) = 'wechat'
                        AND BTRIM(COALESCE(legacy.provider_union_id, '')) <> ''
                    )
                  )
              AND (
                    canonical.id IS NULL
                    OR canonical.user_id <> legacy.user_id
                  )
        $query$ INTO issue_count;
        INSERT INTO migration_history_audit
        VALUES (
            '115_116_legacy_identity_backfill',
            CASE WHEN issue_count = 0 THEN 'ok' ELSE 'needs_maintenance' END,
            format(
                '%s legacy identity rows have a missing or conflicting canonical auth_identities row',
                issue_count
            )
        );
    END IF;

    IF to_regclass(format('%I.accounts', current_schema())) IS NULL THEN
        INSERT INTO migration_history_audit
        VALUES (
            '175_long_context_trigger_namespace',
            'not_applicable',
            'accounts table is absent'
        );
    ELSE
        SELECT
            COUNT(*),
            COUNT(*) FILTER (WHERE function_namespace.nspname = current_schema()),
            COUNT(*) FILTER (
                WHERE function_namespace.nspname = current_schema()
                  AND COALESCE(array_to_string(proc.proconfig, ','), '') LIKE '%search_path=%'
                  AND COALESCE(array_to_string(proc.proconfig, ','), '') LIKE '%' || current_schema() || '%'
            )
        INTO trigger_count, local_function_count, pinned_function_count
        FROM pg_trigger AS trg
        JOIN pg_proc AS proc ON proc.oid = trg.tgfoid
        JOIN pg_namespace AS function_namespace ON function_namespace.oid = proc.pronamespace
        WHERE trg.tgrelid = to_regclass(format('%I.accounts', current_schema()))
          AND NOT trg.tgisinternal
          AND trg.tgname IN (
              'accounts_enforce_openai_long_context_billing_extra',
              'accounts_propagate_openai_long_context_billing_extra'
          );

        INSERT INTO migration_history_audit
        VALUES (
            '175_long_context_trigger_namespace',
            CASE
                WHEN trigger_count = 2
                 AND local_function_count = 2
                 AND pinned_function_count = 2 THEN 'ok'
                ELSE 'needs_maintenance'
            END,
            format(
                '%s/2 triggers exist, %s/2 point to functions in schema %s, and %s/2 pin that schema in search_path',
                trigger_count,
                local_function_count,
                current_schema(),
                pinned_function_count
            )
        );
    END IF;
END $$;

TABLE migration_history_audit;

\echo
\echo 'Statuses other than ok/not_applicable require a reviewed maintenance plan.'
\echo 'Do not replay historical migrations by deleting schema_migrations rows on a live database.'
\echo 'For a 115/116 needs_maintenance result, run the explicit backend maintenance command:'
\echo '  1. Start the current release once and confirm normal migrations (including 191y) complete.'
\echo '  2. go run ./cmd/repair-custom-schema-auth-identities --schema <schema> --execute'
