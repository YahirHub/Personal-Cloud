package catalog

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io/fs"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"personalcloud/internal/storage"
)

const maxThumbnailSourcePixels int64 = 80_000_000

type JobStatus struct {
	StorageID string
	State     string
	Scanned   int
	Images    int
	Updated   int
	Error     string
	StartedAt time.Time
	EndedAt   time.Time
}

type Indexer struct {
	catalog *Catalog
	manager *storage.Manager
	logger  *slog.Logger
	queue   chan string
	stop    chan struct{}
	once    sync.Once
	mu      sync.RWMutex
	jobs    map[string]JobStatus
	pending map[string]bool
}

func NewIndexer(catalog *Catalog, manager *storage.Manager, logger *slog.Logger) *Indexer {
	i := &Indexer{
		catalog: catalog,
		manager: manager,
		logger:  logger,
		queue:   make(chan string, 16),
		stop:    make(chan struct{}),
		jobs:    make(map[string]JobStatus),
		pending: make(map[string]bool),
	}
	go i.worker()
	return i
}

func (i *Indexer) Close() { i.once.Do(func() { close(i.stop) }) }

func (i *Indexer) Enqueue(storageID string) bool {
	i.mu.Lock()
	if job, ok := i.jobs[storageID]; ok {
		if job.State == "scanning" {
			i.pending[storageID] = true
			i.mu.Unlock()
			return true
		}
		if job.State == "queued" {
			i.mu.Unlock()
			return false
		}
	}
	i.jobs[storageID] = JobStatus{StorageID: storageID, State: "queued"}
	i.mu.Unlock()
	select {
	case i.queue <- storageID:
		return true
	default:
		i.mu.Lock()
		job := i.jobs[storageID]
		job.State = "error"
		job.Error = "cola de indexación llena"
		i.jobs[storageID] = job
		i.mu.Unlock()
		return false
	}
}

func (i *Indexer) Status(storageID string) JobStatus {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.jobs[storageID]
}

func (i *Indexer) worker() {
	for {
		select {
		case <-i.stop:
			return
		case storageID := <-i.queue:
			for {
				i.scan(storageID)
				i.mu.Lock()
				again := i.pending[storageID] && i.jobs[storageID].State == "done"
				delete(i.pending, storageID)
				if again {
					job := i.jobs[storageID]
					job.State = "queued"
					i.jobs[storageID] = job
				}
				i.mu.Unlock()
				if !again {
					break
				}
			}
		}
	}
}

func (i *Indexer) scan(storageID string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	i.setJob(storageID, func(job *JobStatus) {
		job.State = "scanning"
		job.StartedAt = time.Now().UTC()
		job.Error = ""
	})
	lease, err := i.manager.Acquire(ctx, storageID, false)
	if err != nil {
		i.failJob(storageID, err)
		return
	}
	defer lease.Release()

	previous := i.catalog.FilesByStorage(storageID)
	previousByID := make(map[string]File, len(previous))
	for _, old := range previous {
		previousByID[old.ID] = old
	}
	seen := make(map[string]struct{}, len(previous))
	batch := make([]File, 0, 200)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := i.catalog.UpsertBatch(ctx, batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	err = filepath.WalkDir(lease.Root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrPermission) {
				return nil
			}
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != lease.Root && (name == "$RECYCLE.BIN" || name == "System Volume Information" || name == ".Trash-1000") {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(lease.Root, path)
		if err != nil {
			return nil
		}
		id := StableID(storageID, rel)
		seen[id] = struct{}{}
		kind, mimeType := classify(path)
		old, existed := previousByID[id]
		if existed && old.Kind == "image" && kind != "image" {
			_ = os.Remove(i.catalog.CachePath(old.ID, "thumbnail"))
			_ = os.Remove(i.catalog.CachePath(old.ID, "preview"))
		}
		file := File{
			ID:           id,
			StorageID:    storageID,
			VirtualRoot:  lease.Volume.VirtualRoot,
			RelativePath: filepath.ToSlash(rel),
			Name:         info.Name(),
			Kind:         kind,
			MIME:         mimeType,
			Size:         info.Size(),
			ModTime:      info.ModTime().UTC(),
		}
		if kind == "image" {
			i.setJob(storageID, func(job *JobStatus) { job.Images++ })
			unchanged := existed && old.Size == info.Size() && old.ModTime.Equal(info.ModTime().UTC())
			width, height, thumb, preview := i.ensureImageCache(path, id, unchanged)
			file.Width, file.Height = width, height
			file.Thumbnail, file.Preview = thumb, preview
		}
		batch = append(batch, file)
		i.setJob(storageID, func(job *JobStatus) { job.Scanned++; job.Updated++ })
		if len(batch) >= 200 {
			return flush()
		}
		return nil
	})
	if err == nil {
		err = flush()
	}
	if err != nil {
		i.failJob(storageID, err)
		return
	}
	var deleted []string
	for _, old := range previous {
		if _, ok := seen[old.ID]; !ok {
			deleted = append(deleted, old.ID)
			_ = os.Remove(i.catalog.CachePath(old.ID, "thumbnail"))
			_ = os.Remove(i.catalog.CachePath(old.ID, "preview"))
		}
	}
	if err := i.catalog.DeleteIDs(ctx, deleted); err != nil {
		i.failJob(storageID, err)
		return
	}
	if i.catalog.ShouldCompact() {
		if err := i.catalog.Compact(ctx); err != nil {
			i.logger.Warn("no se pudo compactar catálogo", "error", err)
		}
	}
	i.setJob(storageID, func(job *JobStatus) {
		job.State = "done"
		job.EndedAt = time.Now().UTC()
	})
}

