package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"personalcloud/internal/store"
)

var (
	ErrOffline  = errors.New("unidad desconectada")
	ErrReadOnly = errors.New("unidad configurada como solo lectura")
)

type runtimeState struct {
	opMu         sync.Mutex
	active       int
	lastActivity time.Time
	mountPoint   string
	mounted      bool
	unmounting   bool
	lastError    string
}

type Manager struct {
	store     *store.Store
	logger    *slog.Logger
	mountRoot string

	mu      sync.Mutex
	runtime map[string]*runtimeState
	stop    chan struct{}
	once    sync.Once
}

type Lease struct {
	Root      string
	Volume    store.StorageVolume
	manager   *Manager
	volumeID  string
	releaseMu sync.Once
}

func NewManager(storage *store.Store, logger *slog.Logger, mountRoot string) *Manager {
	m := &Manager{
		store:     storage,
		logger:    logger,
		mountRoot: mountRoot,
		runtime:   make(map[string]*runtimeState),
		stop:      make(chan struct{}),
	}
	go m.idleLoop()
	return m
}

func (m *Manager) stateFor(volumeID string) *runtimeState {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.runtime[volumeID]
	if state == nil {
		state = &runtimeState{}
		m.runtime[volumeID] = state
	}
	return state
}

func (m *Manager) Close() {
	m.once.Do(func() {
		close(m.stop)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		volumes, err := m.store.ListStorageVolumes(ctx)
		if err != nil {
			return
		}
		for _, cfg := range volumes {
			if !cfg.AutoUnmount {
				continue
			}
			if err := m.Unmount(ctx, cfg.ID); err != nil && !strings.Contains(err.Error(), "operaciones activas") {
				m.logger.Warn("no se pudo desmontar unidad al cerrar", "volume_id", cfg.ID, "error", err)
			}
		}
	})
}

func (l *Lease) Release() {
	if l == nil || l.manager == nil {
		return
	}
	l.releaseMu.Do(func() {
		l.manager.release(l.volumeID)
	})
}

func (m *Manager) Discover(ctx context.Context) ([]DiscoveredVolume, error) {
	return discoverPlatformVolumes(ctx, m.mountRoot)
}

// OnlineRegisteredIDs comprueba presencia física mediante identidad persistente sin
// consultar capacidad ni recorrer contenido. Se usa para vistas que deben reaccionar
// a desconexiones sin despertar discos solo para obtener espacio libre.
func (m *Manager) OnlineRegisteredIDs(ctx context.Context) (map[string]struct{}, error) {
	present, err := discoverPlatformPresence(ctx)
	if err != nil {
		return nil, err
	}
	registered, err := m.store.ListStorageVolumes(ctx)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]struct{})
	for _, cfg := range registered {
		if _, ok := present[strings.ToLower(cfg.PersistentID)]; ok {
			ids[cfg.ID] = struct{}{}
		}
	}
	return ids, nil
}

func (m *Manager) Views(ctx context.Context) ([]View, error) {
	discovered, discoverErr := m.Discover(ctx)
	registered, err := m.store.ListStorageVolumes(ctx)
	if err != nil {
		return nil, err
	}
	byPersistent := make(map[string]DiscoveredVolume, len(discovered))
	for _, volume := range discovered {
		byPersistent[strings.ToLower(volume.PersistentID)] = volume
	}
	registeredKeys := make(map[string]struct{}, len(registered))
	views := make([]View, 0, len(discovered)+len(registered))

	for _, cfg := range registered {
		key := strings.ToLower(cfg.PersistentID)
		registeredKeys[key] = struct{}{}
		detected, online := byPersistent[key]
		view := m.viewFrom(cfg, detected, online)
		views = append(views, view)
		if online && storageIdentityChanged(cfg, detected) {
			_ = m.store.RefreshStorageVolumeDevice(ctx, cfg.ID, detected.Device, detected.VolumeName, detected.MountPoint, detected.FSType, detected.Label, detected.HardwareID)
		}
	}
	for _, detected := range discovered {
		if detected.System {
			continue
		}
		if _, ok := registeredKeys[strings.ToLower(detected.PersistentID)]; ok {
			continue
		}
		views = append(views, View{
			PersistentID:   detected.PersistentID,
			HardwareID:     detected.HardwareID,
			IdentityStable: detected.IdentityStable,
			Name:           detected.Name,
			Label:          detected.Label,
			Platform:       detected.Platform,
			Device:         detected.Device,
			VolumeName:     detected.VolumeName,
			MountPoint:     detected.MountPoint,
			FSType:         detected.FSType,
			Online:         true,
			Mounted:        detected.Mounted,
			System:         detected.System,
			Removable:      detected.Removable,
			Capacity:       detected.Capacity,
			Free:           detected.Free,
			Status:         statusLabel(true, detected.Mounted, false),
		})
	}
	if discoverErr != nil && len(views) == 0 {
		return nil, discoverErr
	}
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].Registered != views[j].Registered {
			return views[i].Registered
		}
		return strings.ToLower(views[i].Name) < strings.ToLower(views[j].Name)
	})
	return views, discoverErr
}

