package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	storagepkg "personalcloud/internal/storage"
)

const (
	downloadTicketTTL = 2 * time.Minute
	downloadTicketAAD = "personalcloud-download-v1"
)

type downloadTicket struct {
	FileID  string `json:"file_id"`
	UserID  string `json:"user_id"`
	Expires int64  `json:"expires"`
}

func (a *App) downloadTicketPost(w http.ResponseWriter, r *http.Request) {
	if !a.requestIsHTTPS(r) && !isLoopbackIP(a.clientIP(r)) {
		http.Error(w, "Las descargas remotas requieren HTTPS.", http.StatusUpgradeRequired)
		return
	}
	if !a.parseProtectedForm(w, r) {
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		http.Error(w, "Sesión no válida.", http.StatusUnauthorized)
		return
	}
	if ok, wait := a.limiter.Allow("download:"+user.ID+":"+a.clientIP(r), downloadTicketPolicy); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retrySeconds(wait)))
		http.Error(w, "Demasiadas solicitudes de descarga.", http.StatusTooManyRequests)
		return
	}
	file, ok := a.catalog.ByID(strings.TrimSpace(r.FormValue("file_id")))
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !a.storageOnline(r, file.StorageID) {
		http.Error(w, "La unidad que contiene este archivo no está conectada.", http.StatusServiceUnavailable)
		return
	}
	token, err := a.encryptDownloadTicket(downloadTicket{FileID: file.ID, UserID: user.ID, Expires: time.Now().UTC().Add(downloadTicketTTL).Unix()})
	if err != nil {
		a.logger.Error("no se pudo crear ticket de descarga", "error", err)
		http.Error(w, "No se pudo preparar la descarga.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	_ = json.NewEncoder(w).Encode(map[string]string{"url": "/descarga/" + token})
}

func (a *App) secureDownloadGet(w http.ResponseWriter, r *http.Request) {
	if !a.requestIsHTTPS(r) && !isLoopbackIP(a.clientIP(r)) {
		http.Error(w, "Las descargas remotas requieren HTTPS.", http.StatusUpgradeRequired)
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		http.NotFound(w, r)
		return
	}
	ticket, err := a.decryptDownloadTicket(r.PathValue("token"))
	if err != nil || ticket.UserID != user.ID || ticket.Expires < time.Now().UTC().Unix() {
		http.NotFound(w, r)
		return
	}
	file, ok := a.catalog.ByID(ticket.FileID)
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
		http.Error(w, "No se pudo abrir el archivo.", http.StatusInternalServerError)
		return
	}
	defer handle.Close()

	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": entry.Name})
	if disposition == "" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	_ = a.store.Audit(r.Context(), user.ID, "file_download", "correcto", a.clientIP(r))
	http.ServeContent(w, r, entry.Name, entry.ModTime, handle.File)
}

func (a *App) encryptDownloadTicket(ticket downloadTicket) (string, error) {
	block, err := aes.NewCipher(a.downloadSecret[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := json.Marshal(ticket)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, plain, []byte(downloadTicketAAD))
	payload := append(nonce, sealed...)
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func (a *App) decryptDownloadTicket(token string) (downloadTicket, error) {
	if len(token) < 32 || len(token) > 2048 {
		return downloadTicket{}, errors.New("ticket inválido")
	}
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return downloadTicket{}, errors.New("ticket inválido")
	}
	block, err := aes.NewCipher(a.downloadSecret[:])
	if err != nil {
		return downloadTicket{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return downloadTicket{}, err
	}
	if len(payload) <= gcm.NonceSize() {
		return downloadTicket{}, errors.New("ticket inválido")
	}
	plain, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], []byte(downloadTicketAAD))
	if err != nil {
		return downloadTicket{}, errors.New("ticket inválido")
	}
	var ticket downloadTicket
	if err := json.Unmarshal(plain, &ticket); err != nil || ticket.FileID == "" || ticket.UserID == "" {
		return downloadTicket{}, errors.New("ticket inválido")
	}
	return ticket, nil
}

func (a *App) storageOnline(r *http.Request, storageID string) bool {
	views, _ := a.storageManager.Views(r.Context())
	for _, view := range views {
		if view.Registered && view.ID == storageID {
			return view.Online
		}
	}
	return false
}
