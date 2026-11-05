package filters

import (
	"math"

	"github.com/soypat/pix"
)

var (
	_ pix.Filter = (*CLAHE)(nil)
)

// CLAHE equalizes the contrast of a [pix.ShapeGray8] image with Contrast Limited Adaptive
// Histogram Equalization, following OpenCV's 8-bit implementation step by step: images
// whose size is not a multiple of the tile grid are padded (reflect 101) for the histograms,
// each tile's histogram is clipped at max(clipLimit·tileArea/256, 1) with the excess spread
// evenly, and pixels blend the lookup tables of the four nearest tiles bilinearly in float32.
//
// For color images, convert to Lab and equalize the L channel.
type CLAHE struct {
	prepared
	clip   float64
	tiles  [2]pix.ControlInt // x, y
	clipC  pix.ControlInt    // clip limit as an integer, as tuned in OpenCV-based tools.
	luts   []uint8           // tilesX·tilesY tables of 256 entries.
	hist   [256]int32
	ext    []uint8 // Padded image when its size is not a multiple of the grid.
	xs     []claheX
	tw, th int // Tile size.
}

type claheX struct {
	x1, x2 int32
	xa     float32
}

// NewCLAHE returns a CLAHE filter with the given clip limit (<=0 disables clipping) and a
// tiles×tiles grid.
func NewCLAHE(clipLimit float64, tiles int) *CLAHE {
	f := &CLAHE{clip: clipLimit}
	f.clipC = pix.ControlInt{Name: "clip_limit", Description: "contrast limit", Value: int64(math.Round(clipLimit)), Min: 0, Max: 255, Step: 1,
		OnChange: func(v int64) error { f.clip = float64(v); return nil }}
	f.tiles[0] = pix.ControlInt{Name: "tiles_x", Description: "tile grid columns", Value: int64(tiles), Min: 1, Max: 256, Step: 1}
	f.tiles[1] = pix.ControlInt{Name: "tiles_y", Description: "tile grid rows", Value: int64(tiles), Min: 1, Max: 256, Step: 1}
	return f
}

// SetClipLimit sets the clip limit, which may be fractional.
func (f *CLAHE) SetClipLimit(clip float64) {
	f.clip = clip
	f.clipC.Value = int64(math.Round(clip))
}

// SetTiles sets the tile grid, which takes effect on the next Prepare.
func (f *CLAHE) SetTiles(tx, ty int) { f.tiles[0].Value, f.tiles[1].Value = int64(tx), int64(ty) }

// Controls implements [pix.Filter]: clip_limit, tiles_x and tiles_y.
func (f *CLAHE) Controls() []pix.Control { return []pix.Control{&f.clipC, &f.tiles[0], &f.tiles[1]} }

// ShapeIO implements [pix.Filter].
func (f *CLAHE) ShapeIO() (output, input pix.Shape) { return pix.ShapeGray8, pix.ShapeGray8 }

// Prepare implements [pix.Filter].
func (f *CLAHE) Prepare(in pix.Dims) (pix.Dims, error) {
	if err := in.Validate(); err != nil {
		return pix.Dims{}, err
	} else if in.Shape != pix.ShapeGray8 {
		return pix.Dims{}, errUnsupportedShape
	}
	tx, ty := int(f.tiles[0].Value), int(f.tiles[1].Value)
	if tx < 1 || ty < 1 {
		return pix.Dims{}, errBadParam
	}
	w, h := in.Width, in.Height
	if w%tx != 0 || h%ty != 0 {
		w += tx - in.Width%tx
		h += ty - in.Height%ty
		f.ext = grow(f.ext, w*h)
	}
	f.tw, f.th = w/tx, h/ty
	f.luts = grow(f.luts, tx*ty*256)
	f.xs = grow(f.xs, in.Width)
	invTW := 1 / float32(f.tw)
	for x := range f.xs {
		txf := float32(x)*invTW - 0.5
		x1 := int32(math.Floor(float64(txf)))
		f.xs[x] = claheX{x1: max(x1, 0), x2: min(x1+1, int32(tx-1)), xa: txf - float32(x1)}
	}
	f.in, f.out = in, pix.DenseDims(in.Width, in.Height, pix.ShapeGray8)
	return f.out, nil
}

