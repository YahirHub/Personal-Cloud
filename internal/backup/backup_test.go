package backup

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateMetadata(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	if err := os.WriteFile(state, []byte("{\"version\":2}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := CreateMetadata(dir, state, []byte("{\"version\":1,\"files\":[]}\n"), time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	seen := map[string]bool{}
	for _, file := range reader.File {
		seen[file.Name] = true
	}
	for _, wanted := range []string{"state.json", "catalog/snapshot.json", "manifest.json"} {
		if !seen[wanted] {
			t.Fatalf("falta %s en backup", wanted)
		}
	}
}
