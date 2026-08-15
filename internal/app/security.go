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
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	formToken := r.PostFormValue("csrf_token")
	if len(cookie.Value) != len(formToken) || formToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(formToken)) == 1
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
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    plain,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(a.cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
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
