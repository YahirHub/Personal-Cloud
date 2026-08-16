package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"personalcloud/internal/catalog"
	storagepkg "personalcloud/internal/storage"
)

const maxUploadBatchFiles = 100

type explorerFilter struct {
	Kind     string
	Modified string
	Source   string
}

func explorerFilterFromRequest(r *http.Request) explorerFilter {
	filter := explorerFilter{
		Kind: strings.TrimSpace(r.URL.Query().Get("tipo")), Modified: strings.TrimSpace(r.URL.Query().Get("modificado")), Source: strings.TrimSpace(r.URL.Query().Get("fuente")),
	}
	switch filter.Kind {
	case "image", "video", "audio", "document", "archive", "other":
	default:
		filter.Kind = ""
	}
	switch filter.Modified {
	case "today", "7d", "30d", "year":
	default:
		filter.Modified = ""
	}
	return filter
}

func applyExplorerFilter(items []explorerItem, filter explorerFilter, now time.Time) []explorerItem {
	if filter.Kind == "" && filter.Modified == "" && filter.Source == "" {
		return items
	}
	out := make([]explorerItem, 0, len(items))
	for _, item := range items {
		if item.IsDir {
			continue
		}
		if filter.Kind != "" && item.Kind != filter.Kind {
			continue
		}
		if filter.Source != "" && !strings.EqualFold(item.VirtualRoot, filter.Source) {
			continue
		}
		if filter.Modified != "" {
			if item.ModTime.IsZero() {
				continue
			}
			var cutoff time.Time
			switch filter.Modified {
			case "today":
				cutoff = now.Add(-24 * time.Hour)
			case "7d":
				cutoff = now.Add(-7 * 24 * time.Hour)
			case "30d":
				cutoff = now.Add(-30 * 24 * time.Hour)
			case "year":
				cutoff = now.Add(-365 * 24 * time.Hour)
			}
			if !cutoff.IsZero() && item.ModTime.Before(cutoff) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func fileListingURL(r *http.Request, mode string, page int) string {
	query := r.URL.Query()
	if mode != "" {
		query.Set("modo", mode)
	} else {
		query.Del("modo")
	}
	if page > 0 {
		query.Set("pagina", strconv.Itoa(page))
	} else {
		query.Del("pagina")
	}
	encoded := query.Encode()
	if encoded == "" {
		return r.URL.Path
	}
	return r.URL.Path + "?" + encoded
}

func explorerFilterCount(filter explorerFilter) int {
	count := 0
	if filter.Kind != "" {
		count++
	}
	if filter.Modified != "" {
		count++
	}
	if filter.Source != "" {
		count++
	}
	return count
}

func (a *App) recentFilesGet(w http.ResponseWriter, r *http.Request) {
	a.fileCollectionGet(w, r, "recent", "Recientes", "Archivos modificados recientemente")
}

func (a *App) starredFilesGet(w http.ResponseWriter, r *http.Request) {
	a.fileCollectionGet(w, r, "starred", "Destacados", "Tus archivos marcados como destacados")
}

func (a *App) fileCollectionGet(w http.ResponseWriter, r *http.Request, collection, title, subtitle string) {
	user := userFromContext(r.Context())
	if user == nil {
		http.Error(w, "sesión no válida", http.StatusUnauthorized)
		return
	}
	mode := a.resolveListingMode(w, r)
	filter := explorerFilterFromRequest(r)
	views, discoverErr := a.storageManager.Views(r.Context())
	byID := make(map[string]storagepkg.View, len(views))
	for _, view := range views {
		if view.Registered {
			byID[view.ID] = view
		}
	}
	stars, _ := a.store.StarredFileIDs(r.Context(), user.ID)
	files := a.catalog.AllFiles()
	items := make([]explorerItem, 0, len(files))
	for _, file := range files {
		_, starred := stars[file.ID]
		if collection == "starred" && !starred {
			continue
		}
		view, ok := byID[file.StorageID]
		if !ok {
			continue
		}
		location := "/" + file.VirtualRoot
		if parent := strings.Trim(path.Dir(strings.ReplaceAll(file.RelativePath, "\\", "/")), "/."); parent != "" {
			location = path.Join(location, parent)
		}
		item := explorerItem{
			ID: file.ID, Name: file.Name, Kind: file.Kind, Size: file.Size, ModTime: file.ModTime,
			DownloadURL: "/archivo/" + file.ID + "/original", Offline: !view.Online,
			StorageName: view.Name, VirtualRoot: file.VirtualRoot, Location: location, Health: file.Health, Starred: starred,
		}
		decorateExplorerFile(&item)
		if file.Thumbnail {
			item.ThumbnailURL = catalogCacheURL(file, "miniatura")
		}
		items = append(items, item)
	}
	items = applyExplorerFilter(items, filter, time.Now().UTC())
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ModTime.Equal(items[j].ModTime) {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return items[i].ModTime.After(items[j].ModTime)
	})
	if len(items) > 500 {
		items = items[:500]
	}
	data := a.csrfData(w, r, pageData{
		Title: title, Description: subtitle, CurrentPath: "/" + collection,
		User: user, ExplorerPath: "/", ExplorerItems: items, ExplorerCanWrite: hasAutoUploadTarget(views),
		Breadcrumbs: []breadcrumbItem{{Name: title, URL: r.URL.Path}}, ListingMode: mode,
		MoveDestinations: a.moveDestinations(r.Context()), MaxUploadBytes: a.cfg.MaxUploadBytes,
		MaxUploadBatchFiles: maxUploadBatchFiles,
		FileCollection:      collection, FileCollectionTitle: title, FileCollectionSubtitle: subtitle,
		FileTypeFilter: filter.Kind, FileModifiedFilter: filter.Modified, FileSourceFilter: filter.Source,
		FileFilterAction: r.URL.Path, FileFilterCount: explorerFilterCount(filter),
	})
	if discoverErr != nil {
		data.StorageError = discoverErr.Error()
	}
	a.render(w, http.StatusOK, "files", data)
}

func (a *App) filesGet(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	mode := a.resolveListingMode(w, r)
	filter := explorerFilterFromRequest(r)
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
	stars, _ := a.store.StarredFileIDs(r.Context(), user.ID)

	data := a.csrfData(w, r, pageData{
		Title:               "Mi unidad",
		Description:         "Explora el namespace virtual sin mantener todas las unidades montadas.",
		CurrentPath:         "/archivos",
		User:                user,
		ExplorerPath:        current,
		Breadcrumbs:         explorerBreadcrumbs(current),
		MaxUploadBytes:      a.cfg.MaxUploadBytes,
		MaxUploadBatchFiles: maxUploadBatchFiles,
		ListingMode:         mode,
		ListingBaseURL:      r.URL.Path,
		MoveDestinations:    a.moveDestinations(r.Context()),
		FileTypeFilter:      filter.Kind, FileModifiedFilter: filter.Modified, FileSourceFilter: filter.Source,
		FileFilterAction: r.URL.Path, FileFilterCount: explorerFilterCount(filter),
	})
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	data.SearchQuery = searchQuery
	if discoverErr != nil {
		data.StorageError = discoverErr.Error()
	}
	data.Info = r.URL.Query().Get("ok")
	data.Error = r.URL.Query().Get("error")

	if current == "/" && searchQuery != "" {
		needle := strings.ToLower(searchQuery)
		matches := make([]explorerItem, 0)
		for _, file := range a.catalog.AllFiles() {
			haystack := strings.ToLower(file.Name + " " + file.VirtualRoot + " " + file.RelativePath)
			if !strings.Contains(haystack, needle) {
				continue
			}
			view, ok := byID[file.StorageID]
			if !ok {
				continue
			}
			location := "/" + file.VirtualRoot
			if parent := strings.Trim(path.Dir(strings.ReplaceAll(file.RelativePath, "\\", "/")), "/."); parent != "" {
				location = path.Join(location, parent)
			}
			item := explorerItem{
				ID:          file.ID,
				Name:        file.Name,
				Kind:        file.Kind,
				Size:        file.Size,
				ModTime:     file.ModTime,
				DownloadURL: "/archivo/" + file.ID + "/original",
				Offline:     !view.Online,
				StorageName: view.Name,
				VirtualRoot: file.VirtualRoot,
				Location:    location,
				Health:      file.Health,
			}
			_, item.Starred = stars[file.ID]
			decorateExplorerFile(&item)
			if file.Thumbnail {
				item.ThumbnailURL = catalogCacheURL(file, "miniatura")
			}
			matches = append(matches, item)
		}
		matches = applyExplorerFilter(matches, filter, time.Now().UTC())
		sort.SliceStable(matches, func(i, j int) bool {
			if matches[i].ModTime.Equal(matches[j].ModTime) {
				return strings.ToLower(matches[i].Name) < strings.ToLower(matches[j].Name)
			}
			return matches[i].ModTime.After(matches[j].ModTime)
		})
		if len(matches) > 300 {
			matches = matches[:300]
		}
		data.SearchMode = true
		data.ExplorerItems = matches
		data.ExplorerCanWrite = hasAutoUploadTarget(views)
		data.Breadcrumbs = []breadcrumbItem{{Name: "Resultados de búsqueda", URL: "/archivos?q=" + url.QueryEscape(searchQuery)}}
		a.render(w, http.StatusOK, "files", data)
		return
	}

	if current == "/" {
		// Mi unidad es un namespace virtual: en vez de exponer las unidades físicas
		// como tarjetas, combina el primer nivel de las unidades disponibles. Así
		// la experiencia coincide con Drive: carpetas arriba y archivos debajo.
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
				ID:          view.ID,
				Name:        view.VirtualRoot,
				StorageName: view.Name,
				URL:         explorerURL("/" + view.VirtualRoot),
				Category:    view.Category,
				Status:      view.Status,
				FileCount:   counts[view.ID],
				TotalBytes:  bytesByStorage[view.ID],
				Capacity:    view.Capacity,
				Free:        view.Free,
				Mounted:     view.Mounted,
				ReadOnly:    view.ReadOnly,
				Offline:     !view.Online,
			})
		}
		sort.SliceStable(roots, func(i, j int) bool { return strings.ToLower(roots[i].Name) < strings.ToLower(roots[j].Name) })
		data.ExplorerRoots = roots // Se conserva sólo para distinguir "sin unidades" de "sin contenido disponible".
		data.ExplorerCanWrite = hasAutoUploadTarget(views)

		allItems := a.virtualRootItems(views)
		applyExplorerStars(allItems, stars)
		allItems = applyExplorerFilter(allItems, filter, time.Now().UTC())
		const pageSize = 100
		page := parsePositiveInt(r.URL.Query().Get("pagina"), 1)
		data.ListingPage, data.ListingPrev, data.ListingNext = page, maxInt(page-1, 1), page+1
		data.ListingHasPrev = page > 1
		data.ListingInfiniteURL = fileListingURL(r, "infinito", 0)
		data.ListingPagesURL = fileListingURL(r, "paginas", 1)
		data.ListingPrevURL = fileListingURL(r, "paginas", data.ListingPrev)
		data.ListingNextURL = fileListingURL(r, "paginas", data.ListingNext)
		if mode == "paginas" {
			data.ExplorerItems, data.ListingHasNext = pageSlice(allItems, page, pageSize)
		} else {
			data.ExplorerItems, data.ExplorerHasMore = offsetSlice(allItems, 0, pageSize)
			data.ExplorerNext = len(data.ExplorerItems)
		}
		data.ExplorerFolders, data.ExplorerFiles = splitExplorerItems(data.ExplorerItems)
		a.render(w, http.StatusOK, "files", data)
		return
	}

	root, relative := splitExplorerPath(current)
	data.ExplorerRoot = root
	data.ExplorerRelative = relative
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
	applyExplorerStars(allItems, stars)
	allItems = applyExplorerFilter(allItems, filter, time.Now().UTC())
	const pageSize = 100
	page := parsePositiveInt(r.URL.Query().Get("pagina"), 1)
	data.ListingPage, data.ListingPrev, data.ListingNext = page, maxInt(page-1, 1), page+1
	data.ListingHasPrev = page > 1
	data.ListingInfiniteURL = fileListingURL(r, "infinito", 0)
	data.ListingPagesURL = fileListingURL(r, "paginas", 1)
	data.ListingPrevURL = fileListingURL(r, "paginas", data.ListingPrev)
	data.ListingNextURL = fileListingURL(r, "paginas", data.ListingNext)
	if mode == "paginas" {
		data.ExplorerItems, data.ListingHasNext = pageSlice(allItems, page, pageSize)
	} else {
		data.ExplorerItems, data.ExplorerHasMore = offsetSlice(allItems, 0, pageSize)
		data.ExplorerNext = len(data.ExplorerItems)
	}
	data.ExplorerFolders, data.ExplorerFiles = splitExplorerItems(data.ExplorerItems)
	a.render(w, http.StatusOK, "files", data)
}

