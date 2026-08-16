package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"personalcloud/internal/catalog"
	"personalcloud/internal/config"
	"personalcloud/internal/storage"
	"personalcloud/internal/store"
	"personalcloud/internal/webui"
)

func TestBootstrapFlow(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	application, err := New(config.Config{Addr: ":0", DataDir: t.TempDir(), SessionTTL: time.Hour}, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	handler := application.Handler()

	home := httptest.NewRecorder()
	handler.ServeHTTP(home, httptest.NewRequest(http.MethodGet, "/", nil))
	if home.Code != http.StatusSeeOther || home.Header().Get("Location") != "/setup" {
		t.Fatalf("inicio debía redirigir a setup: status=%d location=%q", home.Code, home.Header().Get("Location"))
	}

	setupPage := httptest.NewRecorder()
	handler.ServeHTTP(setupPage, httptest.NewRequest(http.MethodGet, "/setup", nil))
	if setupPage.Code != http.StatusOK {
		t.Fatalf("setup GET: %d", setupPage.Code)
	}
	csrf := findCookie(setupPage.Result().Cookies(), csrfCookieName)
	if csrf == nil {
		t.Fatal("setup no emitió cookie CSRF")
	}

	post := func(code string) *httptest.ResponseRecorder {
		form := url.Values{
			"csrf_token":            {csrf.Value},
			"setup_code":            {code},
			"username":              {"admin"},
			"password":              {"contraseña-segura-12345"},
			"password_confirmation": {"contraseña-segura-12345"},
		}
		req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(csrf)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	wrong := post("AAAA-BBBB-CCCC")
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("código incorrecto debía fallar: %d", wrong.Code)
	}

	correct := post(application.setupCode)
	if correct.Code != http.StatusSeeOther || correct.Header().Get("Location") != "/bienvenida" {
		t.Fatalf("setup correcto: status=%d location=%q", correct.Code, correct.Header().Get("Location"))
	}
	session := findCookie(correct.Result().Cookies(), sessionCookieName)
	if session == nil {
		t.Fatal("setup correcto no creó sesión")
	}

	filesReq := httptest.NewRequest(http.MethodGet, "/archivos", nil)
	filesReq.AddCookie(session)
	filesPage := httptest.NewRecorder()
	handler.ServeHTTP(filesPage, filesReq)
	if filesPage.Code != http.StatusOK || !strings.Contains(filesPage.Body.String(), "Archivos") {
		t.Fatalf("explorador autenticado no cargó: status=%d", filesPage.Code)
	}

	after := httptest.NewRecorder()
	handler.ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/setup", nil))
	if after.Code != http.StatusSeeOther || after.Header().Get("Location") != "/iniciar-sesion" {
		t.Fatalf("setup debía quedar cerrado: status=%d location=%q", after.Code, after.Header().Get("Location"))
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func TestClientIPUsesRightmostUntrustedHop(t *testing.T) {
	_, loopback, err := net.ParseCIDR("127.0.0.1/32")
	if err != nil {
		t.Fatal(err)
	}
	application := &App{cfg: config.Config{TrustedProxyNets: []*net.IPNet{loopback}}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:4567"
	req.Header.Set("X-Forwarded-For", "198.51.100.99, 203.0.113.7")

	if got := application.clientIP(req); got != "203.0.113.7" {
		t.Fatalf("IP inesperada: %q", got)
	}
}

func TestChooseAutoStoragePrefersSpecializedCategory(t *testing.T) {
	views := []storage.View{
		{ID: "mixed", Name: "Mixto", Registered: true, Online: true, Category: "mixed", Free: 900 << 30},
		{ID: "photos", Name: "Fotos", Registered: true, Online: true, Category: "photos", Free: 100 << 30},
		{ID: "docs", Name: "Documentos", Registered: true, Online: true, Category: "documents", Free: 500 << 30},
	}
	selected, ok := chooseAutoStorage(views, "IMG_001.jpg")
	if !ok || selected.ID != "photos" {
		t.Fatalf("debía preferir unidad de fotos; got=%+v ok=%v", selected, ok)
	}
	selected, ok = chooseAutoStorage(views, "factura.pdf")
	if !ok || selected.ID != "docs" {
		t.Fatalf("debía preferir documentos; got=%+v ok=%v", selected, ok)
	}
}

func TestChooseAutoStorageUsesFreeSpaceBetweenEquivalentVolumes(t *testing.T) {
	views := []storage.View{
		{ID: "a", Name: "A", Registered: true, Online: true, Category: "photos", Free: 20 << 30},
		{ID: "b", Name: "B", Registered: true, Online: true, Category: "photos", Free: 50 << 30},
	}
	selected, ok := chooseAutoStorage(views, "foto.png")
	if !ok || selected.ID != "b" {
		t.Fatalf("debía elegir mayor espacio libre; got=%+v ok=%v", selected, ok)
	}
}

func TestNormalizeExplorerPathRejectsTraversal(t *testing.T) {
	if _, err := normalizeExplorerPath("Fotos/../privado"); err == nil {
		t.Fatal("debía rechazar traversal")
	}
	got, err := normalizeExplorerPath("Fotos/2026/Agosto")
	if err != nil || got != "/Fotos/2026/Agosto" {
		t.Fatalf("ruta válida inesperada: %q err=%v", got, err)
	}
}

func TestBrowseCatalogDerivesFoldersWithoutMounting(t *testing.T) {
	files := []catalog.File{
		{ID: "1", RelativePath: "2026/Agosto/a.jpg", Name: "a.jpg", Kind: "image", Size: 10},
		{ID: "2", RelativePath: "2026/Julio/b.jpg", Name: "b.jpg", Kind: "image", Size: 20},
	}
	view := storage.View{Name: "Fotos USB", VirtualRoot: "Fotos", Online: false}
	root := browseCatalog(files, "", view)
	if len(root) != 1 || !root[0].IsDir || root[0].Name != "2026" || !root[0].Offline {
		t.Fatalf("directorio derivado inesperado: %+v", root)
	}
	agosto := browseCatalog(files, "2026/Agosto", view)
	if len(agosto) != 1 || agosto[0].Name != "a.jpg" || agosto[0].DownloadURL == "" {
		t.Fatalf("archivo catalogado inesperado: %+v", agosto)
	}
}

func TestVirtualRootItemsShowsImmediateCatalogUpsert(t *testing.T) {
	catalogStore, err := catalog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer catalogStore.Close()
	file := catalog.File{
		ID: catalog.StableID("volume-1", "foto-recien-subida.jpg"), StorageID: "volume-1", VirtualRoot: "Clase",
		RelativePath: "foto-recien-subida.jpg", Name: "foto-recien-subida.jpg", Kind: "image", Size: 42, ModTime: time.Now().UTC(),
	}
	if err := catalogStore.UpsertBatch(context.Background(), []catalog.File{file}); err != nil {
		t.Fatal(err)
	}
	a := &App{catalog: catalogStore}
	items := a.virtualRootItems([]storage.View{{ID: "volume-1", Name: "USB Clase", VirtualRoot: "Clase", Registered: true, Online: true}})
	if len(items) != 1 || items[0].ID != file.ID || items[0].Name != file.Name {
		t.Fatalf("Mi unidad debe reflejar inmediatamente la entrada recién incorporada al catálogo: %+v", items)
	}
}

func TestStorageTemplateExposesIndexingActions(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	registered := pageData{
		Title: "Almacenamiento", User: &user, CSRFToken: "csrf", MaxUploadBytes: 1024,
		StorageItems: []storagePageItem{{View: storage.View{ID: "v1", Registered: true, Online: true, Name: "Fotos", Category: "photos", VirtualRoot: "Fotos", IdleTimeoutSeconds: 300, Capacity: 1000, Free: 500}}},
	}
	var out bytes.Buffer
	if err := renderer.Render(&out, "storage", registered); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Indexar ahora") {
		t.Fatal("una unidad registrada debe mostrar Indexar ahora")
	}

	detected := pageData{
		Title: "Almacenamiento", User: &user, CSRFToken: "csrf", MaxUploadBytes: 1024,
		StorageItems: []storagePageItem{{View: storage.View{PersistentID: "volume:test", Online: true, Name: "USB"}, SuggestedRoot: "USB"}},
	}
	out.Reset()
	if err := renderer.Render(&out, "storage", detected); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Registrar e indexar") {
		t.Fatal("una unidad detectada debe ofrecer Registrar e indexar")
	}
}

func TestStorageTemplateShowsUnifiedCapacity(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	data := pageData{
		Title: "Almacenamiento", User: &user, CSRFToken: "csrf",
		StorageSummary:    storageSummary{Total: 4000, Used: 2750, Free: 1250, PercentUsed: 69, OnlineUnits: 2},
		StorageUsageItems: []storageUsageItem{{ID: "v1", Name: "HDD Aula", VirtualRoot: "datos", FSType: "ntfs", Capacity: 4000, Used: 2750, Free: 1250, PercentUsed: 69, Online: true, Mounted: true}},
	}
	var out bytes.Buffer
	if err := renderer.Render(&out, "storage", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, expected := range []string{"Almacenamiento", "Unidades conectadas", "HDD Aula", "usados", "libres", "69%"} {
		if !strings.Contains(html, expected) {
			t.Fatalf("el resumen unificado debe renderizar %q", expected)
		}
	}
}

func TestStorageTemplateKeepsUploadOutOfStorageCards(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	data := pageData{
		Title: "Almacenamiento", User: &user, CSRFToken: "csrf",
		StorageItems: []storagePageItem{{
			View: storage.View{ID: "v1", PersistentID: "volume:test", HardwareID: "fsserial:1234", Registered: true, Online: true, Name: "USB", Category: "mixed", VirtualRoot: "E", IdleTimeoutSeconds: 300, Capacity: 64 << 30, Free: 32 << 30},
			Job:  catalog.JobStatus{StorageID: "v1", State: "scanning", Scanned: 50, Total: 200}, JobPercent: 25,
		}},
	}
	var out bytes.Buffer
	if err := renderer.Render(&out, "storage", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if strings.Contains(html, "Subir archivo a esta unidad") {
		t.Fatal("almacenamiento no debe contener el formulario grande de subida")
	}
	if !strings.Contains(html, "25%") || !strings.Contains(html, "50 de 200") {
		t.Fatal("la unidad debe mostrar progreso real de indexación")
	}
	if !strings.Contains(html, "fsserial:1234") || !strings.Contains(html, "volume:test") {
		t.Fatal("deben mostrarse identificadores persistentes del volumen")
	}
}

func TestGalleryTemplateIsOfflineAndHasMediaViewer(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	data := pageData{
		Title: "Galería", CurrentPath: "/galeria", User: &user, CSRFToken: "csrf",
		ListingMode: "infinito", ListingBaseURL: "/galeria", MediaHasMore: true, MediaNext: 80,
		Media: []mediaPageItem{{File: catalog.File{ID: "m1", Name: "video.mp4", Kind: "video"}, ThumbnailURL: "/galeria/m1/miniatura", OriginalURL: "/archivo/m1/original"}},
	}
	var out bytes.Buffer
	if err := renderer.Render(&out, "photos", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{"Galería", "data-media-viewer", "A/D", "W/S", "data-gallery-sentinel", "data-viewer-fullscreen", "data-open-gallery-filter", "data-download-file-id"} {
		if !strings.Contains(html, want) {
			t.Fatalf("galería no contiene %q", want)
		}
	}
	if strings.Contains(html, "https://") || strings.Contains(html, "http://") {
		t.Fatal("la galería no debe depender de assets remotos/CDN")
	}
}

func TestFilesTemplateUsesContextualUploadDialog(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	data := pageData{
		Title: "Archivos", CurrentPath: "/archivos", User: &user, CSRFToken: "csrf",
		ExplorerPath: "/E", ExplorerCanWrite: true, ListingMode: "infinito", ListingBaseURL: "/archivos/ver/E",
	}
	var out bytes.Buffer
	if err := renderer.Render(&out, "files", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if !strings.Contains(html, "Subir aquí") || !strings.Contains(html, "data-upload-dialog") {
		t.Fatal("una carpeta concreta debe ofrecer subida mediante botón + diálogo")
	}
	if strings.Contains(html, "upload-panel") {
		t.Fatal("no debe existir panel grande de subida")
	}
}

func TestListingModeDefaultsToInfiniteAndPersistsChoice(t *testing.T) {
	a := &App{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/galeria", nil)
	if got := a.resolveListingMode(rr, req); got != "infinito" {
		t.Fatalf("modo por defecto=%q", got)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/galeria?modo=paginas", nil)
	if got := a.resolveListingMode(rr, req); got != "paginas" {
		t.Fatalf("modo explícito=%q", got)
	}
	cookie := findCookie(rr.Result().Cookies(), listingCookie)
	if cookie == nil || cookie.Value != "paginas" {
		t.Fatal("la preferencia de listado debe persistirse")
	}
}

func TestGallerySelectionAndURLsPreserveFilters(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/galeria?tipo=video&orden=name-az", nil)
	kind, order := gallerySelection(req)
	if kind != "video" || order != "name-az" {
		t.Fatalf("selección=%s/%s", kind, order)
	}
	url := galleryURL(kind, order, "paginas", 3)
	for _, want := range []string{"tipo=video", "orden=name-az", "modo=paginas", "pagina=3"} {
		if !strings.Contains(url, want) {
			t.Fatalf("URL %q no conserva %q", url, want)
		}
	}
}

func TestFilesTemplateMarksOfflineStorageClearly(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}

	rootData := pageData{
		Title: "Archivos", CurrentPath: "/archivos", User: &user, CSRFToken: "csrf",
		ExplorerPath: "/", ListingMode: "infinito",
		ExplorerRoots: []explorerRoot{{Name: "USB", URL: "/archivos/ver/USB", Category: "mixed", Offline: true}},
	}
	var out bytes.Buffer
	if err := renderer.Render(&out, "files", rootData); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if strings.Contains(html, `file-root-card is-offline`) || strings.Contains(html, `data-unit-actions=`) {
		t.Fatal("Mi unidad ya no debe exponer unidades físicas desconectadas como contenido")
	}
	if !strings.Contains(html, `No hay contenido disponible en Mi unidad`) {
		t.Fatal("una raíz sin unidades disponibles debe explicar que no hay contenido accesible")
	}

	itemData := pageData{
		Title: "Archivos", CurrentPath: "/archivos", User: &user, CSRFToken: "csrf",
		ExplorerPath: "/USB", ListingMode: "infinito",
		ExplorerItems: []explorerItem{{ID: "f1", Name: "foto.jpg", Kind: "image", DownloadURL: "/archivo/f1/original", Offline: true}},
		ExplorerFiles: []explorerItem{{ID: "f1", Name: "foto.jpg", Kind: "image", DownloadURL: "/archivo/f1/original", Offline: true}},
	}
	out.Reset()
	if err := renderer.Render(&out, "files", itemData); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `file-row is-offline`) {
		t.Fatal("los elementos de una unidad desconectada deben conservar el estado visual offline")
	}
}

func TestTemplatesExposeBulkSelectionAndSettings(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	gallery := pageData{Title: "Galería", CurrentPath: "/galeria", User: &user, CSRFToken: "csrf", ListingMode: "infinito", Media: []mediaPageItem{{File: catalog.File{ID: "x", Name: "x.jpg", Kind: "image", Health: "damaged"}}}}
	var out bytes.Buffer
	if err := renderer.Render(&out, "photos", gallery); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"data-open-selection-menu", "Seleccionar todo", "data-bulk-toolbar", "data-move-dialog", "Dañado"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("galería sin %q", want)
		}
	}
	out.Reset()
	settings := pageData{Title: "Configuración", CurrentPath: "/configuracion", User: &user, CSRFToken: "csrf", Settings: store.AppSettings{SyncIntervalMinutes: 30}, SettingsSyncText: "Cada 30 min"}
	if err := renderer.Render(&out, "settings", settings); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Sincronizar todo") || !strings.Contains(out.String(), "Verificar integridad") || !strings.Contains(out.String(), "Cada 30 min") {
		t.Fatal("configuración de sincronización incompleta")
	}
}

func TestSettingsExposeIntegrityAndFolderPicker(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	data := pageData{Title: "Configuración", CurrentPath: "/configuracion", User: &user, CSRFToken: "csrf", IntegrityUnits: []integrityUnitView{{ID: "v1", Name: "USB", VirtualRoot: "USB", Online: true, Damaged: 2, DamagedPending: 2, Unchecked: 1, Healthy: 4, Samples: []catalog.File{{Name: "roto.mp4", Health: "damaged", HealthError: "archivo truncado"}}}}}
	var out bytes.Buffer
	if err := renderer.Render(&out, "settings", data); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Verificar integridad", "2 dañados", "roto.mp4", "Eliminar dañados", "/configuracion/sincronizar/v1"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("configuración sin %q", want)
		}
	}
	out.Reset()
	gallery := pageData{Title: "Galería", CurrentPath: "/galeria", User: &user, CSRFToken: "csrf", ListingMode: "infinito", MoveDestinations: []moveDestination{{Name: "USB", VirtualRoot: "USB", Online: true}}}
	if err := renderer.Render(&out, "photos", gallery); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"data-folder-picker", "data-create-folder", "data-move-root", "data-select-icon=\"video\""} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("UI cloud sin %q", want)
		}
	}
}

