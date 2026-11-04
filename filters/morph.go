package filters

import (
	"image"

	"github.com/soypat/pix"
)

var (
	_ pix.Filter = (*Morphology)(nil)
	_ pix.Viewer = (*Morphology)(nil)
)

// MorphOp is a morphological operation of [Morphology].
type MorphOp uint8

const (
	Erode  MorphOp = iota + 1 // erode
	Dilate                    // dilate
	Open                      // open: erode then dilate
	Close                     // close: dilate then erode
)

// Morphology applies a morphological operation with a square kernel of odd size (ksize|1) to
// every byte channel, like OpenCV's erode, dilate and morphologyEx with a rectangular
// structuring element. Pixels outside the image never affect the result (OpenCV's default
// border value). Iterations repeat erosion and dilation: n iterations of size k equal one of
// size n(k-1)+1, and opening and closing erode and dilate n times each.
//
// It runs the van Herk/Gil-Werman algorithm, three comparisons per byte per pass whatever the
// kernel size. It accepts [pix.ShapeGray8] and [pix.ShapeRGBA8888]; an effective size of 1 is
// the identity ([pix.Viewer]).
type Morphology struct {
	prepared
	op    MorphOp
	ksize pix.ControlInt
	iter  pix.ControlInt
	tmp   []uint8 // Image after the horizontal pass, or after the first op of open/close.
	tmp2  []uint8
	g, h  []uint8 // van Herk prefix and suffix scans.
	line  []uint8 // Padded line.
}

// NewMorphology returns a morphology filter.
func NewMorphology(op MorphOp, ksize, iterations int) *Morphology {
	f := &Morphology{op: op}
	f.ksize = pix.ControlInt{Name: "ksize", Description: "kernel size, made odd", Value: int64(ksize), Min: 1, Max: 255, Step: 2}
	f.iter = pix.ControlInt{Name: "iter", Description: "iterations", Value: int64(iterations), Min: 1, Max: 32, Step: 1}
	return f
}

// Op returns the operation.
func (f *Morphology) Op() MorphOp { return f.op }

// EffectiveSize returns the kernel size of a single pass equivalent to the iterations.
func (f *Morphology) EffectiveSize() int {
	k := max(1, int(f.ksize.Value)|1)
	return int(f.iter.Value)*(k-1) + 1
}

// Controls implements [pix.Filter]: ksize and iter.
func (f *Morphology) Controls() []pix.Control { return []pix.Control{&f.ksize, &f.iter} }

// ShapeIO implements [pix.Filter].
func (f *Morphology) ShapeIO() (output, input pix.Shape) { return f.in.Shape, f.in.Shape }

// Prepare implements [pix.Filter].
func (f *Morphology) Prepare(in pix.Dims) (pix.Dims, error) {
	if err := in.Validate(); err != nil {
		return pix.Dims{}, err
	} else if !isGrayOrRGBA(in.Shape) || f.op < Erode || f.op > Close {
		return pix.Dims{}, errUnsupportedShape
	}
	f.in, f.out = in, pix.DenseDims(in.Width, in.Height, in.Shape)
	f.scratch()
	return f.out, nil
}

// scratch grows scratch memory for the prepared dimensions and current kernel size.
func (f *Morphology) scratch() {
	k := f.EffectiveSize()
	row := f.in.SizeRow()
	n := f.in.Height * row
	f.tmp = grow(f.tmp, n)
	if f.op == Open || f.op == Close {
		f.tmp2 = grow(f.tmp2, n)
	}
	lines := max(f.in.Width, f.in.Height) + k
	f.line = grow(f.line, lines)
	f.g = grow(f.g, max(lines, (f.in.Height+k)*row))
	f.h = grow(f.h, max(lines, (f.in.Height+k)*row))
}

// ViewOf implements [pix.Viewer].
func (f *Morphology) ViewOf(in pix.Dims) (image.Rectangle, bool) {
	return image.Rect(0, 0, in.Width, in.Height), f.EffectiveSize() == 1
}

// Process implements [pix.Filter].
func (f *Morphology) Process(dst, src *pix.Buffer) error {
	if err := f.check(dst, src); err != nil {
		return err
	}
	f.scratch() // Kernel size may have changed since Prepare; no-op when it did not grow.
	k := f.EffectiveSize()
	row := f.in.SizeRow()
	srcRows := bufRows{b: src.Buffer(), stride: src.Dims().Stride, row: row}
	dstRows := bufRows{b: dst.Buffer(), stride: dst.Dims().Stride, row: row}
	switch f.op {
	case Erode, Dilate:
		f.pass(dstRows, srcRows, f.op == Dilate, k)
	default:
		mid := bufRows{b: f.tmp2, stride: row, row: row}
		first := f.op == Close // Close dilates first, open erodes first.
		f.pass(mid, srcRows, first, k)
		f.pass(dstRows, mid, !first, k)
	}
	return nil
}

