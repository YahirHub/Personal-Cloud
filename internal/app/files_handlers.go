package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"personalcloud/internal/catalog"
	storagepkg "personalcloud/internal/storage"
)

func (a *App) filesGet(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	mode := a.resolveListingMode(w, r)
	current, err := normalizeExplorerPath(r.PathValue("path"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	views, discoverErr := a.storageManager.Views(r.Context())
	byID := make(map[string]storagepkg.View, len(views))
	for _, view := range views {
		if view.Registered {
			byID[view.ID] = view
		}
	}

	data := a.csrfData(w, r, pageData{
		Title:          "Archivos",
		Description:    "Explora el namespace virtual sin mantener todas las unidades montadas.",
		CurrentPath:    "/archivos",
		User:           user,
		ExplorerPath:   current,
		Breadcrumbs:    explorerBreadcrumbs(current),
		MaxUploadBytes: a.cfg.MaxUploadBytes,
		ListingMode:    mode,
	})
	if discoverErr != nil {
		data.StorageError = discoverErr.Error()
	}
	data.Info = r.URL.Query().Get("ok")
	data.Error = r.URL.Query().Get("error")

	if current == "/" {
		files := a.catalog.AllFiles()
		counts := make(map[string]int)
		bytesByStorage := make(map[string]int64)
		for _, file := range files {
			counts[file.StorageID]++
			bytesByStorage[file.StorageID] += file.Size
		}
		roots := make([]explorerRoot, 0, len(byID))
		for _, view := range byID {
			roots = append(roots, explorerRoot{
				Name:       view.VirtualRoot,
				URL:        explorerURL("/" + view.VirtualRoot),
				Category:   view.Category,
				Status:     view.Status,
				FileCount:  counts[view.ID],
				TotalBytes: bytesByStorage[view.ID],
				Offline:    !view.Online,
			})
		}
		sort.SliceStable(roots, func(i, j int) bool { return strings.ToLower(roots[i].Name) < strings.ToLower(roots[j].Name) })
		data.ExplorerRoots = roots
		data.ExplorerCanWrite = hasAutoUploadTarget(views)
		a.render(w, http.StatusOK, "files", data)
		return
	}

	root, relative := splitExplorerPath(current)
	cfg, err := a.store.StorageVolumeByVirtualRoot(r.Context(), root)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	view, ok := byID[cfg.ID]
	if !ok {
		view = storagepkg.View{ID: cfg.ID, Name: cfg.Name, VirtualRoot: cfg.VirtualRoot, Category: cfg.Category, ReadOnly: cfg.ReadOnly, Registered: true, Online: false, Status: "desconectada"}
	}
	data.ExplorerCanWrite = view.Online && !cfg.ReadOnly
	allItems := browseCatalog(a.catalog.FilesByStorage(cfg.ID), relative, view)
	const pageSize = 100
	page := parsePositiveInt(r.URL.Query().Get("pagina"), 1)
	data.ListingPage, data.ListingPrev, data.ListingNext = page, maxInt(page-1, 1), page+1
	data.ListingHasPrev = page > 1
	if mode == "paginas" {
		data.ExplorerItems, data.ListingHasNext = pageSlice(allItems, page, pageSize)
	} else {
		data.ExplorerItems, data.ExplorerHasMore = offsetSlice(allItems, 0, pageSize)
		data.ExplorerNext = len(data.ExplorerItems)
	}
	a.render(w, http.StatusOK, "files", data)
}

func (a *App) filesListAPI(w http.ResponseWriter, r *http.Request) {
	current, err := normalizeExplorerPath(r.URL.Query().Get("path"))
	if err != nil || current == "/" {
		http.Error(w, "ruta inválida", http.StatusBadRequest)
		return
	}
	root, relative := splitExplorerPath(current)
	cfg, err := a.store.StorageVolumeByVirtualRoot(r.Context(), root)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	views, _ := a.storageManager.Views(r.Context())
	view := storagepkg.View{ID: cfg.ID, Name: cfg.Name, VirtualRoot: cfg.VirtualRoot, Category: cfg.Category, ReadOnly: cfg.ReadOnly, Registered: true, Online: false, Status: "desconectada"}
	for _, candidate := range views {
		if candidate.ID == cfg.ID {
			view = candidate
			break
		}
	}
	items := browseCatalog(a.catalog.FilesByStorage(cfg.ID), relative, view)
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 150 {
		limit = 100
	}
	page, more := offsetSlice(items, offset, limit)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": page, "next": offset + len(page), "has_more": more})
}

func (a *App) filesUploadPost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxUploadBytes+multipartOverhead)
	reader, err := r.MultipartReader()
	if err != nil {
		redirectFilesError(w, r, "/", errors.New("formulario de subida inválido"))
		return
	}

	current := "/"
	var csrfToken, targetDir string
	var uploaded bool
	var destinationStorageID string
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			redirectFilesError(w, r, current, errors.New("no se pudo leer la subida"))
			return
		}
		switch part.FormName() {
		case "csrf_token":
			csrfToken, err = readSmallPart(part, 4096)
		case "current_path":
			var value string
			value, err = readSmallPart(part, 4096)
			if err == nil {
				current, err = normalizeExplorerPath(value)
			}
		case "target_dir":
			targetDir, err = readSmallPart(part, 4096)
			targetDir = strings.TrimSpace(targetDir)
		case "file":
			if !a.validCSRFValue(r, csrfToken) {
				http.Error(w, "La sesión del formulario no es válida.", http.StatusBadRequest)
				_ = part.Close()
				return
			}
			fileName := safeUploadName(part.FileName())
			if fileName == "" {
				err = errors.New("nombre de archivo inválido")
				break
			}
			var virtualTarget string
			virtualTarget, destinationStorageID, err = a.resolveExplorerUploadTarget(r, current, targetDir, fileName)
			if err == nil {
				parent := path.Dir(virtualTarget)
				if parent != "/" {
					err = a.vfs.MkdirAll(r.Context(), parent)
				}
			}
			if err == nil {
				_, err = a.vfs.WriteAtomic(r.Context(), virtualTarget, part, a.cfg.MaxUploadBytes, false)
			}
			if err == nil {
				uploaded = true
			}
		default:
			_, _ = io.Copy(io.Discard, io.LimitReader(part, 4096))
		}
		_ = part.Close()
		if err != nil || uploaded {
			break
		}
	}
	if err != nil {
		redirectFilesError(w, r, current, err)
		return
	}
	if !uploaded {
		redirectFilesError(w, r, current, errors.New("no se recibió ningún archivo"))
		return
	}
	if destinationStorageID != "" {
		a.indexer.Enqueue(destinationStorageID)
	}
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "file_upload_auto", "correcto", a.clientIP(r))
	redirectFilesOK(w, r, current, "Archivo subido; el catálogo se actualizará automáticamente")
}

