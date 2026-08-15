package storage

import (
	"path/filepath"
	"strings"
)

func FileKind(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".heic", ".heif", ".avif", ".dng", ".cr2", ".nef", ".arw":
		return "image"
	case ".mp4", ".mkv", ".mov", ".avi", ".webm", ".m4v":
		return "video"
	case ".mp3", ".flac", ".wav", ".m4a", ".ogg", ".opus", ".aac":
		return "audio"
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".md", ".odt", ".ods":
		return "document"
	case ".zip", ".7z", ".rar", ".tar", ".gz", ".bz2", ".xz":
		return "archive"
	default:
		return "other"
	}
}

func CategoryAllowsFile(category, name string) bool {
	kind := FileKind(name)
	switch category {
	case "photos":
		return kind == "image"
	case "multimedia":
		return kind == "image" || kind == "video" || kind == "audio"
	case "documents":
		return kind == "document" || kind == "archive" || kind == "other"
	case "mixed":
		return true
	default:
		return false
	}
}