// bufRows addresses rows of an image in a byte slice.
type bufRows struct {
	b           []uint8
	stride, row int
}

func (r bufRows) at(y int) []uint8 { return r.b[y*r.stride : y*r.stride+r.row] }

// pass erodes (or dilates when dilate) src into dst with a k×k kernel: horizontally into tmp,
// then vertically into dst.
func (f *Morphology) pass(dst, src bufRows, dilate bool, k int) {
	w, h := f.in.Width, f.in.Height
	bpp := f.in.Shape.BytesPerPixel()
	row := f.in.SizeRow()
	tmp := bufRows{b: f.tmp, stride: row, row: row}
	var neutral uint8 = 255 // Erosion ignores pixels outside the image: they are the maximum.
	if dilate {
		neutral = 0
	}
	r := k / 2
	m := w + 2*r
	line, g, hh := f.line[:m], f.g[:m], f.h[:m]
	fill(line[:r], neutral)
	fill(line[r+w:], neutral)
	mid := line[r : r+w]
	for y := 0; y < h; y++ {
		s, o := src.at(y), tmp.at(y)
		for c := 0; c < bpp; c++ {
			if bpp == 1 {
				copy(mid, s)
			} else {
				for x := range mid {
					mid[x] = s[x*bpp+c]
				}
			}
			if dilate {
				scanMax(g, hh, line, k)
				for x := 0; x < w; x++ {
					o[x*bpp+c] = max(hh[x], g[x+k-1])
				}
			} else {
				scanMin(g, hh, line, k)
				for x := 0; x < w; x++ {
					o[x*bpp+c] = min(hh[x], g[x+k-1])
				}
			}
		}
	}
	// Vertical pass over whole rows: g and hh hold (h+2r) rows of prefix and suffix scans.
	mh := h + 2*r
	rowAt := func(i int) []uint8 {
		y := i - r
		if y < 0 || y >= h {
			return nil // Neutral row.
		}
		return tmp.at(y)
	}
	gr := bufRows{b: f.g, stride: row, row: row}
	hr := bufRows{b: f.h, stride: row, row: row}
	for i := 0; i < mh; i++ {
		gi, x := gr.at(i), rowAt(i)
		switch {
		case i%k == 0 && x == nil:
			fill(gi, neutral)
		case i%k == 0:
			copy(gi, x)
		case x == nil:
			copy(gi, gr.at(i-1))
		default:
			combine(gi, gr.at(i-1), x, dilate)
		}
	}
	for i := mh - 1; i >= 0; i-- {
		hi, x := hr.at(i), rowAt(i)
		last := i%k == k-1 || i == mh-1
		switch {
		case last && x == nil:
			fill(hi, neutral)
		case last:
			copy(hi, x)
		case x == nil:
			copy(hi, hr.at(i+1))
		default:
			combine(hi, hr.at(i+1), x, dilate)
		}
	}
	for y := 0; y < h; y++ {
		combine(dst.at(y), hr.at(y), gr.at(y+k-1), dilate)
	}
}

// scanMax computes van Herk's block prefix (g) and suffix (h) maximum scans of x, blocks of k.
func scanMax(g, h, x []uint8, k int) {
	m := len(x)
	g, h = g[:m], h[:m]
	for b := 0; b < m; b += k {
		e := min(b+k, m)
		gb, hb, xb := g[b:e], h[b:e], x[b:e]
		gb[0] = xb[0]
		for i := 1; i < len(xb); i++ {
			gb[i] = max(gb[i-1], xb[i])
		}
		hb[len(xb)-1] = xb[len(xb)-1]
		for i := len(xb) - 2; i >= 0; i-- {
			hb[i] = max(hb[i+1], xb[i])
		}
	}
}

// scanMin is scanMax for minima.
func scanMin(g, h, x []uint8, k int) {
	m := len(x)
	g, h = g[:m], h[:m]
	for b := 0; b < m; b += k {
		e := min(b+k, m)
		gb, hb, xb := g[b:e], h[b:e], x[b:e]
		gb[0] = xb[0]
		for i := 1; i < len(xb); i++ {
			gb[i] = min(gb[i-1], xb[i])
		}
		hb[len(xb)-1] = xb[len(xb)-1]
		for i := len(xb) - 2; i >= 0; i-- {
			hb[i] = min(hb[i+1], xb[i])
		}
	}
}

func combine(dst, a, b []uint8, dilate bool) {
	if dilate {
		for i := range dst {
			dst[i] = max(a[i], b[i])
		}
	} else {
		for i := range dst {
			dst[i] = min(a[i], b[i])
		}
	}
}

func fill(dst []uint8, v uint8) {
	for i := range dst {
		dst[i] = v
	}
}
