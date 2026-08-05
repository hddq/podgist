package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hddq/podgist/internal/domain"
)

type Store struct {
	db *sql.DB
	q  *Queries
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		db: db,
		q:  New(db),
	}
}

func (s *Store) DB() *sql.DB {
	return s.db
}

// --- Users ---

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	u, err := s.q.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	return &domain.User{
		ID:           u.ID,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt,
	}, nil
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string) (*domain.User, error) {
	u, err := s.q.CreateUser(ctx, CreateUserParams{
		Username:     username,
		PasswordHash: passwordHash,
	})
	if err != nil {
		return nil, err
	}
	return &domain.User{
		ID:           u.ID,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt,
	}, nil
}

// --- Sessions ---

func (s *Store) CreateSession(ctx context.Context, sessionID string, userID int64, expiresAt time.Time) (*domain.Session, error) {
	sess, err := s.q.CreateSession(ctx, CreateSessionParams{
		ID:        sessionID,
		UserID:    userID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, err
	}
	return &domain.Session{
		ID:        sess.ID,
		UserID:    sess.UserID,
		ExpiresAt: sess.ExpiresAt,
		CreatedAt: sess.CreatedAt,
		UpdatedAt: sess.UpdatedAt,
	}, nil
}

func (s *Store) GetUserBySessionID(ctx context.Context, sessionID string, now time.Time) (*domain.User, error) {
	u, err := s.q.GetUserBySessionID(ctx, GetUserBySessionIDParams{
		ID:        sessionID,
		ExpiresAt: now,
	})
	if err != nil {
		return nil, err
	}
	return &domain.User{
		ID:           u.ID,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt,
	}, nil
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	return s.q.DeleteSession(ctx, sessionID)
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	return s.q.DeleteExpiredSessions(ctx, now)
}

func (s *Store) TouchSession(ctx context.Context, sessionID string, expiresAt time.Time) error {
	return s.q.TouchSession(ctx, TouchSessionParams{
		ExpiresAt: expiresAt,
		ID:        sessionID,
	})
}

// --- Devices ---

func (s *Store) GetDevice(ctx context.Context, userID int64, uid string) (*domain.Device, error) {
	d, err := s.q.GetDevice(ctx, GetDeviceParams{
		UserID: userID,
		Uid:    uid,
	})
	if err != nil {
		return nil, err
	}
	return &domain.Device{
		ID:        d.ID,
		UserID:    d.UserID,
		UID:       d.Uid,
		Caption:   d.Caption,
		Type:      domain.DeviceType(d.Type),
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}, nil
}

func (s *Store) GetOrCreateDevice(ctx context.Context, userID int64, uid string) (*domain.Device, error) {
	d, err := s.GetDevice(ctx, userID, uid)
	if err == nil {
		return d, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	created, err := s.q.UpsertDevice(ctx, UpsertDeviceParams{
		UserID: userID,
		Uid:    uid,
	})
	if err != nil {
		return nil, err
	}
	return &domain.Device{
		ID:        created.ID,
		UserID:    created.UserID,
		UID:       created.Uid,
		Caption:   created.Caption,
		Type:      domain.DeviceType(created.Type),
		CreatedAt: created.CreatedAt,
		UpdatedAt: created.UpdatedAt,
	}, nil
}

func (s *Store) UpdateDevice(ctx context.Context, userID int64, uid, caption string, deviceType domain.DeviceType) error {
	return s.q.UpdateDevice(ctx, UpdateDeviceParams{
		Caption: caption,
		Type:    string(deviceType),
		UserID:  userID,
		Uid:     uid,
	})
}

func (s *Store) ListDevices(ctx context.Context, userID int64) ([]domain.Device, error) {
	rows, err := s.q.ListDevices(ctx, userID)
	if err != nil {
		return nil, err
	}
	var devices []domain.Device
	for _, d := range rows {
		devices = append(devices, domain.Device{
			ID:        d.ID,
			UserID:    d.UserID,
			UID:       d.Uid,
			Caption:   d.Caption,
			Type:      domain.DeviceType(d.Type),
			CreatedAt: d.CreatedAt,
			UpdatedAt: d.UpdatedAt,
		})
	}
	return devices, nil
}

func (s *Store) CountSubscriptions(ctx context.Context, userID, deviceID int64) (int, error) {
	count, err := s.q.CountSubscriptions(ctx, CountSubscriptionsParams{
		UserID:   userID,
		DeviceID: deviceID,
	})
	return int(count), err
}

func (s *Store) GetDeviceByID(ctx context.Context, id int64) (*domain.Device, error) {
	d, err := s.q.GetDeviceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &domain.Device{
		ID:        d.ID,
		UserID:    d.UserID,
		UID:       d.Uid,
		Caption:   d.Caption,
		Type:      domain.DeviceType(d.Type),
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}, nil
}

// --- Subscriptions ---

func (s *Store) AddSubscription(ctx context.Context, userID, deviceID int64, podcastURL string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := s.q.WithTx(tx)

	deviceIDs, err := s.syncTargetDeviceIDsTx(ctx, qtx, userID, deviceID)
	if err != nil {
		return err
	}

	for _, targetID := range deviceIDs {
		err := qtx.InsertSubscription(ctx, InsertSubscriptionParams{
			UserID:     userID,
			DeviceID:   targetID,
			PodcastUrl: podcastURL,
		})
		if err != nil {
			return err
		}

		err = qtx.InsertSubscriptionEvent(ctx, InsertSubscriptionEventParams{
			UserID:     userID,
			DeviceID:   targetID,
			PodcastUrl: podcastURL,
			Action:     "subscribe",
			CreatedAt:  now,
		})
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) RemoveSubscription(ctx context.Context, userID, deviceID int64, podcastURL string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := s.q.WithTx(tx)

	deviceIDs, err := s.syncTargetDeviceIDsTx(ctx, qtx, userID, deviceID)
	if err != nil {
		return err
	}

	for _, targetID := range deviceIDs {
		err := qtx.DeleteSubscription(ctx, DeleteSubscriptionParams{
			UserID:     userID,
			DeviceID:   targetID,
			PodcastUrl: podcastURL,
		})
		if err != nil {
			return err
		}

		err = qtx.InsertSubscriptionEvent(ctx, InsertSubscriptionEventParams{
			UserID:     userID,
			DeviceID:   targetID,
			PodcastUrl: podcastURL,
			Action:     "unsubscribe",
			CreatedAt:  now,
		})
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetSubscriptionsSince(ctx context.Context, userID, deviceID int64, since time.Time) (add, remove []string, err error) {
	rows, err := s.q.GetSubscriptionsSince(ctx, GetSubscriptionsSinceParams{
		UserID:    userID,
		DeviceID:  deviceID,
		CreatedAt: since,
	})
	if err != nil {
		return nil, nil, err
	}

	for _, r := range rows {
		if r.Action == "subscribe" {
			add = append(add, r.PodcastUrl)
		} else {
			remove = append(remove, r.PodcastUrl)
		}
	}
	return add, remove, nil
}

func (s *Store) GetCurrentSubscriptions(ctx context.Context, userID, deviceID int64) ([]string, error) {
	return s.q.GetCurrentSubscriptions(ctx, GetCurrentSubscriptionsParams{
		UserID:   userID,
		DeviceID: deviceID,
	})
}

func intPtrToInt64Ptr(p *int) *int64 {
	if p != nil {
		v := int64(*p)
		return &v
	}
	return nil
}

func int64PtrToIntPtr(p *int64) *int {
	if p != nil {
		v := int(*p)
		return &v
	}
	return nil
}

// --- Episode Actions ---

func (s *Store) AddEpisodeAction(ctx context.Context, ea *domain.EpisodeAction) error {
	return s.q.InsertEpisodeAction(ctx, InsertEpisodeActionParams{
		UserID:     ea.UserID,
		DeviceID:   ea.DeviceID,
		PodcastUrl: ea.PodcastURL,
		EpisodeUrl: ea.EpisodeURL,
		Action:     string(ea.Action),
		Timestamp:  ea.Timestamp,
		Started:    intPtrToInt64Ptr(ea.Started),
		Position:   intPtrToInt64Ptr(ea.Position),
		Total:      intPtrToInt64Ptr(ea.Total),
		CreatedAt:  ea.CreatedAt,
	})
}

func (s *Store) GetEpisodeActionsSince(ctx context.Context, userID int64, since *time.Time, podcastURL *string, deviceID *int64, limit int) ([]domain.EpisodeAction, error) {
	var sinceVal any
	if since != nil {
		sinceVal = *since
	}
	var podcastURLVal any
	if podcastURL != nil {
		podcastURLVal = *podcastURL
	}
	var deviceIDVal any
	if deviceID != nil {
		deviceIDVal = *deviceID
	}
	var limitVal *int64
	if limit > 0 {
		l := int64(limit)
		limitVal = &l
	}
	params := GetEpisodeActionsSinceParams{
		UserID:           userID,
		CreatedAfter:     sinceVal,
		PodcastUrlFilter: podcastURLVal,
		DeviceIDFilter:   deviceIDVal,
		LimitVal:         limitVal,
	}
	rows, err := s.q.GetEpisodeActionsSince(ctx, params)
	if err != nil {
		return nil, err
	}
	var actions []domain.EpisodeAction
	for _, r := range rows {
		actions = append(actions, domain.EpisodeAction{
			ID:         r.ID,
			UserID:     r.UserID,
			DeviceID:   r.DeviceID,
			PodcastURL: r.PodcastUrl,
			EpisodeURL: r.EpisodeUrl,
			Action:     domain.EpisodeActionType(r.Action),
			Timestamp:  r.Timestamp,
			Started:    int64PtrToIntPtr(r.Started),
			Position:   int64PtrToIntPtr(r.Position),
			Total:      int64PtrToIntPtr(r.Total),
			CreatedAt:  r.CreatedAt,
		})
	}
	return actions, nil
}

// --- Sync Groups ---

func (s *Store) GetSyncStatus(ctx context.Context, userID int64) (synced [][]string, unsynced []string, err error) {
	devices, err := s.ListDevices(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	rows, err := s.q.GetDeviceSyncGroupMemberships(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	groupMap := make(map[int64][]string)
	groupOrder := make([]int64, 0)
	for _, r := range rows {
		groupID := r.SyncGroupID
		uid := r.Uid
		if _, ok := groupMap[groupID]; !ok {
			groupOrder = append(groupOrder, groupID)
		}
		groupMap[groupID] = append(groupMap[groupID], uid)
	}

	syncedDevices := make(map[string]bool)
	for _, groupID := range groupOrder {
		uids := groupMap[groupID]
		if len(uids) < 2 {
			continue
		}
		synced = append(synced, uids)
		for _, uid := range uids {
			syncedDevices[uid] = true
		}
	}

	for _, d := range devices {
		if !syncedDevices[d.UID] {
			unsynced = append(unsynced, d.UID)
		}
	}

	return synced, unsynced, nil
}

func (s *Store) SetSyncGroup(ctx context.Context, userID int64, deviceUIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := s.q.WithTx(tx)

	var deviceIDs []int64
	for _, uid := range deviceUIDs {
		id, err := qtx.GetDeviceIDByUID(ctx, GetDeviceIDByUIDParams{
			UserID: userID,
			Uid:    uid,
		})
		if err != nil {
			return fmt.Errorf("device %q not found: %w", uid, err)
		}
		deviceIDs = append(deviceIDs, id)
	}

	for _, id := range deviceIDs {
		if err := qtx.DeleteDeviceFromSyncGroups(ctx, id); err != nil {
			return err
		}
	}

	if err := s.cleanupSmallSyncGroupsTx(ctx, qtx, userID); err != nil {
		return err
	}

	groupID, err := qtx.CreateSyncGroup(ctx, userID)
	if err != nil {
		return err
	}

	for _, id := range deviceIDs {
		if err := qtx.InsertSyncGroupMember(ctx, InsertSyncGroupMemberParams{
			DeviceID:    id,
			SyncGroupID: groupID,
		}); err != nil {
			return err
		}
	}

	merged, err := qtx.GetSyncGroupSubscriptions(ctx, GetSyncGroupSubscriptionsParams{
		UserID:    userID,
		DeviceIds: deviceIDs,
	})
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, id := range deviceIDs {
		for _, podcastURL := range merged {
			err = qtx.InsertSubscription(ctx, InsertSubscriptionParams{
				UserID:     userID,
				DeviceID:   id,
				PodcastUrl: podcastURL,
			})
			if err != nil {
				return err
			}
			err = qtx.InsertSubscriptionEvent(ctx, InsertSubscriptionEventParams{
				UserID:     userID,
				DeviceID:   id,
				PodcastUrl: podcastURL,
				Action:     "subscribe",
				CreatedAt:  now,
			})
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (s *Store) StopSync(ctx context.Context, userID int64, uid string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := s.q.WithTx(tx)

	deviceID, err := qtx.GetDeviceIDByUID(ctx, GetDeviceIDByUIDParams{
		UserID: userID,
		Uid:    uid,
	})
	if err != nil {
		return fmt.Errorf("device %q not found: %w", uid, err)
	}

	if err := qtx.DeleteDeviceFromSyncGroups(ctx, deviceID); err != nil {
		return err
	}

	if err := s.cleanupSmallSyncGroupsTx(ctx, qtx, userID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) syncTargetDeviceIDsTx(ctx context.Context, qtx *Queries, userID, deviceID int64) ([]int64, error) {
	ids, err := qtx.GetSyncTargetDeviceIDs(ctx, GetSyncTargetDeviceIDsParams{
		UserID: userID,
		ID:     deviceID,
	})
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []int64{deviceID}, nil
	}
	return ids, nil
}

func (s *Store) cleanupSmallSyncGroupsTx(ctx context.Context, qtx *Queries, userID int64) error {
	groupIDs, err := qtx.FindSmallSyncGroups(ctx, userID)
	if err != nil {
		return err
	}
	if len(groupIDs) > 0 {
		if err := qtx.DeleteSyncGroupMembersByGroupIDs(ctx, groupIDs); err != nil {
			return err
		}
		if err := qtx.DeleteSyncGroupsByIDs(ctx, DeleteSyncGroupsByIDsParams{
			UserID:   userID,
			GroupIds: groupIDs,
		}); err != nil {
			return err
		}
	}
	return nil
}

// --- Settings ---

func (s *Store) GetSettings(ctx context.Context, userID int64, scopeType domain.ScopeType, scopeID string) (map[string]any, error) {
	rows, err := s.q.GetSettings(ctx, GetSettingsParams{
		UserID:    userID,
		ScopeType: string(scopeType),
		ScopeID:   scopeID,
	})
	if err != nil {
		return nil, err
	}
	result := make(map[string]any)
	for _, r := range rows {
		var value any
		if err := json.Unmarshal([]byte(r.Value), &value); err != nil {
			return nil, err
		}
		result[r.Key] = value
	}
	return result, nil
}

func (s *Store) SetSetting(ctx context.Context, userID int64, scopeType domain.ScopeType, scopeID, key string, value any) error {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.q.SetSetting(ctx, SetSettingParams{
		UserID:    userID,
		ScopeType: string(scopeType),
		ScopeID:   scopeID,
		Key:       key,
		Value:     string(valueJSON),
	})
}

func (s *Store) DeleteSetting(ctx context.Context, userID int64, scopeType domain.ScopeType, scopeID, key string) error {
	return s.q.DeleteSetting(ctx, DeleteSettingParams{
		UserID:    userID,
		ScopeType: string(scopeType),
		ScopeID:   scopeID,
		Key:       key,
	})
}

// --- Updates ---

func (s *Store) GetSubscriptionEventsSince(ctx context.Context, userID, deviceID int64, since time.Time) ([]domain.SubscriptionEvent, error) {
	rows, err := s.q.GetSubscriptionEventsSince(ctx, GetSubscriptionEventsSinceParams{
		UserID:    userID,
		DeviceID:  deviceID,
		CreatedAt: since,
	})
	if err != nil {
		return nil, err
	}
	var events []domain.SubscriptionEvent
	for _, r := range rows {
		events = append(events, domain.SubscriptionEvent{
			ID:         r.ID,
			UserID:     r.UserID,
			DeviceID:   r.DeviceID,
			PodcastURL: r.PodcastUrl,
			Action:     r.Action,
			CreatedAt:  r.CreatedAt,
		})
	}
	return events, nil
}

// --- Dashboard ---

func (s *Store) GetDashboardSummary(ctx context.Context, userID int64) (*domain.DashboardSummary, error) {
	row, err := s.q.GetDashboardSummary(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &domain.DashboardSummary{
		SubscriptionCount:  int(row.SubscriptionCount),
		DeviceCount:        int(row.DeviceCount),
		EpisodeActionCount: int(row.EpisodeActionCount),
	}, nil
}

func (s *Store) GetRecentEpisodeActions(ctx context.Context, userID int64, limit int) ([]domain.EpisodeAction, error) {
	rows, err := s.q.GetRecentEpisodeActions(ctx, GetRecentEpisodeActionsParams{
		UserID: userID,
		Limit:  int64(limit),
	})
	if err != nil {
		return nil, err
	}
	var res []domain.EpisodeAction
	for _, r := range rows {
		res = append(res, domain.EpisodeAction{
			ID:           r.ID,
			UserID:       r.UserID,
			DeviceID:     r.DeviceID,
			PodcastURL:   r.PodcastUrl,
			PodcastTitle: r.PodcastTitle,
			EpisodeURL:   r.EpisodeUrl,
			EpisodeTitle: r.EpisodeTitle,
			Action:       domain.EpisodeActionType(r.Action),
			Timestamp:    r.Timestamp,
			Started:      int64PtrToIntPtr(r.Started),
			Position:     int64PtrToIntPtr(r.Position),
			Total:        int64PtrToIntPtr(r.Total),
			CreatedAt:    r.CreatedAt,
		})
	}
	return res, nil
}

func (s *Store) GetPlaybackHistory(ctx context.Context, userID int64, limit int) ([]domain.PlaybackHistoryEntry, error) {
	rows, err := s.q.GetPlaybackHistory(ctx, GetPlaybackHistoryParams{
		UserID: userID,
		Limit:  int64(limit),
	})
	if err != nil {
		return nil, err
	}
	var res []domain.PlaybackHistoryEntry
	for _, r := range rows {
		res = append(res, domain.PlaybackHistoryEntry{
			PodcastURL:   r.PodcastUrl,
			PodcastTitle: r.PodcastTitle,
			EpisodeURL:   r.EpisodeUrl,
			EpisodeTitle: r.EpisodeTitle,
			DeviceID:     r.DeviceID,
			Timestamp:    r.Timestamp,
			Position:     int64PtrToIntPtr(r.Position),
			Total:        int64PtrToIntPtr(r.Total),
		})
	}
	return res, nil
}

func (s *Store) GetAggregatedSubscriptions(ctx context.Context, userID int64) ([]domain.AggregatedSubscription, error) {
	rows, err := s.q.GetAggregatedSubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}
	var res []domain.AggregatedSubscription
	for _, r := range rows {
		var devices []string
		if devicesStr, ok := r.Devices.(string); ok {
			_ = json.Unmarshal([]byte(devicesStr), &devices)
		}
		res = append(res, domain.AggregatedSubscription{
			PodcastURL:   r.PodcastUrl,
			PodcastTitle: r.PodcastTitle,
			Devices:      devices,
		})
	}
	return res, nil
}

func (s *Store) GetDevicesWithSubCount(ctx context.Context, userID int64) ([]domain.DeviceWithSubCount, error) {
	rows, err := s.q.GetDevicesWithSubCount(ctx, userID)
	if err != nil {
		return nil, err
	}
	var res []domain.DeviceWithSubCount
	for _, r := range rows {
		res = append(res, domain.DeviceWithSubCount{
			UID:               r.Uid,
			Caption:           r.Caption,
			Type:              domain.DeviceType(r.Type),
			SubscriptionCount: int(r.SubscriptionCount),
			CreatedAt:         r.CreatedAt,
			UpdatedAt:         r.UpdatedAt,
		})
	}
	return res, nil
}

func (s *Store) GetSessionByID(ctx context.Context, sessionID string) (*domain.Session, error) {
	sess, err := s.q.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return &domain.Session{
		ID:        sess.ID,
		UserID:    sess.UserID,
		ExpiresAt: sess.ExpiresAt,
		CreatedAt: sess.CreatedAt,
		UpdatedAt: sess.UpdatedAt,
	}, nil
}

// --- Podcast Metadata ---

func (s *Store) GetPodcastByURL(ctx context.Context, url string) (*domain.Podcast, error) {
	p, err := s.q.GetPodcastByURL(ctx, url)
	if err != nil {
		return nil, err
	}
	var lastFetchedAt *time.Time
	if !p.LastFetchedAt.IsZero() {
		lastFetchedAt = &p.LastFetchedAt
	}
	return &domain.Podcast{
		ID:            p.ID,
		URL:           p.Url,
		Title:         p.Title,
		Description:   p.Description,
		Author:        p.Author,
		SiteURL:       p.SiteUrl,
		ImageURL:      p.ImageUrl,
		ETag:          p.Etag,
		LastModified:  p.LastModified,
		LastFetchedAt: lastFetchedAt,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}, nil
}

func (s *Store) PodcastEpisodeExists(ctx context.Context, podcastURL, episodeURL string) (bool, error) {
	return s.q.PodcastEpisodeExists(ctx, PodcastEpisodeExistsParams{
		Url:        podcastURL,
		EpisodeUrl: episodeURL,
	})
}

func (s *Store) UpsertPodcastMetadata(ctx context.Context, podcast *domain.Podcast) (int64, error) {
	var lastFetchedAt time.Time
	if podcast.LastFetchedAt != nil {
		lastFetchedAt = *podcast.LastFetchedAt
	}
	return s.q.UpsertPodcastMetadata(ctx, UpsertPodcastMetadataParams{
		Url:           podcast.URL,
		Title:         podcast.Title,
		Description:   podcast.Description,
		Author:        podcast.Author,
		SiteUrl:       podcast.SiteURL,
		ImageUrl:      podcast.ImageURL,
		Etag:          podcast.ETag,
		LastModified:  podcast.LastModified,
		LastFetchedAt: lastFetchedAt,
	})
}

func (s *Store) UpdatePodcastFetchState(ctx context.Context, podcastURL, etag, lastModified string, fetchedAt time.Time) error {
	return s.q.UpdatePodcastFetchState(ctx, UpdatePodcastFetchStateParams{
		Url:           podcastURL,
		Etag:          etag,
		LastModified:  lastModified,
		LastFetchedAt: fetchedAt,
	})
}

func (s *Store) UpsertPodcastEpisodes(ctx context.Context, podcastID int64, episodes []domain.PodcastEpisodeMetadata) error {
	if len(episodes) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := s.q.WithTx(tx)

	for _, ep := range episodes {
		var pubAt time.Time
		if ep.PublishedAt != nil {
			pubAt = *ep.PublishedAt
		}
		var dur *int64
		if ep.DurationSeconds != nil {
			d := int64(*ep.DurationSeconds)
			dur = &d
		}
		err := qtx.UpsertPodcastEpisode(ctx, UpsertPodcastEpisodeParams{
			PodcastID:       podcastID,
			EpisodeUrl:      ep.EpisodeURL,
			Guid:            ep.GUID,
			Title:           ep.Title,
			Description:     ep.Description,
			PublishedAt:     pubAt,
			DurationSeconds: dur,
			MimeType:        ep.MIMEType,
			ByteSize:        ep.ByteSize,
		})
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpsertPodcastWithEpisodes(ctx context.Context, podcast *domain.Podcast, episodes []domain.PodcastEpisodeMetadata) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := s.q.WithTx(tx)

	var lastFetchedAt time.Time
	if podcast.LastFetchedAt != nil {
		lastFetchedAt = *podcast.LastFetchedAt
	}

	podcastID, err := qtx.UpsertPodcastMetadata(ctx, UpsertPodcastMetadataParams{
		Url:           podcast.URL,
		Title:         podcast.Title,
		Description:   podcast.Description,
		Author:        podcast.Author,
		SiteUrl:       podcast.SiteURL,
		ImageUrl:      podcast.ImageURL,
		Etag:          podcast.ETag,
		LastModified:  podcast.LastModified,
		LastFetchedAt: lastFetchedAt,
	})
	if err != nil {
		return err
	}

	for _, ep := range episodes {
		var pubAt time.Time
		if ep.PublishedAt != nil {
			pubAt = *ep.PublishedAt
		}
		var dur *int64
		if ep.DurationSeconds != nil {
			d := int64(*ep.DurationSeconds)
			dur = &d
		}
		err := qtx.UpsertPodcastEpisode(ctx, UpsertPodcastEpisodeParams{
			PodcastID:       podcastID,
			EpisodeUrl:      ep.EpisodeURL,
			Guid:            ep.GUID,
			Title:           ep.Title,
			Description:     ep.Description,
			PublishedAt:     pubAt,
			DurationSeconds: dur,
			MimeType:        ep.MIMEType,
			ByteSize:        ep.ByteSize,
		})
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) PodcastFetchDue(ctx context.Context, podcastURL string, now time.Time, cooldown time.Duration) (bool, *domain.Podcast, error) {
	podcast, err := s.GetPodcastByURL(ctx, podcastURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil, nil
		}
		return false, nil, err
	}
	if podcast.LastFetchedAt == nil {
		return true, podcast, nil
	}
	return now.Sub(*podcast.LastFetchedAt) >= cooldown, podcast, nil
}
