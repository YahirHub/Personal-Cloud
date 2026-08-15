package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"personalcloud/internal/auth"
	"personalcloud/internal/ratelimit"
	"personalcloud/internal/store"
)

var (
	webDAVIPPolicy    = ratelimit.Policy{MaxAttempts: 30, Window: 15 * time.Minute}
	webDAVLoginPolicy = ratelimit.Policy{MaxAttempts: 12, Window: 15 * time.Minute}
)

const webDAVAuthCacheTTL = 5 * time.Minute

type davAuthCacheEntry struct {
	UserID       string
	PasswordHash string
	ExpiresAt    time.Time
}

func (a *App) webdavAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.WebDAVRequireHTTPS && !a.requestIsHTTPS(r) && !isLoopbackIP(a.clientIP(r)) {
			http.Error(w, "WebDAV requiere HTTPS.", http.StatusUpgradeRequired)
			return
		}
		usernameRaw, password, ok := r.BasicAuth()
		if !ok {
			a.webDAVUnauthorized(w)
			return
		}
		username, err := auth.NormalizeUsername(usernameRaw)
		if err != nil {
			if allowed, wait := a.limiter.Allow("webdav-ip:"+a.clientIP(r), webDAVIPPolicy); !allowed {
				w.Header().Set("Retry-After", retryHeader(wait))
				http.Error(w, "Demasiados intentos de autenticación WebDAV.", http.StatusTooManyRequests)
				return
			}
			a.webDAVFailure(w, r, strings.ToLower(strings.TrimSpace(usernameRaw)))
			return
		}

		user, lookupErr := a.store.UserByUsername(r.Context(), username)
		if lookupErr != nil && !errors.Is(lookupErr, store.ErrNotFound) {
			http.Error(w, "No se pudo validar la cuenta.", http.StatusInternalServerError)
			return
		}
		if lookupErr == nil && a.webDAVAuthCached(user, password, time.Now()) {
			a.limiter.Reset("webdav-ip:" + a.clientIP(r))
			a.limiter.Reset("webdav:" + a.clientIP(r) + ":" + username)
			next.ServeHTTP(w, r)
			return
		}

		ipKey := "webdav-ip:" + a.clientIP(r)
		key := "webdav:" + a.clientIP(r) + ":" + username
		if allowed, wait := a.limiter.Allow(ipKey, webDAVIPPolicy); !allowed {
			w.Header().Set("Retry-After", retryHeader(wait))
			http.Error(w, "Demasiados intentos de autenticación WebDAV.", http.StatusTooManyRequests)
			return
		}
		if allowed, wait := a.limiter.Allow(key, webDAVLoginPolicy); !allowed {
			w.Header().Set("Retry-After", retryHeader(wait))
			http.Error(w, "Demasiados intentos de autenticación WebDAV.", http.StatusTooManyRequests)
			return
		}

		passwordHash := a.dummyPasswordHash
		if lookupErr == nil {
			passwordHash = user.PasswordHash
		}
		valid, verifyErr := auth.VerifyPassword(passwordHash, password)
		if verifyErr != nil {
			a.logger.Warn("no se pudo verificar credencial WebDAV", "error", verifyErr)
		}
		if lookupErr != nil || !valid {
			_ = a.store.Audit(r.Context(), "", "webdav_login", "fallido", a.clientIP(r))
			a.webDAVUnauthorized(w)
			return
		}

		a.webDAVRememberAuth(user, password, time.Now())
		a.limiter.Reset(ipKey)
		a.limiter.Reset(key)
		next.ServeHTTP(w, r)
	})
}

func (a *App) webDAVAuthCached(user store.User, password string, now time.Time) bool {
	key := a.webDAVCredentialKey(user.Username, password)
	a.davAuthMu.Lock()
	defer a.davAuthMu.Unlock()
	entry, ok := a.davAuthCache[key]
	if !ok {
		return false
	}
	if now.After(entry.ExpiresAt) || entry.UserID != user.ID || entry.PasswordHash != user.PasswordHash {
		delete(a.davAuthCache, key)
		return false
	}
	return true
}

func (a *App) webDAVRememberAuth(user store.User, password string, now time.Time) {
	key := a.webDAVCredentialKey(user.Username, password)
	a.davAuthMu.Lock()
	defer a.davAuthMu.Unlock()
	if len(a.davAuthCache) >= 1024 {
		for candidate, entry := range a.davAuthCache {
			if now.After(entry.ExpiresAt) {
				delete(a.davAuthCache, candidate)
			}
		}
	}
	a.davAuthCache[key] = davAuthCacheEntry{
		UserID:       user.ID,
		PasswordHash: user.PasswordHash,
		ExpiresAt:    now.Add(webDAVAuthCacheTTL),
	}
}

func (a *App) webDAVCredentialKey(username, password string) string {
	mac := hmac.New(sha256.New, a.davAuthSecret[:])
	_, _ = mac.Write([]byte(strings.ToLower(strings.TrimSpace(username))))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(password))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *App) webDAVFailure(w http.ResponseWriter, r *http.Request, username string) {
	key := "webdav:" + a.clientIP(r) + ":" + username
	_, _ = a.limiter.Allow(key, webDAVLoginPolicy)
	_ = a.store.Audit(r.Context(), "", "webdav_login", "fallido", a.clientIP(r))
	a.webDAVUnauthorized(w)
}

func (a *App) webDAVUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Personal Cloud WebDAV", charset="UTF-8"`)
	http.Error(w, "Autenticación requerida.", http.StatusUnauthorized)
}

func retryHeader(wait time.Duration) string {
	seconds := int(wait.Round(time.Second).Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}

func (a *App) requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote := net.ParseIP(strings.TrimSpace(host))
	if remote == nil || !a.isTrustedProxy(remote) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]), "https")
}

func isLoopbackIP(value string) bool {
	ip := net.ParseIP(value)
	return ip != nil && ip.IsLoopback()
}
