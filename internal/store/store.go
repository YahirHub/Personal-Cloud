package store

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound    = errors.New("registro no encontrado")
	ErrAdminExists = errors.New("ya existe un administrador")
)

const stateVersion = 2

type Store struct {
	mu        sync.RWMutex
	path      string
	auditPath string
	state     persistedState
}

type persistedState struct {
	Version  int             `json:"version"`
	Users    []User          `json:"users"`
	Sessions []Session       `json:"sessions"`
	Volumes  []StorageVolume `json:"volumes,omitempty"`
}

type User struct {
	ID                  string    `json:"id"`
	Username            string    `json:"username"`
	PasswordHash        string    `json:"password_hash"`
	Role                string    `json:"role"`
	OnboardingCompleted bool      `json:"onboarding_completed"`
	CreatedAt           time.Time `json:"created_at"`
}

type StorageVolume struct {
	ID                  string    `json:"id"`
	PersistentID        string    `json:"persistent_id"`
	IdentityStable      bool      `json:"identity_stable"`
	Name                string    `json:"name"`
	Label               string    `json:"label,omitempty"`
	Platform            string    `json:"platform"`
	Device              string    `json:"device,omitempty"`
	VolumeName          string    `json:"volume_name,omitempty"`
	PreferredMountPoint string    `json:"preferred_mount_point,omitempty"`
	FSType              string    `json:"fs_type,omitempty"`
	Category            string    `json:"category"`
	VirtualRoot         string    `json:"virtual_root"`
	IdleTimeoutSeconds  int       `json:"idle_timeout_seconds"`
	AutoUnmount         bool      `json:"auto_unmount"`
	ReadOnly            bool      `json:"read_only"`
	RegisteredAt        time.Time `json:"registered_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TokenHash string    `json:"token_hash"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type AuditEvent struct {
	ID          string    `json:"id"`
	ActorUserID string    `json:"actor_user_id,omitempty"`
	Action      string    `json:"action"`
	Outcome     string    `json:"outcome"`
	RemoteIP    string    `json:"remote_ip"`
	CreatedAt   time.Time `json:"created_at"`
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("ruta de persistencia vacía")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("crear directorio de datos: %w", err)
	}

	s := &Store{
		path:      path,
		auditPath: filepath.Join(filepath.Dir(path), "audit.jsonl"),
	}
	state, err := loadState(path)
	if err != nil {
		return nil, err
	}
	state, changed, err := migrateState(state)
	if err != nil {
		return nil, err
	}
	if err := validateState(state); err != nil {
		return nil, fmt.Errorf("validar estado persistido: %w", err)
	}
	s.state = state

	if changed || !fileExists(path) {
		if err := writeStateAtomic(path, state); err != nil {
			return nil, fmt.Errorf("inicializar estado: %w", err)
		}
	}
	return s, nil
}

func (s *Store) Close() error { return nil }

func (s *Store) AdminExists(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.state.Users {
		if user.Role == "admin" {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) CreateFirstAdmin(ctx context.Context, username, passwordHash string) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	var created User
	err := s.mutate(ctx, func(next *persistedState) error {
		for _, user := range next.Users {
			if user.Role == "admin" {
				return ErrAdminExists
			}
			if strings.EqualFold(user.Username, username) {
				return errors.New("el usuario ya existe")
			}
		}

		id, err := randomID(16)
		if err != nil {
			return err
		}
		created = User{
			ID:           id,
			Username:     username,
			PasswordHash: passwordHash,
			Role:         "admin",
			CreatedAt:    time.Now().UTC(),
		}
		next.Users = append(next.Users, created)
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return created, nil
}

func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.state.Users {
		if strings.EqualFold(user.Username, username) {
			return user, nil
		}
	}
	return User{}, ErrNotFound
}

func (s *Store) UserBySessionTokenHash(ctx context.Context, tokenHash string) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	now := time.Now().UTC()
	s.mu.RLock()
	defer s.mu.RUnlock()

	var userID string
	for _, session := range s.state.Sessions {
		if session.TokenHash == tokenHash && session.ExpiresAt.After(now) {
			userID = session.UserID
			break
		}
	}
	if userID == "" {
		return User{}, ErrNotFound
	}
	for _, user := range s.state.Users {
		if user.ID == userID {
			return user, nil
		}
	}
	return User{}, ErrNotFound
}

func (s *Store) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.mutate(ctx, func(next *persistedState) error {
		userExists := false
		for _, user := range next.Users {
			if user.ID == userID {
				userExists = true
				break
			}
		}
		if !userExists {
			return ErrNotFound
		}
		for _, session := range next.Sessions {
			if session.TokenHash == tokenHash {
				return errors.New("token de sesión duplicado")
			}
		}
		id, err := randomID(16)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		next.Sessions = append(next.Sessions, Session{
			ID:        id,
			UserID:    userID,
			TokenHash: tokenHash,
			ExpiresAt: expiresAt.UTC(),
			CreatedAt: now,
		})
		return nil
	})
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.mutate(ctx, func(next *persistedState) error {
		filtered := next.Sessions[:0]
		for _, session := range next.Sessions {
			if session.TokenHash != tokenHash {
				filtered = append(filtered, session)
			}
		}
		next.Sessions = filtered
		return nil
	})
}

