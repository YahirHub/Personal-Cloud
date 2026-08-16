package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"personalcloud/internal/auth"
	"personalcloud/internal/catalog"
	storagepkg "personalcloud/internal/storage"
	"personalcloud/internal/store"
	"personalcloud/internal/streaming"
)

const shareAuthCookiePrefix = "pc_share_auth_"

type sharePageItem struct {
	ID                string
	FileID            string
	Name              string
	Kind              string
	Size              int64
	Owner             string
	URL               string
	EmbedURL          string
	VideoQualitiesURL string
	VideoPrepareURL   string
	VideoStatusURL    string
	PasswordProtected bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastAccessAt      time.Time
	AccessCount       uint64
	Available         bool
}

type publicShareView struct {
	ID                string
	Token             string
	FileID            string
	Name              string
	Kind              string
	MIME              string
	Size              int64
	Viewer            string
	ContentURL        string
	DownloadURL       string
	EmbedURL          string
	VideoQualitiesURL string
	VideoPrepareURL   string
	VideoStatusURL    string
	PasswordProtected bool
	Available         bool
	AccessToken       string
}

func (a *App) absoluteURL(r *http.Request, value string) string {
	scheme := "http"
	if a.requestIsHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host + value
}

func (a *App) canManageShare(user *store.User, share store.PublicShare) bool {
	return user != nil && (user.Role == "admin" || share.OwnerUserID == user.ID)
}

func (a *App) allowShareManage(w http.ResponseWriter, r *http.Request, user *store.User) bool {
	if user == nil {
		writeJSONError(w, errors.New("sesión no válida"), http.StatusUnauthorized)
		return false
	}
	if ok, wait := a.limiter.Allow("share-manage:"+user.ID+":"+a.clientIP(r), shareManagePolicy); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retrySeconds(wait)))
		writeJSONError(w, errors.New("demasiadas operaciones de compartir"), http.StatusTooManyRequests)
		return false
	}
	return true
}

func (a *App) sharedFilesGet(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	shares, err := a.store.PublicShares(r.Context())
	if err != nil {
		http.Error(w, "No se pudieron leer los enlaces compartidos.", http.StatusInternalServerError)
		return
	}
	sort.SliceStable(shares, func(i, j int) bool { return shares[i].UpdatedAt.After(shares[j].UpdatedAt) })
	users := map[string]string{}
	for _, u := range a.store.UsersSnapshot() {
		users[u.ID] = u.Username
	}
	items := make([]sharePageItem, 0, len(shares))
	for _, share := range shares {
		if user.Role != "admin" && share.OwnerUserID != user.ID {
			continue
		}
		file, ok := a.catalog.ByID(share.FileID)
		name, kind, size, available := "Archivo no disponible", "other", int64(0), false
		if ok {
			name, kind, size = file.Name, file.Kind, file.Size
			available = a.storageOnline(r, file.StorageID)
		}
		items = append(items, sharePageItem{
			ID: share.ID, FileID: share.FileID, Name: name, Kind: kind, Size: size,
			Owner: users[share.OwnerUserID], URL: a.absoluteURL(r, "/s/"+share.Token),
			EmbedURL: a.absoluteURL(r, "/s/"+share.Token+"/embed"), PasswordProtected: share.PasswordHash != "",
			CreatedAt: share.CreatedAt, UpdatedAt: share.UpdatedAt, LastAccessAt: share.LastAccessAt,
			AccessCount: share.AccessCount, Available: available,
		})
	}
	data := a.csrfData(w, r, pageData{
		Title: "Compartidos", Description: "Gestiona los enlaces públicos de tus archivos.", CurrentPath: "/compartidos", User: user, Shares: items,
	})
	data.Info = r.URL.Query().Get("ok")
	data.Error = r.URL.Query().Get("error")
	a.render(w, http.StatusOK, "shared", data)
}

func (a *App) fileShareInfoGet(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	file, ok := a.catalog.ByID(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	share, err := a.store.PublicShareByFile(r.Context(), user.ID, file.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, map[string]any{"shared": false, "file_id": file.ID, "name": file.Name})
		return
	}
	if err != nil {
		writeJSONError(w, errors.New("no se pudo leer el enlace compartido"), http.StatusInternalServerError)
		return
	}
	writeJSON(w, a.shareJSON(r, share, file))
}

