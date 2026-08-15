package app

import (
	"archive/zip"
	"compress/flate"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"personalcloud/internal/catalog"
)

const (
	maxBulkItems     = 500
	batchDownloadTTL = 5 * time.Minute
)

type batchDownload struct {
	UserID  string
	FileIDs []string
	Expires time.Time
}

type moveDestination struct {
	Name        string
	VirtualRoot string
	Online      bool
	ReadOnly    bool
}

func (a *App) moveDestinations(ctx context.Context) []moveDestination {
	views, _ := a.storageManager.Views(ctx)
	out := make([]moveDestination, 0, len(views))
	for _, view := range views {
		if !view.Registered {
			continue
		}
		out = append(out, moveDestination{Name: view.Name, VirtualRoot: view.VirtualRoot, Online: view.Online, ReadOnly: view.ReadOnly})
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func selectedFileIDs(r *http.Request) ([]string, error) {
	values := r.Form["file_id"]
	if len(values) == 0 {
		if one := strings.TrimSpace(r.FormValue("file_id")); one != "" {
			values = []string{one}
		}
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		id := strings.TrimSpace(value)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		if len(out) > maxBulkItems {
			return nil, fmt.Errorf("máximo %d elementos por operación", maxBulkItems)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no se seleccionaron archivos")
	}
	return out, nil
}

func (a *App) allowBulkAction(w http.ResponseWriter, r *http.Request, action string) bool {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSONError(w, errors.New("sesión no válida"), http.StatusUnauthorized)
		return false
	}
	if ok, wait := a.limiter.Allow("bulk:"+action+":"+user.ID+":"+a.clientIP(r), bulkActionPolicy); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retrySeconds(wait)))
		writeJSONError(w, errors.New("demasiadas operaciones; inténtalo de nuevo en unos segundos"), http.StatusTooManyRequests)
		return false
	}
	return true
}

