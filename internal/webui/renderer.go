package webui

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"math"
	"strings"
	"time"

	webassets "personalcloud/web"
)

type Renderer struct {
	base *template.Template
}

func NewRenderer() (*Renderer, error) {
	funcs := template.FuncMap{
		"formatBytes":    formatBytes,
		"formatBytesU":   formatBytesU,
		"formatDuration": formatDuration,
		"formatTime":     formatTime,
		"categoryLabel":  categoryLabel,
		"statusClass":    statusClass,
		"percent":        percent,
		"sub": func(a, b int) int {
			if a < b {
				return 0
			}
			return a - b
		},
	}
	base, err := template.New("base").Funcs(funcs).ParseFS(webassets.Assets, "layouts/*.html", "components/*.html")
	if err != nil {
		return nil, fmt.Errorf("cargar layout: %w", err)
	}
	return &Renderer{base: base}, nil
}

func (r *Renderer) Render(w io.Writer, page string, data any) error {
	clone, err := r.base.Clone()
	if err != nil {
		return err
	}
	if _, err := clone.ParseFS(webassets.Assets, "pages/"+page+".html"); err != nil {
		return fmt.Errorf("cargar página %q: %w", page, err)
	}
	return clone.ExecuteTemplate(w, "base", data)
}

func StaticFS() (fs.FS, error) {
	return fs.Sub(webassets.Assets, "static")
}

func formatBytes(value int64) string {
	if value < 0 {
		value = 0
	}
	return formatBytesU(uint64(value))
}

func formatBytesU(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := uint64(unit), 0
	for n := value / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}

func formatDuration(seconds int) string {
	if seconds <= 0 {
		return "desactivado"
	}
	d := time.Duration(seconds) * time.Second
	if d < time.Minute {
		return d.String()
	}
	if d < time.Hour {
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1f h", d.Hours())
	}
	return fmt.Sprintf("%.1f días", d.Hours()/24)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "—"
	}
	return value.Local().Format("02/01/2006 15:04")
}

func categoryLabel(value string) string {
	switch value {
	case "documents":
		return "Documentos"
	case "photos":
		return "Fotos"
	case "multimedia":
		return "Multimedia"
	default:
		return "Mixto"
	}
}

func statusClass(value string) string {
	value = strings.ToLower(value)
	switch value {
	case "en uso", "montada":
		return "status-online"
	case "desmontada":
		return "status-idle"
	default:
		return "status-offline"
	}
}

func percent(free, capacity uint64) int {
	if capacity == 0 {
		return 0
	}
	used := capacity - minUint64(free, capacity)
	return int(math.Round(float64(used) * 100 / float64(capacity)))
}

func minUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