func (a *App) filesListAPI(w http.ResponseWriter, r *http.Request) {
	current, err := normalizeExplorerPath(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, "ruta inválida", http.StatusBadRequest)
		return
	}
	views, _ := a.storageManager.Views(r.Context())
	if current == "/" {
		items := a.virtualRootItems(views)
		if user := userFromContext(r.Context()); user != nil {
			stars, _ := a.store.StarredFileIDs(r.Context(), user.ID)
			applyExplorerStars(items, stars)
		}
		items = applyExplorerFilter(items, explorerFilterFromRequest(r), time.Now().UTC())
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 150 {
			limit = 100
		}
		page, more := offsetSlice(items, offset, limit)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": page, "next": offset + len(page), "has_more": more})
		return
	}
	root, relative := splitExplorerPath(current)
	cfg, err := a.store.StorageVolumeByVirtualRoot(r.Context(), root)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	view := storagepkg.View{ID: cfg.ID, Name: cfg.Name, VirtualRoot: cfg.VirtualRoot, Category: cfg.Category, ReadOnly: cfg.ReadOnly, Registered: true, Online: false, Status: "desconectada"}
	for _, candidate := range views {
		if candidate.ID == cfg.ID {
			view = candidate
			break
		}
	}
	items := browseCatalog(a.catalog.FilesByStorage(cfg.ID), relative, view)
	if user := userFromContext(r.Context()); user != nil {
		stars, _ := a.store.StarredFileIDs(r.Context(), user.ID)
		applyExplorerStars(items, stars)
	}
	items = applyExplorerFilter(items, explorerFilterFromRequest(r), time.Now().UTC())
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
	r.Body = http.MaxBytesReader(w, r.Body, uploadBatchRequestLimit(a.cfg.MaxUploadBytes))
	reader, err := r.MultipartReader()
	if err != nil {
		redirectFilesError(w, r, "/", errors.New("formulario de subida inválido"))
		return
	}

	current := "/"
	var csrfToken, targetDir, destinationRoot string
	uploaded := 0
	attempted := 0
	failed := make([]string, 0)
	storageIDs := make(map[string]struct{})
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			failed = append(failed, "no se pudo leer el resto de la subida")
			break
		}
		partErr := error(nil)
		switch part.FormName() {
		case "csrf_token":
			csrfToken, partErr = readSmallPart(part, 4096)
		case "current_path":
			var value string
			value, partErr = readSmallPart(part, 4096)
			if partErr == nil {
				current, partErr = normalizeExplorerPath(value)
			}
		case "destination_root":
			destinationRoot, partErr = readSmallPart(part, 4096)
			destinationRoot = strings.Trim(strings.TrimSpace(destinationRoot), "/")
		case "target_dir":
			targetDir, partErr = readSmallPart(part, 4096)
			targetDir = strings.TrimSpace(targetDir)
		case "file":
			if !a.validCSRFValue(r, csrfToken) {
				http.Error(w, "La sesión del formulario no es válida.", http.StatusBadRequest)
				_ = part.Close()
				return
			}
			attempted++
			if attempted > maxUploadBatchFiles {
				partErr = fmt.Errorf("solo se permiten hasta %d archivos por subida", maxUploadBatchFiles)
				break
			}
			fileName := safeUploadName(part.FileName())
			if fileName == "" {
				partErr = errors.New("nombre de archivo inválido")
				break
			}
			virtualTarget, destinationStorageID, resolveErr := a.resolveExplorerUploadTarget(r, current, destinationRoot, targetDir, fileName)
			partErr = resolveErr
			if partErr == nil {
				parent := path.Dir(virtualTarget)
				if parent != "/" {
					partErr = a.vfs.MkdirAll(r.Context(), parent)
				}
			}
			if partErr == nil {
				_, partErr = a.vfs.WriteAtomic(r.Context(), virtualTarget, part, a.cfg.MaxUploadBytes, false)
			}
			if partErr == nil {
				// La entrada mínima se incorpora al catálogo de inmediato. Así Mi unidad
				// refleja el archivo recién escrito sin esperar a que una indexación completa
				// recorra una unidad grande. El indexador en segundo plano enriquecerá luego
				// miniaturas, dimensiones e integridad.
				if syncErr := a.catalogUploadedFile(r.Context(), virtualTarget); syncErr != nil {
					a.logger.Warn("archivo subido pero no se pudo incorporar inmediatamente al catálogo", "path", virtualTarget, "error", syncErr)
				}
				uploaded++
				storageIDs[destinationStorageID] = struct{}{}
			}
		default:
			_, _ = io.Copy(io.Discard, io.LimitReader(part, 4096))
		}
		_ = part.Close()
		if partErr != nil {
			if part.FormName() != "file" {
				redirectFilesError(w, r, current, partErr)
				return
			}
			name := safeUploadName(part.FileName())
			if name == "" {
				name = "archivo"
			}
			failed = append(failed, fmt.Sprintf("%s: %v", name, partErr))
			if attempted > maxUploadBatchFiles {
				break
			}
		}
	}
	if uploaded == 0 {
		if len(failed) > 0 {
			redirectFilesError(w, r, current, errors.New(strings.Join(failed, "; ")))
		} else {
			redirectFilesError(w, r, current, errors.New("no se recibió ningún archivo"))
		}
		return
	}
	for storageID := range storageIDs {
		if storageID != "" {
			a.indexer.Enqueue(storageID)
		}
	}
	user := userFromContext(r.Context())
	action := "file_upload_auto"
	if destinationRoot != "" {
		action = "file_upload_manual"
	}
	if attempted > 1 {
		action += "_batch"
	}
	_ = a.store.Audit(r.Context(), user.ID, action, "correcto", a.clientIP(r))
	message := fmt.Sprintf("%d archivo%s subido%s", uploaded, pluralSuffix(uploaded), pluralSuffix(uploaded))
	if len(failed) == 0 {
		redirectFilesOK(w, r, current, message)
		return
	}
	problem := fmt.Sprintf("No se pudieron subir %d archivos: %s", len(failed), strings.Join(failed, "; "))
	if len(failed) == 1 {
		problem = "No se pudo subir 1 archivo: " + failed[0]
	}
	redirectFilesResult(w, r, current, message, problem)
}

