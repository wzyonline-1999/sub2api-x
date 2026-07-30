-- Keep the fork's diagnostic metadata additive to the official session_id
-- column introduced by migration 187. Existing fork databases may still have
-- session_id as TEXT because the retired migration 159 created it first.
-- Fail quickly instead of queuing an ACCESS EXCLUSIVE lock behind a long-running
-- analytics query. Blue/green activation can retry the migration safely.
SET LOCAL lock_timeout = '5s';

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS session_id_source VARCHAR(32),
    ADD COLUMN IF NOT EXISTS session_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS session_explicit BOOLEAN;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'usage_logs'::regclass
          AND conname = 'chk_usage_logs_session_id_length'
    ) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT chk_usage_logs_session_id_length
            CHECK (session_id IS NULL OR char_length(session_id) <= 255)
            NOT VALID;
    END IF;
END $$;

-- The prompt_cache_key persistence constraint is intentionally deferred to
-- scripts/finalize-usage-log-session-metadata.sql. During a blue/green rollout,
-- the old slot can still persist prompt_cache_key as session_id; adding that
-- constraint here would make old-slot usage-log inserts fail as soon as the new
-- slot starts. The application and repository layers already block it in the
-- new release, and the database constraint is added only after every old slot
-- has drained.

-- The NOT VALID length constraint protects all new writes immediately without
-- rewriting or scanning the hot usage_logs table during application startup.
-- Operators can clean historical rows and validate it in a maintenance window.

-- Do not automatically ALTER the legacy TEXT column to VARCHAR(255) here:
-- changing a large hot table can require an ACCESS EXCLUSIVE lock. New
-- installations already receive VARCHAR(255) from official migration 187;
-- legacy databases are logically bounded by the constraint above and may
-- narrow the physical type separately in a planned maintenance window.
