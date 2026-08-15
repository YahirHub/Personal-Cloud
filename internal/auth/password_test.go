package auth

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestPBKDF2SHA256KnownVector(t *testing.T) {
	got := pbkdf2SHA256([]byte("password"), []byte("salt"), 1, 32)
	const want = "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"
	if hex.EncodeToString(got) != want {
		t.Fatalf("vector PBKDF2 inesperado: %x", got)
	}
}

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("una-contraseña-larga-123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$pbkdf2-sha256$i=600000$") {
		t.Fatalf("formato de hash inesperado: %q", hash)
	}
	ok, err := VerifyPassword(hash, "una-contraseña-larga-123")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("la contraseña correcta no validó")
	}
	ok, err = VerifyPassword(hash, "otra-contraseña-larga")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("una contraseña incorrecta validó")
	}
}

func TestVerifyPasswordRejectsExcessiveWorkFactor(t *testing.T) {
	encoded := "$pbkdf2-sha256$i=999999999$c2FsdC1zYWx0LXNhbHQ$AAAAAAAAAAAAAAAAAAAAAA"
	if _, err := VerifyPassword(encoded, "una-contraseña-larga-123"); err == nil {
		t.Fatal("debía rechazar un factor de trabajo abusivo")
	}
}

func TestNormalizeUsername(t *testing.T) {
	got, err := NormalizeUsername("  Mimi.Admin ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "mimi.admin" {
		t.Fatalf("resultado inesperado: %q", got)
	}
}