func (m *Manager) Register(ctx context.Context, input RegisterInput) (store.StorageVolume, error) {
	input.PersistentID = strings.TrimSpace(input.PersistentID)
	input.Name = strings.TrimSpace(input.Name)
	input.Category = strings.TrimSpace(input.Category)
	input.VirtualRoot = sanitizeVirtualRoot(input.VirtualRoot)
	if input.Name == "" {
		return store.StorageVolume{}, errors.New("el nombre es obligatorio")
	}
	if input.VirtualRoot == "" {
		return store.StorageVolume{}, errors.New("la raíz virtual es obligatoria")
	}
	if !validCategory(input.Category) {
		return store.StorageVolume{}, errors.New("categoría inválida")
	}
	if input.IdleTimeoutSeconds < 30 || input.IdleTimeoutSeconds > 7*24*60*60 {
		return store.StorageVolume{}, errors.New("el tiempo de inactividad debe estar entre 30 segundos y 7 días")
	}

	detected, err := m.findDiscovered(ctx, input.PersistentID)
	if err != nil {
		return store.StorageVolume{}, err
	}
	if detected.System {
		return store.StorageVolume{}, errors.New("no se permite registrar la unidad del sistema")
	}
	if !detected.IdentityStable {
		return store.StorageVolume{}, errors.New("la unidad no tiene una identidad persistente segura")
	}
	preferred := detected.MountPoint
	if preferred == "" {
		preferred = defaultMountPoint(m.mountRoot, detected)
	}
	created, err := m.store.RegisterStorageVolume(ctx, store.RegisterVolumeInput{
		PersistentID:        detected.PersistentID,
		HardwareID:          detected.HardwareID,
		IdentityStable:      detected.IdentityStable,
		Name:                input.Name,
		Label:               detected.Label,
		Platform:            detected.Platform,
		Device:              detected.Device,
		VolumeName:          detected.VolumeName,
		PreferredMountPoint: preferred,
		FSType:              detected.FSType,
		Category:            input.Category,
		VirtualRoot:         input.VirtualRoot,
		IdleTimeoutSeconds:  input.IdleTimeoutSeconds,
		AutoUnmount:         input.AutoUnmount,
		ReadOnly:            input.ReadOnly,
	})
	if err != nil {
		return store.StorageVolume{}, err
	}
	m.mu.Lock()
	m.runtime[created.ID] = &runtimeState{lastActivity: time.Now().UTC(), mountPoint: detected.MountPoint, mounted: detected.Mounted}
	m.mu.Unlock()
	return created, nil
}

