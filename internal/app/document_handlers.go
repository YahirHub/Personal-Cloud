package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"personalcloud/internal/catalog"
	storagepkg "personalcloud/internal/storage"
)

const maxEditableTextBytes int64 = 8 << 20 // 8 MiB: suficiente para documentos de texto sin cargar archivos enormes en el navegador.

// fileViewerKind devuelve el visor local que puede manejar un archivo sin
// depender de servicios externos. Una cadena vacía conserva la apertura normal.
func fileViewerKind(name string) string {
	// Los medios usan el mismo clasificador central del catálogo para que
	// cualquier vista (Mi unidad, Inicio, Recientes, Destacados, búsqueda,
	// etc.) anuncie el reproductor reutilizable que corresponde al archivo.
	if kind := storagepkg.FileKind(name); kind == "image" || kind == "video" || kind == "audio" {
		return kind
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return "markdown"
	case ".html", ".htm", ".xhtml":
		return "html"
	case ".txt", ".text", ".log", ".rst", ".csv", ".tsv", ".json", ".jsonl", ".yaml", ".yml", ".toml", ".xml", ".ini", ".cfg", ".conf", ".env", ".properties", ".tex", ".css", ".scss", ".sass", ".less", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".vue", ".svelte", ".php", ".py", ".pyw", ".go", ".rs", ".java", ".kt", ".kts", ".c", ".h", ".cpp", ".hpp", ".cs", ".swift", ".dart", ".rb", ".sh", ".bash", ".zsh", ".ps1", ".bat", ".cmd", ".sql", ".graphql":
		return "text"
	case ".pdf":
		return "pdf"
	default:
		return ""
	}
}

func fileViewerEditable(name string) bool {
	switch fileViewerKind(name) {
	case "markdown", "html", "text":
		return true
	default:
		return false
	}
}

func textVersion(modUnixNano, size int64) string {
	return strconv.FormatInt(modUnixNano, 36) + "." + strconv.FormatInt(size, 36)
}

func (a *App) fileTextContentGet(w http.ResponseWriter, r *http.Request) {
	file, ok := a.catalog.ByID(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	viewer := fileViewerKind(file.Name)
	if viewer != "markdown" && viewer != "html" && viewer != "text" {
		http.Error(w, "Este archivo no dispone de un visor de texto.", http.StatusUnsupportedMediaType)
		return
	}
	virtualPath := path.Join("/", file.VirtualRoot, file.RelativePath)
	handle, entry, err := a.vfs.OpenRead(r.Context(), virtualPath)
	if err != nil {
		a.writeViewerOpenError(w, r, file, err)
		return
	}
	defer handle.Close()
	if entry.Size > maxEditableTextBytes {
		http.Error(w, fmt.Sprintf("El archivo supera el límite de edición en navegador de %d MiB.", maxEditableTextBytes>>20), http.StatusRequestEntityTooLarge)
		return
	}
	data, err := io.ReadAll(io.LimitReader(handle.File, maxEditableTextBytes+1))
	if err != nil {
		http.Error(w, "No se pudo leer el archivo.", http.StatusInternalServerError)
		return
	}
	if int64(len(data)) > maxEditableTextBytes {
		http.Error(w, fmt.Sprintf("El archivo supera el límite de edición en navegador de %d MiB.", maxEditableTextBytes>>20), http.StatusRequestEntityTooLarge)
		return
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		http.Error(w, "El archivo no es texto UTF-8 válido y no puede editarse de forma segura en el navegador.", http.StatusUnsupportedMediaType)
		return
	}
	starred := false
	if user := userFromContext(r.Context()); user != nil {
		starred, _ = a.store.FileStarred(r.Context(), user.ID, file.ID)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": file.ID, "name": file.Name, "viewer": viewer, "editable": true,
		"content": string(data), "size": entry.Size, "mod_time": entry.ModTime,
		"version": textVersion(entry.ModTime.UnixNano(), entry.Size), "starred": starred,
	})
}

type textSaveRequest struct {
	Content string `json:"content"`
	Version string `json:"version"`
}