func (i *Indexer) ensureImageCache(source, id string, unchanged bool) (int, int, bool, bool) {
	thumbPath := i.catalog.CachePath(id, "thumbnail")
	previewPath := i.catalog.CachePath(id, "preview")
	if unchanged {
		if info, err := os.Stat(thumbPath); err == nil && info.Size() > 0 {
			if pinfo, err := os.Stat(previewPath); err == nil && pinfo.Size() > 0 {
				if width, height := imageDimensions(source); width > 0 {
					return width, height, true, true
				}
			}
		}
	} else {
		_ = os.Remove(thumbPath)
		_ = os.Remove(previewPath)
	}
	file, err := os.Open(source)
	if err != nil {
		return 0, 0, false, false
	}
	config, _, err := image.DecodeConfig(file)
	_ = file.Close()
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return 0, 0, false, false
	}
	if int64(config.Width) > maxThumbnailSourcePixels/int64(config.Height) {
		return config.Width, config.Height, false, false
	}
	file, err = os.Open(source)
	if err != nil {
		return config.Width, config.Height, false, false
	}
	img, _, err := image.Decode(file)
	_ = file.Close()
	if err != nil {
		return config.Width, config.Height, false, false
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	thumbOK := writeScaledJPEG(img, thumbPath, 320, 82) == nil
	previewOK := writeScaledJPEG(img, previewPath, 1600, 88) == nil
	return width, height, thumbOK, previewOK
}

func imageDimensions(source string) (int, int) {
	file, err := os.Open(source)
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0
	}
	return config.Width, config.Height
}

func writeScaledJPEG(src image.Image, destination string, maxDimension, quality int) error {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return errors.New("imagen sin dimensiones")
	}
	targetW, targetH := fitDimensions(width, height, maxDimension)
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	for y := 0; y < targetH; y++ {
		sy := bounds.Min.Y + y*height/targetH
		for x := 0; x < targetW; x++ {
			sx := bounds.Min.X + x*width/targetW
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".thumb-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := jpeg.Encode(tmp, dst, &jpeg.Options{Quality: quality}); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_ = os.Remove(destination)
	return os.Rename(name, destination)
}

func fitDimensions(width, height, maxDimension int) (int, int) {
	if width <= maxDimension && height <= maxDimension {
		return width, height
	}
	if width >= height {
		targetW := maxDimension
		targetH := height * maxDimension / width
		if targetH < 1 {
			targetH = 1
		}
		return targetW, targetH
	}
	targetH := maxDimension
	targetW := width * maxDimension / height
	if targetW < 1 {
		targetW = 1
	}
	return targetW, targetH
}

func classify(path string) (string, string) {
	ext := strings.ToLower(filepath.Ext(path))
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".heic", ".heif", ".avif", ".dng", ".cr2", ".nef", ".arw":
		return "image", mimeType
	case ".mp4", ".mkv", ".mov", ".avi", ".webm", ".m4v":
		return "video", mimeType
	case ".mp3", ".flac", ".wav", ".m4a", ".ogg", ".opus", ".aac":
		return "audio", mimeType
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".md", ".odt", ".ods":
		return "document", mimeType
	case ".zip", ".7z", ".rar", ".tar", ".gz", ".bz2", ".xz":
		return "archive", mimeType
	default:
		return "other", mimeType
	}
}

func (i *Indexer) setJob(storageID string, update func(*JobStatus)) {
	i.mu.Lock()
	defer i.mu.Unlock()
	job := i.jobs[storageID]
	job.StorageID = storageID
	update(&job)
	i.jobs[storageID] = job
}

func (i *Indexer) failJob(storageID string, err error) {
	i.logger.Warn("indexación fallida", "storage_id", storageID, "error", err)
	i.setJob(storageID, func(job *JobStatus) {
		job.State = "error"
		job.Error = fmt.Sprintf("%v", err)
		job.EndedAt = time.Now().UTC()
	})
}