func uploadBatchRequestLimit(perFile int64) int64 {
	if perFile <= 0 {
		return 1 << 30
	}
	const extra = int64(maxUploadBatchFiles+1) * multipartOverhead
	max := int64(^uint64(0) >> 1)
	if perFile > (max-extra)/maxUploadBatchFiles {
		return max
	}
	return perFile*maxUploadBatchFiles + extra
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func (a *App) catalogUploadedFile(ctx context.Context, virtualTarget string) error {
	entry, err := a.vfs.Stat(ctx, virtualTarget)
	if err != nil {
		return err
	}
	if entry.IsDir || entry.VolumeID == "" {
		return errors.New("la subida no corresponde a un archivo regular")
	}
	_, relative := splitExplorerPath(entry.VirtualPath)
	if relative == "" {
		return errors.New("ruta relativa de catálogo inválida")
	}
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(entry.Name)))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	file := catalog.File{
		ID:           catalog.StableID(entry.VolumeID, relative),
		StorageID:    entry.VolumeID,
		VirtualRoot:  entry.VirtualRoot,
		RelativePath: filepath.ToSlash(relative),
		Name:         entry.Name,
		Kind:         storagepkg.FileKind(entry.Name),
		MIME:         mimeType,
		Size:         entry.Size,
		ModTime:      entry.ModTime.UTC(),
	}
	return a.catalog.UpsertBatch(ctx, []catalog.File{file})
}

