package app

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
)

func (a *App) storageIgnoreDamagedPost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	storageID := r.PathValue("id")
	count, err := a.catalog.IgnoreDamaged(r.Context(), storageID)
	if err != nil {
		redirectStorageError(w, r, err)
		return
	}
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "damaged_ignore", fmt.Sprintf("correcto:%d", count), a.clientIP(r))
	redirectDamagedResult(w, r, fmt.Sprintf("Se omitió el aviso de %d elemento(s) dañado(s)", count))
}

func (a *App) storageDeleteDamagedPost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	storageID := r.PathValue("id")
	items := a.catalog.DamagedByStorage(storageID, false)
	deleted := 0
	ids := make([]string, 0, len(items))
	for _, file := range items {
		err := a.vfs.Remove(r.Context(), path.Join("/", file.VirtualRoot, file.RelativePath))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			redirectStorageError(w, r, fmt.Errorf("eliminar %s: %w", file.Name, err))
			return
		}
		a.catalog.RemoveCache(file)
		ids = append(ids, file.ID)
		deleted++
	}
	if err := a.catalog.DeleteIDs(r.Context(), ids); err != nil {
		redirectStorageError(w, r, err)
		return
	}
	user := userFromContext(r.Context())
	_ = a.store.Audit(r.Context(), user.ID, "damaged_delete", fmt.Sprintf("correcto:%d", deleted), a.clientIP(r))
	redirectDamagedResult(w, r, fmt.Sprintf("Se eliminaron %d elemento(s) dañado(s)", deleted))
}

func redirectDamagedResult(w http.ResponseWriter, r *http.Request, message string) {
	base := "/almacenamiento"
	if r.FormValue("return_to") == "configuracion" {
		base = "/configuracion"
	}
	http.Redirect(w, r, base+"?ok="+urlQuery(message), http.StatusSeeOther)
}
