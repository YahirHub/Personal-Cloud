package catalog

import (
	"context"
	"encoding/json"
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
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"personalcloud/internal/storage"
)

const maxThumbnailSourcePixels int64 = 80_000_000
const ImageCacheVersion = 3

type JobStatus struct {
	StorageID string    `json:"storage_id"`
	State     string    `json:"state"`
	Scanned   int       `json:"scanned"`
	Total     int       `json:"total"`
	Images    int       `json:"images"`
	Videos    int       `json:"videos"`
	Audio     int       `json:"audio"`
	Damaged   int       `json:"damaged"`
	Unchecked int       `json:"unchecked"`
	Added     int       `json:"added"`
	Changed   int       `json:"changed"`
	Removed   int       `json:"removed"`
	Updated   int       `json:"updated"`
	VerifyAll bool      `json:"verify_all,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

func (j JobStatus) Percent() int {
	if j.Total <= 0 {
		return 0
	}
	p := j.Scanned * 100 / j.Total
	if p > 100 {
		return 100
	}
	if p < 0 {
		return 0
	}
	return p
}

type Indexer struct {
	catalog    *Catalog
	manager    *storage.Manager
	logger     *slog.Logger
	queue      chan string
	stop       chan struct{}
	once       sync.Once
	mu         sync.RWMutex
	jobs       map[string]JobStatus
	pending    map[string]bool
	verifyNext map[string]bool
	ffmpeg     string
	ffprobe    string
	cacheMu    sync.Mutex
}

func NewIndexer(catalog *Catalog, manager *storage.Manager, logger *slog.Logger) *Indexer {
	ffmpeg, _ := exec.LookPath("ffmpeg")
	ffprobe, _ := exec.LookPath("ffprobe")
	i := &Indexer{catalog: catalog, manager: manager, logger: logger, queue: make(chan string, 16), stop: make(chan struct{}), jobs: make(map[string]JobStatus), pending: make(map[string]bool), verifyNext: make(map[string]bool), ffmpeg: ffmpeg, ffprobe: ffprobe}
	if ffmpeg != "" {
		logger.Info("FFmpeg detectado; se generarán miniaturas de video cuando sea posible", "path", ffmpeg)
	}
	go i.worker()
	return i
}

func (i *Indexer) Close()                { i.once.Do(func() { close(i.stop) }) }
func (i *Indexer) FFmpegAvailable() bool { return i.ffmpeg != "" }

func (i *Indexer) Enqueue(storageID string) bool { return i.enqueue(storageID, false) }

// EnqueueVerify solicita una reconciliación completa que vuelve a validar la
// integridad multimedia aunque tamaño y mtime no hayan cambiado. Se usa solo
// desde acciones explícitas de mantenimiento para no castigar los discos.
func (i *Indexer) EnqueueVerify(storageID string) bool { return i.enqueue(storageID, true) }

func (i *Indexer) enqueue(storageID string, verifyAll bool) bool {
	i.mu.Lock()
	if verifyAll {
		i.verifyNext[storageID] = true
	}
	if job, ok := i.jobs[storageID]; ok {
		if job.State == "scanning" || job.State == "counting" {
			i.pending[storageID] = true
			i.mu.Unlock()
			return true
		}
		if job.State == "queued" {
			i.mu.Unlock()
			return false
		}
	}
	i.jobs[storageID] = JobStatus{StorageID: storageID, State: "queued", VerifyAll: verifyAll}
	i.mu.Unlock()
	select {
	case i.queue <- storageID:
		return true
	default:
		i.setJob(storageID, func(job *JobStatus) { job.State = "error"; job.Error = "cola de indexación llena" })
		return false
	}
}

func (i *Indexer) Status(storageID string) JobStatus {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.jobs[storageID]
}
func (i *Indexer) Statuses() []JobStatus {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]JobStatus, 0, len(i.jobs))
	for _, job := range i.jobs {
		out = append(out, job)
	}
	return out
}

func (i *Indexer) worker() {
	for {
		select {
		case <-i.stop:
			return
		case storageID := <-i.queue:
			for {
				i.mu.Lock()
				verifyAll := i.verifyNext[storageID]
				delete(i.verifyNext, storageID)
				i.mu.Unlock()
				i.scan(storageID, verifyAll)
				i.mu.Lock()
				again := i.pending[storageID] && i.jobs[storageID].State == "done"
				delete(i.pending, storageID)
				if again {
					job := i.jobs[storageID]
					job.State = "queued"
					job.VerifyAll = i.verifyNext[storageID]
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

func (i *Indexer) scan(storageID string, verifyAll bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	i.setJob(storageID, func(job *JobStatus) {
		*job = JobStatus{StorageID: storageID, State: "counting", VerifyAll: verifyAll, StartedAt: time.Now().UTC()}
	})
	lease, err := i.manager.Acquire(ctx, storageID, false)
	if err != nil {
		i.failJob(storageID, err)
		return
	}
	defer lease.Release()

	total, err := countIndexableFiles(ctx, lease.Root)
	if err != nil {
		i.failJob(storageID, err)
		return
	}
	i.setJob(storageID, func(job *JobStatus) { job.State = "scanning"; job.Total = total })

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

	err = filepath.WalkDir(lease.Root, func(source string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrPermission) {
				return nil
			}
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if shouldSkipEntry(lease.Root, source, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(lease.Root, source)
		if err != nil {
			return nil
		}
		id := StableID(storageID, rel)
		seen[id] = struct{}{}
		kind, mimeType := classify(source)
		old, existed := previousByID[id]
		if existed && (old.Kind != kind || old.Size != info.Size() || !old.ModTime.Equal(info.ModTime().UTC())) {
			i.removeCache(old)
		}
		file := File{ID: id, StorageID: storageID, VirtualRoot: lease.Volume.VirtualRoot, RelativePath: filepath.ToSlash(rel), Name: info.Name(), Kind: kind, MIME: mimeType, Size: info.Size(), ModTime: info.ModTime().UTC()}
		unchanged := existed && old.Size == info.Size() && old.ModTime.Equal(info.ModTime().UTC()) && old.Kind == kind
		if !existed {
			i.setJob(storageID, func(job *JobStatus) { job.Added++ })
		} else if !unchanged {
			i.setJob(storageID, func(job *JobStatus) { job.Changed++ })
		}
		if unchanged && old.Health != "" && !verifyAll {
			file.Health, file.HealthError, file.HealthCheckedAt, file.DamageIgnored = old.Health, old.HealthError, old.HealthCheckedAt, old.DamageIgnored
		} else if kind == "image" || kind == "video" || kind == "audio" {
			file.Health, file.HealthError = i.verifyMediaQuick(ctx, source, kind, info.Size())
			file.HealthCheckedAt = time.Now().UTC()
			if file.Health != old.Health || file.HealthError != old.HealthError {
				file.DamageIgnored = false
			}
		}
		if file.Health == "damaged" && !file.DamageIgnored {
			i.setJob(storageID, func(job *JobStatus) { job.Damaged++ })
		} else if file.Health == "unchecked" {
			i.setJob(storageID, func(job *JobStatus) { job.Unchecked++ })
		}
		switch kind {
		case "image":
			i.setJob(storageID, func(job *JobStatus) { job.Images++ })
			cacheCurrent := unchanged && old.CacheVersion == ImageCacheVersion
			file.CacheVersion = ImageCacheVersion
			file.Width, file.Height, file.Thumbnail, file.Preview = i.ensureImageCache(source, id, cacheCurrent)
		case "video":
			i.setJob(storageID, func(job *JobStatus) { job.Videos++ })
			if unchanged && old.Width > 0 && old.Height > 0 {
				file.Width, file.Height = old.Width, old.Height
			} else {
				file.Width, file.Height = i.probeVideoDimensions(ctx, source)
			}
			file.Thumbnail = i.ensureFFmpegThumbnail(ctx, source, id, unchanged, false)
		case "audio":
			i.setJob(storageID, func(job *JobStatus) { job.Audio++ })
			file.Thumbnail = i.ensureFFmpegThumbnail(ctx, source, id, unchanged, true)
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
			i.removeCache(old)
		}
	}
	if err := i.catalog.DeleteIDs(ctx, deleted); err != nil {
		i.failJob(storageID, err)
		return
	}
	i.setJob(storageID, func(job *JobStatus) { job.Removed = len(deleted) })
	if i.catalog.ShouldCompact() {
		if err := i.catalog.Compact(ctx); err != nil {
			i.logger.Warn("no se pudo compactar catálogo", "error", err)
		}
	}
	i.setJob(storageID, func(job *JobStatus) { job.State = "done"; job.Scanned = job.Total; job.EndedAt = time.Now().UTC() })
}

func countIndexableFiles(ctx context.Context, root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(source string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrPermission) {
				return nil
			}
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if shouldSkipEntry(root, source, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err == nil && info.Mode().IsRegular() {
			count++
		}
		return nil
	})
	return count, err
}

func shouldSkipEntry(root, source string, entry fs.DirEntry) bool {
	if entry.Type()&os.ModeSymlink != 0 {
		return true
	}
	if !entry.IsDir() || source == root {
		return false
	}
	switch strings.ToLower(entry.Name()) {
	case "$recycle.bin", "system volume information", ".trash-1000", "lost+found":
		return true
	}
	return false
}

func (i *Indexer) removeCache(file File) {
	_ = os.Remove(i.catalog.CachePath(file.ID, "thumbnail"))
	_ = os.Remove(i.catalog.CachePath(file.ID, "preview"))
}

// EnsureImageCacheCurrent regenera bajo demanda miniatura y vista previa cuando
// cambia la lógica de orientación. Así una actualización corrige cachés antiguas
// al primer acceso sin exigir que el usuario reindexe manualmente toda la unidad.
func (i *Indexer) EnsureImageCacheCurrent(ctx context.Context, id string) (File, error) {
	i.cacheMu.Lock()
	defer i.cacheMu.Unlock()

	file, ok := i.catalog.ByID(id)
	if !ok {
		return File{}, os.ErrNotExist
	}
	if file.Kind != "image" {
		return file, errors.New("el archivo no es una imagen")
	}
	if file.CacheVersion >= ImageCacheVersion && file.Thumbnail && file.Preview && fileExistsNonEmpty(i.catalog.CachePath(id, "thumbnail")) && fileExistsNonEmpty(i.catalog.CachePath(id, "preview")) {
		return file, nil
	}
	lease, err := i.manager.Acquire(ctx, file.StorageID, false)
	if err != nil {
		return file, err
	}
	defer lease.Release()

	relative := filepath.Clean(filepath.FromSlash(file.RelativePath))
	if relative == "." || filepath.IsAbs(relative) {
		return file, errors.New("ruta de imagen inválida")
	}
	source := filepath.Join(lease.Root, relative)
	within, err := filepath.Rel(lease.Root, source)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return file, errors.New("ruta de imagen fuera de la unidad")
	}
	width, height, thumbnail, preview := i.ensureImageCache(source, id, false)
	if !thumbnail || !preview {
		return file, errors.New("no se pudo regenerar la caché de imagen")
	}
	file.Width = width
	file.Height = height
	file.Thumbnail = thumbnail
	file.Preview = preview
	file.CacheVersion = ImageCacheVersion
	if err := i.catalog.UpsertBatch(ctx, []File{file}); err != nil {
		return file, err
	}
	return file, nil
}

func (i *Indexer) ensureFFmpegThumbnail(ctx context.Context, source, id string, unchanged, audio bool) bool {
	if i.ffmpeg == "" {
		return false
	}
	destination := i.catalog.CachePath(id, "thumbnail")
	if unchanged {
		if info, err := os.Stat(destination); err == nil && info.Size() > 0 {
			return true
		}
	} else {
		_ = os.Remove(destination)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return false
	}
	tmp := destination + ".tmp.jpg"
	_ = os.Remove(tmp)
	var args []string
	if audio {
		args = []string{"-hide_banner", "-loglevel", "error", "-y", "-i", source, "-map", "0:v:0", "-frames:v", "1", "-vf", "scale=640:640:force_original_aspect_ratio=decrease", tmp}
	} else {
		args = []string{"-hide_banner", "-loglevel", "error", "-y", "-ss", "1", "-i", source, "-frames:v", "1", "-vf", "scale=640:640:force_original_aspect_ratio=decrease", tmp}
	}
	cmdCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, i.ffmpeg, args...)
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		return false
	}
	info, err := os.Stat(tmp)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(tmp)
		return false
	}
	_ = os.Remove(destination)
	if err := os.Rename(tmp, destination); err != nil {
		_ = os.Remove(tmp)
		return false
	}
	return true
}

func (i *Indexer) ensureImageCache(source, id string, unchanged bool) (int, int, bool, bool) {
	thumbPath := i.catalog.CachePath(id, "thumbnail")
	previewPath := i.catalog.CachePath(id, "preview")
	if unchanged {
		if info, err := os.Stat(thumbPath); err == nil && info.Size() > 0 {
			if pinfo, err := os.Stat(previewPath); err == nil && pinfo.Size() > 0 {
				if w, h := imageDimensions(source); w > 0 {
					return w, h, true, true
				}
			}
		}
	} else {
		_ = os.Remove(thumbPath)
		_ = os.Remove(previewPath)
	}
	orientation := imageOrientation(source)
	file, err := os.Open(source)
	if err != nil {
		return 0, 0, false, false
	}
	config, _, err := image.DecodeConfig(file)
	_ = file.Close()
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return i.ensureFFmpegImageCache(source, id, unchanged)
	}
	displayWidth, displayHeight := orientedDimensions(config.Width, config.Height, orientation)
	if int64(config.Width) > maxThumbnailSourcePixels/int64(config.Height) {
		return displayWidth, displayHeight, false, false
	}
	file, err = os.Open(source)
	if err != nil {
		return displayWidth, displayHeight, false, false
	}
	img, _, err := image.Decode(file)
	_ = file.Close()
	if err != nil {
		return displayWidth, displayHeight, false, false
	}
	img = orientImage(img, orientation)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	return w, h, writeScaledJPEG(img, thumbPath, 320, 82) == nil, writeScaledJPEG(img, previewPath, 1600, 88) == nil
}

func (i *Indexer) ensureFFmpegImageCache(source, id string, unchanged bool) (int, int, bool, bool) {
	if i.ffmpeg == "" {
		return 0, 0, false, false
	}
	thumbPath := i.catalog.CachePath(id, "thumbnail")
	previewPath := i.catalog.CachePath(id, "preview")
	if unchanged {
		thumbOK := fileExistsNonEmpty(thumbPath)
		previewOK := fileExistsNonEmpty(previewPath)
		if thumbOK && previewOK {
			return 0, 0, true, true
		}
	}
	if err := os.MkdirAll(filepath.Dir(previewPath), 0o700); err != nil {
		return 0, 0, false, false
	}
	tmpPreview := previewPath + ".tmp.jpg"
	_ = os.Remove(tmpPreview)
	cmdCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, i.ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", source, "-frames:v", "1",
		"-vf", "scale=1600:1600:force_original_aspect_ratio=decrease",
		tmpPreview,
	)
	if err := cmd.Run(); err != nil || !fileExistsNonEmpty(tmpPreview) {
		_ = os.Remove(tmpPreview)
		return 0, 0, false, false
	}
	previewFile, err := os.Open(tmpPreview)
	if err != nil {
		_ = os.Remove(tmpPreview)
		return 0, 0, false, false
	}
	img, _, err := image.Decode(previewFile)
	_ = previewFile.Close()
	if err != nil {
		_ = os.Remove(tmpPreview)
		return 0, 0, false, false
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if err := writeScaledJPEG(img, thumbPath, 320, 82); err != nil {
		_ = os.Remove(tmpPreview)
		return w, h, false, false
	}
	_ = os.Remove(previewPath)
	if err := os.Rename(tmpPreview, previewPath); err != nil {
		_ = os.Remove(tmpPreview)
		return w, h, true, false
	}
	return w, h, true, true
}

func fileExistsNonEmpty(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

func (i *Indexer) probeVideoDimensions(ctx context.Context, source string) (int, int) {
	if i.ffprobe == "" {
		return 0, 0
	}
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, i.ffprobe,
		"-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height:stream_tags=rotate:stream_side_data=rotation",
		"-of", "json", source,
	).Output()
	if err != nil {
		return 0, 0
	}
	var result struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
			Tags   struct {
				Rotate string `json:"rotate"`
			} `json:"tags"`
			SideData []struct {
				Rotation int `json:"rotation"`
			} `json:"side_data_list"`
		} `json:"streams"`
	}
	if json.Unmarshal(output, &result) != nil || len(result.Streams) == 0 {
		return 0, 0
	}
	stream := result.Streams[0]
	rotation := 0
	if stream.Tags.Rotate == "90" || stream.Tags.Rotate == "-270" {
		rotation = 90
	} else if stream.Tags.Rotate == "270" || stream.Tags.Rotate == "-90" {
		rotation = 270
	}
	for _, side := range stream.SideData {
		if side.Rotation == 90 || side.Rotation == -270 {
			rotation = 90
		} else if side.Rotation == 270 || side.Rotation == -90 {
			rotation = 270
		}
	}
	if rotation == 90 || rotation == 270 {
		return stream.Height, stream.Width
	}
	return stream.Width, stream.Height
}

func imageDimensions(source string) (int, int) {
	file, err := os.Open(source)
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	c, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0
	}
	return orientedDimensions(c.Width, c.Height, imageOrientation(source))
}
func writeScaledJPEG(src image.Image, destination string, maxDimension, quality int) error {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return errors.New("imagen sin dimensiones")
	}
	tw, th := fitDimensions(w, h, maxDimension)
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	for y := 0; y < th; y++ {
		sy := b.Min.Y + y*h/th
		for x := 0; x < tw; x++ {
			sx := b.Min.X + x*w/tw
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
func fitDimensions(w, h, max int) (int, int) {
	if w <= max && h <= max {
		return w, h
	}
	if w >= h {
		th := h * max / w
		if th < 1 {
			th = 1
		}
		return max, th
	}
	tw := w * max / h
	if tw < 1 {
		tw = 1
	}
	return tw, max
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
	case ".mp4", ".mkv", ".mov", ".avi", ".webm", ".m4v", ".mts", ".m2ts":
		return "video", mimeType
	case ".mp3", ".flac", ".wav", ".m4a", ".ogg", ".opus", ".aac", ".wma":
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