func (a *App) resolveExplorerUploadTarget(r *http.Request, current, destinationRoot, targetDir, fileName string) (string, string, error) {
	subdir, err := safeVirtualSubdir(targetDir)
	if err != nil {
		return "", "", err
	}
	if destinationRoot != "" {
		cfg, err := a.store.StorageVolumeByVirtualRoot(r.Context(), destinationRoot)
		if err != nil {
			return "", "", errors.New("unidad de destino no válida")
		}
		if cfg.ReadOnly || !a.storageOnline(r, cfg.ID) {
			return "", "", errors.New("la unidad elegida no está disponible para escritura")
		}
		if !storagepkg.CategoryAllowsFile(cfg.Category, fileName) {
			return "", "", errors.New("el tipo de archivo no está permitido en la unidad elegida")
		}
		base := "/" + cfg.VirtualRoot
		if subdir != "" {
			base = path.Join(base, subdir)
		}
		return path.Join(base, fileName), cfg.ID, nil
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
				directories[strings.ToLower(name)] = explorerItem{Name: name, Kind: "folder", URL: explorerURL(virtual), IsDir: true, Offline: !view.Online, StorageName: view.Name, VirtualRoot: view.VirtualRoot}
			}
			continue
		}
		item := explorerItem{
			ID:          file.ID,
			Name:        file.Name,
			Kind:        file.Kind,
			Size:        file.Size,
			ModTime:     file.ModTime,
			DownloadURL: "/archivo/" + file.ID + "/original",
			ThumbnailURL: func() string {
				if file.Thumbnail {
					return catalogCacheURL(file, "miniatura")
				}
				return ""
			}(),
			Offline:     !view.Online,
			StorageName: view.Name,
			VirtualRoot: file.VirtualRoot,
			Health:      file.Health,
		}
		decorateExplorerFile(&item)
		result = append(result, item)
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

