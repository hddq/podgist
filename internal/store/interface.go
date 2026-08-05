package store

import (
	"context"
	"github.com/hddq/podgist/internal/domain"
	"time"
)

type Store interface {
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
	CreateUser(ctx context.Context, username, passwordHash string) (*domain.User, error)
	CreateSession(ctx context.Context, sessionID string, userID int64, expiresAt time.Time) (*domain.Session, error)
	GetUserBySessionID(ctx context.Context, sessionID string, now time.Time) (*domain.User, error)
	DeleteSession(ctx context.Context, sessionID string) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) error
	TouchSession(ctx context.Context, sessionID string, expiresAt time.Time) error
	GetDevice(ctx context.Context, userID int64, uid string) (*domain.Device, error)
	GetOrCreateDevice(ctx context.Context, userID int64, uid string) (*domain.Device, error)
	UpdateDevice(ctx context.Context, userID int64, uid, caption string, deviceType domain.DeviceType) error
	ListDevices(ctx context.Context, userID int64) ([]domain.Device, error)
	CountSubscriptions(ctx context.Context, userID, deviceID int64) (int, error)
	GetDeviceByID(ctx context.Context, id int64) (*domain.Device, error)
	AddSubscription(ctx context.Context, userID, deviceID int64, podcastURL string, now time.Time) error
	RemoveSubscription(ctx context.Context, userID, deviceID int64, podcastURL string, now time.Time) error
	GetSubscriptionsSince(ctx context.Context, userID, deviceID int64, since time.Time) (add, remove []string, err error)
	GetCurrentSubscriptions(ctx context.Context, userID, deviceID int64) ([]string, error)
	AddEpisodeAction(ctx context.Context, ea *domain.EpisodeAction) error
	GetEpisodeActionsSince(ctx context.Context, userID int64, since *time.Time, podcastURL *string, deviceID *int64, limit int) ([]domain.EpisodeAction, error)
	GetSyncStatus(ctx context.Context, userID int64) (synced [][]string, unsynced []string, err error)
	SetSyncGroup(ctx context.Context, userID int64, deviceUIDs []string) error
	StopSync(ctx context.Context, userID int64, uid string) error
	GetSettings(ctx context.Context, userID int64, scopeType domain.ScopeType, scopeID string) (map[string]any, error)
	SetSetting(ctx context.Context, userID int64, scopeType domain.ScopeType, scopeID, key string, value any) error
	DeleteSetting(ctx context.Context, userID int64, scopeType domain.ScopeType, scopeID, key string) error
	GetSubscriptionEventsSince(ctx context.Context, userID, deviceID int64, since time.Time) ([]domain.SubscriptionEvent, error)
	GetDashboardSummary(ctx context.Context, userID int64) (*domain.DashboardSummary, error)
	GetRecentEpisodeActions(ctx context.Context, userID int64, limit int) ([]domain.EpisodeAction, error)
	GetPlaybackHistory(ctx context.Context, userID int64, limit int) ([]domain.PlaybackHistoryEntry, error)
	GetAggregatedSubscriptions(ctx context.Context, userID int64) ([]domain.AggregatedSubscription, error)
	GetDevicesWithSubCount(ctx context.Context, userID int64) ([]domain.DeviceWithSubCount, error)
	GetSessionByID(ctx context.Context, sessionID string) (*domain.Session, error)
	GetPodcastByURL(ctx context.Context, url string) (*domain.Podcast, error)
	PodcastEpisodeExists(ctx context.Context, podcastURL, episodeURL string) (bool, error)
	UpsertPodcastMetadata(ctx context.Context, podcast *domain.Podcast) (int64, error)
	UpdatePodcastFetchState(ctx context.Context, podcastURL, etag, lastModified string, fetchedAt time.Time) error
	UpsertPodcastEpisodes(ctx context.Context, podcastID int64, episodes []domain.PodcastEpisodeMetadata) error
	UpsertPodcastWithEpisodes(ctx context.Context, podcast *domain.Podcast, episodes []domain.PodcastEpisodeMetadata) error
	PodcastFetchDue(ctx context.Context, podcastURL string, now time.Time, cooldown time.Duration) (bool, *domain.Podcast, error)
}
