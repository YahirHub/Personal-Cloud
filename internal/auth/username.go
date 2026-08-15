package auth

import (
	"errors"
	"regexp"
	"strings"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)

func NormalizeUsername(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !usernamePattern.MatchString(value) {
		return "", errors.New("el usuario debe tener entre 3 y 32 caracteres y usar letras, números, punto, guion o guion bajo")
	}
	return value, nil
}