func (m *Manager) Update(ctx context.Context, id string, input RegisterInput) (store.StorageVolume, error) {
	current, err := m.store.StorageVolumeByID(ctx, id)
	if err != nil {
		return store.StorageVolume{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	input.VirtualRoot = sanitizeVirtualRoot(input.VirtualRoot)
	if input.Name == "" || input.VirtualRoot == "" || !validCategory(input.Category) {
		return store.StorageVolume{}, errors.New("configuración de unidad inválida")
	}
	if input.IdleTimeoutSeconds < 30 || input.IdleTimeoutSeconds > 7*24*60*60 {
		return store.StorageVolume{}, errors.New("el tiempo de inactividad debe estar entre 30 segundos y 7 días")
	}
	return m.store.UpdateStorageVolume(ctx, id, store.RegisterVolumeInput{
		PersistentID:        current.PersistentID,
		HardwareID:          current.HardwareID,
		IdentityStable:      current.IdentityStable,
		Name:                input.Name,
		Label:               current.Label,
		Platform:            current.Platform,
		Device:              current.Device,
		VolumeName:          current.VolumeName,
		PreferredMountPoint: current.PreferredMountPoint,
		FSType:              current.FSType,
		Category:            input.Category,
		VirtualRoot:         input.VirtualRoot,
		IdleTimeoutSeconds:  input.IdleTimeoutSeconds,
		AutoUnmount:         input.AutoUnmount,
		ReadOnly:            input.ReadOnly,
	})
}

func (m *Manager) Acquire(ctx context.Context, volumeID string, write bool) (*Lease, error) {
	cfg, err := m.store.StorageVolumeByID(ctx, volumeID)
	if err != nil {
		return nil, err
	}
	if write && cfg.ReadOnly {
		return nil, ErrReadOnly
	}

	state := m.stateFor(volumeID)
	m.mu.Lock()
	state.active++
	state.lastActivity = time.Now().UTC()
	m.mu.Unlock()

	root, err := m.ensureMounted(ctx, cfg)
	if err != nil {
		m.release(volumeID)
		return nil, err
	}
	return &Lease{Root: root, Volume: cfg, manager: m, volumeID: volumeID}, nil
}

func (m *Manager) Mount(ctx context.Context, volumeID string) (string, error) {
	lease, err := m.Acquire(ctx, volumeID, false)
	if err != nil {
		return "", err
	}
	root := lease.Root
	lease.Release()
	return root, nil
}

func (m *Manager) Unmount(ctx context.Context, volumeID string) error {
	cfg, err := m.store.StorageVolumeByID(ctx, volumeID)
	if err != nil {
		return err
	}
	state := m.stateFor(volumeID)
	state.opMu.Lock()
	defer state.opMu.Unlock()
	m.mu.Lock()
	if state.active > 0 {
		m.mu.Unlock()
		return errors.New("la unidad tiene operaciones activas")
	}
	state.unmounting = true
	mountPoint := state.mountPoint
	m.mu.Unlock()

	if mountPoint == "" {
		detected, findErr := m.findDiscovered(ctx, cfg.PersistentID)
		if findErr != nil {
			m.setUnmountResult(volumeID, "", false, "")
			if errors.Is(findErr, ErrOffline) {
				return nil
			}
			return findErr
		}
		if !detected.Mounted {
			m.setUnmountResult(volumeID, "", false, "")
			return nil
		}
		mountPoint = detected.MountPoint
	}
	if err := unmountPlatformVolume(ctx, cfg, mountPoint); err != nil {
		m.setUnmountResult(volumeID, mountPoint, true, err.Error())
		return err
	}
	m.setUnmountResult(volumeID, "", false, "")
	m.logger.Info("unidad desmontada", "volume_id", volumeID, "name", cfg.Name)
	return nil
}

func (m *Manager) ensureMounted(ctx context.Context, cfg store.StorageVolume) (string, error) {
	state := m.stateFor(cfg.ID)
	state.opMu.Lock()
	defer state.opMu.Unlock()
	detected, err := m.findDiscovered(ctx, cfg.PersistentID)
	if err != nil {
		return "", err
	}
	if detected.Mounted && detected.MountPoint != "" {
		m.mu.Lock()
		state.mountPoint = detected.MountPoint
		state.mounted = true
		state.lastError = ""
		m.mu.Unlock()
		return detected.MountPoint, nil
	}

	root, err := mountPlatformVolume(ctx, cfg, detected, m.mountRoot)
	if err != nil {
		m.mu.Lock()
		state.lastError = err.Error()
		m.mu.Unlock()
		return "", fmt.Errorf("montar %q: %w", cfg.Name, err)
	}
	m.mu.Lock()
	state.mountPoint = root
	state.mounted = true
	state.lastError = ""
	state.lastActivity = time.Now().UTC()
	m.mu.Unlock()
	m.logger.Info("unidad montada bajo demanda", "volume_id", cfg.ID, "name", cfg.Name, "mount", root)
	return root, nil
}

func (m *Manager) findDiscovered(ctx context.Context, persistentID string) (DiscoveredVolume, error) {
	volumes, err := m.Discover(ctx)
	if err != nil && len(volumes) == 0 {
		return DiscoveredVolume{}, err
	}
	for _, volume := range volumes {
		if strings.EqualFold(volume.PersistentID, persistentID) {
			return volume, nil
		}
	}
	return DiscoveredVolume{}, ErrOffline
}

func (m *Manager) viewFrom(cfg store.StorageVolume, detected DiscoveredVolume, online bool) View {
	m.mu.Lock()
	state := m.runtime[cfg.ID]
	if state == nil {
		state = &runtimeState{lastActivity: time.Now().UTC()}
		m.runtime[cfg.ID] = state
	}
	if online {
		state.mountPoint = detected.MountPoint
		state.mounted = detected.Mounted
	}
	active := state.active
	lastActivity := state.lastActivity
	lastError := state.lastError
	m.mu.Unlock()

	mountPoint := cfg.PreferredMountPoint
	mounted := false
	capacity, free := uint64(0), uint64(0)
	removable := false
	if online {
		mountPoint = detected.MountPoint
		mounted = detected.Mounted
		capacity = detected.Capacity
		free = detected.Free
		removable = detected.Removable
	}
	return View{
		ID:                 cfg.ID,
		PersistentID:       cfg.PersistentID,
		HardwareID:         firstNonEmpty(detected.HardwareID, cfg.HardwareID),
		IdentityStable:     cfg.IdentityStable,
		Name:               cfg.Name,
		Label:              cfg.Label,
		Platform:           cfg.Platform,
		Device:             cfg.Device,
		VolumeName:         cfg.VolumeName,
		MountPoint:         mountPoint,
		FSType:             cfg.FSType,
		Category:           cfg.Category,
		VirtualRoot:        cfg.VirtualRoot,
		IdleTimeoutSeconds: cfg.IdleTimeoutSeconds,
		AutoUnmount:        cfg.AutoUnmount,
		ReadOnly:           cfg.ReadOnly,
		Registered:         true,
		Online:             online,
		Mounted:            mounted,
		Removable:          removable,
		Capacity:           capacity,
		Free:               free,
		ActiveHandles:      active,
		LastActivity:       lastActivity,
		Status:             statusLabel(online, mounted, active > 0),
		Error:              lastError,
	}
}

func (m *Manager) release(volumeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.runtime[volumeID]
	if state == nil {
		return
	}
	if state.active > 0 {
		state.active--
	}
	state.lastActivity = time.Now().UTC()
}

func (m *Manager) idleLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.unmountIdle()
		}
	}
}

