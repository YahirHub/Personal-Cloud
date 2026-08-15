package catalog

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyMediaQuickDetectsEmptyAndTruncatedImage(t *testing.T) {
	dir := t.TempDir()
	indexer := &Indexer{}

	empty := filepath.Join(dir, "empty.jpg")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if health, _ := indexer.verifyMediaQuick(context.Background(), empty, "image", 0); health != "damaged" {
		t.Fatalf("archivo vacío health=%q; se esperaba damaged", health)
	}

	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(y * 3), B: uint8((x + y) * 2), A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	if len(data) < 300 {
		t.Fatalf("JPEG de prueba demasiado pequeño: %d", len(data))
	}
	// Conserva cabeceras suficientes para DecodeConfig, pero elimina parte de los datos
	// para que la decodificación completa detecte el truncado.
	truncatedData := data[:len(data)*3/4]
	truncated := filepath.Join(dir, "truncated.jpg")
	if err := os.WriteFile(truncated, truncatedData, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(truncated)
	if err != nil {
		t.Fatal(err)
	}
	_, _, cfgErr := image.DecodeConfig(f)
	_ = f.Close()
	if cfgErr != nil {
		t.Fatalf("la prueba requiere que DecodeConfig aún funcione: %v", cfgErr)
	}
	if health, _ := indexer.verifyMediaQuick(context.Background(), truncated, "image", int64(len(truncatedData))); health != "damaged" {
		t.Fatalf("JPEG truncado health=%q; se esperaba damaged", health)
	}
}
