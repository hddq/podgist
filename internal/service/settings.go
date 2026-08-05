package service

import (
	"context"
	"fmt"

	"github.com/hddq/podgist/internal/domain"
	"github.com/hddq/podgist/internal/store"
)

type SettingsService struct {
	store store.Store
}

func NewSettingsService(s store.Store) *SettingsService {
	return &SettingsService{store: s}
}

func (s *SettingsService) Get(ctx context.Context, userID int64, scopeType domain.ScopeType, scopeID string) (map[string]any, error) {
	settings, err := s.store.GetSettings(ctx, userID, scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = make(map[string]any)
	}
	return settings, nil
}

type SettingsUpdateRequest struct {
	Set    map[string]any `json:"set"`
	Remove []string       `json:"remove"`
}

func (s *SettingsService) Update(ctx context.Context, userID int64, scopeType domain.ScopeType, scopeID string, req *SettingsUpdateRequest) (map[string]any, error) {
	for key, value := range req.Set {
		if err := s.store.SetSetting(ctx, userID, scopeType, scopeID, key, value); err != nil {
			return nil, err
		}
	}
	for _, key := range req.Remove {
		if err := s.store.DeleteSetting(ctx, userID, scopeType, scopeID, key); err != nil {
			return nil, err
		}
	}
	return s.Get(ctx, userID, scopeType, scopeID)
}

func ParseScope(scope, deviceUID, podcastURL, episodeURL string) (domain.ScopeType, string, error) {
	switch scope {
	case "account":
		return domain.ScopeAccount, "", nil
	case "device":
		if deviceUID == "" {
			return "", "", fmt.Errorf("device parameter required")
		}
		return domain.ScopeDevice, deviceUID, nil
	case "podcast":
		if podcastURL == "" {
			return "", "", fmt.Errorf("podcast parameter required")
		}
		return domain.ScopePodcast, podcastURL, nil
	case "episode":
		if podcastURL == "" || episodeURL == "" {
			return "", "", fmt.Errorf("podcast and episode parameters required")
		}
		return domain.ScopeEpisode, podcastURL + "\n" + episodeURL, nil
	default:
		return "", "", fmt.Errorf("undefined scope %s", scope)
	}
}
