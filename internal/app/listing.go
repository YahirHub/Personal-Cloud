package app

import (
	"net/http"
	"strings"
	"time"
)

const listingCookie = "pc_listing_mode"

func (a *App) resolveListingMode(w http.ResponseWriter, r *http.Request) string {
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("modo")))
	if mode == "infinito" || mode == "paginas" {
		http.SetCookie(w, &http.Cookie{Name: listingCookie, Value: mode, Path: "/", MaxAge: int((365 * 24 * time.Hour).Seconds()), SameSite: http.SameSiteLaxMode, HttpOnly: false, Secure: a.cfg.CookieSecure || a.cfg.RequireHTTPS})
		return mode
	}
	if cookie, err := r.Cookie(listingCookie); err == nil && (cookie.Value == "infinito" || cookie.Value == "paginas") {
		return cookie.Value
	}
	return "infinito"
}

func pageSlice[T any](items []T, page, size int) ([]T, bool) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	start := (page - 1) * size
	if start >= len(items) {
		return nil, false
	}
	end := start + size
	more := end < len(items)
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], more
}

func offsetSlice[T any](items []T, offset, size int) ([]T, bool) {
	if offset < 0 {
		offset = 0
	}
	if size < 1 {
		size = 100
	}
	if offset >= len(items) {
		return nil, false
	}
	end := offset + size
	more := end < len(items)
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], more
}
