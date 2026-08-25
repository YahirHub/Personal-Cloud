package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"personalcloud/internal/config"
)

func TestAbsoluteURLUsesConfiguredAppURL(t *testing.T) {
	application := &App{cfg: config.Config{AppURL: "https://ncloud.admvo.org"}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8736/compartidos", nil)
	got := application.absoluteURL(req, "/s/abc/embed")
	if got != "https://ncloud.admvo.org/s/abc/embed" {
		t.Fatalf("URL pública inesperada: %q", got)
	}
}

func TestAbsoluteURLFallsBackToRequestHost(t *testing.T) {
	application := &App{cfg: config.Config{}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	req := httptest.NewRequest(http.MethodGet, "https://ncloud.admvo.org/compartidos", nil)
	got := application.absoluteURL(req, "/s/abc")
	if got != "https://ncloud.admvo.org/s/abc" {
		t.Fatalf("fallback inesperado: %q", got)
	}
}