func (a *App) fileSharePost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	user := userFromContext(r.Context())
	if !a.allowShareManage(w, r, user) {
		return
	}
	file, ok := a.catalog.ByID(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	mode := strings.TrimSpace(r.FormValue("access_mode"))
	if mode != "public" && mode != "password" {
		writeJSONError(w, errors.New("modo de acceso inválido"), http.StatusBadRequest)
		return
	}
	existing, existingErr := a.store.PublicShareByFile(r.Context(), user.ID, file.ID)
	passwordHash := ""
	if mode == "password" {
		password := r.FormValue("password")
		if password == "" && existingErr == nil && existing.PasswordHash != "" {
			passwordHash = existing.PasswordHash
		} else {
			var err error
			passwordHash, err = auth.HashSharePassword(password)
			if err != nil {
				writeJSONError(w, err, http.StatusBadRequest)
				return
			}
		}
	}
	token, _, err := auth.NewSessionToken()
	if err != nil {
		writeJSONError(w, errors.New("no se pudo generar el enlace"), http.StatusInternalServerError)
		return
	}
	share, err := a.store.UpsertPublicShare(r.Context(), user.ID, file.ID, token, passwordHash)
	if err != nil {
		writeJSONError(w, errors.New("no se pudo guardar el enlace compartido"), http.StatusInternalServerError)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "share_upsert", "correcto", a.clientIP(r))
	writeJSON(w, a.shareJSON(r, share, file))
}

func (a *App) shareInfoGet(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	share, err := a.store.PublicShareByID(r.Context(), r.PathValue("id"))
	if err != nil || !a.canManageShare(user, share) {
		http.NotFound(w, r)
		return
	}
	file, ok := a.catalog.ByID(share.FileID)
	if !ok {
		writeJSONError(w, errors.New("el archivo ya no existe en el catálogo"), http.StatusGone)
		return
	}
	writeJSON(w, a.shareJSON(r, share, file))
}

func (a *App) shareConfigurePost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	user := userFromContext(r.Context())
	if !a.allowShareManage(w, r, user) {
		return
	}
	share, err := a.store.PublicShareByID(r.Context(), r.PathValue("id"))
	if err != nil || !a.canManageShare(user, share) {
		http.NotFound(w, r)
		return
	}
	file, ok := a.catalog.ByID(share.FileID)
	if !ok {
		writeJSONError(w, errors.New("el archivo ya no existe en el catálogo"), http.StatusGone)
		return
	}
	mode := strings.TrimSpace(r.FormValue("access_mode"))
	if mode != "public" && mode != "password" {
		writeJSONError(w, errors.New("modo de acceso inválido"), http.StatusBadRequest)
		return
	}
	passwordHash := ""
	if mode == "password" {
		password := r.FormValue("password")
		if password == "" && share.PasswordHash != "" {
			passwordHash = share.PasswordHash
		} else {
			passwordHash, err = auth.HashSharePassword(password)
			if err != nil {
				writeJSONError(w, err, http.StatusBadRequest)
				return
			}
		}
	}
	share, err = a.store.SetPublicSharePassword(r.Context(), share.ID, passwordHash)
	if err != nil {
		writeJSONError(w, errors.New("no se pudo actualizar el enlace compartido"), http.StatusInternalServerError)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "share_configure", "correcto", a.clientIP(r))
	writeJSON(w, a.shareJSON(r, share, file))
}

func (a *App) shareRenewPost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	user := userFromContext(r.Context())
	if !a.allowShareManage(w, r, user) {
		return
	}
	share, err := a.store.PublicShareByID(r.Context(), r.PathValue("id"))
	if err != nil || !a.canManageShare(user, share) {
		http.NotFound(w, r)
		return
	}
	token, _, err := auth.NewSessionToken()
	if err != nil {
		writeJSONError(w, errors.New("no se pudo renovar el enlace"), http.StatusInternalServerError)
		return
	}
	share, err = a.store.RenewPublicShare(r.Context(), share.ID, token)
	if err != nil {
		writeJSONError(w, errors.New("no se pudo renovar el enlace"), http.StatusInternalServerError)
		return
	}
	file, ok := a.catalog.ByID(share.FileID)
	if !ok {
		writeJSONError(w, errors.New("el archivo ya no existe en el catálogo"), http.StatusGone)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "share_renew", "correcto", a.clientIP(r))
	writeJSON(w, a.shareJSON(r, share, file))
}

