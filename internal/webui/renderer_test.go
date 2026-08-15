package webui

import (
	"bytes"
	"strings"
	"testing"
)

type renderUser struct {
	Username string
	Role     string
}

type renderData struct {
	Title       string
	Description string
	CurrentPath string
	CSRFToken   string
	Error       string
	Info        string
	RetryAfter  int
	User        *renderUser
}

func TestRenderPages(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	pages := []string{"setup", "login", "onboarding", "dashboard", "storage", "photos"}
	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			data := renderData{Title: "Prueba", Description: "Prueba", CSRFToken: "token"}
			if page != "setup" && page != "login" {
				data.User = &renderUser{Username: "admin", Role: "admin"}
			}
			var out bytes.Buffer
			if err := renderer.Render(&out, page, data); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "<!doctype html>") {
				t.Fatal("salida HTML incompleta")
			}
		})
	}
}
