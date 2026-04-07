package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hddq/podgist/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// --- Users ---

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	u := &domain.User{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, created_at FROM users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string) (*domain.User, error) {
	u := &domain.User{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id, username, password_hash, created_at`,
		username, passwordHash,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// --- Sessions ---

func (s *Store) CreateSession(ctx context.Context, sessionID string, userID int64, expiresAt time.Time) (*domain.Session, error) {
	session := &domain.Session{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, $3)
		 RETURNING id, user_id, expires_at, created_at, updated_at`,
		sessionID, userID, expiresAt,
	).Scan(&session.ID, &session.UserID, &session.ExpiresAt, &session.CreatedAt, &session.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *Store) GetUserBySessionID(ctx context.Context, sessionID string, now time.Time) (*domain.User, error) {
	u := &domain.User{}
	err := s.pool.QueryRow(ctx,
		`SELECT u.id, u.username, u.password_hash, u.created_at
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.id = $1 AND s.expires_at > $2`,
		sessionID, now,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= $1`, now)
	return err
}

func (s *Store) TouchSession(ctx context.Context, sessionID string, expiresAt time.Time) error {
	cmd, err := s.pool.Exec(ctx,
		`UPDATE sessions SET expires_at = $2, updated_at = now() WHERE id = $1`,
		sessionID, expiresAt,
	)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// --- Devices ---

func (s *Store) GetDevice(ctx context.Context, userID int64, uid string) (*domain.Device, error) {
	d := &domain.Device{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, uid, caption, type, created_at, updated_at FROM devices WHERE user_id = $1 AND uid = $2`,
		userID, uid,
	).Scan(&d.ID, &d.UserID, &d.UID, &d.Caption, &d.Type, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Store) GetOrCreateDevice(ctx context.Context, userID int64, uid string) (*domain.Device, error) {
	d, err := s.GetDevice(ctx, userID, uid)
	if err == nil {
		return d, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	d = &domain.Device{}
	err = s.pool.QueryRow(ctx,
		`INSERT INTO devices (user_id, uid) VALUES ($1, $2)
		 ON CONFLICT (user_id, uid) DO UPDATE SET updated_at = now()
		 RETURNING id, user_id, uid, caption, type, created_at, updated_at`,
		userID, uid,
	).Scan(&d.ID, &d.UserID, &d.UID, &d.Caption, &d.Type, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Store) UpdateDevice(ctx context.Context, userID int64, uid, caption string, deviceType domain.DeviceType) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE devices SET caption = $3, type = $4, updated_at = now() WHERE user_id = $1 AND uid = $2`,
		userID, uid, caption, string(deviceType),
	)
	return err
}

func (s *Store) ListDevices(ctx context.Context, userID int64) ([]domain.Device, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, uid, caption, type, created_at, updated_at FROM devices WHERE user_id = $1 ORDER BY uid`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []domain.Device
	for rows.Next() {
		var d domain.Device
		if err := rows.Scan(&d.ID, &d.UserID, &d.UID, &d.Caption, &d.Type, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (s *Store) CountSubscriptions(ctx context.Context, userID, deviceID int64) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM subscriptions WHERE user_id = $1 AND device_id = $2`,
		userID, deviceID,
	).Scan(&count)
	return count, err
}

func (s *Store) GetDeviceByID(ctx context.Context, id int64) (*domain.Device, error) {
	d := &domain.Device{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, uid, caption, type, created_at, updated_at FROM devices WHERE id = $1`, id,
	).Scan(&d.ID, &d.UserID, &d.UID, &d.Caption, &d.Type, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// --- Subscriptions ---

func (s *Store) AddSubscription(ctx context.Context, userID, deviceID int64, podcastURL string, now time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO subscriptions (user_id, device_id, podcast_url) VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, device_id, podcast_url) DO NOTHING`,
		userID, deviceID, podcastURL,
	)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO subscription_events (user_id, device_id, podcast_url, action, created_at) VALUES ($1, $2, $3, 'subscribe', $4)`,
		userID, deviceID, podcastURL, now,
	)
	return err
}

func (s *Store) RemoveSubscription(ctx context.Context, userID, deviceID int64, podcastURL string, now time.Time) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM subscriptions WHERE user_id = $1 AND device_id = $2 AND podcast_url = $3`,
		userID, deviceID, podcastURL,
	)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO subscription_events (user_id, device_id, podcast_url, action, created_at) VALUES ($1, $2, $3, 'unsubscribe', $4)`,
		userID, deviceID, podcastURL, now,
	)
	return err
}

func (s *Store) GetSubscriptionsSince(ctx context.Context, userID, deviceID int64, since time.Time) (add, remove []string, err error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT ON (podcast_url) podcast_url, action
		 FROM subscription_events
		 WHERE user_id = $1 AND device_id = $2 AND created_at >= $3
		 ORDER BY podcast_url, created_at DESC`,
		userID, deviceID, since,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var url, action string
		if err := rows.Scan(&url, &action); err != nil {
			return nil, nil, err
		}
		if action == "subscribe" {
			add = append(add, url)
		} else {
			remove = append(remove, url)
		}
	}
	return add, remove, rows.Err()
}

func (s *Store) GetCurrentSubscriptions(ctx context.Context, userID, deviceID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT podcast_url FROM subscriptions WHERE user_id = $1 AND device_id = $2 ORDER BY podcast_url`,
		userID, deviceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		urls = append(urls, url)
	}
	return urls, rows.Err()
}

// --- Episode Actions ---

func (s *Store) AddEpisodeAction(ctx context.Context, ea *domain.EpisodeAction) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO episode_actions (user_id, device_id, podcast_url, episode_url, action, timestamp, started, position, total, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		ea.UserID, ea.DeviceID, ea.PodcastURL, ea.EpisodeURL, string(ea.Action),
		ea.Timestamp, ea.Started, ea.Position, ea.Total, ea.CreatedAt,
	)
	return err
}

