-- name: GetUserByUsername :one
SELECT id, username, password_hash, created_at FROM users WHERE username = $1;

-- name: CreateUser :one
INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id, username, password_hash, created_at;

-- name: CreateSession :one
INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, $3)
RETURNING id, user_id, expires_at, created_at, updated_at;

-- name: GetUserBySessionID :one
SELECT u.id, u.username, u.password_hash, u.created_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.id = $1 AND s.expires_at > $2;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= $1;

-- name: TouchSession :exec
UPDATE sessions SET expires_at = $2, updated_at = now() WHERE id = $1;

-- name: GetDevice :one
SELECT id, user_id, uid, caption, type, created_at, updated_at FROM devices WHERE user_id = $1 AND uid = $2;

-- name: GetDeviceByID :one
SELECT id, user_id, uid, caption, type, created_at, updated_at FROM devices WHERE id = $1;

-- name: UpsertDevice :one
INSERT INTO devices (user_id, uid) VALUES ($1, $2)
ON CONFLICT (user_id, uid) DO UPDATE SET updated_at = now()
RETURNING id, user_id, uid, caption, type, created_at, updated_at;

-- name: UpdateDevice :exec
UPDATE devices SET caption = $3, type = $4, updated_at = now() WHERE user_id = $1 AND uid = $2;

-- name: ListDevices :many
SELECT id, user_id, uid, caption, type, created_at, updated_at FROM devices WHERE user_id = $1 ORDER BY uid;

-- name: CountSubscriptions :one
SELECT COUNT(*) FROM subscriptions WHERE user_id = $1 AND device_id = $2;

-- name: InsertSubscription :exec
INSERT INTO subscriptions (user_id, device_id, podcast_url) VALUES ($1, $2, $3)
ON CONFLICT (user_id, device_id, podcast_url) DO NOTHING;

-- name: InsertSubscriptionEvent :exec
INSERT INTO subscription_events (user_id, device_id, podcast_url, action, created_at) VALUES ($1, $2, $3, $4, $5);

-- name: DeleteSubscription :exec
DELETE FROM subscriptions WHERE user_id = $1 AND device_id = $2 AND podcast_url = $3;

-- name: GetSubscriptionsSince :many
SELECT DISTINCT ON (podcast_url) podcast_url, action
FROM subscription_events
WHERE user_id = $1 AND device_id = $2 AND created_at >= $3
ORDER BY podcast_url, created_at DESC;

-- name: GetCurrentSubscriptions :many
SELECT podcast_url FROM subscriptions WHERE user_id = $1 AND device_id = $2 ORDER BY podcast_url;

-- name: InsertEpisodeAction :exec
INSERT INTO episode_actions (user_id, device_id, podcast_url, episode_url, action, timestamp, started, position, total, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: GetEpisodeActionsSince :many
SELECT id, user_id, device_id, podcast_url, episode_url, action, timestamp, started, position, total, created_at
FROM episode_actions 
WHERE user_id = $1 
  AND (sqlc.narg('created_after')::timestamptz IS NULL OR created_at >= sqlc.narg('created_after')::timestamptz)
  AND (sqlc.narg('podcast_url_filter')::text IS NULL OR podcast_url = sqlc.narg('podcast_url_filter')::text)
  AND (sqlc.narg('device_id_filter')::bigint IS NULL OR device_id = sqlc.narg('device_id_filter')::bigint)
ORDER BY created_at ASC
LIMIT sqlc.narg('limit_val')::int;

-- name: GetDeviceSyncGroupMemberships :many
SELECT d.uid, sgm.sync_group_id
FROM sync_group_members sgm
JOIN devices d ON d.id = sgm.device_id
WHERE d.user_id = $1
ORDER BY sgm.sync_group_id, d.uid;

-- name: GetDeviceIDByUID :one
SELECT id FROM devices WHERE user_id = $1 AND uid = $2;

-- name: DeleteDeviceFromSyncGroups :exec
DELETE FROM sync_group_members WHERE device_id = $1;

-- name: CreateSyncGroup :one
INSERT INTO sync_groups (user_id) VALUES ($1) RETURNING id;

-- name: InsertSyncGroupMember :exec
INSERT INTO sync_group_members (device_id, sync_group_id) VALUES ($1, $2);

-- name: GetSyncGroupSubscriptions :many
SELECT DISTINCT podcast_url
FROM subscriptions
WHERE user_id = $1 AND device_id = ANY(sqlc.arg('device_ids')::bigint[])
ORDER BY podcast_url;

-- name: FindSmallSyncGroups :many
SELECT sg.id
FROM sync_groups sg
LEFT JOIN sync_group_members sgm ON sgm.sync_group_id = sg.id
WHERE sg.user_id = $1
GROUP BY sg.id
HAVING COUNT(sgm.device_id) < 2;

-- name: DeleteSyncGroupMembersByGroupIDs :exec
DELETE FROM sync_group_members WHERE sync_group_id = ANY(sqlc.arg('group_ids')::bigint[]);

-- name: DeleteSyncGroupsByIDs :exec
DELETE FROM sync_groups WHERE user_id = $1 AND id = ANY(sqlc.arg('group_ids')::bigint[]);

-- name: GetSyncTargetDeviceIDs :many
SELECT peer.id
FROM devices base
JOIN sync_group_members base_member ON base_member.device_id = base.id
JOIN sync_group_members peer_member ON peer_member.sync_group_id = base_member.sync_group_id
JOIN devices peer ON peer.id = peer_member.device_id
WHERE base.user_id = $1 AND base.id = $2
ORDER BY peer.id;

-- name: GetSettings :many
SELECT key, value FROM settings WHERE user_id = $1 AND scope_type = $2 AND scope_id = $3;

-- name: SetSetting :exec
INSERT INTO settings (user_id, scope_type, scope_id, key, value) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, scope_type, scope_id, key) DO UPDATE SET value = EXCLUDED.value;

