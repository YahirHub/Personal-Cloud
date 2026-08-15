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

	sessionIDs := make(map[string]struct{}, len(state.Sessions))
	tokenHashes := make(map[string]struct{}, len(state.Sessions))
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
