-- Support session diagnostics without blocking the hot usage_logs table.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_session_hash_created_at
    ON usage_logs (session_hash, created_at)
    WHERE session_hash IS NOT NULL;
