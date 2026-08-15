package app

import (
	"time"

	"personalcloud/internal/catalog"
	"personalcloud/internal/storage"
)

type storagePageItem struct {
	storage.View
	Job           catalog.JobStatus
	SuggestedRoot string
}

type photoPageItem struct {
	catalog.File
	ThumbnailURL string
	PreviewURL   string
	OriginalURL  string
}

type explorerItem struct {
	Name        string
	Kind        string
	Size        int64
	ModTime     time.Time
	URL         string
	DownloadURL string
	IsDir       bool
	Offline     bool
	StorageName string
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