-- name: DeleteSetting :exec
DELETE FROM settings WHERE user_id = $1 AND scope_type = $2 AND scope_id = $3 AND key = $4;

-- name: GetSubscriptionEventsSince :many
SELECT id, user_id, device_id, podcast_url, action, created_at
FROM subscription_events
WHERE user_id = $1 AND device_id = $2 AND created_at >= $3
ORDER BY created_at ASC;

-- name: GetDashboardSummary :one
SELECT
    (SELECT count(DISTINCT podcast_url) FROM subscriptions WHERE subscriptions.user_id = $1) AS subscription_count,
    (SELECT count(*) FROM devices WHERE devices.user_id = $1) AS device_count,
    (SELECT count(*) FROM episode_actions WHERE episode_actions.user_id = $1) AS episode_action_count;

-- name: GetRecentEpisodeActions :many
SELECT ea.id, ea.user_id, ea.device_id, ea.podcast_url, COALESCE(p.title, '') AS podcast_title,
       ea.episode_url, COALESCE(pe.title, '') AS episode_title,
       ea.action, ea.timestamp, ea.started, ea.position, ea.total, ea.created_at
FROM episode_actions ea
LEFT JOIN podcasts p ON p.url = ea.podcast_url
LEFT JOIN podcast_episodes pe ON pe.podcast_id = p.id AND pe.episode_url = ea.episode_url
WHERE ea.user_id = $1
ORDER BY ea.timestamp DESC
LIMIT $2;

-- name: GetPlaybackHistory :many
WITH ranked AS (
    SELECT
        ea.podcast_url,
        ea.episode_url,
        ea.device_id,
        ea.timestamp,
        ea.position,
        ea.total,
        ea.created_at,
        ea.id,
        row_number() OVER (
            PARTITION BY ea.podcast_url, ea.episode_url
            ORDER BY ea.timestamp DESC, ea.created_at DESC, ea.id DESC
        ) AS rn
    FROM episode_actions ea
    WHERE ea.user_id = $1
           AND ea.action = 'play'
)
SELECT ranked.podcast_url,
       COALESCE(p.title, '') AS podcast_title,
       ranked.episode_url,
       COALESCE(pe.title, '') AS episode_title,
       ranked.device_id,
       ranked.timestamp,
       ranked.position,
       ranked.total
FROM ranked
LEFT JOIN podcasts p ON p.url = ranked.podcast_url
LEFT JOIN podcast_episodes pe ON pe.podcast_id = p.id AND pe.episode_url = ranked.episode_url
WHERE rn = 1
ORDER BY ranked.timestamp DESC, ranked.created_at DESC, ranked.id DESC
LIMIT $2;

-- name: GetAggregatedSubscriptions :many
SELECT sub.podcast_url, COALESCE(p.title, '') AS podcast_title, array_agg(d.uid ORDER BY d.uid)::text[] AS devices
FROM subscriptions sub
JOIN devices d ON d.id = sub.device_id
LEFT JOIN podcasts p ON p.url = sub.podcast_url
WHERE sub.user_id = $1
GROUP BY sub.podcast_url, p.title
ORDER BY sub.podcast_url;

-- name: GetDevicesWithSubCount :many
SELECT d.uid, d.caption, d.type,
       (SELECT count(*) FROM subscriptions WHERE device_id = d.id) AS subscription_count,
       d.created_at, d.updated_at
FROM devices d
WHERE d.user_id = $1
ORDER BY d.uid;

-- name: GetSessionByID :one
SELECT id, user_id, expires_at, created_at, updated_at
FROM sessions WHERE id = $1;

-- name: GetPodcastByURL :one
SELECT id, url, title, description, author, site_url, image_url,
       etag, last_modified, last_fetched_at, created_at, updated_at
FROM podcasts
WHERE url = $1;

-- name: PodcastEpisodeExists :one
SELECT EXISTS (
    SELECT 1
    FROM podcast_episodes pe
    JOIN podcasts p ON p.id = pe.podcast_id
    WHERE p.url = $1 AND pe.episode_url = $2
);

-- name: UpsertPodcastMetadata :one
INSERT INTO podcasts (
    url, title, description, author, site_url, image_url,
    etag, last_modified, last_fetched_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (url) DO UPDATE SET
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    author = EXCLUDED.author,
    site_url = EXCLUDED.site_url,
    image_url = EXCLUDED.image_url,
    etag = EXCLUDED.etag,
    last_modified = EXCLUDED.last_modified,
    last_fetched_at = EXCLUDED.last_fetched_at,
    updated_at = now()
RETURNING id;

-- name: UpdatePodcastFetchState :exec
UPDATE podcasts
SET etag = $2,
    last_modified = $3,
    last_fetched_at = $4,
    updated_at = now()
WHERE url = $1;

-- name: UpsertPodcastEpisode :exec
INSERT INTO podcast_episodes (
    podcast_id, episode_url, guid, title, description,
    published_at, duration_seconds, mime_type, byte_size
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (podcast_id, episode_url) DO UPDATE SET
    guid = EXCLUDED.guid,
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    published_at = EXCLUDED.published_at,
    duration_seconds = EXCLUDED.duration_seconds,
    mime_type = EXCLUDED.mime_type,
    byte_size = EXCLUDED.byte_size,
    updated_at = now();
