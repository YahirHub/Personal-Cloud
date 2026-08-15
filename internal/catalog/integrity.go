package catalog

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// verifyMediaQuick hace una comprobación conservadora. Las imágenes soportadas
// por Go se decodifican completas cuando su tamaño es razonable; video/audio se
// validan con ffprobe cuando está disponible. "unchecked" nunca se presenta al
// usuario como archivo dañado.
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
			// No afirmes que una imagen gigante está sana solo porque su cabecera es válida.
			// Si ffprobe existe, úsalo como comprobación adicional; de lo contrario deja
			// el estado como desconocido para evitar falsos positivos sin agotar memoria.
			if int64(config.Width) > maxThumbnailSourcePixels/int64(config.Height) {
				if i.ffprobe != "" {
					return i.verifyWithFFprobe(ctx, source, "v:0")
				}
				return "unchecked", "imagen demasiado grande para validación completa sin ffprobe"
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
		// HEIC/AVIF/RAW/WebP pueden no tener decoder stdlib. Si ffprobe existe,
		// úsalo; si no, el estado es desconocido, no dañado.
		if i.ffprobe == "" {
			return "unchecked", ""
		}
		return i.verifyWithFFprobe(ctx, source, "v:0")
	case "video":
		if i.ffprobe == "" {
			return "unchecked", ""
		}
		return i.verifyWithFFprobe(ctx, source, "v:0")
	case "audio":
		if i.ffprobe == "" {
			return "unchecked", ""
		}
		return i.verifyWithFFprobe(ctx, source, "a:0")
	default:
		return "unchecked", ""
	}
}

func (i *Indexer) verifyWithFFprobe(ctx context.Context, source, stream string) (string, string) {
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
