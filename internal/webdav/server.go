package webdav

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"personalcloud/internal/vfs"
)

type Server struct {
	FS             *vfs.FS
	Prefix         string
	MaxUploadBytes int64
	OnMutation     func(context.Context, string)
	locks          *lockManager
}

func New(fs *vfs.FS, prefix string, maxUploadBytes int64) *Server {
	if prefix == "" {
		prefix = "/webdav"
	}
	prefix = "/" + strings.Trim(strings.TrimSpace(prefix), "/")
	return &Server{FS: fs, Prefix: prefix, MaxUploadBytes: maxUploadBytes, locks: newLockManager()}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	virtualPath, ok := s.virtualPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodOptions:
		s.options(w)
	case "PROPFIND":
		s.propfind(w, r, virtualPath)
	case http.MethodGet, http.MethodHead:
		s.get(w, r, virtualPath)
	case http.MethodPut:
		s.put(w, r, virtualPath)
	case http.MethodDelete:
		s.delete(w, r, virtualPath)
	case "MKCOL":
		s.mkcol(w, r, virtualPath)
	case "MOVE":
		s.move(w, r, virtualPath)
	case "COPY":
		s.copy(w, r, virtualPath)
	case "LOCK":
		s.lock(w, r, virtualPath)
	case "UNLOCK":
		s.unlock(w, r, virtualPath)
	default:
		w.Header().Set("Allow", "OPTIONS, PROPFIND, GET, HEAD, PUT, DELETE, MKCOL, MOVE, COPY, LOCK, UNLOCK")
		http.Error(w, "Método WebDAV no soportado.", http.StatusMethodNotAllowed)
	}
}

func (s *Server) notifyMutation(ctx context.Context, virtualPath string) {
	if s.OnMutation != nil {
		s.OnMutation(ctx, virtualPath)
	}
}

