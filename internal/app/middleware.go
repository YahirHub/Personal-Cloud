package app

import (
	"context"
	"net/http"
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; object-src 'none'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (a *App) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		a.logger.Info("http", "method", r.Method, "path", r.URL.Path, "ip", a.clientIP(r), "elapsed_ms", time.Since(start).Milliseconds())
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
