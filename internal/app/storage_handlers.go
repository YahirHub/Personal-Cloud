package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"personalcloud/internal/catalog"
	storagepkg "personalcloud/internal/storage"
)

const multipartOverhead = 2 << 20

func (a *App) dashboardGet(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if !user.OnboardingCompleted {
		http.Redirect(w, r, "/bienvenida", http.StatusSeeOther)
		return
	}
	views, _ := a.storageManager.Views(r.Context())
	stats := a.catalog.Stats()
	var online int
	registered := make([]storagepkg.View, 0, len(views))
	viewByID := make(map[string]storagepkg.View, len(views))
	for _, view := range views {
		if !view.Registered {
			continue
		}
		registered = append(registered, view)
		viewByID[view.ID] = view
		if view.Online {
			online++
		}
	}
	sort.SliceStable(registered, func(i, j int) bool {
		return strings.ToLower(registered[i].VirtualRoot) < strings.ToLower(registered[j].VirtualRoot)
	})

	files := a.catalog.AllFiles()
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].ModTime.Equal(files[j].ModTime) {
			return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
		}
		return files[i].ModTime.After(files[j].ModTime)
	})

	homeFolders := make([]explorerRoot, 0, minInt(len(registered), 4))
	counts := make(map[string]int)
	bytesByStorage := make(map[string]int64)
	for _, file := range files {
		counts[file.StorageID]++
		bytesByStorage[file.StorageID] += file.Size
	}
	for _, view := range registered {
		if len(homeFolders) >= 4 {
			break
		}
		homeFolders = append(homeFolders, explorerRoot{
			Name:       view.VirtualRoot,
			URL:        explorerURL("/" + view.VirtualRoot),
			Category:   view.Category,
			Status:     view.Status,
			FileCount:  counts[view.ID],
			TotalBytes: bytesByStorage[view.ID],
			Offline:    !view.Online,
		})
	}

	homeFiles := make([]homeFileItem, 0, minInt(len(files), 10))
	for _, file := range files {
		if len(homeFiles) >= 10 {
			break
		}
		view, ok := viewByID[file.StorageID]
		if !ok {
			continue
		}
		item := homeFileItem{
			ID:          file.ID,
			Name:        file.Name,
			Kind:        file.Kind,
			Size:        file.Size,
			ModTime:     file.ModTime,
			VirtualRoot: file.VirtualRoot,
			OpenURL:     "/archivo/" + file.ID + "/original",
			Offline:     !view.Online,
			Health:      file.Health,
		}
		if file.Thumbnail {
			item.ThumbnailURL = "/galeria/" + file.ID + "/miniatura"
		}
		homeFiles = append(homeFiles, item)
	}
	data := a.csrfData(w, r, pageData{
		Title:       "Inicio",
		Description: "Resumen de tu nube personal.",
		CurrentPath: "/inicio",
		User:        user,
		Stats: dashboardStats{
			Volumes: registeredVolumeCount(views),
			Online:  online,
			Files:   stats.Files,
			Photos:  stats.Photos,
			Bytes:   stats.Bytes,
		},
		HomeFolders: homeFolders,
		HomeFiles:   homeFiles,
	})
	a.render(w, http.StatusOK, "dashboard", data)
}

func (a *App) storageGet(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	views, discoverErr := a.storageManager.Views(r.Context())
	items := make([]storagePageItem, 0, len(views))
	for _, view := range views {
		item := storagePageItem{View: view, SuggestedRoot: suggestedVirtualRoot(view.Name)}
		if view.Registered {
			item.Job = a.indexer.Status(view.ID)
			item.JobPercent = item.Job.Percent()
			item.DamagedPending = len(a.catalog.DamagedByStorage(view.ID, false))
		}
		items = append(items, item)
	}
	data := a.csrfData(w, r, pageData{
		Title:          "Almacenamiento",
		Description:    "Administra unidades, montaje bajo demanda e indexación.",
		CurrentPath:    "/almacenamiento",
		User:           user,
		StorageItems:   items,
		MaxUploadBytes: a.cfg.MaxUploadBytes,
	})
	if discoverErr != nil {
		data.StorageError = discoverErr.Error()
	}
	if r.URL.Query().Get("ok") != "" {
		data.Info = r.URL.Query().Get("ok")
	}
	if r.URL.Query().Get("error") != "" {
		data.Error = r.URL.Query().Get("error")
	}
	a.render(w, http.StatusOK, "storage", data)
}

