package streaming

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"personalcloud/internal/catalog"
	"personalcloud/internal/vfs"
)

var (
	ErrUnavailable    = errors.New("FFmpeg con codificador H.264 no está disponible")
	ErrInvalidQuality = errors.New("calidad de video inválida")
)

const variantRetention = 72 * time.Hour

type Profile struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Height int    `json:"height"`
}

type Status struct {
	State   string `json:"state"`
	Quality string `json:"quality"`
	URL     string `json:"url,omitempty"`
	Error   string `json:"error,omitempty"`
}

type job struct {
	state string
	err   string
}

type Manager struct {
	vfs      *vfs.FS
	logger   *slog.Logger
	cacheDir string
	ffmpeg   string
	ffprobe  string
	encoder  string

	ctx    context.Context
	cancel context.CancelFunc
	sem    chan struct{}
	wg     sync.WaitGroup
	mu     sync.Mutex
	jobs   map[string]*job
}

func New(dataDir string, filesystem *vfs.FS, logger *slog.Logger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		vfs:      filesystem,
		logger:   logger,
		cacheDir: filepath.Join(dataDir, "cache", "video-variants"),
		ctx:      ctx,
		cancel:   cancel,
		sem:      make(chan struct{}, 1),
		jobs:     make(map[string]*job),
	}
	_ = os.MkdirAll(m.cacheDir, 0o700)
	m.ffmpeg, _ = exec.LookPath("ffmpeg")
	m.ffprobe, _ = exec.LookPath("ffprobe")
	if m.ffmpeg != "" && hasEncoder(m.ffmpeg, "libx264") {
		m.encoder = "libx264"
		logger.Info("streaming multiresolución habilitado", "ffmpeg", m.ffmpeg, "encoder", m.encoder)
	} else if m.ffmpeg != "" {
		logger.Warn("FFmpeg detectado sin libx264; streaming multiresolución deshabilitado", "ffmpeg", m.ffmpeg)
	}
	m.cleanupOldVariants(time.Now())
	return m
}

func (m *Manager) Close() {
	m.cancel()
	m.wg.Wait()
}

func (m *Manager) Available() bool { return m.ffmpeg != "" && m.encoder != "" }

func (m *Manager) ProbeDimensions(ctx context.Context, file catalog.File) (int, int, error) {
	if m.ffprobe == "" || file.Kind != "video" {
		return 0, 0, ErrUnavailable
	}
	virtualPath := path.Join("/", file.VirtualRoot, file.RelativePath)
	handle, _, err := m.vfs.OpenRead(ctx, virtualPath)
	if err != nil {
		return 0, 0, err
	}
	defer handle.Close()
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, m.ffprobe,
		"-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height:stream_tags=rotate:stream_side_data=rotation",
		"-of", "json", handle.File.Name(),
	).Output()
	if err != nil {
		return 0, 0, err
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
	if err := json.Unmarshal(output, &result); err != nil || len(result.Streams) == 0 {
		return 0, 0, errors.New("ffprobe no devolvió dimensiones")
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
		return stream.Height, stream.Width, nil
	}
	return stream.Width, stream.Height, nil
}

func (m *Manager) Profiles(file catalog.File) []Profile {
	profiles := []Profile{{ID: "original", Label: "Original", Height: 0}}
	if !m.Available() || file.Kind != "video" {
		return profiles
	}
	for _, p := range []Profile{
		{ID: "360", Label: "360p", Height: 360},
		{ID: "480", Label: "480p", Height: 480},
		{ID: "720", Label: "720p", Height: 720},
		{ID: "1080", Label: "1080p", Height: 1080},
	} {
		if file.Height > 0 && p.Height > file.Height {
			continue
		}
		profiles = append(profiles, p)
	}
	return profiles
}

func (m *Manager) Prepare(file catalog.File, quality string) (Status, error) {
	if !m.Available() {
		return Status{}, ErrUnavailable
	}
	profile, ok := profileByID(quality)
	if !ok || profile.ID == "original" {
		return Status{}, ErrInvalidQuality
	}
	if file.Kind != "video" {
		return Status{}, errors.New("el archivo no es un video")
	}
	if file.Height > 0 && profile.Height > file.Height {
		return Status{}, ErrInvalidQuality
	}
	if final := m.variantPath(file, profile.ID); fileExists(final) {
		return Status{State: "ready", Quality: profile.ID, URL: variantURL(file.ID, profile.ID)}, nil
	}
	key := jobKey(file, profile.ID)
	m.mu.Lock()
	if existing := m.jobs[key]; existing != nil {
		if existing.state == "error" || existing.state == "ready" {
			delete(m.jobs, key)
		} else {
			status := statusFromJob(existing, file.ID, profile.ID)
			m.mu.Unlock()
			return status, nil
		}
	}
	m.jobs[key] = &job{state: "queued"}
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.transcode(file, profile)
	}()
	return Status{State: "queued", Quality: profile.ID}, nil
}

