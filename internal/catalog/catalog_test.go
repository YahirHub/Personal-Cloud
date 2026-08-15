package catalog

import (
	"context"
	"testing"
	"time"
)

func TestCatalogPersistsEvents(t *testing.T) {
	dir := t.TempDir()
	catalog, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	file := File{ID: StableID("s1", "Fotos/a.jpg"), StorageID: "s1", VirtualRoot: "Fotos", RelativePath: "Fotos/a.jpg", Name: "a.jpg", Kind: "image", Size: 10, ModTime: time.Now().UTC()}
	if err := catalog.UpsertBatch(context.Background(), []File{file}); err != nil {
		t.Fatal(err)
	}
	loaded, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := loaded.ByID(file.ID); !ok || got.Name != "a.jpg" {
		t.Fatalf("archivo no persistió: %#v %v", got, ok)
	}
}

func TestFitDimensions(t *testing.T) {
	w, h := fitDimensions(4000, 3000, 320)
	if w != 320 || h != 240 {
		t.Fatalf("dimensiones inesperadas %dx%d", w, h)
	}
}

func TestIndexerMarksPendingWhenStorageChangesDuringScan(t *testing.T) {
	i := &Indexer{
		queue:   make(chan string, 1),
		jobs:    map[string]JobStatus{"volume-1": {StorageID: "volume-1", State: "scanning"}},
		pending: make(map[string]bool),
	}
	if !i.Enqueue("volume-1") {
		t.Fatal("un cambio durante scan debe quedar pendiente")
	}
	if !i.pending["volume-1"] {
		t.Fatal("no se marcó segunda pasada")
	}
}

func TestJobStatusPercentUsesRealProgress(t *testing.T) {
	cases := []struct {
		job  JobStatus
		want int
	}{
		{JobStatus{State: "counting"}, 0},
		{JobStatus{State: "scanning", Scanned: 25, Total: 100}, 25},
		{JobStatus{State: "scanning", Scanned: 150, Total: 100}, 100},
		{JobStatus{State: "done", Scanned: 100, Total: 100}, 100},
	}
	for _, tc := range cases {
		if got := tc.job.Percent(); got != tc.want {
			t.Fatalf("Percent(%+v)=%d want=%d", tc.job, got, tc.want)
		}
	}
}
