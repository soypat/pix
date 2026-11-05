package filters

import (
	"image"
	"math"

	"github.com/soypat/pix"
)

var (
	_ pix.Filter = (*GaussianBlur)(nil)
	_ pix.Viewer = (*GaussianBlur)(nil)
)

// GaussianBlur blurs with a square Gaussian kernel, separably, in fixed point: kernel
// weights are quantized to 1/256 like OpenCV's bit-exact 8-bit GaussianBlur, rows are
// filtered into 16-bit intermediates and columns rounded back to bytes. Borders reflect
// without repeating the edge (OpenCV's default BORDER_REFLECT_101).
//
// Even kernel sizes are made odd (ksize|1, 16 → 17). With sigma 0, sigma is derived from the
// size as OpenCV does, and sizes 3, 5 and 7 use OpenCV's fixed binomial kernels. It accepts
// [pix.ShapeGray8] and [pix.ShapeRGBA8888]; size 1 is the identity ([pix.Viewer]).
type GaussianBlur struct {
	prepared
	ksize  pix.ControlInt
	sigma  float64
	kernel []uint16 // Q8, sums to 256.
	tmp    []uint16 // Row-filtered image, Q8.
	acc    []uint32 // Column accumulator row.
	pad    []uint8  // Padded source row.
}

// NewGaussianBlur returns a Gaussian blur of size ksize (made odd) and standard deviation
// sigma, or derived from ksize when sigma <= 0.
func NewGaussianBlur(ksize int, sigma float64) *GaussianBlur {
	f := &GaussianBlur{sigma: sigma}
	f.ksize = pix.ControlInt{Name: "ksize", Description: "kernel size, made odd", Value: int64(ksize), Min: 1, Max: 255, Step: 2,
		OnChange: func(v int64) error { f.buildKernel(int(v)); return nil }}
	f.buildKernel(ksize)
	return f
}

// KernelSize returns the effective (odd) kernel size.
func (f *GaussianBlur) KernelSize() int { return len(f.kernel) }

// Kernel returns the Q8 fixed point kernel weights, which sum to 256.
func (f *GaussianBlur) Kernel() []uint16 { return f.kernel }

func (f *GaussianBlur) buildKernel(ksize int) {
	n := max(1, ksize|1)
	w := make([]float64, n)
	switch {
	case f.sigma <= 0 && n == 1:
		w[0] = 1
	case f.sigma <= 0 && n == 3:
		copy(w, []float64{0.25, 0.5, 0.25})
	case f.sigma <= 0 && n == 5:
		copy(w, []float64{0.0625, 0.25, 0.375, 0.25, 0.0625})
	case f.sigma <= 0 && n == 7:
		copy(w, []float64{0.03125, 0.109375, 0.21875, 0.28125, 0.21875, 0.109375, 0.03125})
	default:
		sigma := f.sigma
		if sigma <= 0 {
			sigma = 0.3*(float64(n-1)*0.5-1) + 0.8
		}
		sum := 0.0
		for i := range w {
			d := float64(i) - float64(n-1)/2
			w[i] = math.Exp(-d * d / (2 * sigma * sigma))
			sum += w[i]
		}
		for i := range w {
			w[i] /= sum
		}
	}
	k := make([]uint16, n)
	total := 0
	for i, v := range w {
		k[i] = uint16(math.Round(v * 256))
		total += int(k[i])
	}
	k[n/2] = uint16(int(k[n/2]) + 256 - total) // Exact unit gain: flat images stay flat.
	f.kernel = k
}

// Controls implements [pix.Filter]: ksize.
func (f *GaussianBlur) Controls() []pix.Control { return []pix.Control{&f.ksize} }

// ShapeIO implements [pix.Filter].
func (f *GaussianBlur) ShapeIO() (output, input pix.Shape) { return f.in.Shape, f.in.Shape }

