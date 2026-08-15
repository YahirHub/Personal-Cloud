package app

import (
	"testing"
	"time"

	"personalcloud/internal/store"
)

func TestWebDAVAuthCacheDoesNotStorePlainCredential(t *testing.T) {
	a := &App{davAuthCache: make(map[string]davAuthCacheEntry)}
	copy(a.davAuthSecret[:], []byte("0123456789abcdef0123456789abcdef"))
	user := store.User{ID: "u1", Username: "usuario", PasswordHash: "hash-v1"}
	now := time.Now()
	a.webDAVRememberAuth(user, "secreto-no-debe-quedar", now)
	if !a.webDAVAuthCached(user, "secreto-no-debe-quedar", now.Add(time.Minute)) {
		t.Fatal("credencial válida no fue recuperada desde caché")
	}
	if a.webDAVAuthCached(user, "otra", now.Add(time.Minute)) {
		t.Fatal("una contraseña distinta no debe acertar en caché")
	}
	for key := range a.davAuthCache {
		if key == "secreto-no-debe-quedar" {
			t.Fatal("la contraseña no debe usarse como clave directa")
		}
	}
}

func TestWebDAVAuthCacheInvalidatesPasswordChange(t *testing.T) {
	a := &App{davAuthCache: make(map[string]davAuthCacheEntry)}
	copy(a.davAuthSecret[:], []byte("0123456789abcdef0123456789abcdef"))
	user := store.User{ID: "u1", Username: "usuario", PasswordHash: "hash-v1"}
	now := time.Now()
	a.webDAVRememberAuth(user, "secreto", now)
	user.PasswordHash = "hash-v2"
	if a.webDAVAuthCached(user, "secreto", now.Add(time.Minute)) {
		t.Fatal("cambio de hash debe invalidar la caché")
	}
}

func TestSuggestedVirtualRoot(t *testing.T) {
	got := suggestedVirtualRoot(`Fotos y videos: USB/1`)
	if got != "Fotos-y-videos-USB-1" {
		t.Fatalf("raíz sugerida inesperada: %q", got)
	}
}