func (a *App) elementsMovePost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) || !a.allowBulkAction(w, r, "move") {
		return
	}
	ids, err := selectedFileIDs(r)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	destinationRoot := strings.Trim(strings.TrimSpace(r.FormValue("destination_root")), "/")
	if destinationRoot == "" {
		writeJSONError(w, errors.New("selecciona una unidad de destino"), http.StatusBadRequest)
		return
	}
	dir, err := safeVirtualSubdir(r.FormValue("target_dir"))
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	destination, err := a.store.StorageVolumeByVirtualRoot(r.Context(), destinationRoot)
	if err != nil {
		writeJSONError(w, errors.New("unidad de destino no válida"), http.StatusBadRequest)
		return
	}
	if destination.ReadOnly || !a.storageOnline(r, destination.ID) {
		writeJSONError(w, errors.New("la unidad de destino no está disponible para escritura"), http.StatusConflict)
		return
	}
	base := "/" + destination.VirtualRoot
	if dir != "" {
		base = path.Join(base, dir)
		if err := a.vfs.MkdirAll(r.Context(), base); err != nil {
			writeJSONError(w, fmt.Errorf("crear carpeta de destino: %w", err), http.StatusConflict)
			return
		}
	}

	// Preflight antes de modificar nada: evita movimientos parciales por unidades
	// desconectadas, archivos faltantes conocidos o colisiones evidentes de destino.
	files := make([]catalog.File, 0, len(ids))
	targetNames := make(map[string]struct{}, len(ids))
	missing := 0
	for _, id := range ids {
		file, ok := a.catalog.ByID(id)
		if !ok {
			continue
		}
		if !a.storageOnline(r, file.StorageID) {
			writeJSONError(w, fmt.Errorf("%s está en una unidad desconectada", file.Name), http.StatusConflict)
			return
		}
		from := path.Join("/", file.VirtualRoot, file.RelativePath)
		if _, err := a.vfs.Stat(r.Context(), from); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				a.forgetMissingFile(r.Context(), file)
				missing++
				continue
			}
			writeJSONError(w, fmt.Errorf("comprobar %s: %w", file.Name, err), http.StatusConflict)
			return
		}
		nameKey := strings.ToLower(file.Name)
		if _, duplicate := targetNames[nameKey]; duplicate {
			writeJSONError(w, fmt.Errorf("hay más de un archivo llamado %s en la selección; usa carpetas distintas antes de moverlos juntos", file.Name), http.StatusConflict)
			return
		}
		targetNames[nameKey] = struct{}{}
		target := path.Join(base, file.Name)
		if _, err := a.vfs.Stat(r.Context(), target); err == nil {
			writeJSONError(w, fmt.Errorf("ya existe %s en el destino", file.Name), http.StatusConflict)
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, fmt.Errorf("comprobar destino de %s: %w", file.Name, err), http.StatusConflict)
			return
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		writeJSON(w, map[string]any{"ok": true, "moved": 0, "missing": missing})
		return
	}

	moved := 0
	for _, file := range files {
		target := path.Join(base, file.Name)
		from := path.Join("/", file.VirtualRoot, file.RelativePath)
		entry, err := a.vfs.MoveFile(r.Context(), from, target, false)
		if err != nil {
			writeJSONOperationError(w, fmt.Errorf("mover %s: %w", file.Name, err), http.StatusConflict, moved)
			return
		}
		newRel := strings.TrimPrefix(path.Clean(target), "/"+destination.VirtualRoot+"/")
		movedFile := file
		movedFile.ID = catalog.StableID(destination.ID, filepath.FromSlash(newRel))
		movedFile.StorageID = destination.ID
		movedFile.VirtualRoot = destination.VirtualRoot
		movedFile.RelativePath = filepath.ToSlash(newRel)
		movedFile.Name = entry.Name
		movedFile.ModTime = entry.ModTime.UTC()
		if err := a.catalog.MoveEntry(r.Context(), file.ID, movedFile); err != nil {
			a.logger.Error("archivo movido pero catálogo no actualizado", "old_id", file.ID, "new_id", movedFile.ID, "error", err)
			writeJSONOperationError(w, errors.New("el archivo se movió físicamente pero no se pudo actualizar el catálogo; ejecuta Sincronizar ahora"), http.StatusInternalServerError, moved+1)
			return
		}
		a.catalog.MoveCache(file.ID, movedFile.ID)
		if err := a.store.MoveStarredFileID(r.Context(), file.ID, movedFile.ID); err != nil {
			a.logger.Warn("archivo movido pero no se pudo migrar Destacados", "old_id", file.ID, "new_id", movedFile.ID, "error", err)
		}
		moved++
	}
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "files_move", fmt.Sprintf("correcto:%d faltantes:%d", moved, missing), a.clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "moved": moved, "missing": missing})
}

func (a *App) elementsDeletePost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) || !a.allowBulkAction(w, r, "delete") {
		return
	}
	ids, err := selectedFileIDs(r)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	deleted := 0
	catalogDeletes := make([]string, 0, len(ids))
	for _, id := range ids {
		file, ok := a.catalog.ByID(id)
		if !ok {
			continue
		}
		virtual := path.Join("/", file.VirtualRoot, file.RelativePath)
		err := a.vfs.Remove(r.Context(), virtual)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, fmt.Errorf("eliminar %s: %w", file.Name, err), http.StatusConflict)
			return
		}
		a.catalog.RemoveCache(file)
		catalogDeletes = append(catalogDeletes, file.ID)
		deleted++
	}
	if err := a.catalog.DeleteIDs(r.Context(), catalogDeletes); err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if err := a.store.DeleteStarredFileIDs(r.Context(), catalogDeletes); err != nil {
		a.logger.Warn("no se pudieron limpiar Destacados de archivos eliminados", "error", err)
	}
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "files_delete", fmt.Sprintf("correcto:%d", deleted), a.clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "deleted": deleted})
}

func (a *App) fileStarPost(w http.ResponseWriter, r *http.Request) {
	if !a.validCSRFValue(r, r.Header.Get("X-CSRF-Token")) {
		writeJSONError(w, errors.New("la sesión del formulario no es válida"), http.StatusBadRequest)
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSONError(w, errors.New("sesión no válida"), http.StatusUnauthorized)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if _, ok := a.catalog.ByID(id); !ok {
		http.NotFound(w, r)
		return
	}
	starred := true
	if err := r.ParseForm(); err == nil {
		if raw := strings.TrimSpace(r.FormValue("starred")); raw != "" {
			value, err := strconv.ParseBool(raw)
			if err != nil {
				writeJSONError(w, errors.New("valor starred inválido"), http.StatusBadRequest)
				return
			}
			starred = value
		}
	}
	if err := a.store.SetFileStarred(r.Context(), user.ID, id, starred); err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "file_star", fmt.Sprintf("%s:%s", id, strconv.FormatBool(starred)), a.clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "starred": starred})
}

