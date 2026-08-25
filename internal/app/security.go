package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"time"

	"personalcloud/internal/auth"
)

const (
	csrfCookieName    = "pc_csrf"
	sessionCookieName = "pc_session"
)

func randomBrowserToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (a *App) ensureCSRF(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(csrfCookieName); err == nil && len(cookie.Value) >= 32 {
		return cookie.Value
	}
	token, err := randomBrowserToken()
	if err != nil {
		a.logger.Error("no se pudo crear token CSRF", "error", err)
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   24 * 60 * 60,
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return token
}

func (a *App) validCSRF(r *http.Request) bool {
	return a.validCSRFValue(r, r.PostFormValue("csrf_token"))
}

func (a *App) validCSRFValue(r *http.Request, token string) bool {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	if len(cookie.Value) != len(token) || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(token)) == 1
}

func setAuthNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Add("Vary", "Cookie")
}

func (a *App) createLoginSession(w http.ResponseWriter, r *http.Request, userID string) error {
	plain, digest, err := auth.NewSessionToken()
	if err != nil {
		return err
	}
	expires := time.Now().UTC().Add(a.cfg.SessionTTL)
	if err := a.store.CreateSession(r.Context(), userID, digest, expires); err != nil {
		return err
	}
	// The login response is a credential-setting response. Do not allow a
	// browser or an intermediary to reuse/cache it, and derive Secure from
	// the effective request scheme as well as the global HTTPS policy.
	secure := a.cfg.CookieSecure || a.cfg.RequireHTTPS || a.requestIsHTTPS(r)
	setAuthNoStore(w)
	w.Header().Set("X-PC-Session-Created", "1")
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    plain,
		Path:     "/",
		MaxAge:   int(a.cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (a *App) clearLoginSession(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if err := a.store.DeleteSession(r.Context(), auth.HashToken(cookie.Value)); err != nil {
			a.logger.Warn("no se pudo eliminar la sesión", "error", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}
