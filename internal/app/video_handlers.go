package app

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	"personalcloud/internal/catalog"
	"personalcloud/internal/streaming"
)

type videoProfileStatus struct {
	streaming.Profile
	State string `json:"state"`
	URL   string `json:"url,omitempty"`
}

func (a *App) videoQualitiesGet(w http.ResponseWriter, r *http.Request) {
	file, ok := a.catalog.ByID(r.PathValue("id"))
	if !ok || file.Kind != "video" {
		http.NotFound(w, r)
		return
	}
	if file.Height <= 0 && a.streamer != nil {
		if width, height, err := a.streamer.ProbeDimensions(r.Context(), file); err == nil && width > 0 && height > 0 {
			file.Width = width
			file.Height = height
			_ = a.catalog.UpsertBatch(r.Context(), []catalog.File{file})
		}
	}
	profiles := a.streamer.Profiles(file)
	items := make([]videoProfileStatus, 0, len(profiles))
	for _, profile := range profiles {
		item := videoProfileStatus{Profile: profile, State: "idle"}
		if profile.ID == "original" {
			item.State = "ready"
			item.URL = "/archivo/" + file.ID + "/original"
		} else if status, err := a.streamer.Status(file, profile.ID); err == nil {
			item.State = status.State
			item.URL = status.URL
		}
		items = append(items, item)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ffmpeg":   a.streamer.Available(),
		"profiles": items,
		"width":    file.Width,
		"height":   file.Height,
	})
}

func (a *App) videoVariantPreparePost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		http.Error(w, "Sesión no válida.", http.StatusUnauthorized)
		return
	}
	if ok, wait := a.limiter.Allow("video-transcode:"+user.ID+":"+a.clientIP(r), videoTranscodePolicy); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retrySeconds(wait)))
		http.Error(w, "Demasiadas solicitudes de conversión de video.", http.StatusTooManyRequests)
		return
	}
	file, ok := a.catalog.ByID(r.PathValue("id"))
	if !ok || file.Kind != "video" {
		http.NotFound(w, r)
		return
	}
	if !a.storageOnline(r, file.StorageID) {
		http.Error(w, "La unidad que contiene este video no está conectada.", http.StatusServiceUnavailable)
		return
	}
	quality := strings.TrimSpace(r.FormValue("quality"))
	status, err := a.streamer.Prepare(file, quality)
	if err != nil {
		switch {
		case errors.Is(err, streaming.ErrUnavailable):
			http.Error(w, "FFmpeg con soporte H.264 no está disponible.", http.StatusServiceUnavailable)
		case errors.Is(err, streaming.ErrInvalidQuality):
			http.Error(w, "Resolución no válida para este video.", http.StatusBadRequest)
		default:
			http.Error(w, "No se pudo preparar la resolución solicitada.", http.StatusInternalServerError)
		}
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "video_variant", "solicitado:"+quality, a.clientIP(r))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	code := http.StatusAccepted
	if status.State == "ready" {
		code = http.StatusOK
	}
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(status)
}

func (a *App) videoVariantStatusGet(w http.ResponseWriter, r *http.Request) {
	file, ok := a.catalog.ByID(r.PathValue("id"))
	if !ok || file.Kind != "video" {
		http.NotFound(w, r)
		return
	}
	status, err := a.streamer.Status(file, strings.TrimSpace(r.URL.Query().Get("quality")))
	if err != nil {
		http.Error(w, "Resolución no válida.", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	_ = json.NewEncoder(w).Encode(status)
}

func (a *App) videoVariantGet(w http.ResponseWriter, r *http.Request) {
	file, ok := a.catalog.ByID(r.PathValue("id"))
	if !ok || file.Kind != "video" {
		http.NotFound(w, r)
		return
	}
	variantPath, err := a.streamer.VariantPath(file, r.PathValue("quality"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	handle, err := os.Open(variantPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := mime.TypeByExtension(".mp4")
	if contentType == "" {
		contentType = "video/mp4"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=21600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, file.Name, info.ModTime(), handle)
}
