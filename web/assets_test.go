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

func TestGalleryIncludesVideoQualitySelector(t *testing.T) {
	data, err := fs.ReadFile(Assets, "pages/photos.html")
	if err != nil {
		t.Fatal(err)
	}
	template := string(data)
	for _, expected := range []string{"data-video-quality-control", "data-video-quality", "data-video-quality-status"} {
		if !strings.Contains(template, expected) {
			t.Fatalf("el visor de video debe exponer selector de calidad; falta %q", expected)
		}
	}
}
