package vfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"personalcloud/internal/storage"
	"personalcloud/internal/store"
)

var (
	ErrInvalidPath    = errors.New("ruta virtual inválida")
	ErrCrossVolume    = errors.New("operación entre unidades distintas no soportada")
	ErrCategoryPolicy = errors.New("el tipo de archivo no está permitido en esta unidad")
)

type FS struct {
	manager *storage.Manager
	store   *store.Store
}

type ReadHandle struct {
	File  *os.File
	lease *storage.Lease
}

type Entry struct {
	Name        string
	VirtualPath string
	IsDir       bool
	Size        int64
	ModTime     time.Time
	VolumeID    string
	VirtualRoot string
}

func New(manager *storage.Manager, state *store.Store) *FS {
	return &FS{manager: manager, store: state}
}

func (h *ReadHandle) Close() error {
	if h == nil {
		return nil
	}
	var err error
	if h.File != nil {
		err = h.File.Close()
	}
	if h.lease != nil {
		h.lease.Release()
		h.lease = nil
	}
	return err
}

func (f *FS) StorageID(ctx context.Context, virtualPath string) (string, error) {
	_, _, cfg, err := f.resolve(ctx, virtualPath)
	if err != nil {
		return "", err
	}
	return cfg.ID, nil
}

func (f *FS) Roots(ctx context.Context) ([]Entry, error) {
	volumes, err := f.store.ListStorageVolumes(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(volumes))
	for _, volume := range volumes {
		entries = append(entries, Entry{
			Name:        volume.VirtualRoot,
			VirtualPath: "/" + volume.VirtualRoot,
			IsDir:       true,
			VolumeID:    volume.ID,
			VirtualRoot: volume.VirtualRoot,
			ModTime:     volume.UpdatedAt,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name) })
	return entries, nil
}

func (f *FS) Stat(ctx context.Context, virtualPath string) (Entry, error) {
	root, rel, cfg, err := f.resolve(ctx, virtualPath)
	if err != nil {
		if errors.Is(err, errVirtualRoot) && cleanVirtualPath(virtualPath) == "/" {
			return Entry{Name: "/", VirtualPath: "/", IsDir: true}, nil
		}
		return Entry{}, err
	}
	if rel == "" {
		return Entry{Name: cfg.VirtualRoot, VirtualPath: "/" + cfg.VirtualRoot, IsDir: true, VolumeID: cfg.ID, VirtualRoot: cfg.VirtualRoot, ModTime: cfg.UpdatedAt}, nil
	}
	lease, err := f.manager.Acquire(ctx, cfg.ID, false)
	if err != nil {
		return Entry{}, err
	}
	defer lease.Release()
	physical, err := safePhysicalPath(lease.Root, rel, false)
	if err != nil {
		return Entry{}, err
	}
	info, err := os.Stat(physical)
	if err != nil {
		return Entry{}, err
	}
	return entryFromInfo(root, rel, cfg.ID, cfg.VirtualRoot, info), nil
}

