-- Post-rollout finalizer for the fork's usage-log session metadata.
--
-- DO NOT run this during a rolling or blue/green deployment. Run it only after
-- every application instance is on the release that never persists a raw
-- prompt_cache_key in usage_logs.session_id.
--
-- Usage:
--   psql "$DATABASE_URL" -v schema=public \
--     -f scripts/finalize-usage-log-session-metadata.sql
--
-- For a custom DATABASE_SCHEMA, pass that schema name instead of public.

\set ON_ERROR_STOP on
\if :{?schema}
\else
\set schema public
\endif

SET lock_timeout = '5s';
SET statement_timeout = '10min';

-- Add the contraction-phase guard without scanning historical rows. Existing
-- rows remain untouched; all new writes are checked immediately.
SELECT format(
    'ALTER TABLE %I.usage_logs ADD CONSTRAINT chk_usage_logs_prompt_cache_key_not_persisted CHECK (session_id_source IS DISTINCT FROM %L OR session_id IS NULL) NOT VALID',
    :'schema',
    'prompt_cache_key'
)
WHERE NOT EXISTS (
    SELECT 1
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
    WHERE n.nspname = :'schema'
      AND t.relname = 'usage_logs'
      AND c.conname = 'chk_usage_logs_prompt_cache_key_not_persisted'
)
\gexec

-- This index came from the retired fork migration 160. No repository query
-- filters or orders by session_hash, so remove it concurrently to avoid write
-- amplification on the hot usage_logs table.
DROP INDEX CONCURRENTLY IF EXISTS :"schema".idx_usage_logs_session_hash_created_at;

RESET statement_timeout;
RESET lock_timeout;

-- Optional later maintenance, after clearing any violating historical rows:
-- ALTER TABLE :"schema".usage_logs
--   VALIDATE CONSTRAINT chk_usage_logs_prompt_cache_key_not_persisted;
-- ALTER TABLE :"schema".usage_logs
--   VALIDATE CONSTRAINT chk_usage_logs_session_id_length;
