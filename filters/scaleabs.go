package filters

import (
	"image"
	"math"

	"github.com/soypat/pix"
)

var (
	_ pix.Filter = (*ScaleAbs)(nil)
	_ pix.Viewer = (*ScaleAbs)(nil)
)

// ScaleAbs computes |v·alpha + beta| for every color channel, rounded half to even and
// saturated to 0..255: OpenCV's convertScaleAbs. Taking the absolute value before saturating
// folds negative results back up, which brightness/contrast pipelines tuned with OpenCV
// may rely on. Alpha channels pass through.
//
// It accepts [pix.ShapeGray8] and [pix.ShapeRGBA8888], works in place, and implements
// [pix.Viewer] for the identity alpha=1, beta=0.
type ScaleAbs struct {
	prepared
	alpha, beta float64
	lut         [256]uint8
}

// NewScaleAbs returns a ScaleAbs filter.
func NewScaleAbs(alpha, beta float64) *ScaleAbs {
	f := &ScaleAbs{}
	f.Set(alpha, beta)
	return f
}

// Set sets the scale and offset.
func (f *ScaleAbs) Set(alpha, beta float64) {
	f.alpha, f.beta = alpha, beta
	// OpenCV computes in float32.
	a, b := float32(alpha), float32(beta)
	for i := range f.lut {
		v := float32(i)*a + b
		f.lut[i] = roundSat32(float32(math.Abs(float64(v))))
	}
}

// Params returns the scale and offset.
func (f *ScaleAbs) Params() (alpha, beta float64) { return f.alpha, f.beta }

// Controls implements [pix.Filter]. ScaleAbs has float parameters, set with [ScaleAbs.Set].
func (f *ScaleAbs) Controls() []pix.Control { return nil }

// ShapeIO implements [pix.Filter].
func (f *ScaleAbs) ShapeIO() (output, input pix.Shape) { return f.in.Shape, f.in.Shape }

// Prepare implements [pix.Filter].
func (f *ScaleAbs) Prepare(in pix.Dims) (pix.Dims, error) {
	if err := in.Validate(); err != nil {
		return pix.Dims{}, err
	} else if !isGrayOrRGBA(in.Shape) {
		return pix.Dims{}, errUnsupportedShape
	}
	f.in, f.out = in, pix.DenseDims(in.Width, in.Height, in.Shape)
	return f.out, nil
}

// ViewOf implements [pix.Viewer].
func (f *ScaleAbs) ViewOf(in pix.Dims) (image.Rectangle, bool) {
	return image.Rect(0, 0, in.Width, in.Height), f.alpha == 1 && f.beta == 0
}

// Process implements [pix.Filter]. dst may be src.
func (f *ScaleAbs) Process(dst, src *pix.Buffer) error {
	if err := f.check(dst, src); err != nil {
		return err
	}
	applyLUT(dst, src, &f.lut, 3)
	return nil
}
