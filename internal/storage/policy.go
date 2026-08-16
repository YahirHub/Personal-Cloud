package storage

import (
	"path/filepath"
	"strings"
)

// FileKind clasifica extensiones de forma centralizada para el catálogo y la UI.
// Se mantienen categorías amplias para filtros/permisos, mientras que FileIcon
// aporta una presentación mucho más específica sin depender de recursos remotos.
func FileKind(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".jpe", ".png", ".gif", ".webp", ".heic", ".heif", ".avif", ".bmp", ".tif", ".tiff", ".svg", ".ico", ".dng", ".cr2", ".cr3", ".nef", ".nrw", ".arw", ".raf", ".rw2", ".orf":
		return "image"
	case ".mp4", ".mkv", ".mov", ".avi", ".webm", ".m4v", ".mpeg", ".mpg", ".mpe", ".3gp", ".3g2", ".mts", ".m2ts", ".ts", ".vob", ".ogv":
		return "video"
	case ".mp3", ".flac", ".wav", ".m4a", ".ogg", ".oga", ".opus", ".aac", ".wma", ".aiff", ".aif", ".alac", ".mid", ".midi":
		return "audio"
	case ".pdf", ".doc", ".docx", ".docm", ".dot", ".dotx", ".rtf", ".xls", ".xlsx", ".xlsm", ".xlsb", ".csv", ".tsv", ".ppt", ".pptx", ".pptm", ".pps", ".ppsx", ".txt", ".text", ".md", ".markdown", ".mdown", ".mkd", ".rst", ".tex", ".log", ".odt", ".ods", ".odp", ".pages", ".numbers", ".key", ".epub", ".mobi", ".azw", ".azw3", ".vcf", ".ics", ".html", ".htm", ".xhtml":
		return "document"
	case ".zip", ".7z", ".rar", ".tar", ".gz", ".tgz", ".bz2", ".tbz", ".tbz2", ".xz", ".txz", ".zst", ".lz", ".lzma", ".cab", ".iso", ".img":
		return "archive"
	default:
		return "other"
	}
}

// FileIcon devuelve una clave visual y una etiqueta corta para representar el
// tipo real del archivo. La UI dibuja estos iconos localmente; no hay llamadas
// a CDN en tiempo de ejecución.
func FileIcon(name, kind string) (key, label string) {
	ext := strings.ToLower(filepath.Ext(name))
	if ext != "" {
		ext = strings.TrimPrefix(ext, ".")
	}
	switch ext {
	case "apk", "aab", "xapk", "apks":
		return "android", strings.ToUpper(ext)
	case "pdf":
		return "pdf", "PDF"
	case "doc", "docx", "docm", "dot", "dotx", "rtf":
		return "word", "DOC"
	case "xls", "xlsx", "xlsm", "xlsb", "csv", "tsv":
		return "excel", "XLS"
	case "ppt", "pptx", "pptm", "pps", "ppsx":
		return "powerpoint", "PPT"
	case "odt", "ods", "odp":
		return "office", strings.ToUpper(ext)
	case "md", "markdown", "mdown", "mkd":
		return "markdown", "MD"
	case "txt", "text", "log", "rst", "tex":
		return "text", strings.ToUpper(ext)
	case "json", "jsonl", "yaml", "yml", "toml", "xml":
		return "data", strings.ToUpper(ext)
	case "html", "htm", "xhtml", "css", "scss", "sass", "less", "js", "mjs", "cjs", "ts", "tsx", "jsx", "vue", "svelte", "php", "py", "pyw", "go", "rs", "java", "kt", "kts", "c", "h", "cpp", "hpp", "cs", "swift", "dart", "rb", "sh", "bash", "zsh", "ps1", "bat", "cmd", "sql", "graphql":
		return "code", strings.ToUpper(ext)
	case "db", "sqlite", "sqlite3", "mdb", "accdb":
		return "database", "DB"
	case "zip", "7z", "rar", "tar", "gz", "tgz", "bz2", "xz", "zst", "cab":
		return "archive", strings.ToUpper(ext)
	case "iso", "img", "dmg":
		return "disk", strings.ToUpper(ext)
	case "exe", "msi", "msix", "appx", "appxbundle", "deb", "rpm", "pkg", "appimage":
		return "executable", strings.ToUpper(ext)
	case "epub", "mobi", "azw", "azw3":
		return "ebook", strings.ToUpper(ext)
	case "vcf":
		return "contact", "VCF"
	case "ics":
		return "calendar", "ICS"
	case "ttf", "otf", "woff", "woff2", "eot":
		return "font", strings.ToUpper(ext)
	case "pem", "crt", "cer", "key", "p12", "pfx", "jks", "keystore":
		return "certificate", strings.ToUpper(ext)
	}

	switch kind {
	case "image":
		if ext != "" {
			return "image", shortExtension(ext)
		}
		return "image", "IMG"
	case "video":
		if ext != "" {
			return "video", shortExtension(ext)
		}
		return "video", "VID"
	case "audio":
		if ext != "" {
			return "audio", shortExtension(ext)
		}
		return "audio", "AUD"
	case "archive":
		return "archive", fallbackExtension(ext, "ZIP")
	case "document":
		return "document", fallbackExtension(ext, "DOC")
	default:
		return "file", fallbackExtension(ext, "FILE")
	}
}

func shortExtension(ext string) string {
	upper := strings.ToUpper(ext)
	if len(upper) > 4 {
		return upper[:4]
	}
	return upper
}

func fallbackExtension(ext, fallback string) string {
	if ext == "" {
		return fallback
	}
	return shortExtension(ext)
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