func (a *App) fileRenamePost(w http.ResponseWriter, r *http.Request) {
	if !a.validCSRFValue(r, r.Header.Get("X-CSRF-Token")) {
		writeJSONError(w, errors.New("la sesión del formulario no es válida"), http.StatusBadRequest)
		return
	}
	if !a.allowBulkAction(w, r, "rename") {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	file, ok := a.catalog.ByID(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSONError(w, errors.New("solicitud inválida"), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		writeJSONError(w, errors.New("nombre de archivo inválido"), http.StatusBadRequest)
		return
	}
	if name == file.Name {
		writeJSON(w, map[string]any{"ok": true, "id": file.ID, "name": file.Name})
		return
	}
	cfg, err := a.store.StorageVolumeByID(r.Context(), file.StorageID)
	if err != nil {
		writeJSONError(w, errors.New("unidad del archivo no válida"), http.StatusConflict)
		return
	}
	if cfg.ReadOnly || !a.storageOnline(r, file.StorageID) {
		writeJSONError(w, errors.New("la unidad del archivo no está disponible para escritura"), http.StatusConflict)
		return
	}
	oldRel := filepath.ToSlash(file.RelativePath)
	parent := path.Dir(oldRel)
	if parent == "." {
		parent = ""
	}
	newRel := name
	if parent != "" {
		newRel = path.Join(parent, name)
	}
	from := path.Join("/", file.VirtualRoot, oldRel)
	to := path.Join("/", file.VirtualRoot, newRel)
	if _, err := a.vfs.Stat(r.Context(), to); err == nil {
		writeJSONError(w, fmt.Errorf("ya existe %s en esta carpeta", name), http.StatusConflict)
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		writeJSONError(w, fmt.Errorf("comprobar destino: %w", err), http.StatusConflict)
		return
	}
	entry, err := a.vfs.MoveFile(r.Context(), from, to, false)
	if err != nil {
		writeJSONError(w, fmt.Errorf("renombrar archivo: %w", err), http.StatusConflict)
		return
	}
	moved := file
	moved.ID = catalog.StableID(file.StorageID, filepath.FromSlash(newRel))
	moved.RelativePath = filepath.ToSlash(newRel)
	moved.Name = entry.Name
	moved.ModTime = entry.ModTime.UTC()
	if err := a.catalog.MoveEntry(r.Context(), file.ID, moved); err != nil {
		a.logger.Error("archivo renombrado físicamente pero catálogo no actualizado", "old_id", file.ID, "new_id", moved.ID, "error", err)
		writeJSONError(w, errors.New("el archivo se renombró físicamente pero el catálogo requiere sincronización"), http.StatusInternalServerError)
		return
	}
	a.catalog.MoveCache(file.ID, moved.ID)
	if err := a.store.MoveStarredFileID(r.Context(), file.ID, moved.ID); err != nil {
		a.logger.Warn("no se pudo conservar Destacados al renombrar", "old_id", file.ID, "new_id", moved.ID, "error", err)
	}
	user := userFromContext(r.Context())
	if user != nil {
		_ = a.store.Audit(r.Context(), user.ID, "file_rename", fmt.Sprintf("%s -> %s", file.Name, moved.Name), a.clientIP(r))
	}
	writeJSON(w, map[string]any{"ok": true, "id": moved.ID, "name": moved.Name})
}

func (a *App) batchDownloadTicketPost(w http.ResponseWriter, r *http.Request) {
	if !a.requestIsHTTPS(r) && !isLoopbackIP(a.clientIP(r)) {
		http.Error(w, "Las descargas remotas requieren HTTPS.", http.StatusUpgradeRequired)
		return
	}
	if !a.parseProtectedForm(w, r) || !a.allowBulkAction(w, r, "download") {
		return
	}
	ids, err := selectedFileIDs(r)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	user := userFromContext(r.Context())
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeJSONError(w, errors.New("no se pudo preparar la descarga"), http.StatusInternalServerError)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	a.batchMu.Lock()
	a.batchDownloads[token] = batchDownload{UserID: user.ID, FileIDs: append([]string(nil), ids...), Expires: time.Now().UTC().Add(batchDownloadTTL)}
	a.batchMu.Unlock()
	writeJSON(w, map[string]string{"url": "/descarga-lote/" + token})
}

func (a *App) batchDownloadGet(w http.ResponseWriter, r *http.Request) {
	if !a.requestIsHTTPS(r) && !isLoopbackIP(a.clientIP(r)) {
		http.Error(w, "Las descargas remotas requieren HTTPS.", http.StatusUpgradeRequired)
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		http.NotFound(w, r)
		return
	}
	token := r.PathValue("token")
	a.batchMu.Lock()
	batch, ok := a.batchDownloads[token]
	if ok {
		delete(a.batchDownloads, token) // ticket de un solo uso
	}
	a.batchMu.Unlock()
	if !ok || batch.UserID != user.ID || time.Now().UTC().After(batch.Expires) {
		http.NotFound(w, r)
		return
	}
	select {
	case a.batchZIP <- struct{}{}:
		defer func() { <-a.batchZIP }()
	default:
		http.Error(w, "Ya hay otra descarga ZIP en curso. Espera a que termine para proteger los recursos del servidor.", http.StatusTooManyRequests)
		return
	}

	name := "personal-cloud-" + time.Now().Format("20060102-150405") + ".zip"
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	zw := zip.NewWriter(w)
	zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.BestSpeed)
	})
	buffer := make([]byte, 64*1024)
	var failures []string
	usedNames := make(map[string]int)
	for _, id := range batch.FileIDs {
		file, ok := a.catalog.ByID(id)
		if !ok {
			failures = append(failures, id+": ya no existe en el catálogo")
			continue
		}
		handle, entry, err := a.vfs.OpenRead(r.Context(), path.Join("/", file.VirtualRoot, file.RelativePath))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				a.forgetMissingFile(r.Context(), file)
			}
			failures = append(failures, file.Name+": "+err.Error())
			continue
		}
		zipName := uniqueZipName(file.Name, usedNames)
		header := &zip.FileHeader{Name: zipName, Method: zipMethodFor(file.Name)}
		header.SetModTime(entry.ModTime)
		writer, err := zw.CreateHeader(header)
		if err == nil {
			_, err = io.CopyBuffer(writer, handle.File, buffer)
		}
		_ = handle.Close()
		if err != nil {
			failures = append(failures, file.Name+": "+err.Error())
		}
	}
	if len(failures) > 0 {
		if writer, err := zw.Create("PERSONAL-CLOUD-ERRORES.txt"); err == nil {
			_, _ = io.WriteString(writer, "Algunos elementos no pudieron incluirse:\n\n"+strings.Join(failures, "\n"))
		}
	}
	_ = zw.Close()
	_ = a.store.Audit(r.Context(), user.ID, "files_batch_download", fmt.Sprintf("correcto:%d errores:%d", len(batch.FileIDs)-len(failures), len(failures)), a.clientIP(r))
}

