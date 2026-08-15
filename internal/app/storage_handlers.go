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
	"strconv"
	"strings"
	"time"

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
	for _, view := range views {
		if view.Registered && view.Online {
			online++
		}
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

func (a *App) photosGet(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	offset, _ := strconv.Atoi(r.URL.Query().Get("desde"))
	if offset < 0 {
		offset = 0
	}
	files := a.catalog.ListPhotos(offset, 101)
	hasMore := len(files) > 100
	if hasMore {
		files = files[:100]
	}
	photos := make([]photoPageItem, 0, len(files))
	for _, file := range files {
		item := photoPageItem{File: file, OriginalURL: "/archivos/" + file.ID + "/original"}
		if file.Thumbnail {
			item.ThumbnailURL = "/fotos/" + file.ID + "/miniatura"
		}
		if file.Preview {
			item.PreviewURL = "/fotos/" + file.ID + "/vista-previa"
		}
		photos = append(photos, item)
	}
	data := a.csrfData(w, r, pageData{
		Title:        "Fotos",
		Description:  "Catálogo de fotos disponible aun con los originales desmontados.",
		CurrentPath:  "/fotos",
		User:         user,
		Photos:       photos,
		PhotoOffset:  offset,
		PhotoNext:    offset + len(files),
		PhotoHasMore: hasMore,
	})
	a.render(w, http.StatusOK, "photos", data)
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
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "storage_register", "correcto", a.clientIP(r))
	http.Redirect(w, r, "/almacenamiento?ok="+urlQuery("Unidad registrada: "+created.Name), http.StatusSeeOther)
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

func (a *App) storageUploadPost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cfg, err := a.store.StorageVolumeByID(r.Context(), id)
	if err != nil {
		redirectStorageError(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxUploadBytes+multipartOverhead)
	reader, err := r.MultipartReader()
	if err != nil {
		redirectStorageError(w, r, errors.New("formulario de subida inválido"))
		return
	}
	var csrfToken, targetDir string
	var uploaded bool
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			redirectStorageError(w, r, errors.New("no se pudo leer la subida"))
			return
		}
		name := part.FormName()
		switch name {
		case "csrf_token":
			value, readErr := readSmallPart(part, 4096)
			if readErr != nil {
				redirectStorageError(w, r, readErr)
				return
			}
			csrfToken = value
		case "target_dir":
			value, readErr := readSmallPart(part, 4096)
			if readErr != nil {
				redirectStorageError(w, r, readErr)
				return
			}
			targetDir = strings.TrimSpace(value)
		case "file":
			if !a.validCSRFValue(r, csrfToken) {
				http.Error(w, "La sesión del formulario no es válida.", http.StatusBadRequest)
				return
			}
			fileName := safeUploadName(part.FileName())
			if fileName == "" {
				redirectStorageError(w, r, errors.New("nombre de archivo inválido"))
				return
			}
			virtualDir, cleanErr := safeVirtualSubdir(targetDir)
			if cleanErr != nil {
				redirectStorageError(w, r, cleanErr)
				return
			}
			base := "/" + cfg.VirtualRoot
			if virtualDir != "" {
				base = path.Join(base, virtualDir)
				if err := a.vfs.MkdirAll(r.Context(), base); err != nil {
					redirectStorageError(w, r, err)
					return
				}
			}
			target := path.Join(base, fileName)
			if _, err := a.vfs.WriteAtomic(r.Context(), target, part, a.cfg.MaxUploadBytes, false); err != nil {
				redirectStorageError(w, r, err)
				return
			}
			uploaded = true
		default:
			_, _ = io.Copy(io.Discard, io.LimitReader(part, 4096))
		}
		_ = part.Close()
		if uploaded {
			break
		}
	}
	if !uploaded {
		redirectStorageError(w, r, errors.New("no se recibió ningún archivo"))
		return
	}
	a.indexer.Enqueue(id)
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "file_upload", "correcto", a.clientIP(r))
	http.Redirect(w, r, "/almacenamiento?ok="+urlQuery("Archivo subido correctamente"), http.StatusSeeOther)
}

func (a *App) photoThumbnailGet(w http.ResponseWriter, r *http.Request) {
	a.serveCatalogCache(w, r, "thumbnail")
}

func (a *App) photoPreviewGet(w http.ResponseWriter, r *http.Request) {
	a.serveCatalogCache(w, r, "preview")
}

func (a *App) serveCatalogCache(w http.ResponseWriter, r *http.Request, size string) {
	file, ok := a.catalog.ByID(r.PathValue("id"))
	if !ok || file.Kind != "image" {
		http.NotFound(w, r)
		return
	}
	if size == "thumbnail" && !file.Thumbnail || size == "preview" && !file.Preview {
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
