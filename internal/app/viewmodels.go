package app

import (
	"time"

	"personalcloud/internal/catalog"
	"personalcloud/internal/storage"
)

type storagePageItem struct {
	storage.View
	Job            catalog.JobStatus
	JobPercent     int
	SuggestedRoot  string
	DamagedPending int
}

type mediaPageItem struct {
	catalog.File
	ThumbnailURL string `json:"thumbnail_url"`
	PreviewURL   string `json:"preview_url"`
	OriginalURL  string `json:"original_url"`
}

type explorerItem struct {
	ID          string    `json:"id,omitempty"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mod_time"`
	URL         string    `json:"url"`
	DownloadURL string    `json:"download_url"`
	IsDir       bool      `json:"is_dir"`
	Offline     bool      `json:"offline"`
	StorageName string    `json:"storage_name"`
	Health      string    `json:"health,omitempty"`
}

type breadcrumbItem struct {
	Name string
	URL  string
}

type explorerRoot struct {
	Name       string
	URL        string
	Category   string
	Status     string
	FileCount  int
	TotalBytes int64
	Offline    bool
}

type dashboardStats struct {
	Volumes int
	Online  int
	Files   int
	Photos  int
	Bytes   int64
}
