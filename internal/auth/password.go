package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	passwordScheme      = "pbkdf2-sha256"
	pbkdf2Iterations    = 600_000
	pbkdf2SaltLength    = 16
	pbkdf2KeyLength     = 32
	pbkdf2MinIterations = 100_000
	pbkdf2MaxIterations = 2_000_000
)

func ValidatePassword(password string) error {
	if len(password) < 12 {
		return errors.New("la contraseña debe tener al menos 12 caracteres")
	}
	if len(password) > 128 {
		return errors.New("la contraseña es demasiado larga")
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	return hashPassword(password)
}

// HashSharePassword usa el mismo formato robusto que las contraseñas de usuario,
// pero permite secretos más cortos para enlaces compartidos sin relajar el login.
func HashSharePassword(password string) (string, error) {
	if len(password) < 6 {
		return "", errors.New("la contraseña del enlace debe tener al menos 6 caracteres")
	}
	if len(password) > 128 {
		return "", errors.New("la contraseña del enlace es demasiado larga")
	}
	return hashPassword(password)
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, pbkdf2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generar salt: %w", err)
	}

	key := pbkdf2SHA256([]byte(password), salt, pbkdf2Iterations, pbkdf2KeyLength)
	return fmt.Sprintf("$%s$i=%d$%s$%s",
		passwordScheme,
		pbkdf2Iterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "" || parts[1] != passwordScheme {
		return false, errors.New("hash de contraseña inválido")
	}

	setting := strings.SplitN(parts[2], "=", 2)
	if len(setting) != 2 || setting[0] != "i" {
		return false, errors.New("parámetros PBKDF2 inválidos")
	}
	iterations64, err := strconv.ParseUint(setting[1], 10, 32)
	if err != nil || iterations64 < pbkdf2MinIterations || iterations64 > pbkdf2MaxIterations {
		return false, errors.New("iteraciones PBKDF2 inválidas")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false, errors.New("salt PBKDF2 inválido")
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return false, errors.New("clave PBKDF2 inválida")
	}

	actual := pbkdf2SHA256([]byte(password), salt, int(iterations64), len(expected))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

// pbkdf2SHA256 implementa PBKDF2 con HMAC-SHA-256 usando únicamente la
// biblioteca estándar. El formato persistido incluye esquema e iteraciones para
// permitir aumentar el factor de trabajo en versiones futuras.
func pbkdf2SHA256(password, salt []byte, iterations, keyLength int) []byte {
	const hLen = sha256.Size
	blocks := (keyLength + hLen - 1) / hLen
	derived := make([]byte, 0, blocks*hLen)
	var counter [4]byte

	for block := 1; block <= blocks; block++ {
		binary.BigEndian.PutUint32(counter[:], uint32(block))

		mac := hmac.New(sha256.New, password)
		_, _ = mac.Write(salt)
		_, _ = mac.Write(counter[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)

		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha256.New, password)
			_, _ = mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		derived = append(derived, t...)
	}

	return derived[:keyLength]
}
