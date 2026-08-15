package store

import (
	"errors"
	"fmt"
	"strings"
)

func migrateState(state persistedState) (persistedState, bool, error) {
	if state.Version == 0 {
		state.Version = stateVersion
		return state, true, nil
	}
	if state.Version > stateVersion {
		return persistedState{}, false, fmt.Errorf("estado versión %d no soportado por esta versión del servidor", state.Version)
	}
	if state.Version == 1 {
		state.Version = 2
	}
	if state.Version == 2 {
		state.Version = 3
		return state, true, nil
	}
	if state.Version < stateVersion {
		return persistedState{}, false, fmt.Errorf("falta migración de estado desde versión %d", state.Version)
	}
	return state, false, nil
}

func validateState(state persistedState) error {
	if state.Version != stateVersion {
		return fmt.Errorf("versión de estado inválida: %d", state.Version)
	}

	userIDs := make(map[string]struct{}, len(state.Users))
	usernames := make(map[string]struct{}, len(state.Users))
	adminCount := 0
	for _, user := range state.Users {
		if user.ID == "" || user.Username == "" || user.PasswordHash == "" {
			return errors.New("usuario incompleto")
		}
		if user.Role != "admin" && user.Role != "user" {
			return fmt.Errorf("rol inválido para %q", user.Username)
		}
		if user.Role == "admin" {
			adminCount++
		}
		if _, exists := userIDs[user.ID]; exists {
			return errors.New("id de usuario duplicado")
		}
		userIDs[user.ID] = struct{}{}
		key := strings.ToLower(user.Username)
		if _, exists := usernames[key]; exists {
			return errors.New("nombre de usuario duplicado")
		}
		usernames[key] = struct{}{}
	}
	if adminCount > 1 {
		return errors.New("hay más de un administrador bootstrap")
	}
	if state.Settings.SyncIntervalMinutes < 0 || state.Settings.SyncIntervalMinutes > 10080 || (state.Settings.SyncIntervalMinutes > 0 && state.Settings.SyncIntervalMinutes < 5) {
		return errors.New("intervalo de sincronización inválido")
	}

	sessionIDs := make(map[string]struct{}, len(state.Sessions))
	tokenHashes := make(map[string]struct{}, len(state.Sessions))
	volumeIDs := make(map[string]struct{}, len(state.Volumes))
	persistentIDs := make(map[string]struct{}, len(state.Volumes))
	virtualRoots := make(map[string]struct{}, len(state.Volumes))
	for _, volume := range state.Volumes {
		if volume.ID == "" || strings.TrimSpace(volume.PersistentID) == "" || strings.TrimSpace(volume.Name) == "" {
			return errors.New("unidad registrada incompleta")
		}
		if !validStorageCategory(volume.Category) {
			return fmt.Errorf("categoría de almacenamiento inválida para %q", volume.Name)
		}
		if !validVirtualRoot(volume.VirtualRoot) {
			return fmt.Errorf("raíz virtual inválida para %q", volume.Name)
		}
		if volume.IdleTimeoutSeconds < 30 || volume.IdleTimeoutSeconds > 7*24*60*60 {
			return fmt.Errorf("timeout de inactividad inválido para %q", volume.Name)
		}
		if _, ok := volumeIDs[volume.ID]; ok {
			return errors.New("id de unidad duplicado")
		}
		volumeIDs[volume.ID] = struct{}{}
		pk := strings.ToLower(strings.TrimSpace(volume.PersistentID))
		if _, ok := persistentIDs[pk]; ok {
			return errors.New("identidad persistente de unidad duplicada")
		}
		persistentIDs[pk] = struct{}{}
		rk := strings.ToLower(strings.TrimSpace(volume.VirtualRoot))
		if _, ok := virtualRoots[rk]; ok {
			return errors.New("raíz virtual duplicada")
		}
		virtualRoots[rk] = struct{}{}
	}

	for _, session := range state.Sessions {
		if session.ID == "" || session.UserID == "" || session.TokenHash == "" {
			return errors.New("sesión incompleta")
		}
		if _, exists := userIDs[session.UserID]; !exists {
			return errors.New("sesión referencia un usuario inexistente")
		}
		if _, exists := sessionIDs[session.ID]; exists {
			return errors.New("id de sesión duplicado")
		}
		sessionIDs[session.ID] = struct{}{}
		if _, exists := tokenHashes[session.TokenHash]; exists {
			return errors.New("hash de token duplicado")
		}
		tokenHashes[session.TokenHash] = struct{}{}
	}
	return nil
}

func validStorageCategory(category string) bool {
	switch category {
	case "documents", "photos", "multimedia", "mixed":
		return true
	default:
		return false
	}
}

func validVirtualRoot(root string) bool {
	root = strings.TrimSpace(root)
	if root == "" || root == "." || root == ".." || len(root) > 80 {
		return false
	}
	return !strings.ContainsAny(root, "/\\\x00")
}