func (f *FS) ReadDir(ctx context.Context, virtualPath string) ([]Entry, error) {
	clean := cleanVirtualPath(virtualPath)
	if clean == "/" {
		return f.Roots(ctx)
	}
	root, rel, cfg, err := f.resolve(ctx, virtualPath)
	if err != nil {
		return nil, err
	}
	lease, err := f.manager.Acquire(ctx, cfg.ID, false)
	if err != nil {
		return nil, err
	}
	defer lease.Release()
	physical, err := safePhysicalPath(lease.Root, rel, false)
	if err != nil {
		return nil, err
	}
	items, err := os.ReadDir(physical)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		info, err := item.Info()
		if err != nil {
			continue
		}
		childRel := item.Name()
		if rel != "" {
			childRel = path.Join(filepath.ToSlash(rel), item.Name())
		}
		entries = append(entries, entryFromInfo(root, childRel, cfg.ID, cfg.VirtualRoot, info))
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

func (f *FS) OpenRead(ctx context.Context, virtualPath string) (*ReadHandle, Entry, error) {
	root, rel, cfg, err := f.resolve(ctx, virtualPath)
	if err != nil {
		return nil, Entry{}, err
	}
	if rel == "" {
		return nil, Entry{}, errors.New("no se puede abrir una raíz como archivo")
	}
	lease, err := f.manager.Acquire(ctx, cfg.ID, false)
	if err != nil {
		return nil, Entry{}, err
	}
	physical, err := safePhysicalPath(lease.Root, rel, false)
	if err != nil {
		lease.Release()
		return nil, Entry{}, err
	}
	file, err := os.Open(physical)
	if err != nil {
		lease.Release()
		return nil, Entry{}, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		lease.Release()
		return nil, Entry{}, err
	}
	if info.IsDir() {
		_ = file.Close()
		lease.Release()
		return nil, Entry{}, errors.New("la ruta corresponde a un directorio")
	}
	return &ReadHandle{File: file, lease: lease}, entryFromInfo(root, rel, cfg.ID, cfg.VirtualRoot, info), nil
}

func (f *FS) WriteAtomic(ctx context.Context, virtualPath string, src io.Reader, maxBytes int64, overwrite bool) (int64, error) {
	_, rel, cfg, err := f.resolve(ctx, virtualPath)
	if err != nil {
		return 0, err
	}
	if rel == "" {
		return 0, ErrInvalidPath
	}
	if !storage.CategoryAllowsFile(cfg.Category, filepath.Base(rel)) {
		return 0, ErrCategoryPolicy
	}
	lease, err := f.manager.Acquire(ctx, cfg.ID, true)
	if err != nil {
		return 0, err
	}
	defer lease.Release()
	target, err := safePhysicalPath(lease.Root, rel, true)
	if err != nil {
		return 0, err
	}
	parent := filepath.Dir(target)
	if _, err := os.Stat(parent); err != nil {
		return 0, fmt.Errorf("el directorio de destino no existe: %w", err)
	}
	if !overwrite {
		if _, err := os.Stat(target); err == nil {
			return 0, os.ErrExist
		}
	}
	tmp, err := os.CreateTemp(parent, ".personalcloud-upload-*.tmp")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	reader := src
	if maxBytes > 0 {
		reader = io.LimitReader(src, maxBytes+1)
	}
	written, err := io.Copy(tmp, reader)
	if err != nil {
		cleanup()
		return written, err
	}
	if maxBytes > 0 && written > maxBytes {
		cleanup()
		return written, fmt.Errorf("archivo supera el límite de %d bytes", maxBytes)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return written, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return written, err
	}
	if err := replaceAtomic(tmpName, target, overwrite); err != nil {
		_ = os.Remove(tmpName)
		return written, err
	}
	return written, nil
}

func (f *FS) MkdirAll(ctx context.Context, virtualPath string) error {
	_, rel, cfg, err := f.resolve(ctx, virtualPath)
	if err != nil {
		return err
	}
	if rel == "" {
		return nil
	}
	lease, err := f.manager.Acquire(ctx, cfg.ID, true)
	if err != nil {
		return err
	}
	defer lease.Release()
	target, err := safePhysicalPath(lease.Root, rel, true)
	if err != nil {
		return err
	}
	return os.MkdirAll(target, 0o755)
}

func (f *FS) Mkdir(ctx context.Context, virtualPath string) error {
	_, rel, cfg, err := f.resolve(ctx, virtualPath)
	if err != nil {
		return err
	}
	if rel == "" {
		return os.ErrExist
	}
	lease, err := f.manager.Acquire(ctx, cfg.ID, true)
	if err != nil {
		return err
	}
	defer lease.Release()
	target, err := safePhysicalPath(lease.Root, rel, true)
	if err != nil {
		return err
	}
	return os.Mkdir(target, 0o755)
}

func (f *FS) Remove(ctx context.Context, virtualPath string) error {
	_, rel, cfg, err := f.resolve(ctx, virtualPath)
	if err != nil {
		return err
	}
	if rel == "" {
		return errors.New("no se puede eliminar una raíz virtual")
	}
	lease, err := f.manager.Acquire(ctx, cfg.ID, true)
	if err != nil {
		return err
	}
	defer lease.Release()
	target, err := safePhysicalPath(lease.Root, rel, false)
	if err != nil {
		return err
	}
	return os.RemoveAll(target)
}

func (f *FS) Rename(ctx context.Context, from, to string, overwrite bool) error {
	_, fromRel, fromCfg, err := f.resolve(ctx, from)
	if err != nil {
		return err
	}
	_, toRel, toCfg, err := f.resolve(ctx, to)
	if err != nil {
		return err
	}
	if fromCfg.ID != toCfg.ID {
		return ErrCrossVolume
	}
	if fromRel == "" || toRel == "" {
		return ErrInvalidPath
	}
	lease, err := f.manager.Acquire(ctx, fromCfg.ID, true)
	if err != nil {
		return err
	}
	defer lease.Release()
	source, err := safePhysicalPath(lease.Root, fromRel, false)
	if err != nil {
		return err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() && !storage.CategoryAllowsFile(fromCfg.Category, filepath.Base(toRel)) {
		return ErrCategoryPolicy
	}
	target, err := safePhysicalPath(lease.Root, toRel, true)
	if err != nil {
		return err
	}
	if !overwrite {
		if _, err := os.Stat(target); err == nil {
			return os.ErrExist
		}
	}
	return replaceAtomic(source, target, overwrite)
}

func (f *FS) resolve(ctx context.Context, virtualPath string) (string, string, store.StorageVolume, error) {
	clean := cleanVirtualPath(virtualPath)
	if clean == "/" {
		return "", "", store.StorageVolume{}, errVirtualRoot
	}
	parts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", store.StorageVolume{}, ErrInvalidPath
	}
	cfg, err := f.store.StorageVolumeByVirtualRoot(ctx, parts[0])
	if err != nil {
		return "", "", store.StorageVolume{}, err
	}
	rel := ""
	if len(parts) > 1 {
		rel = filepath.FromSlash(strings.Join(parts[1:], "/"))
	}
	return parts[0], rel, cfg, nil
}

var errVirtualRoot = errors.New("raíz virtual")

func cleanVirtualPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/"
	}
	if strings.ContainsAny(value, "\\\x00") {
		return ""
	}
	for _, part := range strings.Split(strings.ReplaceAll(value, "\\", "/"), "/") {
		if part == ".." {
			return ""
		}
	}
	clean := path.Clean("/" + strings.TrimPrefix(value, "/"))
	if clean == "." {
		return "/"
	}
	return clean
}

