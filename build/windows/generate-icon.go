// generate-icon: writes build/windows/icon.ico as a 32-bit RGBA multi-size
// ICO (16/32/48) using PNG-encoded entries (modern, preserves full color +
// alpha; Windows 10+ supports PNG ICO entries). The design is a bright-blue
// double-arrow (⇄) on dark navy, drawn as geometric shapes so it's legible
// at 16x16.
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
	"os"
	"path/filepath"
)

func drawIcon(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	bg := color.RGBA{0x14, 0x17, 0x1F, 0xFF}
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	accent := color.RGBA{0x5F, 0xB8, 0xFF, 0xFF}
	t := size / 8 // line thickness, scales with size (min ~2 at 16px)
	if t < 2 {
		t = 2
	}
	mid := size / 2
	gap := size / 4 // vertical separation between the two arrows

	// Top arrow → (right), bottom arrow ← (left).
	drawArrow(img, size, mid-gap, +1, t, accent) // right-pointing
	drawArrow(img, size, mid+gap, -1, t, accent) // left-pointing
	return img
}

// drawArrow draws a horizontal arrow centered at y, pointing dir (+1 right,
// -1 left), with the given thickness in accent color.
func drawArrow(img *image.RGBA, size, y, dir, t int, c color.Color) {
	inset := size / 6
	x0 := inset
	x1 := size - inset
	headLen := size / 4
	// shaft (horizontal bar)
	for yy := y - t/2; yy < y+t/2+1; yy++ {
		if yy < 0 || yy >= size {
			continue
		}
		for xx := x0; xx <= x1; xx++ {
			img.Set(xx, yy, c)
		}
	}
	// head (triangle) — for right arrow, apex at x1; for left, apex at x0
	apexX := x1
	baseX := x1 - headLen
	halfH := size / 5
	if dir < 0 {
		apexX = x0
		baseX = x0 + headLen
	}
	for dx := 0; dx < headLen; dx++ {
		var xx int
		if dir > 0 {
			xx = baseX + dx
		} else {
			xx = baseX - dx
		}
		// triangle width grows as we move from apex to base
		h := halfH * dx / headLen
		for yy := y - h; yy <= y+h; yy++ {
			if xx >= 0 && xx < size && yy >= 0 && yy < size {
				img.Set(xx, yy, c)
			}
		}
	}
	_ = apexX
}

func main() {
	sizes := []int{16, 32, 48}
	type entry struct {
		w, h  int
		bytes []byte
	}
	var entries []entry
	for _, s := range sizes {
		img := drawIcon(s)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			panic(err)
		}
		entries = append(entries, entry{w: s, h: s, bytes: buf.Bytes()})
	}

	// ICONDIR (6 bytes): reserved=0, type=1 (ICO), count=N
	dir := make([]byte, 6)
	binary.LittleEndian.PutUint16(dir[0:], 0)
	binary.LittleEndian.PutUint16(dir[2:], 1)
	binary.LittleEndian.PutUint16(dir[4:], uint16(len(entries)))

	// ICONDIRENTRY (16 bytes each)
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
		en[2] = 0                                 // colors (0 for PNG/32-bit)
		en[3] = 0                                 // reserved
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
	os.Stderr.WriteString("wrote " + outPath + " (" + out.String()[:0] + "16/32/48, 32-bit PNG ICO)\n")
}