func (a *App) resolveExplorerUploadTarget(r *http.Request, current, targetDir, fileName string) (string, string, error) {
	subdir, err := safeVirtualSubdir(targetDir)
	if err != nil {
		return "", "", err
	}
	if current != "/" {
		root, relative := splitExplorerPath(current)
		cfg, err := a.store.StorageVolumeByVirtualRoot(r.Context(), root)
		if err != nil {
			return "", "", err
		}
		if cfg.ReadOnly {
			return "", "", storagepkg.ErrReadOnly
		}
		if !storagepkg.CategoryAllowsFile(cfg.Category, fileName) {
			return "", "", errors.New("el tipo de archivo no está permitido en esta unidad")
		}
		base := "/" + cfg.VirtualRoot
		if relative != "" {
			base = path.Join(base, relative)
		}
		if subdir != "" {
			base = path.Join(base, subdir)
		}
		return path.Join(base, fileName), cfg.ID, nil
	}

	views, err := a.storageManager.Views(r.Context())
	if err != nil && len(views) == 0 {
		return "", "", err
	}
	selected, ok := chooseAutoStorage(views, fileName)
	if !ok {
		return "", "", errors.New("no hay una unidad conectada, escribible y compatible con este tipo de archivo")
	}
	base := "/" + selected.VirtualRoot
	if subdir != "" {
		base = path.Join(base, subdir)
	}
	return path.Join(base, fileName), selected.ID, nil
}

func chooseAutoStorage(views []storagepkg.View, fileName string) (storagepkg.View, bool) {
	kind := storagepkg.FileKind(fileName)
	candidates := make([]storagepkg.View, 0, len(views))
	for _, view := range views {
		if !view.Registered || !view.Online || view.ReadOnly || !storagepkg.CategoryAllowsFile(view.Category, fileName) {
			continue
		}
		if view.Capacity > 0 && view.Free == 0 {
			continue
		}
		candidates = append(candidates, view)
	}
	if len(candidates) == 0 {
		return storagepkg.View{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := autoCategoryRank(kind, candidates[i].Category), autoCategoryRank(kind, candidates[j].Category)
		if left != right {
			return left > right
		}
		if candidates[i].Free != candidates[j].Free {
			return candidates[i].Free > candidates[j].Free
		}
		return strings.ToLower(candidates[i].Name) < strings.ToLower(candidates[j].Name)
	})
	return candidates[0], true
}