func (a *App) shareDeletePost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	user := userFromContext(r.Context())
	if !a.allowShareManage(w, r, user) {
		return
	}
	share, err := a.store.PublicShareByID(r.Context(), r.PathValue("id"))
	if err != nil || !a.canManageShare(user, share) {
		http.NotFound(w, r)
		return
	}
	if err := a.store.DeletePublicShare(r.Context(), share.ID); err != nil {
		writeJSONError(w, errors.New("no se pudo dejar de compartir"), http.StatusInternalServerError)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "share_delete", "correcto", a.clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) sharesDeleteAllPost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	user := userFromContext(r.Context())
	if !a.allowShareManage(w, r, user) {
		return
	}
	ownerID := user.ID
	if user.Role == "admin" && strings.EqualFold(r.FormValue("scope"), "all") {
		ownerID = ""
	}
	count, err := a.store.DeletePublicSharesByOwner(r.Context(), ownerID)
	if err != nil {
		writeJSONError(w, errors.New("no se pudieron eliminar los enlaces"), http.StatusInternalServerError)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "shares_delete_all", fmt.Sprintf("correcto:%d", count), a.clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "deleted": count})
}

func (a *App) shareJSON(r *http.Request, share store.PublicShare, file catalog.File) map[string]any {
	return map[string]any{
		"shared": true, "id": share.ID, "file_id": share.FileID, "name": file.Name,
		"url": a.absoluteURL(r, "/s/"+share.Token), "embed_url": a.absoluteURL(r, "/s/"+share.Token+"/embed"),
		"password_protected": share.PasswordHash != "", "created_at": share.CreatedAt, "updated_at": share.UpdatedAt,
		"last_access_at": share.LastAccessAt, "access_count": share.AccessCount,
	}
}

func (a *App) publicShareGet(w http.ResponseWriter, r *http.Request) {
	a.publicShareRender(w, r, false)
}

func (a *App) publicShareEmbedGet(w http.ResponseWriter, r *http.Request) {
	a.publicShareRender(w, r, true)
}

func (a *App) publicShareRender(w http.ResponseWriter, r *http.Request, embed bool) {
	share, err := a.store.PublicShareByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, ok := a.catalog.ByID(share.FileID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	access := strings.TrimSpace(r.URL.Query().Get("access"))
	authorized := share.PasswordHash == "" || a.validShareCookie(r, share) || a.validShareAccessTicket(share, access)
	view := a.publicShareViewFor(r, share, file, access)
	data := pageData{
		Title: file.Name, Description: "Archivo compartido públicamente desde Nube.", PublicSharePage: true,
		PublicShare: &view, SharePasswordRequired: !authorized, ShareEmbed: embed,
	}
	if authorized {
		_ = a.store.TouchPublicShare(r.Context(), share.ID, time.Now().UTC())
	}
	if embed {
		w.Header().Del("X-Frame-Options")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; media-src 'self'; connect-src 'self'; object-src 'none'; frame-ancestors *; base-uri 'self'; form-action 'self'")
	}
	a.render(w, http.StatusOK, "public_share", data)
}

func (a *App) publicSharePasswordPost(w http.ResponseWriter, r *http.Request) {
	share, err := a.store.PublicShareByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if share.PasswordHash == "" {
		http.Redirect(w, r, strings.TrimSuffix(r.URL.Path, "/"), http.StatusSeeOther)
		return
	}
	if ok, wait := a.limiter.Allow("share-password:"+share.ID+":"+a.clientIP(r), sharePasswordPolicy); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retrySeconds(wait)))
		http.Error(w, "Demasiados intentos. Inténtalo de nuevo más tarde.", http.StatusTooManyRequests)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido.", http.StatusBadRequest)
		return
	}
	ok, verifyErr := auth.VerifyPassword(share.PasswordHash, r.FormValue("password"))
	if verifyErr != nil || !ok {
		file, exists := a.catalog.ByID(share.FileID)
		if !exists {
			http.NotFound(w, r)
			return
		}
		view := a.publicShareViewFor(r, share, file, "")
		isEmbed := strings.HasSuffix(r.URL.Path, "/embed")
		data := pageData{Title: file.Name, Description: "Archivo compartido protegido.", PublicSharePage: true, PublicShare: &view, SharePasswordRequired: true, SharePasswordError: "La contraseña no es correcta.", ShareEmbed: isEmbed}
		if isEmbed {
			w.Header().Del("X-Frame-Options")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; media-src 'self'; connect-src 'self'; object-src 'none'; frame-ancestors *; base-uri 'self'; form-action 'self'")
		}
		a.render(w, http.StatusUnauthorized, "public_share", data)
		return
	}
	a.setShareCookie(w, share)
	access := a.newShareAccessTicket(share, 12*time.Hour)
	target := r.URL.Path + "?access=" + access
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (a *App) publicShareVideoAccess(w http.ResponseWriter, r *http.Request) (store.PublicShare, catalog.File, bool) {
	share, err := a.store.PublicShareByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		http.NotFound(w, r)
		return store.PublicShare{}, catalog.File{}, false
	}
	if share.PasswordHash != "" && !a.validShareCookie(r, share) && !a.validShareAccessTicket(share, r.URL.Query().Get("access")) {
		http.Error(w, "Se requiere contraseña.", http.StatusUnauthorized)
		return store.PublicShare{}, catalog.File{}, false
	}
	file, ok := a.catalog.ByID(share.FileID)
	if !ok || file.Kind != "video" {
		http.NotFound(w, r)
		return store.PublicShare{}, catalog.File{}, false
	}
	return share, file, true
}

