package app

import (
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

type dashboardStats struct {
	Volumes int
	Online  int
	Files   int
	Photos  int
	Bytes   int64
}
