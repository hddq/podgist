-- +goose Up

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE devices (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    uid TEXT NOT NULL,
    caption TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL DEFAULT 'other',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, uid)
);

CREATE TABLE subscriptions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    device_id BIGINT NOT NULL REFERENCES devices(id),
    podcast_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, device_id, podcast_url)
);

CREATE TABLE subscription_events (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    device_id BIGINT NOT NULL REFERENCES devices(id),
    podcast_url TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('subscribe', 'unsubscribe')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_subscription_events_user_device_since
    ON subscription_events(user_id, device_id, created_at);

CREATE TABLE episode_actions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    device_id BIGINT REFERENCES devices(id),
    podcast_url TEXT NOT NULL,
    episode_url TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('download', 'play', 'delete', 'new', 'flattr')),
    timestamp TIMESTAMPTZ NOT NULL,
    started INT,
    position INT,
    total INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_episode_actions_user_since
    ON episode_actions(user_id, created_at);

CREATE TABLE sync_groups (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id)
);

CREATE TABLE sync_group_members (
    device_id BIGINT NOT NULL REFERENCES devices(id),
    sync_group_id BIGINT NOT NULL REFERENCES sync_groups(id),
    PRIMARY KEY (device_id)
);

CREATE TABLE settings (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    scope_type TEXT NOT NULL CHECK (scope_type IN ('account', 'device', 'podcast', 'episode')),
    scope_id TEXT NOT NULL DEFAULT '',
    key TEXT NOT NULL,
    value JSONB NOT NULL DEFAULT '""',
    UNIQUE(user_id, scope_type, scope_id, key)
);

-- +goose Down

DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS sync_group_members;
DROP TABLE IF EXISTS sync_groups;
DROP TABLE IF EXISTS episode_actions;
DROP TABLE IF EXISTS subscription_events;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS devices;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
