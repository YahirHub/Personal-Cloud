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
		Title: "Configuración", Description: "Sincronización, integridad y mantenimiento del catálogo.", CurrentPath: "/configuracion", User: user,
		Settings: settings, SettingsSyncText: syncIntervalText(settings.SyncIntervalMinutes), IntegrityUnits: a.integrityUnitViews(r.Context()),
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

func (a *App) settingsSyncUnitPost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || !a.storageOnline(r, id) {
		redirectSettingsError(w, r, "La unidad no está conectada")
		return
	}
	a.indexer.Enqueue(id)
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "sync_unit_manual", "unidad:"+id, a.clientIP(r))
	http.Redirect(w, r, "/configuracion?ok="+urlQuery("Sincronización de unidad solicitada"), http.StatusSeeOther)
}

func (a *App) settingsVerifyNowPost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	ids, err := a.storageManager.OnlineRegisteredIDs(r.Context())
	if err != nil {
		redirectSettingsError(w, r, "No se pudieron consultar las unidades conectadas")
		return
	}
	count := 0
	for id := range ids {
		if a.indexer.EnqueueVerify(id) {
			count++
		}
	}
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "integrity_verify_manual", fmt.Sprintf("unidades:%d", count), a.clientIP(r))
	http.Redirect(w, r, "/configuracion?ok="+urlQuery(fmt.Sprintf("Verificación de integridad solicitada para %d unidad(es)", count)), http.StatusSeeOther)
}

func (a *App) settingsVerifyUnitPost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || !a.storageOnline(r, id) {
		redirectSettingsError(w, r, "La unidad no está conectada")
		return
	}
	a.indexer.EnqueueVerify(id)
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "integrity_verify_unit", "unidad:"+id, a.clientIP(r))
	http.Redirect(w, r, "/configuracion?ok="+urlQuery("Verificación de integridad solicitada"), http.StatusSeeOther)
}

func (a *App) integrityUnitViews(ctx context.Context) []integrityUnitView {
	views, _ := a.storageManager.Views(ctx)
	out := make([]integrityUnitView, 0, len(views))
	for _, view := range views {
		if !view.Registered {
			continue
		}
		counts := a.catalog.HealthCountsByStorage(view.ID)
		samples := a.catalog.DamagedByStorage(view.ID, false)
		if len(samples) > 5 {
			samples = samples[:5]
		}
		job := a.indexer.Status(view.ID)
		out = append(out, integrityUnitView{
			ID: view.ID, Name: view.Name, VirtualRoot: view.VirtualRoot, Online: view.Online,
			Damaged: counts.Damaged, DamagedPending: counts.DamagedPending, Unchecked: counts.Unchecked, Healthy: counts.OK,
			Samples: samples, Job: job, JobPercent: job.Percent(),
		})
	}
	return out
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