func (m *Manager) Status(file catalog.File, quality string) (Status, error) {
	profile, ok := profileByID(quality)
	if !ok || profile.ID == "original" {
		return Status{}, ErrInvalidQuality
	}
	if final := m.variantPath(file, profile.ID); fileExists(final) {
		return Status{State: "ready", Quality: profile.ID, URL: variantURL(file.ID, profile.ID)}, nil
	}
	key := jobKey(file, profile.ID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing := m.jobs[key]; existing != nil {
		if existing.state == "ready" {
			delete(m.jobs, key)
			return Status{State: "idle", Quality: profile.ID}, nil
		}
		return statusFromJob(existing, file.ID, profile.ID), nil
	}
	return Status{State: "idle", Quality: profile.ID}, nil
}

func (m *Manager) VariantPath(file catalog.File, quality string) (string, error) {
	profile, ok := profileByID(quality)
	if !ok || profile.ID == "original" {
		return "", ErrInvalidQuality
	}
	result := m.variantPath(file, profile.ID)
	if !fileExists(result) {
		return "", os.ErrNotExist
	}
	return result, nil
}

func (m *Manager) Cleanup() { m.cleanupOldVariants(time.Now()) }

func (m *Manager) transcode(file catalog.File, profile Profile) {
	select {
	case m.sem <- struct{}{}:
		defer func() { <-m.sem }()
	case <-m.ctx.Done():
		m.updateJob(file, profile.ID, "error", "servidor detenido")
		return
	}
	m.updateJob(file, profile.ID, "transcoding", "")

	virtualPath := path.Join("/", file.VirtualRoot, file.RelativePath)
	handle, _, err := m.vfs.OpenRead(m.ctx, virtualPath)
	if err != nil {
		m.fail(file, profile.ID, err)
		return
	}
	defer handle.Close()

	final := m.variantPath(file, profile.ID)
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		m.fail(file, profile.ID, err)
		return
	}
	m.removeStaleFingerprints(file)
	tmp := filepath.Join(filepath.Dir(final), "."+profile.ID+".tmp.mp4")
	_ = os.Remove(tmp)

	filter := "scale=-2:trunc(min(" + strconv.Itoa(profile.Height) + "\\,ih)/2)*2"
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", handle.File.Name(),
		"-map", "0:v:0", "-map", "0:a:0?", "-sn", "-dn",
		"-vf", filter,
		"-c:v", m.encoder, "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart", "-f", "mp4", tmp,
	}
	cmd := exec.CommandContext(m.ctx, m.ffmpeg, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(tmp)
		message := strings.TrimSpace(string(output))
		if len(message) > 400 {
			message = message[len(message)-400:]
		}
		if message == "" {
			message = err.Error()
		}
		m.fail(file, profile.ID, errors.New(message))
		return
	}
	if !fileExists(tmp) {
		m.fail(file, profile.ID, errors.New("FFmpeg no produjo una variante válida"))
		return
	}
	_ = os.Remove(final)
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		m.fail(file, profile.ID, err)
		return
	}
	m.updateJob(file, profile.ID, "ready", "")
}

func (m *Manager) fail(file catalog.File, quality string, err error) {
	m.logger.Warn("transcodificación de video fallida", "file_id", file.ID, "quality", quality, "error", err)
	m.updateJob(file, quality, "error", err.Error())
}

func (m *Manager) updateJob(file catalog.File, quality, state, message string) {
	key := jobKey(file, quality)
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.jobs[key]
	if current == nil {
		current = &job{}
		m.jobs[key] = current
	}
	current.state = state
	current.err = message
}

func (m *Manager) variantPath(file catalog.File, quality string) string {
	return filepath.Join(m.cacheDir, file.ID, fingerprint(file), quality+"p.mp4")
}

func (m *Manager) removeStaleFingerprints(file catalog.File) {
	root := filepath.Join(m.cacheDir, file.ID)
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	keep := fingerprint(file)
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != keep {
			_ = os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
}

func (m *Manager) cleanupOldVariants(now time.Time) {
	_ = filepath.WalkDir(m.cacheDir, func(p string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".mp4") {
			return nil
		}
		info, err := entry.Info()
		if err == nil && now.Sub(info.ModTime()) > variantRetention {
			_ = os.Remove(p)
		}
		return nil
	})
}

func hasEncoder(ffmpeg, encoder string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-encoders").CombinedOutput()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == encoder {
			return true
		}
	}
	return false
}

func profileByID(id string) (Profile, bool) {
	if id == "original" {
		return Profile{ID: "original", Label: "Original"}, true
	}
	for _, p := range []Profile{{ID: "360", Label: "360p", Height: 360}, {ID: "480", Label: "480p", Height: 480}, {ID: "720", Label: "720p", Height: 720}, {ID: "1080", Label: "1080p", Height: 1080}} {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

func fingerprint(file catalog.File) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d", file.Size, file.ModTime.UnixNano())))
	return hex.EncodeToString(sum[:8])
}

func jobKey(file catalog.File, quality string) string {
	return file.ID + ":" + fingerprint(file) + ":" + quality
}
func variantURL(fileID, quality string) string { return "/archivo/" + fileID + "/video/" + quality }

func statusFromJob(current *job, fileID, quality string) Status {
	status := Status{State: current.state, Quality: quality, Error: current.err}
	if current.state == "ready" {
		status.URL = variantURL(fileID, quality)
	}
	return status
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}
