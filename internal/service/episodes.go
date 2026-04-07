package service

import (
	"context"
	"fmt"
	"time"

	"github.com/hddq/podgist/internal/domain"
	"github.com/hddq/podgist/internal/store"
)

type EpisodeService struct {
	store             *store.Store
	maxEpisodeActions int
}

func NewEpisodeService(s *store.Store, maxActions int) *EpisodeService {
	return &EpisodeService{store: s, maxEpisodeActions: maxActions}
}

type EpisodeActionInput struct {
	Podcast   string `json:"podcast"`
	Episode   string `json:"episode"`
	Device    string `json:"device"`
	Action    string `json:"action"`
	Timestamp string `json:"timestamp"`
	Started   *int   `json:"started"`
	Position  *int   `json:"position"`
	Total     *int   `json:"total"`
}

type EpisodeActionOutput struct {
	Podcast   string `json:"podcast"`
	Episode   string `json:"episode"`
	Action    string `json:"action"`
	Timestamp string `json:"timestamp"`
	Device    string `json:"device,omitempty"`
	Started   *int   `json:"started,omitempty"`
	Position  *int   `json:"position,omitempty"`
	Total     *int   `json:"total,omitempty"`
}

type EpisodeUploadResult struct {
	Timestamp  int64      `json:"timestamp"`
	UpdateURLs [][]string `json:"update_urls"`
}

type EpisodeChanges struct {
	Actions   []EpisodeActionOutput `json:"actions"`
	Timestamp int64                 `json:"timestamp"`
}

func (s *EpisodeService) UploadActions(ctx context.Context, userID int64, actions []EpisodeActionInput, now time.Time) (*EpisodeUploadResult, error) {
	var updateURLs [][]string

	for _, a := range actions {
		if !domain.ValidEpisodeAction(a.Action) {
			return nil, fmt.Errorf("invalid action %q", a.Action)
		}

		podcastURL := normalizeURL(a.Podcast)
		if podcastURL == "" {
			continue
		}
		if a.Podcast != podcastURL {
			updateURLs = append(updateURLs, []string{a.Podcast, podcastURL})
		}

		episodeURL := normalizeURL(a.Episode)
		if episodeURL == "" {
			continue
		}
		if a.Episode != episodeURL {
			updateURLs = append(updateURLs, []string{a.Episode, episodeURL})
		}

		ea := &domain.EpisodeAction{
			UserID:     userID,
			PodcastURL: podcastURL,
			EpisodeURL: episodeURL,
			Action:     domain.EpisodeActionType(a.Action),
			Timestamp:  now,
			Started:    a.Started,
			Position:   a.Position,
			Total:      a.Total,
			CreatedAt:  now,
		}

		if a.Timestamp != "" {
			ts, err := time.Parse(time.RFC3339, a.Timestamp)
			if err != nil {
				ts, err = time.Parse("2006-01-02T15:04:05", a.Timestamp)
				if err != nil {
					return nil, fmt.Errorf("invalid timestamp %q", a.Timestamp)
				}
			}
			ea.Timestamp = ts
		}

		if a.Device != "" {
			dev, err := s.store.GetOrCreateDevice(ctx, userID, a.Device)
			if err != nil {
				return nil, err
			}
			ea.DeviceID = &dev.ID
		}

		if err := s.store.AddEpisodeAction(ctx, ea); err != nil {
			return nil, err
		}
	}

	if updateURLs == nil {
		updateURLs = [][]string{}
	}

	return &EpisodeUploadResult{
		Timestamp:  now.Unix(),
		UpdateURLs: updateURLs,
	}, nil
}

func (s *EpisodeService) GetActions(ctx context.Context, userID int64, since *time.Time, podcastURL *string, deviceID *int64, now time.Time) (*EpisodeChanges, error) {
	actions, err := s.store.GetEpisodeActionsSince(ctx, userID, since, podcastURL, deviceID, s.maxEpisodeActions)
	if err != nil {
		return nil, err
	}

	out := make([]EpisodeActionOutput, 0, len(actions))
	var lastTS int64
	for _, a := range actions {
		o := EpisodeActionOutput{
			Podcast:   a.PodcastURL,
			Episode:   a.EpisodeURL,
			Action:    string(a.Action),
			Timestamp: a.Timestamp.UTC().Format("2006-01-02T15:04:05"),
		}
		if a.DeviceID != nil {
			dev, err := s.store.GetDeviceByID(ctx, *a.DeviceID)
			if err == nil {
				o.Device = dev.UID
			}
		}
		if a.Action == domain.ActionPlay {
			o.Started = a.Started
			o.Position = a.Position
			o.Total = a.Total
		}
		out = append(out, o)
		lastTS = a.CreatedAt.Unix()
	}

	ts := now.Unix()
	if len(actions) > 0 {
		ts = lastTS
	}

	return &EpisodeChanges{
		Actions:   out,
		Timestamp: ts,
	}, nil
}
