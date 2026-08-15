package webdav

import (
	"context"
	"testing"
)

func TestVirtualPath(t *testing.T) {
	s := &Server{Prefix: "/webdav"}
	cases := map[string]string{
		"/webdav":             "/",
		"/webdav/":            "/",
		"/webdav/Fotos/a.jpg": "/Fotos/a.jpg",
	}
	for input, want := range cases {
		got, ok := s.virtualPath(input)
		if !ok || got != want {
			t.Fatalf("virtualPath(%q)=%q,%v want %q", input, got, ok, want)
		}
	}
	if _, ok := s.virtualPath("/otra"); ok {
		t.Fatal("ruta ajena aceptada")
	}
}

func TestLockManager(t *testing.T) {
	m := newLockManager()
	token, err := m.lock("/Fotos/a.jpg", parseLockTimeout("Second-120"), "")
	if err != nil {
		t.Fatal(err)
	}
	if m.allowed("/Fotos/a.jpg", "") {
		t.Fatal("lock ignorado")
	}
	if !m.allowed("/Fotos/a.jpg", token) {
		t.Fatal("token válido rechazado")
	}
	if !m.unlock("/Fotos/a.jpg", token) {
		t.Fatal("unlock falló")
	}
}

func TestMutationCallback(t *testing.T) {
	var got string
	s := &Server{OnMutation: func(_ context.Context, path string) { got = path }}
	s.notifyMutation(context.Background(), "/Fotos/a.jpg")
	if got != "/Fotos/a.jpg" {
		t.Fatalf("callback inesperado: %q", got)
	}
}
