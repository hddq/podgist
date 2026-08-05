package service

import (
	"context"
	"time"

	"github.com/hddq/podgist/internal/store"
)

type UpdatesService struct {
	store store.Store
}

func NewUpdatesService(s store.Store) *UpdatesService {
	return &UpdatesService{store: s}
}

type UpdatesResponse struct {
	Add       []string `json:"add"`
	Remove    []string `json:"rem"`
	Updates   []any    `json:"updates"`
	Timestamp int64    `json:"timestamp"`
}

func (s *UpdatesService) GetUpdates(ctx context.Context, userID, deviceID int64, since time.Time, now time.Time) (*UpdatesResponse, error) {
	events, err := s.store.GetSubscriptionEventsSince(ctx, userID, deviceID, since)
	if err != nil {
		return nil, err
	}

	addSet := make(map[string]bool)
	removeSet := make(map[string]bool)
	for _, e := range events {
		if e.Action == "subscribe" {
			addSet[e.PodcastURL] = true
			delete(removeSet, e.PodcastURL)
		} else {
			removeSet[e.PodcastURL] = true
			delete(addSet, e.PodcastURL)
		}
	}

	add := make([]string, 0, len(addSet))
	for url := range addSet {
		add = append(add, url)
	}
	rem := make([]string, 0, len(removeSet))
	for url := range removeSet {
		rem = append(rem, url)
	}

	return &UpdatesResponse{
		Add:       add,
		Remove:    rem,
		Updates:   []any{},
		Timestamp: now.Unix(),
	}, nil
}