func (s *Server) options(w http.ResponseWriter) {
	w.Header().Set("DAV", "1, 2")
	w.Header().Set("MS-Author-Via", "DAV")
	w.Header().Set("Allow", "OPTIONS, PROPFIND, GET, HEAD, PUT, DELETE, MKCOL, MOVE, COPY, LOCK, UNLOCK")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) propfind(w http.ResponseWriter, r *http.Request, virtualPath string) {
	depth := strings.TrimSpace(r.Header.Get("Depth"))
	if depth == "" {
		depth = "1"
	}
	if depth != "0" && depth != "1" {
		http.Error(w, "Depth infinity no está permitido.", http.StatusForbidden)
		return
	}
	entry, err := s.FS.Stat(r.Context(), virtualPath)
	if err != nil {
		writeVFSError(w, err)
		return
	}
	entries := []vfs.Entry{entry}
	if depth == "1" && entry.IsDir {
		children, err := s.FS.ReadDir(r.Context(), virtualPath)
		if err != nil {
			writeVFSError(w, err)
			return
		}
		entries = append(entries, children...)
	}
	w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?>`)
	_, _ = io.WriteString(w, `<D:multistatus xmlns:D="DAV:">`)
	for _, item := range entries {
		writePropResponse(w, s.href(item.VirtualPath), item)
	}
	_, _ = io.WriteString(w, `</D:multistatus>`)
}

func (s *Server) get(w http.ResponseWriter, r *http.Request, virtualPath string) {
	handle, entry, err := s.FS.OpenRead(r.Context(), virtualPath)
	if err != nil {
		writeVFSError(w, err)
		return
	}
	defer handle.Close()
	http.ServeContent(w, r, entry.Name, entry.ModTime, handle.File)
}

func (s *Server) put(w http.ResponseWriter, r *http.Request, virtualPath string) {
	if !s.locks.allowed(virtualPath, suppliedLockToken(r)) {
		http.Error(w, "Recurso bloqueado.", http.StatusLocked)
		return
	}
	_, statErr := s.FS.Stat(r.Context(), virtualPath)
	existed := statErr == nil
	written, err := s.FS.WriteAtomic(r.Context(), virtualPath, r.Body, s.MaxUploadBytes, true)
	if err != nil {
		writeVFSError(w, err)
		return
	}
	w.Header().Set("Content-Length", "0")
	w.Header().Set("X-Bytes-Written", strconv.FormatInt(written, 10))
	s.notifyMutation(r.Context(), virtualPath)
	if existed {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request, virtualPath string) {
	if !s.locks.allowed(virtualPath, suppliedLockToken(r)) {
		http.Error(w, "Recurso bloqueado.", http.StatusLocked)
		return
	}
	if err := s.FS.Remove(r.Context(), virtualPath); err != nil {
		writeVFSError(w, err)
		return
	}
	s.locks.removePath(virtualPath)
	s.notifyMutation(r.Context(), virtualPath)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) mkcol(w http.ResponseWriter, r *http.Request, virtualPath string) {
	if !s.locks.allowed(virtualPath, suppliedLockToken(r)) {
		http.Error(w, "Recurso bloqueado.", http.StatusLocked)
		return
	}
	if r.ContentLength > 0 {
		http.Error(w, "MKCOL con cuerpo no soportado.", http.StatusUnsupportedMediaType)
		return
	}
	if err := s.FS.Mkdir(r.Context(), virtualPath); err != nil {
		writeVFSError(w, err)
		return
	}
	s.notifyMutation(r.Context(), virtualPath)
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) move(w http.ResponseWriter, r *http.Request, virtualPath string) {
	if !s.locks.allowed(virtualPath, suppliedLockToken(r)) {
		http.Error(w, "Recurso bloqueado.", http.StatusLocked)
		return
	}
	destination, ok := s.destinationPath(r)
	if !ok {
		http.Error(w, "Destination inválido.", http.StatusBadRequest)
		return
	}
	overwrite := !strings.EqualFold(strings.TrimSpace(r.Header.Get("Overwrite")), "F")
	if err := s.FS.Rename(r.Context(), virtualPath, destination, overwrite); err != nil {
		writeVFSError(w, err)
		return
	}
	s.locks.move(virtualPath, destination)
	s.notifyMutation(r.Context(), destination)
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) copy(w http.ResponseWriter, r *http.Request, virtualPath string) {
	destination, ok := s.destinationPath(r)
	if !ok {
		http.Error(w, "Destination inválido.", http.StatusBadRequest)
		return
	}
	overwrite := !strings.EqualFold(strings.TrimSpace(r.Header.Get("Overwrite")), "F")
	handle, entry, err := s.FS.OpenRead(r.Context(), virtualPath)
	if err != nil {
		writeVFSError(w, err)
		return
	}
	defer handle.Close()
	if entry.IsDir {
		// deuda-tecnica: COPY recursivo no persiste todavía; implementarlo si un cliente WebDAV real lo requiere.
		http.Error(w, "COPY recursivo de directorios aún no está soportado.", http.StatusNotImplemented)
		return
	}
	if _, err := s.FS.WriteAtomic(r.Context(), destination, handle.File, s.MaxUploadBytes, overwrite); err != nil {
		writeVFSError(w, err)
		return
	}
	s.notifyMutation(r.Context(), destination)
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) lock(w http.ResponseWriter, r *http.Request, virtualPath string) {
	timeout := parseLockTimeout(r.Header.Get("Timeout"))
	token, err := s.locks.lock(virtualPath, timeout, suppliedLockToken(r))
	if err != nil {
		http.Error(w, "Recurso bloqueado.", http.StatusLocked)
		return
	}
	w.Header().Set("Lock-Token", "<"+token+">")
	w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?><D:prop xmlns:D="DAV:"><D:lockdiscovery><D:activelock><D:locktype><D:write/></D:locktype><D:lockscope><D:exclusive/></D:lockscope><D:depth>infinity</D:depth><D:timeout>Second-%d</D:timeout><D:locktoken><D:href>%s</D:href></D:locktoken></D:activelock></D:lockdiscovery></D:prop>`, int(timeout.Seconds()), xmlEscape(token))
}