func (a *App) galleryGet(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	mode := a.resolveListingMode(w, r)
	kind, sortMode := gallerySelection(r)
	online := a.onlineStorageIDs(r.Context())
	query := catalog.MediaQuery{Kind: kind, Sort: sortMode, StorageIDs: online}
	const pageSize = 80
	page := parsePositiveInt(r.URL.Query().Get("pagina"), 1)
	offset := 0
	if mode == "paginas" {
		offset = (page - 1) * pageSize
	}
	files := a.catalog.ListMediaQuery(offset, pageSize+1, query)
	hasMore := len(files) > pageSize
	if hasMore {
		files = files[:pageSize]
	}
	media := make([]mediaPageItem, 0, len(files))
	for _, file := range files {
		media = append(media, a.mediaItem(file))
	}
	total := a.catalog.MediaCountQuery(query)
	filters := 0
	if kind != "all" {
		filters++
	}
	if sortMode != "file-newest" {
		filters++
	}
	data := a.csrfData(w, r, pageData{
		Title: "Galería", Description: "Imágenes, videos y audio con caché local y originales bajo demanda.", CurrentPath: "/galeria", User: user,
		Media: media, MediaOffset: offset, MediaNext: offset + len(files), MediaHasMore: hasMore, MediaTotal: total,
		GalleryType: kind, GallerySort: sortMode, GalleryFilters: filters,
		ListingMode: mode, ListingBaseURL: "/galeria", ListingPage: page, ListingPrev: maxInt(page-1, 1), ListingNext: page + 1, ListingHasPrev: page > 1, ListingHasNext: hasMore,
		ListingInfiniteURL: galleryURL(kind, sortMode, "infinito", 0),
		ListingPagesURL:    galleryURL(kind, sortMode, "paginas", 1),
		ListingPrevURL:     galleryURL(kind, sortMode, "paginas", maxInt(page-1, 1)),
		ListingNextURL:     galleryURL(kind, sortMode, "paginas", page+1),
		MoveDestinations:   a.moveDestinations(r.Context()),
	})
	a.render(w, http.StatusOK, "photos", data)
}

func (a *App) mediaItem(file catalog.File) mediaPageItem {
	item := mediaPageItem{File: file, OriginalURL: "/archivo/" + file.ID + "/original"}
	if file.Thumbnail {
		item.ThumbnailURL = "/galeria/" + file.ID + "/miniatura"
	}
	if file.Preview {
		item.PreviewURL = "/galeria/" + file.ID + "/vista-previa"
	}
	return item
}

func (a *App) galleryAPI(w http.ResponseWriter, r *http.Request) {
	offset := parsePositiveInt(r.URL.Query().Get("offset"), 0)
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 80)
	if limit > 120 {
		limit = 120
	}
	kind, sortMode := gallerySelection(r)
	query := catalog.MediaQuery{Kind: kind, Sort: sortMode, StorageIDs: a.onlineStorageIDs(r.Context())}
	files := a.catalog.ListMediaQuery(offset, limit+1, query)
	hasMore := len(files) > limit
	if hasMore {
		files = files[:limit]
	}
	items := make([]mediaPageItem, 0, len(files))
	for _, file := range files {
		items = append(items, a.mediaItem(file))
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "next": offset + len(files), "has_more": hasMore})
}

func (a *App) galleryAvailabilityAPI(w http.ResponseWriter, r *http.Request) {
	ids := a.onlineStorageIDs(r.Context())
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"online_storage_ids": out})
}

func (a *App) onlineStorageIDs(ctx context.Context) map[string]struct{} {
	ids, err := a.storageManager.OnlineRegisteredIDs(ctx)
	if err != nil {
		return map[string]struct{}{}
	}
	return ids
}