// Prepare implements [pix.Filter].
func (f *GaussianBlur) Prepare(in pix.Dims) (pix.Dims, error) {
	if err := in.Validate(); err != nil {
		return pix.Dims{}, err
	} else if !isGrayOrRGBA(in.Shape) {
		return pix.Dims{}, errUnsupportedShape
	}
	row := in.SizeRow()
	r := len(f.kernel) / 2
	f.tmp = grow(f.tmp, row*in.Height)
	f.acc = grow(f.acc, row)
	f.pad = grow(f.pad, row+2*r*in.Shape.BytesPerPixel())
	f.in, f.out = in, pix.DenseDims(in.Width, in.Height, in.Shape)
	return f.out, nil
}

// ViewOf implements [pix.Viewer].
func (f *GaussianBlur) ViewOf(in pix.Dims) (image.Rectangle, bool) {
	return image.Rect(0, 0, in.Width, in.Height), len(f.kernel) == 1
}

// Process implements [pix.Filter]. dst may be src.
func (f *GaussianBlur) Process(dst, src *pix.Buffer) error {
	if r, ok := f.ViewOf(f.in); ok && f.viewing(dst, src, r) {
		return nil
	}
	if err := f.checkInPlace(dst, src); err != nil {
		return err
	}
	if len(f.kernel) == 1 {
		for y := 0; y < f.in.Height; y++ {
			copy(dst.Row(y), src.Row(y))
		}
		return nil
	}
	// Prepare sized scratch for the kernel at the time; the kernel may have grown since.
	if r := len(f.kernel) / 2; len(f.pad) < f.in.SizeRow()+2*r*f.in.Shape.BytesPerPixel() {
		f.pad = grow(f.pad, f.in.SizeRow()+2*r*f.in.Shape.BytesPerPixel())
	}
	f.rows(src, 0, f.in.Height)
	f.cols(dst, 0, f.in.Height)
	return nil
}

// rows filters source rows y0..y1 horizontally into tmp.
func (f *GaussianBlur) rows(src *pix.Buffer, y0, y1 int) {
	w, bpp := f.in.Width, f.in.Shape.BytesPerPixel()
	k := f.kernel
	r := len(k) / 2
	rowBytes := w * bpp
	pad := f.pad[:rowBytes+2*r*bpp]
	for y := y0; y < y1; y++ {
		s := src.Row(y)
		for x := -r; x < w+r; x++ {
			sx := reflect101(x, w)
			copy(pad[(x+r)*bpp:(x+r+1)*bpp], s[sx*bpp:(sx+1)*bpp])
		}
		t := f.tmp[y*rowBytes : (y+1)*rowBytes]
		// The kernel is symmetric: pair taps j and n-1-j, halving multiplications.
		kc := uint32(k[r])
		span := 2 * r * bpp
		for i := range t {
			win := pad[i : i+span+1]
			sum := kc * uint32(win[r*bpp])
			for j := 0; j < r; j++ {
				sum += uint32(k[j]) * (uint32(win[j*bpp]) + uint32(win[span-j*bpp]))
			}
			t[i] = uint16(sum) // At most 255·256.
		}
	}
}

// cols filters tmp vertically into dst rows y0..y1.
func (f *GaussianBlur) cols(dst *pix.Buffer, y0, y1 int) {
	h := f.in.Height
	rowBytes := f.in.SizeRow()
	k := f.kernel
	r := len(k) / 2
	acc := f.acc[:rowBytes]
	for y := y0; y < y1; y++ {
		// Center row initializes the accumulator; symmetric rows are added in pairs.
		kc := uint32(k[r])
		tc := f.tmp[y*rowBytes : (y+1)*rowBytes]
		for i, v := range tc {
			acc[i] = kc * uint32(v)
		}
		for j := 0; j < r; j++ {
			kw := uint32(k[j])
			ya, yb := reflect101(y+j-r, h), reflect101(y+r-j, h)
			ta := f.tmp[ya*rowBytes : (ya+1)*rowBytes]
			tb := f.tmp[yb*rowBytes : (yb+1)*rowBytes]
			tb = tb[:len(ta)]
			for i, v := range ta {
				acc[i] += kw * (uint32(v) + uint32(tb[i]))
			}
		}
		o := dst.Row(y)
		o = o[:len(acc)]
		for i, v := range acc {
			o[i] = uint8((v + 1<<15) >> 16)
		}
	}
}
