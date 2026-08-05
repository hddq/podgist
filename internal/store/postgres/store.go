package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

// --- Users ---

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	u, err := s.q.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
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
	err := s.q.TouchSession(ctx, TouchSessionParams{
		ExpiresAt: expiresAt,
		ID:        sessionID,
	})
	return err
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
	d, err := s.q.UpsertDevice(ctx, UpsertDeviceParams{
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

func (s *Store) UpdateDevice(ctx context.Context, userID int64, uid, caption string, deviceType domain.DeviceType) error {
	return s.q.UpdateDevice(ctx, UpdateDeviceParams{
		Caption: caption,
		Type:    string(deviceType),
		UserID:  userID,
		Uid:     uid,
	})
}

func (s *Store) ListDevices(ctx context.Context, userID int64) ([]domain.Device, error) {
	devices, err := s.q.ListDevices(ctx, userID)
	if err != nil {
		return nil, err
	}
	res := make([]domain.Device, len(devices))
	for i, d := range devices {
		res[i] = domain.Device{
			ID:        d.ID,
			UserID:    d.UserID,
			UID:       d.Uid,
			Caption:   d.Caption,
			Type:      domain.DeviceType(d.Type),
			CreatedAt: d.CreatedAt,
			UpdatedAt: d.UpdatedAt,
		}
	}
	return res, nil
}

func (s *Store) CountSubscriptions(ctx context.Context, userID, deviceID int64) (int, error) {
	c, err := s.q.CountSubscriptions(ctx, CountSubscriptionsParams{
		UserID:   userID,
		DeviceID: deviceID,
	})
	return int(c), err
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

	deviceIDs, err := qtx.GetSyncTargetDeviceIDs(ctx, GetSyncTargetDeviceIDsParams{
		UserID: userID,
		ID:     deviceID,
	})
	if err != nil {
		return err
	}
	if len(deviceIDs) == 0 {
		deviceIDs = []int64{deviceID}
	}

	for _, targetID := range deviceIDs {
		err = qtx.InsertSubscription(ctx, InsertSubscriptionParams{
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

	deviceIDs, err := qtx.GetSyncTargetDeviceIDs(ctx, GetSyncTargetDeviceIDsParams{
		UserID: userID,
		ID:     deviceID,
	})
	if err != nil {
		return err
	}
	if len(deviceIDs) == 0 {
		deviceIDs = []int64{deviceID}
	}

	for _, targetID := range deviceIDs {
		err = qtx.DeleteSubscription(ctx, DeleteSubscriptionParams{
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

func (s *Store) GetSubscriptionsSince(ctx context.Context, userID, deviceID int64, since time.Time) ([]string, []string, error) {
	rows, err := s.q.GetSubscriptionsSince(ctx, GetSubscriptionsSinceParams{
		UserID:    userID,
		DeviceID:  deviceID,
		CreatedAt: since,
	})
	if err != nil {
		return nil, nil, err
	}
	var add []string
	var remove []string
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

func nullInt64ToPtr(n sql.NullInt64) *int64 {
	if n.Valid {
		return &n.Int64
	}
	return nil
}

func ptrToNullInt64(p *int64) sql.NullInt64 {
	if p != nil {
		return sql.NullInt64{Int64: *p, Valid: true}
	}
	return sql.NullInt64{}
}

func nullInt32ToPtr(n sql.NullInt32) *int {
	if n.Valid {
		v := int(n.Int32)
		return &v
	}
	return nil
}

func ptrToNullInt32(p *int) sql.NullInt32 {
	if p != nil {
		return sql.NullInt32{Int32: int32(*p), Valid: true}
	}
	return sql.NullInt32{}
}

func ptrToNullTime(p *time.Time) sql.NullTime {
	if p != nil {
		return sql.NullTime{Time: *p, Valid: true}
	}
	return sql.NullTime{}
}

func ptrToNullString(p *string) sql.NullString {
	if p != nil {
		return sql.NullString{String: *p, Valid: true}
	}
	return sql.NullString{}
}

// --- Episode Actions ---

func (s *Store) AddEpisodeAction(ctx context.Context, ea *domain.EpisodeAction) error {
	return s.q.InsertEpisodeAction(ctx, InsertEpisodeActionParams{
		UserID:     ea.UserID,
		DeviceID:   ptrToNullInt64(ea.DeviceID),
		PodcastUrl: ea.PodcastURL,
		EpisodeUrl: ea.EpisodeURL,
		Action:     string(ea.Action),
		Timestamp:  ea.Timestamp,
		Started:    ptrToNullInt32(ea.Started),
		Position:   ptrToNullInt32(ea.Position),
		Total:      ptrToNullInt32(ea.Total),
		CreatedAt:  ea.CreatedAt,
	})
}

func (s *Store) GetEpisodeActionsSince(ctx context.Context, userID int64, since *time.Time, podcastURL *string, deviceID *int64, limit int) ([]domain.EpisodeAction, error) {
	params := GetEpisodeActionsSinceParams{
		UserID:           userID,
		CreatedAfter:     ptrToNullTime(since),
		PodcastUrlFilter: ptrToNullString(podcastURL),
		DeviceIDFilter:   ptrToNullInt64(deviceID),
	}
	if limit > 0 {
		params.LimitVal = sql.NullInt32{Int32: int32(limit), Valid: true}
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
			DeviceID:   nullInt64ToPtr(r.DeviceID),
			PodcastURL: r.PodcastUrl,
			EpisodeURL: r.EpisodeUrl,
			Action:     domain.EpisodeActionType(r.Action),
			Timestamp:  r.Timestamp,
			Started:    nullInt32ToPtr(r.Started),
			Position:   nullInt32ToPtr(r.Position),
			Total:      nullInt32ToPtr(r.Total),
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
		if _, ok := groupMap[r.SyncGroupID]; !ok {
			groupOrder = append(groupOrder, r.SyncGroupID)
		}
		groupMap[r.SyncGroupID] = append(groupMap[r.SyncGroupID], r.Uid)
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
			return err
		}
		deviceIDs = append(deviceIDs, id)
	}

	for _, id := range deviceIDs {
		if err := qtx.DeleteDeviceFromSyncGroups(ctx, id); err != nil {
			return err
		}
	}

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
				// DO NOTHING handles duplicate constraint
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
		return err
	}

	if err := qtx.DeleteDeviceFromSyncGroups(ctx, deviceID); err != nil {
		return err
	}

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

	return tx.Commit()
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
		var v any
		if err := json.Unmarshal(r.Value, &v); err != nil {
			return nil, err
		}
		result[r.Key] = v
	}
	return result, nil
}

func (s *Store) SetSetting(ctx context.Context, userID int64, scopeType domain.ScopeType, scopeID, key string, value any) error {
	valBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.q.SetSetting(ctx, SetSettingParams{
		UserID:    userID,
		ScopeType: string(scopeType),
		ScopeID:   scopeID,
		Key:       key,
		Value:     valBytes,
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

func (s *Store) GetSubscriptionEventsSince(ctx context.Context, userID, deviceID int64, since time.Time) ([]domain.SubscriptionEvent, error) {
	rows, err := s.q.GetSubscriptionEventsSince(ctx, GetSubscriptionEventsSinceParams{
		UserID:    userID,
		DeviceID:  deviceID,
		CreatedAt: since,
	})
	if err != nil {
		return nil, err
	}
	var res []domain.SubscriptionEvent
	for _, r := range rows {
		res = append(res, domain.SubscriptionEvent{
			ID:         r.ID,
			UserID:     r.UserID,
			DeviceID:   r.DeviceID,
			PodcastURL: r.PodcastUrl,
			Action:     r.Action,
			CreatedAt:  r.CreatedAt,
		})
	}
	return res, nil
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
		Limit:  int32(limit),
	})
	if err != nil {
		return nil, err
	}
	var res []domain.EpisodeAction
	for _, r := range rows {
		res = append(res, domain.EpisodeAction{
			ID:           r.ID,
			UserID:       r.UserID,
			DeviceID:     nullInt64ToPtr(r.DeviceID),
			PodcastURL:   r.PodcastUrl,
			PodcastTitle: r.PodcastTitle,
			EpisodeURL:   r.EpisodeUrl,
			EpisodeTitle: r.EpisodeTitle,
			Action:       domain.EpisodeActionType(r.Action),
			Timestamp:    r.Timestamp,
			Started:      nullInt32ToPtr(r.Started),
			Position:     nullInt32ToPtr(r.Position),
			Total:        nullInt32ToPtr(r.Total),
			CreatedAt:    r.CreatedAt,
		})
	}
	return res, nil
}

func (s *Store) GetPlaybackHistory(ctx context.Context, userID int64, limit int) ([]domain.PlaybackHistoryEntry, error) {
	rows, err := s.q.GetPlaybackHistory(ctx, GetPlaybackHistoryParams{
		UserID: userID,
		Limit:  int32(limit),
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
			DeviceID:     nullInt64ToPtr(r.DeviceID),
			Timestamp:    r.Timestamp,
			Position:     nullInt32ToPtr(r.Position),
			Total:        nullInt32ToPtr(r.Total),
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
		res = append(res, domain.AggregatedSubscription{
			PodcastURL:   r.PodcastUrl,
			PodcastTitle: r.PodcastTitle,
			Devices:      r.Devices,
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

// --- Podcasts ---

func (s *Store) GetPodcastByURL(ctx context.Context, url string) (*domain.Podcast, error) {
	p, err := s.q.GetPodcastByURL(ctx, url)
	if err != nil {
		return nil, err
	}
	var lastFetched *time.Time
	if p.LastFetchedAt.Valid {
		lastFetched = &p.LastFetchedAt.Time
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
		LastFetchedAt: lastFetched,
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
	return s.q.UpsertPodcastMetadata(ctx, UpsertPodcastMetadataParams{
		Url:           podcast.URL,
		Title:         podcast.Title,
		Description:   podcast.Description,
		Author:        podcast.Author,
		SiteUrl:       podcast.SiteURL,
		ImageUrl:      podcast.ImageURL,
		Etag:          podcast.ETag,
		LastModified:  podcast.LastModified,
		LastFetchedAt: ptrToNullTime(podcast.LastFetchedAt),
	})
}

func (s *Store) UpdatePodcastFetchState(ctx context.Context, podcastURL, etag, lastModified string, fetchedAt time.Time) error {
	return s.q.UpdatePodcastFetchState(ctx, UpdatePodcastFetchStateParams{
		Etag:          etag,
		LastModified:  lastModified,
		LastFetchedAt: sql.NullTime{Time: fetchedAt, Valid: true},
		Url:           podcastURL,
	})
}

func (s *Store) UpsertPodcastEpisodes(ctx context.Context, podcastID int64, episodes []domain.PodcastEpisodeMetadata) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.q.WithTx(tx)

	for _, ep := range episodes {
		err = qtx.UpsertPodcastEpisode(ctx, UpsertPodcastEpisodeParams{
			PodcastID:       podcastID,
			EpisodeUrl:      ep.EpisodeURL,
			Guid:            ep.GUID,
			Title:           ep.Title,
			Description:     ep.Description,
			PublishedAt:     ptrToNullTime(ep.PublishedAt),
			DurationSeconds: ptrToNullInt32(ep.DurationSeconds),
			MimeType:        ep.MIMEType,
			ByteSize:        ptrToNullInt64(ep.ByteSize),
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

	podcastID, err := qtx.UpsertPodcastMetadata(ctx, UpsertPodcastMetadataParams{
		Url:           podcast.URL,
		Title:         podcast.Title,
		Description:   podcast.Description,
		Author:        podcast.Author,
		SiteUrl:       podcast.SiteURL,
		ImageUrl:      podcast.ImageURL,
		Etag:          podcast.ETag,
		LastModified:  podcast.LastModified,
		LastFetchedAt: ptrToNullTime(podcast.LastFetchedAt),
	})
	if err != nil {
		return err
	}

	for _, ep := range episodes {
		err = qtx.UpsertPodcastEpisode(ctx, UpsertPodcastEpisodeParams{
			PodcastID:       podcastID,
			EpisodeUrl:      ep.EpisodeURL,
			Guid:            ep.GUID,
			Title:           ep.Title,
			Description:     ep.Description,
			PublishedAt:     ptrToNullTime(ep.PublishedAt),
			DurationSeconds: ptrToNullInt32(ep.DurationSeconds),
			MimeType:        ep.MIMEType,
			ByteSize:        ptrToNullInt64(ep.ByteSize),
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
