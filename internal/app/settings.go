package app

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) settingsGet(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	settings, err := a.store.Settings(r.Context())
	if err != nil {
		http.Error(w, "No se pudo leer la configuración.", http.StatusInternalServerError)
		return
	}
	data := a.csrfData(w, r, pageData{
		Title: "Configuración", Description: "Sincronización y mantenimiento del catálogo.", CurrentPath: "/configuracion", User: user,
		Settings: settings, SettingsSyncText: syncIntervalText(settings.SyncIntervalMinutes),
	})
	if value := r.URL.Query().Get("ok"); value != "" {
		data.Info = value
	}
	if value := r.URL.Query().Get("error"); value != "" {
		data.Error = value
	}
	a.render(w, http.StatusOK, "settings", data)
}

func (a *App) settingsSyncPost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	minutes, err := strconv.Atoi(strings.TrimSpace(r.FormValue("sync_interval_minutes")))
	if err != nil {
		redirectSettingsError(w, r, "Intervalo inválido")
		return
	}
	current, _ := a.store.Settings(r.Context())
	current.SyncIntervalMinutes = minutes
	if err := a.store.UpdateSettings(r.Context(), current); err != nil {
		redirectSettingsError(w, r, err.Error())
		return
	}
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "settings_sync_interval", fmt.Sprintf("minutos:%d", minutes), a.clientIP(r))
	http.Redirect(w, r, "/configuracion?ok="+urlQuery("Configuración de sincronización guardada"), http.StatusSeeOther)
}

func (a *App) settingsSyncNowPost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	count := a.enqueueOnlineSync(r.Context())
	_ = a.store.MarkSync(r.Context(), time.Now().UTC())
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "sync_manual", fmt.Sprintf("unidades:%d", count), a.clientIP(r))
	message := fmt.Sprintf("Sincronización solicitada para %d unidad(es) conectada(s)", count)
	http.Redirect(w, r, "/configuracion?ok="+urlQuery(message), http.StatusSeeOther)
}

func (a *App) periodicSyncIfNeeded() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	settings, err := a.store.Settings(ctx)
	if err != nil || settings.SyncIntervalMinutes <= 0 {
		return
	}
	interval := time.Duration(settings.SyncIntervalMinutes) * time.Minute
	if !settings.LastSyncAt.IsZero() && time.Since(settings.LastSyncAt) < interval {
		return
	}
	count := a.enqueueOnlineSync(ctx)
	if count == 0 {
		return
	}
	if err := a.store.MarkSync(ctx, time.Now().UTC()); err != nil {
		a.logger.Warn("no se pudo registrar sincronización periódica", "error", err)
	}
	a.logger.Info("sincronización periódica solicitada", "units", count, "interval_minutes", settings.SyncIntervalMinutes)
}

func (a *App) enqueueOnlineSync(ctx context.Context) int {
	ids, err := a.storageManager.OnlineRegisteredIDs(ctx)
	if err != nil {
		return 0
	}
	count := 0
	for id := range ids {
		if a.indexer.Enqueue(id) {
			count++
		}
	}
	return count
}

func syncIntervalText(minutes int) string {
	if minutes <= 0 {
		return "Desactivada"
	}
	if minutes < 60 {
		return fmt.Sprintf("Cada %d min", minutes)
	}
	if minutes%1440 == 0 {
		return fmt.Sprintf("Cada %d día(s)", minutes/1440)
	}
	if minutes%60 == 0 {
		return fmt.Sprintf("Cada %d h", minutes/60)
	}
	return fmt.Sprintf("Cada %d min", minutes)
}

func redirectSettingsError(w http.ResponseWriter, r *http.Request, message string) {
	http.Redirect(w, r, "/configuracion?error="+urlQuery(message), http.StatusSeeOther)
}
