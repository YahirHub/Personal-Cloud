package webui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

type renderUser struct {
	Username string
	Role     string
}

type renderStats struct {
	Volumes int
	Online  int
	Files   int
	Photos  int
	Bytes   int64
}

type renderData struct {
	Title              string
	Description        string
	CurrentPath        string
	CSRFToken          string
	Error              string
	Info               string
	RetryAfter         int
	User               *renderUser
	Stats              renderStats
	HomeFolders        []any
	HomeFiles          []any
	StorageItems       []any
	StorageError       string
	Media              []any
	ExplorerItems      []any
	ExplorerRoots      []any
	Breadcrumbs        []any
	ExplorerPath       string
	ExplorerCanWrite   bool
	MediaOffset        int
	MediaNext          int
	MediaHasMore       bool
	MediaTotal         int
	ListingMode        string
	ListingBaseURL     string
	ListingInfiniteURL string
	ListingPagesURL    string
	ListingPrevURL     string
	ListingNextURL     string
	GalleryType        string
	GallerySort        string
	GalleryFilters     int
	ListingPage        int
	ListingPrev        int
	ListingNext        int
	ListingHasPrev     bool
	ListingHasNext     bool
	ExplorerHasMore    bool
	ExplorerNext       int
	SearchQuery        string
	SearchMode         bool
	MaxUploadBytes     int64
	MoveDestinations   []struct {
		ID, Name, VirtualRoot, Category string
		Online, ReadOnly                bool
	}
	Settings struct {
		SyncIntervalMinutes int
		LastSyncAt          time.Time
	}
	SettingsSyncText string
	IntegrityUnits   []any
}

func TestRenderPages(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	pages := []string{"setup", "login", "onboarding", "dashboard", "storage", "files", "photos", "settings"}
	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			data := renderData{Title: "Prueba", Description: "Prueba", CSRFToken: "token", ListingMode: "infinito", ListingBaseURL: "/galeria", ExplorerPath: "/"}
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