func TestDriveRootUnitActionsAreFunctionalControls(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	data := pageData{
		Title: "Archivos", CurrentPath: "/archivos", User: &user, CSRFToken: "csrf",
		ExplorerPath: "/", ListingMode: "infinito",
		ExplorerRoots: []explorerRoot{{ID: "volume-1", Name: "Clase A", URL: "/archivos/ver/Clase-A", Category: "mixed", Status: "Montada", FileCount: 12}},
	}
	var out bytes.Buffer
	if err := renderer.Render(&out, "files", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{`data-unit-info`, `data-unit-index`, `data-unit-mount`, `data-unit-dialog`} {
		if !strings.Contains(html, want) {
			t.Fatalf("el shell debe conservar el control funcional de unidades %q", want)
		}
	}
	if strings.Contains(html, `data-unit-actions="volume-1"`) {
		t.Fatal("Mi unidad no debe volver a representar la unidad física como una carpeta del usuario")
	}
	if strings.Contains(html, `<span class="drive-card-more"`) {
		t.Fatal("Mi unidad no debe volver a renderizar tres puntos decorativos")
	}
}

func TestDriveRootRendersFoldersBeforeFiles(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	folder := explorerItem{Name: "Tareas", Kind: "folder", IsDir: true, URL: "/archivos/ver/Clase-A/Tareas", StorageName: "Clase A"}
	file := explorerItem{ID: "f1", Name: "reporte.pdf", Kind: "document", DownloadURL: "/archivo/f1/original", IconKey: "pdf", IconLabel: "PDF"}
	data := pageData{
		Title: "Mi unidad", CurrentPath: "/archivos", User: &user, CSRFToken: "csrf",
		ExplorerPath: "/", ListingMode: "infinito", ExplorerItems: []explorerItem{folder, file},
		ExplorerFolders: []explorerItem{folder}, ExplorerFiles: []explorerItem{file},
	}
	var out bytes.Buffer
	if err := renderer.Render(&out, "files", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	folderPos := strings.Index(html, `class="drive-content-label">Carpetas`)
	filePos := strings.Index(html, `class="drive-content-label">Archivos`)
	if folderPos < 0 || filePos < 0 || folderPos >= filePos {
		t.Fatalf("Mi unidad debe renderizar carpetas antes de archivos: folder=%d file=%d", folderPos, filePos)
	}
	for _, want := range []string{`data-file-filter-form`, `/static/icons/pdf.svg`, `data-bulk-star`} {
		if !strings.Contains(html, want) {
			t.Fatalf("Mi unidad no contiene %q", want)
		}
	}
}

func TestDashboardMatchesDriveSuggestedLayout(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	data := pageData{
		Title: "Inicio", CurrentPath: "/inicio", User: &user, CSRFToken: "csrf",
		HomeFolders: []explorerRoot{{ID: "v1", Name: "Documentos", URL: "/archivos/ver/Documentos"}},
		HomeFiles:   []homeFileItem{{ID: "f1", Name: "tarea.pdf", Kind: "document", VirtualRoot: "Documentos", OpenURL: "/archivo/f1/original"}},
	}
	var out bytes.Buffer
	if err := renderer.Render(&out, "dashboard", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{"Te damos la bienvenida a Nube", "Carpetas sugeridas", "Archivos sugeridos", "Motivo sugerido", "Propietario", "Ubicación", `data-home-view="grid"`, `data-home-view="list"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("página principal Drive incompleta: falta %q", want)
		}
	}
}

func TestCatalogCacheURLForcesCurrentImageVersion(t *testing.T) {
	file := catalog.File{ID: "img-1", Kind: "image", CacheVersion: catalog.ImageCacheVersion - 1}
	got := catalogCacheURL(file, "miniatura")
	want := fmt.Sprintf("/galeria/img-1/miniatura?v=%d", catalog.ImageCacheVersion)
	if got != want {
		t.Fatalf("cache url=%q, quiero %q", got, want)
	}
}

func TestFileContextMenuIncludesInformationAndOfflineSafeActions(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	var out bytes.Buffer
	if err := renderer.Render(&out, "dashboard", pageData{Title: "Inicio", CurrentPath: "/inicio", User: &user, CSRFToken: "csrf"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-context-open`, `data-context-info`, `data-file-info-dialog`, `data-requires-online`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("menú de archivo incompleto: falta %q", want)
		}
	}
}

func TestDriveShellExposesRealRecentStarredAndNewActions(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	data := pageData{
		Title: "Archivos", CurrentPath: "/archivos", User: &user, CSRFToken: "csrf",
		ExplorerPath: "/", ExplorerCanWrite: true, ListingMode: "infinito",
		MoveDestinations: []moveDestination{{Name: "Clase A", VirtualRoot: "Clase-A", Online: true}},
	}
	var out bytes.Buffer
	if err := renderer.Render(&out, "files", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{
		`href="/recientes"`, `href="/destacados"`, `data-global-new`, `data-new-menu`,
		`data-new-folder-dialog`, `data-new-folder-form`, `data-drop-overlay`,
		`data-context-star`, `data-context-rename`, `data-rename-dialog`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("shell Drive incompleto: falta %q", want)
		}
	}
}

func TestDriveFileCollectionRendersAsRealList(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	data := pageData{
		Title: "Destacados", CurrentPath: "/starred", User: &user, CSRFToken: "csrf",
		ExplorerPath: "/", ListingMode: "infinito", FileCollection: "starred",
		FileCollectionTitle: "Destacados", FileCollectionSubtitle: "Tus archivos marcados como destacados",
		ExplorerItems: []explorerItem{{ID: "f1", Name: "tarea.pdf", Kind: "document", DownloadURL: "/archivo/f1/original", Starred: true}},
	}
	var out bytes.Buffer
	if err := renderer.Render(&out, "files", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{"Destacados", "tarea.pdf", `data-starred="true"`, `data-file-view="grid"`, `data-file-view="list"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("colección Drive incompleta: falta %q", want)
		}
	}
}

func TestExplorerFiltersAreFunctional(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	items := []explorerItem{
		{Name: "Foto.jpg", Kind: "image", VirtualRoot: "Clase-A", ModTime: now.Add(-2 * time.Hour)},
		{Name: "Tarea.pdf", Kind: "document", VirtualRoot: "Clase-A", ModTime: now.Add(-10 * 24 * time.Hour)},
		{Name: "Video.mp4", Kind: "video", VirtualRoot: "Clase-B", ModTime: now.Add(-3 * 24 * time.Hour)},
		{Name: "Carpeta", Kind: "folder", VirtualRoot: "Clase-A", IsDir: true},
	}
	got := applyExplorerFilter(items, explorerFilter{Kind: "image", Modified: "7d", Source: "clase-a"}, now)
	if len(got) != 1 || got[0].Name != "Foto.jpg" {
		t.Fatalf("filtro combinado inesperado: %#v", got)
	}
	got = applyExplorerFilter(items, explorerFilter{Modified: "7d"}, now)
	if len(got) != 2 {
		t.Fatalf("filtro temporal debe devolver 2 archivos, obtuvo %d", len(got))
	}
}

func TestDriveFileFiltersRenderAsRealSelects(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	data := pageData{
		Title: "Destacados", CurrentPath: "/starred", User: &user, CSRFToken: "csrf",
		ExplorerPath: "/", ListingMode: "infinito", FileCollection: "starred",
		FileCollectionTitle: "Destacados", FileFilterAction: "/destacados", FileTypeFilter: "image", FileFilterCount: 1,
		MoveDestinations: []moveDestination{{Name: "Clase A", VirtualRoot: "Clase-A", Online: true}},
		ExplorerItems:    []explorerItem{{ID: "f1", Name: "foto.jpg", Kind: "image", DownloadURL: "/archivo/f1/original"}},
	}
	var out bytes.Buffer
	if err := renderer.Render(&out, "files", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{`data-file-filter-form`, `name="tipo"`, `name="modificado"`, `name="fuente"`, `data-clear-file-filters`, `value="image" selected`} {
		if !strings.Contains(html, want) {
			t.Fatalf("filtros Drive incompletos: falta %q", want)
		}
	}
}

func TestFileListingURLPreservesDriveFilters(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/archivos/ver/Clase-A?tipo=image&modificado=7d&fuente=Clase-A&pagina=4", nil)
	got := fileListingURL(req, "paginas", 2)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	for key, want := range map[string]string{"tipo": "image", "modificado": "7d", "fuente": "Clase-A", "modo": "paginas", "pagina": "2"} {
		if query.Get(key) != want {
			t.Fatalf("%s=%q, quiero %q en %q", key, query.Get(key), want, got)
		}
	}
}

func TestUploadDialogDefaultsToMultiplePickerAndKeepsDestinationUnderAdvanced(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	data := pageData{
		Title: "Mi unidad", CurrentPath: "/archivos", User: &user, CSRFToken: "csrf", MaxUploadBytes: 1024,
		MaxUploadBatchFiles: maxUploadBatchFiles,
		ExplorerPath:        "/", ExplorerCanWrite: true, ListingMode: "infinito",
		MoveDestinations: []moveDestination{{Name: "USB Clase", VirtualRoot: "USB", Online: true}},
	}
	var out bytes.Buffer
	if err := renderer.Render(&out, "files", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{`data-upload-picker`, `type="file" name="file" multiple`, `data-upload-advanced-toggle`, `data-upload-advanced-panel hidden`, `name="destination_root"`, `data-upload-folder-picker`, `data-upload-target-dir`, `Automático · usar la mejor unidad`, `USB Clase`} {
		if !strings.Contains(html, want) {
			t.Fatalf("selector de destino de subida incompleto: falta %q", want)
		}
	}
	if strings.Contains(html, `data-upload-location-panel`) || strings.Contains(html, `data-upload-choose-location`) {
		t.Fatal("la ubicación no debe ocupar la vista principal del diálogo; debe vivir bajo Avanzados")
	}
	rootPos := strings.Index(html, `name="destination_root"`)
	filePos := strings.Index(html, `name="file"`)
	if rootPos < 0 || filePos < 0 || rootPos >= filePos {
		t.Fatalf("destination_root debe viajar antes del stream del archivo: root=%d file=%d", rootPos, filePos)
	}
}

func TestUploadBatchRequestLimitCoversMultipleFiles(t *testing.T) {
	const perFile = int64(10 << 20)
	got := uploadBatchRequestLimit(perFile)
	wantMinimum := perFile * maxUploadBatchFiles
	if got < wantMinimum {
		t.Fatalf("límite por lote=%d, debe cubrir al menos %d", got, wantMinimum)
	}
}

func TestSharingUIAndPublicVideoEmbedAreRendered(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	user := store.User{Username: "admin", Role: "admin"}
	var out bytes.Buffer
	if err := renderer.Render(&out, "dashboard", pageData{Title: "Inicio", CurrentPath: "/inicio", User: &user, CSRFToken: "csrf"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`href="/compartidos"`, `data-context-share`, `data-share-dialog`, `data-viewer-share`, `data-document-viewer-share`, `/static/share_management.js`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("UI autenticada de compartir incompleta: falta %q", want)
		}
	}

	out.Reset()
	public := pageData{
		Title: "clase.mp4", PublicSharePage: true, ShareEmbed: true,
		PublicShare: &publicShareView{
			Name: "clase.mp4", Viewer: "video", Size: 1234, Available: true,
			ContentURL: "/s/token/contenido?access=ticket", DownloadURL: "/s/token/contenido?access=ticket&download=1",
			VideoQualitiesURL: "/s/token/video/calidades?access=ticket", VideoPrepareURL: "/s/token/video/preparar?access=ticket", VideoStatusURL: "/s/token/video/estado?access=ticket",
		},
	}
	if err := renderer.Render(&out, "public_share", public); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{`data-public-video-player`, `data-public-video-quality`, `/s/token/video/calidades?access=ticket`, `/static/share.js`} {
		if !strings.Contains(html, want) {
			t.Fatalf("embed público de video incompleto: falta %q", want)
		}
	}
	if strings.Contains(html, `/static/app.js`) || strings.Contains(html, `/static/document_viewer.js`) {
		t.Fatal("la página pública no debe cargar scripts privados del shell autenticado")
	}
}

func TestProtectedPublicShareFormDoesNotDependOnThirdPartyCSRFCookie(t *testing.T) {
	renderer, err := webui.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	data := pageData{
		Title: "privado.pdf", PublicSharePage: true, ShareEmbed: true, SharePasswordRequired: true,
		PublicShare: &publicShareView{Name: "privado.pdf", PasswordProtected: true},
	}
	if err := renderer.Render(&out, "public_share", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if !strings.Contains(html, `name="password"`) {
		t.Fatal("el embed protegido debe mostrar el formulario de contraseña")
	}
	if strings.Contains(html, `name="csrf_token"`) {
		t.Fatal("el desbloqueo público no debe depender de una cookie CSRF de terceros")
	}
}

func TestShareAccessTicketIsSignedAndInvalidatedByPasswordChange(t *testing.T) {
	a := &App{}
	copy(a.shareSecret[:], []byte("0123456789abcdef0123456789abcdef"))
	share := store.PublicShare{ID: "share-1", Token: "token-a", PasswordHash: "hash-a"}
	ticket := a.newShareAccessTicket(share, time.Hour)
	if !a.validShareAccessTicket(share, ticket) {
		t.Fatal("el ticket recién emitido debe ser válido")
	}
	share.PasswordHash = "hash-b"
	if a.validShareAccessTicket(share, ticket) {
		t.Fatal("cambiar la contraseña debe invalidar tickets anteriores")
	}
	share.PasswordHash = "hash-a"
	share.Token = "token-b"
	if a.validShareAccessTicket(share, ticket) {
		t.Fatal("renovar el enlace debe invalidar tickets anteriores")
	}
}