func autoCategoryRank(kind, category string) int {
	switch kind {
	case "image":
		switch category {
		case "photos":
			return 40
		case "multimedia":
			return 30
		case "mixed":
			return 20
		}
	case "video", "audio":
		switch category {
		case "multimedia":
			return 40
		case "mixed":
			return 20
		}
	default:
		switch category {
		case "documents":
			return 40
		case "mixed":
			return 20
		}
	}
	return 0
}

func hasAutoUploadTarget(views []storagepkg.View) bool {
	for _, view := range views {
		if view.Registered && view.Online && !view.ReadOnly {
			return true
		}
	}
	return false
}

func browseCatalog(files []catalog.File, relative string, view storagepkg.View) []explorerItem {
	prefix := strings.Trim(strings.ReplaceAll(relative, "\\", "/"), "/")
	directories := make(map[string]explorerItem)
	result := make([]explorerItem, 0)
	for _, file := range files {
		rel := strings.Trim(strings.ReplaceAll(file.RelativePath, "\\", "/"), "/")
		if prefix != "" {
			wanted := prefix + "/"
			if !strings.HasPrefix(rel, wanted) {
				continue
			}
			rel = strings.TrimPrefix(rel, wanted)
		}
		if rel == "" {
			continue
		}
		if slash := strings.IndexByte(rel, '/'); slash >= 0 {
			name := rel[:slash]
			if _, exists := directories[strings.ToLower(name)]; !exists {
				virtual := "/" + view.VirtualRoot
				if prefix != "" {
					virtual = path.Join(virtual, prefix)
				}
				virtual = path.Join(virtual, name)
				directories[strings.ToLower(name)] = explorerItem{Name: name, Kind: "folder", URL: explorerURL(virtual), IsDir: true, Offline: !view.Online, StorageName: view.Name}
			}
			continue
		}
		result = append(result, explorerItem{
			Name:        file.Name,
			Kind:        file.Kind,
			Size:        file.Size,
			ModTime:     file.ModTime,
			DownloadURL: "/archivo/" + file.ID + "/original",
			Offline:     !view.Online,
			StorageName: view.Name,
		})
	}
	for _, directory := range directories {
		result = append(result, directory)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func normalizeExplorerPath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || value == "/" {
		return "/", nil
	}
	for _, segment := range strings.Split(strings.Trim(value, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." || strings.ContainsRune(segment, 0) {
			return "", errors.New("ruta inválida")
		}
	}
	return "/" + strings.Trim(path.Clean("/"+value), "/"), nil
}

func splitExplorerPath(value string) (root, relative string) {
	clean := strings.Trim(value, "/")
	parts := strings.SplitN(clean, "/", 2)
	if len(parts) == 0 {
		return "", ""
	}
	root = parts[0]
	if len(parts) == 2 {
		relative = parts[1]
	}
	return root, relative
}

func explorerURL(value string) string {
	clean, err := normalizeExplorerPath(value)
	if err != nil || clean == "/" {
		return "/archivos"
	}
	parts := strings.Split(strings.Trim(clean, "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return "/archivos/ver/" + strings.Join(parts, "/")
}

func explorerBreadcrumbs(value string) []breadcrumbItem {
	items := []breadcrumbItem{{Name: "Archivos", URL: "/archivos"}}
	clean, err := normalizeExplorerPath(value)
	if err != nil || clean == "/" {
		return items
	}
	parts := strings.Split(strings.Trim(clean, "/"), "/")
	var current []string
	for _, part := range parts {
		current = append(current, part)
		items = append(items, breadcrumbItem{Name: part, URL: explorerURL("/" + strings.Join(current, "/"))})
	}
	return items
}

func redirectFilesError(w http.ResponseWriter, r *http.Request, current string, err error) {
	target := explorerURL(current)
	http.Redirect(w, r, target+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
}

func redirectFilesOK(w http.ResponseWriter, r *http.Request, current, message string) {
	target := explorerURL(current)
	http.Redirect(w, r, target+"?ok="+url.QueryEscape(message), http.StatusSeeOther)
}