func safePhysicalPath(root, relative string, allowMissing bool) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate, err := filepath.Abs(filepath.Join(rootAbs, relative))
	if err != nil {
		return "", err
	}
	if !withinRoot(rootAbs, candidate) {
		return "", ErrInvalidPath
	}
	if containsSymlink(rootAbs, candidate) {
		return "", ErrInvalidPath
	}
	if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
		if !withinRoot(rootAbs, resolved) {
			return "", ErrInvalidPath
		}
		return resolved, nil
	} else if !allowMissing && !errors.Is(err, os.ErrNotExist) {
		return "", err
	} else if !allowMissing {
		return "", err
	}

	parent := filepath.Dir(candidate)
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			if !withinRoot(rootAbs, resolved) {
				return "", ErrInvalidPath
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		next := filepath.Dir(parent)
		if next == parent || !withinRoot(rootAbs, next) {
			return "", ErrInvalidPath
		}
		parent = next
	}
	return candidate, nil
}

func containsSymlink(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == "." {
		return false
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return false
		}
		if err != nil {
			return false
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

func withinRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func replaceAtomic(source, target string, overwrite bool) error {
	if !overwrite {
		return os.Rename(source, target)
	}
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		return os.Rename(source, target)
	} else if err != nil {
		return err
	}
	backup := target + ".personalcloud-replace"
	_ = os.RemoveAll(backup)
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(source, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	_ = os.RemoveAll(backup)
	if runtime.GOOS != "windows" {
		if dir, err := os.Open(filepath.Dir(target)); err == nil {
			_ = dir.Sync()
			_ = dir.Close()
		}
	}
	return nil
}

func entryFromInfo(root, rel, volumeID, virtualRoot string, info fs.FileInfo) Entry {
	virtualPath := "/" + root
	if rel != "" {
		virtualPath = path.Join(virtualPath, filepath.ToSlash(rel))
	}
	return Entry{
		Name:        info.Name(),
		VirtualPath: virtualPath,
		IsDir:       info.IsDir(),
		Size:        info.Size(),
		ModTime:     info.ModTime(),
		VolumeID:    volumeID,
		VirtualRoot: virtualRoot,
	}
}
