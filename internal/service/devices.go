package service

import (
	"context"

	"github.com/hddq/podgist/internal/domain"
	"github.com/hddq/podgist/internal/store"
)

type DeviceService struct {
	store *store.Store
}

func NewDeviceService(s *store.Store) *DeviceService {
	return &DeviceService{store: s}
}

type DeviceData struct {
	ID            string `json:"id"`
	Caption       string `json:"caption"`
	Type          string `json:"type"`
	Subscriptions int    `json:"subscriptions"`
}

func (s *DeviceService) List(ctx context.Context, userID int64) ([]DeviceData, error) {
	devices, err := s.store.ListDevices(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]DeviceData, 0, len(devices))
	for _, d := range devices {
		count, err := s.store.CountSubscriptions(ctx, userID, d.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, DeviceData{
			ID:            d.UID,
			Caption:       d.Caption,
			Type:          string(d.Type),
			Subscriptions: count,
		})
	}
	return result, nil
}

func (s *DeviceService) Update(ctx context.Context, userID int64, uid string, caption *string, deviceType *string) error {
	dev, err := s.store.GetOrCreateDevice(ctx, userID, uid)
	if err != nil {
		return err
	}

	newCaption := dev.Caption
	if caption != nil {
		newCaption = *caption
	}
	newType := dev.Type
	if deviceType != nil {
		newType = domain.DeviceType(*deviceType)
	}

	return s.store.UpdateDevice(ctx, userID, uid, newCaption, newType)
}

func (s *DeviceService) GetOrCreate(ctx context.Context, userID int64, uid string) (*domain.Device, error) {
	return s.store.GetOrCreateDevice(ctx, userID, uid)
}

func (s *DeviceService) Get(ctx context.Context, userID int64, uid string) (*domain.Device, error) {
	return s.store.GetDevice(ctx, userID, uid)
}