func zipMethodFor(name string) uint16 {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".heic", ".heif", ".avif", ".raw",
		".mp4", ".m4v", ".mkv", ".mov", ".webm", ".avi", ".wmv", ".mpeg", ".mpg",
		".mp3", ".m4a", ".aac", ".flac", ".ogg", ".opus", ".wav",
		".zip", ".7z", ".rar", ".gz", ".bz2", ".xz", ".zst", ".pdf", ".docx", ".xlsx", ".pptx", ".apk", ".aab":
		return zip.Store
	default:
		return zip.Deflate
	}
}

func uniqueZipName(name string, used map[string]int) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "" {
		name = "archivo"
	}
	key := strings.ToLower(name)
	if used[key] == 0 {
		used[key] = 1
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for n := used[key] + 1; ; n++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, n, ext)
		candidateKey := strings.ToLower(candidate)
		if used[candidateKey] == 0 {
			used[key] = n
			used[candidateKey] = 1
			return candidate
		}
	}
}

func (a *App) cleanupBatchDownloads() {
	now := time.Now().UTC()
	a.batchMu.Lock()
	defer a.batchMu.Unlock()
	for token, batch := range a.batchDownloads {
		if now.After(batch.Expires) {
			delete(a.batchDownloads, token)
		}
	}
}