func (m *Manager) unmountIdle() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	volumes, err := m.store.ListStorageVolumes(ctx)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, cfg := range volumes {
		if !cfg.AutoUnmount {
			continue
		}
		m.mu.Lock()
		state := m.runtime[cfg.ID]
		eligible := state != nil && state.active == 0 && !state.unmounting && state.mounted && now.Sub(state.lastActivity) >= time.Duration(cfg.IdleTimeoutSeconds)*time.Second
		m.mu.Unlock()
		if !eligible {
			continue
		}
		if err := m.Unmount(ctx, cfg.ID); err != nil {
			m.logger.Warn("no se pudo desmontar una unidad inactiva", "volume_id", cfg.ID, "name", cfg.Name, "error", err)
		}
	}
}

func (m *Manager) setUnmountResult(volumeID, mountPoint string, mounted bool, errText string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.runtime[volumeID]
	if state == nil {
		state = &runtimeState{}
		m.runtime[volumeID] = state
	}
	state.unmounting = false
	state.mountPoint = mountPoint
	state.mounted = mounted
	state.lastError = errText
	state.lastActivity = time.Now().UTC()
}

func statusLabel(online, mounted, active bool) string {
	if !online {
		return "Desconectada"
	}
	if active {
		return "En uso"
	}
	if mounted {
		return "Montada"
	}
	return "Desmontada"
}

func validCategory(category string) bool {
	switch category {
	case "documents", "photos", "multimedia", "mixed":
		return true
	default:
		return false
	}
}

func sanitizeVirtualRoot(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\\x00") {
		return ""
	}
	return value
}

func defaultMountPoint(root string, volume DiscoveredVolume) string {
	if runtime.GOOS == "windows" {
		return volume.MountPoint
	}
	name := strings.TrimPrefix(volume.PersistentID, "uuid:")
	name = strings.ReplaceAll(name, string(filepath.Separator), "-")
	if len(name) > 36 {
		name = name[:36]
	}
	return filepath.Join(root, name)
}

func storageIdentityChanged(cfg store.StorageVolume, detected DiscoveredVolume) bool {
	if detected.Device != "" && detected.Device != cfg.Device {
		return true
	}
	if detected.VolumeName != "" && detected.VolumeName != cfg.VolumeName {
		return true
	}
	if detected.MountPoint != "" && detected.MountPoint != cfg.PreferredMountPoint {
		return true
	}
	if detected.FSType != "" && detected.FSType != cfg.FSType {
		return true
	}
	if detected.Label != "" && detected.Label != cfg.Label {
		return true
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