func publicShareAccessQuery(r *http.Request) string {
	access := strings.TrimSpace(r.URL.Query().Get("access"))
	if access == "" {
		return ""
	}
	return "?access=" + access
}

func (a *App) publicShareVideoURL(r *http.Request, share store.PublicShare, quality string) string {
	if quality == "original" {
		return "/s/" + share.Token + "/contenido" + publicShareAccessQuery(r)
	}
	return "/s/" + share.Token + "/video/" + quality + publicShareAccessQuery(r)
}

func (a *App) publicShareVideoQualitiesGet(w http.ResponseWriter, r *http.Request) {
	share, file, ok := a.publicShareVideoAccess(w, r)
	if !ok {
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
			item.URL = a.publicShareVideoURL(r, share, "original")
		} else if status, err := a.streamer.Status(file, profile.ID); err == nil {
			item.State = status.State
			if status.State == "ready" {
				item.URL = a.publicShareVideoURL(r, share, profile.ID)
			}
		}
		items = append(items, item)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ffmpeg": a.streamer.Available(), "profiles": items, "width": file.Width, "height": file.Height,
	})
}

func (a *App) publicShareVideoPreparePost(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Personal-Cloud-Share") != "video" {
		http.Error(w, "Solicitud no válida.", http.StatusBadRequest)
		return
	}
	share, file, ok := a.publicShareVideoAccess(w, r)
	if !ok {
		return
	}
	if ok, wait := a.limiter.Allow("share-video:"+share.ID+":"+a.clientIP(r), shareVideoPolicy); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retrySeconds(wait)))
		http.Error(w, "Demasiadas solicitudes de conversión de video.", http.StatusTooManyRequests)
		return
	}
	if !a.storageOnline(r, file.StorageID) {
		http.Error(w, "La unidad que contiene este video no está conectada.", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Solicitud inválida.", http.StatusBadRequest)
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
	if status.State == "ready" {
		status.URL = a.publicShareVideoURL(r, share, quality)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	code := http.StatusAccepted
	if status.State == "ready" {
		code = http.StatusOK
	}
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(status)
}

func (a *App) publicShareVideoStatusGet(w http.ResponseWriter, r *http.Request) {
	share, file, ok := a.publicShareVideoAccess(w, r)
	if !ok {
		return
	}
	quality := strings.TrimSpace(r.URL.Query().Get("quality"))
	status, err := a.streamer.Status(file, quality)
	if err != nil {
		http.Error(w, "Resolución no válida.", http.StatusBadRequest)
		return
	}
	if status.State == "ready" {
		status.URL = a.publicShareVideoURL(r, share, quality)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	_ = json.NewEncoder(w).Encode(status)
}

func (a *App) publicShareVideoVariantGet(w http.ResponseWriter, r *http.Request) {
	_, file, ok := a.publicShareVideoAccess(w, r)
	if !ok {
		return
	}
	quality := strings.TrimSpace(r.PathValue("quality"))
	variantPath, err := a.streamer.VariantPath(file, quality)
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
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": file.Name}))
	w.Header().Set("Cache-Control", "private, max-age=21600")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'self'")
	http.ServeContent(w, r, file.Name, info.ModTime(), handle)
}

func (a *App) publicShareContentGet(w http.ResponseWriter, r *http.Request) {
	share, err := a.store.PublicShareByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if share.PasswordHash != "" && !a.validShareCookie(r, share) && !a.validShareAccessTicket(share, r.URL.Query().Get("access")) {
		http.Error(w, "Se requiere contraseña.", http.StatusUnauthorized)
		return
	}
	file, ok := a.catalog.ByID(share.FileID)
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
			http.NotFound(w, r)
			return
		}
		http.Error(w, "No se pudo abrir el archivo.", http.StatusInternalServerError)
		return
	}
	defer handle.Close()
	contentType := strings.TrimSpace(file.MIME)
	if contentType == "" {
		contentType = mime.TypeByExtension(path.Ext(file.Name))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	download := r.URL.Query().Get("download") == "1"
	if download {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": entry.Name}))
	} else if publicInlineAllowed(file) {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": entry.Name}))
	} else {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": entry.Name}))
	}
	// El contenido pertenece explícitamente a un enlace público. Debe poder
	// vivir dentro de /embed incluso cuando ese reproductor esté anidado en
	// un sitio de otro origen. No heredamos X-Frame-Options del panel privado.
	w.Header().Del("X-Frame-Options")
	if strings.EqualFold(fileViewerKind(file.Name), "html") {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'none'; connect-src 'none'; style-src 'unsafe-inline'; img-src data: blob:; media-src data: blob:; font-src data:; object-src 'none'; frame-src 'none'; child-src 'none'; frame-ancestors *; base-uri 'none'; form-action 'none'; sandbox")
	} else {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors *")
	}
	if fileViewerKind(file.Name) == "markdown" || fileViewerKind(file.Name) == "text" {
		contentType = "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	http.ServeContent(w, r, entry.Name, entry.ModTime, handle.File)
}

func publicInlineAllowed(file catalog.File) bool {
	switch fileViewerKind(file.Name) {
	case "image", "video", "audio", "pdf", "html", "markdown", "text":
		return true
	default:
		return false
	}
}

func (a *App) publicShareViewFor(r *http.Request, share store.PublicShare, file catalog.File, access string) publicShareView {
	query := ""
	if access != "" {
		query = "?access=" + access
	}
	contentURL := "/s/" + share.Token + "/contenido" + query
	downloadSep := "?"
	if query != "" {
		downloadSep = "&"
	}
	downloadURL := contentURL + downloadSep + "download=1"
	viewer := fileViewerKind(file.Name)
	qualityQuery := query
	return publicShareView{
		ID: share.ID, Token: share.Token, FileID: file.ID, Name: file.Name, Kind: file.Kind, MIME: file.MIME, Size: file.Size,
		Viewer: viewer, ContentURL: contentURL, DownloadURL: downloadURL,
		EmbedURL:          a.absoluteURL(r, "/s/"+share.Token+"/embed"),
		VideoQualitiesURL: "/s/" + share.Token + "/video/calidades" + qualityQuery,
		VideoPrepareURL:   "/s/" + share.Token + "/video/preparar" + qualityQuery,
		VideoStatusURL:    "/s/" + share.Token + "/video/estado" + qualityQuery,
		PasswordProtected: share.PasswordHash != "", Available: a.storageOnline(r, file.StorageID), AccessToken: access,
	}
}

func shareCookieName(share store.PublicShare) string {
	return shareAuthCookiePrefix + share.ID
}

func (a *App) shareCookieSignature(share store.PublicShare) string {
	mac := hmac.New(sha256.New, a.shareSecret[:])
	_, _ = mac.Write([]byte(share.ID + "\x00" + share.Token + "\x00" + share.PasswordHash))
	return share.ID + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *App) setShareCookie(w http.ResponseWriter, share store.PublicShare) {
	http.SetCookie(w, &http.Cookie{Name: shareCookieName(share), Value: a.shareCookieSignature(share), Path: "/s/", MaxAge: 12 * 60 * 60, HttpOnly: true, Secure: a.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
}

func (a *App) validShareCookie(r *http.Request, share store.PublicShare) bool {
	cookie, err := r.Cookie(shareCookieName(share))
	if err != nil || cookie.Value == "" {
		return false
	}
	expected := a.shareCookieSignature(share)
	return hmac.Equal([]byte(cookie.Value), []byte(expected))
}

func (a *App) newShareAccessTicket(share store.PublicShare, ttl time.Duration) string {
	exp := time.Now().UTC().Add(ttl).Unix()
	payload := share.ID + "|" + share.Token + "|" + strconv.FormatInt(exp, 10) + "|" + share.PasswordHash
	mac := hmac.New(sha256.New, a.shareSecret[:])
	_, _ = mac.Write([]byte(payload))
	return strconv.FormatInt(exp, 10) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *App) validShareAccessTicket(share store.PublicShare, ticket string) bool {
	parts := strings.Split(ticket, ".")
	if len(parts) != 2 {
		return false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || exp < time.Now().UTC().Unix() || exp > time.Now().UTC().Add(24*time.Hour).Unix() {
		return false
	}
	payload := share.ID + "|" + share.Token + "|" + parts[0] + "|" + share.PasswordHash
	mac := hmac.New(sha256.New, a.shareSecret[:])
	_, _ = mac.Write([]byte(payload))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(parts[1]), []byte(expected))
}