func gallerySelection(r *http.Request) (string, string) {
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tipo")))
	switch kind {
	case "image", "video", "audio":
	default:
		kind = "all"
	}
	sortMode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("orden")))
	switch sortMode {
	case "added-newest", "added-oldest", "file-newest", "file-oldest", "name-az", "name-za":
	default:
		sortMode = "file-newest"
	}
	return kind, sortMode
}

func galleryURL(kind, sortMode, mode string, page int) string {
	values := url.Values{}
	if kind != "" && kind != "all" {
		values.Set("tipo", kind)
	}
	if sortMode != "" && sortMode != "file-newest" {
		values.Set("orden", sortMode)
	}
	if mode != "" {
		values.Set("modo", mode)
	}
	if page > 1 {
		values.Set("pagina", strconv.Itoa(page))
	}
	if len(values) == 0 {
		return "/galeria"
	}
	return "/galeria?" + values.Encode()
}

func (a *App) indexStatusAPI(w http.ResponseWriter, r *http.Request) {
	type status struct {
		catalog.JobStatus
		Percent int `json:"percent"`
	}
	jobs := a.indexer.Statuses()
	out := make([]status, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, status{JobStatus: job, Percent: job.Percent()})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}

func parsePositiveInt(value string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return fallback
	}
	return n
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (a *App) storageRegisterPost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	idle, err := parseIdleSeconds(r.FormValue("idle_seconds"))
	if err != nil {
		redirectStorageError(w, r, err)
		return
	}
	created, err := a.storageManager.Register(r.Context(), storagepkg.RegisterInput{
		PersistentID:       r.FormValue("persistent_id"),
		Name:               r.FormValue("name"),
		Category:           r.FormValue("category"),
		VirtualRoot:        r.FormValue("virtual_root"),
		IdleTimeoutSeconds: idle,
		AutoUnmount:        r.FormValue("auto_unmount") == "on",
		ReadOnly:           r.FormValue("read_only") == "on",
	})
	if err != nil {
		redirectStorageError(w, r, err)
		return
	}
	started := a.indexer.Enqueue(created.ID)
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "storage_register", "correcto", a.clientIP(r))
	message := "Unidad registrada: " + created.Name
	if started {
		message += ". Indexación iniciada automáticamente"
	} else {
		message += ". Usa Indexar ahora para generar el catálogo"
	}
	http.Redirect(w, r, "/almacenamiento?ok="+urlQuery(message), http.StatusSeeOther)
}

func (a *App) storageUpdatePost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	idle, err := parseIdleSeconds(r.FormValue("idle_seconds"))
	if err != nil {
		redirectStorageError(w, r, err)
		return
	}
	id := r.PathValue("id")
	_, err = a.storageManager.Update(r.Context(), id, storagepkg.RegisterInput{
		Name:               r.FormValue("name"),
		Category:           r.FormValue("category"),
		VirtualRoot:        r.FormValue("virtual_root"),
		IdleTimeoutSeconds: idle,
		AutoUnmount:        r.FormValue("auto_unmount") == "on",
		ReadOnly:           r.FormValue("read_only") == "on",
	})
	if err != nil {
		redirectStorageError(w, r, err)
		return
	}
	a.indexer.Enqueue(id)
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "storage_update", "correcto", a.clientIP(r))
	http.Redirect(w, r, "/almacenamiento?ok="+urlQuery("Configuración guardada"), http.StatusSeeOther)
}

func (a *App) storageMountPost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	if _, err := a.storageManager.Mount(ctx, r.PathValue("id")); err != nil {
		redirectStorageError(w, r, err)
		return
	}
	http.Redirect(w, r, "/almacenamiento?ok="+urlQuery("Unidad montada"), http.StatusSeeOther)
}

func (a *App) storageUnmountPost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	if err := a.storageManager.Unmount(ctx, r.PathValue("id")); err != nil {
		redirectStorageError(w, r, err)
		return
	}
	http.Redirect(w, r, "/almacenamiento?ok="+urlQuery("Unidad desmontada"), http.StatusSeeOther)
}

