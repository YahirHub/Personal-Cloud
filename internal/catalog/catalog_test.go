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

func TestMediaQueryFiltersStorageKindAndSort(t *testing.T) {
	c, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	files := []File{
		{ID: "a", StorageID: "online", Name: "b.jpg", Kind: "image", ModTime: now.Add(-time.Hour), IndexedAt: now.Add(-2 * time.Minute)},
		{ID: "b", StorageID: "online", Name: "a.mp4", Kind: "video", ModTime: now, IndexedAt: now.Add(-time.Minute)},
		{ID: "c", StorageID: "offline", Name: "c.jpg", Kind: "image", ModTime: now.Add(time.Hour), IndexedAt: now},
	}
	if err := c.UpsertBatch(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]struct{}{"online": {}}
	got := c.ListMediaQuery(0, 10, MediaQuery{StorageIDs: allowed, Sort: "name-az"})
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" {
		t.Fatalf("filtro/orden inesperado: %+v", got)
	}
	images := c.ListMediaQuery(0, 10, MediaQuery{StorageIDs: allowed, Kind: "image", Sort: "file-newest"})
	if len(images) != 1 || images[0].ID != "a" {
		t.Fatalf("filtro de imagen inesperado: %+v", images)
	}
	if count := c.MediaCountQuery(MediaQuery{StorageIDs: allowed}); count != 2 {
		t.Fatalf("MediaCountQuery=%d want=2", count)
	}
}

func TestUpsertPreservesFirstIndexedAt(t *testing.T) {
	c, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	file := File{ID: "same", StorageID: "s1", Name: "a.jpg", Kind: "image", ModTime: first, IndexedAt: first}
	if err := c.UpsertBatch(context.Background(), []File{file}); err != nil {
		t.Fatal(err)
	}
	file.Size = 42
	file.IndexedAt = time.Time{}
	if err := c.UpsertBatch(context.Background(), []File{file}); err != nil {
		t.Fatal(err)
	}
	got, ok := c.ByID("same")
	if !ok {
		t.Fatal("archivo no encontrado")
	}
	if !got.IndexedAt.Equal(first) {
		t.Fatalf("IndexedAt=%v want=%v", got.IndexedAt, first)
	}
}
