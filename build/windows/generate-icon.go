// generate-icon: writes build/windows/icon.ico as a 32-bit RGBA multi-size
// ICO (16/32/48/256) using PNG-encoded entries. Design: bright-blue square
// with a dark-navy "N" monogram (NetSwitcher). Legible at 16x16.
//
// Usage:  go run build/windows/generate-icon.go
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const (
	accentBlue = 0x5FB8FF // background
	darkNavy   = 0x14171F // the N
)

func rgba(c uint32) color.RGBA {
	return color.RGBA{byte(c >> 16), byte(c >> 8), byte(c), 0xFF}
}

func fillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if x >= 0 && x < img.Bounds().Dx() && y >= 0 && y < img.Bounds().Dy() {
				img.Set(x, y, c)
			}
		}
	}
}

// thickLine draws a line from (x0,y0) to (x1,y1) with the given thickness.
func thickLine(img *image.RGBA, x0, y0, x1, y1, t int, c color.RGBA) {
	dx := math.Abs(float64(x1 - x0))
	dy := math.Abs(float64(y1 - y0))
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy
	x, y := x0, y0
	h := t / 2
	for {
		for yy := y - h; yy <= y+h; yy++ {
			if x >= 0 && x < img.Bounds().Dx() && yy >= 0 && yy < img.Bounds().Dy() {
				img.Set(x, yy, c)
			}
		}
		if x == x1 && y == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x += sx
		}
		if e2 < dx {
			err += dx
			y += sy
		}
	}
}

// drawN draws the NetSwitcher "N" monogram: blue background + dark-navy N.
func drawN(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{rgba(accentBlue)}, image.Point{}, draw.Src)

	t := size / 5 // stroke thickness, scales with size
	if t < 2 {
		t = 2
	}
	margin := size / 4
	x0, x1 := margin, size-margin
	y0, y1 := margin, size-margin

	// Left vertical stroke.
	fillRect(img, x0, y0, x0+t, y1, rgba(darkNavy))
	// Right vertical stroke.
	fillRect(img, x1-t, y0, x1, y1, rgba(darkNavy))
	// Diagonal stroke (top-left → bottom-right).
	thickLine(img, x0, y0, x1, y1, t, rgba(darkNavy))
	return img
}

func main() {
	sizes := []int{16, 32, 48, 256}

	type entry struct {
		w, h  int
		bytes []byte
	}
	var entries []entry
	for _, s := range sizes {
		img := drawN(s)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			panic(err)
		}
		entries = append(entries, entry{w: s, h: s, bytes: buf.Bytes()})
	}

	// ICONDIR (6 bytes)
	dir := make([]byte, 6)
	binary.LittleEndian.PutUint16(dir[0:], 0) // reserved
	binary.LittleEndian.PutUint16(dir[2:], 1) // type = ICO
	binary.LittleEndian.PutUint16(dir[4:], uint16(len(entries)))

	headerSize := 6 + 16*len(entries)
	offset := headerSize
	var out bytes.Buffer
	out.Write(dir)
	for _, e := range entries {
		var en [16]byte
		if e.w >= 256 {
			en[0] = 0
		} else {
			en[0] = byte(e.w)
		}
		if e.h >= 256 {
			en[1] = 0
		} else {
			en[1] = byte(e.h)
		}
		binary.LittleEndian.PutUint16(en[4:], 1)  // planes
		binary.LittleEndian.PutUint16(en[6:], 32) // bit count
		binary.LittleEndian.PutUint32(en[8:], uint32(len(e.bytes)))
		binary.LittleEndian.PutUint32(en[12:], uint32(offset))
		out.Write(en[:])
		offset += len(e.bytes)
	}
	for _, e := range entries {
		out.Write(e.bytes)
	}

	outPath := filepath.Join("build", "windows", "icon.ico")
	if err := os.WriteFile(outPath, out.Bytes(), 0o644); err != nil {
		panic(err)
	}
	os.Stderr.WriteString("wrote " + outPath + " (N monogram, 16/32/48/256, 32-bit PNG ICO)\n")
}