func (a *App) forgetMissingFile(ctx context.Context, file catalog.File) {
	a.catalog.RemoveCache(file)
	if err := a.catalog.DeleteIDs(ctx, []string{file.ID}); err != nil {
		a.logger.Warn("no se pudo retirar archivo faltante del catálogo", "file_id", file.ID, "error", err)
		return
	}
	if err := a.store.DeleteStarredFileIDs(ctx, []string{file.ID}); err != nil {
		a.logger.Warn("no se pudo limpiar Destacados del archivo faltante", "file_id", file.ID, "error", err)
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONOperationError(w http.ResponseWriter, err error, status, completed int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":        false,
		"error":     err.Error(),
		"partial":   completed > 0,
		"completed": completed,
	})
}

func writeJSONError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
}

func (a *App) moveFoldersGet(w http.ResponseWriter, r *http.Request) {
	root := strings.Trim(strings.TrimSpace(r.URL.Query().Get("root")), "/")
	if root == "" {
		writeJSONError(w, errors.New("selecciona una unidad"), http.StatusBadRequest)
		return
	}
	cfg, err := a.store.StorageVolumeByVirtualRoot(r.Context(), root)
	if err != nil {
		writeJSONError(w, errors.New("unidad no válida"), http.StatusBadRequest)
		return
	}
	if !a.storageOnline(r, cfg.ID) {
		writeJSONError(w, errors.New("la unidad no está conectada"), http.StatusConflict)
		return
	}
	dir, err := safeVirtualSubdir(r.URL.Query().Get("path"))
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	virtual := "/" + cfg.VirtualRoot
	if dir != "" {
		virtual = path.Join(virtual, dir)
	}
	entries, err := a.vfs.ReadDir(r.Context(), virtual)
	if err != nil {
		writeJSONError(w, fmt.Errorf("leer carpeta: %w", err), http.StatusConflict)
		return
	}
	folders := make([]map[string]string, 0)
	for _, entry := range entries {
		if !entry.IsDir {
			continue
		}
		child := entry.Name
		if dir != "" {
			child = path.Join(dir, entry.Name)
		}
		folders = append(folders, map[string]string{"name": entry.Name, "path": child})
	}
	writeJSON(w, map[string]any{"root": cfg.VirtualRoot, "path": dir, "folders": folders})
}

func (a *App) moveFolderCreatePost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) || !a.allowBulkAction(w, r, "folder") {
		return
	}
	root := strings.Trim(strings.TrimSpace(r.FormValue("destination_root")), "/")
	cfg, err := a.store.StorageVolumeByVirtualRoot(r.Context(), root)
	if err != nil || cfg.ReadOnly || !a.storageOnline(r, cfg.ID) {
		writeJSONError(w, errors.New("la unidad de destino no está disponible para escritura"), http.StatusConflict)
		return
	}
	parent, err := safeVirtualSubdir(r.FormValue("parent"))
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(strings.ReplaceAll(r.FormValue("name"), "\\", "/"))
	if name == "" || strings.Contains(name, "/") || name == "." || name == ".." || strings.ContainsRune(name, 0) {
		writeJSONError(w, errors.New("nombre de carpeta inválido"), http.StatusBadRequest)
		return
	}
	targetDir := name
	if parent != "" {
		targetDir = path.Join(parent, name)
	}
	virtual := path.Join("/", cfg.VirtualRoot, targetDir)
	if err := a.vfs.MkdirAll(r.Context(), virtual); err != nil {
		writeJSONError(w, fmt.Errorf("crear carpeta: %w", err), http.StatusConflict)
		return
	}
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "folder_create", "destino:/"+cfg.VirtualRoot+"/"+targetDir, a.clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "path": targetDir})
}