func (a *App) virtualRootItems(views []storagepkg.View) []explorerItem {
	items := make([]explorerItem, 0)
	for _, view := range views {
		if !view.Registered || !view.Online {
			continue
		}
		items = append(items, browseCatalog(a.catalog.FilesByStorage(view.ID), "", view)...)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		left, right := strings.ToLower(items[i].Name), strings.ToLower(items[j].Name)
		if left == right {
			return strings.ToLower(items[i].StorageName) < strings.ToLower(items[j].StorageName)
		}
		return left < right
	})
	return items
}

func applyExplorerStars(items []explorerItem, stars map[string]struct{}) {
	if len(stars) == 0 {
		return
	}
	for i := range items {
		if items[i].IsDir || items[i].ID == "" {
			continue
		}
		_, items[i].Starred = stars[items[i].ID]
	}
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
	items := []breadcrumbItem{{Name: "Mi unidad", URL: "/archivos"}}
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

func redirectFilesResult(w http.ResponseWriter, r *http.Request, current, message, problem string) {
	target := explorerURL(current)
	query := url.Values{}
	if message != "" {
		query.Set("ok", message)
	}
	if problem != "" {
		query.Set("error", problem)
	}
	http.Redirect(w, r, target+"?"+query.Encode(), http.StatusSeeOther)
}
