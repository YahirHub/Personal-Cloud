package catalog

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const catalogVersion = 1

type File struct {
	ID           string    `json:"id"`
	StorageID    string    `json:"storage_id"`
	VirtualRoot  string    `json:"virtual_root"`
	RelativePath string    `json:"relative_path"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	MIME         string    `json:"mime"`
	Size         int64     `json:"size"`
	ModTime      time.Time `json:"mod_time"`
	Width        int       `json:"width,omitempty"`
	Height       int       `json:"height,omitempty"`
	Thumbnail    bool      `json:"thumbnail,omitempty"`
	Preview      bool      `json:"preview,omitempty"`
	IndexedAt    time.Time `json:"indexed_at"`
}

type snapshot struct {
	Version int    `json:"version"`
	Seq     uint64 `json:"seq"`
	Files   []File `json:"files"`
}

type event struct {
	Seq  uint64 `json:"seq"`
	Op   string `json:"op"`
	File *File  `json:"file,omitempty"`
	ID   string `json:"id,omitempty"`
}

// deuda-tecnica: índice en memoria, migrar a almacenamiento disk-backed si el catálogo supera ~500k archivos o el arranque/memoria dejan de ser aceptables.
type Catalog struct {
	mu           sync.RWMutex
	dir          string
	snapshotPath string
	logPath      string
	cacheDir     string
	files        map[string]File
	seq          uint64
	events       int
}

type Stats struct {
	Files  int
	Photos int
	Bytes  int64
}

func Open(dataDir string) (*Catalog, error) {
	dir := filepath.Join(dataDir, "catalog")
	cacheDir := filepath.Join(dataDir, "cache")
	for _, path := range []string{dir, filepath.Join(cacheDir, "thumbnails"), filepath.Join(cacheDir, "previews")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nil, fmt.Errorf("crear catálogo: %w", err)
		}
	}
	c := &Catalog{
		dir:          dir,
		snapshotPath: filepath.Join(dir, "snapshot.json"),
		logPath:      filepath.Join(dir, "events.jsonl"),
		cacheDir:     cacheDir,
		files:        make(map[string]File),
	}
	if err := c.load(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Catalog) Close() error { return nil }

func (c *Catalog) ByID(id string) (File, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	file, ok := c.files[id]
	return file, ok
}

func (c *Catalog) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var stats Stats
	stats.Files = len(c.files)
	for _, file := range c.files {
		stats.Bytes += file.Size
		if file.Kind == "image" {
			stats.Photos++
		}
	}
	return stats
}

func (c *Catalog) ListPhotos(offset, limit int) []File {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	c.mu.RLock()
	photos := make([]File, 0)
	for _, file := range c.files {
		if file.Kind == "image" {
			photos = append(photos, file)
		}
	}
	c.mu.RUnlock()
	sort.SliceStable(photos, func(i, j int) bool {
		if photos[i].ModTime.Equal(photos[j].ModTime) {
			return strings.ToLower(photos[i].Name) < strings.ToLower(photos[j].Name)
		}
		return photos[i].ModTime.After(photos[j].ModTime)
	})
	if offset >= len(photos) {
		return nil
	}
	end := offset + limit
	if end > len(photos) {
		end = len(photos)
	}
	return photos[offset:end]
}

type MediaQuery struct {
	Kind       string
	Sort       string
	StorageIDs map[string]struct{}
}

func (c *Catalog) ListMedia(offset, limit int) []File {
	return c.ListMediaQuery(offset, limit, MediaQuery{})
}

func (c *Catalog) ListMediaQuery(offset, limit int, query MediaQuery) []File {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 500 {
		limit = 80
	}
	items := c.mediaMatches(query)
	if offset >= len(items) {
		return nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

func (c *Catalog) MediaCount() int {
	return c.MediaCountQuery(MediaQuery{})
}

func (c *Catalog) MediaCountQuery(query MediaQuery) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	count := 0
	for _, file := range c.files {
		if mediaMatchesQuery(file, query) {
			count++
		}
	}
	return count
}

func (c *Catalog) mediaMatches(query MediaQuery) []File {
	c.mu.RLock()
	items := make([]File, 0)
	for _, file := range c.files {
		if mediaMatchesQuery(file, query) {
			items = append(items, file)
		}
	}
	c.mu.RUnlock()
	sortMedia(items, query.Sort)
	return items
}

func mediaMatchesQuery(file File, query MediaQuery) bool {
	if file.Kind != "image" && file.Kind != "video" && file.Kind != "audio" {
		return false
	}
	if query.Kind != "" && query.Kind != "all" && file.Kind != query.Kind {
		return false
	}
	if query.StorageIDs != nil {
		if _, ok := query.StorageIDs[file.StorageID]; !ok {
			return false
		}
	}
	return true
}

func sortMedia(items []File, mode string) {
	switch mode {
	case "added-oldest":
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].IndexedAt.Equal(items[j].IndexedAt) {
				return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
			}
			return items[i].IndexedAt.Before(items[j].IndexedAt)
		})
	case "file-oldest":
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].ModTime.Equal(items[j].ModTime) {
				return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
			}
			return items[i].ModTime.Before(items[j].ModTime)
		})
	case "name-az":
		sort.SliceStable(items, func(i, j int) bool {
			left, right := strings.ToLower(items[i].Name), strings.ToLower(items[j].Name)
			if left == right {
				return items[i].ModTime.After(items[j].ModTime)
			}
			return left < right
		})
	case "name-za":
		sort.SliceStable(items, func(i, j int) bool {
			left, right := strings.ToLower(items[i].Name), strings.ToLower(items[j].Name)
			if left == right {
				return items[i].ModTime.After(items[j].ModTime)
			}
			return left > right
		})
	case "added-newest":
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].IndexedAt.Equal(items[j].IndexedAt) {
				return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
			}
			return items[i].IndexedAt.After(items[j].IndexedAt)
		})
	default: // file-newest
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].ModTime.Equal(items[j].ModTime) {
				return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
			}
			return items[i].ModTime.After(items[j].ModTime)
		})
	}
}

func (c *Catalog) FilesByStorage(storageID string) []File {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]File, 0)
	for _, file := range c.files {
		if file.StorageID == storageID {
			result = append(result, file)
		}
	}
	return result
}

func (c *Catalog) AllFiles() []File {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]File, 0, len(c.files))
	for _, file := range c.files {
		result = append(result, file)
	}
	sort.SliceStable(result, func(i, j int) bool {
		left := strings.ToLower(result[i].VirtualRoot + "/" + result[i].RelativePath)
		right := strings.ToLower(result[j].VirtualRoot + "/" + result[j].RelativePath)
		return left < right
	})
	return result
}

func (c *Catalog) UpsertBatch(ctx context.Context, files []File) error {
	if len(files) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var data []byte
	for i := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.seq++
		if existing, ok := c.files[files[i].ID]; ok && !existing.IndexedAt.IsZero() {
			files[i].IndexedAt = existing.IndexedAt
		} else if files[i].IndexedAt.IsZero() {
			files[i].IndexedAt = time.Now().UTC()
		}
		e := event{Seq: c.seq, Op: "upsert", File: &files[i]}
		line, err := json.Marshal(e)
		if err != nil {
			return err
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	if err := appendCatalogLog(c.logPath, data); err != nil {
		return err
	}
	for _, file := range files {
		c.files[file.ID] = file
	}
	c.events += len(files)
	return nil
}

func (c *Catalog) DeleteIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var data []byte
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, ok := c.files[id]; !ok {
			continue
		}
		c.seq++
		e := event{Seq: c.seq, Op: "delete", ID: id}
		line, err := json.Marshal(e)
		if err != nil {
			return err
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	if len(data) == 0 {
		return nil
	}
	if err := appendCatalogLog(c.logPath, data); err != nil {
		return err
	}
	for _, id := range ids {
		delete(c.files, id)
	}
	c.events += len(ids)
	return nil
}

func (c *Catalog) Compact(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	files := make([]File, 0, len(c.files))
	for _, file := range c.files {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ID < files[j].ID })
	data, err := json.Marshal(snapshot{Version: catalogVersion, Seq: c.seq, Files: files})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := writeAtomic(c.snapshotPath, data); err != nil {
		return err
	}
	if err := os.WriteFile(c.logPath, nil, 0o600); err != nil {
		return err
	}
	c.events = 0
	return nil
}

func (c *Catalog) ShouldCompact() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.events >= 5000
}

func (c *Catalog) CachePath(fileID, size string) string {
	kind := "thumbnails"
	if size == "preview" {
		kind = "previews"
	}
	prefix := "00"
	if len(fileID) >= 2 {
		prefix = fileID[:2]
	}
	return filepath.Join(c.cacheDir, kind, prefix, fileID+".jpg")
}

func StableID(storageID, relativePath string) string {
	h := sha256.Sum256([]byte(storageID + "\x00" + filepath.ToSlash(relativePath)))
	return hex.EncodeToString(h[:])
}

func (c *Catalog) load() error {
	if data, err := os.ReadFile(c.snapshotPath); err == nil {
		var snap snapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			return fmt.Errorf("snapshot de catálogo inválido: %w", err)
		}
		if snap.Version != catalogVersion {
			return fmt.Errorf("versión de catálogo no soportada: %d", snap.Version)
		}
		c.seq = snap.Seq
		for _, file := range snap.Files {
			c.files[file.ID] = file
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	file, err := os.Open(c.logPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	for scanner.Scan() {
		var e event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return fmt.Errorf("log de catálogo inválido: %w", err)
		}
		if e.Seq <= c.seq {
			continue
		}
		switch e.Op {
		case "upsert":
			if e.File != nil {
				c.files[e.File.ID] = *e.File
			}
		case "delete":
			delete(c.files, e.ID)
		}
		c.seq = e.Seq
		c.events++
	}
	return scanner.Err()
}

func appendCatalogLog(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".catalog-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	backup := path + ".bak"
	if _, err := os.Stat(path); err == nil {
		_ = os.Remove(backup)
		if err := os.Rename(path, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Rename(backup, path)
		return err
	}
	return nil
}

// SnapshotBytes devuelve una representación consistente del catálogo sin tocar
// los originales ni forzar una nueva indexación. Se usa para backups de metadatos.
func (c *Catalog) SnapshotBytes(ctx context.Context) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	files := make([]File, 0, len(c.files))
	for _, file := range c.files {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ID < files[j].ID })
	data, err := json.Marshal(snapshot{Version: catalogVersion, Seq: c.seq, Files: files})
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
