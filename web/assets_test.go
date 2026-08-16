package web

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedUIHasNoRemoteAssets(t *testing.T) {
	patterns := []string{
		"http://",
		"https://",
		`src="//`,
		`src='//`,
		`href="//`,
		`href='//`,
		"url(//",
	}
	if err := fs.WalkDir(Assets, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(Assets, path)
		if err != nil {
			return err
		}
		content := strings.ToLower(string(data))
		// xmlns declara el namespace XML estándar de SVG; no provoca ninguna
		// petición de red y es necesario para servir los iconos como imágenes.
		if strings.HasSuffix(strings.ToLower(path), ".svg") {
			content = strings.ReplaceAll(content, `xmlns="http://www.w3.org/2000/svg"`, "")
			content = strings.ReplaceAll(content, `xmlns='http://www.w3.org/2000/svg'`, "")
		}
		for _, pattern := range patterns {
			if strings.Contains(content, pattern) {
				t.Errorf("asset embebido %s contiene referencia remota %q", path, pattern)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestViewerMetadataStaysAwayFromNativeMediaControls(t *testing.T) {
	data, err := fs.ReadFile(Assets, "static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(data)
	if !strings.Contains(css, ".viewer-meta { position:absolute; z-index:3; top:0;") {
		t.Fatal("los metadatos del visor deben permanecer anclados arriba del stage")
	}
	if strings.Contains(css, ".viewer-shell[data-active-kind=\"video\"] .viewer-footer") {
		t.Fatal("no debe quedar el antiguo footer sobre los controles nativos de video")
	}
}

func TestViewerKeyboardShortcutsUseCapturePhase(t *testing.T) {
	data, err := fs.ReadFile(Assets, "static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	if !strings.Contains(js, "window.addEventListener('keydown', handleViewerKeydown, true)") {
		t.Fatal("los atajos del visor deben registrarse en fase de captura para sobrevivir al foco de controles nativos de video")
	}
	for _, shortcut := range []string{"arrowleft", "arrowright", "'a'", "'d'", "'w'", "'s'"} {
		if !strings.Contains(js, shortcut) {
			t.Fatalf("falta conservar el atajo %s del visor", shortcut)
		}
	}
}

func TestViewerPredecodesImagesBeforeSwap(t *testing.T) {
	data, err := fs.ReadFile(Assets, "static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, expected := range []string{
		"const decodeViewerImage = async",
		"typeof image.decode === 'function'",
		"Number(card.dataset.cacheVersion || 0) >= 2",
		"stage.replaceChildren(image)",
	} {
		if !strings.Contains(js, expected) {
			t.Fatalf("el visor debe precargar y decodificar imágenes antes del swap; falta %q", expected)
		}
	}
}

func TestGalleryIncludesBottomVideoControls(t *testing.T) {
	data, err := fs.ReadFile(Assets, "pages/photos.html")
	if err != nil {
		t.Fatal(err)
	}
	template := string(data)
	for _, expected := range []string{"data-video-controls", "data-video-progress", "data-video-play", "data-video-volume", "data-video-quality-control", "data-video-quality", "data-viewer-fullscreen"} {
		if !strings.Contains(template, expected) {
			t.Fatalf("el visor de video debe integrar controles inferiores; falta %q", expected)
		}
	}
	if strings.Contains(template, "viewer-fullscreen icon-button") || strings.Contains(template, "viewer-quality\"") {
		t.Fatal("calidad y fullscreen no deben volver a controles flotantes separados")
	}
}

func TestBulkSelectionAndTouchActionsAreEmbedded(t *testing.T) {
	data, err := fs.ReadFile(Assets, "static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, expected := range []string{
		"data-open-selection-menu",
		"data-selection-all",
		"data-bulk-download",
		"data-bulk-move",
		"data-bulk-delete",
		"event.pointerType !== 'touch'",
		"window.setTimeout(() =>",
		"showDownloadMenu(pressStartX, pressStartY",
	} {
		if !strings.Contains(js, expected) {
			t.Fatalf("faltó comportamiento offline/reutilizable %q", expected)
		}
	}
}

func TestCustomVideoPlayerUsesLocalControls(t *testing.T) {
	data, err := fs.ReadFile(Assets, "static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, expected := range []string{
		"video.controls = false",
		"const refreshVideoControls",
		"viewerShell.requestFullscreen",
		"data-selection-all",
		"const selectEverythingAvailable",
	} {
		if !strings.Contains(js, expected) {
			t.Fatalf("faltó comportamiento del reproductor/selección %q", expected)
		}
	}
}

func TestSelectsUsePersistentThemeColors(t *testing.T) {
	data, err := fs.ReadFile(Assets, "static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(data)
	for _, expected := range []string{
		"select option, select optgroup",
		"select { color-scheme: dark;",
		".viewer-video-speed select option,.viewer-quality-inline select option",
	} {
		if !strings.Contains(css, expected) {
			t.Fatalf("los selects deben conservar el tema también al desplegar opciones; falta %q", expected)
		}
	}
}

func TestVideoPlayerUsesAdaptiveQualityAndSmoothTimeline(t *testing.T) {
	data, err := fs.ReadFile(Assets, "static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, expected := range []string{
		"requestAnimationFrame(videoProgressLoop)",
		"Range: 'bytes=0-524287'",
		"const chooseAutoQuality",
		"new Option('Auto', 'auto')",
		"Auto · midiendo ancho de banda",
		"const stageVideoSourceSwap",
		"viewer-video-staging",
		"preparando en segundo plano",
		"adaptiveBandwidthFactor",
	} {
		if !strings.Contains(js, expected) {
			t.Fatalf("reproducción adaptativa/timeline incompleta; falta %q", expected)
		}
	}
}

func TestQualitySwitchDoesNotUseBlockingLoader(t *testing.T) {
	data, err := fs.ReadFile(Assets, "static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	start := strings.Index(js, "const prepareVideoQuality = async")
	end := strings.Index(js[start:], "const reevaluateAutoQuality")
	if start < 0 || end < 0 {
		t.Fatal("no se encontró prepareVideoQuality")
	}
	block := js[start : start+end]
	if strings.Contains(block, "showViewerLoader(") || strings.Contains(block, "video.pause()") {
		t.Fatal("cambiar calidad debe prepararse en segundo plano sin pausar ni mostrar loader")
	}
	for _, expected := range []string{"stageVideoSourceSwap", "pendingVideoQuality", "qualitySwitchSequence"} {
		if !strings.Contains(block, expected) {
			t.Fatalf("cambio de calidad no bloqueante incompleto; falta %q", expected)
		}
	}
}

func TestViewerIncludesLocalLoadingOverlay(t *testing.T) {
	data, err := fs.ReadFile(Assets, "pages/photos.html")
	if err != nil {
		t.Fatal(err)
	}
	template := string(data)
	for _, expected := range []string{"data-viewer-loader", "viewer-loading-spinner", "data-viewer-loading-text", `step="0.01"`} {
		if !strings.Contains(template, expected) {
			t.Fatalf("el visor debe conservar loader local para la carga inicial; falta %q", expected)
		}
	}
}

func TestDriveDarkShellIsEmbedded(t *testing.T) {
	cssData, err := fs.ReadFile(Assets, "static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssData)
	for _, expected := range []string{
		"--drive-header-height: 72px",
		".drive-topbar {",
		".drive-search {",
		".drive-new-button {",
		".drive-home-files.is-grid",
		".file-list.drive-file-list.is-grid",
		"color-scheme: dark;",
	} {
		if !strings.Contains(css, expected) {
			t.Fatalf("la interfaz tipo Drive oscura está incompleta; falta %q", expected)
		}
	}

	layout, err := fs.ReadFile(Assets, "layouts/base.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(layout), `template "topbar"`) {
		t.Fatal("el layout autenticado debe incluir la barra superior tipo Drive")
	}

	topbar, err := fs.ReadFile(Assets, "components/topbar.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`action="/archivos"`, `name="q"`, `placeholder="Buscar en Nube"`} {
		if !strings.Contains(string(topbar), expected) {
			t.Fatalf("la búsqueda global debe permanecer visible y funcional; falta %q", expected)
		}
	}
}

func TestDriveViewsAndGlobalNewAreFunctional(t *testing.T) {
	jsData, err := fs.ReadFile(Assets, "static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsData)
	for _, expected := range []string{
		"pc-drive-file-view",
		"pc-drive-home-view",
		"data-file-actions",
		"get('nuevo') === '1'",
	} {
		if !strings.Contains(js, expected) {
			t.Fatalf("la experiencia tipo Drive debe conservar controles reales; falta %q", expected)
		}
	}
}

func TestVendoredFileTypeIconsAreEmbedded(t *testing.T) {
	for _, name := range []string{"android.svg", "pdf.svg", "markdown.svg", "word.svg", "excel.svg", "powerpoint.svg"} {
		data, err := fs.ReadFile(Assets, "static/icons/"+name)
		if err != nil {
			t.Fatalf("icono local %s no embebido: %v", name, err)
		}
		if len(data) < 100 {
			t.Fatalf("icono local %s parece vacío", name)
		}
	}
}

func TestDocumentViewerIsEmbeddedAndOffline(t *testing.T) {
	component, err := fs.ReadFile(Assets, "components/document_viewer.html")
	if err != nil {
		t.Fatal(err)
	}
	template := string(component)
	for _, expected := range []string{
		"data-document-viewer",
		"data-document-viewer-download",
		"data-document-viewer-edit",
		"data-document-viewer-save",
		"data-document-viewer-star",
		"sandbox=\"\"",
	} {
		if !strings.Contains(template, expected) {
			t.Fatalf("visor de documentos incompleto; falta %q", expected)
		}
	}

	jsData, err := fs.ReadFile(Assets, "static/document_viewer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsData)
	for _, expected := range []string{
		"renderMarkdown",
		"/api/archivo/${encodeURIComponent(state.id)}/contenido",
		"/archivo/${encodeURIComponent(state.id)}/pdf",
		"/archivo/${encodeURIComponent(state.id)}/html",
		"Ctrl+S",
		"window.PersonalCloudDocumentViewer",
	} {
		if !strings.Contains(js, expected) {
			t.Fatalf("comportamiento del visor incompleto; falta %q", expected)
		}
	}
}