func (s *Store) GetEpisodeActionsSince(ctx context.Context, userID int64, since *time.Time, podcastURL *string, deviceID *int64, limit int) ([]domain.EpisodeAction, error) {
	query := `SELECT id, user_id, device_id, podcast_url, episode_url, action, timestamp, started, position, total, created_at
		FROM episode_actions WHERE user_id = $1`
	args := []any{userID}
	argN := 2

	if since != nil {
		query += fmt.Sprintf(` AND created_at >= $%d`, argN)
		args = append(args, *since)
		argN++
	}
	if podcastURL != nil {
		query += fmt.Sprintf(` AND podcast_url = $%d`, argN)
		args = append(args, *podcastURL)
		argN++
	}
	if deviceID != nil {
		query += fmt.Sprintf(` AND device_id = $%d`, argN)
		args = append(args, *deviceID)
		argN++
	}

	query += ` ORDER BY created_at ASC`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT $%d`, argN)
		args = append(args, limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []domain.EpisodeAction
	for rows.Next() {
		var a domain.EpisodeAction
		if err := rows.Scan(&a.ID, &a.UserID, &a.DeviceID, &a.PodcastURL, &a.EpisodeURL,
			&a.Action, &a.Timestamp, &a.Started, &a.Position, &a.Total, &a.CreatedAt); err != nil {
			return nil, err
		}
		actions = append(actions, a)
	}
	return actions, rows.Err()
}

// --- Sync Groups ---

func (s *Store) GetSyncStatus(ctx context.Context, userID int64) (synced [][]string, unsynced []string, err error) {
	// Get all devices
	devices, err := s.ListDevices(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	// Get sync group memberships
	rows, err := s.pool.Query(ctx,
		`SELECT d.uid, sgm.sync_group_id
		 FROM sync_group_members sgm
		 JOIN devices d ON d.id = sgm.device_id
		 WHERE d.user_id = $1
		 ORDER BY sgm.sync_group_id, d.uid`,
		userID,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	groupMap := make(map[int64][]string)
	syncedDevices := make(map[string]bool)
	for rows.Next() {
		var uid string
		var groupID int64
		if err := rows.Scan(&uid, &groupID); err != nil {
			return nil, nil, err
		}
		groupMap[groupID] = append(groupMap[groupID], uid)
		syncedDevices[uid] = true
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	for _, uids := range groupMap {
		synced = append(synced, uids)
	}

	for _, d := range devices {
		if !syncedDevices[d.UID] {
			unsynced = append(unsynced, d.UID)
		}
	}

	return synced, unsynced, nil
}

func (s *Store) SetSyncGroup(ctx context.Context, userID int64, deviceUIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Get device IDs
	var deviceIDs []int64
	for _, uid := range deviceUIDs {
		var id int64
		err := tx.QueryRow(ctx,
			`SELECT id FROM devices WHERE user_id = $1 AND uid = $2`, userID, uid,
		).Scan(&id)
		if err != nil {
			return fmt.Errorf("device %q not found: %w", uid, err)
		}
		deviceIDs = append(deviceIDs, id)
	}

	// Remove existing sync group memberships for these devices
	for _, id := range deviceIDs {
		_, err := tx.Exec(ctx, `DELETE FROM sync_group_members WHERE device_id = $1`, id)
		if err != nil {
			return err
		}
	}

	// Clean up empty sync groups
	_, err = tx.Exec(ctx,
		`DELETE FROM sync_groups WHERE user_id = $1 AND id NOT IN (SELECT sync_group_id FROM sync_group_members)`,
		userID,
	)
	if err != nil {
		return err
	}

	// Create new sync group
	var groupID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO sync_groups (user_id) VALUES ($1) RETURNING id`, userID,
	).Scan(&groupID)
	if err != nil {
		return err
	}

	// Add all devices to the group
	for _, id := range deviceIDs {
		_, err := tx.Exec(ctx,
			`INSERT INTO sync_group_members (device_id, sync_group_id) VALUES ($1, $2)`, id, groupID,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *Store) StopSync(ctx context.Context, userID int64, uid string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var deviceID int64
	err = tx.QueryRow(ctx,
		`SELECT id FROM devices WHERE user_id = $1 AND uid = $2`, userID, uid,
	).Scan(&deviceID)
	if err != nil {
		return fmt.Errorf("device %q not found: %w", uid, err)
	}

	_, err = tx.Exec(ctx, `DELETE FROM sync_group_members WHERE device_id = $1`, deviceID)
	if err != nil {
		return err
	}

	// Clean up empty sync groups
	_, err = tx.Exec(ctx,
		`DELETE FROM sync_groups WHERE user_id = $1 AND id NOT IN (SELECT sync_group_id FROM sync_group_members)`,
		userID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// --- Settings ---

func (s *Store) GetSettings(ctx context.Context, userID int64, scopeType domain.ScopeType, scopeID string) (map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT key, value FROM settings WHERE user_id = $1 AND scope_type = $2 AND scope_id = $3`,
		userID, string(scopeType), scopeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]any)
	for rows.Next() {
		var key string
		var valueJSON []byte
		if err := rows.Scan(&key, &valueJSON); err != nil {
			return nil, err
		}
		var value any
		if err := json.Unmarshal(valueJSON, &value); err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, rows.Err()
}

func (s *Store) SetSetting(ctx context.Context, userID int64, scopeType domain.ScopeType, scopeID, key string, value any) error {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO settings (user_id, scope_type, scope_id, key, value) VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (user_id, scope_type, scope_id, key) DO UPDATE SET value = $5`,
		userID, string(scopeType), scopeID, key, valueJSON,
	)
	return err
}

func (s *Store) DeleteSetting(ctx context.Context, userID int64, scopeType domain.ScopeType, scopeID, key string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM settings WHERE user_id = $1 AND scope_type = $2 AND scope_id = $3 AND key = $4`,
		userID, string(scopeType), scopeID, key,
	)
	return err
}

// --- Updates ---

func (s *Store) GetSubscriptionEventsSince(ctx context.Context, userID, deviceID int64, since time.Time) ([]domain.SubscriptionEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, device_id, podcast_url, action, created_at
		 FROM subscription_events
		 WHERE user_id = $1 AND device_id = $2 AND created_at >= $3
		 ORDER BY created_at ASC`,
		userID, deviceID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.SubscriptionEvent
	for rows.Next() {
		var e domain.SubscriptionEvent
		if err := rows.Scan(&e.ID, &e.UserID, &e.DeviceID, &e.PodcastURL, &e.Action, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// WithTx runs fn inside a transaction.
func (s *Store) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
