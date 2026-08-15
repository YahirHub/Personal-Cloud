package catalog

import (
	"encoding/binary"
	"image"
	"image/color"
	"io"
	"os"
)

func imageOrientation(path string) int {
	file, err := os.Open(path)
	if err != nil {
		return 1
	}
	defer file.Close()
	return jpegOrientation(file)
}

func jpegOrientation(r io.Reader) int {
	var soi [2]byte
	if _, err := io.ReadFull(r, soi[:]); err != nil || soi != [2]byte{0xff, 0xd8} {
		return 1
	}
	for {
		marker, err := nextJPEGMarker(r)
		if err != nil {
			return 1
		}
		if marker == 0xd9 || marker == 0xda {
			return 1
		}
		if marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
			continue
		}
		var sizeBytes [2]byte
		if _, err := io.ReadFull(r, sizeBytes[:]); err != nil {
			return 1
		}
		size := int(binary.BigEndian.Uint16(sizeBytes[:])) - 2
		if size < 0 {
			return 1
		}
		if marker != 0xe1 || size > 1<<20 {
			if _, err := io.CopyN(io.Discard, r, int64(size)); err != nil {
				return 1
			}
			continue
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(r, payload); err != nil {
			return 1
		}
		if orientation := exifOrientation(payload); orientation >= 1 && orientation <= 8 {
			return orientation
		}
	}
}

func nextJPEGMarker(r io.Reader) (byte, error) {
	var one [1]byte
	for {
		if _, err := io.ReadFull(r, one[:]); err != nil {
			return 0, err
		}
		if one[0] != 0xff {
			continue
		}
		for {
			if _, err := io.ReadFull(r, one[:]); err != nil {
				return 0, err
			}
			if one[0] != 0xff {
				return one[0], nil
			}
		}
	}
}

func exifOrientation(payload []byte) int {
	if len(payload) < 14 || string(payload[:6]) != "Exif\x00\x00" {
		return 1
	}
	tiff := payload[6:]
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 1
	}
	offset := int(order.Uint32(tiff[4:8]))
	if offset < 0 || offset+2 > len(tiff) {
		return 1
	}
	count := int(order.Uint16(tiff[offset : offset+2]))
	entries := offset + 2
	for n := 0; n < count; n++ {
		pos := entries + n*12
		if pos+12 > len(tiff) {
			break
		}
		if order.Uint16(tiff[pos:pos+2]) != 0x0112 {
			continue
		}
		if order.Uint16(tiff[pos+2:pos+4]) != 3 || order.Uint32(tiff[pos+4:pos+8]) < 1 {
			return 1
		}
		value := int(order.Uint16(tiff[pos+8 : pos+10]))
		if value >= 1 && value <= 8 {
			return value
		}
		return 1
	}
	return 1
}

type orientedImage struct {
	src         image.Image
	orientation int
	bounds      image.Rectangle
}

func orientImage(src image.Image, orientation int) image.Image {
	if orientation < 2 || orientation > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	ow, oh := orientedDimensions(w, h, orientation)
	return &orientedImage{src: src, orientation: orientation, bounds: image.Rect(0, 0, ow, oh)}
}

func orientedDimensions(w, h, orientation int) (int, int) {
	if orientation >= 5 && orientation <= 8 {
		return h, w
	}
	return w, h
}

func (o *orientedImage) ColorModel() color.Model { return o.src.ColorModel() }
func (o *orientedImage) Bounds() image.Rectangle { return o.bounds }
func (o *orientedImage) At(x, y int) color.Color {
	if !image.Pt(x, y).In(o.bounds) {
		return color.RGBA{}
	}
	b := o.src.Bounds()
	w, h := b.Dx(), b.Dy()
	var sx, sy int
	switch o.orientation {
	case 2:
		sx, sy = w-1-x, y
	case 3:
		sx, sy = w-1-x, h-1-y
	case 4:
		sx, sy = x, h-1-y
	case 5:
		sx, sy = y, x
	case 6:
		sx, sy = y, h-1-x
	case 7:
		sx, sy = w-1-y, h-1-x
	case 8:
		sx, sy = w-1-y, x
	default:
		sx, sy = x, y
	}
	return o.src.At(b.Min.X+sx, b.Min.Y+sy)
}
