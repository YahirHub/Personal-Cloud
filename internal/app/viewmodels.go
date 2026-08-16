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
	Starred      bool   `json:"starred"`
}

type explorerItem struct {
	ID           string    `json:"id,omitempty"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	Size         int64     `json:"size"`
	ModTime      time.Time `json:"mod_time"`
	URL          string    `json:"url"`
	DownloadURL  string    `json:"download_url"`
	ThumbnailURL string    `json:"thumbnail_url,omitempty"`
	Location     string    `json:"location,omitempty"`
	IsDir        bool      `json:"is_dir"`
	Offline      bool      `json:"offline"`
	StorageName  string    `json:"storage_name"`
	VirtualRoot  string    `json:"virtual_root,omitempty"`
	Health       string    `json:"health,omitempty"`
	Starred      bool      `json:"starred,omitempty"`
	IconKey      string    `json:"icon_key,omitempty"`
	IconLabel    string    `json:"icon_label,omitempty"`
}

type breadcrumbItem struct {
	Name string
	URL  string
}

type explorerRoot struct {
	ID          string
	Name        string
	StorageName string
	URL         string
	Category    string
	Status      string
	FileCount   int
	TotalBytes  int64
	Capacity    uint64
	Free        uint64
	Mounted     bool
	ReadOnly    bool
	Offline     bool
}

type integrityUnitView struct {
	ID             string
	Name           string
	VirtualRoot    string
	Online         bool
	Damaged        int
	DamagedPending int
	Unchecked      int
	Healthy        int
	Samples        []catalog.File
	Job            catalog.JobStatus
	JobPercent     int
}

type dashboardStats struct {
	Volumes int
	Online  int
	Files   int
	Photos  int
	Bytes   int64
}

type homeFileItem struct {
	ID           string
	Name         string
	Kind         string
	Size         int64
	ModTime      time.Time
	VirtualRoot  string
	ThumbnailURL string
	OpenURL      string
	Offline      bool
	Health       string
	IconKey      string
	IconLabel    string
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func decorateExplorerFile(item *explorerItem) {
	if item == nil || item.IsDir {
		return
	}
	item.IconKey, item.IconLabel = storage.FileIcon(item.Name, item.Kind)
}

func decorateHomeFile(item *homeFileItem) {
	if item == nil {
		return
	}
	item.IconKey, item.IconLabel = storage.FileIcon(item.Name, item.Kind)
}

func splitExplorerItems(items []explorerItem) (folders, files []explorerItem) {
	folders = make([]explorerItem, 0)
	files = make([]explorerItem, 0)
	for _, item := range items {
		if item.IsDir {
			folders = append(folders, item)
		} else {
			files = append(files, item)
		}
	}
	return folders, files
}