func (s *Server) unlock(w http.ResponseWriter, r *http.Request, virtualPath string) {
	token := strings.Trim(strings.TrimSpace(r.Header.Get("Lock-Token")), "<>")
	if token == "" || !s.locks.unlock(virtualPath, token) {
		http.Error(w, "Token de bloqueo inválido.", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) virtualPath(requestPath string) (string, bool) {
	if requestPath == s.Prefix || requestPath == s.Prefix+"/" {
		return "/", true
	}
	if !strings.HasPrefix(requestPath, s.Prefix+"/") {
		return "", false
	}
	return "/" + strings.TrimPrefix(requestPath, s.Prefix+"/"), true
}

func (s *Server) destinationPath(r *http.Request) (string, bool) {
	raw := strings.TrimSpace(r.Header.Get("Destination"))
	if raw == "" {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, r.Host) {
		return "", false
	}
	return s.virtualPath(parsed.Path)
}

func (s *Server) href(virtualPath string) string {
	if virtualPath == "/" {
		return s.Prefix + "/"
	}
	parts := strings.Split(strings.TrimPrefix(virtualPath, "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return s.Prefix + "/" + strings.Join(parts, "/")
}

func writePropResponse(w io.Writer, href string, entry vfs.Entry) {
	resourceType := ""
	contentLength := strconv.FormatInt(entry.Size, 10)
	if entry.IsDir {
		resourceType = `<D:collection/>`
		contentLength = "0"
	}
	_, _ = fmt.Fprintf(w, `<D:response><D:href>%s</D:href><D:propstat><D:prop><D:displayname>%s</D:displayname><D:resourcetype>%s</D:resourcetype><D:getcontentlength>%s</D:getcontentlength><D:getlastmodified>%s</D:getlastmodified></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>`,
		xmlEscape(href), xmlEscape(entry.Name), resourceType, contentLength, xmlEscape(entry.ModTime.UTC().Format(http.TimeFormat)))
}

func xmlEscape(value string) string {
	var builder strings.Builder
	_ = xml.EscapeText(&builder, []byte(value))
	return builder.String()
}

func writeVFSError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, os.ErrNotExist):
		http.Error(w, "Recurso no encontrado.", http.StatusNotFound)
	case errors.Is(err, os.ErrPermission):
		http.Error(w, "Acceso denegado.", http.StatusForbidden)
	case errors.Is(err, os.ErrExist):
		http.Error(w, "El recurso ya existe.", http.StatusPreconditionFailed)
	case errors.Is(err, vfs.ErrCrossVolume):
		http.Error(w, "La operación entre unidades distintas no está soportada.", http.StatusBadGateway)
	default:
		http.Error(w, "No se pudo completar la operación WebDAV.", http.StatusInternalServerError)
	}
}

func suppliedLockToken(r *http.Request) string {
	if token := strings.Trim(strings.TrimSpace(r.Header.Get("Lock-Token")), "<>"); token != "" {
		return token
	}
	value := r.Header.Get("If")
	start := strings.Index(value, "opaquelocktoken:")
	if start < 0 {
		return ""
	}
	end := start
	for end < len(value) && !strings.ContainsRune("> )", rune(value[end])) {
		end++
	}
	return value[start:end]
}

func parseLockTimeout(value string) time.Duration {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "second-") {
		seconds, err := strconv.Atoi(strings.TrimSpace(value[len("Second-"):]))
		if err == nil {
			if seconds < 30 {
				seconds = 30
			}
			if seconds > 3600 {
				seconds = 3600
			}
			return time.Duration(seconds) * time.Second
		}
	}
	return 30 * time.Minute
}

type lockRecord struct {
	Path    string
	Token   string
	Expires time.Time
}

type lockManager struct {
	mu    sync.Mutex
	locks map[string]lockRecord
}

func newLockManager() *lockManager { return &lockManager{locks: make(map[string]lockRecord)} }

func (m *lockManager) lock(path string, timeout time.Duration, supplied string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked()
	key := cleanLockPath(path)
	if current, ok := m.locks[key]; ok {
		if supplied != current.Token {
			return "", errors.New("locked")
		}
		current.Expires = time.Now().Add(timeout)
		m.locks[key] = current
		return current.Token, nil
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := "opaquelocktoken:" + hex.EncodeToString(buf)
	m.locks[key] = lockRecord{Path: key, Token: token, Expires: time.Now().Add(timeout)}
	return token, nil
}

func (m *lockManager) unlock(path, token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked()
	key := cleanLockPath(path)
	current, ok := m.locks[key]
	if !ok || current.Token != token {
		return false
	}
	delete(m.locks, key)
	return true
}

func (m *lockManager) allowed(path, token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked()
	key := cleanLockPath(path)
	for lockedPath, current := range m.locks {
		if key == lockedPath || strings.HasPrefix(key, lockedPath+"/") || strings.HasPrefix(lockedPath, key+"/") {
			return token != "" && current.Token == token
		}
	}
	return true
}

func (m *lockManager) removePath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := cleanLockPath(path)
	for lockedPath := range m.locks {
		if lockedPath == key || strings.HasPrefix(lockedPath, key+"/") {
			delete(m.locks, lockedPath)
		}
	}
}

func (m *lockManager) move(from, to string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fromKey, toKey := cleanLockPath(from), cleanLockPath(to)
	if current, ok := m.locks[fromKey]; ok {
		delete(m.locks, fromKey)
		current.Path = toKey
		m.locks[toKey] = current
	}
}

func (m *lockManager) cleanupLocked() {
	now := time.Now()
	for key, record := range m.locks {
		if record.Expires.Before(now) {
			delete(m.locks, key)
		}
	}
}

func cleanLockPath(value string) string {
	clean := path.Clean("/" + strings.TrimPrefix(value, "/"))
	if clean == "." {
		return "/"
	}
	return clean
}

func ContextWithUser(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, webDAVUserKey{}, username)
}

type webDAVUserKey struct{}
