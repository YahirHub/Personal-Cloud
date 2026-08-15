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
