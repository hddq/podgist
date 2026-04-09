package store

import (
	"context"
	"time"

	"github.com/hddq/podgist/internal/domain"
)

type DashboardSummary struct {
	SubscriptionCount  int `json:"subscription_count"`
	DeviceCount        int `json:"device_count"`
	EpisodeActionCount int `json:"episode_action_count"`
}

func (s *Store) GetDashboardSummary(ctx context.Context, userID int64) (*DashboardSummary, error) {
	var summary DashboardSummary
	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(DISTINCT podcast_url) FROM subscriptions WHERE user_id = $1),
			(SELECT count(*) FROM devices WHERE user_id = $1),
			(SELECT count(*) FROM episode_actions WHERE user_id = $1)
	`, userID).Scan(&summary.SubscriptionCount, &summary.DeviceCount, &summary.EpisodeActionCount)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (s *Store) GetRecentEpisodeActions(ctx context.Context, userID int64, limit int) ([]domain.EpisodeAction, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ea.id, ea.user_id, ea.device_id, ea.podcast_url, ea.episode_url,
		       ea.action, ea.timestamp, ea.started, ea.position, ea.total, ea.created_at
		FROM episode_actions ea
		WHERE ea.user_id = $1
		ORDER BY ea.timestamp DESC
		LIMIT $2
	`, userID, limit)
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

type AggregatedSubscription struct {
	PodcastURL string   `json:"podcast_url"`
	Devices    []string `json:"devices"`
}

func (s *Store) GetAggregatedSubscriptions(ctx context.Context, userID int64) ([]AggregatedSubscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sub.podcast_url, array_agg(d.uid ORDER BY d.uid)
		FROM subscriptions sub
		JOIN devices d ON d.id = sub.device_id
		WHERE sub.user_id = $1
		GROUP BY sub.podcast_url
		ORDER BY sub.podcast_url
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []AggregatedSubscription
	for rows.Next() {
		var s AggregatedSubscription
		if err := rows.Scan(&s.PodcastURL, &s.Devices); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

type DeviceWithSubCount struct {
	UID               string            `json:"uid"`
	Caption           string            `json:"caption"`
	Type              domain.DeviceType `json:"type"`
	SubscriptionCount int               `json:"subscription_count"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

func (s *Store) GetDevicesWithSubCount(ctx context.Context, userID int64) ([]DeviceWithSubCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.uid, d.caption, d.type,
		       (SELECT count(*) FROM subscriptions WHERE device_id = d.id),
		       d.created_at, d.updated_at
		FROM devices d
		WHERE d.user_id = $1
		ORDER BY d.uid
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []DeviceWithSubCount
	for rows.Next() {
		var d DeviceWithSubCount
		if err := rows.Scan(&d.UID, &d.Caption, &d.Type, &d.SubscriptionCount, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (s *Store) GetSessionByID(ctx context.Context, sessionID string) (*domain.Session, error) {
	var sess domain.Session
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, expires_at, created_at, updated_at
		FROM sessions WHERE id = $1
	`, sessionID).Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}
