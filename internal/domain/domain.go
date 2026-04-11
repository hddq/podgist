package domain

import "time"

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

type Session struct {
	ID        string
	UserID    int64
	ExpiresAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

type DeviceType string

const (
	DeviceDesktop DeviceType = "desktop"
	DeviceLaptop  DeviceType = "laptop"
	DeviceMobile  DeviceType = "mobile"
	DeviceServer  DeviceType = "server"
	DeviceOther   DeviceType = "other"
)

func ValidDeviceType(t string) bool {
	switch DeviceType(t) {
	case DeviceDesktop, DeviceLaptop, DeviceMobile, DeviceServer, DeviceOther:
		return true
	}
	return false
}

type Device struct {
	ID        int64
	UserID    int64
	UID       string
	Caption   string
	Type      DeviceType
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Podcast struct {
	ID            int64
	URL           string
	Title         string
	Description   string
	Author        string
	SiteURL       string
	ImageURL      string
	ETag          string
	LastModified  string
	LastFetchedAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type PodcastEpisodeMetadata struct {
	ID              int64
	PodcastID       int64
	EpisodeURL      string
	GUID            string
	Title           string
	Description     string
	PublishedAt     *time.Time
	DurationSeconds *int
	MIMEType        string
	ByteSize        *int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Subscription struct {
	ID        int64
	UserID    int64
	DeviceID  int64
	PodcastID int64
	CreatedAt time.Time
}

type SubscriptionEvent struct {
	ID         int64
	UserID     int64
	DeviceID   int64
	PodcastURL string
	Action     string // "subscribe" or "unsubscribe"
	CreatedAt  time.Time
}

type EpisodeActionType string

const (
	ActionDownload EpisodeActionType = "download"
	ActionPlay     EpisodeActionType = "play"
	ActionDelete   EpisodeActionType = "delete"
	ActionNew      EpisodeActionType = "new"
	ActionFlattr   EpisodeActionType = "flattr"
)

func ValidEpisodeAction(a string) bool {
	switch EpisodeActionType(a) {
	case ActionDownload, ActionPlay, ActionDelete, ActionNew, ActionFlattr:
		return true
	}
	return false
}

type EpisodeAction struct {
	ID           int64
	UserID       int64
	DeviceID     *int64
	PodcastURL   string
	PodcastTitle string
	EpisodeURL   string
	EpisodeTitle string
	Action       EpisodeActionType
	Timestamp    time.Time
	Started      *int
	Position     *int
	Total        *int
	CreatedAt    time.Time
}

type PlaybackHistoryEntry struct {
	PodcastURL   string
	PodcastTitle string
	EpisodeURL   string
	EpisodeTitle string
	DeviceID     *int64
	Timestamp    time.Time
	Position     *int
	Total        *int
}

type SyncGroup struct {
	ID     int64
	UserID int64
}

type DeviceSyncGroupMember struct {
	DeviceID    int64
	SyncGroupID int64
}

type ScopeType string

const (
	ScopeAccount ScopeType = "account"
	ScopeDevice  ScopeType = "device"
	ScopePodcast ScopeType = "podcast"
	ScopeEpisode ScopeType = "episode"
)

type Setting struct {
	ID        int64
	UserID    int64
	ScopeType ScopeType
	ScopeID   string
	Key       string
	Value     string
}
