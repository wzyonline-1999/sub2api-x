-- Add OpenAI session metadata to usage_logs for forward-looking diagnostics.
-- session_id only stores explicit client signals; content-derived fallback
-- seeds are represented by session_hash/session_id_source without raw content.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS session_id TEXT,
    ADD COLUMN IF NOT EXISTS session_id_source VARCHAR(32),
    ADD COLUMN IF NOT EXISTS session_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS session_explicit BOOLEAN;
