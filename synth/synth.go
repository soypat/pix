// Package synth generates images procedurally for tests and benchmarks: fills, rectangles,
// discs, rings, lines, gradients and seeded noise drawn into [pix.Buffer]s of shapes
// [pix.ShapeGray8] and 4-byte-per-pixel shapes such as [pix.ShapeRGBA8888].
//
// Behavioural tests build their inputs with synth instead of storing image files, and
// derive expected outputs from the shapes they drew.
package synth

import (
	"image"
	"math"
	"math/rand/v2"

	"github.com/soypat/pix"
)

// New returns a w×h buffer of shape sh, rows padded to a multiple of align bytes (align<=1
// for dense rows), filled with zeros.
func New(w, h int, sh pix.Shape, align int) *pix.Buffer {
	d := pix.DenseDims(w, h, sh)
	if align > 1 {
		d.Stride = (d.Stride + align - 1) / align * align
	}
	data := make([]byte, d.Stride*h)
	b, err := pix.MakeBuffer(d, data)
	if err != nil {
		panic(err)
	}
	return &b
}

// Gray returns a dense w×h gray image filled with v.
func Gray(w, h int, v uint8) *pix.Buffer {
	b := New(w, h, pix.ShapeGray8, 1)
	Fill(b, Color{v, v, v, 255})
	return b
}

// RGBA returns a dense w×h RGBA image filled with c.
func RGBA(w, h int, c Color) *pix.Buffer {
	b := New(w, h, pix.ShapeRGBA8888, 1)
	Fill(b, c)
	return b
}

// Color is a pixel value: R, G, B and A for 4-byte shapes, or Color[0] for gray.
type Color [4]uint8

// Bounds returns the image rectangle of b.
func Bounds(b *pix.Buffer) image.Rectangle {
	d := b.Dims()
	return image.Rect(0, 0, d.Width, d.Height)
}

// Set sets pixel (x, y) of b to c if it lies within b.
func Set(b *pix.Buffer, x, y int, c Color) {
	d := b.Dims()
	if x < 0 || y < 0 || x >= d.Width || y >= d.Height {
		return
	}
	bpp := d.Shape.BytesPerPixel()
	off := y*d.Stride + x*bpp
	copy(b.Buffer()[off:off+bpp], c[:bpp])
}

// At returns pixel (x, y) of b.
func At(b *pix.Buffer, x, y int) Color {
	d := b.Dims()
	bpp := d.Shape.BytesPerPixel()
	off := y*d.Stride + x*bpp
	var c Color
	copy(c[:bpp], b.Buffer()[off:off+bpp])
	return c
}

// Fill sets every pixel of b to c.
func Fill(b *pix.Buffer, c Color) { FillRect(b, Bounds(b), c) }

// FillRect sets the pixels of r within b to c.
func FillRect(b *pix.Buffer, r image.Rectangle, c Color) {
	r = r.Intersect(Bounds(b))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			Set(b, x, y, c)
		}
	}
}

// FillDisc sets the pixels whose centers lie within radius r of (cx, cy) to c.
func FillDisc(b *pix.Buffer, cx, cy, r float64, c Color) {
	FillRing(b, cx, cy, -1, r, c)
}

// FillRing sets the pixels whose centers are at a distance in (r0, r1] from (cx, cy) to c.
func FillRing(b *pix.Buffer, cx, cy, r0, r1 float64, c Color) {
	y0, y1 := int(math.Floor(cy-r1)), int(math.Ceil(cy+r1))
	x0, x1 := int(math.Floor(cx-r1)), int(math.Ceil(cx+r1))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			d := math.Sqrt(dx*dx + dy*dy)
			if d <= r1 && d > r0 {
				Set(b, x, y, c)
			}
		}
	}
}

// Line draws a 1 pixel thick 8-connected line from (x0, y0) to (x1, y1) inclusive.
func Line(b *pix.Buffer, x0, y0, x1, y1 int, c Color) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := sign(x1-x0), sign(y1-y0)
	e := dx + dy
	for {
		Set(b, x0, y0, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * e
		if e2 >= dy {
			e += dy
			x0 += sx
		}
		if e2 <= dx {
			e += dx
			y0 += sy
		}
	}
}

// GradientX fills b with a horizontal ramp from v0 at the left to v1 at the right in every
// color channel, alpha 255.
func GradientX(b *pix.Buffer, v0, v1 uint8) {
	d := b.Dims()
	for x := 0; x < d.Width; x++ {
		t := 0.0
		if d.Width > 1 {
			t = float64(x) / float64(d.Width-1)
		}
		v := uint8(math.Round(float64(v0) + t*(float64(v1)-float64(v0))))
		FillRect(b, image.Rect(x, 0, x+1, d.Height), Color{v, v, v, 255})
	}
}

// Noise fills b with uniformly random bytes from a PCG generator seeded with seed. Alpha of
// 4-byte shapes is set to 255.
func Noise(b *pix.Buffer, seed uint64) {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	d := b.Dims()
	bpp := d.Shape.BytesPerPixel()
	for y := 0; y < d.Height; y++ {
		row := b.Row(y)
		for i := range row {
			row[i] = uint8(rng.Uint32())
			if bpp == 4 && i%4 == 3 {
				row[i] = 255
			}
		}
	}
}

// Binary fills b with random 0/255 pixels, each 255 with probability p.
func Binary(b *pix.Buffer, seed uint64, p float64) {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	d := b.Dims()
	for y := 0; y < d.Height; y++ {
		for x := 0; x < d.Width; x++ {
			v := uint8(0)
			if rng.Float64() < p {
				v = 255
			}
			Set(b, x, y, Color{v, v, v, 255})
		}
	}
}

// Equal reports whether a and b have the same size, shape and pixel bytes, ignoring padding.
func Equal(a, b *pix.Buffer) bool {
	da, db := a.Dims(), b.Dims()
	if da.Width != db.Width || da.Height != db.Height || da.Shape != db.Shape {
		return false
	}
	for y := 0; y < da.Height; y++ {
		ra, rb := a.Row(y), b.Row(y)
		for i := range ra {
			if ra[i] != rb[i] {
				return false
			}
		}
	}
	return true
}

// MaxDiff returns the largest absolute difference between bytes of a and b, which must have
// equal dimensions, ignoring padding.
func MaxDiff(a, b *pix.Buffer) int {
	da := a.Dims()
	m := 0
	for y := 0; y < da.Height; y++ {
		ra, rb := a.Row(y), b.Row(y)
		for i := range ra {
			m = max(m, abs(int(ra[i])-int(rb[i])))
		}
	}
	return m
}

// Count returns the number of pixels of a gray image that are nonzero.
func Count(b *pix.Buffer) int {
	n := 0
	d := b.Dims()
	for y := 0; y < d.Height; y++ {
		for _, v := range b.Row(y) {
			if v != 0 {
				n++
			}
		}
	}
	return n
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sign(v int) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	}
	return 0
}
