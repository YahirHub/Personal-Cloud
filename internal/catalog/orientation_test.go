package catalog

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

func TestEXIFOrientationLittleEndian(t *testing.T) {
	payload := make([]byte, 6+8+2+12+4)
	copy(payload[:6], []byte("Exif\x00\x00"))
	tiff := payload[6:]
	copy(tiff[:2], []byte("II"))
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 1)
	binary.LittleEndian.PutUint16(tiff[10:12], 0x0112)
	binary.LittleEndian.PutUint16(tiff[12:14], 3)
	binary.LittleEndian.PutUint32(tiff[14:18], 1)
	binary.LittleEndian.PutUint16(tiff[18:20], 6)
	if got := exifOrientation(payload); got != 6 {
		t.Fatalf("orientación=%d, quiero 6", got)
	}

	jpeg := []byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(payload) + 2)}
	jpeg = append(jpeg, payload...)
	jpeg = append(jpeg, 0xff, 0xd9)
	if got := jpegOrientation(bytes.NewReader(jpeg)); got != 6 {
		t.Fatalf("orientación JPEG=%d, quiero 6", got)
	}
}

func TestOrientImageRotate90CW(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	colors := []color.RGBA{
		{R: 1, A: 255}, {R: 2, A: 255}, {R: 3, A: 255},
		{R: 4, A: 255}, {R: 5, A: 255}, {R: 6, A: 255},
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			src.SetRGBA(x, y, colors[y*3+x])
		}
	}
	got := orientImage(src, 6)
	if got.Bounds().Dx() != 2 || got.Bounds().Dy() != 3 {
		t.Fatalf("bounds=%v, quiero 2x3", got.Bounds())
	}
	want := [][]uint8{{4, 1}, {5, 2}, {6, 3}}
	for y := range want {
		for x := range want[y] {
			r, _, _, _ := got.At(x, y).RGBA()
			if uint8(r>>8) != want[y][x] {
				t.Fatalf("pixel %d,%d=%d, quiero %d", x, y, uint8(r>>8), want[y][x])
			}
		}
	}
}
