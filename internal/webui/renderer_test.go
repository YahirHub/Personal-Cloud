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
	Title          string
	Description    string
	CurrentPath    string
	CSRFToken      string
	Error          string
	Info           string
	RetryAfter     int
	User           *renderUser
	Stats          renderStats
	HomeFolders    []any
	HomeFiles      []any
	StorageItems   []any
	StorageError   string
	StorageSummary struct {
		Total       uint64
		Used        uint64
		Free        uint64
		PercentUsed int
		OnlineUnits int
	}
	StorageUsageItems      []any
	StorageLargestFiles    []any
	Media                  []any
	ExplorerItems          []any
	ExplorerFolders        []any
	ExplorerFiles          []any
	ExplorerRoots          []any
	Breadcrumbs            []any
	ExplorerPath           string
	ExplorerRoot           string
	ExplorerRelative       string
	ExplorerCanWrite       bool
	MediaOffset            int
	MediaNext              int
	MediaHasMore           bool
	MediaTotal             int
	ListingMode            string
	ListingBaseURL         string
	ListingInfiniteURL     string
	ListingPagesURL        string
	ListingPrevURL         string
	ListingNextURL         string
	GalleryType            string
	GallerySort            string
	GalleryFilters         int
	ListingPage            int
	ListingPrev            int
	ListingNext            int
	ListingHasPrev         bool
	ListingHasNext         bool
	ExplorerHasMore        bool
	ExplorerNext           int
	SearchQuery            string
	SearchMode             bool
	FileCollection         string
	FileCollectionTitle    string
	FileCollectionSubtitle string
	FileTypeFilter         string
	FileModifiedFilter     string
	FileSourceFilter       string
	FileFilterAction       string
	FileFilterCount        int
	MaxUploadBytes         int64
	MaxUploadBatchFiles    int
	MoveDestinations       []struct {
		ID, Name, VirtualRoot, Category string
		Online, ReadOnly                bool
	}
	Settings struct {
		SyncIntervalMinutes int
		LastSyncAt          time.Time
	}
	SettingsSyncText string
	IntegrityUnits   []any
	PublicSharePage  bool
	Shares           []any
}

func TestRenderPages(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	pages := []string{"setup", "login", "onboarding", "dashboard", "storage", "files", "photos", "settings", "shared"}
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
			html := out.String()
			if !strings.Contains(html, "<!doctype html>") {
				t.Fatal("salida HTML incompleta")
			}
			if !strings.Contains(html, "Powered by ThotiLabs.com") {
				t.Fatal("el layout global debe incluir el footer de ThotiLabs")
			}
		})
	}
}

func TestPercentUsesProvidedPortion(t *testing.T) {
	if got := percent(65, 100); got != 65 {
		t.Fatalf("percent debe representar la porción recibida; got=%d", got)
	}
	if got := percent(150, 100); got != 100 {
		t.Fatalf("percent debe limitar valores mayores al total; got=%d", got)
	}
	if got := percent(1, 0); got != 0 {
		t.Fatalf("percent con total cero debe ser 0; got=%d", got)
	}
}

func TestRenderStorageProgressRepresentsUsedAndFree(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	data := renderData{Title: "Almacenamiento", Description: "Prueba", CSRFToken: "token", FileFilterAction: "/almacenamiento"}
	data.User = &renderUser{Username: "admin", Role: "admin"}
	data.StorageSummary.Total = 100
	data.StorageSummary.Used = 44
	data.StorageSummary.Free = 56
	data.StorageSummary.PercentUsed = 44
	data.StorageSummary.OnlineUnits = 1
	data.StorageUsageItems = []any{map[string]any{
		"ID": "unit-1", "Name": "Unidad", "VirtualRoot": "unidad", "FSType": "ext4",
		"ReadOnly": false, "PercentUsed": 44, "Used": uint64(44), "Free": uint64(56), "Capacity": uint64(100),
	}}

	var out bytes.Buffer
	if err := renderer.Render(&out, "storage", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, expected := range []string{
		`<progress class="drive-storage-track" max="100" value="44"`,
		`<progress class="drive-storage-unit-track" max="100" value="44"`,
		`44% usado y 56% libre`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("barra de almacenamiento renderizada incorrectamente; falta %q", expected)
		}
	}
	if strings.Contains(html, "ZgotmplZ") {
		t.Fatal("la barra de almacenamiento no debe depender de estilos inline rechazados por html/template/CSP")
	}
}
