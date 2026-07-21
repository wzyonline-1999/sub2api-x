-- Subscription quota reset cards and per-window billing generations.
ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS daily_window_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_window_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_window_version BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS subscription_reset_card_grants (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
    issued_count INTEGER NOT NULL CHECK (issued_count > 0),
    remaining_count INTEGER NOT NULL CHECK (remaining_count >= 0 AND remaining_count <= issued_count),
    expires_at TIMESTAMPTZ NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    source VARCHAR(30) NOT NULL DEFAULT 'admin_grant',
    request_id VARCHAR(64) NULL,
    issued_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    notes TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE subscription_reset_card_grants
    ADD COLUMN IF NOT EXISTS request_id VARCHAR(64) NULL;

CREATE INDEX IF NOT EXISTS idx_subscription_reset_card_grants_user
    ON subscription_reset_card_grants(user_id);
CREATE INDEX IF NOT EXISTS idx_subscription_reset_card_grants_group
    ON subscription_reset_card_grants(group_id);
CREATE INDEX IF NOT EXISTS idx_subscription_reset_card_grants_status
    ON subscription_reset_card_grants(status);
CREATE INDEX IF NOT EXISTS idx_subscription_reset_card_grants_expires
    ON subscription_reset_card_grants(expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_reset_card_grants_request
    ON subscription_reset_card_grants(request_id)
    WHERE request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_subscription_reset_card_grants_inventory
    ON subscription_reset_card_grants(user_id, group_id, status, expires_at)
    WHERE remaining_count > 0;

CREATE TABLE IF NOT EXISTS subscription_reset_card_usages (
    id BIGSERIAL PRIMARY KEY,
    grant_id BIGINT NOT NULL REFERENCES subscription_reset_card_grants(id) ON DELETE RESTRICT,
    subscription_id BIGINT NOT NULL REFERENCES user_subscriptions(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
    mode VARCHAR(10) NOT NULL CHECK (mode IN ('manual', 'auto')),
    request_id VARCHAR(128) NULL,
    previous_daily_usage_usd DECIMAL(20,10) NOT NULL DEFAULT 0,
    previous_weekly_usage_usd DECIMAL(20,10) NOT NULL DEFAULT 0,
    previous_monthly_usage_usd DECIMAL(20,10) NOT NULL DEFAULT 0,
    previous_daily_window_start TIMESTAMPTZ NULL,
    previous_weekly_window_start TIMESTAMPTZ NULL,
    previous_monthly_window_start TIMESTAMPTZ NULL,
    used_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscription_reset_card_usages_grant
    ON subscription_reset_card_usages(grant_id);
CREATE INDEX IF NOT EXISTS idx_subscription_reset_card_usages_subscription
    ON subscription_reset_card_usages(subscription_id);
CREATE INDEX IF NOT EXISTS idx_subscription_reset_card_usages_user_used
    ON subscription_reset_card_usages(user_id, used_at DESC);
CREATE INDEX IF NOT EXISTS idx_subscription_reset_card_usages_group_used
    ON subscription_reset_card_usages(group_id, used_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_reset_card_usages_request
    ON subscription_reset_card_usages(request_id)
    WHERE request_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS subscription_reset_preferences (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    auto_use_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT subscription_reset_preferences_user_group_key UNIQUE (user_id, group_id)
);
