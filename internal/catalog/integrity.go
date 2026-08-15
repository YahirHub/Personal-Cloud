package catalog

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// verifyMediaQuick hace una comprobación conservadora. Las imágenes soportadas
// por Go se decodifican completas cuando su tamaño es razonable. Si FFmpeg está
// disponible, video/audio se validan además decodificando muestras del inicio,
// centro y final; esto detecta truncamientos y corrupción común sin decodificar
// horas de contenido durante una sincronización normal.
func (i *Indexer) verifyMediaQuick(ctx context.Context, source, kind string, size int64) (health, detail string) {
	if size <= 0 {
		return "damaged", "archivo vacío"
	}
	switch kind {
	case "image":
		f, err := os.Open(source)
		if err != nil {
			return "damaged", shortIntegrityError(err)
		}
		config, _, cfgErr := image.DecodeConfig(f)
		_ = f.Close()
		if cfgErr == nil && config.Width > 0 && config.Height > 0 {
			if int64(config.Width) > maxThumbnailSourcePixels/int64(config.Height) {
				if i.ffmpeg != "" {
					return i.verifyImageWithFFmpeg(ctx, source)
				}
				return "unchecked", "imagen demasiado grande para validación completa sin FFmpeg"
			}
			f, err = os.Open(source)
			if err != nil {
				return "damaged", shortIntegrityError(err)
			}
			_, _, err = image.Decode(f)
			_ = f.Close()
			if err == nil {
				return "ok", ""
			}
			return "damaged", shortIntegrityError(err)
		}
		// HEIC/AVIF/RAW/WebP pueden no tener decoder stdlib. FFmpeg, si existe,
		// intenta decodificar realmente un frame en vez de confiar solo en metadata.
		if i.ffmpeg != "" {
			return i.verifyImageWithFFmpeg(ctx, source)
		}
		if i.ffprobe != "" {
			return i.verifyWithFFprobe(ctx, source, "v:0")
		}
		return "unchecked", "formato sin decoder disponible"
	case "video":
		return i.verifyAV(ctx, source, "video")
	case "audio":
		return i.verifyAV(ctx, source, "audio")
	default:
		return "unchecked", ""
	}
}

func (i *Indexer) verifyImageWithFFmpeg(ctx context.Context, source string) (string, string) {
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, i.ffmpeg,
		"-hide_banner", "-loglevel", "error", "-xerror", "-i", source,
		"-map", "0:v:0", "-frames:v", "1", "-f", "null", "-",
	).CombinedOutput()
	if err == nil {
		return "ok", ""
	}
	if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
		return "unchecked", "validación excedió 20 s"
	}
	return classifyFFmpegIntegrityError(output, err)
}

func (i *Indexer) verifyAV(ctx context.Context, source, kind string) (string, string) {
	stream := "v:0"
	if kind == "audio" {
		stream = "a:0"
	}
	if i.ffprobe != "" {
		metadataHealth, metadataDetail := i.verifyWithFFprobe(ctx, source, stream)
		if metadataHealth != "ok" {
			return metadataHealth, metadataDetail
		}
	}
	if i.ffmpeg == "" {
		if i.ffprobe != "" {
			return "ok", "metadata válida; FFmpeg no disponible para muestreo de decodificación"
		}
		return "unchecked", "FFmpeg/ffprobe no disponibles"
	}

	duration := i.probeDuration(ctx, source)
	points := integritySamplePoints(duration)
	for _, second := range points {
		sampleCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		args := []string{"-hide_banner", "-loglevel", "error", "-xerror"}
		if second > 0 {
			args = append(args, "-ss", strconv.FormatFloat(second, 'f', 3, 64))
		}
		args = append(args, "-i", source)
		if kind == "audio" {
			args = append(args, "-map", "0:a:0", "-vn")
		} else {
			args = append(args, "-map", "0:v:0", "-an")
		}
		args = append(args, "-t", "1.25", "-f", "null", "-")
		output, err := exec.CommandContext(sampleCtx, i.ffmpeg, args...).CombinedOutput()
		deadline := errors.Is(sampleCtx.Err(), context.DeadlineExceeded)
		cancel()
		if err == nil {
			continue
		}
		if deadline {
			return "unchecked", fmt.Sprintf("validación de %s excedió el tiempo límite", kind)
		}
		health, detail := classifyFFmpegIntegrityError(output, err)
		if health == "damaged" && second > 0 {
			detail = fmt.Sprintf("corrupción cerca de %.1f s: %s", second, detail)
		}
		return health, detail
	}
	return "ok", ""
}

func (i *Indexer) probeDuration(ctx context.Context, source string) float64 {
	if i.ffprobe == "" {
		return 0
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, i.ffprobe,
		"-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", source,
	).Output()
	if err != nil {
		return 0
	}
	value, _ := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	return value
}

func integritySamplePoints(duration float64) []float64 {
	if duration <= 4 {
		return []float64{0}
	}
	points := []float64{0, duration * .5, math.Max(0, duration-2)}
	out := make([]float64, 0, len(points))
	for _, point := range points {
		duplicate := false
		for _, existing := range out {
			if math.Abs(existing-point) < 1 {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, point)
		}
	}
	return out
}

func (i *Indexer) verifyWithFFprobe(ctx context.Context, source, stream string) (string, string) {
	if i.ffprobe == "" {
		return "unchecked", "ffprobe no disponible"
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, i.ffprobe,
		"-v", "error", "-select_streams", stream,
		"-show_entries", "stream=codec_name", "-of", "default=nw=1:nk=1", source,
	)
	output, err := cmd.CombinedOutput()
	if err == nil && strings.TrimSpace(string(output)) != "" {
		return "ok", ""
	}
	if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
		return "unchecked", "validación excedió 15 s"
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" && err != nil {
		detail = err.Error()
	}
	return "damaged", shortIntegrityError(fmt.Errorf("ffprobe: %s", detail))
}

func classifyFFmpegIntegrityError(output []byte, err error) (string, string) {
	detail := strings.TrimSpace(string(output))
	lower := strings.ToLower(detail)
	// Si el FFmpeg instalado no sabe decodificar el formato, no acuses daño.
	for _, marker := range []string{"decoder not found", "unknown decoder", "unsupported codec", "not implemented", "operation not permitted"} {
		if strings.Contains(lower, marker) {
			return "unchecked", shortIntegrityError(fmt.Errorf("FFmpeg no puede verificar este codec: %s", detail))
		}
	}
	if detail == "" && err != nil {
		detail = err.Error()
	}
	return "damaged", shortIntegrityError(fmt.Errorf("FFmpeg: %s", detail))
}

func shortIntegrityError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) > 180 {
		text = text[:180] + "…"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "archivo truncado"
	}
	return text
}
