-- +goose Up

CREATE TABLE podcasts (
    id BIGSERIAL PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    site_url TEXT NOT NULL DEFAULT '',
    image_url TEXT NOT NULL DEFAULT '',
    etag TEXT NOT NULL DEFAULT '',
    last_modified TEXT NOT NULL DEFAULT '',
    last_fetched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_podcasts_last_fetched_at ON podcasts(last_fetched_at);

CREATE TABLE podcast_episodes (
    id BIGSERIAL PRIMARY KEY,
    podcast_id BIGINT NOT NULL REFERENCES podcasts(id) ON DELETE CASCADE,
    episode_url TEXT NOT NULL,
    guid TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    published_at TIMESTAMPTZ,
    duration_seconds INT,
    mime_type TEXT NOT NULL DEFAULT '',
    byte_size BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (podcast_id, episode_url)
);

CREATE INDEX idx_podcast_episodes_podcast_id ON podcast_episodes(podcast_id);
CREATE INDEX idx_podcast_episodes_episode_url ON podcast_episodes(episode_url);

-- +goose Down

DROP TABLE IF EXISTS podcast_episodes;
DROP TABLE IF EXISTS podcasts;