func (a *App) storageIndexPost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	id := r.PathValue("id")
	if _, err := a.store.StorageVolumeByID(r.Context(), id); err != nil {
		redirectStorageError(w, r, err)
		return
	}
	if !a.indexer.Enqueue(id) {
		redirectStorageError(w, r, errors.New("la unidad ya se está indexando o la cola está ocupada"))
		return
	}
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "storage_index", "encolado", a.clientIP(r))
	http.Redirect(w, r, "/almacenamiento?ok="+urlQuery("Indexación iniciada"), http.StatusSeeOther)
}

func (a *App) photoThumbnailGet(w http.ResponseWriter, r *http.Request) {
	a.serveCatalogCache(w, r, "thumbnail")
}

func (a *App) photoPreviewGet(w http.ResponseWriter, r *http.Request) {
	a.serveCatalogCache(w, r, "preview")
}

func (a *App) serveCatalogCache(w http.ResponseWriter, r *http.Request, size string) {
	file, ok := a.catalog.ByID(r.PathValue("id"))
	if !ok || (file.Kind != "image" && file.Kind != "video" && file.Kind != "audio") {
		http.NotFound(w, r)
		return
	}
	if size == "thumbnail" && !file.Thumbnail || size == "preview" && (file.Kind != "image" || !file.Preview) {
		http.NotFound(w, r)
		return
	}
	cachePath := a.catalog.CachePath(file.ID, size)
	cacheFile, err := os.Open(cachePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer cacheFile.Close()
	info, err := cacheFile.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeContent(w, r, file.Name, info.ModTime(), cacheFile)
}

func (a *App) originalFileGet(w http.ResponseWriter, r *http.Request) {
	file, ok := a.catalog.ByID(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	virtualPath := path.Join("/", file.VirtualRoot, file.RelativePath)
	handle, entry, err := a.vfs.OpenRead(r.Context(), virtualPath)
	if err != nil {
		if errors.Is(err, storagepkg.ErrOffline) {
			http.Error(w, "La unidad que contiene este archivo no está conectada.", http.StatusServiceUnavailable)
			return
		}
		if errors.Is(err, os.ErrNotExist) {
			a.forgetMissingFile(r.Context(), file)
			http.Error(w, "El archivo fue eliminado fuera de Personal Cloud y se retiró del catálogo.", http.StatusNotFound)
			return
		}
		http.Error(w, "No se pudo abrir el archivo original.", http.StatusInternalServerError)
		return
	}
	defer handle.Close()
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, entry.Name, entry.ModTime, handle.File)
}

func (a *App) healthGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func registeredVolumeCount(views []storagepkg.View) int {
	var count int
	for _, view := range views {
		if view.Registered {
			count++
		}
	}
	return count
}

func parseIdleSeconds(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 300, nil
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 30 || seconds > 7*24*60*60 {
		return 0, errors.New("el timeout debe estar entre 30 segundos y 604800 segundos")
	}
	return seconds, nil
}

func redirectStorageError(w http.ResponseWriter, r *http.Request, err error) {
	http.Redirect(w, r, "/almacenamiento?error="+urlQuery(err.Error()), http.StatusSeeOther)
}

func urlQuery(value string) string {
	return url.QueryEscape(value)
}

func safeUploadName(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = path.Base(value)
	if value == "" || value == "." || value == ".." || strings.ContainsRune(value, 0) {
		return ""
	}
	return value
}

func safeVirtualSubdir(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || value == "." {
		return "", nil
	}
	if strings.HasPrefix(value, "/") {
		return "", errors.New("la carpeta de destino debe ser relativa")
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." || part == "" {
			return "", errors.New("carpeta de destino inválida")
		}
	}
	return path.Clean(value), nil
}

func readSmallPart(part *multipart.Part, max int64) (string, error) {
	data, err := io.ReadAll(io.LimitReader(part, max+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > max {
		return "", errors.New("campo de formulario demasiado grande")
	}
	return string(data), nil
}

func contextWithTimeout(r *http.Request, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), timeout)
}

func suggestedVirtualRoot(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Unidad"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		invalid := r < 32 || strings.ContainsRune(`/\:*?"<>|`, r)
		if invalid || r == ' ' {
			if b.Len() > 0 && !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
			continue
		}
		b.WriteRune(r)
		lastDash = false
	}
	result := strings.Trim(b.String(), "-.")
	if result == "" {
		return "Unidad"
	}
	return result
}