func (a *App) fileTextContentPost(w http.ResponseWriter, r *http.Request) {
	if !a.validCSRFValue(r, r.Header.Get("X-CSRF-Token")) {
		writeJSONError(w, errors.New("la sesión del formulario no es válida; recarga la página e inténtalo de nuevo"), http.StatusBadRequest)
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSONError(w, errors.New("sesión no válida"), http.StatusUnauthorized)
		return
	}
	if ok, wait := a.limiter.Allow("text-edit:"+user.ID+":"+a.clientIP(r), bulkActionPolicy); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retrySeconds(wait)))
		writeJSONError(w, errors.New("demasiadas ediciones; inténtalo de nuevo en unos segundos"), http.StatusTooManyRequests)
		return
	}
	file, ok := a.catalog.ByID(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !fileViewerEditable(file.Name) {
		writeJSONError(w, errors.New("este tipo de archivo no admite edición desde el navegador"), http.StatusUnsupportedMediaType)
		return
	}

	// JSON puede duplicar algunos caracteres escapados; el cuerpo se limita con
	// margen, pero el contenido real conserva el límite estricto de 8 MiB.
	r.Body = http.MaxBytesReader(w, r.Body, maxEditableTextBytes*4+(64<<10))
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input textSaveRequest
	if err := decoder.Decode(&input); err != nil {
		writeJSONError(w, errors.New("contenido de edición inválido"), http.StatusBadRequest)
		return
	}
	if int64(len(input.Content)) > maxEditableTextBytes {
		writeJSONError(w, fmt.Errorf("el archivo supera el límite de edición de %d MiB", maxEditableTextBytes>>20), http.StatusRequestEntityTooLarge)
		return
	}
	if !utf8.ValidString(input.Content) || strings.IndexByte(input.Content, 0) >= 0 {
		writeJSONError(w, errors.New("el contenido debe ser texto UTF-8 válido"), http.StatusBadRequest)
		return
	}

	virtualPath := path.Join("/", file.VirtualRoot, file.RelativePath)
	current, err := a.vfs.Stat(r.Context(), virtualPath)
	if err != nil {
		a.writeViewerOpenError(w, r, file, err)
		return
	}
	currentVersion := textVersion(current.ModTime.UnixNano(), current.Size)
	if input.Version == "" || input.Version != currentVersion {
		writeJSONError(w, errors.New("el archivo cambió desde que lo abriste; vuelve a cargarlo antes de guardar para no sobrescribir cambios externos"), http.StatusConflict)
		return
	}

	written, err := a.vfs.WriteAtomic(r.Context(), virtualPath, strings.NewReader(input.Content), maxEditableTextBytes, true)
	if err != nil {
		if errors.Is(err, storagepkg.ErrOffline) {
			writeJSONError(w, errors.New("la unidad que contiene este archivo no está conectada"), http.StatusServiceUnavailable)
			return
		}
		writeJSONError(w, fmt.Errorf("no se pudo guardar el archivo: %w", err), http.StatusConflict)
		return
	}
	updated, err := a.vfs.Stat(r.Context(), virtualPath)
	if err != nil {
		writeJSONError(w, errors.New("el archivo se guardó, pero no se pudo actualizar su metadata"), http.StatusInternalServerError)
		return
	}
	file.Size = updated.Size
	file.ModTime = updated.ModTime
	if detected := mime.TypeByExtension(filepath.Ext(file.Name)); detected != "" {
		file.MIME = detected
	}
	if err := a.catalog.UpsertBatch(r.Context(), []catalog.File{file}); err != nil {
		a.logger.Error("archivo de texto guardado pero no se pudo actualizar catálogo", "file_id", file.ID, "error", err)
		writeJSONError(w, errors.New("el archivo se guardó, pero no se pudo actualizar el catálogo"), http.StatusInternalServerError)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "file_text_edit", fmt.Sprintf("archivo:%s bytes:%d", file.ID, written), a.clientIP(r))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true, "id": file.ID, "size": updated.Size, "mod_time": updated.ModTime,
		"version": textVersion(updated.ModTime.UnixNano(), updated.Size),
	})
}

func (a *App) filePDFPreviewGet(w http.ResponseWriter, r *http.Request) {
	file, ok := a.catalog.ByID(r.PathValue("id"))
	if !ok || fileViewerKind(file.Name) != "pdf" {
		http.NotFound(w, r)
		return
	}
	virtualPath := path.Join("/", file.VirtualRoot, file.RelativePath)
	handle, entry, err := a.vfs.OpenRead(r.Context(), virtualPath)
	if err != nil {
		a.writeViewerOpenError(w, r, file, err)
		return
	}
	defer handle.Close()

	// El middleware general impide cualquier framing. Este endpoint es
	// deliberadamente embebible sólo por la propia aplicación para el visor PDF.
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Content-Type", "application/pdf")
	if disposition := mime.FormatMediaType("inline", map[string]string{"filename": entry.Name}); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	http.ServeContent(w, r, entry.Name, entry.ModTime, handle.File)
}

func (a *App) fileHTMLPreviewGet(w http.ResponseWriter, r *http.Request) {
	file, ok := a.catalog.ByID(r.PathValue("id"))
	if !ok || fileViewerKind(file.Name) != "html" {
		http.NotFound(w, r)
		return
	}
	virtualPath := path.Join("/", file.VirtualRoot, file.RelativePath)
	handle, entry, err := a.vfs.OpenRead(r.Context(), virtualPath)
	if err != nil {
		a.writeViewerOpenError(w, r, file, err)
		return
	}
	defer handle.Close()
	if entry.Size > maxEditableTextBytes {
		http.Error(w, fmt.Sprintf("El HTML supera el límite de vista segura de %d MiB.", maxEditableTextBytes>>20), http.StatusRequestEntityTooLarge)
		return
	}
	data, err := io.ReadAll(io.LimitReader(handle.File, maxEditableTextBytes+1))
	if err != nil || int64(len(data)) > maxEditableTextBytes {
		http.Error(w, "No se pudo preparar la vista HTML.", http.StatusInternalServerError)
		return
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		http.Error(w, "El HTML no contiene texto UTF-8 válido.", http.StatusUnsupportedMediaType)
		return
	}

	// El HTML del usuario nunca se ejecuta con los privilegios/origen de Nube:
	// se encierra en un iframe sandbox y esta segunda CSP bloquea scripts, red,
	// formularios, navegación, plugins y recursos externos. Se permiten estilos
	// inline e imágenes data: para conservar una vista estática útil.
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'none'; connect-src 'none'; style-src 'unsafe-inline'; img-src data: blob:; media-src data: blob:; font-src data:; object-src 'none'; frame-src 'none'; child-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'; sandbox")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if disposition := mime.FormatMediaType("inline", map[string]string{"filename": entry.Name}); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	_, _ = w.Write(data)
}

func (a *App) writeViewerOpenError(w http.ResponseWriter, r *http.Request, file catalog.File, err error) {
	if errors.Is(err, storagepkg.ErrOffline) {
		http.Error(w, "La unidad que contiene este archivo no está conectada.", http.StatusServiceUnavailable)
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		a.forgetMissingFile(r.Context(), file)
		http.Error(w, "El archivo fue eliminado fuera de Personal Cloud y se retiró del catálogo.", http.StatusNotFound)
		return
	}
	http.Error(w, "No se pudo abrir el archivo.", http.StatusInternalServerError)
}