func (s *Store) CompleteOnboarding(ctx context.Context, userID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.mutate(ctx, func(next *persistedState) error {
		for i := range next.Users {
			if next.Users[i].ID == userID {
				next.Users[i].OnboardingCompleted = true
				return nil
			}
		}
		return ErrNotFound
	})
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now().UTC()
	s.mu.RLock()
	hasExpired := false
	for _, session := range s.state.Sessions {
		if !session.ExpiresAt.After(now) {
			hasExpired = true
			break
		}
	}
	s.mu.RUnlock()
	if !hasExpired {
		return nil
	}
	return s.mutate(ctx, func(next *persistedState) error {
		filtered := next.Sessions[:0]
		for _, session := range next.Sessions {
			if session.ExpiresAt.After(now) {
				filtered = append(filtered, session)
			}
		}
		next.Sessions = filtered
		return nil
	})
}

func (s *Store) Audit(ctx context.Context, actorUserID, action, outcome, remoteIP string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id, err := randomID(16)
	if err != nil {
		return err
	}
	event := AuditEvent{
		ID:          id,
		ActorUserID: actorUserID,
		Action:      action,
		Outcome:     outcome,
		RemoteIP:    remoteIP,
		CreatedAt:   time.Now().UTC(),
	}
	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("serializar auditoría: %w", err)
	}
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.OpenFile(s.auditPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("abrir auditoría: %w", err)
	}
	_, writeErr := file.Write(line)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("escribir auditoría: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("cerrar auditoría: %w", closeErr)
	}
	return nil
}

func (s *Store) CleanupAudit(ctx context.Context, retention time.Duration, maxRows int) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.Open(s.auditPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("abrir auditoría: %w", err)
	}
	defer file.Close()

	cutoff := time.Time{}
	if retention > 0 {
		cutoff = time.Now().UTC().Add(-retention)
	}
	events := make([]AuditEvent, 0, 1024)
	changed := false
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var event AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return fmt.Errorf("auditoría dañada: %w", err)
		}
		if !cutoff.IsZero() && event.CreatedAt.Before(cutoff) {
			changed = true
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("leer auditoría: %w", err)
	}
	if maxRows > 0 && len(events) > maxRows {
		events = events[len(events)-maxRows:]
		changed = true
	}
	if !changed {
		return nil
	}

	var data []byte
	for _, event := range events {
		line, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("serializar auditoría: %w", err)
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	return writeFileAtomic(s.auditPath, data, 0o600)
}

func (s *Store) mutate(ctx context.Context, change func(*persistedState) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	next := cloneState(s.state)
	if err := change(&next); err != nil {
		return err
	}
	if err := validateState(next); err != nil {
		return fmt.Errorf("estado inválido: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeStateAtomic(s.path, next); err != nil {
		return err
	}
	s.state = next
	return nil
}

func cloneState(state persistedState) persistedState {
	return persistedState{
		Version:  state.Version,
		Users:    append([]User(nil), state.Users...),
		Sessions: append([]Session(nil), state.Sessions...),
		Volumes:  append([]StorageVolume(nil), state.Volumes...),
	}
}

func loadState(path string) (persistedState, error) {
	state, err := readState(path)
	if err == nil {
		return state, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return persistedState{}, fmt.Errorf("leer estado: %w", err)
	}

	backupPath := path + ".bak"
	backup, backupErr := readState(backupPath)
	if backupErr == nil {
		if err := writeStateAtomic(path, backup); err != nil {
			return persistedState{}, fmt.Errorf("restaurar backup de estado: %w", err)
		}
		return backup, nil
	}
	if !errors.Is(backupErr, os.ErrNotExist) {
		return persistedState{}, fmt.Errorf("leer backup de estado: %w", backupErr)
	}
	return persistedState{Version: stateVersion}, nil
}

func readState(path string) (persistedState, error) {
	file, err := os.Open(path)
	if err != nil {
		return persistedState{}, err
	}
	defer file.Close()

	var state persistedState
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&state); err != nil {
		return persistedState{}, fmt.Errorf("JSON inválido: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return persistedState{}, errors.New("contenido adicional después del estado JSON")
	} else if !errors.Is(err, io.EOF) {
		return persistedState{}, fmt.Errorf("leer final del estado JSON: %w", err)
	}
	return state, nil
}

func writeStateAtomic(path string, state persistedState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("serializar estado: %w", err)
	}
	data = append(data, '\n')
	if err := writeFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("persistir estado: %w", err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".personalcloud-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}

	backup := path + ".bak"
	hadCurrent := fileExists(path)
	if hadCurrent {
		_ = os.Remove(backup)
		if err := os.Rename(path, backup); err != nil {
			_ = os.Remove(tmpName)
			return fmt.Errorf("crear backup previo: %w", err)
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		if hadCurrent {
			_ = os.Rename(backup, path)
		}
		_ = os.Remove(tmpName)
		return fmt.Errorf("reemplazar archivo: %w", err)
	}
	if runtime.GOOS != "windows" {
		if directory, err := os.Open(dir); err == nil {
			_ = directory.Sync()
			_ = directory.Close()
		}
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func randomID(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generar id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
