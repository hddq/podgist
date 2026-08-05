package service

import (
	"context"
	"fmt"

	"github.com/hddq/podgist/internal/store"
)

type SyncService struct {
	store store.Store
}

func NewSyncService(s store.Store) *SyncService {
	return &SyncService{store: s}
}

type SyncStatus struct {
	Synchronized    [][]string `json:"synchronized"`
	NotSynchronized []string   `json:"not-synchronized"`
}

func (s *SyncService) GetStatus(ctx context.Context, userID int64) (*SyncStatus, error) {
	synced, unsynced, err := s.store.GetSyncStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	if synced == nil {
		synced = [][]string{}
	}
	if unsynced == nil {
		unsynced = []string{}
	}
	return &SyncStatus{
		Synchronized:    synced,
		NotSynchronized: unsynced,
	}, nil
}

func (s *SyncService) UpdateStatus(ctx context.Context, userID int64, syncList [][]string, stopSync []string) (*SyncStatus, error) {
	for _, devList := range syncList {
		if len(devList) <= 1 {
			return nil, fmt.Errorf("at least two devices are needed to sync")
		}
		if err := s.store.SetSyncGroup(ctx, userID, devList); err != nil {
			return nil, err
		}
	}

	for _, uid := range stopSync {
		if err := s.store.StopSync(ctx, userID, uid); err != nil {
			// Ignore errors for devices not in sync groups (matches Python behavior)
		}
	}

	return s.GetStatus(ctx, userID)
}
