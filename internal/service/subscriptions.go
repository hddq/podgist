package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/hddq/podgist/internal/store"
)

type SubscriptionService struct {
	store    store.Store
	metadata *PodcastMetadataService
}

func NewSubscriptionService(s store.Store, metadata *PodcastMetadataService) *SubscriptionService {
	return &SubscriptionService{store: s, metadata: metadata}
}

type SubscriptionChange struct {
	Add       []string `json:"add"`
	Remove    []string `json:"remove"`
	Timestamp int64    `json:"timestamp"`
}

type SubscriptionUploadResult struct {
	Timestamp  int64      `json:"timestamp"`
	UpdateURLs [][]string `json:"update_urls"`
}

func (s *SubscriptionService) GetSubscriptions(ctx context.Context, userID, deviceID int64, since *time.Time, now time.Time) (*SubscriptionChange, error) {
	if since == nil {
		urls, err := s.store.GetCurrentSubscriptions(ctx, userID, deviceID)
		if err != nil {
			return nil, err
		}
		if urls == nil {
			urls = []string{}
		}
		return &SubscriptionChange{
			Add:       urls,
			Remove:    []string{},
			Timestamp: now.Unix(),
		}, nil
	}

	add, remove, err := s.store.GetSubscriptionsSince(ctx, userID, deviceID, *since)
	if err != nil {
		return nil, err
	}
	if add == nil {
		add = []string{}
	}
	if remove == nil {
		remove = []string{}
	}
	return &SubscriptionChange{
		Add:       add,
		Remove:    remove,
		Timestamp: now.Unix(),
	}, nil
}

func (s *SubscriptionService) UpdateSubscriptions(ctx context.Context, userID, deviceID int64, add, remove []string, now time.Time) (*SubscriptionUploadResult, error) {
	// Check for conflicts
	addSet := make(map[string]bool, len(add))
	for _, u := range add {
		addSet[u] = true
	}
	for _, u := range remove {
		if addSet[u] {
			return nil, fmt.Errorf("cannot add and remove %q at the same time", u)
		}
	}

	var updateURLs [][]string

	for _, rawURL := range add {
		normalized := normalizeURL(rawURL)
		if normalized == "" {
			continue
		}
		if rawURL != normalized {
			updateURLs = append(updateURLs, []string{rawURL, normalized})
		}
		if err := s.store.AddSubscription(ctx, userID, deviceID, normalized, now); err != nil {
			return nil, err
		}
		if s.metadata != nil {
			s.metadata.ScheduleFetch(ctx, normalized)
		}
	}

	for _, rawURL := range remove {
		normalized := normalizeURL(rawURL)
		if normalized == "" {
			continue
		}
		if rawURL != normalized {
			updateURLs = append(updateURLs, []string{rawURL, normalized})
		}
		if err := s.store.RemoveSubscription(ctx, userID, deviceID, normalized, now); err != nil {
			return nil, err
		}
	}

	if updateURLs == nil {
		updateURLs = [][]string{}
	}

	return &SubscriptionUploadResult{
		Timestamp:  now.Unix(),
		UpdateURLs: updateURLs,
	}, nil
}

func normalizeURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	// Ensure scheme
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	// Remove trailing slash from path
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String()
}
