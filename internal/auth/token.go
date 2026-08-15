package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"strings"
)

func NewSessionToken() (plain string, digest string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generar token de sesión: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(buf)
	return plain, HashToken(plain), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func NewSetupCode() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generar código de setup: %w", err)
	}
	encoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	value := strings.TrimRight(encoder.EncodeToString(buf), "=")
	if len(value) > 12 {
		value = value[:12]
	}
	return strings.Join([]string{value[0:4], value[4:8], value[8:12]}, "-"), nil
}