// Process implements [pix.Filter]. dst may be src.
func (f *CLAHE) Process(dst, src *pix.Buffer) error {
	if err := f.checkInPlace(dst, src); err != nil {
		return err
	}
	tx, ty := int(f.tiles[0].Value), int(f.tiles[1].Value)
	if len(f.luts) != tx*ty*256 {
		return errNotPrepared // Tiles changed without Prepare.
	}
	// Source of histograms: the image, or its padded copy.
	w, h := f.in.Width, f.in.Height
	lutSrc := bufRows{b: src.Buffer(), stride: src.Dims().Stride, row: w}
	if f.tw*tx != w || f.th*ty != h {
		ew, eh := f.tw*tx, f.th*ty
		for y := 0; y < eh; y++ {
			s := src.Row(reflect101(y, h))
			e := f.ext[y*ew : (y+1)*ew]
			copy(e, s)
			for x := w; x < ew; x++ {
				e[x] = s[reflect101(x, w)]
			}
		}
		lutSrc = bufRows{b: f.ext, stride: ew, row: ew}
	}
	area := f.tw * f.th
	clipLimit := 0
	if f.clip > 0 {
		clipLimit = max(int(f.clip*float64(area)/256), 1)
	}
	lutScale := float32(255) / float32(area)
	for t := 0; t < tx*ty; t++ {
		f.tileLUT(lutSrc, t%tx, t/tx, clipLimit, lutScale, f.luts[t*256:(t+1)*256])
	}
	f.interpolate(dst, src, 0, h)
	return nil
}

func (f *CLAHE) tileLUT(src bufRows, tx, ty, clipLimit int, lutScale float32, lut []uint8) {
	hist := &f.hist
	*hist = [256]int32{}
	x0, y0 := tx*f.tw, ty*f.th
	for y := y0; y < y0+f.th; y++ {
		for _, v := range src.at(y)[x0 : x0+f.tw] {
			hist[v]++
		}
	}
	if clipLimit > 0 {
		clipped := int32(0)
		cl := int32(clipLimit)
		for i := range hist {
			if hist[i] > cl {
				clipped += hist[i] - cl
				hist[i] = cl
			}
		}
		batch := clipped / 256
		residual := clipped - batch*256
		for i := range hist {
			hist[i] += batch
		}
		if residual != 0 {
			step := max(256/residual, 1)
			for i := int32(0); i < 256 && residual > 0; i, residual = i+step, residual-1 {
				hist[i]++
			}
		}
	}
	sum := int32(0)
	for i := range hist {
		sum += hist[i]
		lut[i] = roundSat32(float32(sum) * lutScale)
	}
}

func (f *CLAHE) interpolate(dst, src *pix.Buffer, y0, y1 int) {
	tx, ty := int32(f.tiles[0].Value), int32(f.tiles[1].Value)
	invTH := 1 / float32(f.th)
	for y := y0; y < y1; y++ {
		tyf := float32(y)*invTH - 0.5
		t1 := int32(math.Floor(float64(tyf)))
		ya := tyf - float32(t1)
		t2 := min(t1+1, ty-1)
		t1 = max(t1, 0)
		lut1, lut2 := f.luts[t1*tx*256:], f.luts[t2*tx*256:]
		s, o := src.Row(y), dst.Row(y)
		for x, v := range s {
			c := f.xs[x]
			a, b := c.x1*256+int32(v), c.x2*256+int32(v)
			res := (float32(lut1[a])*(1-c.xa)+float32(lut1[b])*c.xa)*(1-ya) +
				(float32(lut2[a])*(1-c.xa)+float32(lut2[b])*c.xa)*ya
			o[x] = roundSat32(res)
		}
	}
}
