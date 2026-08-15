package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrVolumeExists = errors.New("la unidad ya está registrada")

type RegisterVolumeInput struct {
	PersistentID        string
	HardwareID          string
	IdentityStable      bool
	Name                string
	Label               string
	Platform            string
	Device              string
	VolumeName          string
	PreferredMountPoint string
	FSType              string
	Category            string
	VirtualRoot         string
	IdleTimeoutSeconds  int
	AutoUnmount         bool
	ReadOnly            bool
}

func (s *Store) ListStorageVolumes(ctx context.Context) ([]StorageVolume, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]StorageVolume(nil), s.state.Volumes...), nil
}

func (s *Store) StorageVolumeByID(ctx context.Context, id string) (StorageVolume, error) {
	if err := ctx.Err(); err != nil {
		return StorageVolume{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, volume := range s.state.Volumes {
		if volume.ID == id {
			return volume, nil
		}
	}
	return StorageVolume{}, ErrNotFound
}

func (s *Store) StorageVolumeByPersistentID(ctx context.Context, persistentID string) (StorageVolume, error) {
	if err := ctx.Err(); err != nil {
		return StorageVolume{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, volume := range s.state.Volumes {
		if strings.EqualFold(volume.PersistentID, persistentID) {
			return volume, nil
		}
	}
	return StorageVolume{}, ErrNotFound
}

func (s *Store) StorageVolumeByVirtualRoot(ctx context.Context, root string) (StorageVolume, error) {
	if err := ctx.Err(); err != nil {
		return StorageVolume{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, volume := range s.state.Volumes {
		if strings.EqualFold(volume.VirtualRoot, root) {
			return volume, nil
		}
	}
	return StorageVolume{}, ErrNotFound
}

func (s *Store) RegisterStorageVolume(ctx context.Context, input RegisterVolumeInput) (StorageVolume, error) {
	if err := ctx.Err(); err != nil {
		return StorageVolume{}, err
	}
	var created StorageVolume
	err := s.mutate(ctx, func(next *persistedState) error {
		for _, volume := range next.Volumes {
			if strings.EqualFold(volume.PersistentID, input.PersistentID) {
				return ErrVolumeExists
			}
			if strings.EqualFold(volume.VirtualRoot, input.VirtualRoot) {
				return fmt.Errorf("la raíz virtual %q ya está en uso", input.VirtualRoot)
			}
		}
		id, err := randomID(16)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		created = StorageVolume{
			ID:                  id,
			PersistentID:        strings.TrimSpace(input.PersistentID),
			HardwareID:          strings.TrimSpace(input.HardwareID),
			IdentityStable:      input.IdentityStable,
			Name:                strings.TrimSpace(input.Name),
			Label:               strings.TrimSpace(input.Label),
			Platform:            strings.TrimSpace(input.Platform),
			Device:              strings.TrimSpace(input.Device),
			VolumeName:          strings.TrimSpace(input.VolumeName),
			PreferredMountPoint: strings.TrimSpace(input.PreferredMountPoint),
			FSType:              strings.TrimSpace(input.FSType),
			Category:            strings.TrimSpace(input.Category),
			VirtualRoot:         strings.TrimSpace(input.VirtualRoot),
			IdleTimeoutSeconds:  input.IdleTimeoutSeconds,
			AutoUnmount:         input.AutoUnmount,
			ReadOnly:            input.ReadOnly,
			RegisteredAt:        now,
			UpdatedAt:           now,
		}
		next.Volumes = append(next.Volumes, created)
		return nil
	})
	if err != nil {
		return StorageVolume{}, err
	}
	return created, nil
}

func (s *Store) UpdateStorageVolume(ctx context.Context, id string, update RegisterVolumeInput) (StorageVolume, error) {
	if err := ctx.Err(); err != nil {
		return StorageVolume{}, err
	}
	var updated StorageVolume
	err := s.mutate(ctx, func(next *persistedState) error {
		index := -1
		for i, volume := range next.Volumes {
			if volume.ID == id {
				index = i
				continue
			}
			if strings.EqualFold(volume.VirtualRoot, update.VirtualRoot) {
				return fmt.Errorf("la raíz virtual %q ya está en uso", update.VirtualRoot)
			}
		}
		if index < 0 {
			return ErrNotFound
		}
		volume := next.Volumes[index]
		volume.Name = strings.TrimSpace(update.Name)
		if strings.TrimSpace(update.HardwareID) != "" {
			volume.HardwareID = strings.TrimSpace(update.HardwareID)
		}
		volume.Category = strings.TrimSpace(update.Category)
		volume.VirtualRoot = strings.TrimSpace(update.VirtualRoot)
		volume.IdleTimeoutSeconds = update.IdleTimeoutSeconds
		volume.AutoUnmount = update.AutoUnmount
		volume.ReadOnly = update.ReadOnly
		if strings.TrimSpace(update.Device) != "" {
			volume.Device = strings.TrimSpace(update.Device)
		}
		if strings.TrimSpace(update.VolumeName) != "" {
			volume.VolumeName = strings.TrimSpace(update.VolumeName)
		}
		if strings.TrimSpace(update.PreferredMountPoint) != "" {
			volume.PreferredMountPoint = strings.TrimSpace(update.PreferredMountPoint)
		}
		if strings.TrimSpace(update.FSType) != "" {
			volume.FSType = strings.TrimSpace(update.FSType)
		}
		volume.UpdatedAt = time.Now().UTC()
		next.Volumes[index] = volume
		updated = volume
		return nil
	})
	if err != nil {
		return StorageVolume{}, err
	}
	return updated, nil
}

func (s *Store) RefreshStorageVolumeDevice(ctx context.Context, id, device, volumeName, mountPoint, fsType, label, hardwareID string) error {
	return s.mutate(ctx, func(next *persistedState) error {
		for i := range next.Volumes {
			if next.Volumes[i].ID != id {
				continue
			}
			if strings.TrimSpace(device) != "" {
				next.Volumes[i].Device = strings.TrimSpace(device)
			}
			if strings.TrimSpace(volumeName) != "" {
				next.Volumes[i].VolumeName = strings.TrimSpace(volumeName)
			}
			if strings.TrimSpace(mountPoint) != "" {
				next.Volumes[i].PreferredMountPoint = strings.TrimSpace(mountPoint)
			}
			if strings.TrimSpace(fsType) != "" {
				next.Volumes[i].FSType = strings.TrimSpace(fsType)
			}
			if strings.TrimSpace(label) != "" {
				next.Volumes[i].Label = strings.TrimSpace(label)
			}
			if strings.TrimSpace(hardwareID) != "" {
				next.Volumes[i].HardwareID = strings.TrimSpace(hardwareID)
			}
			next.Volumes[i].UpdatedAt = time.Now().UTC()
			return nil
		}
		return ErrNotFound
	})
}
