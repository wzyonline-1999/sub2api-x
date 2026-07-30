-- Repair objects that an older custom-schema runner could skip when a
-- same-named constraint existed in the public schema. All checks are scoped to the table
-- resolved by the active search_path, so this migration is safe for both the
-- default public schema and DATABASE_SCHEMA installations.
--
-- Constraints are added NOT VALID to avoid a full scan during application
-- startup. They still protect every new write. Existing rows can be validated
-- later in a maintenance window after any legacy violations are reviewed.
SET LOCAL lock_timeout = '5s';

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS request_type SMALLINT NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'usage_logs'::regclass
          AND conname = 'usage_logs_request_type_check'
    ) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_request_type_check
            CHECK (request_type >= 0 AND request_type <= 5) NOT VALID;
    END IF;
END $$;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS signup_source VARCHAR(20) NOT NULL DEFAULT 'email';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'users'::regclass
          AND conname = 'users_signup_source_check'
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT users_signup_source_check
            CHECK (signup_source IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google', 'dingtalk'))
            NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'auth_identities'::regclass
          AND conname = 'auth_identities_metadata_is_object_check'
    ) THEN
        ALTER TABLE auth_identities
            ADD CONSTRAINT auth_identities_metadata_is_object_check
            CHECK (jsonb_typeof(metadata) = 'object') NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'auth_identity_channels'::regclass
          AND conname = 'auth_identity_channels_metadata_is_object_check'
    ) THEN
        ALTER TABLE auth_identity_channels
            ADD CONSTRAINT auth_identity_channels_metadata_is_object_check
            CHECK (jsonb_typeof(metadata) = 'object') NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'auth_identity_migration_reports'::regclass
          AND conname = 'auth_identity_migration_reports_details_is_object_check'
    ) THEN
        ALTER TABLE auth_identity_migration_reports
            ADD CONSTRAINT auth_identity_migration_reports_details_is_object_check
            CHECK (jsonb_typeof(details) = 'object') NOT VALID;
    END IF;
END $$;

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS template_id BIGINT NULL,
    ADD COLUMN IF NOT EXISTS extra_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS body_override_mode VARCHAR(10) NOT NULL DEFAULT 'off',
    ADD COLUMN IF NOT EXISTS body_override JSONB NULL,
    ADD COLUMN IF NOT EXISTS api_mode VARCHAR(32) NOT NULL DEFAULT 'chat_completions';

ALTER TABLE channel_monitor_request_templates
    ADD COLUMN IF NOT EXISTS api_mode VARCHAR(32) NOT NULL DEFAULT 'chat_completions';

DO $$
DECLARE
    monitor_provider_constraint TEXT;
    template_provider_constraint TEXT;
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'channel_monitors'::regclass
          AND conname = 'channel_monitors_body_mode_check'
    ) THEN
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_body_mode_check
            CHECK (body_override_mode IN ('off', 'merge', 'replace')) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'channel_monitors'::regclass
          AND conname = 'channel_monitors_template_id_fkey'
    ) THEN
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_template_id_fkey
            FOREIGN KEY (template_id)
            REFERENCES channel_monitor_request_templates (id)
            ON DELETE SET NULL
            NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'channel_monitors'::regclass
          AND conname = 'channel_monitors_api_mode_check'
    ) THEN
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_api_mode_check
            CHECK (api_mode IN ('chat_completions', 'responses')) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'channel_monitor_request_templates'::regclass
          AND conname = 'channel_monitor_request_templates_api_mode_check'
    ) THEN
        ALTER TABLE channel_monitor_request_templates
            ADD CONSTRAINT channel_monitor_request_templates_api_mode_check
            CHECK (api_mode IN ('chat_completions', 'responses')) NOT VALID;
    END IF;

    SELECT pg_get_constraintdef(oid)
      INTO monitor_provider_constraint
      FROM pg_constraint
     WHERE conrelid = 'channel_monitors'::regclass
       AND conname = 'channel_monitors_provider_check';

    IF monitor_provider_constraint IS NULL
       OR position('grok' IN monitor_provider_constraint) = 0 THEN
        ALTER TABLE channel_monitors
            DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_provider_check
            CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok')) NOT VALID;
    END IF;

    SELECT pg_get_constraintdef(oid)
      INTO template_provider_constraint
      FROM pg_constraint
     WHERE conrelid = 'channel_monitor_request_templates'::regclass
       AND conname = 'channel_monitor_request_templates_provider_check';

    IF template_provider_constraint IS NULL
       OR position('grok' IN template_provider_constraint) = 0 THEN
        ALTER TABLE channel_monitor_request_templates
            DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;
        ALTER TABLE channel_monitor_request_templates
            ADD CONSTRAINT channel_monitor_request_templates_provider_check
            CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok')) NOT VALID;
    END IF;
END $$;
