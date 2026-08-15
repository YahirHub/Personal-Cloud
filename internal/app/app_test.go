package app

import (
	"bytes"
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
	if !strings.Contains(html, `file-root-card is-offline`) || !strings.Contains(html, `No disponible · catálogo local`) {
		t.Fatal("una unidad desconectada debe mostrarse gris y marcada como no disponible")
	}

	itemData := pageData{
		Title: "Archivos", CurrentPath: "/archivos", User: &user, CSRFToken: "csrf",
		ExplorerPath: "/USB", ListingMode: "infinito",
		ExplorerItems: []explorerItem{{ID: "f1", Name: "foto.jpg", Kind: "image", DownloadURL: "/archivo/f1/original", Offline: true}},
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
	for _, want := range []string{"data-toggle-selection", "data-bulk-toolbar", "data-move-dialog", "Dañado"} {
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
