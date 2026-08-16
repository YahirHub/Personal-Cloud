package app

import (
	"context"
	"net/http"
	"strings"
	"time"

	"personalcloud/internal/auth"
	"personalcloud/internal/store"
)

func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; media-src 'self'; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if a.cfg.RequireHTTPS && a.requestIsHTTPS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logPath := r.URL.Path
		if strings.HasPrefix(logPath, "/descarga/") {
			logPath = "/descarga/{token}"
		} else if strings.HasPrefix(logPath, "/descarga-lote/") {
			logPath = "/descarga-lote/{token}"
		} else if strings.HasPrefix(logPath, "/s/") {
			parts := strings.Split(strings.TrimPrefix(logPath, "/s/"), "/")
			logPath = "/s/{token}"
			if len(parts) > 1 && parts[1] != "" {
				logPath += "/" + parts[1]
			}
		}
		a.logger.Info("http", "method", r.Method, "path", logPath, "ip", a.clientIP(r), "elapsed_ms", time.Since(start).Milliseconds())
	})
}

func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := a.currentUser(r)
		if err != nil || user == nil {
			http.Redirect(w, r, "/iniciar-sesion", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *App) currentUser(r *http.Request) (*store.User, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, nil
	}
	user, err := a.store.UserBySessionTokenHash(r.Context(), auth.HashToken(cookie.Value))
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (a *App) enforceHTTPS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.cfg.RequireHTTPS || a.requestIsHTTPS(r) {
			next.ServeHTTP(w, r)
			return
		}
		host := r.Host
		if host == "" {
			http.Error(w, "HTTPS requerido.", http.StatusUpgradeRequired)
			return
		}
		target := "https://" + host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
}
