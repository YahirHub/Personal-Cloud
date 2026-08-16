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
	"net/url"
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

const (
	shareAuthCookiePrefix = "pc_share_auth_"
	shareEmbedAccessTTL   = 2 * time.Hour
)

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
	// Los grants de recursos nunca deben convertir la URL pública en una
	// credencial portadora. Versiones anteriores dejaban ?access= en la barra
	// de direcciones después del desbloqueo; cualquier navegación GET que aún
	// lo traiga se canonicaliza inmediatamente a la URL limpia.
	if strings.TrimSpace(r.URL.Query().Get("access")) != "" {
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
		return
	}
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
	authorized := a.publicSharePageAuthorized(r, share)
	view := a.publicShareViewFor(r, share, file, "")
	data := pageData{
		Title: file.Name, Description: "Archivo compartido públicamente desde Nube.", PublicSharePage: true,
		PublicShare: &view, SharePasswordRequired: !authorized, ShareEmbed: embed,
	}
	if authorized {
		_ = a.store.TouchPublicShare(r.Context(), share.ID, time.Now().UTC())
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if embed {
		setPublicEmbedHeaders(w)
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
		w.Header().Set("Cache-Control", "private, no-store")
		if isEmbed {
			setPublicEmbedHeaders(w)
		}
		a.render(w, http.StatusUnauthorized, "public_share", data)
		return
	}
	a.setShareCookie(w, share)
	isEmbed := strings.HasSuffix(r.URL.Path, "/embed")
	if !isEmbed {
		// La navegación normal queda autorizada exclusivamente por una cookie
		// HttpOnly de sesión. La barra de direcciones nunca contiene un grant.
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
		return
	}

	// En iframes de terceros algunos navegadores bloquean cookies. Para no
	// romper el embed protegido, la respuesta POST se renderiza directamente
	// y sólo sus subrecursos reciben un grant temporal. Ese grant no aparece
	// en la URL del iframe y además sólo es válido con Referer del share exacto.
	file, exists := a.catalog.ByID(share.FileID)
	if !exists {
		http.NotFound(w, r)
		return
	}
	access := a.newShareAccessTicket(share, shareEmbedAccessTTL)
	view := a.publicShareViewFor(r, share, file, access)
	data := pageData{
		Title: file.Name, Description: "Archivo compartido protegido.", PublicSharePage: true,
		PublicShare: &view, SharePasswordRequired: false, ShareEmbed: true,
	}
	_ = a.store.TouchPublicShare(r.Context(), share.ID, time.Now().UTC())
	w.Header().Set("Cache-Control", "private, no-store")
	setPublicEmbedHeaders(w)
	a.render(w, http.StatusOK, "public_share", data)
}

func (a *App) publicShareVideoAccess(w http.ResponseWriter, r *http.Request) (store.PublicShare, catalog.File, bool) {
	share, err := a.store.PublicShareByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		http.NotFound(w, r)
		return store.PublicShare{}, catalog.File{}, false
	}
	if !a.publicShareResourceAuthorized(r, share) {
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
	share, file, ok := a.publicShareVideoAccess(w, r)
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
	if share.PasswordHash != "" {
		w.Header().Set("Cache-Control", "private, no-store")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=21600")
	}
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
	if !a.publicShareResourceAuthorized(r, share) {
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

func setPublicEmbedHeaders(w http.ResponseWriter) {
	w.Header().Del("X-Frame-Options")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; media-src 'self'; connect-src 'self'; object-src 'none'; frame-ancestors *; base-uri 'self'; form-action 'self'")
}

func (a *App) publicSharePageAuthorized(r *http.Request, share store.PublicShare) bool {
	return share.PasswordHash == "" || a.validShareCookie(r, share)
}

func (a *App) publicShareResourceAuthorized(r *http.Request, share store.PublicShare) bool {
	if share.PasswordHash == "" || a.validShareCookie(r, share) {
		return true
	}
	return a.validShareSubresourceAccess(r, share)
}

func (a *App) validShareSubresourceAccess(r *http.Request, share store.PublicShare) bool {
	access := strings.TrimSpace(r.URL.Query().Get("access"))
	if !a.validShareAccessTicket(share, access) {
		return false
	}
	// Un grant de embed no es una segunda contraseña ni un enlace público.
	// Sólo autoriza solicitudes iniciadas por la página pública exacta. Pegar
	// /contenido?access=... en otra ventana/incógnito carece de este Referer.
	referer := strings.TrimSpace(r.Referer())
	if referer == "" {
		return false
	}
	parsed, err := url.Parse(referer)
	if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Host, r.Host) {
		return false
	}
	base := "/s/" + share.Token
	if parsed.Path != base && parsed.Path != base+"/embed" {
		return false
	}
	if site := strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")); site != "" && site != "same-origin" {
		return false
	}
	return true
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
		PasswordProtected: share.PasswordHash != "", Available: a.storageOnline(r, file.StorageID),
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
	// Cookie de sesión: cerrar la sesión del navegador elimina el desbloqueo.
	// No se persiste una autorización de 12 horas en disco.
	http.SetCookie(w, &http.Cookie{Name: shareCookieName(share), Value: a.shareCookieSignature(share), Path: "/s/", HttpOnly: true, Secure: a.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
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
	if ttl <= 0 || ttl > shareEmbedAccessTTL {
		ttl = shareEmbedAccessTTL
	}
	exp := time.Now().UTC().Add(ttl).Unix()
	expText := strconv.FormatInt(exp, 10)
	payload := "v2|subresource|" + share.ID + "|" + share.Token + "|" + expText + "|" + share.PasswordHash
	mac := hmac.New(sha256.New, a.shareSecret[:])
	_, _ = mac.Write([]byte(payload))
	return "v2." + expText + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *App) validShareAccessTicket(share store.PublicShare, ticket string) bool {
	parts := strings.Split(ticket, ".")
	if len(parts) != 3 || parts[0] != "v2" {
		return false
	}
	now := time.Now().UTC()
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || exp < now.Unix() || exp > now.Add(shareEmbedAccessTTL+time.Minute).Unix() {
		return false
	}
	payload := "v2|subresource|" + share.ID + "|" + share.Token + "|" + parts[1] + "|" + share.PasswordHash
	mac := hmac.New(sha256.New, a.shareSecret[:])
	_, _ = mac.Write([]byte(payload))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(parts[2]), []byte(expected))
}
