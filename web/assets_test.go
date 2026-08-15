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
